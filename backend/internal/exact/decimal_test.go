package exact

import (
	"encoding/json"
	"math/big"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

// decimalVectorFile is the one specification behind two implementations:
// exact.Decimal here and formatLedgerAmount in the frontend's $lib/money. The
// two cannot share code across languages, so they share this file instead — a
// change to either that drifts from the other fails here by name.
const decimalVectorFile = "../../../fixtures/decimal-rendering.json"

func TestDecimalMatchesTheSharedRenderingVectors(t *testing.T) {
	t.Parallel()

	raw, err := os.ReadFile(decimalVectorFile)
	require.NoError(t, err, "the shared decimal vectors must be readable from the Go suite")

	var fixture struct {
		Vectors []struct {
			Name        string `json:"name"`
			Coefficient string `json:"coefficient"`
			Scale       int    `json:"scale"`
			Expected    string `json:"expected"`
		} `json:"vectors"`
	}
	require.NoError(t, json.Unmarshal(raw, &fixture))
	require.NotEmpty(t, fixture.Vectors)

	for _, vector := range fixture.Vectors {
		assert.Equalf(t, vector.Expected, Decimal(MustParse(vector.Coefficient), vector.Scale), vector.Name)
	}
}

// DecimalFromBig renders values the Coefficient gate would refuse. A balance
// can legitimately need more than 38 digits — the ledger holds a 38-digit
// posting and a 2.50 posting quite legally, and their sum needs 39 — and an
// export writes text, not a stored coefficient.
func TestDecimalFromBigRendersBeyondTheCoefficientCeiling(t *testing.T) {
	t.Parallel()

	wide, ok := new(big.Int).SetString(strings.Repeat("9", 38), 10)
	require.True(t, ok)
	// (10^38 - 1) at scale 0, restated at scale 2, plus 250: the sum carries
	// into a 41-character coefficient.
	sum := new(big.Int).Mul(wide, big.NewInt(100))
	sum.Add(sum, big.NewInt(250))

	rendered := DecimalFromBig(sum, 2)
	assert.Equal(t, "100000000000000000000000000000000000001.50", rendered)

	_, err := Parse(sum.String())
	require.Error(t, err, "the same value is refused as a stored coefficient, which is the distinction being tested")
}

func TestDecimalFromBigTreatsNilAsZero(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "0.00", DecimalFromBig(nil, 2))
}
