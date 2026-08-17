package app

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"rekenraam/backend/internal/db"
	"rekenraam/backend/internal/exact"
)

// LedgerOverflowError is returned when an accumulated balance exceeds the
// maximum coefficient precision. This can happen for large crypto holdings
// with many small postings at high scale.
type LedgerOverflowError struct {
	CommodityID int64
}

func (e LedgerOverflowError) Error() string {
	return fmt.Sprintf("balance for commodity %d exceeds maximum precision; try reducing posting scale or splitting the account", e.CommodityID)
}

type BalanceQuantity struct {
	CommodityID         int64
	QuantityValue       exact.Coefficient
	QuantityScale       int
	NormalQuantityValue exact.Coefficient
}

type AccountBalance struct {
	AccountID       int64
	BookID          int64
	IsSystem        bool
	SystemRole      string
	Status          string
	Code            string
	Name            string
	AccountClass    string
	AccountKind     string
	ParentAccountID *int64
	AllowsPostings  bool
	DirectBalances  []BalanceQuantity
	SubtreeBalances []BalanceQuantity
}

type AccountBalancesInput struct {
	AsOf          string
	Status        string
	IncludeSystem bool
}

type AccountBalancesResult struct {
	AsOf     string
	Status   string
	Accounts []AccountBalance
}

type CategoryTotal struct {
	CategoryID       int64
	BookID           int64
	Status           string
	Code             string
	Name             string
	CategoryType     string
	ParentCategoryID *int64
	AllowsPostings   bool
	DirectTotals     []BalanceQuantity
	SubtreeTotals    []BalanceQuantity
}

type CategoryTotalsInput struct {
	AfterDate    string
	BeforeDate   string
	Status       string
	CategoryType string
}

type CategoryTotalsResult struct {
	AfterDate  string
	BeforeDate string
	Status     string
	Categories []CategoryTotal
}

type NetWorthInput struct {
	AsOf   string
	Status string
}

type NetWorthResult struct {
	AsOf                string
	Status              string
	Totals              []BalanceQuantity
	ExcludedSystemRoles []string
}

type NetWorthSeriesInput struct {
	StartDate string
	EndDate   string
	Bucket    string
	Filters   ReportFilters
}

type NetWorthSeriesBucket struct {
	StartDate string
	EndDate   string
	Totals    []BalanceQuantity
}

type NetWorthSeriesResult struct {
	StartDate           string
	EndDate             string
	Bucket              string
	Filters             ReportFiltersEcho
	Buckets             []NetWorthSeriesBucket
	ExcludedSystemRoles []string
}

func (s *TransactionService) AccountBalances(ctx context.Context, input AccountBalancesInput) (AccountBalancesResult, error) {
	asOf, status, err := cleanLedgerAsOfAndStatus(input.AsOf, input.Status, s.now)
	if err != nil {
		return AccountBalancesResult{}, err
	}

	accounts, err := s.repository.LedgerAccountsAsOf(ctx, BookID, asOf)
	if err != nil {
		return AccountBalancesResult{}, fmt.Errorf("read accounts as of date: %w", err)
	}
	postings, err := s.repository.LedgerPostingsThrough(ctx, BookID, asOf, status)
	if err != nil {
		return AccountBalancesResult{}, fmt.Errorf("read ledger postings: %w", err)
	}

	direct := aggregatePostings(postings)
	accountMap := ledgerAccountMap(accounts)
	if !input.IncludeSystem {
		direct = excludeSystemAccountBalances(accountMap, direct)
	}
	subtree := rollupBalances(accountMap, direct)
	resultAccounts := make([]AccountBalance, 0, len(accounts))
	for _, account := range accounts {
		if account.SystemRole.Valid && !input.IncludeSystem {
			continue
		}
		accountBalance, err := toAccountBalance(account, direct[account.ID], subtree[account.ID])
		if err != nil {
			return AccountBalancesResult{}, err
		}
		resultAccounts = append(resultAccounts, accountBalance)
	}

	return AccountBalancesResult{
		AsOf:     asOf,
		Status:   status,
		Accounts: resultAccounts,
	}, nil
}

