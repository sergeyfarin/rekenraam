package app

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"rekenraam/backend/internal/db"
	"rekenraam/backend/internal/exact"
)

// Double entry guarantees that every entry balances. It guarantees nothing
// about which report a posting lands in, and that is where money gets counted
// twice: a transfer read as spending, an investment's clearing leg read as a
// flow, a position counted in both the holding account and the account that
// cleared it. These tests state the expected figures from the household facts
// and then ask separate report implementations for them.

// householdBook is one book with the three shapes that cause miscounting:
// transfers between accounts the report may or may not have selected, an
// investment whose commodity legs pass through a clearing account, and a crypto
// position recorded as an ordinary entry because no subledger command exists.
type householdBook struct {
	*investmentsTestFixture
	savings int64
	card    int64
	expense int64
	wallet  int64
	btc     int64
	trading int64
	usd     int64
	usdCash int64
}

func newHouseholdBook(t *testing.T) *householdBook {
	t.Helper()
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	trading, err := f.investmentService.repository.CommodityTradingAccountID(ctx, BookID)
	require.NoError(t, err)
	return &householdBook{
		investmentsTestFixture: f,
		savings:                seedTestAccountWithClass(t, f.database, "active", true, "asset", "savings"),
		card:                   seedTestAccountWithClass(t, f.database, "active", true, "liability", "credit_card"),
		expense:                seedTestAccountWithClass(t, f.database, "active", true, "expense", "expense"),
		wallet:                 seedTestAccountWithClass(t, f.database, "active", true, "asset", "crypto_wallet"),
		btc:                    seedTestCryptoCommodity(t, f.database, "BTC"),
		trading:                trading,
		usd:                    seedTestCurrencyCommodity(t, f.database, "USD"),
		usdCash:                seedTestAccountWithClass(t, f.database, "active", true, "asset", "checking"),
	}
}

func (h *householdBook) line(account, commodity, value int64, scale int) PostingInput {
	return PostingInput{AccountID: account, CommodityID: commodity, QuantityValue: exact.New(value), QuantityScale: scale}
}

func (h *householdBook) post(t *testing.T, date string, lines ...PostingInput) {
	t.Helper()
	_, err := h.transactionService.CreateTransaction(context.Background(), CreateTransactionInput{
		OwnerUserID: h.ownerUserID, OriginType: "browser_api",
		Spec: TransactionInput{TransactionDate: date, JournalEntries: []JournalEntryInput{{Postings: lines}}}})
	require.NoError(t, err)
}

// amountOf reads one commodity's figure out of a report measure as an exact
// value, so an assertion never depends on the order or scale it came back in.
func amountOf(measure []BalanceQuantity, commodityID int64) *exact.ScaledInt {
	for _, amount := range measure {
		if amount.CommodityID == commodityID {
			return exact.ScaledIntFromCoefficient(amount.QuantityValue, amount.QuantityScale)
		}
	}
	return exact.NewScaledInt()
}

func requireAmount(t *testing.T, measure []BalanceQuantity, commodityID int64, value int64, scale int, what string) {
	t.Helper()
	want := exact.ScaledIntFromInt64(value, scale)
	got := amountOf(measure, commodityID)
	require.Zerof(t, got.Cmp(want), "%s: want %s, got %s", what, want.String(), got.String())
}

