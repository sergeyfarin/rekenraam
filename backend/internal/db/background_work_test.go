package db

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The queue is the thing every durable job in the app stands on, and its two
// concurrency defences — the claim's re-check of the status it selected on, and
// the lease that lets a crashed worker's item be picked up again — had no test
// between them (review 2026-08-24, pass 5).

func TestClaimHandsOneItemToOneWorker(t *testing.T) {
	t.Parallel()

	database, _, _ := migratedInvestmentTestDatabase(t)
	repository := NewBackgroundWorkRepository(database)
	ctx := context.Background()

	const items = 8
	for i := range items {
		_, err := repository.EnqueueBackgroundWork(ctx, 1, "probe",
			`{"n":`+strconv.Itoa(i)+`}`, "2026-06-12T08:00:00Z")
		require.NoError(t, err)
	}

	// More workers than items, all claiming at once: every item must come out
	// exactly once and the queue must empty.
	//
	// What this cannot prove is the claim's *second* defence — the UPDATE
	// re-checking the status its SELECT chose on. Removing that re-check leaves
	// this test passing, because the main pool is SetMaxOpenConns(1) (ADR 0004)
	// and each claim transaction holds the one connection for its whole length,
	// so two SELECTs in this process never interleave. The re-check is there for
	// a second process on the same file, which no in-process test can stage.
	// #105 adds independent-process coverage in cmd/rekenraam's
	// TestTwoProcessWorkClaims. With the current deferred WAL transaction,
	// snapshot/write contention also prevents duplicate ownership; even that
	// test is not proof of this UPDATE predicate in isolation. Its fault checks
	// and limits are recorded in docs/reviews/two-process-sqlite-guards-2026-10-14.md.
	// Said out loud rather than left implied: a test that reads as proof of
	// something it cannot reach is the defect V-1 found.
	const workers = 16
	var (
		mutex   sync.Mutex
		claimed []int64
		start   sync.WaitGroup
		done    sync.WaitGroup
	)
	start.Add(1)
	for worker := range workers {
		done.Add(1)
		go func(worker int) {
			defer done.Done()
			start.Wait()
			item, err := repository.ClaimBackgroundWork(ctx, "probe",
				fmt.Sprintf("worker-%d", worker), "2026-06-12T08:00:00Z", "2026-06-12T08:05:00Z")
			if errors.Is(err, ErrNotFound) {
				return
			}
			if !assert.NoError(t, err) {
				return
			}
			mutex.Lock()
			claimed = append(claimed, item.ID)
			mutex.Unlock()
		}(worker)
	}
	start.Done()
	done.Wait()

	seen := map[int64]bool{}
	for _, id := range claimed {
		assert.Falsef(t, seen[id], "work item %d was claimed twice", id)
		seen[id] = true
	}
	assert.Len(t, claimed, items, "every item is claimed, and no item twice")

	// Nothing is left claimable: a ninth claim finds the queue empty rather
	// than re-handing out something already running under a live lease.
	_, err := repository.ClaimBackgroundWork(ctx, "probe", "latecomer",
		"2026-06-12T08:00:00Z", "2026-06-12T08:05:00Z")
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestAnExpiredLeaseIsReclaimedAndCountsAsAnotherAttempt(t *testing.T) {
	t.Parallel()

	database, _, _ := migratedInvestmentTestDatabase(t)
	repository := NewBackgroundWorkRepository(database)
	ctx := context.Background()

	_, err := repository.EnqueueBackgroundWork(ctx, 1, "probe", `{"n":1}`, "2026-06-12T08:00:00Z")
	require.NoError(t, err)

	first, err := repository.ClaimBackgroundWork(ctx, "probe", "worker-a",
		"2026-06-12T08:00:00Z", "2026-06-12T08:05:00Z")
	require.NoError(t, err)
	assert.Equal(t, 1, first.Attempts)

	// Still inside the lease: the item belongs to worker-a, crashed or not.
	_, err = repository.ClaimBackgroundWork(ctx, "probe", "worker-b",
		"2026-06-12T08:04:59Z", "2026-06-12T08:09:59Z")
	assert.ErrorIs(t, err, ErrNotFound, "a live lease is not stealable")

	// Past it: worker-a is presumed gone and the work is picked up again. The
	// attempt count rises, which is what stops a crash loop from retrying for
	// ever (the T-39 cap depends on this).
	second, err := repository.ClaimBackgroundWork(ctx, "probe", "worker-b",
		"2026-06-12T08:05:01Z", "2026-06-12T08:10:01Z")
	require.NoError(t, err)
	assert.Equal(t, first.ID, second.ID)
	assert.Equal(t, 2, second.Attempts, "a reclaim is another attempt, not a fresh start")
	assert.Equal(t, "worker-b", second.LeaseOwner.String)
}

func TestBackgroundWorkContinuationIsAtomicAndLeaseFenced(t *testing.T) {
	t.Parallel()
	for _, scenario := range []string{"same payload", "existing successor", "stale owner", "enqueue failure"} {
		t.Run(scenario, func(t *testing.T) {
			database, _, _ := migratedInvestmentTestDatabase(t)
			repository := NewBackgroundWorkRepository(database)
			ctx := context.Background()
			const now = "2026-06-12T08:00:00Z"
			const payload = `{"reason":"continuation"}`
			_, err := repository.EnqueueBackgroundWork(ctx, 1, "probe", payload, now)
			require.NoError(t, err)
			item, err := repository.ClaimBackgroundWork(ctx, "probe", "owner", now, "2026-06-12T08:15:00Z")
			require.NoError(t, err)
			successorPayload, owner := payload, "owner"
			if scenario == "existing successor" {
				successorPayload = `{"reason":"next"}`
				_, err = repository.EnqueueBackgroundWork(ctx, 1, "probe", successorPayload, now)
				require.NoError(t, err)
			}
			if scenario == "stale owner" {
				owner = "old-owner"
			}
			if scenario == "enqueue failure" {
				_, err = database.Exec(`CREATE TRIGGER refuse_continuation BEFORE INSERT ON background_work_items BEGIN SELECT RAISE(ABORT, 'injected enqueue failure'); END`)
				require.NoError(t, err)
			}
			err = repository.CompleteBackgroundWorkAndEnqueue(ctx, item.ID, owner, now, successorPayload)
			current, readErr := repository.BackgroundWorkByID(ctx, item.ID)
			require.NoError(t, readErr)
			if scenario == "stale owner" || scenario == "enqueue failure" {
				require.Error(t, err)
				assert.Equal(t, "running", current.Status)
				assert.Equal(t, "owner", current.LeaseOwner.String)
			} else {
				require.NoError(t, err)
				assert.Equal(t, "completed", current.Status)
				next, err := repository.ClaimBackgroundWork(ctx, "probe", "next-owner", now, "2026-06-12T08:15:00Z")
				require.NoError(t, err)
				assert.NotEqual(t, item.ID, next.ID)
				assert.Equal(t, successorPayload, next.PayloadJSON)
				assert.Equal(t, 1, next.Attempts)
				_, err = repository.ClaimBackgroundWork(ctx, "probe", "extra-owner", now, "2026-06-12T08:15:00Z")
				assert.ErrorIs(t, err, ErrNotFound)
			}
		})
	}
}

func TestBackgroundWorkSuccessorsCommitTogetherAndPreserveRetryBudget(t *testing.T) {
	t.Parallel()
	for _, scenario := range []string{"success", "second insert failure", "stale owner", "active retry"} {
		t.Run(scenario, func(t *testing.T) {
			database, _, _ := migratedInvestmentTestDatabase(t)
			r := NewBackgroundWorkRepository(database)
			ctx := context.Background()
			const now = "2026-06-12T08:00:00Z"
			_, err := r.EnqueueBackgroundWork(ctx, 1, "probe", `{}`, now)
			require.NoError(t, err)
			parent, err := r.ClaimBackgroundWork(ctx, "probe", "owner", now, "2026-06-12T10:00:00Z")
			require.NoError(t, err)
			successors := []BackgroundWorkSuccessor{
				{PayloadJSON: `{"retry":1}`, AvailableAt: "2026-06-12T08:01:00Z", Attempts: 1},
				{PayloadJSON: `{"next":2}`, AvailableAt: now},
			}
			owner := "owner"
			if scenario == "stale owner" {
				owner = "old-owner"
			}
			if scenario == "second insert failure" {
				_, err = database.Exec(`CREATE TRIGGER refuse_next BEFORE INSERT ON background_work_items WHEN json_extract(NEW.payload_json,'$.next')=2 BEGIN SELECT RAISE(ABORT, 'injected second insert failure'); END`)
				require.NoError(t, err)
			}
			var active BackgroundWorkItemRecord
			if scenario == "active retry" {
				_, err = r.EnqueueBackgroundWork(ctx, 1, "probe", successors[0].PayloadJSON, now)
				require.NoError(t, err)
				active, err = r.ClaimBackgroundWork(ctx, "probe", "retry-owner", now, "2026-06-12T08:15:00Z")
				require.NoError(t, err)
				require.NoError(t, r.RetryBackgroundWork(ctx, active.ID, "retry-owner", "2026-06-12T09:00:00Z", now, "provider failed"))
				active, err = r.ClaimBackgroundWork(ctx, "probe", "retry-owner", "2026-06-12T09:00:00Z", "2026-06-12T09:15:00Z")
				require.NoError(t, err)
			}
			err = r.CompleteBackgroundWorkWithSuccessors(ctx, parent.ID, owner, now, successors)
			current, readErr := r.BackgroundWorkByID(ctx, parent.ID)
			require.NoError(t, readErr)
			var count int
			require.NoError(t, database.QueryRow(`SELECT count(*) FROM background_work_items WHERE kind='probe'`).Scan(&count))
			if scenario == "second insert failure" || scenario == "stale owner" {
				require.Error(t, err)
				assert.Equal(t, "running", current.Status)
				assert.Equal(t, 1, count, "no partial retry or continuation")
				return
			}
			require.NoError(t, err)
			assert.Equal(t, "completed", current.Status)
			assert.Equal(t, 3, count)
			if scenario == "active retry" {
				current, err = r.BackgroundWorkByID(ctx, active.ID)
				require.NoError(t, err)
				assert.Equal(t, active.Attempts, current.Attempts)
				assert.Equal(t, active.LeaseOwner, current.LeaseOwner)
				assert.Equal(t, active.AvailableAt, current.AvailableAt)
			} else {
				item, err := r.ClaimBackgroundWork(ctx, "probe", "next", now, "2026-06-12T08:15:00Z")
				require.NoError(t, err)
				assert.Equal(t, successors[1].PayloadJSON, item.PayloadJSON)
				item, err = r.ClaimBackgroundWork(ctx, "probe", "retry", "2026-06-12T08:01:00Z", "2026-06-12T08:16:00Z")
				require.NoError(t, err)
				assert.Equal(t, 2, item.Attempts, "the parent's failed download counts toward the cap")
			}
		})
	}
}
