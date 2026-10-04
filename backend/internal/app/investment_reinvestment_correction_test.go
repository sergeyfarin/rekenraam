package app

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"rekenraam/backend/internal/exact"
)

// T-115: a reinvested dividend is corrected like an acquisition paid for by
// income. These pin reversal, replacement, dependent replay, gain disclosure,
// the reconciliation guard and the family fences.

func reinvestOn(t *testing.T, f *investmentsTestFixture, date string, quantity, amount int64) InvestmentTradeResult {
	t.Helper()
	result, err := acknowledgedReinvestedDividend(context.Background(), f.investmentService, reinvestmentInput(f, date, quantity, amount))
	require.NoError(t, err)
	return result
}

func reinvestmentInput(f *investmentsTestFixture, date string, quantity, amount int64) ReinvestedDividendInput {
	return ReinvestedDividendInput{OwnerUserID: f.ownerUserID, TransactionDate: date,
		CommodityID: f.stockCommodityID, HoldingAccountID: f.holdingAccountID,
		IncomeAccountID: &f.incomeAccountID, CashCommodityID: f.eurCommodityID,
		QuantityValue: exact.New(quantity), AmountValue: amount, AmountScale: 2}
}

// reinvestmentWithLaterSale records a January reinvestment of 10 shares for
// 20.00, a February buy of 10 for 200.00 and a March FIFO sale of 5 for
// 150.00, which disposes of reinvested shares: basis 10.00, gain 140.00.
func reinvestmentWithLaterSale(t *testing.T, f *investmentsTestFixture) InvestmentTradeResult {
	t.Helper()
	reinvestment := reinvestOn(t, f, "2026-01-01", 10, 2000)
	buyOn(t, f, "2026-02-01", 10, 20000)
	sale := sellInput(f, "2026-03-01", 5)
	sale.CashAmountValue = 15000
	_, err := f.investmentService.Sell(context.Background(), sale)
	require.NoError(t, err)
	return reinvestment
}

func incomeBalance(t *testing.T, f *investmentsTestFixture) int64 {
	t.Helper()
	rows, err := f.database.Query(`SELECT pv.quantity_value FROM posting_versions pv
		JOIN current_transaction_versions current ON current.id = pv.transaction_version_id
		WHERE pv.account_id = ?`, f.incomeAccountID)
	require.NoError(t, err)
	defer rows.Close()
	total := int64(0)
	for rows.Next() {
		var value exact.Coefficient
		require.NoError(t, rows.Scan(&value))
		amount, err := exact.ScaledIntFromCoefficient(value, 0).Int64()
		require.NoError(t, err)
		total += amount
	}
	require.NoError(t, rows.Err())
	return total
}