// TestTransfersNeverReachSpendingOrIncome. A transfer moves money between
// accounts the household already owns; nothing was earned or spent. The basis
// for these reports is income and expense postings, and a transfer has none —
// but that is a property of the selection, and a selection is exactly the kind
// of thing that changes without anyone noticing.
func TestTransfersNeverReachSpendingOrIncome(t *testing.T) {
	h := newHouseholdBook(t)
	ctx := context.Background()

	h.post(t, "2026-01-01", h.line(h.cashAccountID, h.eurCommodityID, 200000, 2), h.line(h.incomeAccountID, h.eurCommodityID, -200000, 2))
	// Asset to asset.
	h.post(t, "2026-01-05", h.line(h.cashAccountID, h.eurCommodityID, -50000, 2), h.line(h.savings, h.eurCommodityID, 50000, 2))
	// Asset to liability: paying the card down is not spending either.
	h.post(t, "2026-01-06", h.line(h.cashAccountID, h.eurCommodityID, -30000, 2), h.line(h.card, h.eurCommodityID, 30000, 2))
	// Cross-currency, through the clearing account.
	h.post(t, "2026-01-07",
		h.line(h.cashAccountID, h.eurCommodityID, -10000, 2), h.line(h.trading, h.eurCommodityID, 10000, 2),
		h.line(h.usdCash, h.usd, 11000, 2), h.line(h.trading, h.usd, -11000, 2))
	// One real expense, so a zero result cannot come from an empty report.
	h.post(t, "2026-01-08", h.line(h.cashAccountID, h.eurCommodityID, -4500, 2), h.line(h.expense, h.eurCommodityID, 4500, 2))

	spending, err := h.transactionService.Spending(ctx, SpendingInput{
		StartDate: "2026-01-01", EndDate: "2026-01-31", GroupBy: "category", Mode: "spending"})
	require.NoError(t, err)
	requireAmount(t, spending.CommodityTotals, h.eurCommodityID, 4500, 2, "spending is the one expense, not the three transfers")
	requireAmount(t, spending.CommodityTotals, h.usd, 0, 2, "the dollar leg of an FX transfer is not spending")
	require.Len(t, spending.Groups, 1)

	income, err := h.transactionService.Spending(ctx, SpendingInput{
		StartDate: "2026-01-01", EndDate: "2026-01-31", GroupBy: "category", Mode: "income"})
	require.NoError(t, err)
	requireAmount(t, income.CommodityTotals, h.eurCommodityID, 200000, 2, "income is the salary alone")

	categories, err := h.transactionService.CategoryTotals(ctx, CategoryTotalsInput{AfterDate: "2026-01-01", BeforeDate: "2026-01-31"})
	require.NoError(t, err)
	for _, category := range categories.Categories {
		switch category.CategoryID {
		case h.expense:
			requireAmount(t, category.DirectTotals, h.eurCommodityID, 4500, 2, "expense category")
		case h.incomeAccountID:
			requireAmount(t, category.DirectTotals, h.eurCommodityID, -200000, 2, "income category")
		default:
			require.Empty(t, category.DirectTotals, "category %d saw a transfer", category.CategoryID)
		}
	}
}

// TestInvestmentAndCryptoPrincipalIsNeverSpendingOrIncome. Buying an asset is
// not an expense and selling it is not income: the money changed shape. Only
// the dividend is income, and only the withheld tax is an expense.
func TestInvestmentAndCryptoPrincipalIsNeverSpendingOrIncome(t *testing.T) {
	h := newHouseholdBook(t)
	ctx := context.Background()

	h.post(t, "2026-01-01", h.line(h.cashAccountID, h.eurCommodityID, 500000, 2), h.line(h.incomeAccountID, h.eurCommodityID, -500000, 2))
	buyOn(t, h.investmentsTestFixture, "2026-01-02", 10, 10000)
	sale := sellInput(h.investmentsTestFixture, "2026-01-03", 4)
	sale.CashAmountValue = 6000
	_, err := h.investmentService.Sell(ctx, sale)
	require.NoError(t, err)
	// Crypto in and out, as ordinary entries through the clearing account.
	h.post(t, "2026-01-04",
		h.line(h.cashAccountID, h.eurCommodityID, -20000, 2), h.line(h.trading, h.eurCommodityID, 20000, 2),
		h.line(h.wallet, h.btc, 50000000, 8), h.line(h.trading, h.btc, -50000000, 8))
	h.post(t, "2026-01-05",
		h.line(h.cashAccountID, h.eurCommodityID, 25000, 2), h.line(h.trading, h.eurCommodityID, -25000, 2),
		h.line(h.wallet, h.btc, -20000000, 8), h.line(h.trading, h.btc, 20000000, 8))
	withheld, withheldScale := int64(150), 2
	_, err = h.investmentService.Dividend(ctx, DividendInput{
		OwnerUserID: h.ownerUserID, TransactionDate: "2026-01-06",
		CashAccountID: h.cashAccountID, CashCommodityID: h.eurCommodityID,
		IncomeAccountID: &h.incomeAccountID, AmountValue: 1000, AmountScale: 2,
		WithholdingAccountID: &h.expense, WithholdingValue: &withheld, WithholdingScale: &withheldScale})
	require.NoError(t, err)

	spending, err := h.transactionService.Spending(ctx, SpendingInput{
		StartDate: "2026-01-01", EndDate: "2026-01-31", GroupBy: "category", Mode: "spending"})
	require.NoError(t, err)
	requireAmount(t, spending.CommodityTotals, h.eurCommodityID, 150, 2, "only the withheld tax is an expense")
	requireAmount(t, spending.CommodityTotals, h.stockCommodityID, 0, 0, "shares are not spending")
	requireAmount(t, spending.CommodityTotals, h.btc, 0, 8, "coins are not spending")

	income, err := h.transactionService.Spending(ctx, SpendingInput{
		StartDate: "2026-01-01", EndDate: "2026-01-31", GroupBy: "category", Mode: "income"})
	require.NoError(t, err)
	requireAmount(t, income.CommodityTotals, h.eurCommodityID, 501000, 2, "salary plus the gross dividend, and no sale proceeds")
	requireAmount(t, income.CommodityTotals, h.stockCommodityID, 0, 0, "shares are not income")
	requireAmount(t, income.CommodityTotals, h.btc, 0, 8, "coins are not income")
}

