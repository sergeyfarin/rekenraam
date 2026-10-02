package app

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"rekenraam/backend/internal/exact"
)

func TestReverseManualBuyReplaysDependentLIFOSaleAndKeepsHistory(t *testing.T) {
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	buyOn(t, f, "2026-01-01", 10, 10000)
	target := buyOn(t, f, "2026-02-01", 10, 30000)
	sale := sellInput(f, "2026-03-01", 5)
	sale.CostBasisMethod = "lifo"
	_, err := f.investmentService.Sell(ctx, sale)
	require.NoError(t, err)
	input := ReverseInvestmentBuyInput{
		OwnerUserID: f.ownerUserID, TransactionID: target.Transaction.ID,
		Reason: "duplicate broker acquisition",
	}
	impact, err := f.investmentService.ReverseBuyReconciliationImpact(ctx, input)
	require.NoError(t, err)
	require.Empty(t, impact.AffectedCheckpoints)
	reversal, err := acknowledgedReverseBuy(ctx, f.investmentService, input)
	require.NoError(t, err)
	require.Equal(t, "posted", reversal.Status)
	require.Equal(t, target.Transaction.ID, *reversal.CorrectionOfTransactionID)
	for entryIndex, original := range target.Transaction.JournalEntries {
		for postingIndex, posting := range original.Postings {
			require.Equal(t, posting.QuantityValue.Negated(),
				reversal.JournalEntries[entryIndex].Postings[postingIndex].QuantityValue)
		}
	}
	chain, err := f.investmentService.CorrectionChain(ctx, f.ownerUserID, target.Transaction.ID)
	require.NoError(t, err)
	require.Len(t, chain.Operations, 2)
	require.Nil(t, chain.EffectiveTransactionID)
	require.Equal(t, "reverse", chain.Operations[1].CorrectionMode)
	gains, err := f.investmentService.ListRealizedGains(ctx, GainsReportParams{})
	require.NoError(t, err)
	require.Len(t, gains, 1)
	assertMoneyValue(t, -5000, 2, gains[0].DisposedBasisValue, gains[0].DisposedBasisScale,
		"sale reselects the surviving acquisition")
	var priceAudit, reversalAudit int64
	require.NoError(t, f.database.QueryRow(`SELECT voided_audit_event_id FROM price_observations
		WHERE source_transaction_version_id = ?`, target.Transaction.VersionID).Scan(&priceAudit))
	require.NoError(t, f.database.QueryRow(`SELECT created_audit_event_id FROM transactions WHERE id = ?`, reversal.ID).Scan(&reversalAudit))
	require.Equal(t, reversalAudit, priceAudit)
	check, err := selfCheckOver(t, f.database).RunSelfCheck(ctx, "manual")
	require.NoError(t, err)
	require.Equal(t, SelfCheckPassed, check.Status)
	_, err = acknowledgedReverseBuy(ctx, f.investmentService, input)
	require.ErrorIs(t, err, ErrInvestmentBuyAlreadyCorrected)
}

func TestReverseManualBuyRejectsDependentSaleAndRollsBack(t *testing.T) {
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	target := buyOn(t, f, "2026-01-01", 10, 10000)
	_, err := f.investmentService.Sell(ctx, sellInput(f, "2026-02-01", 5))
	require.NoError(t, err)
	input := ReverseInvestmentBuyInput{
		OwnerUserID: f.ownerUserID, TransactionID: target.Transaction.ID,
		Reason: "duplicate broker acquisition",
	}
	_, err = f.investmentService.ReverseBuyReconciliationImpact(ctx, input)
	require.ErrorIs(t, err, ErrInvestmentBuyDependency)
	_, err = acknowledgedReverseBuy(ctx, f.investmentService, input)
	require.ErrorIs(t, err, ErrInvestmentBuyDependency)
	var corrections, revisions int
	require.NoError(t, f.database.QueryRow(`SELECT count(*) FROM investment_operations
		WHERE correction_of_operation_id IS NOT NULL`).Scan(&corrections))
	require.NoError(t, f.database.QueryRow(`SELECT count(*) FROM investment_disposal_revisions`).Scan(&revisions))
	require.Zero(t, corrections)
	require.Zero(t, revisions)
	chain, err := f.investmentService.CorrectionChain(ctx, f.ownerUserID, target.Transaction.ID)
	require.NoError(t, err)
	require.Equal(t, target.Transaction.ID, *chain.EffectiveTransactionID)
}

