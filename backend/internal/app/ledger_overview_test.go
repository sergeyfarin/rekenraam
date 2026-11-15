package app

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"rekenraam/backend/internal/db"
	"rekenraam/backend/internal/exact"
)

func TestLedgerOverviewBrokerageIncludesHiddenHoldingsAndUnavailablePrices(t *testing.T) {
	t.Parallel()
	f := newHomeWidgetFixture(t)
	broker := f.account(t, "Broker", "asset", "brokerage")
	reviseHomeAccount(t, f.homeFixture, f.holdingAccountID, "TEST position", &broker)
	f.move(t, "2026-09-10", broker, f.eurCommodityID, 100000)
	cashChild := f.account(t, "Broker currency cash", "asset", "checking")
	reviseHomeAccount(t, f.homeFixture, cashChild, "Broker currency cash", &broker)
	f.move(t, "2026-09-10", cashChild, f.eurCommodityID, 50000)
	f.buyTen(t)
	other, err := f.investmentService.CreateInstrument(context.Background(), InvestmentInstrumentInput{OwnerUserID: 1, OriginType: "internal", CommodityCode: "OTHER", DisplayName: "Other", InstrumentType: "stock", QuantityScale: 0, PriceScale: 2})
	require.NoError(t, err)
	hold, err := f.investmentService.CreateHoldingAccount(context.Background(), HoldingAccountInput{OwnerUserID: 1, InstrumentID: other.ID, Name: "OTHER holding", ParentAccountID: &broker, OpenedOn: "2026-01-01", EffectiveFrom: "2026-01-01"})
	require.NoError(t, err)
	_, err = f.investmentService.Buy(context.Background(), InvestmentTradeInput{OwnerUserID: 1, TransactionDate: "2026-10-01", CommodityID: other.CommodityID, HoldingAccountID: hold.ID, CashAccountID: broker, CashCommodityID: f.eurCommodityID, QuantityValue: exact.New(2), CashAmountValue: 20000, CashAmountScale: 2})
	require.NoError(t, err)
	service := NewLedgerService(f.transactionService, f.accountService, NewInstitutionService(db.NewInstitutionRepository(f.database)))
	read := func() LedgerOverview {
		result, err := service.Overview(context.Background(), LedgerOverviewInput{OwnerUserID: 1, AsOf: "2026-10-15"})
		require.NoError(t, err)
		return result
	}
	result := read()
	catalog, err := NewSearchService(db.NewSearchRepository(f.database), nil).Catalog(context.Background(), "", 500)
	require.NoError(t, err)
	for _, item := range catalog.Items {
		assert.False(t, item.Kind == "account" && (item.ID == f.holdingAccountID || item.ID == hold.ID))
	}
	var row LedgerAccount
	for _, a := range result.Accounts {
		assert.NotContains(t, []string{"security_holding", "fund_holding"}, a.Account.AccountKind)
		if a.Account.ID == broker {
			row = a
		}
	}
	require.Equal(t, broker, row.Account.ID)
	require.Len(t, row.Positions, 2)
	require.Nil(t, row.Value, "a missing price never becomes zero or a known-only total")
	require.Len(t, row.Cash, 1)
	requireAmount(t, row.Cash, f.eurCommodityID, 130000, 2, "1000 direct plus 500 child cash less 200 paid for OTHER")
	// Both units are priced: 1300 cash + 10*60 + 2*120 = 2140.
	require.Len(t, row.CashAccounts, 1)
	assert.Equal(t, cashChild, row.CashAccounts[0].ID)
	assert.Empty(t, row.OtherAccounts)
	holdings := []int64{}
	for _, account := range row.HoldingAccounts {
		holdings = append(holdings, account.ID)
	}
	assert.ElementsMatch(t, []int64{f.holdingAccountID, hold.ID}, holdings, "hidden holdings are disclosed for links and dependency checks")
	f.price(t, f.stockCommodityID, "2026-10-15", "60")
	f.price(t, other.CommodityID, "2026-10-15", "120")
	result = read()
	for _, a := range result.Accounts {
		if a.Account.ID == broker {
			row = a
		}
	}
	requireHomeCents(t, 214000, row.Value, "cash plus two hidden holdings")
	report, err := f.transactionService.NetWorthSeries(context.Background(), NetWorthSeriesInput{StartDate: "2026-10-15", EndDate: "2026-10-15", Bucket: "day", Filters: ReportFilters{AccountIDs: []int64{broker}, IncludeDescendants: true}, Reporting: &ReportingCurrencyInput{CommodityID: f.eurCommodityID}})
	require.NoError(t, err)
	require.Len(t, report.Buckets, 1)
	assert.Equal(t, report.Buckets[0].Converted, row.Value)
	detail, err := service.Overview(context.Background(), LedgerOverviewInput{OwnerUserID: 1, AsOf: "2026-10-15", AccountID: broker})
	require.NoError(t, err)
	require.Len(t, detail.Accounts, 1)
	assert.Equal(t, row, detail.Accounts[0])
}

