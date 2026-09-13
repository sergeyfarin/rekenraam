package app

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"rekenraam/backend/internal/db"
	"rekenraam/backend/internal/exact"
)

// T-94. The reconciliation guard used to be split across a read and a write
// that were not the same transaction: the service resolved which checkpoints a
// spec would invalidate before BeginTx, refused the write if that list was
// non-empty, and the repository then invalidated whatever list it had been
// handed — treating an empty one as "nothing to do" rather than as a claim
// about checkpoint state that might no longer hold.
//
// Anything that finished a reconciliation in between therefore got a posting
// into its reconciled period with the checkpoint left active. That needs no
// second process and no Go data race: one import commit running while the user
// finishes a reconciliation in another tab is enough. Nothing afterwards can
// see it either, because checkpointIntegrityCheck sums only the postings linked
// to a checkpoint and the stale entry is linked to none.
//
// The guard now lives inside the write transaction, and the service passes
// candidates — pure positions derived from the spec — instead of a resolved
// answer, so there is no stale decision left to carry.

func postEUR(t *testing.T, f *investmentsTestFixture, date string, amount int64) Transaction {
	t.Helper()
	created, err := f.transactionService.CreateTransaction(context.Background(), CreateTransactionInput{
		OwnerUserID: f.ownerUserID, OriginType: "browser_api",
		Spec: TransactionInput{
			TransactionDate: date,
			JournalEntries: []JournalEntryInput{{Postings: []PostingInput{
				{AccountID: f.cashAccountID, CommodityID: f.eurCommodityID, QuantityValue: exact.New(amount)},
				{AccountID: f.incomeAccountID, CommodityID: f.eurCommodityID, QuantityValue: exact.New(-amount)},
			}}},
		},
	})
	require.NoError(t, err)
	return created
}

// reconcileCash reconciles every currently uncleared cash posting to a
// statement and returns the checkpoint that results.
func reconcileCash(t *testing.T, f *investmentsTestFixture, statementDate string, balance int64) int64 {
	t.Helper()
	ctx := context.Background()
	session, err := f.transactionService.StartReconciliation(ctx, StartReconciliationInput{
		OriginType: "browser_api", OwnerUserID: f.ownerUserID, AccountID: f.cashAccountID,
		CommodityID: f.eurCommodityID, StatementDate: statementDate, StatementBalanceValue: exact.New(balance),
	})
	require.NoError(t, err)
	postingIDs := make([]int64, 0, len(session.Candidates))
	for _, candidate := range session.Candidates {
		postingIDs = append(postingIDs, candidate.PostingID)
	}
	_, err = f.transactionService.UpdateReconciliationSelection(ctx, ReconciliationSelectionInput{
		OriginType: "browser_api", OwnerUserID: f.ownerUserID, SessionID: session.ID, PostingVersionIDs: postingIDs,
	})
	require.NoError(t, err)
	_, err = f.transactionService.FinishReconciliation(ctx, FinishReconciliationInput{
		OriginType: "browser_api", OwnerUserID: f.ownerUserID, SessionID: session.ID, ChangeReason: "statement verified",
	})
	require.NoError(t, err)

	checkpoints, err := f.transactionService.repository.ListReconciliationCheckpoints(ctx, BookID, f.cashAccountID, f.eurCommodityID)
	require.NoError(t, err)
	for _, checkpoint := range checkpoints {
		if checkpoint.Status == "active" {
			return checkpoint.ID
		}
	}
	t.Fatal("no active checkpoint after finishing a reconciliation")
	return 0
}

func activeCheckpointIDs(t *testing.T, f *investmentsTestFixture) []int64 {
	t.Helper()
	checkpoints, err := f.transactionService.repository.ListReconciliationCheckpoints(context.Background(), BookID, f.cashAccountID, f.eurCommodityID)
	require.NoError(t, err)
	active := make([]int64, 0, len(checkpoints))
	for _, checkpoint := range checkpoints {
		if checkpoint.Status == "active" {
			active = append(active, checkpoint.ID)
		}
	}
	return active
}

