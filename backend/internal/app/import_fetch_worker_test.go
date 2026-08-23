package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rekenraam/backend/internal/db"
	"rekenraam/backend/internal/onlinesource/trading212"
)

// --- Test fixtures ---

type fakeT212Movement struct {
	ID       string
	Type     string
	DateTime string
	Amount   string
	Currency string
}

func writeFakeT212JSON(w http.ResponseWriter, movements []fakeT212Movement) {
	items := make([]map[string]any, len(movements))
	for i, m := range movements {
		items[i] = map[string]any{
			"id": i + 1, "type": m.Type, "dateTime": m.DateTime,
			"amount": m.Amount, "currency": m.Currency, "reference": m.ID,
		}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"items": items, "nextPagePath": ""})
}

func testWorkerLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func newImportFetchTestService(t *testing.T) (*ImportService, *ImportConnectionService, *db.ImportRepository, *sql.DB) {
	t.Helper()
	database := openConnectionTestDatabase(t)
	connService := NewImportConnectionService(db.NewImportConnectionRepository(database), nil, testKey(), NoOpProber{})
	importRepo := db.NewImportRepository(database)
	bgRepo := db.NewBackgroundWorkRepository(database)
	svc := NewImportService(importRepo, nil, nil, connService, bgRepo, nil)
	return svc, connService, importRepo, database
}

func createTestTrading212Connection(t *testing.T, svc *ImportService, connService *ImportConnectionService, baseURL string) ImportConnection {
	t.Helper()
	if svc != nil {
		svc.trading212BaseURL = baseURL
	}
	conn, err := connService.CreateImportConnection(context.Background(), CreateImportConnectionInput{
		OwnerUserID: 1,
		SourceKind:  "trading212",
		DisplayName: "Test T212",
		APIKey:      "test-api-key",
		ConfigJSON:  "{}",
	})
	require.NoError(t, err)
	return conn
}

func lastBackgroundWorkStatus(t *testing.T, database *sql.DB, kind string) (status string, attempts int) {
	t.Helper()
	err := database.QueryRow(`
		SELECT status, attempts FROM background_work_items WHERE kind = ? ORDER BY id DESC LIMIT 1
	`, kind).Scan(&status, &attempts)
	require.NoError(t, err)
	return status, attempts
}

func sourceMetaOf(t *testing.T, importRepo *db.ImportRepository, batchID int64) trading212BatchMeta {
	t.Helper()
	batch, err := importRepo.ImportBatchByID(context.Background(), BookID, batchID)
	require.NoError(t, err)
	var meta trading212BatchMeta
	require.NoError(t, json.Unmarshal([]byte(batch.SourceMetaJSON), &meta))
	return meta
}

// --- Tests ---

