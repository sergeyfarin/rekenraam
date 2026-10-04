package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// Transfer correction (T-119, ADR 0013 *Transfer Correction Refinement*). A
// reversal posts the exact inverse of the transfer's journal and removes the
// transfer from effective history. Every position it touched replays from its
// remaining effective intents in the same transaction: the source gets its
// units back, and the destination loses the lots the transfer opened. Removing
// those lots is the removed edge the T-132 propagation must be seeded with, so
// the destination is replayed explicitly and anything downstream of it
// propagates from there. The original journal, facts, links, link revisions
// and lot events stay immutable evidence.

var ErrInvestmentTransferChanged = errors.New("investment transfer changed after its correction was prepared")

// TransferOperationRecord pins a posted internal or external-in transfer and
// the positions it moved. A correction rechecks it inside the write.
type TransferOperationRecord struct {
	OperationID          int64
	TransferKind         string
	TransactionID        int64
	TransactionVersionID int64
	CurrentVersionID     int64
	EventDate            string
	// SourceAccountID is zero for an external transfer in.
	SourceAccountID      int64
	DestinationAccountID int64
	CommodityID          int64
	CostCommodityID      int64
	AlreadyCorrected     bool
	ImportedLineage      bool
}

func (r *InvestmentRepository) TransferOperationByTransactionID(ctx context.Context, bookID, transactionID int64) (TransferOperationRecord, error) {
	return transferOperationByTransactionIDQuery(ctx, r.database, bookID, transactionID)
}

func transferOperationByTransactionIDQuery(ctx context.Context, reader saleOperationReader, bookID, transactionID int64) (TransferOperationRecord, error) {
	var record TransferOperationRecord
	var source sql.NullInt64
	var costCommodities, corrected int
	err := reader.QueryRowContext(ctx, `SELECT o.id, f.transfer_kind, linked_version.transaction_id,
		link.transaction_version_id, current.id, f.effective_on, f.source_account_id, f.destination_account_id,
		f.commodity_id,
		(SELECT MIN(x.cost_commodity_id) FROM investment_transfer_lot_links x WHERE x.operation_id = o.id),
		(SELECT count(DISTINCT x.cost_commodity_id) FROM investment_transfer_lot_links x WHERE x.operation_id = o.id),
		EXISTS(SELECT 1 FROM investment_operations successor WHERE successor.correction_of_operation_id = o.id)
		FROM investment_operations o
		JOIN investment_transfer_facts f ON f.operation_id = o.id AND f.book_id = o.book_id
		JOIN investment_operation_journal_links link ON link.operation_id = o.id AND link.book_id = o.book_id AND link.role = 'primary'
		JOIN transaction_versions linked_version ON linked_version.id = link.transaction_version_id
		JOIN current_transaction_versions current ON current.transaction_id = linked_version.transaction_id
		WHERE o.book_id = ? AND linked_version.transaction_id = ? AND f.transfer_kind IN ('internal', 'external_in')`,
		bookID, transactionID).Scan(&record.OperationID, &record.TransferKind, &record.TransactionID,
		&record.TransactionVersionID, &record.CurrentVersionID, &record.EventDate, &source,
		&record.DestinationAccountID, &record.CommodityID, &record.CostCommodityID, &costCommodities, &corrected)
	if errors.Is(err, sql.ErrNoRows) {
		return TransferOperationRecord{}, ErrNotFound
	}
	if err != nil {
		return TransferOperationRecord{}, fmt.Errorf("read investment transfer operation: %w", err)
	}
	if costCommodities != 1 {
		return TransferOperationRecord{}, fmt.Errorf("%w: transfer %d does not carry one cost currency", ErrInvalidDisposalParams, record.OperationID)
	}
	record.SourceAccountID = source.Int64
	record.AlreadyCorrected = corrected != 0
	if record.ImportedLineage, err = investmentOperationHasImportedLineageQuery(ctx, reader, bookID, record.OperationID); err != nil {
		return TransferOperationRecord{}, err
	}
	return record, nil
}

