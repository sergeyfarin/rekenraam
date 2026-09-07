package app

import (
	"fmt"
	"math/big"
	"sort"
	"time"

	"rekenraam/backend/internal/db"
	"rekenraam/backend/internal/exact"
)

type ForecastLearningPattern string

const (
	ForecastLearningDaily          ForecastLearningPattern = "daily"
	ForecastLearningWeekly         ForecastLearningPattern = "weekly"
	ForecastLearningMonthly        ForecastLearningPattern = "monthly"
	ForecastLearningAnnualSeasonal ForecastLearningPattern = "annual_seasonal"
)

type ForecastLearningGroup struct {
	FundingAccountID  int64
	CategoryAccountID int64
	CommodityID       int64
}

type ForecastLearningPurchase struct {
	Group          ForecastLearningGroup
	TransactionID  int64
	JournalEntryID int64
	EntryDate      string
	Amount         *exact.ScaledInt
}

type ForecastLearningClassification struct {
	Purchases []ForecastLearningPurchase
	Excluded  map[string]int
}

type ForecastLearningPeriod struct {
	Start string
	End   string
	Spend *exact.ScaledInt
}

type ForecastLearningEligibility struct {
	Eligible        bool
	Reason          string
	Periods         []ForecastLearningPeriod
	CompletePeriods int
	PositivePeriods int
	ActualStart     string
	ActualEnd       string
}

type ForecastLearningBaseline struct {
	Method string
	Amount *big.Rat
}

type ForecastLearningAllocation struct {
	Date  string
	Units *big.Int
}

type ForecastLearningKnownItem struct {
	Group         ForecastLearningGroup
	ProjectedDate string
	Amount        *exact.ScaledInt
	Ambiguous     bool
}

type ForecastLearningOverlap struct {
	Group      ForecastLearningGroup
	SourceType string
	SourceID   int64
}

// ForecastLearningRecurringOverlaps conservatively maps every funding/category
// combination implicated by an unarchived template or retained recurring
// draft. Ambiguous multi-funding shapes intentionally disable every Cartesian
// combination rather than guessing an allocation.
func ForecastLearningRecurringOverlaps(templates []db.ForecastTemplateRecord, templatePostings []db.ForecastTemplatePostingRecord, drafts []db.ForecastDraftPostingRecord, accountVersions []db.ForecastAccountVersionRecord, asOf string) []ForecastLearningOverlap {
	templateByID := map[int64]db.ForecastTemplateRecord{}
	for _, template := range templates {
		templateByID[template.ID] = template
	}
	result := make([]ForecastLearningOverlap, 0)
	byTemplate := map[int64][]forecastLearningLeg{}
	for _, posting := range templatePostings {
		template := templateByID[posting.TemplateID]
		if template.ArchivedAt.Valid {
			continue
		}
		byTemplate[posting.TemplateID] = append(byTemplate[posting.TemplateID], forecastLearningLeg{accountID: posting.AccountID, commodityID: posting.CommodityID, amount: exact.ScaledIntFromCoefficient(posting.QuantityValue, posting.QuantityScale)})
	}
	for id, legs := range byTemplate {
		result = append(result, learningOverlapsForLegs(legs, accountRulesAt(accountVersions, asOf), "template", id)...)
	}
	byDraft := map[int64][]forecastLearningLeg{}
	draftDates := map[int64]string{}
	for _, posting := range drafts {
		byDraft[posting.TransactionID] = append(byDraft[posting.TransactionID], forecastLearningLeg{accountID: posting.AccountID, commodityID: posting.CommodityID, amount: exact.ScaledIntFromCoefficient(posting.QuantityValue, posting.QuantityScale)})
		draftDates[posting.TransactionID] = posting.EntryDate
	}
	for id, legs := range byDraft {
		date := draftDates[id]
		if date > asOf {
			date = asOf
		}
		result = append(result, learningOverlapsForLegs(legs, accountRulesAt(accountVersions, date), "draft", id)...)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Group != result[j].Group {
			if result[i].Group.FundingAccountID != result[j].Group.FundingAccountID {
				return result[i].Group.FundingAccountID < result[j].Group.FundingAccountID
			}
			if result[i].Group.CategoryAccountID != result[j].Group.CategoryAccountID {
				return result[i].Group.CategoryAccountID < result[j].Group.CategoryAccountID
			}
			return result[i].Group.CommodityID < result[j].Group.CommodityID
		}
		if result[i].SourceType != result[j].SourceType {
			return result[i].SourceType < result[j].SourceType
		}
		return result[i].SourceID < result[j].SourceID
	})
	return result
}

