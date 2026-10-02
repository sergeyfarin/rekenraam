package app

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCreatePreviewReportsEveryCheckpointCommitInvalidates(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := newInvestmentsTestFixture(t)
	postEUR(t, f, "2026-03-01", 10)
	first := reconcileCash(t, f, "2026-03-31", 10)
	postEUR(t, f, "2026-04-01", 10)
	second := reconcileCash(t, f, "2026-04-30", 20)
	spec := ordinaryEntry(f.cashAccountID, f.incomeAccountID, f.eurCommodityID, 5)
	spec.TransactionDate = "2026-03-15"
	// Multiple affected positions must still produce one warning per checkpoint.
	later := spec.JournalEntries[0]
	later.EntryDate = "2026-04-15"
	spec.JournalEntries = append(spec.JournalEntries, later)
	before := buyReplacementPreviewSnapshot(t, f.database)
	for range 2 {
		impact, err := f.transactionService.ReconciliationImpactForCreate(ctx, CreateReconciliationImpactInput{
			OwnerUserID: f.ownerUserID, Spec: spec,
		})
		require.NoError(t, err)
		requireCheckpointImpactIDs(t, impact, first, second)
		for _, ref := range impact.AffectedCheckpoints {
			require.Equal(t, "2026-03-15", ref.EntryDate)
			require.NotEmpty(t, ref.AccountLabel)
			require.NotEmpty(t, ref.CommodityCode)
		}
		require.Equal(t, before, buyReplacementPreviewSnapshot(t, f.database))
	}
	input := CreateTransactionInput{OwnerUserID: f.ownerUserID, OriginType: "browser_api", Spec: spec}
	_, err := f.transactionService.CreateTransaction(ctx, input)
	require.ErrorIs(t, err, ErrReconciliationOverrideRequired)
	require.Equal(t, before, buyReplacementPreviewSnapshot(t, f.database))
	input.ReconciliationOverride = true
	committed, err := f.transactionService.CreateTransaction(ctx, input)
	require.NoError(t, err)
	require.ElementsMatch(t, []int64{first, second}, committed.InvalidatedCheckpointIDs)
	require.Empty(t, activeCheckpointIDs(t, f))
}

func TestSalePreviewReportsEveryCheckpointCommitInvalidates(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := newInvestmentsTestFixture(t)
	buyOn(t, f, "2026-02-01", 10, 20000)
	first := reconcileCash(t, f, "2026-03-31", -200)
	postEUR(t, f, "2026-04-01", 10)
	second := reconcileCash(t, f, "2026-04-30", -190)
	input := sellInput(f, "2026-03-15", 5)
	input.CashAmountValue = 15000
	before := buyReplacementPreviewSnapshot(t, f.database)
	for range 2 {
		impact, err := f.investmentService.TradeReconciliationImpact(ctx, InvestmentImpactSell, input)
		require.NoError(t, err)
		requireCheckpointImpactIDs(t, impact, first, second)
		require.Equal(t, before, buyReplacementPreviewSnapshot(t, f.database))
	}
	_, err := f.investmentService.Sell(ctx, input)
	require.ErrorIs(t, err, ErrReconciliationOverrideRequired)
	require.Equal(t, before, buyReplacementPreviewSnapshot(t, f.database))
	input.ReconciliationOverride = true
	committed, err := f.investmentService.Sell(ctx, input)
	require.NoError(t, err)
	require.ElementsMatch(t, []int64{first, second}, committed.Transaction.InvalidatedCheckpointIDs)
	require.Empty(t, activeCheckpointIDs(t, f))
}

func requireCheckpointImpactIDs(t *testing.T, impact ReconciliationImpact, expected ...int64) {
	t.Helper()
	ids := make([]int64, 0, len(impact.AffectedCheckpoints))
	for _, ref := range impact.AffectedCheckpoints {
		ids = append(ids, ref.CheckpointID)
	}
	require.ElementsMatch(t, expected, ids)
}
