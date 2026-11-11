package app

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"rekenraam/backend/internal/exact"
)

// Same-day event order around a split (combined review of cae8039d, 21106562
// and 129abf3f). A split splits the lots that exist at its replay slot, so a
// trade or delivery ordered before it that day carried pre-split units: its
// implied quote is cash per pre-split unit, and its posted units each became
// the split's factor. Expected figures are hand-derived from the fixture: 10
// TEST at 100 and broker cash 100 at the opening boundary (2026-01-31).

// O1: a trade quote ordered before the day's 2:1 split priced a pre-split
// unit, 100, which is 50 per end-of-day unit; 22 units are worth 1,100.
func TestSplitDayTradeQuoteOrderedBeforeTheSplitPricesPreSplitUnits(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := newReturnsFixture(t)
	f.buyFromBrokerCash(t, "2026-02-02", 10000)
	_, err := acknowledgedSplit(ctx, f.investmentService, splitInput(f.investmentsTestFixture, "2026-02-02", 2, 1))
	require.NoError(t, err)

	result := f.read(t, "2026-02-01", "2026-02-03")
	requireReturnsAmount(t, 1100, result.ClosingValue) // 22 × 50 + 0 cash
	requireReturn(t, "0.000000", result.TimeWeighted)

	_, gains, err := f.investmentService.UnrealizedGainsProjection(ctx, "2026-02-03")
	require.NoError(t, err)
	require.Len(t, gains, 1)
	require.NotNil(t, gains[0].MarketValueValue)
	require.Equal(t, 0, exact.ScaledIntFromInt64(*gains[0].MarketValueValue, int(*gains[0].MarketValueScale)).
		Cmp(exact.ScaledIntFromInt64(110000, 2)), "holdings value 22 units at 50")
}

// O2, the control: a trade ordered after the split carried post-split units,
// so its quote of 50 is already per end-of-day unit; 21 × 50 + 50 cash.
func TestSplitDayTradeQuoteOrderedAfterTheSplitIsUnchanged(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := newReturnsFixture(t)
	_, err := acknowledgedSplit(ctx, f.investmentService, splitInput(f.investmentsTestFixture, "2026-02-02", 2, 1))
	require.NoError(t, err)
	f.buyFromBrokerCash(t, "2026-02-02", 5000)

	result := f.read(t, "2026-02-01", "2026-02-03")
	requireReturnsAmount(t, 1100, result.ClosingValue)
	requireReturn(t, "0.000000", result.TimeWeighted)
}

// O3: a delivery of 1 TEST ordered before the day's split moved one pre-split
// unit, worth two end-of-day units: a contribution of 100, whether the day's
// rate comes from a rescaled stale quote or a fresh post-split one.
func TestSplitDayDeliveryOrderedBeforeTheSplitCountsPreSplitUnits(t *testing.T) {
	t.Parallel()
	for name, fresh := range map[string]bool{"stale quote": false, "fresh quote": true} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f := newReturnsFixture(t)
			seedExternalTransferEquity(t, f.database)
			_, err := f.investmentService.ExternalTransferIn(context.Background(), ExternalTransferInInput{
				OwnerUserID: f.ownerUserID, EffectiveOn: "2026-02-02", HoldingAccountID: f.holdingAccountID,
				CommodityID: f.stockCommodityID, QuantityValue: exact.New(1), CostCommodityID: f.eurCommodityID, BasisKnowledge: "unknown",
			})
			require.NoError(t, err)
			_, err = acknowledgedSplit(context.Background(), f.investmentService, splitInput(f.investmentsTestFixture, "2026-02-02", 2, 1))
			require.NoError(t, err)
			if fresh {
				f.price(t, "2026-02-02", 5000)
			}
			result := f.read(t, "2026-02-01", "2026-02-03")
			requireReturnsAmount(t, 1200, result.ClosingValue) // 22 × 50 + 100
			requireReturnsAmount(t, 100, result.Contributions)
			requireReturn(t, "0.000000", result.TimeWeighted)
		})
	}
}

