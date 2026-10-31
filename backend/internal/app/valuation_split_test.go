package app

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"rekenraam/backend/internal/db"
	"rekenraam/backend/internal/exact"
)

// #407: a raw quote prices a unit of its own day, so a stale quote reused
// after a recorded split is rescaled by the instrument's splits between its
// date and the requested one before it values post-split ledger units. Every
// RateTable consumer reads the same rule. Fixture: the returns fixture (10
// TEST at 100 on 2026-01-31, broker cash 100), a 2:1 split on 2026-02-02 and
// the next quote, 50, on 2026-02-04. Expected values are hand-derived.

func splitValuationFixture(t *testing.T) *returnsFixture {
	t.Helper()
	f := newReturnsFixture(t)
	_, err := acknowledgedSplit(context.Background(), f.investmentService, splitInput(f.investmentsTestFixture, "2026-02-02", 2, 1))
	require.NoError(t, err)
	f.price(t, "2026-02-04", 5000)
	f.transactionService.SetPricingRepository(db.NewPricingRepository(f.database))
	return f
}

// requireConvertedEuros compares a converted figure with whole euros.
func requireConvertedEuros(t *testing.T, euros int64, converted *BalanceQuantity) {
	t.Helper()
	require.NotNil(t, converted)
	require.Equal(t, 0, exact.ScaledIntFromCoefficient(converted.QuantityValue, converted.QuantityScale).
		Cmp(exact.ScaledIntFromInt64(euros*100, 2)), "expected %d, got %se-%d", euros, converted.QuantityValue, converted.QuantityScale)
}

// requireSplitUse finds the rate use for a commodity on a date and compares
// the split factor it applied ("" for none).
func requireSplitUse(t *testing.T, coverage ValuationCoverage, commodityID int64, date, observed, factor string) {
	t.Helper()
	for _, use := range coverage.Used {
		if use.CommodityID == commodityID && use.RequestedDate == date {
			require.Equal(t, observed, use.ObservationDate)
			actual := ""
			if use.SplitRatioNumerator != "" {
				actual = use.SplitRatioNumerator + "/" + use.SplitRatioDenominator
			}
			require.Equal(t, factor, actual)
			return
		}
	}
	t.Fatalf("no rate use for %d on %s", commodityID, date)
}

func TestReturnsSeriesRescalesAStalePreSplitQuote(t *testing.T) {
	t.Parallel()
	f := splitValuationFixture(t)

	result := f.read(t, "2026-02-01", "2026-02-04")
	// 20 units × 100 / 2 = 1,000, plus 100 cash: no change, not 2,100.
	requireSeriesPoint(t, result.TimeWeightedSeries, "2026-02-02", "0.000000", "", "")
	requireSeriesPoint(t, result.TimeWeightedSeries, "2026-02-03", "0.000000", "", "")
	requireReturn(t, "0.000000", result.TimeWeighted)
	requireSplitUse(t, result.TimeWeightedSeries.Valuation, f.stockCommodityID, "2026-02-03", "2026-01-31", "2/1")
}

func TestNetWorthSeriesRescalesAStalePreSplitQuote(t *testing.T) {
	t.Parallel()
	f := splitValuationFixture(t)

	series, err := f.transactionService.NetWorthSeries(context.Background(), NetWorthSeriesInput{
		StartDate: "2026-02-01", EndDate: "2026-02-04", Bucket: "day",
		Reporting: &ReportingCurrencyInput{CommodityID: f.eurCommodityID}})
	require.NoError(t, err)
	require.Len(t, series.Buckets, 4)
	// Checking holds 10,000 − 1,100 = 8,900 throughout; the portfolio 1,100.
	for _, bucket := range series.Buckets {
		requireConvertedEuros(t, 10000, bucket.Converted)
	}
	requireSplitUse(t, *series.Valuation, f.stockCommodityID, "2026-02-01", "2026-01-31", "")
	requireSplitUse(t, *series.Valuation, f.stockCommodityID, "2026-02-02", "2026-01-31", "2/1")
	requireSplitUse(t, *series.Valuation, f.stockCommodityID, "2026-02-04", "2026-02-04", "")
}

