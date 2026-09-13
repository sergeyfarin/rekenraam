package app

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"rekenraam/backend/internal/exact"
)

// T-95. A disposal used to consume whatever lots were open right now, with no
// regard for when they were acquired: selling in May happily consumed a lot
// bought in June. Two things were wrong with that at once. The position went
// negative through history — the register said the shares were held in May when
// they were not — and the sale's cost basis came from an acquisition that had
// not happened yet, so every realized gain computed from it was fiction. Both
// survived because no test ever dated a disposal before its acquisition.
//
// These tests pin the rule that fixes it: a disposal may only consume lots
// whose opened_on is on or before its own event date.

func buyOn(t *testing.T, f *investmentsTestFixture, date string, quantity int64, cashValue int64) InvestmentTradeResult {
	t.Helper()
	result, err := f.investmentService.Buy(context.Background(), InvestmentTradeInput{
		OwnerUserID: f.ownerUserID, TransactionDate: date, CommodityID: f.stockCommodityID,
		HoldingAccountID: f.holdingAccountID, CashAccountID: f.cashAccountID,
		QuantityValue: exact.New(quantity), CashAmountValue: cashValue, CashAmountScale: 2,
		CashCommodityID: f.eurCommodityID,
	})
	require.NoError(t, err)
	return result
}

func sellInput(f *investmentsTestFixture, date string, quantity int64) InvestmentTradeInput {
	return InvestmentTradeInput{
		OwnerUserID: f.ownerUserID, TransactionDate: date, CommodityID: f.stockCommodityID,
		HoldingAccountID: f.holdingAccountID, CashAccountID: f.cashAccountID,
		QuantityValue: exact.New(quantity), CashAmountValue: 10000, CashAmountScale: 2,
		CashCommodityID: f.eurCommodityID,
	}
}

func TestSellRejectsLotsAcquiredAfterTheSaleDateUnderEveryMethod(t *testing.T) {
	for _, method := range []string{"fifo", "lifo", "average_cost", "specific_lot"} {
		t.Run(method, func(t *testing.T) {
			f := newInvestmentsTestFixture(t)
			ctx := context.Background()

			bought := buyOn(t, f, "2026-06-01", 10, 10000)

			input := sellInput(f, "2026-05-01", 10)
			input.CostBasisMethod = method
			if method == "specific_lot" {
				input.LotAllocations = []InvestmentLotAllocationInput{{LotID: *bought.LotID, QuantityValue: exact.New(10)}}
			}

			// The preview has to refuse too. A preview that succeeds and a
			// commit that fails is its own defect: the user is shown a
			// realized gain for a sale the app will not accept.
			_, err := f.investmentService.PreviewSell(ctx, input)
			require.Error(t, err, "preview must refuse a sale dated before the acquisition")

			_, err = f.investmentService.Sell(ctx, input)
			require.Error(t, err, "sale dated before the acquisition must be refused")

			// Nothing partial was left behind by the refusal.
			lots, err := f.investmentService.ListLots(ctx, f.holdingAccountID, f.stockCommodityID)
			require.NoError(t, err)
			require.Len(t, lots, 1)
			require.Equal(t, "open", lots[0].Status)
			require.Equal(t, "10", lots[0].RemainingQuantityValue.String(), "the June lot is untouched")
		})
	}
}

// The boundary is inclusive: buying and selling on the same day is ordinary
// broker activity, not an out-of-order entry. A fix that used `<` instead of
// `<=` would break every day trade and every same-day import.
func TestSellAllowsAcquisitionAndDisposalOnTheSameDay(t *testing.T) {
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()

	buyOn(t, f, "2026-06-01", 10, 10000)

	result, err := f.investmentService.Sell(ctx, sellInput(f, "2026-06-01", 10))
	require.NoError(t, err, "same-day disposal is legitimate")
	require.Len(t, result.Allocations, 1)
}

