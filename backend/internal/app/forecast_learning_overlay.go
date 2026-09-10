package app

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"sort"
	"time"

	"rekenraam/backend/internal/db"
	"rekenraam/backend/internal/exact"
)

const (
	ForecastSpendingModelOff        = "off"
	ForecastSpendingModelAdaptiveV1 = "adaptive_v1"

	// ForecastLearningPolicyVersion names the whole learned-spending policy:
	// eligibility, candidates, gates and allocation. Changing any of them
	// changes this string.
	ForecastLearningPolicyVersion = "adaptive_spending_v1"

	ForecastLearningMaxCategories = 20
	forecastLearningHistoryWeeks  = 52
	forecastLearningHistoryMonths = 60
)

// Exclusion and warning codes are stable API vocabulary; the frontend keys its
// translations off them and must never show a raw backend message.
const (
	ForecastLearningExcludedInsufficientHistory  = "insufficient_history"
	ForecastLearningExcludedInsufficientSeasonal = "insufficient_seasonal_history"
	ForecastLearningExcludedSparseHistory        = "sparse_history"
	ForecastLearningExcludedRecurringOverlap     = "recurring_overlap"
	ForecastLearningExcludedAmbiguousKnown       = "ambiguous_known_spending"
	ForecastLearningExcludedResourceLimit        = "resource_limit"
	ForecastLearningExcludedBusy                 = "busy"
	ForecastLearningExcludedCalendarLimit        = "calendar_limit"
	ForecastLearningExcludedComputationTimeout   = "computation_timeout"
)

const (
	ForecastLearningWarningUniformTiming = "uniform_timing_fallback"
)

// Fallback reasons on the wire distinguish "the learned model was not better"
// from "the baseline was already perfect". Both mean a labelled baseline.
const (
	ForecastLearningFallbackNoImprovement = "no_validated_improvement"
	ForecastLearningFallbackPerfect       = "perfect_baseline"
)

const (
	ForecastLearningStatusReady       = "ready"
	ForecastLearningStatusPartial     = "partial"
	ForecastLearningStatusUnavailable = "unavailable"
)

// ForecastLearningOption is one selectable expense category.
type ForecastLearningOption struct {
	AccountID       int64
	Name            string
	Code            string
	BuiltinLabelKey string
	ParentAccountID *int64
}

type ForecastLearningExclusion struct {
	FundingAccountID  int64
	CategoryAccountID int64
	CommodityID       int64
	Pattern           string
	Reason            string
	SourceType        string
	SourceID          int64
}

// ForecastLearningError renders one retrospective error metric. Value is a
// decimal at Scale; Exact says whether that decimal is the whole rational or a
// rounded rendering of it.
type ForecastLearningError struct {
	Metric string
	Value  exact.Coefficient
	Scale  int
	Exact  bool
}

type ForecastLearningCandidateReport struct {
	Method string
	Errors []ForecastLearningError
}

type ForecastLearningMonthProfile struct {
	Month                int
	Observations         int
	PositiveObservations int
	Minimum              ForecastQuantity
	Maximum              ForecastQuantity
	Estimate             ForecastQuantity
}

type ForecastLearningGroupReport struct {
	FundingAccountID  int64
	CategoryAccountID int64
	CommodityID       int64
	Pattern           string
	PeriodUnit        string

	TrainingStart   string
	TrainingEnd     string
	CompletePeriods int
	PositivePeriods int

	SelectedMethod       string
	SelectedIsLearned    bool
	FallbackReason       string
	TunedSES             string
	TunedBaseline        string
	TuningRangeStart     string
	TuningRangeEnd       string
	TestRangeStart       string
	TestRangeEnd         string
	TestedHorizonPeriods int
	Candidates           []ForecastLearningCandidateReport

	MinimumObserved ForecastQuantity
	MaximumObserved ForecastQuantity
	MonthProfile    []ForecastLearningMonthProfile

	TimingAssumption string
	Warnings         []string
	EstimatedTotal   ForecastQuantity
}

type ForecastLearnedPoint struct {
	Date             string
	EstimatedDelta   ForecastQuantity
	ProjectedBalance ForecastQuantity
}

type ForecastLearnedSeries struct {
	AccountID         int64
	CommodityID       int64
	Points            []ForecastLearnedPoint
	Minimum           ForecastQuantity
	MinimumDate       string
	FirstNegativeDate string
}

