package app

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rekenraam/backend/internal/db"
	"rekenraam/backend/internal/exact"
)

// Pooled internal transfers (T-123). An average-cost source moves a quantity
// at its dated pool rate; source plus destination basis is conserved exactly.

func pooledTransferInput(f *investmentsTestFixture, destinationID int64, date string, quantity exact.Coefficient, scale int) InternalTransferInput {
	return InternalTransferInput{
		OwnerUserID: f.ownerUserID, EffectiveOn: date,
		SourceAccountID: f.holdingAccountID, DestinationAccountID: destinationID,
		CommodityID: f.stockCommodityID, CostCommodityID: f.eurCommodityID,
		QuantityValue: quantity, QuantityScale: scale,
		SourceEvidenceJSON: `{"statement":"pooled move"}`, Memo: "move pooled holdings",
	}
}

// openPositionBasis sums the remaining quantity and basis of an account's
// open lots in the fixture's security and EUR.
func openPositionBasis(t *testing.T, f *investmentsTestFixture, accountID int64) (*exact.ScaledInt, *exact.ScaledInt) {
	t.Helper()
	rows, err := f.database.Query(`SELECT remaining_quantity_value, remaining_quantity_scale,
		remaining_cost_basis_value, remaining_cost_basis_scale FROM current_investment_lots
		WHERE account_id = ? AND commodity_id = ? AND cost_commodity_id = ? AND status = 'open'`,
		accountID, f.stockCommodityID, f.eurCommodityID)
	require.NoError(t, err)
	defer rows.Close()
	quantity, basis := exact.NewScaledInt(), exact.NewScaledInt()
	for rows.Next() {
		var qty, cost exact.Coefficient
		var qtyScale, costScale int
		require.NoError(t, rows.Scan(&qty, &qtyScale, &cost, &costScale))
		quantity.AddCoefficient(qty, qtyScale)
		basis.AddCoefficient(cost, costScale)
	}
	require.NoError(t, rows.Err())
	return quantity, basis
}

func linkedBasis(plan InternalTransferPlan) *exact.ScaledInt {
	total := exact.NewScaledInt()
	for _, link := range plan.Links {
		total.AddInt64(link.CarriedBasisValue, link.CarriedBasisScale)
	}
	return total
}

func positionMethodFamily(t *testing.T, f *investmentsTestFixture, accountID int64) string {
	t.Helper()
	var family string
	err := f.database.QueryRow(`SELECT COALESCE((SELECT method_family FROM investment_position_basis_state
		WHERE account_id = ? AND commodity_id = ? AND cost_commodity_id = ? AND position_side = 'long'), '')`,
		accountID, f.stockCommodityID, f.eurCommodityID).Scan(&family)
	require.NoError(t, err)
	return family
}

func requireConserved(t *testing.T, want *exact.ScaledInt, parts ...*exact.ScaledInt) {
	t.Helper()
	total := exact.NewScaledInt()
	for _, part := range parts {
		total.AddScaled(part)
	}
	require.Zerof(t, total.Cmp(want), "basis not conserved: got %s, want %s", total.String(), want.String())
}

