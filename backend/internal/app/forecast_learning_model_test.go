package app

import (
	"context"
	"fmt"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rekenraam/backend/internal/exact"
)

// learningScaled turns a decimal fixture amount into a ScaledInt at the given
// scale, so fixtures read as money rather than as unscaled coefficients.
func learningScaled(t testing.TB, amount string, scale int) *exact.ScaledInt {
	t.Helper()
	integer, fraction, _ := strings.Cut(amount, ".")
	require.LessOrEqual(t, len(fraction), scale, "amount %q does not fit scale %d", amount, scale)
	digits := integer + fraction + strings.Repeat("0", scale-len(fraction))
	return exact.ScaledIntFromCoefficient(exact.MustParse(digits), scale)
}

// learningDecimal renders a ScaledInt the way a reader writes money.
func learningDecimal(value *exact.ScaledInt) string {
	return exact.DecimalFromBig(value.BigInt(), value.Scale())
}

// learningPeriodsFrom builds consecutive complete periods so a fixture reads as
// a calendar, not as an index. Amounts are decimal strings at the given scale.
func learningPeriodsFrom(t testing.TB, unit, start string, scale int, amounts ...string) []ForecastLearningPeriod {
	t.Helper()
	cursor, err := time.Parse(time.DateOnly, start)
	require.NoError(t, err)
	periods := make([]ForecastLearningPeriod, len(amounts))
	for i, amount := range amounts {
		next := learningNextPeriod(cursor, unit)
		periods[i] = ForecastLearningPeriod{
			Start: cursor.Format(time.DateOnly),
			End:   next.AddDate(0, 0, -1).Format(time.DateOnly),
			Spend: learningScaled(t, amount, scale),
		}
		cursor = next
	}
	return periods
}

func learningRepeat(amount string, count int) []string {
	values := make([]string, count)
	for i := range values {
		values[i] = amount
	}
	return values
}

func learningCandidateMAE(t testing.TB, selection ForecastLearningSelection, method string) *big.Rat {
	t.Helper()
	for _, candidate := range selection.TestCandidates {
		if candidate.Method == method {
			return candidate.OnePeriodMAE
		}
	}
	t.Fatalf("method %q was not evaluated on the holdout", method)
	return nil
}

func learningRat(t testing.TB, value string) *big.Rat {
	t.Helper()
	parsed, ok := new(big.Rat).SetString(value)
	require.True(t, ok, "parse rational %q", value)
	return parsed
}

// TestForecastLearningNoFutureLeakage uses a late regime shift. Every fold that
// honestly sees only its own prefix must be badly wrong about the holdout; only
// an implementation peeking at future periods could score well.
func TestForecastLearningNoFutureLeakage(t *testing.T) {
	amounts := append(learningRepeat("100.00", 12), learningRepeat("1000.00", 4)...)
	periods := learningPeriodsFrom(t, "week", "2025-01-06", 2, amounts...)

	selection, err := SelectForecastLearningModel(ForecastLearningWeekly, periods)
	require.NoError(t, err)

	// The tuning fold sits entirely inside the flat regime, so every candidate
	// ties at zero and the documented tie winners are frozen.
	assert.Equal(t, ForecastLearningMethodSES025, selection.TunedSES)
	assert.Equal(t, ForecastLearningMethodMean8, selection.TunedBaseline)
	for _, candidate := range selection.TuningCandidates {
		assert.Zero(t, candidate.OnePeriodMAE.Sign(), "%s must fit the flat tuning fold exactly", candidate.Method)
	}

	baseline := learningCandidateMAE(t, selection, ForecastLearningMethodMean8)
	learned := learningCandidateMAE(t, selection, ForecastLearningMethodSES025)
	// Expanding origins may absorb the shift one period at a time, but never
	// before it happened. These exact values are unreachable with leakage.
	assert.Equal(t, learningRat(t, "2925/4"), baseline, "mean_8 expanding-origin MAE")
	assert.Equal(t, learningRat(t, "39375/64"), learned, "ses_025 expanding-origin MAE")

	// The frozen four-period total is the clean detector: predicted from the
	// flat prefix it is 400.00 against an actual 4000.00.
	for _, candidate := range selection.TestCandidates {
		assert.Equal(t, learningRat(t, "3600"), candidate.FrozenTotalAbsError,
			"%s must predict the holdout total from the pre-shift prefix only", candidate.Method)
	}

	assert.Equal(t, "2025-03-03", selection.TuningRangeStart)
	assert.Equal(t, "2025-03-30", selection.TuningRangeEnd)
	assert.Equal(t, "2025-03-31", selection.TestRangeStart)
	assert.Equal(t, "2025-04-27", selection.TestRangeEnd)
	assert.Equal(t, 4, selection.TestedHorizonPeriods)
}