type ForecastLearnedSpending struct {
	Status              string
	PolicyVersion       string
	Reason              string
	HistoryCompleteFrom string

	RequestedGroupCount int
	EligibleGroupCount  int
	ExcludedGroupCount  int

	CategoryOptions []ForecastLearningOption
	Exclusions      []ForecastLearningExclusion
	Groups          []ForecastLearningGroupReport
	Series          []ForecastLearnedSeries
	Totals          []ForecastLearnedSeries
	Converted       *ForecastLearnedSeries
	Events          []ForecastEvent
}

// forecastLearningWindow is the bounded history interval the extra read needs.
// A weekly-only request must not read five years because another cadence could
// theoretically want it.
func forecastLearningWindow(input forecastNormalizedInput, asOf string) (string, error) {
	origin, err := time.Parse(time.DateOnly, asOf)
	if err != nil {
		return "", fmt.Errorf("parse forecast learning origin: %w", err)
	}
	needsMonths := false
	for _, override := range input.ExpensePatterns {
		if override.Pattern == ForecastLearningMonthly || override.Pattern == ForecastLearningAnnualSeasonal {
			needsMonths = true
		}
	}
	// With no explicit override every category defaults to weekly.
	start := learningPeriodStart(origin, "week")
	for i := 0; i < forecastLearningHistoryWeeks; i++ {
		start = learningPreviousPeriod(start, "week")
	}
	if needsMonths {
		monthly := learningPeriodStart(origin, "month")
		for i := 0; i < forecastLearningHistoryMonths; i++ {
			monthly = learningPreviousPeriod(monthly, "month")
		}
		if monthly.Before(start) {
			start = monthly
		}
	}
	confirmed, err := time.Parse(time.DateOnly, input.HistoryCompleteFrom)
	if err != nil {
		return "", fmt.Errorf("parse forecast learning history start: %w", err)
	}
	if confirmed.After(start) {
		start = confirmed
	}
	return start.Format(time.DateOnly), nil
}

type forecastLearningBuild struct {
	input    forecastNormalizedInput
	bounds   forecastBounds
	scope    forecastScope
	snapshot db.ForecastSnapshot
	core     *ForecastResult
	accounts map[int64]db.ForecastAccountVersionRecord
	rules    map[int64]db.ForecastCommodityVersionRecord
}

// buildForecastLearnedSpending produces the opt-in overlay. It never mutates
// the core result's own points: estimates live only in the returned object.
func buildForecastLearnedSpending(ctx context.Context, fitter *ForecastLearningFitter, input forecastNormalizedInput,
	bounds forecastBounds, scope forecastScope, snapshot db.ForecastSnapshot, core *ForecastResult) (*ForecastLearnedSpending, error) {
	b := &forecastLearningBuild{
		input: input, bounds: bounds, scope: scope, snapshot: snapshot, core: core,
		accounts: accountRulesAt(snapshot.AccountVersions, bounds.AsOf),
		rules:    commodityRulesAt(snapshot.CommodityVersions, bounds.AsOf),
	}
	overlay := &ForecastLearnedSpending{
		Status:              ForecastLearningStatusUnavailable,
		PolicyVersion:       ForecastLearningPolicyVersion,
		HistoryCompleteFrom: input.HistoryCompleteFrom,
		CategoryOptions:     b.categoryOptions(),
		Exclusions:          []ForecastLearningExclusion{},
		Groups:              []ForecastLearningGroupReport{},
		Series:              []ForecastLearnedSeries{},
		Totals:              []ForecastLearnedSeries{},
		Events:              []ForecastEvent{},
	}

	classification := ClassifyForecastLearningEntries(snapshot.LearningPostings, snapshot.AccountVersions, snapshot.CommodityVersions)
	training, known := b.splitPurchases(classification.Purchases)
	groups := b.groupsOf(training)
	overlay.RequestedGroupCount = len(groups)
	if len(groups) > ForecastLearningGroupLimit {
		overlay.Reason = ForecastLearningExcludedResourceLimit
		return overlay, nil
	}

	disabled := map[ForecastLearningGroup]ForecastLearningOverlap{}
	for _, overlap := range ForecastLearningRecurringOverlaps(snapshot.Templates, snapshot.TemplatePostings, snapshot.DraftPostings, snapshot.AccountVersions, bounds.AsOf) {
		if _, seen := disabled[overlap.Group]; !seen {
			disabled[overlap.Group] = overlap
		}
	}

	histories := make([]ForecastLearningGroupHistory, 0, len(groups))
	eligibility := map[ForecastLearningGroup]ForecastLearningEligibility{}
	for _, group := range groups {
		pattern := b.input.patternFor(group.CategoryAccountID)
		if overlap, ok := disabled[group]; ok {
			overlay.Exclusions = append(overlay.Exclusions, b.exclusion(group, pattern, ForecastLearningExcludedRecurringOverlap, overlap.SourceType, overlap.SourceID))
			continue
		}
		periods, err := ForecastLearningPeriods(training, group, pattern, input.HistoryCompleteFrom, bounds.AsOf)
		if err != nil {
			return nil, err
		}
		if !periods.Eligible {
			overlay.Exclusions = append(overlay.Exclusions, b.exclusion(group, pattern, periods.Reason, "", 0))
			continue
		}
		eligibility[group] = periods
		histories = append(histories, ForecastLearningGroupHistory{Group: group, Pattern: pattern, Periods: periods.Periods})
	}

	fitted, err := fitter.Fit(ctx, ForecastLearningFitRequest{Groups: histories})
	if err != nil {
		var unavailable *ForecastLearningUnavailableError
		if errors.As(err, &unavailable) {
			overlay.Reason = forecastLearningOverlayReason(unavailable.Reason)
			overlay.Exclusions = []ForecastLearningExclusion{}
			return overlay, nil
		}
		return nil, err
	}

	for _, model := range fitted.Groups {
		report, series, events, err := b.estimate(model, eligibility[model.Group], training, known)
		if err != nil {
			return nil, err
		}
		if report == nil {
			overlay.Exclusions = append(overlay.Exclusions, b.exclusion(model.Group, string(model.Pattern), ForecastLearningExcludedAmbiguousKnown, "", 0))
			continue
		}
		overlay.Groups = append(overlay.Groups, *report)
		overlay.Series = append(overlay.Series, series...)
		overlay.Events = append(overlay.Events, events...)
	}

	overlay.EligibleGroupCount = len(overlay.Groups)
	overlay.ExcludedGroupCount = len(overlay.Exclusions)
	switch {
	case overlay.EligibleGroupCount == 0:
		overlay.Status = ForecastLearningStatusUnavailable
		if overlay.Reason == "" && overlay.ExcludedGroupCount > 0 {
			overlay.Reason = overlay.Exclusions[0].Reason
		}
		return overlay, nil
	case overlay.ExcludedGroupCount > 0:
		overlay.Status = ForecastLearningStatusPartial
	default:
		overlay.Status = ForecastLearningStatusReady
	}
	if err := b.combine(overlay); err != nil {
		return nil, err
	}
	if err := b.addConversion(overlay); err != nil {
		return nil, err
	}
	sortForecastEvents(overlay.Events)
	return overlay, nil
}

