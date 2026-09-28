package db

import (
	"context"
	"database/sql"
	"fmt"

	"rekenraam/backend/internal/exact"
)

type CreateExternalTransferInParams struct {
	Lot                CreateInvestmentLotParams
	OriginalAcquiredOn string
	SourceEvidenceJSON string
}

// CreateExternalTransferIn writes the balanced book-boundary journal, lot,
// immutable transfer evidence, checkpoint invalidations and one audit event
// under the shared investment transaction boundary.
func (r *InvestmentRepository) CreateExternalTransferIn(ctx context.Context, journal CreateTransactionParams, transfer CreateExternalTransferInParams) (TransactionRecord, InvestmentLotRecord, error) {
	return executeInvestmentWriteTx(ctx, r.database, journal,
		func(tx *sql.Tx, transaction TransactionRecord, auditEventID int64) (InvestmentLotRecord, error) {
			lot := transfer.Lot
			lot.SourceTransactionID = transaction.ID
			lot.EventKind = "transfer_in"
			record, err := createLotWithAuditTx(ctx, tx, lot, auditEventID, false)
			if err != nil {
				return InvestmentLotRecord{}, err
			}
			operationID, err := investmentOperationIDTx(ctx, tx, lot.BookID, transaction.ID)
			if err != nil {
				return InvestmentLotRecord{}, err
			}
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO investment_transfer_facts
					(operation_id, book_id, transfer_kind, effective_on, commodity_id,
					 destination_account_id, source_evidence_json, created_audit_event_id)
				VALUES (?, ?, 'external_in', ?, ?, ?, ?, ?)
			`, operationID, lot.BookID, lot.OpenedOn, lot.CommodityID, lot.AccountID,
				transfer.SourceEvidenceJSON, auditEventID); err != nil {
				return InvestmentLotRecord{}, fmt.Errorf("record external transfer source: %w", err)
			}
			originalKnowledge := "unknown"
			if transfer.OriginalAcquiredOn != "" {
				originalKnowledge = "known"
			}
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO investment_transfer_lot_links
					(operation_id, link_seq, destination_lot_id, quantity_value, quantity_scale,
					 basis_knowledge, carried_basis_value, carried_basis_scale, cost_commodity_id,
					 original_date_knowledge, original_acquired_on, source_evidence_json)
				VALUES (?, 1, ?, ?, ?, 'known', ?, ?, ?, ?, NULLIF(?, ''), ?)
			`, operationID, record.ID, lot.QuantityValue, lot.QuantityScale,
				exact.New(lot.CostBasisValue), lot.CostBasisScale, lot.CostCommodityID,
				originalKnowledge, transfer.OriginalAcquiredOn, transfer.SourceEvidenceJSON); err != nil {
				return InvestmentLotRecord{}, fmt.Errorf("link external transfer lot: %w", err)
			}
			return record, nil
		}, nil)
}

func (r *InvestmentRepository) ExternalInvestmentTransferEquityAccountID(ctx context.Context, bookID int64) (int64, error) {
	var id int64
	err := r.database.QueryRowContext(ctx, `
		SELECT id FROM accounts WHERE book_id = ? AND system_role = 'external_investment_transfer_equity'
	`, bookID).Scan(&id)
	if err == sql.ErrNoRows {
		return 0, ErrNotFound
	}
	if err != nil {
		return 0, fmt.Errorf("read external investment transfer equity account: %w", err)
	}
	return id, nil
}
