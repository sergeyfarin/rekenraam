package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"rekenraam/backend/internal/db"
	"rekenraam/backend/internal/exact"
	"rekenraam/backend/internal/marketdata"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCleanPriceObservationSpecDefaultsAndCompactsJSON(t *testing.T) {
	spec, err := cleanPriceObservationSpec(PriceObservationInput{
		BaseCommodityID:    10,
		QuoteCommodityID:   20,
		PriceValue:         12345,
		PriceScale:         4,
		ValuationDate:      " 2026-06-12 ",
		ObservedAt:         "2026-06-12T11:59:00Z",
		SourcePublishedAt:  "2026-06-12T12:00:00Z",
		DerivationJSON:     `{" formula" : "manual" }`,
		SeriesMetadataJSON: `{" series" : "manual" }`,
		MetadataJSON:       `{" source" : "test" }`,
	})
	require.NoError(t, err)

	assert.Equal(t, int64(10), spec.BaseCommodityID)
	assert.Equal(t, int64(20), spec.QuoteCommodityID)
	assert.Equal(t, "manual", spec.QuoteType)
	assert.Equal(t, "raw", spec.AdjustmentBasis)
	assert.Equal(t, int64(1), spec.BaseQuantityValue)
	assert.Equal(t, 0, spec.BaseQuantityScale)
	assert.Equal(t, "2026-06-12", spec.ValuationDate)
	assert.Equal(t, `{" formula":"manual"}`, spec.DerivationJSON)
	assert.Equal(t, `{" series":"manual"}`, spec.SeriesMetadataJSON)
	assert.Equal(t, `{" source":"test"}`, spec.MetadataJSON)
}

func TestCleanPriceObservationSpecRejectsInvalidInputs(t *testing.T) {
	valid := PriceObservationInput{
		BaseCommodityID:   10,
		QuoteCommodityID:  20,
		QuoteType:         "official_fixing",
		AdjustmentBasis:   "not_applicable",
		PriceValue:        12345,
		PriceScale:        4,
		BaseQuantityValue: 1,
		BaseQuantityScale: 0,
		ValuationDate:     "2026-06-12",
		MetadataJSON:      `{}`,
	}
	tests := []struct {
		name   string
		mutate func(*PriceObservationInput)
	}{
		{name: "missing base commodity", mutate: func(input *PriceObservationInput) { input.BaseCommodityID = 0 }},
		{name: "missing quote commodity", mutate: func(input *PriceObservationInput) { input.QuoteCommodityID = 0 }},
		{name: "zero price", mutate: func(input *PriceObservationInput) { input.PriceValue = 0 }},
		{name: "negative price", mutate: func(input *PriceObservationInput) { input.PriceValue = -1 }},
		{name: "price scale too high", mutate: func(input *PriceObservationInput) { input.PriceScale = 19 }},
		{name: "invalid valuation date", mutate: func(input *PriceObservationInput) { input.ValuationDate = "2026/06/12" }},
		{name: "invalid quote type", mutate: func(input *PriceObservationInput) { input.QuoteType = "last_trade" }},
		{name: "invalid adjustment basis", mutate: func(input *PriceObservationInput) { input.AdjustmentBasis = "magic" }},
		{name: "negative base quantity", mutate: func(input *PriceObservationInput) { input.BaseQuantityValue = -1 }},
		{name: "base quantity scale too high", mutate: func(input *PriceObservationInput) { input.BaseQuantityScale = 19 }},
		{name: "invalid observed at", mutate: func(input *PriceObservationInput) { input.ObservedAt = "2026-06-12 12:00:00" }},
		{name: "metadata is array", mutate: func(input *PriceObservationInput) { input.MetadataJSON = `[]` }},
		{name: "metadata invalid json", mutate: func(input *PriceObservationInput) { input.MetadataJSON = `{` }},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := valid
			test.mutate(&input)
			_, err := cleanPriceObservationSpec(input)
			require.Error(t, err)
		})
	}
}

func TestPricingRefreshStoresDirectRatesIdempotently(t *testing.T) {
	ctx := context.Background()
	database, service := newPricingRefreshTestService(t, []string{"USD", "EUR"}, marketdata.NewRegistry(fakeFXProvider{
		code: "frankfurter",
		rates: map[string]fakeFXRate{
			"USD/EUR": {value: 91234, scale: 5, publishedAt: "2026-06-12T00:00:00Z"},
			"EUR/USD": {value: 109730, scale: 5, publishedAt: "2026-06-12T00:00:00Z"},
		},
	}))

	run, err := service.RunRefresh(ctx, 1, 0, "manual")
	require.NoError(t, err)
	assert.Equal(t, "succeeded", run.Status)
	assert.Equal(t, 2, run.ItemsTotal)

	prices, err := db.NewPricingRepository(database).ListPriceObservations(ctx, BookID, 1, 2, 10)
	require.NoError(t, err)
	require.Len(t, prices, 1)
	assert.Equal(t, int64(91234), prices[0].PriceValue)
	assert.False(t, prices[0].IsDerived)

	_, err = service.RunRefresh(ctx, 1, 0, "manual")
	require.NoError(t, err)
	prices, err = db.NewPricingRepository(database).ListPriceObservations(ctx, BookID, 1, 2, 10)
	require.NoError(t, err)
	assert.Len(t, prices, 1)
}

func TestPricingRefreshStoresDerivedRateWithVintageMetadata(t *testing.T) {
	ctx := context.Background()
	database, service := newPricingRefreshTestService(t, []string{"USD", "EUR", "GBP"}, marketdata.NewRegistry(fakeFXProvider{
		code: "frankfurter",
		rates: map[string]fakeFXRate{
			"USD/GBP": {value: 8000, scale: 4, publishedAt: "2026-06-12T00:00:00Z"},
			"GBP/EUR": {value: 12500, scale: 4, publishedAt: "2026-06-11T00:00:00Z"},
			"EUR/GBP": {value: 8000, scale: 4, publishedAt: "2026-06-11T00:00:00Z"},
			"GBP/USD": {value: 12500, scale: 4, publishedAt: "2026-06-12T00:00:00Z"},
		},
	}))

	run, err := service.RunRefresh(ctx, 1, 0, "manual")
	require.NoError(t, err)
	assert.Equal(t, "succeeded", run.Status)

	prices, err := db.NewPricingRepository(database).ListPriceObservations(ctx, BookID, 1, 2, 10)
	require.NoError(t, err)
	require.NotEmpty(t, prices)
	derived := prices[0]
	assert.True(t, derived.IsDerived)
	assert.Equal(t, int64(1000000000000), derived.PriceValue)
	assert.Equal(t, 12, derived.PriceScale)
	assert.Contains(t, derived.DerivationJSON, `"mixed_vintage":true`)
	assert.Contains(t, derived.DerivationJSON, `"source_published_at":"2026-06-11T00:00:00Z"`)
}