// TestStageParseResult_ReStagingSameBatchFlagsNeedsAttentionNotDuplicateNew
// guards a scenario the pagination-continuation work made real: if
// runTrading212Fetch is retried after stageParseResult already succeeded but
// a later step (updating source_meta or the connection cursor) failed, the
// retry re-fetches and re-stages the same movements into the same batch.
// Before stageParseResult seeded its within-batch dedupe set from rows
// already staged by an earlier call, this produced confusing duplicate
// "new"-looking rows in the preview UI (still ledger-safe at commit time via
// FindCommitIdentity's per-row check, but visually misleading). Confirms the
// second call correctly flags them "needs_attention" instead.
func TestStageParseResult_ReStagingSameBatchFlagsNeedsAttentionNotDuplicateNew(t *testing.T) {
	svc, connService, importRepo, _ := newImportFetchTestService(t)
	conn := createTestTrading212Connection(t, svc, connService, "http://example.invalid")
	ctx := context.Background()

	nowStr := "2026-06-30T00:00:00Z"
	batch, err := importRepo.CreateImportBatch(ctx, db.CreateImportBatchParams{
		BookID: BookID, SourceKind: "trading212",
		ConnectionID:   sql.NullInt64{Int64: conn.ID, Valid: true},
		SourceMetaJSON: `{"fetch_status":"fetching"}`, CreatedAt: nowStr, ActorUserID: 1,
	})
	require.NoError(t, err)

	// A fresh ParseResult per call, matching production: runTrading212Fetch
	// re-parses freshly-fetched JSON each attempt, so DedupeFingerprint is
	// always the raw pre-hash value going in. stageParseResult hashes it in
	// place, so reusing one ParseResult value across two calls (as opposed
	// to two separately-constructed ones) would double-hash the second call
	// and silently miss the very match this test exists to check for.
	newParseResult := func() ParseResult {
		return ParseResult{Rows: []StagedRow{
			{DedupeFingerprint: buildTrading212Fingerprint(conn.ID, "ref-retry", 0), Date: "2024-01-01", Amount: "10.00", CommodityHint: "EUR", ExternalRef: "ref-retry"},
		}}
	}

	// First call: the fetch worker's normal path.
	firstRows, firstBase, err := svc.stageParseResult(ctx, batch.ID, newParseResult())
	require.NoError(t, err)
	require.Len(t, firstRows, 1)
	assert.Equal(t, "new", firstRows[0].DedupeStatus)
	assert.Equal(t, 0, firstBase)

	// Second call on the SAME batch: simulates a retry that re-fetched and
	// re-parsed the same movement (e.g. cursor hadn't advanced yet when the
	// prior attempt failed after staging but before recording progress).
	secondRows, secondBase, err := svc.stageParseResult(ctx, batch.ID, newParseResult())
	require.NoError(t, err)
	require.Len(t, secondRows, 2, "both the original and re-staged row exist — this only affects labeling, not row count")
	assert.Equal(t, 1, secondBase, "row_index continues from where the first call left off, not reset to 0")

	statuses := make([]string, len(secondRows))
	for i, r := range secondRows {
		statuses[i] = r.DedupeStatus
	}
	assert.Contains(t, statuses, "needs_attention", "the re-staged duplicate must be flagged, not silently labeled new")
}

func TestStartOnlineImport_WorkerStagesRowsAndUpdatesCursor(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/equity/history/transactions" {
			writeFakeT212JSON(w, nil)
			return
		}
		writeFakeT212JSON(w, []fakeT212Movement{
			{ID: "ref-1", Type: "DEPOSIT", DateTime: "2024-01-01T00:00:00Z", Amount: "100.00", Currency: "EUR"},
			{ID: "ref-2", Type: "DIVIDEND", DateTime: "2024-01-02T00:00:00Z", Amount: "1.23", Currency: "EUR"},
		})
	}))
	defer server.Close()

	svc, connService, importRepo, _ := newImportFetchTestService(t)
	conn := createTestTrading212Connection(t, svc, connService, server.URL)
	ctx := context.Background()

	result, err := svc.StartOnlineImport(ctx, StartOnlineImportInput{OwnerUserID: 1, ConnectionID: conn.ID})
	require.NoError(t, err)
	assert.Equal(t, "previewing", result.Batch.Status)
	require.NotNil(t, result.Batch.ConnectionID)
	assert.Equal(t, conn.ID, *result.Batch.ConnectionID)

	meta := sourceMetaOf(t, importRepo, result.Batch.ID)
	assert.Equal(t, "fetching", meta.FetchStatus)

	svc.runDueTrading212Fetches(ctx, testWorkerLogger(), "worker-1")

	meta = sourceMetaOf(t, importRepo, result.Batch.ID)
	assert.Equal(t, "ready", meta.FetchStatus)
	assert.Equal(t, []string{"EUR"}, meta.CurrencyHints)

	rows, err := importRepo.ListAllImportStagedRows(ctx, result.Batch.ID)
	require.NoError(t, err)
	require.Len(t, rows, 2)
	for _, row := range rows {
		assert.Equal(t, "new", row.DedupeStatus)
	}

	updatedConn, err := connService.GetImportConnection(ctx, conn.ID)
	require.NoError(t, err)
	assert.Equal(t, "2024-01-02T00:00:00Z", updatedConn.TransactionsCursor)
	assert.Equal(t, "ready", updatedConn.LastFetchStatus)
}