// TestNetWorthCountsEachPositionExactlyOnce. Every commodity movement has a
// clearing leg, so an account set that includes the clearing account counts the
// same position twice — once where it landed and once where it came from.
func TestNetWorthCountsEachPositionExactlyOnce(t *testing.T) {
	h := newHouseholdBook(t)
	ctx := context.Background()

	h.post(t, "2026-01-01", h.line(h.cashAccountID, h.eurCommodityID, 500000, 2), h.line(h.incomeAccountID, h.eurCommodityID, -500000, 2))
	buyOn(t, h.investmentsTestFixture, "2026-01-02", 10, 12000)
	h.post(t, "2026-01-03",
		h.line(h.cashAccountID, h.eurCommodityID, -20000, 2), h.line(h.trading, h.eurCommodityID, 20000, 2),
		h.line(h.wallet, h.btc, 50000000, 8), h.line(h.trading, h.btc, -50000000, 8))
	h.post(t, "2026-01-04", h.line(h.cashAccountID, h.eurCommodityID, -30000, 2), h.line(h.savings, h.eurCommodityID, 30000, 2))
	h.post(t, "2026-01-05", h.line(h.card, h.eurCommodityID, -45000, 2), h.line(h.expense, h.eurCommodityID, 45000, 2))

	worth, err := h.transactionService.NetWorth(ctx, NetWorthInput{AsOf: "2026-01-31"})
	require.NoError(t, err)
	// 5000 salary, less 120 of shares and 200 of coins; the savings transfer
	// stays inside assets. The card owes 450. 5000 - 120 - 200 - 450 = 4230.
	requireAmount(t, worth.Totals, h.eurCommodityID, 423000, 2, "euro net worth")
	requireAmount(t, worth.Totals, h.stockCommodityID, 10, 0, "ten shares, counted once")
	requireAmount(t, worth.Totals, h.btc, 50000000, 8, "half a coin, counted once")
	require.Contains(t, worth.ExcludedSystemRoles, "commodity_trading")

	// Priced, the same position is one number. 10 shares at 15.00 and 0.5 BTC
	// at 40,000.00 on top of 4230.00 of money: 4230 + 150 + 20000 = 24,380.00.
	pricing := NewPricingService(db.NewPricingRepository(h.database))
	h.transactionService.SetPricingRepository(db.NewPricingRepository(h.database))
	for _, price := range []struct {
		base  int64
		value int64
	}{{h.stockCommodityID, 1500}, {h.btc, 4000000}} {
		_, err := pricing.CreatePrice(ctx, PriceObservationInput{
			OwnerUserID: h.ownerUserID, BaseCommodityID: price.base, QuoteCommodityID: h.eurCommodityID,
			QuoteType: "close", PriceValue: price.value, PriceScale: 2,
			ValuationDate: "2026-01-31", ObservedAt: "2026-01-31T12:00:00Z"})
		require.NoError(t, err)
	}
	series, err := h.transactionService.NetWorthSeries(ctx, NetWorthSeriesInput{
		StartDate: "2026-01-01", EndDate: "2026-01-31", Bucket: "month",
		Reporting: &ReportingCurrencyInput{CommodityID: h.eurCommodityID, MaxStalenessDay: 30}})
	require.NoError(t, err)
	require.Len(t, series.Buckets, 1)
	require.NotNil(t, series.Buckets[0].Converted)
	require.True(t, series.Valuation.Complete)
	require.Zero(t, exact.ScaledIntFromCoefficient(series.Buckets[0].Converted.QuantityValue, series.Buckets[0].Converted.QuantityScale).
		Cmp(exact.ScaledIntFromInt64(2438000, 2)), "converted net worth, got %s", series.Buckets[0].Converted.QuantityValue.String())
}

