package app

import (
	"context"
	"encoding/json"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"rekenraam/backend/internal/db"
	"rekenraam/backend/internal/exact"
	"rekenraam/backend/internal/marketdata"
)

type futureFXProvider struct{ calls *atomic.Int64 }

func (p futureFXProvider) Code() string { return "frankfurter" }
func (p futureFXProvider) Name() string { return "Future demand fixture" }
func (p futureFXProvider) Capabilities() marketdata.ProviderCapabilities {
	return marketdata.ProviderCapabilities{FXRates: true, HistoricalFXRates: true}
}
func (p futureFXProvider) FetchFXRates(ctx context.Context, request marketdata.FXRateRequest) ([]marketdata.FXRateObservation, error) {
	p.calls.Add(1)
	return (fakeFXProvider{code: "frankfurter", rates: map[string]fakeFXRate{
		"USD/EUR": {value: 91, scale: 2}, "EUR/USD": {value: 110, scale: 2},
	}}).FetchFXRates(ctx, request)
}

type futureFXFixture struct {
	postedCurrencyFixture
	registry *marketdata.Registry
	now      time.Time
	calls    atomic.Int64
}

func newFutureFXFixture(t *testing.T, zone string, now time.Time) *futureFXFixture {
	t.Helper()
	f := &futureFXFixture{now: now}
	f.postedCurrencyFixture = newPostedCurrencyRefreshFixture(t, futureFXProvider{calls: &f.calls})
	f.registry = marketdata.NewRegistry(futureFXProvider{calls: &f.calls})
	_, err := f.database.Exec(`INSERT INTO user_preferences(user_id,time_zone,created_at,created_by_user_id,updated_at,updated_by_user_id)
		VALUES(1,?,'2026-06-12T00:00:00Z',1,'2026-06-12T00:00:00Z',1)
		ON CONFLICT(user_id) DO UPDATE SET time_zone=excluded.time_zone`, zone)
	require.NoError(t, err)
	f.restart(t, false)
	return f
}

func (f *futureFXFixture) restart(t *testing.T, reopen bool) {
	t.Helper()
	if reopen {
		var sequence int
		var name, filename string
		require.NoError(t, f.database.QueryRow(`PRAGMA database_list`).Scan(&sequence, &name, &filename))
		require.NoError(t, f.database.Close())
		var err error
		f.database, err = db.Open(context.Background(), db.SQLiteFileURL(filename))
		require.NoError(t, err)
		t.Cleanup(func() { _ = f.database.Close() })
	}
	f.pricing = NewPricingService(db.NewPricingRepository(f.database), f.registry)
	f.pricing.SetSettings(NewSettingsService(db.NewSettingsRepository(f.database)))
	f.pricing.SetNowForTest(func() time.Time { return f.now })
}

func (f *futureFXFixture) makeOutboxDue(t *testing.T) {
	t.Helper()
	_, err := f.database.Exec(`UPDATE background_work_items SET available_at=?
		WHERE kind=? AND status='pending' AND json_extract(payload_json,'$.reason')='transaction_entered'`,
		f.now.UTC().Format(time.RFC3339), fxCoverageWorkKind)
	require.NoError(t, err)
}

func (f *futureFXFixture) tick() {
	f.pricing.runDueFXCoverage(context.Background(), testLogger(), "future-demand-worker")
}

func (f *futureFXFixture) pending(t *testing.T) []db.BackgroundWorkItemRecord {
	t.Helper()
	items, err := f.pricing.repository.ListBackgroundWorkByStatus(context.Background(), BookID, fxCoverageWorkKind, "pending", 100)
	require.NoError(t, err)
	return items
}

