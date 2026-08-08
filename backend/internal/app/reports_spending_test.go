package app

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rekenraam/backend/internal/exact"
)

func quantity(t *testing.T, commodityID int64, normalValue string, scale int) BalanceQuantity {
	t.Helper()
	coefficient, err := exact.Parse(normalValue)
	require.NoError(t, err)
	return BalanceQuantity{
		CommodityID:         commodityID,
		QuantityValue:       coefficient,
		QuantityScale:       scale,
		NormalQuantityValue: coefficient,
	}
}

func TestShareBasisPointsIsExactAndRoundsHalfAwayFromZero(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		group BalanceQuantity
		total BalanceQuantity
		want  int64
	}{
		{
			name:  "a quarter of the total is 2500 basis points",
			group: quantity(t, 1, "2500", 2),
			total: quantity(t, 1, "10000", 2),
			want:  2500,
		},
		{
			name:  "the whole total is 10000 basis points",
			group: quantity(t, 1, "10000", 2),
			total: quantity(t, 1, "10000", 2),
			want:  10000,
		},
		{
			// A group and the report-wide total may carry different scales.
			// Dividing without aligning first would report 100x the real share.
			name:  "group and total at different scales align before dividing",
			group: quantity(t, 1, "500", 2),
			total: quantity(t, 1, "100000", 4),
			want:  5000,
		},
		{
			name:  "one third rounds half away from zero to 3333",
			group: quantity(t, 1, "100", 2),
			total: quantity(t, 1, "300", 2),
			want:  3333,
		},
		{
			// 1/32 = 312.5 basis points exactly. Banker's rounding would give
			// 312 (round half to even); half-away-from-zero gives 313.
			name:  "a positive .5 remainder rounds away from zero, not to even",
			group: quantity(t, 1, "1", 0),
			total: quantity(t, 1, "32", 0),
			want:  313,
		},
		{
			// The mirror of the case above: -312.5 must go to -313, not -312.
			// Truncating integer division in Go would silently give -312.
			name:  "a negative .5 remainder rounds away from zero too",
			group: quantity(t, 1, "-1", 0),
			total: quantity(t, 1, "32", 0),
			want:  -313,
		},
		{
			// A negative denominator arises when the whole report nets to an
			// inflow. The sign of the share must follow the ratio, not the
			// numerator alone.
			name:  "a negative report total still yields a correctly signed share",
			group: quantity(t, 1, "-2500", 2),
			total: quantity(t, 1, "-10000", 2),
			want:  2500,
		},
		{
			name:  "a refund makes a group's share negative",
			group: quantity(t, 1, "-2500", 2),
			total: quantity(t, 1, "10000", 2),
			want:  -2500,
		},
		{
			// A share of nothing has no meaning; returning 0 is the only
			// answer that does not invent one.
			name:  "a zero report total yields a zero share rather than dividing",
			group: quantity(t, 1, "2500", 2),
			total: quantity(t, 1, "0", 2),
			want:  0,
		},
		{
			// Coefficients are arbitrary-precision decimal strings. A share
			// computed through float64 would lose the low digits here.
			name:  "a coefficient beyond int64 still yields an exact share",
			group: quantity(t, 1, "92233720368547758080", 2),
			total: quantity(t, 1, "184467440737095516160", 2),
			want:  5000,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.want, shareBasisPoints(test.group, test.total))
		})
	}
}

func TestRankingCommodityPicksLargestMagnitudeAcrossScales(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		totals []BalanceQuantity
		want   int64
	}{
		{
			name:   "an empty report has no ranking commodity",
			totals: []BalanceQuantity{},
			want:   0,
		},
		{
			name:   "a single commodity ranks itself",
			totals: []BalanceQuantity{quantity(t, 7, "100", 2)},
			want:   7,
		},
		{
			// 1.00 EUR (scale 2) vs 0.000000000000000002 of a crypto
			// (scale 18). Comparing raw coefficients would rank the crypto
			// higher purely because it carries more digits.
			name: "a high-scale tiny holding does not outrank a larger low-scale total",
			totals: []BalanceQuantity{
				quantity(t, 1, "100", 2),
				quantity(t, 2, "2", 18),
			},
			want: 1,
		},
		{
			name: "magnitude ignores sign so a net inflow can still rank",
			totals: []BalanceQuantity{
				quantity(t, 1, "-90000", 2),
				quantity(t, 2, "100", 2),
			},
			want: 1,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.want, rankingCommodity(test.totals))
		})
	}
}