func TestReplaceReinvestedDividendAmountAndQuantityReplaysDependentSale(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := newInvestmentsTestFixture(t)
	original := reinvestmentWithLaterSale(t, f)
	replacement := reinvestmentInput(f, "2026-01-01", 4, 4000)
	input := ReplaceReinvestedDividendInput{OwnerUserID: f.ownerUserID, TransactionID: original.Transaction.ID,
		Reason: "broker restated the reinvestment", Replacement: replacement}

	before := buyReplacementPreviewSnapshot(t, f.database)
	impact, err := f.investmentService.ReplaceReinvestedDividendReconciliationImpact(ctx, input)
	require.NoError(t, err)
	require.Equal(t, before, buyReplacementPreviewSnapshot(t, f.database), "preview writes nothing")
	require.NotNil(t, impact.GainImpact)
	require.Len(t, impact.GainImpact.Changes, 1)
	// FIFO now takes 4 reinvested shares (40.00) and 1 bought share (20.00).
	requireScaled(t, 1000, 2, impact.GainImpact.Changes[0].Before.DisposedBasis, "basis before")
	requireScaled(t, 6000, 2, impact.GainImpact.Changes[0].After.DisposedBasis, "basis after")
	requireScaled(t, 9000, 2, impact.GainImpact.Changes[0].After.Gain, "gain after")

	_, err = f.investmentService.ReplaceReinvestedDividend(ctx, input)
	require.ErrorIs(t, err, ErrGainImpactAcknowledgementRequired)
	input.GainImpactAcknowledgement = "stale"
	_, err = f.investmentService.ReplaceReinvestedDividend(ctx, input)
	require.ErrorIs(t, err, ErrGainImpactAcknowledgementStale)
	require.Equal(t, before, buyReplacementPreviewSnapshot(t, f.database), "refusals write nothing")

	input.GainImpactAcknowledgement = impact.GainImpact.Acknowledgement
	result, err := f.investmentService.ReplaceReinvestedDividend(ctx, input)
	require.NoError(t, err)
	require.Equal(t, original.Transaction.ID, result.CorrectedTransactionID)
	require.Equal(t, original.Transaction.ID, *result.Inverse.CorrectionOfTransactionID)
	for entryIndex, entry := range original.Transaction.JournalEntries {
		for postingIndex, posting := range entry.Postings {
			require.Equal(t, posting.QuantityValue.Negated(), result.Inverse.JournalEntries[entryIndex].Postings[postingIndex].QuantityValue)
		}
	}
	require.Equal(t, int64(-4000), incomeBalance(t, f), "income is the replacement amount only")

	gains, err := f.investmentService.ListRealizedGains(ctx, GainsReportParams{})
	require.NoError(t, err)
	require.Len(t, gains, 1)
	assertMoneyValue(t, 9000, 2, gains[0].RealizedGainValue, gains[0].RealizedGainScale, "replayed sale")

	var kind, eventKind, operation string
	require.NoError(t, f.database.QueryRow(`SELECT o.operation_kind, e.event_kind, audit.operation
		FROM investment_operations o JOIN investment_lots l ON l.operation_id = o.id
		JOIN investment_lot_events e ON e.lot_id = l.id
		JOIN audit_events audit ON audit.id = o.created_audit_event_id
		WHERE o.correction_of_operation_id IS NOT NULL ORDER BY e.id LIMIT 1`).Scan(&kind, &eventKind, &operation))
	require.Equal(t, "reinvested_dividend", kind)
	require.Equal(t, "reinvested_dividend", eventKind)
	require.Equal(t, "investment.reinvested_dividend.replace", operation)
	var voided int
	require.NoError(t, f.database.QueryRow(`SELECT count(*) FROM price_observations
		WHERE source_transaction_version_id = ? AND voided_at IS NOT NULL`, original.Transaction.VersionID).Scan(&voided))
	require.Equal(t, 1, voided, "the original implied price is retired")
	chain, err := f.investmentService.CorrectionChain(ctx, f.ownerUserID, original.Transaction.ID)
	require.NoError(t, err)
	require.Len(t, chain.Operations, 2)
	require.Equal(t, result.Replacement.Transaction.ID, *chain.EffectiveTransactionID)
	requireInvestmentSelfCheckPasses(t, f)

	_, err = f.investmentService.ReplaceReinvestedDividend(ctx, input)
	require.ErrorIs(t, err, ErrInvestmentReinvestmentAlreadyCorrected)
}

func TestReverseReinvestedDividendReplaysDependentSale(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := newInvestmentsTestFixture(t)
	original := reinvestmentWithLaterSale(t, f)
	input := ReverseReinvestedDividendInput{OwnerUserID: f.ownerUserID, TransactionID: original.Transaction.ID,
		Reason: "the fund paid cash, not shares"}
	impact, err := f.investmentService.ReverseReinvestedDividendReconciliationImpact(ctx, input)
	require.NoError(t, err)
	require.Len(t, impact.GainImpact.Changes, 1)
	requireScaled(t, 14000, 2, impact.GainImpact.Changes[0].Before.Gain, "gain before")
	requireScaled(t, 5000, 2, impact.GainImpact.Changes[0].After.Gain, "the sale reselects the bought shares")
	_, err = f.investmentService.ReverseReinvestedDividend(ctx, input)
	require.ErrorIs(t, err, ErrGainImpactAcknowledgementRequired)

	input.GainImpactAcknowledgement = impact.GainImpact.Acknowledgement
	reversal, err := f.investmentService.ReverseReinvestedDividend(ctx, input)
	require.NoError(t, err)
	require.Equal(t, original.Transaction.ID, *reversal.CorrectionOfTransactionID)
	require.Zero(t, incomeBalance(t, f), "reversed income nets to zero")
	chain, err := f.investmentService.CorrectionChain(ctx, f.ownerUserID, original.Transaction.ID)
	require.NoError(t, err)
	require.Nil(t, chain.EffectiveTransactionID)
	require.Equal(t, "reverse", chain.Operations[1].CorrectionMode)
	lots, err := f.investmentService.ListLots(ctx, f.holdingAccountID, f.stockCommodityID)
	require.NoError(t, err)
	remaining := exact.ScaledIntFromInt64(0, 0)
	for _, lot := range lots {
		quantity := exact.ScaledIntFromCoefficient(lot.RemainingQuantityValue, lot.QuantityScale)
		remaining.AddScaled(quantity)
	}
	require.Zero(t, remaining.Cmp(exact.ScaledIntFromInt64(5, 0)), "only the bought shares remain")
	requireInvestmentSelfCheckPasses(t, f)
}

