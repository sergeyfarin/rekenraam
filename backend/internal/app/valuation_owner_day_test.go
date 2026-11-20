package app

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rekenraam/backend/internal/db"
	"rekenraam/backend/internal/exact"
)

func TestRateTableUsesThePinnedOwnerDay(t *testing.T) {
	t.Parallel()
	f := newPostedCurrencyFixture(t)
	ctx := context.Background()
	settings := NewSettingsService(db.NewSettingsRepository(f.database))
	f.transactionService.SetSettings(settings)

	const nextDay = "2026-10-16"
	afterMidnight := time.Date(2026, 10, 16, 0, 0, 1, 0, time.UTC)
	price := ManualPriceInput{
		OwnerUserID: f.ownerUserID, BaseCommodityID: f.foreign, QuoteCommodityID: f.eurCommodityID,
		ValuationDate: pinnedTestDay, PriceValue: "125", PriceScale: 2, BaseQuantityValue: "1", Reason: "Synthetic FX assertion",
	}
	_, err := f.pricing.CreateManualPrice(ctx, price)
	require.NoError(t, err)
	price.ValuationDate, price.PriceValue = nextDay, "150"
	_, err = f.pricing.CreateManualPrice(ctx, price)
	require.NoError(t, err)

	check := func(ctx context.Context, wantDate, wantAmount string) {
		t.Helper()
		rates, err := f.transactionService.NewRateTable(ctx, ReportingCurrencyInput{CommodityID: f.eurCommodityID}, []int64{f.foreign}, pinnedTestDay, nextDay)
		require.NoError(t, err)
		// The requested range includes the 16th, so the reporting window
		// alone cannot exclude the quote from that day.
		amount, gotDate, ok := rates.ConvertExactObserved(exact.New(100), 0, f.foreign, nextDay)
		require.True(t, ok)
		assert.Equal(t, wantAmount, amount.RatString())
		assert.Equal(t, wantDate, gotDate)
		assert.Equal(t, wantDate, rates.Coverage("EUR").ValuationAsOf)
		assert.Equal(t, afterMidnight.Format(time.RFC3339Nano), rates.Coverage("EUR").KnowledgeCutoff)
	}

	crossOwnerMidnight(f.transactionService.SetNowForTest)
	check(ctx, pinnedTestDay, "125")

	f.transactionService.SetNowForTest(func() time.Time { return afterMidnight })
	historicalDate := "2026-10-14"
	check(db.WithReferenceDate(ctx, historicalDate), nextDay, "150")

	pinned, err := settings.referenceContext(ctx, afterMidnight.Add(-2*time.Second))
	require.NoError(t, err)
	check(db.WithReferenceDate(pinned, historicalDate), pinnedTestDay, "125")
}

func TestPricingWorkspaceUsesThePinnedOwnerDay(t *testing.T) {
	t.Parallel()
	f := newPostedCurrencyFixture(t)
	f.pricing.SetSettings(NewSettingsService(db.NewSettingsRepository(f.database)))
	crossOwnerMidnight(f.pricing.SetNowForTest)

	workspace, err := f.pricing.Workspace(context.Background(), "")
	require.NoError(t, err)
	assert.Equal(t, pinnedTestDay, workspace.AsOf)

	crossOwnerMidnight(f.pricing.SetNowForTest)
	workspace, err = f.pricing.Workspace(context.Background(), "2026-10-16")
	require.NoError(t, err)
	assert.Equal(t, "2026-10-16", workspace.AsOf, "an explicit future diagnostic date is allowed")
}