// TestCashflowNetMovementEqualsTheSameAccountsNetWorthChange is the strongest
// cross-check available without a second ledger: cashflow classifies postings
// by counterpart, net worth folds balances per account, and the two share no
// code. For one account set over one window they must agree exactly, or one of
// them is counting something twice.
func TestCashflowNetMovementEqualsTheSameAccountsNetWorthChange(t *testing.T) {
	h := newHouseholdBook(t)
	ctx := context.Background()
	scope := []int64{h.cashAccountID, h.savings}

	// An opening position in the prior bucket, so the later bucket's change is
	// not simply its closing balance.
	h.post(t, "2026-01-20", h.line(h.cashAccountID, h.eurCommodityID, 300000, 2), h.line(h.incomeAccountID, h.eurCommodityID, -300000, 2))
	// February: salary, an expense, a transfer inside the scope, a transfer out
	// of it, an investment, a crypto purchase and a card payment.
	h.post(t, "2026-02-03", h.line(h.cashAccountID, h.eurCommodityID, 250000, 2), h.line(h.incomeAccountID, h.eurCommodityID, -250000, 2))
	h.post(t, "2026-02-04", h.line(h.cashAccountID, h.eurCommodityID, -6000, 2), h.line(h.expense, h.eurCommodityID, 6000, 2))
	h.post(t, "2026-02-05", h.line(h.cashAccountID, h.eurCommodityID, -40000, 2), h.line(h.savings, h.eurCommodityID, 40000, 2))
	h.post(t, "2026-02-06", h.line(h.cashAccountID, h.eurCommodityID, -25000, 2), h.line(h.usdCash, h.eurCommodityID, 25000, 2))
	buyOn(t, h.investmentsTestFixture, "2026-02-07", 10, 11000)
	h.post(t, "2026-02-08",
		h.line(h.cashAccountID, h.eurCommodityID, -20000, 2), h.line(h.trading, h.eurCommodityID, 20000, 2),
		h.line(h.wallet, h.btc, 50000000, 8), h.line(h.trading, h.btc, -50000000, 8))
	h.post(t, "2026-02-09", h.line(h.cashAccountID, h.eurCommodityID, -35000, 2), h.line(h.card, h.eurCommodityID, 35000, 2))
	sale := sellInput(h.investmentsTestFixture, "2026-02-10", 3)
	sale.CashAmountValue = 4000
	_, err := h.investmentService.Sell(ctx, sale)
	require.NoError(t, err)

	cashflow, err := h.transactionService.Cashflow(ctx, CashflowInput{
		StartDate: "2026-02-01", EndDate: "2026-02-28", Bucket: "month",
		Filters: ReportFilters{AccountIDs: scope}})
	require.NoError(t, err)
	require.Len(t, cashflow.Buckets, 1)

	worth, err := h.transactionService.NetWorthSeries(ctx, NetWorthSeriesInput{
		StartDate: "2026-01-01", EndDate: "2026-02-28", Bucket: "month",
		Filters: ReportFilters{AccountIDs: scope}})
	require.NoError(t, err)
	require.Len(t, worth.Buckets, 2)

	change := amountOf(worth.Buckets[1].Totals, h.eurCommodityID)
	change.SubScaled(amountOf(worth.Buckets[0].Totals, h.eurCommodityID))
	movement := amountOf(cashflow.Buckets[0].NetMovement, h.eurCommodityID)
	require.Zerof(t, movement.Cmp(change),
		"cashflow net movement %s must equal the same accounts' net worth change %s", movement.String(), change.String())

	// And the figure itself, from the facts: +2500 salary, -60 expense, -250
	// out of scope, -110 shares, -200 coins, -350 card, +40 of sale proceeds.
	// The 400 transfer between the two selected accounts nets to nothing.
	// 2500 - 60 - 250 - 110 - 200 - 350 + 40 = 1570.
	requireAmount(t, cashflow.Buckets[0].NetMovement, h.eurCommodityID, 157000, 2, "February net movement")
	requireAmount(t, cashflow.Buckets[0].Inflow, h.eurCommodityID, 250000, 2, "only the salary is inflow")
	requireAmount(t, cashflow.Buckets[0].Outflow, h.eurCommodityID, 6000, 2, "only the expense is outflow")
	requireAmount(t, cashflow.Buckets[0].TransferIn, h.eurCommodityID, 4000, 2, "sale proceeds arrive as financing")
	// 250 out of scope + 110 shares + 200 coins + 350 card = 910.
	requireAmount(t, cashflow.Buckets[0].TransferOut, h.eurCommodityID, 91000, 2, "out of scope, shares, coins and the card")
	// No countable commodity ever entered or left a cash account.
	for _, measure := range [][]BalanceQuantity{
		cashflow.Buckets[0].TransferIn, cashflow.Buckets[0].TransferOut,
		cashflow.Buckets[0].Inflow, cashflow.Buckets[0].Outflow, cashflow.Buckets[0].NetMovement,
	} {
		requireAmount(t, measure, h.stockCommodityID, 0, 0, "shares in a cash measure")
		requireAmount(t, measure, h.btc, 0, 8, "coins in a cash measure")
	}
}