type forecastLearningLeg struct {
	accountID   int64
	commodityID int64
	amount      *exact.ScaledInt
}

func learningOverlapsForLegs(legs []forecastLearningLeg, accounts map[int64]db.ForecastAccountVersionRecord, sourceType string, sourceID int64) []ForecastLearningOverlap {
	funders := make([]forecastLearningLeg, 0)
	expenses := make([]forecastLearningLeg, 0)
	for _, leg := range legs {
		account, ok := accounts[leg.accountID]
		if !ok || account.SystemRole.Valid || !account.AllowsPostings {
			continue
		}
		if (account.AccountClass == "asset" || account.AccountClass == "liability") && leg.amount.Sign() != 0 {
			funders = append(funders, leg)
		}
		if account.AccountClass == "expense" && leg.amount.Sign() != 0 {
			expenses = append(expenses, leg)
		}
	}
	result := make([]ForecastLearningOverlap, 0)
	seen := map[ForecastLearningGroup]bool{}
	for _, funder := range funders {
		for _, expense := range expenses {
			if funder.commodityID != expense.commodityID {
				continue
			}
			group := ForecastLearningGroup{FundingAccountID: funder.accountID, CategoryAccountID: expense.accountID, CommodityID: funder.commodityID}
			if !seen[group] {
				seen[group] = true
				result = append(result, ForecastLearningOverlap{Group: group, SourceType: sourceType, SourceID: sourceID})
			}
		}
	}
	return result
}

func ClassifyForecastLearningEntries(postings []db.ForecastLearningPostingRecord, accountVersions []db.ForecastAccountVersionRecord, commodityVersions []db.ForecastCommodityVersionRecord) ForecastLearningClassification {
	result := ForecastLearningClassification{Purchases: []ForecastLearningPurchase{}, Excluded: map[string]int{}}
	byEntry := map[int64][]db.ForecastLearningPostingRecord{}
	entryIDs := make([]int64, 0)
	for _, posting := range postings {
		if _, ok := byEntry[posting.JournalEntryID]; !ok {
			entryIDs = append(entryIDs, posting.JournalEntryID)
		}
		byEntry[posting.JournalEntryID] = append(byEntry[posting.JournalEntryID], posting)
	}
	sort.Slice(entryIDs, func(i, j int) bool { return entryIDs[i] < entryIDs[j] })
	for _, entryID := range entryIDs {
		purchases, reason := classifyForecastLearningEntry(byEntry[entryID], accountVersions, commodityVersions)
		if reason != "" {
			result.Excluded[reason]++
			continue
		}
		result.Purchases = append(result.Purchases, purchases...)
	}
	return result
}

