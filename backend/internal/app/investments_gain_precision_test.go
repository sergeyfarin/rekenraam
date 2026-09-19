package app

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"rekenraam/backend/internal/exact"
)

// T-101. The realized-gains report restated the disposed cost basis to the
// scale the *proceeds* happened to be entered at, then subtracted. Selling for
// 11 EUR entered as value 11 at scale 0 therefore truncated a 10.99 EUR basis
// to 10 EUR and reported a 1 EUR gain instead of 0.01 EUR — a hundredfold
// error, visible in the UI, produced by nothing but how the same amount was
// typed. Entering the identical proceeds as 1100 at scale 2 gave the right
// answer, which is the tell: the scale a figure was entered at says nothing
// about the precision its difference needs.
//
// The gain is now computed at whichever scale is deeper and carries its own
// realized_gain_scale, the way the unrealized gain already did.

// gainOf reads the single realized gain the fixture's trades produced, as an
// exact value at its own scale.
func gainOf(t *testing.T, f *investmentsTestFixture) *exact.ScaledInt {
	t.Helper()
	gains, err := f.investmentService.ListRealizedGains(context.Background(), GainsReportParams{})
	require.NoError(t, err)
	require.Len(t, gains, 1)
	return exact.ScaledIntFromInt64(gains[0].RealizedGainValue, gains[0].RealizedGainScale)
}

// TestRealizedGainIsTheSameHoweverProceedsWereEntered runs both scale
// directions of the reported case: the same 11 EUR of proceeds, once at scale 0
// and once at scale 2, against the same 10.99 EUR basis.
func TestRealizedGainIsTheSameHoweverProceedsWereEntered(t *testing.T) {
	for _, entered := range []struct {
		name  string
		value int64
		scale int
	}{
		{name: "scale 0", value: 11, scale: 0},
		{name: "scale 2", value: 1100, scale: 2},
	} {
		t.Run(entered.name, func(t *testing.T) {
			f := newInvestmentsTestFixture(t)
			ctx := context.Background()
			buyOn(t, f, "2026-01-01", 1, 1099)
			sale := sellInput(f, "2026-02-01", 1)
			sale.CashAmountValue = entered.value
			sale.CashAmountScale = entered.scale

			preview, err := f.investmentService.PreviewSell(ctx, sale)
			require.NoError(t, err)
			_, err = f.investmentService.Sell(ctx, sale)
			require.NoError(t, err)

			oneCent := exact.ScaledIntFromInt64(1, 2)
			require.Zero(t, gainOf(t, f).Cmp(oneCent), "11 EUR less a 10.99 EUR basis is 0.01 EUR either way")
			require.Zero(t, exact.ScaledIntFromInt64(preview.RealizedGain, preview.RealizedGainScale).Cmp(oneCent))
		})
	}
}

// TestRealizedGainMatchesItsPreview pins the equality the defect broke: the
// preview always subtracted at a common scale, so preview and report disagreed
// by a factor of a hundred on the same sale. They are the same number.
func TestRealizedGainMatchesItsPreview(t *testing.T) {
	for _, sale := range []struct {
		name          string
		basis         int64
		proceedsValue int64
		proceedsScale int
		quantity      int64
		expectedGain  int64
		expectedScale int
	}{
		{name: "gain at shallow proceeds scale", basis: 1099, proceedsValue: 11, proceedsScale: 0, quantity: 1, expectedGain: 1, expectedScale: 2},
		{name: "loss at shallow proceeds scale", basis: 1101, proceedsValue: 11, proceedsScale: 0, quantity: 1, expectedGain: -1, expectedScale: 2},
		{name: "loss the old rounding would have hidden", basis: 1001, proceedsValue: 10, proceedsScale: 0, quantity: 1, expectedGain: -1, expectedScale: 2},
		{name: "deep proceeds against a shallow basis", basis: 1000, proceedsValue: 100025, proceedsScale: 4, quantity: 1, expectedGain: 25, expectedScale: 4},
	} {
		t.Run(sale.name, func(t *testing.T) {
			f := newInvestmentsTestFixture(t)
			ctx := context.Background()
			buyOn(t, f, "2026-01-01", sale.quantity, sale.basis)
			input := sellInput(f, "2026-02-01", sale.quantity)
			input.CashAmountValue = sale.proceedsValue
			input.CashAmountScale = sale.proceedsScale

			preview, err := f.investmentService.PreviewSell(ctx, input)
			require.NoError(t, err)
			_, err = f.investmentService.Sell(ctx, input)
			require.NoError(t, err)

			expected := exact.ScaledIntFromInt64(sale.expectedGain, sale.expectedScale)
			reported := gainOf(t, f)
			require.Zero(t, reported.Cmp(expected), "reported %s, want %s", reported.String(), expected.String())
			require.Zero(t, exact.ScaledIntFromInt64(preview.RealizedGain, preview.RealizedGainScale).Cmp(reported),
				"preview and report must be the same number")
		})
	}
}

// TestRealizedGainClosesAMixedScaleMultiLotPositionExactly disposes several
// lots bought at different basis scales in one sale, which is where a
// per-entry rounding error would accumulate rather than cancel. Closing the
// whole position means the gain is exactly proceeds less total cost.
func TestRealizedGainClosesAMixedScaleMultiLotPositionExactly(t *testing.T) {
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	for _, buy := range []struct {
		quantity   int64
		basisValue int64
		basisScale int
	}{
		{quantity: 1, basisValue: 1099, basisScale: 2},
		{quantity: 1, basisValue: 100001, basisScale: 4},
		{quantity: 1, basisValue: 9, basisScale: 0},
	} {
		input := sellInput(f, "2026-01-01", buy.quantity)
		input.CashAmountValue = buy.basisValue
		input.CashAmountScale = buy.basisScale
		_, err := f.investmentService.Buy(ctx, input)
		require.NoError(t, err)
	}

	// 10.99 + 10.0001 + 9 = 29.9901 of cost, sold for 30 EUR.
	sale := sellInput(f, "2026-02-01", 3)
	sale.CashAmountValue = 30
	sale.CashAmountScale = 0
	_, err := f.investmentService.Sell(ctx, sale)
	require.NoError(t, err)

	gains, err := f.investmentService.ListRealizedGains(ctx, GainsReportParams{})
	require.NoError(t, err)
	total := exact.NewScaledInt()
	for _, gain := range gains {
		total.AddInt64(gain.RealizedGainValue, gain.RealizedGainScale)
	}
	require.Zero(t, total.Cmp(exact.ScaledIntFromInt64(99, 4)), "30 less 29.9901 is 0.0099, got %s", total.String())
}
