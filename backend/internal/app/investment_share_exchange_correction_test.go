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

// #179: a share exchange is corrected by its own reversal and replacement, and
// an exchange dated behind later activity is admitted by replay at its slot.

func reverseShareExchangeInput(f *investmentsTestFixture, transactionID int64) ReverseShareExchangeInput {
	return ReverseShareExchangeInput{OwnerUserID: f.ownerUserID, TransactionID: transactionID, Reason: "wrong merger terms"}
}

func replaceShareExchangeInput(f *investmentsTestFixture, transactionID, newID int64, date string, numerator, denominator int64) ReplaceShareExchangeInput {
	return ReplaceShareExchangeInput{OwnerUserID: f.ownerUserID, TransactionID: transactionID, Reason: "wrong merger terms",
		EffectiveOn: date, DestinationCommodityID: newID, RatioNumerator: numerator, RatioDenominator: denominator,
		Memo: "merger"}
}

func lotStatus(t *testing.T, f *investmentsTestFixture, lotID int64) string {
	t.Helper()
	var status string
	require.NoError(t, f.database.QueryRow(`SELECT status FROM current_investment_lots WHERE id = ?`, lotID).Scan(&status))
	return status
}

func TestShareExchangeReversalRestoresTheOldHoldingAndRetiresTheNewLots(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	newID := seedTestSecurityCommodity(t, f, "NEWCO")
	first := buyOn(t, f, "2026-02-01", 60, 60000)
	second := buyOn(t, f, "2026-03-01", 40, 40000)
	exchanged, err := f.investmentService.ShareExchange(ctx, shareExchangeInput(f, newID, "2026-06-01", 3, 2))
	require.NoError(t, err)
	transactionsBefore := f.transactionCount(t)
	var auditsBefore int
	require.NoError(t, f.database.QueryRow(`SELECT COUNT(*) FROM audit_events`).Scan(&auditsBefore))

	// The generic transfer correction does not reach an exchange.
	_, err = f.investmentService.ReverseTransfer(ctx, reverseTransferInput(f, exchanged.Transaction.ID))
	require.ErrorIs(t, err, ErrInvestmentTransferNotFound)

	inverse, err := f.investmentService.ReverseShareExchange(ctx, reverseShareExchangeInput(f, exchanged.Transaction.ID))
	require.NoError(t, err)

	assert.Equal(t, "2026-06-01", inverse.TransactionDate, "the inverse keeps the exchange date")
	require.NotNil(t, inverse.CorrectionOfTransactionID)
	assert.Equal(t, exchanged.Transaction.ID, *inverse.CorrectionOfTransactionID)
	assert.Equal(t, transactionsBefore+1, f.transactionCount(t))
	var auditsAfter int
	require.NoError(t, f.database.QueryRow(`SELECT COUNT(*) FROM audit_events`).Scan(&auditsAfter))
	assert.Equal(t, auditsBefore+1, auditsAfter, "one audit event for the whole correction")
	assert.Equal(t, "100", positionQuantity(t, f, f.holdingAccountID, f.stockCommodityID))
	assert.Equal(t, "0", positionQuantity(t, f, f.holdingAccountID, newID))
	requireScaled(t, 60000, 2, lotRemainingBasis(t, f, *first.LotID), "the first lot gets its basis back")
	requireScaled(t, 40000, 2, lotRemainingBasis(t, f, *second.LotID), "the second lot gets its basis back")
	for _, link := range exchanged.Plan.Links {
		assert.Equal(t, "closed", lotStatus(t, f, link.DestinationLotID), "the new lot is retired, not deleted")
	}
	var facts, links int
	require.NoError(t, f.database.QueryRow(`SELECT COUNT(*) FROM investment_transfer_facts`).Scan(&facts))
	require.NoError(t, f.database.QueryRow(`SELECT COUNT(*) FROM investment_transfer_lot_links`).Scan(&links))
	assert.Equal(t, [2]int{1, 2}, [2]int{facts, links}, "the fact and links stay as evidence")
	// Clearing mirrors each held instrument again: -100 old, nothing new.
	assert.Zero(t, commodityTradingBalance(t, f, f.stockCommodityID).Cmp(exact.ScaledIntFromInt64(-100, 0)))
	assert.Zero(t, commodityTradingBalance(t, f, newID).Sign())
	requireInvestmentSelfCheckPasses(t, f)

	_, err = f.investmentService.ReverseShareExchange(ctx, reverseShareExchangeInput(f, exchanged.Transaction.ID))
	require.ErrorIs(t, err, ErrShareExchangeAlreadyCorrected)
	_, err = f.investmentService.ReverseShareExchange(ctx, reverseShareExchangeInput(f, first.Transaction.ID))
	require.ErrorIs(t, err, ErrShareExchangeNotFound)
}

