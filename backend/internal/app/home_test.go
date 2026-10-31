package app

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"rekenraam/backend/internal/db"
	"rekenraam/backend/internal/exact"
)

// homeFixture is a book on 2026-10-15 (UTC): a EUR checking account and a
// brokerage holding of TEST, with EUR as the default currency. Prices are
// entered by hand so every figure below is derived on paper.
type homeFixture struct {
	*investmentsTestFixture
	pricing *PricingService
	equity  int64
}

func newHomeFixture(t *testing.T) homeFixture {
	t.Helper()
	// Without the investment pricing service a buy records no trade-implied
	// price, so the only observations are the ones a test enters.
	f := newInvestmentsTestFixtureWithOptions(t, false)
	_, err := f.database.Exec(`UPDATE books SET default_currency_commodity_id=? WHERE id=1`, f.eurCommodityID)
	require.NoError(t, err)
	now := func() time.Time { return time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC) }
	pricing := NewPricingService(db.NewPricingRepository(f.database))
	pricing.SetNowForTest(now)
	f.transactionService.SetNowForTest(now)
	f.transactionService.SetPricingRepository(db.NewPricingRepository(f.database))
	return homeFixture{f, pricing, seedTestAccountWithClass(t, f.database, "active", true, "equity", "equity")}
}

func (f homeFixture) move(t *testing.T, date string, accountID, commodityID int64, cents int64) {
	t.Helper()
	_, err := f.transactionService.CreateTransaction(context.Background(), CreateTransactionInput{
		OwnerUserID: 1, OriginType: "internal", Spec: TransactionInput{TransactionDate: date, Status: "posted",
			JournalEntries: []JournalEntryInput{{Postings: []PostingInput{
				{AccountID: accountID, CommodityID: commodityID, QuantityValue: exact.New(cents), QuantityScale: 2},
				{AccountID: f.equity, CommodityID: commodityID, QuantityValue: exact.New(-cents), QuantityScale: 2},
			}}}}})
	require.NoError(t, err)
}

// buyTen buys 10 TEST for €500.00 on 2026-09-28 and prices it there at €50.
func (f homeFixture) buyTen(t *testing.T) {
	t.Helper()
	_, err := f.investmentService.Buy(context.Background(), InvestmentTradeInput{
		OwnerUserID: f.ownerUserID, TransactionDate: "2026-09-28",
		CommodityID: f.stockCommodityID, HoldingAccountID: f.holdingAccountID,
		CashAccountID: f.cashAccountID, CashCommodityID: f.eurCommodityID,
		QuantityValue: exact.New(10), CashAmountValue: 50000, CashAmountScale: 2,
	})
	require.NoError(t, err)
	f.price(t, f.stockCommodityID, "2026-09-28", "50")
}

func (f homeFixture) price(t *testing.T, base int64, date, value string) {
	t.Helper()
	f.priceIn(t, base, f.eurCommodityID, date, value)
}

func (f homeFixture) priceIn(t *testing.T, base, quote int64, date, value string) {
	t.Helper()
	_, err := f.pricing.CreateManualPrice(context.Background(), ManualPriceInput{OwnerUserID: 1,
		BaseCommodityID: base, QuoteCommodityID: quote, ValuationDate: date,
		PriceValue: value, PriceScale: 0, BaseQuantityValue: "1", Reason: "Home fixture"})
	require.NoError(t, err)
}

// requirePricingLists asserts that Prices, filtered the way Home links to it,
// lists a pair for each input Home names: the destination shows the problem.
func (f homeFixture) requirePricingLists(t *testing.T, inputs []HomeValuationInput, filter string) {
	t.Helper()
	workspace, err := f.pricing.Workspace(context.Background(), "2026-10-15")
	require.NoError(t, err)
	require.NotEmpty(t, inputs)
	for _, input := range inputs {
		statuses := []string{}
		for _, coverage := range workspace.Coverage {
			if coverage.BaseCommodityID == input.CommodityID {
				statuses = append(statuses, coverage.Status)
			}
		}
		switch filter {
		case "unusable":
			assert.Condition(t, func() bool {
				for _, status := range statuses {
					if status != "usable" {
						return true
					}
				}
				return false
			}, "%s: Prices filtered to unusable pairs lists it (statuses %v)", input.Code, statuses)
		case "stale":
			assert.Contains(t, statuses, "stale", "%s: Prices filtered to stale pairs lists it", input.Code)
		default:
			t.Fatalf("unknown Pricing filter %q", filter)
		}
	}
}

