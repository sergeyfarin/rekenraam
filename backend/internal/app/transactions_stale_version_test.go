package app

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"rekenraam/backend/internal/db"
	"rekenraam/backend/internal/exact"
)

// T-94, second pass. Moving the checkpoint guard inside the write transaction
// closed the stale-*checkpoint* hole, but the write still applied a spec and a
// candidate list computed against a transaction version read before it. Those
// two halves can describe different edits: a description-only edit produces an
// empty candidate list — correctly, at the time — and by the time it commits,
// another request has changed the amount and reconciled it. Applying the
// prepared spec then restores the old amount underneath an active checkpoint,
// with no override anywhere in sight.
//
// A write now names the version it was prepared against, and the write
// transaction refuses to apply it to anything else.

// preparedMetadataEdit returns the spec and candidates a description-only edit
// produces, exactly as the service computes them, together with the version
// they describe.
func preparedMetadataEdit(t *testing.T, f *investmentsTestFixture, transaction Transaction) (db.TransactionSpec, []db.PeriodScopedCheckpointRef, int64) {
	t.Helper()
	ctx := context.Background()
	current, err := f.transactionService.repository.TransactionByID(ctx, BookID, transaction.ID)
	require.NoError(t, err)
	input := transactionInputFromTransaction(transaction)
	input.Description = "memo edit"
	spec, err := f.transactionService.cleanTransactionSpec(ctx, input, cleanTransactionOptions{
		ForcedStatus: "posted", ExistingLineKeys: lineKeySet(current), ExistingPostings: existingPostingStateSet(current),
	})
	require.NoError(t, err)
	candidates := reconciliationCandidates(current, spec)
	require.Empty(t, candidates, "a description-only edit touches no reconciled position")
	return spec, candidates, current.VersionID
}

func TestStaleMetadataEditCannotOverwriteANewerReconciledAmount(t *testing.T) {
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	original := postEUR(t, f, "2026-01-01", 10)

	spec, candidates, preparedVersionID := preparedMetadataEdit(t, f, original)

	// Meanwhile: the amount changes and the new amount is reconciled.
	changed := transactionInputFromTransaction(original)
	changed.JournalEntries[0].Postings[0].QuantityValue = exact.New(20)
	changed.JournalEntries[0].Postings[1].QuantityValue = exact.New(-20)
	_, err := f.transactionService.UpdateTransaction(ctx, UpdateTransactionInput{
		OwnerUserID: f.ownerUserID, OriginType: "browser_api", TransactionID: original.ID, Spec: changed,
	})
	require.NoError(t, err)
	checkpointID := reconcileCash(t, f, "2026-01-31", 20)

	_, err = f.transactionService.repository.UpdateTransaction(ctx, db.UpdateTransactionParams{
		BookID: BookID, TransactionID: original.ID, ActorUserID: f.ownerUserID,
		OriginType: "browser_api", Operation: "transaction.update", Spec: spec,
		RecordedAt: "2026-09-13T12:00:00Z", ChangeReason: "memo edit",
		CheckpointCandidates: candidates, ExpectedVersionID: preparedVersionID,
	})
	require.ErrorIs(t, err, db.ErrTransactionVersionStale)

	// The reconciled amount and its checkpoint are both untouched.
	after, err := f.transactionService.repository.TransactionByID(ctx, BookID, original.ID)
	require.NoError(t, err)
	require.Equal(t, "20", after.JournalEntries[0].Postings[0].QuantityValue.String())
	require.Equal(t, []int64{checkpointID}, activeCheckpointIDs(t, f))
}

