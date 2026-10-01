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
	var zero T
	tx, err := database.BeginTx(ctx, nil)
	if err != nil {
		return TransactionRecord{}, zero, fmt.Errorf("begin investment write: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			rollbackTx(ctx, tx)
		}
	}()
	if guard != nil {
		if err := guard(tx); err != nil {
			return TransactionRecord{}, zero, err
		}
	}
	transaction, auditEventID, err := createTransactionWithAuditTx(ctx, tx, params)
	if err != nil {
		return TransactionRecord{}, zero, err
	}
	result, err := effect(tx, transaction, auditEventID)
	if err != nil {
		return TransactionRecord{}, zero, err
	}
	invalidatedIDs, err := invalidateCreateTransactionCheckpointsTx(ctx, tx, params, auditEventID)
	if err != nil {
		return TransactionRecord{}, zero, err
	}
	transaction.InvalidatedCheckpointIDs = invalidatedIDs
	if postWrite != nil {
		if err := postWrite(tx, transaction.ID); err != nil {
			return TransactionRecord{}, zero, err
		}
	}
	if err := tx.Commit(); err != nil {
		return TransactionRecord{}, zero, fmt.Errorf("commit investment write: %w", err)
	}
	committed = true
	return transaction, result, nil
}