// TestForecastLearningFallsBackUnlessValidatedBetter drives the level gate over
// its exact boundaries. A dataset where the baseline wins is a passing test.
func TestForecastLearningFallsBackUnlessValidatedBetter(t *testing.T) {
	candidate := func(method, mae, total string) ForecastLearningCandidate {
		return ForecastLearningCandidate{
			Method:              method,
			OnePeriodMAE:        learningRat(t, mae),
			FrozenTotalAbsError: learningRat(t, total),
		}
	}
	cases := []struct {
		name             string
		baseline         ForecastLearningCandidate
		learned          ForecastLearningCandidate
		expectedMethod   string
		expectedFallback string
	}{
		{
			name:           "exactly ten percent better wins",
			baseline:       candidate(ForecastLearningMethodMean8, "100", "400"),
			learned:        candidate(ForecastLearningMethodSES025, "90", "400"),
			expectedMethod: ForecastLearningMethodSES025,
		},
		{
			name:             "a hair short of ten percent falls back",
			baseline:         candidate(ForecastLearningMethodMean8, "100", "400"),
			learned:          candidate(ForecastLearningMethodSES025, "9001/100", "400"),
			expectedMethod:   ForecastLearningMethodMean8,
			expectedFallback: ForecastLearningFallbackMarginNotMet,
		},
		{
			name:             "a tie falls back to the baseline",
			baseline:         candidate(ForecastLearningMethodMean8, "100", "400"),
			learned:          candidate(ForecastLearningMethodSES050, "100", "400"),
			expectedMethod:   ForecastLearningMethodMean8,
			expectedFallback: ForecastLearningFallbackMarginNotMet,
		},
		{
			name:             "a zero-error baseline is never displaced",
			baseline:         candidate(ForecastLearningMethodLastPeriod, "0", "0"),
			learned:          candidate(ForecastLearningMethodSES025, "0", "0"),
			expectedMethod:   ForecastLearningMethodLastPeriod,
			expectedFallback: ForecastLearningFallbackZeroErrorBaseline,
		},
		{
			name:             "a better period MAE cannot buy a worse horizon total",
			baseline:         candidate(ForecastLearningMethodMean8, "100", "400"),
			learned:          candidate(ForecastLearningMethodSES025, "10", "401"),
			expectedMethod:   ForecastLearningMethodMean8,
			expectedFallback: ForecastLearningFallbackWorseHorizonTotal,
		},
		{
			name:           "an equal horizon total is not worse",
			baseline:       candidate(ForecastLearningMethodMean8, "100", "400"),
			learned:        candidate(ForecastLearningMethodSES025, "10", "400"),
			expectedMethod: ForecastLearningMethodSES025,
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			method, fallback := forecastLearningLevelGate(testCase.baseline, testCase.learned)
			assert.Equal(t, testCase.expectedMethod, method)
			assert.Equal(t, testCase.expectedFallback, fallback)
		})
	}

	t.Run("flat history keeps the labelled baseline", func(t *testing.T) {
		periods := learningPeriodsFrom(t, "week", "2025-01-06", 2, learningRepeat("100.00", 16)...)
		selection, err := SelectForecastLearningModel(ForecastLearningWeekly, periods)
		require.NoError(t, err)
		assert.Equal(t, ForecastLearningMethodMean8, selection.Selected)
		assert.False(t, selection.SelectedIsLearned)
		assert.Equal(t, ForecastLearningFallbackZeroErrorBaseline, selection.FallbackReason)
		assert.Equal(t, learningRat(t, "100"), selection.Forecast)
	})

	t.Run("an alternating series keeps the mean baseline", func(t *testing.T) {
		amounts := make([]string, 16)
		for i := range amounts {
			amounts[i] = "0.00"
			if i%2 == 1 {
				amounts[i] = "200.00"
			}
		}
		periods := learningPeriodsFrom(t, "week", "2025-01-06", 2, amounts...)
		selection, err := SelectForecastLearningModel(ForecastLearningWeekly, periods)
		require.NoError(t, err)
		assert.Equal(t, ForecastLearningMethodMean8, selection.Selected)
		assert.False(t, selection.SelectedIsLearned)
		assert.Equal(t, ForecastLearningFallbackMarginNotMet, selection.FallbackReason)
		// Refit over the whole window, not over the holdout it was tested on.
		assert.Equal(t, learningRat(t, "100"), selection.Forecast)
	})
}

