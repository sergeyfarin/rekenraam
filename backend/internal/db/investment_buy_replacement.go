package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

var ErrInvestmentCorrectionDependency = errors.New("investment correction cannot satisfy a dependent operation")

// BuyOperationRecord pins a posted acquisition — a buy or a reinvested
// dividend (T-115) — and its one immutable opening lot. A correction must
// recheck it inside the write transaction.
type BuyOperationRecord struct {
	OperationID          int64
	OperationKind        string
	TransactionID        int64
	TransactionVersionID int64
	CurrentVersionID     int64
	LotID                int64
	EventDate            string
	AccountID            int64
	CommodityID          int64
	CostCommodityID      int64
	AlreadyCorrected     bool
	Imported             bool
	ImportedLineage      bool
	SourceIdentityID     int64
	SourceEffectSeq      int64
}

func (r *InvestmentRepository) BuyOperationByTransactionID(ctx context.Context, bookID, transactionID int64) (BuyOperationRecord, error) {
	return buyOperationByTransactionIDQuery(ctx, r.database, bookID, transactionID)
}

func buyOperationByTransactionIDQuery(ctx context.Context, reader saleOperationReader, bookID, transactionID int64) (BuyOperationRecord, error) {
	var record BuyOperationRecord
	var corrected, imported int
	err := reader.QueryRowContext(ctx, `SELECT o.id, o.operation_kind, linked_version.transaction_id, link.transaction_version_id,
		current.id, f.id, o.event_date, f.account_id, f.commodity_id, f.cost_commodity_id,
		EXISTS(SELECT 1 FROM investment_operations successor WHERE successor.correction_of_operation_id = o.id),
		(audit.origin_type = 'import' OR EXISTS(SELECT 1 FROM import_commit_identity_effects effect
			WHERE effect.operation_id = o.id)),
		COALESCE((SELECT effect.identity_id FROM import_commit_identity_effects effect
			WHERE effect.operation_id = o.id), 0),
		COALESCE((SELECT effect.effect_seq FROM import_commit_identity_effects effect
			WHERE effect.operation_id = o.id), 0)
		FROM investment_operations o
		JOIN audit_events audit ON audit.id = o.created_audit_event_id
		JOIN investment_lots f ON f.operation_id = o.id AND f.position_side = 'long'
		JOIN investment_operation_journal_links link ON link.operation_id = o.id AND link.book_id = o.book_id AND link.role = 'primary'
		JOIN transaction_versions linked_version ON linked_version.id = link.transaction_version_id
		JOIN current_transaction_versions current ON current.transaction_id = linked_version.transaction_id
		WHERE o.book_id = ? AND linked_version.transaction_id = ?
		AND o.operation_kind IN ('buy', 'reinvested_dividend')
		AND (SELECT count(*) FROM investment_lots one WHERE one.operation_id = o.id) = 1`,
		bookID, transactionID).Scan(&record.OperationID, &record.OperationKind, &record.TransactionID,
		&record.TransactionVersionID, &record.CurrentVersionID, &record.LotID,
		&record.EventDate, &record.AccountID, &record.CommodityID, &record.CostCommodityID,
		&corrected, &imported, &record.SourceIdentityID, &record.SourceEffectSeq)
	if errors.Is(err, sql.ErrNoRows) {
		return BuyOperationRecord{}, ErrNotFound
	}
	if err != nil {
		return BuyOperationRecord{}, fmt.Errorf("read investment buy operation: %w", err)
	}
	record.AlreadyCorrected = corrected != 0
	record.Imported = imported != 0
	record.ImportedLineage, err = investmentOperationHasImportedLineageQuery(ctx, reader, bookID, record.OperationID)
	if err != nil {
		return BuyOperationRecord{}, err
	}
	return record, nil
}

func checkBuyOperationForCorrectionTx(ctx context.Context, tx *sql.Tx, bookID int64, expected BuyOperationRecord) (BuyOperationRecord, error) {
	current, err := buyOperationByTransactionIDQuery(ctx, tx, bookID, expected.TransactionID)
	if err != nil {
		return BuyOperationRecord{}, err
	}
	if current.AlreadyCorrected {
		return BuyOperationRecord{}, ErrInvestmentOperationAlreadyCorrected
	}
	// An imported fill is correctable only when its committed source row still
	// identifies this exact operation. The immutable identity continues to
	// dedupe a retry, while correction_of_operation_id links the replacement.
	if current.ImportedLineage {
		linked, err := investmentOperationHasCommittedSourceQuery(ctx, tx, bookID, current.OperationID)
		if err != nil {
			return BuyOperationRecord{}, err
		}
		if !linked {
			return BuyOperationRecord{}, ErrInvestmentImportedCorrection
		}
	}
	if current != expected {
		return BuyOperationRecord{}, ErrInvestmentSaleChanged
	}
	if err := checkInvestmentSourceJournalTx(ctx, tx, bookID, current.TransactionID,
		current.EventDate, current.TransactionVersionID, current.CurrentVersionID); err != nil {
		return BuyOperationRecord{}, err
	}
	return current, nil
}