func TestFXFutureDemandSurvivesRestartUntilOwnerMidnight(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, zone, now, due, wake string }{
		{"weekday", "UTC", "2026-06-12T12:00:00Z", "2026-06-15", "2026-06-15T00:00:00Z"},
		{"west", "America/Los_Angeles", "2026-06-12T12:00:00Z", "2026-06-15", "2026-06-15T07:00:00Z"},
		{"east", "Asia/Tokyo", "2026-06-12T12:00:00Z", "2026-06-15", "2026-06-14T15:00:00Z"},
		{"spring_dst", "Europe/Amsterdam", "2027-03-27T12:00:00Z", "2027-03-29", "2027-03-28T22:00:00Z"},
		{"autumn_dst", "Europe/Amsterdam", "2026-10-23T12:00:00Z", "2026-10-26", "2026-10-25T23:00:00Z"},
		{"midnight_gap", "America/Santiago", "2026-09-04T12:00:00Z", "2026-09-06", "2026-09-06T04:00:00Z"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			now, err := time.Parse(time.RFC3339, tc.now)
			require.NoError(t, err)
			wake, err := time.Parse(time.RFC3339, tc.wake)
			require.NoError(t, err)
			f := newFutureFXFixture(t, tc.zone, now)
			f.post(t, tc.due, "posted", "10000", 2)
			f.makeOutboxDue(t)
			f.tick()
			items := f.pending(t)
			require.Len(t, items, 1, "future-only demand must retain one durable wakeup")
			assert.Equal(t, tc.wake, items[0].AvailableAt)
			assert.Zero(t, items[0].Attempts, "waiting does not consume provider attempts")
			assert.False(t, items[0].LastError.Valid)
			assert.Zero(t, f.calls.Load())
			f.restart(t, true)
			f.now = wake.Add(-time.Second)
			f.tick()
			require.Len(t, f.pending(t), 1)
			assert.Zero(t, f.calls.Load(), "UTC midnight must not wake a west-of-UTC owner early")
			f.now = wake
			f.tick()
			assert.Empty(t, f.pending(t))
			want := 2
			if isWeekend(tc.due) {
				want = 0 // A due wait still respects the existing publication policy.
			}
			assert.EqualValues(t, want, f.calls.Load())
			assert.Equal(t, want, postedCurrencyObservations(t, f.postedCurrencyFixture, tc.due))
		})
	}
}

func TestFXFutureDemandManualAndScheduledRefreshStayDeferred(t *testing.T) {
	t.Parallel()
	f := newFutureFXFixture(t, "UTC", time.Date(2026, 6, 12, 12, 0, 0, 0, time.UTC))
	f.post(t, "2026-06-15", "posted", "10000", 2)
	f.makeOutboxDue(t)
	for range 3 {
		for _, trigger := range []string{"manual", "scheduled"} {
			run, err := f.pricing.RunRefresh(context.Background(), 1, marketDataSourceIDByCode(t, f.database, "frankfurter"), trigger)
			require.NoError(t, err)
			assert.Zero(t, run.ItemsTotal, "a future-only currency creates no current provider request")
			assert.Equal(t, "succeeded", run.Status)
		}
		f.tick()
	}
	items := f.pending(t)
	require.Len(t, items, 1, "repeated sweeps deduplicate their future wakeup")
	assert.Zero(t, items[0].Attempts)
	assert.Zero(t, f.calls.Load())
	var before, after int
	require.NoError(t, f.database.QueryRow(`SELECT count(*) FROM background_work_items`).Scan(&before))
	for range 10 {
		f.tick()
	}
	require.NoError(t, f.database.QueryRow(`SELECT count(*) FROM background_work_items`).Scan(&after))
	assert.Equal(t, before, after, "waiting must not churn queue rows")
}