// The new instrument was sold; reversing the exchange would leave that sale
// without units. The sale is named and nothing is written.
func TestShareExchangeReversalNamesADestinationSaleWithoutWriting(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	newID := seedTestSecurityCommodity(t, f, "NEWCO")
	buyOn(t, f, "2026-02-01", 10, 10000)
	exchanged, err := f.investmentService.ShareExchange(ctx, shareExchangeInput(f, newID, "2026-06-01", 1, 1))
	require.NoError(t, err)
	sale := sellInput(f, "2026-07-01", 4)
	sale.CommodityID = newID
	sold, err := f.investmentService.Sell(ctx, sale)
	require.NoError(t, err)
	before := buyReplacementPreviewSnapshot(t, f.database)

	_, err = f.investmentService.ReverseShareExchangeReconciliationImpact(ctx, reverseShareExchangeInput(f, exchanged.Transaction.ID))
	var dependency InvestmentTransferDependencyError
	require.ErrorAs(t, err, &dependency)
	assert.Equal(t, transferOperationID(t, f, sold.Transaction.ID), dependency.OperationID)
	_, err = f.investmentService.ReverseShareExchange(ctx, reverseShareExchangeInput(f, exchanged.Transaction.ID))
	require.ErrorIs(t, err, ErrInvestmentTransferDependency)
	assert.Equal(t, before, buyReplacementPreviewSnapshot(t, f.database))
}

// A new lot moved on to another holding; reversing the exchange would remove
// it, so the onward transfer is named.
func TestShareExchangeReversalNamesAnOnwardTransfer(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	newID := seedTestSecurityCommodity(t, f, "NEWCO")
	destinationID := seedTestAccountWithClass(t, f.database, "active", true, "asset", "security_holding")
	buyOn(t, f, "2026-02-01", 10, 10000)
	exchanged, err := f.investmentService.ShareExchange(ctx, shareExchangeInput(f, newID, "2026-06-01", 1, 1))
	require.NoError(t, err)
	onward := internalTransferFromLot(f, destinationID, exchanged.Plan.Links[0].DestinationLotID, exact.New(3), 0)
	onward.CommodityID, onward.EffectiveOn = newID, "2026-07-01"
	moved, err := f.investmentService.InternalTransfer(ctx, onward)
	require.NoError(t, err)
	before := buyReplacementPreviewSnapshot(t, f.database)

	_, err = f.investmentService.ReverseShareExchange(ctx, reverseShareExchangeInput(f, exchanged.Transaction.ID))
	var dependency InvestmentTransferDependencyError
	require.ErrorAs(t, err, &dependency)
	assert.Equal(t, transferOperationID(t, f, moved.Transaction.ID), dependency.OperationID)
	assert.Equal(t, before, buyReplacementPreviewSnapshot(t, f.database))
}

