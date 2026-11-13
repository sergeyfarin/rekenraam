package app

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"

	"rekenraam/backend/internal/db"
	"rekenraam/backend/internal/exact"
)

// LedgerOverview is the account index and the brokerage summary (#448).
// Financial inputs are read once, from the same snapshot, including unsettled
// investment cash. Valuation uses the Net worth report's rate table and rules.
// Holdings remain financial accounts, but never become account-index rows.
type LedgerOverview struct {
	AsOf         string
	Accounts     []LedgerAccount
	Institutions []Institution
	Commodities  []db.ReportingCurrencyRecord
	NextCursor   string
}
type LedgerAccount struct {
	CashAccounts      []Account
	Account           Account
	Balances          []BalanceQuantity
	Cash              []BalanceQuantity
	Unsettled         []BalanceQuantity
	Positions         []LedgerPosition
	Value             *BalanceQuantity
	Valuation         *ValuationCoverage
	UnclassifiedShort bool
	Sources           []db.LedgerImportSource
}
type LedgerPosition struct {
	HoldingAccountID int64
	CommodityID      int64
	Quantity         BalanceQuantity
	Value            *BalanceQuantity
}
type LedgerOverviewInput struct {
	OwnerUserID int64
	AsOf        string
	Cursor      string
	AccountID   int64
}
type LedgerService struct {
	transactions *TransactionService
	accounts     *AccountService
	institutions *InstitutionService
}

