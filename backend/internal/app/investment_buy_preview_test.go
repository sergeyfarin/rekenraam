package app

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"rekenraam/backend/internal/exact"
)

func TestBuyPreviewRejectsChangedInternalTransferBasisWithoutWriting(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := newInvestmentsTestFixture(t)
	source := buyOn(t, f, "2026-02-01", 3, 1000)
	_, err := f.investmentService.Sell(ctx, sellInput(f, "2026-03-01", 2))
	require.NoError(t, err)
	destination := seedTestAccountWithClass(t, f.database, "active", true, "asset", "security_holding")
	transfer, err := f.investmentService.InternalTransfer(ctx, internalTransferFromLot(f, destination, *source.LotID, exact.New(1), 0))
	require.NoError(t, err)
	input := InvestmentTradeInput{OwnerUserID: f.ownerUserID, TransactionDate: "2026-01-01",
		CommodityID: f.stockCommodityID, HoldingAccountID: f.holdingAccountID,
		CashAccountID: f.cashAccountID, CashCommodityID: f.eurCommodityID,
		QuantityValue: exact.New(2), CashAmountValue: 100, CashAmountScale: 2}
	before := buyReplacementPreviewSnapshot(t, f.database)
	_, err = f.investmentService.Buy(ctx, input)
	require.ErrorIs(t, err, ErrInvestmentBuyDependency)
	require.Equal(t, before, buyReplacementPreviewSnapshot(t, f.database))
	_, err = f.investmentService.TradeReconciliationImpact(ctx, InvestmentImpactBuy, input)
	require.ErrorIs(t, err, ErrInvestmentBuyDependency)
	var dependency InvestmentBuyDependencyError
	require.ErrorAs(t, err, &dependency)
	var operationID int64
	require.NoError(t, f.database.QueryRow(`SELECT l.operation_id FROM investment_operation_journal_links l JOIN transaction_versions v ON v.id = l.transaction_version_id WHERE v.transaction_id = ?`, transfer.Transaction.ID).Scan(&operationID))
	require.Equal(t, operationID, dependency.OperationID)
	require.Zero(t, dependency.DecisionID)
	require.Equal(t, before, buyReplacementPreviewSnapshot(t, f.database))
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
	checkpoint := reconcileCash(t, f, "2026-04-01", -50)
	input := InvestmentTradeInput{OwnerUserID: f.ownerUserID, TransactionDate: "2026-01-01",
		CommodityID: f.stockCommodityID, HoldingAccountID: f.holdingAccountID,
		CashAccountID: f.cashAccountID, CashCommodityID: f.eurCommodityID,
		QuantityValue: exact.New(10), CashAmountValue: 2000, CashAmountScale: 2}
	before := buyReplacementPreviewSnapshot(t, f.database)
	for range 2 {
		impact, err := f.investmentService.TradeReconciliationImpact(ctx, InvestmentImpactBuy, input)
		require.NoError(t, err)
		require.Len(t, impact.AffectedCheckpoints, 1)
		require.Equal(t, checkpoint, impact.AffectedCheckpoints[0].CheckpointID)
		require.Equal(t, before, buyReplacementPreviewSnapshot(t, f.database))
	}
	_, err = f.investmentService.Buy(ctx, input)
	require.ErrorIs(t, err, ErrReconciliationOverrideRequired)
	require.Equal(t, before, buyReplacementPreviewSnapshot(t, f.database))
	input.ReconciliationOverride = true
	_, err = f.investmentService.Buy(ctx, input)
	require.NoError(t, err)
	gains, err := f.investmentService.ListRealizedGains(ctx, GainsReportParams{})
	require.NoError(t, err)
	require.Len(t, gains, 1)
	assertMoneyValue(t, 14000, 2, gains[0].RealizedGainValue, gains[0].RealizedGainScale, "committed FIFO gain includes backdated buy")
}