func TestShareExchangeReplacementChangesRatioAndInstrument(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	newID := seedTestSecurityCommodity(t, f, "NEWCO")
	otherID := seedTestSecurityCommodity(t, f, "OTHERCO")
	bought := buyOn(t, f, "2026-02-01", 10, 10000)
	exchanged, err := f.investmentService.ShareExchange(ctx, shareExchangeInput(f, newID, "2026-06-01", 1, 1))
	require.NoError(t, err)
	var auditsBefore int
	require.NoError(t, f.database.QueryRow(`SELECT COUNT(*) FROM audit_events`).Scan(&auditsBefore))

	input := replaceShareExchangeInput(f, exchanged.Transaction.ID, otherID, "2026-06-01", 4, 2)
	before := buyReplacementPreviewSnapshot(t, f.database)
	preview, err := f.investmentService.PreviewShareExchangeReplacement(ctx, input)
	require.NoError(t, err)
	assert.Equal(t, before, buyReplacementPreviewSnapshot(t, f.database), "preview writes nothing")
	result, err := f.investmentService.ReplaceShareExchange(ctx, input)
	require.NoError(t, err)

	assert.Equal(t, exchanged.Transaction.ID, result.CorrectedTransactionID)
	require.NotNil(t, result.Inverse.CorrectionOfTransactionID)
	require.NotNil(t, result.Replacement.CorrectionOfTransactionID)
	assert.Equal(t, exchanged.Transaction.ID, *result.Replacement.CorrectionOfTransactionID)
	assert.Equal(t, [2]int64{2, 1}, [2]int64{result.Plan.RatioNumerator, result.Plan.RatioDenominator}, "stored in lowest terms")
	require.Len(t, result.Plan.Links, 1)
	link := result.Plan.Links[0]
	assert.Equal(t, *bought.LotID, link.SourceLotID)
	assert.Equal(t, "20", link.DestinationQuantityValue.String())
	assert.Equal(t, "2026-02-01", link.OriginalAcquiredOn)
	committed := link
	committed.DestinationLotID = 0
	assert.Equal(t, preview.Plan.Links[0], committed, "the preview showed what the commit wrote")
	requireScaled(t, 10000, 2, lotRemainingBasis(t, f, link.DestinationLotID), "the basis carries over")
	assert.Equal(t, "0", positionQuantity(t, f, f.holdingAccountID, f.stockCommodityID))
	assert.Equal(t, "0", positionQuantity(t, f, f.holdingAccountID, newID))
	assert.Equal(t, "20", positionQuantity(t, f, f.holdingAccountID, otherID))
	assert.Equal(t, "closed", lotStatus(t, f, exchanged.Plan.Links[0].DestinationLotID))
	var auditsAfter int
	require.NoError(t, f.database.QueryRow(`SELECT COUNT(*) FROM audit_events`).Scan(&auditsAfter))
	assert.Equal(t, auditsBefore+1, auditsAfter)
	assert.Zero(t, commodityTradingBalance(t, f, f.stockCommodityID).Sign())
	assert.Zero(t, commodityTradingBalance(t, f, newID).Sign(), "the inverse cancels the first exchange's new units")
	assert.Zero(t, commodityTradingBalance(t, f, otherID).Cmp(exact.ScaledIntFromInt64(-20, 0)))
	requireInvestmentSelfCheckPasses(t, f)

	// The chain now explains the replacement.
	chain, err := f.investmentService.CorrectionChain(ctx, f.ownerUserID, exchanged.Transaction.ID)
	require.NoError(t, err)
	require.NotNil(t, chain.EffectiveShareExchange)
	assert.Equal(t, otherID, chain.EffectiveShareExchange.DestinationCommodityID)

	_, err = f.investmentService.ReplaceShareExchange(ctx, input)
	require.ErrorIs(t, err, ErrShareExchangeAlreadyCorrected)
}

// Changing the ratio changes what a later sale of the new instrument
// disposed of; the restated gain needs the preview's acknowledgement.
func TestShareExchangeReplacementRestatesALaterGainWithAcknowledgement(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	newID := seedTestSecurityCommodity(t, f, "NEWCO")
	buyOn(t, f, "2026-02-01", 10, 10000)
	exchanged, err := f.investmentService.ShareExchange(ctx, shareExchangeInput(f, newID, "2026-06-01", 1, 1))
	require.NoError(t, err)
	sale := sellInput(f, "2026-07-01", 5)
	sale.CommodityID = newID
	sold, err := f.investmentService.Sell(ctx, sale)
	require.NoError(t, err)
	requireScaled(t, 5000, 2, saleEffectiveBasis(t, f, sold.Transaction.ID), "half the holding")

	input := replaceShareExchangeInput(f, exchanged.Transaction.ID, newID, "2026-06-01", 2, 1)
	before := buyReplacementPreviewSnapshot(t, f.database)
	_, err = f.investmentService.ReplaceShareExchange(ctx, input)
	require.ErrorIs(t, err, ErrGainImpactAcknowledgementRequired)
	input.GainImpactAcknowledgement = "stale"
	_, err = f.investmentService.ReplaceShareExchange(ctx, input)
	require.ErrorIs(t, err, ErrGainImpactAcknowledgementStale)
	assert.Equal(t, before, buyReplacementPreviewSnapshot(t, f.database), "refusals write nothing")

	input.GainImpactAcknowledgement = ""
	preview, err := f.investmentService.PreviewShareExchangeReplacement(ctx, input)
	require.NoError(t, err)
	require.NotNil(t, preview.Impact.GainImpact)
	require.Len(t, preview.Impact.GainImpact.Changes, 1)
	requireScaled(t, 5000, 2, preview.Impact.GainImpact.Changes[0].Before.DisposedBasis, "basis before")
	requireScaled(t, 2500, 2, preview.Impact.GainImpact.Changes[0].After.DisposedBasis, "basis after")
	input.GainImpactAcknowledgement = preview.Impact.GainImpact.Acknowledgement
	_, err = f.investmentService.ReplaceShareExchange(ctx, input)
	require.NoError(t, err)

	requireScaled(t, 2500, 2, saleEffectiveBasis(t, f, sold.Transaction.ID), "5 of 20 new units")
	assert.Equal(t, "15", positionQuantity(t, f, f.holdingAccountID, newID))
	requireInvestmentSelfCheckPasses(t, f)
}

