package app

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"rekenraam/backend/internal/exact"
)

// T-97. A lot used to carry one scale for both its acquisition record and its
// remaining balance, and that scale was however many decimal places the user
// happened to type. Every disposal rule then treated it as a hard constraint,
// so buying 10 shares as "10" made a later sale of half a share impossible
// while buying the identical position as "10.0" allowed it. The refusal was
// safe — nothing was corrupted — but it was also unactionable: the only escape
// was to have typed the purchase differently months earlier.
//
// Disposals now widen both sides to whichever carries more precision. Nothing
// narrows, so no quantity is rounded away to make the arithmetic line up, and
// the acquisition record never moves.

func buyAtScale(t *testing.T, f *investmentsTestFixture, date string, value int64, scale int, cashValue int64) InvestmentTradeResult {
	t.Helper()
	result, err := f.investmentService.Buy(context.Background(), InvestmentTradeInput{
		OwnerUserID: f.ownerUserID, TransactionDate: date, CommodityID: f.stockCommodityID,
		HoldingAccountID: f.holdingAccountID, CashAccountID: f.cashAccountID,
		QuantityValue: exact.New(value), QuantityScale: scale,
		CashAmountValue: cashValue, CashAmountScale: 2, CashCommodityID: f.eurCommodityID,
	})
	require.NoError(t, err)
	return result
}

// The reported case, across every method: buy 10 whole shares, sell half a
// share. The commodity permits six decimal places, so the sale was always
// within the position's precision.
func TestSellHalfAShareFromAWholeShareLotUnderEveryMethod(t *testing.T) {
	for _, method := range []string{"fifo", "lifo", "average_cost", "specific_lot"} {
		t.Run(method, func(t *testing.T) {
			f := newInvestmentsTestFixture(t)
			ctx := context.Background()

			bought := buyAtScale(t, f, "2026-01-01", 10, 0, 10000)

			sale := InvestmentTradeInput{
				OwnerUserID: f.ownerUserID, TransactionDate: "2026-02-01", CommodityID: f.stockCommodityID,
				HoldingAccountID: f.holdingAccountID, CashAccountID: f.cashAccountID,
				QuantityValue: exact.New(5), QuantityScale: 1, // 0.5 shares
				CashAmountValue: 600, CashAmountScale: 2, CashCommodityID: f.eurCommodityID,
				CostBasisMethod: method,
			}
			if method == "specific_lot" {
				sale.LotAllocations = []InvestmentLotAllocationInput{
					{LotID: *bought.LotID, QuantityValue: exact.New(5), QuantityScale: 1},
				}
			}

			_, err := f.investmentService.PreviewSell(ctx, sale)
			require.NoError(t, err, "preview must offer the sale the commit will accept")

			sold, err := f.investmentService.Sell(ctx, sale)
			require.NoError(t, err, "half a share is within the commodity's precision")

			// 0.5 of a 10-share lot costing 100.00 carries 5.00 of basis.
			disposedBasis := exact.NewScaledInt()
			for _, allocation := range sold.Allocations {
				disposedBasis.AddInt64(allocation.CostBasisValue, allocation.CostBasisScale)
			}
			require.Equal(t, 0, disposedBasis.Cmp(exact.ScaledIntFromInt64(500, 2)))

			lots, err := f.investmentService.ListLots(ctx, f.holdingAccountID, f.stockCommodityID)
			require.NoError(t, err)
			require.Len(t, lots, 1)
			require.Equal(t, "open", lots[0].Status)

			remaining := exact.NewScaledInt()
			remaining.AddCoefficient(lots[0].RemainingQuantityValue, lots[0].RemainingQuantityScale)
			require.Equal(t, 0, remaining.Cmp(exact.ScaledIntFromInt64(95, 1)), "9.5 shares left")

			remainingBasis := exact.NewScaledInt()
			remainingBasis.AddInt64(lots[0].RemainingCostBasisValue, lots[0].RemainingCostBasisScale)
			require.Equal(t, 0, remainingBasis.Cmp(exact.ScaledIntFromInt64(9500, 2)))

			// The acquisition record is evidence, not a projection: it still
			// says what was bought, in the form it was entered.
			require.Equal(t, "10", lots[0].QuantityValue.String())
			require.Equal(t, 0, lots[0].QuantityScale)
		})
	}
}

