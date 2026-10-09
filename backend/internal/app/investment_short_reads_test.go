package app

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rekenraam/backend/internal/db"
	"rekenraam/backend/internal/exact"
)

// #174: positions, lots, unrealized gains and net worth read the side. A short
// is a separate row with positive owed quantity, its value is a negative
// exposure, and its open result is the opening proceeds still held less the
// current cost to cover. A missing price stays unavailable, never zero.

func priceTestStockAt(t *testing.T, f *investmentsTestFixture, date string, value int64) {
	t.Helper()
	_, err := NewPricingService(db.NewPricingRepository(f.database)).CreatePrice(context.Background(), PriceObservationInput{
		OwnerUserID: f.ownerUserID, BaseCommodityID: f.stockCommodityID, QuoteCommodityID: f.eurCommodityID,
		QuoteType: "close", PriceValue: value, PriceScale: 2, ValuationDate: date, ObservedAt: date + "T18:00:00Z"})
	require.NoError(t, err)
}

func TestShortPositionReadsCarrySideAndSignedExposure(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	opening := shortSaleOn(t, f, "2026-03-02", 10, 10000)
	priceTestStockAt(t, f, "2026-03-03", 700)

	positions, err := f.investmentService.Positions(ctx)
	require.NoError(t, err)
	require.Len(t, positions, 1)
	assert.Equal(t, "short", positions[0].PositionSide)
	assert.Equal(t, "10", positions[0].QuantityValue.String(), "owed units stay positive")
	assertMoneyValue(t, 10000, 2, positions[0].RemainingCostBasisValue, positions[0].RemainingCostBasisScale, "opening proceeds held")

	lots, err := f.investmentService.ListLots(ctx, f.holdingAccountID, f.stockCommodityID)
	require.NoError(t, err)
	require.Len(t, lots, 1)
	assert.Equal(t, *opening.LotID, lots[0].ID)
	assert.Equal(t, "short", lots[0].PositionSide)

	unrealized, err := f.investmentService.ListUnrealizedGains(ctx)
	require.NoError(t, err)
	require.Len(t, unrealized, 1)
	entry := unrealized[0]
	assert.Equal(t, "short", entry.PositionSide)
	require.NotNil(t, entry.MarketValueValue)
	require.NotNil(t, entry.UnrealizedGainValue)
	assertMoneyValue(t, -7000, 2, *entry.MarketValueValue, *entry.MarketValueScale, "ten owed units at 7.00")
	assertMoneyValue(t, 3000, 2, *entry.UnrealizedGainValue, *entry.UnrealizedGainScale, "100.00 held − 70.00 to cover")

	// Net worth is journal-based: −10 shares at 7.00 beside 100.00 of cash.
	f.transactionService.SetPricingRepository(db.NewPricingRepository(f.database))
	series, err := f.transactionService.NetWorthSeries(ctx, NetWorthSeriesInput{
		StartDate: "2026-03-01", EndDate: "2026-03-31", Bucket: "month",
		Reporting: &ReportingCurrencyInput{CommodityID: f.eurCommodityID, MaxStalenessDay: 30}})
	require.NoError(t, err)
	require.Len(t, series.Buckets, 1)
	assert.Empty(t, series.Buckets[0].UnclassifiedShorts, "a named short does not withhold the total")
	require.NotNil(t, series.Buckets[0].Converted)
	assert.Zero(t, exact.ScaledIntFromCoefficient(series.Buckets[0].Converted.QuantityValue, series.Buckets[0].Converted.QuantityScale).
		Cmp(exact.ScaledIntFromInt64(3000, 2)), "converted net worth, got %s", series.Buckets[0].Converted.QuantityValue.String())
}

func TestShortPositionWithoutPriceIsUnavailableNotZero(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixtureWithOptions(t, false)
	shortSaleOn(t, f, "2026-03-02", 10, 10000)
	unrealized, err := f.investmentService.ListUnrealizedGains(context.Background())
	require.NoError(t, err)
	require.Len(t, unrealized, 1)
	assert.Equal(t, "short", unrealized[0].PositionSide)
	assert.Equal(t, db.ValuationNoPrice, unrealized[0].ValuationUnavailable)
	assert.Nil(t, unrealized[0].MarketValueValue)
	assert.Nil(t, unrealized[0].UnrealizedGainValue)
}

func TestLongAndShortHoldingsOfOneInstrumentStaySeparateRows(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	otherHolding := seedTestAccountWithClass(t, f.database, "active", true, "asset", "security_holding")
	buy := shortInput(f, "2026-03-02", 5, 5000)
	buy.HoldingAccountID = otherHolding
	_, err := f.investmentService.Buy(ctx, buy)
	require.NoError(t, err)
	shortSaleOn(t, f, "2026-03-02", 2, 2000)
	priceTestStockAt(t, f, "2026-03-03", 1200)

	positions, err := f.investmentService.Positions(ctx)
	require.NoError(t, err)
	sides := map[int64]string{}
	for _, position := range positions {
		sides[position.AccountID] = position.PositionSide
	}
	assert.Equal(t, map[int64]string{otherHolding: "long", f.holdingAccountID: "short"}, sides, "no netting across holdings")

	unrealized, err := f.investmentService.ListUnrealizedGains(ctx)
	require.NoError(t, err)
	require.Len(t, unrealized, 2)
	for _, entry := range unrealized {
		require.NotNil(t, entry.UnrealizedGainValue)
		if entry.PositionSide == "short" {
			// 20.00 held − 24.00 to cover.
			assertMoneyValue(t, -400, 2, *entry.UnrealizedGainValue, *entry.UnrealizedGainScale, "short loses as the price rises")
		} else {
			// 60.00 value − 50.00 cost.
			assertMoneyValue(t, 1000, 2, *entry.UnrealizedGainValue, *entry.UnrealizedGainScale, "long gains")
		}
	}
}