func TestReverseReinvestedDividendImpossibleThroughLaterDisposalRollsBack(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := newInvestmentsTestFixture(t)
	original := reinvestOn(t, f, "2026-01-01", 10, 2000)
	_, err := f.investmentService.Sell(ctx, sellInput(f, "2026-02-01", 5))
	require.NoError(t, err)
	before := buyReplacementPreviewSnapshot(t, f.database)
	input := ReverseReinvestedDividendInput{OwnerUserID: f.ownerUserID, TransactionID: original.Transaction.ID,
		Reason: "duplicate", GainImpactAcknowledgement: "any"}
	_, err = f.investmentService.ReverseReinvestedDividendReconciliationImpact(ctx, input)
	require.ErrorIs(t, err, ErrInvestmentBuyDependency)
	_, err = f.investmentService.ReverseReinvestedDividend(ctx, input)
	require.ErrorIs(t, err, ErrInvestmentBuyDependency)
	// Shrinking it below the sold quantity is just as impossible.
	_, err = f.investmentService.ReplaceReinvestedDividendReconciliationImpact(ctx, ReplaceReinvestedDividendInput{
		OwnerUserID: f.ownerUserID, TransactionID: original.Transaction.ID, Reason: "fewer shares",
		Replacement: reinvestmentInput(f, "2026-01-01", 4, 2000)})
	require.ErrorIs(t, err, ErrInvestmentBuyDependency)
	require.Equal(t, before, buyReplacementPreviewSnapshot(t, f.database))
}

func TestReinvestedDividendCorrectionRequiresReconciliationOverride(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := newInvestmentsTestFixture(t)
	original := reinvestOn(t, f, "2026-01-01", 10, 2000)
	checkpointID := reconcileHolding(t, f, "2026-02-01", 10)
	input := ReplaceReinvestedDividendInput{OwnerUserID: f.ownerUserID, TransactionID: original.Transaction.ID,
		Reason: "broker restated", Replacement: reinvestmentInput(f, "2026-01-01", 12, 2400)}
	impact, err := f.investmentService.ReplaceReinvestedDividendReconciliationImpact(ctx, input)
	require.NoError(t, err)
	require.Len(t, impact.AffectedCheckpoints, 1)
	require.Equal(t, checkpointID, impact.AffectedCheckpoints[0].CheckpointID)
	_, err = f.investmentService.ReplaceReinvestedDividend(ctx, input)
	require.ErrorIs(t, err, ErrReconciliationOverrideRequired)
	input.ReconciliationOverride = true
	_, err = f.investmentService.ReplaceReinvestedDividend(ctx, input)
	require.NoError(t, err)
	var status string
	require.NoError(t, f.database.QueryRow(`SELECT status FROM reconciliation_checkpoints WHERE id = ?`, checkpointID).Scan(&status))
	require.Equal(t, "invalidated", status)
	requireInvestmentSelfCheckPasses(t, f)
}

