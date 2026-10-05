package app

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rekenraam/backend/internal/exact"
)

// T-147: cash in lieu of a split's fraction. The contract example: 3 shares
// split 3-for-2 hold 4.5; the 0.5 fraction settles for 8.00 EUR.

func cashInLieuSetup(t *testing.T) (*investmentsTestFixture, InvestmentSplitResult) {
	t.Helper()
	f := newInvestmentsTestFixture(t)
	buyOn(t, f, "2026-05-01", 3, 3000)
	split, err := f.investmentService.Split(context.Background(), splitInput(f, "2026-06-01", 3, 2))
	require.NoError(t, err)
	return f, split
}

func cashInLieuInput(f *investmentsTestFixture, splitTransactionID int64) CashInLieuInput {
	return CashInLieuInput{OwnerUserID: f.ownerUserID, SplitTransactionID: splitTransactionID,
		DisposalOn: "2026-06-01", PaymentOn: "2026-06-05", QuantityValue: exact.New(5), QuantityScale: 1,
		CashAccountID: f.cashAccountID, CurrencyID: f.eurCommodityID, ProceedsValue: 800, ProceedsScale: 2,
		Memo: "cash in lieu"}
}

func TestCashInLieuDisposesFractionWithCashOnItsPaymentDate(t *testing.T) {
	t.Parallel()
	f, split := cashInLieuSetup(t)
	ctx := context.Background()
	preview, impact, err := f.investmentService.PreviewCashInLieu(ctx, cashInLieuInput(f, split.Transaction.ID))
	require.NoError(t, err)
	assert.Empty(t, impact.AffectedCheckpoints)
	result, err := f.investmentService.CashInLieu(ctx, cashInLieuInput(f, split.Transaction.ID))
	require.NoError(t, err)
	assert.Equal(t, preview.Allocations, result.Allocations, "preview takes what the commit takes")

	require.Len(t, result.Transaction.JournalEntries, 2, "security and cash on their own dates")
	assert.Equal(t, "2026-06-01", result.Transaction.JournalEntries[0].EntryDate)
	assert.Equal(t, "2026-06-05", result.Transaction.JournalEntries[1].EntryDate)
	var kind string
	var linkedSplit int64
	require.NoError(t, f.database.QueryRow(`SELECT o.operation_kind, f.split_operation_id
		FROM investment_cash_in_lieu_facts f JOIN investment_operations o ON o.id = f.operation_id`).Scan(&kind, &linkedSplit))
	assert.Equal(t, "cash_in_lieu", kind)
	assert.Equal(t, transferOperationID(t, f, split.Transaction.ID), linkedSplit)

	quantity, remaining := openPositionBasis(t, f, f.holdingAccountID)
	requireScaled(t, 4, 0, quantity, "4.5 less the 0.5 fraction")
	disposed := saleEffectiveBasis(t, f, result.Transaction.ID)
	disposed.AddScaled(remaining)
	requireScaled(t, 3000, 2, disposed, "the fraction's basis plus the rest conserve 30.00")
	requireInvestmentSelfCheckPasses(t, f)
}

func TestCashInLieuRefusesWholeShares(t *testing.T) {
	t.Parallel()
	f, split := cashInLieuSetup(t)
	input := cashInLieuInput(f, split.Transaction.ID)
	input.QuantityValue, input.QuantityScale = exact.New(1), 0
	_, err := f.investmentService.CashInLieu(context.Background(), input)
	require.ErrorIs(t, err, ErrCashInLieuNotFraction)
}

func TestCashInLieuRefusesDisposalBeforeItsSplit(t *testing.T) {
	t.Parallel()
	f, split := cashInLieuSetup(t)
	input := cashInLieuInput(f, split.Transaction.ID)
	input.DisposalOn = "2026-05-15"
	_, err := f.investmentService.CashInLieu(context.Background(), input)
	var validation ValidationError
	require.ErrorAs(t, err, &validation)
}

// Reversing the split it settles would leave the cash in lieu settling
// nothing: the split correction is refused with the cash in lieu named.
func TestSplitReversalRefusedWhileCashInLieuSettlesIt(t *testing.T) {
	t.Parallel()
	f, split := cashInLieuSetup(t)
	ctx := context.Background()
	cil, err := f.investmentService.CashInLieu(ctx, cashInLieuInput(f, split.Transaction.ID))
	require.NoError(t, err)
	before := f.transactionCount(t)
	_, err = acknowledgedReverseSplit(ctx, f.investmentService, ReverseInvestmentSplitInput{
		OwnerUserID: f.ownerUserID, TransactionID: split.Transaction.ID, Reason: "entered in error"})
	require.ErrorIs(t, err, ErrInvestmentSplitDependency)
	var dependency InvestmentSplitDependencyError
	require.ErrorAs(t, err, &dependency)
	assert.Equal(t, transferOperationID(t, f, cil.Transaction.ID), dependency.OperationID)
	assert.Equal(t, before, f.transactionCount(t))
}

