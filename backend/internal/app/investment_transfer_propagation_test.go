package app

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rekenraam/backend/internal/db"
	"rekenraam/backend/internal/exact"
)

// T-132: corrected history that changes an internal transfer's carried basis
// replays the destination and everything downstream of it in the same
// command, instead of refusing. Units, destination lots and original dates
// stay fixed; a replay that would change them is still refused by name.

// effectiveTransferBasis is a transfer's current carried basis for one link:
// its latest revision, else the immutable link, plus how many revisions exist.
func effectiveTransferBasis(t *testing.T, f *investmentsTestFixture, transactionID int64, linkSeq int) (*exact.ScaledInt, int) {
	t.Helper()
	var value exact.Coefficient
	var scale, revisions int
	require.NoError(t, f.database.QueryRow(`
		SELECT COALESCE(r.carried_basis_value, x.carried_basis_value), COALESCE(r.carried_basis_scale, x.carried_basis_scale),
			(SELECT count(*) FROM investment_transfer_link_revisions all_r
				WHERE all_r.operation_id = x.operation_id AND all_r.link_seq = x.link_seq)
		FROM investment_transfer_lot_links x
		LEFT JOIN latest_investment_transfer_link_revisions r ON r.operation_id = x.operation_id AND r.link_seq = x.link_seq
		WHERE x.operation_id = ? AND x.link_seq = ?`, transferOperationID(t, f, transactionID), linkSeq).Scan(&value, &scale, &revisions))
	return exact.ScaledIntFromCoefficient(value, scale), revisions
}

func lotRemainingBasis(t *testing.T, f *investmentsTestFixture, lotID int64) *exact.ScaledInt {
	t.Helper()
	var value exact.Coefficient
	var scale int
	require.NoError(t, f.database.QueryRow(`SELECT remaining_cost_basis_value, remaining_cost_basis_scale
		FROM current_investment_lots WHERE id = ?`, lotID).Scan(&value, &scale))
	return exact.ScaledIntFromCoefficient(value, scale)
}

func saleEffectiveBasis(t *testing.T, f *investmentsTestFixture, saleTransactionID int64) *exact.ScaledInt {
	t.Helper()
	var value exact.Coefficient
	var scale int
	require.NoError(t, f.database.QueryRow(`
		SELECT COALESCE(r.disposed_basis_value, d.disposed_basis_value), COALESCE(r.disposed_basis_scale, d.disposed_basis_scale)
		FROM investment_disposal_decisions d
		LEFT JOIN latest_investment_disposal_revisions r ON r.decision_id = d.id
		WHERE d.transaction_id = ?`, saleTransactionID).Scan(&value, &scale))
	return exact.ScaledIntFromCoefficient(value, scale)
}

func requireInvestmentSelfCheckPasses(t *testing.T, f *investmentsTestFixture) {
	t.Helper()
	run, err := selfCheckOver(t, f.database).RunSelfCheck(context.Background(), "manual")
	require.NoError(t, err)
	for _, check := range []string{CheckInvestmentFoundation, CheckLotReconciliation} {
		result := resultFor(t, run, check)
		assert.Equalf(t, SelfCheckPassed, result.Status, "%s: %s %v", check, result.Summary, result.Sample)
	}
}

func replaceBuyPrice(f *investmentsTestFixture, buy InvestmentTradeResult, date string, quantity, cash int64) ReplaceInvestmentBuyInput {
	return ReplaceInvestmentBuyInput{OwnerUserID: f.ownerUserID, TransactionID: buy.Transaction.ID,
		Reason: "broker correction", Replacement: InvestmentTradeInput{TransactionDate: date,
			CommodityID: f.stockCommodityID, HoldingAccountID: f.holdingAccountID,
			CashAccountID: f.cashAccountID, CashCommodityID: f.eurCommodityID,
			QuantityValue: exact.New(quantity), CashAmountValue: cash, CashAmountScale: 2}}
}

