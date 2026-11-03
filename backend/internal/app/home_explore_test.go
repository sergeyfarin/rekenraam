package app

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"rekenraam/backend/internal/db"
)

// More to explore (#479): each summary is its canonical read for the same
// dates, scope and currency. Figures are derived on paper.

func TestHomeCashflowWidgetIsTheReportForTheMonth(t *testing.T) {
	t.Parallel()
	f := newHomeWidgetFixture(t)
	// September's €5,000.00 in from equity and €999.00 of groceries are before
	// the month. October: salary €2,500.00 in; €1,670.00 spent; €300.00 moved
	// to a brokerage account outside the liquid-cash scope.
	f.octoberSpending(t)
	salary := f.account(t, "Salary", "income", "income")
	f.spend(t, "2026-10-05", f.cashAccountID, salary, f.eurCommodityID, -250000)
	brokerage := f.account(t, "Brokerage", "asset", "brokerage")
	f.spend(t, "2026-10-06", f.cashAccountID, brokerage, f.eurCommodityID, 30000)

	summary := f.read(t).Cashflow.Summary
	require.NotNil(t, summary)
	assert.Equal(t, "2026-10-01", summary.StartDate)
	assert.Equal(t, "2026-10-15", summary.EndDate)
	assert.Equal(t, "default_liquid_cash", summary.CashScope)
	assert.Equal(t, 1, summary.AccountCount, "checking only; the brokerage is outside the scope")
	requireHomeCents(t, 250000, summary.ConvertedInflow, "money in")
	requireHomeCents(t, 167000, summary.ConvertedOutflow, "money out")
	requireHomeCents(t, 83000, summary.ConvertedOperatingNet, "in minus out, transfers excluded")
	requireHomeCents(t, -30000, summary.ConvertedTransferNet, "the move to the brokerage is a transfer, not spending")
	require.Len(t, summary.OperatingNet, 1)
	requireHomeCents(t, 83000, &summary.OperatingNet[0], "exact in euro")

	report, err := f.transactionService.Cashflow(context.Background(), CashflowInput{StartDate: "2026-10-01", EndDate: "2026-10-15", Bucket: "month",
		Reporting: &ReportingCurrencyInput{CommodityID: f.eurCommodityID}})
	require.NoError(t, err)
	require.Len(t, report.Buckets, 1)
	bucket := report.Buckets[0]
	assert.Equal(t, bucket.Inflow, summary.Inflow, "the report's own figures")
	assert.Equal(t, bucket.Outflow, summary.Outflow)
	assert.Equal(t, bucket.ConvertedOperatingNet, summary.ConvertedOperatingNet)
}

func TestHomeCashflowWidgetNeverConvertsPartially(t *testing.T) {
	t.Parallel()
	f := newHomeWidgetFixture(t)
	dollars := f.account(t, "Dollar checking", "asset", "checking")
	salary := f.account(t, "Salary", "income", "income")
	f.spend(t, "2026-10-05", f.cashAccountID, salary, f.eurCommodityID, -100000)
	f.spend(t, "2026-10-05", dollars, salary, f.usd, -50000)

	// No USD rate: each currency keeps its exact total and nothing is converted.
	summary := f.read(t).Cashflow.Summary
	require.NotNil(t, summary)
	assert.Nil(t, summary.ConvertedInflow)
	assert.Nil(t, summary.ConvertedOperatingNet)
	assert.Len(t, summary.Inflow, 2)

	// 1 USD = €0.90 on the day: €1,000.00 + €450.00.
	f.rate(t, "2026-10-05")
	summary = f.read(t).Cashflow.Summary
	requireHomeCents(t, 145000, summary.ConvertedInflow, "converted at the entry date")
}

func TestHomeHoldingsWidgetValuesPositionsAsNetWorthDoes(t *testing.T) {
	t.Parallel()
	f := newHomeWidgetFixture(t)
	f.move(t, "2026-09-10", f.cashAccountID, f.eurCommodityID, 100000)
	brokerage := f.account(t, "Brokerage", "asset", "brokerage")
	reviseHomeAccount(t, f.homeFixture, f.holdingAccountID, "TEST holding", &brokerage)
	// 10 TEST priced at €50 on 28 Sep: beyond the report's window on 15 Oct, so
	// the position has no value, exactly as net worth has none.
	f.buyTen(t)

	home := f.read(t)
	summary := home.Holdings.Summary
	require.NotNil(t, summary)
	assert.Equal(t, 1, summary.PositionCount)
	assert.Equal(t, 1, summary.UnpricedCount)
	require.Len(t, summary.Holdings, 1)
	row := summary.Holdings[0]
	assert.Equal(t, f.holdingAccountID, row.AccountID)
	assert.Equal(t, brokerage, row.BrokerageID)
	assert.Equal(t, "Brokerage", row.BrokerageName)
	assert.Nil(t, row.Value)
	assert.Nil(t, home.Briefing.Headlines[0].Value, "net worth is unavailable for the same reason")

	// €60 on 14 Oct: 10 × €60.
	f.price(t, f.stockCommodityID, "2026-10-14", "60")
	home = f.read(t)
	summary = home.Holdings.Summary
	assert.Zero(t, summary.UnpricedCount)
	requireHomeCents(t, 60000, summary.Holdings[0].Value, "ten at €60")
	require.Len(t, summary.Holdings[0].Balances, 1)
	assert.Equal(t, f.stockCommodityID, summary.Holdings[0].Balances[0].CommodityID)
	labelled := map[int64]bool{}
	for _, commodity := range home.Commodities {
		labelled[commodity.ID] = true
	}
	assert.True(t, labelled[f.stockCommodityID], "the instrument is labelled without a second read")
}

