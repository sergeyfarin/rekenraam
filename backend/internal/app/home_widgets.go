package app

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"math/big"
	"slices"
	"strings"
	"time"

	"rekenraam/backend/internal/db"
	"rekenraam/backend/internal/exact"
)

// Home widgets (#478).
//
// Each default My Home widget is a bounded summary of one canonical read: the
// Spending report, the budget month, account balances, the recurring templates
// and the Net worth series. Home adds no financial arithmetic of its own beyond
// choosing and ordering rows; a widget's figure is the canonical read's figure
// for the same dates, scope and currency.
//
// A widget's failure is its own. The briefing is Home's snapshot and fails the
// whole read; a widget that cannot be read carries its error instead, so one
// broken summary never hides the others or reads as an empty book.

// homeWidgetRows bounds a compact widget's list (three or four rows).
const (
	homeSpendingRows  = 3
	homeBudgetRows    = 3
	homeAccountRows   = 4
	homeRecurringRows = 3
	// homeTrendMonths is how far back the net worth trend reaches: the end of
	// the month twelve months before this one, then each month end, then today.
	homeTrendMonths = 12
)

type HomeService struct {
	transactions *TransactionService
	budgets      *BudgetService
	recurring    *RecurringService
}

func NewHomeService(transactions *TransactionService, budgets *BudgetService, recurring *RecurringService) *HomeService {
	return &HomeService{transactions: transactions, budgets: budgets, recurring: recurring}
}

// HomeWidget is one summary or the error that kept it from being read.
type HomeWidget[T any] struct {
	Summary *T
	Err     error
}

type Home struct {
	Briefing      HomeBriefing
	Spending      HomeWidget[HomeSpendingSummary]
	Budgets       HomeWidget[HomeBudgetSummary]
	Accounts      HomeWidget[HomeAccountsSummary]
	Recurring     HomeWidget[HomeRecurringSummary]
	NetWorthTrend HomeWidget[HomeNetWorthTrendSummary]
	// Commodities describes every commodity any summary names, so Home needs no
	// second read to label an amount.
	Commodities []db.ReportingCurrencyRecord
}

// HomeSpendingSummary is the Spending report for this month to date, grouped
// by category, in the book's default currency, over every account.
type HomeSpendingSummary struct {
	StartDate string
	EndDate   string
	// Total is the report's converted total; nil when there is no default
	// currency or any posting cannot be converted, never partial.
	Total           *BalanceQuantity
	CommodityTotals []BalanceQuantity
	// Categories are the report's first rows, in its order.
	Categories    []HomeSpendingCategory
	CategoryCount int
}

type HomeSpendingCategory struct {
	CategoryID int64
	Code       string
	Name       string
	// BuiltinKey labels a built-in or starter category that has no name.
	BuiltinKey string
	Converted  *BalanceQuantity
	Totals     []BalanceQuantity
}

// HomeBudgetSummary is this month's expense budget in one currency: the
// default currency when it has an expense target, otherwise the lowest
// commodity that does. Unlike currencies are never totalled together.
type HomeBudgetSummary struct {
	PeriodStart string
	PeriodEnd   string
	// CommodityID is 0 when no expense category has a target this month.
	CommodityID int64
	// OtherCommodityCount counts further currencies with an expense target.
	OtherCommodityCount int
	// Total is the budget month's expense total in CommodityID.
	Total *BudgetCategoryAmount
	// Categories are the targeted expense categories most in need of
	// attention: over budget first (most over first), then by the share of
	// the target used.
	Categories    []HomeBudgetCategory
	CategoryCount int
}

type HomeBudgetCategory struct {
	CategoryID int64
	Code       string
	Name       string
	BuiltinKey string
	Amount     BudgetCategoryAmount
}

// HomeAccountsSummary lists the largest open accounts until pins exist (#442).
// An account's balance includes the accounts under it, so a brokerage carries
// its holdings; nested accounts are not listed again. The balances come from
// the briefing's fold, so they share the headline's snapshot.
type HomeAccountsSummary struct {
	AsOf      string
	OpenCount int
	Accounts  []HomeAccountRow
}

type HomeAccountRow struct {
	AccountID    int64
	Code         string
	Name         string
	AccountClass string
	AccountKind  string
	// Balances are the account's exact balances including its subtree.
	Balances []BalanceQuantity
	// Value is Balances in the default currency at AsOf; nil when any part
	// cannot be converted. It orders the list, largest magnitude first.
	Value *BalanceQuantity
}

