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
	requireScaled(t, 3333333, 6, coefScaled(result.Effects[0].AllocatedValue, result.Effects[0].AllocatedScale), "one share of three")
	requireScaled(t, 6666667, 6, coefScaled(result.Effects[1].AllocatedValue, result.Effects[1].AllocatedScale), "remainder on the last lot")
	total := exact.NewScaledInt()
	for _, effect := range result.Effects {
		total.AddCoefficient(effect.AllocatedValue, effect.AllocatedScale)
		assert.Zero(t, effect.ExcessValue.Sign(), "basis covers each share")
	}
	requireScaled(t, 1000, 2, total, "allocations sum to the receipt")
	requireScaled(t, 46666667, 6, lotRemainingBasis(t, f, *first.LotID), "first lot")
	requireScaled(t, 93333333, 6, lotRemainingBasis(t, f, *second.LotID), "second lot")
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

// T-148: a return of capital dated behind a later sale is admitted through
// replay: the sale's basis is revised and its gain change needs the preview's
// acknowledgement.
func TestBackdatedCapitalReturnRevisesLaterSaleUnderAcknowledgement(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	buyOn(t, f, "2026-05-01", 2, 2000)
	sold, err := f.investmentService.Sell(ctx, sellInput(f, "2026-07-01", 1))
	require.NoError(t, err)
	requireScaled(t, 1000, 2, saleEffectiveBasis(t, f, sold.Transaction.ID), "before the return")

	input := capitalReturnInput(f, "2026-06-01", 400)
	_, err = f.investmentService.CapitalReturn(ctx, input)
	require.ErrorIs(t, err, db.ErrGainImpactAcknowledgementRequired)
	preview, err := f.investmentService.PreviewCapitalReturn(ctx, input)
	require.NoError(t, err)
	require.NotNil(t, preview.Impact.GainImpact)
	input.GainImpactAcknowledgement = preview.Impact.GainImpact.Acknowledgement
	result, err := f.investmentService.CapitalReturn(ctx, input)
	require.NoError(t, err)
	require.Len(t, result.Effects, 1)
	requireScaled(t, 400, 2, coefScaled(result.Effects[0].ReductionValue, result.Effects[0].ReductionScale), "reduction at its slot")
	requireScaled(t, 800, 2, saleEffectiveBasis(t, f, sold.Transaction.ID), "the later sale takes the reduced basis")
	quantity, basis := openPositionBasis(t, f, f.holdingAccountID)
	requireScaled(t, 1, 0, quantity, "one share left")
	requireScaled(t, 800, 2, basis, "at its reduced basis")
	requireInvestmentSelfCheckPasses(t, f)
}

// capitalReturnRevisionCount counts a return of capital's revisions.
func capitalReturnRevisionCount(t *testing.T, f *investmentsTestFixture) int {
	t.Helper()
	var n int
	require.NoError(t, f.database.QueryRow(`SELECT COUNT(*) FROM investment_capital_return_revisions`).Scan(&n))
	return n
}

// T-148: history that changes the reduction/excess split revises the effects;
// the receipt and the original effects stay immutable evidence.
func TestBuyCorrectionRevisesCapitalReturnReductionAndExcess(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	buy := buyOn(t, f, "2026-05-01", 1, 700)
	roc, err := f.investmentService.CapitalReturn(ctx, capitalReturnInput(f, "2026-06-01", 1000))
	require.NoError(t, err)
	transactionsBefore := f.transactionCount(t)

	replaced, err := acknowledgedReplaceBuy(ctx, f.investmentService, replaceBuyPrice(f, buy, "2026-05-01", 1, 500))
	require.NoError(t, err)
	assert.Equal(t, transactionsBefore+2, f.transactionCount(t), "only the buy correction posts; the receipt is unchanged")
	assert.Equal(t, 1, capitalReturnRevisionCount(t, f))
	var reduction, excess exact.Coefficient
	var reductionScale, excessScale int
	var lotID int64
	require.NoError(t, f.database.QueryRow(`SELECT e.lot_id, e.reduction_value, e.reduction_scale, e.excess_value, e.excess_scale
		FROM investment_capital_return_revision_effects e`).Scan(&lotID, &reduction, &reductionScale, &excess, &excessScale))
	assert.Equal(t, *replaced.Replacement.LotID, lotID, "the revision names the corrected lot")
	requireScaled(t, 500, 2, coefScaled(reduction, reductionScale), "reduction follows the corrected basis")
	requireScaled(t, 500, 2, coefScaled(excess, excessScale), "excess grows by the difference")
	assert.Zero(t, lotRemainingBasis(t, f, *replaced.Replacement.LotID).Sign())
	original := roc.Effects[0]
	var storedReduction exact.Coefficient
	var storedScale int
	require.NoError(t, f.database.QueryRow(`SELECT reduction_value, reduction_scale FROM investment_capital_return_effects`).Scan(&storedReduction, &storedScale))
	assert.Zero(t, coefScaled(storedReduction, storedScale).Cmp(coefScaled(original.ReductionValue, original.ReductionScale)),
		"the original effect is immutable evidence")
	requireInvestmentSelfCheckPasses(t, f)
}