// TestNetWorthSeriesAgreesWithPointInTimeAtEveryBucketEnd. The series folds one
// ledger read forward across buckets; the point-in-time endpoint re-reads for
// one date. They are separate implementations of one number.
func TestNetWorthSeriesAgreesWithPointInTimeAtEveryBucketEnd(t *testing.T) {
	h := newHouseholdBook(t)
	ctx := context.Background()

	h.post(t, "2026-01-10", h.line(h.cashAccountID, h.eurCommodityID, 100000, 2), h.line(h.incomeAccountID, h.eurCommodityID, -100000, 2))
	h.post(t, "2026-02-10", h.line(h.cashAccountID, h.eurCommodityID, -20000, 2), h.line(h.savings, h.eurCommodityID, 20000, 2))
	h.post(t, "2026-03-10", h.line(h.card, h.eurCommodityID, -30000, 2), h.line(h.expense, h.eurCommodityID, 30000, 2))
	buyOn(t, h.investmentsTestFixture, "2026-04-10", 10, 10000)
	h.post(t, "2026-05-10",
		h.line(h.cashAccountID, h.eurCommodityID, -20000, 2), h.line(h.trading, h.eurCommodityID, 20000, 2),
		h.line(h.wallet, h.btc, 50000000, 8), h.line(h.trading, h.btc, -50000000, 8))

	// An account opened partway through, and later renamed: both add versions
	// the series has to replay in order.
	later, err := h.accountService.CreateAccount(ctx, CreateAccountInput{
		OwnerUserID: h.ownerUserID, OriginType: "browser_api", Operation: "account.create",
		Name: "Opened later", AccountClass: "asset", AccountKind: "other_asset",
		DefaultCommodityID: &h.eurCommodityID, OpenedOn: "2026-04-01", EffectiveFrom: "2026-04-01"})
	require.NoError(t, err)
	h.post(t, "2026-04-20", h.line(later.ID, h.eurCommodityID, 5000, 2), h.line(h.cashAccountID, h.eurCommodityID, -5000, 2))
	_, err = h.accountService.UpdateAccount(ctx, UpdateAccountInput{
		OwnerUserID: h.ownerUserID, OriginType: "browser_api", Operation: "account.update",
		AccountID: later.ID, Code: later.Code, Name: "Renamed later", AccountClass: "asset",
		AccountKind: "other_asset", DefaultCommodityID: &h.eurCommodityID,
		OpenedOn: later.OpenedOn, EffectiveFrom: "2026-06-01"})
	require.NoError(t, err)

	series, err := h.transactionService.NetWorthSeries(ctx, NetWorthSeriesInput{
		StartDate: "2026-01-01", EndDate: "2026-06-30", Bucket: "month"})
	require.NoError(t, err)
	require.Len(t, series.Buckets, 6)
	for _, bucket := range series.Buckets {
		point, err := h.transactionService.NetWorth(ctx, NetWorthInput{AsOf: bucket.EndDate})
		require.NoError(t, err)
		require.Len(t, bucket.Totals, len(point.Totals), "bucket %s names a different set of commodities", bucket.EndDate)
		for _, want := range point.Totals {
			require.Zerof(t, amountOf(bucket.Totals, want.CommodityID).Cmp(
				exact.ScaledIntFromCoefficient(want.QuantityValue, want.QuantityScale)),
				"bucket %s commodity %d", bucket.EndDate, want.CommodityID)
		}
	}
	// The last bucket, from the facts: 1000 salary less 100 of shares, 200 of
	// coins and a 300 card balance; the savings and other-asset transfers stay
	// inside the total.
	requireAmount(t, series.Buckets[5].Totals, h.eurCommodityID, 40000, 2, "June euro net worth")
	requireAmount(t, series.Buckets[5].Totals, h.stockCommodityID, 10, 0, "June shares")
	requireAmount(t, series.Buckets[5].Totals, h.btc, 50000000, 8, "June coins")
}