func TestPooledTransferAverageDefaultBeforeFirstSaleCarriesPoolRate(t *testing.T) {
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	destinationID := seedTestAccountWithClass(t, f.database, "active", true, "asset", "security_holding")
	first := buyOn(t, f, "2026-01-01", 1, 1000)
	buyOn(t, f, "2026-01-02", 1, 2000)
	account := setHoldingCostBasisMethod(t, f, "average_cost")

	input := pooledTransferInput(f, destinationID, "2026-06-01", exact.New(1), 0)
	preview, err := f.investmentService.PreviewInternalTransfer(ctx, input)
	require.NoError(t, err)
	before := f.transactionCount(t)
	var auditsBefore int
	require.NoError(t, f.database.QueryRow(`SELECT COUNT(*) FROM audit_events`).Scan(&auditsBefore))
	result, err := f.investmentService.InternalTransfer(ctx, input)
	require.NoError(t, err)
	assert.Equal(t, before+1, f.transactionCount(t))
	var auditsAfter int
	require.NoError(t, f.database.QueryRow(`SELECT COUNT(*) FROM audit_events`).Scan(&auditsAfter))
	assert.Equal(t, auditsBefore+1, auditsAfter, "one audited operation")

	// The FIFO-linked January lot carries the pool rate (15.00), not its own
	// 10.00; the preview showed exactly what the commit carried.
	require.Len(t, result.Plan.Links, 1)
	link := result.Plan.Links[0]
	assert.Equal(t, db.InternalTransferAverageCostPool, result.Plan.BasisAllocation)
	assert.Equal(t, "average_cost", result.Plan.CostBasisMethod)
	assert.Equal(t, "account", result.Plan.ResolutionTier)
	assert.Equal(t, *first.LotID, link.SourceLotID)
	assert.Equal(t, "2026-01-01", link.OriginalAcquiredOn)
	assertMoneyValue(t, 1500, 2, link.CarriedBasisValue, link.CarriedBasisScale, "pool-rate carried basis")
	require.Len(t, preview.Plan.Links, 1)
	assert.Equal(t, link.SourceLotID, preview.Plan.Links[0].SourceLotID)
	assertMoneyValue(t, link.CarriedBasisValue, link.CarriedBasisScale,
		preview.Plan.Links[0].CarriedBasisValue, preview.Plan.Links[0].CarriedBasisScale, "preview equals commit")
	assert.Zero(t, preview.Plan.Links[0].DestinationLotID, "preview exposes no temporary lot ID")

	sourceQty, sourceBasis := openPositionBasis(t, f, f.holdingAccountID)
	destinationQty, destinationBasis := openPositionBasis(t, f, destinationID)
	assert.Zero(t, sourceQty.Cmp(exact.ScaledIntFromInt64(1, 0)))
	assert.Zero(t, destinationQty.Cmp(exact.ScaledIntFromInt64(1, 0)))
	requireConserved(t, exact.ScaledIntFromInt64(3000, 2), sourceBasis, destinationBasis)

	// The policy is snapshotted with its provenance, and the source is now
	// locked to average cost even though it has never been sold from.
	var allocation, method, tier string
	var accountVersionID int64
	require.NoError(t, f.database.QueryRow(`SELECT basis_allocation, cost_basis_method, method_resolution_tier,
		method_account_version_id FROM investment_transfer_facts WHERE transfer_kind = 'internal'`).Scan(
		&allocation, &method, &tier, &accountVersionID))
	assert.Equal(t, "average_cost_pool", allocation)
	assert.Equal(t, "average_cost", method)
	assert.Equal(t, "account", tier)
	assert.Equal(t, account.VersionID, accountVersionID)
	assert.Equal(t, "average_cost", positionMethodFamily(t, f, f.holdingAccountID))
	// Original acquisition evidence is never rewritten.
	var originalBasis string
	require.NoError(t, f.database.QueryRow(`SELECT cost_basis_value FROM investment_lots WHERE id = ?`,
		*first.LotID).Scan(&originalBasis))
	assert.Equal(t, "1000", originalBasis)
	assert.Equal(t, SelfCheckPassed, resultFor(t, mustRunInvestmentSelfCheck(t, f), CheckInvestmentFoundation).Status)
}

func TestPooledTransferFromPartiallySoldPoolUsesDatedPoolAndPositionLock(t *testing.T) {
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	destinationID := seedTestAccountWithClass(t, f.database, "active", true, "asset", "security_holding")
	buyOn(t, f, "2026-01-01", 2, 2000)
	sale := sellInput(f, "2026-02-01", 1)
	sale.CostBasisMethod = "average_cost"
	_, err := f.investmentService.Sell(ctx, sale)
	require.NoError(t, err)
	buyOn(t, f, "2026-03-01", 1, 2000)
	// Pool: 10.00 survivor + 20.00 later buy over 2 shares. The book default
	// is FIFO, but the open average-cost position decides the allocation.
	_, poolBasis := openPositionBasis(t, f, f.holdingAccountID)
	requireConserved(t, exact.ScaledIntFromInt64(3000, 2), poolBasis)

	result, err := f.investmentService.InternalTransfer(ctx,
		pooledTransferInput(f, destinationID, "2026-04-01", exact.New(1), 0))
	require.NoError(t, err)
	assert.Equal(t, "position_lock", result.Plan.ResolutionTier)
	assert.Equal(t, "average_cost", result.Plan.CostBasisMethod)
	assert.Zero(t, linkedBasis(result.Plan).Cmp(exact.ScaledIntFromInt64(1500, 2)), "dated pool rate")
	_, sourceBasis := openPositionBasis(t, f, f.holdingAccountID)
	_, destinationBasis := openPositionBasis(t, f, destinationID)
	requireConserved(t, exact.ScaledIntFromInt64(3000, 2), sourceBasis, destinationBasis)

	// Selected lots remain refused for this source; a pooled request against
	// an individual-lot source is refused the other way round.
	_, err = f.investmentService.InternalTransfer(ctx,
		internalTransferFromLot(f, destinationID, result.Plan.Links[0].SourceLotID, exact.New(1), 0))
	require.ErrorIs(t, err, db.ErrAverageCostTransferRequiresPoolAllocation)
	assert.Equal(t, SelfCheckPassed, resultFor(t, mustRunInvestmentSelfCheck(t, f), CheckInvestmentFoundation).Status)
}

