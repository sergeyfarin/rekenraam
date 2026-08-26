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
	for i := 0; i < items; i++ {
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
	for worker := 0; worker < workers; worker++ {
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
