package app

import (
	"context"
	"database/sql"
	"testing"

	"github.com/stretchr/testify/require"

	"rekenraam/backend/internal/db"
	"rekenraam/backend/internal/exact"
)

// T-102. Cashflow asked "does this journal entry touch a selected cash
// account?" once for the whole entry, then classified every other posting in
// it. A journal entry balances separately in each commodity, though, so one
// entry can carry movements that have nothing to do with each other: a share
// purchase is euros leaving the cash account for the trading account, and
// shares arriving from it. Only the euro pair crosses the cash boundary. The
// share pair was classified anyway, and a 100 EUR purchase reported ten shares
// transferred in and 100 EUR *and* ten shares out.
//
// Net movement stayed right the whole time — the spurious legs balance — which
// is why the report's own arithmetic identity could not detect it. These tests
// assert the gross measures, not just the net.

func amountFor(t *testing.T, amounts []BalanceQuantity, commodityID int64) (BalanceQuantity, bool) {
	t.Helper()
	for _, amount := range amounts {
		if amount.CommodityID == commodityID {
			return amount, true
		}
	}
	return BalanceQuantity{}, false
}

func requireNoCommodity(t *testing.T, amounts []BalanceQuantity, commodityID int64, measure string) {
	t.Helper()
	_, found := amountFor(t, amounts, commodityID)
	require.False(t, found, "%s must not name a commodity that never entered or left the cash scope", measure)
}

func TestCashflowExcludesTheSecurityLegsOfABuy(t *testing.T) {
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	buyOn(t, f, "2026-01-01", 10, 10000)

	report, err := f.transactionService.Cashflow(ctx, CashflowInput{
		StartDate: "2026-01-01", EndDate: "2026-01-31", Bucket: "month",
	})
	require.NoError(t, err)
	require.Len(t, report.Buckets, 1)
	bucket := report.Buckets[0]

	requireNoCommodity(t, bucket.TransferIn, f.stockCommodityID, "transfer in")
	requireNoCommodity(t, bucket.TransferOut, f.stockCommodityID, "transfer out")
	require.Empty(t, bucket.TransferIn, "buying shares brings nothing into cash")

	// The euro counterpart stays: it is what explains the cash actually leaving.
	out, found := amountFor(t, bucket.TransferOut, f.eurCommodityID)
	require.True(t, found)
	require.Zero(t, out.QuantityValue.Cmp(exact.New(10000)))
	require.Equal(t, 2, out.QuantityScale)

	net, found := amountFor(t, bucket.NetMovement, f.eurCommodityID)
	require.True(t, found)
	require.Zero(t, net.QuantityValue.Cmp(exact.New(-10000)), "net movement was already correct and stays correct")
}

func TestCashflowExcludesTheSecurityLegsOfASell(t *testing.T) {
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	buyOn(t, f, "2026-01-01", 10, 10000)
	sale := sellInput(f, "2026-02-01", 10)
	sale.CashAmountValue = 12000
	_, err := f.investmentService.Sell(ctx, sale)
	require.NoError(t, err)

	report, err := f.transactionService.Cashflow(ctx, CashflowInput{
		StartDate: "2026-02-01", EndDate: "2026-02-28", Bucket: "month",
	})
	require.NoError(t, err)
	require.Len(t, report.Buckets, 1)
	bucket := report.Buckets[0]

	requireNoCommodity(t, bucket.TransferIn, f.stockCommodityID, "transfer in")
	requireNoCommodity(t, bucket.TransferOut, f.stockCommodityID, "transfer out")
	in, found := amountFor(t, bucket.TransferIn, f.eurCommodityID)
	require.True(t, found)
	require.Zero(t, in.QuantityValue.Cmp(exact.New(12000)), "the proceeds are what arrived in cash")
	require.Empty(t, bucket.TransferOut)
}

