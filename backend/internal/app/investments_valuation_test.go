package app

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"rekenraam/backend/internal/exact"
)

// T-99. Market value is quantity × price, so its coefficient carries the sum of
// both scales. A fractional disposal widens the position's quantity scale — six
// places, at this fixture's commodity precision — and that alone was enough to
// push a perfectly ordinary six-figure position's market value past int64, at
// which point the read model dropped the value and the gain and the UI showed
// the position as if nobody had ever priced it.
//
// The valuation is now restated at a scale that fits whenever the digits it is
// losing are trailing zeros, and a value that genuinely cannot be represented
// says so instead of impersonating a missing price.

func scaledFromPointer(t *testing.T, value *int64, scale *int) *exact.ScaledInt {
	t.Helper()
	require.NotNil(t, value)
	require.NotNil(t, scale)
	return exact.ScaledIntFromInt64(*value, *scale)
}

func TestFractionalSaleRetainsPricedMarketValue(t *testing.T) {
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()

	// 1,000 shares at 100.00 EUR each.
	buyOn(t, f, "2026-01-01", 1000, 10000000)

	before, err := f.investmentService.ListUnrealizedGains(ctx)
	require.NoError(t, err)
	require.Len(t, before, 1)
	require.Equal(t, "", before[0].ValuationUnavailable)
	require.Equal(t, 0, scaledFromPointer(t, before[0].MarketValueValue, before[0].MarketValueScale).
		Cmp(exact.ScaledIntFromInt64(10000000, 2)), "100,000.00 EUR before the sale")

	// Sell 0.000001 shares for 0.000100 EUR — the same 100 EUR per share.
	sale := sellInput(f, "2026-02-01", 1)
	sale.QuantityScale = 6
	sale.CashAmountValue = 100
	sale.CashAmountScale = 6
	_, err = f.investmentService.Sell(ctx, sale)
	require.NoError(t, err)

	after, err := f.investmentService.ListUnrealizedGains(ctx)
	require.NoError(t, err)
	require.Len(t, after, 1)
	require.NotNil(t, after[0].LatestPriceValue, "the sale implies a price")
	require.Equal(t, "", after[0].ValuationUnavailable,
		"a priced position must keep a valuation across a valid fractional sale")

	// 999.999999 shares × 100 EUR = 99,999.9999 EUR exactly.
	require.Equal(t, 0, scaledFromPointer(t, after[0].MarketValueValue, after[0].MarketValueScale).
		Cmp(exact.ScaledIntFromInt64(999999999, 4)), "99,999.9999 EUR after the sale")

	// The gain is that market value less what remains of the cost basis, and it
	// survives the same restatement.
	gain := scaledFromPointer(t, after[0].UnrealizedGainValue, after[0].UnrealizedGainScale)
	expectedGain := exact.ScaledIntFromInt64(999999999, 4)
	expectedGain.AddInt64(-after[0].RemainingCostBasisValue, after[0].RemainingCostBasisScale)
	require.Equal(t, 0, gain.Cmp(expectedGain))
}
