package app

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"rekenraam/backend/internal/db"
	"rekenraam/backend/internal/exact"
)

// #182: a known-basis acquisition into a position that already holds an
// unknown-basis lot is admitted. Its range guard checks the known subtotal,
// the position reads as unknown basis, and the new lot keeps its exact basis.

// requireMixedPositionAfterKnownAcquisition checks the admitted outcome: one
// unknown-basis position, a known lot at its exact basis, and a LIFO sale of
// only that lot that stays known.
func requireMixedPositionAfterKnownAcquisition(t *testing.T, f *investmentsTestFixture, knownLotID int64, quantity int64, basis string, saleDate string) {
	t.Helper()
	ctx := context.Background()
	positions, err := f.investmentService.Positions(ctx)
	require.NoError(t, err)
	require.Len(t, positions, 1)
	require.Equal(t, db.InvestmentBasisUnknown, positions[0].BasisKnowledge, "any unknown lot makes the position unknown")
	lotQuantity, lotBasis, knowledge := lotStateByID(t, f, knownLotID)
	require.Equal(t, db.InvestmentBasisKnown, knowledge)
	require.Equal(t, exact.New(quantity).String(), lotQuantity)
	require.True(t, lotBasis.Valid)
	require.Equal(t, basis, lotBasis.String, "the known lot keeps its exact basis")

	sale := sellInput(f, saleDate, 1)
	sale.CostBasisMethod = "specific_lot"
	sale.LotAllocations = []InvestmentLotAllocationInput{{LotID: knownLotID, QuantityValue: exact.New(1)}}
	_, err = f.investmentService.Sell(ctx, sale)
	require.NoError(t, err)
	disposal := latestDisposal(t, f)
	require.Equal(t, db.InvestmentBasisKnown, disposal.knowledge, "a sale of only known lots stays known")
	require.Len(t, disposal.allocations, 1)
	requireHealthyUnresolvedBook(t, f)
}

func TestKnownBuyBesideAnUnknownBasisLotPostsInEitherOrder(t *testing.T) {
	t.Parallel()
	for _, unknownFirst := range []bool{true, false} {
		name := "known then unknown"
		if unknownFirst {
			name = "unknown then known"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f := newInvestmentsTestFixture(t)
			ctx := context.Background()
			seedExternalTransferEquity(t, f.database)
			transfer := func() {
				_, err := f.investmentService.ExternalTransferIn(ctx, unknownTransferInInput(f, "2026-01-15", "2020-03-01"))
				require.NoError(t, err)
			}
			var bought InvestmentTradeResult
			if unknownFirst {
				transfer()
				bought = buyOn(t, f, "2026-02-01", 10, 10000)
			} else {
				bought = buyOn(t, f, "2026-01-01", 10, 10000)
				transfer()
			}
			requireMixedPositionAfterKnownAcquisition(t, f, *bought.LotID, 10, "10000", "2026-03-01")
		})
	}
}

func TestReinvestedDividendBesideAnUnknownBasisLotPosts(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	seedExternalTransferEquity(t, f.database)
	_, err := f.investmentService.ExternalTransferIn(ctx, unknownTransferInInput(f, "2026-01-15", "2020-03-01"))
	require.NoError(t, err)
	reinvested, err := acknowledgedReinvestedDividend(ctx, f.investmentService, ReinvestedDividendInput{
		OwnerUserID: f.ownerUserID, TransactionDate: "2026-02-01", CommodityID: f.stockCommodityID,
		HoldingAccountID: f.holdingAccountID, IncomeAccountID: &f.incomeAccountID,
		QuantityValue: exact.New(2), AmountValue: 2500, AmountScale: 2, CashCommodityID: f.eurCommodityID,
	})
	require.NoError(t, err)
	require.NotNil(t, reinvested.LotID)
	requireMixedPositionAfterKnownAcquisition(t, f, *reinvested.LotID, 2, "2500", "2026-03-01")
}

func TestBuyReplacementBesideAnUnknownBasisLotPosts(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	seedExternalTransferEquity(t, f.database)
	_, err := f.investmentService.ExternalTransferIn(ctx, unknownTransferInInput(f, "2026-01-15", "2020-03-01"))
	require.NoError(t, err)
	bought := buyOn(t, f, "2026-02-01", 10, 10000)
	replaced, err := acknowledgedReplaceBuy(ctx, f.investmentService, ReplaceInvestmentBuyInput{
		OwnerUserID: f.ownerUserID, TransactionID: bought.Transaction.ID, Reason: "broker fee was missing",
		Replacement: InvestmentTradeInput{
			TransactionDate: "2026-02-01", HoldingAccountID: f.holdingAccountID,
			CommodityID: f.stockCommodityID, CashAccountID: f.cashAccountID,
			CashCommodityID: f.eurCommodityID, QuantityValue: exact.New(10),
			CashAmountValue: 10150, CashAmountScale: 2,
		},
	})
	require.NoError(t, err)
	require.NotNil(t, replaced.Replacement.LotID)
	requireMixedPositionAfterKnownAcquisition(t, f, *replaced.Replacement.LotID, 10, "10150", "2026-03-01")
}

// T-104 still holds beside an unknown lot: the known subtotal must fit, and a
// refusal rolls everything back.
func TestKnownBuyBesideAnUnknownBasisLotStillRefusesUnrepresentableKnownSubtotal(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	seedExternalTransferEquity(t, f.database)
	buyOn(t, f, "2026-01-01", 3, 1000)
	sale := sellInput(f, "2026-01-10", 1)
	sale.CashAmountValue = 500
	_, err := f.investmentService.Sell(ctx, sale)
	require.NoError(t, err)
	_, err = f.investmentService.ExternalTransferIn(ctx, unknownTransferInInput(f, "2026-01-15", "2020-03-01"))
	require.NoError(t, err)
	before := snapshotFinancialState(t, f)
	buy := sellInput(f, "2026-02-01", 1)
	buy.CashAmountValue = 1_000_000_000_000_000
	_, err = f.investmentService.Buy(ctx, buy)
	var validation ValidationError
	require.ErrorAs(t, err, &validation)
	require.Contains(t, err.Error(), "position basis exceeds supported exact range")
	require.Equal(t, before, snapshotFinancialState(t, f))
	// Control: a representable known buy still posts.
	buyOn(t, f, "2026-02-01", 1, 1000)
	requireHealthyUnresolvedBook(t, f)
}