func (f homeFixture) briefing(t *testing.T) HomeBriefing {
	t.Helper()
	result, err := f.transactionService.HomeBriefing(context.Background(), 1)
	require.NoError(t, err)
	return result
}

func homeHeadline(t *testing.T, briefing HomeBriefing, id string) HomeHeadline {
	t.Helper()
	for _, headline := range briefing.Headlines {
		if headline.ID == id {
			return headline
		}
	}
	t.Fatalf("headline %s missing", id)
	return HomeHeadline{}
}

func requireHomeCents(t *testing.T, want int64, got *BalanceQuantity, label string) {
	t.Helper()
	require.NotNil(t, got, label)
	assert.Zero(t, exact.ScaledIntFromCoefficient(got.QuantityValue, got.QuantityScale).Cmp(exact.ScaledIntFromInt64(want, 2)),
		"%s: got %s at scale %d", label, got.QuantityValue, got.QuantityScale)
}

func TestHomeBriefingAgreesWithNetWorthReportAndMeasuresTheMonth(t *testing.T) {
	t.Parallel()
	f := newHomeFixture(t)
	f.move(t, "2026-09-10", f.cashAccountID, f.eurCommodityID, 100000)
	f.buyTen(t)
	f.move(t, "2026-10-05", f.cashAccountID, f.eurCommodityID, 20000)
	f.price(t, f.stockCommodityID, "2026-10-13", "60")

	briefing := f.briefing(t)
	assert.Equal(t, "2026-10-15", briefing.AsOf)
	assert.Equal(t, "2026-09-30", briefing.StartBoundary)
	assert.False(t, briefing.EmptyBook)
	require.NotNil(t, briefing.ReportingCurrency)
	assert.Equal(t, "EUR", briefing.ReportingCurrency.Code)
	assert.Equal(t, "€", briefing.ReportingCurrency.DisplaySymbol)

	// 30 Sep: cash 1000 - 500 = 500, plus 10 × €50 (28 Sep) = €1,000.00.
	// 15 Oct: cash 500 + 200 = 700, plus 10 × €60 (13 Oct) = €1,300.00.
	netWorth := homeHeadline(t, briefing, HomeHeadlineNetWorth)
	assert.Empty(t, netWorth.Unavailable)
	requireHomeCents(t, 130000, netWorth.Value, "net worth today")
	requireHomeCents(t, 100000, netWorth.StartValue, "net worth at the end of September")
	requireHomeCents(t, 30000, netWorth.Change, "net worth change")
	assert.Equal(t, 1, netWorth.AccountCount, "the holding account is part of the brokerage, not an account of its own")
	assert.Empty(t, netWorth.Missing)
	assert.Empty(t, netWorth.Stale, "a two-day-old price is inside the pricing policy's 3 days, which Prices calls usable")

	cash := homeHeadline(t, briefing, HomeHeadlineCash)
	requireHomeCents(t, 70000, cash.Value, "cash today")
	requireHomeCents(t, 50000, cash.StartValue, "cash at the end of September")
	requireHomeCents(t, 20000, cash.Change, "cash change")
	assert.Equal(t, 1, cash.AccountCount)
	assert.Empty(t, cash.Stale)

	report, err := f.transactionService.NetWorthSeries(context.Background(), NetWorthSeriesInput{
		StartDate: "2026-09-30", EndDate: "2026-10-15", Bucket: "month",
		Reporting: &ReportingCurrencyInput{CommodityID: f.eurCommodityID}})
	require.NoError(t, err)
	require.Len(t, report.Buckets, 2)
	assert.Equal(t, *report.Buckets[0].Converted, *netWorth.StartValue, "Home's start agrees with the report")
	assert.Equal(t, *report.Buckets[1].Converted, *netWorth.Value, "Home's headline agrees with the report")
}