func TestPooledTransferRefusedForIndividualLotSourceWithoutWriting(t *testing.T) {
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	destinationID := seedTestAccountWithClass(t, f.database, "active", true, "asset", "security_holding")
	buyOn(t, f, "2026-01-01", 2, 2000)
	input := pooledTransferInput(f, destinationID, "2026-02-01", exact.New(1), 0)
	before := buyReplacementPreviewSnapshot(t, f.database)
	_, err := f.investmentService.PreviewInternalTransfer(ctx, input)
	require.ErrorIs(t, err, db.ErrPooledTransferRequiresAverageCost)
	_, err = f.investmentService.InternalTransfer(ctx, input)
	require.ErrorIs(t, err, db.ErrPooledTransferRequiresAverageCost)
	require.Equal(t, before, buyReplacementPreviewSnapshot(t, f.database))
	assert.Empty(t, positionMethodFamily(t, f, f.holdingAccountID))
}

func TestPooledTransferMixedScalesConserveBasis(t *testing.T) {
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	destinationID := seedTestAccountWithClass(t, f.database, "active", true, "asset", "security_holding")
	setHoldingCostBasisMethod(t, f, "average_cost")
	buyOn(t, f, "2026-01-01", 1, 1000)
	_, err := f.investmentService.Buy(ctx, InvestmentTradeInput{
		OwnerUserID: f.ownerUserID, TransactionDate: "2026-01-02", CommodityID: f.stockCommodityID,
		HoldingAccountID: f.holdingAccountID, CashAccountID: f.cashAccountID,
		QuantityValue: exact.New(15), QuantityScale: 1, CashAmountValue: 30000, CashAmountScale: 3,
		CashCommodityID: f.eurCommodityID,
	})
	require.NoError(t, err)
	// 2.5 shares pooled at 40.00: moving 0.500 (scale 3) carries 8.00.
	result, err := f.investmentService.InternalTransfer(ctx,
		pooledTransferInput(f, destinationID, "2026-02-01", exact.New(500), 3))
	require.NoError(t, err)
	carried := linkedBasis(result.Plan)
	assert.Zero(t, carried.Cmp(exact.ScaledIntFromInt64(800, 2)), "carried %s", carried.String())
	moved := exact.NewScaledInt()
	for _, link := range result.Plan.Links {
		moved.AddCoefficient(link.QuantityValue, link.QuantityScale)
	}
	assert.Zero(t, moved.Cmp(exact.ScaledIntFromInt64(5, 1)))
	sourceQty, sourceBasis := openPositionBasis(t, f, f.holdingAccountID)
	_, destinationBasis := openPositionBasis(t, f, destinationID)
	assert.Zero(t, sourceQty.Cmp(exact.ScaledIntFromInt64(2, 0)))
	requireConserved(t, exact.ScaledIntFromInt64(4000, 2), sourceBasis, destinationBasis)
	assert.Equal(t, SelfCheckPassed, resultFor(t, mustRunInvestmentSelfCheck(t, f), CheckInvestmentFoundation).Status)
}