// forecastLearningOverlayReason maps a fitter refusal to its API code. A
// learning-only deadline is a computation timeout, not an infrastructure error.
func forecastLearningOverlayReason(reason string) string {
	switch reason {
	case ForecastLearningReasonBusy:
		return ForecastLearningExcludedBusy
	case ForecastLearningReasonDeadline, ForecastLearningReasonCanceled:
		return ForecastLearningExcludedComputationTimeout
	default:
		return ForecastLearningExcludedResourceLimit
	}
}

func (b *forecastLearningBuild) exclusion(group ForecastLearningGroup, pattern any, reason, sourceType string, sourceID int64) ForecastLearningExclusion {
	return ForecastLearningExclusion{
		FundingAccountID: group.FundingAccountID, CategoryAccountID: group.CategoryAccountID,
		CommodityID: group.CommodityID, Pattern: fmt.Sprint(pattern), Reason: reason,
		SourceType: sourceType, SourceID: sourceID,
	}
}

// categoryOptions lists the postable expense accounts an owner may select.
func (b *forecastLearningBuild) categoryOptions() []ForecastLearningOption {
	ids := make([]int64, 0, len(b.accounts))
	for id, account := range b.accounts {
		if account.SystemRole.Valid || account.AccountClass != "expense" || !account.AllowsPostings || account.Status != "active" {
			continue
		}
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	options := make([]ForecastLearningOption, 0, len(ids))
	for _, id := range ids {
		account := b.accounts[id]
		options = append(options, ForecastLearningOption{
			AccountID: id, Name: account.Name.String, Code: account.Code.String,
			BuiltinLabelKey: account.SystemRole.String, ParentAccountID: pointerFromNull(account.ParentAccountID),
		})
	}
	return options
}

// splitPurchases separates training history (dated through the as-of date)
// from known future spending the core projection already carries. Both come
// from the same classification of the same snapshot.
func (b *forecastLearningBuild) splitPurchases(purchases []ForecastLearningPurchase) (training, known []ForecastLearningPurchase) {
	for _, purchase := range purchases {
		if !b.inScope(purchase.Group) {
			continue
		}
		if purchase.EntryDate <= b.bounds.AsOf {
			training = append(training, purchase)
			continue
		}
		if purchase.EntryDate <= b.bounds.Through {
			known = append(known, purchase)
		}
	}
	return training, known
}

func (b *forecastLearningBuild) inScope(group ForecastLearningGroup) bool {
	if _, ok := b.scope.Accounts[group.FundingAccountID]; !ok {
		return false
	}
	if len(b.input.ExpenseCategoryIDs) == 0 {
		return true
	}
	return containsInt64(b.input.ExpenseCategoryIDs, group.CategoryAccountID)
}

func (b *forecastLearningBuild) groupsOf(purchases []ForecastLearningPurchase) []ForecastLearningGroup {
	seen := map[ForecastLearningGroup]bool{}
	groups := make([]ForecastLearningGroup, 0)
	for _, purchase := range purchases {
		if !seen[purchase.Group] {
			seen[purchase.Group] = true
			groups = append(groups, purchase.Group)
		}
	}
	sort.Slice(groups, func(i, j int) bool {
		if groups[i].FundingAccountID != groups[j].FundingAccountID {
			return groups[i].FundingAccountID < groups[j].FundingAccountID
		}
		if groups[i].CategoryAccountID != groups[j].CategoryAccountID {
			return groups[i].CategoryAccountID < groups[j].CategoryAccountID
		}
		return groups[i].CommodityID < groups[j].CommodityID
	})
	return groups
}

// outputScale is the funding account's currency standard scale, lowered by an
// account precision override when one applies.
func (b *forecastLearningBuild) outputScale(group ForecastLearningGroup) int {
	scale := b.rules[group.CommodityID].StandardScale
	if account, ok := b.accounts[group.FundingAccountID]; ok && account.QuantityScaleOverride.Valid && int(account.QuantityScaleOverride.Int64) < scale {
		scale = int(account.QuantityScaleOverride.Int64)
	}
	return scale
}

// estimate turns one fitted model into dated estimated outflows. It returns a
// nil report when ambiguous known spending makes the group unsafe to estimate.
func (b *forecastLearningBuild) estimate(model ForecastLearningGroupModel, periods ForecastLearningEligibility,
	training, known []ForecastLearningPurchase) (*ForecastLearningGroupReport, []ForecastLearnedSeries, []ForecastEvent, error) {
	selection := model.Selection
	scale := b.outputScale(model.Group)
	report := &ForecastLearningGroupReport{
		FundingAccountID: model.Group.FundingAccountID, CategoryAccountID: model.Group.CategoryAccountID,
		CommodityID: model.Group.CommodityID, Pattern: string(model.Pattern), PeriodUnit: selection.Unit,
		TrainingStart: periods.ActualStart, TrainingEnd: periods.ActualEnd,
		CompletePeriods: periods.CompletePeriods, PositivePeriods: periods.PositivePeriods,
		SelectedMethod: selection.Selected, SelectedIsLearned: selection.SelectedIsLearned,
		FallbackReason: forecastLearningWireFallback(selection.FallbackReason),
		TunedSES:       selection.TunedSES, TunedBaseline: selection.TunedBaseline,
		TuningRangeStart: selection.TuningRangeStart, TuningRangeEnd: selection.TuningRangeEnd,
		TestRangeStart: selection.TestRangeStart, TestRangeEnd: selection.TestRangeEnd,
		TestedHorizonPeriods: selection.TestedHorizonPeriods,
		Candidates:           forecastLearningCandidateReports(selection.TestCandidates, scale),
		Warnings:             append([]string{}, selection.Warnings...),
		TimingAssumption:     "observed_calendar_profile",
		MonthProfile:         []ForecastLearningMonthProfile{},
	}
	if selection.Variation.Min != nil {
		report.MinimumObserved = forecastLearningQuantity(selection.Variation.Min, scale)
		report.MaximumObserved = forecastLearningQuantity(selection.Variation.Max, scale)
	}

	bins := ForecastLearningTimingBins(training, model.Group, model.Pattern, periods.Periods)
	knownItems := b.knownItems(model.Group, known)
	estimated := map[string]*big.Int{}
	total := new(big.Int)
	uniform := false

	unit := selection.Unit
	origin, err := time.Parse(time.DateOnly, b.bounds.AsOf)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("parse forecast learning origin: %w", err)
	}
	through, err := time.Parse(time.DateOnly, b.bounds.Through)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("parse forecast learning horizon: %w", err)
	}
	for start := learningPeriodStart(origin, unit); !start.After(through); start = learningNextPeriod(start, unit) {
		level, err := forecastLearningLevelFor(selection, start)
		if err != nil {
			return nil, nil, nil, err
		}
		if level == nil {
			continue
		}
		periodStart := start.Format(time.DateOnly)
		knownSpend, ambiguous, err := ForecastLearningKnownPeriodSpend(knownItems, model.Group, model.Pattern, periodStart)
		if err != nil {
			return nil, nil, nil, err
		}
		if ambiguous {
			// A period whose known spending cannot be trusted must not be
			// topped up with an apparently complete estimate.
			return nil, nil, nil, nil
		}
		units, _ := ForecastLearningResidual(level, knownSpend, scale)
		if units.Sign() == 0 {
			continue
		}
		dates, err := ForecastLearningAllocationDates(model.Pattern, periodStart, b.bounds.AsOf)
		if err != nil {
			return nil, nil, nil, err
		}
		if len(dates) == 0 {
			continue
		}
		weights, err := ForecastLearningDateWeights(model.Pattern, dates, bins)
		if err != nil {
			return nil, nil, nil, err
		}
		allocations, fellBack, err := AllocateForecastLearningUnits(units, dates, weights)
		if err != nil {
			return nil, nil, nil, err
		}
		uniform = uniform || fellBack
		for _, allocation := range allocations {
			// The horizon truncates the tail of a period without inflating the
			// days that remain inside it.
			if allocation.Date > b.bounds.Through || allocation.Units.Sign() == 0 {
				continue
			}
			if estimated[allocation.Date] == nil {
				estimated[allocation.Date] = new(big.Int)
			}
			estimated[allocation.Date].Add(estimated[allocation.Date], allocation.Units)
			total.Add(total, allocation.Units)
		}
	}
	if uniform {
		report.Warnings = append(report.Warnings, ForecastLearningWarningUniformTiming)
		report.TimingAssumption = "uniform"
	}
	if model.Pattern == ForecastLearningAnnualSeasonal {
		report.MonthProfile = forecastLearningMonthProfiles(selection, scale)
	}
	coefficient, err := exact.FromBig(total)
	if err != nil {
		return nil, nil, nil, LedgerOverflowError{CommodityID: model.Group.CommodityID}
	}
	report.EstimatedTotal = ForecastQuantity{Value: coefficient, Scale: scale}

	series, events, err := b.seriesFor(model.Group, scale, estimated)
	if err != nil {
		return nil, nil, nil, err
	}
	return report, series, events, nil
}

