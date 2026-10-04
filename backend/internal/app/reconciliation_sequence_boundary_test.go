package app

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"rekenraam/backend/internal/exact"
)

// T-120 #135. A checkpoint is a boundary at (statement_date,
// statement_account_sequence), and a posting is inside it only when its own
// (entry_date, account_day_sequence) is at or before that boundary. The guard
// used to test eligibility against the latest checkpoint and then cascade by
// date alone, so a same-day posting after an earlier checkpoint's boundary
// invalidated that checkpoint as soon as any later checkpoint existed — and a
// same-day reorder was judged against the latest checkpoint only, so a swap
// across an earlier boundary changed its reconciled balance with no guard.

// marchAndAprilCheckpoints reconciles one 31 March posting (sequence 1) into a
// March checkpoint, then adds a second 31 March posting (sequence 2, after the
// March boundary) and an April posting and reconciles both into April.
// Further same-day amounts land after afterMarch, also inside April.
func marchAndAprilCheckpoints(t *testing.T, f *investmentsTestFixture, further ...int64) (march, april int64, afterMarch Transaction) {
	t.Helper()
	postEUR(t, f, "2026-03-31", 10)
	march = reconcileCash(t, f, "2026-03-31", 10)
	afterMarch = postEUR(t, f, "2026-03-31", 7)
	aprilBalance := int64(27)
	for _, amount := range further {
		postEUR(t, f, "2026-03-31", amount)
		aprilBalance += amount
	}
	postEUR(t, f, "2026-04-10", 10)
	april = reconcileCash(t, f, "2026-04-30", aprilBalance)
	return march, april, afterMarch
}

func cashPostingLineID(t *testing.T, f *investmentsTestFixture, transaction Transaction) int64 {
	t.Helper()
	for _, entry := range transaction.JournalEntries {
		for _, posting := range entry.Postings {
			if posting.AccountID == f.cashAccountID {
				return posting.PostingLineID
			}
		}
	}
	t.Fatal("transaction has no cash posting")
	return 0
}

// The issue's named case: a new 31 March posting lands at sequence 2, after the
// March boundary. April changes; March does not, in preview and commit alike.
func TestSameDayPostingAfterEarlierCheckpointPreservesIt(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := newInvestmentsTestFixture(t)
	march, april, _ := marchAndAprilCheckpoints(t, f)

	spec := ordinaryEntry(f.cashAccountID, f.incomeAccountID, f.eurCommodityID, 5)
	spec.TransactionDate = "2026-03-31"
	impact, err := f.transactionService.ReconciliationImpactForCreate(ctx, CreateReconciliationImpactInput{
		OwnerUserID: f.ownerUserID, Spec: spec,
	})
	require.NoError(t, err)
	requireCheckpointImpactIDs(t, impact, april)

	input := CreateTransactionInput{OwnerUserID: f.ownerUserID, OriginType: "browser_api", Spec: spec}
	_, err = f.transactionService.CreateTransaction(ctx, input)
	require.ErrorIs(t, err, ErrReconciliationOverrideRequired, "April's balance changes, so the guard still holds")
	require.ElementsMatch(t, []int64{march, april}, activeCheckpointIDs(t, f))

	input.ReconciliationOverride = true
	input.ChangeReason = "late entry"
	created, err := f.transactionService.CreateTransaction(ctx, input)
	require.NoError(t, err)
	require.Equal(t, []int64{april}, created.InvalidatedCheckpointIDs)
	require.Equal(t, []int64{march}, activeCheckpointIDs(t, f), "March's boundary is before the new posting")
}

