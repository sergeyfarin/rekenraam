package app

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
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
	OwnerUserID   int64
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
	OwnerUserID  int64
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
	OwnerUserID int64
	AsOf        string
	Status      string
}

type NetWorthResult struct {
	AsOf   string
	Status string
	// Totals are trade-date: they include Unsettled.
	Totals              []BalanceQuantity
	Unsettled           []UnsettledAmount
	UnclassifiedShorts  []UnclassifiedShortPosition
	ExcludedSystemRoles []string
}

// UnsettledAmount is what an investment operation has recognised on its unit
// date but not yet posted to an account, or posted before it (#410). A trade
// posts its units on the trade date and its cash on the settlement date; in
// between, net worth counts the cash not yet paid or received here, on the
// account that settles it. Account balances stay posting-based: net-worth
// totals are the posted balances plus these amounts.
type UnsettledAmount struct {
	AccountID int64
	Amount    BalanceQuantity
}

// UnclassifiedShortPosition is a negative countable position without an
// explicit short-sale contract. Ordinary ledger entries may create one while
// an out-of-order import is being completed.
type UnclassifiedShortPosition struct {
	AccountID   int64
	CommodityID int64
}

type NetWorthSeriesInput struct {
	StartDate string
	EndDate   string
	Bucket    string
	Filters   ReportFilters
	// Reporting is optional. Net worth is a stock — a quantity measured *at* a
	// date — so each bucket converts at its own end date rather than at one
	// rate for the whole series.
	Reporting *ReportingCurrencyInput
}

type NetWorthSeriesBucket struct {
	StartDate          string
	EndDate            string
	Totals             []BalanceQuantity
	Unsettled          []UnsettledAmount
	UnclassifiedShorts []UnclassifiedShortPosition
	// Converted is this bucket's net worth in the reporting currency, at the
	// bucket's end date. Absent when a commodity cannot be converted or an
	// unclassified short makes the bucket unreliable.
	Converted *BalanceQuantity
}

type NetWorthSeriesResult struct {
	StartDate           string
	EndDate             string
	Bucket              string
	Filters             ReportFiltersEcho
	Buckets             []NetWorthSeriesBucket
	ExcludedSystemRoles []string
	Valuation           *ValuationCoverage
}

func (s *TransactionService) AccountBalances(ctx context.Context, input AccountBalancesInput) (AccountBalancesResult, error) {
	ctx, referenceErr := s.settings.referenceContext(ctx, s.now())
	if referenceErr != nil {
		return AccountBalancesResult{}, referenceErr
	}

	asOf, status, err := s.cleanLedgerAsOfAndStatus(ctx, input.OwnerUserID, input.AsOf, input.Status)
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

	resultAccounts, err := accountBalancesFrom(accounts, aggregatePostings(postings), input.IncludeSystem)
	if err != nil {
		return AccountBalancesResult{}, err
	}

	return AccountBalancesResult{
		AsOf:     asOf,
		Status:   status,
		Accounts: resultAccounts,
	}, nil
}

// accountBalancesFrom turns per-account posted balances into each account's
// direct and subtree balances. Home's briefing fold hands it the same running
// balances (#478), so the two reads cannot disagree.
func accountBalancesFrom(accounts []db.LedgerAccountRecord, direct map[int64]map[int64]*exact.ScaledInt, includeSystem bool) ([]AccountBalance, error) {
	accountMap := ledgerAccountMap(accounts)
	if !includeSystem {
		direct = excludeSystemAccountBalances(accountMap, direct)
	}
	subtree := rollupBalances(accountMap, direct)
	result := make([]AccountBalance, 0, len(accounts))
	for _, account := range accounts {
		if account.SystemRole.Valid && !includeSystem {
			continue
		}
		accountBalance, err := toAccountBalance(account, direct[account.ID], subtree[account.ID])
		if err != nil {
			return nil, err
		}
		result = append(result, accountBalance)
	}
	return result, nil
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
		beforeDate, err = s.settings.OwnerTodayAt(ctx, input.OwnerUserID, s.now())
		if err != nil {
			return CategoryTotalsResult{}, err
		}
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
	ctx, referenceErr := s.settings.referenceContext(ctx, s.now())
	if referenceErr != nil {
		return NetWorthResult{}, referenceErr
	}

	asOf, status, err := s.cleanLedgerAsOfAndStatus(ctx, input.OwnerUserID, input.AsOf, input.Status)
	if err != nil {
		return NetWorthResult{}, err
	}
	var bucket netWorthBucket
	err = s.repository.WithLedgerSnapshot(ctx, func(ledger *db.LedgerSnapshotReader) error {
		var readErr error
		bucket, readErr = netWorthTotals(ctx, ledger, asOf, status)
		return readErr
	})
	if err != nil {
		return NetWorthResult{}, err
	}

	return NetWorthResult{
		AsOf:                asOf,
		Status:              status,
		Totals:              bucket.totals,
		Unsettled:           bucket.unsettled,
		UnclassifiedShorts:  bucket.unclassifiedShorts,
		ExcludedSystemRoles: []string{"commodity_trading"},
	}, nil
}

