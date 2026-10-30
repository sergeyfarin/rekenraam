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
	// Missing are the inputs Value lacks on AsOf; Stale those it valued from
	// an earlier observation inside the staleness window. Each commodity
	// appears once, sorted by kind then code.
	Missing []HomeValuationInput
	Stale   []HomeValuationInput
}

type HomeBriefing struct {
	AsOf          string
	StartBoundary string
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
	accounts, err := s.repository.LedgerAccountsAsOf(ctx, BookID, asOf)
	if err != nil {
		return HomeBriefing{}, fmt.Errorf("read home accounts: %w", err)
	}

	briefing := HomeBriefing{AsOf: asOf, StartBoundary: startBoundary, EmptyBook: true}
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

	scopes := []struct {
		id         string
		count      int
		accountIDs []int64
	}{
		{HomeHeadlineNetWorth, netWorthCount, nil},
		{HomeHeadlineCash, len(cashAccountIDs), cashAccountIDs},
	}
	for _, scope := range scopes {
		headline := HomeHeadline{ID: scope.id, AccountCount: scope.count, Missing: []HomeValuationInput{}, Stale: []HomeValuationInput{}}
		switch {
		case scope.count == 0:
			headline.Unavailable = HomeUnavailableNoAccounts
		case briefing.ReportingCurrency == nil:
			headline.Unavailable = HomeUnavailableNoReportingCurrency
		default:
			if err := s.valueHomeHeadline(ctx, &headline, scope.accountIDs, startBoundary, asOf, briefing.ReportingCurrency.ID); err != nil {
				return HomeBriefing{}, err
			}
		}
		briefing.Headlines = append(briefing.Headlines, headline)
	}
	return briefing, nil
}

// valueHomeHeadline reads both boundaries through the Net worth report's
// series and converts each through its own coverage, so a gap at the start of
// the month is not reported as a gap today.
func (s *TransactionService) valueHomeHeadline(ctx context.Context, headline *HomeHeadline, accountIDs []int64, startBoundary, asOf string, reportingID int64) error {
	series, err := s.NetWorthSeries(ctx, NetWorthSeriesInput{
		StartDate: startBoundary,
		EndDate:   asOf,
		Bucket:    "month",
		Filters:   ReportFilters{AccountIDs: accountIDs},
	})
	if err != nil {
		return err
	}
	if len(series.Buckets) != 2 || series.Buckets[0].EndDate != startBoundary || series.Buckets[1].EndDate != asOf {
		return fmt.Errorf("home series has unexpected buckets")
	}
	start, today := series.Buckets[0], series.Buckets[1]
	if len(today.UnclassifiedShorts) > 0 {
		headline.Unavailable = HomeUnavailableUnclassifiedShort
		return nil
	}

	seen := make([]int64, 0)
	for _, bucket := range series.Buckets {
		for _, total := range bucket.Totals {
			seen = append(seen, total.CommodityID)
		}
	}
	rates, err := s.NewRateTable(ctx, ReportingCurrencyInput{CommodityID: reportingID}, seen, startBoundary, asOf)
	if err != nil {
		return err
	}

	todayRates := rates.withFreshCoverage()
	headline.Value = homeConverted(todayRates, today.Totals, asOf)
	coverage := todayRates.Coverage("")
	if err := s.classifyHomeCoverage(ctx, headline, coverage); err != nil {
		return err
	}
	if headline.Value == nil {
		headline.Unavailable = HomeUnavailableValuationGaps
		return nil
	}
	if len(start.UnclassifiedShorts) == 0 {
		headline.StartValue = homeConverted(rates.withFreshCoverage(), start.Totals, startBoundary)
	}
	if headline.StartValue != nil {
		change := new(big.Int).Sub(headline.Value.QuantityValue.BigInt(), headline.StartValue.QuantityValue.BigInt())
		coefficient, err := exact.FromBig(change)
		if err != nil {
			return fmt.Errorf("home change: %w", err)
		}
		headline.Change = &BalanceQuantity{CommodityID: reportingID, QuantityValue: coefficient, QuantityScale: headline.Value.QuantityScale, NormalQuantityValue: coefficient}
	}
	return nil
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

// classifyHomeCoverage turns one date's coverage into inputs by kind. A gap on
// a chained conversion is the quote currency's missing rate, not the
// instrument's price, so it is reported against that currency.
func (s *TransactionService) classifyHomeCoverage(ctx context.Context, headline *HomeHeadline, coverage ValuationCoverage) error {
	missing := map[int64]bool{}
	for _, gap := range coverage.Gaps {
		id := gap.CommodityID
		if gap.ViaCommodityID != 0 {
			id = gap.ViaCommodityID
		}
		missing[id] = true
	}
	stale := map[int64]bool{}
	for _, use := range coverage.Used {
		if use.Stale && !missing[use.CommodityID] {
			stale[use.CommodityID] = true
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
