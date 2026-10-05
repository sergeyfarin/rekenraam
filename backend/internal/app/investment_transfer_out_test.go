package app

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rekenraam/backend/internal/db"
	"rekenraam/backend/internal/exact"
)

// Slice 5 external outbound transfer: security legs H −q, T +q; a bridge
// journal T −b, E +b with b the exact basis the depletion took; no gain.

// newTransferOutFixture adds the transfer equity account the bridge posts to.
func newTransferOutFixture(t *testing.T) *investmentsTestFixture {
	t.Helper()
	f := newInvestmentsTestFixture(t)
	seedExternalTransferEquity(t, f.database)
	return f
}

func transferOutOfLot(f *investmentsTestFixture, date string, lotID int64, quantity int64) ExternalTransferOutInput {
	return ExternalTransferOutInput{
		OwnerUserID: f.ownerUserID, EffectiveOn: date, SourceAccountID: f.holdingAccountID,
		CommodityID: f.stockCommodityID, CostCommodityID: f.eurCommodityID,
		Allocations:        []InvestmentLotAllocationInput{{LotID: lotID, QuantityValue: exact.New(quantity)}},
		SourceEvidenceJSON: `{"broker_reported_basis":"19.00"}`, Memo: "moved to another broker",
	}
}

// bridgePostings returns an outbound transfer's bridge journal postings by
// account system role.
func bridgePostings(t *testing.T, f *investmentsTestFixture, transactionID int64) map[string]*exact.ScaledInt {
	t.Helper()
	rows, err := f.database.Query(`SELECT a.system_role, pv.quantity_value, pv.quantity_scale, pv.commodity_id
		FROM investment_operation_journal_links primary_link
		JOIN transaction_versions primary_version ON primary_version.id = primary_link.transaction_version_id
		JOIN investment_operation_journal_links bridge ON bridge.operation_id = primary_link.operation_id
			AND bridge.role = 'transfer_bridge'
		JOIN posting_versions pv ON pv.transaction_version_id = bridge.transaction_version_id
		JOIN accounts a ON a.id = pv.account_id
		WHERE primary_link.role = 'primary' AND primary_version.transaction_id = ?`, transactionID)
	require.NoError(t, err)
	defer rows.Close()
	postings := map[string]*exact.ScaledInt{}
	for rows.Next() {
		var role, value string
		var scale int
		var commodityID int64
		require.NoError(t, rows.Scan(&role, &value, &scale, &commodityID))
		assert.Equal(t, f.eurCommodityID, commodityID, "the bridge is in the basis currency")
		postings[role] = exact.ScaledIntFromCoefficient(exact.Coefficient(value), scale)
	}
	require.NoError(t, rows.Err())
	return postings
}

func TestExternalTransferOutPostsSecurityLegsAndExactBasisBridge(t *testing.T) {
	t.Parallel()
	f := newTransferOutFixture(t)
	ctx := context.Background()
	buy := buyOn(t, f, "2026-05-01", 3, 3000)
	var auditsBefore int
	require.NoError(t, f.database.QueryRow(`SELECT COUNT(*) FROM audit_events`).Scan(&auditsBefore))

	result, err := f.investmentService.ExternalTransferOut(ctx, transferOutOfLot(f, "2026-06-01", *buy.LotID, 2))
	require.NoError(t, err)
	assert.Equal(t, db.InternalTransferSelectedLots, result.Plan.BasisAllocation)
	require.Len(t, result.Plan.Links, 1)
	assert.Equal(t, "2026-05-01", result.Plan.Links[0].OriginalAcquiredOn)
	requireScaled(t, 2000, 2, exact.ScaledIntFromInt64(result.Plan.BasisValue, result.Plan.BasisScale), "basis carried out")

	require.Len(t, result.Transaction.JournalEntries, 1)
	postings := result.Transaction.JournalEntries[0].Postings
	require.Len(t, postings, 2, "the primary journal moves only the security")
	assert.Equal(t, f.holdingAccountID, postings[0].AccountID)
	assert.Equal(t, "-2", postings[0].QuantityValue.String())
	assert.Equal(t, "2", postings[1].QuantityValue.String())
	bridge := bridgePostings(t, f, result.Transaction.ID)
	require.Len(t, bridge, 2)
	requireScaled(t, -2000, 2, bridge["commodity_trading"], "T −b")
	requireScaled(t, 2000, 2, bridge["external_investment_transfer_equity"], "E +b")

	var auditsAfter, gains int
	require.NoError(t, f.database.QueryRow(`SELECT COUNT(*) FROM audit_events`).Scan(&auditsAfter))
	assert.Equal(t, auditsBefore+1, auditsAfter, "security journal and bridge share one audit event")
	require.NoError(t, f.database.QueryRow(`SELECT COUNT(*) FROM investment_disposal_decisions`).Scan(&gains))
	assert.Zero(t, gains, "an outbound transfer is not a disposal")
	requireScaled(t, 1000, 2, lotRemainingBasis(t, f, *buy.LotID), "remaining lot keeps the rest")
	var label string
	require.NoError(t, f.database.QueryRow(`SELECT link.role FROM investment_operation_journal_links link
		WHERE link.role = 'transfer_bridge'`).Scan(&label))
	assert.Equal(t, "transfer_bridge", label)
	requireInvestmentSelfCheckPasses(t, f)
}

