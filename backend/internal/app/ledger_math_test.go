package app

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rekenraam/backend/internal/db"
	"rekenraam/backend/internal/exact"
)

func TestCalendarBucketBoundsUsesCalendarBoundariesAndClipsRange(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		startDate string
		endDate   string
		bucket    string
		want      []calendarBucketBound
	}{
		{
			name:      "weekly buckets end Sunday",
			startDate: "2026-06-03",
			endDate:   "2026-06-15",
			bucket:    "week",
			want: []calendarBucketBound{
				{startDate: "2026-06-03", endDate: "2026-06-07"},
				{startDate: "2026-06-08", endDate: "2026-06-14"},
				{startDate: "2026-06-15", endDate: "2026-06-15"},
			},
		},
		{
			name:      "monthly buckets cross month boundary",
			startDate: "2026-02-28",
			endDate:   "2026-04-02",
			bucket:    "month",
			want: []calendarBucketBound{
				{startDate: "2026-02-28", endDate: "2026-02-28"},
				{startDate: "2026-03-01", endDate: "2026-03-31"},
				{startDate: "2026-04-01", endDate: "2026-04-02"},
			},
		},
		{
			name:      "quarterly buckets cross quarter boundary",
			startDate: "2026-03-31",
			endDate:   "2026-04-02",
			bucket:    "quarter",
			want: []calendarBucketBound{
				{startDate: "2026-03-31", endDate: "2026-03-31"},
				{startDate: "2026-04-01", endDate: "2026-04-02"},
			},
		},
		{
			name:      "yearly buckets cross year boundary",
			startDate: "2025-12-31",
			endDate:   "2026-01-02",
			bucket:    "year",
			want: []calendarBucketBound{
				{startDate: "2025-12-31", endDate: "2025-12-31"},
				{startDate: "2026-01-01", endDate: "2026-01-02"},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := calendarBucketBounds(test.startDate, test.endDate, test.bucket)

			require.NoError(t, err)
			assert.Equal(t, test.want, got)
		})
	}
}

// ---------------------------------------------------------------------------
// scaledDivision
// ---------------------------------------------------------------------------

func TestScaledDivisionExactResult(t *testing.T) {
	// 100 USD (scale 2) / 4 BTC (scale 0) = 25 USD/BTC at scale 8
	price, err := scaledDivision(10000, 2, exact.New(4), 0, 8)
	require.NoError(t, err)
	assert.Equal(t, int64(2500000000), price) // 25.00000000
}

func TestScaledDivisionRoundsHalfUp(t *testing.T) {
	// 100 USD / 3 BTC → 33.33333333... at scale 8
	// truncation would give 3333333333 (33.33333333)
	// rounded gives 3333333333 — last significant digit is 3, no rounding up
	price, err := scaledDivision(10000, 2, exact.New(3), 0, 8)
	require.NoError(t, err)
	// 100/3 = 33.3333... → at scale 8: 3333333333 (rounds to 33.33333333)
	assert.Equal(t, int64(3333333333), price)
}

func TestScaledDivisionRoundsUp(t *testing.T) {
	// 2 / 3 at scale 4 = 0.6666... → rounds to 6667 (0.6667)
	price, err := scaledDivision(2, 0, exact.New(3), 0, 4)
	require.NoError(t, err)
	assert.Equal(t, int64(6667), price) // 0.6667, rounded half-up
}

func TestScaledDivisionRoundsAfterNegativeExponent(t *testing.T) {
	price, err := scaledDivision(5000, 12, exact.New(1), 0, 8)
	require.NoError(t, err)
	assert.Equal(t, int64(1), price)
}

func TestScaledDivisionRoundsNegativeHalfAwayFromZero(t *testing.T) {
	price, err := scaledDivision(-1, 0, exact.New(2), 0, 0)
	require.NoError(t, err)
	assert.Equal(t, int64(-1), price)
}

func TestScaledDivisionDivisionByZero(t *testing.T) {
	_, err := scaledDivision(100, 2, exact.New(0), 0, 8)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "division by zero")
}

