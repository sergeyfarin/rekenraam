package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"rekenraam/backend/internal/db"
	"rekenraam/backend/internal/exact"
	"rekenraam/backend/internal/marketdata"
)

// #274: a chunk limit bounds execution, never the historical horizon. Drive
// the real worker through more than two chunks, including a service restart.
func TestFXCoverageWorkerHolidayDoesNotStarveLaterPublicationDates(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/rates", r.URL.Path)
		base, quote := r.URL.Query().Get("base"), r.URL.Query().Get("quotes")
		rate := "0.8"
		if base == "EUR" {
			rate = "1.25"
		}
		date := r.URL.Query().Get("date")
		if date == "2026-06-10" {
			date = "2026-06-09"
		}
		_, _ = fmt.Fprintf(w, `{"base":%q,"date":%q,"rates":{%q:%s}}`, base, date, quote, rate)
	}))
	defer server.Close()
	registry := marketdata.DefaultRegistryAt(server.URL, "", "", "")
	database, service := newPricingRefreshTestService(t, []string{"USD", "EUR"}, registry)
	// Remove fixture activation work; the one request below owns this horizon.
	_, err := database.Exec(`DELETE FROM background_work_items`)
	require.NoError(t, err)
	sourceID := marketDataSourceIDByCode(t, database, "frankfurter")
	_, err = service.SavePolicy(ctx, PricingPolicyInput{OwnerUserID: 1, DefaultSourceID: &sourceID,
		MaxBackfillDays: 1, WeekendPolicy: "skip", RoundingMode: "half_up"})
	require.NoError(t, err)
	_, err = service.repository.EnqueueBackgroundWork(ctx, BookID, fxCoverageWorkKind,
		`{"reason":"manual","start_date":"2026-06-09","currency_id":2}`, "2026-06-12T00:00:00Z")
	require.NoError(t, err)
	for tick := 0; tick < 12 && pendingFXCoverageWorkCount(t, database) > 0; tick++ {
		// Recreate the service between ticks: no in-memory progress can rescue it.
		service = NewPricingService(db.NewPricingRepository(database), registry)
		service.SetNowForTest(func() time.Time { return time.Date(2026, 6, 12, 12, 0, 0, 0, time.UTC) })
		service.runDueFXCoverage(ctx, testLogger(), fmt.Sprintf("worker-%d", tick))
	}
	assert.Zero(t, pendingFXCoverageWorkCount(t, database))
	for _, pair := range [][2]int64{{1, 2}, {2, 1}} {
		prices, err := service.ListPrices(ctx, pair[0], pair[1], 20, false)
		require.NoError(t, err)
		require.Len(t, prices, 3, "June 9, 11 and 12 must be covered despite the June 10 publication gap")
		for i, price := range prices {
			assert.Equal(t, []string{"2026-06-12", "2026-06-11", "2026-06-09"}[i], price.ValuationDate)
			if pair[0] == 1 {
				assert.Equal(t, int64(8), price.PriceValue)
				assert.Equal(t, 1, price.PriceScale)
			} else {
				assert.Equal(t, int64(125), price.PriceValue)
				assert.Equal(t, 2, price.PriceScale)
			}
		}
	}
}

type fxProgressFixture struct {
	database *sql.DB
	service  *PricingService
	registry *marketdata.Registry
	now      time.Time
}

// All provider I/O goes through the production override registry to a stub.
func newFXProgressFixture(t *testing.T, codes []string, start string, respond func(http.ResponseWriter, *http.Request)) *fxProgressFixture {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(respond))
	t.Cleanup(server.Close)
	registry := marketdata.DefaultRegistryAt(server.URL, "", "", "")
	database, service := newPricingRefreshTestService(t, codes, registry)
	f := &fxProgressFixture{database: database, service: service, registry: registry, now: time.Date(2026, 6, 12, 12, 0, 0, 0, time.UTC)}
	_, err := database.Exec(`DELETE FROM background_work_items; UPDATE market_data_sources SET status='disabled' WHERE kind='provider' AND code<>'frankfurter'`)
	require.NoError(t, err)
	source := marketDataSourceIDByCode(t, database, "frankfurter")
	_, err = service.SavePolicy(context.Background(), PricingPolicyInput{OwnerUserID: 1, DefaultSourceID: &source, MaxBackfillDays: 1, WeekendPolicy: "skip", RoundingMode: "half_up"})
	require.NoError(t, err)
	f.enqueue(t, start)
	return f
}

