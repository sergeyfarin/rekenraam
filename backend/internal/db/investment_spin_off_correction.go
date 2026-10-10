package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"rekenraam/backend/internal/exact"
)

// Spin-off correction (#183), following the share exchange correction (#179)
// and ADR 0013 *Transfer Correction Refinement*. A reversal posts the exact
// inverse of the spin-off journal, removes the spin-off from effective history
// and replays the parent, whose lots get their basis back, then the new
// holding, which loses the lots the spin-off opened. A replacement does the
// same and records corrected terms at the replaced spin-off's correction-root
// slot. The original journal, fact, links, link revisions and lot events stay
// immutable evidence; every effect commits under one audit event.

// ErrSpinOffOperationChanged means the spin-off or its journal moved after
// its correction was planned.
var ErrSpinOffOperationChanged = errors.New("spin-off changed after its correction was prepared")

// SpinOffOperationRecord pins a posted spin-off and the holdings it touched.
// A correction rechecks it inside the write.
type SpinOffOperationRecord struct {
	OperationID            int64
	TransactionID          int64
	TransactionVersionID   int64
	CurrentVersionID       int64
	EventDate              string
	AccountID              int64
	DestinationAccountID   int64
	CommodityID            int64
	DestinationCommodityID int64
	RatioNumerator         int64
	RatioDenominator       int64
	BasisFractionValue     exact.Coefficient
	BasisFractionScale     int
	SourceEvidenceJSON     string
	AlreadyCorrected       bool
	ImportedLineage        bool
}

func (r *InvestmentRepository) SpinOffOperationByTransactionID(ctx context.Context, bookID, transactionID int64) (SpinOffOperationRecord, error) {
	return spinOffOperationByTransactionIDQuery(ctx, r.database, bookID, transactionID)
}

func spinOffOperationByTransactionIDQuery(ctx context.Context, reader transferOperationReader, bookID, transactionID int64) (SpinOffOperationRecord, error) {
	var record SpinOffOperationRecord
	var corrected int
	err := reader.QueryRowContext(ctx, `SELECT o.id, linked_version.transaction_id, link.transaction_version_id,
		current.id, f.effective_on, f.source_account_id, f.destination_account_id, f.commodity_id,
		f.destination_commodity_id, f.ratio_numerator, f.ratio_denominator, f.basis_fraction_value, f.basis_fraction_scale,
		f.source_evidence_json, EXISTS(SELECT 1 FROM investment_operations successor WHERE successor.correction_of_operation_id = o.id)
		FROM investment_operations o
		JOIN investment_transfer_facts f ON f.operation_id = o.id AND f.book_id = o.book_id AND f.transfer_kind = 'spin_off'
		JOIN investment_operation_journal_links link ON link.operation_id = o.id AND link.book_id = o.book_id AND link.role = 'primary'
		JOIN transaction_versions linked_version ON linked_version.id = link.transaction_version_id
		JOIN current_transaction_versions current ON current.transaction_id = linked_version.transaction_id
		WHERE o.book_id = ? AND linked_version.transaction_id = ? AND o.operation_kind = 'spin_off'`,
		bookID, transactionID).Scan(&record.OperationID, &record.TransactionID, &record.TransactionVersionID,
		&record.CurrentVersionID, &record.EventDate, &record.AccountID, &record.DestinationAccountID,
		&record.CommodityID, &record.DestinationCommodityID, &record.RatioNumerator, &record.RatioDenominator,
		&record.BasisFractionValue, &record.BasisFractionScale, &record.SourceEvidenceJSON, &corrected)
	if errors.Is(err, sql.ErrNoRows) {
		return SpinOffOperationRecord{}, ErrNotFound
	}
	if err != nil {
		return SpinOffOperationRecord{}, fmt.Errorf("read spin-off operation: %w", err)
	}
	record.AlreadyCorrected = corrected != 0
	if record.ImportedLineage, err = investmentOperationHasImportedLineageQuery(ctx, reader, bookID, record.OperationID); err != nil {
		return SpinOffOperationRecord{}, err
	}
	return record, nil
}