func (s *TransactionService) CategoryTotals(ctx context.Context, input CategoryTotalsInput) (CategoryTotalsResult, error) {
	status, err := cleanLedgerStatus(input.Status)
	if err != nil {
		return CategoryTotalsResult{}, err
	}
	afterDate, err := cleanOptionalDate(input.AfterDate, "after date")
	if err != nil {
		return CategoryTotalsResult{}, err
	}
	beforeDate, err := cleanOptionalDate(input.BeforeDate, "before date")
	if err != nil {
		return CategoryTotalsResult{}, err
	}
	if beforeDate == "" {
		beforeDate = s.now().UTC().Format(time.DateOnly)
	}
	if afterDate != "" && afterDate > beforeDate {
		return CategoryTotalsResult{}, ValidationError{Message: "after date must be on or before before date"}
	}
	categoryType := strings.TrimSpace(input.CategoryType)
	if categoryType != "" && categoryType != "income" && categoryType != "expense" {
		return CategoryTotalsResult{}, ValidationError{Message: "category type is invalid"}
	}

	accounts, err := s.repository.LedgerAccountsAsOf(ctx, BookID, beforeDate)
	if err != nil {
		return CategoryTotalsResult{}, fmt.Errorf("read category accounts: %w", err)
	}
	postings, err := s.repository.LedgerCategoryPostings(ctx, BookID, afterDate, beforeDate, status, categoryType)
	if err != nil {
		return CategoryTotalsResult{}, fmt.Errorf("read category postings: %w", err)
	}

	direct := aggregatePostings(postings)
	accountMap := ledgerAccountMap(accounts)
	subtree := rollupBalances(accountMap, direct)
	categories := make([]CategoryTotal, 0)
	for _, account := range accounts {
		if account.SystemRole.Valid || (account.AccountClass != "income" && account.AccountClass != "expense") {
			continue
		}
		if categoryType != "" && account.AccountClass != categoryType {
			continue
		}
		categoryTotal, err := toCategoryTotal(account, direct[account.ID], subtree[account.ID])
		if err != nil {
			return CategoryTotalsResult{}, err
		}
		categories = append(categories, categoryTotal)
	}

	return CategoryTotalsResult{
		AfterDate:  afterDate,
		BeforeDate: beforeDate,
		Status:     status,
		Categories: categories,
	}, nil
}

func (s *TransactionService) NetWorth(ctx context.Context, input NetWorthInput) (NetWorthResult, error) {
	asOf, status, err := cleanLedgerAsOfAndStatus(input.AsOf, input.Status, s.now)
	if err != nil {
		return NetWorthResult{}, err
	}
	totals, err := s.netWorthTotals(ctx, asOf, status, ReportFilters{})
	if err != nil {
		return NetWorthResult{}, err
	}

	return NetWorthResult{
		AsOf:                asOf,
		Status:              status,
		Totals:              totals,
		ExcludedSystemRoles: []string{"commodity_trading"},
	}, nil
}

// NetWorthSeries returns exact asset/liability totals at each calendar bucket
// end. Reports deliberately include posted records only: drafts, voided
// transactions, and soft-deleted transactions are not part of financial truth.
func (s *TransactionService) NetWorthSeries(ctx context.Context, input NetWorthSeriesInput) (NetWorthSeriesResult, error) {
	startDate, endDate, bucket, err := cleanNetWorthSeriesInput(input)
	if err != nil {
		return NetWorthSeriesResult{}, err
	}

	// Descendants are resolved per reporting date inside netWorthTotals; the
	// echo uses the series end date so the response names one stable expansion.
	filters, _, err := s.resolveReportFilters(ctx, input.Filters, endDate)
	if err != nil {
		return NetWorthSeriesResult{}, err
	}

	bounds, err := calendarBucketBounds(startDate, endDate, bucket)
	if err != nil {
		return NetWorthSeriesResult{}, err
	}
	resultBuckets := make([]NetWorthSeriesBucket, 0, len(bounds))
	for _, bound := range bounds {
		totals, err := s.netWorthTotals(ctx, bound.endDate, "posted", input.Filters)
		if err != nil {
			return NetWorthSeriesResult{}, err
		}
		resultBuckets = append(resultBuckets, NetWorthSeriesBucket{
			StartDate: bound.startDate,
			EndDate:   bound.endDate,
			Totals:    totals,
		})
	}

	return NetWorthSeriesResult{
		StartDate:           startDate,
		EndDate:             endDate,
		Bucket:              bucket,
		Filters:             filters,
		Buckets:             resultBuckets,
		ExcludedSystemRoles: []string{"commodity_trading"},
	}, nil
}

