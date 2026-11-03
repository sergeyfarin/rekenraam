package app

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rekenraam/backend/internal/db"
	"rekenraam/backend/internal/marketdata"
)

func fxTargetCodes(targets []fxRefreshTarget) []string {
	codes := make([]string, 0, len(targets))
	for _, target := range targets {
		codes = append(codes, target.Base.Code+"/"+target.Quote.Code)
	}
	slices.Sort(codes)
	return codes
}

// #465: a currency used only by a posted multi-currency receivable is durable
// demand even though no account default names it. The historical refresh
// planner must select the same base-relative pairs as the posted outbox.
func TestFXRefreshTargetsIncludePostedOnlyCurrency(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := newPostedCurrencyFixture(t)
	f.post(t, "2026-06-12", "posted", "10000", 2)

	demand, err := f.pricing.repository.FXDemand(ctx, BookID)
	require.NoError(t, err)
	require.Contains(t, demand, db.PricingCurrencyDemandRecord{CommodityID: f.foreign, Code: "USD", StartDate: "2026-06-12"})

	policy, err := f.pricing.pricingPolicyOrDefault(ctx)
	require.NoError(t, err)
	targets, err := f.pricing.fxRefreshTargets(ctx, policy, demand)
	require.NoError(t, err)
	assert.Equal(t, []string{"EUR/USD", "USD/EUR"}, fxTargetCodes(targets),
		"the dated demand owner and the target planner must agree on posted currencies")
}

func newPostedCurrencyRefreshFixture(t *testing.T, provider marketdata.FXRateProvider) postedCurrencyFixture {
	t.Helper()
	f := newPostedCurrencyFixture(t)
	f.pricing = NewPricingService(db.NewPricingRepository(f.database), marketdata.NewRegistry(provider))
	f.pricing.SetNowForTest(func() time.Time { return time.Date(2026, 6, 12, 12, 0, 0, 0, time.UTC) })
	_, err := f.database.Exec(`DELETE FROM background_work_items; UPDATE market_data_sources SET status='disabled' WHERE kind='provider' AND code<>'frankfurter'`)
	require.NoError(t, err)
	sourceID := marketDataSourceIDByCode(t, f.database, "frankfurter")
	_, err = f.pricing.SavePolicy(context.Background(), PricingPolicyInput{OwnerUserID: 1, DefaultSourceID: &sourceID,
		MaxBackfillDays: 10, WeekendPolicy: "skip", RoundingMode: "half_up"})
	require.NoError(t, err)
	return f
}

func postedCurrencyObservations(t *testing.T, f postedCurrencyFixture, date string) int {
	t.Helper()
	var observations int
	require.NoError(t, f.database.QueryRow(`SELECT count(*) FROM price_observations
		WHERE valuation_date=? AND voided_at IS NULL
		AND ((base_commodity_id=? AND quote_commodity_id=?) OR (base_commodity_id=? AND quote_commodity_id=?))`,
		date, f.foreign, f.eurCommodityID, f.eurCommodityID, f.foreign).Scan(&observations))
	return observations
}

func insertAccountVersion(t *testing.T, f postedCurrencyFixture, accountClass, accountKind string, allowsPostings int) {
	t.Helper()
	_, err := f.database.Exec(`INSERT INTO account_versions(account_id,version_seq,effective_from,recorded_at,changed_by_user_id,change_reason,status,opened_on,name,account_class,account_kind,default_commodity_id,allows_postings)
		VALUES(?,2,'2026-06-12','2026-06-12T12:00:00Z',1,'demand scope fixture','active','2026-01-01','Demand receivable',?,?,?,?)`,
		f.cashAccountID, accountClass, accountKind, f.foreign, allowsPostings)
	require.NoError(t, err)
}

