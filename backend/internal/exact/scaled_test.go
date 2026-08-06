package exact

import (
	"math"
	"math/big"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// ScaledInt accumulation
// ---------------------------------------------------------------------------

func TestScaledIntAddSameScale(t *testing.T) {
	a := NewScaledInt()
	a.AddCoefficient(New(100), 2)
	a.AddCoefficient(New(250), 2)

	got, err := a.Coefficient()
	require.NoError(t, err)
	assert.Equal(t, "350", got.String())
	assert.Equal(t, 2, a.Scale())
}

func TestScaledIntAddAlignsToDeeperScale(t *testing.T) {
	// 1.00 at scale 2  +  0.001 at scale 3  =  1.001 at scale 3
	a := NewScaledInt()
	a.AddCoefficient(New(100), 2) // 100 * 10^-2 = 1.00
	a.AddCoefficient(New(1), 3)   // 1   * 10^-3 = 0.001

	got, err := a.Coefficient()
	require.NoError(t, err)
	assert.Equal(t, "1001", got.String()) // 1.001 * 10^3
	assert.Equal(t, 3, a.Scale())
}

func TestScaledIntAddNegativeValues(t *testing.T) {
	a := NewScaledInt()
	a.AddCoefficient(New(500), 2)
	a.AddCoefficient(New(-200), 2)

	got, err := a.Coefficient()
	require.NoError(t, err)
	assert.Equal(t, "300", got.String())
}

func TestScaledIntAddDoesNotMutateSourceCoefficient(t *testing.T) {
	// BigInt() returns a fresh *big.Int, so align+Mul on the addend must not
	// corrupt the original Coefficient value.
	original := MustParse("12345")
	a := NewScaledInt()
	a.AddCoefficient(original, 0)
	a.AddCoefficient(original, 2) // triggers alignment: addend gets Mul by Pow10(2)

	// original must still read as 12345
	assert.Equal(t, "12345", original.String())

	// a should equal 12345 + 12345*100 = 12345 + 1234500 = 1246845 at scale 2
	got, err := a.Coefficient()
	require.NoError(t, err)
	assert.Equal(t, "1246845", got.String())
	assert.Equal(t, 2, a.Scale())
}

func TestScaledIntAddDoesNotMutateSourceBigInt(t *testing.T) {
	source := big.NewInt(7)
	a := NewScaledInt()
	a.Add(source, 0)
	a.Align(4)
	a.Add(source, 0) // addend must be scaled without touching source

	assert.Equal(t, "7", source.String())
	got, err := a.Coefficient()
	require.NoError(t, err)
	assert.Equal(t, "140000", got.String()) // 14 at scale 4
}

func TestScaledIntAddScaled(t *testing.T) {
	a := NewScaledInt()
	a.AddCoefficient(New(100), 2)

	b := NewScaledInt()
	b.AddCoefficient(New(50), 3) // different scale

	a.AddScaled(b)

	// 100 @ scale 2  +  50 @ scale 3
	// align a to scale 3: 1000 @ scale 3
	// 1000 + 50 = 1050 @ scale 3
	got, err := a.Coefficient()
	require.NoError(t, err)
	assert.Equal(t, "1050", got.String())
	assert.Equal(t, 3, a.Scale())

	// The operand must be untouched.
	assert.Equal(t, 3, b.Scale())
	assert.Equal(t, "50", b.BigInt().String())
}

func TestScaledIntAddScaledNilIsNoop(t *testing.T) {
	a := NewScaledInt()
	a.AddCoefficient(New(42), 0)
	a.AddScaled(nil) // must not panic

	got, err := a.Coefficient()
	require.NoError(t, err)
	assert.Equal(t, "42", got.String())
}

func TestScaledIntSubScaledAlignsToDeeperScale(t *testing.T) {
	a := ScaledIntFromInt64(100, 2) // 1.00
	b := ScaledIntFromInt64(1, 3)   // 0.001
	a.SubScaled(b)

	got, err := a.Coefficient()
	require.NoError(t, err)
	assert.Equal(t, "999", got.String()) // 0.999 at scale 3
	assert.Equal(t, 3, a.Scale())
	assert.Equal(t, "1", b.BigInt().String())
}

func TestScaledIntSubScaledNilIsNoop(t *testing.T) {
	a := ScaledIntFromInt64(42, 0)
	a.SubScaled(nil)

	got, err := a.Coefficient()
	require.NoError(t, err)
	assert.Equal(t, "42", got.String())
}

func TestScaledIntNegatedLeavesReceiverUnchanged(t *testing.T) {
	a := ScaledIntFromInt64(250, 2)
	negated := a.Negated()

	assert.Equal(t, "-250", negated.BigInt().String())
	assert.Equal(t, 2, negated.Scale())
	assert.Equal(t, "250", a.BigInt().String())
}

func TestScaledIntBigIntCopiesValue(t *testing.T) {
	a := ScaledIntFromInt64(5, 0)
	copied := a.BigInt()
	copied.Add(copied, big.NewInt(1))

	assert.Equal(t, "5", a.BigInt().String())
}

func TestScaledIntSign(t *testing.T) {
	assert.Equal(t, 0, NewScaledInt().Sign())
	assert.Equal(t, 1, ScaledIntFromInt64(1, 12).Sign())
	assert.Equal(t, -1, ScaledIntFromInt64(-1, 12).Sign())
}

// ---------------------------------------------------------------------------
// ScaledInt alignment and comparison
// ---------------------------------------------------------------------------

func TestScaledIntAlignOnlyDeepens(t *testing.T) {
	a := ScaledIntFromInt64(1234, 2) // 12.34
	a.Align(0)                       // shallower: ignored

	assert.Equal(t, 2, a.Scale())
	assert.Equal(t, "1234", a.BigInt().String())
}

func TestScaledIntAlignPanicsOnNegativeScale(t *testing.T) {
	assert.Panics(t, func() { NewScaledInt().Align(-1) })
}

func TestScaledIntCmpIgnoresScale(t *testing.T) {
	// 1.5 at scale 1 vs 1.500 at scale 3 are equal.
	assert.Equal(t, 0, ScaledIntFromInt64(15, 1).Cmp(ScaledIntFromInt64(1500, 3)))
	assert.Equal(t, -1, ScaledIntFromInt64(15, 1).Cmp(ScaledIntFromInt64(1501, 3)))
	assert.Equal(t, 1, ScaledIntFromInt64(1501, 3).Cmp(ScaledIntFromInt64(15, 1)))
}

func TestScaledIntCmpDoesNotMutateOperands(t *testing.T) {
	a := ScaledIntFromInt64(15, 1)
	b := ScaledIntFromInt64(1500, 3)
	a.Cmp(b)

	assert.Equal(t, "15", a.BigInt().String())
	assert.Equal(t, 1, a.Scale())
	assert.Equal(t, "1500", b.BigInt().String())
	assert.Equal(t, 3, b.Scale())
}

func TestScaledIntTruncatedToDeeperScaleIsLossless(t *testing.T) {
	restated := ScaledIntFromInt64(-1234, 2).TruncatedTo(5)

	assert.Equal(t, 5, restated.Scale())
	assert.Equal(t, "-1234000", restated.BigInt().String())
}

func TestScaledIntTruncatedToShallowerScaleTruncatesTowardZero(t *testing.T) {
	assert.Equal(t, "12", ScaledIntFromInt64(1299, 4).TruncatedTo(2).BigInt().String())
	assert.Equal(t, "-12", ScaledIntFromInt64(-1299, 4).TruncatedTo(2).BigInt().String())
}

func TestScaledIntTruncatedToLeavesReceiverUnchanged(t *testing.T) {
	a := ScaledIntFromInt64(1299, 4)
	a.TruncatedTo(0)

	assert.Equal(t, 4, a.Scale())
	assert.Equal(t, "1299", a.BigInt().String())
}

// ---------------------------------------------------------------------------
// ScaledInt overflow reporting
// ---------------------------------------------------------------------------

func TestScaledIntInt64RejectsOverflow(t *testing.T) {
	a := ScaledIntFromInt64(math.MaxInt64, 0)
	a.AddInt64(1, 0)

	_, err := a.Int64()
	require.ErrorIs(t, err, ErrInt64Range)
}

func TestScaledIntInt64ReturnsValueInRange(t *testing.T) {
	value, err := ScaledIntFromInt64(math.MinInt64, 2).Int64()
	require.NoError(t, err)
	assert.Equal(t, int64(math.MinInt64), value)
}

func TestScaledIntCoefficientRejectsOverflow(t *testing.T) {
	// 10^38 — 39 digits, one past the storage boundary.
	tooWide, ok := new(big.Int).SetString("1"+strings.Repeat("0", 38), 10)
	require.True(t, ok)
	a := ScaledIntFromBig(tooWide, 0)

	_, err := a.Coefficient()
	require.Error(t, err, "Coefficient must reject values exceeding MaxCoefficientDigits")
}

func TestScaledIntCryptoScaleAlignment(t *testing.T) {
	// Mix a scale-2 fiat posting with a scale-24 crypto posting for the SAME
	// commodity (unusual but must not panic or corrupt).
	a := NewScaledInt()
	a.AddCoefficient(New(1), 2)  // 0.01 at scale 2
	a.AddCoefficient(New(1), 24) // 10^-24 at scale 24

	// After alignment both are at scale 24.
	// 0.01 = 1 * 10^-2 → coefficient 10^22 at scale 24
	// 10^-24 → coefficient 1 at scale 24
	// Total coefficient = 10^22 + 1
	got, err := a.Coefficient()
	require.NoError(t, err)

	expected, _ := new(big.Int).SetString("10000000000000000000001", 10) // 10^22 + 1
	assert.Equal(t, expected.String(), got.String())
	assert.Equal(t, 24, a.Scale())
}

func TestScaledIntAddsAtThirtyEightDigitBoundary(t *testing.T) {
	// The largest positive 38-digit coefficient remains valid. Arithmetic is
	// performed with big.Int, so no machine-integer overflow occurs first.
	a := NewScaledInt()
	a.AddCoefficient(MustParse("99999999999999999999999999999999999998"), 24)
	a.AddCoefficient(New(1), 24)

	got, err := a.Coefficient()
	require.NoError(t, err)
	assert.Equal(t, "99999999999999999999999999999999999999", got.String())
}

func TestScaledIntReportsThirtyNineDigitResult(t *testing.T) {
	// Crossing the documented 38-digit storage boundary is rejected instead
	// of wrapping, even though the intermediate big.Int remains exact.
	a := NewScaledInt()
	a.AddCoefficient(MustParse("99999999999999999999999999999999999999"), 24)
	a.AddCoefficient(New(1), 24)

	_, err := a.Coefficient()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "at most 38 digits")
}

// ---------------------------------------------------------------------------
// Pow10
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
		got := Pow10(tc.scale)
		assert.Equal(t, tc.expected, got.String(), "Pow10(%d)", tc.scale)
	}
}

func TestPow10ReturnsFreshValue(t *testing.T) {
	// Callers mutate the result in place; the cache must not be corrupted.
	first := Pow10(3)
	first.Add(first, big.NewInt(1))

	assert.Equal(t, "1000", Pow10(3).String())
}

func TestPow10PanicsOnNegativeScale(t *testing.T) {
	assert.Panics(t, func() { Pow10(-1) }, "Pow10 must panic on negative scale")
}

func TestPow10PanicsOnExcessiveScale(t *testing.T) {
	assert.Panics(t, func() { Pow10(MaxPow10Scale + 1) }, "Pow10 must panic when scale > MaxPow10Scale")
}
