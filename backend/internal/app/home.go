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

// Home briefing (#476).
//
// Home's headline is the Net worth report's figure, not a second calculation:
// the totals come from the same series and convert through the same rate
// table, at the same dates, as `/reports/net-worth` with the book's default
// currency. A headline is complete-only. A missing price or rate leaves it
// unavailable and says what is missing; it is never a partial figure.

// Headline identities. Cash needs no price, so it is the fallback while net
// worth is unavailable.
const (
	HomeHeadlineNetWorth = "net_worth"
	HomeHeadlineCash     = "cash"
)

// Why a headline has no value.
const (
	HomeUnavailableValuationGaps       = "valuation_gaps"
	HomeUnavailableUnclassifiedShort   = "unclassified_short"
	HomeUnavailableNoReportingCurrency = "no_reporting_currency"
	HomeUnavailableNoAccounts          = "no_accounts"
)

// homeCashAccountKinds is Home's cash scope: money accounts that need no
// price. Brokerage cash belongs to its brokerage, which #448 shows as one
// account, so it stays in net worth only.
var homeCashAccountKinds = map[string]bool{
	"cash":                true,
	"checking":            true,
	"savings":             true,
	"other_money_account": true,
}

// homeHoldingAccountKinds are per-instrument accounts under a brokerage. They
// stay in the ledger and in net worth, but an owner does not count them as
// accounts of their own (#442).
var homeHoldingAccountKinds = map[string]bool{
	"security_holding": true,
	"fund_holding":     true,
}

// HomeValuationInput is one value a headline needs and lacks or holds only
// from an earlier day. Kind is "price" for anything that is not a currency and
// "rate" for a currency's exchange rate.
type HomeValuationInput struct {
	CommodityID int64
	Code        string
	Kind        string
}

type HomeHeadline struct {
	ID string
	// AccountCount is the headline's scope as the owner counts accounts: open
	// asset and liability accounts, without system or holding accounts.
	AccountCount int
	// Value is at the briefing's AsOf; StartValue at StartBoundary, the end of
	// the previous month. Change is Value - StartValue, present only when both
	// are. All three are in the reporting currency.
	Value       *BalanceQuantity
	StartValue  *BalanceQuantity
	Change      *BalanceQuantity
	Unavailable string
	// Missing are the inputs Value lacks on AsOf. Stale are those it valued
	// from an observation the pricing policy calls out of date: older than its
	// staleness days, as Prices and Review judge it, yet inside the report's
	// window. Each leg of a chained conversion is judged and reported on its
	// own. Each commodity appears once, sorted by kind then code.
	Missing []HomeValuationInput
	Stale   []HomeValuationInput
}

type HomeBriefing struct {
	AsOf          string
	StartBoundary string
	// TrendStart is the end of the month twelve months ago; NetWorthTrend is
	// net worth at each month end from there and today, from the same fold as
	// the headlines, so its last two points are the headline's start and value.
	// Empty when net worth is not valued (no accounts, no default currency).
	TrendStart    string
	NetWorthTrend []HomeTrendPoint
	// Accounts are the owner's accounts with their posted balances today, from
	// the same fold, as the account balances read returns them (no system
	// accounts). Empty for an empty book.
	Accounts []AccountBalance
	// ReportingCurrency is the book's default currency; nil when none is set.
	ReportingCurrency *db.ReportingCurrencyRecord
	// EmptyBook is true when the book has no asset or liability account an
	// owner created, open or closed.
	EmptyBook bool
	Headlines []HomeHeadline
}