// A replacement dated before a later sale of the old instrument would take
// the units that sale disposed of; the sale is named.
func TestShareExchangeReplacementDatedBeforeAnOldSaleNamesIt(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	newID := seedTestSecurityCommodity(t, f, "NEWCO")
	buyOn(t, f, "2026-02-01", 10, 10000)
	sold, err := f.investmentService.Sell(ctx, sellInput(f, "2026-04-01", 3))
	require.NoError(t, err)
	exchanged, err := f.investmentService.ShareExchange(ctx, shareExchangeInput(f, newID, "2026-06-01", 1, 1))
	require.NoError(t, err)
	require.Equal(t, "7", exchanged.Plan.SourceQuantityValue.String())
	before := buyReplacementPreviewSnapshot(t, f.database)

	_, err = f.investmentService.ReplaceShareExchange(ctx, replaceShareExchangeInput(f, exchanged.Transaction.ID, newID, "2026-03-01", 1, 1))
	var dependency InvestmentTransferDependencyError
	require.ErrorAs(t, err, &dependency)
	assert.Equal(t, transferOperationID(t, f, sold.Transaction.ID), dependency.OperationID)
	assert.Equal(t, before, buyReplacementPreviewSnapshot(t, f.database))
}

// Moving the exchange later takes in a purchase that was made after the
// original date: the whole holding open at the new slot converts.
func TestShareExchangeReplacementMovedLaterTakesTheWholeHoldingThen(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	newID := seedTestSecurityCommodity(t, f, "NEWCO")
	buyOn(t, f, "2026-02-01", 10, 10000)
	exchanged, err := f.investmentService.ShareExchange(ctx, shareExchangeInput(f, newID, "2026-06-01", 1, 1))
	require.NoError(t, err)
	late := buyOn(t, f, "2026-08-01", 5, 7500)

	result, err := f.investmentService.ReplaceShareExchange(ctx, replaceShareExchangeInput(f, exchanged.Transaction.ID, newID, "2026-09-01", 1, 1))
	require.NoError(t, err)
	require.Len(t, result.Plan.Links, 2)
	assert.Equal(t, "15", result.Plan.DestinationQuantityValue.String())
	assert.Equal(t, *late.LotID, result.Plan.Links[1].SourceLotID)
	assert.Equal(t, "0", positionQuantity(t, f, f.holdingAccountID, f.stockCommodityID))
	assert.Equal(t, "15", positionQuantity(t, f, f.holdingAccountID, newID))
	assert.Equal(t, "2026-09-01", result.Replacement.TransactionDate)
	requireInvestmentSelfCheckPasses(t, f)
}

// A replacement must change something; one that changes nothing is refused.
func TestShareExchangeReplacementMustChangeTheTerms(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	newID := seedTestSecurityCommodity(t, f, "NEWCO")
	buyOn(t, f, "2026-02-01", 10, 10000)
	exchanged, err := f.investmentService.ShareExchange(ctx, shareExchangeInput(f, newID, "2026-06-01", 2, 1))
	require.NoError(t, err)
	_, err = f.investmentService.ReplaceShareExchange(ctx, replaceShareExchangeInput(f, exchanged.Transaction.ID, newID, "2026-06-01", 4, 2))
	assert.ErrorAs(t, err, &ValidationError{})
	input := reverseShareExchangeInput(f, exchanged.Transaction.ID)
	input.Reason = " "
	_, err = f.investmentService.ReverseShareExchange(ctx, input)
	assert.ErrorAs(t, err, &ValidationError{})
}

