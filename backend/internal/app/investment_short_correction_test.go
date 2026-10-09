package app

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// #175: native correction of named shorts. A short sale is corrected like a
// buy and a cover like a sale; both replay the short position, preserve the
// original journal and lot facts, disclose changed cover results, and name a
// later cover the correction would strand.

func shortOpenLots(t *testing.T, f *investmentsTestFixture) int {
	t.Helper()
	var open int
	require.NoError(t, f.database.QueryRow(`SELECT COUNT(*) FROM current_investment_lots
		WHERE position_side = 'short' AND status = 'open'`).Scan(&open))
	return open
}

// reverseShortCoverAcknowledged previews the reversal, which removes the
// cover's committed result, and commits it with that acknowledgement.
func reverseShortCoverAcknowledged(t *testing.T, f *investmentsTestFixture, transactionID int64, reason string) {
	t.Helper()
	ctx := context.Background()
	input := ReverseInvestmentSaleInput{OwnerUserID: f.ownerUserID, TransactionID: transactionID, Reason: reason}
	_, err := f.investmentService.ReverseShortCover(ctx, input)
	require.ErrorIs(t, err, ErrGainImpactAcknowledgementRequired, "removing a realized cover result is disclosed")
	impact, err := f.investmentService.ReverseShortCoverReconciliationImpact(ctx, input)
	require.NoError(t, err)
	require.NotNil(t, impact.GainImpact)
	require.Len(t, impact.GainImpact.Changes, 1)
	require.Nil(t, impact.GainImpact.Changes[0].After, "the cover's result is removed")
	input.GainImpactAcknowledgement = impact.GainImpact.Acknowledgement
	_, err = f.investmentService.ReverseShortCover(ctx, input)
	require.NoError(t, err)
}

func TestReverseShortCoverRestoresOwedUnits(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	shortSaleOn(t, f, "2026-03-02", 10, 10000)
	cover, err := f.investmentService.ShortCover(ctx, coverInput(f, "2026-05-01", 10, 7000, "fifo"))
	require.NoError(t, err)
	chain, err := f.investmentService.CorrectionChain(ctx, f.ownerUserID, cover.Transaction.ID)
	require.NoError(t, err)
	require.True(t, chain.CanCorrectShortCover)
	require.False(t, chain.CanReverseSale, "a sale command never reverses a cover")
	require.Zero(t, shortOpenLots(t, f))

	_, err = f.investmentService.ReverseSale(ctx, ReverseInvestmentSaleInput{OwnerUserID: f.ownerUserID,
		TransactionID: cover.Transaction.ID, Reason: "wrong command"})
	require.ErrorIs(t, err, ErrInvestmentSaleNotFound, "the sale family is fenced from covers")

	reverseShortCoverAcknowledged(t, f, cover.Transaction.ID, "cover was entered twice")
	assert.Equal(t, 1, shortOpenLots(t, f), "the ten borrowed units are owed again")
	gains, err := f.investmentService.ListRealizedGains(ctx, GainsReportParams{})
	require.NoError(t, err)
	assert.Empty(t, gains, "a reversed cover realizes nothing")
	chain, err = f.investmentService.CorrectionChain(ctx, f.ownerUserID, cover.Transaction.ID)
	require.NoError(t, err)
	assert.False(t, chain.CanCorrectShortCover)
	shortSelfCheckPasses(t, f)
}

func TestReverseShortSaleNamesTheCoverThatNeedsIt(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	opening := shortSaleOn(t, f, "2026-03-02", 10, 10000)
	cover, err := f.investmentService.ShortCover(ctx, coverInput(f, "2026-05-01", 10, 7000, "fifo"))
	require.NoError(t, err)
	_, err = f.investmentService.ReverseBuy(ctx, ReverseInvestmentBuyInput{OwnerUserID: f.ownerUserID,
		TransactionID: opening.Transaction.ID, Reason: "wrong command"})
	require.ErrorIs(t, err, ErrInvestmentBuyNotFound, "the buy family is fenced from short sales")

	before := buyReplacementPreviewSnapshot(t, f.database)
	input := ReverseInvestmentBuyInput{OwnerUserID: f.ownerUserID, TransactionID: opening.Transaction.ID, Reason: "no such trade"}
	_, err = f.investmentService.ReverseShortSaleReconciliationImpact(ctx, input)
	var dependency InvestmentShortDependencyError
	require.ErrorAs(t, err, &dependency)
	_, err = f.investmentService.ReverseShortSale(ctx, input)
	require.ErrorAs(t, err, &dependency)
	assert.Equal(t, *cover.DisposalDecision.ID, dependency.DecisionID)
	require.Equal(t, before, buyReplacementPreviewSnapshot(t, f.database), "nothing written")

	reverseShortCoverAcknowledged(t, f, cover.Transaction.ID, "no such trade")
	_, err = f.investmentService.ReverseShortSale(ctx, input)
	require.NoError(t, err)
	assert.Zero(t, shortOpenLots(t, f))
	positions, err := f.investmentService.Positions(ctx)
	require.NoError(t, err)
	assert.Empty(t, positions)
	shortSelfCheckPasses(t, f)
}