// learningSeasonalAmounts lays out three years of monthly spending with an
// explicit per-month-number profile.
func learningSeasonalAmounts(years int, profile map[time.Month]string, overrides map[string]string) []string {
	amounts := make([]string, 0, years*12)
	for year := 0; year < years; year++ {
		for month := time.January; month <= time.December; month++ {
			amount := "100.00"
			if value, ok := profile[month]; ok {
				amount = value
			}
			if value, ok := overrides[fmt.Sprintf("%d-%02d", year, int(month))]; ok {
				amount = value
			}
			amounts = append(amounts, amount)
		}
	}
	return amounts
}

// TestForecastLearningSeasonalPeaks proves July and August stay separate bins
// through selection and refit, so a summer trip that shifts between them is not
// smoothed into one averaged month.
func TestForecastLearningSeasonalPeaks(t *testing.T) {
	amounts := learningSeasonalAmounts(3,
		map[time.Month]string{time.July: "1000.00", time.August: "800.00"},
		map[string]string{"0-07": "900.00", "1-07": "1100.00"})
	periods := learningPeriodsFrom(t, "month", "2023-01-01", 2, amounts...)
	require.Len(t, periods, 36)

	selection, err := SelectForecastLearningModel(ForecastLearningAnnualSeasonal, periods)
	require.NoError(t, err)

	assert.Equal(t, ForecastLearningMethodSameMonth2Y, selection.Selected)
	assert.True(t, selection.SelectedIsLearned)
	assert.Empty(t, selection.FallbackReason)

	// Refit over all 36 months: July averages its two latest observations,
	// August keeps its own level, and ordinary months stay ordinary.
	assert.Equal(t, learningRat(t, "1050"), selection.MonthlyForecast[time.July])
	assert.Equal(t, learningRat(t, "800"), selection.MonthlyForecast[time.August])
	assert.Equal(t, learningRat(t, "100"), selection.MonthlyForecast[time.January])
	assert.NotEqual(t, selection.MonthlyForecast[time.July], selection.MonthlyForecast[time.August],
		"a shared summer bin would erase the difference between the two peaks")

	// A 366-day horizon reaches the same month number a second time and must
	// reuse the observed level rather than a previous forecast.
	assert.Len(t, selection.MonthlyForecast, 12)

	// Every calendar month is evaluated, not only the summer ones.
	assert.Equal(t, 12, selection.TestedHorizonPeriods)
	assert.Equal(t, "2025-01-01", selection.TestRangeStart)
	assert.Equal(t, "2025-12-31", selection.TestRangeEnd)
}