// checkSpinOffForCorrectionTx applies the shared correction fences: not
// already corrected, an imported lineage still names its committed source,
// and neither the spin-off nor its journal moved since it was planned.
func checkSpinOffForCorrectionTx(ctx context.Context, tx *sql.Tx, bookID int64, expected SpinOffOperationRecord) error {
	current, err := spinOffOperationByTransactionIDQuery(ctx, tx, bookID, expected.TransactionID)
	if err != nil {
		return err
	}
	if current.AlreadyCorrected {
		return ErrInvestmentOperationAlreadyCorrected
	}
	if current.ImportedLineage {
		linked, err := investmentOperationHasCommittedSourceQuery(ctx, tx, bookID, current.OperationID)
		if err != nil {
			return err
		}
		if !linked {
			return ErrInvestmentImportedCorrection
		}
	}
	if current != expected {
		return ErrSpinOffOperationChanged
	}
	if err := checkInvestmentSourceJournalTx(ctx, tx, bookID, current.TransactionID,
		current.EventDate, current.TransactionVersionID, current.CurrentVersionID); err != nil {
		if errors.Is(err, ErrInvestmentSaleChanged) {
			return ErrSpinOffOperationChanged
		}
		return err
	}
	return nil
}

// holdings lists the parent holding, then the new one.
func (e SpinOffOperationRecord) holdings() [][2]int64 {
	return [][2]int64{{e.AccountID, e.CommodityID}, {e.DestinationAccountID, e.DestinationCommodityID}}
}

// ReverseSpinOff posts the inverse journal and removes the spin-off from
// effective history, replaying both holdings under one audit event.
func (r *InvestmentRepository) ReverseSpinOff(ctx context.Context, params CreateTransactionParams, expected SpinOffOperationRecord) (TransactionRecord, error) {
	return r.reverseSpinOff(ctx, params, expected, false)
}

// PreviewSpinOffReversal runs the reversal writer, both replays, gain
// comparison and checkpoint invalidation, then rolls back.
func (r *InvestmentRepository) PreviewSpinOffReversal(ctx context.Context, params CreateTransactionParams, expected SpinOffOperationRecord) (SimulatedInvestmentWrite, error) {
	transaction, err := r.reverseSpinOff(ctx, params, expected, true)
	if err != nil {
		return SimulatedInvestmentWrite{}, err
	}
	return simulatedInvestmentWrite(transaction), nil
}

