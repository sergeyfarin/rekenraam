package app

import (
	"database/sql"
	"fmt"
	"sort"
	"time"

	"rekenraam/backend/internal/db"
	"rekenraam/backend/internal/exact"
)

type forecastNormalizedInput struct {
	HorizonDays         int
	AccountIDs          []int64
	IncludeDescendants  bool
	ReportingCurrencyID *int64
	FXMethod            string
	SpendingModel       string
	HistoryCompleteFrom string
	ExpenseCategoryIDs  []int64
	ExpensePatterns     []forecastCategoryPattern
}

// forecastCategoryPattern is one category-wide cadence override. This version
// deliberately has no per-funding-account variant: an override applies to the
// category across every selected funding account.
type forecastCategoryPattern struct {
	CategoryID int64
	Pattern    ForecastLearningPattern
}

func (i forecastNormalizedInput) learningEnabled() bool {
	return i.SpendingModel == ForecastSpendingModelAdaptiveV1
}

// patternFor resolves the cadence for a category. An absent override defaults
// to weekly, as the API contract states.
func (i forecastNormalizedInput) patternFor(categoryID int64) ForecastLearningPattern {
	for _, override := range i.ExpensePatterns {
		if override.CategoryID == categoryID {
			return override.Pattern
		}
	}
	return ForecastLearningWeekly
}

type forecastBounds struct {
	AsOf    string
	First   string
	Through string
}

type forecastScope struct {
	AccountIDs []int64
	Accounts   map[int64]db.ForecastAccountVersionRecord
}