func TestBuyCorrectionCarriesChangedBasisToLinkedInternalTransfer(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	destinationID := seedTestAccountWithClass(t, f.database, "active", true, "asset", "security_holding")
	buy := buyOn(t, f, "2026-05-01", 3, 3000)
	transfer := chainTransfer(t, f, f.holdingAccountID, destinationID, *buy.LotID, "2026-06-01")
	transactionsBefore := f.transactionCount(t)

	replaced, err := acknowledgedReplaceBuy(ctx, f.investmentService, replaceBuyPrice(f, buy, "2026-05-01", 3, 3300))
	require.NoError(t, err)

	basis, revisions := effectiveTransferBasis(t, f, transfer.Transaction.ID, 1)
	requireScaled(t, 1100, 2, basis, "carried basis follows the corrected price")
	assert.Equal(t, 1, revisions)
	requireScaled(t, 1100, 2, lotRemainingBasis(t, f, transfer.DestinationLotIDs[0]), "destination lot")
	requireScaled(t, 2200, 2, lotRemainingBasis(t, f, *replaced.Replacement.LotID), "replacement source lot keeps the rest")
	var depleted int64
	require.NoError(t, f.database.QueryRow(`SELECT source_lot_id FROM investment_transfer_link_revisions`).Scan(&depleted))
	assert.Equal(t, *replaced.Replacement.LotID, depleted, "the transfer now depletes the corrected acquisition")
	// Only the correction's reversal and replacement post: an in-book transfer
	// moves units, never basis, so its journal is untouched.
	assert.Equal(t, transactionsBefore+2, f.transactionCount(t))
	var originalBasis exact.Coefficient
	var originalScale int
	require.NoError(t, f.database.QueryRow(`SELECT carried_basis_value, carried_basis_scale
		FROM investment_transfer_lot_links`).Scan(&originalBasis, &originalScale))
	requireScaled(t, 1000, 2, exact.ScaledIntFromCoefficient(originalBasis, originalScale),
		"the first committed link is immutable evidence")
	requireInvestmentSelfCheckPasses(t, f)
}

// Source → destination → further transfer → specific-lot sale. The election
// still names the lot it chose, and the sale's basis follows the chain.
func TestChainedTransferBasisChangePropagatesToDownstreamSpecificLotSale(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	b := seedTestAccountWithClass(t, f.database, "active", true, "asset", "security_holding")
	c := seedTestAccountWithClass(t, f.database, "active", true, "asset", "security_holding")
	buy := buyOn(t, f, "2026-05-01", 3, 3000)
	toB := chainTransfer(t, f, f.holdingAccountID, b, *buy.LotID, "2026-06-01")
	toC := chainTransfer(t, f, b, c, toB.DestinationLotIDs[0], "2026-06-15")
	sale := sellInput(f, "2026-07-01", 1)
	sale.HoldingAccountID, sale.CostBasisMethod = c, "specific_lot"
	sale.LotAllocations = []InvestmentLotAllocationInput{{LotID: toC.DestinationLotIDs[0], QuantityValue: exact.New(1)}}
	sold, err := f.investmentService.Sell(ctx, sale)
	require.NoError(t, err)
	requireScaled(t, 1000, 2, saleEffectiveBasis(t, f, sold.Transaction.ID), "sale basis before correction")

	input := replaceBuyPrice(f, buy, "2026-05-01", 3, 3300)
	impact, err := f.investmentService.ReplaceBuyReconciliationImpact(ctx, input)
	require.NoError(t, err)
	require.NotNil(t, impact.GainImpact)
	require.Len(t, impact.GainImpact.Changes, 1, "the downstream sale's gain change is disclosed")
	assert.Equal(t, c, impact.GainImpact.Changes[0].Before.AccountID)
	requireScaled(t, 1100, 2, impact.GainImpact.Changes[0].After.DisposedBasis, "disclosed basis after correction")

	input.GainImpactAcknowledgement = impact.GainImpact.Acknowledgement
	_, err = f.investmentService.ReplaceBuy(ctx, input)
	require.NoError(t, err)
	for _, transfer := range []InternalTransferResult{toB, toC} {
		basis, revisions := effectiveTransferBasis(t, f, transfer.Transaction.ID, 1)
		requireScaled(t, 1100, 2, basis, "each hop carries the corrected basis")
		assert.Equal(t, 1, revisions)
	}
	requireScaled(t, 1100, 2, saleEffectiveBasis(t, f, sold.Transaction.ID), "downstream sale basis")
	var elected int64
	require.NoError(t, f.database.QueryRow(`SELECT a.lot_id FROM latest_investment_disposal_revisions r
		JOIN investment_disposal_revision_allocations a ON a.revision_id = r.id
		JOIN investment_disposal_decisions d ON d.id = r.decision_id WHERE d.transaction_id = ?`,
		sold.Transaction.ID).Scan(&elected))
	assert.Equal(t, toC.DestinationLotIDs[0], elected, "the specific-lot election is kept")

	// Every replayed position lies inside the T-124 dependency closure.
	closure, err := f.investmentService.repository.InvestmentReplayClosure(ctx, BookID,
		[]db.InvestmentReplayPosition{replayPosition(f, f.holdingAccountID, "2026-05-01")})
	require.NoError(t, err)
	inClosure := map[int64]bool{}
	for _, position := range closure {
		inClosure[position.AccountID] = true
	}
	rows, err := f.database.Query(`SELECT DISTINCT d.account_id FROM investment_transfer_link_revisions r
		JOIN investment_transfer_lot_links x ON x.operation_id = r.operation_id AND x.link_seq = r.link_seq
		JOIN investment_lots d ON d.id = x.destination_lot_id`)
	require.NoError(t, err)
	defer rows.Close()
	for rows.Next() {
		var account int64
		require.NoError(t, rows.Scan(&account))
		assert.Truef(t, inClosure[account], "replayed destination %d is outside the closure", account)
	}
	require.NoError(t, rows.Err())
	requireInvestmentSelfCheckPasses(t, f)
}

