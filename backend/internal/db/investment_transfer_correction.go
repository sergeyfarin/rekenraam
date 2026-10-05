package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"

	"rekenraam/backend/internal/exact"
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

// InternalTransferReplacementRecord is the inverse and replacement journals
// and what the replacement transfer carried.
type InternalTransferReplacementRecord struct {
	Inverse     TransactionRecord
	Replacement TransactionRecord
	Result      InternalTransferResult
}

// ReplaceInternalTransfer reverses an internal transfer and records corrected
// terms as its successor at the correction root's same-day slot, replaying
// every position either transfer moved under one audit event.
func (r *InvestmentRepository) ReplaceInternalTransfer(ctx context.Context, expected TransferOperationRecord,
	inverseParams, replacementParams CreateTransactionParams, transfer CreateInternalTransferParams,
) (InternalTransferReplacementRecord, error) {
	return r.replaceInternalTransfer(ctx, expected, inverseParams, replacementParams, transfer, false)
}

// SimulateInternalTransferReplacement runs the replacement writer, every
// replay and the gain comparison, then rolls back. Destination lot IDs are
// cleared because they were never durable.
func (r *InvestmentRepository) SimulateInternalTransferReplacement(ctx context.Context, expected TransferOperationRecord,
	inverseParams, replacementParams CreateTransactionParams, transfer CreateInternalTransferParams,
) (SimulatedInvestmentWrite, InternalTransferResult, error) {
	record, err := r.replaceInternalTransfer(ctx, expected, inverseParams, replacementParams, transfer, true)
	if err != nil {
		return SimulatedInvestmentWrite{}, InternalTransferResult{}, err
	}
	record.Result.DestinationLotIDs = nil
	for index := range record.Result.Links {
		record.Result.Links[index].DestinationLotID = 0
	}
	return simulatedInvestmentWrite(record.Inverse, record.Replacement), record.Result, nil
}

// replaceInternalTransfer computes the replacement's source depletion the way
// a split computes its effects: the source replays its effective history with
// the new transfer as the subject at the replaced transfer's slot, so the
// depletion is the one the transfer date sees, not today's lots. Those
// depletions become the replacement's lot events, its destination lots open
// by replay admission, and then every position either transfer touched
// replays from the committed facts, with propagation downstream.
func (r *InvestmentRepository) replaceInternalTransfer(ctx context.Context, expected TransferOperationRecord,
	inverseParams, replacementParams CreateTransactionParams, transfer CreateInternalTransferParams, preview bool,
) (InternalTransferReplacementRecord, error) {
	if expected.OperationID <= 0 || expected.TransferKind != "internal" || expected.SourceAccountID <= 0 ||
		inverseParams.BookID <= 0 || inverseParams.BookID != replacementParams.BookID ||
		inverseParams.BookID != transfer.BookID ||
		inverseParams.ActorUserID != replacementParams.ActorUserID ||
		inverseParams.Spec.InvestmentOperationKind != "" ||
		inverseParams.Spec.TransactionKind != "investment" || inverseParams.Spec.Status != "posted" ||
		inverseParams.Spec.TransactionDate != expected.EventDate ||
		replacementParams.Spec.InvestmentOperationKind != "internal_transfer" ||
		replacementParams.Spec.TransactionKind != "investment" || replacementParams.Spec.Status != "posted" ||
		replacementParams.Spec.TransactionDate != transfer.EffectiveOn ||
		transfer.SourceAccountID <= 0 || transfer.DestinationAccountID <= 0 ||
		transfer.SourceAccountID == transfer.DestinationAccountID ||
		transfer.CommodityID != expected.CommodityID || transfer.CostCommodityID != expected.CostCommodityID ||
		(len(transfer.Allocations) == 0) != (transfer.PooledQuantityValue.Sign() > 0) ||
		replacementParams.InvestmentCorrectionOfOperationID != expected.OperationID ||
		replacementParams.InvestmentCorrectionMode != "replace" || replacementParams.InvestmentCorrectionReason == "" ||
		!inverseParams.CorrectionOfTransactionID.Valid || inverseParams.CorrectionOfTransactionID.Int64 != expected.TransactionID ||
		!replacementParams.CorrectionOfTransactionID.Valid || replacementParams.CorrectionOfTransactionID.Int64 != expected.TransactionID {
		return InternalTransferReplacementRecord{}, fmt.Errorf("%w: internal transfer replacement is incomplete", ErrInvalidDisposalParams)
	}
	write := executeInvestmentJournalsWithGuardTx[InternalTransferResult]
	if preview {
		write = previewInvestmentJournalsWithGuardTx[InternalTransferResult]
	}
	journals, result, err := write(ctx, r.database,
		[]CreateTransactionParams{inverseParams, replacementParams},
		func(tx *sql.Tx) error {
			_, err := checkTransferOperationForCorrectionTx(ctx, tx, inverseParams.BookID, expected)
			return err
		}, func(tx *sql.Tx, journals []TransactionRecord, auditEventID int64) (InternalTransferResult, error) {
			inverse, replacement := journals[0], journals[1]
			operationID, err := investmentOperationIDTx(ctx, tx, transfer.BookID, replacement.ID)
			if err != nil {
				return InternalTransferResult{}, err
			}
			if _, err := tx.ExecContext(ctx, `INSERT INTO investment_operation_journal_links
				(book_id, operation_id, transaction_version_id, link_seq, role)
				VALUES (?, ?, ?, 2, 'reversal')`, transfer.BookID, operationID, inverse.VersionID); err != nil {
				return InternalTransferResult{}, fmt.Errorf("link transfer replacement inverse: %w", err)
			}
			policy, err := internalTransferPolicyTx(ctx, tx, transfer)
			if err != nil {
				return InternalTransferResult{}, err
			}
			if err := insertInternalTransferFactTx(ctx, tx, transfer, policy, operationID, auditEventID); err != nil {
				return InternalTransferResult{}, err
			}
			moved, err := subjectTransferDepletionTx(ctx, tx, expected.OperationID, "internal_transfer", transfer, policy, replacement,
				replacementParams, operationID, auditEventID)
			if err != nil {
				return InternalTransferResult{}, err
			}
			result, err := openInternalTransferDestinationsTx(ctx, tx, transfer, policy, replacement,
				replacementParams, operationID, auditEventID, moved, true)
			if err != nil {
				return InternalTransferResult{}, err
			}
			for _, position := range replacedTransferPositions(expected, transfer) {
				if err := replayCorrectedPositionTx(ctx, tx, transfer.BookID, position,
					operationID, auditEventID, replacementParams.ActorUserID, replacementParams.CreatedAt); err != nil {
					return InternalTransferResult{}, err
				}
			}
			return result, nil
		}, nil)
	if err != nil {
		return InternalTransferReplacementRecord{}, err
	}
	return InternalTransferReplacementRecord{Inverse: journals[0], Replacement: journals[1], Result: result}, nil
}