func TestEveryLifecycleWriteRefusesASupersededVersion(t *testing.T) {
	ctx := context.Background()

	// Each case leaves the transaction in the state its write needs, captures
	// the version at that point, then lets one more write supersede it.
	cases := []struct {
		name    string
		prepare func(t *testing.T, f *investmentsTestFixture, id int64) int64
		write   func(f *investmentsTestFixture, id int64, staleVersionID int64) error
	}{
		{
			name: "update",
			prepare: func(t *testing.T, f *investmentsTestFixture, id int64) int64 {
				stale := currentVersionID(t, f, id)
				editAmount(t, f, id, 20)
				return stale
			},
			write: func(f *investmentsTestFixture, id int64, staleVersionID int64) error {
				_, err := f.transactionService.repository.UpdateTransaction(ctx, db.UpdateTransactionParams{
					BookID: BookID, TransactionID: id, ActorUserID: f.ownerUserID,
					OriginType: "browser_api", Operation: "transaction.update", Spec: currentSpec(f, id),
					RecordedAt: "2026-09-13T12:00:00Z", ChangeReason: "edited", ExpectedVersionID: staleVersionID,
				})
				return err
			},
		},
		{
			name: "void",
			prepare: func(t *testing.T, f *investmentsTestFixture, id int64) int64 {
				stale := currentVersionID(t, f, id)
				editAmount(t, f, id, 20)
				return stale
			},
			write: func(f *investmentsTestFixture, id int64, staleVersionID int64) error {
				_, err := f.transactionService.repository.VoidTransaction(ctx, db.VoidTransactionParams{
					BookID: BookID, TransactionID: id, ActorUserID: f.ownerUserID,
					OriginType: "browser_api", Operation: "transaction.void",
					RecordedAt: "2026-09-13T12:00:00Z", ChangeReason: "voided", ExpectedVersionID: staleVersionID,
				})
				return err
			},
		},
		{
			name: "unvoid",
			prepare: func(t *testing.T, f *investmentsTestFixture, id int64) int64 {
				voidTransaction(t, f, id)
				stale := currentVersionID(t, f, id)
				unvoidTransaction(t, f, id)
				voidTransaction(t, f, id)
				return stale
			},
			write: func(f *investmentsTestFixture, id int64, staleVersionID int64) error {
				_, err := f.transactionService.repository.UnvoidTransaction(ctx, db.TransactionLifecycleParams{
					BookID: BookID, TransactionID: id, ActorUserID: f.ownerUserID,
					OriginType: "browser_api", Operation: "transaction.unvoid",
					RecordedAt: "2026-09-13T12:00:00Z", ChangeReason: "unvoided", ExpectedVersionID: staleVersionID,
				})
				return err
			},
		},
		{
			name: "soft_delete",
			prepare: func(t *testing.T, f *investmentsTestFixture, id int64) int64 {
				stale := currentVersionID(t, f, id)
				editAmount(t, f, id, 20)
				return stale
			},
			write: func(f *investmentsTestFixture, id int64, staleVersionID int64) error {
				_, err := f.transactionService.repository.SetTransactionDeleted(ctx, db.SetTransactionDeletedParams{
					TransactionLifecycleParams: db.TransactionLifecycleParams{
						BookID: BookID, TransactionID: id, ActorUserID: f.ownerUserID,
						OriginType: "browser_api", Operation: "transaction.soft_delete",
						RecordedAt: "2026-09-13T12:00:00Z", ChangeReason: "deleted", ExpectedVersionID: staleVersionID,
					}, Deleted: true,
				})
				return err
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newInvestmentsTestFixture(t)
			created := postEUR(t, f, "2026-01-01", 10)
			staleVersionID := tc.prepare(t, f, created.ID)

			require.ErrorIs(t, tc.write(f, created.ID, staleVersionID), db.ErrTransactionVersionStale)

			// And with no version at all — the case of a future caller that
			// adds a write path and forgets the check exists.
			require.ErrorIs(t, tc.write(f, created.ID, 0), db.ErrTransactionVersionStale)
		})
	}
}

func TestStaleDraftPromotionIsRefused(t *testing.T) {
	f, recurring, template := recurringFixture(t)
	ctx := context.Background()
	_, err := recurring.CreateTemplate(ctx, template)
	require.NoError(t, err)
	_, err = recurring.GenerateDue(ctx, GenerateRecurringInput{OwnerUserID: f.ownerUserID})
	require.NoError(t, err)

	draftID := soleDraftID(t, f)
	draft, err := f.transactionService.Transaction(ctx, draftID)
	require.NoError(t, err)
	require.Equal(t, "draft", draft.Status)

	// A promotion prepared against this version: the candidates are the
	// postings' real positions, because promotion changes no posting's own
	// fields.
	current, err := f.transactionService.repository.TransactionByID(ctx, BookID, draftID)
	require.NoError(t, err)
	staleVersionID := current.VersionID
	promotion := transactionInputFromTransaction(draft)
	promotion.Status = "posted"
	spec, err := f.transactionService.cleanTransactionSpec(ctx, promotion, cleanTransactionOptions{
		ForcedStatus: "posted", ExistingLineKeys: lineKeySet(current), ExistingPostings: existingPostingStateSet(current),
	})
	require.NoError(t, err)
	candidates := periodScopedCandidatesFromRecord(current)

	// The draft is edited while the promotion is in flight.
	edited := transactionInputFromTransaction(draft)
	edited.Description = "reviewed"
	_, err = f.transactionService.UpdateTransaction(ctx, UpdateTransactionInput{
		OwnerUserID: f.ownerUserID, OriginType: "browser_api", TransactionID: draftID, Spec: edited,
	})
	require.NoError(t, err)

	_, err = f.transactionService.repository.UpdateTransaction(ctx, db.UpdateTransactionParams{
		BookID: BookID, TransactionID: draftID, ActorUserID: f.ownerUserID,
		OriginType: "browser_api", Operation: "transaction.post", Spec: spec,
		RecordedAt: "2026-09-13T12:00:00Z", ChangeReason: "posted",
		CheckpointCandidates: candidates, ExpectedVersionID: staleVersionID,
	})
	require.ErrorIs(t, err, db.ErrTransactionVersionStale)

	after, err := f.transactionService.Transaction(ctx, draftID)
	require.NoError(t, err)
	require.Equal(t, "draft", after.Status, "the stale promotion did not post the draft")
}

