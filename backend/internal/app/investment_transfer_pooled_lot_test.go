package app

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rekenraam/backend/internal/db"
	"rekenraam/backend/internal/exact"
)

// T-135: an average-cost source moves its units as one pooled destination
// lot by default. The lot's identity and quantity are fixed; its carried
// basis, its original date (the latest among the units moved) and the source
// lots the pool took them from follow corrected history, so backdating into
// the source is admitted instead of refused.

// pooledLotLink is a pooled_lot transfer's effective link: carried basis,
// original date and revision count.
func pooledLotLink(t *testing.T, f *investmentsTestFixture, transactionID int64) (*exact.ScaledInt, string, int) {
	t.Helper()
	var value exact.Coefficient
	var scale, revisions int
	var date string
	require.NoError(t, f.database.QueryRow(`
		SELECT x.carried_basis_value, x.carried_basis_scale, COALESCE(x.original_acquired_on, ''),
			(SELECT count(*) FROM investment_transfer_link_revisions r WHERE r.operation_id = x.operation_id)
		FROM effective_investment_transfer_links x WHERE x.operation_id = ? AND x.source_lot_id IS NULL`,
		transferOperationID(t, f, transactionID)).Scan(&value, &scale, &date, &revisions))
	return exact.ScaledIntFromCoefficient(value, scale), date, revisions
}

func destinationSale(f *investmentsTestFixture, destinationID int64, date string, method string, lots ...InvestmentLotAllocationInput) InvestmentTradeInput {
	input := tradeOn(f, destinationID, date, 1, 5000)
	input.CostBasisMethod = method
	input.LotAllocations = lots
	return input
}

func TestPooledLotTransferOpensOneDestinationLotWithLatestUnitDate(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	destinationID := seedTestAccountWithClass(t, f.database, "active", true, "asset", "security_holding")
	setHoldingCostBasisMethod(t, f, "average_cost")
	first := buyOn(t, f, "2026-01-01", 2, 2000)
	second := buyOn(t, f, "2026-01-15", 2, 4000)
	buyOn(t, f, "2026-01-20", 2, 6000)

	// 3 of 6 shares pooled at 120.00 carry 60.00. FIFO lineage takes two
	// January 1 units and one January 15 unit; the lot is dated by the latest.
	result, err := f.investmentService.InternalTransfer(ctx,
		pooledTransferInput(f, destinationID, "2026-02-01", exact.New(3), 0))
	require.NoError(t, err)
	assert.Equal(t, db.InternalTransferPooledLot, result.Plan.DestinationLineage)
	require.Len(t, result.Plan.Links, 1)
	require.Len(t, result.DestinationLotIDs, 1)
	link := result.Plan.Links[0]
	assert.Zero(t, link.SourceLotID)
	assert.Equal(t, "3", link.QuantityValue.String())
	assertMoneyValue(t, 6000, 2, link.CarriedBasisValue, link.CarriedBasisScale, "pooled carried basis")
	assert.Equal(t, "known", link.OriginalDateKnowledge)
	assert.Equal(t, "2026-01-15", link.OriginalAcquiredOn)

	var openedOn, quantity string
	var basis exact.Coefficient
	var basisScale int
	require.NoError(t, f.database.QueryRow(`SELECT opened_on, quantity_value, cost_basis_value, cost_basis_scale
		FROM investment_lots WHERE id = ?`, result.DestinationLotIDs[0]).Scan(&openedOn, &quantity, &basis, &basisScale))
	assert.Equal(t, "2026-02-01", openedOn, "the lot enters this account on the transfer date")
	assert.Equal(t, "3", quantity)
	requireScaled(t, 6000, 2, exact.ScaledIntFromCoefficient(basis, basisScale), "destination opening basis")
	// Both source depletions are effects of the one operation, in pool order.
	rows, err := f.database.Query(`SELECT e.lot_id FROM investment_operation_lot_effects x
		JOIN investment_lot_events e ON e.id = x.lot_event_id AND e.event_kind = 'transfer_out'
		WHERE x.operation_id = ? ORDER BY x.effect_seq`, transferOperationID(t, f, result.Transaction.ID))
	require.NoError(t, err)
	var sources []int64
	for rows.Next() {
		var lotID int64
		require.NoError(t, rows.Scan(&lotID))
		sources = append(sources, lotID)
	}
	require.NoError(t, rows.Close())
	assert.Equal(t, []int64{*first.LotID, *second.LotID}, sources)

	// Moving the pooled lot on from an individual-lot account keeps its date.
	third := seedTestAccountWithClass(t, f.database, "active", true, "asset", "security_holding")
	onwardInput := internalTransferFromLot(f, third, result.DestinationLotIDs[0], exact.New(1), 0)
	onwardInput.SourceAccountID = destinationID
	moved, err := f.investmentService.InternalTransfer(ctx, onwardInput)
	require.NoError(t, err)
	assert.Equal(t, "2026-01-15", moved.Plan.Links[0].OriginalAcquiredOn)
	requireInvestmentSelfCheckPasses(t, f)
}

