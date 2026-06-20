package app

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"rekenraam/backend/internal/db"
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
	repository := db.NewPricingRepository(database)
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
