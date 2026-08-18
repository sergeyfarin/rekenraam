package app

import (
	"database/sql"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rekenraam/backend/internal/db"
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

func TestNormalizeReportFiltersIsIdempotent(t *testing.T) {
	t.Parallel()

	filters := ReportFilters{
		AccountIDs:         []int64{7, 4, 0, 4},
		IncludeDescendants: true,
		CommodityIDs:       []int64{2, -1, 2},
	}
	once := normalizeReportFilters(filters)

	assert.Equal(t, []int64{4, 7}, once.AccountIDs)
	assert.Equal(t, []int64{2}, once.CommodityIDs)
	assert.True(t, once.IncludeDescendants)
	assert.Equal(t, once, normalizeReportFilters(once),
		"a caller may normalize once and re-expand later; the two passes must agree")
}

func TestReportFilterSetTreatsAnEmptySelectionAsUnrestricted(t *testing.T) {
	t.Parallel()

	// An empty selection means "no restriction on that dimension". An empty
	// non-nil set would mean "match nothing" and silently zero out a report.
	assert.Nil(t, idSet(nil))
	assert.Nil(t, idSet([]int64{}))

	unrestricted := reportFilterSet{accountIDs: idSet(nil), commodityIDs: idSet(nil)}
	assert.True(t, unrestricted.includes(11, 22))
}

func TestReportFilterSetRequiresEveryRestrictedDimension(t *testing.T) {
	t.Parallel()

	set := reportFilterSet{accountIDs: idSet([]int64{1, 2}), commodityIDs: idSet([]int64{9})}

	assert.True(t, set.includes(1, 9))
	assert.False(t, set.includes(3, 9), "account outside the selection is excluded")
	assert.False(t, set.includes(1, 8), "commodity outside the selection is excluded")
}

func TestEchoedReportFiltersRoundTripsTheNormalizedSelection(t *testing.T) {
	t.Parallel()

	echo := ReportFiltersEcho{
		AccountIDs:         []int64{4, 7},
		IncludeDescendants: true,
		CommodityIDs:       []int64{2},
		ResolvedAccountIDs: []int64{4, 7, 12},
	}

	assert.Equal(t, ReportFilters{
		AccountIDs:         []int64{4, 7},
		IncludeDescendants: true,
		CommodityIDs:       []int64{2},
	}, echoedReportFilters(echo),
		"the series re-expands the requested selection per bucket, not the end-date expansion")
}

func TestReportFilterSetFromExpandsDescendantsOnlyWhenAsked(t *testing.T) {
	t.Parallel()

	accounts := []db.LedgerAccountRecord{
		{ID: 1},
		{ID: 2, ParentAccountID: sql.NullInt64{Int64: 1, Valid: true}},
		{ID: 3, ParentAccountID: sql.NullInt64{Int64: 2, Valid: true}},
		{ID: 4},
	}

	flat := reportFilterSetFrom(accounts, ReportFilters{AccountIDs: []int64{1}})
	assert.True(t, flat.includes(1, 0))
	assert.False(t, flat.includes(2, 0), "a child is not selected without include_descendants")

	deep := reportFilterSetFrom(accounts, ReportFilters{AccountIDs: []int64{1}, IncludeDescendants: true})
	assert.True(t, deep.includes(2, 0))
	assert.True(t, deep.includes(3, 0), "expansion reaches grandchildren")
	assert.False(t, deep.includes(4, 0))
}

// The two report filter paths must agree on what an unusable selection means.
// SQL-side filtering (spending) treats it as "no filter"; the in-Go path used to
// build an empty non-nil set from it and silently return an all-zero report.
func TestReportFilterSetFromTreatsAnUnusableSelectionAsNoFilter(t *testing.T) {
	t.Parallel()

	filters := ReportFilters{AccountIDs: []int64{0, -1}, CommodityIDs: []int64{-2}}

	set := reportFilterSetFrom([]db.LedgerAccountRecord{{ID: 1}}, filters)

	assert.True(t, set.includes(1, 5), "an unusable selection restricts nothing, it does not zero the report")
	assert.Nil(t, normalizeReportFilters(filters).AccountIDs,
		"the SQL-side path sees the same empty selection and applies no filter")
}