// HomeRecurringSummary counts the active schedules and lists the next due.
type HomeRecurringSummary struct {
	Today        string
	ActiveCount  int
	DraftCount   int
	BlockedCount int
	Upcoming     []HomeRecurringItem
}

type HomeRecurringItem struct {
	TemplateID int64
	Name       string
	NextDueOn  string
	// Amounts is the transaction's size per commodity: the sum of its debit
	// postings, the figure the Recurring page shows.
	Amounts []BalanceQuantity
}

// HomeNetWorthTrendSummary is the Net worth series from the end of the month
// twelve months ago to today, by month, in the default currency. It comes from
// the briefing's own fold, so it shares the headline's snapshot.
type HomeNetWorthTrendSummary struct {
	StartDate string
	EndDate   string
	Points    []HomeTrendPoint
	// Change is the last point minus the first, present only when both are.
	Change *BalanceQuantity
}

type HomeTrendPoint struct {
	Date  string
	Value *BalanceQuantity
}

// Home answers the whole composed Home read for owner-local today.
func (s *HomeService) Home(ctx context.Context, ownerUserID int64) (Home, error) {
	briefing, err := s.transactions.HomeBriefing(ctx, ownerUserID)
	if err != nil {
		return Home{}, err
	}
	home := Home{Briefing: briefing}
	var reportingID int64
	if briefing.ReportingCurrency != nil {
		reportingID = briefing.ReportingCurrency.ID
	}

	home.Spending = homeWidget(s.spendingSummary(ctx, briefing.AsOf, reportingID))
	home.Budgets = homeWidget(s.budgetSummary(ctx, briefing.AsOf, reportingID))
	home.Accounts = homeWidget(s.accountsSummary(ctx, briefing, reportingID))
	home.Recurring = homeWidget(s.recurringSummary(ctx, ownerUserID, briefing.AsOf))
	home.NetWorthTrend = homeWidget(netWorthTrendSummary(briefing))
	// A cancelled request is not a widget failure.
	for _, widgetErr := range []error{home.Spending.Err, home.Budgets.Err, home.Accounts.Err, home.Recurring.Err, home.NetWorthTrend.Err} {
		if errors.Is(widgetErr, context.Canceled) || errors.Is(widgetErr, context.DeadlineExceeded) {
			return Home{}, widgetErr
		}
	}

	if err := s.labelHomeCategories(ctx, home); err != nil {
		return Home{}, err
	}
	home.Commodities, err = s.homeCommodities(ctx, home)
	if err != nil {
		return Home{}, err
	}
	return home, nil
}