func TestCashflowRescalesAStalePreSplitQuote(t *testing.T) {
	t.Parallel()
	f := splitValuationFixture(t)
	// One post-split unit delivered into broker cash on 02-03, priced by the
	// 01-31 quote: 100 / 2 = 50.
	_, err := f.transactionService.CreateTransaction(context.Background(), CreateTransactionInput{
		OwnerUserID: f.ownerUserID, OriginType: "browser_api",
		Spec: TransactionInput{TransactionDate: "2026-02-03", JournalEntries: []JournalEntryInput{{Postings: []PostingInput{
			{AccountID: f.brokerCashID, CommodityID: f.stockCommodityID, QuantityValue: exact.New(1), QuantityScale: 0},
			{AccountID: f.equityID, CommodityID: f.stockCommodityID, QuantityValue: exact.New(-1), QuantityScale: 0},
		}}}},
	})
	require.NoError(t, err)

	result, err := f.transactionService.Cashflow(context.Background(), CashflowInput{
		StartDate: "2026-02-03", EndDate: "2026-02-03", Bucket: "day",
		Filters:   ReportFilters{AccountIDs: []int64{f.brokerCashID}},
		Reporting: &ReportingCurrencyInput{CommodityID: f.eurCommodityID}})
	require.NoError(t, err)
	require.Len(t, result.Buckets, 1)
	requireConvertedEuros(t, 50, result.Buckets[0].ConvertedNetMovement)
	requireSplitUse(t, *result.Valuation, f.stockCommodityID, "2026-02-03", "2026-01-31", "2/1")
}

// Holdings that record different factors for one day leave a value spanning
// that day unknown, never a guessed factor; a quote on or after the day is
// unaffected.
func TestValuationRefusesAStaleQuoteAcrossConflictingSplits(t *testing.T) {
	t.Parallel()
	f := newReturnsFixture(t)
	second := seedTestAccountWithClass(t, f.database, "active", true, "asset", "security_holding")
	for holding, ratio := range map[int64]int64{f.holdingAccountID: 2, second: 3} {
		input := splitInput(f.investmentsTestFixture, "2026-02-02", ratio, 1)
		input.HoldingAccountID = holding
		_, err := acknowledgedSplit(context.Background(), f.investmentService, input)
		require.NoError(t, err)
	}
	f.price(t, "2026-02-04", 5000)
	f.transactionService.SetPricingRepository(db.NewPricingRepository(f.database))

	series, err := f.transactionService.NetWorthSeries(context.Background(), NetWorthSeriesInput{
		StartDate: "2026-02-01", EndDate: "2026-02-04", Bucket: "day",
		Reporting: &ReportingCurrencyInput{CommodityID: f.eurCommodityID}})
	require.NoError(t, err)
	requireConvertedEuros(t, 10000, series.Buckets[0].Converted)
	require.Nil(t, series.Buckets[1].Converted)
	require.Nil(t, series.Buckets[2].Converted)
	requireConvertedEuros(t, 10000, series.Buckets[3].Converted)
	require.False(t, series.Valuation.Complete)
	require.Equal(t, []RateGap{{CommodityID: f.stockCommodityID, Reason: RateMissingConflictingSplits, NearestObservationDate: "2026-01-31"}},
		series.Valuation.Gaps)
}

