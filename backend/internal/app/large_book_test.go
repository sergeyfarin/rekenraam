package app

import (
	"archive/zip"
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"runtime/metrics"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rekenraam/backend/internal/db"
	"rekenraam/backend/internal/testdb"
)

// A realistic upper bound for one person's book (#108): a quarter of a
// million postings across a few thousand accounts, in two currencies, spread
// over five years. Everything is generated in SQL so seeding costs seconds,
// not minutes.
const (
	largeBookTransactions  = 125_000
	largeBookPostings      = 2 * largeBookTransactions
	largeBookGroupAccounts = 20   // non-posting parents, 10 asset + 10 expense
	largeBookLeafAccounts  = 1500 // per side, so 3020 accounts in all
)

// Sharp-regression ceilings, not targets. Measured on 2026-10-16 (see
// docs/developer-workflow.md → Large-book measurement); each ceiling sits
// several times above what was measured so a slow runner passes and an
// accidental whole-book materialisation does not.
const (
	largeBookExportWallCeiling        = 60 * time.Second
	largeBookExportPeakHeapCeiling    = 256 << 20
	largeBookSelfCheckWallCeiling     = 60 * time.Second
	largeBookSelfCheckPeakHeapCeiling = 256 << 20
)

// seedLargeBook writes the generated book into a fresh migrated database and
// returns its writer and URL.
func seedLargeBook(tb testing.TB) (*sql.DB, string) {
	tb.Helper()

	ctx := context.Background()
	writer, databaseURL := testdb.Open(tb)

	tx, err := writer.BeginTx(ctx, nil)
	require.NoError(tb, err)
	defer func() { _ = tx.Rollback() }()

	statements := []string{
		`INSERT INTO users (id, username, password_hash, is_owner, created_at, updated_at)
		 VALUES (1, 'owner', 'x', 1, '2020-01-01T00:00:00Z', '2020-01-01T00:00:00Z')`,
		`INSERT INTO books (id, code, name, owner_user_id, created_at, updated_at, updated_by_user_id)
		 VALUES (1, 'personal', 'Personal', 1, '2020-01-01T00:00:00Z', '2020-01-01T00:00:00Z', 1)`,
		`INSERT INTO commodities (id, book_id, code, kind, is_builtin, created_at, created_by_user_id)
		 VALUES (1, 1, 'USD', 'currency', 1, '2020-01-01T00:00:00Z', 1),
		        (2, 1, 'EUR', 'currency', 1, '2020-01-01T00:00:00Z', 1)`,
		`INSERT INTO commodity_versions (
			commodity_id, version_seq, effective_from, recorded_at, changed_by_user_id, change_reason,
			status, symbol, display_symbol, name, standard_scale, max_quantity_scale
		) VALUES (1, 1, '2020-01-01', '2020-01-01T00:00:00Z', 1, 'seed', 'active', '$', '$', 'US Dollar', 2, 2),
		         (2, 1, '2020-01-01', '2020-01-01T00:00:00Z', 1, 'seed', 'active', '€', '€', 'Euro', 2, 2)`,
		`WITH RECURSIVE seq(n) AS (SELECT 1 UNION ALL SELECT n + 1 FROM seq WHERE n < ?)
		 INSERT INTO accounts (id, book_id, created_at, created_by_user_id)
		 SELECT n, 1, '2020-01-01T00:00:00Z', 1 FROM seq`,
		// 1–10 asset groups, 11–20 expense groups; leaves hang under them in
		// turn so account paths have a real parent to resolve.
		`WITH RECURSIVE seq(n) AS (SELECT 1 UNION ALL SELECT n + 1 FROM seq WHERE n < ?)
		 INSERT INTO account_versions (
			account_id, version_seq, effective_from, recorded_at, changed_by_user_id, change_reason,
			status, opened_on, name, account_class, account_kind, parent_account_id, allows_postings
		 )
		 SELECT n, 1, '2020-01-01', '2020-01-01T00:00:00Z', 1, 'seed', 'active', '2020-01-01',
			CASE WHEN n <= 20 THEN 'Group ' ELSE 'Account ' END || n,
			CASE WHEN n <= 10 OR (n > 20 AND n <= 20 + ?) THEN 'asset' ELSE 'expense' END,
			CASE WHEN n <= 10 OR (n > 20 AND n <= 20 + ?) THEN 'checking' ELSE 'expense' END,
			CASE WHEN n <= 20 THEN NULL
			     WHEN n <= 20 + ? THEN 1 + (n % 10)
			     ELSE 11 + (n % 10) END,
			CASE WHEN n <= 20 THEN 0 ELSE 1 END
		 FROM seq`,
		`WITH RECURSIVE seq(n) AS (SELECT 1 UNION ALL SELECT n + 1 FROM seq WHERE n < ?)
		 INSERT INTO transactions (id, book_id, created_at, created_by_user_id)
		 SELECT n, 1, '2020-01-01T00:00:00Z', 1 FROM seq`,
		`WITH RECURSIVE seq(n) AS (SELECT 1 UNION ALL SELECT n + 1 FROM seq WHERE n < ?)
		 INSERT INTO transaction_versions (
			id, book_id, transaction_id, version_seq, status, transaction_kind, transaction_date,
			recorded_at, changed_by_user_id, change_reason
		 )
		 SELECT n, 1, n, 1, 'posted', 'ordinary', date('2021-01-01', '+' || (n % 1826) || ' days'),
			'2026-01-01T00:00:00Z', 1, 'seed'
		 FROM seq`,
		`INSERT INTO journal_entries (id, book_id, transaction_version_id, entry_seq, entry_date, entry_kind)
		 SELECT id, 1, id, 1, transaction_date, 'ordinary' FROM transaction_versions`,
		`INSERT INTO posting_lines (id, book_id, transaction_id, line_key, created_at, created_by_user_id)
		 SELECT 2 * id - 1, 1, id, 'a', '2020-01-01T00:00:00Z', 1 FROM transactions
		 UNION ALL
		 SELECT 2 * id, 1, id, 'b', '2020-01-01T00:00:00Z', 1 FROM transactions`,
		// One in five transactions is in EUR, chosen by a stride independent
		// of the leaf cycle so every leaf holds both currencies. Amounts vary
		// so per-account totals are not trivially round.
		`INSERT INTO posting_versions (
			book_id, transaction_version_id, journal_entry_id, posting_line_id, line_seq,
			account_id, quantity_value, quantity_scale, commodity_id, reconciliation_status
		 )
		 SELECT 1, id, id, 2 * id - 1, 1, 21 + (id % ?),
			'-' || (1 + (id * 7919) % 99999), 2, 1 + ((id / ?) % 5 = 0), 'uncleared'
		 FROM transaction_versions
		 UNION ALL
		 SELECT 1, id, id, 2 * id, 2, 21 + ? + ((id * 31) % ?),
			CAST(1 + (id * 7919) % 99999 AS TEXT), 2, 1 + ((id / ?) % 5 = 0), 'uncleared'
		 FROM transaction_versions`,
	}
	args := [][]any{
		nil, nil, nil, nil,
		{largeBookGroupAccounts + 2*largeBookLeafAccounts},
		{largeBookGroupAccounts + 2*largeBookLeafAccounts, largeBookLeafAccounts, largeBookLeafAccounts, largeBookLeafAccounts},
		{largeBookTransactions},
		{largeBookTransactions},
		nil, nil,
		{largeBookLeafAccounts, largeBookLeafAccounts, largeBookLeafAccounts, largeBookLeafAccounts, largeBookLeafAccounts},
	}
	for i, statement := range statements {
		_, err := tx.ExecContext(ctx, statement, args[i]...)
		require.NoError(tb, err, "large-book seed statement %d", i)
	}
	require.NoError(tb, tx.Commit())

	var postings int
	require.NoError(tb, writer.QueryRowContext(ctx, `SELECT count(*) FROM posting_versions`).Scan(&postings))
	require.Equal(tb, largeBookPostings, postings)
	return writer, databaseURL
}

