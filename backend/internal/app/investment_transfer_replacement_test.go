package app

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rekenraam/backend/internal/db"
	"rekenraam/backend/internal/exact"
)

// T-119: internal transfer replacement. The new transfer depletes its source
// at the replaced transfer's correction-root slot, then every position either
// transfer moved replays under one audit event.

func acknowledgedReplaceTransfer(ctx context.Context, s *InvestmentService, input ReplaceInvestmentTransferInput) (ReplaceInvestmentTransferResult, error) {
	if input.GainImpactAcknowledgement == "" {
		if preview, err := s.PreviewTransferReplacement(ctx, input); err == nil && preview.Impact.GainImpact != nil {
			input.GainImpactAcknowledgement = preview.Impact.GainImpact.Acknowledgement
		}
	}
	return s.ReplaceTransfer(ctx, input)
}

func replaceTransferInput(f *investmentsTestFixture, transactionID int64, replacement InternalTransferInput) ReplaceInvestmentTransferInput {
	return ReplaceInvestmentTransferInput{OwnerUserID: f.ownerUserID, TransactionID: transactionID,
		Reason: "statement shows a different move", Replacement: replacement}
}

func selectedLotMove(f *investmentsTestFixture, destinationID, lotID int64, date string, quantity int64) InternalTransferInput {
	input := internalTransferFromLot(f, destinationID, lotID, exact.New(quantity), 0)
	input.EffectiveOn = date
	return input
}

// More units arrive than first recorded; the destination's later sale keeps
// its units and the preview carries exactly what the commit carries.
func TestReplaceInternalTransferQuantityReplaysDestinationSale(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	destinationID := seedTestAccountWithClass(t, f.database, "active", true, "asset", "security_holding")
	buy := buyOn(t, f, "2026-05-01", 3, 3000)
	transfer, err := f.investmentService.InternalTransfer(ctx, selectedLotMove(f, destinationID, *buy.LotID, "2026-06-01", 2))
	require.NoError(t, err)
	sale := sellInput(f, "2026-07-01", 1)
	sale.HoldingAccountID = destinationID
	sold, err := f.investmentService.Sell(ctx, sale)
	require.NoError(t, err)
	transactionsBefore := f.transactionCount(t)

	input := replaceTransferInput(f, transfer.Transaction.ID, selectedLotMove(f, destinationID, *buy.LotID, "2026-06-01", 3))
	preview, err := f.investmentService.PreviewTransferReplacement(ctx, input)
	require.NoError(t, err)
	result, err := acknowledgedReplaceTransfer(ctx, f.investmentService, input)
	require.NoError(t, err)

	assert.Equal(t, transactionsBefore+2, f.transactionCount(t), "an inverse and a replacement journal")
	assert.Equal(t, transfer.Transaction.ID, result.CorrectedTransactionID)
	require.Len(t, result.Replacement.Plan.Links, 1)
	require.Len(t, preview.Plan.Links, 1)
	assert.Equal(t, preview.Plan.Links[0].CarriedBasisValue, result.Replacement.Plan.Links[0].CarriedBasisValue)
	requireScaled(t, 3000, 2, exact.ScaledIntFromInt64(result.Replacement.Plan.Links[0].CarriedBasisValue,
		result.Replacement.Plan.Links[0].CarriedBasisScale), "all three units carry the whole basis")
	assert.Equal(t, "2026-05-01", result.Replacement.Plan.Links[0].OriginalAcquiredOn)
	assert.Equal(t, "0", positionQuantity(t, f, f.holdingAccountID, f.stockCommodityID))
	assert.Equal(t, "2", positionQuantity(t, f, destinationID, f.stockCommodityID))
	requireScaled(t, 1000, 2, saleEffectiveBasis(t, f, sold.Transaction.ID), "the sale's unit keeps its basis")
	requireInvestmentSelfCheckPasses(t, f)

	_, err = f.investmentService.ReplaceTransfer(ctx, input)
	require.ErrorIs(t, err, ErrInvestmentTransferAlreadyCorrected)
	// The replacement is itself correctable: the chain continues from it.
	again := replaceTransferInput(f, result.Replacement.Transaction.ID,
		selectedLotMove(f, destinationID, *buy.LotID, "2026-06-01", 2))
	_, err = acknowledgedReplaceTransfer(ctx, f.investmentService, again)
	require.NoError(t, err)
	assert.Equal(t, "1", positionQuantity(t, f, f.holdingAccountID, f.stockCommodityID))
	requireInvestmentSelfCheckPasses(t, f)
}