func TestExternalTransferOutFromAverageCostPoolCarriesPoolRate(t *testing.T) {
	t.Parallel()
	f := newTransferOutFixture(t)
	ctx := context.Background()
	setHoldingCostBasisMethod(t, f, "average_cost")
	buyOn(t, f, "2026-02-01", 2, 2000)
	buyOn(t, f, "2026-02-02", 2, 4000)
	input := transferOutOfLot(f, "2026-03-01", 0, 0)
	input.Allocations, input.QuantityValue = nil, exact.New(1)

	preview, err := f.investmentService.PreviewExternalTransferOut(ctx, input)
	require.NoError(t, err)
	result, err := f.investmentService.ExternalTransferOut(ctx, input)
	require.NoError(t, err)
	assert.Equal(t, db.InternalTransferAverageCostPool, result.Plan.BasisAllocation)
	requireScaled(t, 1500, 2, exact.ScaledIntFromInt64(result.Plan.BasisValue, result.Plan.BasisScale), "pool rate 60.00 / 4")
	assert.Equal(t, preview.Plan.BasisValue, result.Plan.BasisValue, "preview carries what the commit carries")
	quantity, basis := openPositionBasis(t, f, f.holdingAccountID)
	requireScaled(t, 3, 0, quantity, "units left in the pool")
	requireScaled(t, 4500, 2, basis, "basis left in the pool")
	requireScaled(t, 1500, 2, bridgePostings(t, f, result.Transaction.ID)["external_investment_transfer_equity"], "E +b")
	requireInvestmentSelfCheckPasses(t, f)
}

func TestExternalTransferOutPreviewWritesNothing(t *testing.T) {
	t.Parallel()
	f := newTransferOutFixture(t)
	buy := buyOn(t, f, "2026-05-01", 3, 3000)
	transactionsBefore := f.transactionCount(t)
	preview, err := f.investmentService.PreviewExternalTransferOut(context.Background(), transferOutOfLot(f, "2026-06-01", *buy.LotID, 1))
	require.NoError(t, err)
	requireScaled(t, 1000, 2, exact.ScaledIntFromInt64(preview.Plan.BasisValue, preview.Plan.BasisScale), "previewed basis")
	assert.Equal(t, transactionsBefore, f.transactionCount(t))
	requireScaled(t, 3000, 2, lotRemainingBasis(t, f, *buy.LotID), "lot untouched")
}

func TestExternalTransferOutOfZeroBasisLotPostsNoBridge(t *testing.T) {
	t.Parallel()
	f := newTransferOutFixture(t)
	in, err := f.investmentService.ExternalTransferIn(context.Background(), ExternalTransferInInput{
		OwnerUserID: f.ownerUserID, EffectiveOn: "2026-05-01", HoldingAccountID: f.holdingAccountID,
		CommodityID: f.stockCommodityID, QuantityValue: exact.New(2), CarriedBasisValue: 0, CarriedBasisScale: 2,
		CostCommodityID: f.eurCommodityID, SourceEvidenceJSON: `{"gift":true}`,
	})
	require.NoError(t, err)
	result, err := f.investmentService.ExternalTransferOut(context.Background(), transferOutOfLot(f, "2026-06-01", *in.LotID, 2))
	require.NoError(t, err)
	assert.Zero(t, result.Plan.BasisValue)
	assert.Empty(t, bridgePostings(t, f, result.Transaction.ID), "a known zero basis bridges nothing")
	requireInvestmentSelfCheckPasses(t, f)
}