// The acceptance case: a backdated buy into the average-cost source after a
// pooled transfer changes the pool's FIFO lineage. The transfer's lot keeps
// its ID and quantity; its basis and date are revised and the destination's
// FIFO sale is replayed against the new date.
func TestBackdatedBuyRevisesPooledLotTransferAndDestinationFIFOSale(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	destinationID := seedTestAccountWithClass(t, f.database, "active", true, "asset", "security_holding")
	setHoldingCostBasisMethod(t, f, "average_cost")
	buyOn(t, f, "2026-02-01", 2, 2000)
	ownLot, err := f.investmentService.Buy(ctx, tradeOn(f, destinationID, "2026-01-15", 1, 3000))
	require.NoError(t, err)
	transfer, err := f.investmentService.InternalTransfer(ctx,
		pooledTransferInput(f, destinationID, "2026-03-01", exact.New(1), 0))
	require.NoError(t, err)
	pooledLot := transfer.DestinationLotIDs[0]
	basis, date, revisions := pooledLotLink(t, f, transfer.Transaction.ID)
	requireScaled(t, 1000, 2, basis, "pool rate before backdating")
	assert.Equal(t, "2026-02-01", date)
	assert.Zero(t, revisions)

	// FIFO in the destination: its own January 15 lot is older than the
	// pooled lot's February 1 date, so the April sale takes 30.00.
	sale, err := f.investmentService.Sell(ctx, destinationSale(f, destinationID, "2026-04-01", "fifo"))
	require.NoError(t, err)
	requireScaled(t, 3000, 2, saleEffectiveBasis(t, f, sale.Transaction.ID), "destination FIFO before backdating")

	// January 1: 2 shares at 40.00. The pool is now 4 shares at 60.00, so the
	// March transfer carries 15.00 from the January 1 lot, dated January 1.
	input := backdatedBuy(f, "2026-01-01", 2, 4000)
	before := buyReplacementPreviewSnapshot(t, f.database)
	impact := previewBuyGainImpact(t, f, input)
	assert.NotEmpty(t, impact.Changes, "the destination sale's gain changes")
	require.Equal(t, before, buyReplacementPreviewSnapshot(t, f.database), "preview writes nothing")
	backdated := acknowledgedBuy(t, f, input)

	basis, date, revisions = pooledLotLink(t, f, transfer.Transaction.ID)
	requireScaled(t, 1500, 2, basis, "carried basis at the new pool rate")
	assert.Equal(t, "2026-01-01", date, "the pool's FIFO lineage now moves the January 1 unit")
	assert.Equal(t, 1, revisions)
	var depletedLot int64
	var depletedQuantity string
	var depletedBasis exact.Coefficient
	var depletedScale int
	require.NoError(t, f.database.QueryRow(`SELECT d.source_lot_id, d.quantity_value, d.cost_basis_value, d.cost_basis_scale
		FROM latest_investment_transfer_link_revisions r
		JOIN investment_transfer_link_revision_depletions d ON d.revision_id = r.id`).Scan(
		&depletedLot, &depletedQuantity, &depletedBasis, &depletedScale))
	assert.Equal(t, *backdated.LotID, depletedLot)
	assert.Equal(t, "1", depletedQuantity)
	requireScaled(t, 1500, 2, exact.ScaledIntFromCoefficient(depletedBasis, depletedScale), "revised depletion basis")

	// Same lot ID and quantity; the FIFO sale now takes the older pooled lot.
	var quantity string
	require.NoError(t, f.database.QueryRow(`SELECT quantity_value FROM investment_lots WHERE id = ?`, pooledLot).Scan(&quantity))
	assert.Equal(t, "1", quantity)
	requireScaled(t, 1500, 2, saleEffectiveBasis(t, f, sale.Transaction.ID), "destination FIFO after backdating")
	requireScaled(t, 3000, 2, lotRemainingBasis(t, f, *ownLot.LotID), "own lot survives")
	sourceQty, sourceBasis := openPositionBasis(t, f, f.holdingAccountID)
	requireScaled(t, 3, 0, sourceQty, "source keeps three shares")
	requireScaled(t, 4500, 2, sourceBasis, "source keeps the rest of the pool")
	requireInvestmentSelfCheckPasses(t, f)
}