func TestHomeBriefingMissingPriceLeavesNetWorthUnavailableAndCashStanding(t *testing.T) {
	t.Parallel()
	f := newHomeFixture(t)
	f.move(t, "2026-09-10", f.cashAccountID, f.eurCommodityID, 100000)
	// The only TEST price is 17 days old on 15 Oct: outside the 7-day window.
	f.buyTen(t)
	f.move(t, "2026-10-05", f.cashAccountID, f.eurCommodityID, 20000)

	briefing := f.briefing(t)
	netWorth := homeHeadline(t, briefing, HomeHeadlineNetWorth)
	assert.Equal(t, HomeUnavailableValuationGaps, netWorth.Unavailable)
	assert.Nil(t, netWorth.Value, "never a partial net worth")
	assert.Nil(t, netWorth.Change)
	assert.Equal(t, []HomeValuationInput{{CommodityID: f.stockCommodityID, Code: "TEST", Kind: "price"}}, netWorth.Missing)
	assert.Empty(t, netWorth.Stale, "missing outranks out of date")
	f.requirePricingLists(t, netWorth.Missing, "unusable")

	cash := homeHeadline(t, briefing, HomeHeadlineCash)
	assert.Empty(t, cash.Unavailable)
	requireHomeCents(t, 70000, cash.Value, "cash needs no price")
	requireHomeCents(t, 20000, cash.Change, "cash change")
}

// Out of date is the pricing policy's word, the one Prices and Review use: an
// observation older than its staleness days (3 by default) that the report's
// 7-day window still values with.
func TestHomeBriefingOutOfDateFollowsThePricingPolicy(t *testing.T) {
	t.Parallel()
	f := newHomeFixture(t)
	f.move(t, "2026-09-10", f.cashAccountID, f.eurCommodityID, 100000)
	f.buyTen(t)
	f.price(t, f.stockCommodityID, "2026-10-10", "60")

	netWorth := homeHeadline(t, f.briefing(t), HomeHeadlineNetWorth)
	requireHomeCents(t, 110000, netWorth.Value, "a five-day-old price still values the holding")
	assert.Empty(t, netWorth.Missing)
	assert.Equal(t, []HomeValuationInput{{CommodityID: f.stockCommodityID, Code: "TEST", Kind: "price"}}, netWorth.Stale)
	f.requirePricingLists(t, netWorth.Stale, "stale")

	setHomeStalenessPolicy(t, f, 6)
	netWorth = homeHeadline(t, f.briefing(t), HomeHeadlineNetWorth)
	assert.Empty(t, netWorth.Stale, "a longer policy makes the same price current")
}

func setHomeStalenessPolicy(t *testing.T, f homeFixture, days int) {
	t.Helper()
	_, err := f.database.Exec(`INSERT INTO pricing_policies (book_id, staleness_max_days, created_at, created_by_user_id, updated_at, updated_by_user_id)
		VALUES (1, ?, '2026-01-01T00:00:00Z', 1, '2026-01-01T00:00:00Z', 1)
		ON CONFLICT (book_id) DO UPDATE SET staleness_max_days = excluded.staleness_max_days`, days)
	require.NoError(t, err)
}