// Fewer units than the destination later sold: the sale is named and nothing
// is written.
func TestReplaceInternalTransferBelowDestinationSaleRefusedWithoutWriting(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	destinationID := seedTestAccountWithClass(t, f.database, "active", true, "asset", "security_holding")
	buy := buyOn(t, f, "2026-05-01", 3, 3000)
	transfer, err := f.investmentService.InternalTransfer(ctx, selectedLotMove(f, destinationID, *buy.LotID, "2026-06-01", 2))
	require.NoError(t, err)
	sale := sellInput(f, "2026-07-01", 2)
	sale.HoldingAccountID = destinationID
	_, err = f.investmentService.Sell(ctx, sale)
	require.NoError(t, err)
	before := buyReplacementPreviewSnapshot(t, f.database)

	input := replaceTransferInput(f, transfer.Transaction.ID, selectedLotMove(f, destinationID, *buy.LotID, "2026-06-01", 1))
	_, err = f.investmentService.PreviewTransferReplacement(ctx, input)
	require.ErrorIs(t, err, ErrInvestmentTransferDependency)
	_, err = f.investmentService.ReplaceTransfer(ctx, input)
	require.ErrorIs(t, err, ErrInvestmentTransferDependency)
	assert.Equal(t, before, buyReplacementPreviewSnapshot(t, f.database))
}

// The transfer moves earlier and takes the lot a later source sale took. The
// depletion is computed at the transfer's own slot (that lot is sold out
// today) and the sale is restated onto the other lot with acknowledgement.
func TestReplaceInternalTransferEarlierDateDepletesAtItsSlotAndRestatesSourceSale(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	destinationID := seedTestAccountWithClass(t, f.database, "active", true, "asset", "security_holding")
	cheap := buyOn(t, f, "2026-05-01", 1, 1000)
	dear := buyOn(t, f, "2026-05-02", 1, 5000)
	sold, err := f.investmentService.Sell(ctx, sellInput(f, "2026-07-01", 1))
	require.NoError(t, err)
	requireScaled(t, 1000, 2, saleEffectiveBasis(t, f, sold.Transaction.ID), "FIFO first took the cheap lot")
	transfer, err := f.investmentService.InternalTransfer(ctx, selectedLotMove(f, destinationID, *dear.LotID, "2026-08-01", 1))
	require.NoError(t, err)

	input := replaceTransferInput(f, transfer.Transaction.ID, selectedLotMove(f, destinationID, *cheap.LotID, "2026-06-01", 1))
	_, err = f.investmentService.ReplaceTransfer(ctx, input)
	require.ErrorIs(t, err, db.ErrGainImpactAcknowledgementRequired)
	result, err := acknowledgedReplaceTransfer(ctx, f.investmentService, input)
	require.NoError(t, err)

	requireScaled(t, 5000, 2, saleEffectiveBasis(t, f, sold.Transaction.ID), "the sale now takes the dear lot")
	require.Len(t, result.Replacement.Plan.Links, 1)
	assert.Equal(t, *cheap.LotID, result.Replacement.Plan.Links[0].SourceLotID)
	requireScaled(t, 1000, 2, lotRemainingBasis(t, f, result.Replacement.DestinationLotIDs[0]), "the destination holds the cheap unit")
	assert.Equal(t, "0", positionQuantity(t, f, f.holdingAccountID, f.stockCommodityID))
	assert.Equal(t, "1", positionQuantity(t, f, destinationID, f.stockCommodityID))
	requireInvestmentSelfCheckPasses(t, f)
}

// The remedy ADR 0013 names: a source_lots transfer out of an average-cost
// pool refuses a backdated buy; re-recorded as one pooled lot it admits it.
func TestReplaceSourceLotsTransferAsPooledLotAdmitsBackdatedBuy(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	destinationID := seedTestAccountWithClass(t, f.database, "active", true, "asset", "security_holding")
	setHoldingCostBasisMethod(t, f, "average_cost")
	buyOn(t, f, "2026-02-01", 2, 2000)
	transfer, err := f.investmentService.InternalTransfer(ctx,
		sourceLotsTransferInput(f, destinationID, "2026-03-01", exact.New(1), 0))
	require.NoError(t, err)
	backdated := InvestmentTradeInput{OwnerUserID: f.ownerUserID, TransactionDate: "2026-01-01",
		CommodityID: f.stockCommodityID, HoldingAccountID: f.holdingAccountID,
		CashAccountID: f.cashAccountID, CashCommodityID: f.eurCommodityID,
		QuantityValue: exact.New(2), CashAmountValue: 4000, CashAmountScale: 2}
	_, err = f.investmentService.Buy(ctx, backdated)
	require.ErrorIs(t, err, ErrInvestmentBuyDependency)

	_, err = acknowledgedReplaceTransfer(ctx, f.investmentService, replaceTransferInput(f, transfer.Transaction.ID,
		pooledTransferInput(f, destinationID, "2026-03-01", exact.New(1), 0)))
	require.NoError(t, err)
	_, err = f.investmentService.Buy(ctx, backdated)
	require.NoError(t, err)

	assert.Equal(t, "3", positionQuantity(t, f, f.holdingAccountID, f.stockCommodityID))
	assert.Equal(t, "1", positionQuantity(t, f, destinationID, f.stockCommodityID))
	_, basis := openPositionBasis(t, f, destinationID)
	requireScaled(t, 1500, 2, basis, "the pooled lot carries the revised pool rate")
	requireInvestmentSelfCheckPasses(t, f)
}