func (s *TransactionService) netWorthTotals(ctx context.Context, asOf string, status string, filters ReportFilters) ([]BalanceQuantity, error) {

	accounts, err := s.repository.LedgerAccountsAsOf(ctx, BookID, asOf)
	if err != nil {
		return nil, fmt.Errorf("read net worth accounts: %w", err)
	}
	postings, err := s.repository.LedgerPostingsThrough(ctx, BookID, asOf, status)
	if err != nil {
		return nil, fmt.Errorf("read net worth postings: %w", err)
	}

	// Account descendants are resolved as of this bucket's own date, because
	// parent links are versioned and a reparenting must not retroactively move
	// history between buckets.
	var accountFilter map[int64]bool
	if len(filters.AccountIDs) > 0 {
		selected := dedupeIDs(filters.AccountIDs)
		if filters.IncludeDescendants {
			selected, err = s.reportAccountSubtreeIDs(ctx, selected, asOf)
			if err != nil {
				return nil, err
			}
		}
		accountFilter = make(map[int64]bool, len(selected))
		for _, accountID := range selected {
			accountFilter[accountID] = true
		}
	}
	var commodityFilter map[int64]bool
	if len(filters.CommodityIDs) > 0 {
		commodityFilter = make(map[int64]bool, len(filters.CommodityIDs))
		for _, commodityID := range filters.CommodityIDs {
			commodityFilter[commodityID] = true
		}
	}

	accountMap := ledgerAccountMap(accounts)
	totals := map[int64]*exact.ScaledInt{}
	for _, posting := range postings {
		account, ok := accountMap[posting.AccountID]
		if !ok || (account.AccountClass != "asset" && account.AccountClass != "liability") {
			continue
		}
		if account.SystemRole.Valid && account.SystemRole.String == "commodity_trading" {
			continue
		}
		if accountFilter != nil && !accountFilter[posting.AccountID] {
			continue
		}
		if commodityFilter != nil && !commodityFilter[posting.CommodityID] {
			continue
		}
		addPosting(totals, posting)
	}
	quantities, err := balanceMapToQuantities(totals, "")
	if err != nil {
		return nil, err
	}
	return quantities, nil
}

type calendarBucketBound struct {
	startDate string
	endDate   string
}

func cleanNetWorthSeriesInput(input NetWorthSeriesInput) (string, string, string, error) {
	startDate, err := cleanRequiredDate(strings.TrimSpace(input.StartDate), "start date")
	if err != nil {
		return "", "", "", err
	}
	endDate, err := cleanRequiredDate(strings.TrimSpace(input.EndDate), "end date")
	if err != nil {
		return "", "", "", err
	}
	if startDate > endDate {
		return "", "", "", ValidationError{Message: "start date must be on or before end date"}
	}
	bucket := strings.TrimSpace(input.Bucket)
	if !reportBuckets[bucket] {
		return "", "", "", ValidationError{Message: "bucket must be day, week, month, quarter, or year"}
	}
	return startDate, endDate, bucket, nil
}

var reportBuckets = map[string]bool{
	"day":     true,
	"week":    true,
	"month":   true,
	"quarter": true,
	"year":    true,
}

func calendarBucketBounds(startDate string, endDate string, bucket string) ([]calendarBucketBound, error) {
	start, err := time.Parse(time.DateOnly, startDate)
	if err != nil {
		return nil, fmt.Errorf("parse validated start date: %w", err)
	}
	end, err := time.Parse(time.DateOnly, endDate)
	if err != nil {
		return nil, fmt.Errorf("parse validated end date: %w", err)
	}

	bounds := make([]calendarBucketBound, 0)
	current := start
	for !current.After(end) {
		bucketEnd := calendarBucketEnd(current, bucket)
		if bucketEnd.After(end) {
			bucketEnd = end
		}
		bounds = append(bounds, calendarBucketBound{
			startDate: current.Format(time.DateOnly),
			endDate:   bucketEnd.Format(time.DateOnly),
		})
		current = bucketEnd.AddDate(0, 0, 1)
	}
	return bounds, nil
}

