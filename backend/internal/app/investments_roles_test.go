package app

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"rekenraam/backend/internal/exact"
)

// T-98. The investment commands carry an exemption from the guard that keeps
// ordinary postings out of subledger-managed holding accounts, because they
// write the lots those postings stand for. A command that spends the exemption
// on a posting it writes no lot for produces a position the journal and the
// subledger disagree about — ten shares in one, none in the other — with both
// halves passing their own checks.

func validBuy(f *investmentsTestFixture) InvestmentTradeInput {
	return InvestmentTradeInput{
		OwnerUserID: f.ownerUserID, TransactionDate: "2026-01-01",
		CommodityID: f.stockCommodityID, HoldingAccountID: f.holdingAccountID,
		CashAccountID: f.cashAccountID, QuantityValue: exact.New(10),
		CashAmountValue: 10000, CashAmountScale: 2, CashCommodityID: f.eurCommodityID,
	}
}

func TestBuyRejectsRoleCombinationsThatNoLotCanAccountFor(t *testing.T) {
	cases := []struct {
		name    string
		mutate  func(*testing.T, *investmentsTestFixture, *InvestmentTradeInput)
		message string
	}{
		{
			name: "holding account settles its own purchase",
			mutate: func(t *testing.T, f *investmentsTestFixture, in *InvestmentTradeInput) {
				in.CashAccountID = f.holdingAccountID
				in.CashCommodityID = f.stockCommodityID
			},
			message: "cash account cannot be a holding account managed by the investment subledger",
		},
		{
			name: "a different holding account settles the purchase",
			mutate: func(t *testing.T, f *investmentsTestFixture, in *InvestmentTradeInput) {
				in.CashAccountID = seedTestAccountWithClass(t, f.database, "active", true, "asset", "security_holding")
			},
			message: "cash account cannot be a holding account managed by the investment subledger",
		},
		{
			name: "an ordinary asset account holds the shares",
			mutate: func(t *testing.T, f *investmentsTestFixture, in *InvestmentTradeInput) {
				in.HoldingAccountID = f.cashAccountID
			},
			message: "holding account must be a holding account managed by the investment subledger",
		},
		{
			name: "income account settles the purchase",
			mutate: func(t *testing.T, f *investmentsTestFixture, in *InvestmentTradeInput) {
				in.CashAccountID = f.incomeAccountID
			},
			message: "cash account has the wrong account class for this role",
		},
		{
			name: "the security settles its own purchase",
			mutate: func(t *testing.T, f *investmentsTestFixture, in *InvestmentTradeInput) {
				in.CashCommodityID = f.stockCommodityID
			},
			message: "settlement commodity must be a currency",
		},
		{
			name: "a currency is bought as if it were an instrument",
			mutate: func(t *testing.T, f *investmentsTestFixture, in *InvestmentTradeInput) {
				in.CommodityID = f.eurCommodityID
			},
			message: "traded commodity must be an instrument, not a currency",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newInvestmentsTestFixture(t)
			ctx := context.Background()
			input := validBuy(f)
			tc.mutate(t, f, &input)

			_, err := f.investmentService.Buy(ctx, input)
			require.Error(t, err)
			require.EqualError(t, err, tc.message)

			// Nothing may survive a rejected trade: no lot, no journal half.
			lots, listErr := f.investmentService.ListLots(ctx, input.HoldingAccountID, input.CommodityID)
			require.NoError(t, listErr)
			require.Empty(t, lots)
		})
	}
}

func TestSellRejectsAHoldingAccountAsItsCashLeg(t *testing.T) {
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	buyOn(t, f, "2026-01-01", 10, 10000)

	sale := sellInput(f, "2026-02-01", 5)
	sale.CashAccountID = f.holdingAccountID
	_, err := f.investmentService.Sell(ctx, sale)
	require.EqualError(t, err, "cash account cannot be a holding account managed by the investment subledger")

	// The preview is a separate entry point and must refuse identically, so the
	// UI cannot show a plausible preview of a trade that will not commit.
	_, err = f.investmentService.PreviewSell(ctx, sale)
	require.EqualError(t, err, "cash account cannot be a holding account managed by the investment subledger")

	lots, err := f.investmentService.ListLots(ctx, f.holdingAccountID, f.stockCommodityID)
	require.NoError(t, err)
	require.Len(t, lots, 1)
	require.Equal(t, "10", lots[0].RemainingQuantityValue.String(), "the rejected sale disposed of nothing")
}

func TestWriteOffNeedsNoCashRoleAtAll(t *testing.T) {
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	buyOn(t, f, "2026-01-01", 10, 10000)

	_, err := f.investmentService.WriteOff(ctx, InvestmentWriteOffInput{
		OwnerUserID: f.ownerUserID, TransactionDate: "2026-02-01",
		CommodityID: f.stockCommodityID, HoldingAccountID: f.holdingAccountID,
		QuantityValue: exact.New(10), Reason: "delisted",
	})
	require.NoError(t, err, "a write-off has no cash leg, so it has no settlement role to satisfy")
}