func TestLedgerOverviewFollowsLargeCatalogAndListsOnlyUnparentedHoldings(t *testing.T) {
	t.Parallel()
	f := newHomeWidgetFixture(t)
	for range 120 {
		f.account(t, "Large book account", "asset", "checking")
	}
	service := NewLedgerService(f.transactionService, f.accountService, NewInstitutionService(db.NewInstitutionRepository(f.database)))
	first, err := service.Overview(context.Background(), LedgerOverviewInput{OwnerUserID: 1, AsOf: "2026-10-15"})
	require.NoError(t, err)
	require.Len(t, first.Accounts, 100)
	require.NotEmpty(t, first.NextCursor)
	second, err := service.Overview(context.Background(), LedgerOverviewInput{OwnerUserID: 1, AsOf: first.AsOf, Cursor: first.NextCursor})
	require.NoError(t, err)
	assert.Empty(t, second.NextCursor)
	ids := map[int64]bool{}
	for _, row := range append(first.Accounts, second.Accounts...) {
		assert.False(t, ids[row.Account.ID])
		ids[row.Account.ID] = true
		// The fixture's holding has no parent, so it is its own row.
		assert.Equal(t, row.Account.ID == f.holdingAccountID, homeHoldingAccountKinds[row.Account.AccountKind])
	}
	assert.Len(t, ids, 122, "120 added accounts, the original cash account and the unparented holding")
	_, err = service.Overview(context.Background(), LedgerOverviewInput{OwnerUserID: 1, Cursor: "bad"})
	require.ErrorAs(t, err, new(ValidationError))
}

// A holding outside any ordinary parent is its own row (it cannot belong to
// another), so the index still shows the value Net worth counts. A non-cash
// child is linked, counted in value, and never presented as cash.
func TestLedgerOverviewShowsOrphanHoldingAndSeparatesNonCashChildren(t *testing.T) {
	t.Parallel()
	f := newHomeWidgetFixture(t)
	f.buyTen(t)
	f.price(t, f.stockCommodityID, "2026-10-15", "60")
	broker := f.account(t, "Broker", "asset", "brokerage")
	f.move(t, "2026-09-10", broker, f.eurCommodityID, 100000)
	receivable := f.account(t, "Broker receivable", "asset", "receivable")
	reviseHomeAccount(t, f.homeFixture, receivable, "Broker receivable", &broker)
	f.move(t, "2026-09-10", receivable, f.eurCommodityID, 25000)
	service := NewLedgerService(f.transactionService, f.accountService, NewInstitutionService(db.NewInstitutionRepository(f.database)))
	result, err := service.Overview(context.Background(), LedgerOverviewInput{OwnerUserID: 1, AsOf: "2026-10-15"})
	require.NoError(t, err)
	byID := map[int64]LedgerAccount{}
	for _, row := range result.Accounts {
		byID[row.Account.ID] = row
	}
	orphan, ok := byID[f.holdingAccountID]
	require.True(t, ok, "an unparented holding keeps a row")
	require.Len(t, orphan.Positions, 1)
	assert.Equal(t, f.holdingAccountID, orphan.Positions[0].HoldingAccountID)
	requireHomeCents(t, 60000, orphan.Value, "ten units at 60")
	_, listed := byID[receivable]
	assert.False(t, listed, "a brokerage child belongs to the brokerage row")
	row := byID[broker]
	assert.Empty(t, row.CashAccounts)
	require.Len(t, row.OtherAccounts, 1)
	assert.Equal(t, receivable, row.OtherAccounts[0].ID)
	requireAmount(t, row.Cash, f.eurCommodityID, 100000, 2, "the receivable is not cash")
	requireHomeCents(t, 125000, row.Value, "the receivable still counts in the row's value")
	assert.Empty(t, row.HoldingAccounts)
	assert.Empty(t, orphan.Cash, "a holding is never cash")
	// A top-level non-cash account reports its value but no cash.
	card := f.account(t, "Card", "liability", "credit_card")
	f.move(t, "2026-09-10", card, f.eurCommodityID, -3000)
	result, err = service.Overview(context.Background(), LedgerOverviewInput{OwnerUserID: 1, AsOf: "2026-10-15"})
	require.NoError(t, err)
	found := false
	for _, r := range result.Accounts {
		if r.Account.ID == card {
			found = true
			assert.Empty(t, r.Cash)
			requireAmount(t, r.Balances, f.eurCommodityID, -3000, 2, "its balance is still reported")
		}
	}
	assert.True(t, found, "the card has its own row")
}