// TestCashflowReportingTotalsDropTheSecurityLegs is the same buy seen through a
// reporting currency, where the mistake was not merely a stray commodity but a
// wrong euro number: the shares were valued at the buy-implied price and added
// to both gross measures, turning 0 in / 100 out into 100 in / 200 out.
func TestCashflowReportingTotalsDropTheSecurityLegs(t *testing.T) {
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	buyOn(t, f, "2026-01-01", 10, 10000)
	f.transactionService.SetPricingRepository(db.NewPricingRepository(f.database))

	report, err := f.transactionService.Cashflow(ctx, CashflowInput{
		StartDate: "2026-01-01", EndDate: "2026-01-31", Bucket: "month",
		Reporting: &ReportingCurrencyInput{CommodityID: f.eurCommodityID, MaxStalenessDay: 30},
	})
	require.NoError(t, err)
	require.Len(t, report.Buckets, 1)
	bucket := report.Buckets[0]

	require.NotNil(t, bucket.ConvertedTransferIn)
	require.Zero(t, bucket.ConvertedTransferIn.QuantityValue.Sign(), "no euros of value entered cash")
	require.NotNil(t, bucket.ConvertedTransferOut)
	require.Zero(t, bucket.ConvertedTransferOut.QuantityValue.Cmp(exact.New(10000)))
	require.NotNil(t, bucket.ConvertedNetMovement)
	require.Zero(t, bucket.ConvertedNetMovement.QuantityValue.Cmp(exact.New(-10000)))
}

// TestCashflowStillClassifiesEveryCommodityThatTouchesCash is the guard against
// over-narrowing. A cross-currency movement is two commodity groups that each
// touch a selected cash account, and both must still be classified — the rule
// is "the group touches cash", not "the group is the entry's first commodity".
func TestCashflowStillClassifiesEveryCommodityThatTouchesCash(t *testing.T) {
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	usdCommodityID := seedTestCurrencyCommodity(t, f.database, "USD")
	usdCashID := seedTestAccountWithClass(t, f.database, "active", true, "asset", "checking")

	// One entry, two balanced commodity groups, each with a cash leg and an
	// equity counterpart outside the cash scope.
	_, err := f.transactionService.CreateTransaction(ctx, CreateTransactionInput{
		OwnerUserID: f.ownerUserID, OriginType: "browser_api",
		Spec: TransactionInput{
			TransactionDate: "2026-03-01",
			JournalEntries: []JournalEntryInput{{Postings: []PostingInput{
				{AccountID: f.cashAccountID, CommodityID: f.eurCommodityID, QuantityValue: exact.New(-10000), QuantityScale: 2},
				{AccountID: f.incomeAccountID, CommodityID: f.eurCommodityID, QuantityValue: exact.New(10000), QuantityScale: 2},
				{AccountID: usdCashID, CommodityID: usdCommodityID, QuantityValue: exact.New(11000), QuantityScale: 2},
				{AccountID: f.incomeAccountID, CommodityID: usdCommodityID, QuantityValue: exact.New(-11000), QuantityScale: 2},
			}}},
		},
	})
	require.NoError(t, err)

	report, err := f.transactionService.Cashflow(ctx, CashflowInput{
		StartDate: "2026-03-01", EndDate: "2026-03-31", Bucket: "month",
	})
	require.NoError(t, err)
	require.Len(t, report.Buckets, 1)
	bucket := report.Buckets[0]

	// Both counterparts are income-class, so both are classified — the euro one
	// as a negative inflow, the dollar one as a positive inflow. What matters
	// here is that neither commodity group was skipped.
	_, foundEUR := amountFor(t, bucket.Inflow, f.eurCommodityID)
	require.True(t, foundEUR, "the euro group touches cash and must still be classified")
	_, foundUSD := amountFor(t, bucket.Inflow, usdCommodityID)
	require.True(t, foundUSD, "the dollar group touches cash and must still be classified")
}

// seedTestCurrencyCommodity adds a second currency to the fixture book, which
// otherwise has only EUR and the test security.
func seedTestCurrencyCommodity(t *testing.T, database *sql.DB, code string) int64 {
	t.Helper()
	ctx := context.Background()
	result, err := database.ExecContext(ctx, `
		INSERT INTO commodities (book_id, code, kind, is_builtin, created_at, created_by_user_id)
		VALUES (?, ?, 'currency', 1, '2026-01-01T00:00:00Z', 1)
	`, BookID, code)
	require.NoError(t, err)
	commodityID, err := result.LastInsertId()
	require.NoError(t, err)
	_, err = database.ExecContext(ctx, `
		INSERT INTO commodity_versions (
			commodity_id, version_seq, effective_from, recorded_at, changed_by_user_id,
			change_reason, status, symbol, display_symbol, name, standard_scale, max_quantity_scale
		) VALUES (?, 1, '2026-01-01', '2026-01-01T00:00:00Z', 1, 'test fixture', 'active', ?, ?, ?, 2, 6)
	`, commodityID, code, code, code)
	require.NoError(t, err)
	return commodityID
}
