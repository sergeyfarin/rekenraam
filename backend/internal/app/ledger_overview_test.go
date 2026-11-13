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

func TestLedgerOverviewFollowsLargeCatalogAndNeverListsHoldings(t *testing.T) {
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
		assert.False(t, homeHoldingAccountKinds[row.Account.AccountKind])
	}
	assert.Len(t, ids, 121, "120 added accounts and the original cash account")
	_, err = service.Overview(context.Background(), LedgerOverviewInput{OwnerUserID: 1, Cursor: "bad"})
	require.ErrorAs(t, err, new(ValidationError))
}