func TestReplaceShortSaleRevisesLaterCoverWithAcknowledgement(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	opening := shortSaleOn(t, f, "2026-03-02", 10, 10000)
	_, err := f.investmentService.ShortCover(ctx, coverInput(f, "2026-05-01", 4, 3000, "fifo"))
	require.NoError(t, err)

	input := ReplaceInvestmentBuyInput{OwnerUserID: f.ownerUserID, TransactionID: opening.Transaction.ID,
		Reason: "proceeds were 120.00", Replacement: shortInput(f, "2026-03-02", 10, 12000)}
	before := buyReplacementPreviewSnapshot(t, f.database)
	impact, err := f.investmentService.ReplaceShortSaleReconciliationImpact(ctx, input)
	require.NoError(t, err)
	require.Equal(t, before, buyReplacementPreviewSnapshot(t, f.database))
	require.NotNil(t, impact.GainImpact)
	require.Len(t, impact.GainImpact.Changes, 1)
	requireScaled(t, 1000, 2, impact.GainImpact.Changes[0].Before.Gain, "40.00 − 30.00")
	requireScaled(t, 1800, 2, impact.GainImpact.Changes[0].After.Gain, "48.00 − 30.00")

	_, err = f.investmentService.ReplaceShortSale(ctx, input)
	require.ErrorIs(t, err, ErrGainImpactAcknowledgementRequired)
	require.Equal(t, before, buyReplacementPreviewSnapshot(t, f.database))
	input.GainImpactAcknowledgement = impact.GainImpact.Acknowledgement
	result, err := f.investmentService.ReplaceShortSale(ctx, input)
	require.NoError(t, err)
	require.NotNil(t, result.Replacement.LotID)
	var side string
	require.NoError(t, f.database.QueryRow(`SELECT position_side FROM investment_lots WHERE id = ?`, *result.Replacement.LotID).Scan(&side))
	assert.Equal(t, "short", side)
	may := shortGainOn(t, f, "2026-05-01")
	assertMoneyValue(t, 1800, 2, may.RealizedGainValue, may.RealizedGainScale, "committed")
	var originals int
	require.NoError(t, f.database.QueryRow(`SELECT COUNT(*) FROM investment_lots WHERE position_side = 'short'`).Scan(&originals))
	assert.Equal(t, 2, originals, "the original opening lot stays as evidence")
	shortSelfCheckPasses(t, f)
}

func TestReplaceShortCoverChangesMethodAmountAndDate(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	shortSaleOn(t, f, "2026-03-02", 5, 5000) // 10.00 per unit
	shortSaleOn(t, f, "2026-03-16", 5, 7500) // 15.00 per unit
	cover, err := f.investmentService.ShortCover(ctx, coverInput(f, "2026-05-01", 5, 4000, "fifo"))
	require.NoError(t, err)
	_, err = f.investmentService.ShortCover(ctx, coverInput(f, "2026-06-01", 5, 4000, "fifo"))
	require.NoError(t, err)

	// LIFO in April: the May slot now takes the 15.00 lot, so June's FIFO
	// cover takes the 10.00 one.
	input := ReplaceInvestmentSaleInput{OwnerUserID: f.ownerUserID, TransactionID: cover.Transaction.ID,
		Reason: "it was a LIFO cover in April", Replacement: coverInput(f, "2026-04-01", 5, 3500, "lifo")}
	impact, err := f.investmentService.ReplaceShortCoverReconciliationImpact(ctx, input)
	require.NoError(t, err)
	input.GainImpactAcknowledgement = impact.GainImpact.Acknowledgement
	_, err = f.investmentService.ReplaceShortCover(ctx, input)
	require.NoError(t, err)
	gains := gainsByDate(t, f)
	require.NotContains(t, gains, "2026-05-01")
	assertMoneyValue(t, 4000, 2, gains["2026-04-01"].RealizedGainValue, gains["2026-04-01"].RealizedGainScale, "75.00 − 35.00")
	assertMoneyValue(t, 1000, 2, gains["2026-06-01"].RealizedGainValue, gains["2026-06-01"].RealizedGainScale, "50.00 − 40.00")
	shortSelfCheckPasses(t, f)
}

func TestGenericMutationCannotStrandShortOperations(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	opening := shortSaleOn(t, f, "2026-03-02", 10, 10000)
	cover, err := f.investmentService.ShortCover(ctx, coverInput(f, "2026-05-01", 4, 3000, "fifo"))
	require.NoError(t, err)
	before := buyReplacementPreviewSnapshot(t, f.database)
	for _, transaction := range []Transaction{opening.Transaction, cover.Transaction} {
		_, err := f.transactionService.VoidTransaction(ctx, VoidTransactionInput{OwnerUserID: f.ownerUserID,
			TransactionID: transaction.ID, ChangeReason: "must be fenced"})
		require.ErrorIs(t, err, ErrInvestmentWorkflowRequired)
		_, err = f.transactionService.UpdateTransaction(ctx, UpdateTransactionInput{OwnerUserID: f.ownerUserID,
			TransactionID: transaction.ID, Spec: transactionInputFromTransaction(transaction), ChangeReason: "must be fenced"})
		require.ErrorIs(t, err, ErrInvestmentWorkflowRequired)
	}
	require.Equal(t, before, buyReplacementPreviewSnapshot(t, f.database))
}