// The reconciled old holding would change. The guard runs after every replay
// and must roll all of them back; the override admits it.
func TestShareExchangeReversalLateReconciliationRefusalRollsBackEverything(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	newID := seedTestSecurityCommodity(t, f, "NEWCO")
	buyOn(t, f, "2026-02-01", 10, 10000)
	exchanged, err := f.investmentService.ShareExchange(ctx, shareExchangeInput(f, newID, "2026-06-01", 1, 1))
	require.NoError(t, err)
	reconcileHolding(t, f, "2026-06-30", 0)
	input := reverseShareExchangeInput(f, exchanged.Transaction.ID)
	impact, err := f.investmentService.ReverseShareExchangeReconciliationImpact(ctx, input)
	require.NoError(t, err)
	require.NotEmpty(t, impact.AffectedCheckpoints)
	before := buyReplacementPreviewSnapshot(t, f.database)

	_, err = f.investmentService.ReverseShareExchange(ctx, input)
	require.ErrorIs(t, err, ErrReconciliationOverrideRequired)
	assert.Equal(t, before, buyReplacementPreviewSnapshot(t, f.database))

	input.ReconciliationOverride = true
	_, err = f.investmentService.ReverseShareExchange(ctx, input)
	require.NoError(t, err)
	assert.Equal(t, "10", positionQuantity(t, f, f.holdingAccountID, f.stockCommodityID))
	requireInvestmentSelfCheckPasses(t, f)
}

// An exchange dated behind a later sale of the old instrument converts the
// whole holding open at its slot. The sale is revised to the units bought
// after the exchange, a restated gain that needs the acknowledgement.
func TestShareExchangeBackdatedBehindAnOldSaleRevisesIt(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	newID := seedTestSecurityCommodity(t, f, "NEWCO")
	early := buyOn(t, f, "2026-01-01", 10, 10000)
	late := buyOn(t, f, "2026-08-01", 5, 10000)
	sale := sellInput(f, "2026-09-01", 3)
	sale.CostBasisMethod = "fifo"
	sold, err := f.investmentService.Sell(ctx, sale)
	require.NoError(t, err)
	requireScaled(t, 3000, 2, saleEffectiveBasis(t, f, sold.Transaction.ID), "FIFO took the January lot")

	input := shareExchangeInput(f, newID, "2026-06-01", 1, 1)
	before := shareExchangeWriteCounts(t, f.database)
	_, err = f.investmentService.ShareExchange(ctx, input)
	require.ErrorIs(t, err, ErrGainImpactAcknowledgementRequired)
	assert.Equal(t, before, shareExchangeWriteCounts(t, f.database), "nothing is written on refusal")
	preview, err := f.investmentService.PreviewShareExchange(ctx, input)
	require.NoError(t, err)
	require.NotNil(t, preview.Impact.GainImpact)
	input.GainImpactAcknowledgement = preview.Impact.GainImpact.Acknowledgement
	result, err := f.investmentService.ShareExchange(ctx, input)
	require.NoError(t, err)

	require.Len(t, result.Plan.Links, 1)
	assert.Equal(t, *early.LotID, result.Plan.Links[0].SourceLotID, "only the holding open in June converts")
	assert.Equal(t, "10", result.Plan.DestinationQuantityValue.String())
	requireScaled(t, 6000, 2, saleEffectiveBasis(t, f, sold.Transaction.ID), "the sale now takes the August lot")
	assert.Equal(t, "2", positionQuantity(t, f, f.holdingAccountID, f.stockCommodityID))
	assert.Equal(t, "open", lotStatus(t, f, *late.LotID))
	assert.Equal(t, "10", positionQuantity(t, f, f.holdingAccountID, newID))
	requireScaled(t, 10000, 2, lotRemainingBasis(t, f, result.Plan.Links[0].DestinationLotID), "the January basis carries over")
	requireInvestmentSelfCheckPasses(t, f)

	// The backdated exchange is itself correctable.
	_, err = f.investmentService.ReverseShareExchange(ctx, ReverseShareExchangeInput{OwnerUserID: f.ownerUserID,
		TransactionID: result.Transaction.ID, Reason: "recorded twice"})
	require.ErrorIs(t, err, ErrGainImpactAcknowledgementRequired)
}

