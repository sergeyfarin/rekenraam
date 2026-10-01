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
	committed := false
	defer func() {
		if !committed {
			rollbackTx(ctx, tx)
		}
	}()
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
	for index, journal := range params {
		invalidatedIDs, err := invalidateCreateTransactionCheckpointsTx(ctx, tx, journal, auditEventID)
		if err != nil {
			return nil, zero, err
		}
		journals[index].InvalidatedCheckpointIDs = invalidatedIDs
	}
	if postWrite != nil {
		if err := postWrite(tx, journals, auditEventID); err != nil {
			return nil, zero, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, zero, fmt.Errorf("commit investment write: %w", err)
	}
	committed = true
	return journals, result, nil
}
