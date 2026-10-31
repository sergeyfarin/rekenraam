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

// homeWidgetFixture extends the Home fixture (2026-10-15, EUR default) with
// expense categories, a USD currency and the budget and recurring services
// Home composes. Every figure below is derived on paper.
type homeWidgetFixture struct {
	homeFixture
	home      *HomeService
	budgets   *BudgetService
	recurring *RecurringService
	usd       int64
	groceries int64
	rent      int64
	dining    int64
	transport int64
}

func newHomeWidgetFixture(t *testing.T) homeWidgetFixture {
	t.Helper()
	f := newHomeFixture(t)
	now := func() time.Time { return time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC) }
	settings := NewSettingsService(db.NewSettingsRepository(f.database))
	budgets := NewBudgetService(db.NewBudgetRepository(f.database), settings)
	budgets.SetNowForTest(now)
	recurring := NewRecurringService(db.NewRecurringRepository(f.database), f.transactionService, settings)
	recurring.SetNowForTest(now)
	w := homeWidgetFixture{homeFixture: f, home: NewHomeService(f.transactionService, budgets, recurring), budgets: budgets, recurring: recurring,
		usd: seedTestCurrencyCommodity(t, f.database, "USD")}
	nameHomeAccount(t, f, f.cashAccountID, "Checking")
	w.groceries = w.category(t, "Groceries")
	w.rent = w.category(t, "Rent")
	w.dining = w.category(t, "Dining out")
	w.transport = w.category(t, "Transport")
	return w
}

// reviseHomeAccount appends a version (account versions are append-only) with
// a new name and, when parent is set, a parent account.
func reviseHomeAccount(t *testing.T, f homeFixture, accountID int64, name string, parent *int64) {
	t.Helper()
	_, err := f.database.Exec(`
		INSERT INTO account_versions (
			account_id, version_seq, effective_from, recorded_at, changed_by_user_id, change_reason, status, opened_on, closed_on,
			code, name, account_class, account_kind, allows_postings, parent_account_id)
		SELECT account_id, version_seq + 1, effective_from, '2026-01-02T00:00:00Z', 1, 'Home fixture', status, opened_on, closed_on,
			?, ?, account_class, account_kind, allows_postings, COALESCE(?, parent_account_id)
		FROM account_versions WHERE account_id = ? ORDER BY version_seq DESC LIMIT 1`, name, name, parent, accountID)
	require.NoError(t, err)
}

func nameHomeAccount(t *testing.T, f homeFixture, accountID int64, name string) {
	t.Helper()
	reviseHomeAccount(t, f, accountID, name, nil)
}

func (f homeWidgetFixture) category(t *testing.T, name string) int64 {
	t.Helper()
	id := seedTestAccountWithClass(t, f.database, "active", true, "expense", "expense")
	nameHomeAccount(t, f.homeFixture, id, name)
	return id
}

func (f homeWidgetFixture) account(t *testing.T, name, class, kind string) int64 {
	t.Helper()
	id := seedTestAccountWithClass(t, f.database, "active", true, class, kind)
	nameHomeAccount(t, f.homeFixture, id, name)
	return id
}

// spend posts cents of commodity from account to an expense category.
func (f homeWidgetFixture) spend(t *testing.T, date string, accountID, categoryID, commodityID, cents int64) {
	t.Helper()
	_, err := f.transactionService.CreateTransaction(context.Background(), CreateTransactionInput{
		OwnerUserID: 1, OriginType: "internal", Spec: TransactionInput{TransactionDate: date, Status: "posted",
			JournalEntries: []JournalEntryInput{{Postings: []PostingInput{
				{AccountID: categoryID, CommodityID: commodityID, QuantityValue: exact.New(cents), QuantityScale: 2},
				{AccountID: accountID, CommodityID: commodityID, QuantityValue: exact.New(-cents), QuantityScale: 2},
			}}}}})
	require.NoError(t, err)
}

func (f homeWidgetFixture) target(t *testing.T, categoryID, commodityID, cents int64) {
	t.Helper()
	_, err := f.budgets.SetTarget(context.Background(), SetBudgetTargetInput{
		BudgetMutationContext: BudgetMutationContext{OwnerUserID: 1, OriginType: "internal", Reason: "Home fixture"},
		CategoryID:            categoryID, CommodityID: commodityID, PeriodStart: "2026-10-01",
		QuantityValue: exact.New(cents), QuantityScale: 2})
	require.NoError(t, err)
}