// The same boundary through an edit and a void of an existing posting that sits
// after March's boundary on March's statement date.
func TestEditAndVoidAfterEarlierSameDayCheckpointPreserveIt(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("edit", func(t *testing.T) {
		t.Parallel()
		f := newInvestmentsTestFixture(t)
		march, april, afterMarch := marchAndAprilCheckpoints(t, f)
		spec := transactionInputFromTransaction(afterMarch)
		for index := range spec.JournalEntries[0].Postings {
			posting := &spec.JournalEntries[0].Postings[index]
			if posting.AccountID == f.cashAccountID {
				posting.QuantityValue = exact.New(8)
			} else {
				posting.QuantityValue = exact.New(-8)
			}
		}
		impact, err := f.transactionService.ReconciliationImpactForUpdate(ctx, UpdateReconciliationImpactInput{
			OwnerUserID: f.ownerUserID, TransactionID: afterMarch.ID, Spec: spec,
		})
		require.NoError(t, err)
		requireCheckpointImpactIDs(t, impact, april)

		updated, err := f.transactionService.UpdateTransaction(ctx, UpdateTransactionInput{
			OwnerUserID: f.ownerUserID, OriginType: "browser_api", TransactionID: afterMarch.ID, Spec: spec,
			ChangeReason: "statement typo", ReconciliationOverride: true,
		})
		require.NoError(t, err)
		require.Equal(t, []int64{april}, updated.InvalidatedCheckpointIDs)
		require.Equal(t, []int64{march}, activeCheckpointIDs(t, f))
	})

	t.Run("void", func(t *testing.T) {
		t.Parallel()
		f := newInvestmentsTestFixture(t)
		march, april, afterMarch := marchAndAprilCheckpoints(t, f)
		voided, err := f.transactionService.VoidTransaction(ctx, VoidTransactionInput{
			OwnerUserID: f.ownerUserID, OriginType: "browser_api", TransactionID: afterMarch.ID,
			ChangeReason: "duplicate", ReconciliationOverride: true,
		})
		require.NoError(t, err)
		require.Equal(t, []int64{april}, voided.InvalidatedCheckpointIDs)
		require.Equal(t, []int64{march}, activeCheckpointIDs(t, f))
	})
}

// A same-day reorder across March's boundary swaps which posting March
// reconciled. It used to be judged against April alone — both positions are
// inside April — so it went through without an override and left March active
// over a balance the ledger no longer has.
func TestPostingMoveAcrossEarlierSameDayCheckpointRequiresOverride(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := newInvestmentsTestFixture(t)
	march, april, afterMarch := marchAndAprilCheckpoints(t, f)
	input := MovePostingInput{
		OwnerUserID: f.ownerUserID, OriginType: "browser_api", AccountID: f.cashAccountID,
		PostingLineID: cashPostingLineID(t, f, afterMarch), Direction: "earlier",
	}

	_, err := f.transactionService.MovePosting(ctx, input)
	require.ErrorIs(t, err, ErrReconciliationOverrideRequired)
	require.ElementsMatch(t, []int64{march, april}, activeCheckpointIDs(t, f))

	input.ReconciliationOverride = true
	moved, err := f.transactionService.MovePosting(ctx, input)
	require.NoError(t, err)
	require.ElementsMatch(t, []int64{march, april}, moved.InvalidatedCheckpointIDs,
		"March changes, and every later checkpoint goes with it")
	require.Empty(t, activeCheckpointIDs(t, f))
}

// A reorder entirely after March's boundary changes neither checkpoint's
// balance: both positions stay after March and before April.
func TestPostingMoveAfterEveryCrossedBoundaryNeedsNoOverride(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := newInvestmentsTestFixture(t)
	march, april, afterMarch := marchAndAprilCheckpoints(t, f, 3)

	moved, err := f.transactionService.MovePosting(ctx, MovePostingInput{
		OwnerUserID: f.ownerUserID, OriginType: "browser_api", AccountID: f.cashAccountID,
		PostingLineID: cashPostingLineID(t, f, afterMarch), Direction: "later",
	})
	require.NoError(t, err)
	require.Empty(t, moved.InvalidatedCheckpointIDs)
	require.ElementsMatch(t, []int64{march, april}, activeCheckpointIDs(t, f))
}