// The units went to another account. The first destination empties and the
// lots open in the corrected one.
func TestReplaceInternalTransferDestinationAccountMovesLots(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	wrong := seedTestAccountWithClass(t, f.database, "active", true, "asset", "security_holding")
	right := seedTestAccountWithClass(t, f.database, "active", true, "asset", "security_holding")
	buy := buyOn(t, f, "2026-05-01", 3, 3000)
	transfer, err := f.investmentService.InternalTransfer(ctx, selectedLotMove(f, wrong, *buy.LotID, "2026-06-01", 2))
	require.NoError(t, err)

	_, err = acknowledgedReplaceTransfer(ctx, f.investmentService, replaceTransferInput(f, transfer.Transaction.ID,
		selectedLotMove(f, right, *buy.LotID, "2026-06-01", 2)))
	require.NoError(t, err)

	assert.Equal(t, "1", positionQuantity(t, f, f.holdingAccountID, f.stockCommodityID))
	assert.Equal(t, "0", positionQuantity(t, f, wrong, f.stockCommodityID))
	assert.Equal(t, "2", positionQuantity(t, f, right, f.stockCommodityID))
	requireInvestmentSelfCheckPasses(t, f)
}

// The reconciled source balance changes; the guard refuses after the subject
// depletion, destination lots and every replay are written.
func TestReplaceTransferLateReconciliationRefusalRollsBackEverything(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	destinationID := seedTestAccountWithClass(t, f.database, "active", true, "asset", "security_holding")
	buy := buyOn(t, f, "2026-05-01", 3, 3000)
	transfer, err := f.investmentService.InternalTransfer(ctx, selectedLotMove(f, destinationID, *buy.LotID, "2026-06-01", 1))
	require.NoError(t, err)
	reconcileHolding(t, f, "2026-06-30", 2)
	input := replaceTransferInput(f, transfer.Transaction.ID, selectedLotMove(f, destinationID, *buy.LotID, "2026-06-01", 2))
	preview, err := f.investmentService.PreviewTransferReplacement(ctx, input)
	require.NoError(t, err)
	require.NotEmpty(t, preview.Impact.AffectedCheckpoints)
	before := buyReplacementPreviewSnapshot(t, f.database)

	_, err = f.investmentService.ReplaceTransfer(ctx, input)
	require.ErrorIs(t, err, ErrReconciliationOverrideRequired)
	assert.Equal(t, before, buyReplacementPreviewSnapshot(t, f.database))

	input.ReconciliationOverride = true
	_, err = acknowledgedReplaceTransfer(ctx, f.investmentService, input)
	require.NoError(t, err)
	assert.Equal(t, "1", positionQuantity(t, f, f.holdingAccountID, f.stockCommodityID))
	requireInvestmentSelfCheckPasses(t, f)
}

// External-in transfers are reversed, not replaced, through this command,
// and the security cannot change.
func TestReplaceTransferFencesKindAndSecurity(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	seedExternalTransferEquity(t, f.database)
	ctx := context.Background()
	in, err := f.investmentService.ExternalTransferIn(ctx, knownTransferInput(f))
	require.NoError(t, err)
	destinationID := seedTestAccountWithClass(t, f.database, "active", true, "asset", "security_holding")
	_, err = f.investmentService.ReplaceTransfer(ctx, replaceTransferInput(f, in.Transaction.ID,
		selectedLotMove(f, destinationID, *in.LotID, "2026-06-01", 1)))
	require.ErrorIs(t, err, ErrInvestmentTransferNotFound)

	buy := buyOn(t, f, "2026-05-01", 3, 3000)
	transfer, err := f.investmentService.InternalTransfer(ctx, selectedLotMove(f, destinationID, *buy.LotID, "2026-06-01", 1))
	require.NoError(t, err)
	other := selectedLotMove(f, destinationID, *buy.LotID, "2026-06-01", 1)
	other.CostCommodityID = f.stockCommodityID
	_, err = f.investmentService.ReplaceTransfer(ctx, replaceTransferInput(f, transfer.Transaction.ID, other))
	var validation ValidationError
	require.ErrorAs(t, err, &validation)
}