func TestRefreshFXTarget_DerivesMultiHopChainWhenPolicyAllowsMoreHops(t *testing.T) {
	ctx := context.Background()
	// USD -> EUR -> GBP -> JPY is the only chain the provider can serve: no
	// direct USD/JPY, and no single intermediate covers it either.
	database, service := newPricingRefreshTestService(t, []string{"USD", "EUR", "GBP", "JPY"}, marketdata.NewRegistry(fakeFXProvider{
		code: "frankfurter",
		rates: map[string]fakeFXRate{
			"USD/EUR": {value: 9000, scale: 4, publishedAt: "2026-06-12T00:00:00Z"},
			"EUR/GBP": {value: 8000, scale: 4, publishedAt: "2026-06-12T00:00:00Z"},
			"GBP/JPY": {value: 2000000, scale: 4, publishedAt: "2026-06-12T00:00:00Z"},
		},
	}))
	target := fxTargetForTest(t, database, 1, 4)
	now := time.Date(2026, 6, 12, 12, 0, 0, 0, time.UTC)

	_, err := service.refreshFXTarget(ctx, 1, target, policyWithHops(1), 0, "", now)
	require.Error(t, err, "one hop cannot reach JPY; the knob must actually bound the search")

	outcome, err := service.refreshFXTarget(ctx, 1, target, policyWithHops(2), 0, "", now)

	require.NoError(t, err)
	assert.True(t, outcome.Observation.IsDerived)
	// 0.9 USD/EUR * 0.8 EUR/GBP * 200 GBP/JPY = 144 JPY per USD, at scale 12.
	assert.Equal(t, int64(144_000_000_000_000), outcome.Observation.PriceValue)
	assert.Equal(t, fxDerivedPriceScale, outcome.Observation.PriceScale)
	assert.Contains(t, outcome.Observation.DerivationJSON, `"formula":"USD/JPY = (USD/EUR) * (EUR/GBP) * (GBP/JPY)"`)
	assert.Contains(t, outcome.Observation.MetadataJSON, `"via_currency_codes":["EUR","GBP"]`)
	assert.Contains(t, outcome.Observation.DerivationJSON, `"mixed_vintage":false`)

	var legs struct {
		Legs []struct {
			ObservationID int64 `json:"observation_id"`
		} `json:"legs"`
	}
	require.NoError(t, json.Unmarshal([]byte(outcome.Observation.DerivationJSON), &legs))
	assert.Len(t, legs.Legs, 3, "every leg of the chain must be recorded, not just the first two")
}

func TestRefreshFXTarget_PrefersShorterChainOverDeeperOneFoundFirst(t *testing.T) {
	ctx := context.Background()
	// Two routes exist to JPY: USD -> EUR -> GBP -> JPY (two hops) and
	// USD -> ZAR -> JPY (one hop). Candidates are ordered by code, so EUR is
	// tried first and a plain depth-first search would return its two-hop
	// chain. The shorter route must win instead, because every extra leg
	// compounds rounding and widens the vintage spread.
	database, service := newPricingRefreshTestService(t, []string{"USD", "EUR", "GBP", "JPY", "ZAR"}, marketdata.NewRegistry(fakeFXProvider{
		code: "frankfurter",
		rates: map[string]fakeFXRate{
			"USD/EUR": {value: 9000, scale: 4, publishedAt: "2026-06-12T00:00:00Z"},
			"EUR/GBP": {value: 8000, scale: 4, publishedAt: "2026-06-12T00:00:00Z"},
			"GBP/JPY": {value: 2000000, scale: 4, publishedAt: "2026-06-12T00:00:00Z"},
			"USD/ZAR": {value: 9000, scale: 4, publishedAt: "2026-06-12T00:00:00Z"},
			"ZAR/JPY": {value: 1600000, scale: 4, publishedAt: "2026-06-12T00:00:00Z"},
		},
	}))
	target := fxTargetForTest(t, database, 1, 4)

	outcome, err := service.refreshFXTarget(ctx, 1, target, policyWithHops(2), 0, "", time.Date(2026, 6, 12, 12, 0, 0, 0, time.UTC))

	require.NoError(t, err)
	assert.Contains(t, outcome.Observation.DerivationJSON, `"formula":"USD/JPY = (USD/ZAR) * (ZAR/JPY)"`)
	assert.Contains(t, outcome.Observation.MetadataJSON, `"via_currency_codes":["ZAR"]`)
}

func TestRefreshFXTarget_ZeroHopsDisablesTriangulationEntirely(t *testing.T) {
	ctx := context.Background()
	database, service := newPricingRefreshTestService(t, []string{"USD", "EUR", "GBP"}, marketdata.NewRegistry(fakeFXProvider{
		code: "frankfurter",
		rates: map[string]fakeFXRate{
			"USD/EUR": {value: 9000, scale: 4, publishedAt: "2026-06-12T00:00:00Z"},
			"EUR/GBP": {value: 8000, scale: 4, publishedAt: "2026-06-12T00:00:00Z"},
		},
	}))
	target := fxTargetForTest(t, database, 1, 3)

	_, err := service.refreshFXTarget(ctx, 1, target, policyWithHops(0), 0, "", time.Date(2026, 6, 12, 12, 0, 0, 0, time.UTC))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "FX rate USD/GBP is unavailable")
}

func TestPricingRefreshRecordsFailureHealth(t *testing.T) {
	ctx := context.Background()
	database, service := newPricingRefreshTestService(t, []string{"USD", "EUR"}, marketdata.NewRegistry(fakeFXProvider{
		code:  "frankfurter",
		rates: map[string]fakeFXRate{},
	}))

	run, err := service.RunRefresh(ctx, 1, 0, "manual")
	require.NoError(t, err)
	assert.Equal(t, "failed", run.Status)
	assert.Equal(t, 2, run.ItemsFailed)

	health, err := db.NewPricingRepository(database).ListRefreshState(ctx, BookID)
	require.NoError(t, err)
	require.NotEmpty(t, health)
	assert.True(t, health[0].LastError.Valid)
}