func TestForecastLearningSeasonalValidation(t *testing.T) {
	t.Run("the frozen year sees only earlier observations", func(t *testing.T) {
		amounts := learningSeasonalAmounts(3,
			map[time.Month]string{time.July: "1000.00", time.August: "800.00"},
			map[string]string{"0-07": "900.00", "1-07": "1100.00"})
		periods := learningPeriodsFrom(t, "month", "2023-01-01", 2, amounts...)
		selection, err := SelectForecastLearningModel(ForecastLearningAnnualSeasonal, periods)
		require.NoError(t, err)

		var same, last, flat ForecastLearningCandidate
		for _, candidate := range selection.TestCandidates {
			switch candidate.Method {
			case ForecastLearningMethodSameMonth2Y:
				same = candidate
			case ForecastLearningMethodSeasonalLastYear:
				last = candidate
			case ForecastLearningMethodFlatMean12:
				flat = candidate
			}
		}
		require.NotNil(t, flat.OnePeriodMAE, "the flat reference is always reported")

		// seasonal_last_year predicts July 2025 from July 2024 (1100.00), never
		// from the 1000.00 it is being scored against.
		assert.Equal(t, learningRat(t, "100"), last.FrozenMaxAbsError)
		assert.Equal(t, learningRat(t, "25/3"), last.OnePeriodMAE)
		assert.Equal(t, learningRat(t, "100"), last.FrozenTotalAbsError)
		assert.Zero(t, same.OnePeriodMAE.Sign())
		assert.Zero(t, same.FrozenPathMAE.Sign())
		assert.Zero(t, same.FrozenTotalAbsError.Sign())
		assert.Positive(t, flat.OnePeriodMAE.Sign(), "a flat mean cannot track a July peak")
		assert.NotContains(t, selection.Warnings, ForecastLearningWarningWeakSeasonality)
	})

	t.Run("a zero-error seasonal baseline is kept and labelled weak", func(t *testing.T) {
		amounts := learningSeasonalAmounts(3, nil, nil)
		periods := learningPeriodsFrom(t, "month", "2023-01-01", 2, amounts...)
		selection, err := SelectForecastLearningModel(ForecastLearningAnnualSeasonal, periods)
		require.NoError(t, err)

		assert.Equal(t, ForecastLearningMethodSeasonalLastYear, selection.Selected)
		assert.False(t, selection.SelectedIsLearned)
		assert.Equal(t, ForecastLearningFallbackZeroErrorBaseline, selection.FallbackReason)
		// Perfectly flat spending is not better than the flat reference, and
		// the pattern stays seasonal rather than being switched to weekly.
		assert.Contains(t, selection.Warnings, ForecastLearningWarningWeakSeasonality)
		assert.Equal(t, ForecastLearningAnnualSeasonal, selection.Pattern)
		assert.Equal(t, learningRat(t, "100"), selection.MonthlyForecast[time.July])
	})

	t.Run("a peak seen once in three years is called out", func(t *testing.T) {
		// July is spent in one year only, so its peak rests on a single
		// observation even though the month number is observed three times.
		amounts := learningSeasonalAmounts(3, map[time.Month]string{time.July: "0.00"},
			map[string]string{"1-07": "3000.00"})
		periods := learningPeriodsFrom(t, "month", "2023-01-01", 2, amounts...)
		selection, err := SelectForecastLearningModel(ForecastLearningAnnualSeasonal, periods)
		require.NoError(t, err)

		assert.Contains(t, selection.Warnings, ForecastLearningWarningSparseAnnual)
		for _, month := range selection.Variation.Months {
			if month.Month == time.July {
				assert.Equal(t, 3, month.Observations)
				assert.Equal(t, 1, month.PositiveObservations)
			}
		}
	})

	t.Run("the seasonal gate rejects a candidate that is worse anywhere", func(t *testing.T) {
		candidate := func(method, mae, path, total, maximum string) ForecastLearningCandidate {
			return ForecastLearningCandidate{
				Method:              method,
				OnePeriodMAE:        learningRat(t, mae),
				FrozenPathMAE:       learningRat(t, path),
				FrozenTotalAbsError: learningRat(t, total),
				FrozenMaxAbsError:   learningRat(t, maximum),
			}
		}
		last := candidate(ForecastLearningMethodSeasonalLastYear, "100", "100", "100", "100")
		cases := []struct {
			name             string
			same             ForecastLearningCandidate
			expectedMethod   string
			expectedFallback string
		}{
			{
				name:           "better everywhere",
				same:           candidate(ForecastLearningMethodSameMonth2Y, "90", "100", "100", "100"),
				expectedMethod: ForecastLearningMethodSameMonth2Y,
			},
			{
				name:             "margin not met",
				same:             candidate(ForecastLearningMethodSameMonth2Y, "91", "10", "10", "10"),
				expectedMethod:   ForecastLearningMethodSeasonalLastYear,
				expectedFallback: ForecastLearningFallbackMarginNotMet,
			},
			{
				name:             "worse frozen path",
				same:             candidate(ForecastLearningMethodSameMonth2Y, "10", "101", "10", "10"),
				expectedMethod:   ForecastLearningMethodSeasonalLastYear,
				expectedFallback: ForecastLearningFallbackWorseFrozenPath,
			},
			{
				name:             "worse annual total",
				same:             candidate(ForecastLearningMethodSameMonth2Y, "10", "10", "101", "10"),
				expectedMethod:   ForecastLearningMethodSeasonalLastYear,
				expectedFallback: ForecastLearningFallbackWorseAnnualTotal,
			},
			{
				name:             "worse worst month",
				same:             candidate(ForecastLearningMethodSameMonth2Y, "10", "10", "10", "101"),
				expectedMethod:   ForecastLearningMethodSeasonalLastYear,
				expectedFallback: ForecastLearningFallbackWorseMaximumMonth,
			},
		}
		for _, testCase := range cases {
			t.Run(testCase.name, func(t *testing.T) {
				method, fallback := forecastLearningSeasonalGate(testCase.same, last)
				assert.Equal(t, testCase.expectedMethod, method)
				assert.Equal(t, testCase.expectedFallback, fallback)
			})
		}

		zero := candidate(ForecastLearningMethodSeasonalLastYear, "0", "0", "0", "0")
		method, fallback := forecastLearningSeasonalGate(candidate(ForecastLearningMethodSameMonth2Y, "0", "0", "0", "0"), zero)
		assert.Equal(t, ForecastLearningMethodSeasonalLastYear, method)
		assert.Equal(t, ForecastLearningFallbackZeroErrorBaseline, fallback)
	})
}