// #466: coverage start dates and refresh targets come from one demand read, so
// an account default either contributes both or neither. A default that cannot
// accept postings or whose commodity version is archived has no future FX
// obligation; a posted currency keeps its obligation regardless.
func TestFXDemandCoverageAndTargetsShareAccountDefaultScope(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name      string
		wantStart string
		prepare   func(t *testing.T, f postedCurrencyFixture)
	}{
		{name: "active_postable_default", wantStart: "2026-01-01", prepare: func(t *testing.T, f postedCurrencyFixture) {
			insertAccountVersion(t, f, "asset", "receivable", 1)
		}},
		{name: "non_postable_default", prepare: func(t *testing.T, f postedCurrencyFixture) {
			insertAccountVersion(t, f, "asset", "receivable", 0)
		}},
		{name: "archived_currency_default", prepare: func(t *testing.T, f postedCurrencyFixture) {
			insertAccountVersion(t, f, "asset", "receivable", 1)
			_, err := f.database.Exec(`INSERT INTO commodity_versions(commodity_id,version_seq,effective_from,recorded_at,changed_by_user_id,change_reason,status,symbol,display_symbol,name,standard_scale,max_quantity_scale)
				VALUES(?,2,'2026-06-12','2026-06-12T12:00:00Z',1,'archived fixture','archived','USD','USD','US dollar',2,6)`, f.foreign)
			require.NoError(t, err)
		}},
		{name: "posted_on_non_postable_account", wantStart: "2026-06-12", prepare: func(t *testing.T, f postedCurrencyFixture) {
			f.post(t, "2026-06-12", "posted", "10000", 2)
			_, err := f.database.Exec(`INSERT INTO account_versions(account_id,version_seq,effective_from,recorded_at,changed_by_user_id,change_reason,status,opened_on,name,account_class,account_kind,allows_postings)
				VALUES(?,2,'2026-06-12','2026-06-12T12:00:00Z',1,'non-postable fixture','active','2026-01-01','Read-only receivable','asset','receivable',0)`, f.cashAccountID)
			require.NoError(t, err)
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			f := newPostedCurrencyFixture(t)
			test.prepare(t, f)

			demand, err := f.pricing.fxDemand(ctx)
			require.NoError(t, err)
			policy, err := f.pricing.pricingPolicyOrDefault(ctx)
			require.NoError(t, err)
			targets, err := f.pricing.fxRefreshTargets(ctx, policy, demand)
			require.NoError(t, err)

			demandIDs := make([]int64, 0, len(demand))
			for _, record := range demand {
				demandIDs = append(demandIDs, record.CommodityID)
			}
			targetIDs := make([]int64, 0, len(targets)*2)
			seenTargetIDs := map[int64]bool{}
			for _, target := range targets {
				for _, id := range []int64{target.Base.ID, target.Quote.ID} {
					if !seenTargetIDs[id] {
						seenTargetIDs[id] = true
						targetIDs = append(targetIDs, id)
					}
				}
			}
			wantDemandIDs := []int64{}
			wantTargetIDs := []int64{}
			if test.wantStart != "" {
				wantDemandIDs = []int64{f.foreign}
				wantTargetIDs = []int64{f.eurCommodityID, f.foreign}
				assert.Contains(t, demand, db.PricingCurrencyDemandRecord{CommodityID: f.foreign, Code: "USD", StartDate: test.wantStart})
			}
			assert.ElementsMatch(t, wantDemandIDs, demandIDs,
				"the demand read owns every coverage start date")
			assert.ElementsMatch(t, wantTargetIDs, targetIDs,
				"every demanded start date must own a refresh target and every planned currency must own a start date")
		})
	}
}

// #465: the durable worker must actually fetch the posted-only pair/date, not
// merely list it. The provider registry is the deterministic stub boundary;
// unregistered production providers cannot perform network I/O here.
func TestFXCoverageWorkerFetchesPostedOnlyCurrency(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := newPostedCurrencyRefreshFixture(t, fakeFXProvider{
		code: "frankfurter",
		rates: map[string]fakeFXRate{
			"USD/EUR": {value: 91234, scale: 5},
			"EUR/USD": {value: 109730, scale: 5},
		},
	})

	f.post(t, "2026-06-12", "posted", "10000", 2)
	// Two postings in one transaction are one currency demand, not one work
	// item per posting.
	var pending int
	require.NoError(t, f.database.QueryRow(`SELECT count(*) FROM background_work_items
		WHERE kind=? AND status='pending' AND json_extract(payload_json,'$.reason')='transaction_entered'`,
		fxCoverageWorkKind).Scan(&pending))
	assert.Equal(t, 1, pending)
	// The SQLite outbox uses the real clock; make its demand due on the frozen
	// owner-local fixture clock.
	_, err := f.database.Exec(`UPDATE background_work_items SET available_at=? WHERE status='pending' AND json_extract(payload_json,'$.reason')='transaction_entered'`, "2026-06-12T12:00:00Z")
	require.NoError(t, err)

	f.pricing.runDueFXCoverage(ctx, testLogger(), "posted-currency-worker")

	assert.Zero(t, pendingFXCoverageWorkCount(t, f.database))
	assert.Equal(t, 2, postedCurrencyObservations(t, f, "2026-06-12"),
		"the worker must fetch the missing posted-currency pair in both directions")
}