// rate enters 1 USD = 0.9 EUR on date.
func (f homeWidgetFixture) rate(t *testing.T, date string) {
	t.Helper()
	_, err := f.pricing.CreateManualPrice(context.Background(), ManualPriceInput{OwnerUserID: 1,
		BaseCommodityID: f.usd, QuoteCommodityID: f.eurCommodityID, ValuationDate: date,
		PriceValue: "9", PriceScale: 1, BaseQuantityValue: "1", Reason: "Home fixture"})
	require.NoError(t, err)
}

// octoberSpending is €1,670.00 across four categories in October, plus a
// September purchase the month leaves out:
// Rent 1,200.00, Groceries 300.00 + 50.00, Dining out 80.00, Transport 40.00.
func (f homeWidgetFixture) octoberSpending(t *testing.T) {
	t.Helper()
	f.move(t, "2026-09-01", f.cashAccountID, f.eurCommodityID, 500000)
	f.spend(t, "2026-09-30", f.cashAccountID, f.groceries, f.eurCommodityID, 99900)
	f.spend(t, "2026-10-01", f.cashAccountID, f.rent, f.eurCommodityID, 120000)
	f.spend(t, "2026-10-03", f.cashAccountID, f.groceries, f.eurCommodityID, 30000)
	f.spend(t, "2026-10-10", f.cashAccountID, f.groceries, f.eurCommodityID, 5000)
	f.spend(t, "2026-10-11", f.cashAccountID, f.dining, f.eurCommodityID, 8000)
	f.spend(t, "2026-10-12", f.cashAccountID, f.transport, f.eurCommodityID, 4000)
}

func (f homeWidgetFixture) read(t *testing.T) Home {
	t.Helper()
	home, err := f.home.Home(context.Background(), 1)
	require.NoError(t, err)
	for name, widgetErr := range map[string]error{"spending": home.Spending.Err, "budgets": home.Budgets.Err, "accounts": home.Accounts.Err,
		"recurring": home.Recurring.Err, "net worth trend": home.NetWorthTrend.Err} {
		require.NoError(t, widgetErr, name)
	}
	return home
}

func TestHomeSpendingWidgetAgreesWithTheReportAndConvertsCompleteOnly(t *testing.T) {
	t.Parallel()
	f := newHomeWidgetFixture(t)
	f.octoberSpending(t)

	spending := f.read(t).Spending.Summary
	require.NotNil(t, spending)
	assert.Equal(t, "2026-10-01", spending.StartDate)
	assert.Equal(t, "2026-10-15", spending.EndDate)
	requireHomeCents(t, 167000, spending.Total, "October spending, September left out")
	assert.Equal(t, 4, spending.CategoryCount)
	require.Len(t, spending.Categories, 3, "three rows at most")
	for index, want := range []struct {
		id    int64
		name  string
		cents int64
	}{{f.rent, "Rent", 120000}, {f.groceries, "Groceries", 35000}, {f.dining, "Dining out", 8000}} {
		assert.Equal(t, want.id, spending.Categories[index].CategoryID)
		assert.Equal(t, want.name, spending.Categories[index].Name)
		requireHomeCents(t, want.cents, spending.Categories[index].Converted, want.name)
	}

	report, err := f.transactionService.Spending(context.Background(), SpendingInput{StartDate: "2026-10-01", EndDate: "2026-10-15",
		GroupBy: "category", Mode: "spending", Reporting: &ReportingCurrencyInput{CommodityID: f.eurCommodityID}})
	require.NoError(t, err)
	assert.Equal(t, *report.ConvertedTotal, *spending.Total, "the widget's figure is the report's")
	assert.Equal(t, report.CommodityTotals, spending.CommodityTotals)
	assert.Equal(t, *report.Groups[0].CategoryID, spending.Categories[0].CategoryID, "rows keep the report's order")

	// $20.00 of dining from a USD account with no rate: the total is never
	// partial, while each currency's exact total stays.
	wallet := f.account(t, "Dollar savings", "asset", "savings")
	f.move(t, "2026-10-01", wallet, f.usd, 10000)
	f.spend(t, "2026-10-12", wallet, f.dining, f.usd, 2000)
	spending = f.read(t).Spending.Summary
	assert.Nil(t, spending.Total, "unconvertible USD leaves no total")
	require.Len(t, spending.CommodityTotals, 2)
	assert.ElementsMatch(t, []int64{f.eurCommodityID, f.usd}, []int64{spending.CommodityTotals[0].CommodityID, spending.CommodityTotals[1].CommodityID})

	// With 1 USD = €0.90 on the day: 1,670.00 + 18.00.
	f.rate(t, "2026-10-12")
	spending = f.read(t).Spending.Summary
	requireHomeCents(t, 168800, spending.Total, "USD converted at its entry date")
}