// NetWorthSeries returns exact asset/liability totals at each calendar bucket
// end. Reports deliberately include posted records only: drafts, voided
// transactions, and soft-deleted transactions are not part of financial truth.
func (s *TransactionService) NetWorthSeries(ctx context.Context, input NetWorthSeriesInput) (NetWorthSeriesResult, error) {
	ctx, referenceErr := s.settings.referenceContext(ctx, s.now())
	if referenceErr != nil {
		return NetWorthSeriesResult{}, referenceErr
	}

	startDate, endDate, bucket, err := cleanNetWorthSeriesInput(input)
	if err != nil {
		return NetWorthSeriesResult{}, err
	}

	// Filters are validated and normalized once here; descendants are then
	// re-resolved per reporting date inside netWorthTotals, because parent links
	// are versioned. The echo uses the series end date so the response names one
	// stable expansion.
	filters, _, err := s.resolveReportFilters(ctx, input.Filters, endDate)
	if err != nil {
		return NetWorthSeriesResult{}, err
	}
	normalizedFilters := echoedReportFilters(filters)

	bounds, err := calendarBucketBounds(startDate, endDate, bucket)
	if err != nil {
		return NetWorthSeriesResult{}, err
	}
	var bucketTotals []netWorthBucket
	err = s.repository.WithLedgerSnapshot(ctx, func(ledger *db.LedgerSnapshotReader) error {
		var readErr error
		bucketTotals, readErr = netWorthSeriesTotals(ctx, ledger, bounds, normalizedFilters)
		return readErr
	})
	if err != nil {
		return NetWorthSeriesResult{}, err
	}
	// One rate table for the whole series, covering every commodity that shows
	// up in any bucket. Each bucket then converts at its own end date.
	var rates *RateTable
	if input.Reporting != nil {
		seen := make([]int64, 0)
		for _, bucket := range bucketTotals {
			for _, total := range bucket.totals {
				seen = append(seen, total.CommodityID)
			}
		}
		rates, err = s.NewRateTable(ctx, *input.Reporting, seen, startDate, endDate)
		if err != nil {
			return NetWorthSeriesResult{}, err
		}
	}

	resultBuckets := make([]NetWorthSeriesBucket, 0, len(bounds))
	for index, bound := range bounds {
		bucketResult := NetWorthSeriesBucket{
			StartDate:          bound.startDate,
			EndDate:            bound.endDate,
			Totals:             bucketTotals[index].totals,
			Unsettled:          bucketTotals[index].unsettled,
			UnclassifiedShorts: bucketTotals[index].unclassifiedShorts,
		}
		if rates != nil && len(bucketResult.UnclassifiedShorts) == 0 {
			converted := NewConvertedTotal(rates.Scale())
			for _, total := range bucketTotals[index].totals {
				value, ok := rates.Convert(total.QuantityValue, total.QuantityScale, total.CommodityID, bound.endDate)
				converted.Add(value, ok)
			}
			if value, scale, ok := converted.Value(); ok {
				bucketResult.Converted = &BalanceQuantity{
					CommodityID:         rates.QuoteCommodityID(),
					QuantityValue:       value,
					QuantityScale:       scale,
					NormalQuantityValue: value,
				}
			}
		}
		resultBuckets = append(resultBuckets, bucketResult)
	}

	return NetWorthSeriesResult{
		StartDate:           startDate,
		EndDate:             endDate,
		Bucket:              bucket,
		Filters:             filters,
		Buckets:             resultBuckets,
		ExcludedSystemRoles: []string{"commodity_trading"},
		Valuation:           s.valuationCoverage(ctx, rates),
	}, nil
}

