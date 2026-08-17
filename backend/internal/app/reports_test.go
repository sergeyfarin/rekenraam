package app

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rekenraam/backend/internal/exact"
)

func quantity(value string, scale int) BalanceQuantity {
	return BalanceQuantity{
		CommodityID:         1,
		QuantityValue:       exact.MustParse(value),
		QuantityScale:       scale,
		NormalQuantityValue: exact.MustParse(value),
	}
}

// Shares are integer-only arithmetic: no float ever touches the money path, and
// rounding is half-up on magnitude so a printed report reproduces exactly.
func TestSpendingShareBasisPointsRoundsHalfUpWithoutFloats(t *testing.T) {
	t.Parallel()

	for name, testCase := range map[string]struct {
		value BalanceQuantity
		total BalanceQuantity
		want  int64
	}{
		"whole quarter":          {quantity("10000", 2), quantity("40000", 2), 2500},
		"whole three quarters":   {quantity("30000", 2), quantity("40000", 2), 7500},
		"entire total":           {quantity("5000", 2), quantity("5000", 2), 10000},
		"one third rounds up":    {quantity("100", 2), quantity("300", 2), 3333},
		"two thirds rounds up":   {quantity("200", 2), quantity("300", 2), 6667},
		"exact half rounds up":   {quantity("1", 0), quantity("20000", 0), 1},
		"mixed scales align":     {quantity("1000", 3), quantity("400", 2), 2500},
		"negative group is kept": {quantity("-10000", 2), quantity("40000", 2), -2500},
		"negative denominator":   {quantity("10000", 2), quantity("-40000", 2), -2500},
	} {
		share, err := shareBasisPoints(testCase.value, testCase.total)
		require.NoError(t, err, name)
		require.NotNil(t, share, name)
		assert.Equal(t, testCase.want, *share, name)
	}
}

func TestSpendingShareBasisPointsOmitsShareWithoutUsableTotal(t *testing.T) {
	t.Parallel()

	zeroTotal, err := shareBasisPoints(quantity("10000", 2), quantity("0", 2))
	require.NoError(t, err)
	assert.Nil(t, zeroTotal, "a zero commodity total must not produce a fabricated share")

	absentTotal, err := shareBasisPoints(quantity("10000", 2), BalanceQuantity{CommodityID: 1})
	require.NoError(t, err)
	assert.Nil(t, absentTotal)
}

func TestReportMagnitudesNegatesCreditNormalIncomeOnly(t *testing.T) {
	t.Parallel()

	amounts := map[int64]*exact.ScaledInt{
		1: exact.ScaledIntFromInt64(-200000, 2),
	}

	expense, err := reportMagnitudes(amounts, false)
	require.NoError(t, err)
	require.Len(t, expense, 1)
	assert.Equal(t, "-200000", expense[0].QuantityValue.String())

	income, err := reportMagnitudes(amounts, true)
	require.NoError(t, err)
	require.Len(t, income, 1)
	assert.Equal(t, "200000", income[0].QuantityValue.String(),
		"income is credit-normal in the ledger and reports as a positive magnitude")
}

func TestDedupeIDsSortsAndDropsNonPositiveDuplicates(t *testing.T) {
	t.Parallel()

	assert.Equal(t, []int64{1, 4, 7}, dedupeIDs([]int64{7, 4, 1, 4, 7}))
	assert.Nil(t, dedupeIDs([]int64{0, -3}))
	assert.Nil(t, dedupeIDs(nil))
}