func (r *InvestmentRepository) reverseSpinOff(ctx context.Context, params CreateTransactionParams, expected SpinOffOperationRecord, preview bool) (TransactionRecord, error) {
	if params.BookID <= 0 || params.ActorUserID <= 0 || expected.OperationID <= 0 ||
		params.Spec.InvestmentOperationKind != "reversal" || params.Spec.TransactionKind != "investment" ||
		params.Spec.Status != "posted" || params.Spec.TransactionDate != expected.EventDate ||
		params.InvestmentCorrectionOfOperationID != expected.OperationID ||
		params.InvestmentCorrectionMode != "reverse" || params.InvestmentCorrectionReason == "" ||
		!params.CorrectionOfTransactionID.Valid || params.CorrectionOfTransactionID.Int64 != expected.TransactionID {
		return TransactionRecord{}, fmt.Errorf("%w: spin-off reversal is incomplete", ErrInvalidDisposalParams)
	}
	guard := func(tx *sql.Tx) error {
		return checkSpinOffForCorrectionTx(ctx, tx, params.BookID, expected)
	}
	effect := func(tx *sql.Tx, transaction TransactionRecord, auditEventID int64) (struct{}, error) {
		operationID, err := investmentOperationIDTx(ctx, tx, params.BookID, transaction.ID)
		if err != nil {
			return struct{}{}, err
		}
		return struct{}{}, replayHoldingsTx(ctx, tx, params.BookID, expected.holdings(),
			operationID, auditEventID, params.ActorUserID, params.CreatedAt)
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

// SpinOffReplacementRecord is the inverse and replacement journals and what
// the replacement spin-off divided.
type SpinOffReplacementRecord struct {
	Inverse     TransactionRecord
	Replacement TransactionRecord
	Plan        SpinOffPlan
}

// ReplaceSpinOff reverses a spin-off and records corrected terms as its
// successor at the correction root's same-day slot, replaying every holding
// either spin-off touched under one audit event.
func (r *InvestmentRepository) ReplaceSpinOff(ctx context.Context, expected SpinOffOperationRecord,
	inverseParams, replacementParams CreateTransactionParams, spinOff CreateSpinOffParams,
) (SpinOffReplacementRecord, error) {
	return r.replaceSpinOff(ctx, expected, inverseParams, replacementParams, spinOff, false)
}

// SimulateSpinOffReplacement runs the replacement writer and every replay,
// then rolls back. New lot IDs are cleared because they were never durable.
func (r *InvestmentRepository) SimulateSpinOffReplacement(ctx context.Context, expected SpinOffOperationRecord,
	inverseParams, replacementParams CreateTransactionParams, spinOff CreateSpinOffParams,
) (SimulatedInvestmentWrite, SpinOffPlan, error) {
	record, err := r.replaceSpinOff(ctx, expected, inverseParams, replacementParams, spinOff, true)
	if err != nil {
		return SimulatedInvestmentWrite{}, SpinOffPlan{}, err
	}
	for index := range record.Plan.Links {
		record.Plan.Links[index].DestinationLotID = 0
	}
	return simulatedInvestmentWrite(record.Inverse, record.Replacement), record.Plan, nil
}

func (r *InvestmentRepository) replaceSpinOff(ctx context.Context, expected SpinOffOperationRecord,
	inverseParams, replacementParams CreateTransactionParams, spinOff CreateSpinOffParams, preview bool,
) (SpinOffReplacementRecord, error) {
	if expected.OperationID <= 0 || inverseParams.BookID <= 0 ||
		inverseParams.BookID != replacementParams.BookID || inverseParams.BookID != spinOff.BookID ||
		inverseParams.ActorUserID != replacementParams.ActorUserID ||
		inverseParams.Spec.InvestmentOperationKind != "" ||
		inverseParams.Spec.TransactionKind != "investment" || inverseParams.Spec.Status != "posted" ||
		inverseParams.Spec.TransactionDate != expected.EventDate ||
		replacementParams.Spec.InvestmentOperationKind != "spin_off" ||
		replacementParams.Spec.TransactionKind != "investment" || replacementParams.Spec.Status != "posted" ||
		replacementParams.Spec.TransactionDate != spinOff.EffectiveOn ||
		spinOff.AccountID != expected.AccountID || spinOff.CommodityID != expected.CommodityID ||
		spinOff.ReplacesOperationID != expected.OperationID ||
		replacementParams.InvestmentCorrectionOfOperationID != expected.OperationID ||
		replacementParams.InvestmentCorrectionMode != "replace" || replacementParams.InvestmentCorrectionReason == "" ||
		!inverseParams.CorrectionOfTransactionID.Valid || inverseParams.CorrectionOfTransactionID.Int64 != expected.TransactionID ||
		!replacementParams.CorrectionOfTransactionID.Valid || replacementParams.CorrectionOfTransactionID.Int64 != expected.TransactionID {
		return SpinOffReplacementRecord{}, fmt.Errorf("%w: spin-off replacement is incomplete", ErrInvalidDisposalParams)
	}
	write := executeInvestmentJournalsWithGuardTx[SpinOffPlan]
	if preview {
		write = previewInvestmentJournalsWithGuardTx[SpinOffPlan]
	}
	journals, plan, err := write(ctx, r.database,
		[]CreateTransactionParams{inverseParams, replacementParams},
		func(tx *sql.Tx) error {
			return checkSpinOffForCorrectionTx(ctx, tx, inverseParams.BookID, expected)
		}, func(tx *sql.Tx, journals []TransactionRecord, auditEventID int64) (SpinOffPlan, error) {
			inverse, replacement := journals[0], journals[1]
			operationID, err := investmentOperationIDTx(ctx, tx, spinOff.BookID, replacement.ID)
			if err != nil {
				return SpinOffPlan{}, err
			}
			if _, err := tx.ExecContext(ctx, `INSERT INTO investment_operation_journal_links
				(book_id, operation_id, transaction_version_id, link_seq, role)
				VALUES (?, ?, ?, 2, 'reversal')`, spinOff.BookID, operationID, inverse.VersionID); err != nil {
				return SpinOffPlan{}, fmt.Errorf("link spin-off replacement inverse: %w", err)
			}
			// The writer replays the parent and the replacement's new holding;
			// a replaced new holding elsewhere loses its lots here.
			plan, err := writeSpinOffTx(ctx, tx, spinOff, replacement, replacementParams, auditEventID)
			if err != nil {
				return SpinOffPlan{}, err
			}
			replaced := [2]int64{expected.DestinationAccountID, expected.DestinationCommodityID}
			if replaced == [2]int64{spinOff.DestinationAccountID, spinOff.DestinationCommodityID} {
				return plan, nil
			}
			return plan, replayHoldingsTx(ctx, tx, spinOff.BookID, [][2]int64{replaced},
				operationID, auditEventID, replacementParams.ActorUserID, replacementParams.CreatedAt)
		}, nil)
	if err != nil {
		return SpinOffReplacementRecord{}, err
	}
	return SpinOffReplacementRecord{Inverse: journals[0], Replacement: journals[1], Plan: plan}, nil
}