// subjectTransferDepletionTx replays the transfer's source with the new
// transfer as the subject at slotOperationID's correction-root slot (the
// replaced transfer for a replacement, the transfer itself for a backdated
// outbound, T-143), then records the depletions it reported as the transfer's
// transfer_out lot events.
func subjectTransferDepletionTx(ctx context.Context, tx *sql.Tx, slotOperationID int64, operationKind string,
	transfer CreateInternalTransferParams, policy internalTransferPolicy, replacement TransactionRecord,
	journal CreateTransactionParams, operationID, auditEventID int64,
) ([]LotDisposalRecord, error) {
	roots, err := investmentReplayOrderOperationIDsQuery(ctx, tx, transfer.BookID)
	if err != nil {
		return nil, err
	}
	orderID := roots[slotOperationID]
	if orderID <= 0 {
		return nil, fmt.Errorf("%w: transfer %d has no correction root", ErrInvalidDisposalParams, slotOperationID)
	}
	intents, err := investmentReplayIntentsQuery(ctx, tx, transfer.BookID, transfer.SourceAccountID,
		transfer.CommodityID, transfer.CostCommodityID, "long")
	if err != nil {
		return nil, err
	}
	subject := InvestmentReplayIntent{OperationID: operationID, OrderOperationID: orderID,
		OperationKind: operationKind, EventDate: transfer.EffectiveOn, TransferIsSubject: true,
		TransactionID: replacement.ID, AuditEventID: auditEventID,
		CreatedByUserID: journal.ActorUserID, CreatedAt: journal.CreatedAt}
	switch {
	case policy.allocation == InternalTransferSelectedLots:
		for index, allocation := range transfer.Allocations {
			if allocation.LotID <= 0 || allocation.QuantityValue.Sign() <= 0 {
				return nil, ErrInvalidDisposalParams
			}
			one := subject
			one.Kind, one.EffectSeq, one.LinkSeq = "transfer_out", index+1, index+1
			one.LotID, one.RecordedLotID = allocation.LotID, allocation.LotID
			one.QuantityValue, one.QuantityScale = allocation.QuantityValue, allocation.QuantityScale
			intents = append(intents, one)
		}
	default:
		subject.Kind, subject.EffectSeq = "pooled_transfer_out", 1
		if policy.lineage == InternalTransferPooledLot {
			subject.Kind = "pooled_lot_transfer_out"
		}
		subject.QuantityValue, subject.QuantityScale = transfer.PooledQuantityValue, transfer.PooledQuantityScale
		intents = append(intents, subject)
	}
	projection, err := simulateInvestmentReplayTx(ctx, tx, transfer.BookID, transfer.SourceAccountID,
		transfer.CommodityID, transfer.CostCommodityID, intents)
	if err != nil {
		// A shortfall of the subject itself is the command's own refusal; a
		// later operation it breaks is a named dependency.
		return nil, err
	}
	if len(projection.SubjectTransferOut) == 0 {
		return nil, fmt.Errorf("%w: replacement transfer moved nothing", ErrInvalidDisposalParams)
	}
	moved := make([]LotDisposalRecord, 0, len(projection.SubjectTransferOut))
	for _, depletion := range projection.SubjectTransferOut {
		result, err := tx.ExecContext(ctx, `INSERT INTO investment_lot_events (
			book_id, lot_id, event_kind, transaction_id, event_date, quantity_value, quantity_scale,
			cost_basis_value, cost_basis_scale, cost_basis_method, metadata_json,
			created_at, created_by_user_id, created_audit_event_id
		) VALUES (?, ?, 'transfer_out', ?, ?, ?, ?, ?, ?, NULLIF(?, ''), ?, ?, ?, ?)`,
			transfer.BookID, depletion.LotID, replacement.ID, transfer.EffectiveOn,
			depletion.QuantityValue.Negated(), depletion.QuantityScale,
			exact.New(-depletion.CostBasisValue), depletion.CostBasisScale,
			projection.SubjectTransferMethod, transfer.SourceEvidenceJSON,
			journal.CreatedAt, journal.ActorUserID, auditEventID)
		if err != nil {
			return nil, fmt.Errorf("record replacement transfer depletion: %w", err)
		}
		if depletion.EventID, err = result.LastInsertId(); err != nil {
			return nil, fmt.Errorf("read replacement transfer depletion id: %w", err)
		}
		moved = append(moved, depletion)
	}
	return moved, nil
}

