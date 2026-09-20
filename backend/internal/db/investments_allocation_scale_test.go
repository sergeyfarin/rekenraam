package db

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// T-103. The scale a partial disposal splits basis at is a policy, and a
// policy has to be decided by something other than how a purchase was typed.
// These pin the three rules that make it one: the commodity's ceiling decides
// it, a basis already recorded deeper than the ceiling is never narrowed to
// reach it, and it backs off only far enough to keep the position inside the
// int64 its projection columns are.
func TestCostBasisAllocationScaleIsDecidedByTheCommodityNotTheInput(t *testing.T) {
	// The same 10 EUR, written three ways. All three must reach the ceiling.
	for _, recorded := range []recordedBasis{
		{value: 10, scale: 0},
		{value: 1000, scale: 2},
		{value: 100000, scale: 4},
	} {
		assert.Equal(t, 6, costBasisAllocationScale(6, []recordedBasis{recorded}),
			"%d at scale %d", recorded.value, recorded.scale)
	}
}

func TestCostBasisAllocationScaleNeverNarrowsARecordedBasis(t *testing.T) {
	// A basis recorded deeper than the commodity's current ceiling keeps its
	// own precision: reaching the ceiling would mean truncating money that is
	// already on the books.
	assert.Equal(t, 8, costBasisAllocationScale(2, []recordedBasis{{value: 1000000001, scale: 8}}))
}

func TestCostBasisAllocationScaleBacksOffForRangeOnMagnitudeNotSpelling(t *testing.T) {
	// 90 trillion units at scale 2 cannot be restated to scale 6 in an int64
	// (it would need 9e18 × 10 000), so the policy steps back until it fits.
	large := []recordedBasis{{value: 90_000_000_000_000_00, scale: 2}}
	scale := costBasisAllocationScale(6, large)
	assert.Less(t, scale, 6, "the ceiling does not fit this magnitude")
	assert.GreaterOrEqual(t, scale, 2, "and it never goes below what is already recorded")
	assert.True(t, basisFitsInt64At(large, scale))
	assert.False(t, basisFitsInt64At(large, scale+1), "it backs off exactly as far as it must")

	// Written with more decimals, the same amount reaches the same scale —
	// which is the property the whole policy exists to hold.
	sameAmountDeeper := []recordedBasis{{value: 90_000_000_000_000_0000, scale: 4}}
	assert.Equal(t, scale, costBasisAllocationScale(6, sameAmountDeeper))
}

func TestCostBasisAllocationScaleConsidersThePositionsTotal(t *testing.T) {
	// Two lots chosen so that each one alone can be restated to the ceiling
	// but their sum cannot. A position's basis is added up — by the projection
	// rows a disposal rewrites and by everything that reports a position — so
	// a scale at which only the parts fit is no use.
	lots := []recordedBasis{{value: 500_000_000_000_000, scale: 2}, {value: 500_000_000_000_000, scale: 2}}
	require.True(t, basisFitsInt64At(lots[:1], 6), "one lot of the pair does fit the ceiling")
	require.Equal(t, 6, costBasisAllocationScale(6, lots[:1]))

	assert.Less(t, costBasisAllocationScale(6, lots), 6, "the pair does not, so the pair does not get the ceiling")
	assert.True(t, basisFitsInt64At(lots, costBasisAllocationScale(6, lots)))
}

func TestBasisFitsInt64AtRejectsAnOverflowingRestatement(t *testing.T) {
	require.True(t, basisFitsInt64At([]recordedBasis{{value: math.MaxInt64, scale: 6}}, 6))
	require.False(t, basisFitsInt64At([]recordedBasis{{value: math.MaxInt64, scale: 6}}, 7))
}
