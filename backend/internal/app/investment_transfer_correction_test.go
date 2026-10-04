package app

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rekenraam/backend/internal/db"
	"rekenraam/backend/internal/exact"
)

// T-119: transfer correction. A reversal removes an internal or external-in
// transfer from effective history and replays every position it moved, plus
// whatever propagation reaches downstream, under one audit event.

func acknowledgedReverseTransfer(ctx context.Context, s *InvestmentService, input ReverseInvestmentTransferInput) (Transaction, error) {
	if input.GainImpactAcknowledgement == "" {
		if impact, err := s.ReverseTransferReconciliationImpact(ctx, input); err == nil && impact.GainImpact != nil {
			input.GainImpactAcknowledgement = impact.GainImpact.Acknowledgement
		}
	}
	return s.ReverseTransfer(ctx, input)
}

func reverseTransferInput(f *investmentsTestFixture, transactionID int64) ReverseInvestmentTransferInput {
	return ReverseInvestmentTransferInput{OwnerUserID: f.ownerUserID, TransactionID: transactionID,
		Reason: "moved to the wrong account"}
}

func TestReverseInternalTransferRestoresSourceAndRemovesDestinationLot(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	destinationID := seedTestAccountWithClass(t, f.database, "active", true, "asset", "security_holding")
	buy := buyOn(t, f, "2026-05-01", 3, 3000)
	transfer := chainTransfer(t, f, f.holdingAccountID, destinationID, *buy.LotID, "2026-06-01")
	transactionsBefore := f.transactionCount(t)

	inverse, err := acknowledgedReverseTransfer(ctx, f.investmentService, reverseTransferInput(f, transfer.Transaction.ID))
	require.NoError(t, err)

	assert.Equal(t, "2026-06-01", inverse.TransactionDate, "the inverse keeps the transfer date")
	assert.Equal(t, transactionsBefore+1, f.transactionCount(t))
	assert.Equal(t, "3", positionQuantity(t, f, f.holdingAccountID, f.stockCommodityID))
	assert.Equal(t, "0", positionQuantity(t, f, destinationID, f.stockCommodityID))
	requireScaled(t, 3000, 2, lotRemainingBasis(t, f, *buy.LotID), "the source lot gets its basis back")
	var status string
	require.NoError(t, f.database.QueryRow(`SELECT status FROM current_investment_lots WHERE id = ?`,
		transfer.DestinationLotIDs[0]).Scan(&status))
	assert.Equal(t, "closed", status, "the destination lot is retired, not deleted")
	var links, facts int
	require.NoError(t, f.database.QueryRow(`SELECT count(*) FROM investment_transfer_lot_links`).Scan(&links))
	require.NoError(t, f.database.QueryRow(`SELECT count(*) FROM investment_transfer_facts`).Scan(&facts))
	assert.Equal(t, 1, links, "the link stays as evidence")
	assert.Equal(t, 1, facts)
	assert.Empty(t, positionMethodFamily(t, f, destinationID))
	requireInvestmentSelfCheckPasses(t, f)

	_, err = f.investmentService.ReverseTransfer(ctx, reverseTransferInput(f, transfer.Transaction.ID))
	require.ErrorIs(t, err, ErrInvestmentTransferAlreadyCorrected)
}

// The destination sold the transferred unit; reversing the transfer would
// leave that sale without units. The sale is named and nothing is written.
func TestReverseInternalTransferRefusedWhenDestinationSoldWithoutWriting(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	destinationID := seedTestAccountWithClass(t, f.database, "active", true, "asset", "security_holding")
	buy := buyOn(t, f, "2026-05-01", 3, 3000)
	transfer := chainTransfer(t, f, f.holdingAccountID, destinationID, *buy.LotID, "2026-06-01")
	sale := sellInput(f, "2026-07-01", 1)
	sale.HoldingAccountID = destinationID
	sold, err := f.investmentService.Sell(ctx, sale)
	require.NoError(t, err)
	before := buyReplacementPreviewSnapshot(t, f.database)

	_, err = f.investmentService.ReverseTransferReconciliationImpact(ctx, reverseTransferInput(f, transfer.Transaction.ID))
	var dependency InvestmentTransferDependencyError
	require.ErrorAs(t, err, &dependency)
	assert.Equal(t, transferOperationID(t, f, sold.Transaction.ID), dependency.OperationID)
	_, err = f.investmentService.ReverseTransfer(ctx, reverseTransferInput(f, transfer.Transaction.ID))
	require.ErrorIs(t, err, ErrInvestmentTransferDependency)
	assert.Equal(t, before, buyReplacementPreviewSnapshot(t, f.database))
}