// A policy longer than the report's window cannot make Prices call usable a
// price the report, and so Home, cannot value with: it is listed as stale.
func TestPricingNeverCallsUsableAPriceTheReportCannotUse(t *testing.T) {
	t.Parallel()
	f := newHomeFixture(t)
	f.move(t, "2026-09-10", f.cashAccountID, f.eurCommodityID, 100000)
	f.buyTen(t)
	f.price(t, f.stockCommodityID, "2026-10-07", "60")
	setHomeStalenessPolicy(t, f, 10)

	netWorth := homeHeadline(t, f.briefing(t), HomeHeadlineNetWorth)
	assert.Equal(t, []HomeValuationInput{{CommodityID: f.stockCommodityID, Code: "TEST", Kind: "price"}}, netWorth.Missing, "eight days is outside the report's window")
	f.requirePricingLists(t, netWorth.Missing, "unusable")
}

// A chained conversion has two observations; each is judged on its own date
// and blamed on its own commodity, so a stale rate never sends the owner to a
// current stock price.
func TestHomeBriefingStaleChainedLegIsBlamedOnItsOwnCommodity(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		priceDate, rateDate string
		staleRate           bool
	}{
		"stale rate, current price": {priceDate: "2026-10-15", rateDate: "2026-10-10", staleRate: true},
		"stale price, current rate": {priceDate: "2026-10-10", rateDate: "2026-10-15"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f := newHomeFixture(t)
			usd := seedTestCurrencyCommodity(t, f.database, "USD")
			f.move(t, "2026-09-10", f.cashAccountID, f.eurCommodityID, 100000)
			f.buyTen(t) // its EUR price is outside the window on 15 Oct
			f.priceIn(t, f.stockCommodityID, usd, tc.priceDate, "60")
			f.price(t, usd, tc.rateDate, "1")

			netWorth := homeHeadline(t, f.briefing(t), HomeHeadlineNetWorth)
			requireHomeCents(t, 110000, netWorth.Value, "10 TEST at $60 at 1.00 plus €500")
			want := HomeValuationInput{CommodityID: f.stockCommodityID, Code: "TEST", Kind: "price"}
			if tc.staleRate {
				want = HomeValuationInput{CommodityID: usd, Code: "USD", Kind: "rate"}
			}
			assert.Equal(t, []HomeValuationInput{want}, netWorth.Stale)
			f.requirePricingLists(t, netWorth.Stale, "stale")
		})
	}
}

func TestHomeBriefingMissingRateIsCountedAsARate(t *testing.T) {
	t.Parallel()
	f := newHomeFixture(t)
	f.move(t, "2026-09-10", f.cashAccountID, f.eurCommodityID, 100000)
	usd := seedTestCurrencyCommodity(t, f.database, "USD")
	receivable := seedTestAccountWithClass(t, f.database, "active", true, "asset", "receivable")
	f.move(t, "2026-10-01", receivable, usd, 5000)

	netWorth := homeHeadline(t, f.briefing(t), HomeHeadlineNetWorth)
	assert.Equal(t, HomeUnavailableValuationGaps, netWorth.Unavailable)
	assert.Equal(t, []HomeValuationInput{{CommodityID: usd, Code: "USD", Kind: "rate"}}, netWorth.Missing)
	assert.Equal(t, 2, netWorth.AccountCount)

	f.price(t, usd, "2026-10-15", "1")
	netWorth = homeHeadline(t, f.briefing(t), HomeHeadlineNetWorth)
	requireHomeCents(t, 105000, netWorth.Value, "€1,000 plus $50 at 1.00")
	// The USD balance did not exist on 30 Sep, so the start needs no rate.
	requireHomeCents(t, 100000, netWorth.StartValue, "net worth at the end of September")
	requireHomeCents(t, 5000, netWorth.Change, "net worth change")
}

func TestHomeBriefingWithoutDefaultCurrencyNamesWhy(t *testing.T) {
	t.Parallel()
	f := newHomeFixture(t)
	_, err := f.database.Exec(`UPDATE books SET default_currency_commodity_id=NULL WHERE id=1`)
	require.NoError(t, err)
	f.move(t, "2026-09-10", f.cashAccountID, f.eurCommodityID, 100000)

	briefing := f.briefing(t)
	assert.Nil(t, briefing.ReportingCurrency)
	for _, headline := range briefing.Headlines {
		assert.Equal(t, HomeUnavailableNoReportingCurrency, headline.Unavailable, headline.ID)
		assert.Nil(t, headline.Value, headline.ID)
	}
}

