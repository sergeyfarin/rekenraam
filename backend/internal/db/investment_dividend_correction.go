package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// DividendOperationRecord pins a posted cash dividend (T-115). A cash
// dividend opens no lot and replays nothing; its correction is an audited
// inverse journal, optionally followed by a replacement, under the shared
// investment writer's guard and checkpoint invalidation.
type DividendOperationRecord struct {
	OperationID          int64
	TransactionID        int64
	TransactionVersionID int64
	CurrentVersionID     int64
	EventDate            string
	AlreadyCorrected     bool
	ImportedLineage      bool
}

func (r *InvestmentRepository) DividendOperationByTransactionID(ctx context.Context, bookID, transactionID int64) (DividendOperationRecord, error) {
	return dividendOperationByTransactionIDQuery(ctx, r.database, bookID, transactionID)
}

func dividendOperationByTransactionIDQuery(ctx context.Context, reader saleOperationReader, bookID, transactionID int64) (DividendOperationRecord, error) {
	var record DividendOperationRecord
	var corrected int
	err := reader.QueryRowContext(ctx, `SELECT o.id, linked_version.transaction_id, link.transaction_version_id,
		current.id, o.event_date,
		EXISTS(SELECT 1 FROM investment_operations successor WHERE successor.correction_of_operation_id = o.id)
		FROM investment_operations o
		JOIN investment_operation_journal_links link ON link.operation_id = o.id AND link.book_id = o.book_id AND link.role = 'primary'
		JOIN transaction_versions linked_version ON linked_version.id = link.transaction_version_id
		JOIN current_transaction_versions current ON current.transaction_id = linked_version.transaction_id
		WHERE o.book_id = ? AND linked_version.transaction_id = ? AND o.operation_kind = 'dividend'`,
		bookID, transactionID).Scan(&record.OperationID, &record.TransactionID, &record.TransactionVersionID,
		&record.CurrentVersionID, &record.EventDate, &corrected)
	if errors.Is(err, sql.ErrNoRows) {
		return DividendOperationRecord{}, ErrNotFound
	}
	if err != nil {
		return DividendOperationRecord{}, fmt.Errorf("read investment dividend operation: %w", err)
	}
	record.AlreadyCorrected = corrected != 0
	record.ImportedLineage, err = investmentOperationHasImportedLineageQuery(ctx, reader, bookID, record.OperationID)
	if err != nil {
		return DividendOperationRecord{}, err
	}
	return record, nil
}

func checkDividendOperationForCorrectionTx(ctx context.Context, tx *sql.Tx, bookID int64, expected DividendOperationRecord) error {
	current, err := dividendOperationByTransactionIDQuery(ctx, tx, bookID, expected.TransactionID)
	if err != nil {
		return err
	}
	if current.AlreadyCorrected {
		return ErrInvestmentOperationAlreadyCorrected
	}
	// An imported dividend is correctable only while its committed source row
	// still identifies this operation; the identity keeps deduping re-fetches.
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
		return ErrInvestmentSaleChanged
	}
	return checkInvestmentSourceJournalTx(ctx, tx, bookID, current.TransactionID,
		current.EventDate, current.TransactionVersionID, current.CurrentVersionID)
}

// ReverseDividend posts the exact inverse of a cash dividend as a reversal
// operation. Reconciled cash, income or withholding balances are guarded.
func (r *InvestmentRepository) ReverseDividend(ctx context.Context, params CreateTransactionParams, expected DividendOperationRecord) (TransactionRecord, error) {
	return r.reverseDividend(ctx, params, expected, false)
}

// PreviewDividendReversal runs the reversal writer and rolls it back.
func (r *InvestmentRepository) PreviewDividendReversal(ctx context.Context, params CreateTransactionParams, expected DividendOperationRecord) (SimulatedInvestmentWrite, error) {
	transaction, err := r.reverseDividend(ctx, params, expected, true)
	if err != nil {
		return SimulatedInvestmentWrite{}, err
	}
	return simulatedInvestmentWrite(transaction), nil
}

func (r *InvestmentRepository) reverseDividend(ctx context.Context, params CreateTransactionParams, expected DividendOperationRecord, preview bool) (TransactionRecord, error) {
	if params.BookID <= 0 || params.ActorUserID <= 0 || expected.OperationID <= 0 ||
		params.Spec.InvestmentOperationKind != "reversal" || params.Spec.TransactionKind != "investment" ||
		params.Spec.Status != "posted" || params.Spec.TransactionDate != expected.EventDate ||
		params.InvestmentCorrectionOfOperationID != expected.OperationID ||
		params.InvestmentCorrectionMode != "reverse" || params.InvestmentCorrectionReason == "" ||
		!params.CorrectionOfTransactionID.Valid || params.CorrectionOfTransactionID.Int64 != expected.TransactionID {
		return TransactionRecord{}, fmt.Errorf("%w: dividend reversal is incomplete", ErrInvalidDisposalParams)
	}
	guard := func(tx *sql.Tx) error {
		return checkDividendOperationForCorrectionTx(ctx, tx, params.BookID, expected)
	}
	effect := func(*sql.Tx, TransactionRecord, int64) (struct{}, error) { return struct{}{}, nil }
	var transaction TransactionRecord
	var err error
	if preview {
		transaction, _, err = previewInvestmentWriteWithGuardTx(ctx, r.database, params, guard, effect)
	} else {
		transaction, _, err = executeInvestmentWriteWithGuardTx(ctx, r.database, params, guard, effect, nil)
	}
	return transaction, err
}

