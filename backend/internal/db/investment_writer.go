package db

import (
	"context"
	"database/sql"
	"fmt"
)

// executeInvestmentWriteTx commits an investment journal, its domain effects,
// reconciliation changes and optional import identity together.
func executeInvestmentWriteTx[T any](ctx context.Context, database *sql.DB, params CreateTransactionParams,
	effect func(*sql.Tx, TransactionRecord, int64) (T, error), postWrite func(*sql.Tx, int64) error,
) (TransactionRecord, T, error) {
	return executeInvestmentWriteWithGuardTx(ctx, database, params, nil, effect, postWrite)
}

// previewInvestmentWriteTx is the single-journal counterpart to the shared
// compound preview writer. It uses the same effects and rollback boundary.
func previewInvestmentWriteTx[T any](ctx context.Context, database *sql.DB, params CreateTransactionParams,
	effect func(*sql.Tx, TransactionRecord, int64) (T, error), postWrite func(*sql.Tx, int64) error,
) (TransactionRecord, T, error) {
	if postWrite != nil {
		var zero T
		return TransactionRecord{}, zero, fmt.Errorf("investment preview cannot accept source evidence")
	}
	journals, result, err := previewInvestmentJournalsWithGuardTx(ctx, database, []CreateTransactionParams{params}, nil,
		func(tx *sql.Tx, journals []TransactionRecord, auditEventID int64) (T, error) {
			return effect(tx, journals[0], auditEventID)
		}, nil)
	if err != nil {
		var zero T
		return TransactionRecord{}, zero, err
	}
	return journals[0], result, nil
}

// previewInvestmentWriteWithGuardTx is the rolled-back counterpart of
// executeInvestmentWriteWithGuardTx: same guard, effects and checkpoint
// invalidation, then rollback. Only checkpoint refs and gain impact escape.
func previewInvestmentWriteWithGuardTx[T any](ctx context.Context, database *sql.DB, params CreateTransactionParams,
	guard func(*sql.Tx) error, effect func(*sql.Tx, TransactionRecord, int64) (T, error),
) (TransactionRecord, T, error) {
	journals, result, err := previewInvestmentJournalsWithGuardTx(ctx, database, []CreateTransactionParams{params}, guard,
		func(tx *sql.Tx, journals []TransactionRecord, auditEventID int64) (T, error) {
			return effect(tx, journals[0], auditEventID)
		}, nil)
	if err != nil {
		var zero T
		return TransactionRecord{}, zero, err
	}
	return journals[0], result, nil
}

// SimulatedInvestmentWrite is the durable-ID-free result of a rolled-back
// investment command: every checkpoint its writer invalidated, once each, and
// for an opted-in command the committed-disposal gain changes (T-114/T-126).
type SimulatedInvestmentWrite struct {
	InvalidatedCheckpointRefs []CheckpointInvalidationRef
	GainImpact                *InvestmentGainImpact
}

func simulatedInvestmentWrite(journals ...TransactionRecord) SimulatedInvestmentWrite {
	var result SimulatedInvestmentWrite
	seen := make(map[int64]bool)
	for index, journal := range journals {
		if index == 0 {
			result.GainImpact = journal.GainImpact
		}
		for _, ref := range journal.InvalidatedCheckpointRefs {
			if !seen[ref.CheckpointID] {
				seen[ref.CheckpointID] = true
				result.InvalidatedCheckpointRefs = append(result.InvalidatedCheckpointRefs, ref)
			}
		}
	}
	return result
}

// Correction guards run under the same SQLite write transaction, before a
// successor journal exists. Checking after insertion would see the command's
// own successor and incorrectly reject an otherwise effective source.
func executeInvestmentWriteWithGuardTx[T any](ctx context.Context, database *sql.DB, params CreateTransactionParams,
	guard func(*sql.Tx) error, effect func(*sql.Tx, TransactionRecord, int64) (T, error),
	postWrite func(*sql.Tx, int64) error,
) (TransactionRecord, T, error) {
	journals, result, err := executeInvestmentJournalsWithGuardTx(ctx, database, []CreateTransactionParams{params}, guard,
		func(tx *sql.Tx, journals []TransactionRecord, auditEventID int64) (T, error) {
			return effect(tx, journals[0], auditEventID)
		}, func(tx *sql.Tx, journals []TransactionRecord, _ int64) error {
			if postWrite != nil {
				return postWrite(tx, journals[0].ID)
			}
			return nil
		})
	if err != nil {
		var zero T
		return TransactionRecord{}, zero, err
	}
	return journals[0], result, nil
}