// forecastLearningLevelFor resolves the period estimate. Seasonal groups read
// their own calendar month, so a July trip lands in July.
func forecastLearningLevelFor(selection ForecastLearningSelection, start time.Time) (*big.Rat, error) {
	if selection.Pattern != ForecastLearningAnnualSeasonal {
		return selection.Forecast, nil
	}
	level, ok := selection.MonthlyForecast[start.Month()]
	if !ok {
		return nil, fmt.Errorf("forecast learning seasonal model has no level for %s", start.Month())
	}
	return level, nil
}

// knownItems converts future posted purchases and the core projection's own
// future recurring events into exact known spend for the subtraction step.
func (b *forecastLearningBuild) knownItems(group ForecastLearningGroup, known []ForecastLearningPurchase) []ForecastLearningKnownItem {
	items := make([]ForecastLearningKnownItem, 0, len(known))
	for _, purchase := range known {
		if purchase.Group != group {
			continue
		}
		items = append(items, ForecastLearningKnownItem{Group: group, ProjectedDate: purchase.EntryDate, Amount: purchase.Amount})
	}
	// A saved draft or computed template touching this group would normally
	// have disabled it upstream. If one still reaches here the group is
	// ambiguous rather than estimable, so mark it and let the caller suppress.
	for _, event := range b.core.Events {
		if event.Source != "draft" && event.Source != "template" {
			continue
		}
		for _, amount := range event.Amounts {
			if amount.AccountID == group.FundingAccountID && amount.CommodityID == group.CommodityID {
				items = append(items, ForecastLearningKnownItem{Group: group, ProjectedDate: event.ProjectedDate, Amount: exact.NewScaledInt(), Ambiguous: true})
			}
		}
	}
	return items
}