func TestHomeBudgetWidgetLeadsWithOverspendingInOneCurrency(t *testing.T) {
	t.Parallel()
	f := newHomeWidgetFixture(t)
	f.octoberSpending(t)

	budgets := f.read(t).Budgets.Summary
	require.NotNil(t, budgets)
	assert.Zero(t, budgets.CommodityID, "no target: not set up")
	assert.Nil(t, budgets.Total)
	assert.Empty(t, budgets.Categories)

	// Groceries 350 of 400 (87.5%), Rent 1,200 of 1,200 (100%), Dining out
	// 80 of 50 (30 over), Transport 40 of 100 (40%), and a USD target that is
	// another currency, never added to the euro figures.
	f.target(t, f.groceries, f.eurCommodityID, 40000)
	f.target(t, f.rent, f.eurCommodityID, 120000)
	f.target(t, f.dining, f.eurCommodityID, 5000)
	f.target(t, f.transport, f.eurCommodityID, 10000)
	f.target(t, f.dining, f.usd, 1000)

	budgets = f.read(t).Budgets.Summary
	assert.Equal(t, "2026-10-01", budgets.PeriodStart)
	assert.Equal(t, "2026-10-31", budgets.PeriodEnd)
	assert.Equal(t, f.eurCommodityID, budgets.CommodityID, "the default currency leads")
	assert.Equal(t, 1, budgets.OtherCommodityCount)
	assert.Equal(t, 4, budgets.CategoryCount)
	require.Len(t, budgets.Categories, 3)
	assert.Equal(t, []string{"Dining out", "Rent", "Groceries"},
		[]string{budgets.Categories[0].Name, budgets.Categories[1].Name, budgets.Categories[2].Name}, "over budget first, then share used")
	requireBudgetCents(t, -3000, budgets.Categories[0].Amount.Remaining, "Dining out is 30.00 over")

	// Targets 1,750.00, actuals 1,670.00, 80.00 left: the budget month's total.
	require.NotNil(t, budgets.Total)
	requireBudgetCents(t, 175000, budgets.Total.Target, "target")
	requireBudgetCents(t, 167000, budgets.Total.Actual, "actual")
	requireBudgetCents(t, 8000, budgets.Total.Remaining, "left")
	month, err := f.budgets.Month(context.Background(), "2026-10-01")
	require.NoError(t, err)
	for _, total := range month.Totals {
		if total.CategoryType == "expense" && total.Amount.CommodityID == f.eurCommodityID {
			assert.Equal(t, total.Amount, *budgets.Total, "the widget's total is the budget page's")
		}
	}
}

func requireBudgetCents(t *testing.T, want int64, got BudgetAmount, label string) {
	t.Helper()
	assert.Zero(t, exact.ScaledIntFromCoefficient(got.QuantityValue, got.QuantityScale).Cmp(exact.ScaledIntFromInt64(want, 2)),
		"%s: got %s at scale %d", label, got.QuantityValue, got.QuantityScale)
}

