package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

var (
	ErrInvestmentOperationAlreadyCorrected = errors.New("investment operation already has a correction")
	ErrInvestmentImportedCorrection        = errors.New("imported investment operation requires source correction")
	ErrInvestmentSaleChanged               = errors.New("investment sale changed after reversal was prepared")
)

// SaleOperationRecord is the immutable identity and position of a posted long
// sale. A caller must still recheck it inside the correcting write transaction.
type SaleOperationRecord struct {
	OperationID          int64
	TransactionID        int64
	TransactionVersionID int64
	CurrentVersionID     int64
	EventDate            string
	AccountID            int64
	CommodityID          int64
	CostCommodityID      int64
	AlreadyCorrected     bool
	Imported             bool
	SourceIdentityID     int64
	SourceEffectSeq      int64
}

func (r *InvestmentRepository) SaleOperationByID(ctx context.Context, bookID, operationID int64) (SaleOperationRecord, error) {
	return saleOperationByIDQuery(ctx, r.database, bookID, operationID)
}

func (r *InvestmentRepository) SaleOperationByTransactionID(ctx context.Context, bookID, transactionID int64) (SaleOperationRecord, error) {
	var operationID int64
	err := r.database.QueryRowContext(ctx, `SELECT id FROM investment_operations
		WHERE book_id = ? AND transaction_id = ? AND operation_kind = 'sell'`, bookID, transactionID).Scan(&operationID)
	if errors.Is(err, sql.ErrNoRows) {
		return SaleOperationRecord{}, ErrNotFound
	}
	if err != nil {
		return SaleOperationRecord{}, fmt.Errorf("find investment sale by transaction: %w", err)
	}
	return r.SaleOperationByID(ctx, bookID, operationID)
}

type saleOperationReader interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func saleOperationByIDQuery(ctx context.Context, reader saleOperationReader, bookID, operationID int64) (SaleOperationRecord, error) {
	var record SaleOperationRecord
	var corrected, imported int
	err := reader.QueryRowContext(ctx, `
		SELECT o.id, o.transaction_id, link.transaction_version_id, current.id, o.event_date,
			d.account_id, d.commodity_id, d.cost_commodity_id,
			EXISTS(SELECT 1 FROM investment_operations successor
				WHERE successor.correction_of_operation_id = o.id),
			(audit.origin_type = 'import' OR EXISTS(SELECT 1 FROM import_commit_identity_effects effect
				WHERE effect.operation_id = o.id)),
			COALESCE((SELECT effect.identity_id FROM import_commit_identity_effects effect
				WHERE effect.operation_id = o.id), 0),
			COALESCE((SELECT effect.effect_seq FROM import_commit_identity_effects effect
				WHERE effect.operation_id = o.id), 0)
		FROM investment_operations o
		JOIN audit_events audit ON audit.id = o.created_audit_event_id
		JOIN investment_disposal_decisions d ON d.operation_id = o.id AND d.position_side = 'long'
		JOIN investment_operation_journal_links link ON link.operation_id = o.id AND link.role = 'primary'
		JOIN current_transaction_versions current ON current.transaction_id = o.transaction_id
		WHERE o.book_id = ? AND o.id = ? AND o.operation_kind = 'sell'
	`, bookID, operationID).Scan(&record.OperationID, &record.TransactionID,
		&record.TransactionVersionID, &record.CurrentVersionID, &record.EventDate, &record.AccountID,
		&record.CommodityID, &record.CostCommodityID, &corrected, &imported,
		&record.SourceIdentityID, &record.SourceEffectSeq)
	if errors.Is(err, sql.ErrNoRows) {
		return SaleOperationRecord{}, ErrNotFound
	}
	if err != nil {
		return SaleOperationRecord{}, fmt.Errorf("read investment sale operation: %w", err)
	}
	record.AlreadyCorrected = corrected != 0
	record.Imported = imported != 0
	return record, nil
}