// resourceProbe records wall time, cumulative allocation, and the peak of live
// heap objects above the starting level while one operation runs. The peak is
// sampled, so it can only under-read; ceilings are set with that in mind.
type resourceProbe struct {
	started    time.Time
	startAlloc uint64
	startHeap  uint64
	peakHeap   uint64
	stop       chan struct{}
	done       sync.WaitGroup
}

type resourceUsage struct {
	Wall      time.Duration
	Allocated uint64
	PeakHeap  uint64
}

func liveHeapBytes() uint64 {
	sample := []metrics.Sample{{Name: "/memory/classes/heap/objects:bytes"}}
	metrics.Read(sample)
	return sample[0].Value.Uint64()
}

func startResourceProbe() *resourceProbe {
	runtime.GC()
	var stats runtime.MemStats
	runtime.ReadMemStats(&stats)
	probe := &resourceProbe{
		started:    time.Now(),
		startAlloc: stats.TotalAlloc,
		startHeap:  liveHeapBytes(),
		stop:       make(chan struct{}),
	}
	probe.peakHeap = probe.startHeap
	probe.done.Add(1)
	go func() {
		defer probe.done.Done()
		ticker := time.NewTicker(2 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-probe.stop:
				return
			case <-ticker.C:
				probe.peakHeap = max(probe.peakHeap, liveHeapBytes())
			}
		}
	}()
	return probe
}