// seriesFor turns dated estimate units into one funding-account series and its
// matching estimated events. Spending reduces a balance, so every estimated
// delta is negative.
func (b *forecastLearningBuild) seriesFor(group ForecastLearningGroup, scale int, estimated map[string]*big.Int) ([]ForecastLearnedSeries, []ForecastEvent, error) {
	if len(estimated) == 0 {
		return nil, nil, nil
	}
	commodity := b.rules[group.CommodityID]
	events := make([]ForecastEvent, 0, len(estimated))
	dates := make([]string, 0, len(estimated))
	for date := range estimated {
		dates = append(dates, date)
	}
	sort.Strings(dates)
	for _, date := range dates {
		units := new(big.Int).Neg(estimated[date])
		coefficient, err := exact.FromBig(units)
		if err != nil {
			return nil, nil, LedgerOverflowError{CommodityID: group.CommodityID}
		}
		events = append(events, ForecastEvent{
			Key:           fmt.Sprintf("estimate:%d:%d:%d:%s", group.FundingAccountID, group.CategoryAccountID, group.CommodityID, date),
			Source:        "estimated_spending",
			SourceDate:    date,
			ProjectedDate: date,
			Amounts: []ForecastEventAmount{{
				AccountID: group.FundingAccountID, CommodityID: group.CommodityID,
				CommodityCode: commodity.Code, Quantity: ForecastQuantity{Value: coefficient, Scale: scale},
			}},
		})
	}
	return []ForecastLearnedSeries{{AccountID: group.FundingAccountID, CommodityID: group.CommodityID}}, events, nil
}