func NewLedgerService(transactions *TransactionService, accounts *AccountService, institutions *InstitutionService) *LedgerService {
	return &LedgerService{transactions: transactions, accounts: accounts, institutions: institutions}
}
func (s *LedgerService) Overview(ctx context.Context, input LedgerOverviewInput) (LedgerOverview, error) {
	result := LedgerOverview{Accounts: []LedgerAccount{}, Institutions: []Institution{}, Commodities: []db.ReportingCurrencyRecord{}}
	afterID := int64(0)
	if input.Cursor != "" {
		var err error
		afterID, err = strconv.ParseInt(input.Cursor, 10, 64)
		if err != nil || afterID <= 0 {
			return result, ValidationError{Message: "cursor is invalid"}
		}
	}
	asOf, _, err := s.transactions.cleanLedgerAsOfAndStatus(ctx, input.OwnerUserID, input.AsOf, "posted")
	if err != nil {
		return result, err
	}
	result.AsOf = asOf
	// Follow the existing metadata catalog's cursor; never truncate a large book.
	metadata := []Account{}
	cursor := ""
	for {
		page, err := s.accounts.ListAccounts(ctx, ListAccountsInput{IncludeArchived: true, Cursor: cursor})
		if err != nil {
			return result, err
		}
		metadata = append(metadata, page.Accounts...)
		if page.NextCursor == "" {
			break
		}
		cursor = page.NextCursor
	}
	result.Institutions, err = s.institutions.ListInstitutions(ctx, ListInstitutionsInput{IncludeArchived: true})
	if err != nil {
		return result, err
	}
	var accounts []db.LedgerAccountRecord
	var kinds map[int64]string
	running := map[int64]map[int64]*exact.ScaledInt{}
	var unsettled unsettledTrades
	shorts := namedShortQuantities{}
	err = s.transactions.repository.WithLedgerSnapshot(ctx, func(ledger *db.LedgerSnapshotReader) error {
		var err error
		accounts, err = ledger.LedgerAccountsAsOf(ctx, BookID, asOf)
		if err != nil {
			return err
		}
		postings, err := ledger.LedgerPostingsThrough(ctx, BookID, asOf, "posted")
		if err != nil {
			return err
		}
		for _, posting := range postings {
			foldPosting(running, posting)
		}
		kinds, err = ledger.LedgerCommodityKinds(ctx, BookID)
		if err != nil {
			return err
		}
		events, err := ledger.ShortLotQuantityEventsThrough(ctx, BookID, asOf)
		if err != nil {
			return err
		}
		for _, event := range events {
			shorts.fold(event)
		}
		unsettled, err = readUnsettledTrades(ctx, ledger, kinds)
		return err
	})
	if err != nil {
		return result, fmt.Errorf("read ledger overview: %w", err)
	}
	sources, err := s.accounts.repository.LedgerImportSources(ctx, BookID)
	if err != nil {
		return result, err
	}
	pending := unsettled.at(asOf)
	// All visible rows use direct balances plus their hidden holding descendants.
	// Ordinary children outside brokerages retain their own rows.
	parent := map[int64]int64{}
	holding := map[int64]bool{}
	brokerage := map[int64]bool{}
	for _, a := range accounts {
		parent[a.ID] = a.ParentAccountID.Int64
		holding[a.ID] = homeHoldingAccountKinds[a.AccountKind]
		brokerage[a.ID] = a.AccountKind == "brokerage"
	}
	// Cash children belong to their brokerage too. A brokerage is one index
	// row, while the explicit cash account remains a legal entry/reconcile target.
	ownerFor := func(id int64) int64 {
		fallback := id
		seen := map[int64]bool{}
		for p := id; p > 0 && !seen[p]; p = parent[p] {
			seen[p] = true
			if brokerage[p] {
				return p
			}
			if holding[fallback] && !holding[p] {
				fallback = p
			}
		}
		return fallback
	}
	metadataByID := map[int64]Account{}
	for _, account := range metadata {
		metadataByID[account.ID] = account
	}
	ids := []int64{}
	for _, amounts := range running {
		for id := range amounts {
			ids = append(ids, id)
		}
	}
	for _, amounts := range pending {
		for id := range amounts {
			ids = append(ids, id)
		}
	}
	var rates *RateTable
	if s.transactions.pricingRepository != nil {
		currency, currencyErr := s.transactions.pricingRepository.DefaultCurrency(ctx, BookID)
		if currencyErr != nil && !errors.Is(currencyErr, db.ErrNotFound) {
			return result, currencyErr
		}
		if currencyErr == nil {
			ids = append(ids, currency.ID)
			rates, err = s.transactions.NewRateTable(ctx, ReportingCurrencyInput{CommodityID: currency.ID}, dedupeIDs(ids), asOf, asOf)
			if err != nil {
				return result, err
			}
		}
		commodities, err := s.transactions.pricingRepository.ReportingCurrencies(ctx, BookID, dedupeIDs(ids))
		if err != nil {
			return result, err
		}
		for _, c := range commodities {
			result.Commodities = append(result.Commodities, c)
		}
		slices.SortFunc(result.Commodities, func(a, b db.ReportingCurrencyRecord) int { return compareLedgerID(a.ID, b.ID) })
	}
	slices.SortFunc(metadata, func(a, b Account) int { return compareLedgerID(a.ID, b.ID) })
	for _, a := range metadata {
		if a.IsSystem || (input.AccountID == 0 && homeHoldingAccountKinds[a.AccountKind]) || (a.AccountClass != "asset" && a.AccountClass != "liability") {
			continue
		}
		if input.AccountID > 0 && a.ID != input.AccountID {
			continue
		}
		if input.AccountID == 0 && (a.ID <= afterID || ownerFor(a.ID) != a.ID) {
			continue
		}
		if len(result.Accounts) == 100 && input.AccountID == 0 {
			result.NextCursor = strconv.FormatInt(result.Accounts[len(result.Accounts)-1].Account.ID, 10)
			break
		}
		scope := []int64{a.ID}
		for id := range parent {
			if id != a.ID && ownerFor(id) == a.ID {
				scope = append(scope, id)
			}
		}
		slices.Sort(scope)

		bucket, err := netWorthBucketTotals(accounts, running, pending, unsettled.accounts, kinds, ReportFilters{AccountIDs: scope}, shorts)
		if err != nil {
			return result, err
		}
		row := LedgerAccount{Account: a, Balances: bucket.totals, Cash: []BalanceQuantity{}, Unsettled: []BalanceQuantity{}, Positions: []LedgerPosition{}, UnclassifiedShort: len(bucket.unclassifiedShorts) > 0}
		slices.SortFunc(row.Balances, func(a, b BalanceQuantity) int { return compareLedgerID(a.CommodityID, b.CommodityID) })
		for _, amount := range bucket.unsettled {
			row.Unsettled = append(row.Unsettled, amount.Amount)
		}
		for _, quantity := range bucket.totals {
			if kinds[quantity.CommodityID] == "currency" {
				row.Cash = append(row.Cash, quantity)
			}
		}
		for _, id := range scope {
			for commodityID, quantity := range running[id] {
				if kinds[commodityID] == "currency" || quantity.Sign() == 0 {
					continue
				}
				value, err := balanceMapToQuantities(map[int64]*exact.ScaledInt{commodityID: quantity}, "")
				if err != nil {
					return result, err
				}
				position := LedgerPosition{HoldingAccountID: id, CommodityID: commodityID, Quantity: value[0]}
				if rates != nil && !shorts.unexplainedNegative(id, commodityID, quantity) {
					position.Value = homeConverted(rates.withFreshCoverage(), value, asOf)
				}
				row.Positions = append(row.Positions, position)
			}
		}
		slices.SortFunc(row.Positions, func(a, b LedgerPosition) int {
			if c := compareLedgerID(a.CommodityID, b.CommodityID); c != 0 {
				return c
			}
			return compareLedgerID(a.HoldingAccountID, b.HoldingAccountID)
		})
		if rates != nil {
			local := rates.withFreshCoverage()
			value := homeConverted(local, bucket.totals, asOf)
			if !row.UnclassifiedShort {
				row.Value = value
			}
			coverage := local.Coverage("")
			row.Valuation = &coverage
		}
		row.CashAccounts = []Account{}
		for _, id := range scope {
			if id != a.ID && !holding[id] {
				if account, ok := metadataByID[id]; ok {
					row.CashAccounts = append(row.CashAccounts, account)
				}
			}
		}
		row.Sources = []db.LedgerImportSource{}
		for _, source := range sources {
			if slices.Contains(scope, source.AccountID) {
				row.Sources = append(row.Sources, source)
			}
		}
		result.Accounts = append(result.Accounts, row)
	}
	if input.AccountID > 0 && len(result.Accounts) == 0 {
		return result, ErrAccountNotFound
	}
	return result, nil
}
func compareLedgerID(a, b int64) int {
	if a < b {
		return -1
	}
	if a > b {
		return 1
	}
	return 0
}