func (f *fxProgressFixture) enqueue(t *testing.T, start string) {
	t.Helper()
	encoded, err := json.Marshal(fxCoverageWorkPayload{Reason: "manual", StartDate: start})
	require.NoError(t, err)
	_, err = f.service.repository.EnqueueBackgroundWork(context.Background(), BookID, fxCoverageWorkKind, string(encoded), f.now.Format(time.RFC3339))
	require.NoError(t, err)
}

func (f *fxProgressFixture) tick(t *testing.T, reopen bool) {
	t.Helper()
	if reopen {
		var seq int
		var name, filename string
		require.NoError(t, f.database.QueryRow(`PRAGMA database_list`).Scan(&seq, &name, &filename))
		require.NoError(t, f.database.Close())
		var err error
		f.database, err = db.Open(context.Background(), db.SQLiteFileURL(filename))
		require.NoError(t, err)
		t.Cleanup(func() { _ = f.database.Close() })
	}
	f.service = NewPricingService(db.NewPricingRepository(f.database), f.registry)
	f.service.SetNowForTest(func() time.Time { return f.now })
	f.service.runDueFXCoverage(context.Background(), testLogger(), "progress-worker")
}

func (f *fxProgressFixture) drain(t *testing.T, reopen bool) {
	t.Helper()
	for range 40 {
		f.tick(t, reopen)
		var due int
		require.NoError(t, f.database.QueryRow(`SELECT count(*) FROM background_work_items WHERE kind=? AND status='pending' AND available_at<=?`, fxCoverageWorkKind, f.now.Format(time.RFC3339)).Scan(&due))
		if due == 0 {
			return
		}
	}
	require.FailNow(t, "backfill did not drain within the bounded fixture")
}

func writeFXProgressRate(w http.ResponseWriter, r *http.Request, date string) {
	_, _ = fmt.Fprintf(w, `{"base":%q,"date":%q,"rates":{%q:1}}`, r.URL.Query().Get("base"), date, r.URL.Query().Get("quotes"))
}

func TestFXCoverageCursorSurvivesReopenAcrossManyGapsAndPairs(t *testing.T) {
	t.Parallel()
	f := newFXProgressFixture(t, []string{"USD", "EUR", "GBP"}, "2026-06-01", func(w http.ResponseWriter, r *http.Request) {
		date := r.URL.Query().Get("date")
		// Ten weekdays: four return earlier evidence, leaving six fixings.
		if earlier := map[string]string{"2026-06-02": "2026-06-01", "2026-06-04": "2026-06-03", "2026-06-09": "2026-06-08", "2026-06-10": "2026-06-08"}[date]; earlier != "" {
			date = earlier
		}
		writeFXProgressRate(w, r, date)
	})
	f.drain(t, true)
	assert.Zero(t, pendingFXCoverageWorkCount(t, f.database))
	for _, pair := range [][2]int64{{1, 2}, {1, 3}, {2, 1}, {3, 1}} {
		prices, err := f.service.ListPrices(context.Background(), pair[0], pair[1], 100, false)
		require.NoError(t, err)
		require.Len(t, prices, 6)
		for i, date := range []string{"2026-06-12", "2026-06-11", "2026-06-08", "2026-06-05", "2026-06-03", "2026-06-01"} {
			assert.Equal(t, date, prices[i].ValuationDate)
			assert.Equal(t, int64(1), prices[i].PriceValue)
			assert.Zero(t, prices[i].PriceScale)
		}
	}
	var attempts, oversized, earlierEvidence int
	require.NoError(t, f.database.QueryRow(`SELECT sum(items_total) FROM market_data_ingest_runs`).Scan(&attempts))
	require.NoError(t, f.database.QueryRow(`SELECT count(*) FROM market_data_ingest_runs WHERE items_total>1`).Scan(&oversized))
	require.NoError(t, f.database.QueryRow(`SELECT count(*) FROM market_data_ingest_items WHERE status='skipped' AND json_extract(raw_json,'$.requested_date')<>json_extract(raw_json,'$.observed_date')`).Scan(&earlierEvidence))
	assert.Equal(t, 40, attempts, "ten weekdays in four directions visited once")
	assert.Zero(t, oversized, "each execution obeys the one-attempt budget")
	assert.Equal(t, 16, earlierEvidence, "earlier evidence is recorded as earlier, never a fabricated quote")
}