func TestFXFutureDemandWakeRechecksCancelledAndReplacedPostings(t *testing.T) {
	t.Parallel()
	for _, action := range []string{"void", "delete", "replace_currency", "move_later"} {
		t.Run(action, func(t *testing.T) {
			t.Parallel()
			f := newFutureFXFixture(t, "UTC", time.Date(2026, 6, 12, 12, 0, 0, 0, time.UTC))
			tx := f.post(t, "2026-06-15", "posted", "10000", 2)
			f.makeOutboxDue(t)
			f.tick()
			require.Len(t, f.pending(t), 1)
			var err error
			switch action {
			case "void":
				_, err = f.transactionService.VoidTransaction(context.Background(), VoidTransactionInput{OwnerUserID: 1, OriginType: "internal", TransactionID: tx.ID, ChangeReason: "Cancel future fixture"})
			case "delete":
				_, err = f.transactionService.SoftDeleteTransaction(context.Background(), TransactionLifecycleInput{OwnerUserID: 1, OriginType: "internal", TransactionID: tx.ID, ChangeReason: "Cancel future fixture"})
			default:
				date, commodity := "2026-06-15", f.eurCommodityID
				if action == "move_later" {
					date, commodity = "2026-06-17", f.foreign
				}
				_, err = f.transactionService.UpdateTransaction(context.Background(), UpdateTransactionInput{OwnerUserID: 1, OriginType: "internal", TransactionID: tx.ID, ExpectedVersionID: tx.VersionID,
					Spec: TransactionInput{TransactionDate: date, Status: "posted", JournalEntries: []JournalEntryInput{{Postings: []PostingInput{
						{AccountID: f.cashAccountID, CommodityID: commodity, QuantityValue: exact.New(10000), QuantityScale: 2},
						{AccountID: f.other, CommodityID: commodity, QuantityValue: exact.New(-10000), QuantityScale: 2},
					}}}}})
			}
			require.NoError(t, err)
			f.now = time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC)
			f.tick()
			assert.Zero(t, f.calls.Load(), "stale queued dates must not force a request")
			if action == "move_later" {
				items := f.pending(t)
				require.Len(t, items, 1)
				assert.Equal(t, "2026-06-17T00:00:00Z", items[0].AvailableAt)
				assert.Zero(t, items[0].Attempts)
				f.now = time.Date(2026, 6, 17, 0, 0, 0, 0, time.UTC)
				f.tick()
				assert.EqualValues(t, 2, f.calls.Load())
				assert.Equal(t, 2, postedCurrencyObservations(t, f.postedCurrencyFixture, "2026-06-17"))
			}
			assert.Empty(t, f.pending(t))
		})
	}
}

func TestFXFutureWakeDoesNotCoalesceAwayNewCurrentDemand(t *testing.T) {
	t.Parallel()
	f := newFutureFXFixture(t, "UTC", time.Date(2026, 6, 12, 12, 0, 0, 0, time.UTC))
	f.post(t, "2026-06-15", "posted", "10000", 2)
	f.makeOutboxDue(t)
	f.tick()
	require.Len(t, f.pending(t), 1)
	f.post(t, "2026-06-12", "posted", "10000", 2)
	f.makeOutboxDue(t)
	f.tick()
	assert.EqualValues(t, 2, f.calls.Load(), "the real posting outbox must preserve an earlier current request")
	assert.Equal(t, 2, postedCurrencyObservations(t, f.postedCurrencyFixture, "2026-06-12"))
}

func TestFXFuturePostingDoesNotDeferIndependentCurrentDemand(t *testing.T) {
	t.Parallel()
	f := newFutureFXFixture(t, "UTC", time.Date(2026, 6, 12, 12, 0, 0, 0, time.UTC))
	f.post(t, "2026-06-12", "posted", "10000", 2)
	f.post(t, "2026-06-15", "posted", "10000", 2)
	run, err := f.pricing.RunRefresh(context.Background(), 1, marketDataSourceIDByCode(t, f.database, "frankfurter"), "manual")
	require.NoError(t, err)
	assert.Equal(t, 2, run.ItemsSucceeded)
	assert.EqualValues(t, 2, f.calls.Load())
}

func TestFXFutureDemandDeferralHandoffIsAtomic(t *testing.T) {
	t.Parallel()
	f := newFutureFXFixture(t, "UTC", time.Date(2026, 6, 12, 12, 0, 0, 0, time.UTC))
	f.post(t, "2026-06-15", "posted", "10000", 2)
	f.makeOutboxDue(t)
	_, err := f.database.Exec(`CREATE TRIGGER reject_future_wake BEFORE INSERT ON background_work_items WHEN json_extract(NEW.payload_json,'$.reason')='deferred' BEGIN SELECT RAISE(ABORT,'future wake enqueue refused'); END`)
	require.NoError(t, err)
	f.tick()
	items := f.pending(t)
	require.Len(t, items, 1, "the original obligation must remain retryable")
	assert.True(t, items[0].LastError.Valid)
	var payload fxCoverageWorkPayload
	require.NoError(t, json.Unmarshal([]byte(items[0].PayloadJSON), &payload))
	assert.Equal(t, "transaction_entered", payload.Reason)
	assert.Zero(t, f.calls.Load())
	_, err = f.database.Exec(`DROP TRIGGER reject_future_wake`)
	require.NoError(t, err)
	f.now, err = time.Parse(time.RFC3339, items[0].AvailableAt)
	require.NoError(t, err)
	f.tick()
	items = f.pending(t)
	require.Len(t, items, 1)
	assert.Equal(t, "2026-06-15T00:00:00Z", items[0].AvailableAt)
	assert.Zero(t, items[0].Attempts)
}