// ReverseSale posts an inverse journal and installs the replayed long position
// under one audit event. The original journal, decision, and lot events remain
// immutable. Imported sales stay fenced until source identity correction is
// part of the same command.
func (r *InvestmentRepository) ReverseSale(ctx context.Context, params CreateTransactionParams, expected SaleOperationRecord) (TransactionRecord, error) {
	if params.BookID <= 0 || params.ActorUserID <= 0 || expected.OperationID <= 0 ||
		params.Spec.InvestmentOperationKind != "reversal" || params.Spec.Status != "posted" ||
		params.Spec.TransactionDate != expected.EventDate ||
		params.InvestmentCorrectionOfOperationID != expected.OperationID ||
		params.InvestmentCorrectionMode != "reverse" ||
		params.InvestmentCorrectionReason == "" ||
		!params.CorrectionOfTransactionID.Valid || params.CorrectionOfTransactionID.Int64 != expected.TransactionID {
		return TransactionRecord{}, fmt.Errorf("%w: sale reversal is incomplete", ErrInvalidDisposalParams)
	}
	tx, err := r.database.BeginTx(ctx, nil)
	if err != nil {
		return TransactionRecord{}, fmt.Errorf("begin sale reversal: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			rollbackTx(ctx, tx)
		}
	}()
	current, err := checkSaleOperationForCorrectionTx(ctx, tx, params.BookID, expected, false)
	if err != nil {
		return TransactionRecord{}, err
	}
	transaction, auditEventID, err := createTransactionWithAuditTx(ctx, tx, params)
	if err != nil {
		return TransactionRecord{}, err
	}
	operationID, err := investmentOperationIDTx(ctx, tx, params.BookID, transaction.ID)
	if err != nil {
		return TransactionRecord{}, err
	}
	intents, err := investmentReplayIntentsQuery(ctx, tx, params.BookID,
		current.AccountID, current.CommodityID, current.CostCommodityID, "long")
	if err != nil {
		return TransactionRecord{}, err
	}
	projection, err := simulateInvestmentReplayTx(ctx, tx, params.BookID,
		current.AccountID, current.CommodityID, current.CostCommodityID, intents)
	if err != nil {
		return TransactionRecord{}, err
	}
	if err := persistInvestmentReplayProjectionTx(ctx, tx, params.BookID,
		current.AccountID, current.CommodityID, current.CostCommodityID,
		operationID, auditEventID, params.ActorUserID, params.CreatedAt,
		intents, projection); err != nil {
		return TransactionRecord{}, err
	}
	if err := voidTradePricesForVersionTx(ctx, tx, params, current.TransactionVersionID, auditEventID); err != nil {
		return TransactionRecord{}, err
	}
	transaction.InvalidatedCheckpointIDs, err = invalidateCreateTransactionCheckpointsTx(ctx, tx, params, auditEventID)
	if err != nil {
		return TransactionRecord{}, err
	}
	if err := tx.Commit(); err != nil {
		return TransactionRecord{}, fmt.Errorf("commit sale reversal: %w", err)
	}
	committed = true
	return transaction, nil
}

func checkSaleOperationForCorrectionTx(ctx context.Context, tx *sql.Tx, bookID int64, expected SaleOperationRecord, allowImportedReplacement bool) (SaleOperationRecord, error) {
	current, err := saleOperationByIDQuery(ctx, tx, bookID, expected.OperationID)
	if err != nil {
		return SaleOperationRecord{}, err
	}
	if current.AlreadyCorrected {
		return SaleOperationRecord{}, ErrInvestmentOperationAlreadyCorrected
	}
	if current.Imported && (!allowImportedReplacement || current.SourceIdentityID == 0 || current.SourceEffectSeq == 0) {
		return SaleOperationRecord{}, ErrInvestmentImportedCorrection
	}
	if current != expected {
		return SaleOperationRecord{}, ErrInvestmentSaleChanged
	}
	if err := checkInvestmentSourceJournalTx(ctx, tx, bookID, current.TransactionID,
		current.EventDate, current.TransactionVersionID, current.CurrentVersionID); err != nil {
		return SaleOperationRecord{}, err
	}
	return current, nil
}