func TestReverseManualBuyRefusesSpecificLotElectionForRemovedAcquisition(t *testing.T) {
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	buyOn(t, f, "2026-01-01", 10, 10000)
	target := buyOn(t, f, "2026-02-01", 10, 30000)
	sale := sellInput(f, "2026-03-01", 5)
	sale.CostBasisMethod = "specific_lot"
	sale.LotAllocations = []InvestmentLotAllocationInput{{LotID: *target.LotID, QuantityValue: exact.New(5)}}
	_, err := f.investmentService.Sell(ctx, sale)
	require.NoError(t, err)
	before := f.transactionCount(t)
	input := ReverseInvestmentBuyInput{OwnerUserID: f.ownerUserID, TransactionID: target.Transaction.ID, Reason: "duplicate acquisition"}
	_, err = f.investmentService.ReverseBuyReconciliationImpact(ctx, input)
	require.ErrorIs(t, err, ErrInvestmentBuyDependency)
	_, err = acknowledgedReverseBuy(ctx, f.investmentService, input)
	require.ErrorIs(t, err, ErrInvestmentBuyDependency)
	require.Equal(t, before, f.transactionCount(t))
	var corrections, revisions int
	require.NoError(t, f.database.QueryRow(`SELECT count(*) FROM investment_operations
		WHERE correction_of_operation_id IS NOT NULL`).Scan(&corrections))
	require.NoError(t, f.database.QueryRow(`SELECT count(*) FROM investment_disposal_revisions`).Scan(&revisions))
	require.Zero(t, corrections)
	require.Zero(t, revisions)
}

func TestReverseManualBuyRequiresReconciliationOverride(t *testing.T) {
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	target := buyOn(t, f, "2026-01-01", 10, 10000)
	checkpointID := reconcileCash(t, f, "2026-03-01", -100)
	input := ReverseInvestmentBuyInput{
		OwnerUserID: f.ownerUserID, TransactionID: target.Transaction.ID,
		Reason: "duplicate broker acquisition",
	}
	impact, err := f.investmentService.ReverseBuyReconciliationImpact(ctx, input)
	require.NoError(t, err)
	require.Len(t, impact.AffectedCheckpoints, 1)
	require.Equal(t, checkpointID, impact.AffectedCheckpoints[0].CheckpointID)
	_, err = acknowledgedReverseBuy(ctx, f.investmentService, input)
	require.ErrorIs(t, err, ErrReconciliationOverrideRequired)
	require.Equal(t, []int64{checkpointID}, activeCheckpointIDs(t, f))
	input.ReconciliationOverride = true
	_, err = acknowledgedReverseBuy(ctx, f.investmentService, input)
	require.NoError(t, err)
	require.Empty(t, activeCheckpointIDs(t, f))
	check, err := selfCheckOver(t, f.database).RunSelfCheck(ctx, "manual")
	require.NoError(t, err)
	require.Equal(t, SelfCheckPassed, check.Status)
}

func TestReverseImportedBuyRemainsFenced(t *testing.T) {
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	target, err := f.investmentService.Buy(ctx, InvestmentTradeInput{
		OwnerUserID: f.ownerUserID, OriginType: "import", TransactionDate: "2026-01-01",
		CommodityID: f.stockCommodityID, HoldingAccountID: f.holdingAccountID,
		CashAccountID: f.cashAccountID, CashCommodityID: f.eurCommodityID,
		QuantityValue: exact.New(10), CashAmountValue: 10000, CashAmountScale: 2,
	})
	require.NoError(t, err)
	_, err = acknowledgedReverseBuy(ctx, f.investmentService, ReverseInvestmentBuyInput{
		OwnerUserID: f.ownerUserID, TransactionID: target.Transaction.ID, Reason: "remove import",
	})
	require.ErrorIs(t, err, ErrInvestmentImportedBuy)
}