func TestFXFutureDemandWaitPreservesProviderRetryBudget(t *testing.T) {
	t.Parallel()
	f := newFutureFXFixture(t, "UTC", time.Date(2026, 6, 12, 12, 0, 0, 0, time.UTC))
	f.post(t, "2026-06-15", "posted", "10000", 2)
	f.makeOutboxDue(t)
	f.tick()
	require.Len(t, f.pending(t), 1)
	f.registry = marketdata.NewRegistry() // An unavailable provider after the wait is a real failure.
	f.restart(t, false)
	f.now = time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC)
	f.tick()
	items := f.pending(t)
	require.Len(t, items, 2)
	identities := map[int64]string{}
	for _, item := range items {
		assert.Equal(t, 1, item.Attempts, "the first provider failure starts the retry budget")
		identities[item.ID] = item.PayloadJSON
	}
	for range 20 {
		items = f.pending(t)
		if len(items) == 0 {
			break
		}
		var next time.Time
		for _, item := range items {
			assert.Equal(t, identities[item.ID], item.PayloadJSON, "retry identity must remain stable")
			due, err := time.Parse(time.RFC3339, item.AvailableAt)
			require.NoError(t, err)
			if next.IsZero() || due.Before(next) {
				next = due
			}
		}
		f.now = next
		f.tick()
	}
	assert.Empty(t, f.pending(t))
	failed, err := f.pricing.repository.ListBackgroundWorkByStatus(context.Background(), BookID, fxCoverageWorkKind, "failed", 100)
	require.NoError(t, err)
	require.Len(t, failed, 2)
	for _, item := range failed {
		assert.Equal(t, 8, item.Attempts)
		assert.Equal(t, identities[item.ID], item.PayloadJSON)
		assert.True(t, item.LastError.Valid)
	}
}

func TestFXFutureDemandSurvivesAnotherCurrencyChunkLimit(t *testing.T) {
	t.Parallel()
	f := newFutureFXFixture(t, "UTC", time.Date(2026, 6, 12, 12, 0, 0, 0, time.UTC))
	f.post(t, "2026-06-12", "posted", "10000", 2)
	gbp := seedTestCurrencyCommodity(t, f.database, "GBP")
	other := f.postedCurrencyFixture
	other.foreign = gbp
	other.post(t, "2026-06-15", "posted", "10000", 2)
	source := marketDataSourceIDByCode(t, f.database, "frankfurter")
	_, err := f.pricing.SavePolicy(context.Background(), PricingPolicyInput{OwnerUserID: 1, DefaultSourceID: &source,
		MaxBackfillDays: 1, WeekendPolicy: "skip", RoundingMode: "half_up"})
	require.NoError(t, err)
	run, successors, err := f.pricing.runFXCoverage(context.Background(), 1, fxCoverageWorkPayload{Reason: "manual"})
	require.NoError(t, err)
	assert.Equal(t, 1, run.ItemsTotal)
	require.Len(t, successors, 2, "both the current continuation and independent future wait must survive")
	var deferred, continuation bool
	for _, successor := range successors {
		var payload fxCoverageWorkPayload
		require.NoError(t, json.Unmarshal([]byte(successor.PayloadJSON), &payload))
		if payload.Reason == "deferred" {
			deferred = true
			assert.Equal(t, gbp, payload.CurrencyID)
			assert.Equal(t, "2026-06-15T00:00:00Z", successor.AvailableAt)
		}
		continuation = continuation || payload.Next != nil
	}
	assert.True(t, deferred)
	assert.True(t, continuation)
}