func TestListRefreshRunsReportsHasMoreWithoutReturningHiddenRows(t *testing.T) {
	ctx := context.Background()
	database, service := newPricingRefreshTestService(t, []string{"USD"}, marketdata.NewRegistry())
	repository := db.NewPricingRepository(database)

	for i := 0; i < 3; i++ {
		startedAt := time.Date(2026, 6, 12, 12, i, 0, 0, time.UTC).Format(time.RFC3339)
		_, err := repository.RecordRefreshRun(ctx, BookID, 1, "manual", "succeeded", startedAt, startedAt, 0, 0, 0, "")
		require.NoError(t, err)
	}

	runs, err := service.ListRefreshRuns(ctx, 2)

	require.NoError(t, err)
	assert.True(t, runs.HasMore)
	require.Len(t, runs.Runs, 2)
	assert.Equal(t, "2026-06-12T12:02:00Z", runs.Runs[0].StartedAt)
	assert.Equal(t, "2026-06-12T12:01:00Z", runs.Runs[1].StartedAt)
}

func TestFXCoverageBackfillsHistoricalPublicationDatesIdempotently(t *testing.T) {
	ctx := context.Background()
	database, service := newPricingRefreshTestService(t, []string{"USD", "EUR"}, marketdata.NewRegistry(fakeFXProvider{
		code: "frankfurter",
		rates: map[string]fakeFXRate{
			"USD/EUR": {value: 91234, scale: 5},
			"EUR/USD": {value: 109730, scale: 5},
		},
	}))
	_, err := database.ExecContext(ctx, `
		INSERT INTO accounts (id, book_id, created_at, created_by_user_id)
		VALUES (3, 1, '2026-06-09T00:00:00Z', 1);
		INSERT INTO account_versions (
			account_id, version_seq, effective_from, recorded_at, changed_by_user_id,
			change_reason, status, opened_on, name, account_class, account_kind,
			default_commodity_id, allows_postings
		) VALUES (
			3, 1, '2026-06-09', '2026-06-09T00:00:00Z', 1,
			'test historical account', 'active', '2026-06-09', 'Historical EUR cash',
			'asset', 'cash', 2, 1
		)
	`)
	require.NoError(t, err)

	run, hasMore, err := service.runFXCoverage(ctx, 1, 2, "2026-06-09", "domain")
	require.NoError(t, err)
	assert.False(t, hasMore)
	assert.Equal(t, "succeeded", run.Status)
	assert.Equal(t, 8, run.ItemsTotal)

	prices, err := db.NewPricingRepository(database).ListPriceObservations(ctx, BookID, 1, 2, 20)
	require.NoError(t, err)
	require.Len(t, prices, 4)
	assert.Equal(t, "2026-06-12", prices[0].ValuationDate)
	assert.Equal(t, "2026-06-09", prices[3].ValuationDate)

	run, hasMore, err = service.runFXCoverage(ctx, 1, 2, "2026-06-09", "recovery")
	require.NoError(t, err)
	assert.False(t, hasMore)
	assert.Equal(t, 0, run.ItemsTotal)
}

func TestBackgroundWorkExpiredLeaseCanBeReclaimed(t *testing.T) {
	ctx := context.Background()
	database, _ := newPricingRefreshTestService(t, []string{"USD"}, marketdata.NewRegistry())
	repository := db.NewBackgroundWorkRepository(database)
	_, err := repository.EnqueueBackgroundWork(ctx, BookID, "test.work", `{}`, "2026-06-12T00:00:00Z")
	require.NoError(t, err)

	first, err := repository.ClaimBackgroundWork(ctx, "test.work", "worker-one", "2026-06-12T01:00:00Z", "2026-06-12T01:05:00Z")
	require.NoError(t, err)
	assert.Equal(t, 1, first.Attempts)

	_, err = repository.ClaimBackgroundWork(ctx, "test.work", "worker-two", "2026-06-12T01:04:00Z", "2026-06-12T01:09:00Z")
	require.ErrorIs(t, err, db.ErrNotFound)

	reclaimed, err := repository.ClaimBackgroundWork(ctx, "test.work", "worker-two", "2026-06-12T01:06:00Z", "2026-06-12T01:11:00Z")
	require.NoError(t, err)
	assert.Equal(t, first.ID, reclaimed.ID)
	assert.Equal(t, 2, reclaimed.Attempts)
	require.NoError(t, repository.CompleteBackgroundWork(ctx, reclaimed.ID, "worker-two", "2026-06-12T01:07:00Z"))
}

func TestBackgroundWorkEnqueueCoalescesActiveDuplicates(t *testing.T) {
	ctx := context.Background()
	database, _ := newPricingRefreshTestService(t, []string{"USD"}, marketdata.NewRegistry())
	repository := db.NewBackgroundWorkRepository(database)

	first, err := repository.EnqueueBackgroundWork(ctx, BookID, "test.work", `{"id":1}`, "2026-06-12T00:00:00Z")
	require.NoError(t, err)

	duplicate, err := repository.EnqueueBackgroundWork(ctx, BookID, "test.work", `{"id":1}`, "2026-06-12T00:01:00Z")
	require.NoError(t, err)
	assert.Equal(t, first.ID, duplicate.ID)
	assert.Equal(t, first.AvailableAt, duplicate.AvailableAt)

	claimed, err := repository.ClaimBackgroundWork(ctx, "test.work", "worker-one", "2026-06-12T01:00:00Z", "2026-06-12T01:05:00Z")
	require.NoError(t, err)
	assert.Equal(t, first.ID, claimed.ID)
	require.NoError(t, repository.CompleteBackgroundWork(ctx, claimed.ID, "worker-one", "2026-06-12T01:01:00Z"))

	next, err := repository.EnqueueBackgroundWork(ctx, BookID, "test.work", `{"id":1}`, "2026-06-12T02:00:00Z")
	require.NoError(t, err)
	assert.NotEqual(t, first.ID, next.ID)
	assert.Equal(t, "2026-06-12T02:00:00Z", next.AvailableAt)
}

// --- CreatePrice / CreateTradeImpliedPrice ---

func TestCreatePrice_HappyPathAndDuplicateSameDateAppends(t *testing.T) {
	// cleanPriceObservationSpec's own rejections are already covered by
	// TestCleanPriceObservationSpecRejectsInvalidInputs — this exercises the
	// service method itself against the real repository.
	ctx := context.Background()
	_, service := newPricingRefreshTestService(t, []string{"USD", "EUR"}, marketdata.NewRegistry())

	created, err := service.CreatePrice(ctx, PriceObservationInput{
		OwnerUserID: 1, BaseCommodityID: 1, QuoteCommodityID: 2,
		QuoteType: "manual", PriceValue: 91234, PriceScale: 5, ValuationDate: "2026-06-12",
		IsManual: true, MetadataJSON: `{}`,
	})
	require.NoError(t, err)
	assert.Equal(t, int64(91234), created.PriceValue)
	assert.True(t, created.IsManual)

	// A second manual observation for the same pair/date is not rejected or
	// merged — there's no uniqueness constraint on (pair, date) for manual
	// entries (only on (source_id, provider_observation_id), which manual
	// entries never set). Both rows exist; the caller/read model decides
	// which one is authoritative (most recent by recorded_at).
	second, err := service.CreatePrice(ctx, PriceObservationInput{
		OwnerUserID: 1, BaseCommodityID: 1, QuoteCommodityID: 2,
		QuoteType: "manual", PriceValue: 91500, PriceScale: 5, ValuationDate: "2026-06-12",
		MetadataJSON: `{}`,
	})
	require.NoError(t, err)
	assert.NotEqual(t, created.ID, second.ID)

	prices, err := service.ListPrices(ctx, 1, 2, 10)
	require.NoError(t, err)
	require.Len(t, prices, 2)
}

