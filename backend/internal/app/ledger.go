package app

import (
	"context"
	"database/sql"
	"fmt"
	"math/big"
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

	accounts, err := s.repository.LedgerAccountsAsOf(ctx, BookID, asOf)
	if err != nil {
		return NetWorthResult{}, fmt.Errorf("read net worth accounts: %w", err)
	}
	postings, err := s.repository.LedgerPostingsThrough(ctx, BookID, asOf, status)
	if err != nil {
		return NetWorthResult{}, fmt.Errorf("read net worth postings: %w", err)
	}

	accountMap := ledgerAccountMap(accounts)
	totals := map[int64]*scaledAmount{}
	for _, posting := range postings {
		account, ok := accountMap[posting.AccountID]
		if !ok || (account.AccountClass != "asset" && account.AccountClass != "liability") {
			continue
		}
		if account.SystemRole.Valid && account.SystemRole.String == "commodity_trading" {
			continue
		}
		addPosting(totals, posting)
	}
	quantities, err := balanceMapToQuantities(totals, "")
	if err != nil {
		return NetWorthResult{}, err
	}

	return NetWorthResult{
		AsOf:                asOf,
		Status:              status,
		Totals:              quantities,
		ExcludedSystemRoles: []string{"commodity_trading"},
	}, nil
}

func (s *TransactionService) accountRegisterRunningBalances(ctx context.Context, params db.ListTransactionsParams, entries []db.AccountRegisterEntryRecord) (map[int64]BalanceQuantity, error) {
	if len(entries) == 0 {
		return map[int64]BalanceQuantity{}, nil
	}
	postings, err := s.repository.AccountRegisterRunningPostings(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("read register running postings: %w", err)
	}

	running := map[int64]*scaledAmount{}
	byPostingID := map[int64]BalanceQuantity{}
	for _, posting := range postings {
		addPosting(running, posting)
		amount := running[posting.CommodityID]
		value, err := amount.coefficient()
		if err != nil {
			return nil, LedgerOverflowError{CommodityID: posting.CommodityID}
		}
		byPostingID[posting.PostingID] = BalanceQuantity{
			CommodityID:         posting.CommodityID,
			QuantityValue:       value,
			QuantityScale:       amount.scale,
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

func aggregatePostings(postings []db.LedgerPostingRecord) map[int64]map[int64]*scaledAmount {
	balances := map[int64]map[int64]*scaledAmount{}
	for _, posting := range postings {
		accountBalances := balances[posting.AccountID]
		if accountBalances == nil {
			accountBalances = map[int64]*scaledAmount{}
			balances[posting.AccountID] = accountBalances
		}
		addPosting(accountBalances, posting)
	}
	return balances
}

func excludeSystemAccountBalances(accounts map[int64]db.LedgerAccountRecord, balances map[int64]map[int64]*scaledAmount) map[int64]map[int64]*scaledAmount {
	filtered := map[int64]map[int64]*scaledAmount{}
	for accountID, accountBalances := range balances {
		account, ok := accounts[accountID]
		if ok && account.SystemRole.Valid {
			continue
		}
		filtered[accountID] = accountBalances
	}
	return filtered
}

func addPosting(amounts map[int64]*scaledAmount, posting db.LedgerPostingRecord) {
	amount := amounts[posting.CommodityID]
	if amount == nil {
		amount = newScaledAmount()
		amounts[posting.CommodityID] = amount
	}
	amount.add(posting.QuantityValue, posting.QuantityScale)
}

func rollupBalances(accounts map[int64]db.LedgerAccountRecord, direct map[int64]map[int64]*scaledAmount) map[int64]map[int64]*scaledAmount {
	rolled := map[int64]map[int64]*scaledAmount{}
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
				target = map[int64]*scaledAmount{}
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

func addBalanceMap(target map[int64]*scaledAmount, source map[int64]*scaledAmount) {
	for commodityID, amount := range source {
		targetAmount := target[commodityID]
		if targetAmount == nil {
			targetAmount = newScaledAmount()
			target[commodityID] = targetAmount
		}
		targetAmount.addScaled(amount)
	}
}

func balanceMapToQuantities(balances map[int64]*scaledAmount, normalClass string) ([]BalanceQuantity, error) {
	if len(balances) == 0 {
		return []BalanceQuantity{}, nil
	}
	quantities := make([]BalanceQuantity, 0, len(balances))
	for commodityID, amount := range balances {
		value, err := amount.coefficient()
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
			QuantityScale:       amount.scale,
			NormalQuantityValue: normalValue,
		})
	}
	return quantities, nil
}

func toAccountBalance(account db.LedgerAccountRecord, direct map[int64]*scaledAmount, subtree map[int64]*scaledAmount) (AccountBalance, error) {
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

func toCategoryTotal(account db.LedgerAccountRecord, direct map[int64]*scaledAmount, subtree map[int64]*scaledAmount) (CategoryTotal, error) {
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

type scaledAmount struct {
	value *big.Int
	scale int
}

func newScaledAmount() *scaledAmount {
	return &scaledAmount{value: big.NewInt(0)}
}

func (a *scaledAmount) add(value exact.Coefficient, scale int) {
	a.align(scale)
	addend := value.BigInt()
	if scale < a.scale {
		addend.Mul(addend, pow10(a.scale-scale))
	}
	a.value.Add(a.value, addend)
}

func (a *scaledAmount) addScaled(other *scaledAmount) {
	if other == nil {
		return
	}
	a.align(other.scale)
	addend := new(big.Int).Set(other.value)
	if other.scale < a.scale {
		addend.Mul(addend, pow10(a.scale-other.scale))
	}
	a.value.Add(a.value, addend)
}

func (a *scaledAmount) align(scale int) {
	if scale < 0 {
		panic(fmt.Sprintf("scaledAmount.align: negative scale %d", scale))
	}
	if scale <= a.scale {
		return
	}
	a.value.Mul(a.value, pow10(scale-a.scale))
	a.scale = scale
}

func (a *scaledAmount) coefficient() (exact.Coefficient, error) {
	return exact.FromBig(a.value)
}