// The source sold under FIFO after moving its oldest lot away. Reversing the
// transfer returns that lot, so the sale now takes it: a restated gain the
// user must acknowledge from the preview.
func TestReverseInternalTransferRestatesSourceFIFOSaleWithAcknowledgement(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	destinationID := seedTestAccountWithClass(t, f.database, "active", true, "asset", "security_holding")
	cheap := buyOn(t, f, "2026-05-01", 1, 1000)
	buyOn(t, f, "2026-05-02", 1, 5000)
	transfer := chainTransfer(t, f, f.holdingAccountID, destinationID, *cheap.LotID, "2026-06-01")
	sold, err := f.investmentService.Sell(ctx, sellInput(f, "2026-07-01", 1))
	require.NoError(t, err)
	requireScaled(t, 5000, 2, saleEffectiveBasis(t, f, sold.Transaction.ID), "FIFO took the only lot left")

	input := reverseTransferInput(f, transfer.Transaction.ID)
	_, err = f.investmentService.ReverseTransfer(ctx, input)
	require.ErrorIs(t, err, db.ErrGainImpactAcknowledgementRequired)
	impact, err := f.investmentService.ReverseTransferReconciliationImpact(ctx, input)
	require.NoError(t, err)
	require.NotNil(t, impact.GainImpact)
	input.GainImpactAcknowledgement = impact.GainImpact.Acknowledgement
	_, err = f.investmentService.ReverseTransfer(ctx, input)
	require.NoError(t, err)

	requireScaled(t, 1000, 2, saleEffectiveBasis(t, f, sold.Transaction.ID), "FIFO now takes the returned older lot")
	assert.Equal(t, "1", positionQuantity(t, f, f.holdingAccountID, f.stockCommodityID))
	requireInvestmentSelfCheckPasses(t, f)
}

// A → B → C with source lots. Reversing A → B would remove the lot B moved on,
// so the onward transfer is named. Unwinding from the end works.
func TestReverseTransferChainRefusesUpstreamUntilOnwardTransferReversed(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	b := seedTestAccountWithClass(t, f.database, "active", true, "asset", "security_holding")
	c := seedTestAccountWithClass(t, f.database, "active", true, "asset", "security_holding")
	buy := buyOn(t, f, "2026-05-01", 3, 3000)
	toB := chainTransfer(t, f, f.holdingAccountID, b, *buy.LotID, "2026-06-01")
	toC := chainTransfer(t, f, b, c, toB.DestinationLotIDs[0], "2026-06-15")

	_, err := f.investmentService.ReverseTransfer(ctx, reverseTransferInput(f, toB.Transaction.ID))
	var dependency InvestmentTransferDependencyError
	require.ErrorAs(t, err, &dependency)
	assert.Equal(t, transferOperationID(t, f, toC.Transaction.ID), dependency.OperationID)

	_, err = acknowledgedReverseTransfer(ctx, f.investmentService, reverseTransferInput(f, toC.Transaction.ID))
	require.NoError(t, err)
	_, err = acknowledgedReverseTransfer(ctx, f.investmentService, reverseTransferInput(f, toB.Transaction.ID))
	require.NoError(t, err)
	assert.Equal(t, "3", positionQuantity(t, f, f.holdingAccountID, f.stockCommodityID))
	assert.Equal(t, "0", positionQuantity(t, f, b, f.stockCommodityID))
	assert.Equal(t, "0", positionQuantity(t, f, c, f.stockCommodityID))
	requireInvestmentSelfCheckPasses(t, f)
}

// A source replacement revised the transfer's carried basis before the
// transfer was reversed. The revision stays as evidence but no longer
// describes current state, so self-check must not count it.
func TestReverseRevisedInternalTransferKeepsRevisionAsEvidence(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	destinationID := seedTestAccountWithClass(t, f.database, "active", true, "asset", "security_holding")
	buy := buyOn(t, f, "2026-05-01", 3, 3000)
	transfer := chainTransfer(t, f, f.holdingAccountID, destinationID, *buy.LotID, "2026-06-01")
	replaced, err := acknowledgedReplaceBuy(ctx, f.investmentService, replaceBuyPrice(f, buy, "2026-05-01", 3, 3300))
	require.NoError(t, err)
	_, revisions := effectiveTransferBasis(t, f, transfer.Transaction.ID, 1)
	require.Equal(t, 1, revisions)

	_, err = acknowledgedReverseTransfer(ctx, f.investmentService, reverseTransferInput(f, transfer.Transaction.ID))
	require.NoError(t, err)

	assert.Equal(t, "3", positionQuantity(t, f, f.holdingAccountID, f.stockCommodityID))
	requireScaled(t, 3300, 2, lotRemainingBasis(t, f, *replaced.Replacement.LotID), "the corrected lot is whole again")
	_, revisions = effectiveTransferBasis(t, f, transfer.Transaction.ID, 1)
	assert.Equal(t, 1, revisions, "no revision is appended for a removed transfer")
	requireInvestmentSelfCheckPasses(t, f)
}