func TestCreatePrice_RequiresOwner(t *testing.T) {
	ctx := context.Background()
	_, service := newPricingRefreshTestService(t, []string{"USD"}, marketdata.NewRegistry())
	_, err := service.CreatePrice(ctx, PriceObservationInput{
		BaseCommodityID: 1, QuoteCommodityID: 1, PriceValue: 100, PriceScale: 2, ValuationDate: "2026-06-12",
	})
	require.Error(t, err)
}

func TestCreateTradeImpliedPrice_RejectsZeroQuantityOrCash(t *testing.T) {
	// The happy path (half-up rounding, correct scale) is already exercised
	// indirectly by every investments Buy/Sell test that wires a real
	// PricingService — this covers the validation edge this file doesn't.
	ctx := context.Background()
	_, service := newPricingRefreshTestService(t, []string{"USD"}, marketdata.NewRegistry())

	err := service.CreateTradeImpliedPrice(ctx, CreateTradeImpliedPriceInput{
		CommodityID: 1, QuoteCommodityID: 1, PriceDate: "2026-06-12",
		QuantityValue: exact.New(0), QuantityScale: 0, CashValue: 1000, CashScale: 2,
	})
	require.Error(t, err)

	err = service.CreateTradeImpliedPrice(ctx, CreateTradeImpliedPriceInput{
		CommodityID: 1, QuoteCommodityID: 1, PriceDate: "2026-06-12",
		QuantityValue: exact.New(10), QuantityScale: 0, CashValue: 0, CashScale: 2,
	})
	require.Error(t, err)
}

// --- SavePolicy / cleanPricingPolicySpec / localDailyTimeUTC ---

func TestSavePolicy_RoundTripsAndComputesUTCFromOwnerTimeZone(t *testing.T) {
	ctx := context.Background()
	database, service := newPricingRefreshTestService(t, []string{"USD"}, marketdata.NewRegistry())
	setOwnerTimeZone(t, database, "Europe/Amsterdam")

	saved, err := service.SavePolicy(ctx, PricingPolicyInput{
		OwnerUserID: 1, RefreshEnabled: true,
		RefreshHourLocal: testIntPtr(9), RefreshMinuteLocal: testIntPtr(0),
		TriangulationMaxHops: 1, RoundingMode: "half_up", WeekendPolicy: "skip",
	})
	require.NoError(t, err)
	// 09:00 Europe/Amsterdam in June (CEST, UTC+2) -> 07:00 UTC.
	assert.Equal(t, 7, saved.RefreshHourUTC)
	assert.Equal(t, 0, saved.RefreshMinuteUTC)
	assert.Equal(t, 9, saved.RefreshHourLocal)

	fetched, err := service.GetPolicy(ctx)
	require.NoError(t, err)
	assert.Equal(t, saved.ID, fetched.ID)
	assert.Equal(t, 7, fetched.RefreshHourUTC)
}

func TestCleanPricingPolicySpec_RejectsInvalidInputs(t *testing.T) {
	valid := PricingPolicyInput{
		RefreshHourUTC: 4, RefreshMinuteUTC: 0,
		TriangulationMaxHops: 1, RoundingMode: "half_up", WeekendPolicy: "skip",
	}
	tests := []struct {
		name   string
		mutate func(*PricingPolicyInput)
	}{
		{"hour UTC out of range", func(i *PricingPolicyInput) { i.RefreshHourUTC = 24 }},
		{"minute UTC out of range", func(i *PricingPolicyInput) { i.RefreshMinuteUTC = 60 }},
		{"hour local out of range", func(i *PricingPolicyInput) { i.RefreshHourLocal = testIntPtr(-1) }},
		{"minute local out of range", func(i *PricingPolicyInput) { i.RefreshMinuteLocal = testIntPtr(60) }},
		{"triangulation hops too high", func(i *PricingPolicyInput) { i.TriangulationMaxHops = 5 }},
		{"triangulation hops negative", func(i *PricingPolicyInput) { i.TriangulationMaxHops = -1 }},
		{"invalid rounding mode", func(i *PricingPolicyInput) { i.RoundingMode = "banker" }},
		{"invalid weekend policy", func(i *PricingPolicyInput) { i.WeekendPolicy = "ignore" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := valid
			test.mutate(&input)
			_, err := cleanPricingPolicySpec(input)
			require.Error(t, err)
		})
	}
}

// TestLocalDailyTimeUTC_TracksDSTTransition is the classic silent-drift bug
// in daily schedulers: a naive implementation caches a fixed UTC offset
// computed once, which goes wrong the moment the clock changes. This
// function instead recomputes the local->UTC offset from `now`'s date every
// call, so the same 09:00 Europe/Amsterdam wall-clock time correctly maps
// to different UTC hours either side of the 2026-03-29 spring-forward.
func TestLocalDailyTimeUTC_TracksDSTTransition(t *testing.T) {
	winter := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC) // CET, UTC+1
	summer := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC) // CEST, UTC+2

	winterHour, winterMinute, err := localDailyTimeUTC(9, 0, "Europe/Amsterdam", winter)
	require.NoError(t, err)
	assert.Equal(t, 8, winterHour)
	assert.Equal(t, 0, winterMinute)

	summerHour, summerMinute, err := localDailyTimeUTC(9, 0, "Europe/Amsterdam", summer)
	require.NoError(t, err)
	assert.Equal(t, 7, summerHour)
	assert.Equal(t, 0, summerMinute)

	assert.NotEqual(t, winterHour, summerHour, "a fixed-offset implementation would wrongly return the same UTC hour year-round")
}

func TestLocalDailyTimeUTC_RejectsInvalidTimeZone(t *testing.T) {
	_, _, err := localDailyTimeUTC(9, 0, "Not/A_Zone", time.Now())
	require.Error(t, err)
}

// --- SaveSourceAssignment ---