// replacedTransferPositions lists every long position the replaced and the
// replacement transfer move: sources first, then destinations, each once.
func replacedTransferPositions(expected TransferOperationRecord, transfer CreateInternalTransferParams) []investmentReplayPositionKey {
	candidates := []investmentReplayPositionKey{
		{expected.SourceAccountID, expected.CommodityID, expected.CostCommodityID},
		{transfer.SourceAccountID, transfer.CommodityID, transfer.CostCommodityID},
		{expected.DestinationAccountID, expected.CommodityID, expected.CostCommodityID},
		{transfer.DestinationAccountID, transfer.CommodityID, transfer.CostCommodityID},
	}
	positions := make([]investmentReplayPositionKey, 0, len(candidates))
	for _, candidate := range candidates {
		if !slices.Contains(positions, candidate) {
			positions = append(positions, candidate)
		}
	}
	return positions
}

// TransferTerms are a transfer's committed terms, which a replacement form
// starts from. An internal transfer has its selected source lots and
// quantities, or its pooled quantity and lineage. An external transfer in has
// its quantity, carried basis and original acquisition date.
type TransferTerms struct {
	TransferKind         string
	EffectiveOn          string
	SourceAccountID      int64
	DestinationAccountID int64
	CommodityID          int64
	CostCommodityID      int64
	BasisAllocation      string
	DestinationLineage   string
	Allocations          []LotAllocation
	QuantityValue        exact.Coefficient
	QuantityScale        int
	CarriedBasisValue    exact.Coefficient
	CarriedBasisScale    int
	OriginalAcquiredOn   string
	SourceEvidenceJSON   string
}