func TestExternalTransferOutBehindLaterDepletionIsRefused(t *testing.T) {
	t.Parallel()
	f := newTransferOutFixture(t)
	buy := buyOn(t, f, "2026-05-01", 3, 3000)
	_, err := f.investmentService.Sell(context.Background(), sellInput(f, "2026-07-01", 1))
	require.NoError(t, err)
	transactionsBefore := f.transactionCount(t)
	_, err = f.investmentService.ExternalTransferOut(context.Background(), transferOutOfLot(f, "2026-06-01", *buy.LotID, 1))
	require.ErrorIs(t, err, db.ErrOutOfOrderPositionEvent)
	assert.Equal(t, transactionsBefore, f.transactionCount(t))
}

// Until dated bridge adjustments ship, history that would change the basis an
// outbound transfer already carried out of the book is refused with the
// transfer named, and nothing is written.
func TestBuyCorrectionThatChangesOutboundBasisIsRefusedWithTransferNamed(t *testing.T) {
	t.Parallel()
	f := newTransferOutFixture(t)
	ctx := context.Background()
	buy := buyOn(t, f, "2026-05-01", 3, 3000)
	out, err := f.investmentService.ExternalTransferOut(ctx, transferOutOfLot(f, "2026-06-01", *buy.LotID, 2))
	require.NoError(t, err)
	transactionsBefore := f.transactionCount(t)

	_, err = acknowledgedReplaceBuy(ctx, f.investmentService, replaceBuyPrice(f, buy, "2026-05-01", 3, 3300))
	require.ErrorIs(t, err, ErrInvestmentBuyDependency)
	var dependency InvestmentBuyDependencyError
	require.ErrorAs(t, err, &dependency)
	assert.Equal(t, transferOperationID(t, f, out.Transaction.ID), dependency.OperationID, "the outbound transfer is named")
	assert.Equal(t, transactionsBefore, f.transactionCount(t))
	requireInvestmentSelfCheckPasses(t, f)
}

func TestExternalTransferOutIntoReconciledPeriodNeedsOverride(t *testing.T) {
	t.Parallel()
	f := newTransferOutFixture(t)
	ctx := context.Background()
	buy := buyOn(t, f, "2026-05-01", 3, 3000)
	checkpointID := reconcileHolding(t, f, "2026-05-31", 3)
	input := transferOutOfLot(f, "2026-05-20", *buy.LotID, 1)

	impact, err := f.investmentService.PreviewExternalTransferOutReconciliationImpact(ctx, input)
	require.NoError(t, err)
	require.Len(t, impact.AffectedCheckpoints, 1)
	assert.Equal(t, checkpointID, impact.AffectedCheckpoints[0].CheckpointID)
	_, err = f.investmentService.ExternalTransferOut(ctx, input)
	require.ErrorIs(t, err, ErrReconciliationOverrideRequired)

	input.ReconciliationOverride, input.ChangeReason = true, "broker statement shows the move"
	result, err := f.investmentService.ExternalTransferOut(ctx, input)
	require.NoError(t, err)
	assert.Contains(t, result.Transaction.InvalidatedCheckpointIDs, checkpointID)
	requireInvestmentSelfCheckPasses(t, f)
}

func TestSelfCheckDetectsOutboundBridgeThatDisagreesWithLinks(t *testing.T) {
	t.Parallel()
	f := newTransferOutFixture(t)
	buy := buyOn(t, f, "2026-05-01", 3, 3000)
	_, err := f.investmentService.ExternalTransferOut(context.Background(), transferOutOfLot(f, "2026-06-01", *buy.LotID, 2))
	require.NoError(t, err)
	// Damage the bridge by detaching it; the links still carry 20.00.
	_, err = f.database.Exec(`DROP TRIGGER investment_operation_journal_links_no_update`)
	require.NoError(t, err)
	_, err = f.database.Exec(`UPDATE investment_operation_journal_links SET role = 'detached' WHERE role = 'transfer_bridge'`)
	require.NoError(t, err)
	run, err := selfCheckOver(t, f.database).RunSelfCheck(context.Background(), "manual")
	require.NoError(t, err)
	result := resultFor(t, run, CheckInvestmentFoundation)
	assert.Equal(t, SelfCheckFailed, result.Status)
	assert.Contains(t, result.Summary, "outbound transfers bridge a different basis")
}