func TestSaveSourceAssignment_MultipleActiveAssignmentsResolveByPriority(t *testing.T) {
	ctx := context.Background()
	database, service := newPricingRefreshTestService(t, []string{"USD", "EUR"}, marketdata.NewRegistry())
	repository := db.NewPricingRepository(database)
	frankfurterID := marketDataSourceIDByCode(t, database, "frankfurter")
	ecbID := marketDataSourceIDByCode(t, database, "ecb_euro_reference_rates")

	low, err := service.SaveSourceAssignment(ctx, PricingSourceAssignmentInput{
		OwnerUserID: 1, CommodityID: 1, QuoteCommodityID: 2, SourceID: frankfurterID,
		Priority: 100, Status: "active", EffectiveFrom: "2026-01-01",
	})
	require.NoError(t, err)

	high, err := service.SaveSourceAssignment(ctx, PricingSourceAssignmentInput{
		OwnerUserID: 1, CommodityID: 1, QuoteCommodityID: 2, SourceID: ecbID,
		Priority: 10, Status: "active", EffectiveFrom: "2026-01-01",
	})
	require.NoError(t, err)
	assert.NotEqual(t, low.ID, high.ID, "a second assignment for the same pair is not auto-archived")

	assignments, err := service.ListSourceAssignments(ctx)
	require.NoError(t, err)
	assert.Len(t, assignments, 2)

	winner, err := repository.SourceAssignmentForPair(ctx, BookID, 1, 2, "2026-06-01")
	require.NoError(t, err)
	assert.Equal(t, ecbID, winner.SourceID, "lower priority number wins")

	updated, err := service.SaveSourceAssignment(ctx, PricingSourceAssignmentInput{
		OwnerUserID: 1, AssignmentID: high.ID, CommodityID: 1, QuoteCommodityID: 2, SourceID: ecbID,
		Priority: 10, Status: "archived", EffectiveFrom: "2026-01-01",
	})
	require.NoError(t, err)
	assert.Equal(t, "archived", updated.Status)

	winnerAfterArchive, err := repository.SourceAssignmentForPair(ctx, BookID, 1, 2, "2026-06-01")
	require.NoError(t, err)
	assert.Equal(t, frankfurterID, winnerAfterArchive.SourceID)
}

func TestCleanPricingSourceAssignmentSpec_RejectsInvalidInputs(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	valid := PricingSourceAssignmentInput{CommodityID: 1, QuoteCommodityID: 2, SourceID: 3, Status: "active", EffectiveFrom: "2026-01-01"}
	tests := []struct {
		name   string
		mutate func(*PricingSourceAssignmentInput)
	}{
		{"missing commodity", func(i *PricingSourceAssignmentInput) { i.CommodityID = 0 }},
		{"missing source", func(i *PricingSourceAssignmentInput) { i.SourceID = 0 }},
		{"negative priority", func(i *PricingSourceAssignmentInput) { i.Priority = -1 }},
		{"invalid status", func(i *PricingSourceAssignmentInput) { i.Status = "deleted" }},
		{"effective_to before effective_from", func(i *PricingSourceAssignmentInput) { i.EffectiveTo = "2025-01-01" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := valid
			test.mutate(&input)
			_, err := cleanPricingSourceAssignmentSpec(input, now)
			require.Error(t, err)
		})
	}
}

// --- Scheduler ---

func TestRunScheduledRefreshIfDue(t *testing.T) {
	t.Run("not yet due", func(t *testing.T) {
		ctx := context.Background()
		database, service := newPricingRefreshTestService(t, []string{"USD"}, marketdata.NewRegistry())
		service.SetNowForTest(func() time.Time { return time.Date(2026, 6, 12, 3, 0, 0, 0, time.UTC) }) // default policy fires at 04:00 UTC
		service.runScheduledRefreshIfDue(ctx, testLogger())
		assert.Equal(t, 0, countScheduledRuns(t, database))
	})

	t.Run("due enqueues exactly one run", func(t *testing.T) {
		ctx := context.Background()
		database, service := newPricingRefreshTestService(t, []string{"USD"}, marketdata.NewRegistry())
		service.SetNowForTest(func() time.Time { return time.Date(2026, 6, 12, 4, 5, 0, 0, time.UTC) })
		service.runScheduledRefreshIfDue(ctx, testLogger())
		assert.Equal(t, 1, countScheduledRuns(t, database))
	})

	t.Run("still due but already ran today runs no second time", func(t *testing.T) {
		ctx := context.Background()
		database, service := newPricingRefreshTestService(t, []string{"USD"}, marketdata.NewRegistry())
		service.SetNowForTest(func() time.Time { return time.Date(2026, 6, 12, 4, 5, 0, 0, time.UTC) })
		service.runScheduledRefreshIfDue(ctx, testLogger())
		require.Equal(t, 1, countScheduledRuns(t, database))

		service.SetNowForTest(func() time.Time { return time.Date(2026, 6, 12, 4, 30, 0, 0, time.UTC) })
		service.runScheduledRefreshIfDue(ctx, testLogger())
		assert.Equal(t, 1, countScheduledRuns(t, database), "a second tick the same local day must not enqueue a second scheduled run")
	})

	t.Run("disabled policy runs nothing", func(t *testing.T) {
		ctx := context.Background()
		database, service := newPricingRefreshTestService(t, []string{"USD"}, marketdata.NewRegistry())
		_, err := service.SavePolicy(ctx, PricingPolicyInput{
			OwnerUserID: 1, RefreshEnabled: false, RefreshHourUTC: 4, RefreshMinuteUTC: 0,
			TriangulationMaxHops: 1, RoundingMode: "half_up", WeekendPolicy: "skip",
		})
		require.NoError(t, err)
		service.SetNowForTest(func() time.Time { return time.Date(2026, 6, 12, 4, 5, 0, 0, time.UTC) })
		service.runScheduledRefreshIfDue(ctx, testLogger())
		assert.Equal(t, 0, countScheduledRuns(t, database))
	})
}

// --- Worker ---

func TestRunDueFXCoverage_ProcessesLeasedWorkExactlyOnce(t *testing.T) {
	ctx := context.Background()
	database, service := newPricingRefreshTestService(t, []string{"USD"}, marketdata.NewRegistry())
	repository := db.NewBackgroundWorkRepository(database)

	// Fixture setup creates a currency-holding account, which fires a real
	// DB trigger (migrations/0001, "currency_activated") that auto-enqueues
	// its own fxCoverageWorkKind item. Drain that first so the assertion
	// below is about the item this test itself enqueues, not a baseline
	// artifact of the fixture.
	service.runDueFXCoverage(ctx, testLogger(), "drain-worker")
	require.Equal(t, 0, pendingFXCoverageWorkCount(t, database))

	_, err := repository.EnqueueBackgroundWork(ctx, BookID, fxCoverageWorkKind, `{"reason":"manual"}`, "2026-06-12T00:00:00Z")
	require.NoError(t, err)

	service.runDueFXCoverage(ctx, testLogger(), "worker-one")

	var completed int
	require.NoError(t, database.QueryRowContext(ctx, `SELECT COUNT(*) FROM background_work_items WHERE kind = ? AND status = 'completed'`, fxCoverageWorkKind).Scan(&completed))
	assert.Equal(t, 2, completed, "the drained trigger item plus this test's own item")
	assert.Equal(t, 0, pendingFXCoverageWorkCount(t, database), "no item should be left pending or re-queued")
}