// A→B, later B→A, then a sale in A. The return leg's new basis cannot reach
// back before it, so propagation settles.
func TestTransferCycleBasisPropagationSettles(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	b := seedTestAccountWithClass(t, f.database, "active", true, "asset", "security_holding")
	buy := buyOn(t, f, "2026-05-01", 3, 3000)
	out := chainTransfer(t, f, f.holdingAccountID, b, *buy.LotID, "2026-06-01")
	back := chainTransfer(t, f, b, f.holdingAccountID, out.DestinationLotIDs[0], "2026-06-15")
	sold, err := f.investmentService.Sell(ctx, sellInput(f, "2026-07-01", 3))
	require.NoError(t, err)

	_, err = acknowledgedReplaceBuy(ctx, f.investmentService, replaceBuyPrice(f, buy, "2026-05-01", 3, 6000))
	require.NoError(t, err)
	for _, transfer := range []InternalTransferResult{out, back} {
		basis, revisions := effectiveTransferBasis(t, f, transfer.Transaction.ID, 1)
		requireScaled(t, 2000, 2, basis, "both legs carry the corrected basis")
		assert.Equal(t, 1, revisions, "each leg is revised once")
	}
	requireScaled(t, 6000, 2, saleEffectiveBasis(t, f, sold.Transaction.ID), "the sale sees every corrected unit")
	requireInvestmentSelfCheckPasses(t, f)
}