// The interleaving itself: validate, let a reconciliation land, then commit.
// The prepared write carries no override, so the commit must refuse rather than
// slip a posting into a period that is now reconciled.
func TestCreateTransactionRejectsACheckpointCreatedAfterValidation(t *testing.T) {
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()

	postEUR(t, f, "2026-01-01", 10)
	input := CreateTransactionInput{
		OwnerUserID: f.ownerUserID, OriginType: "browser_api",
		Spec: TransactionInput{
			TransactionDate: "2026-01-02",
			JournalEntries: []JournalEntryInput{{Postings: []PostingInput{
				{AccountID: f.cashAccountID, CommodityID: f.eurCommodityID, QuantityValue: exact.New(10)},
				{AccountID: f.incomeAccountID, CommodityID: f.eurCommodityID, QuantityValue: exact.New(-10)},
			}}},
		},
	}

	// Validated while nothing was reconciled: this is the point the old code
	// made its decision and then kept.
	params, err := f.transactionService.prepareCreateTransactionForWrite(ctx, input)
	require.NoError(t, err)

	checkpointID := reconcileCash(t, f, "2026-01-31", 10)

	_, err = f.transactionService.repository.CreateTransaction(ctx, params)
	require.ErrorIs(t, err, db.ErrReconciliationOverrideRequired,
		"the commit must decide against the checkpoints that exist when it commits")

	require.Equal(t, []int64{checkpointID}, activeCheckpointIDs(t, f), "the checkpoint stands, and so does its claim")
	var transactionCount int
	require.NoError(t, f.database.QueryRowContext(ctx, `SELECT count(*) FROM transactions`).Scan(&transactionCount))
	require.Equal(t, 1, transactionCount, "the refused write left nothing behind")
}

// The same interleaving with an override granted. This is the half that a
// pre-resolved ref list gets wrong in the other direction: the list was empty
// when it was built, so the write would have committed and invalidated nothing,
// leaving an active checkpoint asserting a balance the new posting just changed.
func TestCreateTransactionWithOverrideInvalidatesACheckpointCreatedAfterValidation(t *testing.T) {
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()

	postEUR(t, f, "2026-01-01", 10)
	input := CreateTransactionInput{
		OwnerUserID: f.ownerUserID, OriginType: "browser_api", ReconciliationOverride: true,
		ChangeReason: "entered late, statement already reconciled",
		Spec: TransactionInput{
			TransactionDate: "2026-01-02",
			JournalEntries: []JournalEntryInput{{Postings: []PostingInput{
				{AccountID: f.cashAccountID, CommodityID: f.eurCommodityID, QuantityValue: exact.New(10)},
				{AccountID: f.incomeAccountID, CommodityID: f.eurCommodityID, QuantityValue: exact.New(-10)},
			}}},
		},
	}
	params, err := f.transactionService.prepareCreateTransactionForWrite(ctx, input)
	require.NoError(t, err)

	checkpointID := reconcileCash(t, f, "2026-01-31", 10)

	record, err := f.transactionService.repository.CreateTransaction(ctx, params)
	require.NoError(t, err)
	require.Equal(t, []int64{checkpointID}, record.InvalidatedCheckpointIDs,
		"a checkpoint that appeared after validation must still be invalidated by the write that crosses it")
	require.Empty(t, activeCheckpointIDs(t, f))
}