// TestProcessFXCoverageWork_InvalidPayloadFailsWithoutRetry uses a
// syntactically valid JSON payload with a semantically bad field
// (start_date isn't a real date) — background_work_items.payload_json has a
// DB-level json_valid() CHECK constraint, so malformed JSON syntax can never
// reach this far; the only reachable "invalid payload" is one that parses
// but fails the service's own semantic validation.
func TestProcessFXCoverageWork_InvalidPayloadFailsWithoutRetry(t *testing.T) {
	ctx := context.Background()
	database, service := newPricingRefreshTestService(t, []string{"USD"}, marketdata.NewRegistry())
	repository := db.NewBackgroundWorkRepository(database)

	// Drain the fixture's own trigger-seeded item first so ClaimBackgroundWork
	// (oldest-first) deterministically picks up this test's own item, not the
	// one the "currency_activated" DB trigger enqueued during setup.
	service.runDueFXCoverage(ctx, testLogger(), "drain-worker")
	require.Equal(t, 0, pendingFXCoverageWorkCount(t, database))

	_, err := repository.EnqueueBackgroundWork(ctx, BookID, fxCoverageWorkKind, `{"start_date":"not-a-date"}`, "2026-06-12T00:00:00Z")
	require.NoError(t, err)

	item, err := repository.ClaimBackgroundWork(ctx, fxCoverageWorkKind, "worker-one", "2026-06-12T00:01:00Z", "2026-06-12T00:06:00Z")
	require.NoError(t, err)

	service.processFXCoverageWork(ctx, testLogger(), "worker-one", item)

	var status, lastError string
	require.NoError(t, database.QueryRowContext(ctx, `SELECT status, last_error FROM background_work_items WHERE id = ?`, item.ID).Scan(&status, &lastError))
	assert.Equal(t, "failed", status)
	assert.Contains(t, lastError, "invalid FX coverage start date")
}

func TestProcessFXCoverageWork_ProviderFailureRetriesWithBackoff(t *testing.T) {
	ctx := context.Background()
	// No default currency configured for book 1's pricing policy resolution
	// path here is fine — the fixture always sets one; instead force a
	// failure via a currency ID that doesn't resolve to any FX target so
	// s.runFXCoverage's own dependency (DefaultCurrency) still succeeds but
	// CurrentBookOwnerID fails: point the work at a book with no owner.
	database, service := newPricingRefreshTestService(t, []string{"USD"}, marketdata.NewRegistry())
	repository := db.NewBackgroundWorkRepository(database)

	// Drain the fixture's own trigger-seeded item first (while the default
	// currency is still configured, so draining itself succeeds) — see the
	// comment in TestProcessFXCoverageWork_InvalidPayloadFailsWithoutRetry.
	service.runDueFXCoverage(ctx, testLogger(), "drain-worker")
	require.Equal(t, 0, pendingFXCoverageWorkCount(t, database))

	_, err := database.ExecContext(ctx, `UPDATE books SET default_currency_commodity_id = NULL WHERE id = 1`)
	require.NoError(t, err)
	_, err = repository.EnqueueBackgroundWork(ctx, BookID, fxCoverageWorkKind, `{"reason":"manual"}`, "2026-06-12T00:00:00Z")
	require.NoError(t, err)

	item, err := repository.ClaimBackgroundWork(ctx, fxCoverageWorkKind, "worker-one", "2026-06-12T00:01:00Z", "2026-06-12T00:06:00Z")
	require.NoError(t, err)

	service.processFXCoverageWork(ctx, testLogger(), "worker-one", item)

	var status string
	var attempts int
	var availableAt string
	require.NoError(t, database.QueryRowContext(ctx, `SELECT status, attempts, available_at FROM background_work_items WHERE id = ?`, item.ID).Scan(&status, &attempts, &availableAt))
	assert.Equal(t, "pending", status, "a retryable failure releases the lease for another attempt, it does not fail permanently")
	assert.Equal(t, 1, attempts)
	assert.Greater(t, availableAt, "2026-06-12T00:06:00Z", "retry must be scheduled after the backoff delay, not immediately")
}

func TestProcessFXCoverageWork_GivesUpAfterMaxAttempts(t *testing.T) {
	ctx := context.Background()
	database, service := newPricingRefreshTestService(t, []string{"USD"}, marketdata.NewRegistry())
	repository := db.NewBackgroundWorkRepository(database)

	// Drain the fixture's own trigger-seeded item first — see the comment in
	// TestProcessFXCoverageWork_InvalidPayloadFailsWithoutRetry.
	service.runDueFXCoverage(ctx, testLogger(), "drain-worker")
	require.Equal(t, 0, pendingFXCoverageWorkCount(t, database))

	// Same permanently-failing setup as the backoff test: no default currency
	// means every attempt fails for a reason retrying cannot fix (T-39).
	_, err := database.ExecContext(ctx, `UPDATE books SET default_currency_commodity_id = NULL WHERE id = 1`)
	require.NoError(t, err)
	_, err = repository.EnqueueBackgroundWork(ctx, BookID, fxCoverageWorkKind, `{"reason":"manual"}`, "2026-06-12T00:00:00Z")
	require.NoError(t, err)

	// One attempt short of the cap: this attempt still retries.
	_, err = database.ExecContext(ctx, `UPDATE background_work_items SET attempts = ? WHERE kind = ?`, maxFXCoverageAttempts-2, fxCoverageWorkKind)
	require.NoError(t, err)

	item, err := repository.ClaimBackgroundWork(ctx, fxCoverageWorkKind, "worker-one", "2026-06-12T00:01:00Z", "2026-06-12T00:06:00Z")
	require.NoError(t, err)
	require.Equal(t, maxFXCoverageAttempts-1, item.Attempts)
	service.processFXCoverageWork(ctx, testLogger(), "worker-one", item)
	assert.Equal(t, "pending", fxCoverageWorkStatus(t, database, item.ID), "below the cap the item still retries")

	// The next attempt hits the cap and gives up permanently.
	item, err = repository.ClaimBackgroundWork(ctx, fxCoverageWorkKind, "worker-one", "2026-07-12T00:01:00Z", "2026-07-12T00:06:00Z")
	require.NoError(t, err)
	require.Equal(t, maxFXCoverageAttempts, item.Attempts)
	service.processFXCoverageWork(ctx, testLogger(), "worker-one", item)
	assert.Equal(t, "failed", fxCoverageWorkStatus(t, database, item.ID), "at the cap the item stops retrying forever")

	failed, err := service.FailedBackgroundWork(ctx)
	require.NoError(t, err)
	require.Len(t, failed, 1, "the given-up item surfaces in pricing source health")
	assert.Equal(t, item.ID, failed[0].ID)
	assert.Equal(t, maxFXCoverageAttempts, failed[0].Attempts)
	assert.NotEmpty(t, failed[0].LastError)
}