// A purchase backdated before the effective date joins the entitled lots, so
// the receipt is re-allocated per share across both.
func TestBackdatedBuyReallocatesCapitalReturnAcrossEntitledLots(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	buyOn(t, f, "2026-05-01", 1, 5000)
	_, err := f.investmentService.CapitalReturn(ctx, capitalReturnInput(f, "2026-06-01", 1000))
	require.NoError(t, err)
	_, err = f.investmentService.Sell(ctx, sellInput(f, "2026-07-01", 1))
	require.NoError(t, err)

	input := InvestmentTradeInput{OwnerUserID: f.ownerUserID, TransactionDate: "2026-04-01",
		CommodityID: f.stockCommodityID, HoldingAccountID: f.holdingAccountID,
		CashAccountID: f.cashAccountID, CashCommodityID: f.eurCommodityID,
		QuantityValue: exact.New(1), CashAmountValue: 5000, CashAmountScale: 2}
	impact, err := f.investmentService.TradeReconciliationImpact(ctx, InvestmentImpactBuy, input)
	require.NoError(t, err)
	if impact.GainImpact != nil {
		input.GainImpactAcknowledgement = impact.GainImpact.Acknowledgement
	}
	_, err = f.investmentService.Buy(ctx, input)
	require.NoError(t, err)
	var effects int
	require.NoError(t, f.database.QueryRow(`SELECT COUNT(*) FROM investment_capital_return_revision_effects`).Scan(&effects))
	assert.Equal(t, 2, effects, "both lots are entitled now")
	requireInvestmentSelfCheckPasses(t, f)
}

