package db

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestInvestmentReplayIntentsReadImmutableSources(t *testing.T) {
	ctx := context.Background()
	database := seedReplayTestBook(t)
	repo := NewInvestmentRepository(database)
	intents, err := repo.ListInvestmentReplayIntents(ctx, 1, 15, 2, 1, "long")
	require.NoError(t, err)
	require.Len(t, intents, 3)
	require.Equal(t, []int64{1, 2, 3}, []int64{intents[0].OperationID, intents[1].OperationID, intents[2].OperationID})
	require.Equal(t, []int64{1, 2, 3}, []int64{intents[0].OrderOperationID, intents[1].OrderOperationID, intents[2].OrderOperationID})
	require.Equal(t, "opening", intents[0].Kind)
	require.Equal(t, "10", intents[0].QuantityValue.String())
	require.Equal(t, "5", intents[1].QuantityValue.String(), "read the opening, not the 2.5 shares left in its projection")
	require.Equal(t, "60000", intents[1].AmountValue.String())
	require.Equal(t, "disposal", intents[2].Kind)
	require.Equal(t, "fifo", intents[2].CostBasisMethod)
	require.Equal(t, "fallback", intents[2].DecisionSource.ResolutionTier)
	require.Equal(t, "150000", intents[2].AmountValue.String())
	require.Empty(t, intents[2].SpecificLots, "FIFO must be reselected on replay")
}

func TestInvestmentReplayIntentsExcludeReversedOperation(t *testing.T) {
	ctx := context.Background()
	database := seedReplayTestBook(t)
	_, err := database.ExecContext(ctx, `
		INSERT INTO investment_operations
			(book_id, operation_kind, event_date, created_at, created_audit_event_id,
			 correction_of_operation_id, correction_mode, correction_reason)
		VALUES (1, 'reversal', '2026-04-02', '2026-09-14T00:00:00Z', 31,
			3, 'reverse', 'duplicate broker sale')`)
	require.NoError(t, err)

	intents, err := NewInvestmentRepository(database).ListInvestmentReplayIntents(ctx, 1, 15, 2, 1, "long")
	require.NoError(t, err)
	require.Len(t, intents, 2)
	require.Equal(t, []int64{1, 2}, []int64{intents[0].OperationID, intents[1].OperationID})
	snapshot, err := database.BeginTx(ctx, nil)
	require.NoError(t, err)
	defer snapshot.Rollback()
	operations, err := NewExportRepository(database).ExportInvestmentOperations(ctx, snapshot, 1)
	require.NoError(t, err)
	var reversal *ExportInvestmentOperationRecord
	for index := range operations {
		if operations[index].CorrectionOfOperationID.Valid {
			reversal = &operations[index]
		}
	}
	require.NotNil(t, reversal)
	require.Equal(t, int64(3), reversal.CorrectionOfOperationID.Int64)
	require.Equal(t, "reverse", reversal.CorrectionMode.String)
	require.Equal(t, "duplicate broker sale", reversal.CorrectionReason.String)
	require.NoError(t, snapshot.Commit())

	_, err = database.ExecContext(ctx, `
		INSERT INTO investment_operations
			(book_id, operation_kind, event_date, created_at, created_audit_event_id,
			 correction_of_operation_id, correction_mode, correction_reason)
		VALUES (1, 'reversal', '2026-04-02', '2026-09-14T00:00:00Z', 31,
			3, 'reverse', 'second reversal')`)
	require.Error(t, err, "one predecessor cannot fork into two correction chains")
	_, err = database.ExecContext(ctx, `
		INSERT INTO investment_operations
			(book_id, operation_kind, event_date, created_at, created_audit_event_id,
			 correction_of_operation_id, correction_mode, correction_reason)
		VALUES (1, 'reversal', '2026-04-02', '2026-09-14T00:00:00Z', 31,
			5, 'reverse', 'reverse a reversal')`)
	require.Error(t, err, "a pure reversal cannot be corrected again")
}

func seedReplayTestBook(t *testing.T) *sql.DB {
	t.Helper()
	ctx := context.Background()
	database := openTestDatabase(t)
	require.NoError(t, Migrate(ctx, database))
	seed, err := os.ReadFile(filepath.Join("testdata", "v01_seed.sql"))
	require.NoError(t, err)
	load, err := database.BeginTx(ctx, nil)
	require.NoError(t, err)
	_, err = load.ExecContext(ctx, `PRAGMA defer_foreign_keys = ON`)
	require.NoError(t, err)
	_, err = load.ExecContext(ctx, string(seed))
	require.NoError(t, err)
	require.NoError(t, load.Commit())
	return database
}

func TestInvestmentReplayIntentsOrderSameDayByOperationAndEffect(t *testing.T) {
	intents := []InvestmentReplayIntent{
		{OperationID: 3, EffectSeq: 1, EventDate: "2026-01-02"},
		{OperationID: 2, EffectSeq: 2, EventDate: "2026-01-01"},
		{OperationID: 2, EffectSeq: 1, EventDate: "2026-01-01"},
		{OperationID: 1, EffectSeq: 1, EventDate: "2026-01-01"},
	}
	sortInvestmentReplayIntents(intents)
	require.Equal(t, []int64{1, 2, 2, 3}, []int64{intents[0].OperationID, intents[1].OperationID, intents[2].OperationID, intents[3].OperationID})
	require.Equal(t, 1, intents[1].EffectSeq)
	require.Equal(t, 2, intents[2].EffectSeq)
}

func TestInvestmentReplayReplacementKeepsRootSameDaySlot(t *testing.T) {
	ctx := context.Background()
	database := seedReplayTestBook(t)
	result, err := database.ExecContext(ctx, `INSERT INTO investment_operations
		(book_id, operation_kind, event_date, created_at, created_audit_event_id,
		 correction_of_operation_id, correction_mode, correction_reason)
		VALUES (1, 'buy', '2026-04-02', '2026-09-28T00:00:00Z', 31,
			1, 'replace', 'correct original buy')`)
	require.NoError(t, err)
	replacementID, err := result.LastInsertId()
	require.NoError(t, err)
	orderIDs, err := investmentReplayOrderOperationIDsQuery(ctx, database, 1)
	require.NoError(t, err)
	require.EqualValues(t, 1, orderIDs[replacementID])
	intents := []InvestmentReplayIntent{
		{OperationID: 3, OrderOperationID: orderIDs[3], EffectSeq: 1, EventDate: "2026-04-02"},
		{OperationID: replacementID, OrderOperationID: orderIDs[replacementID], EffectSeq: 1, EventDate: "2026-04-02"},
	}
	sortInvestmentReplayIntents(intents)
	require.Equal(t, replacementID, intents[0].OperationID, "corrected buy must precede a later same-day sale")
	require.EqualValues(t, 3, intents[1].OperationID)
}
