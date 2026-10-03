package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"rekenraam/backend/internal/exact"
)

// Native split reversal and replacement (T-129). A reversal posts the exact
// inverse of everything the split's journals currently move (its primary
// journal plus every adjustment), removes the split from the effective
// history and replays each cost-currency position of the holding. A
// replacement does the same and records a new split, with corrected terms, at
// the replaced split's correction-root slot. Original facts, effects and
// journals stay immutable; every effect commits under one audit event.

// SplitOperationRecord pins a posted split and the aggregate quantity its
// journals currently move. A correction rechecks it inside the write.
type SplitOperationRecord struct {
	OperationID          int64
	TransactionID        int64
	TransactionVersionID int64
	CurrentVersionID     int64
	EventDate            string
	AccountID            int64
	CommodityID          int64
	RatioNumerator       int64
	RatioDenominator     int64
	// JournalDelta is the split's primary plus adjustment journal quantity on
	// its holding, normalized so records compare by value.
	JournalDeltaValue exact.Coefficient
	JournalDeltaScale int
	AlreadyCorrected  bool
	ImportedLineage   bool
}

func (r *InvestmentRepository) SplitOperationByTransactionID(ctx context.Context, bookID, transactionID int64) (SplitOperationRecord, error) {
	return splitOperationByTransactionIDQuery(ctx, r.database, bookID, transactionID)
}

// splitOperationReader reads a split from the pool or inside a write.
type splitOperationReader interface {
	saleOperationReader
	queryer
}

func splitOperationByTransactionIDQuery(ctx context.Context, reader splitOperationReader, bookID, transactionID int64) (SplitOperationRecord, error) {
	var record SplitOperationRecord
	var corrected int
	err := reader.QueryRowContext(ctx, `SELECT o.id, linked_version.transaction_id, link.transaction_version_id,
		current.id, f.effective_on, f.account_id, f.commodity_id, f.ratio_numerator, f.ratio_denominator,
		EXISTS(SELECT 1 FROM investment_operations successor WHERE successor.correction_of_operation_id = o.id)
		FROM investment_operations o
		JOIN investment_split_facts f ON f.operation_id = o.id AND f.book_id = o.book_id
		JOIN investment_operation_journal_links link ON link.operation_id = o.id AND link.book_id = o.book_id AND link.role = 'primary'
		JOIN transaction_versions linked_version ON linked_version.id = link.transaction_version_id
		JOIN current_transaction_versions current ON current.transaction_id = linked_version.transaction_id
		WHERE o.book_id = ? AND linked_version.transaction_id = ? AND o.operation_kind = 'split'`,
		bookID, transactionID).Scan(&record.OperationID, &record.TransactionID, &record.TransactionVersionID,
		&record.CurrentVersionID, &record.EventDate, &record.AccountID, &record.CommodityID,
		&record.RatioNumerator, &record.RatioDenominator, &corrected)
	if errors.Is(err, sql.ErrNoRows) {
		return SplitOperationRecord{}, ErrNotFound
	}
	if err != nil {
		return SplitOperationRecord{}, fmt.Errorf("read investment split operation: %w", err)
	}
	record.AlreadyCorrected = corrected != 0
	if record.ImportedLineage, err = investmentOperationHasImportedLineageQuery(ctx, reader, bookID, record.OperationID); err != nil {
		return SplitOperationRecord{}, err
	}
	rows, err := reader.QueryContext(ctx, `SELECT pv.quantity_value, pv.quantity_scale
		FROM investment_operation_journal_links link
		JOIN posting_versions pv ON pv.transaction_version_id = link.transaction_version_id
		WHERE link.operation_id = ? AND link.book_id = ? AND link.role IN ('primary', 'split_adjustment')
			AND pv.account_id = ? AND pv.commodity_id = ?`,
		record.OperationID, bookID, record.AccountID, record.CommodityID)
	if err != nil {
		return SplitOperationRecord{}, fmt.Errorf("read split journal quantities: %w", err)
	}
	delta := exact.NewScaledInt()
	for rows.Next() {
		var value exact.Coefficient
		var scale int
		if err := rows.Scan(&value, &scale); err != nil {
			rows.Close()
			return SplitOperationRecord{}, fmt.Errorf("scan split journal quantity: %w", err)
		}
		delta.AddCoefficient(value, scale)
	}
	if err := errors.Join(rows.Err(), rows.Close()); err != nil {
		return SplitOperationRecord{}, fmt.Errorf("read split journal quantities: %w", err)
	}
	normalized := delta.Normalized()
	if record.JournalDeltaValue, err = normalized.Coefficient(); err != nil {
		return SplitOperationRecord{}, err
	}
	record.JournalDeltaScale = normalized.Scale()
	return record, nil
}