func TestHomeReturnsWidgetsWithoutASavedRunAreEmpty(t *testing.T) {
	t.Parallel()
	f := newHomeWidgetFixture(t)
	f.buyTen(t)
	home := f.read(t)
	require.NotNil(t, home.Returns.Summary)
	assert.Zero(t, home.Returns.Summary.RunID, "Home never calculates a return")
	require.NotNil(t, home.Allocation.Summary)
	assert.Zero(t, home.Allocation.Summary.RunID)
	assert.Empty(t, home.Allocation.Summary.Rows)
}

func TestHomeIncomeAndBackupsWidgetsOfANewBook(t *testing.T) {
	t.Parallel()
	f := newHomeWidgetFixture(t)
	home := f.read(t)
	require.NotNil(t, home.Income.Summary)
	assert.Zero(t, home.Income.Summary.InstrumentCount)
	assert.Zero(t, home.Income.Summary.UnassignedCount)
	assert.Empty(t, home.Income.Summary.Unassigned)

	backups := home.Backups.Summary
	require.NotNil(t, backups)
	assert.True(t, backups.Enabled, "the default policy backs up nightly")
	assert.NotEmpty(t, backups.NextRunAt)
	assert.Empty(t, backups.LastSuccessAt)
	assert.Empty(t, backups.LatestStatus, "nothing has run yet")
}

func TestHomeExploreFailureStaysLocal(t *testing.T) {
	t.Parallel()
	f := newHomeWidgetFixture(t)
	f.octoberSpending(t)
	broken := openConnectionTestDatabase(t)
	require.NoError(t, broken.Close())
	f.home.backups = NewBackupService(db.NewBackupRepository(broken), db.NewBackgroundWorkRepository(broken), broken, "", t.TempDir())
	f.home.investments = nil

	home, err := f.home.Home(context.Background(), 1)
	require.NoError(t, err)
	assert.Error(t, home.Backups.Err)
	assert.Error(t, home.Returns.Err)
	assert.Error(t, home.Allocation.Err)
	assert.Error(t, home.Income.Err)
	assert.NoError(t, home.Cashflow.Err)
	assert.NoError(t, home.Holdings.Err)
	requireHomeCents(t, 167000, home.Spending.Summary.Total, "My Home still answers")
}

// A foreign holding is valued through a price and a rate; an out-of-date rate
// is the holding's own stale input, reported against the currency, never the
// current price (#479 review).
func TestHomeHoldingsJudgeTheirOwnPricesAndRates(t *testing.T) {
	t.Parallel()
	f := newHomeWidgetFixture(t)
	f.move(t, "2026-09-10", f.cashAccountID, f.eurCommodityID, 100000)
	f.buyTen(t)
	// $60 today; 1 USD = €0.90 from five days ago, older than the policy's
	// three days and inside the report's window: 10 × 60 × 0.9 = €540.
	f.priceIn(t, f.stockCommodityID, f.usd, "2026-10-15", "60")
	f.rate(t, "2026-10-10")

	home := f.read(t)
	summary := home.Holdings.Summary
	require.NotNil(t, summary)
	requireHomeCents(t, 54000, summary.Holdings[0].Value, "valued through the rate")
	assert.Empty(t, summary.Missing)
	assert.Equal(t, []HomeValuationInput{{CommodityID: f.usd, Code: "USD", Kind: "rate"}}, summary.Stale)
	assert.Equal(t, home.Briefing.Headlines[0].Stale, summary.Stale, "the same judgement as the headline")
}

// An unclassified short leaves net worth unavailable, but its inputs are still
// judged: Prices never reads an unchecked set as up to date (#479 review).
func TestHomeJudgesValuationInputsBehindAnUnclassifiedShort(t *testing.T) {
	t.Parallel()
	f := newHomeWidgetFixture(t)
	f.move(t, "2026-09-10", f.cashAccountID, f.eurCommodityID, 100000)
	f.buyTen(t) // priced on 28 Sep only: missing today
	_, _, coins := cryptoFixture(t, f.investmentsTestFixture)
	coins("2026-10-02", -500000000, 100000)

	home := f.read(t)
	netWorth := home.Briefing.Headlines[0]
	assert.Equal(t, HomeUnavailableUnclassifiedShort, netWorth.Unavailable)
	assert.Nil(t, netWorth.Value)
	assert.Contains(t, netWorth.Missing, HomeValuationInput{CommodityID: f.stockCommodityID, Code: "TEST", Kind: "price"})
	assert.Contains(t, home.Holdings.Summary.Missing, HomeValuationInput{CommodityID: f.stockCommodityID, Code: "TEST", Kind: "price"})
}