func (r *InvestmentRepository) TransferTerms(ctx context.Context, bookID, operationID int64) (TransferTerms, error) {
	var terms TransferTerms
	var source sql.NullInt64
	var allocation, lineage sql.NullString
	err := r.database.QueryRowContext(ctx, `SELECT f.transfer_kind, f.effective_on, f.source_account_id,
		f.destination_account_id, f.commodity_id, f.basis_allocation, f.destination_lineage, f.source_evidence_json
		FROM investment_transfer_facts f
		WHERE f.book_id = ? AND f.operation_id = ? AND f.transfer_kind IN ('internal', 'external_in')`,
		bookID, operationID).Scan(&terms.TransferKind, &terms.EffectiveOn, &source, &terms.DestinationAccountID,
		&terms.CommodityID, &allocation, &lineage, &terms.SourceEvidenceJSON)
	if errors.Is(err, sql.ErrNoRows) {
		return TransferTerms{}, ErrNotFound
	}
	if err != nil {
		return TransferTerms{}, fmt.Errorf("read transfer terms: %w", err)
	}
	terms.SourceAccountID, terms.BasisAllocation, terms.DestinationLineage = source.Int64, allocation.String, lineage.String
	// The committed link rows hold what was moved: each selected lot and its
	// quantity, a pool's lots (whose quantities sum to the move), or the one
	// external lot with its carried basis and original date.
	rows, err := r.database.QueryContext(ctx, `SELECT x.source_lot_id, x.quantity_value, x.quantity_scale,
		x.cost_commodity_id, x.carried_basis_value, x.carried_basis_scale, COALESCE(x.original_acquired_on, '')
		FROM investment_transfer_lot_links x WHERE x.operation_id = ? ORDER BY x.link_seq`, operationID)
	if err != nil {
		return TransferTerms{}, fmt.Errorf("read transfer links: %w", err)
	}
	defer rows.Close()
	total, basis := exact.NewScaledInt(), exact.NewScaledInt()
	for rows.Next() {
		var sourceLot sql.NullInt64
		var link LotAllocation
		var carried sql.NullString
		var carriedScale sql.NullInt64
		if err := rows.Scan(&sourceLot, &link.QuantityValue, &link.QuantityScale, &terms.CostCommodityID,
			&carried, &carriedScale, &terms.OriginalAcquiredOn); err != nil {
			return TransferTerms{}, fmt.Errorf("scan transfer link: %w", err)
		}
		total.AddCoefficient(link.QuantityValue, link.QuantityScale)
		if carried.Valid {
			basis.AddCoefficient(exact.Coefficient(carried.String), int(carriedScale.Int64))
		}
		if terms.BasisAllocation == InternalTransferSelectedLots {
			link.LotID = sourceLot.Int64
			terms.Allocations = append(terms.Allocations, link)
		}
	}
	if err := rows.Err(); err != nil {
		return TransferTerms{}, fmt.Errorf("iterate transfer links: %w", err)
	}
	if terms.BasisAllocation != InternalTransferSelectedLots {
		if terms.QuantityValue, err = total.Coefficient(); err != nil {
			return TransferTerms{}, err
		}
		terms.QuantityScale = total.Scale()
	}
	if terms.TransferKind == "external_in" {
		if terms.CarriedBasisValue, err = basis.Coefficient(); err != nil {
			return TransferTerms{}, err
		}
		terms.CarriedBasisScale = basis.Scale()
	} else {
		terms.OriginalAcquiredOn = ""
	}
	return terms, nil
}

// ExternalTransferInReplacementRecord is the inverse and replacement journals
// and the replacement's lot.
type ExternalTransferInReplacementRecord struct {
	Inverse     TransactionRecord
	Replacement TransactionRecord
	Lot         InvestmentLotRecord
}

// ReplaceExternalTransferIn reverses an external transfer in and records
// corrected terms as its successor. The bridge difference is the inverse
// bridge plus the new one, both appended; the original journal and source
// evidence stay. The new lot opens by replay admission at the correction
// root's same-day slot, then the old and new holdings replay.
func (r *InvestmentRepository) ReplaceExternalTransferIn(ctx context.Context, expected TransferOperationRecord,
	inverseParams, replacementParams CreateTransactionParams, transfer CreateExternalTransferInParams,
) (ExternalTransferInReplacementRecord, error) {
	return r.replaceExternalTransferIn(ctx, expected, inverseParams, replacementParams, transfer, false)
}

// SimulateExternalTransferInReplacement runs the replacement writer and every
// replay, then rolls back.
func (r *InvestmentRepository) SimulateExternalTransferInReplacement(ctx context.Context, expected TransferOperationRecord,
	inverseParams, replacementParams CreateTransactionParams, transfer CreateExternalTransferInParams,
) (SimulatedInvestmentWrite, error) {
	record, err := r.replaceExternalTransferIn(ctx, expected, inverseParams, replacementParams, transfer, true)
	if err != nil {
		return SimulatedInvestmentWrite{}, err
	}
	return simulatedInvestmentWrite(record.Inverse, record.Replacement), nil
}