func TestCashInLieuOfReversedSplitIsRefused(t *testing.T) {
	t.Parallel()
	f, split := cashInLieuSetup(t)
	ctx := context.Background()
	_, err := acknowledgedReverseSplit(ctx, f.investmentService, ReverseInvestmentSplitInput{
		OwnerUserID: f.ownerUserID, TransactionID: split.Transaction.ID, Reason: "entered in error"})
	require.NoError(t, err)
	_, err = f.investmentService.CashInLieu(ctx, cashInLieuInput(f, split.Transaction.ID))
	require.ErrorIs(t, err, ErrCashInLieuSplitNotFound)
}

// T-150: reversing a cash in lieu restores the fraction and frees its split
// for correction again.
func TestReverseCashInLieuRestoresFractionAndFreesSplit(t *testing.T) {
	t.Parallel()
	f, split := cashInLieuSetup(t)
	ctx := context.Background()
	cil, err := f.investmentService.CashInLieu(ctx, cashInLieuInput(f, split.Transaction.ID))
	require.NoError(t, err)
	input := ReverseInvestmentSaleInput{OwnerUserID: f.ownerUserID, TransactionID: cil.Transaction.ID, Reason: "paid in shares instead"}
	impact, err := f.investmentService.ReverseCashInLieuReconciliationImpact(ctx, input)
	require.NoError(t, err)
	if impact.GainImpact != nil {
		input.GainImpactAcknowledgement = impact.GainImpact.Acknowledgement
	}
	_, err = f.investmentService.ReverseCashInLieu(ctx, input)
	require.NoError(t, err)
	quantity, _ := openPositionBasis(t, f, f.holdingAccountID)
	requireScaled(t, 45, 1, quantity, "the fraction is held again")
	_, err = f.investmentService.ReverseSale(ctx, input)
	require.ErrorIs(t, err, ErrInvestmentSaleNotFound, "a cash in lieu is not a sale")
	requireInvestmentSelfCheckPasses(t, f)

	_, err = acknowledgedReverseSplit(ctx, f.investmentService, ReverseInvestmentSplitInput{
		OwnerUserID: f.ownerUserID, TransactionID: split.Transaction.ID, Reason: "entered in error"})
	require.NoError(t, err, "with the cash in lieu reversed, the split may be corrected")
}

func TestReplaceCashInLieuKeepsItsSplitAndPreviewMatchesCommit(t *testing.T) {
	t.Parallel()
	f, split := cashInLieuSetup(t)
	ctx := context.Background()
	cil, err := f.investmentService.CashInLieu(ctx, cashInLieuInput(f, split.Transaction.ID))
	require.NoError(t, err)
	replacement := cashInLieuInput(f, 0)
	replacement.ProceedsValue, replacement.PaymentOn = 850, "2026-06-08"
	input := ReplaceCashInLieuInput{OwnerUserID: f.ownerUserID, TransactionID: cil.Transaction.ID,
		Reason: "broker paid 8.50", Replacement: replacement}
	impact, err := f.investmentService.ReplaceCashInLieuReconciliationImpact(ctx, input)
	require.NoError(t, err)
	if impact.GainImpact != nil {
		input.GainImpactAcknowledgement = impact.GainImpact.Acknowledgement
	}
	result, err := f.investmentService.ReplaceCashInLieu(ctx, input)
	require.NoError(t, err)
	require.Len(t, result.Replacement.Transaction.JournalEntries, 2)
	assert.Equal(t, "2026-06-08", result.Replacement.Transaction.JournalEntries[1].EntryDate)
	var linked int64
	require.NoError(t, f.database.QueryRow(`SELECT f.split_operation_id FROM investment_cash_in_lieu_facts f
		JOIN effective_investment_operations o ON o.id = f.operation_id`).Scan(&linked))
	assert.Equal(t, transferOperationID(t, f, split.Transaction.ID), linked, "the successor settles the same split")
	requireInvestmentSelfCheckPasses(t, f)
}
