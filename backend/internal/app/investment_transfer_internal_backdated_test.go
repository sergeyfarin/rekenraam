package app

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rekenraam/backend/internal/db"
	"rekenraam/backend/internal/exact"
)

// #167: a new internal transfer dated behind a later depletion of either
// holding is admitted through replay at its own slot. Its source depletion is
// the one that date sees, its destination lots open behind later destination
// events, and both holdings replay with everything downstream, atomically.

// commitBackdatedInternalTransfer previews input, requires the preview's
// disclosed gain changes to be acknowledged before commit, commits, and
// checks that the preview's plan is what the commit carried. It returns the
// number of disclosed gain changes.
func commitBackdatedInternalTransfer(t *testing.T, f *investmentsTestFixture, input InternalTransferInput) (InternalTransferResult, int) {
	t.Helper()
	ctx := context.Background()
	before := buyReplacementPreviewSnapshot(t, f.database)
	preview, err := f.investmentService.PreviewInternalTransfer(ctx, input)
	require.NoError(t, err)
	require.Equal(t, before, buyReplacementPreviewSnapshot(t, f.database), "the preview writes nothing")
	changes := 0
	if preview.Impact.GainImpact != nil && len(preview.Impact.GainImpact.Changes) > 0 {
		changes = len(preview.Impact.GainImpact.Changes)
		_, err := f.investmentService.InternalTransfer(ctx, input)
		require.ErrorIs(t, err, db.ErrGainImpactAcknowledgementRequired)
		require.Equal(t, before, buyReplacementPreviewSnapshot(t, f.database), "an unacknowledged commit writes nothing")
		input.GainImpactAcknowledgement = preview.Impact.GainImpact.Acknowledgement
	}
	result, err := f.investmentService.InternalTransfer(ctx, input)
	require.NoError(t, err)
	committed := result.Plan
	committed.Links = append([]InternalTransferLink(nil), committed.Links...)
	for index := range committed.Links {
		committed.Links[index].DestinationLotID = 0
	}
	assert.Equal(t, preview.Plan, committed, "the preview's plan is what the commit carried")
	return result, changes
}

// The audit reproduction: buy 3 on May 1 and 3 on May 15, sell 3 FIFO on
// July 1, then move 2 from the first lot on June 1. The sale is revised to
// the first lot's last unit plus two from the second.
func TestBackdatedInternalTransferReplaysLaterSourceSale(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	destinationID := seedTestAccountWithClass(t, f.database, "active", true, "asset", "security_holding")
	first := buyOn(t, f, "2026-05-01", 3, 3000)
	buyOn(t, f, "2026-05-15", 3, 6000)
	sold, err := f.investmentService.Sell(ctx, sellInput(f, "2026-07-01", 3))
	require.NoError(t, err)
	requireScaled(t, 3000, 2, saleEffectiveBasis(t, f, sold.Transaction.ID), "FIFO took the first lot")

	result, changes := commitBackdatedInternalTransfer(t, f,
		internalTransferFromLot(f, destinationID, *first.LotID, exact.New(2), 0))
	assert.Equal(t, 1, changes, "the later sale's gain change is disclosed")
	require.Len(t, result.Plan.Links, 1)
	link := result.Plan.Links[0]
	assert.Equal(t, *first.LotID, link.SourceLotID)
	requireScaled(t, 2000, 2, exact.ScaledIntFromInt64(link.CarriedBasisValue, link.CarriedBasisScale), "carried at its own slot")
	assert.Equal(t, "2026-05-01", link.OriginalAcquiredOn)
	requireScaled(t, 5000, 2, saleEffectiveBasis(t, f, sold.Transaction.ID), "sale revised by replay")
	requireScaled(t, 2000, 2, lotRemainingBasis(t, f, result.DestinationLotIDs[0]), "destination lot")
	requireInvestmentSelfCheckPasses(t, f)
}

