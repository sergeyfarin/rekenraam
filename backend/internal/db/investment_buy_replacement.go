package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

var ErrInvestmentCorrectionDependency = errors.New("investment correction cannot satisfy a dependent operation")

// BuyOperationRecord pins the posted source buy and its one immutable opening
// lot. A correction must recheck it inside the write transaction.
type BuyOperationRecord struct {
	OperationID          int64
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
	err := reader.QueryRowContext(ctx, `SELECT o.id, linked_version.transaction_id, link.transaction_version_id,
		current.id, f.lot_id, o.event_date, f.account_id, f.commodity_id, f.cost_commodity_id,
		EXISTS(SELECT 1 FROM investment_operations successor WHERE successor.correction_of_operation_id = o.id),
		(audit.origin_type = 'import' OR EXISTS(SELECT 1 FROM import_commit_identity_effects effect
			WHERE effect.operation_id = o.id)),
		COALESCE((SELECT effect.identity_id FROM import_commit_identity_effects effect
			WHERE effect.operation_id = o.id), 0),
		COALESCE((SELECT effect.effect_seq FROM import_commit_identity_effects effect
			WHERE effect.operation_id = o.id), 0)
		FROM investment_operations o
		JOIN audit_events audit ON audit.id = o.created_audit_event_id
		JOIN investment_lot_facts f ON f.operation_id = o.id AND f.position_side = 'long'
		JOIN investment_operation_journal_links link ON link.operation_id = o.id AND link.book_id = o.book_id AND link.role = 'primary'
		JOIN transaction_versions linked_version ON linked_version.id = link.transaction_version_id
		JOIN current_transaction_versions current ON current.transaction_id = linked_version.transaction_id
		WHERE o.book_id = ? AND linked_version.transaction_id = ? AND o.operation_kind = 'buy'
		AND (SELECT count(*) FROM investment_lot_facts one WHERE one.operation_id = o.id) = 1`,
		bookID, transactionID).Scan(&record.OperationID, &record.TransactionID,
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
	if expected.OperationID <= 0 || expected.LotID <= 0 ||
		inverseParams.BookID <= 0 || inverseParams.BookID != replacementParams.BookID ||
		inverseParams.BookID != lotParams.BookID ||
		inverseParams.ActorUserID != replacementParams.ActorUserID ||
		inverseParams.ActorUserID != lotParams.CreatedByUserID ||
		inverseParams.Spec.InvestmentOperationKind != "" ||
		inverseParams.Spec.TransactionKind != "investment" || inverseParams.Spec.Status != "posted" ||
		replacementParams.Spec.InvestmentOperationKind != "buy" ||
		replacementParams.Spec.TransactionKind != "investment" || replacementParams.Spec.Status != "posted" ||
		inverseParams.Spec.TransactionDate != expected.EventDate ||
		replacementParams.Spec.TransactionDate != expected.EventDate ||
		lotParams.OpenedOn != expected.EventDate ||
		lotParams.AccountID != expected.AccountID || lotParams.CommodityID != expected.CommodityID ||
		lotParams.CostCommodityID != expected.CostCommodityID ||
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
	journals, lot, err := executeInvestmentJournalsWithGuardTx(ctx, r.database,
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
			intents, err := investmentReplayIntentsQuery(ctx, tx, replacementParams.BookID,
				current.AccountID, current.CommodityID, current.CostCommodityID, "long")
			if err != nil {
				return InvestmentLotRecord{}, err
			}
			projection, err := simulateInvestmentReplayTx(ctx, tx, replacementParams.BookID,
				current.AccountID, current.CommodityID, current.CostCommodityID, intents)
			if err != nil {
				if errors.Is(err, ErrInsufficientLots) || errors.Is(err, ErrNotFound) {
					return InvestmentLotRecord{}, fmt.Errorf("%w: %w", ErrInvestmentCorrectionDependency, err)
				}
				return InvestmentLotRecord{}, err
			}
			if err := persistInvestmentReplayProjectionTx(ctx, tx, replacementParams.BookID,
				current.AccountID, current.CommodityID, current.CostCommodityID,
				operationID, auditEventID, replacementParams.ActorUserID, replacementParams.CreatedAt,
				intents, projection); err != nil {
				return InvestmentLotRecord{}, err
			}
			if err := voidTradePricesForVersionTx(ctx, tx, inverseParams, current.TransactionVersionID, auditEventID); err != nil {
				return InvestmentLotRecord{}, err
			}
			return lot, nil
		}, func(tx *sql.Tx, _ []TransactionRecord, auditEventID int64) error {
			if postWrite != nil {
				if err := postWrite(tx, operationID, auditEventID); err != nil {
					return fmt.Errorf("record buy source revision: %w", err)
				}
			}
			return nil
		})
	if err != nil {
		return BuyReplacementRecord{}, err
	}
	return BuyReplacementRecord{Inverse: journals[0], Replacement: journals[1], Lot: lot}, nil
}