func TestPooledTransfersRepeatedUntilFullKeepExactRemainder(t *testing.T) {
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	destinationID := seedTestAccountWithClass(t, f.database, "active", true, "asset", "security_holding")
	setHoldingCostBasisMethod(t, f, "average_cost")
	// Three lots, 10.00 total: a third is not representable at any scale.
	buyOn(t, f, "2026-01-01", 1, 333)
	buyOn(t, f, "2026-01-02", 1, 333)
	buyOn(t, f, "2026-01-03", 1, 334)
	total := exact.ScaledIntFromInt64(1000, 2)
	carried := exact.NewScaledInt()
	for index, date := range []string{"2026-02-01", "2026-03-01", "2026-04-01"} {
		result, err := f.investmentService.InternalTransfer(ctx,
			pooledTransferInput(f, destinationID, date, exact.New(1), 0))
		require.NoError(t, err, "transfer %d", index+1)
		carried.AddScaled(linkedBasis(result.Plan))
		_, sourceBasis := openPositionBasis(t, f, f.holdingAccountID)
		_, destinationBasis := openPositionBasis(t, f, destinationID)
		// Conserved after every partial step, not only at the end.
		requireConserved(t, total, sourceBasis, destinationBasis)
		requireConserved(t, total, sourceBasis, carried)
	}
	// The final full move carries the exact remainder; nothing is lost to
	// truncation, and the closed source releases its method lock.
	assert.Zero(t, carried.Cmp(total), "carried %s", carried.String())
	sourceQty, _ := openPositionBasis(t, f, f.holdingAccountID)
	assert.Zero(t, sourceQty.Sign())
	assert.Empty(t, positionMethodFamily(t, f, f.holdingAccountID))
	var links []string
	rows, err := f.database.Query(`SELECT carried_basis_value FROM investment_transfer_lot_links ORDER BY operation_id, link_seq`)
	require.NoError(t, err)
	for rows.Next() {
		var value string
		require.NoError(t, rows.Scan(&value))
		links = append(links, value)
	}
	require.NoError(t, rows.Close())
	// 10.00/3 truncates at the six-digit allocation scale; the survivor
	// absorbs the remainder and the last move carries it.
	assert.Equal(t, []string{"3333333", "3333333", "3333334"}, links)
	assert.Equal(t, SelfCheckPassed, resultFor(t, mustRunInvestmentSelfCheck(t, f), CheckInvestmentFoundation).Status)
}

func TestPooledTransferSpanningSourceLotsKeepsLineageAndOriginalDates(t *testing.T) {
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	destinationID := seedTestAccountWithClass(t, f.database, "active", true, "asset", "security_holding")
	setHoldingCostBasisMethod(t, f, "average_cost")
	first := buyOn(t, f, "2026-01-01", 2, 2000)
	second := buyOn(t, f, "2026-01-15", 2, 4000)
	buyOn(t, f, "2026-01-20", 2, 6000)
	result, err := f.investmentService.InternalTransfer(ctx,
		pooledTransferInput(f, destinationID, "2026-02-01", exact.New(3), 0))
	require.NoError(t, err)
	require.Len(t, result.Plan.Links, 2)
	require.Len(t, result.DestinationLotIDs, 2)
	assert.Equal(t, *first.LotID, result.Plan.Links[0].SourceLotID)
	assert.Equal(t, "2", result.Plan.Links[0].QuantityValue.String())
	assert.Equal(t, "2026-01-01", result.Plan.Links[0].OriginalAcquiredOn)
	assert.Equal(t, *second.LotID, result.Plan.Links[1].SourceLotID)
	assert.Equal(t, "1", result.Plan.Links[1].QuantityValue.String())
	assert.Equal(t, "2026-01-15", result.Plan.Links[1].OriginalAcquiredOn)
	// 6 shares at 120.00: 3 shares carry 60.00 at the pool rate of 20.00.
	assertMoneyValue(t, 4000, 2, result.Plan.Links[0].CarriedBasisValue, result.Plan.Links[0].CarriedBasisScale)
	assertMoneyValue(t, 2000, 2, result.Plan.Links[1].CarriedBasisValue, result.Plan.Links[1].CarriedBasisScale)
	for index, lotID := range result.DestinationLotIDs {
		var openedOn, basis string
		require.NoError(t, f.database.QueryRow(`SELECT opened_on, cost_basis_value FROM investment_lots WHERE id = ?`,
			lotID).Scan(&openedOn, &basis))
		assert.Equal(t, "2026-02-01", openedOn, "destination lot %d opens on the transfer date", index)
		assert.Equal(t, exact.New(result.Plan.Links[index].CarriedBasisValue).String(), basis)
	}
	// A further move of a transferred lot keeps the first original date.
	destinationResult, err := f.investmentService.InternalTransfer(ctx, InternalTransferInput{
		OwnerUserID: f.ownerUserID, EffectiveOn: "2026-03-01", SourceAccountID: destinationID,
		DestinationAccountID: f.holdingAccountID, CommodityID: f.stockCommodityID, CostCommodityID: f.eurCommodityID,
		Allocations: []InvestmentLotAllocationInput{{LotID: result.DestinationLotIDs[1], QuantityValue: exact.New(1)}},
	})
	require.NoError(t, err)
	assert.Equal(t, "2026-01-15", destinationResult.Plan.Links[0].OriginalAcquiredOn)
	assert.Equal(t, db.InternalTransferSelectedLots, destinationResult.Plan.BasisAllocation)
	assert.Equal(t, SelfCheckPassed, resultFor(t, mustRunInvestmentSelfCheck(t, f), CheckInvestmentFoundation).Status)
}