func TestFXCoverageFailedDatesDoNotStarveHistoryAndRetainBoundedRetry(t *testing.T) {
	t.Parallel()
	for _, mode := range []string{"transient", "unsupported", "malformed", "future date", "missing date"} {
		t.Run(mode, func(t *testing.T) {
			var repaired atomic.Bool
			var failedDateCalls atomic.Int32
			f := newFXProgressFixture(t, []string{"USD", "EUR", "GBP"}, "2026-06-09", func(w http.ResponseWriter, r *http.Request) {
				date := r.URL.Query().Get("date")
				if r.URL.Query().Get("base") == "EUR" && r.URL.Query().Get("quotes") == "USD" && date == "2026-06-09" {
					failedDateCalls.Add(1)
					if !repaired.Load() {
						switch mode {
						case "transient":
							w.WriteHeader(503)
						case "unsupported":
							w.WriteHeader(400)
						case "malformed":
							_, _ = fmt.Fprint(w, `{`)
						case "future date":
							writeFXProgressRate(w, r, "2026-06-13")
						case "missing date":
							writeFXProgressRate(w, r, "")
						}
						return
					}
				}
				writeFXProgressRate(w, r, date)
			})
			f.drain(t, true)
			var observations, failedItems, skippedFailures int
			require.NoError(t, f.database.QueryRow(`SELECT count(*) FROM price_observations`).Scan(&observations))
			require.NoError(t, f.database.QueryRow(`SELECT count(*) FROM market_data_ingest_items WHERE status='failed' AND json_extract(raw_json,'$.requested_date')='2026-06-09'`).Scan(&failedItems))
			require.NoError(t, f.database.QueryRow(`SELECT count(*) FROM market_data_ingest_items WHERE status='skipped' AND json_extract(raw_json,'$.base_commodity_id')=2 AND json_extract(raw_json,'$.quote_commodity_id')=1 AND json_extract(raw_json,'$.requested_date')='2026-06-09'`).Scan(&skippedFailures))
			assert.Equal(t, 15, observations, "all other dates and pairs must finish before the failed date retries")
			assert.Equal(t, 1, failedItems)
			assert.Zero(t, skippedFailures, "a failed response is never called a holiday")
			workspace, err := f.service.Workspace(context.Background(), "2026-06-09")
			require.NoError(t, err)
			for _, coverage := range workspace.Coverage {
				if coverage.BaseCommodityID == 2 && coverage.QuoteCommodityID == 1 {
					assert.Equal(t, "missing", coverage.Status)
				}
				if coverage.BaseCommodityID == 3 && coverage.QuoteCommodityID == 1 {
					assert.Equal(t, "usable", coverage.Status)
				}
			}
			pending, err := f.service.repository.ListBackgroundWorkByStatus(context.Background(), BookID, fxCoverageWorkKind, "pending", 10)
			require.NoError(t, err)
			require.Len(t, pending, 1)
			assert.Equal(t, 1, pending[0].Attempts)
			assert.Greater(t, pending[0].AvailableAt, f.now.Format(time.RFC3339))
			for range maxFXCoverageAttempts - 1 {
				f.now = f.now.Add(24 * time.Hour)
				f.drain(t, true)
			}
			failed, err := f.service.FailedBackgroundWork(context.Background())
			require.NoError(t, err)
			require.Len(t, failed, 1)
			assert.Equal(t, maxFXCoverageAttempts, failed[0].Attempts)
			assert.Equal(t, int32(8), failedDateCalls.Load())
			assert.NotEmpty(t, failed[0].LastError)
			assert.Contains(t, failed[0].LastError, "EUR/USD on 2026-06-09", "the existing failed-work UI must identify the affected scope")
			repaired.Store(true)
			_, err = f.service.RetryBackgroundWork(context.Background(), failed[0].ID)
			require.NoError(t, err)
			f.drain(t, true)
			failed, err = f.service.FailedBackgroundWork(context.Background())
			require.NoError(t, err)
			assert.Empty(t, failed)
			require.NoError(t, f.database.QueryRow(`SELECT count(*) FROM price_observations`).Scan(&observations))
			assert.Equal(t, 16, observations, "manual retry repairs exactly one missing publication")
		})
	}
}