// combine folds every group's estimated events onto the core projected curves.
// Core points are read, never modified.
func (b *forecastLearningBuild) combine(overlay *ForecastLearnedSpending) error {
	deltas := map[forecastPair]map[string]*exact.ScaledInt{}
	for _, event := range overlay.Events {
		for _, amount := range event.Amounts {
			pair := forecastPair{AccountID: amount.AccountID, CommodityID: amount.CommodityID}
			if deltas[pair] == nil {
				deltas[pair] = map[string]*exact.ScaledInt{}
			}
			if deltas[pair][event.ProjectedDate] == nil {
				deltas[pair][event.ProjectedDate] = exact.NewScaledInt()
			}
			deltas[pair][event.ProjectedDate].AddCoefficient(amount.Quantity.Value, amount.Quantity.Scale)
		}
	}
	series := make([]ForecastLearnedSeries, 0, len(b.core.Series))
	totals := map[int64]map[string]*exact.ScaledInt{}
	totalDeltas := map[int64]map[string]*exact.ScaledInt{}
	for _, core := range b.core.Series {
		pair := forecastPair{AccountID: core.AccountID, CommodityID: core.CommodityID}
		row, err := b.learnedSeries(core, deltas[pair])
		if err != nil {
			return err
		}
		if totals[core.CommodityID] == nil {
			totals[core.CommodityID] = map[string]*exact.ScaledInt{}
			totalDeltas[core.CommodityID] = map[string]*exact.ScaledInt{}
		}
		for _, point := range row.Points {
			if totals[core.CommodityID][point.Date] == nil {
				totals[core.CommodityID][point.Date] = exact.NewScaledInt()
			}
			totals[core.CommodityID][point.Date].AddCoefficient(point.ProjectedBalance.Value, point.ProjectedBalance.Scale)
			if totalDeltas[core.CommodityID][point.Date] == nil {
				totalDeltas[core.CommodityID][point.Date] = exact.NewScaledInt()
			}
			totalDeltas[core.CommodityID][point.Date].AddCoefficient(point.EstimatedDelta.Value, point.EstimatedDelta.Scale)
		}
		series = append(series, row)
	}
	overlay.Series = series
	overlay.Totals = make([]ForecastLearnedSeries, 0, len(totals))
	commodityIDs := make([]int64, 0, len(totals))
	for id := range totals {
		commodityIDs = append(commodityIDs, id)
	}
	sort.Slice(commodityIDs, func(i, j int) bool { return commodityIDs[i] < commodityIDs[j] })
	for _, id := range commodityIDs {
		aggregate, err := b.aggregateSeries(id, totals[id], totalDeltas[id])
		if err != nil {
			return err
		}
		overlay.Totals = append(overlay.Totals, aggregate)
	}
	return nil
}

