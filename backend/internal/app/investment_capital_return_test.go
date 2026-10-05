package app

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rekenraam/backend/internal/db"
	"rekenraam/backend/internal/exact"
)

// T-146: return of capital. Cash posts on the payment date; every lot open on
// the effective date loses min(allocated, basis) of basis, and the rest is an
// unresolved excess.

func capitalReturnInput(f *investmentsTestFixture, effectiveOn string, amount int64) CapitalReturnInput {
	return CapitalReturnInput{OwnerUserID: f.ownerUserID, HoldingAccountID: f.holdingAccountID,
		CommodityID: f.stockCommodityID, CashAccountID: f.cashAccountID, CurrencyID: f.eurCommodityID,
		EffectiveOn: effectiveOn, PaymentOn: effectiveOn, AmountValue: exact.New(amount), AmountScale: 2,
		SourceEvidenceJSON: `{"notice":"ROC 2026"}`, Memo: "return of capital"}
}

func coefScaled(value exact.Coefficient, scale int) *exact.ScaledInt {
	return exact.ScaledIntFromCoefficient(value, scale)
}

// The contract example: 10.00 on one lot with 7.00 basis reduces it to zero
// and leaves 3.00 unresolved; the cash posts the full 10.00.
func TestCapitalReturnBeyondBasisReducesToZeroAndRecordsExcess(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	buy := buyOn(t, f, "2026-05-01", 1, 700)
	result, err := f.investmentService.CapitalReturn(ctx, capitalReturnInput(f, "2026-06-01", 1000))
	require.NoError(t, err)
	require.Len(t, result.Effects, 1)
	effect := result.Effects[0]
	assert.Equal(t, *buy.LotID, effect.LotID)
	requireScaled(t, 1000, 2, coefScaled(effect.AllocatedValue, effect.AllocatedScale), "allocated")
	requireScaled(t, 700, 2, coefScaled(effect.ReductionValue, effect.ReductionScale), "reduction is the basis")
	requireScaled(t, 300, 2, coefScaled(effect.ExcessValue, effect.ExcessScale), "unresolved excess")
	assert.Zero(t, lotRemainingBasis(t, f, *buy.LotID).Sign(), "basis floors at zero")

	require.Len(t, result.Transaction.JournalEntries, 1)
	postings := result.Transaction.JournalEntries[0].Postings
	require.Len(t, postings, 2)
	assert.Equal(t, f.cashAccountID, postings[0].AccountID)
	assert.Equal(t, "1000", postings[0].QuantityValue.String(), "the full receipt posts to cash")
	var decisions int
	require.NoError(t, f.database.QueryRow(`SELECT COUNT(*) FROM investment_disposal_decisions`).Scan(&decisions))
	assert.Zero(t, decisions, "not a disposal, not income")
	requireInvestmentSelfCheckPasses(t, f)
}

// Per share across lots: truncated shares with the exact remainder on the
// last lot in stable order, summing to the receipt.
func TestCapitalReturnAllocatesPerShareWithExactRemainder(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	first := buyOn(t, f, "2026-05-01", 1, 5000)
	second := buyOn(t, f, "2026-05-02", 2, 10000)
	result, err := f.investmentService.CapitalReturn(ctx, capitalReturnInput(f, "2026-06-01", 1000))
	require.NoError(t, err)
	require.Len(t, result.Effects, 2)
	requireScaled(t, 333, 2, coefScaled(result.Effects[0].AllocatedValue, result.Effects[0].AllocatedScale), "one share of three")
	requireScaled(t, 667, 2, coefScaled(result.Effects[1].AllocatedValue, result.Effects[1].AllocatedScale), "remainder on the last lot")
	total := exact.NewScaledInt()
	for _, effect := range result.Effects {
		total.AddCoefficient(effect.AllocatedValue, effect.AllocatedScale)
		assert.Zero(t, effect.ExcessValue.Sign(), "basis covers each share")
	}
	requireScaled(t, 1000, 2, total, "allocations sum to the receipt")
	requireScaled(t, 4667, 2, lotRemainingBasis(t, f, *first.LotID), "first lot")
	requireScaled(t, 9333, 2, lotRemainingBasis(t, f, *second.LotID), "second lot")
	requireInvestmentSelfCheckPasses(t, f)
}

func TestLaterSaleRealizesGainOnReducedBasis(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	buyOn(t, f, "2026-05-01", 2, 2000)
	_, err := f.investmentService.CapitalReturn(ctx, capitalReturnInput(f, "2026-06-01", 400))
	require.NoError(t, err)
	sold, err := f.investmentService.Sell(ctx, sellInput(f, "2026-07-01", 1))
	require.NoError(t, err)
	requireScaled(t, 800, 2, saleEffectiveBasis(t, f, sold.Transaction.ID), "half of 20.00 less 4.00")
	requireInvestmentSelfCheckPasses(t, f)
}