func calendarBucketEnd(date time.Time, bucket string) time.Time {
	switch bucket {
	case "day":
		return date
	case "week":
		daysUntilSunday := (int(time.Sunday) - int(date.Weekday()) + 7) % 7
		return date.AddDate(0, 0, daysUntilSunday)
	case "month":
		return time.Date(date.Year(), date.Month()+1, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, -1)
	case "quarter":
		quarterStartMonth := ((int(date.Month())-1)/3)*3 + 1
		return time.Date(date.Year(), time.Month(quarterStartMonth+3), 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, -1)
	case "year":
		return time.Date(date.Year()+1, time.January, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, -1)
	default:
		panic("validated report bucket")
	}
}

func (s *TransactionService) accountRegisterRunningBalances(ctx context.Context, params db.ListTransactionsParams, entries []db.AccountRegisterEntryRecord) (map[int64]BalanceQuantity, error) {
	if len(entries) == 0 {
		return map[int64]BalanceQuantity{}, nil
	}
	postings, err := s.repository.AccountRegisterRunningPostings(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("read register running postings: %w", err)
	}

	running := map[int64]*exact.ScaledInt{}
	byPostingID := map[int64]BalanceQuantity{}
	for _, posting := range postings {
		addPosting(running, posting)
		amount := running[posting.CommodityID]
		value, err := amount.Coefficient()
		if err != nil {
			return nil, LedgerOverflowError{CommodityID: posting.CommodityID}
		}
		byPostingID[posting.PostingID] = BalanceQuantity{
			CommodityID:         posting.CommodityID,
			QuantityValue:       value,
			QuantityScale:       amount.Scale(),
			NormalQuantityValue: value,
		}
	}

	return byPostingID, nil
}

func cleanLedgerAsOfAndStatus(asOfInput string, statusInput string, now func() time.Time) (string, string, error) {
	asOf := strings.TrimSpace(asOfInput)
	if asOf == "" {
		asOf = now().UTC().Format(time.DateOnly)
	}
	cleanedAsOf, err := cleanRequiredDate(asOf, "as of date")
	if err != nil {
		return "", "", err
	}
	status, err := cleanLedgerStatus(statusInput)
	if err != nil {
		return "", "", err
	}
	return cleanedAsOf, status, nil
}

var ledgerStatuses = map[string]bool{
	"posted": true,
	"voided": true,
}

func cleanLedgerStatus(value string) (string, error) {
	status := strings.TrimSpace(value)
	if status == "" {
		return "posted", nil
	}
	if !ledgerStatuses[status] {
		return "", ValidationError{Message: "ledger status must be 'posted' or 'voided'"}
	}
	return status, nil
}

func ledgerAccountMap(accounts []db.LedgerAccountRecord) map[int64]db.LedgerAccountRecord {
	accountMap := make(map[int64]db.LedgerAccountRecord, len(accounts))
	for _, account := range accounts {
		accountMap[account.ID] = account
	}
	return accountMap
}

func aggregatePostings(postings []db.LedgerPostingRecord) map[int64]map[int64]*exact.ScaledInt {
	balances := map[int64]map[int64]*exact.ScaledInt{}
	for _, posting := range postings {
		accountBalances := balances[posting.AccountID]
		if accountBalances == nil {
			accountBalances = map[int64]*exact.ScaledInt{}
			balances[posting.AccountID] = accountBalances
		}
		addPosting(accountBalances, posting)
	}
	return balances
}

func excludeSystemAccountBalances(accounts map[int64]db.LedgerAccountRecord, balances map[int64]map[int64]*exact.ScaledInt) map[int64]map[int64]*exact.ScaledInt {
	filtered := map[int64]map[int64]*exact.ScaledInt{}
	for accountID, accountBalances := range balances {
		account, ok := accounts[accountID]
		if ok && account.SystemRole.Valid {
			continue
		}
		filtered[accountID] = accountBalances
	}
	return filtered
}

func addPosting(amounts map[int64]*exact.ScaledInt, posting db.LedgerPostingRecord) {
	amount := amounts[posting.CommodityID]
	if amount == nil {
		amount = exact.NewScaledInt()
		amounts[posting.CommodityID] = amount
	}
	amount.AddCoefficient(posting.QuantityValue, posting.QuantityScale)
}