// netWorthTotals answers the point-in-time net worth for one date. The series
// endpoint does not use it: reading the whole ledger per reporting date is
// quadratic in the number of buckets, so netWorthSeriesTotals folds one read
// forward across buckets instead.
type netWorthBucket struct {
	totals             []BalanceQuantity
	unsettled          []UnsettledAmount
	unclassifiedShorts []UnclassifiedShortPosition
}

// netWorthLedger is what net worth reads, all from one snapshot: posted
// balances and the unsettled trades beside them must be one ledger state, or
// a commit between the two reads pairs old units with new unsettled cash
// (#410).
type netWorthLedger interface {
	LedgerAccountsAsOf(ctx context.Context, bookID int64, asOf string) ([]db.LedgerAccountRecord, error)
	LedgerAccountVersionsThrough(ctx context.Context, bookID int64, asOf string) ([]db.LedgerAccountVersionRecord, error)
	LedgerPostingsThrough(ctx context.Context, bookID int64, asOf string, status string) ([]db.LedgerPostingRecord, error)
	LedgerCommodityKinds(ctx context.Context, bookID int64) (map[int64]string, error)
	ShortLotQuantityEventsThrough(ctx context.Context, bookID int64, through string) ([]db.ShortLotQuantityEvent, error)
	InvestmentOperationPostings(ctx context.Context, bookID int64) ([]db.LedgerPostingRecord, map[int64]db.InvestmentOperationLink, error)
}

func netWorthTotals(ctx context.Context, ledger netWorthLedger, asOf string, status string) (netWorthBucket, error) {
	accounts, err := ledger.LedgerAccountsAsOf(ctx, BookID, asOf)
	if err != nil {
		return netWorthBucket{}, fmt.Errorf("read net worth accounts: %w", err)
	}
	postings, err := ledger.LedgerPostingsThrough(ctx, BookID, asOf, status)
	if err != nil {
		return netWorthBucket{}, fmt.Errorf("read net worth postings: %w", err)
	}
	kinds, err := ledger.LedgerCommodityKinds(ctx, BookID)
	if err != nil {
		return netWorthBucket{}, err
	}

	running := map[int64]map[int64]*exact.ScaledInt{}
	for _, posting := range postings {
		foldPosting(running, posting)
	}
	shortEvents, err := ledger.ShortLotQuantityEventsThrough(ctx, BookID, asOf)
	if err != nil {
		return netWorthBucket{}, err
	}
	shorts := namedShortQuantities{}
	for _, event := range shortEvents {
		shorts.fold(event)
	}
	unsettled := unsettledTrades{}
	if status == "posted" {
		if unsettled, err = readUnsettledTrades(ctx, ledger, kinds); err != nil {
			return netWorthBucket{}, err
		}
	}
	return netWorthBucketTotals(accounts, running, unsettled.at(asOf), unsettled.accounts, kinds, ReportFilters{}, shorts)
}

// unsettledTrades are the postings investment operations recognise on one
// date but post on another: a trade's cash, its clearing leg and any
// separately paid charge (#410, the returns plan's trade-date recognition),
// and a dividend's or payment in lieu's cash between its ex date and payment
// (dividends receivable).
type unsettledTrades struct {
	postings []unsettledPosting
	// accounts classifies a settling account that has no version yet on a
	// reporting date: its first version, so an obligation recognised before
	// the account opens still counts.
	accounts map[int64]db.LedgerAccountRecord
}

type unsettledPosting struct {
	recognisedOn string
	posting      db.LedgerPostingRecord
}

