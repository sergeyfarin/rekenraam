package app

import (
	"context"
	"fmt"
	"math/big"
	"math/rand/v2"
	"testing"

	"github.com/stretchr/testify/require"

	"rekenraam/backend/internal/db"
	"rekenraam/backend/internal/exact"
)

// Independent oracle: do not use production scale alignment to validate its
// results. big.Rat parses the decimal definition directly, without floats.
func financialRat(coefficient string, scale int) *big.Rat {
	value, ok := new(big.Rat).SetString(fmt.Sprintf("%se-%d", coefficient, scale))
	if !ok {
		panic("invalid test decimal")
	}
	return value
}

func requireFinancialAmount(t *testing.T, amounts []BalanceQuantity, commodity int64, want string) {
	t.Helper()
	got := new(big.Rat)
	for _, amount := range amounts {
		if amount.CommodityID == commodity {
			got.Add(got, financialRat(amount.QuantityValue.String(), amount.QuantityScale))
		}
	}
	expected, ok := new(big.Rat).SetString(want)
	require.True(t, ok)
	require.Zero(t, got.Cmp(expected), "commodity %d: got %s, want %s", commodity, got.RatString(), want)
}

// Gross measures need their own oracle. Adding an unrelated, balanced currency
// group must not change any measure, even with unusual signs, different account
// classes, shuffled postings or coefficients wider than int64.
func TestFinancialCashflowIgnoresEveryOutsideCommodityGroup(t *testing.T) {
	for _, class := range []string{"income", "expense", "asset", "liability", "equity"} {
		t.Run(class, func(t *testing.T) {
			for seed := uint64(1); seed <= 12; seed++ {
				rng := rand.New(rand.NewPCG(seed, 37))
				postings := []db.ReportCashflowPostingRecord{
					{AccountID: 1, CommodityID: 1, QuantityValue: exact.New(-12345), QuantityScale: 2, AccountClass: "asset"},
					{AccountID: 2, CommodityID: 1, QuantityValue: exact.New(123450), QuantityScale: 3, AccountClass: "expense"},
				}
				for c := int64(2); c <= 4; c++ {
					value := exact.MustParse("123456789012345678901234567890")
					if seed%2 == 0 {
						value = value.Negated()
					}
					postings = append(postings,
						db.ReportCashflowPostingRecord{AccountID: 10 + c, CommodityID: c, QuantityValue: value, QuantityScale: int(seed % 7), AccountClass: class},
						db.ReportCashflowPostingRecord{AccountID: 20 + c, CommodityID: c, QuantityValue: value.Negated(), QuantityScale: int(seed % 7), AccountClass: "asset"})
				}
				rng.Shuffle(len(postings), func(i, j int) { postings[i], postings[j] = postings[j], postings[i] })
				totals := newCashflowTotals(nil)
				classifyCashflowEntry(postings, map[int64]bool{1: true}, totals)
				b, err := totals.toBucket("2026-01-01", "2026-01-31")
				require.NoError(t, err)
				requireFinancialAmount(t, b.Outflow, 1, "123.45")
				requireFinancialAmount(t, b.OperatingNet, 1, "-123.45")
				requireFinancialAmount(t, b.NetMovement, 1, "-123.45")
				require.Empty(t, b.Inflow)
				require.Empty(t, b.TransferIn)
				require.Empty(t, b.TransferOut)
				for _, measure := range [][]BalanceQuantity{b.Outflow, b.OperatingNet, b.TransferNet, b.NetMovement} {
					for _, amount := range measure {
						require.Equal(t, int64(1), amount.CommodityID, "seed %d: unrelated commodity was classified", seed)
					}
				}
			}
		})
	}
}

