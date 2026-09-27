package db

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCorrectionChainReadsReplacementAndTerminalReversalFromAnyLinkedTransaction(t *testing.T) {
	ctx := context.Background()
	database := seedReplayTestBook(t)
	_, err := database.ExecContext(ctx, `INSERT INTO investment_operations
		(book_id, operation_kind, event_date, created_at, created_audit_event_id,
		 correction_of_operation_id, correction_mode, correction_reason)
		VALUES (1, 'sell', '2026-04-02', '2026-09-14T00:00:00Z', 31,
		3, 'replace', 'corrected broker proceeds')`)
	require.NoError(t, err)
	_, err = database.ExecContext(ctx, `INSERT INTO investment_operations
		(book_id, operation_kind, event_date, created_at, created_audit_event_id,
		 correction_of_operation_id, correction_mode, correction_reason)
		VALUES (1, 'reversal', '2026-04-02', '2026-09-15T00:00:00Z', 31,
		5, 'reverse', 'broker canceled fill')`)
	require.NoError(t, err)
	chain, err := NewInvestmentRepository(database).CorrectionChainByTransactionID(ctx, 1, 9)
	require.NoError(t, err)
	require.Len(t, chain, 3)
	require.Equal(t, []int64{3, 5, 6}, []int64{chain[0].OperationID, chain[1].OperationID, chain[2].OperationID})
	require.Equal(t, "replace", chain[1].CorrectionMode.String)
	require.Equal(t, "reverse", chain[2].CorrectionMode.String)
	require.False(t, chain[2].TransactionID.Valid, "operation-only successors remain visible")
	_, err = NewInvestmentRepository(database).CorrectionChainByTransactionID(ctx, 2, 9)
	require.ErrorIs(t, err, ErrNotFound)
}