// A destination lot opens behind a later destination sale. FIFO orders by
// the carried original date, so the sale now takes the older moved unit.
func TestBackdatedInternalTransferReplaysLaterDestinationSale(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	destinationID := seedTestAccountWithClass(t, f.database, "active", true, "asset", "security_holding")
	source := buyOn(t, f, "2026-01-01", 1, 1000)
	_, err := f.investmentService.Buy(ctx, tradeOn(f, destinationID, "2026-02-01", 1, 2000))
	require.NoError(t, err)
	sold, err := f.investmentService.Sell(ctx, tradeOn(f, destinationID, "2026-04-01", 1, 5000))
	require.NoError(t, err)
	requireScaled(t, 2000, 2, saleEffectiveBasis(t, f, sold.Transaction.ID), "the only lot then")

	input := internalTransferFromLot(f, destinationID, *source.LotID, exact.New(1), 0)
	input.EffectiveOn = "2026-03-01"
	result, changes := commitBackdatedInternalTransfer(t, f, input)
	assert.Equal(t, 1, changes)
	requireScaled(t, 1000, 2, saleEffectiveBasis(t, f, sold.Transaction.ID), "FIFO takes the January unit")
	requireScaled(t, 0, 0, lotRemainingBasis(t, f, result.DestinationLotIDs[0]), "the moved unit was sold")
	requireInvestmentSelfCheckPasses(t, f)
}

func TestBackdatedInternalTransferReplaysSourceAndDestinationSalesTogether(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	destinationID := seedTestAccountWithClass(t, f.database, "active", true, "asset", "security_holding")
	first := buyOn(t, f, "2026-05-01", 3, 3000)
	buyOn(t, f, "2026-05-15", 3, 6000)
	sourceSale, err := f.investmentService.Sell(ctx, sellInput(f, "2026-07-01", 3))
	require.NoError(t, err)
	_, err = f.investmentService.Buy(ctx, tradeOn(f, destinationID, "2026-05-20", 1, 5000))
	require.NoError(t, err)
	destinationSale, err := f.investmentService.Sell(ctx, tradeOn(f, destinationID, "2026-07-01", 1, 9000))
	require.NoError(t, err)

	_, changes := commitBackdatedInternalTransfer(t, f,
		internalTransferFromLot(f, destinationID, *first.LotID, exact.New(2), 0))
	assert.Equal(t, 2, changes, "both later sales' gain changes are disclosed")
	requireScaled(t, 5000, 2, saleEffectiveBasis(t, f, sourceSale.Transaction.ID), "source sale")
	requireScaled(t, 1000, 2, saleEffectiveBasis(t, f, destinationSale.Transaction.ID), "destination sale takes a May 1 unit")
	requireInvestmentSelfCheckPasses(t, f)
}

// An average-cost source moves from its pool as dated at the transfer, in
// either lineage; the later average sale is revised at the new pool rate.
func TestBackdatedPooledInternalTransferReplaysLaterAverageSale(t *testing.T) {
	t.Parallel()
	for _, lineage := range []string{db.InternalTransferPooledLot, db.InternalTransferSourceLots} {
		t.Run(lineage, func(t *testing.T) {
			t.Parallel()
			f := newInvestmentsTestFixture(t)
			ctx := context.Background()
			destinationID := seedTestAccountWithClass(t, f.database, "active", true, "asset", "security_holding")
			setHoldingCostBasisMethod(t, f, "average_cost")
			buyOn(t, f, "2026-01-01", 2, 2000)
			buyOn(t, f, "2026-02-01", 2, 6000)
			sale := sellInput(f, "2026-03-01", 2)
			sale.CostBasisMethod = "average_cost"
			sold, err := f.investmentService.Sell(ctx, sale)
			require.NoError(t, err)
			requireScaled(t, 4000, 2, saleEffectiveBasis(t, f, sold.Transaction.ID), "two of four at 20.00")

			input := pooledTransferInput(f, destinationID, "2026-01-15", exact.New(1), 0)
			input.DestinationLineage = lineage
			result, changes := commitBackdatedInternalTransfer(t, f, input)
			assert.Equal(t, 1, changes)
			assert.Equal(t, lineage, result.Plan.DestinationLineage)
			requireScaled(t, 1000, 2, linkedBasis(result.Plan), "one of two January units at 10.00")
			// March pool: three units for 70.00; two of them at the pool's
			// allocation scale, the remainder staying with the last unit.
			requireScaled(t, 46666666, 6, saleEffectiveBasis(t, f, sold.Transaction.ID), "sale at the new pool rate")
			sourceQuantity, sourceBasis := openPositionBasis(t, f, f.holdingAccountID)
			requireScaled(t, 1, 0, sourceQuantity, "source units")
			requireConserved(t, exact.ScaledIntFromInt64(8000, 2), sourceBasis,
				saleEffectiveBasis(t, f, sold.Transaction.ID), linkedBasis(result.Plan))
			requireInvestmentSelfCheckPasses(t, f)
		})
	}
}