func TestRefreshImportConnection_IncrementalOnlyNewMovementsAndSkipsCommitted(t *testing.T) {
	movements := []fakeT212Movement{
		{ID: "ref-1", Type: "DEPOSIT", DateTime: "2024-01-01T00:00:00Z", Amount: "100.00", Currency: "EUR"},
		{ID: "ref-2", Type: "DIVIDEND", DateTime: "2024-01-02T00:00:00Z", Amount: "1.23", Currency: "EUR"},
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/equity/history/transactions" {
			writeFakeT212JSON(w, nil)
			return
		}
		writeFakeT212JSON(w, movements)
	}))
	defer server.Close()

	svc, connService, importRepo, database := newImportFetchTestService(t)
	conn := createTestTrading212Connection(t, svc, connService, server.URL)
	ctx := context.Background()

	// Seed the minimal account + transaction rows import_commit_identities'
	// FKs require, then simulate ref-1 already committed by a prior import.
	_, err := database.ExecContext(ctx, `
		INSERT INTO accounts (id, book_id, created_at, created_by_user_id) VALUES (1, 1, '2024-01-01T00:00:00Z', 1);
		INSERT INTO transactions (id, book_id, created_at, created_by_user_id) VALUES (999, 1, '2024-01-01T00:00:00Z', 1);
	`)
	require.NoError(t, err)

	fp := hashFingerprint(buildTrading212Fingerprint(conn.ID, "ref-1", 0))
	tx, err := database.BeginTx(ctx, nil)
	require.NoError(t, err)
	require.NoError(t, importRepo.CreateCommitIdentity(ctx, tx, db.CreateImportCommitIdentityParams{
		BookID: BookID, DedupeFingerprint: fp, CommittedTransactionID: 999,
		SourceKind: "trading212", AccountID: 1, CreatedAt: "2024-01-01T00:00:00Z",
	}))
	require.NoError(t, tx.Commit())

	result, err := svc.StartOnlineImport(ctx, StartOnlineImportInput{OwnerUserID: 1, ConnectionID: conn.ID})
	require.NoError(t, err)
	svc.runDueTrading212Fetches(ctx, testWorkerLogger(), "worker-1")

	rows, err := importRepo.ListAllImportStagedRows(ctx, result.Batch.ID)
	require.NoError(t, err)
	require.Len(t, rows, 2)
	byFingerprint := map[string]string{}
	for _, row := range rows {
		byFingerprint[row.DedupeFingerprint] = row.DedupeStatus
	}
	assert.Equal(t, "duplicate", byFingerprint[fp], "already-committed movement should be flagged duplicate, not re-imported")

	// A third, later movement appears upstream; refresh should fetch it.
	movements = append(movements, fakeT212Movement{ID: "ref-3", Type: "FEE", DateTime: "2024-01-03T00:00:00Z", Amount: "-0.50", Currency: "EUR"})

	refreshResult, err := svc.RefreshImportConnection(ctx, RefreshImportConnectionInput{OwnerUserID: 1, ConnectionID: conn.ID})
	require.NoError(t, err)
	svc.runDueTrading212Fetches(ctx, testWorkerLogger(), "worker-2")

	refreshRows, err := importRepo.ListAllImportStagedRows(ctx, refreshResult.Batch.ID)
	require.NoError(t, err)
	// The saved cursor is ref-2's exact timestamp (it was the newest
	// movement seen in the first fetch). The fetcher deliberately re-scans
	// movements at the cursor's exact timestamp rather than dropping them
	// (see trading212.Fetcher's "<" cursor comparison — a same-timestamp
	// movement split across two fetches must not be silently lost), so ref-2
	// legitimately reappears here alongside the genuinely new ref-3. Neither
	// is a duplicate transaction/dedupe_status "duplicate": ref-2 was never
	// committed (only ref-1 was, in the setup above), so re-seeing it in a
	// fresh batch is correct, not an over-import.
	require.Len(t, refreshRows, 2, "incremental refresh should pull movements at-or-after the saved cursor")
	rawByID := map[string]string{}
	for _, row := range refreshRows {
		rawByID[row.RawJSON] = row.DedupeStatus
	}
	var sawRef2, sawRef3 bool
	for raw, status := range rawByID {
		if strings.Contains(raw, "ref-2") {
			sawRef2 = true
			assert.Equal(t, "new", status, "ref-2 was never committed, so re-scanning it is not a duplicate")
		}
		if strings.Contains(raw, "ref-3") {
			sawRef3 = true
			assert.Equal(t, "new", status)
		}
	}
	assert.True(t, sawRef2, "expected the same-timestamp-as-cursor movement to be re-scanned")
	assert.True(t, sawRef3, "expected the genuinely new movement to be fetched")

	updatedConn, err := connService.GetImportConnection(ctx, conn.ID)
	require.NoError(t, err)
	assert.Equal(t, "2024-01-03T00:00:00Z", updatedConn.TransactionsCursor)
}