// checkSplitOperationForCorrectionTx applies the shared correction fences:
// not already corrected, an imported lineage still names its committed source,
// and nothing about the split or its journals moved since it was planned.
func checkSplitOperationForCorrectionTx(ctx context.Context, tx *sql.Tx, bookID int64, expected SplitOperationRecord) (SplitOperationRecord, error) {
	current, err := splitOperationByTransactionIDQuery(ctx, tx, bookID, expected.TransactionID)
	if err != nil {
		return SplitOperationRecord{}, err
	}
	if current.AlreadyCorrected {
		return SplitOperationRecord{}, ErrInvestmentOperationAlreadyCorrected
	}
	if current.ImportedLineage {
		linked, err := investmentOperationHasCommittedSourceQuery(ctx, tx, bookID, current.OperationID)
		if err != nil {
			return SplitOperationRecord{}, err
		}
		if !linked {
			return SplitOperationRecord{}, ErrInvestmentImportedCorrection
		}
	}
	if current != expected {
		return SplitOperationRecord{}, ErrSplitPositionChanged
	}
	if err := checkInvestmentSourceJournalTx(ctx, tx, bookID, current.TransactionID,
		current.EventDate, current.TransactionVersionID, current.CurrentVersionID); err != nil {
		return SplitOperationRecord{}, err
	}
	return current, nil
}

// replaySplitHoldingTx replays every long cost-currency position of a holding
// against its effective intents and persists the result. A later disposal the
// corrected history cannot satisfy is returned as a named dependency.
func replaySplitHoldingTx(ctx context.Context, tx *sql.Tx, bookID, accountID, commodityID, causedByOperationID,
	auditEventID, actorUserID int64, createdAt string) error {
	rows, err := tx.QueryContext(ctx, `SELECT DISTINCT cost_commodity_id FROM investment_lots
		WHERE book_id = ? AND account_id = ? AND commodity_id = ? AND position_side = 'long'
		ORDER BY cost_commodity_id`, bookID, accountID, commodityID)
	if err != nil {
		return fmt.Errorf("read split holding currencies: %w", err)
	}
	var costIDs []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return fmt.Errorf("scan split holding currency: %w", err)
		}
		costIDs = append(costIDs, id)
	}
	if err := errors.Join(rows.Err(), rows.Close()); err != nil {
		return fmt.Errorf("read split holding currencies: %w", err)
	}
	for _, costID := range costIDs {
		intents, err := investmentReplayIntentsQuery(ctx, tx, bookID, accountID, commodityID, costID, "long")
		if err != nil {
			return err
		}
		projection, err := simulateInvestmentReplayTx(ctx, tx, bookID, accountID, commodityID, costID, intents)
		if err != nil {
			return err
		}
		if err := persistInvestmentReplayProjectionTx(ctx, tx, bookID, accountID, commodityID, costID,
			causedByOperationID, auditEventID, actorUserID, createdAt, intents, projection); err != nil {
			return err
		}
	}
	return nil
}

// ReverseSplit posts the inverse of the split's current journal delta and
// removes the split from effective history, replaying the whole holding.
func (r *InvestmentRepository) ReverseSplit(ctx context.Context, params CreateTransactionParams, expected SplitOperationRecord) (TransactionRecord, error) {
	return r.reverseSplit(ctx, params, expected, false)
}