// A destination specific-lot sale elected the pooled lot. Backdating into the
// source keeps that election valid: the lot is the same, only its basis moves.
func TestBackdatedBuyKeepsDestinationSpecificLotElectionOnPooledLot(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	destinationID := seedTestAccountWithClass(t, f.database, "active", true, "asset", "security_holding")
	setHoldingCostBasisMethod(t, f, "average_cost")
	buyOn(t, f, "2026-02-01", 2, 2000)
	transfer, err := f.investmentService.InternalTransfer(ctx,
		pooledTransferInput(f, destinationID, "2026-03-01", exact.New(2), 0))
	require.NoError(t, err)
	pooledLot := transfer.DestinationLotIDs[0]
	sale, err := f.investmentService.Sell(ctx, destinationSale(f, destinationID, "2026-04-01", "specific_lot",
		InvestmentLotAllocationInput{LotID: pooledLot, QuantityValue: exact.New(1)}))
	require.NoError(t, err)
	requireScaled(t, 1000, 2, saleEffectiveBasis(t, f, sale.Transaction.ID), "specific lot before backdating")

	// January 1: 2 shares at 40.00 make the pool 4 shares at 60.00; moving 2
	// now carries 30.00, so the elected unit is worth 15.00.
	acknowledgedBuy(t, f, backdatedBuy(f, "2026-01-01", 2, 4000))
	basis, date, _ := pooledLotLink(t, f, transfer.Transaction.ID)
	requireScaled(t, 3000, 2, basis, "carried basis")
	assert.Equal(t, "2026-01-01", date)
	var allocated int64
	require.NoError(t, f.database.QueryRow(`SELECT a.lot_id FROM latest_investment_disposal_revisions r
		JOIN investment_disposal_decisions d ON d.id = r.decision_id
		JOIN investment_disposal_revision_allocations a ON a.revision_id = r.id
		WHERE d.transaction_id = ?`, sale.Transaction.ID).Scan(&allocated))
	assert.Equal(t, pooledLot, allocated, "the election still names the pooled lot")
	requireScaled(t, 1500, 2, saleEffectiveBasis(t, f, sale.Transaction.ID), "specific lot after backdating")
	requireScaled(t, 1500, 2, lotRemainingBasis(t, f, pooledLot), "remaining pooled unit")
	requireInvestmentSelfCheckPasses(t, f)
}

// History that leaves the pool's depletion unchanged appends no revision.
func TestPooledLotTransferReplaysUnchangedWithoutRevision(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	destinationID := seedTestAccountWithClass(t, f.database, "active", true, "asset", "security_holding")
	setHoldingCostBasisMethod(t, f, "average_cost")
	buyOn(t, f, "2026-01-01", 1, 1000)
	buyOn(t, f, "2026-01-02", 2, 2001)
	transfer, err := f.investmentService.InternalTransfer(ctx,
		pooledTransferInput(f, destinationID, "2026-02-01", exact.New(2), 0))
	require.NoError(t, err)
	sale, err := f.investmentService.Sell(ctx, sellInput(f, "2026-03-01", 1))
	require.NoError(t, err)
	_, err = acknowledgedReverseSale(ctx, f.investmentService, ReverseInvestmentSaleInput{
		OwnerUserID: f.ownerUserID, TransactionID: sale.Transaction.ID, Reason: "duplicate sale",
	})
	require.NoError(t, err)
	_, _, revisions := pooledLotLink(t, f, transfer.Transaction.ID)
	assert.Zero(t, revisions)
	intents, err := f.investmentService.repository.ListInvestmentReplayIntents(ctx,
		BookID, f.holdingAccountID, f.stockCommodityID, f.eurCommodityID, "long")
	require.NoError(t, err)
	var pooled int
	for _, intent := range intents {
		if intent.Kind == "pooled_lot_transfer_out" {
			pooled++
			assert.Equal(t, "2", intent.QuantityValue.String())
			assert.Len(t, intent.PooledDepletions, 2)
		}
		assert.NotContains(t, []string{"transfer_out", "pooled_transfer_out"}, intent.Kind)
	}
	assert.Equal(t, 1, pooled)
	requireInvestmentSelfCheckPasses(t, f)
}

