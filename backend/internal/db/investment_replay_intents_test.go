package db

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestInvestmentReplayIntentsReadImmutableSources(t *testing.T) {
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

	repo := NewInvestmentRepository(database)
	intents, err := repo.ListInvestmentReplayIntents(ctx, 1, 15, 2, 1, "long")
	require.NoError(t, err)
	require.Len(t, intents, 3)
	require.Equal(t, []int64{1, 2, 3}, []int64{intents[0].OperationID, intents[1].OperationID, intents[2].OperationID})
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