func TestStartOnlineImport_GuardsAgainstConcurrentFetch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/equity/history/transactions" {
			writeFakeT212JSON(w, nil)
			return
		}
		writeFakeT212JSON(w, nil)
	}))
	defer server.Close()

	svc, connService, _, _ := newImportFetchTestService(t)
	conn := createTestTrading212Connection(t, svc, connService, server.URL)
	ctx := context.Background()

	_, err := svc.StartOnlineImport(ctx, StartOnlineImportInput{OwnerUserID: 1, ConnectionID: conn.ID})
	require.NoError(t, err)

	// The worker hasn't drained the first fetch yet, so the batch is still "fetching".
	_, err = svc.StartOnlineImport(ctx, StartOnlineImportInput{OwnerUserID: 1, ConnectionID: conn.ID})
	require.ErrorIs(t, err, ErrImportFetchInProgress)

	_, err = svc.RefreshImportConnection(ctx, RefreshImportConnectionInput{OwnerUserID: 1, ConnectionID: conn.ID})
	require.ErrorIs(t, err, ErrImportFetchInProgress)
}

// TestStartOnlineImport_ConcurrentStartsRaceSafely fires many concurrent
// StartOnlineImport calls for the same connection and asserts exactly one
// wins. Before StartOnlineImportBatch made the guard-check + batch-insert +
// enqueue atomic, the guard's read and the batch's write were separate
// statements, so two goroutines could both observe "no in-flight fetch"
// before either had inserted its batch — this test would have been flaky
// (occasionally >1 success) against that version.
func TestStartOnlineImport_ConcurrentStartsRaceSafely(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/equity/history/transactions" {
			writeFakeT212JSON(w, nil)
			return
		}
		writeFakeT212JSON(w, nil)
	}))
	defer server.Close()

	svc, connService, importRepo, _ := newImportFetchTestService(t)
	conn := createTestTrading212Connection(t, svc, connService, server.URL)
	ctx := context.Background()

	const attempts = 12
	var wg sync.WaitGroup
	var successes int64
	var conflicts int64
	var unexpected int64
	for range attempts {
		wg.Go(func() {
			_, err := svc.StartOnlineImport(ctx, StartOnlineImportInput{OwnerUserID: 1, ConnectionID: conn.ID})
			switch {
			case err == nil:
				atomic.AddInt64(&successes, 1)
			case errors.Is(err, ErrImportFetchInProgress):
				atomic.AddInt64(&conflicts, 1)
			default:
				atomic.AddInt64(&unexpected, 1)
				t.Errorf("unexpected error from concurrent StartOnlineImport: %v", err)
			}
		})
	}
	wg.Wait()

	assert.Equal(t, int64(0), unexpected)
	assert.Equal(t, int64(1), successes, "exactly one concurrent start should win the in-flight guard")
	assert.Equal(t, int64(attempts-1), conflicts)

	// Only one previewing/fetching batch should exist for this connection —
	// confirms the race didn't leave a second, orphaned batch behind either.
	batches, err := importRepo.ListImportBatches(ctx, db.ListImportBatchesParams{BookID: BookID, Limit: 100})
	require.NoError(t, err)
	require.Len(t, batches, 1)
	assert.Equal(t, conn.ID, batches[0].ConnectionID.Int64)
}