func TestFXCoverageFreshDemandRevisitsLatePublicationAndEarlierDates(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	var published atomic.Bool
	f := newFXProgressFixture(t, []string{"USD", "EUR"}, "2026-06-09", func(w http.ResponseWriter, r *http.Request) {
		date := r.URL.Query().Get("date")
		if date == "2026-06-10" && !published.Load() {
			date = "2026-06-09"
		}
		writeFXProgressRate(w, r, date)
	})
	// Make the initial request currency-scoped, as the real posting outbox is.
	_, err := f.database.Exec(`UPDATE background_work_items SET payload_json='{"reason":"transaction_entered","start_date":"2026-06-09","currency_id":2}'`)
	require.NoError(t, err)
	f.tick(t, false) // four one-date chunks; the reverse pair is still queued.
	pending, err := f.service.repository.ListBackgroundWorkByStatus(ctx, BookID, fxCoverageWorkKind, "pending", 10)
	require.NoError(t, err)
	require.Len(t, pending, 1)
	assert.Contains(t, pending[0].PayloadJSON, `"next"`)
	// Bookkeeping currency versions must admit the backdated fixture postings.
	_, err = f.database.Exec(`INSERT INTO commodity_versions(commodity_id,version_seq,effective_from,recorded_at,changed_by_user_id,change_reason,status,symbol,display_symbol,name,standard_scale,max_quantity_scale)
 SELECT commodity_id,2,'0001-01-01','2026-06-12T00:00:00Z',1,'backdated fixture','active',symbol,display_symbol,name,standard_scale,max_quantity_scale FROM commodity_versions WHERE version_seq=1`)
	require.NoError(t, err)
	left := seedTestAccount(t, f.database, "active", true)
	right := seedTestAccount(t, f.database, "active", true)
	post := func(date string) {
		transactions := NewTransactionService(db.NewTransactionRepository(f.database), db.NewPayeeRepository(f.database), db.NewAccountRepository(f.database), db.NewCommodityRepository(f.database))
		_, err := transactions.CreateTransaction(ctx, CreateTransactionInput{OwnerUserID: 1, OriginType: "browser_api", Spec: TransactionInput{
			TransactionDate: date, JournalEntries: []JournalEntryInput{{Postings: []PostingInput{
				{AccountID: left, CommodityID: 2, QuantityValue: exact.MustParse("1")},
				{AccountID: right, CommodityID: 2, QuantityValue: exact.MustParse("-1")},
			}}},
		}})
		require.NoError(t, err)
		// The SQLite outbox uses the real clock, while this fixture pins the
		// owner date to June 12. Make its work due on the fixture clock.
		_, err = f.database.Exec(`UPDATE background_work_items SET available_at=? WHERE status='pending' AND json_extract(payload_json,'$.reason')='transaction_entered'`, f.now.Format(time.RFC3339))
		require.NoError(t, err)
	}
	published.Store(true)
	post("2026-06-10")
	var fresh int
	require.NoError(t, f.database.QueryRow(`SELECT count(*) FROM background_work_items WHERE status='pending' AND json_extract(payload_json,'$.reason')='transaction_entered' AND json_extract(payload_json,'$.start_date')='2026-06-10'`).Scan(&fresh))
	assert.Equal(t, 1, fresh, "a suffix continuation cannot swallow fresh financial demand")
	f.drain(t, true)
	for _, pair := range [][2]int64{{1, 2}, {2, 1}} {
		var count int
		require.NoError(t, f.database.QueryRow(`SELECT count(*) FROM price_observations WHERE base_commodity_id=? AND quote_commodity_id=? AND valuation_date='2026-06-10'`, pair[0], pair[1]).Scan(&count))
		assert.Equal(t, 1, count, "late publication arrives once in each direction")
	}
	post("2026-06-08")
	f.drain(t, true)
	for _, pair := range [][2]int64{{1, 2}, {2, 1}} {
		var count int
		require.NoError(t, f.database.QueryRow(`SELECT count(*) FROM price_observations WHERE base_commodity_id=? AND quote_commodity_id=? AND valuation_date='2026-06-08'`, pair[0], pair[1]).Scan(&count))
		assert.Equal(t, 1, count, "a backdated posting extends demand before the old scan's start")
	}
	// A new explicit refresh is a fresh missing-date sweep, even after old
	// cursors completed. Existing publications remain idempotent.
	var before, after int
	require.NoError(t, f.database.QueryRow(`SELECT count(*) FROM price_observations`).Scan(&before))
	f.enqueue(t, "2026-06-08")
	f.drain(t, true)
	require.NoError(t, f.database.QueryRow(`SELECT count(*) FROM price_observations`).Scan(&after))
	assert.Equal(t, before, after)
}