// TestForecastLearningVariationIsNotConfidence checks that reported spread is
// observed history with counts, carrying no band, percentile or jitter.
func TestForecastLearningVariationIsNotConfidence(t *testing.T) {
	t.Run("weekly reports the fitted window range", func(t *testing.T) {
		amounts := append(learningRepeat("100.00", 15), "250.00")
		periods := learningPeriodsFrom(t, "week", "2025-01-06", 2, amounts...)
		selection, err := SelectForecastLearningModel(ForecastLearningWeekly, periods)
		require.NoError(t, err)

		assert.Equal(t, ForecastLearningVariationKind, selection.Variation.Kind)
		assert.Equal(t, "week", selection.Variation.Unit)
		assert.Equal(t, 16, selection.Variation.Periods)
		assert.Equal(t, "100.00", learningDecimal(selection.Variation.Min))
		assert.Equal(t, "250.00", learningDecimal(selection.Variation.Max))
		assert.Nil(t, selection.Variation.Months, "a weekly model has no month profile")
		// The tested horizon is four weeks; anything beyond continues the same
		// assumption and must be labelled as such by the caller.
		assert.Equal(t, 4, selection.TestedHorizonPeriods)
		assert.Equal(t, 16, selection.FittedPeriods)
	})

	t.Run("annual reports per-month range and counts", func(t *testing.T) {
		amounts := learningSeasonalAmounts(3,
			map[time.Month]string{time.July: "1000.00"},
			map[string]string{"0-07": "900.00", "1-07": "1100.00"})
		periods := learningPeriodsFrom(t, "month", "2023-01-01", 2, amounts...)
		selection, err := SelectForecastLearningModel(ForecastLearningAnnualSeasonal, periods)
		require.NoError(t, err)

		assert.Equal(t, ForecastLearningVariationKind, selection.Variation.Kind)
		require.Len(t, selection.Variation.Months, 12)
		assert.Equal(t, time.January, selection.Variation.Months[0].Month)
		for _, month := range selection.Variation.Months {
			assert.Equal(t, 3, month.Observations, "%s", month.Month)
			if month.Month == time.July {
				assert.Equal(t, "900.00", learningDecimal(month.Min))
				assert.Equal(t, "1100.00", learningDecimal(month.Max))
				assert.Equal(t, 3, month.PositiveObservations)
				continue
			}
			assert.Equal(t, "100.00", learningDecimal(month.Min))
			assert.Equal(t, "100.00", learningDecimal(month.Max))
		}
		assert.Equal(t, 12, selection.TestedHorizonPeriods)
	})

	t.Run("repeated fits are deterministic", func(t *testing.T) {
		periods := learningPeriodsFrom(t, "week", "2025-01-06", 2,
			append(learningRepeat("100.00", 12), "180.00", "90.00", "140.00", "110.00")...)
		first, err := SelectForecastLearningModel(ForecastLearningWeekly, periods)
		require.NoError(t, err)
		second, err := SelectForecastLearningModel(ForecastLearningWeekly, periods)
		require.NoError(t, err)
		assert.Equal(t, first.Selected, second.Selected)
		assert.Zero(t, first.Forecast.Cmp(second.Forecast), "no jitter may be added to a fitted level")
	})
}