// HomeBriefing answers Home's two briefing headlines for owner-local today.
func (s *TransactionService) HomeBriefing(ctx context.Context, ownerUserID int64) (HomeBriefing, error) {
	ctx, referenceErr := s.settings.referenceContext(ctx, s.now())
	if referenceErr != nil {
		return HomeBriefing{}, referenceErr
	}
	asOf, err := s.settings.OwnerTodayAt(ctx, ownerUserID, s.now())
	if err != nil {
		return HomeBriefing{}, err
	}
	startBoundary, err := previousMonthEnd(asOf)
	if err != nil {
		return HomeBriefing{}, err
	}
	trendStart, err := homeTrendStart(asOf)
	if err != nil {
		return HomeBriefing{}, err
	}
	accounts, err := s.repository.LedgerAccountsAsOf(ctx, BookID, asOf)
	if err != nil {
		return HomeBriefing{}, fmt.Errorf("read home accounts: %w", err)
	}

	briefing := HomeBriefing{AsOf: asOf, StartBoundary: startBoundary, TrendStart: trendStart, NetWorthTrend: []HomeTrendPoint{}, Accounts: []AccountBalance{}, EmptyBook: true}
	netWorthCount := 0
	cashAccountIDs := make([]int64, 0)
	for _, account := range accounts {
		if account.SystemRole.Valid || !countsNetWorthAccount(account) {
			continue
		}
		briefing.EmptyBook = false
		if account.Status != "active" || homeHoldingAccountKinds[account.AccountKind] {
			continue
		}
		netWorthCount++
		if homeCashAccountKinds[account.AccountKind] {
			cashAccountIDs = append(cashAccountIDs, account.ID)
		}
	}
	sort.Slice(cashAccountIDs, func(i, j int) bool { return cashAccountIDs[i] < cashAccountIDs[j] })

	if s.pricingRepository != nil {
		currency, err := s.pricingRepository.DefaultCurrency(ctx, BookID)
		switch {
		case errors.Is(err, db.ErrNotFound):
		case err != nil:
			return HomeBriefing{}, err
		default:
			reporting, err := s.pricingRepository.ReportingCurrency(ctx, BookID, currency.ID)
			if err != nil {
				return HomeBriefing{}, err
			}
			briefing.ReportingCurrency = &reporting
		}
	}

	// The count is what the owner has open; whether to value is whether the
	// scope holds anything. Closed balances stay in net worth, so a book whose
	// accounts are all closed still has one. Cash is open money accounts only.
	scopes := []struct {
		id         string
		count      int
		valued     bool
		accountIDs []int64
	}{
		{HomeHeadlineNetWorth, netWorthCount, !briefing.EmptyBook, nil},
		{HomeHeadlineCash, len(cashAccountIDs), len(cashAccountIDs) > 0, cashAccountIDs},
	}
	valued := make([]*HomeHeadline, 0, len(scopes))
	filters := make([]ReportFilters, 0, len(scopes))
	briefing.Headlines = make([]HomeHeadline, len(scopes))
	for index, scope := range scopes {
		headline := &briefing.Headlines[index]
		*headline = HomeHeadline{ID: scope.id, AccountCount: scope.count, Missing: []HomeValuationInput{}, Stale: []HomeValuationInput{}}
		switch {
		case !scope.valued:
			headline.Unavailable = HomeUnavailableNoAccounts
		case briefing.ReportingCurrency == nil:
			headline.Unavailable = HomeUnavailableNoReportingCurrency
		default:
			valued = append(valued, headline)
			filters = append(filters, ReportFilters{AccountIDs: scope.accountIDs})
		}
	}
	if briefing.EmptyBook {
		return briefing, nil
	}
	fold, err := s.foldHome(ctx, filters, trendStart, startBoundary, asOf)
	if err != nil {
		return HomeBriefing{}, err
	}
	briefing.Accounts, err = accountBalancesFrom(accounts, fold.running, false)
	if err != nil {
		return HomeBriefing{}, err
	}
	if len(valued) > 0 {
		trend, err := s.valueHomeHeadlines(ctx, valued, fold, startBoundary, asOf, briefing.ReportingCurrency.ID)
		if err != nil {
			return HomeBriefing{}, err
		}
		if valued[0].ID == HomeHeadlineNetWorth {
			briefing.NetWorthTrend = trend
		}
	}
	return briefing, nil
}

// homeFold is Home's one pass over the ledger: monthly from the trend's start
// to today, totalled per valued headline scope, and the running per-account
// balances it ends with.
type homeFold struct {
	bounds  []calendarBucketBound
	scoped  [][]netWorthBucket
	running map[int64]map[int64]*exact.ScaledInt
}