// An exchange dated behind a later sale it would leave without units refuses
// with the sale named and writes nothing.
func TestShareExchangeBackdatedBehindAnUnsatisfiableSaleNamesIt(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	newID := seedTestSecurityCommodity(t, f, "NEWCO")
	buyOn(t, f, "2026-02-01", 10, 10000)
	sold, err := f.investmentService.Sell(ctx, sellInput(f, "2026-07-01", 2))
	require.NoError(t, err)
	before := shareExchangeWriteCounts(t, f.database)

	for _, attempt := range []func() error{
		func() error {
			_, err := f.investmentService.PreviewShareExchange(ctx, shareExchangeInput(f, newID, "2026-06-01", 1, 1))
			return err
		},
		func() error {
			_, err := f.investmentService.ShareExchange(ctx, shareExchangeInput(f, newID, "2026-06-01", 1, 1))
			return err
		},
	} {
		err := attempt()
		var dependency InvestmentTransferDependencyError
		require.ErrorAs(t, err, &dependency)
		assert.Equal(t, transferOperationID(t, f, sold.Transaction.ID), dependency.OperationID)
	}
	assert.Equal(t, before, shareExchangeWriteCounts(t, f.database))
}

// An exchange dated behind a later sale of the new instrument opens its lots
// by replay admission. FIFO orders by original acquisition date, so the July
// sale now takes the exchanged January units: a revised decision.
func TestShareExchangeBackdatedBehindANewInstrumentSaleRevisesIt(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	newID := seedTestSecurityCommodity(t, f, "NEWCO")
	buyOn(t, f, "2026-01-01", 10, 20000)
	newBuy := tradeOn(f, f.holdingAccountID, "2026-02-01", 5, 5000)
	newBuy.CommodityID = newID
	_, err := f.investmentService.Buy(ctx, newBuy)
	require.NoError(t, err)
	sale := sellInput(f, "2026-07-01", 5)
	sale.CommodityID, sale.CostBasisMethod = newID, "fifo"
	sold, err := f.investmentService.Sell(ctx, sale)
	require.NoError(t, err)
	requireScaled(t, 5000, 2, saleEffectiveBasis(t, f, sold.Transaction.ID), "the only NEWCO lot")

	input := shareExchangeInput(f, newID, "2026-06-01", 1, 1)
	preview, err := f.investmentService.PreviewShareExchange(ctx, input)
	require.NoError(t, err)
	require.NotNil(t, preview.Impact.GainImpact)
	input.GainImpactAcknowledgement = preview.Impact.GainImpact.Acknowledgement
	result, err := f.investmentService.ShareExchange(ctx, input)
	require.NoError(t, err)

	requireScaled(t, 10000, 2, saleEffectiveBasis(t, f, sold.Transaction.ID), "five of the exchanged January units")
	assert.Equal(t, "10", positionQuantity(t, f, f.holdingAccountID, newID))
	requireScaled(t, 10000, 2, lotRemainingBasis(t, f, result.Plan.Links[0].DestinationLotID), "half the carried basis remains")
	requireInvestmentSelfCheckPasses(t, f)
}

// The export keeps the whole correction chain: the original exchange, its
// reversal, and a replacement of another exchange, each linked to what it
// corrects, with the original facts still present.
func TestShareExchangeCorrectionsExportAsOperations(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := newInvestmentsTestFixture(t)
	newID := seedTestSecurityCommodity(t, f, "NEWCO")
	buyOn(t, f, "2026-02-01", 10, 10000)
	first, err := f.investmentService.ShareExchange(ctx, shareExchangeInput(f, newID, "2026-06-01", 1, 1))
	require.NoError(t, err)
	reversal, err := f.investmentService.ReverseShareExchange(ctx, reverseShareExchangeInput(f, first.Transaction.ID))
	require.NoError(t, err)
	second, err := f.investmentService.ShareExchange(ctx, shareExchangeInput(f, newID, "2026-06-02", 1, 1))
	require.NoError(t, err)
	replaced, err := f.investmentService.ReplaceShareExchange(ctx,
		replaceShareExchangeInput(f, second.Transaction.ID, newID, "2026-06-02", 3, 1))
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
	require.Equal(t, []string{"reversal", "reverse", "wrong merger terms"}, []string{reversed[2], reversed[6], reversed[7]})
	require.Equal(t, byTransaction[strconv.FormatInt(first.Transaction.ID, 10)][0], reversed[5])
	replacement := byTransaction[strconv.FormatInt(replaced.Replacement.ID, 10)]
	original := byTransaction[strconv.FormatInt(second.Transaction.ID, 10)]
	require.Equal(t, []string{"share_exchange", original[0], "replace", "wrong merger terms"},
		[]string{replacement[2], replacement[5], replacement[6], replacement[7]})
	var facts int
	require.NoError(t, f.database.QueryRow(`SELECT COUNT(*) FROM investment_transfer_facts WHERE transfer_kind = 'exchange'`).Scan(&facts))
	assert.Equal(t, 3, facts, "every exchange fact stays as evidence")
	assert.Equal(t, "30", positionQuantity(t, f, f.holdingAccountID, newID))
	requireInvestmentSelfCheckPasses(t, f)
}