// #408: holdings and unrealized gains price current ledger units with the
// selected observation directly, not through RateTable, under the same rule.
// As of 02-03 the 01-31 quote prices pre-split units: 20 × 100 / 2 = 1,000
// against the 1,000 basis. As of 02-04 the post-split quote, 50, is fresh.
// Prices judges a pair by what valuation can do with it, not by age alone: a
// recent quote across a day the holdings disagree on prices unknown units,
// so Home's "needs a price" link must still list it.
func TestPricingCoverageRefusesARecentQuoteAcrossConflictingSplits(t *testing.T) {
	t.Parallel()
	f := newReturnsFixture(t)
	_, err := f.database.Exec(`UPDATE books SET default_currency_commodity_id=? WHERE id=1`, f.eurCommodityID)
	require.NoError(t, err)
	second := seedTestAccountWithClass(t, f.database, "active", true, "asset", "security_holding")
	for holding, ratio := range map[int64]int64{f.holdingAccountID: 2, second: 3} {
		input := splitInput(f.investmentsTestFixture, "2026-02-02", ratio, 1)
		input.HoldingAccountID = holding
		_, err := acknowledgedSplit(context.Background(), f.investmentService, input)
		require.NoError(t, err)
	}

	stock := func(asOf string) PricingCoverage {
		t.Helper()
		workspace, err := f.pricing.Workspace(context.Background(), asOf)
		require.NoError(t, err)
		for _, coverage := range workspace.Coverage {
			if coverage.BaseCommodityID == f.stockCommodityID && coverage.QuoteCommodityID == f.eurCommodityID {
				return coverage
			}
		}
		t.Fatalf("no stock coverage on %s: %+v", asOf, workspace.Coverage)
		return PricingCoverage{}
	}
	before := stock("2026-02-01")
	require.Equal(t, "usable", before.Status, "before the split day the quote is current")

	across := stock("2026-02-03")
	require.NotNil(t, across.AgeDays)
	require.LessOrEqual(t, *across.AgeDays, DefaultPricingStalenessDays, "recent enough by age alone")
	require.Equal(t, "missing", across.Status)
	require.Equal(t, RateMissingConflictingSplits, across.Reason)
	require.NotNil(t, across.Observation, "the quote stays visible with why it cannot be used")

	f.price(t, "2026-02-04", 5000)
	require.Equal(t, "usable", stock("2026-02-04").Status, "a quote after the split day is unaffected")
}

func TestUnrealizedGainsRescaleAStalePreSplitQuote(t *testing.T) {
	t.Parallel()
	f := splitValuationFixture(t)

	for date, factor := range map[string]string{"2026-02-01": "", "2026-02-03": "2/1", "2026-02-04": ""} {
		_, entries, err := f.investmentService.UnrealizedGainsProjection(context.Background(), date)
		require.NoError(t, err)
		require.Len(t, entries, 1, date)
		entry := entries[0]
		require.NotNil(t, entry.MarketValueValue, date)
		require.Equal(t, 0, exact.ScaledIntFromInt64(*entry.MarketValueValue, *entry.MarketValueScale).
			Cmp(exact.ScaledIntFromInt64(100000, 2)), "%s market value %de-%d", date, *entry.MarketValueValue, *entry.MarketValueScale)
		require.Equal(t, int64(0), *entry.UnrealizedGainValue, date)
		actual := ""
		if entry.SplitRatioNumerator != "" {
			actual = entry.SplitRatioNumerator + "/" + entry.SplitRatioDenominator
		}
		require.Equal(t, factor, actual, date)
	}
}

// The current holdings read discloses the factor on the position it prices
// with a stale pre-split quote.
func TestPositionsDiscloseTheSplitAfterTheirQuote(t *testing.T) {
	t.Parallel()
	f := newReturnsFixture(t)
	_, err := acknowledgedSplit(context.Background(), f.investmentService, splitInput(f.investmentsTestFixture, "2026-02-02", 2, 1))
	require.NoError(t, err)

	positions, err := f.investmentService.Positions(context.Background())
	require.NoError(t, err)
	require.Len(t, positions, 1)
	require.Equal(t, "2026-01-31", positions[0].LatestPriceDate)
	require.Equal(t, "2", positions[0].SplitRatioNumerator)
	require.Equal(t, "1", positions[0].SplitRatioDenominator)
}

// Holdings that disagree on a day leave a position valued across it unknown.
func TestUnrealizedGainsRefuseAStaleQuoteAcrossConflictingSplits(t *testing.T) {
	t.Parallel()
	f := newReturnsFixture(t)
	second := seedTestAccountWithClass(t, f.database, "active", true, "asset", "security_holding")
	for holding, ratio := range map[int64]int64{f.holdingAccountID: 2, second: 3} {
		input := splitInput(f.investmentsTestFixture, "2026-02-02", ratio, 1)
		input.HoldingAccountID = holding
		_, err := acknowledgedSplit(context.Background(), f.investmentService, input)
		require.NoError(t, err)
	}

	_, entries, err := f.investmentService.UnrealizedGainsProjection(context.Background(), "2026-02-03")
	require.NoError(t, err)
	require.Len(t, entries, 1)
	require.Nil(t, entries[0].MarketValueValue)
	require.Nil(t, entries[0].UnrealizedGainValue)
	require.Equal(t, db.ValuationConflictingSplits, entries[0].ValuationUnavailable)
}
