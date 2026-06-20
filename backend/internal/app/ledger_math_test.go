package app

import (
	"math"
	"math/big"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rekenraam/backend/internal/db"
	"rekenraam/backend/internal/exact"
)

// ---------------------------------------------------------------------------
// scaledAmount
// ---------------------------------------------------------------------------

func TestScaledAmountAddSameScale(t *testing.T) {
	a := newScaledAmount()
	a.add(exact.New(100), 2)
	a.add(exact.New(250), 2)

	got, err := a.coefficient()
	require.NoError(t, err)
	assert.Equal(t, "350", got.String())
	assert.Equal(t, 2, a.scale)
}

func TestScaledAmountAddAlignsToDeeperScale(t *testing.T) {
	// 1.00 at scale 2  +  0.001 at scale 3  =  1.001 at scale 3
	a := newScaledAmount()
	a.add(exact.New(100), 2) // 100 * 10^-2 = 1.00
	a.add(exact.New(1), 3)   // 1   * 10^-3 = 0.001

	got, err := a.coefficient()
	require.NoError(t, err)
	assert.Equal(t, "1001", got.String()) // 1.001 * 10^3
	assert.Equal(t, 3, a.scale)
}

func TestScaledAmountAddNegativeValues(t *testing.T) {
	a := newScaledAmount()
	a.add(exact.New(500), 2)
	a.add(exact.New(-200), 2)

	got, err := a.coefficient()
	require.NoError(t, err)
	assert.Equal(t, "300", got.String())
}

func TestScaledAmountAddDoesNotMutateSourceCoefficient(t *testing.T) {
	// BigInt() returns a fresh *big.Int, so align+Mul on the addend must not
	// corrupt the original Coefficient value.
	original := exact.MustParse("12345")
	a := newScaledAmount()
	a.add(original, 0)
	a.add(original, 2) // triggers alignment: addend gets Mul by pow10(2)

	// original must still read as 12345
	assert.Equal(t, "12345", original.String())

	// a should equal 12345 + 12345*100 = 12345 + 1234500 = 1246845 at scale 2
	got, err := a.coefficient()
	require.NoError(t, err)
	assert.Equal(t, "1246845", got.String())
	assert.Equal(t, 2, a.scale)
}

func TestScaledAmountAddScaled(t *testing.T) {
	a := newScaledAmount()
	a.add(exact.New(100), 2)

	b := newScaledAmount()
	b.add(exact.New(50), 3) // different scale

	a.addScaled(b)

	// 100 @ scale 2  +  50 @ scale 3
	// align a to scale 3: 1000 @ scale 3
	// 1000 + 50 = 1050 @ scale 3
	got, err := a.coefficient()
	require.NoError(t, err)
	assert.Equal(t, "1050", got.String())
	assert.Equal(t, 3, a.scale)
}

func TestScaledAmountAddScaledNilIsNoop(t *testing.T) {
	a := newScaledAmount()
	a.add(exact.New(42), 0)
	a.addScaled(nil) // must not panic

	got, err := a.coefficient()
	require.NoError(t, err)
	assert.Equal(t, "42", got.String())
}

func TestScaledAmountCoefficientRejectsOverflow(t *testing.T) {
	// Manually stuff a value larger than 38 digits into scaledAmount.
	a := newScaledAmount()
	a.value, _ = new(big.Int).SetString("1"+string(make([]byte, 38)), 10) // 10^38 — 39 digits
	a.scale = 0

	_, err := a.coefficient()
	require.Error(t, err, "coefficient must reject values exceeding MaxCoefficientDigits")
}

func TestScaledAmountCryptoScaleAlignment(t *testing.T) {
	// Mix a scale-2 fiat posting with a scale-24 crypto posting for the SAME
	// commodity (unusual but must not panic or corrupt).
	a := newScaledAmount()
	a.add(exact.New(1), 2)  // 0.01 at scale 2
	a.add(exact.New(1), 24) // 10^-24 at scale 24

	// After alignment both are at scale 24.
	// 0.01 = 1 * 10^-2 → coefficient 10^22 at scale 24
	// 10^-24 → coefficient 1 at scale 24
	// Total coefficient = 10^22 + 1
	got, err := a.coefficient()
	require.NoError(t, err)

	expected, _ := new(big.Int).SetString("10000000000000000000001", 10) // 10^22 + 1
	assert.Equal(t, expected.String(), got.String())
	assert.Equal(t, 24, a.scale)
}

func TestScaledAmountAddsAtThirtyEightDigitBoundary(t *testing.T) {
	// The largest positive 38-digit coefficient remains valid. Arithmetic is
	// performed with big.Int, so no machine-integer overflow occurs first.
	a := newScaledAmount()
	a.add(exact.MustParse("99999999999999999999999999999999999998"), 24)
	a.add(exact.New(1), 24)

	got, err := a.coefficient()
	require.NoError(t, err)
	assert.Equal(t, "99999999999999999999999999999999999999", got.String())
}

func TestScaledAmountReportsThirtyNineDigitResult(t *testing.T) {
	// Crossing the documented 38-digit storage boundary is rejected instead
	// of wrapping, even though the intermediate big.Int remains exact.
	a := newScaledAmount()
	a.add(exact.MustParse("99999999999999999999999999999999999999"), 24)
	a.add(exact.New(1), 24)

	_, err := a.coefficient()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "at most 38 digits")
}

// ---------------------------------------------------------------------------
// pow10
// ---------------------------------------------------------------------------

func TestPow10(t *testing.T) {
	cases := []struct {
		scale    int
		expected string
	}{
		{0, "1"},
		{1, "10"},
		{2, "100"},
		{12, "1000000000000"},
		{24, "1000000000000000000000000"},
		{60, "1000000000000000000000000000000000000000000000000000000000000"},
	}
	for _, tc := range cases {
		got := pow10(tc.scale)
		assert.Equal(t, tc.expected, got.String(), "pow10(%d)", tc.scale)
	}
}

func TestPow10PanicsOnNegativeScale(t *testing.T) {
	assert.Panics(t, func() { pow10(-1) }, "pow10 must panic on negative scale")
}

func TestPow10PanicsOnExcessiveScale(t *testing.T) {
	assert.Panics(t, func() { pow10(maxPow10Scale + 1) }, "pow10 must panic when scale > maxPow10Scale")
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

	got, err := amount.coefficient()
	require.NoError(t, err)
	assert.Equal(t, "700", got.String()) // 5.00 + 3.00 - 1.00 = 7.00
	assert.Equal(t, 2, amount.scale)
}

func TestAggregatePostingsMixedScalesSameCommodity(t *testing.T) {
	postings := []db.LedgerPostingRecord{
		{AccountID: 1, CommodityID: 10, QuantityValue: exact.New(1), QuantityScale: 0},  // 1
		{AccountID: 1, CommodityID: 10, QuantityValue: exact.New(10), QuantityScale: 1}, // 1.0
	}

	balances := aggregatePostings(postings)
	amount := balances[1][10]
	require.NotNil(t, amount)

	got, err := amount.coefficient()
	require.NoError(t, err)
	assert.Equal(t, "20", got.String()) // 1 + 1 = 2, at scale 1 = coefficient 20
	assert.Equal(t, 1, amount.scale)
}