// Unknown basis moves behind a later sale as unknown; the sale stays
// unresolved and takes the lot's remaining unit.
func TestBackdatedInternalTransferCarriesUnknownBasisBehindLaterSale(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	seedExternalTransferEquity(t, f.database)
	destinationID := seedTestAccountWithClass(t, f.database, "active", true, "asset", "security_holding")
	source, err := f.investmentService.ExternalTransferIn(ctx, unknownTransferInInput(f, "2026-05-01", "2020-03-01"))
	require.NoError(t, err)
	_, err = f.investmentService.Sell(ctx, sellInput(f, "2026-07-01", 1))
	require.NoError(t, err)

	result, _ := commitBackdatedInternalTransfer(t, f,
		internalTransferFromLot(f, destinationID, *source.LotID, exact.New(1), 0))
	require.Len(t, result.Plan.Links, 1)
	assert.Equal(t, db.InvestmentBasisUnknown, result.Plan.Links[0].BasisKnowledge)
	assert.Equal(t, "2020-03-01", result.Plan.Links[0].OriginalAcquiredOn)
	assert.Equal(t, []string{"unknown"}, linkKnowledge(t, f, result.Transaction.ID))
	quantity, _, knowledge := lotStateByID(t, f, result.DestinationLotIDs[0])
	assert.Equal(t, "1", quantity)
	assert.Equal(t, db.InvestmentBasisUnknown, knowledge)
	assert.Equal(t, db.InvestmentBasisUnknown, latestDisposal(t, f).knowledge)
	requireHealthyUnresolvedBook(t, f)
}

// On the later sale's own date the transfer takes the last same-day slot, so
// no replay is needed and the sale keeps what it took.
func TestInternalTransferOnLaterSaleDateTakesTheLastSameDaySlot(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	destinationID := seedTestAccountWithClass(t, f.database, "active", true, "asset", "security_holding")
	first := buyOn(t, f, "2026-05-01", 3, 3000)
	sold, err := f.investmentService.Sell(ctx, sellInput(f, "2026-07-01", 2))
	require.NoError(t, err)

	input := internalTransferFromLot(f, destinationID, *first.LotID, exact.New(2), 0)
	input.EffectiveOn = "2026-07-01"
	_, err = f.investmentService.InternalTransfer(ctx, input)
	require.ErrorIs(t, err, ErrInvestmentLotsInsufficient, "the sale entered first already took two")

	input.Allocations[0].QuantityValue = exact.New(1)
	result, changes := commitBackdatedInternalTransfer(t, f, input)
	assert.Zero(t, changes)
	requireScaled(t, 2000, 2, saleEffectiveBasis(t, f, sold.Transaction.ID), "the sale is unchanged")
	requireScaled(t, 1000, 2, linkedBasis(result.Plan), "the last unit moves")
	requireInvestmentSelfCheckPasses(t, f)
}

// A→B dated behind B's later pooled_lot move to C: B's pool is revised, and
// the change carries on to C's lot through a link revision.
func TestBackdatedInternalTransferIntoChainedHoldingRevisesDownstreamPooledLot(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	_, err := f.investmentService.SaveCostBasisProfile(ctx, CostBasisProfileInput{
		OwnerUserID: f.ownerUserID, Name: "Book average cost", Method: "average_cost", IsDefault: true, Status: "active",
	})
	require.NoError(t, err)
	b := seedTestAccountWithClass(t, f.database, "active", true, "asset", "security_holding")
	c := seedTestAccountWithClass(t, f.database, "active", true, "asset", "security_holding")
	buyOn(t, f, "2026-05-01", 2, 2000)
	_, err = f.investmentService.Buy(ctx, tradeOn(f, b, "2026-05-01", 2, 6000))
	require.NoError(t, err)
	onward := pooledTransferInput(f, c, "2026-07-01", exact.New(2), 0)
	onward.SourceAccountID = b
	toC, err := f.investmentService.InternalTransfer(ctx, onward)
	require.NoError(t, err)
	requireScaled(t, 6000, 2, linkedBasis(toC.Plan), "B's whole pool")

	_, changes := commitBackdatedInternalTransfer(t, f, pooledTransferInput(f, b, "2026-06-01", exact.New(2), 0))
	assert.Zero(t, changes, "no disposal changed")
	basis, _, revisions := pooledLotLink(t, f, toC.Transaction.ID)
	requireScaled(t, 4000, 2, basis, "two of four units for 80.00")
	assert.Equal(t, 1, revisions)
	requireScaled(t, 4000, 2, lotRemainingBasis(t, f, toC.DestinationLotIDs[0]), "C's lot follows")
	requireInvestmentSelfCheckPasses(t, f)
}