func normalizeForecastInput(input ForecastInput) (forecastNormalizedInput, error) {
	horizon := input.HorizonDays
	if horizon == 0 {
		horizon = forecastDefaultHorizonDays
	}
	if horizon < 1 || horizon > forecastMaxHorizonDays {
		return forecastNormalizedInput{}, ValidationError{Message: "forecast horizon must be between 1 and 366 days"}
	}
	seen := map[int64]bool{}
	ids := make([]int64, 0, len(input.AccountIDs))
	for _, id := range input.AccountIDs {
		if id <= 0 {
			return forecastNormalizedInput{}, ValidationError{Message: "forecast account id is invalid"}
		}
		if !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	if len(ids) > forecastMaxRootAccounts {
		return forecastNormalizedInput{}, ValidationError{Message: "forecast accepts at most 100 root accounts"}
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	if (input.ReportingCurrencyID == nil) != (input.FXMethod == "") {
		return forecastNormalizedInput{}, ValidationError{Message: "reporting currency and fx method must be supplied together"}
	}
	if input.ReportingCurrencyID != nil {
		if *input.ReportingCurrencyID <= 0 {
			return forecastNormalizedInput{}, ValidationError{Message: "reporting currency is invalid"}
		}
		if input.FXMethod != "constant_as_of" {
			return forecastNormalizedInput{}, ValidationError{Message: "forecast fx method is invalid"}
		}
	}
	normalized := forecastNormalizedInput{HorizonDays: horizon, AccountIDs: ids, IncludeDescendants: input.IncludeDescendants, ReportingCurrencyID: input.ReportingCurrencyID, FXMethod: input.FXMethod}
	if err := normalizeForecastLearningInput(input, &normalized); err != nil {
		return forecastNormalizedInput{}, err
	}
	return normalized, nil
}

// normalizeForecastLearningInput validates the opt-in model parameters. With
// the model off, every model-specific parameter is an orphan and rejected, so
// the off request keeps exactly its pre-M3 meaning.
func normalizeForecastLearningInput(input ForecastInput, normalized *forecastNormalizedInput) error {
	model := input.SpendingModel
	if model == "" {
		model = ForecastSpendingModelOff
	}
	if model != ForecastSpendingModelOff && model != ForecastSpendingModelAdaptiveV1 {
		return ValidationError{Message: "spending model must be off or adaptive_v1"}
	}
	if model == ForecastSpendingModelOff {
		if input.HistoryCompleteFrom != "" || len(input.ExpenseCategoryIDs) > 0 || len(input.ExpensePatterns) > 0 {
			return ValidationError{Message: "spending model parameters require spending_model=adaptive_v1"}
		}
		normalized.SpendingModel = ForecastSpendingModelOff
		return nil
	}
	if input.HistoryCompleteFrom == "" {
		return ValidationError{Message: "history_complete_from is required with spending_model=adaptive_v1"}
	}
	confirmed, err := time.Parse(time.DateOnly, input.HistoryCompleteFrom)
	if err != nil {
		return ValidationError{Message: "history_complete_from must be an ISO 8601 date"}
	}
	categories := make([]int64, 0, len(input.ExpenseCategoryIDs))
	seen := map[int64]bool{}
	for _, id := range input.ExpenseCategoryIDs {
		if id <= 0 {
			return ValidationError{Message: "expense category id is invalid"}
		}
		if !seen[id] {
			seen[id] = true
			categories = append(categories, id)
		}
	}
	if len(categories) > ForecastLearningMaxCategories {
		return ValidationError{Message: "forecast accepts at most 20 expense categories"}
	}
	sort.Slice(categories, func(i, j int) bool { return categories[i] < categories[j] })
	if len(input.ExpensePatterns) > ForecastLearningMaxCategories {
		return ValidationError{Message: "forecast accepts at most 20 expense pattern overrides"}
	}
	overrides := make([]forecastCategoryPattern, 0, len(input.ExpensePatterns))
	overridden := map[int64]bool{}
	for _, override := range input.ExpensePatterns {
		if override.CategoryID <= 0 {
			return ValidationError{Message: "expense pattern category id is invalid"}
		}
		if overridden[override.CategoryID] {
			return ValidationError{Message: "expense pattern overrides must be unique per category"}
		}
		switch override.Pattern {
		case ForecastLearningDaily, ForecastLearningWeekly, ForecastLearningMonthly, ForecastLearningAnnualSeasonal:
		default:
			return ValidationError{Message: "expense pattern must be daily, weekly, monthly or annual_seasonal"}
		}
		// An override outside an explicit category selection would silently do
		// nothing, which reads as a working setting that is not applied.
		if len(categories) > 0 && !seen[override.CategoryID] {
			return ValidationError{Message: "expense pattern category is outside the selected expense categories"}
		}
		overridden[override.CategoryID] = true
		overrides = append(overrides, forecastCategoryPattern{CategoryID: override.CategoryID, Pattern: override.Pattern})
	}
	sort.Slice(overrides, func(i, j int) bool { return overrides[i].CategoryID < overrides[j].CategoryID })
	normalized.SpendingModel = ForecastSpendingModelAdaptiveV1
	normalized.HistoryCompleteFrom = confirmed.Format(time.DateOnly)
	normalized.ExpenseCategoryIDs = categories
	normalized.ExpensePatterns = overrides
	return nil
}

// validateForecastLearningCategories rejects IDs that are not expense accounts
// in this snapshot, resolved from the same as-of date as every other identity.
func validateForecastLearningCategories(versions []db.ForecastAccountVersionRecord, asOf string, input forecastNormalizedInput) error {
	if !input.learningEnabled() || len(input.ExpenseCategoryIDs) == 0 {
		return nil
	}
	current := accountRulesAt(versions, asOf)
	for _, id := range input.ExpenseCategoryIDs {
		account, ok := current[id]
		if !ok || account.SystemRole.Valid || account.AccountClass != "expense" || !account.AllowsPostings {
			return ValidationError{Message: "expense category must identify a postable expense account"}
		}
	}
	return nil
}

func validateForecastReportingCurrency(versions []db.ForecastCommodityVersionRecord, asOf string, input forecastNormalizedInput) error {
	if input.ReportingCurrencyID == nil {
		return nil
	}
	commodity, ok := commodityRulesAt(versions, asOf)[*input.ReportingCurrencyID]
	if !ok || commodity.Kind != "currency" {
		return ValidationError{Message: "reporting currency must identify an existing currency"}
	}
	return nil
}

func forecastDateBounds(nowUTC time.Time, timeZone string, horizon int) (forecastBounds, error) {
	location, err := time.LoadLocation(timeZone)
	if err != nil {
		return forecastBounds{}, fmt.Errorf("load forecast time zone: %w", err)
	}
	asOfTime, err := time.Parse(time.DateOnly, nowUTC.In(location).Format(time.DateOnly))
	if err != nil {
		return forecastBounds{}, fmt.Errorf("parse forecast date: %w", err)
	}
	through := asOfTime.AddDate(0, 0, horizon)
	if through.Year() > 9999 {
		return forecastBounds{}, ValidationError{Message: "forecast horizon exceeds the supported calendar"}
	}
	return forecastBounds{AsOf: asOfTime.Format(time.DateOnly), First: asOfTime.AddDate(0, 0, 1).Format(time.DateOnly), Through: through.Format(time.DateOnly)}, nil
}

func resolveForecastScope(versions []db.ForecastAccountVersionRecord, asOf string, input forecastNormalizedInput) (forecastScope, error) {
	current := accountRulesAt(versions, asOf)
	selected := map[int64]bool{}
	if len(input.AccountIDs) == 0 {
		for id, account := range current {
			if defaultForecastAccount(account) {
				selected[id] = true
			}
		}
	} else {
		for _, rootID := range input.AccountIDs {
			root, ok := current[rootID]
			if !ok || root.SystemRole.Valid || (root.AccountClass != "asset" && root.AccountClass != "liability") {
				return forecastScope{}, ValidationError{Message: "forecast account is invalid"}
			}
			if root.AllowsPostings && root.AccountKind == "security_holding" {
				return forecastScope{}, ValidationError{Message: "security holding accounts cannot be forecast"}
			}
			selected[rootID] = true
			if input.IncludeDescendants {
				for id := range current {
					if isForecastDescendant(id, rootID, current) {
						selected[id] = true
					}
				}
			}
		}
	}
	accounts := map[int64]db.ForecastAccountVersionRecord{}
	ids := make([]int64, 0, len(selected))
	for id := range selected {
		account := current[id]
		if account.AllowsPostings && account.AccountKind != "security_holding" && (account.AccountClass == "asset" || account.AccountClass == "liability") {
			accounts[id] = account
			ids = append(ids, id)
		}
	}
	if len(ids) > forecastMaxAccounts {
		return forecastScope{}, ErrForecastTooLarge
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return forecastScope{AccountIDs: ids, Accounts: accounts}, nil
}

func defaultForecastAccount(account db.ForecastAccountVersionRecord) bool {
	if account.SystemRole.Valid || account.Status != "active" || !account.AllowsPostings || (account.AccountClass != "asset" && account.AccountClass != "liability") {
		return false
	}
	switch account.AccountKind {
	case "cash", "checking", "savings", "brokerage_cash":
		return true
	default:
		return false
	}
}

func isForecastDescendant(id, root int64, accounts map[int64]db.ForecastAccountVersionRecord) bool {
	seen := map[int64]bool{}
	for id != root {
		if seen[id] {
			return false
		}
		seen[id] = true
		account, ok := accounts[id]
		if !ok || !account.ParentAccountID.Valid {
			return false
		}
		id = account.ParentAccountID.Int64
	}
	return true
}

func accountRulesAt(versions []db.ForecastAccountVersionRecord, date string) map[int64]db.ForecastAccountVersionRecord {
	result := map[int64]db.ForecastAccountVersionRecord{}
	for _, version := range versions {
		if version.EffectiveFrom > date {
			continue
		}
		current, ok := result[version.AccountID]
		if !ok || version.EffectiveFrom > current.EffectiveFrom || (version.EffectiveFrom == current.EffectiveFrom && version.VersionSeq > current.VersionSeq) {
			result[version.AccountID] = version
		}
	}
	return result
}

func commodityRulesAt(versions []db.ForecastCommodityVersionRecord, date string) map[int64]db.ForecastCommodityVersionRecord {
	result := map[int64]db.ForecastCommodityVersionRecord{}
	for _, version := range versions {
		if version.EffectiveFrom > date {
			continue
		}
		current, ok := result[version.CommodityID]
		if !ok || version.EffectiveFrom > current.EffectiveFrom || (version.EffectiveFrom == current.EffectiveFrom && version.VersionSeq > current.VersionSeq) {
			result[version.CommodityID] = version
		}
	}
	return result
}

type forecastPosting struct {
	AccountID   int64
	CommodityID int64
	Value       exact.Coefficient
	Scale       int
}

type forecastRuleLookup struct {
	accounts    map[int64][]db.ForecastAccountVersionRecord
	commodities map[int64][]db.ForecastCommodityVersionRecord
}

func newForecastRuleLookup(snapshot db.ForecastSnapshot) forecastRuleLookup {
	result := forecastRuleLookup{accounts: map[int64][]db.ForecastAccountVersionRecord{}, commodities: map[int64][]db.ForecastCommodityVersionRecord{}}
	for _, version := range snapshot.AccountVersions {
		result.accounts[version.AccountID] = append(result.accounts[version.AccountID], version)
	}
	for _, version := range snapshot.CommodityVersions {
		result.commodities[version.CommodityID] = append(result.commodities[version.CommodityID], version)
	}
	return result
}

func (r forecastRuleLookup) accountAt(id int64, date string) (db.ForecastAccountVersionRecord, bool) {
	var result db.ForecastAccountVersionRecord
	found := false
	for _, version := range r.accounts[id] {
		if version.EffectiveFrom <= date && (!found || version.EffectiveFrom > result.EffectiveFrom || (version.EffectiveFrom == result.EffectiveFrom && version.VersionSeq > result.VersionSeq)) {
			result, found = version, true
		}
	}
	return result, found
}

func (r forecastRuleLookup) commodityAt(id int64, date string) (db.ForecastCommodityVersionRecord, bool) {
	var result db.ForecastCommodityVersionRecord
	found := false
	for _, version := range r.commodities[id] {
		if version.EffectiveFrom <= date && (!found || version.EffectiveFrom > result.EffectiveFrom || (version.EffectiveFrom == result.EffectiveFrom && version.VersionSeq > result.VersionSeq)) {
			result, found = version, true
		}
	}
	return result, found
}

func validateForecastPostings(postings []forecastPosting, sourceDate, projectionDate string, rules forecastRuleLookup) bool {
	if len(postings) < 2 {
		return false
	}
	totals := map[int64]*exact.ScaledInt{}
	for _, posting := range postings {
		if !forecastPostingAllowed(posting, sourceDate, rules) || (projectionDate != sourceDate && !forecastPostingAllowed(posting, projectionDate, rules)) {
			return false
		}
		if totals[posting.CommodityID] == nil {
			totals[posting.CommodityID] = exact.NewScaledInt()
		}
		totals[posting.CommodityID].AddCoefficient(posting.Value, posting.Scale)
	}
	for _, total := range totals {
		if total.Sign() != 0 {
			return false
		}
	}
	return true
}

func forecastPostingAllowed(posting forecastPosting, date string, rules forecastRuleLookup) bool {
	account, ok := rules.accountAt(posting.AccountID, date)
	if !ok || account.Status != "active" || !account.AllowsPostings || date < account.OpenedOn || (account.ClosedOn.Valid && date > account.ClosedOn.String) {
		return false
	}
	if account.DefaultCommodityID.Valid && account.DefaultCommodityID.Int64 != posting.CommodityID {
		return false
	}
	if account.QuantityScaleOverride.Valid && int64(posting.Scale) > account.QuantityScaleOverride.Int64 {
		return false
	}
	commodity, ok := rules.commodityAt(posting.CommodityID, date)
	if !ok || commodity.Status != "active" || commodity.Kind != "currency" || posting.Scale < 0 || posting.Scale > commodity.MaxQuantityScale || posting.Scale > exact.MaxScaleForCommodityKind(commodity.Kind) {
		return false
	}
	return true
}

func pointerFromNull(value sql.NullInt64) *int64 {
	if !value.Valid {
		return nil
	}
	result := value.Int64
	return &result
}
