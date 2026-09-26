package db

import (
	"context"
	"database/sql"
	"fmt"
)

// executeInvestmentWriteTx is the only commit boundary for current investment
// commands. The effect callback may create a lot, allocate a disposal, or do
// nothing for a cash dividend. An import identity callback sees the completed
// command but still runs before commit.
func executeInvestmentWriteTx[T any](ctx context.Context, database *sql.DB, params CreateTransactionParams,
	effect func(*sql.Tx, TransactionRecord, int64) (T, error), postWrite func(*sql.Tx, int64) error,
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