// openingLotEventKind is the lot event an acquisition operation records; a
// replacement must open its lot with the same kind as the original.
func openingLotEventKind(operationKind string) string {
	if operationKind == "reinvested_dividend" {
		return "reinvested_dividend"
	}
	return "acquisition"
}

type BuyReplacementRecord struct {
	Inverse     TransactionRecord
	Replacement TransactionRecord
	Lot         InvestmentLotRecord
}

// ReplaceBuy admits an old acquisition through replay only after the
// corrected opening and every dependent disposal are proven possible. The
// original journal and lot facts stay immutable; all new effects commit under
// one audit event or roll back together.
func (r *InvestmentRepository) ReplaceBuy(ctx context.Context, expected BuyOperationRecord,
	inverseParams, replacementParams CreateTransactionParams, lotParams CreateInvestmentLotParams,
) (BuyReplacementRecord, error) {
	return r.ReplaceBuyWithPostWrite(ctx, expected, inverseParams, replacementParams, lotParams, nil)
}

// ReplaceBuyWithPostWrite lets a source correction record its accepted staged
// row in the same transaction as the inverse, replacement, and lot replay.
func (r *InvestmentRepository) ReplaceBuyWithPostWrite(ctx context.Context, expected BuyOperationRecord,
	inverseParams, replacementParams CreateTransactionParams, lotParams CreateInvestmentLotParams,
	postWrite func(*sql.Tx, int64, int64) error,
) (BuyReplacementRecord, error) {
	return r.replaceBuy(ctx, expected, inverseParams, replacementParams, lotParams, postWrite, false)
}

// SimulateBuyReplacement proves the same opening, ordering, specific-lot
// lineage, dependent disposals and transfer guards as the write. All journals,
// audit, lot/revision, price and checkpoint effects are rolled back; source
// acceptance is never invoked and no temporary IDs escape.
func (r *InvestmentRepository) SimulateBuyReplacement(ctx context.Context, expected BuyOperationRecord,
	inverseParams, replacementParams CreateTransactionParams, lotParams CreateInvestmentLotParams,
) (SimulatedInvestmentWrite, error) {
	record, err := r.replaceBuy(ctx, expected, inverseParams, replacementParams, lotParams, nil, true)
	if err != nil {
		return SimulatedInvestmentWrite{}, err
	}
	return simulatedInvestmentWrite(record.Inverse, record.Replacement), nil
}