func TestRetryBackgroundWork_RequeuesFailedItemAndResetsAttempts(t *testing.T) {
	ctx := context.Background()
	database, service := newPricingRefreshTestService(t, []string{"USD"}, marketdata.NewRegistry())
	repository := db.NewBackgroundWorkRepository(database)

	service.runDueFXCoverage(ctx, testLogger(), "drain-worker")
	require.Equal(t, 0, pendingFXCoverageWorkCount(t, database))

	created, err := repository.EnqueueBackgroundWork(ctx, BookID, fxCoverageWorkKind, `{"reason":"manual"}`, "2026-06-12T00:00:00Z")
	require.NoError(t, err)
	item, err := repository.ClaimBackgroundWork(ctx, fxCoverageWorkKind, "worker-one", "2026-06-12T00:01:00Z", "2026-06-12T00:06:00Z")
	require.NoError(t, err)
	require.NoError(t, repository.FailBackgroundWork(ctx, item.ID, "worker-one", "2026-06-12T00:02:00Z", "provider unreachable"))

	requeued, err := service.RetryBackgroundWork(ctx, created.ID)

	require.NoError(t, err)
	assert.Equal(t, 0, requeued.Attempts, "backoff restarts from the bottom, not at the 6h cap")
	assert.Equal(t, "pending", fxCoverageWorkStatus(t, database, created.ID))

	// It is genuinely claimable again, not just marked pending.
	claimed, err := repository.ClaimBackgroundWork(ctx, fxCoverageWorkKind, "worker-two", "2026-06-13T00:00:00Z", "2026-06-13T00:05:00Z")
	require.NoError(t, err)
	assert.Equal(t, created.ID, claimed.ID)
}

func TestRetryBackgroundWork_RejectsMissingAndAlreadyQueuedWork(t *testing.T) {
	ctx := context.Background()
	database, service := newPricingRefreshTestService(t, []string{"USD"}, marketdata.NewRegistry())
	repository := db.NewBackgroundWorkRepository(database)

	service.runDueFXCoverage(ctx, testLogger(), "drain-worker")
	require.Equal(t, 0, pendingFXCoverageWorkCount(t, database))

	_, err := service.RetryBackgroundWork(ctx, 987654)
	assert.ErrorIs(t, err, ErrPricingBackgroundWorkNotFound)

	pending, err := repository.EnqueueBackgroundWork(ctx, BookID, fxCoverageWorkKind, `{"reason":"manual"}`, "2026-06-12T00:00:00Z")
	require.NoError(t, err)
	_, err = service.RetryBackgroundWork(ctx, pending.ID)
	assert.ErrorIs(t, err, ErrPricingBackgroundWorkNotFound, "only failed work is re-enqueueable")

	// A failed item whose payload matches still-active work must not be
	// revived: the active-unique index allows only one live copy.
	_, err = database.ExecContext(ctx, `
		INSERT INTO background_work_items (book_id, kind, payload_json, status, attempts, available_at, last_error, created_at, updated_at)
		VALUES (?, ?, ?, 'failed', ?, '2026-06-12T00:00:00Z', 'boom', '2026-06-12T00:00:00Z', '2026-06-12T00:00:00Z')
	`, BookID, fxCoverageWorkKind, `{"reason":"manual"}`, maxFXCoverageAttempts)
	require.NoError(t, err)
	var failedID int64
	require.NoError(t, database.QueryRowContext(ctx, `SELECT id FROM background_work_items WHERE status = 'failed'`).Scan(&failedID))

	_, err = service.RetryBackgroundWork(ctx, failedID)
	assert.ErrorIs(t, err, ErrPricingBackgroundWorkActive)
}

func TestRunDueFXCoverage_PoisonItemDoesNotWedgeQueue(t *testing.T) {
	ctx := context.Background()
	database, service := newPricingRefreshTestService(t, []string{"USD"}, marketdata.NewRegistry())
	repository := db.NewBackgroundWorkRepository(database)

	// Drain the fixture's own trigger-seeded item first — see the comment in
	// TestRunDueFXCoverage_ProcessesLeasedWorkExactlyOnce.
	service.runDueFXCoverage(ctx, testLogger(), "drain-worker")
	require.Equal(t, 0, pendingFXCoverageWorkCount(t, database))

	baselineCompleted := countFXCoverageWorkByStatus(t, database, "completed")

	_, err := repository.EnqueueBackgroundWork(ctx, BookID, fxCoverageWorkKind, `{"start_date":"not-a-date"}`, "2026-06-12T00:00:00Z")
	require.NoError(t, err)
	_, err = repository.EnqueueBackgroundWork(ctx, BookID, fxCoverageWorkKind, `{"reason":"manual","currency_id":999}`, "2026-06-12T00:00:01Z")
	require.NoError(t, err)

	service.runDueFXCoverage(ctx, testLogger(), "worker-one")

	failed := countFXCoverageWorkByStatus(t, database, "failed")
	completed := countFXCoverageWorkByStatus(t, database, "completed")
	assert.Equal(t, 1, failed, "the poison (malformed) item fails immediately")
	assert.Equal(t, 1, completed-baselineCompleted, "the second, valid item still gets processed in the same worker pass")
}

// --- test helpers ---

func policyWithHops(hops int) PricingPolicy {
	return PricingPolicy{BookID: BookID, TriangulationMaxHops: hops, RoundingMode: "half_up", WeekendPolicy: "skip"}
}

func fxTargetForTest(t *testing.T, database *sql.DB, baseCommodityID int64, quoteCommodityID int64) fxRefreshTarget {
	t.Helper()
	repository := db.NewPricingRepository(database)
	base, err := repository.CurrencyByID(context.Background(), BookID, baseCommodityID)
	require.NoError(t, err)
	quote, err := repository.CurrencyByID(context.Background(), BookID, quoteCommodityID)
	require.NoError(t, err)
	return fxRefreshTarget{Base: base, Quote: quote}
}