// Known household facts, exercised through real write and report services.
// Expected gross numbers are stated from those facts, not reconstructed from
// the report's own operating_net + transfer_net identity.
func TestFinancialCashflowGrossAmountsFollowSelectedAccounts(t *testing.T) {
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	savings := seedTestAccountWithClass(t, f.database, "active", true, "asset", "savings")
	usd := seedTestCurrencyCommodity(t, f.database, "USD")
	usdCash := seedTestAccountWithClass(t, f.database, "active", true, "asset", "checking")
	expense := seedTestAccountWithClass(t, f.database, "active", true, "expense", "expense")
	trading, err := f.investmentService.repository.CommodityTradingAccountID(ctx, BookID)
	require.NoError(t, err)
	post := func(lines ...PostingInput) {
		_, err := f.transactionService.CreateTransaction(ctx, CreateTransactionInput{OwnerUserID: f.ownerUserID, OriginType: "browser_api", Spec: TransactionInput{TransactionDate: "2026-01-01", JournalEntries: []JournalEntryInput{{Postings: lines}}}})
		require.NoError(t, err)
	}
	line := func(account, commodity, value int64, scale int) PostingInput {
		return PostingInput{AccountID: account, CommodityID: commodity, QuantityValue: exact.New(value), QuantityScale: scale}
	}
	// Salary 1000; split purchase 30 + 20; refund 5; transfer 200 to savings.
	post(line(f.cashAccountID, f.eurCommodityID, 1000, 0), line(f.incomeAccountID, f.eurCommodityID, -100000, 2))
	post(line(f.cashAccountID, f.eurCommodityID, -50, 0), line(expense, f.eurCommodityID, 3000, 2), line(expense, f.eurCommodityID, 20000, 3))
	post(line(f.cashAccountID, f.eurCommodityID, 500, 2), line(expense, f.eurCommodityID, -5, 0))
	post(line(f.cashAccountID, f.eurCommodityID, -20000, 2), line(savings, f.eurCommodityID, 200, 0))
	// Real FX transfer: 100 EUR leaves and 110 USD arrives, separately balanced.
	post(line(f.cashAccountID, f.eurCommodityID, -10000, 2), line(trading, f.eurCommodityID, 10000, 2), line(usdCash, usd, 11000, 2), line(trading, usd, -11000, 2))
	buyOn(t, f, "2026-01-02", 10, 10000)
	sale := sellInput(f, "2026-01-03", 10)
	sale.CashAmountValue = 12000
	_, err = f.investmentService.Sell(ctx, sale)
	require.NoError(t, err)
	withheld, scale := int64(200), 2
	_, err = f.investmentService.Dividend(ctx, DividendInput{OwnerUserID: f.ownerUserID, TransactionDate: "2026-01-04", CashAccountID: f.cashAccountID, CashCommodityID: f.eurCommodityID, IncomeAccountID: &f.incomeAccountID, AmountValue: 10, AmountScale: 0, WithholdingAccountID: &expense, WithholdingValue: &withheld, WithholdingScale: &scale})
	require.NoError(t, err)
	for _, scope := range []struct {
		name                                  string
		ids                                   []int64
		in, out, transferIn, transferOut, net string
		dollars                               bool
	}{
		{"all liquid cash", nil, "1010", "47", "120", "200", "883", true},
		{"checking only", []int64{f.cashAccountID}, "1010", "47", "120", "400", "683", false},
		{"savings only", []int64{savings}, "0", "0", "200", "0", "200", false},
		{"USD only", []int64{usdCash}, "0", "0", "0", "0", "0", true},
	} {
		t.Run(scope.name, func(t *testing.T) {
			report, err := f.transactionService.Cashflow(ctx, CashflowInput{StartDate: "2026-01-01", EndDate: "2026-01-31", Bucket: "month", Filters: ReportFilters{AccountIDs: scope.ids}})
			require.NoError(t, err)
			require.Len(t, report.Buckets, 1)
			b := report.Buckets[0]
			for _, m := range []struct {
				got  []BalanceQuantity
				want string
			}{{b.Inflow, scope.in}, {b.Outflow, scope.out}, {b.TransferIn, scope.transferIn}, {b.TransferOut, scope.transferOut}, {b.NetMovement, scope.net}} {
				requireFinancialAmount(t, m.got, f.eurCommodityID, m.want)
				for _, a := range m.got {
					require.NotEqual(t, f.stockCommodityID, a.CommodityID)
				}
			}
			dollars := "0"
			if scope.dollars {
				dollars = "110"
			}
			requireFinancialAmount(t, b.TransferIn, usd, dollars)
			requireFinancialAmount(t, b.NetMovement, usd, dollars)
			requireFinancialAmount(t, b.Outflow, usd, "0")
			requireFinancialAmount(t, b.TransferOut, usd, "0")
			requireFinancialAmount(t, b.Inflow, usd, "0")
		})
	}
}