func (b *forecastLearningBuild) learnedSeries(core ForecastSeries, deltas map[string]*exact.ScaledInt) (ForecastLearnedSeries, error) {
	result := ForecastLearnedSeries{AccountID: core.AccountID, CommodityID: core.CommodityID, Points: make([]ForecastLearnedPoint, 0, len(core.Points))}
	running := exact.NewScaledInt()
	var minimum *exact.ScaledInt
	for _, point := range core.Points {
		if delta, ok := deltas[point.Date]; ok {
			running.AddScaled(delta)
		}
		balance := exact.NewScaledInt()
		balance.AddCoefficient(point.ProjectedBalance.Value, point.ProjectedBalance.Scale)
		balance.AddScaled(running)
		if err := checkForecastValue(balance, core.CommodityID); err != nil {
			return ForecastLearnedSeries{}, err
		}
		estimatedDelta := exact.NewScaledInt()
		if delta, ok := deltas[point.Date]; ok {
			estimatedDelta.AddScaled(delta)
		}
		scale := point.ProjectedBalance.Scale
		result.Points = append(result.Points, ForecastLearnedPoint{
			Date:             point.Date,
			EstimatedDelta:   forecastLearningQuantity(estimatedDelta, scale),
			ProjectedBalance: forecastLearningQuantity(balance, scale),
		})
		if minimum == nil || balance.Cmp(minimum) < 0 {
			copied := exact.NewScaledInt()
			copied.AddScaled(balance)
			minimum, result.MinimumDate = copied, point.Date
			result.Minimum = forecastLearningQuantity(balance, scale)
		}
		if result.FirstNegativeDate == "" && balance.Sign() < 0 {
			result.FirstNegativeDate = point.Date
		}
	}
	return result, nil
}

func (b *forecastLearningBuild) aggregateSeries(commodityID int64, balances, deltas map[string]*exact.ScaledInt) (ForecastLearnedSeries, error) {
	result := ForecastLearnedSeries{CommodityID: commodityID, Points: make([]ForecastLearnedPoint, 0, len(balances))}
	dates := make([]string, 0, len(balances))
	for date := range balances {
		dates = append(dates, date)
	}
	sort.Strings(dates)
	scale := b.rules[commodityID].StandardScale
	var minimum *exact.ScaledInt
	for _, date := range dates {
		balance := balances[date]
		if err := checkForecastValue(balance, commodityID); err != nil {
			return ForecastLearnedSeries{}, err
		}
		if err := checkForecastValue(deltas[date], commodityID); err != nil {
			return ForecastLearnedSeries{}, err
		}
		result.Points = append(result.Points, ForecastLearnedPoint{
			Date:             date,
			EstimatedDelta:   forecastLearningQuantity(deltas[date], scale),
			ProjectedBalance: forecastLearningQuantity(balance, scale),
		})
		if minimum == nil || balance.Cmp(minimum) < 0 {
			minimum, result.MinimumDate = balance, date
			result.Minimum = forecastLearningQuantity(balance, scale)
		}
	}
	return result, nil
}

// addConversion restates the learned overlay on top of the already converted
// core curve. Its coverage is deliberately independent: a currency used only
// by a learned group may make this curve unavailable without changing the core
// valuation's complete status.
func (b *forecastLearningBuild) addConversion(overlay *ForecastLearnedSpending) error {
	if b.input.ReportingCurrencyID == nil || b.core.Valuation == nil || !b.core.Valuation.Complete || b.core.Converted == nil {
		return nil
	}
	quoteID := *b.input.ReportingCurrencyID
	quoteScale := b.core.Valuation.ReportingCurrencyScale
	rates := map[int64]db.ForecastRateRecord{}
	for _, rate := range b.snapshot.Rates {
		current, exists := rates[rate.BaseCommodityID]
		if rate.ValuationDate > b.bounds.AsOf || exists && !laterForecastRate(rate, current) {
			continue
		}
		rates[rate.BaseCommodityID] = rate
	}
	needed := map[int64]bool{}
	for _, event := range overlay.Events {
		for _, amount := range event.Amounts {
			if amount.Quantity.Value.Sign() != 0 {
				needed[amount.CommodityID] = true
			}
		}
	}
	for commodityID := range needed {
		if commodityID == quoteID {
			continue
		}
		rate, ok := rates[commodityID]
		if !ok {
			return nil
		}
		age, err := forecastDateAge(b.bounds.AsOf, rate.ValuationDate)
		if err != nil {
			return err
		}
		if age > forecastMaxRateStalenessDays {
			return nil
		}
	}

	deltas := map[string]*exact.ScaledInt{}
	for _, event := range overlay.Events {
		for _, amount := range event.Amounts {
			converted, err := convertForecastQuantity(amount.Quantity, amount.CommodityID, quoteID, quoteScale, rates)
			if err != nil {
				return err
			}
			if deltas[event.ProjectedDate] == nil {
				deltas[event.ProjectedDate] = exact.NewScaledInt()
			}
			deltas[event.ProjectedDate].AddCoefficient(converted.Value, converted.Scale)
		}
	}
	result := ForecastLearnedSeries{CommodityID: quoteID, Points: make([]ForecastLearnedPoint, 0, len(b.core.Converted.Points))}
	running := exact.NewScaledInt()
	var minimum *exact.ScaledInt
	for _, point := range b.core.Converted.Points {
		delta := exact.NewScaledInt()
		if value := deltas[point.Date]; value != nil {
			delta.AddScaled(value)
			running.AddScaled(value)
		}
		balance := exact.ScaledIntFromCoefficient(point.ProjectedBalance.Value, point.ProjectedBalance.Scale)
		balance.AddScaled(running)
		if err := checkForecastValue(delta, quoteID); err != nil {
			return err
		}
		if err := checkForecastValue(balance, quoteID); err != nil {
			return err
		}
		result.Points = append(result.Points, ForecastLearnedPoint{
			Date: point.Date, EstimatedDelta: forecastLearningQuantity(delta, quoteScale),
			ProjectedBalance: forecastLearningQuantity(balance, quoteScale),
		})
		if minimum == nil || balance.Cmp(minimum) < 0 {
			minimum = exact.NewScaledInt()
			minimum.AddScaled(balance)
			result.Minimum = forecastLearningQuantity(balance, quoteScale)
			result.MinimumDate = point.Date
		}
		if result.FirstNegativeDate == "" && balance.Sign() < 0 {
			result.FirstNegativeDate = point.Date
		}
	}
	overlay.Converted = &result
	return nil
}

