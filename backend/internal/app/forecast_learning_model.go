package app

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"time"

	"rekenraam/backend/internal/exact"
)

// Method names are part of the learned-spending vocabulary. M3 turns them into
// versioned schema values; nothing here may be renamed silently afterwards.
const (
	ForecastLearningMethodMean8            = "mean_8"
	ForecastLearningMethodLastPeriod       = "last_period"
	ForecastLearningMethodSES025           = "ses_025"
	ForecastLearningMethodSES050           = "ses_050"
	ForecastLearningMethodSameMonth2Y      = "same_month_2y"
	ForecastLearningMethodSeasonalLastYear = "seasonal_last_year"
	ForecastLearningMethodFlatMean12       = "flat_mean_12"
)

// Fallback reasons say which gate rejected the learned candidate. A learned
// model that simply lost is a normal outcome, not an error.
const (
	ForecastLearningFallbackZeroErrorBaseline = "zero_error_baseline"
	ForecastLearningFallbackMarginNotMet      = "baseline_not_beaten_by_margin"
	ForecastLearningFallbackWorseHorizonTotal = "worse_four_period_total"
	ForecastLearningFallbackWorseFrozenPath   = "worse_frozen_path"
	ForecastLearningFallbackWorseAnnualTotal  = "worse_annual_total"
	ForecastLearningFallbackWorseMaximumMonth = "worse_maximum_month"
)

const (
	// ForecastLearningWarningWeakSeasonality keeps an owner-selected seasonal
	// pattern visible while admitting it did not beat a flat reference. The
	// plan forbids silently converting it into weekly spending.
	ForecastLearningWarningWeakSeasonality = "seasonality_not_better_than_flat"
	// ForecastLearningWarningSparseAnnual marks a month whose peak rests on a
	// single positive observation. The 36-month minimum guarantees at least
	// three observations of every month number, so the count alone is never
	// the sparse case; one holiday seen once in three years is.
	ForecastLearningWarningSparseAnnual = "sparse_annual_observations"
)

const (
	ForecastLearningReasonResourceLimit = "resource_limit"
	ForecastLearningReasonBusy          = "busy"
	ForecastLearningReasonDeadline      = "deadline"
	ForecastLearningReasonCanceled      = "canceled"
)

// ForecastLearningGroupLimit bounds eligible model groups. Section 2 of the
// plan forbids fitting the first 100 of 101 groups, so exceeding it is refused
// outright rather than truncated.
const ForecastLearningGroupLimit = 100

const (
	forecastLearningTuningPeriods = 4
	forecastLearningTestPeriods   = 4
	forecastLearningSeasonalTest  = 12
	// ForecastLearningVariationKind labels observed history. It is deliberately
	// not a confidence band, prediction interval or plausible future range.
	ForecastLearningVariationKind = "observed_historical_range"
)

// ForecastLearningUnavailableError reports that no learned overlay can be
// produced. It never carries a partial group prefix: the caller returns the
// unchanged core forecast plus this reason.
type ForecastLearningUnavailableError struct{ Reason string }

func (e *ForecastLearningUnavailableError) Error() string {
	return fmt.Sprintf("forecast learning unavailable: %s", e.Reason)
}

// ForecastLearningCandidate records one method's exact retrospective errors.
// Unused metrics stay nil for the pattern that does not define them.
type ForecastLearningCandidate struct {
	Method string
	// OnePeriodMAE is the mean absolute error of one-period-ahead predictions
	// at expanding origins.
	OnePeriodMAE *big.Rat
	// FrozenTotalAbsError is the absolute error of a single frozen prediction
	// of the whole holdout total, made once at the first holdout origin.
	FrozenTotalAbsError *big.Rat
	// FrozenPathMAE and FrozenMaxAbsError describe the same frozen path per
	// period; they are annual-seasonal only.
	FrozenPathMAE     *big.Rat
	FrozenMaxAbsError *big.Rat
}

// ForecastLearningMonthVariation is the observed spread for one calendar month
// number across the fitted window. Observations is usually small.
type ForecastLearningMonthVariation struct {
	Month time.Month
	// Observations counts complete months of this month number in the window;
	// PositiveObservations counts those with spend, which is what a seasonal
	// peak actually rests on.
	Observations         int
	PositiveObservations int
	Min                  *exact.ScaledInt
	Max                  *exact.ScaledInt
}