// A checkpoint can also move rather than appear. Reconciling again supersedes
// the first checkpoint with a later one, and the write has to invalidate
// whichever is active at commit time, not the one it saw while validating.
func TestCreateTransactionInvalidatesTheCheckpointActiveAtCommitTime(t *testing.T) {
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()

	postEUR(t, f, "2026-01-01", 10)
	firstCheckpointID := reconcileCash(t, f, "2026-01-31", 10)

	input := CreateTransactionInput{
		OwnerUserID: f.ownerUserID, OriginType: "browser_api", ReconciliationOverride: true,
		ChangeReason: "entered late, statement already reconciled",
		Spec: TransactionInput{
			TransactionDate: "2026-01-15",
			JournalEntries: []JournalEntryInput{{Postings: []PostingInput{
				{AccountID: f.cashAccountID, CommodityID: f.eurCommodityID, QuantityValue: exact.New(5)},
				{AccountID: f.incomeAccountID, CommodityID: f.eurCommodityID, QuantityValue: exact.New(-5)},
			}}},
		},
	}
	params, err := f.transactionService.prepareCreateTransactionForWrite(ctx, input)
	require.NoError(t, err)

	// Another posting arrives and is reconciled on a later statement, which
	// supersedes the checkpoint the preparation above resolved against.
	postEUR(t, f, "2026-02-01", 10)
	secondCheckpointID := reconcileCash(t, f, "2026-02-28", 20)
	require.NotEqual(t, firstCheckpointID, secondCheckpointID)

	record, err := f.transactionService.repository.CreateTransaction(ctx, params)
	require.NoError(t, err)
	// The January posting sits inside both statements' periods, so both go —
	// what matters here is that the later checkpoint, which did not exist when
	// the write was validated, is among them.
	require.Contains(t, record.InvalidatedCheckpointIDs, secondCheckpointID,
		"the write invalidates the checkpoint that is active now, not only the one validation saw")
	require.Empty(t, activeCheckpointIDs(t, f))
}

// The other write paths take the same treatment, so the repository must refuse
// them on its own rather than trusting that a caller checked. Calling the
// repository directly is the point: it stands in for any caller whose own check
// has gone stale, and for a future caller that forgets to check at all.
func TestWritePathsEnforceTheCheckpointBoundaryInTheRepository(t *testing.T) {
	ctx := context.Background()

	setup := func(t *testing.T) (*investmentsTestFixture, Transaction, []db.PeriodScopedCheckpointRef) {
		t.Helper()
		f := newInvestmentsTestFixture(t)
		created := postEUR(t, f, "2026-01-01", 10)
		reconcileCash(t, f, "2026-01-31", 10)
		current, err := f.transactionService.repository.TransactionByID(ctx, BookID, created.ID)
		require.NoError(t, err)
		return f, created, periodScopedCandidatesFromRecord(current)
	}

	t.Run("update", func(t *testing.T) {
		f, created, candidates := setup(t)
		current, err := f.transactionService.repository.TransactionByID(ctx, BookID, created.ID)
		require.NoError(t, err)
		spec, err := f.transactionService.cleanTransactionSpec(ctx, transactionInputFromTransaction(created), cleanTransactionOptions{
			ForcedStatus: "posted", ExistingLineKeys: lineKeySet(current), ExistingPostings: existingPostingStateSet(current),
		})
		require.NoError(t, err)
		_, err = f.transactionService.repository.UpdateTransaction(ctx, db.UpdateTransactionParams{
			BookID: BookID, TransactionID: created.ID, ActorUserID: f.ownerUserID,
			OriginType: "browser_api", Operation: "transaction.update",
			Spec: spec, RecordedAt: "2026-03-01T00:00:00Z",
			ChangeReason: "edited", CheckpointCandidates: candidates,
		})
		require.ErrorIs(t, err, db.ErrReconciliationOverrideRequired)
	})

	t.Run("void", func(t *testing.T) {
		f, created, candidates := setup(t)
		_, err := f.transactionService.repository.VoidTransaction(ctx, db.VoidTransactionParams{
			BookID: BookID, TransactionID: created.ID, ActorUserID: f.ownerUserID,
			OriginType: "browser_api", Operation: "transaction.void", RecordedAt: "2026-03-01T00:00:00Z",
			ChangeReason: "voided", CheckpointCandidates: candidates,
		})
		require.ErrorIs(t, err, db.ErrReconciliationOverrideRequired)
	})

	t.Run("soft_delete", func(t *testing.T) {
		f, created, candidates := setup(t)
		_, err := f.transactionService.repository.SetTransactionDeleted(ctx, db.SetTransactionDeletedParams{
			TransactionLifecycleParams: db.TransactionLifecycleParams{
				BookID: BookID, TransactionID: created.ID, ActorUserID: f.ownerUserID,
				OriginType: "browser_api", Operation: "transaction.soft_delete", RecordedAt: "2026-03-01T00:00:00Z",
				ChangeReason: "deleted", CheckpointCandidates: candidates,
			}, Deleted: true,
		})
		require.ErrorIs(t, err, db.ErrReconciliationOverrideRequired)
	})
}