func TestSortSpendingGroupsRanksWithinOneCommodity(t *testing.T) {
	t.Parallel()

	group := func(categoryID int64, totals ...SpendingGroupTotal) SpendingGroup {
		id := categoryID
		return SpendingGroup{CategoryAccountID: &id, Totals: totals}
	}
	total := func(commodityID int64, value string, scale int) SpendingGroupTotal {
		return SpendingGroupTotal{BalanceQuantity: quantity(t, commodityID, value, scale)}
	}

	t.Run("groups rank by descending magnitude in the ranking commodity", func(t *testing.T) {
		groups := []SpendingGroup{
			group(1, total(1, "500", 2)),
			group(2, total(1, "9000", 2)),
			group(3, total(1, "1200", 2)),
		}

		sortSpendingGroups(groups, 1)

		assert.Equal(t, []int64{2, 3, 1}, spendingCategoryIDs(groups))
	})

	t.Run("a group with no total in the ranking commodity sorts after those that have one", func(t *testing.T) {
		groups := []SpendingGroup{
			group(1, total(2, "999999", 2)),
			group(2, total(1, "100", 2)),
		}

		sortSpendingGroups(groups, 1)

		assert.Equal(t, []int64{2, 1}, spendingCategoryIDs(groups))
	})

	t.Run("differing scales are aligned before comparing magnitudes", func(t *testing.T) {
		// 5.00 vs 12.0000 — comparing coefficients raw would put 500 behind
		// 120000 incorrectly ordered only by digit count.
		groups := []SpendingGroup{
			group(1, total(1, "500", 2)),
			group(2, total(1, "120000", 4)),
		}

		sortSpendingGroups(groups, 1)

		assert.Equal(t, []int64{2, 1}, spendingCategoryIDs(groups))
	})

	t.Run("equal magnitudes fall back to a stable identity order", func(t *testing.T) {
		groups := []SpendingGroup{
			group(9, total(1, "100", 2)),
			group(4, total(1, "100", 2)),
		}

		sortSpendingGroups(groups, 1)

		assert.Equal(t, []int64{4, 9}, spendingCategoryIDs(groups))
	})

	t.Run("the unassigned bucket sorts last among equals", func(t *testing.T) {
		groups := []SpendingGroup{
			{Unassigned: true, Totals: []SpendingGroupTotal{total(1, "100", 2)}},
			group(1, total(1, "100", 2)),
		}

		sortSpendingGroups(groups, 1)

		require.NotNil(t, groups[0].CategoryAccountID)
		assert.True(t, groups[1].Unassigned)
	})
}

func spendingCategoryIDs(groups []SpendingGroup) []int64 {
	ids := make([]int64, 0, len(groups))
	for _, group := range groups {
		if group.CategoryAccountID != nil {
			ids = append(ids, *group.CategoryAccountID)
		}
	}
	return ids
}

func TestCleanSpendingInputRejectsUnsupportedQueries(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		input     SpendingReportInput
		wantError string
	}{
		{
			name:      "an inverted range is rejected rather than returning nothing",
			input:     SpendingReportInput{StartDate: "2026-03-01", EndDate: "2026-02-01"},
			wantError: "start date must be on or before end date",
		},
		{
			name:      "an unknown grouping is rejected rather than silently defaulted",
			input:     SpendingReportInput{StartDate: "2026-01-01", EndDate: "2026-01-31", GroupBy: "tag"},
			wantError: "group by must be category or payee",
		},
		{
			name:      "an unknown direction is rejected",
			input:     SpendingReportInput{StartDate: "2026-01-01", EndDate: "2026-01-31", Direction: "transfer"},
			wantError: "direction must be expense or income",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, _, _, _, err := cleanSpendingInput(test.input)

			var validationErr ValidationError
			require.ErrorAs(t, err, &validationErr)
			assert.Equal(t, test.wantError, validationErr.Message)
		})
	}
}

func TestCleanSpendingInputDefaultsToExpenseByCategory(t *testing.T) {
	t.Parallel()

	startDate, endDate, groupBy, direction, err := cleanSpendingInput(SpendingReportInput{
		StartDate: "2026-01-01",
		EndDate:   "2026-01-31",
	})

	require.NoError(t, err)
	assert.Equal(t, "2026-01-01", startDate)
	assert.Equal(t, "2026-01-31", endDate)
	assert.Equal(t, "category", groupBy)
	assert.Equal(t, "expense", direction)
}