// checkTransferOperationForCorrectionTx applies the shared correction fences:
// not already corrected, an imported lineage still names its committed source,
// and neither the transfer nor its journal moved since it was planned.
func checkTransferOperationForCorrectionTx(ctx context.Context, tx *sql.Tx, bookID int64, expected TransferOperationRecord) (TransferOperationRecord, error) {
	current, err := transferOperationByTransactionIDQuery(ctx, tx, bookID, expected.TransactionID)
	if err != nil {
		return TransferOperationRecord{}, err
	}
	if current.AlreadyCorrected {
		return TransferOperationRecord{}, ErrInvestmentOperationAlreadyCorrected
	}
	if current.ImportedLineage {
		linked, err := investmentOperationHasCommittedSourceQuery(ctx, tx, bookID, current.OperationID)
		if err != nil {
			return TransferOperationRecord{}, err
		}
		if !linked {
			return TransferOperationRecord{}, ErrInvestmentImportedCorrection
		}
	}
	if current != expected {
		return TransferOperationRecord{}, ErrInvestmentTransferChanged
	}
	if err := checkInvestmentSourceJournalTx(ctx, tx, bookID, current.TransactionID,
		current.EventDate, current.TransactionVersionID, current.CurrentVersionID); err != nil {
		if errors.Is(err, ErrInvestmentSaleChanged) {
			return TransferOperationRecord{}, ErrInvestmentTransferChanged
		}
		return TransferOperationRecord{}, err
	}
	return current, nil
}

// transferPositions lists the long positions a transfer moved: the source
// (internal only) first, so a dependency the restored units break is named
// before one the removed destination lots break.
func (t TransferOperationRecord) transferPositions() []investmentReplayPositionKey {
	destination := investmentReplayPositionKey{t.DestinationAccountID, t.CommodityID, t.CostCommodityID}
	if t.SourceAccountID <= 0 {
		return []investmentReplayPositionKey{destination}
	}
	return []investmentReplayPositionKey{
		{t.SourceAccountID, t.CommodityID, t.CostCommodityID}, destination}
}

// ReverseTransfer posts the inverse journal and removes the transfer from
// effective history, replaying both ends under one audit event.
func (r *InvestmentRepository) ReverseTransfer(ctx context.Context, params CreateTransactionParams, expected TransferOperationRecord) (TransactionRecord, error) {
	return r.reverseTransfer(ctx, params, expected, false)
}

// PreviewTransferReversal runs the reversal writer, both replays, gain
// comparison and checkpoint invalidation, then rolls back.
func (r *InvestmentRepository) PreviewTransferReversal(ctx context.Context, params CreateTransactionParams, expected TransferOperationRecord) (SimulatedInvestmentWrite, error) {
	transaction, err := r.reverseTransfer(ctx, params, expected, true)
	if err != nil {
		return SimulatedInvestmentWrite{}, err
	}
	return simulatedInvestmentWrite(transaction), nil
}

func (r *InvestmentRepository) reverseTransfer(ctx context.Context, params CreateTransactionParams, expected TransferOperationRecord, preview bool) (TransactionRecord, error) {
	if params.BookID <= 0 || params.ActorUserID <= 0 || expected.OperationID <= 0 ||
		expected.DestinationAccountID <= 0 || expected.CostCommodityID <= 0 ||
		(expected.TransferKind == "internal") != (expected.SourceAccountID > 0) ||
		params.Spec.InvestmentOperationKind != "reversal" || params.Spec.TransactionKind != "investment" ||
		params.Spec.Status != "posted" || params.Spec.TransactionDate != expected.EventDate ||
		params.InvestmentCorrectionOfOperationID != expected.OperationID ||
		params.InvestmentCorrectionMode != "reverse" || params.InvestmentCorrectionReason == "" ||
		!params.CorrectionOfTransactionID.Valid || params.CorrectionOfTransactionID.Int64 != expected.TransactionID {
		return TransactionRecord{}, fmt.Errorf("%w: transfer reversal is incomplete", ErrInvalidDisposalParams)
	}
	guard := func(tx *sql.Tx) error {
		_, err := checkTransferOperationForCorrectionTx(ctx, tx, params.BookID, expected)
		return err
	}
	effect := func(tx *sql.Tx, transaction TransactionRecord, auditEventID int64) (struct{}, error) {
		operationID, err := investmentOperationIDTx(ctx, tx, params.BookID, transaction.ID)
		if err != nil {
			return struct{}{}, err
		}
		for _, position := range expected.transferPositions() {
			if err := replayCorrectedPositionTx(ctx, tx, params.BookID, position,
				operationID, auditEventID, params.ActorUserID, params.CreatedAt); err != nil {
				return struct{}{}, err
			}
		}
		return struct{}{}, nil
	}
	var transaction TransactionRecord
	var err error
	if preview {
		transaction, _, err = previewInvestmentWriteWithGuardTx(ctx, r.database, params, guard, effect)
	} else {
		transaction, _, err = executeInvestmentWriteWithGuardTx(ctx, r.database, params, guard, effect, nil)
	}
	return transaction, err
}