func TestPooledTransferJoinsDestinationAverageCostPool(t *testing.T) {
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	destinationID := seedTestAccountWithClass(t, f.database, "active", true, "asset", "security_holding")
	setHoldingCostBasisMethod(t, f, "average_cost")
	buyOn(t, f, "2026-01-01", 2, 4000) // source pool: 20.00 per share
	destinationBuy := InvestmentTradeInput{
		OwnerUserID: f.ownerUserID, TransactionDate: "2026-01-01", CommodityID: f.stockCommodityID,
		HoldingAccountID: destinationID, CashAccountID: f.cashAccountID,
		QuantityValue: exact.New(2), CashAmountValue: 2000, CashAmountScale: 2, CashCommodityID: f.eurCommodityID,
	}
	_, err := f.investmentService.Buy(ctx, destinationBuy) // destination: 10.00 per share
	require.NoError(t, err)
	destinationSale := sellInput(f, "2026-01-15", 1)
	destinationSale.HoldingAccountID = destinationID
	destinationSale.CostBasisMethod = "average_cost"
	_, err = f.investmentService.Sell(ctx, destinationSale)
	require.NoError(t, err)
	assert.Equal(t, "average_cost", positionMethodFamily(t, f, destinationID))

	_, err = f.investmentService.InternalTransfer(ctx,
		pooledTransferInput(f, destinationID, "2026-02-01", exact.New(1), 0))
	require.NoError(t, err)
	// The destination keeps its own lock; its pool is now 10.00 + 20.00 over
	// two shares, so its next average-cost sale disposes 15.00 per share.
	assert.Equal(t, "average_cost", positionMethodFamily(t, f, destinationID))
	destinationSale.TransactionDate = "2026-03-01"
	destinationSale.CashAmountValue = 2500
	_, err = f.investmentService.Sell(ctx, destinationSale)
	require.NoError(t, err)
	gains, err := f.investmentService.ListRealizedGains(ctx, GainsReportParams{})
	require.NoError(t, err)
	var found bool
	for _, gain := range gains {
		if gain.AccountID == destinationID && gain.DisposalDate == "2026-03-01" {
			found = true
			assertMoneyValue(t, -1500, 2, gain.DisposedBasisValue, gain.DisposedBasisScale, "destination pool rate (credit sign)")
			assertMoneyValue(t, 1000, 2, gain.RealizedGainValue, gain.RealizedGainScale)
		}
	}
	require.True(t, found)
	assert.Equal(t, SelfCheckPassed, resultFor(t, mustRunInvestmentSelfCheck(t, f), CheckInvestmentFoundation).Status)
}