func readUnsettledTrades(ctx context.Context, ledger netWorthLedger, kinds map[int64]string) (unsettledTrades, error) {
	postings, links, err := ledger.InvestmentOperationPostings(ctx, BookID)
	if err != nil {
		return unsettledTrades{}, err
	}
	recognised := recognitionDates(postings, links, kinds)
	trades := unsettledTrades{accounts: map[int64]db.LedgerAccountRecord{}}
	for _, posting := range postings {
		if recognition, ok := recognised[posting.TransactionID]; ok && posting.EntryDate != recognition.date {
			trades.postings = append(trades.postings, unsettledPosting{recognisedOn: recognition.date, posting: posting})
			trades.accounts[posting.AccountID] = db.LedgerAccountRecord{}
		}
	}
	if len(trades.postings) == 0 {
		return trades, nil
	}
	versions, err := ledger.LedgerAccountVersionsThrough(ctx, BookID, "9999-12-31")
	if err != nil {
		return unsettledTrades{}, fmt.Errorf("read unsettled settlement accounts: %w", err)
	}
	opened := map[int64]bool{}
	for _, version := range versions {
		if _, settles := trades.accounts[version.ID]; settles && !opened[version.ID] {
			trades.accounts[version.ID] = version.LedgerAccountRecord
			opened[version.ID] = true
		}
	}
	return trades, nil
}

// at returns, per account, what is recognised but not yet posted on asOf: a
// posting dated after asOf whose units are recognised by then, less one
// already posted whose units are not.
func (u unsettledTrades) at(asOf string) map[int64]map[int64]*exact.ScaledInt {
	pending := map[int64]map[int64]*exact.ScaledInt{}
	for _, trade := range u.postings {
		posting := trade.posting
		switch {
		case trade.recognisedOn <= asOf && asOf < posting.EntryDate:
		case posting.EntryDate <= asOf && asOf < trade.recognisedOn:
			posting.QuantityValue = posting.QuantityValue.Negated()
		default:
			continue
		}
		foldPosting(pending, posting)
	}
	return pending
}

// namedShortQuantities is the dated quantity each holding owes through named
// short lots (#173). A negative balance up to that quantity is an explained
// short; anything beyond it stays unclassified.
type namedShortQuantities map[int64]map[int64]*exact.ScaledInt

func (q namedShortQuantities) fold(event db.ShortLotQuantityEvent) {
	if q[event.AccountID] == nil {
		q[event.AccountID] = map[int64]*exact.ScaledInt{}
	}
	if q[event.AccountID][event.CommodityID] == nil {
		q[event.AccountID][event.CommodityID] = exact.NewScaledInt()
	}
	q[event.AccountID][event.CommodityID].AddCoefficient(event.QuantityValue, event.QuantityScale)
}

// unexplainedNegative reports whether balance is more negative than the
// holding's named short quantity.
func (q namedShortQuantities) unexplainedNegative(accountID, commodityID int64, balance *exact.ScaledInt) bool {
	if balance.Sign() >= 0 {
		return false
	}
	owed := q[accountID][commodityID]
	if owed == nil {
		return true
	}
	return balance.Cmp(owed.Negated()) < 0
}

// netWorthSeriesTotals computes every bucket's totals from a single pass over
// the ledger, in bucket order.
//
// Each bucket reports a cumulative balance as of its own end date, so the naive
// shape is one full ledger read per bucket and the cost is buckets x postings
// (T-51). Here the postings are read once for the whole series and folded
// forward into a running balance, and each bucket is answered from that balance.
//
// The fold is kept per account rather than collapsed straight into commodity
// totals, because the decisions that turn balances into net worth are all
// effective-dated: which accounts are assets or liabilities, which carry an
// excluded system role, and how an account subtree expands are resolved against
// the snapshot in effect at each bucket's end. Collapsing early would bake one
// bucket's classification into every later one.
func netWorthSeriesTotals(ctx context.Context, ledger netWorthLedger, bounds []calendarBucketBound, filters ReportFilters) ([]netWorthBucket, error) {
	scoped, _, err := netWorthScopedSeriesTotals(ctx, ledger, bounds, []ReportFilters{filters})
	if err != nil || scoped == nil {
		return nil, err
	}
	return scoped[0], nil
}