// A failure after the journals, replay and price retirement — at checkpoint
// invalidation — leaves no partial correction, and a retry succeeds.
func TestReinvestedDividendReplacementRollsBackLateFailure(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := newInvestmentsTestFixture(t)
	original := reinvestmentWithLaterSale(t, f)
	reconcileHolding(t, f, "2026-04-01", 15)
	// The replacement changes the reconciled holding quantity, so the command
	// reaches checkpoint invalidation; an amount-only correction nets to zero
	// in the holding and leaves the checkpoint active (T-120 #135).
	input := ReplaceReinvestedDividendInput{OwnerUserID: f.ownerUserID, TransactionID: original.Transaction.ID,
		Reason: "broker restated", ReconciliationOverride: true, Replacement: reinvestmentInput(f, "2026-01-01", 11, 4000)}
	impact, err := f.investmentService.ReplaceReinvestedDividendReconciliationImpact(ctx, input)
	require.NoError(t, err)
	input.GainImpactAcknowledgement = impact.GainImpact.Acknowledgement
	before := buyReplacementPreviewSnapshot(t, f.database)
	_, err = f.database.Exec(`CREATE TRIGGER reject_reinvestment_checkpoint AFTER UPDATE ON reconciliation_checkpoints
		WHEN NEW.status = 'invalidated' BEGIN SELECT RAISE(ABORT, 'forced checkpoint failure'); END`)
	require.NoError(t, err)
	_, err = f.investmentService.ReplaceReinvestedDividend(ctx, input)
	require.ErrorContains(t, err, "forced checkpoint failure")
	_, err = f.database.Exec(`DROP TRIGGER reject_reinvestment_checkpoint`)
	require.NoError(t, err)
	require.Equal(t, before, buyReplacementPreviewSnapshot(t, f.database))
	_, err = f.investmentService.ReplaceReinvestedDividend(ctx, input)
	require.NoError(t, err)
	requireInvestmentSelfCheckPasses(t, f)
}

func TestReinvestmentAndBuyCorrectionsStayInTheirFamilies(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := newInvestmentsTestFixture(t)
	reinvestment := reinvestOn(t, f, "2026-01-01", 10, 2000)
	buy := buyOn(t, f, "2026-02-01", 10, 20000)
	_, err := f.investmentService.ReverseBuyReconciliationImpact(ctx, ReverseInvestmentBuyInput{
		OwnerUserID: f.ownerUserID, TransactionID: reinvestment.Transaction.ID, Reason: "wrong command"})
	require.ErrorIs(t, err, ErrInvestmentBuyNotFound)
	_, err = f.investmentService.ReverseReinvestedDividendReconciliationImpact(ctx, ReverseReinvestedDividendInput{
		OwnerUserID: f.ownerUserID, TransactionID: buy.Transaction.ID, Reason: "wrong command"})
	require.ErrorIs(t, err, ErrInvestmentReinvestmentNotFound)
	moved := reinvestmentInput(f, "2026-01-02", 10, 2000)
	_, err = f.investmentService.ReplaceReinvestedDividendReconciliationImpact(ctx, ReplaceReinvestedDividendInput{
		OwnerUserID: f.ownerUserID, TransactionID: reinvestment.Transaction.ID, Reason: "moved", Replacement: moved})
	var validation ValidationError
	require.ErrorAs(t, err, &validation, "date changes are a separate correction family")
}

func reconcileHolding(t *testing.T, f *investmentsTestFixture, statementDate string, balance int64) int64 {
	t.Helper()
	ctx := context.Background()
	session, err := f.transactionService.StartReconciliation(ctx, StartReconciliationInput{
		OriginType: "browser_api", OwnerUserID: f.ownerUserID, AccountID: f.holdingAccountID,
		CommodityID: f.stockCommodityID, StatementDate: statementDate, StatementBalanceValue: exact.New(balance),
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
		OriginType: "browser_api", OwnerUserID: f.ownerUserID, SessionID: session.ID, ChangeReason: "statement verified",
	})
	require.NoError(t, err)
	checkpoints, err := f.transactionService.repository.ListReconciliationCheckpoints(ctx, BookID, f.holdingAccountID, f.stockCommodityID)
	require.NoError(t, err)
	for _, checkpoint := range checkpoints {
		if checkpoint.Status == "active" {
			return checkpoint.ID
		}
	}
	t.Fatal("no active holding checkpoint after finishing a reconciliation")
	return 0
}