func TestPooledTransferBasisPropagatesWhenLineageIsUnchanged(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	destinationID := seedTestAccountWithClass(t, f.database, "active", true, "asset", "security_holding")
	setHoldingCostBasisMethod(t, f, "average_cost")
	buyOn(t, f, "2026-02-01", 2, 2000)
	second := buyOn(t, f, "2026-02-02", 2, 4000)
	transfer, err := f.investmentService.InternalTransfer(ctx,
		pooledTransferInput(f, destinationID, "2026-03-01", exact.New(1), 0))
	require.NoError(t, err)
	before, _ := effectiveTransferBasis(t, f, transfer.Transaction.ID, 1)
	requireScaled(t, 1500, 2, before, "pool rate at transfer")

	// The second lot's price changes, not its date: the pool depletes the same
	// FIFO lineage at a new rate.
	_, err = acknowledgedReplaceBuy(ctx, f.investmentService, replaceBuyPrice(f, second, "2026-02-02", 2, 8000))
	require.NoError(t, err)
	after, revisions := effectiveTransferBasis(t, f, transfer.Transaction.ID, 1)
	requireScaled(t, 2500, 2, after, "carried basis at the corrected pool rate")
	assert.Equal(t, 1, revisions)
	requireScaled(t, 2500, 2, lotRemainingBasis(t, f, transfer.DestinationLotIDs[0]), "destination lot")
	requireInvestmentSelfCheckPasses(t, f)
}

func TestSameDayTransferAndDependentSaleReplayTogether(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	b := seedTestAccountWithClass(t, f.database, "active", true, "asset", "security_holding")
	buy := buyOn(t, f, "2026-05-01", 3, 3000)
	chainTransfer(t, f, f.holdingAccountID, b, *buy.LotID, "2026-06-01")
	sale := sellInput(f, "2026-06-01", 1)
	sale.HoldingAccountID = b
	sold, err := f.investmentService.Sell(ctx, sale)
	require.NoError(t, err)

	_, err = acknowledgedReplaceBuy(ctx, f.investmentService, replaceBuyPrice(f, buy, "2026-05-01", 3, 6000))
	require.NoError(t, err)
	requireScaled(t, 2000, 2, saleEffectiveBasis(t, f, sold.Transaction.ID), "the same-day sale follows the transfer slot")
	requireInvestmentSelfCheckPasses(t, f)
}

// The writer refuses a stale acknowledgement after domain effects, including
// every propagated revision, have been written; nothing may survive.
func TestTransferPropagationRollsBackOnLateRefusal(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	b := seedTestAccountWithClass(t, f.database, "active", true, "asset", "security_holding")
	buy := buyOn(t, f, "2026-05-01", 3, 3000)
	transfer := chainTransfer(t, f, f.holdingAccountID, b, *buy.LotID, "2026-06-01")
	sale := sellInput(f, "2026-07-01", 1)
	sale.HoldingAccountID = b
	_, err := f.investmentService.Sell(ctx, sale)
	require.NoError(t, err)

	input := replaceBuyPrice(f, buy, "2026-05-01", 3, 6000)
	input.GainImpactAcknowledgement = "0000000000000000000000000000000000000000000000000000000000000000"
	before := buyReplacementPreviewSnapshot(t, f.database)
	_, err = f.investmentService.ReplaceBuy(ctx, input)
	require.Truef(t, errors.Is(err, ErrGainImpactAcknowledgementStale) || errors.Is(err, ErrGainImpactAcknowledgementRequired),
		"want a gain acknowledgement refusal, got %v", err)
	assert.Equal(t, before, buyReplacementPreviewSnapshot(t, f.database))
	_, revisions := effectiveTransferBasis(t, f, transfer.Transaction.ID, 1)
	assert.Zero(t, revisions)
}

// Units, lots and dates stay fixed. Removing the transferred acquisition is
// still refused with the transfer named. (Buy replacement cannot move a date
// at all yet, T-116; replay itself also refuses a successor lot opened on
// another date, because the link's original date orders the destination.)
func TestRemovingTransferredAcquisitionStaysNamedRefusal(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	b := seedTestAccountWithClass(t, f.database, "active", true, "asset", "security_holding")
	buy := buyOn(t, f, "2026-05-01", 3, 3000)
	transfer := chainTransfer(t, f, f.holdingAccountID, b, *buy.LotID, "2026-06-01")
	before := buyReplacementPreviewSnapshot(t, f.database)
	_, err := acknowledgedReverseBuy(context.Background(), f.investmentService, ReverseInvestmentBuyInput{
		OwnerUserID: f.ownerUserID, TransactionID: buy.Transaction.ID, Reason: "duplicate"})
	require.ErrorIs(t, err, ErrInvestmentBuyDependency)
	var dependency InvestmentBuyDependencyError
	require.ErrorAs(t, err, &dependency)
	assert.Equal(t, transferOperationID(t, f, transfer.Transaction.ID), dependency.OperationID)
	assert.Equal(t, before, buyReplacementPreviewSnapshot(t, f.database))
}