func classifyForecastLearningEntry(rows []db.ForecastLearningPostingRecord, accountVersions []db.ForecastAccountVersionRecord, commodityVersions []db.ForecastCommodityVersionRecord) ([]ForecastLearningPurchase, string) {
	if len(rows) == 0 {
		return nil, "empty_entry"
	}
	first := rows[0]
	if first.RecurringOccurrenceID.Valid {
		return nil, "recurring_linked"
	}
	if first.TransactionKind != "ordinary" || first.EntryKind != "ordinary" {
		return nil, "unsupported_kind"
	}
	accounts := accountRulesAt(accountVersions, first.EntryDate)
	commodities := commodityRulesAt(commodityVersions, first.EntryDate)
	type accountCommodity struct{ account, commodity int64 }
	aggregated := map[accountCommodity]*exact.ScaledInt{}
	for _, row := range rows {
		if row.TransactionID != first.TransactionID || row.TransactionVersionID != first.TransactionVersionID || row.EntryDate != first.EntryDate || row.TransactionKind != first.TransactionKind || row.EntryKind != first.EntryKind || row.RecurringOccurrenceID.Valid {
			return nil, "inconsistent_entry"
		}
		key := accountCommodity{row.AccountID, row.CommodityID}
		account, ok := accounts[row.AccountID]
		if !ok {
			return nil, "ineligible_account"
		}
		if (account.AccountClass == "asset" || account.AccountClass == "liability") && row.QuantityValue.Sign() >= 0 {
			return nil, "mixed_sign_or_non_purchase"
		}
		if account.AccountClass == "expense" && row.QuantityValue.Sign() <= 0 {
			return nil, "refund_or_mixed_sign"
		}
		if aggregated[key] == nil {
			aggregated[key] = exact.NewScaledInt()
		}
		aggregated[key].AddCoefficient(row.QuantityValue, row.QuantityScale)
	}
	commodityID := int64(0)
	funders := make([]accountCommodity, 0, 1)
	expenses := make([]accountCommodity, 0)
	total := exact.NewScaledInt()
	for key, amount := range aggregated {
		if amount.Sign() == 0 {
			continue
		}
		if commodityID == 0 {
			commodityID = key.commodity
		}
		if key.commodity != commodityID || commodities[key.commodity].Kind != "currency" {
			return nil, "mixed_or_non_currency"
		}
		account, ok := accounts[key.account]
		if !ok || account.SystemRole.Valid || !account.AllowsPostings {
			return nil, "ineligible_account"
		}
		switch account.AccountClass {
		case "asset", "liability":
			if amount.Sign() >= 0 || account.AccountKind == "security_holding" {
				return nil, "mixed_sign_or_non_purchase"
			}
			funders = append(funders, key)
		case "expense":
			if amount.Sign() <= 0 {
				return nil, "refund_or_mixed_sign"
			}
			expenses = append(expenses, key)
		default:
			return nil, "income_equity_or_other"
		}
		total.AddScaled(amount)
	}
	if len(funders) != 1 || len(expenses) == 0 {
		return nil, "funding_shape"
	}
	if total.Sign() != 0 {
		return nil, "unbalanced_entry"
	}
	sort.Slice(expenses, func(i, j int) bool { return expenses[i].account < expenses[j].account })
	result := make([]ForecastLearningPurchase, 0, len(expenses))
	for _, expense := range expenses {
		result = append(result, ForecastLearningPurchase{
			Group:         ForecastLearningGroup{FundingAccountID: funders[0].account, CategoryAccountID: expense.account, CommodityID: commodityID},
			TransactionID: first.TransactionID, JournalEntryID: first.JournalEntryID, EntryDate: first.EntryDate,
			Amount: exact.ScaledIntFromBig(aggregated[expense].BigInt(), aggregated[expense].Scale()),
		})
	}
	return result, ""
}