// #465: manual and scheduled refreshes are the same planner as posted work.
func TestManualAndScheduledRefreshIncludePostedOnlyCurrency(t *testing.T) {
	t.Parallel()
	for _, trigger := range []string{"manual", "scheduled"} {
		t.Run(trigger, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			f := newPostedCurrencyRefreshFixture(t, fakeFXProvider{
				code: "frankfurter",
				rates: map[string]fakeFXRate{
					"USD/EUR": {value: 91234, scale: 5},
					"EUR/USD": {value: 109730, scale: 5},
				},
			})
			f.post(t, "2026-06-12", "posted", "10000", 2)
			_, err := f.pricing.RunRefresh(ctx, 1, marketDataSourceIDByCode(t, f.database, "frankfurter"), trigger)
			require.NoError(t, err)
			assert.Equal(t, 2, postedCurrencyObservations(t, f, "2026-06-12"),
				"the %s path must visit the posted-only pair", trigger)
		})
	}
}

// #465: historical demand is not the same as current exposure. A zero balance,
// a closed account and an archived currency keep their posted rate obligation;
// drafts, voids and deletes never create one.
func TestFXRefreshPostedCurrencyDemandScope(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name    string
		want    bool
		prepare func(t *testing.T, f postedCurrencyFixture)
	}{
		{name: "posted", want: true, prepare: func(t *testing.T, f postedCurrencyFixture) {
			f.post(t, "2026-06-12", "posted", "10000", 2)
		}},
		{name: "zero_balance", want: true, prepare: func(t *testing.T, f postedCurrencyFixture) {
			f.post(t, "2026-06-12", "posted", "10000", 2)
			f.post(t, "2026-06-12", "posted", "-10000", 2)
		}},
		{name: "closed_account", want: true, prepare: func(t *testing.T, f postedCurrencyFixture) {
			f.post(t, "2026-06-12", "posted", "10000", 2)
			_, err := f.database.Exec(`INSERT INTO account_versions(account_id,version_seq,effective_from,recorded_at,changed_by_user_id,change_reason,status,opened_on,closed_on,name,account_class,account_kind,allows_postings)
				VALUES(?,2,'2026-06-12','2026-06-12T12:00:00Z',1,'closed fixture','closed','2026-01-01','2026-06-12','Closed receivable','asset','receivable',1)`, f.cashAccountID)
			require.NoError(t, err)
		}},
		{name: "archived_currency", want: true, prepare: func(t *testing.T, f postedCurrencyFixture) {
			f.post(t, "2026-06-12", "posted", "10000", 2)
			_, err := f.database.Exec(`INSERT INTO commodity_versions(commodity_id,version_seq,effective_from,recorded_at,changed_by_user_id,change_reason,status,symbol,display_symbol,name,standard_scale,max_quantity_scale)
				VALUES(?,2,'2026-06-12','2026-06-12T12:00:00Z',1,'archived fixture','archived','USD','USD','US dollar',2,6)`, f.foreign)
			require.NoError(t, err)
		}},
		{name: "income_only", want: true, prepare: func(t *testing.T, f postedCurrencyFixture) {
			f.cashAccountID = f.incomeAccountID
			f.post(t, "2026-06-12", "posted", "10000", 2)
		}},
		{name: "draft", want: false, prepare: func(t *testing.T, f postedCurrencyFixture) {
			f.post(t, "2026-06-12", "draft", "10000", 2)
		}},
		{name: "deleted", want: false, prepare: func(t *testing.T, f postedCurrencyFixture) {
			tx := f.post(t, "2026-06-12", "posted", "10000", 2)
			_, err := f.transactionService.SoftDeleteTransaction(context.Background(), TransactionLifecycleInput{
				OwnerUserID: 1, OriginType: "internal", TransactionID: tx.ID, ChangeReason: "scope fixture"})
			require.NoError(t, err)
		}},
		{name: "voided", want: false, prepare: func(t *testing.T, f postedCurrencyFixture) {
			tx := f.post(t, "2026-06-12", "posted", "10000", 2)
			_, err := f.transactionService.VoidTransaction(context.Background(), VoidTransactionInput{
				OwnerUserID: 1, OriginType: "internal", TransactionID: tx.ID, ChangeReason: "scope fixture"})
			require.NoError(t, err)
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			f := newPostedCurrencyFixture(t)
			test.prepare(t, f)
			ctx := context.Background()
			demand, err := f.pricing.fxDemand(ctx)
			require.NoError(t, err)
			policy, err := f.pricing.pricingPolicyOrDefault(ctx)
			require.NoError(t, err)
			targets, err := f.pricing.fxRefreshTargets(ctx, policy, demand)
			require.NoError(t, err)
			want := []string{}
			if test.want {
				want = []string{"EUR/USD", "USD/EUR"}
			}
			assert.Equal(t, want, fxTargetCodes(targets))
		})
	}
}