type DividendReplacementRecord struct {
	Inverse     TransactionRecord
	Replacement TransactionRecord
}

// ReplaceDividend posts the inverse and the corrected dividend under one
// audit event. The replacement operation links the inverse as its reversal
// journal, exactly like a trade replacement; the original stays history.
func (r *InvestmentRepository) ReplaceDividend(ctx context.Context, expected DividendOperationRecord,
	inverseParams, replacementParams CreateTransactionParams,
) (DividendReplacementRecord, error) {
	return r.replaceDividend(ctx, expected, inverseParams, replacementParams, false)
}

// SimulateDividendReplacement runs the replacement writer and rolls it back.
func (r *InvestmentRepository) SimulateDividendReplacement(ctx context.Context, expected DividendOperationRecord,
	inverseParams, replacementParams CreateTransactionParams,
) (SimulatedInvestmentWrite, error) {
	record, err := r.replaceDividend(ctx, expected, inverseParams, replacementParams, true)
	if err != nil {
		return SimulatedInvestmentWrite{}, err
	}
	return simulatedInvestmentWrite(record.Inverse, record.Replacement), nil
}

func (r *InvestmentRepository) replaceDividend(ctx context.Context, expected DividendOperationRecord,
	inverseParams, replacementParams CreateTransactionParams, preview bool,
) (DividendReplacementRecord, error) {
	if expected.OperationID <= 0 || inverseParams.BookID <= 0 || inverseParams.BookID != replacementParams.BookID ||
		inverseParams.ActorUserID != replacementParams.ActorUserID ||
		inverseParams.Spec.InvestmentOperationKind != "" ||
		inverseParams.Spec.TransactionKind != "investment" || inverseParams.Spec.Status != "posted" ||
		replacementParams.Spec.InvestmentOperationKind != "dividend" ||
		replacementParams.Spec.TransactionKind != "investment" || replacementParams.Spec.Status != "posted" ||
		inverseParams.Spec.TransactionDate != expected.EventDate ||
		replacementParams.Spec.TransactionDate != expected.EventDate ||
		replacementParams.InvestmentCorrectionOfOperationID != expected.OperationID ||
		replacementParams.InvestmentCorrectionMode != "replace" ||
		replacementParams.InvestmentCorrectionReason == "" ||
		!inverseParams.CorrectionOfTransactionID.Valid ||
		inverseParams.CorrectionOfTransactionID.Int64 != expected.TransactionID ||
		!replacementParams.CorrectionOfTransactionID.Valid ||
		replacementParams.CorrectionOfTransactionID.Int64 != expected.TransactionID {
		return DividendReplacementRecord{}, fmt.Errorf("%w: dividend replacement is incomplete", ErrInvalidDisposalParams)
	}
	write := executeInvestmentJournalsWithGuardTx[struct{}]
	if preview {
		write = previewInvestmentJournalsWithGuardTx[struct{}]
	}
	journals, _, err := write(ctx, r.database,
		[]CreateTransactionParams{inverseParams, replacementParams},
		func(tx *sql.Tx) error {
			return checkDividendOperationForCorrectionTx(ctx, tx, inverseParams.BookID, expected)
		}, func(tx *sql.Tx, journals []TransactionRecord, _ int64) (struct{}, error) {
			inverse, replacement := journals[0], journals[1]
			operationID, err := investmentOperationIDTx(ctx, tx, replacementParams.BookID, replacement.ID)
			if err != nil {
				return struct{}{}, err
			}
			if _, err := tx.ExecContext(ctx, `INSERT INTO investment_operation_journal_links
		(book_id, operation_id, transaction_version_id, link_seq, role)
		VALUES (?, ?, ?, 2, 'reversal')`, replacementParams.BookID, operationID, inverse.VersionID); err != nil {
				return struct{}{}, fmt.Errorf("link dividend replacement inverse: %w", err)
			}
			return struct{}{}, nil
		}, nil)
	if err != nil {
		return DividendReplacementRecord{}, err
	}
	return DividendReplacementRecord{Inverse: journals[0], Replacement: journals[1]}, nil
}