// labelHomeCategories gives the listed categories their built-in keys in one
// read, so a starter category without a name is labelled like everywhere else.
func (s *HomeService) labelHomeCategories(ctx context.Context, home Home) error {
	ids := []int64{}
	if summary := home.Spending.Summary; summary != nil {
		for _, category := range summary.Categories {
			ids = append(ids, category.CategoryID)
		}
	}
	if summary := home.Budgets.Summary; summary != nil {
		for _, category := range summary.Categories {
			ids = append(ids, category.CategoryID)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	accounts, err := s.transactions.accountRepository.AccountsByIDs(ctx, BookID, dedupeIDs(ids))
	if err != nil {
		return fmt.Errorf("label home categories: %w", err)
	}
	if summary := home.Spending.Summary; summary != nil {
		for index := range summary.Categories {
			summary.Categories[index].BuiltinKey = accounts[summary.Categories[index].CategoryID].BuiltinKey.String
		}
	}
	if summary := home.Budgets.Summary; summary != nil {
		for index := range summary.Categories {
			summary.Categories[index].BuiltinKey = accounts[summary.Categories[index].CategoryID].BuiltinKey.String
		}
	}
	return nil
}

func homeWidget[T any](summary *T, err error) HomeWidget[T] {
	if err != nil {
		return HomeWidget[T]{Err: err}
	}
	return HomeWidget[T]{Summary: summary}
}

func homeMonthStart(asOf string) string { return asOf[:8] + "01" }

func (s *HomeService) spendingSummary(ctx context.Context, asOf string, reportingID int64) (*HomeSpendingSummary, error) {
	input := SpendingInput{StartDate: homeMonthStart(asOf), EndDate: asOf, GroupBy: "category", Mode: "spending"}
	if reportingID != 0 {
		input.Reporting = &ReportingCurrencyInput{CommodityID: reportingID}
	}
	report, err := s.transactions.Spending(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("home spending: %w", err)
	}
	summary := &HomeSpendingSummary{StartDate: report.StartDate, EndDate: report.EndDate, Total: report.ConvertedTotal,
		CommodityTotals: report.CommodityTotals, Categories: []HomeSpendingCategory{}, CategoryCount: len(report.Groups)}
	for _, group := range report.Groups[:min(len(report.Groups), homeSpendingRows)] {
		category := HomeSpendingCategory{Code: group.Code, Name: group.Name, Converted: group.Converted, Totals: make([]BalanceQuantity, 0, len(group.Totals))}
		if group.CategoryID != nil {
			category.CategoryID = *group.CategoryID
		}
		for _, total := range group.Totals {
			category.Totals = append(category.Totals, BalanceQuantity{CommodityID: total.CommodityID, QuantityValue: total.QuantityValue, QuantityScale: total.QuantityScale, NormalQuantityValue: total.QuantityValue})
		}
		summary.Categories = append(summary.Categories, category)
	}
	return summary, nil
}

func (s *HomeService) budgetSummary(ctx context.Context, asOf string, reportingID int64) (*HomeBudgetSummary, error) {
	month, err := s.budgets.Month(ctx, homeMonthStart(asOf))
	if err != nil {
		return nil, fmt.Errorf("home budgets: %w", err)
	}
	summary := &HomeBudgetSummary{PeriodStart: month.PeriodStart, PeriodEnd: month.PeriodEnd, Categories: []HomeBudgetCategory{}}

	targeted := map[int64][]HomeBudgetCategory{}
	for _, category := range month.Categories {
		if category.CategoryType != "expense" {
			continue
		}
		for _, amount := range category.Amounts {
			if amount.Target.QuantityValue.Sign() > 0 {
				targeted[amount.CommodityID] = append(targeted[amount.CommodityID], HomeBudgetCategory{CategoryID: category.ID, Code: category.Code, Name: category.Name, Amount: amount})
			}
		}
	}
	if len(targeted) == 0 {
		return summary, nil
	}
	commodities := make([]int64, 0, len(targeted))
	for id := range targeted {
		commodities = append(commodities, id)
	}
	slices.Sort(commodities)
	summary.CommodityID = commodities[0]
	if targeted[reportingID] != nil {
		summary.CommodityID = reportingID
	}
	summary.OtherCommodityCount = len(commodities) - 1
	for _, total := range month.Totals {
		if total.CategoryType == "expense" && total.Amount.CommodityID == summary.CommodityID {
			amount := total.Amount
			summary.Total = &amount
		}
	}

	rows := targeted[summary.CommodityID]
	slices.SortStableFunc(rows, compareBudgetAttention)
	summary.CategoryCount = len(rows)
	summary.Categories = rows[:min(len(rows), homeBudgetRows)]
	return summary, nil
}

// compareBudgetAttention orders over-budget categories first, most over first,
// then the rest by the share of their target used, highest first. Every row
// has a positive target in one commodity, so shares compare exactly by
// cross-multiplying at a common scale.
func compareBudgetAttention(a, b HomeBudgetCategory) int {
	scaled := func(amount BudgetAmount) *exact.ScaledInt {
		return exact.ScaledIntFromCoefficient(amount.QuantityValue, amount.QuantityScale)
	}
	remainingA, remainingB := scaled(a.Amount.Remaining), scaled(b.Amount.Remaining)
	overA, overB := remainingA.Sign() < 0, remainingB.Sign() < 0
	switch {
	case overA && !overB:
		return -1
	case overB && !overA:
		return 1
	case overA && overB:
		if c := remainingA.Cmp(remainingB); c != 0 {
			return c
		}
	default:
		actualA, targetA, actualB, targetB := scaled(a.Amount.Actual), scaled(a.Amount.Target), scaled(b.Amount.Actual), scaled(b.Amount.Target)
		scale := max(actualA.Scale(), targetA.Scale(), actualB.Scale(), targetB.Scale())
		for _, value := range []*exact.ScaledInt{actualA, targetA, actualB, targetB} {
			value.Align(scale)
		}
		usedA := new(big.Int).Mul(actualA.BigInt(), targetB.BigInt())
		usedB := new(big.Int).Mul(actualB.BigInt(), targetA.BigInt())
		if c := usedB.Cmp(usedA); c != 0 {
			return c
		}
	}
	return cmp.Or(strings.Compare(a.Name, b.Name), cmp.Compare(a.CategoryID, b.CategoryID))
}

// accountsSummary ranks the balances the briefing's fold ended with: the
// account balances read for today, without a second pass over the ledger.
func (s *HomeService) accountsSummary(ctx context.Context, briefing HomeBriefing, reportingID int64) (*HomeAccountsSummary, error) {
	asOf := briefing.AsOf
	// The owner's open accounts, as the briefing counts them: asset and
	// liability accounts without system or per-instrument holding accounts.
	listed := map[int64]bool{}
	for _, account := range briefing.Accounts {
		if !account.IsSystem && account.Status == "active" && (account.AccountClass == "asset" || account.AccountClass == "liability") && !homeHoldingAccountKinds[account.AccountKind] {
			listed[account.AccountID] = true
		}
	}
	rows := make([]HomeAccountRow, 0, len(listed))
	seen := make([]int64, 0)
	for _, account := range briefing.Accounts {
		if !listed[account.AccountID] || (account.ParentAccountID != nil && listed[*account.ParentAccountID]) {
			continue
		}
		rows = append(rows, HomeAccountRow{AccountID: account.AccountID, Code: account.Code, Name: account.Name,
			AccountClass: account.AccountClass, AccountKind: account.AccountKind, Balances: account.SubtreeBalances})
		for _, balance := range account.SubtreeBalances {
			seen = append(seen, balance.CommodityID)
		}
	}
	if reportingID != 0 && len(rows) > 0 {
		rates, err := s.transactions.NewRateTable(ctx, ReportingCurrencyInput{CommodityID: reportingID}, seen, asOf, asOf)
		if err != nil {
			return nil, fmt.Errorf("home account values: %w", err)
		}
		for index := range rows {
			rows[index].Value = homeConverted(rates, rows[index].Balances, asOf)
		}
	}
	slices.SortStableFunc(rows, func(a, b HomeAccountRow) int {
		switch {
		case a.Value != nil && b.Value != nil:
			magnitudeA := new(big.Int).Abs(exact.ScaledIntFromCoefficient(a.Value.QuantityValue, a.Value.QuantityScale).BigInt())
			magnitudeB := new(big.Int).Abs(exact.ScaledIntFromCoefficient(b.Value.QuantityValue, b.Value.QuantityScale).BigInt())
			// One rate table converts every value to the same scale.
			if c := magnitudeB.Cmp(magnitudeA); c != 0 {
				return c
			}
		case a.Value != nil:
			return -1
		case b.Value != nil:
			return 1
		}
		return cmp.Or(strings.Compare(a.Name, b.Name), cmp.Compare(a.AccountID, b.AccountID))
	})
	return &HomeAccountsSummary{AsOf: asOf, OpenCount: len(rows), Accounts: rows[:min(len(rows), homeAccountRows)]}, nil
}

func (s *HomeService) recurringSummary(ctx context.Context, ownerUserID int64, asOf string) (*HomeRecurringSummary, error) {
	counts, err := s.recurring.Summary(ctx, ownerUserID)
	if err != nil {
		return nil, fmt.Errorf("home recurring counts: %w", err)
	}
	templates, err := s.recurring.ListTemplates(ctx, ownerUserID, false)
	if err != nil {
		return nil, fmt.Errorf("home recurring templates: %w", err)
	}
	summary := &HomeRecurringSummary{Today: asOf, DraftCount: counts.DraftCount, BlockedCount: counts.BlockedCount, Upcoming: []HomeRecurringItem{}}
	due := make([]RecurringTemplate, 0, len(templates))
	for _, template := range templates {
		if !template.Spec.Enabled || template.ArchivedAt != "" {
			continue
		}
		summary.ActiveCount++
		if template.NextDueOn != "" {
			due = append(due, template)
		}
	}
	slices.SortStableFunc(due, func(a, b RecurringTemplate) int {
		return cmp.Or(strings.Compare(a.NextDueOn, b.NextDueOn), strings.Compare(a.Spec.Name, b.Spec.Name), cmp.Compare(a.ID, b.ID))
	})
	for _, template := range due[:min(len(due), homeRecurringRows)] {
		postings := make([]db.RecurringDuePosting, 0, len(template.Spec.Postings))
		for _, posting := range template.Spec.Postings {
			postings = append(postings, db.RecurringDuePosting{CommodityID: posting.CommodityID, QuantityValue: posting.QuantityValue, QuantityScale: posting.QuantityScale})
		}
		amounts, err := recurringAmounts(postings)
		if err != nil {
			return nil, err
		}
		item := HomeRecurringItem{TemplateID: template.ID, Name: template.Spec.Name, NextDueOn: template.NextDueOn, Amounts: make([]BalanceQuantity, 0, len(amounts))}
		for _, amount := range amounts {
			item.Amounts = append(item.Amounts, BalanceQuantity{CommodityID: amount.CommodityID, QuantityValue: amount.DebitValue, QuantityScale: amount.QuantityScale, NormalQuantityValue: amount.DebitValue})
		}
		summary.Upcoming = append(summary.Upcoming, item)
	}
	return summary, nil
}

// netWorthTrendSummary reads the trend the briefing folded with its
// headlines: one snapshot, so the trend always ends on the headline.
func netWorthTrendSummary(briefing HomeBriefing) (*HomeNetWorthTrendSummary, error) {
	summary := &HomeNetWorthTrendSummary{StartDate: briefing.TrendStart, EndDate: briefing.AsOf, Points: briefing.NetWorthTrend}
	if len(summary.Points) > 1 {
		first, last := summary.Points[0].Value, summary.Points[len(summary.Points)-1].Value
		if first != nil && last != nil {
			change, err := exact.FromBig(new(big.Int).Sub(last.QuantityValue.BigInt(), first.QuantityValue.BigInt()))
			if err != nil {
				return nil, fmt.Errorf("home net worth trend change: %w", err)
			}
			summary.Change = &BalanceQuantity{CommodityID: last.CommodityID, QuantityValue: change, QuantityScale: last.QuantityScale, NormalQuantityValue: change}
		}
	}
	return summary, nil
}

// homeTrendStart is the end of the month twelve months before date's month.
func homeTrendStart(date string) (string, error) {
	parsed, err := time.Parse(time.DateOnly, date)
	if err != nil {
		return "", fmt.Errorf("parse home date %q: %w", date, err)
	}
	return time.Date(parsed.Year(), parsed.Month()-homeTrendMonths+1, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, -1).Format(time.DateOnly), nil
}

// homeCommodities reads every commodity a summary names in one query.
func (s *HomeService) homeCommodities(ctx context.Context, home Home) ([]db.ReportingCurrencyRecord, error) {
	ids := map[int64]bool{}
	add := func(quantities ...*BalanceQuantity) {
		for _, quantity := range quantities {
			if quantity != nil {
				ids[quantity.CommodityID] = true
			}
		}
	}
	addAll := func(quantities []BalanceQuantity) {
		for index := range quantities {
			add(&quantities[index])
		}
	}
	if summary := home.Spending.Summary; summary != nil {
		add(summary.Total)
		addAll(summary.CommodityTotals)
		for _, category := range summary.Categories {
			add(category.Converted)
			addAll(category.Totals)
		}
	}
	if summary := home.Budgets.Summary; summary != nil && summary.CommodityID != 0 {
		ids[summary.CommodityID] = true
	}
	if summary := home.Accounts.Summary; summary != nil {
		for _, row := range summary.Accounts {
			add(row.Value)
			addAll(row.Balances)
		}
	}
	if summary := home.Recurring.Summary; summary != nil {
		for _, item := range summary.Upcoming {
			addAll(item.Amounts)
		}
	}
	if summary := home.NetWorthTrend.Summary; summary != nil {
		for _, point := range summary.Points {
			add(point.Value)
		}
	}
	if len(ids) == 0 || s.transactions.pricingRepository == nil {
		return []db.ReportingCurrencyRecord{}, nil
	}
	sorted := make([]int64, 0, len(ids))
	for id := range ids {
		sorted = append(sorted, id)
	}
	slices.Sort(sorted)
	records, err := s.transactions.pricingRepository.ReportingCurrencies(ctx, BookID, sorted)
	if err != nil {
		return nil, err
	}
	result := make([]db.ReportingCurrencyRecord, 0, len(sorted))
	for _, id := range sorted {
		if record, ok := records[id]; ok {
			result = append(result, record)
		}
	}
	return result, nil
}