// ForecastLearningVariation is historical variation, never a confidence band.
// It carries no probability, percentile or jitter of any kind.
type ForecastLearningVariation struct {
	Kind    string
	Unit    string
	Periods int
	Min     *exact.ScaledInt
	Max     *exact.ScaledInt
	Months  []ForecastLearningMonthVariation
}

// ForecastLearningSelection is the frozen outcome of chronological evaluation
// for one group.
type ForecastLearningSelection struct {
	Pattern           ForecastLearningPattern
	Unit              string
	Selected          string
	SelectedIsLearned bool
	FallbackReason    string

	// TunedSES and TunedBaseline are chosen on the tuning fold and frozen
	// before the holdout is touched.
	TunedSES      string
	TunedBaseline string

	TuningCandidates []ForecastLearningCandidate
	TestCandidates   []ForecastLearningCandidate

	TuningRangeStart string
	TuningRangeEnd   string
	TestRangeStart   string
	TestRangeEnd     string

	// TestedHorizonPeriods is the horizon actually validated. Longer output is
	// a continuation of the same assumptions, not a tested projection.
	TestedHorizonPeriods int
	FittedPeriods        int

	// Forecast is the refit level for daily/weekly/monthly patterns.
	Forecast *big.Rat
	// MonthlyForecast is the refit per-calendar-month level for the annual
	// seasonal pattern. A target beyond twelve months reuses these observed
	// month-number values rather than any earlier forecast.
	MonthlyForecast map[time.Month]*big.Rat

	Variation ForecastLearningVariation
	Warnings  []string
}

// ForecastLearningGroupHistory is one group's complete-period history, already
// filtered by M1's eligibility, overlap and coverage rules.
type ForecastLearningGroupHistory struct {
	Group   ForecastLearningGroup
	Pattern ForecastLearningPattern
	Periods []ForecastLearningPeriod
}

type ForecastLearningGroupModel struct {
	Group     ForecastLearningGroup
	Pattern   ForecastLearningPattern
	Selection ForecastLearningSelection
}

type ForecastLearningFitRequest struct {
	Groups []ForecastLearningGroupHistory
}

type ForecastLearningFitResult struct {
	Groups []ForecastLearningGroupModel
}

// ForecastLearningFitter serialises fitting to one operation per process. A
// second concurrent request is refused immediately rather than queued.
type ForecastLearningFitter struct{ slot chan struct{} }

func NewForecastLearningFitter() *ForecastLearningFitter {
	return &ForecastLearningFitter{slot: make(chan struct{}, 1)}
}

// Fit selects a model per group. Any resource refusal returns a zero result:
// the caller must never see a prefix of the requested groups.
func (f *ForecastLearningFitter) Fit(ctx context.Context, request ForecastLearningFitRequest) (ForecastLearningFitResult, error) {
	if len(request.Groups) > ForecastLearningGroupLimit {
		return ForecastLearningFitResult{}, &ForecastLearningUnavailableError{Reason: ForecastLearningReasonResourceLimit}
	}
	if err := forecastLearningContextError(ctx); err != nil {
		return ForecastLearningFitResult{}, err
	}
	select {
	case f.slot <- struct{}{}:
	default:
		return ForecastLearningFitResult{}, &ForecastLearningUnavailableError{Reason: ForecastLearningReasonBusy}
	}
	defer func() { <-f.slot }()

	models := make([]ForecastLearningGroupModel, 0, len(request.Groups))
	for _, history := range request.Groups {
		if err := forecastLearningContextError(ctx); err != nil {
			return ForecastLearningFitResult{}, err
		}
		selection, err := SelectForecastLearningModel(history.Pattern, history.Periods)
		if err != nil {
			return ForecastLearningFitResult{}, err
		}
		models = append(models, ForecastLearningGroupModel{Group: history.Group, Pattern: history.Pattern, Selection: selection})
	}
	return ForecastLearningFitResult{Groups: models}, nil
}

// forecastLearningContextError separates a learning-only computation deadline
// from a genuine infrastructure failure. Both stop work; only this one becomes
// an overlay reason rather than an API error.
func forecastLearningContextError(ctx context.Context) error {
	err := ctx.Err()
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return &ForecastLearningUnavailableError{Reason: ForecastLearningReasonDeadline}
	case err != nil:
		return &ForecastLearningUnavailableError{Reason: ForecastLearningReasonCanceled}
	}
	return nil
}

