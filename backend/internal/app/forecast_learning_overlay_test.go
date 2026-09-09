package app

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rekenraam/backend/internal/db"
	"rekenraam/backend/internal/exact"
)

const (
	overlayAsOf    = "2026-06-30"
	overlayThrough = "2026-07-30"
)

// overlayHistory posts one grocery purchase a week for the given number of
// completed weeks before the as-of date, all funded from the checking account.
func overlayHistory(t testing.TB, weeks int, amount string) []db.ForecastLearningPostingRecord {
	t.Helper()
	origin, err := time.Parse(time.DateOnly, overlayAsOf)
	require.NoError(t, err)
	start := learningPeriodStart(origin, "week")
	rows := make([]db.ForecastLearningPostingRecord, 0, weeks*2)
	for i := weeks; i >= 1; i-- {
		date := start.AddDate(0, 0, -7*i).Format(time.DateOnly)
		id := int64(1000 + i)
		rows = append(rows, learningRows(id, id, "ordinary", "ordinary", date,
			learningLeg(1, "-"+amount), learningLeg(3, amount))...)
	}
	return rows
}

// overlayHistoryStart is the Monday the generated history begins on. Confirmed
// coverage must match it: an earlier date would add complete zero-spend weeks.
func overlayHistoryStart(t testing.TB, weeks int) string {
	t.Helper()
	origin, err := time.Parse(time.DateOnly, overlayAsOf)
	require.NoError(t, err)
	return learningPeriodStart(origin, "week").AddDate(0, 0, -7*weeks).Format(time.DateOnly)
}

func overlaySnapshot(t testing.TB, postings []db.ForecastLearningPostingRecord) db.ForecastSnapshot {
	t.Helper()
	accounts, commodities := learningAccounts()
	return db.ForecastSnapshot{
		TimeZone: "UTC", AccountVersions: accounts, CommodityVersions: commodities,
		LearningPostings: postings, PayeeNames: map[int64]string{},
	}
}

// overlayCore is a minimal core result: one flat projected curve per day, which
// makes any estimated movement in the overlay easy to read off.
func overlayCore(t testing.TB, opening string) *ForecastResult {
	t.Helper()
	first, err := time.Parse(time.DateOnly, overlayAsOf)
	require.NoError(t, err)
	through, err := time.Parse(time.DateOnly, overlayThrough)
	require.NoError(t, err)
	points := make([]ForecastPoint, 0)
	for date := first.AddDate(0, 0, 1); !date.After(through); date = date.AddDate(0, 0, 1) {
		points = append(points, ForecastPoint{
			Date:             date.Format(time.DateOnly),
			ProjectedBalance: ForecastQuantity{Value: exact.MustParse(opening), Scale: 2},
			RecordedBalance:  ForecastQuantity{Value: exact.MustParse(opening), Scale: 2},
		})
	}
	return &ForecastResult{
		AsOfDate: overlayAsOf, FirstDate: first.AddDate(0, 0, 1).Format(time.DateOnly), ThroughDate: overlayThrough,
		Series: []ForecastSeries{{AccountID: 1, CommodityID: 1, Points: points}},
		Events: []ForecastEvent{},
	}
}

func overlayInput(historyFrom string, categories []int64, patterns ...forecastCategoryPattern) forecastNormalizedInput {
	return forecastNormalizedInput{
		HorizonDays: 30, IncludeDescendants: true,
		SpendingModel: ForecastSpendingModelAdaptiveV1, HistoryCompleteFrom: historyFrom,
		ExpenseCategoryIDs: categories, ExpensePatterns: patterns,
	}
}

func overlayBuild(t testing.TB, input forecastNormalizedInput, snapshot db.ForecastSnapshot, core *ForecastResult) *ForecastLearnedSpending {
	t.Helper()
	bounds := forecastBounds{AsOf: overlayAsOf, First: core.FirstDate, Through: overlayThrough}
	scope := forecastTestScope(snapshot, 1)
	overlay, err := buildForecastLearnedSpending(context.Background(), NewForecastLearningFitter(), input, bounds, scope, snapshot, core)
	require.NoError(t, err)
	require.NotNil(t, overlay)
	return overlay
}