func forecastLearningQuantity(value *exact.ScaledInt, scale int) ForecastQuantity {
	aligned := exact.NewScaledInt()
	aligned.AddScaled(value)
	aligned.Align(scale)
	coefficient, err := aligned.Coefficient()
	if err != nil {
		return ForecastQuantity{Value: exact.New(0), Scale: scale}
	}
	return ForecastQuantity{Value: coefficient, Scale: aligned.Scale()}
}

// forecastLearningWireFallback separates a model that simply was not better
// from a baseline that was already exact.
func forecastLearningWireFallback(reason string) string {
	switch reason {
	case "":
		return ""
	case ForecastLearningFallbackZeroErrorBaseline:
		return ForecastLearningFallbackPerfect
	default:
		return ForecastLearningFallbackNoImprovement
	}
}

func forecastLearningCandidateReports(candidates []ForecastLearningCandidate, scale int) []ForecastLearningCandidateReport {
	reports := make([]ForecastLearningCandidateReport, 0, len(candidates))
	for _, candidate := range candidates {
		report := ForecastLearningCandidateReport{Method: candidate.Method, Errors: []ForecastLearningError{}}
		for _, metric := range []struct {
			name  string
			value *big.Rat
		}{
			{"one_period_mae", candidate.OnePeriodMAE},
			{"frozen_total_abs_error", candidate.FrozenTotalAbsError},
			{"frozen_path_mae", candidate.FrozenPathMAE},
			{"frozen_max_abs_error", candidate.FrozenMaxAbsError},
		} {
			if metric.value == nil {
				continue
			}
			report.Errors = append(report.Errors, forecastLearningErrorValue(metric.name, metric.value, scale))
		}
		reports = append(reports, report)
	}
	return reports
}

// forecastLearningErrorValue renders an error metric with extra digits so that
// most values are exact, and says plainly when one had to be rounded.
func forecastLearningErrorValue(metric string, value *big.Rat, scale int) ForecastLearningError {
	reported := scale + 4
	scaled := new(big.Rat).Mul(value, new(big.Rat).SetInt(exact.Pow10(reported)))
	units := roundRatHalfAway(scaled)
	coefficient, err := exact.FromBig(units)
	if err != nil {
		return ForecastLearningError{Metric: metric, Value: exact.New(0), Scale: reported, Exact: false}
	}
	return ForecastLearningError{Metric: metric, Value: coefficient, Scale: reported, Exact: scaled.IsInt()}
}

func forecastLearningMonthProfiles(selection ForecastLearningSelection, scale int) []ForecastLearningMonthProfile {
	profiles := make([]ForecastLearningMonthProfile, 0, len(selection.Variation.Months))
	for _, month := range selection.Variation.Months {
		profile := ForecastLearningMonthProfile{
			Month: int(month.Month), Observations: month.Observations, PositiveObservations: month.PositiveObservations,
			Minimum: forecastLearningQuantity(month.Min, scale), Maximum: forecastLearningQuantity(month.Max, scale),
		}
		if level, ok := selection.MonthlyForecast[month.Month]; ok {
			scaled := new(big.Rat).Mul(level, new(big.Rat).SetInt(exact.Pow10(scale)))
			if coefficient, err := exact.FromBig(roundRatHalfAway(scaled)); err == nil {
				profile.Estimate = ForecastQuantity{Value: coefficient, Scale: scale}
			}
		}
		profiles = append(profiles, profile)
	}
	return profiles
}