// How a purchase was typed must not decide anything about what follows. The
// same position entered two ways has to behave identically.
func TestPurchaseTextPrecisionDoesNotChangeWhatCanBeSold(t *testing.T) {
	sell := func(t *testing.T, buyValue int64, buyScale int) InvestmentTradeResult {
		t.Helper()
		f := newInvestmentsTestFixture(t)
		buyAtScale(t, f, "2026-01-01", buyValue, buyScale, 10000)
		sold, err := f.investmentService.Sell(context.Background(), InvestmentTradeInput{
			OwnerUserID: f.ownerUserID, TransactionDate: "2026-02-01", CommodityID: f.stockCommodityID,
			HoldingAccountID: f.holdingAccountID, CashAccountID: f.cashAccountID,
			QuantityValue: exact.New(5), QuantityScale: 1,
			CashAmountValue: 600, CashAmountScale: 2, CashCommodityID: f.eurCommodityID,
		})
		require.NoError(t, err)
		return sold
	}

	typedWhole := sell(t, 10, 0)     // "10"
	typedDecimal := sell(t, 1000, 2) // "10.00"

	wholeBasis := exact.NewScaledInt()
	for _, allocation := range typedWhole.Allocations {
		wholeBasis.AddInt64(allocation.CostBasisValue, allocation.CostBasisScale)
	}
	decimalBasis := exact.NewScaledInt()
	for _, allocation := range typedDecimal.Allocations {
		decimalBasis.AddInt64(allocation.CostBasisValue, allocation.CostBasisScale)
	}
	require.Equal(t, 0, wholeBasis.Cmp(decimalBasis), "the same position sold the same way must cost the same basis")
}

// Selling the whole lot in fractional pieces has to close it exactly, with no
// dust left behind by the widening.
func TestFractionalSalesDrainALotExactly(t *testing.T) {
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()

	buyAtScale(t, f, "2026-01-01", 1, 0, 10000)

	for i := 0; i < 4; i++ {
		_, err := f.investmentService.Sell(ctx, InvestmentTradeInput{
			OwnerUserID: f.ownerUserID, TransactionDate: "2026-02-01", CommodityID: f.stockCommodityID,
			HoldingAccountID: f.holdingAccountID, CashAccountID: f.cashAccountID,
			QuantityValue: exact.New(25), QuantityScale: 2, // 0.25 shares
			CashAmountValue: 3000, CashAmountScale: 2, CashCommodityID: f.eurCommodityID,
		})
		require.NoError(t, err, "quarter %d", i+1)
	}

	lots, err := f.investmentService.ListLots(ctx, f.holdingAccountID, f.stockCommodityID)
	require.NoError(t, err)
	require.Len(t, lots, 1)
	require.Equal(t, "closed", lots[0].Status, "four quarters is the whole share")

	remaining := exact.NewScaledInt()
	remaining.AddCoefficient(lots[0].RemainingQuantityValue, lots[0].RemainingQuantityScale)
	require.Equal(t, 0, remaining.Sign())
	require.Zero(t, lots[0].RemainingCostBasisValue, "a drained lot carries no residual basis")
}

// A sale finer than the position holds is still a shortfall, not a rounding
// opportunity. Widening must not turn "you do not have that" into a silent
// truncation.
func TestSellRejectsMoreThanTheLotHoldsAtAnyScale(t *testing.T) {
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()

	buyAtScale(t, f, "2026-01-01", 1, 0, 10000)

	_, err := f.investmentService.Sell(ctx, InvestmentTradeInput{
		OwnerUserID: f.ownerUserID, TransactionDate: "2026-02-01", CommodityID: f.stockCommodityID,
		HoldingAccountID: f.holdingAccountID, CashAccountID: f.cashAccountID,
		QuantityValue: exact.New(1001), QuantityScale: 3, // 1.001 shares
		CashAmountValue: 12000, CashAmountScale: 2, CashCommodityID: f.eurCommodityID,
	})
	require.ErrorIs(t, err, ErrInvestmentLotsInsufficient)
}
