package app

import (
	"bytes"
	"context"
	"encoding/csv"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rekenraam/backend/internal/db"
	"rekenraam/backend/internal/exact"
)

// #183: a spin-off is corrected by its own reversal and replacement, and a
// spin-off dated behind later activity is admitted by replay at its slot.

func reverseSpinOffInput(f *investmentsTestFixture, transactionID int64) ReverseSpinOffInput {
	return ReverseSpinOffInput{OwnerUserID: f.ownerUserID, TransactionID: transactionID, Reason: "wrong issuer allocation"}
}

func replaceSpinOffInput(f *investmentsTestFixture, transactionID, newID int64, date string, numerator, denominator int64,
	fraction string, fractionScale int) ReplaceSpinOffInput {
	return ReplaceSpinOffInput{OwnerUserID: f.ownerUserID, TransactionID: transactionID, Reason: "wrong issuer allocation",
		EffectiveOn: date, DestinationCommodityID: newID, RatioNumerator: numerator, RatioDenominator: denominator,
		BasisFractionValue: exact.Coefficient(fraction), BasisFractionScale: fractionScale, Memo: "spin-off"}
}

// reconcileInstrument reconciles one instrument of the holding account.
func reconcileInstrument(t *testing.T, f *investmentsTestFixture, commodityID int64, statementDate string, balance int64) {
	t.Helper()
	ctx := context.Background()
	session, err := f.transactionService.StartReconciliation(ctx, StartReconciliationInput{
		OriginType: "browser_api", OwnerUserID: f.ownerUserID, AccountID: f.holdingAccountID,
		CommodityID: commodityID, StatementDate: statementDate, StatementBalanceValue: exact.New(balance),
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
}

func auditEventCount(t *testing.T, f *investmentsTestFixture) int {
	t.Helper()
	var count int
	require.NoError(t, f.database.QueryRow(`SELECT COUNT(*) FROM audit_events`).Scan(&count))
	return count
}

func TestSpinOffReversalRestoresParentBasisAndRetiresTheNewLots(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	newID := seedTestSecurityCommodity(t, f, "SPINCO")
	first := buyOn(t, f, "2026-02-01", 60, 60000)
	second := buyOn(t, f, "2026-03-01", 40, 40000)
	spun, err := f.investmentService.SpinOff(ctx, spinOffInput(f, newID, "2026-06-01", 1, 2, "2", 1))
	require.NoError(t, err)
	requireScaled(t, 48000, 2, lotRemainingBasis(t, f, *first.LotID), "the first lot gave up 20 %")
	transactionsBefore, auditsBefore := f.transactionCount(t), auditEventCount(t, f)

	// The generic transfer and exchange corrections do not reach a spin-off.
	_, err = f.investmentService.ReverseTransfer(ctx, reverseTransferInput(f, spun.Transaction.ID))
	require.ErrorIs(t, err, ErrInvestmentTransferNotFound)
	_, err = f.investmentService.ReverseShareExchange(ctx, reverseShareExchangeInput(f, spun.Transaction.ID))
	require.ErrorIs(t, err, ErrShareExchangeNotFound)
	chain, err := f.investmentService.CorrectionChain(ctx, f.ownerUserID, spun.Transaction.ID)
	require.NoError(t, err)
	assert.True(t, chain.CanCorrectSpinOff)
	assert.False(t, chain.CanCorrectShareExchange)

	inverse, err := f.investmentService.ReverseSpinOff(ctx, reverseSpinOffInput(f, spun.Transaction.ID))
	require.NoError(t, err)

	assert.Equal(t, "2026-06-01", inverse.TransactionDate, "the inverse keeps the spin-off date")
	require.NotNil(t, inverse.CorrectionOfTransactionID)
	assert.Equal(t, spun.Transaction.ID, *inverse.CorrectionOfTransactionID)
	assert.Equal(t, transactionsBefore+1, f.transactionCount(t))
	assert.Equal(t, auditsBefore+1, auditEventCount(t, f), "one audit event for the whole correction")
	assert.Equal(t, "100", positionQuantity(t, f, f.holdingAccountID, f.stockCommodityID), "the parent kept its units throughout")
	assert.Equal(t, "0", positionQuantity(t, f, f.holdingAccountID, newID))
	requireScaled(t, 60000, 2, lotRemainingBasis(t, f, *first.LotID), "the first lot gets its basis back")
	requireScaled(t, 40000, 2, lotRemainingBasis(t, f, *second.LotID), "the second lot gets its basis back")
	for _, link := range spun.Plan.Links {
		assert.Equal(t, "closed", lotStatus(t, f, link.DestinationLotID), "the new lot is retired, not deleted")
	}
	var facts, links, reductions int
	require.NoError(t, f.database.QueryRow(`SELECT COUNT(*) FROM investment_transfer_facts`).Scan(&facts))
	require.NoError(t, f.database.QueryRow(`SELECT COUNT(*) FROM investment_transfer_lot_links`).Scan(&links))
	require.NoError(t, f.database.QueryRow(`SELECT COUNT(*) FROM investment_lot_events WHERE event_kind = 'basis_reduction'`).Scan(&reductions))
	assert.Equal(t, [3]int{1, 2, 2}, [3]int{facts, links, reductions}, "the fact, links and reductions stay as evidence")
	var effective int
	require.NoError(t, f.database.QueryRow(`SELECT COUNT(*) FROM effective_investment_lot_events WHERE event_kind = 'basis_reduction'`).Scan(&effective))
	assert.Zero(t, effective, "a reversed spin-off reduces nothing")
	assert.Zero(t, commodityTradingBalance(t, f, newID).Sign(), "the inverse cancels the new units in clearing")
	requireSpinOffSelfCheckPasses(t, f)

	chain, err = f.investmentService.CorrectionChain(ctx, f.ownerUserID, spun.Transaction.ID)
	require.NoError(t, err)
	assert.False(t, chain.CanCorrectSpinOff)
	assert.Nil(t, chain.EffectiveSpinOff)
	_, err = f.investmentService.ReverseSpinOff(ctx, reverseSpinOffInput(f, spun.Transaction.ID))
	require.ErrorIs(t, err, ErrSpinOffAlreadyCorrected)
	_, err = f.investmentService.ReverseSpinOff(ctx, reverseSpinOffInput(f, first.Transaction.ID))
	require.ErrorIs(t, err, ErrSpinOffNotFound)
}

// The new units were sold; reversing the spin-off would leave that sale
// without units. The sale is named and nothing is written.
func TestSpinOffReversalNamesANewInstrumentSaleWithoutWriting(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	newID := seedTestSecurityCommodity(t, f, "SPINCO")
	buyOn(t, f, "2026-02-01", 10, 10000)
	spun, err := f.investmentService.SpinOff(ctx, spinOffInput(f, newID, "2026-06-01", 1, 1, "1", 1))
	require.NoError(t, err)
	sale := sellInput(f, "2026-07-01", 4)
	sale.CommodityID = newID
	sold, err := f.investmentService.Sell(ctx, sale)
	require.NoError(t, err)
	before := buyReplacementPreviewSnapshot(t, f.database)

	_, err = f.investmentService.ReverseSpinOffReconciliationImpact(ctx, reverseSpinOffInput(f, spun.Transaction.ID))
	var dependency InvestmentTransferDependencyError
	require.ErrorAs(t, err, &dependency)
	assert.Equal(t, transferOperationID(t, f, sold.Transaction.ID), dependency.OperationID)
	_, err = f.investmentService.ReverseSpinOff(ctx, reverseSpinOffInput(f, spun.Transaction.ID))
	require.ErrorIs(t, err, ErrInvestmentTransferDependency)
	assert.Equal(t, before, buyReplacementPreviewSnapshot(t, f.database))
}

// A new lot moved on to another holding; reversing the spin-off would remove
// it, so the onward transfer is named.
func TestSpinOffReversalNamesAnOnwardTransfer(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	newID := seedTestSecurityCommodity(t, f, "SPINCO")
	destinationID := seedTestAccountWithClass(t, f.database, "active", true, "asset", "security_holding")
	buyOn(t, f, "2026-02-01", 10, 10000)
	spun, err := f.investmentService.SpinOff(ctx, spinOffInput(f, newID, "2026-06-01", 1, 1, "1", 1))
	require.NoError(t, err)
	onward := internalTransferFromLot(f, destinationID, spun.Plan.Links[0].DestinationLotID, exact.New(3), 0)
	onward.CommodityID, onward.EffectiveOn = newID, "2026-07-01"
	moved, err := f.investmentService.InternalTransfer(ctx, onward)
	require.NoError(t, err)
	before := buyReplacementPreviewSnapshot(t, f.database)

	_, err = f.investmentService.ReverseSpinOff(ctx, reverseSpinOffInput(f, spun.Transaction.ID))
	var dependency InvestmentTransferDependencyError
	require.ErrorAs(t, err, &dependency)
	assert.Equal(t, transferOperationID(t, f, moved.Transaction.ID), dependency.OperationID)
	assert.Equal(t, before, buyReplacementPreviewSnapshot(t, f.database))
}

func TestSpinOffReplacementChangesFractionRatioAndInstrument(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	newID := seedTestSecurityCommodity(t, f, "SPINCO")
	otherID := seedTestSecurityCommodity(t, f, "OTHERCO")
	bought := buyOn(t, f, "2026-02-01", 10, 10000)
	spun, err := f.investmentService.SpinOff(ctx, spinOffInput(f, newID, "2026-06-01", 1, 1, "1", 1))
	require.NoError(t, err)
	auditsBefore := auditEventCount(t, f)

	input := replaceSpinOffInput(f, spun.Transaction.ID, otherID, "2026-06-01", 4, 2, "25", 2)
	before := buyReplacementPreviewSnapshot(t, f.database)
	preview, err := f.investmentService.PreviewSpinOffReplacement(ctx, input)
	require.NoError(t, err)
	assert.Equal(t, before, buyReplacementPreviewSnapshot(t, f.database), "preview writes nothing")
	result, err := f.investmentService.ReplaceSpinOff(ctx, input)
	require.NoError(t, err)

	assert.Equal(t, spun.Transaction.ID, result.CorrectedTransactionID)
	require.NotNil(t, result.Inverse.CorrectionOfTransactionID)
	require.NotNil(t, result.Replacement.CorrectionOfTransactionID)
	assert.Equal(t, spun.Transaction.ID, *result.Replacement.CorrectionOfTransactionID)
	assert.Equal(t, [2]int64{2, 1}, [2]int64{result.Plan.RatioNumerator, result.Plan.RatioDenominator}, "stored in lowest terms")
	require.Len(t, result.Plan.Links, 1)
	link := result.Plan.Links[0]
	assert.Equal(t, *bought.LotID, link.SourceLotID)
	assert.Equal(t, [2]string{"10", "20"}, [2]string{link.SourceQuantityValue.String(), link.DestinationQuantityValue.String()})
	requireScaled(t, 2500, 2, exact.ScaledIntFromInt64(link.AllocatedBasisValue, link.AllocatedBasisScale), "25 % moves")
	requireScaled(t, 7500, 2, exact.ScaledIntFromInt64(link.RemainingBasisValue, link.RemainingBasisScale), "75 % stays")
	assert.Equal(t, "2026-02-01", link.OriginalAcquiredOn)
	committed := link
	committed.DestinationLotID = 0
	assert.Equal(t, preview.Plan.Links[0], committed, "the preview showed what the commit wrote")
	requireScaled(t, 7500, 2, lotRemainingBasis(t, f, *bought.LotID), "the parent keeps only the corrected remainder")
	requireScaled(t, 2500, 2, lotRemainingBasis(t, f, link.DestinationLotID), "the new lot opens at the corrected allocation")
	assert.Equal(t, "10", positionQuantity(t, f, f.holdingAccountID, f.stockCommodityID))
	assert.Equal(t, "0", positionQuantity(t, f, f.holdingAccountID, newID))
	assert.Equal(t, "20", positionQuantity(t, f, f.holdingAccountID, otherID))
	assert.Equal(t, "closed", lotStatus(t, f, spun.Plan.Links[0].DestinationLotID))
	assert.Equal(t, auditsBefore+1, auditEventCount(t, f))
	var evidence string
	require.NoError(t, f.database.QueryRow(`SELECT source_evidence_json FROM investment_transfer_facts
		WHERE destination_commodity_id = ?`, otherID).Scan(&evidence))
	assert.JSONEq(t, `{"notice":"issuer allocation","ex_date":"2026-05-29"}`, evidence, "omitted evidence keeps the recorded evidence")
	assert.Zero(t, commodityTradingBalance(t, f, newID).Sign(), "the inverse cancels the first spin-off's new units")
	assert.Zero(t, commodityTradingBalance(t, f, otherID).Cmp(exact.ScaledIntFromInt64(-20, 0)))
	requireSpinOffSelfCheckPasses(t, f)

	// The chain now explains the replacement.
	chain, err := f.investmentService.CorrectionChain(ctx, f.ownerUserID, spun.Transaction.ID)
	require.NoError(t, err)
	require.NotNil(t, chain.EffectiveSpinOff)
	assert.Equal(t, otherID, chain.EffectiveSpinOff.DestinationCommodityID)
	assert.Equal(t, [2]any{exact.Coefficient("25"), 2}, [2]any{chain.EffectiveSpinOff.Plan.BasisFractionValue, chain.EffectiveSpinOff.Plan.BasisFractionScale})
	assert.True(t, chain.CanCorrectSpinOff)

	_, err = f.investmentService.ReplaceSpinOff(ctx, input)
	require.ErrorIs(t, err, ErrSpinOffAlreadyCorrected)
}

// A corrected fraction changes the basis a later parent sale disposed of; the
// restated gain needs the preview's acknowledgement.
func TestSpinOffReplacementRestatesALaterParentGainWithAcknowledgement(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	newID := seedTestSecurityCommodity(t, f, "SPINCO")
	buyOn(t, f, "2026-02-01", 10, 10000)
	spun, err := f.investmentService.SpinOff(ctx, spinOffInput(f, newID, "2026-06-01", 1, 1, "1", 1))
	require.NoError(t, err)
	sold, err := f.investmentService.Sell(ctx, sellInput(f, "2026-07-01", 5))
	require.NoError(t, err)
	requireScaled(t, 4500, 2, saleEffectiveBasis(t, f, sold.Transaction.ID), "half of the 90 % the parent kept")

	input := replaceSpinOffInput(f, spun.Transaction.ID, newID, "2026-06-01", 1, 1, "3", 1)
	before := buyReplacementPreviewSnapshot(t, f.database)
	_, err = f.investmentService.ReplaceSpinOff(ctx, input)
	require.ErrorIs(t, err, ErrGainImpactAcknowledgementRequired)
	input.GainImpactAcknowledgement = "stale"
	_, err = f.investmentService.ReplaceSpinOff(ctx, input)
	require.ErrorIs(t, err, ErrGainImpactAcknowledgementStale)
	assert.Equal(t, before, buyReplacementPreviewSnapshot(t, f.database), "refusals write nothing")

	input.GainImpactAcknowledgement = ""
	preview, err := f.investmentService.PreviewSpinOffReplacement(ctx, input)
	require.NoError(t, err)
	require.NotNil(t, preview.Impact.GainImpact)
	require.Len(t, preview.Impact.GainImpact.Changes, 1)
	requireScaled(t, 4500, 2, preview.Impact.GainImpact.Changes[0].Before.DisposedBasis, "basis before")
	requireScaled(t, 3500, 2, preview.Impact.GainImpact.Changes[0].After.DisposedBasis, "basis after")
	input.GainImpactAcknowledgement = preview.Impact.GainImpact.Acknowledgement
	_, err = f.investmentService.ReplaceSpinOff(ctx, input)
	require.NoError(t, err)

	requireScaled(t, 3500, 2, saleEffectiveBasis(t, f, sold.Transaction.ID), "half of the 70 % the parent now keeps")
	assert.Equal(t, "10", positionQuantity(t, f, f.holdingAccountID, newID))
	requireSpinOffSelfCheckPasses(t, f)
}

// Moving the spin-off past a parent sale entitles fewer units, which a later
// sale of the new instrument can no longer be met from: that sale is named.
func TestSpinOffReplacementMovedPastAParentSaleNamesANewInstrumentSale(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	newID := seedTestSecurityCommodity(t, f, "SPINCO")
	buyOn(t, f, "2026-02-01", 10, 10000)
	spun, err := f.investmentService.SpinOff(ctx, spinOffInput(f, newID, "2026-06-01", 1, 1, "1", 1))
	require.NoError(t, err)
	_, err = f.investmentService.Sell(ctx, sellInput(f, "2026-08-01", 4))
	require.NoError(t, err)
	sale := sellInput(f, "2026-09-01", 8)
	sale.CommodityID = newID
	sold, err := f.investmentService.Sell(ctx, sale)
	require.NoError(t, err)
	before := buyReplacementPreviewSnapshot(t, f.database)

	_, err = f.investmentService.ReplaceSpinOff(ctx, replaceSpinOffInput(f, spun.Transaction.ID, newID, "2026-08-15", 1, 1, "1", 1))
	var dependency InvestmentTransferDependencyError
	require.ErrorAs(t, err, &dependency)
	assert.Equal(t, transferOperationID(t, f, sold.Transaction.ID), dependency.OperationID)
	assert.Equal(t, before, buyReplacementPreviewSnapshot(t, f.database))
}

// Moving the spin-off later entitles a purchase made after the original date:
// every lot open at the new slot is entitled.
func TestSpinOffReplacementMovedLaterEntitlesTheHoldingThen(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	newID := seedTestSecurityCommodity(t, f, "SPINCO")
	buyOn(t, f, "2026-02-01", 10, 10000)
	spun, err := f.investmentService.SpinOff(ctx, spinOffInput(f, newID, "2026-06-01", 1, 1, "1", 1))
	require.NoError(t, err)
	late := buyOn(t, f, "2026-08-01", 5, 7500)

	result, err := f.investmentService.ReplaceSpinOff(ctx, replaceSpinOffInput(f, spun.Transaction.ID, newID, "2026-09-01", 1, 1, "1", 1))
	require.NoError(t, err)
	require.Len(t, result.Plan.Links, 2)
	assert.Equal(t, "15", result.Plan.DestinationQuantityValue.String())
	assert.Equal(t, *late.LotID, result.Plan.Links[1].SourceLotID)
	requireScaled(t, 6750, 2, lotRemainingBasis(t, f, *late.LotID), "the late lot gives up 10 % too")
	assert.Equal(t, "15", positionQuantity(t, f, f.holdingAccountID, f.stockCommodityID))
	assert.Equal(t, "15", positionQuantity(t, f, f.holdingAccountID, newID))
	assert.Equal(t, "2026-09-01", result.Replacement.TransactionDate)
	requireSpinOffSelfCheckPasses(t, f)
}

// A replacement must change something; one that changes nothing is refused.
// Equivalent terms (2:2 for 1:1, 0.10 for 0.1) are the same terms.
func TestSpinOffReplacementMustChangeTheTerms(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	newID := seedTestSecurityCommodity(t, f, "SPINCO")
	buyOn(t, f, "2026-02-01", 10, 10000)
	spun, err := f.investmentService.SpinOff(ctx, spinOffInput(f, newID, "2026-06-01", 1, 1, "1", 1))
	require.NoError(t, err)
	_, err = f.investmentService.ReplaceSpinOff(ctx, replaceSpinOffInput(f, spun.Transaction.ID, newID, "2026-06-01", 2, 2, "10", 2))
	assert.ErrorAs(t, err, &ValidationError{})
	// The recorded evidence sent back (as the correction form pre-fills it)
	// is not a change either, whatever its key order.
	same := replaceSpinOffInput(f, spun.Transaction.ID, newID, "2026-06-01", 1, 1, "1", 1)
	same.SourceEvidenceJSON = `{"ex_date":"2026-05-29","notice":"issuer allocation"}`
	_, err = f.investmentService.ReplaceSpinOff(ctx, same)
	assert.ErrorAs(t, err, &ValidationError{})
	same.SourceEvidenceJSON = `{"notice":"issuer restatement"}`
	_, err = f.investmentService.PreviewSpinOffReplacement(ctx, same)
	assert.NoError(t, err, "changed evidence alone is a correction")
	input := reverseSpinOffInput(f, spun.Transaction.ID)
	input.Reason = " "
	_, err = f.investmentService.ReverseSpinOff(ctx, input)
	assert.ErrorAs(t, err, &ValidationError{})
}

// The reconciled new instrument would change. The guard runs after every
// replay and must roll all of them back; the override admits it.
func TestSpinOffReversalLateReconciliationRefusalRollsBackEverything(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	newID := seedTestSecurityCommodity(t, f, "SPINCO")
	bought := buyOn(t, f, "2026-02-01", 10, 10000)
	spun, err := f.investmentService.SpinOff(ctx, spinOffInput(f, newID, "2026-06-01", 1, 1, "1", 1))
	require.NoError(t, err)
	reconcileInstrument(t, f, newID, "2026-06-30", 10)
	input := reverseSpinOffInput(f, spun.Transaction.ID)
	impact, err := f.investmentService.ReverseSpinOffReconciliationImpact(ctx, input)
	require.NoError(t, err)
	require.NotEmpty(t, impact.AffectedCheckpoints)
	before := buyReplacementPreviewSnapshot(t, f.database)

	_, err = f.investmentService.ReverseSpinOff(ctx, input)
	require.ErrorIs(t, err, ErrReconciliationOverrideRequired)
	assert.Equal(t, before, buyReplacementPreviewSnapshot(t, f.database))
	requireScaled(t, 9000, 2, lotRemainingBasis(t, f, *bought.LotID), "the parent stays reduced")

	input.ReconciliationOverride = true
	_, err = f.investmentService.ReverseSpinOff(ctx, input)
	require.NoError(t, err)
	assert.Equal(t, "0", positionQuantity(t, f, f.holdingAccountID, newID))
	requireScaled(t, 10000, 2, lotRemainingBasis(t, f, *bought.LotID), "the parent basis is back")
	requireSpinOffSelfCheckPasses(t, f)
}

// A spin-off dated behind a later parent sale entitles every lot open at its
// slot with the units it held then. The sale is revised to the reduced
// basis, a restated gain that needs the acknowledgement.
func TestSpinOffBackdatedBehindAParentSaleRevisesIt(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	newID := seedTestSecurityCommodity(t, f, "SPINCO")
	early := buyOn(t, f, "2026-01-01", 10, 10000)
	late := buyOn(t, f, "2026-08-01", 5, 10000)
	sale := sellInput(f, "2026-09-01", 3)
	sale.CostBasisMethod = "fifo"
	sold, err := f.investmentService.Sell(ctx, sale)
	require.NoError(t, err)
	requireScaled(t, 3000, 2, saleEffectiveBasis(t, f, sold.Transaction.ID), "FIFO took the January lot")

	input := spinOffInput(f, newID, "2026-06-01", 1, 1, "25", 2)
	before := shareExchangeWriteCounts(t, f.database)
	_, err = f.investmentService.SpinOff(ctx, input)
	require.ErrorIs(t, err, ErrGainImpactAcknowledgementRequired)
	assert.Equal(t, before, shareExchangeWriteCounts(t, f.database), "nothing is written on refusal")
	preview, err := f.investmentService.PreviewSpinOff(ctx, input)
	require.NoError(t, err)
	assert.Equal(t, before, shareExchangeWriteCounts(t, f.database), "preview writes nothing")
	require.NotNil(t, preview.Impact.GainImpact)
	require.Len(t, preview.Impact.GainImpact.Changes, 1)
	requireScaled(t, 2250, 2, preview.Impact.GainImpact.Changes[0].After.DisposedBasis, "the sale's reduced basis")
	input.GainImpactAcknowledgement = preview.Impact.GainImpact.Acknowledgement
	result, err := f.investmentService.SpinOff(ctx, input)
	require.NoError(t, err)

	require.Len(t, result.Plan.Links, 1)
	link := result.Plan.Links[0]
	assert.Equal(t, *early.LotID, link.SourceLotID, "only the holding open in June is entitled")
	assert.Equal(t, [2]string{"10", "10"}, [2]string{link.SourceQuantityValue.String(), link.DestinationQuantityValue.String()},
		"entitled with the units held in June, before the sale")
	requireScaled(t, 2500, 2, exact.ScaledIntFromInt64(link.AllocatedBasisValue, link.AllocatedBasisScale), "a quarter of the June basis")
	committed := link
	committed.DestinationLotID = 0
	assert.Equal(t, preview.Plan.Links[0], committed, "the preview showed what the commit wrote")
	requireScaled(t, 2250, 2, saleEffectiveBasis(t, f, sold.Transaction.ID), "three of ten units at 75.00")
	requireScaled(t, 5250, 2, lotRemainingBasis(t, f, *early.LotID), "seven units at 75 % remain")
	requireScaled(t, 10000, 2, lotRemainingBasis(t, f, *late.LotID), "the August lot is untouched")
	assert.Equal(t, "12", positionQuantity(t, f, f.holdingAccountID, f.stockCommodityID))
	assert.Equal(t, "10", positionQuantity(t, f, f.holdingAccountID, newID))
	requireScaled(t, 2500, 2, lotRemainingBasis(t, f, link.DestinationLotID), "the new lot opens at the allocation")
	requireSpinOffSelfCheckPasses(t, f)

	// The backdated spin-off is itself correctable; undoing it restates the
	// sale again.
	_, err = f.investmentService.ReverseSpinOff(ctx, reverseSpinOffInput(f, result.Transaction.ID))
	require.ErrorIs(t, err, ErrGainImpactAcknowledgementRequired)
}

// A spin-off dated behind a later sale of the new instrument opens its lots by
// replay admission. FIFO orders by original acquisition date, so the July
// sale now takes the distributed units that follow the January purchase.
func TestSpinOffBackdatedBehindANewInstrumentSaleRevisesIt(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	newID := seedTestSecurityCommodity(t, f, "SPINCO")
	bought := buyOn(t, f, "2026-01-01", 10, 20000)
	newBuy := tradeOn(f, f.holdingAccountID, "2026-02-01", 5, 5000)
	newBuy.CommodityID = newID
	_, err := f.investmentService.Buy(ctx, newBuy)
	require.NoError(t, err)
	sale := sellInput(f, "2026-07-01", 5)
	sale.CommodityID, sale.CostBasisMethod = newID, "fifo"
	sold, err := f.investmentService.Sell(ctx, sale)
	require.NoError(t, err)
	requireScaled(t, 5000, 2, saleEffectiveBasis(t, f, sold.Transaction.ID), "the only SPINCO lot")

	input := spinOffInput(f, newID, "2026-06-01", 1, 1, "25", 2)
	preview, err := f.investmentService.PreviewSpinOff(ctx, input)
	require.NoError(t, err)
	require.NotNil(t, preview.Impact.GainImpact)
	input.GainImpactAcknowledgement = preview.Impact.GainImpact.Acknowledgement
	result, err := f.investmentService.SpinOff(ctx, input)
	require.NoError(t, err)

	requireScaled(t, 2500, 2, saleEffectiveBasis(t, f, sold.Transaction.ID), "five of the ten distributed units at 50.00")
	requireScaled(t, 15000, 2, lotRemainingBasis(t, f, *bought.LotID), "the parent keeps 75 %")
	assert.Equal(t, "10", positionQuantity(t, f, f.holdingAccountID, newID))
	requireScaled(t, 2500, 2, lotRemainingBasis(t, f, result.Plan.Links[0].DestinationLotID), "half the allocation remains")
	requireSpinOffSelfCheckPasses(t, f)
}

// A link revision of a reversed spin-off stays evidence only: the parent gets
// its corrected basis back whole and self-check reads no revision as a live
// effect (validate-and-ship items 24 and 26).
func TestSpinOffReversalKeepsALinkRevisionAsEvidence(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	newID := seedTestSecurityCommodity(t, f, "SPINCO")
	bought := buyOn(t, f, "2026-02-01", 10, 10000)
	spun, err := f.investmentService.SpinOff(ctx, spinOffInput(f, newID, "2026-06-01", 1, 1, "1", 1))
	require.NoError(t, err)
	replaced, err := f.investmentService.ReplaceBuy(ctx, buyReplacementForNetting(f, bought.Transaction.ID, "2026-02-01", 10, 15000))
	require.NoError(t, err)
	var revisions int
	require.NoError(t, f.database.QueryRow(`SELECT COUNT(*) FROM investment_transfer_link_revisions`).Scan(&revisions))
	require.Equal(t, 1, revisions)
	requireScaled(t, 1500, 2, lotRemainingBasis(t, f, spun.Plan.Links[0].DestinationLotID), "the revision reached the new lot")

	_, err = f.investmentService.ReverseSpinOff(ctx, reverseSpinOffInput(f, spun.Transaction.ID))
	require.NoError(t, err)
	require.NotNil(t, replaced.Replacement.LotID)
	requireScaled(t, 15000, 2, lotRemainingBasis(t, f, *replaced.Replacement.LotID), "the corrected basis is whole again")
	assert.Equal(t, "closed", lotStatus(t, f, spun.Plan.Links[0].DestinationLotID))
	require.NoError(t, f.database.QueryRow(`SELECT COUNT(*) FROM investment_transfer_link_revisions`).Scan(&revisions))
	assert.Equal(t, 1, revisions, "the revision stays as evidence")
	requireSpinOffSelfCheckPasses(t, f)
}

// The export keeps the whole correction chain: the original spin-off, its
// reversal, and a replacement of another spin-off, each linked to what it
// corrects, with the original facts still present.
func TestSpinOffCorrectionsExportAsOperations(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := newInvestmentsTestFixture(t)
	newID := seedTestSecurityCommodity(t, f, "SPINCO")
	buyOn(t, f, "2026-02-01", 10, 10000)
	first, err := f.investmentService.SpinOff(ctx, spinOffInput(f, newID, "2026-06-01", 1, 1, "1", 1))
	require.NoError(t, err)
	reversal, err := f.investmentService.ReverseSpinOff(ctx, reverseSpinOffInput(f, first.Transaction.ID))
	require.NoError(t, err)
	second, err := f.investmentService.SpinOff(ctx, spinOffInput(f, newID, "2026-06-02", 1, 1, "1", 1))
	require.NoError(t, err)
	replaced, err := f.investmentService.ReplaceSpinOff(ctx,
		replaceSpinOffInput(f, second.Transaction.ID, newID, "2026-06-02", 3, 1, "2", 1))
	require.NoError(t, err)

	snapshot, err := db.NewExportRepository(f.database).Snapshot(ctx)
	require.NoError(t, err)
	var out bytes.Buffer
	_, err = NewExportService(db.NewExportRepository(f.database)).writeInvestmentOperationsCSV(ctx, &out, snapshot)
	require.NoError(t, err)
	require.NoError(t, snapshot.Rollback())
	rows, err := csv.NewReader(bytes.NewReader(out.Bytes())).ReadAll()
	require.NoError(t, err)
	byTransaction := make(map[string][]string)
	for _, row := range rows[1:] {
		byTransaction[row[1]] = row
	}
	reversed := byTransaction[strconv.FormatInt(reversal.ID, 10)]
	require.Equal(t, []string{"reversal", "reverse", "wrong issuer allocation"}, []string{reversed[2], reversed[6], reversed[7]})
	require.Equal(t, byTransaction[strconv.FormatInt(first.Transaction.ID, 10)][0], reversed[5])
	replacement := byTransaction[strconv.FormatInt(replaced.Replacement.ID, 10)]
	original := byTransaction[strconv.FormatInt(second.Transaction.ID, 10)]
	require.Equal(t, []string{"spin_off", original[0], "replace", "wrong issuer allocation"},
		[]string{replacement[2], replacement[5], replacement[6], replacement[7]})
	var facts int
	require.NoError(t, f.database.QueryRow(`SELECT COUNT(*) FROM investment_transfer_facts WHERE transfer_kind = 'spin_off'`).Scan(&facts))
	assert.Equal(t, 3, facts, "every spin-off fact stays as evidence")
	assert.Equal(t, "30", positionQuantity(t, f, f.holdingAccountID, newID))
	requireScaled(t, 8000, 2, lotRemainingBasis(t, f, replaced.Plan.Links[0].SourceLotID), "only the replacement's 20 % is effective")
	requireSpinOffSelfCheckPasses(t, f)
}

// A correction is planned against the spin-off outside its write. If another
// command corrects the spin-off first, the stale correction must refuse
// inside the write instead of posting a second inverse.
func TestSpinOffCorrectionRechecksTheSpinOffInsideTheWrite(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	newID := seedTestSecurityCommodity(t, f, "SPINCO")
	bought := buyOn(t, f, "2026-02-01", 10, 10000)
	spun, err := f.investmentService.SpinOff(ctx, spinOffInput(f, newID, "2026-06-01", 1, 1, "1", 1))
	require.NoError(t, err)
	operation, reversal, err := f.investmentService.prepareSpinOffReversalWrite(ctx, reverseSpinOffInput(f, spun.Transaction.ID))
	require.NoError(t, err)
	replacedOperation, inverse, replacement, spinOff, err := f.investmentService.prepareSpinOffReplacementWrite(ctx,
		replaceSpinOffInput(f, spun.Transaction.ID, newID, "2026-06-01", 2, 1, "1", 1))
	require.NoError(t, err)

	_, err = f.investmentService.ReverseSpinOff(ctx, reverseSpinOffInput(f, spun.Transaction.ID))
	require.NoError(t, err)
	before := buyReplacementPreviewSnapshot(t, f.database)
	_, err = f.investmentService.repository.ReverseSpinOff(ctx, reversal, operation)
	require.ErrorIs(t, err, db.ErrInvestmentOperationAlreadyCorrected)
	_, err = f.investmentService.repository.ReplaceSpinOff(ctx, replacedOperation, inverse, replacement, spinOff)
	require.ErrorIs(t, err, db.ErrInvestmentOperationAlreadyCorrected)
	assert.Equal(t, before, buyReplacementPreviewSnapshot(t, f.database))
	requireScaled(t, 10000, 2, lotRemainingBasis(t, f, *bought.LotID), "exactly one reversal took effect")
}