func TestHomeAccountsWidgetRanksOpenAccountsByValueWithTheirHoldings(t *testing.T) {
	t.Parallel()
	f := newHomeWidgetFixture(t)
	// Checking: 1,000.00 in, 500.00 to buy 10 TEST, so 500.00.
	f.move(t, "2026-09-10", f.cashAccountID, f.eurCommodityID, 100000)
	f.buyTen(t)
	f.price(t, f.stockCommodityID, "2026-10-14", "60")
	brokerage := f.account(t, "Brokerage", "asset", "brokerage")
	reviseHomeAccount(t, f.homeFixture, f.holdingAccountID, "TEST holding", &brokerage)
	card := f.account(t, "Card", "liability", "credit_card")
	f.move(t, "2026-10-02", card, f.eurCommodityID, -200000)
	dollars := f.account(t, "Dollar savings", "asset", "savings")
	f.move(t, "2026-10-02", dollars, f.usd, 100000)
	pocket := f.account(t, "Pocket", "asset", "cash")
	f.move(t, "2026-10-02", pocket, f.eurCommodityID, 1000)
	closed := seedTestAccount(t, f.database, "closed", true)
	nameHomeAccount(t, f.homeFixture, closed, "Old account")

	names := func(summary *HomeAccountsSummary) []string {
		result := []string{}
		for _, row := range summary.Accounts {
			result = append(result, row.Name)
		}
		return result
	}
	// Without a USD rate the dollar account has no value and ranks after every
	// account that has one: Card 2,000.00, Brokerage 10 × €60, Checking 500.00,
	// Pocket 10.00.
	accounts := f.read(t).Accounts.Summary
	require.NotNil(t, accounts)
	assert.Equal(t, "2026-10-15", accounts.AsOf)
	assert.Equal(t, 5, accounts.OpenCount, "closed and holding accounts are not counted")
	assert.Equal(t, []string{"Card", "Brokerage", "Checking", "Pocket"}, names(accounts))

	// 1 USD = €0.90: 1,000.00 USD is €900.00 and moves up.
	f.rate(t, "2026-10-15")
	accounts = f.read(t).Accounts.Summary
	assert.Equal(t, []string{"Card", "Dollar savings", "Brokerage", "Checking"}, names(accounts))
	requireHomeCents(t, -200000, accounts.Accounts[0].Value, "a liability ranks by its size")
	requireHomeCents(t, 90000, accounts.Accounts[1].Value, "dollars in euro")
	requireHomeCents(t, 60000, accounts.Accounts[2].Value, "the brokerage carries its holding")
	requireHomeCents(t, 50000, accounts.Accounts[3].Value, "checking")

	balances, err := f.transactionService.AccountBalances(context.Background(), AccountBalancesInput{OwnerUserID: 1, AsOf: "2026-10-15"})
	require.NoError(t, err)
	for _, account := range balances.Accounts {
		if account.AccountID == brokerage {
			assert.Equal(t, account.SubtreeBalances, accounts.Accounts[2].Balances, "the row is the account's own balance read")
		}
	}
}

func TestHomeRecurringWidgetListsTheNextDueActiveSchedules(t *testing.T) {
	t.Parallel()
	f := newHomeWidgetFixture(t)
	schedule := func(name string, day int, startsOn string, cents int64) RecurringTemplate {
		postings := []db.RecurringTemplatePostingSpec{
			{AccountID: f.rent, CommodityID: f.eurCommodityID, QuantityValue: exact.New(cents), QuantityScale: 2},
			{AccountID: f.cashAccountID, CommodityID: f.eurCommodityID, QuantityValue: exact.New(-cents), QuantityScale: 2},
		}
		template, err := f.recurring.CreateTemplate(context.Background(), WriteRecurringTemplateInput{OwnerUserID: 1, Patch: RecurringTemplatePatch{
			Name: recurringTestPtr(name), Frequency: recurringTestPtr("monthly"), StartsOn: recurringTestPtr(startsOn),
			DayOfMonth: NullablePatch[int]{Set: true, Value: recurringTestPtr(day)}, Postings: &postings}})
		require.NoError(t, err)
		return template
	}
	schedule("Rent", 1, "2026-10-16", 120000)
	schedule("Energy", 20, "2026-10-16", 14200)
	schedule("Insurance", 25, "2026-10-16", 4550)
	schedule("Gym", 28, "2026-10-16", 3000)
	paused := schedule("Paused", 17, "2026-10-16", 1000)
	_, err := f.recurring.UpdateTemplate(context.Background(), WriteRecurringTemplateInput{OwnerUserID: 1, TemplateID: paused.ID,
		Patch: RecurringTemplatePatch{Enabled: recurringTestPtr(false)}})
	require.NoError(t, err)
	archived := schedule("Archived", 18, "2026-10-16", 1000)
	_, err = f.recurring.ArchiveTemplate(context.Background(), WriteRecurringTemplateInput{OwnerUserID: 1, TemplateID: archived.ID})
	require.NoError(t, err)

	recurring := f.read(t).Recurring.Summary
	require.NotNil(t, recurring)
	assert.Equal(t, 4, recurring.ActiveCount, "paused and archived schedules are not active")
	require.Len(t, recurring.Upcoming, 3)
	for index, want := range []struct {
		name, date string
		cents      int64
	}{{"Energy", "2026-10-20", 14200}, {"Insurance", "2026-10-25", 4550}, {"Gym", "2026-10-28", 3000}} {
		item := recurring.Upcoming[index]
		assert.Equal(t, want.name, item.Name)
		assert.Equal(t, want.date, item.NextDueOn, "%s's next due date, as the Recurring page shows it", want.name)
		require.Len(t, item.Amounts, 1)
		requireHomeCents(t, want.cents, &item.Amounts[0], want.name)
	}
	counts, err := f.recurring.Summary(context.Background(), 1)
	require.NoError(t, err)
	assert.Equal(t, counts.DraftCount, recurring.DraftCount)
	assert.Equal(t, counts.BlockedCount, recurring.BlockedCount)
}

