package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// Share exchange correction (#179), following the transfer correction pattern
// (ADR 0013 *Transfer Correction Refinement*). A reversal posts the exact
// inverse of the exchange journal, removes the exchange from effective history
// and replays the old holding, which gets its lots back, then the new one,
// which loses the lots the exchange opened. A replacement does the same and
// records corrected terms at the replaced exchange's correction-root slot.
// The original journal, fact, links, link revisions and lot events stay
// immutable evidence; every effect commits under one audit event.

// ErrShareExchangeOperationChanged means the exchange or its journal moved
// after its correction was planned.
var ErrShareExchangeOperationChanged = errors.New("share exchange changed after its correction was prepared")

// ShareExchangeOperationRecord pins a posted exchange and the holdings it
// moved. A correction rechecks it inside the write.
type ShareExchangeOperationRecord struct {
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
	AlreadyCorrected       bool
	ImportedLineage        bool
}

func (r *InvestmentRepository) ShareExchangeOperationByTransactionID(ctx context.Context, bookID, transactionID int64) (ShareExchangeOperationRecord, error) {
	return shareExchangeOperationByTransactionIDQuery(ctx, r.database, bookID, transactionID)
}

func shareExchangeOperationByTransactionIDQuery(ctx context.Context, reader transferOperationReader, bookID, transactionID int64) (ShareExchangeOperationRecord, error) {
	var record ShareExchangeOperationRecord
	var corrected int
	err := reader.QueryRowContext(ctx, `SELECT o.id, linked_version.transaction_id, link.transaction_version_id,
		current.id, f.effective_on, f.source_account_id, f.destination_account_id, f.commodity_id,
		f.destination_commodity_id, f.ratio_numerator, f.ratio_denominator,
		EXISTS(SELECT 1 FROM investment_operations successor WHERE successor.correction_of_operation_id = o.id)
		FROM investment_operations o
		JOIN investment_transfer_facts f ON f.operation_id = o.id AND f.book_id = o.book_id AND f.transfer_kind = 'exchange'
		JOIN investment_operation_journal_links link ON link.operation_id = o.id AND link.book_id = o.book_id AND link.role = 'primary'
		JOIN transaction_versions linked_version ON linked_version.id = link.transaction_version_id
		JOIN current_transaction_versions current ON current.transaction_id = linked_version.transaction_id
		WHERE o.book_id = ? AND linked_version.transaction_id = ? AND o.operation_kind = 'share_exchange'`,
		bookID, transactionID).Scan(&record.OperationID, &record.TransactionID, &record.TransactionVersionID,
		&record.CurrentVersionID, &record.EventDate, &record.AccountID, &record.DestinationAccountID,
		&record.CommodityID, &record.DestinationCommodityID, &record.RatioNumerator, &record.RatioDenominator, &corrected)
	if errors.Is(err, sql.ErrNoRows) {
		return ShareExchangeOperationRecord{}, ErrNotFound
	}
	if err != nil {
		return ShareExchangeOperationRecord{}, fmt.Errorf("read share exchange operation: %w", err)
	}
	record.AlreadyCorrected = corrected != 0
	if record.ImportedLineage, err = investmentOperationHasImportedLineageQuery(ctx, reader, bookID, record.OperationID); err != nil {
		return ShareExchangeOperationRecord{}, err
	}
	return record, nil
}

// checkShareExchangeForCorrectionTx applies the shared correction fences: not
// already corrected, an imported lineage still names its committed source,
// and neither the exchange nor its journal moved since it was planned.
func checkShareExchangeForCorrectionTx(ctx context.Context, tx *sql.Tx, bookID int64, expected ShareExchangeOperationRecord) error {
	current, err := shareExchangeOperationByTransactionIDQuery(ctx, tx, bookID, expected.TransactionID)
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
		return ErrShareExchangeOperationChanged
	}
	if err := checkInvestmentSourceJournalTx(ctx, tx, bookID, current.TransactionID,
		current.EventDate, current.TransactionVersionID, current.CurrentVersionID); err != nil {
		if errors.Is(err, ErrInvestmentSaleChanged) {
			return ErrShareExchangeOperationChanged
		}
		return err
	}
	return nil
}