// SelectForecastLearningModel runs the chronological evaluation for one group
// and returns the frozen selection. Every fold fits only on its own date
// prefix; no candidate ever observes the period it predicts.
func SelectForecastLearningModel(pattern ForecastLearningPattern, periods []ForecastLearningPeriod) (ForecastLearningSelection, error) {
	switch pattern {
	case ForecastLearningDaily, ForecastLearningWeekly, ForecastLearningMonthly:
		return selectForecastLearningLevel(pattern, periods)
	case ForecastLearningAnnualSeasonal:
		return selectForecastLearningSeasonal(periods)
	default:
		return ForecastLearningSelection{}, fmt.Errorf("unsupported forecast learning pattern %q", pattern)
	}
}

func selectForecastLearningLevel(pattern ForecastLearningPattern, periods []ForecastLearningPeriod) (ForecastLearningSelection, error) {
	total := len(periods)
	reserved := forecastLearningTuningPeriods + forecastLearningTestPeriods
	if total < 16 {
		return ForecastLearningSelection{}, fmt.Errorf("forecast learning level selection needs 16 complete periods, got %d", total)
	}
	spend := forecastLearningRats(periods)
	unit := "week"
	if pattern == ForecastLearningMonthly {
		unit = "month"
	}

	tuningStart := total - reserved
	testStart := total - forecastLearningTestPeriods

	// Tuning fold: pick one SES alpha and one baseline, then freeze both.
	tuning := []ForecastLearningCandidate{
		forecastLearningLevelFold(ForecastLearningMethodMean8, spend, tuningStart, testStart, false),
		forecastLearningLevelFold(ForecastLearningMethodLastPeriod, spend, tuningStart, testStart, false),
		forecastLearningLevelFold(ForecastLearningMethodSES025, spend, tuningStart, testStart, false),
		forecastLearningLevelFold(ForecastLearningMethodSES050, spend, tuningStart, testStart, false),
	}
	tunedSES := forecastLearningBetter(tuning, ForecastLearningMethodSES025, ForecastLearningMethodSES050)
	tunedBaseline := forecastLearningBetter(tuning, ForecastLearningMethodMean8, ForecastLearningMethodLastPeriod)

	// Holdout fold: only the two frozen methods are measured, so the final
	// error can never select an alpha for us.
	test := []ForecastLearningCandidate{
		forecastLearningLevelFold(tunedBaseline, spend, testStart, total, true),
		forecastLearningLevelFold(tunedSES, spend, testStart, total, true),
	}
	baseline, learned := test[0], test[1]

	selection := ForecastLearningSelection{
		Pattern:              pattern,
		Unit:                 unit,
		TunedSES:             tunedSES,
		TunedBaseline:        tunedBaseline,
		TuningCandidates:     tuning,
		TestCandidates:       test,
		TuningRangeStart:     periods[tuningStart].Start,
		TuningRangeEnd:       periods[testStart-1].End,
		TestRangeStart:       periods[testStart].Start,
		TestRangeEnd:         periods[total-1].End,
		TestedHorizonPeriods: forecastLearningTestPeriods,
		FittedPeriods:        total,
		Variation:            forecastLearningLevelVariation(unit, periods),
	}
	selection.Selected, selection.FallbackReason = forecastLearningLevelGate(baseline, learned)
	selection.SelectedIsLearned = selection.FallbackReason == ""
	// Refit the selected method on every available complete period. Selection
	// is frozen first, so this cannot change which method won.
	selection.Forecast = forecastLearningLevelPredict(selection.Selected, spend)
	return selection, nil
}

// forecastLearningLevelGate applies the plan's asymmetric policy: the learned
// candidate must be materially better on both metrics, and a baseline with no
// error at all can never be displaced.
func forecastLearningLevelGate(baseline, learned ForecastLearningCandidate) (string, string) {
	switch {
	case baseline.OnePeriodMAE.Sign() == 0:
		return baseline.Method, ForecastLearningFallbackZeroErrorBaseline
	case !forecastLearningBeatsByTenPercent(learned.OnePeriodMAE, baseline.OnePeriodMAE):
		return baseline.Method, ForecastLearningFallbackMarginNotMet
	case learned.FrozenTotalAbsError.Cmp(baseline.FrozenTotalAbsError) > 0:
		return baseline.Method, ForecastLearningFallbackWorseHorizonTotal
	default:
		return learned.Method, ""
	}
}

