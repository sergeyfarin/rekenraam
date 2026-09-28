package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

var ErrInvestmentCorrectionDependency = errors.New("investment correction cannot satisfy a dependent disposal")

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
}

func (r *InvestmentRepository) BuyOperationByTransactionID(ctx context.Context, bookID, transactionID int64) (BuyOperationRecord, error) {
	return buyOperationByTransactionIDQuery(ctx, r.database, bookID, transactionID)
}

func buyOperationByTransactionIDQuery(ctx context.Context, reader saleOperationReader, bookID, transactionID int64) (BuyOperationRecord, error) {
	var record BuyOperationRecord
	var corrected, imported int
	err := reader.QueryRowContext(ctx, `SELECT o.id, o.transaction_id, link.transaction_version_id,
		current.id, f.lot_id, o.event_date, f.account_id, f.commodity_id, f.cost_commodity_id,
		EXISTS(SELECT 1 FROM investment_operations successor WHERE successor.correction_of_operation_id = o.id),
		(audit.origin_type = 'import' OR EXISTS(SELECT 1 FROM import_commit_identity_effects effect
			WHERE effect.operation_id = o.id))
		FROM investment_operations o
		JOIN audit_events audit ON audit.id = o.created_audit_event_id
		JOIN investment_lot_facts f ON f.operation_id = o.id AND f.position_side = 'long'
		JOIN investment_operation_journal_links link ON link.operation_id = o.id AND link.role = 'primary'
		JOIN current_transaction_versions current ON current.transaction_id = o.transaction_id
		WHERE o.book_id = ? AND o.transaction_id = ? AND o.operation_kind = 'buy'
		AND (SELECT count(*) FROM investment_lot_facts one WHERE one.operation_id = o.id) = 1`,
		bookID, transactionID).Scan(&record.OperationID, &record.TransactionID,
		&record.TransactionVersionID, &record.CurrentVersionID, &record.LotID,
		&record.EventDate, &record.AccountID, &record.CommodityID, &record.CostCommodityID,
		&corrected, &imported)
	if errors.Is(err, sql.ErrNoRows) {
		return BuyOperationRecord{}, ErrNotFound
	}
	if err != nil {
		return BuyOperationRecord{}, fmt.Errorf("read investment buy operation: %w", err)
	}
	record.AlreadyCorrected = corrected != 0
	record.Imported = imported != 0
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
	if current.Imported {
		return BuyOperationRecord{}, ErrInvestmentImportedCorrection
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
	tx, err := r.database.BeginTx(ctx, nil)
	if err != nil {
		return BuyReplacementRecord{}, fmt.Errorf("begin buy replacement: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			rollbackTx(ctx, tx)
		}
	}()
	current, err := checkBuyOperationForCorrectionTx(ctx, tx, inverseParams.BookID, expected)
	if err != nil {
		return BuyReplacementRecord{}, err
	}
	if _, err := readBookForUpdate(ctx, tx, inverseParams.BookID); err != nil {
		return BuyReplacementRecord{}, err
	}
	for _, params := range []CreateTransactionParams{inverseParams, replacementParams} {
		if err := requireAccountRuleDependenciesTx(ctx, tx, params.Spec, params.AccountRuleDependencies); err != nil {
			return BuyReplacementRecord{}, err
		}
	}
	auditEventID, err := insertAuditEvent(ctx, tx, AuditEventParams{
		BookID: inverseParams.BookID, ActorUserID: inverseParams.ActorUserID,
		AuthSessionID: inverseParams.AuthSessionID, OccurredAt: inverseParams.CreatedAt,
		RequestID: inverseParams.RequestID, OriginType: inverseParams.OriginType,
		Operation: inverseParams.Operation, Reason: inverseParams.ChangeReason,
	})
	if err != nil {
		return BuyReplacementRecord{}, err
	}
	inverse, err := insertTransactionWithAuditEventTx(ctx, tx, inverseParams, auditEventID)
	if err != nil {
		return BuyReplacementRecord{}, err
	}
	replacement, err := insertTransactionWithAuditEventTx(ctx, tx, replacementParams, auditEventID)
	if err != nil {
		return BuyReplacementRecord{}, err
	}
	operationID, err := investmentOperationIDTx(ctx, tx, replacementParams.BookID, replacement.ID)
	if err != nil {
		return BuyReplacementRecord{}, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO investment_operation_journal_links
		(book_id, operation_id, transaction_version_id, link_seq, role)
		VALUES (?, ?, ?, 2, 'reversal')`, replacementParams.BookID, operationID, inverse.VersionID); err != nil {
		return BuyReplacementRecord{}, fmt.Errorf("link buy replacement inverse: %w", err)
	}
	lotParams.SourceTransactionID = replacement.ID
	lotParams.CreatedAt = replacementParams.CreatedAt
	lot, err := createLotWithAuditTx(ctx, tx, lotParams, auditEventID, true)
	if err != nil {
		return BuyReplacementRecord{}, err
	}
	intents, err := investmentReplayIntentsQuery(ctx, tx, replacementParams.BookID,
		current.AccountID, current.CommodityID, current.CostCommodityID, "long")
	if err != nil {
		return BuyReplacementRecord{}, err
	}
	projection, err := simulateInvestmentReplayTx(ctx, tx, replacementParams.BookID,
		current.AccountID, current.CommodityID, current.CostCommodityID, intents)
	if err != nil {
		if errors.Is(err, ErrInsufficientLots) || errors.Is(err, ErrNotFound) {
			return BuyReplacementRecord{}, fmt.Errorf("%w: %w", ErrInvestmentCorrectionDependency, err)
		}
		return BuyReplacementRecord{}, err
	}
	if err := persistInvestmentReplayProjectionTx(ctx, tx, replacementParams.BookID,
		current.AccountID, current.CommodityID, current.CostCommodityID,
		operationID, auditEventID, replacementParams.ActorUserID, replacementParams.CreatedAt,
		intents, projection); err != nil {
		return BuyReplacementRecord{}, err
	}
	if err := voidTradePricesForVersionTx(ctx, tx, inverseParams, current.TransactionVersionID, auditEventID); err != nil {
		return BuyReplacementRecord{}, err
	}
	inverse.InvalidatedCheckpointIDs, err = invalidateCreateTransactionCheckpointsTx(ctx, tx, inverseParams, auditEventID)
	if err != nil {
		return BuyReplacementRecord{}, err
	}
	replacement.InvalidatedCheckpointIDs, err = invalidateCreateTransactionCheckpointsTx(ctx, tx, replacementParams, auditEventID)
	if err != nil {
		return BuyReplacementRecord{}, err
	}
	if err := tx.Commit(); err != nil {
		return BuyReplacementRecord{}, fmt.Errorf("commit buy replacement: %w", err)
	}
	committed = true
	return BuyReplacementRecord{Inverse: inverse, Replacement: replacement, Lot: lot}, nil
}