func TestHomeNetWorthTrendIsTheSeriesAndEndsOnTheHeadline(t *testing.T) {
	t.Parallel()
	f := newHomeWidgetFixture(t)
	f.move(t, "2026-09-10", f.cashAccountID, f.eurCommodityID, 100000)
	f.buyTen(t)
	f.move(t, "2026-10-05", f.cashAccountID, f.eurCommodityID, 20000)
	f.price(t, f.stockCommodityID, "2026-10-13", "60")

	home := f.read(t)
	trend := home.NetWorthTrend.Summary
	require.NotNil(t, trend)
	assert.Equal(t, "2025-10-31", trend.StartDate)
	assert.Equal(t, "2026-10-15", trend.EndDate)
	require.Len(t, trend.Points, 13, "the end of October 2025, eleven month ends, today")
	assert.Equal(t, "2025-10-31", trend.Points[0].Date)
	assert.Equal(t, "2026-02-28", trend.Points[4].Date)
	assert.Equal(t, "2026-09-30", trend.Points[11].Date)
	requireHomeCents(t, 0, trend.Points[0].Value, "nothing held a year ago")
	requireHomeCents(t, 100000, trend.Points[11].Value, "30 Sep: 500.00 cash + 10 × €50")
	requireHomeCents(t, 130000, trend.Points[12].Value, "15 Oct: 700.00 cash + 10 × €60")
	requireHomeCents(t, 130000, trend.Change, "a year's change")
	netWorth := homeHeadline(t, home.Briefing, HomeHeadlineNetWorth)
	assert.Equal(t, *netWorth.Value, *trend.Points[12].Value, "the trend ends on the headline")

	series, err := f.transactionService.NetWorthSeries(context.Background(), NetWorthSeriesInput{StartDate: "2025-10-31", EndDate: "2026-10-15",
		Bucket: "month", Reporting: &ReportingCurrencyInput{CommodityID: f.eurCommodityID}})
	require.NoError(t, err)
	require.Len(t, series.Buckets, len(trend.Points))
	for index, bucket := range series.Buckets {
		assert.Equal(t, bucket.EndDate, trend.Points[index].Date)
		assert.Equal(t, bucket.Converted, trend.Points[index].Value, "point %d is the report's", index)
	}
}

// A price missing today leaves today's point, and so the change, empty; it
// never becomes a partial figure, and earlier months keep their values.
func TestHomeNetWorthTrendMissingPriceLeavesTodayEmpty(t *testing.T) {
	t.Parallel()
	f := newHomeWidgetFixture(t)
	f.move(t, "2026-09-10", f.cashAccountID, f.eurCommodityID, 100000)
	f.buyTen(t)

	trend := f.read(t).NetWorthTrend.Summary
	require.Len(t, trend.Points, 13)
	requireHomeCents(t, 100000, trend.Points[11].Value, "the 28 Sep price values 30 Sep")
	assert.Nil(t, trend.Points[12].Value, "a 17-day-old price is missing today")
	assert.Nil(t, trend.Change)
}

func TestHomeWidgetsOfABookWithoutActivityAreZero(t *testing.T) {
	t.Parallel()
	f := newHomeWidgetFixture(t)
	home := f.read(t)

	requireHomeCents(t, 0, home.Spending.Summary.Total, "nothing spent")
	assert.Empty(t, home.Spending.Summary.Categories)
	assert.Zero(t, home.Budgets.Summary.CommodityID)
	assert.Equal(t, 1, home.Accounts.Summary.OpenCount)
	assert.Empty(t, home.Recurring.Summary.Upcoming)
	assert.Zero(t, home.Recurring.Summary.ActiveCount)
	requireHomeCents(t, 0, home.NetWorthTrend.Summary.Change, "no change")
}

// Without a default currency nothing converts: amounts stay exact per
// commodity and the trend has no points.
func TestHomeWidgetsWithoutADefaultCurrency(t *testing.T) {
	t.Parallel()
	f := newHomeWidgetFixture(t)
	f.octoberSpending(t)
	_, err := f.database.Exec(`UPDATE books SET default_currency_commodity_id = NULL WHERE id = 1`)
	require.NoError(t, err)

	home := f.read(t)
	assert.Nil(t, home.Spending.Summary.Total)
	require.Len(t, home.Spending.Summary.CommodityTotals, 1)
	requireHomeCents(t, 167000, &home.Spending.Summary.CommodityTotals[0], "the exact euro total")
	assert.Empty(t, home.NetWorthTrend.Summary.Points)
	for _, row := range home.Accounts.Summary.Accounts {
		assert.Nil(t, row.Value)
	}
}