// forecastLearningLevelFold evaluates one method over [from, to). Each origin
// refits the frozen algorithm on its own prefix only. When frozen is set it
// also predicts the whole fold total once, from the first origin's prefix.
func forecastLearningLevelFold(method string, spend []*big.Rat, from, to int, frozen bool) ForecastLearningCandidate {
	candidate := ForecastLearningCandidate{Method: method}
	sum := new(big.Rat)
	for origin := from; origin < to; origin++ {
		predicted := forecastLearningLevelPredict(method, spend[:origin])
		sum.Add(sum, forecastLearningAbs(new(big.Rat).Sub(predicted, spend[origin])))
	}
	candidate.OnePeriodMAE = new(big.Rat).Quo(sum, big.NewRat(int64(to-from), 1))
	if !frozen {
		return candidate
	}
	level := forecastLearningLevelPredict(method, spend[:from])
	predictedTotal := new(big.Rat).Mul(level, big.NewRat(int64(to-from), 1))
	actualTotal := new(big.Rat)
	for _, value := range spend[from:to] {
		actualTotal.Add(actualTotal, value)
	}
	candidate.FrozenTotalAbsError = forecastLearningAbs(new(big.Rat).Sub(predictedTotal, actualTotal))
	return candidate
}

func forecastLearningLevelPredict(method string, prefix []*big.Rat) *big.Rat {
	if len(prefix) == 0 {
		return new(big.Rat)
	}
	switch method {
	case ForecastLearningMethodMean8:
		return forecastLearningMeanTail(prefix, 8)
	case ForecastLearningMethodLastPeriod:
		return new(big.Rat).Set(prefix[len(prefix)-1])
	case ForecastLearningMethodSES025:
		return forecastLearningSES(prefix, big.NewRat(1, 4))
	case ForecastLearningMethodSES050:
		return forecastLearningSES(prefix, big.NewRat(1, 2))
	default:
		return new(big.Rat)
	}
}

// forecastLearningSES is simple exponential smoothing with an exact rational
// alpha. There is no trend term: the forecast for every future period is the
// final level.
func forecastLearningSES(spend []*big.Rat, alpha *big.Rat) *big.Rat {
	level := new(big.Rat).Set(spend[0])
	complement := new(big.Rat).Sub(big.NewRat(1, 1), alpha)
	for _, value := range spend[1:] {
		next := new(big.Rat).Mul(alpha, value)
		next.Add(next, new(big.Rat).Mul(complement, level))
		level = next
	}
	return level
}

func selectForecastLearningSeasonal(periods []ForecastLearningPeriod) (ForecastLearningSelection, error) {
	total := len(periods)
	if total < 36 {
		return ForecastLearningSelection{}, fmt.Errorf("forecast learning seasonal selection needs 36 complete months, got %d", total)
	}
	spend := forecastLearningRats(periods)
	months, err := forecastLearningMonths(periods)
	if err != nil {
		return ForecastLearningSelection{}, err
	}
	testStart := total - forecastLearningSeasonalTest

	methods := []string{ForecastLearningMethodSameMonth2Y, ForecastLearningMethodSeasonalLastYear, ForecastLearningMethodFlatMean12}
	test := make([]ForecastLearningCandidate, 0, len(methods))
	for _, method := range methods {
		candidate, err := forecastLearningSeasonalFold(method, spend, months, testStart, total)
		if err != nil {
			return ForecastLearningSelection{}, err
		}
		test = append(test, candidate)
	}
	same, last, flat := test[0], test[1], test[2]

	selection := ForecastLearningSelection{
		Pattern:              ForecastLearningAnnualSeasonal,
		Unit:                 "month",
		TunedBaseline:        ForecastLearningMethodSeasonalLastYear,
		TestCandidates:       test,
		TestRangeStart:       periods[testStart].Start,
		TestRangeEnd:         periods[total-1].End,
		TestedHorizonPeriods: forecastLearningSeasonalTest,
		FittedPeriods:        total,
		Variation:            forecastLearningSeasonalVariation(periods, months),
	}
	selection.Selected, selection.FallbackReason = forecastLearningSeasonalGate(same, last)
	selection.SelectedIsLearned = selection.FallbackReason == ""

	selected := same
	if selection.Selected == ForecastLearningMethodSeasonalLastYear {
		selected = last
	}
	// The owner chose a seasonal pattern; a weak one stays seasonal and is
	// labelled, rather than being converted into a flat weekly average.
	if selected.OnePeriodMAE.Cmp(flat.OnePeriodMAE) >= 0 {
		selection.Warnings = append(selection.Warnings, ForecastLearningWarningWeakSeasonality)
	}
	for _, month := range selection.Variation.Months {
		if month.PositiveObservations == 1 {
			selection.Warnings = append(selection.Warnings, ForecastLearningWarningSparseAnnual)
			break
		}
	}

	// Refit the selected fixed method over the whole bounded window, one level
	// per calendar month number.
	selection.MonthlyForecast = make(map[time.Month]*big.Rat, 12)
	for month := time.January; month <= time.December; month++ {
		value, err := forecastLearningSeasonalPredict(selection.Selected, spend, months, month)
		if err != nil {
			return ForecastLearningSelection{}, err
		}
		selection.MonthlyForecast[month] = value
	}
	return selection, nil
}

