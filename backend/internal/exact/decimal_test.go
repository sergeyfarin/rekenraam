package exact

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDecimalKeepsTheStoredScale(t *testing.T) {
	t.Parallel()

	for name, testCase := range map[string]struct {
		value Coefficient
		scale int
		want  string
	}{
		"whole units at scale zero":     {MustParse("1200"), 0, "1200"},
		"two decimal places":            {MustParse("1200"), 2, "12.00"},
		"negative keeps its sign":       {MustParse("-12345"), 2, "-123.45"},
		"value narrower than the scale": {MustParse("5"), 3, "0.005"},
		"exactly as wide as the scale":  {MustParse("-25"), 2, "-0.25"},
		"zero renders its scale":        {MustParse("0"), 2, "0.00"},
		"zero at scale zero":            {MustParse("0"), 0, "0"},
		"crypto scale stays exact":      {MustParse("1"), 24, "0.000000000000000000000001"},
		"beyond int64 range":            {MustParse("123456789012345678901234567890"), 4, "12345678901234567890123456.7890"},
	} {
		assert.Equal(t, testCase.want, Decimal(testCase.value, testCase.scale), name)
	}
}

// A negative scale is not reachable through the schema, but rendering must not
// silently produce a separator in the wrong place if one ever arrives.
func TestDecimalTreatsANegativeScaleAsWhole(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "-42", Decimal(MustParse("-42"), -3))
}