// TestCategorySubtreeCountsEachPostingOnce. A parent category's subtree total
// includes its own postings and its children's; counting a parent's direct
// postings into its own subtree twice is the classic rollup error.
func TestCategorySubtreeCountsEachPostingOnce(t *testing.T) {
	h := newHouseholdBook(t)
	ctx := context.Background()
	parent, err := h.accountService.CreateAccount(ctx, CreateAccountInput{
		OwnerUserID: h.ownerUserID, OriginType: "browser_api", Operation: "account.create",
		Name: "Household", AccountClass: "expense", AccountKind: "expense",
		OpenedOn: "2026-01-01", EffectiveFrom: "2026-01-01"})
	require.NoError(t, err)
	child, err := h.accountService.CreateAccount(ctx, CreateAccountInput{
		OwnerUserID: h.ownerUserID, OriginType: "browser_api", Operation: "account.create",
		Name: "Groceries", AccountClass: "expense", AccountKind: "expense",
		ParentAccountID: &parent.ID, OpenedOn: "2026-01-01", EffectiveFrom: "2026-01-01"})
	require.NoError(t, err)
	grandchild, err := h.accountService.CreateAccount(ctx, CreateAccountInput{
		OwnerUserID: h.ownerUserID, OriginType: "browser_api", Operation: "account.create",
		Name: "Bakery", AccountClass: "expense", AccountKind: "expense",
		ParentAccountID: &child.ID, OpenedOn: "2026-01-01", EffectiveFrom: "2026-01-01"})
	require.NoError(t, err)

	for _, spend := range []struct {
		account int64
		value   int64
	}{{parent.ID, 1000}, {child.ID, 2500}, {grandchild.ID, 700}} {
		h.post(t, "2026-01-10",
			h.line(spend.account, h.eurCommodityID, spend.value, 2),
			h.line(h.cashAccountID, h.eurCommodityID, -spend.value, 2))
	}

	categories, err := h.transactionService.CategoryTotals(ctx, CategoryTotalsInput{
		AfterDate: "2026-01-01", BeforeDate: "2026-01-31"})
	require.NoError(t, err)
	for _, category := range categories.Categories {
		switch category.CategoryID {
		case parent.ID:
			requireAmount(t, category.DirectTotals, h.eurCommodityID, 1000, 2, "parent direct")
			requireAmount(t, category.SubtreeTotals, h.eurCommodityID, 4200, 2, "parent subtree is 10 + 25 + 7")
		case child.ID:
			requireAmount(t, category.DirectTotals, h.eurCommodityID, 2500, 2, "child direct")
			requireAmount(t, category.SubtreeTotals, h.eurCommodityID, 3200, 2, "child subtree is 25 + 7")
		case grandchild.ID:
			requireAmount(t, category.DirectTotals, h.eurCommodityID, 700, 2, "grandchild direct")
			requireAmount(t, category.SubtreeTotals, h.eurCommodityID, 700, 2, "a leaf's subtree is itself")
		}
	}

	// The spending report groups by the account the posting names, so its total
	// is each posting once — never the rolled-up subtree on top of them.
	spending, err := h.transactionService.Spending(ctx, SpendingInput{
		StartDate: "2026-01-01", EndDate: "2026-01-31", GroupBy: "category", Mode: "spending"})
	require.NoError(t, err)
	requireAmount(t, spending.CommodityTotals, h.eurCommodityID, 4200, 2, "spending counts each posting once")
	require.Len(t, spending.Groups, 3)
}