func ForecastLearningPeriods(purchases []ForecastLearningPurchase, group ForecastLearningGroup, pattern ForecastLearningPattern, historyCompleteFrom, asOf string) (ForecastLearningEligibility, error) {
	confirmed, err := time.Parse(time.DateOnly, historyCompleteFrom)
	if err != nil {
		return ForecastLearningEligibility{}, fmt.Errorf("parse learning history start: %w", err)
	}
	origin, err := time.Parse(time.DateOnly, asOf)
	if err != nil {
		return ForecastLearningEligibility{}, fmt.Errorf("parse learning origin: %w", err)
	}
	var unit string
	var maxPeriods, minimum, minPositive int
	switch pattern {
	case ForecastLearningDaily, ForecastLearningWeekly:
		unit, maxPeriods, minimum, minPositive = "week", 52, 16, 8
	case ForecastLearningMonthly:
		unit, maxPeriods, minimum, minPositive = "month", 60, 16, 8
	case ForecastLearningAnnualSeasonal:
		unit, maxPeriods, minimum = "month", 60, 36
	default:
		return ForecastLearningEligibility{}, fmt.Errorf("unsupported forecast learning pattern %q", pattern)
	}
	firstStart := learningPeriodStart(confirmed, unit)
	if confirmed.After(firstStart) {
		firstStart = learningNextPeriod(firstStart, unit)
	}
	currentStart := learningPeriodStart(origin, unit)
	lastStart := learningPreviousPeriod(currentStart, unit)
	if firstStart.After(lastStart) {
		return ForecastLearningEligibility{Reason: learningInsufficientReason(pattern)}, nil
	}
	// Bound before materializing. A very old confirmation date must not create
	// an unbounded slice only to discard its prefix afterward.
	windowStart := lastStart
	for i := 1; i < maxPeriods; i++ {
		windowStart = learningPreviousPeriod(windowStart, unit)
	}
	if firstStart.Before(windowStart) {
		firstStart = windowStart
	}
	all := make([]ForecastLearningPeriod, 0)
	for start := firstStart; !start.After(lastStart); start = learningNextPeriod(start, unit) {
		end := learningNextPeriod(start, unit).AddDate(0, 0, -1)
		all = append(all, ForecastLearningPeriod{Start: start.Format(time.DateOnly), End: end.Format(time.DateOnly), Spend: exact.NewScaledInt()})
	}
	for _, purchase := range purchases {
		if purchase.Group != group {
			continue
		}
		for i := range all {
			if purchase.EntryDate >= all[i].Start && purchase.EntryDate <= all[i].End {
				all[i].Spend.AddScaled(purchase.Amount)
				break
			}
		}
	}
	positive := 0
	for _, period := range all {
		if period.Spend.Sign() > 0 {
			positive++
		}
	}
	eligible := len(all) >= minimum && positive >= minPositive
	reason := ""
	if !eligible {
		reason = learningInsufficientReason(pattern)
		if len(all) >= minimum && pattern != ForecastLearningAnnualSeasonal {
			reason = "sparse_history"
		}
	}
	if pattern == ForecastLearningAnnualSeasonal && len(all) >= 36 {
		blocksWithSpend := 0
		for block := 0; block < 3; block++ {
			hasSpend := false
			for _, period := range all[len(all)-12*(block+1) : len(all)-12*block] {
				hasSpend = hasSpend || period.Spend.Sign() > 0
			}
			if hasSpend {
				blocksWithSpend++
			}
		}
		eligible = blocksWithSpend >= 2
		if !eligible {
			reason = "sparse_history"
		}
	}
	result := ForecastLearningEligibility{Eligible: eligible, Reason: reason, Periods: all, CompletePeriods: len(all), PositivePeriods: positive}
	if len(all) > 0 {
		result.ActualStart, result.ActualEnd = all[0].Start, all[len(all)-1].End
	}
	return result, nil
}

func learningInsufficientReason(pattern ForecastLearningPattern) string {
	if pattern == ForecastLearningAnnualSeasonal {
		return "insufficient_seasonal_history"
	}
	return "insufficient_history"
}

func learningPeriodStart(date time.Time, unit string) time.Time {
	if unit == "month" {
		return time.Date(date.Year(), date.Month(), 1, 0, 0, 0, 0, time.UTC)
	}
	daysSinceMonday := (int(date.Weekday()) + 6) % 7
	return time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, time.UTC).AddDate(0, 0, -daysSinceMonday)
}

