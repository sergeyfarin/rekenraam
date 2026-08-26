package exact

import (
	"math/big"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func bigOf(t *testing.T, value string) *big.Int {
	t.Helper()
	parsed, ok := new(big.Int).SetString(value, 10)
	require.True(t, ok, value)
	return parsed
}

func TestMulDivRoundConvertsAtTheRequestedScale(t *testing.T) {
	t.Parallel()

	for name, testCase := range map[string]struct {
		value, multiplier, divisor          string
		valueScale, mulScale, divScale, out int
		want                                string
	}{
		// 100.00 USD at 0.920000 EUR/USD, per 1 unit, to 2 places = 92.00.
		"ordinary conversion": {
			value: "10000", valueScale: 2,
			multiplier: "920000", mulScale: 6,
			divisor: "1", divScale: 0,
			out: 2, want: "9200",
		},
		// A price quoted per 100 units divides by the base quantity.
		"price quoted per hundred": {
			value: "10000", valueScale: 2,
			multiplier: "9200", mulScale: 2,
			divisor: "100", divScale: 0,
			out: 2, want: "9200",
		},
		// 0.005 at scale 2 is the rounding boundary: half goes away from zero.
		"exact half rounds up": {
			value: "1", valueScale: 0,
			multiplier: "5", mulScale: 1,
			divisor: "1", divScale: 0,
			out: 0, want: "1",
		},
		"negative exact half rounds away from zero": {
			value: "-1", valueScale: 0,
			multiplier: "5", mulScale: 1,
			divisor: "1", divScale: 0,
			out: 0, want: "-1",
		},
		"just under half rounds down": {
			value: "49", valueScale: 2,
			multiplier: "1", mulScale: 0,
			divisor: "1", divScale: 0,
			out: 0, want: "0",
		},
		// Deepening rather than truncating: a scale-0 amount converted to 8
		// places keeps every digit the rate carries.
		"result deeper than the inputs": {
			value: "3", valueScale: 0,
			multiplier: "333333", mulScale: 6,
			divisor: "1", divScale: 0,
			out: 8, want: "99999900",
		},
		// Beyond int64 on purpose: this is what app.scaledDivision cannot do.
		"value wider than int64": {
			value: "123456789012345678901234567890", valueScale: 2,
			multiplier: "2000000", mulScale: 6,
			divisor: "1", divScale: 0,
			out: 2, want: "246913578024691357802469135780",
		},
	} {
		got, err := MulDivRound(
			bigOf(t, testCase.value), testCase.valueScale,
			bigOf(t, testCase.multiplier), testCase.mulScale,
			bigOf(t, testCase.divisor), testCase.divScale,
			testCase.out,
		)
		require.NoErrorf(t, err, name)
		assert.Equalf(t, testCase.want, got.String(), name)
	}
}

// A credit and a debit of the same size must convert to the same size, or a
// converted total stops balancing in a way nobody can find.
func TestMulDivRoundIsSymmetricAroundZero(t *testing.T) {
	t.Parallel()

	for _, amount := range []string{"12345", "1", "99999999", "5"} {
		positive, err := MulDivRound(bigOf(t, amount), 2, bigOf(t, "333333"), 6, big.NewInt(1), 0, 2)
		require.NoError(t, err)
		negative, err := MulDivRound(bigOf(t, "-"+amount), 2, bigOf(t, "333333"), 6, big.NewInt(1), 0, 2)
		require.NoError(t, err)

		assert.Equalf(t, positive.String(), new(big.Int).Neg(negative).String(),
			"converting %s and -%s must produce the same magnitude", amount, amount)
	}
}

func TestMulDivRoundRefusesTheUnanswerable(t *testing.T) {
	t.Parallel()

	_, err := MulDivRound(big.NewInt(1), 0, big.NewInt(1), 0, big.NewInt(0), 0, 2)
	require.Error(t, err, "a zero rate is not a rate")

	_, err = MulDivRound(nil, 0, big.NewInt(1), 0, big.NewInt(1), 0, 2)
	require.Error(t, err)

	_, err = MulDivRound(big.NewInt(1), -1, big.NewInt(1), 0, big.NewInt(1), 0, 2)
	require.Error(t, err, "a negative scale is a bug upstream, not a value to interpret")
}

// Zero converts to zero at any rate and any scale — the case a report hits far
// more often than any other.
func TestMulDivRoundConvertsZeroExactly(t *testing.T) {
	t.Parallel()

	got, err := MulDivRound(big.NewInt(0), 2, bigOf(t, "920000"), 6, big.NewInt(1), 0, 4)
	require.NoError(t, err)
	assert.Equal(t, "0", got.String())
}