func (p *resourceProbe) finish() resourceUsage {
	wall := time.Since(p.started)
	close(p.stop)
	p.done.Wait()
	p.peakHeap = max(p.peakHeap, liveHeapBytes())
	var stats runtime.MemStats
	runtime.ReadMemStats(&stats)
	return resourceUsage{
		Wall:      wall,
		Allocated: stats.TotalAlloc - p.startAlloc,
		PeakHeap:  p.peakHeap - min(p.startHeap, p.peakHeap),
	}
}

func largeBookEnabled(tb testing.TB) {
	tb.Helper()
	if os.Getenv("REKENRAAM_LARGE_BOOK") != "1" {
		tb.Skip("large-book measurement runs only with REKENRAAM_LARGE_BOOK=1 (scripts/measure-large-book.sh)")
	}
}

func exportLargeBook(tb testing.TB, writer *sql.DB, path string) {
	tb.Helper()
	file, err := os.Create(path)
	require.NoError(tb, err)
	defer func() { require.NoError(tb, file.Close()) }()
	require.NoError(tb, NewExportService(db.NewExportRepository(writer)).WriteBundle(context.Background(), file, ExportFilter{}))
}

func largeBookSelfCheckService(tb testing.TB, writer *sql.DB, databaseURL string) *SelfCheckService {
	tb.Helper()
	readOnly, err := db.OpenReadOnly(context.Background(), databaseURL)
	require.NoError(tb, err)
	tb.Cleanup(func() { require.NoError(tb, readOnly.Close()) })
	return NewSelfCheckService(db.NewSelfCheckRepository(writer, readOnly))
}