// holdings lists the old holding, then the new one.
func (e ShareExchangeOperationRecord) holdings() [][2]int64 {
	return [][2]int64{{e.AccountID, e.CommodityID}, {e.DestinationAccountID, e.DestinationCommodityID}}
}

// ReverseShareExchange posts the inverse journal and removes the exchange
// from effective history, replaying both holdings under one audit event.
func (r *InvestmentRepository) ReverseShareExchange(ctx context.Context, params CreateTransactionParams, expected ShareExchangeOperationRecord) (TransactionRecord, error) {
	return r.reverseShareExchange(ctx, params, expected, false)
}

// PreviewShareExchangeReversal runs the reversal writer, both replays, gain
// comparison and checkpoint invalidation, then rolls back.
func (r *InvestmentRepository) PreviewShareExchangeReversal(ctx context.Context, params CreateTransactionParams, expected ShareExchangeOperationRecord) (SimulatedInvestmentWrite, error) {
	transaction, err := r.reverseShareExchange(ctx, params, expected, true)
	if err != nil {
		return SimulatedInvestmentWrite{}, err
	}
	return simulatedInvestmentWrite(transaction), nil
}

func (r *InvestmentRepository) reverseShareExchange(ctx context.Context, params CreateTransactionParams, expected ShareExchangeOperationRecord, preview bool) (TransactionRecord, error) {
	if params.BookID <= 0 || params.ActorUserID <= 0 || expected.OperationID <= 0 ||
		params.Spec.InvestmentOperationKind != "reversal" || params.Spec.TransactionKind != "investment" ||
		params.Spec.Status != "posted" || params.Spec.TransactionDate != expected.EventDate ||
		params.InvestmentCorrectionOfOperationID != expected.OperationID ||
		params.InvestmentCorrectionMode != "reverse" || params.InvestmentCorrectionReason == "" ||
		!params.CorrectionOfTransactionID.Valid || params.CorrectionOfTransactionID.Int64 != expected.TransactionID {
		return TransactionRecord{}, fmt.Errorf("%w: share exchange reversal is incomplete", ErrInvalidDisposalParams)
	}
	guard := func(tx *sql.Tx) error {
		return checkShareExchangeForCorrectionTx(ctx, tx, params.BookID, expected)
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

// ShareExchangeReplacementRecord is the inverse and replacement journals and
// what the replacement exchange moved.
type ShareExchangeReplacementRecord struct {
	Inverse     TransactionRecord
	Replacement TransactionRecord
	Plan        ShareExchangePlan
}

// ReplaceShareExchange reverses an exchange and records corrected terms as its
// successor at the correction root's same-day slot, replaying every holding
// either exchange moved under one audit event.
func (r *InvestmentRepository) ReplaceShareExchange(ctx context.Context, expected ShareExchangeOperationRecord,
	inverseParams, replacementParams CreateTransactionParams, exchange CreateShareExchangeParams,
) (ShareExchangeReplacementRecord, error) {
	return r.replaceShareExchange(ctx, expected, inverseParams, replacementParams, exchange, false)
}

// SimulateShareExchangeReplacement runs the replacement writer and every
// replay, then rolls back. Destination lot IDs are cleared because they were
// never durable.
func (r *InvestmentRepository) SimulateShareExchangeReplacement(ctx context.Context, expected ShareExchangeOperationRecord,
	inverseParams, replacementParams CreateTransactionParams, exchange CreateShareExchangeParams,
) (SimulatedInvestmentWrite, ShareExchangePlan, error) {
	record, err := r.replaceShareExchange(ctx, expected, inverseParams, replacementParams, exchange, true)
	if err != nil {
		return SimulatedInvestmentWrite{}, ShareExchangePlan{}, err
	}
	for index := range record.Plan.Links {
		record.Plan.Links[index].DestinationLotID = 0
	}
	return simulatedInvestmentWrite(record.Inverse, record.Replacement), record.Plan, nil
}

func (r *InvestmentRepository) replaceShareExchange(ctx context.Context, expected ShareExchangeOperationRecord,
	inverseParams, replacementParams CreateTransactionParams, exchange CreateShareExchangeParams, preview bool,
) (ShareExchangeReplacementRecord, error) {
	if expected.OperationID <= 0 || inverseParams.BookID <= 0 ||
		inverseParams.BookID != replacementParams.BookID || inverseParams.BookID != exchange.BookID ||
		inverseParams.ActorUserID != replacementParams.ActorUserID ||
		inverseParams.Spec.InvestmentOperationKind != "" ||
		inverseParams.Spec.TransactionKind != "investment" || inverseParams.Spec.Status != "posted" ||
		inverseParams.Spec.TransactionDate != expected.EventDate ||
		replacementParams.Spec.InvestmentOperationKind != "share_exchange" ||
		replacementParams.Spec.TransactionKind != "investment" || replacementParams.Spec.Status != "posted" ||
		replacementParams.Spec.TransactionDate != exchange.EffectiveOn ||
		exchange.AccountID != expected.AccountID || exchange.CommodityID != expected.CommodityID ||
		exchange.ReplacesOperationID != expected.OperationID ||
		replacementParams.InvestmentCorrectionOfOperationID != expected.OperationID ||
		replacementParams.InvestmentCorrectionMode != "replace" || replacementParams.InvestmentCorrectionReason == "" ||
		!inverseParams.CorrectionOfTransactionID.Valid || inverseParams.CorrectionOfTransactionID.Int64 != expected.TransactionID ||
		!replacementParams.CorrectionOfTransactionID.Valid || replacementParams.CorrectionOfTransactionID.Int64 != expected.TransactionID {
		return ShareExchangeReplacementRecord{}, fmt.Errorf("%w: share exchange replacement is incomplete", ErrInvalidDisposalParams)
	}
	write := executeInvestmentJournalsWithGuardTx[ShareExchangePlan]
	if preview {
		write = previewInvestmentJournalsWithGuardTx[ShareExchangePlan]
	}
	journals, plan, err := write(ctx, r.database,
		[]CreateTransactionParams{inverseParams, replacementParams},
		func(tx *sql.Tx) error {
			return checkShareExchangeForCorrectionTx(ctx, tx, inverseParams.BookID, expected)
		}, func(tx *sql.Tx, journals []TransactionRecord, auditEventID int64) (ShareExchangePlan, error) {
			inverse, replacement := journals[0], journals[1]
			operationID, err := investmentOperationIDTx(ctx, tx, exchange.BookID, replacement.ID)
			if err != nil {
				return ShareExchangePlan{}, err
			}
			if _, err := tx.ExecContext(ctx, `INSERT INTO investment_operation_journal_links
				(book_id, operation_id, transaction_version_id, link_seq, role)
				VALUES (?, ?, ?, 2, 'reversal')`, exchange.BookID, operationID, inverse.VersionID); err != nil {
				return ShareExchangePlan{}, fmt.Errorf("link share exchange replacement inverse: %w", err)
			}
			// The writer replays the old holding and the replacement's new
			// holding; a replaced new holding elsewhere loses its lots here.
			plan, err := writeShareExchangeTx(ctx, tx, exchange, replacement, replacementParams, auditEventID)
			if err != nil {
				return ShareExchangePlan{}, err
			}
			replaced := [2]int64{expected.DestinationAccountID, expected.DestinationCommodityID}
			if replaced == [2]int64{exchange.DestinationAccountID, exchange.DestinationCommodityID} {
				return plan, nil
			}
			return plan, replayHoldingsTx(ctx, tx, exchange.BookID, [][2]int64{replaced},
				operationID, auditEventID, replacementParams.ActorUserID, replacementParams.CreatedAt)
		}, nil)
	if err != nil {
		return ShareExchangeReplacementRecord{}, err
	}
	return ShareExchangeReplacementRecord{Inverse: journals[0], Replacement: journals[1], Plan: plan}, nil
}