// A nested brokerage is its own index row, so no amount is counted in two
// rows. One account's detail still covers its whole subtree and matches Net
// worth for the account and its descendants (#567 review).
func TestLedgerOverviewDetailCoversNestedBrokerageWhileIndexRowsPartition(t *testing.T) {
	t.Parallel()
	f := newHomeWidgetFixture(t)
	parentBroker := f.account(t, "Parent broker", "asset", "brokerage")
	f.move(t, "2026-09-10", parentBroker, f.eurCommodityID, 100000)
	childBroker := f.account(t, "Child broker", "asset", "brokerage")
	reviseHomeAccount(t, f.homeFixture, childBroker, "Child broker", &parentBroker)
	f.move(t, "2026-09-10", childBroker, f.eurCommodityID, 50000)
	childCash := f.account(t, "Child broker cash", "asset", "checking")
	reviseHomeAccount(t, f.homeFixture, childCash, "Child broker cash", &childBroker)
	f.move(t, "2026-09-10", childCash, f.eurCommodityID, 20000)
	service := NewLedgerService(f.transactionService, f.accountService, NewInstitutionService(db.NewInstitutionRepository(f.database)))
	index, err := service.Overview(context.Background(), LedgerOverviewInput{OwnerUserID: 1, AsOf: "2026-10-15"})
	require.NoError(t, err)
	byID := map[int64]LedgerAccount{}
	for _, row := range index.Accounts {
		byID[row.Account.ID] = row
	}
	requireHomeCents(t, 100000, byID[parentBroker].Value, "the parent row leaves the nested brokerage to its own row")
	requireHomeCents(t, 70000, byID[childBroker].Value, "the nested brokerage row holds its own subtree")
	_, listed := byID[childCash]
	assert.False(t, listed, "cash under the nested brokerage belongs to its row")

	detail, err := service.Overview(context.Background(), LedgerOverviewInput{OwnerUserID: 1, AsOf: "2026-10-15", AccountID: parentBroker})
	require.NoError(t, err)
	require.Len(t, detail.Accounts, 1)
	row := detail.Accounts[0]
	requireHomeCents(t, 170000, row.Value, "the parent's detail covers its whole subtree")
	requireAmount(t, row.Cash, f.eurCommodityID, 170000, 2, "a nested brokerage's cash and its cash children are cash")
	cash := []int64{}
	for _, account := range row.CashAccounts {
		cash = append(cash, account.ID)
	}
	assert.ElementsMatch(t, []int64{childBroker, childCash}, cash)
	report, err := f.transactionService.NetWorthSeries(context.Background(), NetWorthSeriesInput{StartDate: "2026-10-15", EndDate: "2026-10-15", Bucket: "day", Filters: ReportFilters{AccountIDs: []int64{parentBroker}, IncludeDescendants: true}, Reporting: &ReportingCurrencyInput{CommodityID: f.eurCommodityID}})
	require.NoError(t, err)
	require.Len(t, report.Buckets, 1)
	assert.Equal(t, report.Buckets[0].Converted, row.Value)

	// The nested brokerage's own detail is its index row.
	nested, err := service.Overview(context.Background(), LedgerOverviewInput{OwnerUserID: 1, AsOf: "2026-10-15", AccountID: childBroker})
	require.NoError(t, err)
	require.Len(t, nested.Accounts, 1)
	assert.Equal(t, byID[childBroker], nested.Accounts[0])
}