func checkInvestmentSourceJournalTx(ctx context.Context, tx *sql.Tx, bookID, transactionID int64,
	eventDate string, sourceVersionID, expectedCurrentVersionID int64) error {
	var currentVersionID int64
	var status string
	var transactionKind string
	var transactionDate string
	var deletedAt sql.NullString
	if err := tx.QueryRowContext(ctx, `
		SELECT v.id, v.status, v.transaction_kind, v.transaction_date, t.deleted_at
		FROM transactions t JOIN current_transaction_versions v ON v.transaction_id = t.id
		WHERE t.book_id = ? AND t.id = ?
	`, bookID, transactionID).Scan(&currentVersionID, &status, &transactionKind, &transactionDate, &deletedAt); err != nil {
		return fmt.Errorf("check investment source transaction version: %w", err)
	}
	if currentVersionID != expectedCurrentVersionID || status != "posted" || transactionKind != "investment" || transactionDate != eventDate || deletedAt.Valid {
		return ErrInvestmentSaleChanged
	}
	if currentVersionID != sourceVersionID {
		unchanged, err := saleJournalEconomicsUnchangedTx(ctx, tx, sourceVersionID, currentVersionID)
		if err != nil {
			return err
		}
		if !unchanged {
			return ErrInvestmentSaleChanged
		}
	}
	return nil
}

// Reconciliation may create a new transaction version while leaving a sale's
// economic postings intact. An inverse can use that version, but only after
// rechecking its immutable source quantities against the current journal.
func saleJournalEconomicsUnchangedTx(ctx context.Context, tx *sql.Tx, sourceVersionID, currentVersionID int64) (bool, error) {
	var unchanged int
	err := tx.QueryRowContext(ctx, `WITH source AS (
		SELECT e.entry_seq, e.entry_date, e.entry_kind, p.line_seq, p.account_id,
			p.commodity_id, p.quantity_value, p.quantity_scale
		FROM journal_entries e JOIN posting_versions p ON p.journal_entry_id = e.id
		WHERE e.transaction_version_id = ?
	), current AS (
		SELECT e.entry_seq, e.entry_date, e.entry_kind, p.line_seq, p.account_id,
			p.commodity_id, p.quantity_value, p.quantity_scale
		FROM journal_entries e JOIN posting_versions p ON p.journal_entry_id = e.id
		WHERE e.transaction_version_id = ?
	)
	SELECT NOT EXISTS(SELECT * FROM source EXCEPT SELECT * FROM current)
		AND NOT EXISTS(SELECT * FROM current EXCEPT SELECT * FROM source)`, sourceVersionID, currentVersionID).Scan(&unchanged)
	if err != nil {
		return false, fmt.Errorf("compare sale journal versions: %w", err)
	}
	return unchanged != 0, nil
}

func voidTradePricesForVersionTx(ctx context.Context, tx *sql.Tx, params CreateTransactionParams, versionID, auditEventID int64) error {
	rows, err := tx.QueryContext(ctx, `SELECT id FROM price_observations
		WHERE book_id = ? AND source_transaction_version_id = ? AND voided_at IS NULL
		ORDER BY id`, params.BookID, versionID)
	if err != nil {
		return fmt.Errorf("find trade prices to retire: %w", err)
	}
	var rootIDs []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return fmt.Errorf("scan trade price to retire: %w", err)
		}
		rootIDs = append(rootIDs, id)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return fmt.Errorf("iterate trade prices to retire: %w", err)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("close trade prices to retire: %w", err)
	}
	for _, rootID := range rootIDs {
		// A prior root may have retired a second root through derivation.
		var active int
		if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM price_observations
			WHERE book_id = ? AND id = ? AND voided_at IS NULL`, params.BookID, rootID).Scan(&active); err != nil {
			return fmt.Errorf("check trade price before retirement: %w", err)
		}
		if active == 0 {
			continue
		}
		_, err := voidPriceObservationWithAuditTx(ctx, tx, VoidPriceObservationParams{
			BookID: params.BookID, ObservationID: rootID, ActorUserID: params.ActorUserID,
			VoidedAt: params.CreatedAt, VoidReason: params.ChangeReason,
		}, auditEventID)
		if err != nil {
			return fmt.Errorf("retire trade price %d: %w", rootID, err)
		}
	}
	return nil
}