func TestFXCoverageCompletedSweepDoesNotHideLatePublicationFromManualRefresh(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	var published atomic.Bool
	f := newFXProgressFixture(t, []string{"USD", "EUR"}, "2026-06-09", func(w http.ResponseWriter, r *http.Request) {
		date := r.URL.Query().Get("date")
		if date == "" {
			date = "2026-06-12"
		}
		if date == "2026-06-10" && !published.Load() {
			date = "2026-06-09"
		}
		writeFXProgressRate(w, r, date)
	})
	// Durable account demand survives completion of the initial queue item.
	_, err := f.database.Exec(`INSERT INTO accounts(id,book_id,created_at,created_by_user_id) VALUES(3,1,'2026-06-09T00:00:00Z',1);
	 INSERT INTO account_versions(account_id,version_seq,effective_from,recorded_at,changed_by_user_id,change_reason,status,opened_on,name,account_class,account_kind,default_commodity_id,allows_postings)
	 VALUES(3,1,'2026-06-09','2026-06-09T00:00:00Z',1,'historical demand','active','2026-06-09','Historical EUR','asset','cash',2,1)`)
	require.NoError(t, err)
	f.drain(t, true)
	assert.Zero(t, pendingFXCoverageWorkCount(t, f.database))
	var count int
	require.NoError(t, f.database.QueryRow(`SELECT count(*) FROM price_observations`).Scan(&count))
	assert.Equal(t, 6, count)
	published.Store(true)
	_, err = f.service.RunRefresh(ctx, 1, 0, "manual")
	require.NoError(t, err)
	f.drain(t, true)
	require.NoError(t, f.database.QueryRow(`SELECT count(*) FROM price_observations`).Scan(&count))
	assert.Equal(t, 8, count, "fresh manual refresh must revisit the earlier-date response from the completed sweep")
	_, err = f.service.RunRefresh(ctx, 1, 0, "manual")
	require.NoError(t, err)
	f.drain(t, true)
	require.NoError(t, f.database.QueryRow(`SELECT count(*) FROM price_observations`).Scan(&count))
	assert.Equal(t, 8, count, "repeat refresh remains idempotent")
}

func TestFXCoverageInvalidCursorFailsBeforeStartingAnIngestRun(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	database, service := newPricingRefreshTestService(t, []string{"USD"}, marketdata.NewRegistry())
	_, err := database.Exec(`DELETE FROM background_work_items`)
	require.NoError(t, err)
	for _, payload := range []string{
		`{"next":{"base_commodity_id":0,"quote_commodity_id":2,"date":"2026-06-09"}}`,
		`{"next":{"base_commodity_id":1,"quote_commodity_id":1,"date":"2026-06-09"}}`,
		`{"next":{"base_commodity_id":1,"quote_commodity_id":2,"date":"invalid"}}`,
		`{"through_date":"invalid"}`,
		`{"next":{"base_commodity_id":1,"quote_commodity_id":2,"date":"2026-06-09"},"retry":{"base_commodity_id":1,"quote_commodity_id":2,"date":"2026-06-09"}}`,
	} {
		item, err := service.repository.EnqueueBackgroundWork(ctx, BookID, fxCoverageWorkKind, payload, "2026-06-12T00:00:00Z")
		require.NoError(t, err)
		service.runDueFXCoverage(ctx, testLogger(), "invalid-cursor")
		item, err = service.repository.BackgroundWorkByID(ctx, item.ID)
		require.NoError(t, err)
		assert.Equal(t, "failed", item.Status)
		assert.Equal(t, 1, item.Attempts)
		assert.Contains(t, item.LastError.String, "invalid FX coverage progress")
	}
	var count int
	require.NoError(t, database.QueryRow(`SELECT count(*) FROM market_data_ingest_runs`).Scan(&count))
	assert.Zero(t, count)
}

func TestFXRefreshValuationCeilingUsesOwnerLocalToday(t *testing.T) {
	t.Parallel()
	f := newFXProgressFixture(t, []string{"USD", "EUR"}, "2026-06-12", func(w http.ResponseWriter, r *http.Request) {
		writeFXProgressRate(w, r, "2026-06-13")
	})
	setOwnerTimeZone(t, f.database, "Europe/Amsterdam")
	// The run pins the owner day through settings, as production wires it.
	f.service.SetSettings(NewSettingsService(db.NewSettingsRepository(f.database)))
	f.service.SetNowForTest(func() time.Time { return time.Date(2026, 6, 12, 22, 30, 0, 0, time.UTC) })
	run, err := f.service.runFXRefresh(context.Background(), 1, 0, "manual")
	require.NoError(t, err)
	assert.Equal(t, "succeeded", run.Status, "June 13 is already today in the owner's zone")
	assert.Equal(t, 2, run.ItemsSucceeded)
}