// executeInvestmentJournalsWithGuardTx owns a single audited write for one or
// more journals. The first journal supplies command audit attribution. Guards
// precede all journals; domain effects precede checkpoint invalidation; source
// acceptance runs last, with every stage covered by the same rollback.
func executeInvestmentJournalsWithGuardTx[T any](ctx context.Context, database *sql.DB, params []CreateTransactionParams,
	guard func(*sql.Tx) error, effect func(*sql.Tx, []TransactionRecord, int64) (T, error),
	postWrite func(*sql.Tx, []TransactionRecord, int64) error,
) ([]TransactionRecord, T, error) {
	return runInvestmentJournalsWithGuardTx(ctx, database, params, guard, effect, postWrite, true)
}

// previewInvestmentJournalsWithGuardTx runs the exact journal and domain path,
// then rolls back. Its temporary IDs must never be exposed as durable records.
// Source acceptance is deliberately omitted from a preview.
func previewInvestmentJournalsWithGuardTx[T any](ctx context.Context, database *sql.DB, params []CreateTransactionParams,
	guard func(*sql.Tx) error, effect func(*sql.Tx, []TransactionRecord, int64) (T, error),
	postWrite func(*sql.Tx, []TransactionRecord, int64) error,
) ([]TransactionRecord, T, error) {
	if postWrite != nil {
		var zero T
		return nil, zero, fmt.Errorf("investment preview cannot accept source evidence")
	}
	return runInvestmentJournalsWithGuardTx(ctx, database, params, guard, effect, nil, false)
}

func runInvestmentJournalsWithGuardTx[T any](ctx context.Context, database *sql.DB, params []CreateTransactionParams,
	guard func(*sql.Tx) error, effect func(*sql.Tx, []TransactionRecord, int64) (T, error),
	postWrite func(*sql.Tx, []TransactionRecord, int64) error, persist bool,
) ([]TransactionRecord, T, error) {
	var zero T
	if len(params) == 0 {
		return nil, zero, fmt.Errorf("investment write requires a journal")
	}
	for _, journal := range params {
		if journal.BookID != params[0].BookID || journal.ActorUserID != params[0].ActorUserID {
			return nil, zero, fmt.Errorf("investment journals must share book and actor")
		}
		// The combined checkpoint guard applies one decision to the command.
		if journal.ReconciliationOverride != params[0].ReconciliationOverride {
			return nil, zero, fmt.Errorf("investment journals must share one reconciliation override")
		}
	}
	tx, err := database.BeginTx(ctx, nil)
	if err != nil {
		return nil, zero, fmt.Errorf("begin investment write: %w", err)
	}
	finished := false
	defer func() {
		if !finished {
			rollbackTx(ctx, tx)
		}
	}()
	// Capture the effective disposals before any guard, journal or corrective
	// intent can change them; comparing inside replay persistence alone would
	// miss reversed and superseded disposals.
	gainPolicy := params[0].GainImpact
	var gainsBefore map[InvestmentGainIdentity]investmentGainSnapshotEntry
	if gainPolicy != nil {
		if gainsBefore, err = investmentGainSnapshotTx(ctx, tx, params[0].BookID); err != nil {
			return nil, zero, err
		}
	}
	if guard != nil {
		if err := guard(tx); err != nil {
			return nil, zero, err
		}
	}
	// Check every journal's pinned rules before creating any audit or journal.
	for _, journal := range params {
		if err := requireAccountRuleDependenciesTx(ctx, tx, journal.Spec, journal.AccountRuleDependencies); err != nil {
			return nil, zero, err
		}
	}
	var first TransactionRecord
	var auditEventID int64
	if params[0].AuditOnly {
		// A journal-free command (#168) has exactly its header.
		if len(params) != 1 {
			return nil, zero, fmt.Errorf("a journal-free investment command has no journals")
		}
		auditEventID, err = openInvestmentCommandAuditTx(ctx, tx, params[0])
	} else {
		first, auditEventID, err = createTransactionWithAuditTx(ctx, tx, params[0])
	}
	if err != nil {
		return nil, zero, err
	}
	journals := make([]TransactionRecord, 0, len(params))
	journals = append(journals, first)
	for _, journal := range params[1:] {
		record, err := insertTransactionWithAuditEventTx(ctx, tx, journal, auditEventID)
		if err != nil {
			return nil, zero, err
		}
		journals = append(journals, record)
	}
	result, err := effect(tx, journals, auditEventID)
	if err != nil {
		return nil, zero, err
	}
	// One combined guard for the whole command (T-120 #135): every journal it
	// appended — inverse, replacement, and any split adjustment replay posted
	// that the caller could not name in advance (T-129) — is netted per
	// account and commodity at each checkpoint's own boundary. Guarding each
	// journal alone invalidated checkpoints whose balance the command as a
	// whole leaves unchanged. A preview reports exactly this set. It runs
	// before the gain acknowledgement, so a command needing both is refused
	// for reconciliation first, as the per-journal early check used to.
	deltas, err := commandCheckpointDeltasTx(ctx, tx, params[0].BookID, auditEventID)
	if err != nil {
		return nil, zero, err
	}
	refs, err := netCheckpointInvalidationRefs(ctx, tx, params[0].BookID, deltas)
	if err != nil {
		return nil, zero, err
	}
	if len(refs) > 0 {
		if !params[0].ReconciliationOverride {
			return nil, zero, ErrReconciliationOverrideRequired
		}
		invalidatedIDs, err := invalidateReconciliationCheckpoints(ctx, tx, checkpointInvalidationParams{
			BookID: params[0].BookID, Refs: refs, ActorUserID: params[0].ActorUserID,
			AuditEventID: auditEventID, OccurredAt: params[0].CreatedAt,
			Reason: params[0].InvalidateCheckpointReason,
		})
		if err != nil {
			return nil, zero, err
		}
		journals[0].InvalidatedCheckpointIDs = invalidatedIDs
		if !persist {
			journals[0].InvalidatedCheckpointRefs = refs
		}
	}
	if gainPolicy != nil {
		gainsAfter, err := investmentGainSnapshotTx(ctx, tx, params[0].BookID)
		if err != nil {
			return nil, zero, err
		}
		impact := compareInvestmentGainSnapshots(gainsBefore, gainsAfter)
		if persist {
			if err := requireInvestmentGainAcknowledgement(*gainPolicy, impact); err != nil {
				return nil, zero, err
			}
		}
		journals[0].GainImpact = &impact
	}
	if postWrite != nil {
		if err := postWrite(tx, journals, auditEventID); err != nil {
			return nil, zero, err
		}
	}
	if persist {
		if err := tx.Commit(); err != nil {
			return nil, zero, fmt.Errorf("commit investment write: %w", err)
		}
	} else if err := tx.Rollback(); err != nil {
		return nil, zero, fmt.Errorf("rollback investment preview: %w", err)
	}
	finished = true
	return journals, result, nil
}