// forecastLearningSeasonalGate protects peak months: the two-year mean must
// improve one-month accuracy materially and may not be worse on the frozen
// year's path, its annual total, or its worst single month.
func forecastLearningSeasonalGate(same, last ForecastLearningCandidate) (string, string) {
	switch {
	case last.OnePeriodMAE.Sign() == 0:
		return last.Method, ForecastLearningFallbackZeroErrorBaseline
	case !forecastLearningBeatsByTenPercent(same.OnePeriodMAE, last.OnePeriodMAE):
		return last.Method, ForecastLearningFallbackMarginNotMet
	case same.FrozenPathMAE.Cmp(last.FrozenPathMAE) > 0:
		return last.Method, ForecastLearningFallbackWorseFrozenPath
	case same.FrozenTotalAbsError.Cmp(last.FrozenTotalAbsError) > 0:
		return last.Method, ForecastLearningFallbackWorseAnnualTotal
	case same.FrozenMaxAbsError.Cmp(last.FrozenMaxAbsError) > 0:
		return last.Method, ForecastLearningFallbackWorseMaximumMonth
	default:
		return same.Method, ""
	}
}

// forecastLearningSeasonalFold measures twelve expanding one-month origins and
// one twelve-month path frozen at the first holdout origin.
func forecastLearningSeasonalFold(method string, spend []*big.Rat, months []time.Month, from, to int) (ForecastLearningCandidate, error) {
	candidate := ForecastLearningCandidate{Method: method}
	sum := new(big.Rat)
	for origin := from; origin < to; origin++ {
		predicted, err := forecastLearningSeasonalPredict(method, spend[:origin], months[:origin], months[origin])
		if err != nil {
			return candidate, err
		}
		sum.Add(sum, forecastLearningAbs(new(big.Rat).Sub(predicted, spend[origin])))
	}
	candidate.OnePeriodMAE = new(big.Rat).Quo(sum, big.NewRat(int64(to-from), 1))

	frozenSum, predictedTotal, actualTotal := new(big.Rat), new(big.Rat), new(big.Rat)
	maximum := new(big.Rat)
	for target := from; target < to; target++ {
		predicted, err := forecastLearningSeasonalPredict(method, spend[:from], months[:from], months[target])
		if err != nil {
			return candidate, err
		}
		absolute := forecastLearningAbs(new(big.Rat).Sub(predicted, spend[target]))
		frozenSum.Add(frozenSum, absolute)
		predictedTotal.Add(predictedTotal, predicted)
		actualTotal.Add(actualTotal, spend[target])
		if absolute.Cmp(maximum) > 0 {
			maximum = absolute
		}
	}
	candidate.FrozenPathMAE = new(big.Rat).Quo(frozenSum, big.NewRat(int64(to-from), 1))
	candidate.FrozenTotalAbsError = forecastLearningAbs(new(big.Rat).Sub(predictedTotal, actualTotal))
	candidate.FrozenMaxAbsError = maximum
	return candidate, nil
}