// The bundle export and the self-check stay inside a bounded envelope on a
// realistically large book, and still produce the right answer there: every
// posting exported, the trial balance per commodity, and a passing check.
func TestLargeBookExportAndSelfCheckStayBounded(t *testing.T) {
	t.Parallel()
	largeBookEnabled(t)

	writer, databaseURL := seedLargeBook(t)

	bundlePath := filepath.Join(t.TempDir(), "bundle.zip")
	probe := startResourceProbe()
	exportLargeBook(t, writer, bundlePath)
	exportUsage := probe.finish()
	t.Logf("bundle export: wall=%s allocated=%dMiB peak-heap=%dMiB", exportUsage.Wall.Round(time.Millisecond), exportUsage.Allocated>>20, exportUsage.PeakHeap>>20)

	archive, err := zip.OpenReader(bundlePath)
	require.NoError(t, err)
	defer func() { require.NoError(t, archive.Close()) }()
	var manifest bundleManifest
	for _, entry := range archive.File {
		if entry.Name != "manifest.json" {
			continue
		}
		reader, err := entry.Open()
		require.NoError(t, err)
		raw, err := io.ReadAll(reader)
		require.NoError(t, err)
		require.NoError(t, reader.Close())
		require.NoError(t, json.Unmarshal(raw, &manifest))
	}
	rows := map[string]int64{}
	for _, file := range manifest.Files {
		rows[file.Name] = file.Rows
	}
	assert.EqualValues(t, largeBookPostings, rows["ledger.csv"])
	assert.EqualValues(t, largeBookGroupAccounts+2*largeBookLeafAccounts, rows["accounts.csv"])
	// Every leaf account posts in both currencies, so the trial balance has
	// one row per leaf and commodity.
	assert.EqualValues(t, 2*2*largeBookLeafAccounts, rows["trial-balance.csv"])

	service := largeBookSelfCheckService(t, writer, databaseURL)
	probe = startResourceProbe()
	run, err := service.RunSelfCheck(context.Background(), "manual")
	checkUsage := probe.finish()
	require.NoError(t, err)
	t.Logf("self-check: wall=%s allocated=%dMiB peak-heap=%dMiB", checkUsage.Wall.Round(time.Millisecond), checkUsage.Allocated>>20, checkUsage.PeakHeap>>20)
	assert.Equal(t, SelfCheckPassed, run.Status, "%+v", run.Results)

	assert.Less(t, exportUsage.Wall, largeBookExportWallCeiling, "bundle export wall time")
	assert.Less(t, exportUsage.PeakHeap, uint64(largeBookExportPeakHeapCeiling), "bundle export peak heap")
	assert.Less(t, checkUsage.Wall, largeBookSelfCheckWallCeiling, "self-check wall time")
	assert.Less(t, checkUsage.PeakHeap, uint64(largeBookSelfCheckPeakHeapCeiling), "self-check peak heap")

	// The shell polls this whole-book read. Currency discovery streams exact
	// posted balances (#464), without retaining the posting history or adding
	// per-row queries as the book grows.
	_, err = writer.Exec(`UPDATE books SET default_currency_commodity_id=1 WHERE id=1`)
	require.NoError(t, err)
	settings := NewSettingsService(db.NewSettingsRepository(writer))
	pricing := NewPricingService(db.NewPricingRepository(writer))
	pricing.SetSettings(settings)
	attention := NewAttentionService(db.NewAttentionRepository(writer), pricing)
	probe = startResourceProbe()
	counts := attention.Read(context.Background())
	attentionUsage := probe.finish()
	t.Logf("attention: wall=%s allocated=%dMiB peak-heap=%dMiB", attentionUsage.Wall.Round(time.Millisecond), attentionUsage.Allocated>>20, attentionUsage.PeakHeap>>20)
	assert.True(t, counts.Complete, "%+v", counts.Sources)
	assert.False(t, counts.HasFailures)
	assert.Equal(t, 1, counts.TotalKnown, "the non-default EUR exposure needs EUR/USD even without account defaults or a series")
	for _, source := range counts.Sources {
		if source.ID == "drafts" || source.ID == "transaction_review" {
			require.NotNil(t, source.Count)
			assert.Zero(t, *source.Count)
		}
		if source.ID == "rates" {
			require.NotNil(t, source.Count)
			assert.Equal(t, 1, *source.Count)
		}
	}
	assert.Less(t, attentionUsage.Wall, 5*time.Second, "attention wall time")
	assert.Less(t, attentionUsage.PeakHeap, uint64(64<<20), "attention peak heap")

	// Home loads with the shell. Both headlines come from one ledger read and
	// one fold, not one per headline; each default widget is one canonical
	// read (#478), never one per account, category or schedule.
	transactions := NewTransactionService(db.NewTransactionRepository(writer), db.NewPayeeRepository(writer), db.NewAccountRepository(writer), db.NewCommodityRepository(writer))
	transactions.SetPricingRepository(db.NewPricingRepository(writer))
	homeService := NewHomeService(transactions, NewBudgetService(db.NewBudgetRepository(writer), settings),
		NewRecurringService(db.NewRecurringRepository(writer), transactions, settings))
	probe = startResourceProbe()
	composed, err := homeService.Home(context.Background(), 1)
	homeUsage := probe.finish()
	require.NoError(t, err)
	t.Logf("home: wall=%s allocated=%dMiB peak-heap=%dMiB", homeUsage.Wall.Round(time.Millisecond), homeUsage.Allocated>>20, homeUsage.PeakHeap>>20)
	for name, widgetErr := range map[string]error{"spending": composed.Spending.Err, "budgets": composed.Budgets.Err, "accounts": composed.Accounts.Err,
		"recurring": composed.Recurring.Err, "net worth trend": composed.NetWorthTrend.Err} {
		assert.NoError(t, widgetErr, name)
	}
	home := composed.Briefing
	require.Len(t, home.Headlines, 2)
	for _, headline := range home.Headlines {
		assert.Equal(t, HomeUnavailableValuationGaps, headline.Unavailable, "%s: the EUR balances have no USD rate", headline.ID)
		assert.Equal(t, []HomeValuationInput{{CommodityID: 2, Code: "EUR", Kind: "rate"}}, headline.Missing, headline.ID)
	}
	assert.Less(t, homeUsage.Wall, 10*time.Second, "home wall time")
	assert.Less(t, homeUsage.PeakHeap, uint64(256<<20), "home peak heap")
}

func BenchmarkLargeBookBundleExport(b *testing.B) {
	largeBookEnabled(b)
	writer, _ := seedLargeBook(b)
	path := filepath.Join(b.TempDir(), "bundle.zip")
	b.ReportAllocs()
	for b.Loop() {
		exportLargeBook(b, writer, path)
	}
}

func BenchmarkLargeBookSelfCheck(b *testing.B) {
	largeBookEnabled(b)
	writer, databaseURL := seedLargeBook(b)
	service := largeBookSelfCheckService(b, writer, databaseURL)
	b.ReportAllocs()
	for b.Loop() {
		_, err := service.RunSelfCheck(context.Background(), "manual")
		require.NoError(b, err)
	}
}