// TestForecastLearningOptionValidation covers the whole opt-in query contract,
// including that every model parameter is an orphan while the model is off.
func TestForecastLearningOptionValidation(t *testing.T) {
	cases := []struct {
		name    string
		input   ForecastInput
		message string
	}{
		{
			name:    "unknown model",
			input:   ForecastInput{SpendingModel: "neural_v9"},
			message: "spending model must be off or adaptive_v1",
		},
		{
			name:    "history without the model",
			input:   ForecastInput{HistoryCompleteFrom: "2024-01-01"},
			message: "spending model parameters require spending_model=adaptive_v1",
		},
		{
			name:    "categories without the model",
			input:   ForecastInput{ExpenseCategoryIDs: []int64{3}},
			message: "spending model parameters require spending_model=adaptive_v1",
		},
		{
			name:    "patterns without the model",
			input:   ForecastInput{ExpensePatterns: []ForecastExpensePattern{{CategoryID: 3, Pattern: ForecastLearningWeekly}}},
			message: "spending model parameters require spending_model=adaptive_v1",
		},
		{
			name:    "model without confirmed history",
			input:   ForecastInput{SpendingModel: ForecastSpendingModelAdaptiveV1},
			message: "history_complete_from is required",
		},
		{
			name:    "unparseable history date",
			input:   ForecastInput{SpendingModel: ForecastSpendingModelAdaptiveV1, HistoryCompleteFrom: "01/01/2024"},
			message: "history_complete_from must be an ISO 8601 date",
		},
		{
			name: "unsupported pattern",
			input: ForecastInput{SpendingModel: ForecastSpendingModelAdaptiveV1, HistoryCompleteFrom: "2024-01-01",
				ExpensePatterns: []ForecastExpensePattern{{CategoryID: 3, Pattern: "quarterly"}}},
			message: "expense pattern must be daily, weekly, monthly or annual_seasonal",
		},
		{
			name: "duplicate override",
			input: ForecastInput{SpendingModel: ForecastSpendingModelAdaptiveV1, HistoryCompleteFrom: "2024-01-01",
				ExpensePatterns: []ForecastExpensePattern{{CategoryID: 3, Pattern: ForecastLearningWeekly}, {CategoryID: 3, Pattern: ForecastLearningMonthly}}},
			message: "expense pattern overrides must be unique per category",
		},
		{
			name: "override outside the selected categories",
			input: ForecastInput{SpendingModel: ForecastSpendingModelAdaptiveV1, HistoryCompleteFrom: "2024-01-01",
				ExpenseCategoryIDs: []int64{3}, ExpensePatterns: []ForecastExpensePattern{{CategoryID: 4, Pattern: ForecastLearningWeekly}}},
			message: "outside the selected expense categories",
		},
		{
			name:    "non-positive category",
			input:   ForecastInput{SpendingModel: ForecastSpendingModelAdaptiveV1, HistoryCompleteFrom: "2024-01-01", ExpenseCategoryIDs: []int64{0}},
			message: "expense category id is invalid",
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			_, err := normalizeForecastInput(testCase.input)
			require.ErrorContains(t, err, testCase.message)
		})
	}

	t.Run("too many categories", func(t *testing.T) {
		ids := make([]int64, ForecastLearningMaxCategories+1)
		for i := range ids {
			ids[i] = int64(i + 1)
		}
		_, err := normalizeForecastInput(ForecastInput{SpendingModel: ForecastSpendingModelAdaptiveV1, HistoryCompleteFrom: "2024-01-01", ExpenseCategoryIDs: ids})
		require.ErrorContains(t, err, "at most 20 expense categories")
	})

	t.Run("the model off keeps its pre-M3 meaning", func(t *testing.T) {
		normalized, err := normalizeForecastInput(ForecastInput{HorizonDays: 90})
		require.NoError(t, err)
		assert.Equal(t, ForecastSpendingModelOff, normalized.SpendingModel)
		assert.False(t, normalized.learningEnabled())
		assert.Empty(t, normalized.HistoryCompleteFrom)
	})

	t.Run("an absent override defaults to weekly", func(t *testing.T) {
		normalized, err := normalizeForecastInput(ForecastInput{
			SpendingModel: ForecastSpendingModelAdaptiveV1, HistoryCompleteFrom: "2024-01-01",
			ExpensePatterns: []ForecastExpensePattern{{CategoryID: 3, Pattern: ForecastLearningMonthly}},
		})
		require.NoError(t, err)
		assert.Equal(t, ForecastLearningMonthly, normalized.patternFor(3))
		assert.Equal(t, ForecastLearningWeekly, normalized.patternFor(4))
	})

	t.Run("only expense accounts may be selected", func(t *testing.T) {
		accounts, _ := learningAccounts()
		input := overlayInput("2024-01-01", []int64{1})
		require.ErrorContains(t, validateForecastLearningCategories(accounts, overlayAsOf, input), "postable expense account")
		require.NoError(t, validateForecastLearningCategories(accounts, overlayAsOf, overlayInput("2024-01-01", []int64{3})))
	})
}