func TestSelfCheckReportsStaleCapitalReturnRevision(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	buy := buyOn(t, f, "2026-05-01", 1, 700)
	_, err := f.investmentService.CapitalReturn(ctx, capitalReturnInput(f, "2026-06-01", 1000))
	require.NoError(t, err)
	_, err = acknowledgedReplaceBuy(ctx, f.investmentService, replaceBuyPrice(f, buy, "2026-05-01", 1, 500))
	require.NoError(t, err)
	requireInvestmentSelfCheckPasses(t, f)
	_, err = f.database.Exec(`DROP TRIGGER investment_capital_return_revision_effects_no_update`)
	require.NoError(t, err)
	_, err = f.database.Exec(`UPDATE investment_capital_return_revision_effects SET reduction_value = '400', excess_value = '600'`)
	require.NoError(t, err)
	run, err := selfCheckOver(t, f.database).RunSelfCheck(ctx, "manual")
	require.NoError(t, err)
	replay := resultFor(t, run, CheckInvestmentReplay)
	assert.Equal(t, SelfCheckFailed, replay.Status)
	assert.Contains(t, replay.Summary, "returns of capital with stale effects")
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

// T-148: reversing a return of capital restores the basis it reduced and
// revises later disposals under the preview's acknowledgement.
func TestReverseCapitalReturnRestoresBasisAndRevisesLaterSale(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	buyOn(t, f, "2026-05-01", 2, 2000)
	roc, err := f.investmentService.CapitalReturn(ctx, capitalReturnInput(f, "2026-06-01", 400))
	require.NoError(t, err)
	sold, err := f.investmentService.Sell(ctx, sellInput(f, "2026-07-01", 1))
	require.NoError(t, err)
	requireScaled(t, 800, 2, saleEffectiveBasis(t, f, sold.Transaction.ID), "on the reduced basis")

	input := ReverseInvestmentCapitalReturnInput{OwnerUserID: f.ownerUserID, TransactionID: roc.Transaction.ID,
		Reason: "notice was withdrawn"}
	_, err = f.investmentService.ReverseCapitalReturn(ctx, input)
	require.ErrorIs(t, err, db.ErrGainImpactAcknowledgementRequired)
	impact, err := f.investmentService.ReverseCapitalReturnReconciliationImpact(ctx, input)
	require.NoError(t, err)
	require.NotNil(t, impact.GainImpact)
	input.GainImpactAcknowledgement = impact.GainImpact.Acknowledgement
	inverse, err := f.investmentService.ReverseCapitalReturn(ctx, input)
	require.NoError(t, err)
	require.Len(t, inverse.JournalEntries, 1)
	assert.Equal(t, "-400", inverse.JournalEntries[0].Postings[0].QuantityValue.String(), "the receipt is inverted")
	requireScaled(t, 1000, 2, saleEffectiveBasis(t, f, sold.Transaction.ID), "the sale regains the full basis")
	_, basis := openPositionBasis(t, f, f.holdingAccountID)
	requireScaled(t, 1000, 2, basis, "the remaining share too")
	chain, err := f.investmentService.CorrectionChain(ctx, f.ownerUserID, roc.Transaction.ID)
	require.NoError(t, err)
	assert.False(t, chain.CanReverseCapitalReturn, "not offered twice")
	_, err = f.investmentService.ReverseCapitalReturn(ctx, input)
	require.ErrorIs(t, err, ErrInvestmentCapitalReturnAlreadyCorrected)
	requireInvestmentSelfCheckPasses(t, f)
}

// T-148: an explicit entitlement reduces only the lots the corporate action
// names; others keep their basis.
func TestExplicitEntitlementReducesOnlyNamedLots(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	first := buyOn(t, f, "2026-05-01", 2, 2000)
	second := buyOn(t, f, "2026-05-15", 2, 4000)
	input := capitalReturnInput(f, "2026-06-01", 400)
	input.EntitledLotIDs = []int64{*first.LotID}
	result, err := f.investmentService.CapitalReturn(ctx, input)
	require.NoError(t, err)
	require.Len(t, result.Effects, 1)
	assert.Equal(t, *first.LotID, result.Effects[0].LotID)
	requireScaled(t, 1600, 2, lotRemainingBasis(t, f, *first.LotID), "named lot reduced")
	requireScaled(t, 4000, 2, lotRemainingBasis(t, f, *second.LotID), "unnamed lot untouched")
	var rule string
	require.NoError(t, f.database.QueryRow(`SELECT entitlement_rule FROM investment_capital_return_facts`).Scan(&rule))
	assert.Equal(t, "explicit_lots", rule)
	requireInvestmentSelfCheckPasses(t, f)
}

func TestExplicitEntitlementRefusesLotNotOpenOnEffectiveDate(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	buyOn(t, f, "2026-05-01", 2, 2000)
	later := buyOn(t, f, "2026-06-15", 1, 1000)
	input := capitalReturnInput(f, "2026-06-01", 400)
	input.EntitledLotIDs = []int64{*later.LotID}
	_, err := f.investmentService.CapitalReturn(context.Background(), input)
	var validation ValidationError
	require.ErrorAs(t, err, &validation)
}

// A named lot whose acquisition is corrected is followed to its successor,
// and the reduction is revised on that lot.
func TestExplicitEntitlementFollowsCorrectedAcquisition(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	first := buyOn(t, f, "2026-05-01", 1, 300)
	buyOn(t, f, "2026-05-15", 1, 5000)
	input := capitalReturnInput(f, "2026-06-01", 400)
	input.EntitledLotIDs = []int64{*first.LotID}
	_, err := f.investmentService.CapitalReturn(ctx, input)
	require.NoError(t, err)
	replaced, err := acknowledgedReplaceBuy(ctx, f.investmentService, replaceBuyPrice(f, first, "2026-05-01", 1, 1000))
	require.NoError(t, err)
	var lotID int64
	var reduction exact.Coefficient
	var scale int
	require.NoError(t, f.database.QueryRow(`SELECT lot_id, reduction_value, reduction_scale
		FROM investment_capital_return_revision_effects`).Scan(&lotID, &reduction, &scale))
	assert.Equal(t, *replaced.Replacement.LotID, lotID, "the entitlement follows the corrected lot")
	requireScaled(t, 400, 2, coefScaled(reduction, scale), "now fully within the corrected basis")
	requireInvestmentSelfCheckPasses(t, f)
}