func TestHomeBriefingEmptyBookIsNotAZeroNetWorth(t *testing.T) {
	t.Parallel()
	database := openConnectionTestDatabase(t)
	service := NewTransactionService(db.NewTransactionRepository(database), db.NewPayeeRepository(database), db.NewAccountRepository(database), db.NewCommodityRepository(database))
	service.SetPricingRepository(db.NewPricingRepository(database))
	seedCommodityTradingAccount(t, database)

	result, err := service.HomeBriefing(context.Background(), 1)
	require.NoError(t, err)
	assert.True(t, result.EmptyBook, "system accounts are not an owner's book")
	require.Len(t, result.Headlines, 2)
	for _, headline := range result.Headlines {
		assert.Equal(t, HomeUnavailableNoAccounts, headline.Unavailable, headline.ID)
		assert.Nil(t, headline.Value, headline.ID)
		assert.Zero(t, headline.AccountCount, headline.ID)
	}
}

func TestHomeBriefingClosedAccountsCountInNetWorthButNotInScope(t *testing.T) {
	t.Parallel()
	f := newHomeFixture(t)
	f.move(t, "2026-09-10", f.cashAccountID, f.eurCommodityID, 100000)
	seedTestAccountWithClass(t, f.database, "closed", true, "asset", "savings")

	briefing := f.briefing(t)
	assert.Equal(t, 1, homeHeadline(t, briefing, HomeHeadlineNetWorth).AccountCount)
	assert.Equal(t, 1, homeHeadline(t, briefing, HomeHeadlineCash).AccountCount)
}

// Closed balances stay in net worth, so closing the last open account leaves
// a figure to show; the count is what the owner has open, not whether to value.
func TestHomeBriefingBookOfClosedAccountsStillHasANetWorth(t *testing.T) {
	t.Parallel()
	f := newHomeFixture(t)
	f.move(t, "2026-09-10", f.cashAccountID, f.eurCommodityID, 100000)
	_, err := f.accountService.CloseAccount(context.Background(), AccountLifecycleInput{OwnerUserID: f.ownerUserID, OriginType: "browser_api", AccountID: f.cashAccountID, ClosedOn: "2026-10-12", EffectiveFrom: "2026-10-12"})
	require.NoError(t, err)

	briefing := f.briefing(t)
	assert.False(t, briefing.EmptyBook)
	netWorth := homeHeadline(t, briefing, HomeHeadlineNetWorth)
	assert.Zero(t, netWorth.AccountCount)
	assert.Empty(t, netWorth.Unavailable)
	requireHomeCents(t, 100000, netWorth.Value, "the closed account's balance")
	report, err := f.transactionService.NetWorthSeries(context.Background(), NetWorthSeriesInput{
		StartDate: "2026-09-30", EndDate: "2026-10-15", Bucket: "month",
		Reporting: &ReportingCurrencyInput{CommodityID: f.eurCommodityID}})
	require.NoError(t, err)
	assert.Equal(t, *report.Buckets[1].Converted, *netWorth.Value, "Home agrees with the report")

	cash := homeHeadline(t, briefing, HomeHeadlineCash)
	assert.Equal(t, HomeUnavailableNoAccounts, cash.Unavailable, "cash is open money accounts")
}

func TestHomePreviousMonthEnd(t *testing.T) {
	t.Parallel()
	for date, want := range map[string]string{
		"2026-10-15": "2026-09-30",
		"2026-10-01": "2026-09-30",
		"2026-03-31": "2026-02-28",
		"2028-03-01": "2028-02-29",
		"2027-01-01": "2026-12-31",
	} {
		got, err := previousMonthEnd(date)
		require.NoError(t, err)
		assert.Equal(t, want, got, date)
	}
}