// TestForecastLearningWindowMatchesRequestedCadences proves a weekly-only
// request does not read five years of history.
func TestForecastLearningWindowMatchesRequestedCadences(t *testing.T) {
	weekly, err := forecastLearningWindow(overlayInput("2015-01-01", nil), overlayAsOf)
	require.NoError(t, err)
	assert.Equal(t, "2025-06-30", weekly, "52 complete weeks back from the as-of week")

	seasonal, err := forecastLearningWindow(overlayInput("2015-01-01", []int64{3}, forecastCategoryPattern{CategoryID: 3, Pattern: ForecastLearningAnnualSeasonal}), overlayAsOf)
	require.NoError(t, err)
	assert.Equal(t, "2021-06-01", seasonal, "60 complete months back for a seasonal request")

	// A later confirmed coverage date bounds the read further; it never widens
	// it beyond the cadence's own lookback.
	input := overlayInput("2026-01-05", nil)
	bounded, err := forecastLearningWindow(input, overlayAsOf)
	require.NoError(t, err)
	assert.Equal(t, "2026-01-05", bounded)
}

func TestForecastLearningOverlayEstimatesEligibleGroups(t *testing.T) {
	snapshot := overlaySnapshot(t, overlayHistory(t, 20, "7000"))
	core := overlayCore(t, "100000")
	overlay := overlayBuild(t, overlayInput(overlayHistoryStart(t, 20), nil), snapshot, core)

	assert.Equal(t, ForecastLearningStatusReady, overlay.Status)
	assert.Equal(t, ForecastLearningPolicyVersion, overlay.PolicyVersion)
	assert.Equal(t, 1, overlay.RequestedGroupCount)
	assert.Equal(t, 1, overlay.EligibleGroupCount)
	assert.Empty(t, overlay.Exclusions)
	require.Len(t, overlay.Groups, 1)

	group := overlay.Groups[0]
	assert.Equal(t, int64(1), group.FundingAccountID)
	assert.Equal(t, int64(3), group.CategoryAccountID)
	assert.Equal(t, string(ForecastLearningWeekly), group.Pattern)
	assert.Equal(t, "week", group.PeriodUnit)
	assert.Equal(t, 20, group.CompletePeriods)
	assert.Equal(t, 20, group.PositivePeriods)
	// Flat history means the baseline is already exact, and it is labelled as
	// a baseline rather than dressed up as a learned model.
	assert.Equal(t, ForecastLearningMethodMean8, group.SelectedMethod)
	assert.False(t, group.SelectedIsLearned)
	assert.Equal(t, ForecastLearningFallbackPerfect, group.FallbackReason)
	assert.Positive(t, group.EstimatedTotal.Value.Sign(), "an eligible group produces some estimate")

	// Estimates only ever reduce a balance, and the core curve is untouched.
	require.Len(t, overlay.Series, 1)
	assert.Equal(t, len(core.Series[0].Points), len(overlay.Series[0].Points))
	for index, point := range overlay.Series[0].Points {
		assert.Equal(t, core.Series[0].Points[index].Date, point.Date)
		assert.LessOrEqual(t, point.EstimatedDelta.Value.Sign(), 0, "an estimate never invents income")
	}
	assert.Equal(t, "100000", core.Series[0].Points[0].ProjectedBalance.Value.String(),
		"the core projection must not be rewritten by the overlay")

	last := overlay.Series[0].Points[len(overlay.Series[0].Points)-1]
	assert.Negative(t, last.ProjectedBalance.Value.Cmp(exact.MustParse("100000")),
		"estimated spending lowers the projected balance")

	require.NotEmpty(t, overlay.Events)
	for _, event := range overlay.Events {
		assert.Equal(t, "estimated_spending", event.Source)
		assert.Zero(t, event.TransactionID, "an estimate references no saved record")
		assert.Zero(t, event.OccurrenceID)
		assert.Contains(t, event.Key, "estimate:1:3:1:")
		require.Len(t, event.Amounts, 1)
		assert.Negative(t, event.Amounts[0].Quantity.Value.Sign())
	}
	require.Len(t, overlay.Totals, 1)
	assert.Equal(t, int64(1), overlay.Totals[0].CommodityID)
}

