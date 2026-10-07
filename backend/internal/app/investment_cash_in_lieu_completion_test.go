package app

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"rekenraam/backend/internal/exact"
)

func TestCashInLieuPreviewsRunSplitFactGuardAndRollBack(t *testing.T) {
	t.Parallel()
	f, split := cashInLieuSetup(t)
	ctx := context.Background()
	input := cashInLieuInput(f, split.Transaction.ID)
	cil, err := f.investmentService.CashInLieu(ctx, input)
	require.NoError(t, err)
	before := buyReplacementPreviewSnapshot(t, f.database)
	_, err = f.database.Exec(`CREATE TRIGGER reject_cash_in_lieu_fact BEFORE INSERT ON investment_cash_in_lieu_facts BEGIN SELECT RAISE(ABORT, 'forced cash in lieu fact refusal'); END`)
	require.NoError(t, err)
	_, _, err = f.investmentService.PreviewCashInLieu(ctx, input)
	require.ErrorContains(t, err, "forced cash in lieu fact refusal")
	_, err = f.investmentService.ReplaceCashInLieuReconciliationImpact(ctx, ReplaceCashInLieuInput{OwnerUserID: f.ownerUserID, TransactionID: cil.Transaction.ID, Reason: "correct amount", Replacement: input})
	require.ErrorContains(t, err, "forced cash in lieu fact refusal")
	require.Equal(t, before, buyReplacementPreviewSnapshot(t, f.database))
}

func TestCashInLieuDatedLotsIncludeClosedHoldingAndReplacementRootSlot(t *testing.T) {
	t.Parallel()
	f, split := cashInLieuSetup(t)
	ctx := context.Background()
	input := cashInLieuInput(f, split.Transaction.ID)
	cil, err := f.investmentService.CashInLieu(ctx, input)
	require.NoError(t, err)
	_, err = f.investmentService.Sell(ctx, sellInput(f, "2026-06-01", 4))
	require.NoError(t, err)
	before := buyReplacementPreviewSnapshot(t, f.database)
	lots, err := f.investmentService.CashInLieuAvailableLots(ctx, f.ownerUserID, split.Transaction.ID, f.eurCommodityID, "2026-06-01", cil.Transaction.ID)
	require.NoError(t, err)
	require.Len(t, lots, 1)
	requireScaled(t, 45, 1, exact.ScaledIntFromCoefficient(exact.MustParse(lots[0].QuantityValue), lots[0].QuantityScale), "replacement slot precedes the later same-day sale even when the position has closed")
	current, err := f.investmentService.CashInLieuAvailableLots(ctx, f.ownerUserID, split.Transaction.ID, f.eurCommodityID, "2026-06-01", 0)
	require.NoError(t, err)
	require.Empty(t, current, "a new entry takes its new same-day slot")
	require.Equal(t, before, buyReplacementPreviewSnapshot(t, f.database))
	input.QuantityValue = exact.New(25)
	input.QuantityScale = 2
	replacement := ReplaceCashInLieuInput{OwnerUserID: f.ownerUserID, TransactionID: cil.Transaction.ID, Reason: "fraction corrected", Replacement: input}
	preview, impact, err := f.investmentService.PreviewCashInLieuReplacement(ctx, replacement)
	require.NoError(t, err)
	replacement.GainImpactAcknowledgement = impact.GainImpact.Acknowledgement
	corrected, err := f.investmentService.ReplaceCashInLieu(ctx, replacement)
	require.NoError(t, err)
	require.Equal(t, preview.Allocations, corrected.Replacement.Allocations)
	requireInvestmentSelfCheckPasses(t, f)
}

func TestCashInLieuReplacementRefusesAnotherSplitAndLateFailureRollsBack(t *testing.T) {
	t.Parallel()
	f, split := cashInLieuSetup(t)
	ctx := context.Background()
	input := cashInLieuInput(f, split.Transaction.ID)
	cil, err := f.investmentService.CashInLieu(ctx, input)
	require.NoError(t, err)
	replacement := ReplaceCashInLieuInput{OwnerUserID: f.ownerUserID, TransactionID: cil.Transaction.ID, Reason: "cash corrected", Replacement: input}
	replacement.Replacement.SplitTransactionID = cil.Transaction.ID
	_, _, err = f.investmentService.PreviewCashInLieuReplacement(ctx, replacement)
	require.Error(t, err)
	_, err = f.investmentService.ReplaceCashInLieu(ctx, replacement)
	require.Error(t, err)
	replacement.Replacement.SplitTransactionID = split.Transaction.ID
	replacement.Replacement.ProceedsValue = 850
	_, impact, err := f.investmentService.PreviewCashInLieuReplacement(ctx, replacement)
	require.NoError(t, err)
	replacement.GainImpactAcknowledgement = impact.GainImpact.Acknowledgement
	before := buyReplacementPreviewSnapshot(t, f.database)
	_, err = f.database.Exec(`CREATE TRIGGER reject_cil_fact_late BEFORE INSERT ON investment_cash_in_lieu_facts BEGIN SELECT RAISE(ABORT, 'late cash in lieu failure'); END`)
	require.NoError(t, err)
	_, err = f.investmentService.ReplaceCashInLieu(ctx, replacement)
	require.ErrorContains(t, err, "late cash in lieu failure")
	require.Equal(t, before, buyReplacementPreviewSnapshot(t, f.database))
}