// netWorthScopedSeriesTotals is netWorthSeriesTotals for several account
// scopes at once: the ledger is read and folded once, and each scope totals
// the same running balances, so Home's two headlines cost one pass (#476).
// It also returns the running per-account posted balances at the last
// bucket's end, which Home's Accounts widget reads instead of a second pass
// over the ledger (#478).
func netWorthScopedSeriesTotals(ctx context.Context, ledger netWorthLedger, bounds []calendarBucketBound, scopes []ReportFilters) ([][]netWorthBucket, map[int64]map[int64]*exact.ScaledInt, error) {
	if len(bounds) == 0 {
		return nil, nil, nil
	}
	seriesEnd := bounds[len(bounds)-1].endDate

	versions, err := ledger.LedgerAccountVersionsThrough(ctx, BookID, seriesEnd)
	if err != nil {
		return nil, nil, fmt.Errorf("read net worth accounts: %w", err)
	}
	// Reports are posted-only: drafts, voided, and soft-deleted transactions are
	// never part of financial truth.
	postings, err := ledger.LedgerPostingsThrough(ctx, BookID, seriesEnd, "posted")
	if err != nil {
		return nil, nil, fmt.Errorf("read net worth postings: %w", err)
	}
	kinds, err := ledger.LedgerCommodityKinds(ctx, BookID)
	if err != nil {
		return nil, nil, err
	}
	shortEvents, err := ledger.ShortLotQuantityEventsThrough(ctx, BookID, seriesEnd)
	if err != nil {
		return nil, nil, err
	}
	shorts := namedShortQuantities{}
	nextShort := 0
	unsettled, err := readUnsettledTrades(ctx, ledger, kinds)
	if err != nil {
		return nil, nil, err
	}

	timeline := newLedgerAccountTimeline(versions)
	running := map[int64]map[int64]*exact.ScaledInt{}
	next := 0

	scopedTotals := make([][]netWorthBucket, len(scopes))
	for index := range scopes {
		scopedTotals[index] = make([]netWorthBucket, 0, len(bounds))
	}
	for _, bound := range bounds {
		// Postings arrive ordered by entry date and bucket ends ascend, so one
		// forward cursor covers the whole series.
		for next < len(postings) && postings[next].EntryDate <= bound.endDate {
			foldPosting(running, postings[next])
			next++
		}
		for nextShort < len(shortEvents) && shortEvents[nextShort].EventDate <= bound.endDate {
			shorts.fold(shortEvents[nextShort])
			nextShort++
		}
		accounts, pending := timeline.snapshotThrough(bound.endDate), unsettled.at(bound.endDate)
		for index, filters := range scopes {
			totals, err := netWorthBucketTotals(accounts, running, pending, unsettled.accounts, kinds, filters, shorts)
			if err != nil {
				return nil, nil, err
			}
			scopedTotals[index] = append(scopedTotals[index], totals)
		}
	}
	return scopedTotals, running, nil
}