func (s *TransactionService) foldHome(ctx context.Context, filters []ReportFilters, trendStart, startBoundary, asOf string) (homeFold, error) {
	scopes := make([]ReportFilters, len(filters))
	for index, filter := range filters {
		resolved, _, err := s.resolveReportFilters(ctx, filter, asOf)
		if err != nil {
			return homeFold{}, err
		}
		scopes[index] = echoedReportFilters(resolved)
	}
	bounds, err := calendarBucketBounds(trendStart, asOf, "month")
	if err != nil {
		return homeFold{}, err
	}
	last := len(bounds) - 1
	if last < 1 || bounds[0].endDate != trendStart || bounds[last-1].endDate != startBoundary || bounds[last].endDate != asOf {
		return homeFold{}, fmt.Errorf("home series has unexpected buckets")
	}
	fold := homeFold{bounds: bounds}
	err = s.repository.WithLedgerSnapshot(ctx, func(ledger *db.LedgerSnapshotReader) error {
		var readErr error
		fold.scoped, fold.running, readErr = netWorthScopedSeriesTotals(ctx, ledger, bounds, scopes)
		return readErr
	})
	return fold, err
}

// valueHomeHeadlines values what the Net worth report's series computes for
// each headline's accounts from Home's one fold, and converts each boundary
// through its own coverage, so a gap at the start of the month is not reported
// as a gap today. The fold runs monthly from the trend's start, so the first
// headline's twelve-month trend (#478) comes with it, which it returns.
func (s *TransactionService) valueHomeHeadlines(ctx context.Context, headlines []*HomeHeadline, fold homeFold, startBoundary, asOf string, reportingID int64) ([]HomeTrendPoint, error) {
	bounds, scoped := fold.bounds, fold.scoped
	last := len(bounds) - 1
	seen := make([]int64, 0)
	for _, buckets := range scoped {
		for _, bucket := range buckets {
			for _, total := range bucket.totals {
				seen = append(seen, total.CommodityID)
			}
		}
	}
	rates, err := s.NewRateTable(ctx, ReportingCurrencyInput{CommodityID: reportingID}, seen, bounds[0].endDate, asOf)
	if err != nil {
		return nil, err
	}
	staleness, err := s.pricingStalenessDays(ctx)
	if err != nil {
		return nil, err
	}
	for index, headline := range headlines {
		if err := s.valueHomeHeadline(ctx, headline, rates, scoped[index][last-1], scoped[index][last], startBoundary, asOf, staleness); err != nil {
			return nil, err
		}
	}
	// Each point as the report's series converts it: complete-only, and none
	// while a short position is unclassified.
	trend := make([]HomeTrendPoint, 0, len(bounds))
	for index, bound := range bounds {
		point := HomeTrendPoint{Date: bound.endDate}
		if bucket := scoped[0][index]; len(bucket.unclassifiedShorts) == 0 {
			point.Value = homeConverted(rates.withFreshCoverage(), bucket.totals, bound.endDate)
		}
		trend = append(trend, point)
	}
	return trend, nil
}

func (s *TransactionService) valueHomeHeadline(ctx context.Context, headline *HomeHeadline, rates *RateTable, start, today netWorthBucket, startBoundary, asOf string, staleness int) error {
	if len(today.unclassifiedShorts) > 0 {
		headline.Unavailable = HomeUnavailableUnclassifiedShort
		return nil
	}
	todayRates := rates.withFreshCoverage()
	headline.Value = homeConverted(todayRates, today.totals, asOf)
	if err := s.classifyHomeCoverage(ctx, headline, todayRates.Coverage(""), asOf, staleness); err != nil {
		return err
	}
	if headline.Value == nil {
		headline.Unavailable = HomeUnavailableValuationGaps
		return nil
	}
	if len(start.unclassifiedShorts) == 0 {
		headline.StartValue = homeConverted(rates.withFreshCoverage(), start.totals, startBoundary)
	}
	if headline.StartValue != nil {
		change := new(big.Int).Sub(headline.Value.QuantityValue.BigInt(), headline.StartValue.QuantityValue.BigInt())
		coefficient, err := exact.FromBig(change)
		if err != nil {
			return fmt.Errorf("home change: %w", err)
		}
		headline.Change = &BalanceQuantity{CommodityID: rates.QuoteCommodityID(), QuantityValue: coefficient, QuantityScale: headline.Value.QuantityScale, NormalQuantityValue: coefficient}
	}
	return nil
}