func learningNextPeriod(date time.Time, unit string) time.Time {
	if unit == "month" {
		return date.AddDate(0, 1, 0)
	}
	return date.AddDate(0, 0, 7)
}

func learningPreviousPeriod(date time.Time, unit string) time.Time {
	if unit == "month" {
		return date.AddDate(0, -1, 0)
	}
	return date.AddDate(0, 0, -7)
}

func ForecastLearningBaselines(periods []ForecastLearningPeriod) []ForecastLearningBaseline {
	if len(periods) == 0 {
		return nil
	}
	start := max(0, len(periods)-8)
	sum := new(big.Rat)
	for _, period := range periods[start:] {
		sum.Add(sum, scaledIntRat(period.Spend))
	}
	mean := new(big.Rat).Quo(sum, big.NewRat(int64(len(periods)-start), 1))
	last := scaledIntRat(periods[len(periods)-1].Spend)
	return []ForecastLearningBaseline{{Method: "mean_8", Amount: mean}, {Method: "last_period", Amount: last}}
}

func ForecastLearningSeasonalBaselines(periods []ForecastLearningPeriod, targetMonth time.Month) []ForecastLearningBaseline {
	matching := make([]*big.Rat, 0, 2)
	for i := len(periods) - 1; i >= 0 && len(matching) < 2; i-- {
		date, _ := time.Parse(time.DateOnly, periods[i].Start)
		if date.Month() == targetMonth {
			matching = append(matching, scaledIntRat(periods[i].Spend))
		}
	}
	if len(matching) == 0 {
		return nil
	}
	sameMonth := new(big.Rat).Set(matching[0])
	if len(matching) == 2 {
		sameMonth.Add(sameMonth, matching[1]).Quo(sameMonth, big.NewRat(2, 1))
	}
	flatStart := max(0, len(periods)-12)
	flat := new(big.Rat)
	for _, period := range periods[flatStart:] {
		flat.Add(flat, scaledIntRat(period.Spend))
	}
	flat.Quo(flat, big.NewRat(int64(len(periods)-flatStart), 1))
	return []ForecastLearningBaseline{{Method: "same_month_2y", Amount: sameMonth}, {Method: "seasonal_last_year", Amount: new(big.Rat).Set(matching[0])}, {Method: "flat_mean_12", Amount: flat}}
}

func scaledIntRat(value *exact.ScaledInt) *big.Rat {
	return new(big.Rat).SetFrac(value.BigInt(), exact.Pow10(value.Scale()))
}

// ForecastLearningResidual subtracts exact known spend before rounding once to
// output units. It returns a non-negative integer coefficient and the exact
// delta introduced by output rounding.
func ForecastLearningResidual(baseline *big.Rat, known *exact.ScaledInt, outputScale int) (*big.Int, *big.Rat) {
	residual := new(big.Rat).Sub(new(big.Rat).Set(baseline), scaledIntRat(known))
	if residual.Sign() <= 0 {
		return new(big.Int), new(big.Rat)
	}
	scaled := new(big.Rat).Mul(residual, new(big.Rat).SetInt(exact.Pow10(outputScale)))
	units := roundRatHalfAway(scaled)
	allocated := new(big.Rat).SetFrac(new(big.Int).Set(units), exact.Pow10(outputScale))
	return units, new(big.Rat).Sub(allocated, residual)
}