// The reviewer's chained-FX probe, kept: an unheld benchmark quoted in USD
// with its split recorded in an empty holding stays split-neutral, and only
// the FX move (0.90 → 0.80) remains.
func TestSplitNeutralChainedBenchmark(t *testing.T) {
	t.Parallel()
	f := newReturnsFixture(t)
	usd := f.commodity(t, "USD", "currency", 2)
	index := f.commodity(t, "CHIDX", "security", 0)
	input := splitInput(f.investmentsTestFixture, "2026-02-02", 2, 1)
	input.CommodityID = index
	_, err := acknowledgedSplit(context.Background(), f.investmentService, input)
	require.NoError(t, err)
	f.observe(t, index, usd, "raw", "2026-01-31", 10000)
	f.observe(t, index, usd, "raw", "2026-02-04", 5000)
	f.observe(t, usd, f.eurCommodityID, "raw", "2026-01-31", 90)
	f.observe(t, usd, f.eurCommodityID, "raw", "2026-02-03", 80)
	benchmark := f.readBenchmark(t, "2026-02-01", "2026-02-04", index, "").Benchmark
	requireReturn(t, "-0.111111", benchmarkPoint(t, benchmark, "2026-02-03"))
	requireReturn(t, "-0.111111", benchmark.TimeWeighted)
}

// buyFromBrokerCash buys 1 TEST into the holding for the given cents, inside
// the portfolio; the buy also records its trade-implied quote.
func (f *returnsFixture) buyFromBrokerCash(t *testing.T, date string, cents int64) {
	t.Helper()
	_, err := f.investmentService.Buy(context.Background(), InvestmentTradeInput{
		OwnerUserID: f.ownerUserID, TransactionDate: date, CommodityID: f.stockCommodityID,
		HoldingAccountID: f.holdingAccountID, CashAccountID: f.brokerCashID,
		QuantityValue: exact.New(1), CashAmountValue: cents, CashAmountScale: 2, CashCommodityID: f.eurCommodityID,
	})
	require.NoError(t, err)
}

// O4–O7, review of 942507ae: a quote prices the instrument, so its basis on a
// split day is instrument-wide, decided by every holding that recorded the
// day's split, not only the trade's own holding. Quantities stay a holding's.

// O4: holding A sells its 10 TEST at 100 before holding B records the day's
// 2:1 split; A records none (it holds nothing after). The sale priced a
// pre-split unit, so B's 2 units are worth 100, not 200.
func TestSplitDayQuoteFromAnotherHoldingUsesTheInstrumentBasis(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := newReturnsFixture(t)
	second := f.secondHoldingWithOneUnit(t)
	_, err := f.investmentService.Sell(ctx, InvestmentTradeInput{
		OwnerUserID: f.ownerUserID, TransactionDate: "2026-02-02", CommodityID: f.stockCommodityID,
		HoldingAccountID: f.holdingAccountID, CashAccountID: f.brokerCashID, QuantityValue: exact.New(10),
		CashAmountValue: 100000, CashAmountScale: 2, CashCommodityID: f.eurCommodityID, CostBasisMethod: "fifo",
	})
	require.NoError(t, err)
	input := splitInput(f.investmentsTestFixture, "2026-02-02", 2, 1)
	input.HoldingAccountID = second
	_, err = acknowledgedSplit(ctx, f.investmentService, input)
	require.NoError(t, err)

	result := f.readBenchmark(t, "2026-02-01", "2026-02-03", f.stockCommodityID, "")
	requireReturnsAmount(t, 1100, result.ClosingValue) // 1,000 cash + 2 × 50
	requireReturn(t, "0.000000", result.TimeWeighted)
	requireReturn(t, "0.000000", result.Benchmark.TimeWeighted)
}

// O5: replacing the trade and then the split keeps each in its original slot.
func TestSplitDayQuoteBasisSurvivesTradeAndSplitReplacement(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := newReturnsFixture(t)
	input := InvestmentTradeInput{
		OwnerUserID: f.ownerUserID, TransactionDate: "2026-02-02", CommodityID: f.stockCommodityID,
		HoldingAccountID: f.holdingAccountID, CashAccountID: f.brokerCashID, QuantityValue: exact.New(1),
		CashAmountValue: 10000, CashAmountScale: 2, CashCommodityID: f.eurCommodityID,
	}
	trade, err := f.investmentService.Buy(ctx, input)
	require.NoError(t, err)
	split, err := acknowledgedSplit(ctx, f.investmentService, splitInput(f.investmentsTestFixture, "2026-02-02", 2, 1))
	require.NoError(t, err)
	_, err = acknowledgedReplaceBuy(ctx, f.investmentService, ReplaceInvestmentBuyInput{
		OwnerUserID: f.ownerUserID, TransactionID: trade.Transaction.ID, Reason: "restated same trade", Replacement: input,
	})
	require.NoError(t, err)
	_, err = acknowledgedReplaceSplit(ctx, f.investmentService, replaceSplitInput(f.investmentsTestFixture, split.Transaction.ID, "2026-02-02", 4, 1))
	require.NoError(t, err)

	result := f.readBenchmark(t, "2026-02-01", "2026-02-03", f.stockCommodityID, "")
	requireReturnsAmount(t, 1100, result.ClosingValue) // 44 × 25
	requireReturn(t, "0.000000", result.TimeWeighted)
	requireReturn(t, "0.000000", result.Benchmark.TimeWeighted)
}

