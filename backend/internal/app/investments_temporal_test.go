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
	assertMoneyValue(t, 10000, 2, sold.Allocations[0].CostBasisValue, sold.Allocations[0].CostBasisScale, "basis comes from the January acquisition, not June's")

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
	assertMoneyValue(t, 10000, 2, sold.Allocations[0].CostBasisValue, sold.Allocations[0].CostBasisScale)
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

// T-95, second pass. Filtering by opened_on keeps a disposal from consuming a
// lot that did not exist yet, but it does not make the lot's *remaining* basis
// historical. A later average-cost sale has already redistributed pooled basis
// across the survivors, so an older lot that passes the date filter can hand a
// backdated sale basis that only exists because of purchases and sales that
// came after it. The projection is only true as of the last disposal applied
// to it, and events dated before that one are now refused outright.

func TestBackdatedSaleIsRefusedOnceALaterSaleHasPooledTheBasis(t *testing.T) {
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()

	buyOn(t, f, "2026-01-01", 10, 10000) // 10 shares at 10.00
	buyOn(t, f, "2026-06-01", 10, 30000) // 10 shares at 30.00

	later := sellInput(f, "2026-07-01", 5)
	later.CostBasisMethod = "average_cost"
	_, err := f.investmentService.Sell(ctx, later)
	require.NoError(t, err)

	// January's surviving 5 shares now carry 100.00 of pooled basis rather than
	// the 50.00 they were bought for, so a March sale selecting that lot would
	// take basis created by a June purchase and a July sale.
	earlier := sellInput(f, "2026-03-01", 5)
	earlier.CostBasisMethod = "average_cost"

	_, err = f.investmentService.PreviewSell(ctx, earlier)
	require.EqualError(t, err, "investment events must be entered in chronological order: a disposal dated 2026-03-01 is before this position's disposal on 2026-07-01")

	_, err = f.investmentService.Sell(ctx, earlier)
	require.EqualError(t, err, "investment events must be entered in chronological order: a disposal dated 2026-03-01 is before this position's disposal on 2026-07-01")

	// The refusal left the position exactly as the July sale did.
	positions, err := f.investmentService.Positions(ctx)
	require.NoError(t, err)
	require.Len(t, positions, 1)
	require.Equal(t, "15", positions[0].QuantityValue.String())
}

func TestBackdatedWriteOffIsRefusedAfterASale(t *testing.T) {
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	buyOn(t, f, "2026-01-01", 10, 10000)
	_, err := f.investmentService.Sell(ctx, sellInput(f, "2026-07-01", 5))
	require.NoError(t, err)

	input := InvestmentWriteOffInput{
		OwnerUserID: f.ownerUserID, TransactionDate: "2026-03-01", CommodityID: f.stockCommodityID,
		HoldingAccountID: f.holdingAccountID, QuantityValue: exact.New(1),
		Reason: "delisted", ChangeReason: "delisted",
	}
	_, err = f.investmentService.PreviewWriteOff(ctx, input)
	require.ErrorContains(t, err, "chronological order")
	_, err = f.investmentService.WriteOff(ctx, input)
	require.ErrorContains(t, err, "chronological order")
}

func TestBackdatedPurchaseIsRefusedAfterASale(t *testing.T) {
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	buyOn(t, f, "2026-01-01", 10, 10000)
	_, err := f.investmentService.Sell(ctx, sellInput(f, "2026-07-01", 5))
	require.NoError(t, err)

	// The July sale priced a pool this lot was not in, and nothing recomputes
	// it, so the lot cannot be slipped in behind the sale.
	_, err = f.investmentService.Buy(ctx, InvestmentTradeInput{
		OwnerUserID: f.ownerUserID, TransactionDate: "2026-03-01",
		CommodityID: f.stockCommodityID, HoldingAccountID: f.holdingAccountID,
		CashAccountID: f.cashAccountID, QuantityValue: exact.New(10),
		CashAmountValue: 30000, CashAmountScale: 2, CashCommodityID: f.eurCommodityID,
	})
	require.EqualError(t, err, "investment events must be entered in chronological order: an acquisition dated 2026-03-01 is before this position's disposal on 2026-07-01")

	lots, err := f.investmentService.ListLots(ctx, f.holdingAccountID, f.stockCommodityID)
	require.NoError(t, err)
	require.Len(t, lots, 1, "the refused purchase created no lot")
}

func TestBackdatedSaleStillWorksWhenOnlyPurchasesFollowIt(t *testing.T) {
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	buyOn(t, f, "2026-01-01", 10, 10000)
	buyOn(t, f, "2026-06-01", 10, 30000)

	// Nothing has rewritten the projection yet: the June lot's basis is still
	// exactly what was paid for it, and the opened_on filter keeps it out of a
	// March sale's pool. So this is computable, and it stays allowed.
	sale := sellInput(f, "2026-03-01", 5)
	sale.CostBasisMethod = "average_cost"
	preview, err := f.investmentService.PreviewSell(ctx, sale)
	require.NoError(t, err)
	require.Len(t, preview.Allocations, 1)
	assertMoneyValue(t, 5000, 2, preview.Allocations[0].CostBasisValue, preview.Allocations[0].CostBasisScale, "5 shares of the 10.00 January lot")

	_, err = f.investmentService.Sell(ctx, sale)
	require.NoError(t, err)
}

func TestSameDayEventsStayLegalInEntryOrder(t *testing.T) {
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	buyOn(t, f, "2026-01-01", 10, 10000)

	// Buy and sell on one day, then sell again on that same day: the guard is
	// inclusive, so a day's own events are ordered by entry, not refused.
	_, err := f.investmentService.Sell(ctx, sellInput(f, "2026-01-01", 4))
	require.NoError(t, err)
	_, err = f.investmentService.Sell(ctx, sellInput(f, "2026-01-01", 3))
	require.NoError(t, err)

	positions, err := f.investmentService.Positions(ctx)
	require.NoError(t, err)
	require.Len(t, positions, 1)
	require.Equal(t, "3", positions[0].QuantityValue.String())
}