// TestStartOnlineImportBatch_PayloadFailureLeavesNoStrandedBatch exercises
// the repository method directly: if building the work-item payload fails
// after the batch row would otherwise have been inserted, the whole
// transaction must roll back — no batch, no orphaned "fetching" state that
// would permanently trip the in-flight guard with nothing to ever process it.
func TestStartOnlineImportBatch_PayloadFailureLeavesNoStrandedBatch(t *testing.T) {
	svc, connService, importRepo, database := newImportFetchTestService(t)
	conn := createTestTrading212Connection(t, svc, connService, "http://example.invalid")
	ctx := context.Background()

	boom := errors.New("boom: payload build failed")
	_, err := importRepo.StartOnlineImportBatch(ctx, db.StartOnlineImportBatchParams{
		BookID:         BookID,
		ConnectionID:   conn.ID,
		SourceKind:     "trading212",
		SourceMetaJSON: `{"fetch_status":"fetching"}`,
		CreatedAt:      "2026-06-30T00:00:00Z",
		ActorUserID:    1,
		WorkKind:       trading212FetchWorkKind,
	}, func(batchID int64) (string, error) {
		return "", boom
	})
	require.ErrorIs(t, err, boom)

	var batchCount, workCount int
	require.NoError(t, database.QueryRowContext(ctx, `SELECT COUNT(*) FROM import_batches`).Scan(&batchCount))
	require.NoError(t, database.QueryRowContext(ctx, `SELECT COUNT(*) FROM background_work_items`).Scan(&workCount))
	assert.Equal(t, 0, batchCount, "the batch insert must roll back when the payload build fails")
	assert.Equal(t, 0, workCount)

	// The guard must not think a fetch is in progress after the rollback.
	_, startErr := svc.StartOnlineImport(ctx, StartOnlineImportInput{OwnerUserID: 1, ConnectionID: conn.ID})
	require.NoError(t, startErr)
}

func TestProcessTrading212FetchWork_UnauthorizedIsTerminal(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/equity/history/transactions" {
			writeFakeT212JSON(w, nil)
			return
		}
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	svc, connService, importRepo, database := newImportFetchTestService(t)
	conn := createTestTrading212Connection(t, svc, connService, server.URL)
	ctx := context.Background()

	result, err := svc.StartOnlineImport(ctx, StartOnlineImportInput{OwnerUserID: 1, ConnectionID: conn.ID})
	require.NoError(t, err)

	svc.runDueTrading212Fetches(ctx, testWorkerLogger(), "worker-1")

	batch, err := importRepo.ImportBatchByID(ctx, BookID, result.Batch.ID)
	require.NoError(t, err)
	assert.Equal(t, "failed", batch.Status)

	// The frontend polls source_meta.fetch_status, not batch.status — both
	// must flip to "failed" or the UI spins forever on stale "fetching" meta.
	meta := sourceMetaOf(t, importRepo, result.Batch.ID)
	assert.Equal(t, "failed", meta.FetchStatus)
	assert.NotEmpty(t, meta.Error)

	updatedConn, err := connService.GetImportConnection(ctx, conn.ID)
	require.NoError(t, err)
	assert.Equal(t, "failed", updatedConn.LastFetchStatus)

	status, attempts := lastBackgroundWorkStatus(t, database, trading212FetchWorkKind)
	assert.Equal(t, "failed", status, "an unauthorized key should fail terminally, not retry")
	assert.Equal(t, 1, attempts)
}

func TestProcessTrading212FetchWork_TransientErrorRetriesWithoutFailingBatch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/equity/history/transactions" {
			writeFakeT212JSON(w, nil)
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	svc, connService, importRepo, database := newImportFetchTestService(t)
	conn := createTestTrading212Connection(t, svc, connService, server.URL)
	ctx := context.Background()

	result, err := svc.StartOnlineImport(ctx, StartOnlineImportInput{OwnerUserID: 1, ConnectionID: conn.ID})
	require.NoError(t, err)

	svc.runDueTrading212Fetches(ctx, testWorkerLogger(), "worker-1")

	batch, err := importRepo.ImportBatchByID(ctx, BookID, result.Batch.ID)
	require.NoError(t, err)
	assert.Equal(t, "previewing", batch.Status, "a transient provider error must not wedge or fail the batch")

	meta := sourceMetaOf(t, importRepo, result.Batch.ID)
	assert.Equal(t, "fetching", meta.FetchStatus)

	status, attempts := lastBackgroundWorkStatus(t, database, trading212FetchWorkKind)
	assert.Equal(t, "pending", status, "should be rescheduled for retry, not marked failed")
	assert.Equal(t, 1, attempts)
}