// O6: a trade between two same-day splits of its holding priced units after
// the first and before the second: 50 is 25 per end-of-day unit.
func TestSplitDayTradeBetweenSuccessiveSplits(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := newReturnsFixture(t)
	_, err := acknowledgedSplit(ctx, f.investmentService, splitInput(f.investmentsTestFixture, "2026-02-02", 2, 1))
	require.NoError(t, err)
	f.buyFromBrokerCash(t, "2026-02-02", 5000)
	_, err = acknowledgedSplit(ctx, f.investmentService, splitInput(f.investmentsTestFixture, "2026-02-02", 2, 1))
	require.NoError(t, err)

	result := f.readBenchmark(t, "2026-02-01", "2026-02-03", f.stockCommodityID, "")
	requireReturnsAmount(t, 1100, result.ClosingValue) // 42 × 25 + 50
	requireReturn(t, "0.000000", result.TimeWeighted)
	requireReturn(t, "0.000000", result.Benchmark.TimeWeighted)
}

// O7: holding B records the day's split before a trade in holding A, which
// records it after: the units the trade's quote priced are unknown, so the
// value it would give is a conflicting_splits gap, never a guess.
func TestSplitDayQuoteBetweenHoldingsSplitsIsAGap(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := newReturnsFixture(t)
	second := f.secondHoldingWithOneUnit(t)
	input := splitInput(f.investmentsTestFixture, "2026-02-02", 2, 1)
	input.HoldingAccountID = second
	_, err := acknowledgedSplit(ctx, f.investmentService, input)
	require.NoError(t, err)
	f.buyFromBrokerCash(t, "2026-02-02", 10000)
	_, err = acknowledgedSplit(ctx, f.investmentService, splitInput(f.investmentsTestFixture, "2026-02-02", 2, 1))
	require.NoError(t, err)

	result := f.read(t, "2026-02-01", "2026-02-03")
	require.Nil(t, result.ClosingValue.Amount)
	require.Equal(t, ReturnReasonValuationUnavailable, result.ClosingValue.Reason)
	require.Len(t, result.Valuation.Gaps, 1)
	require.Equal(t, RateMissingConflictingSplits, result.Valuation.Gaps[0].Reason)

	_, gains, err := f.investmentService.UnrealizedGainsProjection(ctx, "2026-02-03")
	require.NoError(t, err)
	for _, entry := range gains {
		require.Nil(t, entry.MarketValueValue, "holdings refuse the disputed quote too")
	}
}

// secondHoldingWithOneUnit opens another TEST holding with 1 unit bought from
// broker cash for 100 before the opening boundary.
func (f *returnsFixture) secondHoldingWithOneUnit(t *testing.T) int64 {
	t.Helper()
	second := seedTestAccountWithClass(t, f.database, "active", true, "asset", "security_holding")
	_, err := f.investmentService.Buy(context.Background(), InvestmentTradeInput{
		OwnerUserID: f.ownerUserID, TransactionDate: "2026-01-25", CommodityID: f.stockCommodityID,
		HoldingAccountID: second, CashAccountID: f.brokerCashID, QuantityValue: exact.New(1),
		CashAmountValue: 10000, CashAmountScale: 2, CashCommodityID: f.eurCommodityID,
	})
	require.NoError(t, err)
	return second
}

// A split recorded on a later day is not pending for an earlier trade quote:
// only splits on the quote's own day can change which units it priced (#561).
// The quote of 100 prices 11 units at 1,100 before the split; after it, the
// stale quote is rescaled to 50 for 22 units, still 1,100.
func TestTradeQuoteBeforeALaterDaySplitIsNotPending(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := newReturnsFixture(t)
	f.buyFromBrokerCash(t, "2026-02-02", 10000)
	_, err := acknowledgedSplit(ctx, f.investmentService, splitInput(f.investmentsTestFixture, "2026-02-05", 2, 1))
	require.NoError(t, err)

	before := f.read(t, "2026-02-01", "2026-02-03")
	requireReturnsAmount(t, 1100, before.ClosingValue)
	requireReturn(t, "0.000000", before.TimeWeighted)

	after := f.read(t, "2026-02-01", "2026-02-06")
	requireReturnsAmount(t, 1100, after.ClosingValue)
	requireReturn(t, "0.000000", after.TimeWeighted)
}