func TestDividendRejectsAnIncomeAccountThatIsNotIncome(t *testing.T) {
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	holding := f.holdingAccountID

	_, err := f.investmentService.Dividend(ctx, DividendInput{
		OwnerUserID: f.ownerUserID, TransactionDate: "2026-02-01",
		CashAccountID: f.cashAccountID, CashCommodityID: f.eurCommodityID,
		IncomeAccountID: &holding, AmountValue: 5000, AmountScale: 2,
	})
	// A holding account is an asset, so the class rule catches it first — the
	// subledger rule behind it is what stops the asset accounts that are not
	// holdings.
	require.EqualError(t, err, "dividend income account has the wrong account class for this role")

	cash := f.cashAccountID
	_, err = f.investmentService.Dividend(ctx, DividendInput{
		OwnerUserID: f.ownerUserID, TransactionDate: "2026-02-01",
		CashAccountID: f.cashAccountID, CashCommodityID: f.eurCommodityID,
		IncomeAccountID: &cash, AmountValue: 5000, AmountScale: 2,
	})
	require.EqualError(t, err, "dividend income account has the wrong account class for this role")
}

func TestDividendRejectsAWithholdingAccountThatCannotHoldTax(t *testing.T) {
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	income := f.incomeAccountID
	value := int64(500)

	// Withholding into the income it was deducted from is the one place it can
	// never go; a liability (the usual "tax payable") is fine.
	withholding := f.incomeAccountID
	_, err := f.investmentService.Dividend(ctx, DividendInput{
		OwnerUserID: f.ownerUserID, TransactionDate: "2026-02-01",
		CashAccountID: f.cashAccountID, CashCommodityID: f.eurCommodityID,
		IncomeAccountID: &income, AmountValue: 5000, AmountScale: 2,
		WithholdingValue: &value, WithholdingAccountID: &withholding,
	})
	require.EqualError(t, err, "withholding account has the wrong account class for this role")

	withholding = seedTestAccountWithClass(t, f.database, "active", true, "liability", "other_liability")
	_, err = f.investmentService.Dividend(ctx, DividendInput{
		OwnerUserID: f.ownerUserID, TransactionDate: "2026-02-01",
		CashAccountID: f.cashAccountID, CashCommodityID: f.eurCommodityID,
		IncomeAccountID: &income, AmountValue: 5000, AmountScale: 2,
		WithholdingValue: &value, WithholdingAccountID: &withholding,
	})
	require.NoError(t, err)
}

func TestReinvestedDividendRejectsASecurityAsItsSettlementCommodity(t *testing.T) {
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	income := f.incomeAccountID

	_, err := f.investmentService.ReinvestedDividend(ctx, ReinvestedDividendInput{
		OwnerUserID: f.ownerUserID, TransactionDate: "2026-02-01",
		CommodityID: f.stockCommodityID, HoldingAccountID: f.holdingAccountID,
		IncomeAccountID: &income, QuantityValue: exact.New(1), QuantityScale: 0,
		AmountValue: 1000, AmountScale: 2, CashCommodityID: f.stockCommodityID,
	})
	require.EqualError(t, err, "settlement commodity must be a currency")
}

func TestValidTradesStillCommitUnderTheRoleRules(t *testing.T) {
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()

	buy, err := f.investmentService.Buy(ctx, validBuy(f))
	require.NoError(t, err)
	require.NotZero(t, buy.LotID)

	sale := sellInput(f, "2026-02-01", 5)
	_, err = f.investmentService.PreviewSell(ctx, sale)
	require.NoError(t, err)
	_, err = f.investmentService.Sell(ctx, sale)
	require.NoError(t, err)
}

func TestWriteOffPreviewRefusesTheSameRolesItsCommitWould(t *testing.T) {
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	buyOn(t, f, "2026-01-01", 10, 10000)

	// An ordinary asset account cannot hold the shares, and the preview has to
	// say so rather than showing a loss the commit will refuse to record.
	input := InvestmentWriteOffInput{
		OwnerUserID: f.ownerUserID, TransactionDate: "2026-02-01",
		CommodityID: f.stockCommodityID, HoldingAccountID: f.cashAccountID,
		QuantityValue: exact.New(10), Reason: "delisted",
	}
	_, err := f.investmentService.PreviewWriteOff(ctx, input)
	require.EqualError(t, err, "holding account must be a holding account managed by the investment subledger")
	_, err = f.investmentService.WriteOff(ctx, input)
	require.EqualError(t, err, "holding account must be a holding account managed by the investment subledger")
}