func transferOperationID(t *testing.T, f *investmentsTestFixture, transactionID int64) int64 {
	t.Helper()
	var id int64
	require.NoError(t, f.database.QueryRow(`SELECT link.operation_id FROM investment_operation_journal_links link
		JOIN transaction_versions version ON version.id = link.transaction_version_id
		WHERE version.transaction_id = ? AND link.role = 'primary'`, transactionID).Scan(&id))
	return id
}

// A revision changes what both lots reconcile against, so damage to it must
// surface in self-check rather than pass as a sound projection.
func TestSelfCheckDetectsDamagedTransferLinkRevision(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	destinationID := seedTestAccountWithClass(t, f.database, "active", true, "asset", "security_holding")
	buy := buyOn(t, f, "2026-05-01", 3, 3000)
	chainTransfer(t, f, f.holdingAccountID, destinationID, *buy.LotID, "2026-06-01")
	_, err := acknowledgedReplaceBuy(context.Background(), f.investmentService, replaceBuyPrice(f, buy, "2026-05-01", 3, 3300))
	require.NoError(t, err)
	requireInvestmentSelfCheckPasses(t, f)

	_, err = f.database.Exec(`DROP TRIGGER investment_transfer_link_revisions_no_update`)
	require.NoError(t, err)
	_, err = f.database.Exec(`UPDATE investment_transfer_link_revisions SET carried_basis_value = '1200', carried_basis_scale = 2`)
	require.NoError(t, err)
	run, err := selfCheckOver(t, f.database).RunSelfCheck(context.Background(), "manual")
	require.NoError(t, err)
	assert.Equal(t, SelfCheckFailed, resultFor(t, run, CheckLotReconciliation).Status)
}

func TestTransferLinkRevisionsStayInsideTheirLinkAndChain(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	destinationID := seedTestAccountWithClass(t, f.database, "active", true, "asset", "security_holding")
	buy := buyOn(t, f, "2026-05-01", 3, 3000)
	transfer := chainTransfer(t, f, f.holdingAccountID, destinationID, *buy.LotID, "2026-06-01")
	_, err := acknowledgedReplaceBuy(context.Background(), f.investmentService, replaceBuyPrice(f, buy, "2026-05-01", 3, 3300))
	require.NoError(t, err)
	var revisionID, causedBy, audit, sourceLot int64
	require.NoError(t, f.database.QueryRow(`SELECT id, caused_by_operation_id, created_audit_event_id, source_lot_id
		FROM investment_transfer_link_revisions`).Scan(&revisionID, &causedBy, &audit, &sourceLot))
	operationID := transferOperationID(t, f, transfer.Transaction.ID)
	insert := func(seq int, supersedes any, lot int64) error {
		_, err := f.database.Exec(`INSERT INTO investment_transfer_link_revisions (book_id, operation_id, link_seq,
			revision_seq, caused_by_operation_id, supersedes_revision_id, source_lot_id, carried_basis_value,
			carried_basis_scale, created_at, created_audit_event_id)
			VALUES (1, ?, 1, ?, ?, ?, ?, '1300', 2, '2026-10-03T00:00:00Z', ?)`,
			operationID, seq, causedBy, supersedes, lot, audit)
		return err
	}
	assert.ErrorContains(t, insert(4, revisionID, sourceLot), "outside its link or chain", "skipped revision number")
	assert.ErrorContains(t, insert(3, revisionID, transfer.DestinationLotIDs[0]), "outside its link or chain",
		"source lot in another account")
	require.NoError(t, insert(3, revisionID, sourceLot))
	_, err = f.database.Exec(`DELETE FROM investment_transfer_link_revisions`)
	assert.ErrorContains(t, err, "immutable")
}