func TestPooledTransferLaterSourceAndDestinationSalesUseCarriedBasis(t *testing.T) {
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	destinationID := seedTestAccountWithClass(t, f.database, "active", true, "asset", "security_holding")
	setHoldingCostBasisMethod(t, f, "average_cost")
	buyOn(t, f, "2026-01-01", 1, 1000)
	buyOn(t, f, "2026-01-02", 3, 5000) // pool: 4 shares at 60.00
	_, err := f.investmentService.InternalTransfer(ctx,
		pooledTransferInput(f, destinationID, "2026-02-01", exact.New(2), 0))
	require.NoError(t, err)
	sourceSale := sellInput(f, "2026-03-01", 2)
	sourceSale.CashAmountValue = 4000
	_, err = f.investmentService.Sell(ctx, sourceSale)
	require.NoError(t, err)
	// The destination has no default of its own here: its FIFO lots each
	// carry 15.00 per share from the source pool.
	destinationSale := sellInput(f, "2026-03-01", 1)
	destinationSale.HoldingAccountID = destinationID
	destinationSale.CashAmountValue = 2000
	_, err = f.investmentService.Sell(ctx, destinationSale)
	require.NoError(t, err)
	gains, err := f.investmentService.ListRealizedGains(ctx, GainsReportParams{})
	require.NoError(t, err)
	byAccount := map[int64]RealizedGainEntry{}
	for _, gain := range gains {
		byAccount[gain.AccountID] = gain
	}
	require.Len(t, byAccount, 2)
	assertMoneyValue(t, -3000, 2, byAccount[f.holdingAccountID].DisposedBasisValue, byAccount[f.holdingAccountID].DisposedBasisScale)
	assertMoneyValue(t, -1500, 2, byAccount[destinationID].DisposedBasisValue, byAccount[destinationID].DisposedBasisScale)
	// Every unit's basis is accounted for exactly once (disposals are signed
	// as credits): 30.00 + 15.00 sold,
	// 15.00 still held in the destination.
	_, sourceBasis := openPositionBasis(t, f, f.holdingAccountID)
	_, destinationBasis := openPositionBasis(t, f, destinationID)
	requireConserved(t, exact.ScaledIntFromInt64(6000, 2), sourceBasis, destinationBasis,
		exact.ScaledIntFromInt64(-byAccount[f.holdingAccountID].DisposedBasisValue, byAccount[f.holdingAccountID].DisposedBasisScale),
		exact.ScaledIntFromInt64(-byAccount[destinationID].DisposedBasisValue, byAccount[destinationID].DisposedBasisScale))
	assert.Equal(t, SelfCheckPassed, resultFor(t, mustRunInvestmentSelfCheck(t, f), CheckInvestmentFoundation).Status)
}

func TestPooledTransferReplaysUnchangedAfterLaterSaleReversal(t *testing.T) {
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
	// Reversing the later sale replays the source position from its intents,
	// including the pooled transfer, which reproduces its carried basis.
	_, err = acknowledgedReverseSale(ctx, f.investmentService, ReverseInvestmentSaleInput{
		OwnerUserID: f.ownerUserID, TransactionID: sale.Transaction.ID, Reason: "duplicate sale",
	})
	require.NoError(t, err)
	_, sourceBasis := openPositionBasis(t, f, f.holdingAccountID)
	requireConserved(t, exact.ScaledIntFromInt64(3001, 2), sourceBasis, linkedBasis(transfer.Plan))
	intents, err := f.investmentService.repository.ListInvestmentReplayIntents(ctx,
		BookID, f.holdingAccountID, f.stockCommodityID, f.eurCommodityID, "long")
	require.NoError(t, err)
	var pooled int
	for _, intent := range intents {
		if intent.Kind == "pooled_transfer_out" {
			pooled++
			assert.Equal(t, "2", intent.QuantityValue.String())
			assert.Len(t, intent.PooledLinks, len(transfer.Plan.Links))
		}
		assert.NotEqual(t, "transfer_out", intent.Kind, "pooled links never replay as selected lots")
	}
	assert.Equal(t, 1, pooled)
	assert.Equal(t, SelfCheckPassed, resultFor(t, mustRunInvestmentSelfCheck(t, f), CheckInvestmentFoundation).Status)
}

func TestBackdatedBuyRefusesChangedPooledTransferBasisWithoutWriting(t *testing.T) {
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	destinationID := seedTestAccountWithClass(t, f.database, "active", true, "asset", "security_holding")
	setHoldingCostBasisMethod(t, f, "average_cost")
	buyOn(t, f, "2026-02-01", 2, 2000)
	transfer, err := f.investmentService.InternalTransfer(ctx,
		pooledTransferInput(f, destinationID, "2026-03-01", exact.New(1), 0))
	require.NoError(t, err)
	// A January purchase joins the March pool and would change the basis the
	// destination lot already carries; it is refused with the transfer named.
	input := InvestmentTradeInput{OwnerUserID: f.ownerUserID, TransactionDate: "2026-01-01",
		CommodityID: f.stockCommodityID, HoldingAccountID: f.holdingAccountID,
		CashAccountID: f.cashAccountID, CashCommodityID: f.eurCommodityID,
		QuantityValue: exact.New(2), CashAmountValue: 4000, CashAmountScale: 2}
	before := buyReplacementPreviewSnapshot(t, f.database)
	_, err = f.investmentService.TradeReconciliationImpact(ctx, InvestmentImpactBuy, input)
	require.ErrorIs(t, err, ErrInvestmentBuyDependency)
	_, err = f.investmentService.Buy(ctx, input)
	require.ErrorIs(t, err, ErrInvestmentBuyDependency)
	var dependency InvestmentBuyDependencyError
	require.ErrorAs(t, err, &dependency)
	var operationID int64
	require.NoError(t, f.database.QueryRow(`SELECT l.operation_id FROM investment_operation_journal_links l
		JOIN transaction_versions v ON v.id = l.transaction_version_id WHERE v.transaction_id = ?`,
		transfer.Transaction.ID).Scan(&operationID))
	assert.Equal(t, operationID, dependency.OperationID)
	require.Equal(t, before, buyReplacementPreviewSnapshot(t, f.database))
}