// A zero grand total is insufficient: errors may cancel across entries or
// currencies. These go through CreateTransaction and assert no durable residue.
func TestFinancialPostingBoundaryRejectsCancellationAcrossEntriesOrCommodities(t *testing.T) {
	for _, mode := range []string{"different entries", "different commodities", "different scales"} {
		t.Run(mode, func(t *testing.T) {
			f := newInvestmentsTestFixture(t)
			ctx := context.Background()
			spec := ordinaryEntry(f.cashAccountID, f.incomeAccountID, f.eurCommodityID, 100)
			switch mode {
			case "different entries":
				second := ordinaryEntry(f.cashAccountID, f.incomeAccountID, f.eurCommodityID, -100).JournalEntries[0]
				spec.JournalEntries[0].Postings[0].QuantityValue = exact.New(101)
				second.Postings[0].QuantityValue = exact.New(-101)
				spec.JournalEntries = append(spec.JournalEntries, second)
			case "different commodities":
				spec.JournalEntries[0].Postings[1].CommodityID = f.stockCommodityID
			case "different scales":
				spec.JournalEntries[0].Postings[0].QuantityScale = 1
			}
			before := snapshotFinancialState(t, f)
			_, err := f.transactionService.CreateTransaction(ctx, CreateTransactionInput{OwnerUserID: f.ownerUserID, OriginType: "browser_api", Spec: spec})
			var validation ValidationError
			require.ErrorAs(t, err, &validation)
			require.Contains(t, validation.Message, "not balanced")
			require.Equal(t, before, snapshotFinancialState(t, f))
		})
	}
}

func TestFinancialBalancedWideCoefficientsSurviveSplittingAndReordering(t *testing.T) {
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	// 30-digit money exceeds int64 and JS's exact integer range. Mixed scales and
	// split counterpart rows must preserve it all the way to the ledger read.
	amount := exact.MustParse("123456789012345678901234567890")
	half := exact.MustParse("61728394506172839450617283945")
	spec := ordinaryEntry(f.cashAccountID, f.incomeAccountID, f.eurCommodityID, 1)
	spec.JournalEntries[0].Postings = []PostingInput{
		{AccountID: f.incomeAccountID, CommodityID: f.eurCommodityID, QuantityValue: half.Negated(), QuantityScale: 2},
		{AccountID: f.cashAccountID, CommodityID: f.eurCommodityID, QuantityValue: exact.MustParse(amount.String() + "0"), QuantityScale: 3},
		{AccountID: f.incomeAccountID, CommodityID: f.eurCommodityID, QuantityValue: half.Negated(), QuantityScale: 2},
	}
	_, err := f.transactionService.CreateTransaction(ctx, CreateTransactionInput{OwnerUserID: f.ownerUserID, OriginType: "browser_api", Spec: spec})
	require.NoError(t, err)
	report, err := f.transactionService.Cashflow(ctx, CashflowInput{StartDate: "2026-01-01", EndDate: "2026-01-31", Bucket: "month"})
	require.NoError(t, err)
	require.Len(t, report.Buckets, 1)
	expected := "1234567890123456789012345678.90"
	requireFinancialAmount(t, report.Buckets[0].Inflow, f.eurCommodityID, expected)
	requireFinancialAmount(t, report.Buckets[0].NetMovement, f.eurCommodityID, expected)
}