// A correction is planned against the exchange outside its write. If another
// command corrects the exchange first, the stale correction must refuse
// inside the write instead of posting a second inverse.
func TestShareExchangeCorrectionRechecksTheExchangeInsideTheWrite(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	newID := seedTestSecurityCommodity(t, f, "NEWCO")
	buyOn(t, f, "2026-02-01", 10, 10000)
	exchanged, err := f.investmentService.ShareExchange(ctx, shareExchangeInput(f, newID, "2026-06-01", 1, 1))
	require.NoError(t, err)
	operation, reversal, err := f.investmentService.prepareShareExchangeReversalWrite(ctx, reverseShareExchangeInput(f, exchanged.Transaction.ID))
	require.NoError(t, err)
	replacedOperation, inverse, replacement, exchange, err := f.investmentService.prepareShareExchangeReplacementWrite(ctx,
		replaceShareExchangeInput(f, exchanged.Transaction.ID, newID, "2026-06-01", 2, 1))
	require.NoError(t, err)

	_, err = f.investmentService.ReverseShareExchange(ctx, reverseShareExchangeInput(f, exchanged.Transaction.ID))
	require.NoError(t, err)
	before := buyReplacementPreviewSnapshot(t, f.database)
	_, err = f.investmentService.repository.ReverseShareExchange(ctx, reversal, operation)
	require.ErrorIs(t, err, db.ErrInvestmentOperationAlreadyCorrected)
	_, err = f.investmentService.repository.ReplaceShareExchange(ctx, replacedOperation, inverse, replacement, exchange)
	require.ErrorIs(t, err, db.ErrInvestmentOperationAlreadyCorrected)
	assert.Equal(t, before, buyReplacementPreviewSnapshot(t, f.database))
	assert.Equal(t, "10", positionQuantity(t, f, f.holdingAccountID, f.stockCommodityID))
}

// A link revision of a reversed exchange stays evidence only: the old lot
// gets its corrected basis back and self-check reads no revision as a live
// effect (validate-and-ship items 24 and 26).
func TestShareExchangeReversalKeepsALinkRevisionAsEvidence(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	newID := seedTestSecurityCommodity(t, f, "NEWCO")
	bought := buyOn(t, f, "2026-02-01", 10, 10000)
	exchanged, err := f.investmentService.ShareExchange(ctx, shareExchangeInput(f, newID, "2026-06-01", 3, 2))
	require.NoError(t, err)
	replaced, err := f.investmentService.ReplaceBuy(ctx, buyReplacementForNetting(f, bought.Transaction.ID, "2026-02-01", 10, 15000))
	require.NoError(t, err)
	var revisions int
	require.NoError(t, f.database.QueryRow(`SELECT COUNT(*) FROM investment_transfer_link_revisions`).Scan(&revisions))
	require.Equal(t, 1, revisions)

	_, err = f.investmentService.ReverseShareExchange(ctx, reverseShareExchangeInput(f, exchanged.Transaction.ID))
	require.NoError(t, err)
	require.NotNil(t, replaced.Replacement.LotID)
	requireScaled(t, 15000, 2, lotRemainingBasis(t, f, *replaced.Replacement.LotID), "the corrected basis is back in the old holding")
	assert.Equal(t, "closed", lotStatus(t, f, exchanged.Plan.Links[0].DestinationLotID))
	require.NoError(t, f.database.QueryRow(`SELECT COUNT(*) FROM investment_transfer_link_revisions`).Scan(&revisions))
	assert.Equal(t, 1, revisions, "the revision stays as evidence")
	requireInvestmentSelfCheckPasses(t, f)
}