func (r *InvestmentRepository) replaceExternalTransferIn(ctx context.Context, expected TransferOperationRecord,
	inverseParams, replacementParams CreateTransactionParams, transfer CreateExternalTransferInParams, preview bool,
) (ExternalTransferInReplacementRecord, error) {
	lot := transfer.Lot
	if expected.OperationID <= 0 || expected.TransferKind != "external_in" ||
		inverseParams.BookID <= 0 || inverseParams.BookID != replacementParams.BookID || inverseParams.BookID != lot.BookID ||
		inverseParams.ActorUserID != replacementParams.ActorUserID || inverseParams.ActorUserID != lot.CreatedByUserID ||
		inverseParams.Spec.InvestmentOperationKind != "" ||
		inverseParams.Spec.TransactionKind != "investment" || inverseParams.Spec.Status != "posted" ||
		inverseParams.Spec.TransactionDate != expected.EventDate ||
		replacementParams.Spec.InvestmentOperationKind != "external_transfer_in" ||
		replacementParams.Spec.TransactionKind != "investment" || replacementParams.Spec.Status != "posted" ||
		replacementParams.Spec.TransactionDate != lot.OpenedOn ||
		lot.AccountID <= 0 || lot.CommodityID != expected.CommodityID || lot.CostCommodityID != expected.CostCommodityID ||
		replacementParams.InvestmentCorrectionOfOperationID != expected.OperationID ||
		replacementParams.InvestmentCorrectionMode != "replace" || replacementParams.InvestmentCorrectionReason == "" ||
		!inverseParams.CorrectionOfTransactionID.Valid || inverseParams.CorrectionOfTransactionID.Int64 != expected.TransactionID ||
		!replacementParams.CorrectionOfTransactionID.Valid || replacementParams.CorrectionOfTransactionID.Int64 != expected.TransactionID {
		return ExternalTransferInReplacementRecord{}, fmt.Errorf("%w: external transfer replacement is incomplete", ErrInvalidDisposalParams)
	}
	write := executeInvestmentJournalsWithGuardTx[InvestmentLotRecord]
	if preview {
		write = previewInvestmentJournalsWithGuardTx[InvestmentLotRecord]
	}
	journals, record, err := write(ctx, r.database,
		[]CreateTransactionParams{inverseParams, replacementParams},
		func(tx *sql.Tx) error {
			_, err := checkTransferOperationForCorrectionTx(ctx, tx, inverseParams.BookID, expected)
			return err
		}, func(tx *sql.Tx, journals []TransactionRecord, auditEventID int64) (InvestmentLotRecord, error) {
			inverse, replacement := journals[0], journals[1]
			operationID, err := investmentOperationIDTx(ctx, tx, lot.BookID, replacement.ID)
			if err != nil {
				return InvestmentLotRecord{}, err
			}
			if _, err := tx.ExecContext(ctx, `INSERT INTO investment_operation_journal_links
				(book_id, operation_id, transaction_version_id, link_seq, role)
				VALUES (?, ?, ?, 2, 'reversal')`, lot.BookID, operationID, inverse.VersionID); err != nil {
				return InvestmentLotRecord{}, fmt.Errorf("link transfer replacement inverse: %w", err)
			}
			opened, err := writeExternalTransferInTx(ctx, tx, replacementParams, transfer, replacement, auditEventID, true)
			if err != nil {
				return InvestmentLotRecord{}, err
			}
			for _, position := range correctedTradePositions(
				investmentReplayPositionKey{expected.DestinationAccountID, expected.CommodityID, expected.CostCommodityID},
				investmentReplayPositionKey{lot.AccountID, lot.CommodityID, lot.CostCommodityID}) {
				if err := replayCorrectedPositionTx(ctx, tx, lot.BookID, position, operationID, auditEventID,
					replacementParams.ActorUserID, replacementParams.CreatedAt); err != nil {
					return InvestmentLotRecord{}, err
				}
			}
			return investmentLotByIDTx(ctx, tx, lot.BookID, opened.ID)
		}, nil)
	if err != nil {
		return ExternalTransferInReplacementRecord{}, err
	}
	return ExternalTransferInReplacementRecord{Inverse: journals[0], Replacement: journals[1], Lot: record}, nil
}