// openInvestmentCommandAuditTx opens the audit event of a journal-free
// investment command under the book write lock, as a journal's would be.
func openInvestmentCommandAuditTx(ctx context.Context, tx *sql.Tx, params CreateTransactionParams) (int64, error) {
	if _, err := readBookForUpdate(ctx, tx, params.BookID); err != nil {
		return 0, err
	}
	return insertAuditEvent(ctx, tx, AuditEventParams{
		BookID: params.BookID, ActorUserID: params.ActorUserID, AuthSessionID: params.AuthSessionID,
		OccurredAt: params.CreatedAt, RequestID: params.RequestID, OriginType: params.OriginType,
		Operation: params.Operation, Reason: params.ChangeReason,
	})
}

// insertJournalFreeInvestmentOperationTx records the operation a journal-free
// command describes (#168): its kind, date and correction link come from the
// header, under the command's audit event, with no journal link.
func insertJournalFreeInvestmentOperationTx(ctx context.Context, tx *sql.Tx, params CreateTransactionParams, auditEventID int64) (int64, error) {
	if !params.AuditOnly || params.Spec.InvestmentOperationKind == "" {
		return 0, fmt.Errorf("%w: a journal-free operation needs an audit-only header", ErrInvalidDisposalParams)
	}
	result, err := tx.ExecContext(ctx, `INSERT INTO investment_operations (
			book_id, operation_kind, event_date, created_at, created_audit_event_id,
			correction_of_operation_id, correction_mode, correction_reason
		) VALUES (?, ?, ?, ?, ?, NULLIF(?, 0), NULLIF(?, ''), NULLIF(?, ''))`,
		params.BookID, params.Spec.InvestmentOperationKind, params.Spec.TransactionDate, params.CreatedAt,
		auditEventID, params.InvestmentCorrectionOfOperationID, params.InvestmentCorrectionMode,
		params.InvestmentCorrectionReason)
	if err != nil {
		return 0, fmt.Errorf("insert journal-free investment operation: %w", err)
	}
	operationID, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("read journal-free investment operation id: %w", err)
	}
	// The same date role a journal-backed operation of the kind records.
	dateRole := "effective"
	if params.Spec.InvestmentOperationKind == "reversal" {
		dateRole = "trade"
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO investment_operation_dates (operation_id, date_role, event_date)
		VALUES (?, ?, ?)`, operationID, dateRole, params.Spec.TransactionDate); err != nil {
		return 0, fmt.Errorf("record journal-free investment operation date: %w", err)
	}
	return operationID, nil
}