// TestForecastLearningPreservesExactMoney keeps the fit rational end to end,
// including coefficients past the float64 integer range.
func TestForecastLearningPreservesExactMoney(t *testing.T) {
	t.Run("smoothing is exact rational arithmetic", func(t *testing.T) {
		spend := []*big.Rat{learningRat(t, "1"), new(big.Rat), new(big.Rat)}
		assert.Equal(t, learningRat(t, "9/16"), forecastLearningSES(spend, big.NewRat(1, 4)))
		assert.Equal(t, learningRat(t, "1/4"), forecastLearningSES(spend, big.NewRat(1, 2)))
	})

	t.Run("coefficients beyond 2^53 survive selection", func(t *testing.T) {
		huge := "90071992547409.91"
		periods := learningPeriodsFrom(t, "week", "2025-01-06", 2, learningRepeat(huge, 16)...)
		selection, err := SelectForecastLearningModel(ForecastLearningWeekly, periods)
		require.NoError(t, err)
		assert.Equal(t, learningRat(t, "9007199254740991/100"), selection.Forecast)
		assert.Equal(t, huge, learningDecimal(selection.Variation.Max))
	})

	t.Run("mixed posted scales share one exact level", func(t *testing.T) {
		periods := learningPeriodsFrom(t, "week", "2025-01-06", 2, learningRepeat("100.00", 16)...)
		// An older posting kept four decimals; the level must reflect it
		// exactly rather than being truncated to the account scale.
		periods[15].Spend = learningScaled(t, "100.0001", 4)
		selection, err := SelectForecastLearningModel(ForecastLearningWeekly, periods)
		require.NoError(t, err)
		assert.Equal(t, "100.0001", learningDecimal(periods[15].Spend))
		assert.Positive(t, selection.Forecast.Cmp(learningRat(t, "100")))
		assert.Equal(t, learningRat(t, "8000001/80000"), selection.Forecast)
	})
}