func TestForecastLearningOverlayReportsExcludedGroups(t *testing.T) {
	t.Run("too little history is excluded by name", func(t *testing.T) {
		snapshot := overlaySnapshot(t, overlayHistory(t, 10, "7000"))
		overlay := overlayBuild(t, overlayInput(overlayHistoryStart(t, 10), nil), snapshot, overlayCore(t, "100000"))
		assert.Equal(t, ForecastLearningStatusUnavailable, overlay.Status)
		assert.Equal(t, 0, overlay.EligibleGroupCount)
		require.Len(t, overlay.Exclusions, 1)
		assert.Equal(t, ForecastLearningExcludedInsufficientHistory, overlay.Exclusions[0].Reason)
		assert.Equal(t, ForecastLearningExcludedInsufficientHistory, overlay.Reason)
		assert.Empty(t, overlay.Series, "an unavailable overlay returns no learned curve")
	})

	t.Run("a seasonal request without 36 months says so", func(t *testing.T) {
		snapshot := overlaySnapshot(t, overlayHistory(t, 20, "7000"))
		input := overlayInput(overlayHistoryStart(t, 20), []int64{3}, forecastCategoryPattern{CategoryID: 3, Pattern: ForecastLearningAnnualSeasonal})
		overlay := overlayBuild(t, input, snapshot, overlayCore(t, "100000"))
		require.Len(t, overlay.Exclusions, 1)
		assert.Equal(t, ForecastLearningExcludedInsufficientSeasonal, overlay.Exclusions[0].Reason)
		assert.Equal(t, string(ForecastLearningAnnualSeasonal), overlay.Exclusions[0].Pattern)
	})

	t.Run("a category outside the selection is never grouped", func(t *testing.T) {
		snapshot := overlaySnapshot(t, overlayHistory(t, 20, "7000"))
		overlay := overlayBuild(t, overlayInput(overlayHistoryStart(t, 20), []int64{4}), snapshot, overlayCore(t, "100000"))
		assert.Equal(t, 0, overlay.RequestedGroupCount)
		assert.Equal(t, ForecastLearningStatusUnavailable, overlay.Status)
		assert.Empty(t, overlay.Exclusions)
	})

	t.Run("mixed eligibility reports partial", func(t *testing.T) {
		postings := overlayHistory(t, 20, "7000")
		// A second category with only a few weeks cannot be estimated, but it
		// must not hide the category that can.
		origin, err := time.Parse(time.DateOnly, overlayAsOf)
		require.NoError(t, err)
		start := learningPeriodStart(origin, "week")
		for i := 3; i >= 1; i-- {
			date := start.AddDate(0, 0, -7*i).Format(time.DateOnly)
			id := int64(2000 + i)
			postings = append(postings, learningRows(id, id, "ordinary", "ordinary", date,
				learningLeg(1, "-2500"), learningLeg(4, "2500"))...)
		}
		overlay := overlayBuild(t, overlayInput(overlayHistoryStart(t, 20), nil), overlaySnapshot(t, postings), overlayCore(t, "100000"))
		assert.Equal(t, ForecastLearningStatusPartial, overlay.Status)
		assert.Equal(t, 2, overlay.RequestedGroupCount)
		assert.Equal(t, 1, overlay.EligibleGroupCount)
		require.Len(t, overlay.Exclusions, 1)
		assert.Equal(t, int64(4), overlay.Exclusions[0].CategoryAccountID)
		// Twenty complete weeks with only three positive ones is sparse
		// history, which is a different message from having no history.
		assert.Equal(t, ForecastLearningExcludedSparseHistory, overlay.Exclusions[0].Reason)
	})
}

