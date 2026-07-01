package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rekenraam/backend/internal/db"
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
	connService := NewImportConnectionService(db.NewImportConnectionRepository(database), testKey(), NoOpProber{})
	importRepo := db.NewImportRepository(database)
	bgRepo := db.NewBackgroundWorkRepository(database)
	svc := NewImportService(importRepo, nil, nil, connService, bgRepo)
	return svc, connService, importRepo, database
}

func createTestTrading212Connection(t *testing.T, connService *ImportConnectionService, baseURL string) ImportConnection {
	t.Helper()
	conn, err := connService.CreateImportConnection(context.Background(), CreateImportConnectionInput{
		OwnerUserID: 1,
		SourceKind:  "trading212",
		DisplayName: "Test T212",
		APIKey:      "test-api-key",
		ConfigJSON:  fmt.Sprintf(`{"base_url":%q}`, baseURL),
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

func TestStartOnlineImport_WorkerStagesRowsAndUpdatesCursor(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeFakeT212JSON(w, []fakeT212Movement{
			{ID: "ref-1", Type: "DEPOSIT", DateTime: "2024-01-01T00:00:00Z", Amount: "100.00", Currency: "EUR"},
			{ID: "ref-2", Type: "DIVIDEND", DateTime: "2024-01-02T00:00:00Z", Amount: "1.23", Currency: "EUR"},
		})
	}))
	defer server.Close()

	svc, connService, importRepo, _ := newImportFetchTestService(t)
	conn := createTestTrading212Connection(t, connService, server.URL)
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
	assert.Equal(t, "2024-01-02T00:00:00Z", updatedConn.FetchCursor)
	assert.Equal(t, "ready", updatedConn.LastFetchStatus)
}

func TestRefreshImportConnection_IncrementalOnlyNewMovementsAndSkipsCommitted(t *testing.T) {
	movements := []fakeT212Movement{
		{ID: "ref-1", Type: "DEPOSIT", DateTime: "2024-01-01T00:00:00Z", Amount: "100.00", Currency: "EUR"},
		{ID: "ref-2", Type: "DIVIDEND", DateTime: "2024-01-02T00:00:00Z", Amount: "1.23", Currency: "EUR"},
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeFakeT212JSON(w, movements)
	}))
	defer server.Close()

	svc, connService, importRepo, database := newImportFetchTestService(t)
	conn := createTestTrading212Connection(t, connService, server.URL)
	ctx := context.Background()

	// Seed the minimal account + transaction rows import_commit_identities'
	// FKs require, then simulate ref-1 already committed by a prior import.
	_, err := database.ExecContext(ctx, `
		INSERT INTO accounts (id, book_id, created_at, created_by_user_id) VALUES (1, 1, '2024-01-01T00:00:00Z', 1);
		INSERT INTO transactions (id, book_id, created_at, created_by_user_id) VALUES (999, 1, '2024-01-01T00:00:00Z', 1);
	`)
	require.NoError(t, err)

	fp := hashFingerprint(buildTrading212Fingerprint(conn.ID, "ref-1", 0))
	tx, err := importRepo.DB().BeginTx(ctx, nil)
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

	// A third, later movement appears upstream; refresh should fetch only it.
	movements = append(movements, fakeT212Movement{ID: "ref-3", Type: "FEE", DateTime: "2024-01-03T00:00:00Z", Amount: "-0.50", Currency: "EUR"})

	refreshResult, err := svc.RefreshImportConnection(ctx, RefreshImportConnectionInput{OwnerUserID: 1, ConnectionID: conn.ID})
	require.NoError(t, err)
	svc.runDueTrading212Fetches(ctx, testWorkerLogger(), "worker-2")

	refreshRows, err := importRepo.ListAllImportStagedRows(ctx, refreshResult.Batch.ID)
	require.NoError(t, err)
	require.Len(t, refreshRows, 1, "incremental refresh should only pull movements after the saved cursor")
	assert.Contains(t, refreshRows[0].RawJSON, "ref-3")

	updatedConn, err := connService.GetImportConnection(ctx, conn.ID)
	require.NoError(t, err)
	assert.Equal(t, "2024-01-03T00:00:00Z", updatedConn.FetchCursor)
}

func TestStartOnlineImport_GuardsAgainstConcurrentFetch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeFakeT212JSON(w, nil)
	}))
	defer server.Close()

	svc, connService, _, _ := newImportFetchTestService(t)
	conn := createTestTrading212Connection(t, connService, server.URL)
	ctx := context.Background()

	_, err := svc.StartOnlineImport(ctx, StartOnlineImportInput{OwnerUserID: 1, ConnectionID: conn.ID})
	require.NoError(t, err)

	// The worker hasn't drained the first fetch yet, so the batch is still "fetching".
	_, err = svc.StartOnlineImport(ctx, StartOnlineImportInput{OwnerUserID: 1, ConnectionID: conn.ID})
	require.ErrorIs(t, err, ErrImportFetchInProgress)

	_, err = svc.RefreshImportConnection(ctx, RefreshImportConnectionInput{OwnerUserID: 1, ConnectionID: conn.ID})
	require.ErrorIs(t, err, ErrImportFetchInProgress)
}

func TestProcessTrading212FetchWork_UnauthorizedIsTerminal(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	svc, connService, importRepo, database := newImportFetchTestService(t)
	conn := createTestTrading212Connection(t, connService, server.URL)
	ctx := context.Background()

	result, err := svc.StartOnlineImport(ctx, StartOnlineImportInput{OwnerUserID: 1, ConnectionID: conn.ID})
	require.NoError(t, err)

	svc.runDueTrading212Fetches(ctx, testWorkerLogger(), "worker-1")

	batch, err := importRepo.ImportBatchByID(ctx, BookID, result.Batch.ID)
	require.NoError(t, err)
	assert.Equal(t, "failed", batch.Status)

	updatedConn, err := connService.GetImportConnection(ctx, conn.ID)
	require.NoError(t, err)
	assert.Equal(t, "failed", updatedConn.LastFetchStatus)

	status, attempts := lastBackgroundWorkStatus(t, database, trading212FetchWorkKind)
	assert.Equal(t, "failed", status, "an unauthorized key should fail terminally, not retry")
	assert.Equal(t, 1, attempts)
}

func TestProcessTrading212FetchWork_TransientErrorRetriesWithoutFailingBatch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	svc, connService, importRepo, database := newImportFetchTestService(t)
	conn := createTestTrading212Connection(t, connService, server.URL)
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
		writeFakeT212JSON(w, []fakeT212Movement{
			{ID: "ref-1", Type: "DEPOSIT", DateTime: "2024-01-01T00:00:00Z", Amount: "10.00", Currency: "EUR"},
		})
	}))
	defer server.Close()

	svc, connService, importRepo, _ := newImportFetchTestService(t)
	conn := createTestTrading212Connection(t, connService, server.URL)
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