// Temporal eligibility is about which lots a method may *choose*, not only
// about outright rejection. With one eligible lot and one future lot, FIFO must
// consume the eligible one and must refuse to reach into the future lot for the
// remainder — reporting insufficient lots rather than quietly overdrawing.
func TestSellSelectsOnlyLotsHeldOnTheSaleDate(t *testing.T) {
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()

	january := buyOn(t, f, "2026-01-01", 10, 10000)
	buyOn(t, f, "2026-06-01", 10, 30000)

	// 10 of the 20 shares now open were not held in March.
	sold, err := f.investmentService.Sell(ctx, sellInput(f, "2026-03-01", 10))
	require.NoError(t, err)
	require.Len(t, sold.Allocations, 1)
	require.Equal(t, *january.LotID, sold.Allocations[0].LotID, "only the January lot was held in March")
	require.Equal(t, int64(10000), sold.Allocations[0].CostBasisValue, "basis comes from the January acquisition, not June's")

	// And the position cannot be overdrawn into the future lot.
	_, err = f.investmentService.Sell(ctx, sellInput(f, "2026-03-01", 5))
	require.ErrorIs(t, err, ErrInvestmentLotsInsufficient, "the June lot is not available to a March sale")
}

// LIFO is the method a future lot distorts without going negative: the newest
// lot wins, so an ineligible later acquisition silently supplies the basis for
// an earlier sale while the quantities still look correct.
func TestSellUnderLIFOIgnoresLotsAcquiredAfterTheSaleDate(t *testing.T) {
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()

	january := buyOn(t, f, "2026-01-01", 10, 10000)
	buyOn(t, f, "2026-06-01", 10, 30000)

	input := sellInput(f, "2026-03-01", 10)
	input.CostBasisMethod = "lifo"
	sold, err := f.investmentService.Sell(ctx, input)
	require.NoError(t, err)
	require.Len(t, sold.Allocations, 1)
	require.Equal(t, *january.LotID, sold.Allocations[0].LotID,
		"LIFO's newest lot must be the newest one actually held on the sale date")
	require.Equal(t, int64(10000), sold.Allocations[0].CostBasisValue)
}

// A specific-lot disposal names its lot outright, so it never passes through
// the selection queries. It needs its own refusal, and that refusal should say
// what is wrong rather than reporting a shortfall the user can see they do not
// have.
func TestSellWithSpecificLotRejectsAnAllocationDatedAfterTheSale(t *testing.T) {
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()

	buyOn(t, f, "2026-01-01", 10, 10000)
	june := buyOn(t, f, "2026-06-01", 10, 30000)

	input := sellInput(f, "2026-03-01", 10)
	input.CostBasisMethod = "specific_lot"
	input.LotAllocations = []InvestmentLotAllocationInput{{LotID: *june.LotID, QuantityValue: exact.New(10)}}

	_, err := f.investmentService.Sell(ctx, input)
	var validation ValidationError
	require.ErrorAs(t, err, &validation, "naming a future lot is a bad request, not a shortfall")
	require.Contains(t, validation.Message, "after the disposal date")
}

// A write-off is a disposal with zero proceeds and shares the disposal engine,
// so it inherits the same rule. It has its own request type and its own handler
// (T-38), which is exactly the shape of thing that gets fixed on four paths and
// missed on the fifth.
func TestWriteOffRejectsLotsAcquiredAfterTheWriteOffDate(t *testing.T) {
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()

	buyOn(t, f, "2026-06-01", 10, 10000)

	input := InvestmentWriteOffInput{
		OwnerUserID: f.ownerUserID, TransactionDate: "2026-05-01", CommodityID: f.stockCommodityID,
		HoldingAccountID: f.holdingAccountID, QuantityValue: exact.New(10),
		Reason: "delisted", ChangeReason: "delisted",
	}

	_, err := f.investmentService.PreviewWriteOff(ctx, input)
	require.Error(t, err, "preview must refuse a write-off dated before the acquisition")

	_, err = f.investmentService.WriteOff(ctx, input)
	require.Error(t, err, "write-off dated before the acquisition must be refused")
}