// TestEditingVoidingAndDeletingLeaveOneVersionInEveryReport. A transaction's
// history is kept as versions, and reports read the current one. If any report
// read the version table directly, editing an amount would count both the old
// and the new figure, and every correction would inflate the year.
func TestEditingVoidingAndDeletingLeaveOneVersionInEveryReport(t *testing.T) {
	h := newHouseholdBook(t)
	ctx := context.Background()

	spend := func(date string, value int64) Transaction {
		t.Helper()
		created, err := h.transactionService.CreateTransaction(ctx, CreateTransactionInput{
			OwnerUserID: h.ownerUserID, OriginType: "browser_api",
			Spec: TransactionInput{TransactionDate: date, JournalEntries: []JournalEntryInput{{Postings: []PostingInput{
				h.line(h.expense, h.eurCommodityID, value, 2),
				h.line(h.cashAccountID, h.eurCommodityID, -value, 2)}}}}})
		require.NoError(t, err)
		return created
	}

	edited := spend("2026-01-05", 10000) // 100.00, corrected to 25.00 below
	voided := spend("2026-01-06", 5000)
	deleted := spend("2026-01-07", 7000)
	kept := spend("2026-01-08", 1500)
	require.NotZero(t, kept.ID)

	// A correction, twice over: an edit that lands a third version must not
	// leave the first two behind either.
	for _, corrected := range []int64{4000, 2500} {
		current, err := h.transactionService.Transaction(ctx, edited.ID)
		require.NoError(t, err)
		spec := transactionInputFromTransaction(current)
		spec.JournalEntries[0].Postings[0].QuantityValue = exact.New(corrected)
		spec.JournalEntries[0].Postings[1].QuantityValue = exact.New(-corrected)
		_, err = h.transactionService.UpdateTransaction(ctx, UpdateTransactionInput{
			OwnerUserID: h.ownerUserID, OriginType: "browser_api", TransactionID: edited.ID,
			Spec: spec, ChangeReason: "corrected the amount"})
		require.NoError(t, err)
	}
	_, err := h.transactionService.VoidTransaction(ctx, VoidTransactionInput{
		OwnerUserID: h.ownerUserID, OriginType: "browser_api", TransactionID: voided.ID,
		ChangeReason: "never happened"})
	require.NoError(t, err)
	_, err = h.transactionService.SoftDeleteTransaction(ctx, TransactionLifecycleInput{
		OwnerUserID: h.ownerUserID, OriginType: "browser_api", TransactionID: deleted.ID,
		ChangeReason: "entered on the wrong book"})
	require.NoError(t, err)

	// 25.00 of corrected spending plus 15.00 that was never touched. The
	// voided and soft-deleted entries are not financial truth, and none of the
	// superseded versions of the corrected one are either.
	spending, err := h.transactionService.Spending(ctx, SpendingInput{
		StartDate: "2026-01-01", EndDate: "2026-01-31", GroupBy: "category", Mode: "spending"})
	require.NoError(t, err)
	requireAmount(t, spending.CommodityTotals, h.eurCommodityID, 4000, 2, "spending counts one version of each transaction")

	categories, err := h.transactionService.CategoryTotals(ctx, CategoryTotalsInput{
		AfterDate: "2026-01-01", BeforeDate: "2026-01-31"})
	require.NoError(t, err)
	for _, category := range categories.Categories {
		if category.CategoryID == h.expense {
			requireAmount(t, category.DirectTotals, h.eurCommodityID, 4000, 2, "category totals")
		}
	}

	worth, err := h.transactionService.NetWorth(ctx, NetWorthInput{AsOf: "2026-01-31"})
	require.NoError(t, err)
	requireAmount(t, worth.Totals, h.eurCommodityID, -4000, 2, "the cash side, once each")

	cashflow, err := h.transactionService.Cashflow(ctx, CashflowInput{
		StartDate: "2026-01-01", EndDate: "2026-01-31", Bucket: "month",
		Filters: ReportFilters{AccountIDs: []int64{h.cashAccountID}}})
	require.NoError(t, err)
	requireAmount(t, cashflow.Buckets[0].Outflow, h.eurCommodityID, 4000, 2, "cashflow outflow")
	requireAmount(t, cashflow.Buckets[0].NetMovement, h.eurCommodityID, -4000, 2, "cashflow net movement")
}