func TestCapitalReturnPreviewWritesNothing(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	buy := buyOn(t, f, "2026-05-01", 1, 700)
	before := f.transactionCount(t)
	preview, err := f.investmentService.PreviewCapitalReturn(context.Background(), capitalReturnInput(f, "2026-06-01", 1000))
	require.NoError(t, err)
	require.Len(t, preview.Effects, 1)
	requireScaled(t, 300, 2, coefScaled(preview.Effects[0].ExcessValue, preview.Effects[0].ExcessScale), "previewed excess")
	assert.Equal(t, before, f.transactionCount(t))
	requireScaled(t, 700, 2, lotRemainingBasis(t, f, *buy.LotID), "basis untouched")
}

func TestCapitalReturnWithoutHoldingsIsRefused(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	buyOn(t, f, "2026-07-01", 1, 700)
	_, err := f.investmentService.CapitalReturn(context.Background(), capitalReturnInput(f, "2026-06-01", 1000))
	require.ErrorIs(t, err, ErrCapitalReturnNoHoldings, "the only lot opens after the effective date")
}

func TestCapitalReturnBehindLaterDepletionIsRefused(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	buyOn(t, f, "2026-05-01", 2, 2000)
	_, err := f.investmentService.Sell(context.Background(), sellInput(f, "2026-07-01", 1))
	require.NoError(t, err)
	_, err = f.investmentService.CapitalReturn(context.Background(), capitalReturnInput(f, "2026-06-01", 400))
	require.ErrorIs(t, err, db.ErrOutOfOrderPositionEvent)
}

// Until revisions ship (T-148), history that would change the reduction or
// excess is refused with the return of capital named, and nothing is written.
func TestBuyCorrectionThatChangesCapitalReturnIsRefusedWithItNamed(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	buy := buyOn(t, f, "2026-05-01", 1, 700)
	roc, err := f.investmentService.CapitalReturn(ctx, capitalReturnInput(f, "2026-06-01", 1000))
	require.NoError(t, err)
	before := f.transactionCount(t)
	_, err = acknowledgedReplaceBuy(ctx, f.investmentService, replaceBuyPrice(f, buy, "2026-05-01", 1, 500))
	require.ErrorIs(t, err, ErrInvestmentBuyDependency)
	var dependency InvestmentBuyDependencyError
	require.ErrorAs(t, err, &dependency)
	assert.Equal(t, transferOperationID(t, f, roc.Transaction.ID), dependency.OperationID)
	assert.Equal(t, before, f.transactionCount(t))
	requireInvestmentSelfCheckPasses(t, f)
}

// A later sale's reversal replays the position through the return of capital
// unchanged.
func TestCapitalReturnReplaysUnchangedThroughLaterSaleReversal(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	buyOn(t, f, "2026-05-01", 2, 2000)
	_, err := f.investmentService.CapitalReturn(ctx, capitalReturnInput(f, "2026-06-01", 400))
	require.NoError(t, err)
	sold, err := f.investmentService.Sell(ctx, sellInput(f, "2026-07-01", 1))
	require.NoError(t, err)
	_, err = acknowledgedReverseSale(ctx, f.investmentService, ReverseInvestmentSaleInput{
		OwnerUserID: f.ownerUserID, TransactionID: sold.Transaction.ID, Reason: "duplicate"})
	require.NoError(t, err)
	quantity, basis := openPositionBasis(t, f, f.holdingAccountID)
	requireScaled(t, 2, 0, quantity, "units restored")
	requireScaled(t, 1600, 2, basis, "basis stays reduced")
	requireInvestmentSelfCheckPasses(t, f)
}

func TestSelfCheckDetectsCapitalReturnThatDoesNotConserveItsReceipt(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	buyOn(t, f, "2026-05-01", 1, 700)
	_, err := f.investmentService.CapitalReturn(context.Background(), capitalReturnInput(f, "2026-06-01", 1000))
	require.NoError(t, err)
	_, err = f.database.Exec(`DROP TRIGGER investment_capital_return_effects_no_update`)
	require.NoError(t, err)
	_, err = f.database.Exec(`UPDATE investment_capital_return_effects SET excess_value = '200'`)
	require.NoError(t, err)
	run, err := selfCheckOver(t, f.database).RunSelfCheck(context.Background(), "manual")
	require.NoError(t, err)
	result := resultFor(t, run, CheckInvestmentFoundation)
	assert.Equal(t, SelfCheckFailed, result.Status)
	assert.Contains(t, result.Summary, "returns of capital do not conserve")
}
