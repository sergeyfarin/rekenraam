package app

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"rekenraam/backend/internal/exact"
)

// #596. A statement showing no movement since the last reconciliation — the
// normal case for a dormant account — finishes with nothing selected. The
// difference must still be zero, so the checkpoint only records that the bank
// agrees with the last reconciled balance on that date.

func TestEmptyStatementFinishesAsChainedCheckpointWithoutPostings(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	postEUR(t, f, "2026-01-15", 100)
	january := reconcileCash(t, f, "2026-01-31", 100)
	february := reconcileCash(t, f, "2026-02-28", 100)

	require.Equal(t, []int64{february, january}, activeCheckpointIDs(t, f))
	require.Zero(t, checkpointPostingCount(t, f, february))
	checkpoints, err := f.transactionService.repository.ListReconciliationCheckpoints(context.Background(), BookID, f.cashAccountID, f.eurCommodityID)
	require.NoError(t, err)
	require.Equal(t, february, checkpoints[0].ID)
	require.True(t, checkpoints[0].PreviousCheckpointID.Valid)
	require.Equal(t, january, checkpoints[0].PreviousCheckpointID.Int64)
	require.Equal(t, "2026-02-28", checkpoints[0].StatementDate)
	require.Zero(t, checkpoints[0].StatementAccountSequence)

	result := checkpointIntegrity(t, f)
	require.Equal(t, SelfCheckPassed, result.Status, result.Summary)
}

func TestEmptyStatementStillNeedsZeroDifference(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := newInvestmentsTestFixture(t)
	postEUR(t, f, "2026-01-15", 100)
	reconcileCash(t, f, "2026-01-31", 100)
	session := startCashReconciliation(t, f, "2026-02-28", 90)
	require.Equal(t, exact.New(-10), session.Difference.QuantityValue, "a new session shows its real difference, not 0")
	_, err := f.transactionService.FinishReconciliation(ctx, FinishReconciliationInput{
		OriginType: "browser_api", OwnerUserID: f.ownerUserID, SessionID: session.ID, ChangeReason: "statement verified",
	})
	require.ErrorIs(t, err, ErrReconciliationNotBalanced)
}

func TestLatePostingInsideEmptyStatementNeedsOverride(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := newInvestmentsTestFixture(t)
	postEUR(t, f, "2026-01-15", 100)
	january := reconcileCash(t, f, "2026-01-31", 100)
	february := reconcileCash(t, f, "2026-02-28", 100)

	spec := ordinaryEntry(f.cashAccountID, f.incomeAccountID, f.eurCommodityID, 5)
	spec.TransactionDate = "2026-02-10"
	_, err := f.transactionService.CreateTransaction(ctx, CreateTransactionInput{OwnerUserID: f.ownerUserID, OriginType: "browser_api", Spec: spec})
	require.ErrorIs(t, err, ErrReconciliationOverrideRequired)

	created := postEURWithOverride(t, f, "2026-02-10", 5)
	require.Equal(t, []int64{february}, created.InvalidatedCheckpointIDs)
	require.Equal(t, []int64{january}, activeCheckpointIDs(t, f))
}