// forecastLearningSeasonalPredict uses only observations in the given prefix.
// A target more than twelve months ahead therefore reuses the same observed
// month-number values instead of consuming an earlier forecast.
func forecastLearningSeasonalPredict(method string, spend []*big.Rat, months []time.Month, target time.Month) (*big.Rat, error) {
	switch method {
	case ForecastLearningMethodFlatMean12:
		if len(spend) == 0 {
			return nil, fmt.Errorf("forecast learning flat reference needs at least one observed month")
		}
		return forecastLearningMeanTail(spend, 12), nil
	case ForecastLearningMethodSameMonth2Y, ForecastLearningMethodSeasonalLastYear:
		wanted := 1
		if method == ForecastLearningMethodSameMonth2Y {
			wanted = 2
		}
		matched := make([]*big.Rat, 0, wanted)
		for i := len(spend) - 1; i >= 0 && len(matched) < wanted; i-- {
			if months[i] == target {
				matched = append(matched, spend[i])
			}
		}
		if len(matched) == 0 {
			return nil, fmt.Errorf("forecast learning seasonal method %q has no observation of month %s", method, target)
		}
		mean := new(big.Rat)
		for _, value := range matched {
			mean.Add(mean, value)
		}
		return mean.Quo(mean, big.NewRat(int64(len(matched)), 1)), nil
	default:
		return nil, fmt.Errorf("unsupported forecast learning seasonal method %q", method)
	}
}

// forecastLearningBeatsByTenPercent is 10*learned <= 9*baseline, evaluated
// exactly. It is an engineering policy margin, not a significance test.
func forecastLearningBeatsByTenPercent(learned, baseline *big.Rat) bool {
	left := new(big.Rat).Mul(learned, big.NewRat(10, 1))
	right := new(big.Rat).Mul(baseline, big.NewRat(9, 1))
	return left.Cmp(right) <= 0
}

// forecastLearningBetter returns the lower-MAE method, preferring first on a
// tie. Callers pass the plan's tie winner (ses_025, mean_8) first.
func forecastLearningBetter(candidates []ForecastLearningCandidate, first, second string) string {
	var left, right *big.Rat
	for _, candidate := range candidates {
		switch candidate.Method {
		case first:
			left = candidate.OnePeriodMAE
		case second:
			right = candidate.OnePeriodMAE
		}
	}
	if left == nil || right == nil || left.Cmp(right) <= 0 {
		return first
	}
	return second
}

func forecastLearningMeanTail(spend []*big.Rat, count int) *big.Rat {
	start := max(0, len(spend)-count)
	sum := new(big.Rat)
	for _, value := range spend[start:] {
		sum.Add(sum, value)
	}
	return sum.Quo(sum, big.NewRat(int64(len(spend)-start), 1))
}

func forecastLearningRats(periods []ForecastLearningPeriod) []*big.Rat {
	values := make([]*big.Rat, len(periods))
	for i, period := range periods {
		values[i] = scaledIntRat(period.Spend)
	}
	return values
}

func forecastLearningMonths(periods []ForecastLearningPeriod) ([]time.Month, error) {
	months := make([]time.Month, len(periods))
	for i, period := range periods {
		start, err := time.Parse(time.DateOnly, period.Start)
		if err != nil {
			return nil, fmt.Errorf("parse forecast learning period start: %w", err)
		}
		months[i] = start.Month()
	}
	return months, nil
}

func forecastLearningAbs(value *big.Rat) *big.Rat {
	return value.Abs(value)
}

func forecastLearningLevelVariation(unit string, periods []ForecastLearningPeriod) ForecastLearningVariation {
	variation := ForecastLearningVariation{Kind: ForecastLearningVariationKind, Unit: unit, Periods: len(periods)}
	for _, period := range periods {
		if variation.Min == nil || period.Spend.Cmp(variation.Min) < 0 {
			variation.Min = period.Spend
		}
		if variation.Max == nil || period.Spend.Cmp(variation.Max) > 0 {
			variation.Max = period.Spend
		}
	}
	return variation
}

func forecastLearningSeasonalVariation(periods []ForecastLearningPeriod, months []time.Month) ForecastLearningVariation {
	variation := ForecastLearningVariation{Kind: ForecastLearningVariationKind, Unit: "month", Periods: len(periods)}
	byMonth := make(map[time.Month]*ForecastLearningMonthVariation, 12)
	for i, period := range periods {
		entry, ok := byMonth[months[i]]
		if !ok {
			entry = &ForecastLearningMonthVariation{Month: months[i], Min: period.Spend, Max: period.Spend}
			byMonth[months[i]] = entry
		}
		entry.Observations++
		if period.Spend.Sign() > 0 {
			entry.PositiveObservations++
		}
		if period.Spend.Cmp(entry.Min) < 0 {
			entry.Min = period.Spend
		}
		if period.Spend.Cmp(entry.Max) > 0 {
			entry.Max = period.Spend
		}
	}
	for month := time.January; month <= time.December; month++ {
		if entry, ok := byMonth[month]; ok {
			variation.Months = append(variation.Months, *entry)
		}
	}
	return variation
}