// pricingStalenessDays is the pricing policy's out-of-date threshold, the one
// Prices and the Review attention source apply.
func (s *TransactionService) pricingStalenessDays(ctx context.Context) (int, error) {
	policy, err := s.pricingRepository.GetPricingPolicy(ctx, BookID)
	switch {
	case errors.Is(err, db.ErrNotFound):
		return DefaultPricingStalenessDays, nil
	case err != nil:
		return 0, fmt.Errorf("read pricing policy: %w", err)
	}
	return policy.StalenessMaxDays, nil
}

// homeConverted is the complete-only converted total, or nil.
func homeConverted(rates *RateTable, totals []BalanceQuantity, onDate string) *BalanceQuantity {
	converted := NewConvertedTotal(rates.Scale())
	for _, total := range totals {
		value, ok := rates.Convert(total.QuantityValue, total.QuantityScale, total.CommodityID, onDate)
		converted.Add(value, ok)
	}
	value, scale, ok := converted.Value()
	if !ok {
		return nil
	}
	return &BalanceQuantity{CommodityID: rates.QuoteCommodityID(), QuantityValue: value, QuantityScale: scale, NormalQuantityValue: value}
}

// classifyHomeCoverage turns one date's coverage into inputs by kind. A
// chained conversion has two observations, the instrument's price in a quote
// currency and that currency's rate, so a gap or an out-of-date observation is
// reported against the leg it belongs to, never the instrument for the rate.
func (s *TransactionService) classifyHomeCoverage(ctx context.Context, headline *HomeHeadline, coverage ValuationCoverage, asOf string, staleness int) error {
	missing := map[int64]bool{}
	for _, gap := range coverage.Gaps {
		id := gap.CommodityID
		if gap.ViaCommodityID != 0 {
			id = gap.ViaCommodityID
		}
		missing[id] = true
	}
	current, err := shiftISODate(asOf, -staleness)
	if err != nil {
		return err
	}
	stale := map[int64]bool{}
	for _, use := range coverage.Used {
		if use.ObservationDate < current && !missing[use.CommodityID] {
			stale[use.CommodityID] = true
		}
		if use.ViaCommodityID != 0 && use.ViaObservationDate < current && !missing[use.ViaCommodityID] {
			stale[use.ViaCommodityID] = true
		}
	}
	ids := make([]int64, 0, len(missing)+len(stale))
	for id := range missing {
		ids = append(ids, id)
	}
	for id := range stale {
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		return nil
	}
	codes, err := s.pricingRepository.CommodityCodes(ctx, BookID, ids)
	if err != nil {
		return err
	}
	kinds, err := s.repository.LedgerCommodityKinds(ctx, BookID)
	if err != nil {
		return err
	}
	input := func(id int64) HomeValuationInput {
		kind := "price"
		if kinds[id] == "currency" {
			kind = "rate"
		}
		return HomeValuationInput{CommodityID: id, Code: codes[id], Kind: kind}
	}
	for id := range missing {
		headline.Missing = append(headline.Missing, input(id))
	}
	for id := range stale {
		headline.Stale = append(headline.Stale, input(id))
	}
	sortHomeInputs(headline.Missing)
	sortHomeInputs(headline.Stale)
	return nil
}

func sortHomeInputs(inputs []HomeValuationInput) {
	sort.Slice(inputs, func(i, j int) bool {
		if inputs[i].Kind != inputs[j].Kind {
			return inputs[i].Kind < inputs[j].Kind
		}
		if inputs[i].Code != inputs[j].Code {
			return inputs[i].Code < inputs[j].Code
		}
		return inputs[i].CommodityID < inputs[j].CommodityID
	})
}

// previousMonthEnd is the last day of the month before date's month: the
// "since the start of the month" boundary, measured at the end of that day.
func previousMonthEnd(date string) (string, error) {
	parsed, err := time.Parse(time.DateOnly, date)
	if err != nil {
		return "", fmt.Errorf("parse home date %q: %w", date, err)
	}
	return time.Date(parsed.Year(), parsed.Month(), 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, -1).Format(time.DateOnly), nil
}