// netWorthBucketTotals sums the running per-account balances that count as net
// worth under the account snapshot in effect on the reporting date, plus the
// unsettled amounts those accounts will post.
func netWorthBucketTotals(
	accounts []db.LedgerAccountRecord,
	running map[int64]map[int64]*exact.ScaledInt,
	pending map[int64]map[int64]*exact.ScaledInt,
	settling map[int64]db.LedgerAccountRecord,
	kinds map[int64]string,
	filters ReportFilters,
	namedShorts namedShortQuantities,
) (netWorthBucket, error) {
	filterSet := reportFilterSetFrom(accounts, filters)
	accountMap := ledgerAccountMap(accounts)

	counts := func(accountID int64) bool {
		account, ok := accountMap[accountID]
		return ok && countsNetWorthAccount(account)
	}
	totals := map[int64]*exact.ScaledInt{}
	add := func(commodityID int64, amount *exact.ScaledInt) {
		total := totals[commodityID]
		if total == nil {
			total = exact.NewScaledInt()
			totals[commodityID] = total
		}
		total.AddScaled(amount)
	}
	shorts := []UnclassifiedShortPosition{}
	for accountID, balances := range running {
		if !counts(accountID) {
			continue
		}
		for commodityID, amount := range balances {
			if !filterSet.includes(accountID, commodityID) {
				continue
			}
			if kinds[commodityID] != "currency" && namedShorts.unexplainedNegative(accountID, commodityID, amount) {
				shorts = append(shorts, UnclassifiedShortPosition{AccountID: accountID, CommodityID: commodityID})
			}
			add(commodityID, amount)
		}
	}
	unsettled := []UnsettledAmount{}
	for accountID, amounts := range pending {
		account, ok := accountMap[accountID]
		if !ok {
			account = settling[accountID]
		}
		if !countsNetWorthAccount(account) {
			continue
		}
		included := map[int64]*exact.ScaledInt{}
		for commodityID, amount := range amounts {
			if amount.Sign() == 0 || !filterSet.includes(accountID, commodityID) {
				continue
			}
			included[commodityID] = amount
			add(commodityID, amount)
		}
		quantities, err := balanceMapToQuantities(included, "")
		if err != nil {
			return netWorthBucket{}, err
		}
		for _, quantity := range quantities {
			unsettled = append(unsettled, UnsettledAmount{AccountID: accountID, Amount: quantity})
		}
	}
	sort.Slice(unsettled, func(i, j int) bool {
		if unsettled[i].AccountID != unsettled[j].AccountID {
			return unsettled[i].AccountID < unsettled[j].AccountID
		}
		return unsettled[i].Amount.CommodityID < unsettled[j].Amount.CommodityID
	})
	quantities, err := balanceMapToQuantities(totals, "")
	if err != nil {
		return netWorthBucket{}, err
	}
	sort.Slice(shorts, func(i, j int) bool {
		if shorts[i].AccountID != shorts[j].AccountID {
			return shorts[i].AccountID < shorts[j].AccountID
		}
		return shorts[i].CommodityID < shorts[j].CommodityID
	})
	return netWorthBucket{totals: quantities, unsettled: unsettled, unclassifiedShorts: shorts}, nil
}

// Closure does not remove a balance from net worth or current FX discovery.
func countsNetWorthAccount(account db.LedgerAccountRecord) bool {
	if account.AccountClass != "asset" && account.AccountClass != "liability" {
		return false
	}
	return !account.SystemRole.Valid || account.SystemRole.String != "commodity_trading"
}

// foldPosting accumulates one posting into the running per-account balances.
func foldPosting(running map[int64]map[int64]*exact.ScaledInt, posting db.LedgerPostingRecord) {
	balances := running[posting.AccountID]
	if balances == nil {
		balances = map[int64]*exact.ScaledInt{}
		running[posting.AccountID] = balances
	}
	addPosting(balances, posting)
}

// ledgerAccountTimeline replays effective-dated account versions in ascending
// order, so a walk over ascending reporting dates gets each snapshot from one
// read instead of a LedgerAccountsAsOf per date.
type ledgerAccountTimeline struct {
	versions []db.LedgerAccountVersionRecord
	next     int
	current  map[int64]db.LedgerAccountRecord
}

func newLedgerAccountTimeline(versions []db.LedgerAccountVersionRecord) *ledgerAccountTimeline {
	return &ledgerAccountTimeline{versions: versions, current: map[int64]db.LedgerAccountRecord{}}
}

// snapshotThrough returns the account versions in effect on asOf. Calls must
// pass non-decreasing dates: the cursor only moves forward.
func (t *ledgerAccountTimeline) snapshotThrough(asOf string) []db.LedgerAccountRecord {
	for t.next < len(t.versions) && t.versions[t.next].EffectiveFrom <= asOf {
		version := t.versions[t.next]
		t.current[version.ID] = version.LedgerAccountRecord
		t.next++
	}
	accounts := make([]db.LedgerAccountRecord, 0, len(t.current))
	for _, account := range t.current {
		accounts = append(accounts, account)
	}
	return accounts
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

func (s *TransactionService) cleanLedgerAsOfAndStatus(ctx context.Context, ownerUserID int64, asOfInput string, statusInput string) (string, string, error) {
	asOf := strings.TrimSpace(asOfInput)
	if asOf == "" {
		today, err := s.settings.OwnerTodayAt(ctx, ownerUserID, s.now())
		if err != nil {
			return "", "", err
		}
		asOf = today
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
