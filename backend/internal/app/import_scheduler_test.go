package app

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rekenraam/backend/internal/db"
)

func enableAutoRefresh(t *testing.T, connService *ImportConnectionService, conn ImportConnection) {
	t.Helper()
	_, err := connService.UpdateImportConnection(context.Background(), UpdateImportConnectionInput{
		OwnerUserID:        1,
		ID:                 conn.ID,
		DisplayName:        conn.DisplayName,
		ConfigJSON:         conn.ConfigJSON,
		AutoRefreshEnabled: boolPtr(true),
	})
	require.NoError(t, err)
}

func countBackgroundWork(t *testing.T, database *sql.DB, kind string) int {
	t.Helper()
	var count int
	require.NoError(t, database.QueryRow(`SELECT COUNT(*) FROM background_work_items WHERE kind = ?`, kind).Scan(&count))
	return count
}

// TestRunDueTrading212AutoRefreshes_DisabledConnectionNeverTriggers confirms
// the default (auto_refresh_enabled=0, matching the migration's DEFAULT 0)
// never fires the scheduler — no regression for every connection created
// before Slice 4a shipped.
func TestRunDueTrading212AutoRefreshes_DisabledConnectionNeverTriggers(t *testing.T) {
	svc, connService, _, database := newImportFetchTestService(t)
	createTestTrading212Connection(t, svc, connService, "http://example.invalid")

	svc.runDueTrading212AutoRefreshes(context.Background(), testWorkerLogger())

	assert.Equal(t, 0, countBackgroundWork(t, database, trading212FetchWorkKind))
}

// TestRunDueTrading212AutoRefreshes_NeverFetchedTriggersImmediately confirms
// enabling auto-refresh on a connection that has never been fetched doesn't
// wait a full interval — first-time enable should trigger on the very next
// tick.
func TestRunDueTrading212AutoRefreshes_NeverFetchedTriggersImmediately(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeFakeT212JSON(w, nil)
	}))
	defer server.Close()

	svc, connService, _, database := newImportFetchTestService(t)
	conn := createTestTrading212Connection(t, svc, connService, server.URL)
	enableAutoRefresh(t, connService, conn)

	svc.runDueTrading212AutoRefreshes(context.Background(), testWorkerLogger())

	assert.Equal(t, 1, countBackgroundWork(t, database, trading212FetchWorkKind))
}

// TestRunDueTrading212AutoRefreshes_BoundaryJustUnderAndOverInterval checks
// the 24h-since-last-attempt cadence at its boundary: a connection fetched
// just under the interval ago must not trigger, and the same connection
// considered from just over the interval later must.
func TestRunDueTrading212AutoRefreshes_BoundaryJustUnderAndOverInterval(t *testing.T) {
	restore := SetAutoRefreshIntervalForTest(24 * time.Hour)
	defer restore()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeFakeT212JSON(w, nil)
	}))
	defer server.Close()

	svc, connService, _, database := newImportFetchTestService(t)
	conn := createTestTrading212Connection(t, svc, connService, server.URL)
	enableAutoRefresh(t, connService, conn)

	lastFetched := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	require.NoError(t, connService.UpdateFetchCursor(context.Background(), conn.ID, "", "", "", "ready", lastFetched.Format(time.RFC3339)))

	// Just under 24h later: not due yet.
	svc.SetNowForTest(func() time.Time { return lastFetched.Add(23*time.Hour + 59*time.Minute) })
	svc.runDueTrading212AutoRefreshes(context.Background(), testWorkerLogger())
	assert.Equal(t, 0, countBackgroundWork(t, database, trading212FetchWorkKind), "must not trigger before the interval elapses")

	// Just over 24h later: due.
	svc.SetNowForTest(func() time.Time { return lastFetched.Add(24*time.Hour + time.Minute) })
	svc.runDueTrading212AutoRefreshes(context.Background(), testWorkerLogger())
	assert.Equal(t, 1, countBackgroundWork(t, database, trading212FetchWorkKind), "must trigger once the interval has elapsed")
}

// TestRunDueTrading212AutoRefreshes_InFlightGuardSkipsWithoutError confirms
// a connection whose previous fetch never completed (still "fetching") is
// treated as a normal skip by the scheduler, not logged/counted as a
// failure — the existing StartOnlineImportBatch in-flight guard already
// returns ErrImportFetchInProgress for this, and the scheduler must not
// double-enqueue a second fetch on top of it.
func TestRunDueTrading212AutoRefreshes_InFlightGuardSkipsWithoutError(t *testing.T) {
	svc, connService, _, database := newImportFetchTestService(t)
	conn := createTestTrading212Connection(t, svc, connService, "http://example.invalid")
	enableAutoRefresh(t, connService, conn)

	// Start a fetch and leave it unfinished (never call runDueTrading212Fetches),
	// simulating a still-in-progress fetch — e.g. a deep multi-continuation
	// backfill that hasn't caught up yet.
	_, err := svc.StartOnlineImport(context.Background(), StartOnlineImportInput{OwnerUserID: 1, ConnectionID: conn.ID})
	require.NoError(t, err)
	require.Equal(t, 1, countBackgroundWork(t, database, trading212FetchWorkKind))

	// last_fetched_at is still unset (the in-progress fetch hasn't completed),
	// so the connection reads as "due" — the in-flight guard, not the cadence
	// check, is what must stop a second enqueue here.
	svc.runDueTrading212AutoRefreshes(context.Background(), testWorkerLogger())

	assert.Equal(t, 1, countBackgroundWork(t, database, trading212FetchWorkKind), "the in-flight guard must prevent a second concurrent fetch")
}

// TestListDueAutoRefreshConnectionIDs_ScopedToSourceKind confirms the query
// used by the scheduler only returns rows for the requested source_kind, so
// a future non-trading212 online source with auto-refresh enabled can't be
// swept up by the trading212-only scheduler.
func TestListDueAutoRefreshConnectionIDs_ScopedToSourceKind(t *testing.T) {
	database := openConnectionTestDatabase(t)
	connService := NewImportConnectionService(db.NewImportConnectionRepository(database), nil, testKey(), NoOpProber{})
	conn := createTestTrading212Connection(t, nil, connService, "http://example.invalid")
	enableAutoRefresh(t, connService, conn)

	ids, err := connService.ListDueAutoRefreshConnectionIDs(context.Background(), "trading212", time.Now().UTC())
	require.NoError(t, err)
	assert.Equal(t, []int64{conn.ID}, ids)

	ids, err = connService.ListDueAutoRefreshConnectionIDs(context.Background(), "some_other_source", time.Now().UTC())
	require.NoError(t, err)
	assert.Empty(t, ids)
}