func TestImportFetchWork_RestartReclaimsExpiredLease(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/equity/history/transactions" {
			writeFakeT212JSON(w, nil)
			return
		}
		writeFakeT212JSON(w, []fakeT212Movement{
			{ID: "ref-1", Type: "DEPOSIT", DateTime: "2024-01-01T00:00:00Z", Amount: "10.00", Currency: "EUR"},
		})
	}))
	defer server.Close()

	svc, connService, importRepo, _ := newImportFetchTestService(t)
	conn := createTestTrading212Connection(t, svc, connService, server.URL)
	ctx := context.Background()

	base := time.Date(2026, 6, 30, 12, 0, 0, 0, time.UTC)
	current := base
	svc.SetNowForTest(func() time.Time { return current })

	result, err := svc.StartOnlineImport(ctx, StartOnlineImportInput{OwnerUserID: 1, ConnectionID: conn.ID})
	require.NoError(t, err)

	// Simulate a worker that claimed the work item and then crashed before
	// doing anything else (e.g. the process was killed mid-fetch).
	claimed, err := svc.backgroundWork.ClaimBackgroundWork(ctx, trading212FetchWorkKind, "crashed-worker",
		current.Format(time.RFC3339), current.Add(15*time.Minute).Format(time.RFC3339))
	require.NoError(t, err)
	assert.Equal(t, 1, claimed.Attempts)

	// The batch is still "fetching" — nothing has run yet.
	meta := sourceMetaOf(t, importRepo, result.Batch.ID)
	assert.Equal(t, "fetching", meta.FetchStatus)

	// Time passes beyond the lease; a restarted process (new worker id) reclaims it.
	current = current.Add(20 * time.Minute)
	svc.runDueTrading212Fetches(ctx, testWorkerLogger(), "restarted-worker")

	meta = sourceMetaOf(t, importRepo, result.Batch.ID)
	assert.Equal(t, "ready", meta.FetchStatus)

	rows, err := importRepo.ListAllImportStagedRows(ctx, result.Batch.ID)
	require.NoError(t, err)
	assert.Len(t, rows, 1, "the reclaimed fetch should complete with no duplicate staged rows")
}