// The guard must not turn every ordinary edit into something that needs a
// reconciliation override: a metadata-only edit against the current version is
// exactly as allowed as it was before.
func TestOrdinaryMetadataEditOfAReconciledTransactionStillCommits(t *testing.T) {
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	created := postEUR(t, f, "2026-01-01", 10)
	checkpointID := reconcileCash(t, f, "2026-01-31", 10)

	input := transactionInputFromTransaction(created)
	input.Description = "renamed after reconciling"
	updated, err := f.transactionService.UpdateTransaction(ctx, UpdateTransactionInput{
		OwnerUserID: f.ownerUserID, OriginType: "browser_api", TransactionID: created.ID, Spec: input,
	})
	require.NoError(t, err)
	require.Equal(t, "renamed after reconciling", updated.Description)
	require.Equal(t, []int64{checkpointID}, activeCheckpointIDs(t, f), "the checkpoint stays active")
}

func currentVersionID(t *testing.T, f *investmentsTestFixture, id int64) int64 {
	t.Helper()
	record, err := f.transactionService.repository.TransactionByID(context.Background(), BookID, id)
	require.NoError(t, err)
	return record.VersionID
}

func editAmount(t *testing.T, f *investmentsTestFixture, id int64, amount int64) {
	t.Helper()
	ctx := context.Background()
	current, err := f.transactionService.Transaction(ctx, id)
	require.NoError(t, err)
	spec := transactionInputFromTransaction(current)
	spec.JournalEntries[0].Postings[0].QuantityValue = exact.New(amount)
	spec.JournalEntries[0].Postings[1].QuantityValue = exact.New(-amount)
	_, err = f.transactionService.UpdateTransaction(ctx, UpdateTransactionInput{
		OwnerUserID: f.ownerUserID, OriginType: "browser_api", TransactionID: id, Spec: spec,
	})
	require.NoError(t, err)
}

func voidTransaction(t *testing.T, f *investmentsTestFixture, id int64) {
	t.Helper()
	_, err := f.transactionService.VoidTransaction(context.Background(), TransactionLifecycleInput{
		OwnerUserID: f.ownerUserID, OriginType: "browser_api", TransactionID: id, ChangeReason: "voided",
	})
	require.NoError(t, err)
}

func unvoidTransaction(t *testing.T, f *investmentsTestFixture, id int64) {
	t.Helper()
	_, err := f.transactionService.UnvoidTransaction(context.Background(), TransactionLifecycleInput{
		OwnerUserID: f.ownerUserID, OriginType: "browser_api", TransactionID: id, ChangeReason: "unvoided",
	})
	require.NoError(t, err)
}

// currentSpec is a valid spec for the transaction exactly as it stands, so the
// only thing wrong with the write that carries it is the version it names.
func currentSpec(f *investmentsTestFixture, id int64) db.TransactionSpec {
	ctx := context.Background()
	transaction, err := f.transactionService.Transaction(ctx, id)
	if err != nil {
		panic(err)
	}
	current, err := f.transactionService.repository.TransactionByID(ctx, BookID, id)
	if err != nil {
		panic(err)
	}
	spec, err := f.transactionService.cleanTransactionSpec(ctx, transactionInputFromTransaction(transaction), cleanTransactionOptions{
		ForcedStatus: "posted", ExistingLineKeys: lineKeySet(current), ExistingPostings: existingPostingStateSet(current),
	})
	if err != nil {
		panic(err)
	}
	return spec
}

func soleDraftID(t *testing.T, f *investmentsTestFixture) int64 {
	t.Helper()
	var id int64
	require.NoError(t, f.database.QueryRow(`
		SELECT transaction_id FROM current_transaction_versions WHERE status = 'draft'
	`).Scan(&id))
	return id
}