func rollupBalances(accounts map[int64]db.LedgerAccountRecord, direct map[int64]map[int64]*exact.ScaledInt) map[int64]map[int64]*exact.ScaledInt {
	rolled := map[int64]map[int64]*exact.ScaledInt{}
	for accountID, balances := range direct {
		currentID := accountID
		seen := map[int64]bool{}
		for {
			if seen[currentID] {
				break
			}
			seen[currentID] = true
			target := rolled[currentID]
			if target == nil {
				target = map[int64]*exact.ScaledInt{}
				rolled[currentID] = target
			}
			addBalanceMap(target, balances)
			account, ok := accounts[currentID]
			if !ok || !account.ParentAccountID.Valid {
				break
			}
			currentID = account.ParentAccountID.Int64
		}
	}
	return rolled
}

func addBalanceMap(target map[int64]*exact.ScaledInt, source map[int64]*exact.ScaledInt) {
	for commodityID, amount := range source {
		targetAmount := target[commodityID]
		if targetAmount == nil {
			targetAmount = exact.NewScaledInt()
			target[commodityID] = targetAmount
		}
		targetAmount.AddScaled(amount)
	}
}

func balanceMapToQuantities(balances map[int64]*exact.ScaledInt, normalClass string) ([]BalanceQuantity, error) {
	if len(balances) == 0 {
		return []BalanceQuantity{}, nil
	}
	quantities := make([]BalanceQuantity, 0, len(balances))
	for commodityID, amount := range balances {
		value, err := amount.Coefficient()
		if err != nil {
			return nil, LedgerOverflowError{CommodityID: commodityID}
		}
		normalValue := value
		if normalClass == "liability" || normalClass == "equity" || normalClass == "income" {
			normalValue = normalValue.Negated()
		}
		quantities = append(quantities, BalanceQuantity{
			CommodityID:         commodityID,
			QuantityValue:       value,
			QuantityScale:       amount.Scale(),
			NormalQuantityValue: normalValue,
		})
	}
	return quantities, nil
}

func toAccountBalance(account db.LedgerAccountRecord, direct map[int64]*exact.ScaledInt, subtree map[int64]*exact.ScaledInt) (AccountBalance, error) {
	directBalances, err := balanceMapToQuantities(direct, account.AccountClass)
	if err != nil {
		return AccountBalance{}, err
	}
	subtreeBalances, err := balanceMapToQuantities(subtree, account.AccountClass)
	if err != nil {
		return AccountBalance{}, err
	}
	return AccountBalance{
		AccountID:       account.ID,
		BookID:          account.BookID,
		IsSystem:        account.SystemRole.Valid,
		SystemRole:      nullableString(account.SystemRole),
		Status:          account.Status,
		Code:            nullableString(account.Code),
		Name:            nullableString(account.Name),
		AccountClass:    account.AccountClass,
		AccountKind:     account.AccountKind,
		ParentAccountID: nullableSQLInt64Ptr(account.ParentAccountID),
		AllowsPostings:  account.AllowsPostings,
		DirectBalances:  directBalances,
		SubtreeBalances: subtreeBalances,
	}, nil
}

func toCategoryTotal(account db.LedgerAccountRecord, direct map[int64]*exact.ScaledInt, subtree map[int64]*exact.ScaledInt) (CategoryTotal, error) {
	directTotals, err := balanceMapToQuantities(direct, account.AccountClass)
	if err != nil {
		return CategoryTotal{}, err
	}
	subtreeTotals, err := balanceMapToQuantities(subtree, account.AccountClass)
	if err != nil {
		return CategoryTotal{}, err
	}
	return CategoryTotal{
		CategoryID:       account.ID,
		BookID:           account.BookID,
		Status:           account.Status,
		Code:             nullableString(account.Code),
		Name:             nullableString(account.Name),
		CategoryType:     account.AccountClass,
		ParentCategoryID: nullableSQLInt64Ptr(account.ParentAccountID),
		AllowsPostings:   account.AllowsPostings,
		DirectTotals:     directTotals,
		SubtreeTotals:    subtreeTotals,
	}, nil
}

func nullableSQLInt64Ptr(value sql.NullInt64) *int64 {
	if !value.Valid {
		return nil
	}
	copied := value.Int64
	return &copied
}
