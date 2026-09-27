package db

import (
	"testing"

	"github.com/stretchr/testify/require"
	"rekenraam/backend/internal/exact"
)

func TestAllocateDisposalProceedsConservesMixedScaleRemainder(t *testing.T) {
	for _, proceeds := range []int64{10001, -10001} {
		allocations := []LotDisposalRecord{
			{QuantityValue: exact.New(1), QuantityScale: 0, CostBasisScale: 6},
			{QuantityValue: exact.New(100), QuantityScale: 2, CostBasisScale: 6},
			{QuantityValue: exact.New(1000), QuantityScale: 3, CostBasisScale: 6},
		}
		require.NoError(t, allocateDisposalProceeds(allocations, proceeds, 2))
		wantFirst := int64(33336666)
		wantLast := int64(33336668)
		if proceeds < 0 {
			wantFirst = -wantFirst
			wantLast = -wantLast
		}
		require.Equal(t, wantFirst, allocations[0].ProceedsValue)
		require.Equal(t, wantFirst, allocations[1].ProceedsValue)
		require.Equal(t, wantLast, allocations[2].ProceedsValue)
		for _, allocation := range allocations {
			require.Equal(t, 6, allocation.ProceedsScale)
		}
		require.Equal(t, proceeds*10000, allocations[0].ProceedsValue+allocations[1].ProceedsValue+allocations[2].ProceedsValue)
	}
}

func TestAllocateDisposalProceedsUsesDeepestInt64Scale(t *testing.T) {
	allocations := []LotDisposalRecord{{QuantityValue: exact.New(1), CostBasisScale: 6}}
	require.NoError(t, allocateDisposalProceeds(allocations, 9223372036854775807, 2))
	require.Equal(t, int64(9223372036854775807), allocations[0].ProceedsValue)
	require.Equal(t, 2, allocations[0].ProceedsScale)
}
