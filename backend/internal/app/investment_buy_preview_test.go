package app

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"rekenraam/backend/internal/exact"
)

// A rounding remainder after a partial FIFO sale: an earlier buy changes
// which opening funds that sale, so the transfer now carries a different
// basis. Preview runs the same propagation as commit and writes nothing.
func TestBuyPreviewMatchesCommitWhenTransferBasisPropagates(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := newInvestmentsTestFixture(t)
	source := buyOn(t, f, "2026-02-01", 3, 1000)
	_, err := f.investmentService.Sell(ctx, sellInput(f, "2026-03-01", 2))
	require.NoError(t, err)
	destination := seedTestAccountWithClass(t, f.database, "active", true, "asset", "security_holding")
	transfer, err := f.investmentService.InternalTransfer(ctx, internalTransferFromLot(f, destination, *source.LotID, exact.New(1), 0))
	require.NoError(t, err)
	original, _ := effectiveTransferBasis(t, f, transfer.Transaction.ID, 1)
	input := InvestmentTradeInput{OwnerUserID: f.ownerUserID, TransactionDate: "2026-01-01",
		CommodityID: f.stockCommodityID, HoldingAccountID: f.holdingAccountID,
		CashAccountID: f.cashAccountID, CashCommodityID: f.eurCommodityID,
		QuantityValue: exact.New(2), CashAmountValue: 100, CashAmountScale: 2}
	before := buyReplacementPreviewSnapshot(t, f.database)
	input.GainImpactAcknowledgement = previewBuyGainImpact(t, f, input).Acknowledgement
	require.Equal(t, before, buyReplacementPreviewSnapshot(t, f.database), "preview writes nothing")
	_, err = f.investmentService.Buy(ctx, input)
	require.NoError(t, err)
	revised, revisions := effectiveTransferBasis(t, f, transfer.Transaction.ID, 1)
	require.Equal(t, 1, revisions)
	require.NotZero(t, revised.Cmp(original), "the carried basis changed")
	require.Zero(t, revised.Cmp(lotRemainingBasis(t, f, transfer.DestinationLotIDs[0])))
	conserved := lotRemainingBasis(t, f, *source.LotID)
	conserved.AddScaled(revised)
	requireScaled(t, 1000, 2, conserved, "source plus destination keep the February basis")
	requireInvestmentSelfCheckPasses(t, f)
}

func TestBuyPreviewReplaysAndRollsBackReconciledHistory(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := newInvestmentsTestFixture(t)
	buyOn(t, f, "2026-02-01", 10, 20000)
	sale := sellInput(f, "2026-03-01", 5)
	sale.CashAmountValue = 15000
	_, err := f.investmentService.Sell(ctx, sale)
	require.NoError(t, err)
	gainsBefore, err := f.investmentService.ListRealizedGains(ctx, GainsReportParams{})
	require.NoError(t, err)
	require.Len(t, gainsBefore, 1)
	assertMoneyValue(t, 5000, 2, gainsBefore[0].RealizedGainValue, gainsBefore[0].RealizedGainScale, "original FIFO gain before replay")
	checkpoint := reconcileCash(t, f, "2026-04-01", -50)
	buyOn(t, f, "2026-05-01", 1, 100)
	laterCheckpoint := reconcileCash(t, f, "2026-05-10", -51)
	input := InvestmentTradeInput{OwnerUserID: f.ownerUserID, TransactionDate: "2026-01-01",
		CommodityID: f.stockCommodityID, HoldingAccountID: f.holdingAccountID,
		CashAccountID: f.cashAccountID, CashCommodityID: f.eurCommodityID,
		QuantityValue: exact.New(10), CashAmountValue: 2000, CashAmountScale: 2}
	before := buyReplacementPreviewSnapshot(t, f.database)
	for range 2 {
		impact, err := f.investmentService.TradeReconciliationImpact(ctx, InvestmentImpactBuy, input)
		require.NoError(t, err)
		require.Len(t, impact.AffectedCheckpoints, 2)
		require.ElementsMatch(t, []int64{checkpoint, laterCheckpoint}, []int64{impact.AffectedCheckpoints[0].CheckpointID, impact.AffectedCheckpoints[1].CheckpointID})
		require.Equal(t, before, buyReplacementPreviewSnapshot(t, f.database))
	}
	_, err = f.investmentService.Buy(ctx, input)
	require.ErrorIs(t, err, ErrReconciliationOverrideRequired)
	require.Equal(t, before, buyReplacementPreviewSnapshot(t, f.database))
	input.ReconciliationOverride = true
	_, err = f.investmentService.Buy(ctx, input)
	require.ErrorIs(t, err, ErrGainImpactAcknowledgementRequired)
	require.Equal(t, before, buyReplacementPreviewSnapshot(t, f.database))
	impact, err := f.investmentService.TradeReconciliationImpact(ctx, InvestmentImpactBuy, input)
	require.NoError(t, err)
	input.GainImpactAcknowledgement = impact.GainImpact.Acknowledgement
	result, err := f.investmentService.Buy(ctx, input)
	require.NoError(t, err)
	require.ElementsMatch(t, []int64{checkpoint, laterCheckpoint}, result.Transaction.InvalidatedCheckpointIDs)
	gains, err := f.investmentService.ListRealizedGains(ctx, GainsReportParams{})
	require.NoError(t, err)
	require.Len(t, gains, 1)
	assertMoneyValue(t, 14000, 2, gains[0].RealizedGainValue, gains[0].RealizedGainScale, "committed FIFO gain includes backdated buy")
}