// PreviewSplitReversal runs the reversal writer, its replay, gain comparison
// and checkpoint invalidation, then rolls back.
func (r *InvestmentRepository) PreviewSplitReversal(ctx context.Context, params CreateTransactionParams, expected SplitOperationRecord) (SimulatedInvestmentWrite, error) {
	transaction, err := r.reverseSplit(ctx, params, expected, true)
	if err != nil {
		return SimulatedInvestmentWrite{}, err
	}
	return simulatedInvestmentWrite(transaction), nil
}

func (r *InvestmentRepository) reverseSplit(ctx context.Context, params CreateTransactionParams, expected SplitOperationRecord, preview bool) (TransactionRecord, error) {
	if params.BookID <= 0 || params.ActorUserID <= 0 || expected.OperationID <= 0 ||
		params.Spec.InvestmentOperationKind != "reversal" || params.Spec.TransactionKind != "investment" ||
		params.Spec.Status != "posted" || params.Spec.TransactionDate != expected.EventDate ||
		params.InvestmentCorrectionOfOperationID != expected.OperationID ||
		params.InvestmentCorrectionMode != "reverse" || params.InvestmentCorrectionReason == "" ||
		!params.CorrectionOfTransactionID.Valid || params.CorrectionOfTransactionID.Int64 != expected.TransactionID {
		return TransactionRecord{}, fmt.Errorf("%w: split reversal is incomplete", ErrInvalidDisposalParams)
	}
	if err := requireSplitInverseJournal(params, expected); err != nil {
		return TransactionRecord{}, err
	}
	guard := func(tx *sql.Tx) error {
		_, err := checkSplitOperationForCorrectionTx(ctx, tx, params.BookID, expected)
		return err
	}
	effect := func(tx *sql.Tx, transaction TransactionRecord, auditEventID int64) (struct{}, error) {
		operationID, err := investmentOperationIDTx(ctx, tx, params.BookID, transaction.ID)
		if err != nil {
			return struct{}{}, err
		}
		return struct{}{}, replaySplitHoldingTx(ctx, tx, params.BookID, expected.AccountID, expected.CommodityID,
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

// requireSplitInverseJournal checks that a correction's inverse journal moves
// exactly the negation of the split's current journal delta on its holding.
func requireSplitInverseJournal(params CreateTransactionParams, expected SplitOperationRecord) error {
	holding := exact.NewScaledInt()
	for _, entry := range params.Spec.JournalEntries {
		if entry.EntryDate != expected.EventDate {
			return fmt.Errorf("%w: split inverse must be dated to the split", ErrInvalidDisposalParams)
		}
		for _, posting := range entry.Postings {
			if posting.AccountID == expected.AccountID && posting.CommodityID == expected.CommodityID {
				holding.AddCoefficient(posting.QuantityValue, posting.QuantityScale)
			}
		}
	}
	holding.AddCoefficient(expected.JournalDeltaValue, expected.JournalDeltaScale)
	if holding.Sign() != 0 {
		return fmt.Errorf("%w: split inverse does not cancel the split's journal delta", ErrInvalidDisposalParams)
	}
	return nil
}

// SplitReplacementRecord is the inverse and replacement journals and the
// replacement split's committed effects.
type SplitReplacementRecord struct {
	Inverse     TransactionRecord
	Replacement TransactionRecord
	Plan        SplitPlan
}

// ReplaceSplit reverses a split and records corrected terms as its successor,
// replaying the holding once under one audit event.
func (r *InvestmentRepository) ReplaceSplit(ctx context.Context, expected SplitOperationRecord,
	inverseParams, replacementParams CreateTransactionParams, split CreateSplitParams,
) (SplitReplacementRecord, error) {
	return r.replaceSplit(ctx, expected, inverseParams, replacementParams, split, false)
}

// SimulateSplitReplacement runs the replacement writer and rolls back.
func (r *InvestmentRepository) SimulateSplitReplacement(ctx context.Context, expected SplitOperationRecord,
	inverseParams, replacementParams CreateTransactionParams, split CreateSplitParams,
) (SimulatedInvestmentWrite, error) {
	record, err := r.replaceSplit(ctx, expected, inverseParams, replacementParams, split, true)
	if err != nil {
		return SimulatedInvestmentWrite{}, err
	}
	return simulatedInvestmentWrite(record.Inverse, record.Replacement), nil
}

func (r *InvestmentRepository) replaceSplit(ctx context.Context, expected SplitOperationRecord,
	inverseParams, replacementParams CreateTransactionParams, split CreateSplitParams, preview bool,
) (SplitReplacementRecord, error) {
	if expected.OperationID <= 0 || inverseParams.BookID <= 0 ||
		inverseParams.BookID != replacementParams.BookID || inverseParams.BookID != split.BookID ||
		inverseParams.ActorUserID != replacementParams.ActorUserID ||
		inverseParams.Spec.InvestmentOperationKind != "" ||
		inverseParams.Spec.TransactionKind != "investment" || inverseParams.Spec.Status != "posted" ||
		replacementParams.Spec.InvestmentOperationKind != "split" ||
		replacementParams.Spec.TransactionKind != "investment" || replacementParams.Spec.Status != "posted" ||
		inverseParams.Spec.TransactionDate != expected.EventDate ||
		replacementParams.Spec.TransactionDate != split.EffectiveOn ||
		split.AccountID != expected.AccountID || split.CommodityID != expected.CommodityID ||
		split.ReplacesOperationID != expected.OperationID ||
		replacementParams.InvestmentCorrectionOfOperationID != expected.OperationID ||
		replacementParams.InvestmentCorrectionMode != "replace" ||
		replacementParams.InvestmentCorrectionReason == "" ||
		!inverseParams.CorrectionOfTransactionID.Valid ||
		inverseParams.CorrectionOfTransactionID.Int64 != expected.TransactionID ||
		!replacementParams.CorrectionOfTransactionID.Valid ||
		replacementParams.CorrectionOfTransactionID.Int64 != expected.TransactionID {
		return SplitReplacementRecord{}, fmt.Errorf("%w: split replacement is incomplete", ErrInvalidDisposalParams)
	}
	if err := requireSplitInverseJournal(inverseParams, expected); err != nil {
		return SplitReplacementRecord{}, err
	}
	write := executeInvestmentJournalsWithGuardTx[SplitPlan]
	if preview {
		write = previewInvestmentJournalsWithGuardTx[SplitPlan]
	}
	journals, plan, err := write(ctx, r.database,
		[]CreateTransactionParams{inverseParams, replacementParams},
		func(tx *sql.Tx) error {
			_, err := checkSplitOperationForCorrectionTx(ctx, tx, inverseParams.BookID, expected)
			return err
		}, func(tx *sql.Tx, journals []TransactionRecord, auditEventID int64) (SplitPlan, error) {
			inverse, replacement := journals[0], journals[1]
			operationID, err := investmentOperationIDTx(ctx, tx, replacementParams.BookID, replacement.ID)
			if err != nil {
				return SplitPlan{}, err
			}
			if _, err := tx.ExecContext(ctx, `INSERT INTO investment_operation_journal_links
				(book_id, operation_id, transaction_version_id, link_seq, role)
				VALUES (?, ?, ?, 2, 'reversal')`, replacementParams.BookID, operationID, inverse.VersionID); err != nil {
				return SplitPlan{}, fmt.Errorf("link split replacement inverse: %w", err)
			}
			return writeSplitEffectsTx(ctx, tx, split, replacement, auditEventID,
				replacementParams.ActorUserID, replacementParams.CreatedAt)
		}, nil)
	if err != nil {
		return SplitReplacementRecord{}, err
	}
	return SplitReplacementRecord{Inverse: journals[0], Replacement: journals[1], Plan: plan}, nil
}