// ForecastLearningKnownPeriodSpend sums full-precision known eligible spend
// over the cadence period. ProjectedDate is already the core precedence date,
// so edited recurring drafts naturally reduce the period they now affect.
func ForecastLearningKnownPeriodSpend(items []ForecastLearningKnownItem, group ForecastLearningGroup, pattern ForecastLearningPattern, periodStart string) (*exact.ScaledInt, bool, error) {
	start, err := time.Parse(time.DateOnly, periodStart)
	if err != nil {
		return nil, false, fmt.Errorf("parse known-spend period: %w", err)
	}
	unit := "week"
	if pattern == ForecastLearningMonthly || pattern == ForecastLearningAnnualSeasonal {
		unit = "month"
	}
	end := learningNextPeriod(start, unit).AddDate(0, 0, -1).Format(time.DateOnly)
	total := exact.NewScaledInt()
	ambiguous := false
	for _, item := range items {
		if item.Group != group || item.ProjectedDate < periodStart || item.ProjectedDate > end {
			continue
		}
		if item.Ambiguous {
			ambiguous = true
			continue
		}
		if item.Amount != nil && item.Amount.Sign() > 0 {
			total.AddScaled(item.Amount)
		}
	}
	return total, ambiguous, nil
}

// ForecastLearningTimingBins learns transparent calendar weights from the last
// eight complete periods. Daily and annual patterns deliberately use uniform
// timing and therefore return no bins.
func ForecastLearningTimingBins(purchases []ForecastLearningPurchase, group ForecastLearningGroup, pattern ForecastLearningPattern, periods []ForecastLearningPeriod) []*big.Int {
	binCount := 0
	switch pattern {
	case ForecastLearningWeekly:
		binCount = 7
	case ForecastLearningMonthly:
		binCount = 31
	default:
		return nil
	}
	bins := make([]*big.Int, binCount)
	for i := range bins {
		bins[i] = new(big.Int)
	}
	start := 0
	if len(periods) > 8 {
		start = len(periods) - 8
	}
	if len(periods) == 0 {
		return bins
	}
	first, last := periods[start].Start, periods[len(periods)-1].End
	maxScale := 0
	for _, purchase := range purchases {
		if purchase.Group == group && purchase.EntryDate >= first && purchase.EntryDate <= last && purchase.Amount.Scale() > maxScale {
			maxScale = purchase.Amount.Scale()
		}
	}
	for _, purchase := range purchases {
		if purchase.Group != group || purchase.EntryDate < first || purchase.EntryDate > last {
			continue
		}
		date, err := time.Parse(time.DateOnly, purchase.EntryDate)
		if err != nil {
			continue
		}
		index := date.Day() - 1
		if pattern == ForecastLearningWeekly {
			index = (int(date.Weekday()) + 6) % 7
		}
		amount := purchase.Amount.BigInt()
		amount.Mul(amount, exact.Pow10(maxScale-purchase.Amount.Scale()))
		bins[index].Add(bins[index], amount)
	}
	return bins
}

// ForecastLearningAllocationDates returns the whole remaining calendar period,
// including private dates beyond the public horizon. Callers allocate first and
// clip later so an outside-E share is never moved into the visible forecast.
func ForecastLearningAllocationDates(pattern ForecastLearningPattern, periodStart, afterDate string) ([]string, error) {
	start, err := time.Parse(time.DateOnly, periodStart)
	if err != nil {
		return nil, fmt.Errorf("parse forecast learning period start: %w", err)
	}
	after, err := time.Parse(time.DateOnly, afterDate)
	if err != nil {
		return nil, fmt.Errorf("parse forecast learning allocation boundary: %w", err)
	}
	unit := "week"
	if pattern == ForecastLearningMonthly || pattern == ForecastLearningAnnualSeasonal {
		unit = "month"
	}
	end := learningNextPeriod(start, unit).AddDate(0, 0, -1)
	dates := make([]string, 0)
	for date := start; !date.After(end); date = date.AddDate(0, 0, 1) {
		if date.After(after) {
			dates = append(dates, date.Format(time.DateOnly))
		}
	}
	return dates, nil
}

