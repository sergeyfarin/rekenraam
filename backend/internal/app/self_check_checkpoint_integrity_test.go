package app

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"rekenraam/backend/internal/exact"
)

// checkpoint_integrity reported healthy books as damaged in two ways. It
// compared a checkpoint's statement balance with the postings that session
// cleared alone, ignoring the starting balance the session carried from the
// previous checkpoint, so every second reconciliation failed. And it treated a
// snapshot posting as superseded whenever its transaction gained a version, so
// a description edit — always allowed without a guard — failed it too. The
// check now adds the starting balance, and a posting is stale only when its
// line's current financial facts or position no longer match the snapshot.

func checkpointIntegrity(t *testing.T, f *investmentsTestFixture) SelfCheckResult {
	t.Helper()
	run, err := selfCheckOver(t, f.database).RunSelfCheck(context.Background(), "manual")
	require.NoError(t, err)
	for _, result := range run.Results {
		if result.CheckID == CheckCheckpointIntegrity {
			return result
		}
	}
	t.Fatal("self-check has no checkpoint integrity result")
	return SelfCheckResult{}
}

func TestCheckpointIntegrityPassesAcrossChainedReconciliations(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	postEUR(t, f, "2026-03-31", 10)
	reconcileCash(t, f, "2026-03-31", 10)
	postEUR(t, f, "2026-03-31", 7)
	postEUR(t, f, "2026-04-10", 10)
	reconcileCash(t, f, "2026-04-30", 27)
	require.Len(t, activeCheckpointIDs(t, f), 2)
	result := checkpointIntegrity(t, f)
	require.Equal(t, SelfCheckPassed, result.Status, result.Summary)
}

func TestCheckpointIntegrityIgnoresNonFinancialEdits(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := newInvestmentsTestFixture(t)
	created := postEUR(t, f, "2026-03-01", 10)
	reconcileCash(t, f, "2026-03-31", 10)
	spec := transactionInputFromTransaction(created)
	spec.Description = "corrected description"
	_, err := f.transactionService.UpdateTransaction(ctx, UpdateTransactionInput{
		OwnerUserID: f.ownerUserID, OriginType: "browser_api", TransactionID: created.ID,
		Spec: spec, ChangeReason: "description only",
	})
	require.NoError(t, err)
	require.Len(t, activeCheckpointIDs(t, f), 1, "a non-financial edit needs no guard")
	result := checkpointIntegrity(t, f)
	require.Equal(t, SelfCheckPassed, result.Status, result.Summary)
}

// The true positives the check exists for still fail it: a reconciled amount
// changed under a checkpoint left active, and a statement balance that no
// longer adds up.
func TestCheckpointIntegrityStillFlagsChangedReconciledFacts(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("changed amount under an active checkpoint", func(t *testing.T) {
		t.Parallel()
		f := newInvestmentsTestFixture(t)
		created := postEUR(t, f, "2026-03-01", 10)
		checkpointID := reconcileCash(t, f, "2026-03-31", 10)
		spec := transactionInputFromTransaction(created)
		spec.JournalEntries[0].Postings[0].QuantityValue = exact.New(12)
		spec.JournalEntries[0].Postings[1].QuantityValue = exact.New(-12)
		_, err := f.transactionService.UpdateTransaction(ctx, UpdateTransactionInput{
			OwnerUserID: f.ownerUserID, OriginType: "browser_api", TransactionID: created.ID,
			Spec: spec, ChangeReason: "amount", ReconciliationOverride: true,
		})
		require.NoError(t, err)
		// Damage: the checkpoint the override invalidated is put back.
		_, err = f.database.ExecContext(ctx, `UPDATE reconciliation_checkpoints SET status = 'active' WHERE id = ?`, checkpointID)
		require.NoError(t, err)
		result := checkpointIntegrity(t, f)
		require.Equal(t, SelfCheckFailed, result.Status)
		require.Contains(t, result.Summary, "superseded postings")
		require.Equal(t, []int64{checkpointID}, result.Sample)
	})

	t.Run("statement balance no longer adds up", func(t *testing.T) {
		t.Parallel()
		f := newInvestmentsTestFixture(t)
		postEUR(t, f, "2026-03-01", 10)
		reconcileCash(t, f, "2026-03-31", 10)
		postEUR(t, f, "2026-04-01", 5)
		checkpointID := reconcileCash(t, f, "2026-04-30", 15)
		_, err := f.database.ExecContext(ctx, `UPDATE reconciliation_checkpoints SET statement_balance_value = '16' WHERE id = ?`, checkpointID)
		require.NoError(t, err)
		result := checkpointIntegrity(t, f)
		require.Equal(t, SelfCheckFailed, result.Status)
		require.Contains(t, result.Summary, "no longer sum")
		require.Equal(t, []int64{checkpointID}, result.Sample)
	})
}