func TestPooledTransferLateReconciliationRefusalRollsBackEverything(t *testing.T) {
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	setHoldingCostBasisMethod(t, f, "average_cost")
	buyOn(t, f, "2026-01-01", 1, 1000)
	buyOn(t, f, "2026-01-02", 1, 2000)
	// Reconcile the source holding after the transfer date: the transfer's
	// posting would enter a reconciled period, which the guard refuses after
	// the pool depletion, destination lots and links are already written.
	session, err := f.transactionService.StartReconciliation(ctx, StartReconciliationInput{
		OriginType: "browser_api", OwnerUserID: f.ownerUserID, AccountID: f.holdingAccountID,
		CommodityID: f.stockCommodityID, StatementDate: "2026-06-30", StatementBalanceValue: exact.New(2),
	})
	require.NoError(t, err)
	postingIDs := make([]int64, 0, len(session.Candidates))
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

	destinationID := seedTestAccountWithClass(t, f.database, "active", true, "asset", "security_holding")
	input := pooledTransferInput(f, destinationID, "2026-06-01", exact.New(1), 0)
	preview, err := f.investmentService.PreviewInternalTransfer(ctx, input)
	require.NoError(t, err)
	require.NotEmpty(t, preview.Impact.AffectedCheckpoints)
	before := buyReplacementPreviewSnapshot(t, f.database)
	_, err = f.investmentService.InternalTransfer(ctx, input)
	require.ErrorIs(t, err, ErrReconciliationOverrideRequired)
	require.Equal(t, before, buyReplacementPreviewSnapshot(t, f.database))
	assert.Empty(t, positionMethodFamily(t, f, f.holdingAccountID), "no lock survives the rollback")
	var facts, events int
	require.NoError(t, f.database.QueryRow(`SELECT COUNT(*) FROM investment_transfer_facts`).Scan(&facts))
	require.NoError(t, f.database.QueryRow(`SELECT COUNT(*) FROM investment_lot_events WHERE event_kind IN ('transfer_out', 'transfer_in')`).Scan(&events))
	assert.Zero(t, facts)
	assert.Zero(t, events)

	input.ReconciliationOverride = true
	_, err = f.investmentService.InternalTransfer(ctx, input)
	require.NoError(t, err)
	assert.Equal(t, SelfCheckPassed, resultFor(t, mustRunInvestmentSelfCheck(t, f), CheckInvestmentFoundation).Status)
}

func TestPooledTransferRefusesUnknownBasisWithoutWriting(t *testing.T) {
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	destinationID := seedTestAccountWithClass(t, f.database, "active", true, "asset", "security_holding")
	setHoldingCostBasisMethod(t, f, "average_cost")
	buyOn(t, f, "2026-01-01", 1, 1000)
	unknown := buyOn(t, f, "2026-01-02", 1, 1000)
	// Unknown basis has no transfer contract yet; it is never pooled as zero.
	_, err := f.database.Exec(`UPDATE investment_lot_state SET basis_knowledge = 'unknown',
		remaining_cost_basis_value = NULL, remaining_cost_basis_scale = NULL WHERE lot_id = ?`, *unknown.LotID)
	require.NoError(t, err)
	before := buyReplacementPreviewSnapshot(t, f.database)
	input := pooledTransferInput(f, destinationID, "2026-02-01", exact.New(1), 0)
	_, err = f.investmentService.PreviewInternalTransfer(ctx, input)
	var validation ValidationError
	require.ErrorAs(t, err, &validation)
	_, err = f.investmentService.InternalTransfer(ctx, input)
	require.ErrorAs(t, err, &validation)
	require.Equal(t, before, buyReplacementPreviewSnapshot(t, f.database))
}