// TestForecastLearningLimitsNeverReturnGroupPrefixes proves every refusal path
// returns no groups at all, so the caller falls back to the unchanged core
// forecast rather than a partial learned overlay.
func TestForecastLearningLimitsNeverReturnGroupPrefixes(t *testing.T) {
	history := func(count int) []ForecastLearningGroupHistory {
		periods := learningPeriodsFrom(t, "week", "2025-01-06", 2, learningRepeat("100.00", 16)...)
		groups := make([]ForecastLearningGroupHistory, count)
		for i := range groups {
			groups[i] = ForecastLearningGroupHistory{
				Group:   ForecastLearningGroup{FundingAccountID: 1, CategoryAccountID: int64(i + 10), CommodityID: 1},
				Pattern: ForecastLearningWeekly,
				Periods: periods,
			}
		}
		return groups
	}

	t.Run("the limit itself is allowed", func(t *testing.T) {
		result, err := NewForecastLearningFitter().Fit(t.Context(), ForecastLearningFitRequest{Groups: history(ForecastLearningGroupLimit)})
		require.NoError(t, err)
		assert.Len(t, result.Groups, ForecastLearningGroupLimit)
	})

	t.Run("one group over the limit refuses outright", func(t *testing.T) {
		result, err := NewForecastLearningFitter().Fit(t.Context(), ForecastLearningFitRequest{Groups: history(ForecastLearningGroupLimit + 1)})
		var unavailable *ForecastLearningUnavailableError
		require.ErrorAs(t, err, &unavailable)
		assert.Equal(t, ForecastLearningReasonResourceLimit, unavailable.Reason)
		assert.Empty(t, result.Groups, "no prefix of the eligible groups may be returned")
	})

	t.Run("a second concurrent fit is refused rather than queued", func(t *testing.T) {
		fitter := NewForecastLearningFitter()
		fitter.slot <- struct{}{}
		result, err := fitter.Fit(t.Context(), ForecastLearningFitRequest{Groups: history(1)})
		var unavailable *ForecastLearningUnavailableError
		require.ErrorAs(t, err, &unavailable)
		assert.Equal(t, ForecastLearningReasonBusy, unavailable.Reason)
		assert.Empty(t, result.Groups)
		<-fitter.slot
	})

	t.Run("cancellation and deadline stop work with distinct reasons", func(t *testing.T) {
		canceled, cancel := context.WithCancel(t.Context())
		cancel()
		result, err := NewForecastLearningFitter().Fit(canceled, ForecastLearningFitRequest{Groups: history(2)})
		var unavailable *ForecastLearningUnavailableError
		require.ErrorAs(t, err, &unavailable)
		assert.Equal(t, ForecastLearningReasonCanceled, unavailable.Reason)
		assert.Empty(t, result.Groups)

		expired, stop := context.WithDeadline(t.Context(), time.Now().Add(-time.Second))
		defer stop()
		result, err = NewForecastLearningFitter().Fit(expired, ForecastLearningFitRequest{Groups: history(2)})
		require.ErrorAs(t, err, &unavailable)
		assert.Equal(t, ForecastLearningReasonDeadline, unavailable.Reason)
		assert.Empty(t, result.Groups)
	})

	t.Run("the slot is released after a completed fit", func(t *testing.T) {
		fitter := NewForecastLearningFitter()
		_, err := fitter.Fit(t.Context(), ForecastLearningFitRequest{Groups: history(1)})
		require.NoError(t, err)
		_, err = fitter.Fit(t.Context(), ForecastLearningFitRequest{Groups: history(1)})
		require.NoError(t, err)
	})
}

// TestForecastLearningPatternsAreExclusive checks the four cadences coexist,
// each keeping its own period system and its own single model.
func TestForecastLearningPatternsAreExclusive(t *testing.T) {
	weekly := learningPeriodsFrom(t, "week", "2025-01-06", 2, learningRepeat("100.00", 16)...)
	monthly := learningPeriodsFrom(t, "month", "2024-01-01", 2, learningRepeat("400.00", 20)...)
	seasonal := learningPeriodsFrom(t, "month", "2023-01-01", 2,
		learningSeasonalAmounts(3, map[time.Month]string{time.July: "1000.00"}, nil)...)

	groups := []ForecastLearningGroupHistory{
		{Group: ForecastLearningGroup{FundingAccountID: 1, CategoryAccountID: 3, CommodityID: 1}, Pattern: ForecastLearningDaily, Periods: weekly},
		{Group: ForecastLearningGroup{FundingAccountID: 1, CategoryAccountID: 4, CommodityID: 1}, Pattern: ForecastLearningWeekly, Periods: weekly},
		{Group: ForecastLearningGroup{FundingAccountID: 2, CategoryAccountID: 3, CommodityID: 1}, Pattern: ForecastLearningMonthly, Periods: monthly},
		{Group: ForecastLearningGroup{FundingAccountID: 2, CategoryAccountID: 4, CommodityID: 1}, Pattern: ForecastLearningAnnualSeasonal, Periods: seasonal},
	}
	result, err := NewForecastLearningFitter().Fit(t.Context(), ForecastLearningFitRequest{Groups: groups})
	require.NoError(t, err)
	require.Len(t, result.Groups, len(groups))

	units := map[ForecastLearningPattern]string{
		ForecastLearningDaily:          "week",
		ForecastLearningWeekly:         "week",
		ForecastLearningMonthly:        "month",
		ForecastLearningAnnualSeasonal: "month",
	}
	for _, model := range result.Groups {
		assert.Equal(t, units[model.Pattern], model.Selection.Unit)
		assert.NotEmpty(t, model.Selection.Selected, "every group gets exactly one method")
		if model.Pattern == ForecastLearningAnnualSeasonal {
			assert.Nil(t, model.Selection.Forecast, "a seasonal model has no single flat level")
			assert.Len(t, model.Selection.MonthlyForecast, 12)
			continue
		}
		assert.NotNil(t, model.Selection.Forecast)
		assert.Nil(t, model.Selection.MonthlyForecast, "a level model has no month profile")
	}
}