func (r *InvestmentRepository) replaceBuy(ctx context.Context, expected BuyOperationRecord,
	inverseParams, replacementParams CreateTransactionParams, lotParams CreateInvestmentLotParams,
	postWrite func(*sql.Tx, int64, int64) error, preview bool,
) (BuyReplacementRecord, error) {
	if expected.OperationID <= 0 || expected.LotID <= 0 ||
		(expected.OperationKind != "buy" && expected.OperationKind != "reinvested_dividend") ||
		lotParams.EventKind != openingLotEventKind(expected.OperationKind) ||
		inverseParams.BookID <= 0 || inverseParams.BookID != replacementParams.BookID ||
		inverseParams.BookID != lotParams.BookID ||
		inverseParams.ActorUserID != replacementParams.ActorUserID ||
		inverseParams.ActorUserID != lotParams.CreatedByUserID ||
		inverseParams.Spec.InvestmentOperationKind != "" ||
		inverseParams.Spec.TransactionKind != "investment" || inverseParams.Spec.Status != "posted" ||
		replacementParams.Spec.InvestmentOperationKind != expected.OperationKind ||
		replacementParams.Spec.TransactionKind != "investment" || replacementParams.Spec.Status != "posted" ||
		inverseParams.Spec.TransactionDate != expected.EventDate ||
		lotParams.OpenedOn != replacementParams.Spec.TransactionDate ||
		lotParams.AccountID <= 0 || lotParams.CommodityID <= 0 || lotParams.CostCommodityID <= 0 ||
		replacementParams.InvestmentCorrectionOfOperationID != expected.OperationID ||
		replacementParams.InvestmentCorrectionMode != "replace" ||
		replacementParams.InvestmentCorrectionReason == "" ||
		!inverseParams.CorrectionOfTransactionID.Valid ||
		inverseParams.CorrectionOfTransactionID.Int64 != expected.TransactionID ||
		!replacementParams.CorrectionOfTransactionID.Valid ||
		replacementParams.CorrectionOfTransactionID.Int64 != expected.TransactionID {
		return BuyReplacementRecord{}, fmt.Errorf("%w: buy replacement is incomplete", ErrInvalidDisposalParams)
	}
	var current BuyOperationRecord
	var operationID int64
	var acceptSource func(*sql.Tx, []TransactionRecord, int64) error
	if postWrite != nil {
		acceptSource = func(tx *sql.Tx, _ []TransactionRecord, auditEventID int64) error {
			if err := postWrite(tx, operationID, auditEventID); err != nil {
				return fmt.Errorf("record buy source revision: %w", err)
			}
			return nil
		}
	}
	write := executeInvestmentJournalsWithGuardTx[InvestmentLotRecord]
	if preview {
		write = previewInvestmentJournalsWithGuardTx[InvestmentLotRecord]
	}
	journals, lot, err := write(ctx, r.database,
		[]CreateTransactionParams{inverseParams, replacementParams},
		func(tx *sql.Tx) error {
			var err error
			current, err = checkBuyOperationForCorrectionTx(ctx, tx, inverseParams.BookID, expected)
			return err
		}, func(tx *sql.Tx, journals []TransactionRecord, auditEventID int64) (InvestmentLotRecord, error) {
			inverse, replacement := journals[0], journals[1]
			var err error
			operationID, err = investmentOperationIDTx(ctx, tx, replacementParams.BookID, replacement.ID)
			if err != nil {
				return InvestmentLotRecord{}, err
			}
			if _, err := tx.ExecContext(ctx, `INSERT INTO investment_operation_journal_links
		(book_id, operation_id, transaction_version_id, link_seq, role)
		VALUES (?, ?, ?, 2, 'reversal')`, replacementParams.BookID, operationID, inverse.VersionID); err != nil {
				return InvestmentLotRecord{}, fmt.Errorf("link buy replacement inverse: %w", err)
			}
			lotParams.SourceTransactionID = replacement.ID
			lotParams.CreatedAt = replacementParams.CreatedAt
			lot, err := createLotWithAuditTx(ctx, tx, lotParams, auditEventID, true)
			if err != nil {
				return InvestmentLotRecord{}, err
			}
			// A corrected date, holding, instrument or cost currency (T-116)
			// leaves the source position without the acquisition and opens it
			// in another; both replay in this transaction or neither does.
			for _, position := range correctedTradePositions(
				investmentReplayPositionKey{current.AccountID, current.CommodityID, current.CostCommodityID},
				investmentReplayPositionKey{lotParams.AccountID, lotParams.CommodityID, lotParams.CostCommodityID}) {
				if err := replayCorrectedPositionTx(ctx, tx, replacementParams.BookID, position,
					operationID, auditEventID, replacementParams.ActorUserID, replacementParams.CreatedAt); err != nil {
					return InvestmentLotRecord{}, err
				}
			}
			if err := voidTradePricesForVersionTx(ctx, tx, inverseParams, current.TransactionVersionID, auditEventID); err != nil {
				return InvestmentLotRecord{}, err
			}
			return lot, nil
		}, acceptSource)
	if err != nil {
		return BuyReplacementRecord{}, err
	}
	return BuyReplacementRecord{Inverse: journals[0], Replacement: journals[1], Lot: lot}, nil
}

// correctedTradePositions lists the source position and, when a correction
// moved the trade, its new position. Replaying the source first names a
// dependency the removal breaks before one the new position cannot satisfy.
func correctedTradePositions(source, replacement investmentReplayPositionKey) []investmentReplayPositionKey {
	if source == replacement {
		return []investmentReplayPositionKey{source}
	}
	return []investmentReplayPositionKey{source, replacement}
}

// replayCorrectedPositionTx replays one long position from its committed
// effective intents and installs the result, propagating changed transfer
// basis. A later decision the corrected history cannot satisfy is a named
// correction dependency.
func replayCorrectedPositionTx(ctx context.Context, tx *sql.Tx, bookID int64, position investmentReplayPositionKey,
	operationID, auditEventID, actorUserID int64, createdAt string,
) error {
	intents, err := investmentReplayIntentsQuery(ctx, tx, bookID,
		position.accountID, position.commodityID, position.costCommodityID, "long")
	if err != nil {
		return err
	}
	projection, err := simulateInvestmentReplayTx(ctx, tx, bookID,
		position.accountID, position.commodityID, position.costCommodityID, intents)
	if err != nil {
		if errors.Is(err, ErrInsufficientLots) || errors.Is(err, ErrNotFound) {
			return fmt.Errorf("%w: %w", ErrInvestmentCorrectionDependency, err)
		}
		return err
	}
	return persistInvestmentReplayProjectionTx(ctx, tx, bookID,
		position.accountID, position.commodityID, position.costCommodityID,
		operationID, auditEventID, actorUserID, createdAt, intents, projection)
}