// #465: a future posted entry creates durable target identity but no request
// before its owner-local date; ADR 0010 bounds coverage through today.
func TestFXRefreshFuturePostedCurrencyDoesNotFetchBeforeItsDate(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := newPostedCurrencyFixture(t)
	f.post(t, "2026-06-13", "posted", "10000", 2)

	demand, err := f.pricing.fxDemand(ctx)
	require.NoError(t, err)
	policy, err := f.pricing.pricingPolicyOrDefault(ctx)
	require.NoError(t, err)
	targets, err := f.pricing.fxRefreshTargets(ctx, policy, demand)
	require.NoError(t, err)
	assert.Equal(t, []string{"EUR/USD", "USD/EUR"}, fxTargetCodes(targets))

	run, successors, err := f.pricing.runFXCoverage(ctx, 1, fxCoverageWorkPayload{
		Reason: "transaction_entered", CurrencyID: f.foreign, StartDate: "2026-06-13"})
	require.NoError(t, err)
	assert.Zero(t, run.ItemsTotal)
	require.Len(t, successors, 1, "future demand retains a durable owner-date wakeup (#471)")
	assert.Equal(t, "2026-06-13T00:00:00Z", successors[0].AvailableAt)
	assert.Zero(t, successors[0].Attempts)
	assert.Zero(t, postedCurrencyObservations(t, f, "2026-06-13"))
}

// #465: adding the target must surface an unsupported provider as explicit
// failed work, never as an empty successful sweep that implies coverage.
func TestFXCoveragePostedOnlyCurrencyWithoutProviderFailsExplicitly(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := newPostedCurrencyFixture(t)
	f.pricing = NewPricingService(db.NewPricingRepository(f.database), marketdata.NewRegistry())
	f.pricing.SetNowForTest(func() time.Time { return time.Date(2026, 6, 12, 12, 0, 0, 0, time.UTC) })
	f.post(t, "2026-06-12", "posted", "10000", 2)

	run, successors, err := f.pricing.runFXCoverage(ctx, 1, fxCoverageWorkPayload{
		Reason: "transaction_entered", CurrencyID: f.foreign, StartDate: "2026-06-12"})
	require.NoError(t, err)
	assert.Equal(t, "failed", run.Status)
	assert.Equal(t, 2, run.ItemsTotal)
	assert.Equal(t, 2, run.ItemsFailed)
	require.Len(t, successors, 2, "each failed pair/date must retain a bounded retry")
	assert.Zero(t, postedCurrencyObservations(t, f, "2026-06-12"))
}