func TestForecastLearningSelectionRejectsShortHistory(t *testing.T) {
	short := learningPeriodsFrom(t, "week", "2025-01-06", 2, learningRepeat("100.00", 15)...)
	_, err := SelectForecastLearningModel(ForecastLearningWeekly, short)
	require.ErrorContains(t, err, "16 complete periods")

	shortMonths := learningPeriodsFrom(t, "month", "2023-01-01", 2, learningRepeat("100.00", 35)...)
	_, err = SelectForecastLearningModel(ForecastLearningAnnualSeasonal, shortMonths)
	require.ErrorContains(t, err, "36 complete months")

	_, err = SelectForecastLearningModel(ForecastLearningPattern("quarterly"), short)
	require.ErrorContains(t, err, "unsupported forecast learning pattern")
}

func BenchmarkForecastLearningLevelSelection(b *testing.B) {
	amounts := make([]string, 52)
	for i := range amounts {
		amounts[i] = fmt.Sprintf("%d.%02d", 90+i, i%100)
	}
	periods := learningPeriodsFrom(b, "week", "2024-01-01", 2, amounts...)
	b.ReportAllocs()
	for b.Loop() {
		if _, err := SelectForecastLearningModel(ForecastLearningWeekly, periods); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkForecastLearningSeasonalSelection(b *testing.B) {
	amounts := make([]string, 60)
	for i := range amounts {
		amounts[i] = fmt.Sprintf("%d.%02d", 100+i*3, i%100)
	}
	periods := learningPeriodsFrom(b, "month", "2021-01-01", 2, amounts...)
	b.ReportAllocs()
	for b.Loop() {
		if _, err := SelectForecastLearningModel(ForecastLearningAnnualSeasonal, periods); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkForecastLearningFit is the declared mixed-pattern workload: the full
// 100-group ceiling at maximum history for each cadence.
func BenchmarkForecastLearningFit(b *testing.B) {
	weeklyAmounts := make([]string, 52)
	for i := range weeklyAmounts {
		weeklyAmounts[i] = fmt.Sprintf("%d.%02d", 90+i, i%100)
	}
	monthlyAmounts := make([]string, 60)
	for i := range monthlyAmounts {
		monthlyAmounts[i] = fmt.Sprintf("%d.%02d", 100+i*3, i%100)
	}
	weekly := learningPeriodsFrom(b, "week", "2024-01-01", 2, weeklyAmounts...)
	monthly := learningPeriodsFrom(b, "month", "2021-01-01", 2, monthlyAmounts...)

	patterns := []ForecastLearningPattern{ForecastLearningDaily, ForecastLearningWeekly, ForecastLearningMonthly, ForecastLearningAnnualSeasonal}
	groups := make([]ForecastLearningGroupHistory, ForecastLearningGroupLimit)
	for i := range groups {
		pattern := patterns[i%len(patterns)]
		periods := weekly
		if pattern == ForecastLearningMonthly || pattern == ForecastLearningAnnualSeasonal {
			periods = monthly
		}
		groups[i] = ForecastLearningGroupHistory{
			Group:   ForecastLearningGroup{FundingAccountID: int64(i%10 + 1), CategoryAccountID: int64(i + 10), CommodityID: 1},
			Pattern: pattern,
			Periods: periods,
		}
	}
	request := ForecastLearningFitRequest{Groups: groups}
	fitter := NewForecastLearningFitter()
	ctx := context.Background()
	b.ReportAllocs()
	for b.Loop() {
		if _, err := fitter.Fit(ctx, request); err != nil {
			b.Fatal(err)
		}
	}
}
