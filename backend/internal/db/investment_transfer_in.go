package db

import (
	"context"
	"database/sql"
	"fmt"
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
	return r.createExternalTransferIn(ctx, journal, transfer, false)
}

// SimulateExternalTransferIn runs the complete transfer-in writer, including
// any replay of later disposals, then rolls back (T-117).
func (r *InvestmentRepository) SimulateExternalTransferIn(ctx context.Context, journal CreateTransactionParams, transfer CreateExternalTransferInParams) (SimulatedInvestmentWrite, error) {
	transaction, _, err := r.createExternalTransferIn(ctx, journal, transfer, true)
	if err != nil {
		return SimulatedInvestmentWrite{}, err
	}
	return simulatedInvestmentWrite(transaction), nil
}

// A transfer-in dated before a later depletion is admitted the way a
// backdated purchase is: its lot opens on the transfer date (availability)
// while its link carries the original acquisition date (FIFO/LIFO order), and
// the position replays every later decision under its recorded policy. The
// link is written before replay so that ordering sees the original date.
func (r *InvestmentRepository) createExternalTransferIn(ctx context.Context, journal CreateTransactionParams, transfer CreateExternalTransferInParams, preview bool) (TransactionRecord, InvestmentLotRecord, error) {
	write := executeInvestmentWriteTx[InvestmentLotRecord]
	if preview {
		write = previewInvestmentWriteTx[InvestmentLotRecord]
	}
	return write(ctx, r.database, journal,
		func(tx *sql.Tx, transaction TransactionRecord, auditEventID int64) (InvestmentLotRecord, error) {
			return writeExternalTransferInTx(ctx, tx, journal, transfer, transaction, auditEventID, false)
		}, nil)
}

// writeExternalTransferInTx records the transfer's lot, facts and link for a
// journal the enclosing writer just posted. A transfer dated before a later
// depletion is admitted through replay of its holding. A correction passes
// correcting, which always admits the lot by replay and leaves the replay of
// every touched holding to the caller.
func writeExternalTransferInTx(ctx context.Context, tx *sql.Tx, journal CreateTransactionParams,
	transfer CreateExternalTransferInParams, transaction TransactionRecord, auditEventID int64, correcting bool,
) (InvestmentLotRecord, error) {
	lot := transfer.Lot
	lot.SourceTransactionID = transaction.ID
	lot.EventKind = "transfer_in"
	knowledge := normalizedBasisKnowledge(lot.OpeningBasisKnowledge)
	if knowledge == InvestmentBasisUnknown {
		for _, entry := range transaction.JournalEntries {
			for _, posting := range entry.Postings {
				if posting.CommodityID != lot.CommodityID {
					return InvestmentLotRecord{}, fmt.Errorf("%w: unknown transfer basis requires a security-only journal", ErrInvalidDisposalParams)
				}
			}
		}
	}
	latest, err := latestPositionRewriteDateTx(ctx, tx, lot.BookID, lot.AccountID, lot.CommodityID)
	if err != nil {
		return InvestmentLotRecord{}, err
	}
	replayAdmission := correcting || (latest != "" && lot.OpenedOn < latest)
	record, err := createLotWithAuditTx(ctx, tx, lot, auditEventID, replayAdmission)
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
		VALUES (?, 1, ?, ?, ?, ?, ?, ?, ?, ?, NULLIF(?, ''), ?)
	`, operationID, record.ID, lot.QuantityValue, lot.QuantityScale,
		knowledge, nullableBasisValue(lot.CostBasisValue, knowledge), nullableBasisScale(lot.CostBasisScale, knowledge), lot.CostCommodityID,
		originalKnowledge, transfer.OriginalAcquiredOn, transfer.SourceEvidenceJSON); err != nil {
		return InvestmentLotRecord{}, fmt.Errorf("link external transfer lot: %w", err)
	}
	if correcting || !replayAdmission {
		return record, nil
	}
	intents, err := investmentReplayIntentsQuery(ctx, tx, lot.BookID, lot.AccountID,
		lot.CommodityID, lot.CostCommodityID, "long")
	if err != nil {
		return InvestmentLotRecord{}, err
	}
	projection, err := simulateInvestmentReplayTx(ctx, tx, lot.BookID, lot.AccountID,
		lot.CommodityID, lot.CostCommodityID, intents)
	if err != nil {
		return InvestmentLotRecord{}, err
	}
	if err := persistInvestmentReplayProjectionTx(ctx, tx, lot.BookID, lot.AccountID,
		lot.CommodityID, lot.CostCommodityID, operationID, auditEventID, journal.ActorUserID,
		journal.CreatedAt, intents, projection); err != nil {
		return InvestmentLotRecord{}, err
	}
	return investmentLotByIDTx(ctx, tx, lot.BookID, record.ID)
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