func ForecastLearningDateWeights(pattern ForecastLearningPattern, dates []string, bins []*big.Int) ([]*big.Int, error) {
	weights := make([]*big.Int, len(dates))
	for i, raw := range dates {
		date, err := time.Parse(time.DateOnly, raw)
		if err != nil {
			return nil, fmt.Errorf("parse forecast learning allocation date: %w", err)
		}
		weights[i] = big.NewInt(1)
		switch pattern {
		case ForecastLearningWeekly:
			if len(bins) != 7 {
				return nil, fmt.Errorf("weekly forecast learning weights require seven bins")
			}
			weights[i] = new(big.Int).Set(bins[(int(date.Weekday())+6)%7])
		case ForecastLearningMonthly:
			if len(bins) != 31 {
				return nil, fmt.Errorf("monthly forecast learning weights require 31 bins")
			}
			weights[i] = new(big.Int).Set(bins[date.Day()-1])
			if date.AddDate(0, 0, 1).Month() != date.Month() {
				for day := date.Day() + 1; day <= 31; day++ {
					weights[i].Add(weights[i], bins[day-1])
				}
			}
		}
	}
	return weights, nil
}

func roundRatHalfAway(value *big.Rat) *big.Int {
	numerator := new(big.Int).Set(value.Num())
	sign := numerator.Sign()
	numerator.Abs(numerator)
	denominator := value.Denom()
	quotient, remainder := new(big.Int), new(big.Int)
	quotient.QuoRem(numerator, denominator, remainder)
	if new(big.Int).Lsh(remainder, 1).Cmp(denominator) >= 0 {
		quotient.Add(quotient, big.NewInt(1))
	}
	if sign < 0 {
		quotient.Neg(quotient)
	}
	return quotient
}

// AllocateForecastLearningUnits uses floor plus largest remainder. Dates are
// sorted and therefore provide the deterministic tie break required by M1.
func AllocateForecastLearningUnits(units *big.Int, dates []string, weights []*big.Int) ([]ForecastLearningAllocation, bool, error) {
	if units.Sign() < 0 || len(dates) != len(weights) {
		return nil, false, fmt.Errorf("invalid forecast learning allocation")
	}
	type candidate struct {
		date      string
		weight    *big.Int
		units     *big.Int
		remainder *big.Int
	}
	items := make([]candidate, len(dates))
	totalWeight := new(big.Int)
	for i := range dates {
		if weights[i] == nil || weights[i].Sign() < 0 {
			return nil, false, fmt.Errorf("invalid forecast learning weight")
		}
		items[i] = candidate{date: dates[i], weight: new(big.Int).Set(weights[i])}
		totalWeight.Add(totalWeight, weights[i])
	}
	uniform := totalWeight.Sign() == 0
	if uniform {
		totalWeight.SetInt64(int64(len(items)))
		for i := range items {
			items[i].weight.SetInt64(1)
		}
	}
	if len(items) == 0 {
		if units.Sign() == 0 {
			return []ForecastLearningAllocation{}, uniform, nil
		}
		return nil, uniform, fmt.Errorf("no remaining forecast learning dates")
	}
	allocated := new(big.Int)
	for i := range items {
		numerator := new(big.Int).Mul(units, items[i].weight)
		items[i].units, items[i].remainder = new(big.Int), new(big.Int)
		items[i].units.QuoRem(numerator, totalWeight, items[i].remainder)
		allocated.Add(allocated, items[i].units)
	}
	remaining := new(big.Int).Sub(units, allocated)
	sort.SliceStable(items, func(i, j int) bool {
		comparison := items[i].remainder.Cmp(items[j].remainder)
		if comparison != 0 {
			return comparison > 0
		}
		return items[i].date < items[j].date
	})
	for i := int64(0); i < remaining.Int64(); i++ {
		items[i%int64(len(items))].units.Add(items[i%int64(len(items))].units, big.NewInt(1))
	}
	sort.Slice(items, func(i, j int) bool { return items[i].date < items[j].date })
	result := make([]ForecastLearningAllocation, len(items))
	for i := range items {
		result[i] = ForecastLearningAllocation{Date: items[i].date, Units: items[i].units}
	}
	return result, uniform, nil
}
