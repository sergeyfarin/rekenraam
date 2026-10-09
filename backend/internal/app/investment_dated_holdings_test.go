package app

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// #166: historical entry selects holdings and lots as they stood at the
// command's dated slot, including holdings a later sale closed. The read is
// discovery only; the writers recheck at commit.

func datedHoldingsByCurrency(t *testing.T, f *investmentsTestFixture, asOf string) map[int64]DatedHolding {
	t.Helper()
	holdings, err := f.investmentService.DatedHoldings(context.Background(), f.ownerUserID, asOf)
	require.NoError(t, err)
	byCurrency := make(map[int64]DatedHolding, len(holdings))
	for _, holding := range holdings {
		require.Equal(t, f.holdingAccountID, holding.AccountID)
		require.Equal(t, f.stockCommodityID, holding.CommodityID)
		byCurrency[holding.CostCommodityID] = holding
	}
	return byCurrency
}

func TestDatedHoldingsListAHoldingALaterSaleClosed(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	buyOn(t, f, "2026-05-01", 2, 2000)
	_, err := f.investmentService.Sell(context.Background(), sellInput(f, "2026-07-01", 2))
	require.NoError(t, err)

	assert.Empty(t, datedHoldingsByCurrency(t, f, "2026-04-30"), "nothing was held yet")
	june := datedHoldingsByCurrency(t, f, "2026-06-01")[f.eurCommodityID]
	assert.Equal(t, "2", june.QuantityValue.String(), "held in June though closed today")
	require.Len(t, june.Lots, 1)
	assert.Equal(t, "2026-05-01", june.Lots[0].OpenedOn)
	assert.Equal(t, "selected_lots", june.TransferBasisAllocation)
	assert.Empty(t, datedHoldingsByCurrency(t, f, "2026-07-01"), "an entry on the sale date follows the sale")
}

func TestDatedHoldingsShowPartiallyConsumedLotsAtTheirDate(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	first := buyOn(t, f, "2026-05-01", 3, 3000)
	second := buyOn(t, f, "2026-05-15", 3, 4500)
	_, err := f.investmentService.Sell(ctx, sellInput(f, "2026-07-01", 4))
	require.NoError(t, err)

	june := datedHoldingsByCurrency(t, f, "2026-06-01")[f.eurCommodityID]
	assert.Equal(t, "6", june.QuantityValue.String())
	require.Len(t, june.Lots, 2)
	assert.Equal(t, []int64{*first.LotID, *second.LotID}, []int64{june.Lots[0].LotID, june.Lots[1].LotID})

	july := datedHoldingsByCurrency(t, f, "2026-07-01")[f.eurCommodityID]
	assert.Equal(t, "2", july.QuantityValue.String(), "FIFO took the first lot and one of the second")
	require.Len(t, july.Lots, 1)
	assert.Equal(t, *second.LotID, july.Lots[0].LotID)
	assert.Equal(t, "2", july.Lots[0].QuantityValue.String())
}

func TestDatedHoldingsSeparateCostCurrenciesAndFollowSameDayOrder(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	usd := seedTestCurrencyCommodity(t, f.database, "USD")
	buyOn(t, f, "2026-05-01", 2, 2000)
	dollars := tradeOn(f, f.holdingAccountID, "2026-05-01", 5, 5000)
	dollars.CashCommodityID = usd
	_, err := f.investmentService.Buy(ctx, dollars)
	require.NoError(t, err)
	// Bought and partly sold on the same day: the slot follows both.
	sale := sellInput(f, "2026-05-01", 1)
	_, err = f.investmentService.Sell(ctx, sale)
	require.NoError(t, err)

	holdings := datedHoldingsByCurrency(t, f, "2026-05-01")
	require.Len(t, holdings, 2)
	assert.Equal(t, "1", holdings[f.eurCommodityID].QuantityValue.String())
	assert.Equal(t, "5", holdings[usd].QuantityValue.String())
}

func TestDatedHoldingsRejectAnInvalidDate(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	_, err := f.investmentService.DatedHoldings(context.Background(), f.ownerUserID, "2026-13-01")
	var validation ValidationError
	require.ErrorAs(t, err, &validation)
}