func fxCoverageWorkStatus(t *testing.T, database *sql.DB, id int64) string {
	t.Helper()
	var status string
	require.NoError(t, database.QueryRowContext(context.Background(), `SELECT status FROM background_work_items WHERE id = ?`, id).Scan(&status))
	return status
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func testIntPtr(v int) *int {
	return &v
}

func setOwnerTimeZone(t *testing.T, database *sql.DB, timeZone string) {
	t.Helper()
	_, err := database.ExecContext(context.Background(), `
		INSERT INTO user_preferences (user_id, time_zone, created_at, created_by_user_id, updated_at, updated_by_user_id)
		VALUES (1, ?, '2026-01-01T00:00:00Z', 1, '2026-01-01T00:00:00Z', 1)
	`, timeZone)
	require.NoError(t, err)
}

func marketDataSourceIDByCode(t *testing.T, database *sql.DB, code string) int64 {
	t.Helper()
	var id int64
	require.NoError(t, database.QueryRowContext(context.Background(), `SELECT id FROM market_data_sources WHERE code = ?`, code).Scan(&id))
	return id
}

func countScheduledRuns(t *testing.T, database *sql.DB) int {
	t.Helper()
	var count int
	require.NoError(t, database.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM market_data_ingest_runs WHERE trigger = 'scheduled'`).Scan(&count))
	return count
}

func pendingFXCoverageWorkCount(t *testing.T, database *sql.DB) int {
	t.Helper()
	var count int
	require.NoError(t, database.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM background_work_items WHERE kind = ? AND status IN ('pending', 'running')`, fxCoverageWorkKind).Scan(&count))
	return count
}

func countFXCoverageWorkByStatus(t *testing.T, database *sql.DB, status string) int {
	t.Helper()
	var count int
	require.NoError(t, database.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM background_work_items WHERE kind = ? AND status = ?`, fxCoverageWorkKind, status).Scan(&count))
	return count
}

type fakeFXRate struct {
	value       int64
	scale       int
	publishedAt string
}

type fakeFXProvider struct {
	code  string
	rates map[string]fakeFXRate
}

func (p fakeFXProvider) Code() string {
	return p.code
}

func (p fakeFXProvider) Name() string {
	return p.code
}

func (p fakeFXProvider) Capabilities() marketdata.ProviderCapabilities {
	return marketdata.ProviderCapabilities{FXRates: true, HistoricalFXRates: true}
}

func (p fakeFXProvider) FetchFXRates(_ context.Context, request marketdata.FXRateRequest) ([]marketdata.FXRateObservation, error) {
	base := request.BaseCode
	var observations []marketdata.FXRateObservation
	for _, quote := range request.QuoteCodes {
		rate, ok := p.rates[base+"/"+quote]
		if !ok {
			continue
		}
		publishedAt := rate.publishedAt
		if request.Date != "" {
			publishedAt = request.Date + "T00:00:00Z"
		} else if publishedAt == "" {
			publishedAt = "2026-06-12T00:00:00Z"
		}
		observations = append(observations, marketdata.FXRateObservation{
			SourceCode:            p.code,
			BaseCode:              base,
			QuoteCode:             quote,
			QuoteType:             "official_fixing",
			AdjustmentBasis:       "not_applicable",
			PriceValue:            rate.value,
			PriceScale:            rate.scale,
			BaseQuantityValue:     1,
			BaseQuantityScale:     0,
			ValuationDate:         publishedAt[:10],
			SourcePublishedAt:     publishedAt,
			ProviderSeriesID:      p.code + ":" + base + ":" + quote,
			ProviderObservationID: p.code + ":" + publishedAt[:10] + ":" + base + ":" + quote,
			MetadataJSON:          `{"provider":"fake"}`,
			RawJSON:               "{}",
		})
	}
	return observations, nil
}

func newPricingRefreshTestService(t *testing.T, currencyCodes []string, registry *marketdata.Registry) (*sql.DB, *PricingService) {
	t.Helper()
	ctx := context.Background()
	database, err := db.Open(ctx, "file:"+filepath.Join(t.TempDir(), "pricing.sqlite"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, database.Close()) })
	require.NoError(t, db.Migrate(ctx, database))

	_, err = database.ExecContext(ctx, `
		INSERT INTO users (id, username, password_hash, is_owner, created_at, updated_at)
		VALUES (1, 'owner', 'hash', 1, '2026-06-12T00:00:00Z', '2026-06-12T00:00:00Z');
		INSERT INTO books (id, owner_user_id, code, name, default_currency_commodity_id, updated_by_user_id, created_at, updated_at)
		VALUES (1, 1, 'personal', 'Personal', NULL, 1, '2026-06-12T00:00:00Z', '2026-06-12T00:00:00Z');
	`)
	require.NoError(t, err)

	for index, code := range currencyCodes {
		id := int64(index + 1)
		_, err = database.ExecContext(ctx, `
			INSERT INTO commodities (id, book_id, code, kind, is_builtin, created_at, created_by_user_id)
			VALUES (?, 1, ?, 'currency', 1, '2026-06-12T00:00:00Z', 1);
			INSERT INTO commodity_versions (
				commodity_id, version_seq, effective_from, recorded_at, changed_by_user_id,
				change_reason, status, symbol, display_symbol, name, standard_scale, max_quantity_scale
			)
			VALUES (?, 1, '2026-06-12', '2026-06-12T00:00:00Z', 1, 'test currency', 'active', ?, ?, ?, 2, 8);
			INSERT INTO accounts (id, book_id, created_at, created_by_user_id)
			VALUES (?, 1, '2026-06-12T00:00:00Z', 1);
			INSERT INTO account_versions (
				account_id, version_seq, effective_from, recorded_at, changed_by_user_id,
				change_reason, status, opened_on, name, account_class, account_kind,
				default_commodity_id, allows_postings
			)
			VALUES (?, 1, '2026-06-12', '2026-06-12T00:00:00Z', 1, 'test account', 'active', '2026-06-12', ?, 'asset', 'cash', ?, 1);
		`, id, code, id, code, code, code, id, id, code+" cash", id)
		require.NoError(t, err)
	}
	_, err = database.ExecContext(ctx, `UPDATE books SET default_currency_commodity_id = 1 WHERE id = 1`)
	require.NoError(t, err)

	service := NewPricingService(db.NewPricingRepository(database), registry)
	service.SetNowForTest(func() time.Time {
		return time.Date(2026, 6, 12, 12, 0, 0, 0, time.UTC)
	})
	return database, service
}