func TestPooledLotLineageNeedsPooledQuantity(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	destinationID := seedTestAccountWithClass(t, f.database, "active", true, "asset", "security_holding")
	buy := buyOn(t, f, "2026-01-01", 2, 2000)
	input := internalTransferFromLot(f, destinationID, *buy.LotID, exact.New(1), 0)
	input.DestinationLineage = db.InternalTransferPooledLot
	before := buyReplacementPreviewSnapshot(t, f.database)
	_, err := f.investmentService.InternalTransfer(ctx, input)
	var validation ValidationError
	require.ErrorAs(t, err, &validation)
	input.DestinationLineage = "whole_pool"
	_, err = f.investmentService.InternalTransfer(ctx, input)
	require.ErrorAs(t, err, &validation)
	require.Equal(t, before, buyReplacementPreviewSnapshot(t, f.database))
}

// A revised pooled-lot depletion set that no longer carries its link's basis
// must surface in self-check.
func TestSelfCheckDetectsDamagedPooledLotRevision(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	destinationID := seedTestAccountWithClass(t, f.database, "active", true, "asset", "security_holding")
	setHoldingCostBasisMethod(t, f, "average_cost")
	buyOn(t, f, "2026-02-01", 2, 2000)
	_, err := f.investmentService.InternalTransfer(ctx,
		pooledTransferInput(f, destinationID, "2026-03-01", exact.New(1), 0))
	require.NoError(t, err)
	acknowledgedBuy(t, f, backdatedBuy(f, "2026-01-01", 2, 4000))
	requireInvestmentSelfCheckPasses(t, f)

	_, err = f.database.Exec(`DROP TRIGGER investment_transfer_link_revision_depletions_no_update`)
	require.NoError(t, err)
	_, err = f.database.Exec(`UPDATE investment_transfer_link_revision_depletions SET cost_basis_value = '14000000'`)
	require.NoError(t, err)
	run, err := selfCheckOver(t, f.database).RunSelfCheck(ctx, "manual")
	require.NoError(t, err)
	foundation := resultFor(t, run, CheckInvestmentFoundation)
	assert.Equal(t, SelfCheckFailed, foundation.Status)
	assert.Contains(t, foundation.Summary, "pooled transfer depletion sets")
	assert.Equal(t, SelfCheckFailed, resultFor(t, run, CheckLotReconciliation).Status)
}

// Reversing an acquisition the pool's lineage moved is admitted while the
// pool still covers the transfer: the pooled lot is re-carried from the lots
// that remain. A source_lots transfer refuses the same reversal
// (TestRemovingTransferredAcquisitionStaysNamedRefusal).
func TestReversingPooledAcquisitionRecarriesPooledLotTransfer(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	destinationID := seedTestAccountWithClass(t, f.database, "active", true, "asset", "security_holding")
	setHoldingCostBasisMethod(t, f, "average_cost")
	duplicate := buyOn(t, f, "2026-01-01", 1, 1000)
	kept := buyOn(t, f, "2026-01-15", 2, 6000)
	transfer, err := f.investmentService.InternalTransfer(ctx,
		pooledTransferInput(f, destinationID, "2026-03-01", exact.New(1), 0))
	require.NoError(t, err)
	basis, date, _ := pooledLotLink(t, f, transfer.Transaction.ID)
	requireScaled(t, 2333333, 5, basis.TruncatedTo(5), "pool rate of 70.00 over 3 shares")
	assert.Equal(t, "2026-01-01", date)

	_, err = acknowledgedReverseBuy(ctx, f.investmentService, ReverseInvestmentBuyInput{
		OwnerUserID: f.ownerUserID, TransactionID: duplicate.Transaction.ID, Reason: "duplicate"})
	require.NoError(t, err)
	basis, date, revisions := pooledLotLink(t, f, transfer.Transaction.ID)
	requireScaled(t, 3000, 2, basis, "re-carried from the remaining pool")
	assert.Equal(t, "2026-01-15", date)
	assert.Equal(t, 1, revisions)
	var depletedLot int64
	require.NoError(t, f.database.QueryRow(`SELECT d.source_lot_id FROM latest_investment_transfer_link_revisions r
		JOIN investment_transfer_link_revision_depletions d ON d.revision_id = r.id`).Scan(&depletedLot))
	assert.Equal(t, *kept.LotID, depletedLot)
	requireInvestmentSelfCheckPasses(t, f)
}