// TestForecastLearningOverlayRespectsRecurringOverlap proves a category already
// covered by a template is dropped rather than estimated a second time.
func TestForecastLearningOverlayRespectsRecurringOverlap(t *testing.T) {
	snapshot := overlaySnapshot(t, overlayHistory(t, 20, "7000"))
	snapshot.Templates = []db.ForecastTemplateRecord{{ID: 77, BookID: 1, Name: "Groceries", Enabled: true, TransactionKind: "ordinary", Frequency: "monthly", IntervalCount: 1, StartsOn: "2025-01-01", GenerateFrom: "2025-01-01"}}
	snapshot.TemplatePostings = []db.ForecastTemplatePostingRecord{
		{TemplateID: 77, PostingID: 1, LineSeq: 1, AccountID: 1, CommodityID: 1, QuantityValue: exact.MustParse("-7000"), QuantityScale: 2},
		{TemplateID: 77, PostingID: 2, LineSeq: 2, AccountID: 3, CommodityID: 1, QuantityValue: exact.MustParse("7000"), QuantityScale: 2},
	}
	overlay := overlayBuild(t, overlayInput(overlayHistoryStart(t, 20), nil), snapshot, overlayCore(t, "100000"))

	assert.Equal(t, ForecastLearningStatusUnavailable, overlay.Status)
	require.Len(t, overlay.Exclusions, 1)
	assert.Equal(t, ForecastLearningExcludedRecurringOverlap, overlay.Exclusions[0].Reason)
	assert.Equal(t, "template", overlay.Exclusions[0].SourceType)
	assert.Equal(t, int64(77), overlay.Exclusions[0].SourceID)
	assert.Empty(t, overlay.Events, "a recurring bill is never learned and re-added")
}

// TestForecastLearningOverlaySubtractsKnownFutureSpending proves a purchase the
// core projection already carries is not estimated a second time.
func TestForecastLearningOverlaySubtractsKnownFutureSpending(t *testing.T) {
	base := overlayBuild(t, overlayInput(overlayHistoryStart(t, 20), nil), overlaySnapshot(t, overlayHistory(t, 20, "7000")), overlayCore(t, "100000"))
	require.Len(t, base.Groups, 1)

	postings := overlayHistory(t, 20, "7000")
	// A large posted purchase inside the first projected week already covers
	// that week's learned level.
	postings = append(postings, learningRows(9001, 9001, "ordinary", "ordinary", "2026-07-01",
		learningLeg(1, "-20000"), learningLeg(3, "20000"))...)
	withKnown := overlayBuild(t, overlayInput(overlayHistoryStart(t, 20), nil), overlaySnapshot(t, postings), overlayCore(t, "100000"))
	require.Len(t, withKnown.Groups, 1)

	assert.Negative(t, withKnown.Groups[0].EstimatedTotal.Value.Cmp(base.Groups[0].EstimatedTotal.Value),
		"known future spending must reduce the estimate for its own period")
}

func TestForecastLearningOverlayRefusesWithoutGroupPrefix(t *testing.T) {
	snapshot := overlaySnapshot(t, overlayHistory(t, 20, "7000"))
	core := overlayCore(t, "100000")
	bounds := forecastBounds{AsOf: overlayAsOf, First: core.FirstDate, Through: overlayThrough}
	scope := forecastTestScope(snapshot, 1)

	fitter := NewForecastLearningFitter()
	fitter.slot <- struct{}{}
	overlay, err := buildForecastLearnedSpending(context.Background(), fitter, overlayInput(overlayHistoryStart(t, 20), nil), bounds, scope, snapshot, core)
	require.NoError(t, err)
	assert.Equal(t, ForecastLearningStatusUnavailable, overlay.Status)
	assert.Equal(t, ForecastLearningExcludedBusy, overlay.Reason)
	assert.Empty(t, overlay.Groups)
	assert.Empty(t, overlay.Series)
	assert.Empty(t, overlay.Exclusions, "a resource refusal is not a per-group exclusion")
	<-fitter.slot

	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	overlay, err = buildForecastLearnedSpending(canceled, NewForecastLearningFitter(), overlayInput(overlayHistoryStart(t, 20), nil), bounds, scope, snapshot, core)
	require.NoError(t, err)
	assert.Equal(t, ForecastLearningExcludedComputationTimeout, overlay.Reason)
	assert.Empty(t, overlay.Series)
}

// TestForecastLearningOverlayListsCategoryOptions proves the owner can see what
// they may select without any history being required.
func TestForecastLearningOverlayListsCategoryOptions(t *testing.T) {
	overlay := overlayBuild(t, overlayInput("2024-01-01", nil), overlaySnapshot(t, nil), overlayCore(t, "100000"))
	require.NotEmpty(t, overlay.CategoryOptions)
	ids := make([]int64, 0, len(overlay.CategoryOptions))
	for _, option := range overlay.CategoryOptions {
		ids = append(ids, option.AccountID)
	}
	assert.Equal(t, []int64{3, 4}, ids, "only postable expense accounts are selectable")
	assert.Equal(t, ForecastLearningStatusUnavailable, overlay.Status)
	assert.Equal(t, "2024-01-01", overlay.HistoryCompleteFrom)
}