func TestBackdatedInternalTransferThatBreaksLaterSaleIsRefusedWithSaleNamed(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	destinationID := seedTestAccountWithClass(t, f.database, "active", true, "asset", "security_holding")
	buy := buyOn(t, f, "2026-05-01", 3, 3000)
	sold, err := f.investmentService.Sell(ctx, sellInput(f, "2026-07-01", 3))
	require.NoError(t, err)
	before := buyReplacementPreviewSnapshot(t, f.database)
	input := internalTransferFromLot(f, destinationID, *buy.LotID, exact.New(1), 0)
	_, err = f.investmentService.PreviewInternalTransfer(ctx, input)
	require.ErrorIs(t, err, ErrInvestmentSaleDependency)
	_, err = f.investmentService.InternalTransfer(ctx, input)
	require.ErrorIs(t, err, ErrInvestmentSaleDependency)
	var dependency InvestmentSaleDependencyError
	require.ErrorAs(t, err, &dependency)
	assert.Equal(t, transferOperationID(t, f, sold.Transaction.ID), dependency.OperationID)
	require.Equal(t, before, buyReplacementPreviewSnapshot(t, f.database))
}

// A later transfer is a dependency like a later sale: here the source's July
// move of the whole lot now finds one unit too few.
func TestBackdatedInternalTransferThatBreaksLaterSourceTransferIsRefused(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	destinationID := seedTestAccountWithClass(t, f.database, "active", true, "asset", "security_holding")
	buy := buyOn(t, f, "2026-05-01", 2, 2000)
	later := internalTransferFromLot(f, destinationID, *buy.LotID, exact.New(2), 0)
	later.EffectiveOn = "2026-07-01"
	moved, err := f.investmentService.InternalTransfer(ctx, later)
	require.NoError(t, err)
	before := buyReplacementPreviewSnapshot(t, f.database)
	_, err = f.investmentService.InternalTransfer(ctx,
		internalTransferFromLot(f, destinationID, *buy.LotID, exact.New(1), 0))
	require.ErrorIs(t, err, ErrInvestmentSaleDependency)
	var dependency InvestmentSaleDependencyError
	require.ErrorAs(t, err, &dependency)
	assert.Equal(t, transferOperationID(t, f, moved.Transaction.ID), dependency.OperationID)
	require.Equal(t, before, buyReplacementPreviewSnapshot(t, f.database))
}

// An acknowledgement planned before the book moved on is stale: the commit
// refuses and every fact, lot, revision, journal and audit row is unchanged.
func TestBackdatedInternalTransferRefusesStaleAcknowledgementWithoutWriting(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	destinationID := seedTestAccountWithClass(t, f.database, "active", true, "asset", "security_holding")
	first := buyOn(t, f, "2026-05-01", 3, 3000)
	buyOn(t, f, "2026-05-15", 3, 6000)
	_, err := f.investmentService.Sell(ctx, sellInput(f, "2026-07-01", 2))
	require.NoError(t, err)
	input := internalTransferFromLot(f, destinationID, *first.LotID, exact.New(2), 0)
	preview, err := f.investmentService.PreviewInternalTransfer(ctx, input)
	require.NoError(t, err)
	require.NotNil(t, preview.Impact.GainImpact)

	_, err = f.investmentService.Sell(ctx, sellInput(f, "2026-07-02", 1))
	require.NoError(t, err)
	before := buyReplacementPreviewSnapshot(t, f.database)
	input.GainImpactAcknowledgement = preview.Impact.GainImpact.Acknowledgement
	_, err = f.investmentService.InternalTransfer(ctx, input)
	require.ErrorIs(t, err, db.ErrGainImpactAcknowledgementStale)
	require.Equal(t, before, buyReplacementPreviewSnapshot(t, f.database))
}