func TestScaledDivisionOverflow(t *testing.T) {
	// Very small denominator (1 unit at scale 24) and large numerator/scale
	// should overflow int64.
	tiny := exact.MustParse("1") // 1 * 10^-24
	_, err := scaledDivision(1, 0, tiny, 24, 12)
	// price = 1 / (10^-24) * 10^12 = 10^36 — too large for int64
	require.Error(t, err)
	assert.Contains(t, err.Error(), "overflow")
}

func TestScaledDivisionCryptoNumeratorScale(t *testing.T) {
	// 5 USD (scale 2) / 50 tokens (scale 8) = 0.1 USD/token.
	price, err := scaledDivision(500, 2, exact.MustParse("5000000000"), 8, 8)
	require.NoError(t, err)
	assert.Equal(t, int64(10_000_000), price)
}

func TestScaledDivisionTinyCryptoAmountByLargeFiatAmountReportsOverflow(t *testing.T) {
	// 10 billion quote units / 10^-21 crypto units = 10^31 quote units per
	// crypto unit. The calculation is exact, but the current price coefficient
	// is int64 and therefore cannot store the result.
	_, err := scaledDivision(10_000_000_000, 0, exact.New(1), 21, 0)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "overflow")
}

// ---------------------------------------------------------------------------
// scaledFXProduct
// ---------------------------------------------------------------------------

func TestScaledFXProductUsesWideIntermediate(t *testing.T) {
	// Both factors fit int64. Their raw product does not, but after applying
	// the scales the result is exactly math.MaxInt64 and must remain lossless.
	first := PriceObservation{PriceValue: math.MaxInt64, PriceScale: 18, BaseQuantityValue: 1}
	second := PriceObservation{PriceValue: 1_000_000_000_000_000_000, PriceScale: 18, BaseQuantityValue: 1}

	got, err := scaledFXProduct(first, second, 18)
	require.NoError(t, err)
	assert.Equal(t, int64(math.MaxInt64), got)
}

func TestScaledFXProductReportsResultOverflow(t *testing.T) {
	first := PriceObservation{PriceValue: math.MaxInt64, BaseQuantityValue: 1}
	second := PriceObservation{PriceValue: 2, BaseQuantityValue: 1}

	_, err := scaledFXProduct(first, second, 0)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "overflows int64")
}

func TestScaledFXProductRoundsHalfUpAtResultScale(t *testing.T) {
	first := PriceObservation{PriceValue: 1, BaseQuantityValue: 2}
	second := PriceObservation{PriceValue: 1, BaseQuantityValue: 1}

	got, err := scaledFXProduct(first, second, 0)
	require.NoError(t, err)
	assert.Equal(t, int64(1), got)
}

// ---------------------------------------------------------------------------
// aggregatePostings / rollupBalances integration
// ---------------------------------------------------------------------------

func TestAggregatePostingsAccumulatesCorrectly(t *testing.T) {
	postings := []db.LedgerPostingRecord{
		{AccountID: 1, CommodityID: 10, QuantityValue: exact.New(500), QuantityScale: 2},
		{AccountID: 1, CommodityID: 10, QuantityValue: exact.New(300), QuantityScale: 2},
		{AccountID: 1, CommodityID: 10, QuantityValue: exact.New(-100), QuantityScale: 2},
	}

	balances := aggregatePostings(postings)
	amount := balances[1][10]
	require.NotNil(t, amount)

	got, err := amount.Coefficient()
	require.NoError(t, err)
	assert.Equal(t, "700", got.String()) // 5.00 + 3.00 - 1.00 = 7.00
	assert.Equal(t, 2, amount.Scale())
}

func TestAggregatePostingsMixedScalesSameCommodity(t *testing.T) {
	postings := []db.LedgerPostingRecord{
		{AccountID: 1, CommodityID: 10, QuantityValue: exact.New(1), QuantityScale: 0},  // 1
		{AccountID: 1, CommodityID: 10, QuantityValue: exact.New(10), QuantityScale: 1}, // 1.0
	}

	balances := aggregatePostings(postings)
	amount := balances[1][10]
	require.NotNil(t, amount)

	got, err := amount.Coefficient()
	require.NoError(t, err)
	assert.Equal(t, "20", got.String()) // 1 + 1 = 2, at scale 1 = coefficient 20
	assert.Equal(t, 1, amount.Scale())
}
