package app

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"rekenraam/backend/internal/exact"
)

func TestReinvestmentPreviewPropagatesTransferBasisWithoutWriting(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := newInvestmentsTestFixture(t)
	source := buyOn(t, f, "2026-02-01", 3, 1000)
	_, err := f.investmentService.Sell(ctx, sellInput(f, "2026-03-01", 2))
	require.NoError(t, err)
	destination := seedTestAccountWithClass(t, f.database, "active", true, "asset", "security_holding")
	transfer, err := f.investmentService.InternalTransfer(ctx, internalTransferFromLot(f, destination, *source.LotID, exact.New(1), 0))
	require.NoError(t, err)
	input := ReinvestedDividendInput{OwnerUserID: f.ownerUserID, TransactionDate: "2026-01-01",
		CommodityID: f.stockCommodityID, HoldingAccountID: f.holdingAccountID,
		IncomeAccountID: &f.incomeAccountID, CashCommodityID: f.eurCommodityID,
		QuantityValue: exact.New(2), AmountValue: 100, AmountScale: 2}
	before := buyReplacementPreviewSnapshot(t, f.database)
	impact, err := f.investmentService.ReinvestedDividendReconciliationImpact(ctx, input)
	require.NoError(t, err)
	require.NotNil(t, impact.GainImpact)
	require.Equal(t, before, buyReplacementPreviewSnapshot(t, f.database), "preview writes nothing")
	input.GainImpactAcknowledgement = impact.GainImpact.Acknowledgement
	_, err = f.investmentService.ReinvestedDividend(ctx, input)
	require.NoError(t, err)
	_, revisions := effectiveTransferBasis(t, f, transfer.Transaction.ID, 1)
	require.Equal(t, 1, revisions)
	requireInvestmentSelfCheckPasses(t, f)
}

func TestReinvestmentPreviewReplaysAndRollsBackReconciledHolding(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := newInvestmentsTestFixture(t)
	buyOn(t, f, "2026-02-01", 10, 20000)
	sale := sellInput(f, "2026-03-01", 5)
	sale.CashAmountValue = 15000
	_, err := f.investmentService.Sell(ctx, sale)
	require.NoError(t, err)
	session, err := f.transactionService.StartReconciliation(ctx, StartReconciliationInput{
		OriginType: "browser_api", OwnerUserID: f.ownerUserID, AccountID: f.holdingAccountID,
		CommodityID: f.stockCommodityID, StatementDate: "2026-04-01", StatementBalanceValue: exact.New(5),
	})
	require.NoError(t, err)
	var postingIDs []int64
	for _, candidate := range session.Candidates {
		postingIDs = append(postingIDs, candidate.PostingID)
	}
	_, err = f.transactionService.UpdateReconciliationSelection(ctx, ReconciliationSelectionInput{
		OriginType: "browser_api", OwnerUserID: f.ownerUserID, SessionID: session.ID, PostingVersionIDs: postingIDs,
	})
	require.NoError(t, err)
	_, err = f.transactionService.FinishReconciliation(ctx, FinishReconciliationInput{
		OriginType: "browser_api", OwnerUserID: f.ownerUserID, SessionID: session.ID, ChangeReason: "holding statement verified",
	})
	require.NoError(t, err)
	checkpoints, err := f.transactionService.repository.ListReconciliationCheckpoints(ctx, BookID, f.holdingAccountID, f.stockCommodityID)
	require.NoError(t, err)
	require.Len(t, checkpoints, 1)
	input := ReinvestedDividendInput{OwnerUserID: f.ownerUserID, TransactionDate: "2026-01-01",
		CommodityID: f.stockCommodityID, HoldingAccountID: f.holdingAccountID,
		IncomeAccountID: &f.incomeAccountID, CashCommodityID: f.eurCommodityID,
		QuantityValue: exact.New(10), AmountValue: 2000, AmountScale: 2}
	before := buyReplacementPreviewSnapshot(t, f.database)
	for range 2 {
		impact, err := f.investmentService.ReinvestedDividendReconciliationImpact(ctx, input)
		require.NoError(t, err)
		require.Len(t, impact.AffectedCheckpoints, 1)
		require.Equal(t, checkpoints[0].ID, impact.AffectedCheckpoints[0].CheckpointID)
		require.Equal(t, "2026-01-01", impact.AffectedCheckpoints[0].EntryDate)
		require.NotEmpty(t, impact.AffectedCheckpoints[0].AccountLabel)
		require.NotEmpty(t, impact.AffectedCheckpoints[0].CommodityCode)
		require.Equal(t, before, buyReplacementPreviewSnapshot(t, f.database))
	}
	_, err = acknowledgedReinvestedDividend(ctx, f.investmentService, input)
	require.ErrorIs(t, err, ErrReconciliationOverrideRequired)
	require.Equal(t, before, buyReplacementPreviewSnapshot(t, f.database))
	input.ReconciliationOverride = true
	result, err := acknowledgedReinvestedDividend(ctx, f.investmentService, input)
	require.NoError(t, err)
	require.Equal(t, []int64{checkpoints[0].ID}, result.Transaction.InvalidatedCheckpointIDs)
	gains, err := f.investmentService.ListRealizedGains(ctx, GainsReportParams{})
	require.NoError(t, err)
	require.Len(t, gains, 1)
	assertMoneyValue(t, 14000, 2, gains[0].RealizedGainValue, gains[0].RealizedGainScale, "reinvestment revised FIFO gain")
	check, err := selfCheckOver(t, f.database).RunSelfCheck(ctx, "manual")
	require.NoError(t, err)
	require.Equal(t, SelfCheckPassed, check.Status)
}
