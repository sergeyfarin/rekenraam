package app

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"rekenraam/backend/internal/exact"
)

func TestReplayIntentKeepsSpecificLotElectionButNotFIFOSelection(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	trade := InvestmentTradeInput{OwnerUserID: f.ownerUserID, CommodityID: f.stockCommodityID,
		HoldingAccountID: f.holdingAccountID, CashAccountID: f.cashAccountID,
		CashCommodityID: f.eurCommodityID, QuantityValue: exact.New(1),
		CashAmountValue: 10000, CashAmountScale: 2}
	trade.TransactionDate = "2026-01-01"
	first, err := f.investmentService.Buy(ctx, trade)
	require.NoError(t, err)
	trade.TransactionDate = "2026-01-02"
	second, err := f.investmentService.Buy(ctx, trade)
	require.NoError(t, err)

	trade.TransactionDate = "2026-01-03"
	trade.CostBasisMethod = "specific_lot"
	trade.LotAllocations = []InvestmentLotAllocationInput{{LotID: *second.LotID, QuantityValue: exact.New(1)}}
	_, err = f.investmentService.Sell(ctx, trade)
	require.NoError(t, err)
	intents, err := f.investmentService.repository.ListInvestmentReplayIntents(ctx, BookID,
		f.holdingAccountID, f.stockCommodityID, f.eurCommodityID, "long")
	require.NoError(t, err)
	require.Len(t, intents, 3)
	require.Equal(t, *first.LotID, intents[0].LotID)
	require.Equal(t, *second.LotID, intents[1].LotID)
	require.Equal(t, "specific_lot", intents[2].CostBasisMethod)
	require.Len(t, intents[2].SpecificLots, 1)
	require.Equal(t, *second.LotID, intents[2].SpecificLots[0].LotID)
	require.Equal(t, "1", intents[2].SpecificLots[0].QuantityValue.String())
}