// One widget that cannot be read fails alone: the briefing and the other
// widgets still answer, and a cancelled request is never a widget failure.
func TestHomeWidgetFailureStaysLocal(t *testing.T) {
	t.Parallel()
	f := newHomeWidgetFixture(t)
	f.octoberSpending(t)
	broken := openConnectionTestDatabase(t)
	require.NoError(t, broken.Close())
	f.home.recurring = NewRecurringService(db.NewRecurringRepository(broken), f.transactionService, NewSettingsService(db.NewSettingsRepository(broken)))

	home, err := f.home.Home(context.Background(), 1)
	require.NoError(t, err)
	require.Error(t, home.Recurring.Err)
	assert.Nil(t, home.Recurring.Summary)
	assert.NoError(t, home.Spending.Err)
	requireHomeCents(t, 167000, home.Spending.Summary.Total, "spending still answers")
	assert.NoError(t, home.Budgets.Err)
	assert.NoError(t, home.Accounts.Err)
	assert.NoError(t, home.NetWorthTrend.Err)
	assert.NotEmpty(t, home.Briefing.Headlines)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = f.home.Home(ctx, 1)
	require.ErrorIs(t, err, context.Canceled)
}

func TestHomeCommoditiesLabelEveryWidgetAmount(t *testing.T) {
	t.Parallel()
	f := newHomeWidgetFixture(t)
	f.octoberSpending(t)
	wallet := f.account(t, "Dollar savings", "asset", "savings")
	f.move(t, "2026-10-01", wallet, f.usd, 10000)

	home := f.read(t)
	codes := map[int64]string{}
	for _, commodity := range home.Commodities {
		codes[commodity.ID] = commodity.Code
	}
	assert.Equal(t, "EUR", codes[f.eurCommodityID])
	assert.Equal(t, "USD", codes[f.usd], "the dollar account's balance is labelled")
}

func homeBudgetActual(t *testing.T, home Home, commodityID int64) BudgetAmount {
	t.Helper()
	require.NotNil(t, home.Budgets.Summary)
	require.NotNil(t, home.Budgets.Summary.Total)
	require.Equal(t, commodityID, home.Budgets.Summary.CommodityID)
	return home.Budgets.Summary.Total.Actual
}

// Spending and budgets are separate reads. An expense committed between them
// would show €1,670 spent beside €1,675 of budget actuals; Home sees the book
// change and reads again, so both cards describe the same state.
func TestHomeRereadsWhenTheBookChangesBetweenSummaries(t *testing.T) {
	t.Parallel()
	f := newHomeWidgetFixture(t)
	f.octoberSpending(t)
	f.target(t, f.groceries, f.eurCommodityID, 40000)
	written := 0
	f.home.afterRead = func(summary string) {
		if summary == "spending" && written == 0 {
			written++
			f.spend(t, "2026-10-14", f.cashAccountID, f.groceries, f.eurCommodityID, 500)
		}
	}

	home := f.read(t)
	assert.Equal(t, 1, written)
	assert.True(t, home.Consistent)
	requireHomeCents(t, 167500, home.Spending.Summary.Total, "spending includes the expense")
	requireBudgetCents(t, 167500, homeBudgetActual(t, home, f.eurCommodityID), "budget actuals agree")
}

// A book that changes on every read is never shown as one state: the last
// read comes back marked inconsistent, for the page to say so.
func TestHomeSaysWhenTheBookKeepsChanging(t *testing.T) {
	t.Parallel()
	f := newHomeWidgetFixture(t)
	f.octoberSpending(t)
	writes := 0
	f.home.afterRead = func(summary string) {
		if summary == "spending" {
			writes++
			f.spend(t, "2026-10-14", f.cashAccountID, f.groceries, f.eurCommodityID, 500)
		}
	}

	home := f.read(t)
	assert.Equal(t, homeComposeAttempts, writes)
	assert.False(t, home.Consistent)
	require.NotNil(t, home.Spending.Summary)

	f.home.afterRead = nil
	assert.True(t, f.read(t).Consistent, "a quiet book reads consistently")
}