// TestStartOnlineImport_ContinuesPastPageBudgetWithoutTruncatingOrDuplicating
// simulates a Trading 212 account with more history than a single Fetch call
// will page through (trading212.maxPages), verifying the worker's
// continuation mechanism recovers the full history across multiple chunks
// instead of silently truncating it (the gap fixed alongside this test).
func TestStartOnlineImport_ContinuesPastPageBudgetWithoutTruncatingOrDuplicating(t *testing.T) {
	restore := trading212.SetMaxPagesForTest(3)
	defer restore()

	// 7 movements, newest-first (page 0 = newest, matching the documented
	// provider ordering the incremental-stop logic assumes): dates
	// 2024-01-07 down to 2024-01-01.
	const totalMovements = 7
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/equity/history/transactions" {
			writeFakeT212JSON(w, nil)
			return
		}
		p := r.URL.Query().Get("p")
		page := 0
		if p != "" {
			page, _ = strconv.Atoi(p)
		}
		day := totalMovements - page
		next := ""
		if page+1 < totalMovements {
			next = fmt.Sprintf("/equity/history/transactions?p=%d", page+1)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"items": []map[string]any{
				{"id": page, "type": "DEPOSIT", "dateTime": fmt.Sprintf("2024-01-%02dT00:00:00Z", day), "amount": "10.00", "currency": "EUR", "reference": fmt.Sprintf("ref-%d", page)},
			},
			"nextPagePath": next,
		})
	}))
	defer server.Close()

	svc, connService, importRepo, _ := newImportFetchTestService(t)
	conn := createTestTrading212Connection(t, svc, connService, server.URL)
	ctx := context.Background()

	result, err := svc.StartOnlineImport(ctx, StartOnlineImportInput{OwnerUserID: 1, ConnectionID: conn.ID})
	require.NoError(t, err)

	// Each chunk's completion immediately enqueues (and makes due) the next
	// continuation/stage, and runDueTrading212Fetches claims up to 4 items
	// per call. This chain needs 5 work items: 3 transactions chunks (7
	// movements / maxPages=3 per chunk) plus one orders-stage and one
	// dividends-stage item (both empty against this fake server, but still
	// real hops in the stage machinery) — two ticks comfortably cover it.
	svc.runDueTrading212Fetches(ctx, testWorkerLogger(), "worker-1")
	svc.runDueTrading212Fetches(ctx, testWorkerLogger(), "worker-1")

	meta := sourceMetaOf(t, importRepo, result.Batch.ID)
	assert.Equal(t, "ready", meta.FetchStatus, "the batch should reach ready once the last chunk naturally exhausts the provider")

	rows, err := importRepo.ListAllImportStagedRows(ctx, result.Batch.ID)
	require.NoError(t, err)
	assert.Len(t, rows, totalMovements, "every movement across all chunks should be staged exactly once, none lost to the page cap")

	seenFingerprints := map[string]bool{}
	for _, row := range rows {
		assert.False(t, seenFingerprints[row.DedupeFingerprint], "no duplicate fingerprint across chunks: %s", row.DedupeFingerprint)
		seenFingerprints[row.DedupeFingerprint] = true
		assert.Equal(t, "new", row.DedupeStatus)
	}

	// row_index must be unique and monotonic across chunks (each
	// stageParseResult call continues from where the previous one left off).
	rowIndexes := map[int]bool{}
	for _, row := range rows {
		assert.False(t, rowIndexes[row.RowIndex], "duplicate row_index %d across chunks", row.RowIndex)
		rowIndexes[row.RowIndex] = true
	}

	updatedConn, err := connService.GetImportConnection(ctx, conn.ID)
	require.NoError(t, err)
	assert.Equal(t, "2024-01-07T00:00:00Z", updatedConn.TransactionsCursor, "the persisted cursor must be the true overall newest timestamp, not just the last chunk's")
	assert.Equal(t, "ready", updatedConn.LastFetchStatus)
}

func TestBuildTransactionSpec_OnlineRowResolvesCurrencyAndIsNeedsReview(t *testing.T) {
	row := db.ImportStagedRowRecord{
		NormalizedJSON: `{
			"date": "2024-01-02",
			"amount": "12.34",
			"commodity_hint": "EUR",
			"payee_hint": "Trading 212",
			"memo": "DIVIDEND",
			"external_ref": "ref-2"
		}`,
	}
	resolution := ImportRowResolution{
		AccountID:   3,
		CommodityID: 7,
		CategoryID:  ptrInt64(9),
	}

	spec, err := buildTransactionSpec(row, resolution)
	require.NoError(t, err)

	assert.Equal(t, "posted", spec.Status)
	assert.True(t, spec.NeedsReview, "imported rows must land needs_review, never auto-approved")
	assert.Equal(t, "2024-01-02", spec.TransactionDate)
	require.Len(t, spec.JournalEntries, 1)
	postings := spec.JournalEntries[0].Postings
	require.Len(t, postings, 2, "a balanced import posting has exactly two legs")

	account := postings[0]
	assert.Equal(t, int64(3), account.AccountID)
	assert.Equal(t, int64(7), account.CommodityID)
	assert.Equal(t, "1234", string(account.QuantityValue))
	assert.Equal(t, 2, account.QuantityScale)

	offset := postings[1]
	assert.Equal(t, int64(9), offset.AccountID)
	assert.Equal(t, int64(7), offset.CommodityID)
	assert.Equal(t, "-1234", string(offset.QuantityValue))
	assert.Equal(t, 2, offset.QuantityScale)
}
