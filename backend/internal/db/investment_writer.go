package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
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
	first, auditEventID, err := createTransactionWithAuditTx(ctx, tx, params[0])
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
	for index, journal := range params {
		invalidatedIDs, err := invalidateCreateTransactionCheckpointsTx(ctx, tx, journal, auditEventID)
		if err != nil {
			return nil, zero, err
		}
		journals[index].InvalidatedCheckpointIDs = invalidatedIDs
		if !persist {
			refs, err := investmentInvalidatedCheckpointRefsTx(ctx, tx, journal.BookID, invalidatedIDs, journal.CheckpointCandidates)
			if err != nil {
				return nil, zero, err
			}
			journals[index].InvalidatedCheckpointRefs = refs
		}
	}
	// Replay may have posted split adjustment journals the caller could not
	// name in advance (T-129). They meet the same guard, override and reason
	// as the command's own journals, and a preview reports what they touch.
	adjustments, err := splitAdjustmentCheckpointCandidatesTx(ctx, tx, params[0].BookID, auditEventID)
	if err != nil {
		return nil, zero, err
	}
	if len(adjustments) > 0 {
		guarded := params[0]
		guarded.CheckpointCandidates = adjustments
		invalidatedIDs, err := invalidateCreateTransactionCheckpointsTx(ctx, tx, guarded, auditEventID)
		if err != nil {
			return nil, zero, err
		}
		journals[0].InvalidatedCheckpointIDs = append(journals[0].InvalidatedCheckpointIDs, invalidatedIDs...)
		if !persist {
			refs, err := investmentInvalidatedCheckpointRefsTx(ctx, tx, guarded.BookID, invalidatedIDs, adjustments)
			if err != nil {
				return nil, zero, err
			}
			journals[0].InvalidatedCheckpointRefs = append(journals[0].InvalidatedCheckpointRefs, refs...)
		}
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

// Hydrate only the checkpoint IDs actually invalidated by the writer, while
// still in its transaction. This does not calculate another boundary decision.
func investmentInvalidatedCheckpointRefsTx(ctx context.Context, tx *sql.Tx, bookID int64,
	ids []int64, candidates []PeriodScopedCheckpointRef,
) ([]CheckpointInvalidationRef, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	encoded, err := json.Marshal(ids)
	if err != nil {
		return nil, fmt.Errorf("encode invalidated checkpoint IDs: %w", err)
	}
	rows, err := tx.QueryContext(ctx, `SELECT id, account_id, commodity_id, statement_date, statement_account_sequence
        FROM reconciliation_checkpoints WHERE book_id = ? AND id IN (SELECT value FROM json_each(?))
        ORDER BY account_id, commodity_id, statement_date, id`, bookID, string(encoded))
	if err != nil {
		return nil, fmt.Errorf("read investment preview checkpoints: %w", err)
	}
	defer rows.Close()
	var refs []CheckpointInvalidationRef
	for rows.Next() {
		var ref CheckpointInvalidationRef
		if err := rows.Scan(&ref.CheckpointID, &ref.AccountID, &ref.CommodityID, &ref.StatementDate, &ref.StatementAccountSequence); err != nil {
			return nil, fmt.Errorf("scan investment preview checkpoint: %w", err)
		}
		for _, candidate := range candidates {
			if candidate.AccountID == ref.AccountID && candidate.CommodityID == ref.CommodityID &&
				(ref.EntryDate == "" || candidate.EntryDate < ref.EntryDate) {
				ref.EntryDate = candidate.EntryDate
			}
		}
		if ref.EntryDate == "" {
			return nil, fmt.Errorf("investment preview checkpoint has no affected account/commodity")
		}
		refs = append(refs, ref)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate investment preview checkpoints: %w", err)
	}
	if len(refs) != len(ids) {
		return nil, fmt.Errorf("investment preview checkpoint metadata is incomplete")
	}
	return refs, nil
}

// splitAdjustmentCheckpointCandidatesTx lists every position touched by a
// split adjustment journal posted under this audit event. Replay decides those
// journals inside the domain effects, so they are found afterwards.
func splitAdjustmentCheckpointCandidatesTx(ctx context.Context, tx *sql.Tx, bookID, auditEventID int64) ([]PeriodScopedCheckpointRef, error) {
	rows, err := tx.QueryContext(ctx, `SELECT pv.account_id, pv.commodity_id, je.entry_date
		FROM investment_split_revisions r
		JOIN posting_versions pv ON pv.transaction_version_id = r.adjustment_transaction_version_id
		JOIN journal_entries je ON je.id = pv.journal_entry_id
		WHERE r.book_id = ? AND r.created_audit_event_id = ?
		ORDER BY pv.id`, bookID, auditEventID)
	if err != nil {
		return nil, fmt.Errorf("read split adjustment positions: %w", err)
	}
	defer rows.Close()
	var candidates []PeriodScopedCheckpointRef
	for rows.Next() {
		candidate := PeriodScopedCheckpointRef{AccountDaySequence: math.MaxInt64}
		if err := rows.Scan(&candidate.AccountID, &candidate.CommodityID, &candidate.EntryDate); err != nil {
			return nil, fmt.Errorf("scan split adjustment position: %w", err)
		}
		candidates = append(candidates, candidate)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate split adjustment positions: %w", err)
	}
	return candidates, nil
}