// A pooled_lot transfer out of an average-cost pool is reversed: the pool
// gets its units and exact basis back, the one destination lot is retired.
func TestReversePooledLotTransferRestoresPoolBasisExactly(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	setHoldingCostBasisMethod(t, f, "average_cost")
	buyOn(t, f, "2026-01-01", 1, 1000)
	buyOn(t, f, "2026-01-02", 2, 2001)
	destinationID := seedTestAccountWithClass(t, f.database, "active", true, "asset", "security_holding")
	quantityBefore, basisBefore := openPositionBasis(t, f, f.holdingAccountID)
	transfer, err := f.investmentService.InternalTransfer(ctx, pooledTransferInput(f, destinationID, "2026-06-01", exact.New(2), 0))
	require.NoError(t, err)
	require.Equal(t, db.InternalTransferPooledLot, transfer.Plan.DestinationLineage)

	_, err = acknowledgedReverseTransfer(ctx, f.investmentService, reverseTransferInput(f, transfer.Transaction.ID))
	require.NoError(t, err)

	quantityAfter, basisAfter := openPositionBasis(t, f, f.holdingAccountID)
	assert.Zero(t, quantityBefore.Cmp(quantityAfter))
	assert.Zero(t, basisBefore.Cmp(basisAfter), "the pool basis is conserved exactly")
	assert.Equal(t, "0", positionQuantity(t, f, destinationID, f.stockCommodityID))
	requireInvestmentSelfCheckPasses(t, f)
}

// An external transfer in is reversed with its equity bridge: the lot is
// retired and the book-boundary journal inverted.
func TestReverseExternalTransferInRetiresLotAndInvertsBridge(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	seedExternalTransferEquity(t, f.database)
	ctx := context.Background()
	in, err := f.investmentService.ExternalTransferIn(ctx, knownTransferInput(f))
	require.NoError(t, err)
	original, err := f.transactionService.Transaction(ctx, in.Transaction.ID)
	require.NoError(t, err)

	inverse, err := acknowledgedReverseTransfer(ctx, f.investmentService, reverseTransferInput(f, in.Transaction.ID))
	require.NoError(t, err)

	assert.Equal(t, "0", positionQuantity(t, f, f.holdingAccountID, f.stockCommodityID))
	require.Len(t, inverse.JournalEntries, len(original.JournalEntries))
	for entry := range original.JournalEntries {
		require.Len(t, inverse.JournalEntries[entry].Postings, len(original.JournalEntries[entry].Postings))
		for posting, want := range original.JournalEntries[entry].Postings {
			got := inverse.JournalEntries[entry].Postings[posting]
			assert.Equal(t, want.AccountID, got.AccountID)
			assert.Zero(t, exact.ScaledIntFromCoefficient(want.QuantityValue.Negated(), want.QuantityScale).Cmp(
				exact.ScaledIntFromCoefficient(got.QuantityValue, got.QuantityScale)), "every leg, bridge included, is inverted")
		}
	}
	requireInvestmentSelfCheckPasses(t, f)
}

// The reconciled source balance would change. The guard runs after both
// replays are written and must roll all of them back; the override admits it.
func TestReverseTransferLateReconciliationRefusalRollsBackEverything(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	destinationID := seedTestAccountWithClass(t, f.database, "active", true, "asset", "security_holding")
	buy := buyOn(t, f, "2026-05-01", 3, 3000)
	transfer := chainTransfer(t, f, f.holdingAccountID, destinationID, *buy.LotID, "2026-06-01")
	reconcileHolding(t, f, "2026-06-30", 2)
	input := reverseTransferInput(f, transfer.Transaction.ID)
	impact, err := f.investmentService.ReverseTransferReconciliationImpact(ctx, input)
	require.NoError(t, err)
	require.NotEmpty(t, impact.AffectedCheckpoints)
	before := buyReplacementPreviewSnapshot(t, f.database)

	_, err = f.investmentService.ReverseTransfer(ctx, input)
	require.ErrorIs(t, err, ErrReconciliationOverrideRequired)
	assert.Equal(t, before, buyReplacementPreviewSnapshot(t, f.database))

	input.ReconciliationOverride = true
	_, err = acknowledgedReverseTransfer(ctx, f.investmentService, input)
	require.NoError(t, err)
	assert.Equal(t, "3", positionQuantity(t, f, f.holdingAccountID, f.stockCommodityID))
	requireInvestmentSelfCheckPasses(t, f)
}
