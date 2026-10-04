package app

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rekenraam/backend/internal/db"
	"rekenraam/backend/internal/exact"
)

// T-118: write-off reversal and replacement.

func writeOffOn(t *testing.T, f *investmentsTestFixture, date string, quantity int64, method string) InvestmentTradeResult {
	t.Helper()
	input := backdatedWriteOff(f, date, quantity)
	input.CostBasisMethod = method
	result, err := f.investmentService.WriteOff(context.Background(), input)
	require.NoError(t, err)
	return result
}

func writeOffReplacement(f *investmentsTestFixture, date string, quantity int64, method string) InvestmentWriteOffInput {
	return InvestmentWriteOffInput{TransactionDate: date, CommodityID: f.stockCommodityID,
		HoldingAccountID: f.holdingAccountID, QuantityValue: exact.New(quantity),
		Reason: "fund closed", CostBasisMethod: method}
}

func acknowledgedReverseWriteOff(ctx context.Context, s *InvestmentService, input ReverseInvestmentSaleInput) (Transaction, error) {
	if impact, err := s.ReverseWriteOffReconciliationImpact(ctx, input); err == nil && impact.GainImpact != nil {
		input.GainImpactAcknowledgement = impact.GainImpact.Acknowledgement
	}
	return s.ReverseWriteOff(ctx, input)
}

func acknowledgedReplaceWriteOff(ctx context.Context, s *InvestmentService, input ReplaceInvestmentWriteOffInput) (ReplaceInvestmentSaleResult, error) {
	if impact, err := s.ReplaceWriteOffReconciliationImpact(ctx, input); err == nil && impact.GainImpact != nil {
		input.GainImpactAcknowledgement = impact.GainImpact.Acknowledgement
	}
	return s.ReplaceWriteOff(ctx, input)
}

func requireZeroProceeds(t *testing.T, f *investmentsTestFixture, transactionID int64) {
	t.Helper()
	var proceeds string
	require.NoError(t, f.database.QueryRow(`SELECT d.proceeds_value FROM investment_disposal_decisions d
		WHERE d.transaction_id = ?`, transactionID).Scan(&proceeds))
	assert.Equal(t, "0", proceeds)
	transaction, err := f.transactionService.Transaction(context.Background(), transactionID)
	require.NoError(t, err)
	for _, entry := range transaction.JournalEntries {
		for _, posting := range entry.Postings {
			assert.NotEqual(t, f.cashAccountID, posting.AccountID, "a write-off has no cash leg")
		}
	}
}

// A full write-off is reversed: the inverse restores every unit, the
// original decision stays as immutable evidence, and self-check passes.
func TestReverseFullWriteOffRestoresPosition(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := newInvestmentsTestFixture(t)
	buyOn(t, f, "2026-01-01", 10, 100000)
	written := writeOffOn(t, f, "2026-02-01", 10, "fifo")
	assert.Equal(t, "0", positionQuantity(t, f, f.holdingAccountID, f.stockCommodityID))

	inverse, err := acknowledgedReverseWriteOff(ctx, f.investmentService, ReverseInvestmentSaleInput{
		OwnerUserID: f.ownerUserID, TransactionID: written.Transaction.ID, Reason: "the fund reopened"})
	require.NoError(t, err)
	assert.Equal(t, "2026-02-01", inverse.TransactionDate)
	assert.Equal(t, "10", positionQuantity(t, f, f.holdingAccountID, f.stockCommodityID))
	var decisions int
	require.NoError(t, f.database.QueryRow(`SELECT count(*) FROM investment_disposal_decisions
		WHERE transaction_id = ?`, written.Transaction.ID).Scan(&decisions))
	assert.Equal(t, 1, decisions, "the original decision stays as evidence")
	gains, err := f.investmentService.ListRealizedGains(ctx, GainsReportParams{})
	require.NoError(t, err)
	assert.Empty(t, gains)
	var operation string
	require.NoError(t, f.database.QueryRow(`SELECT operation FROM audit_events a
		JOIN transactions t ON t.created_audit_event_id = a.id WHERE t.id = ?`, inverse.ID).Scan(&operation))
	assert.Equal(t, "investment.write_off.reverse", operation)
	requireSelfCheckPasses(t, f)
}

// A partial write-off is replaced by a smaller one at zero proceeds. A later
// FIFO sale then consumes different shares, and its gain change is disclosed.
func TestReplacePartialWriteOffReplaysDependentSale(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := newInvestmentsTestFixture(t)
	buyOn(t, f, "2026-01-01", 10, 100000)
	buyOn(t, f, "2026-02-01", 10, 300000)
	written := writeOffOn(t, f, "2026-03-01", 10, "fifo")
	sale := tradeOn(f, f.holdingAccountID, "2026-04-01", 5, 200000)
	sale.CostBasisMethod = "fifo"
	sold, err := f.investmentService.Sell(ctx, sale)
	require.NoError(t, err)

	input := ReplaceInvestmentWriteOffInput{OwnerUserID: f.ownerUserID, TransactionID: written.Transaction.ID,
		Reason: "only four shares were delisted", Replacement: writeOffReplacement(f, "2026-03-01", 4, "fifo")}
	impact, err := f.investmentService.ReplaceWriteOffReconciliationImpact(ctx, input)
	require.NoError(t, err)
	assert.Equal(t, db.GainImpactRevised, gainChangeFor(t, impact, sold.Transaction.ID).Kind)
	_, err = f.investmentService.ReplaceWriteOff(ctx, input)
	require.ErrorIs(t, err, ErrGainImpactAcknowledgementRequired)
	input.GainImpactAcknowledgement = impact.GainImpact.Acknowledgement
	result, err := f.investmentService.ReplaceWriteOff(ctx, input)
	require.NoError(t, err)

	requireZeroProceeds(t, f, result.Replacement.Transaction.ID)
	assert.Equal(t, "11", positionQuantity(t, f, f.holdingAccountID, f.stockCommodityID))
	// The four January shares are written off; the sale takes the other six
	// January shares' first five at 100 each instead of February's at 300.
	gains := gainsByDate(t, f)
	assert.Zero(t, exact.ScaledIntFromInt64(gains["2026-04-01"].DisposedBasisValue, gains["2026-04-01"].DisposedBasisScale).
		Cmp(exact.ScaledIntFromInt64(-50000, 2)))
	assert.Zero(t, exact.ScaledIntFromInt64(gains["2026-03-01"].RealizedGainValue, gains["2026-03-01"].RealizedGainScale).
		Cmp(exact.ScaledIntFromInt64(-40000, 2)))
	requireSelfCheckPasses(t, f)
}

// Reversing a write-off restores units a later sale did not need; reducing a
// later write-off below a dependent sale's requirement is named instead.
func TestReplaceWriteOffRefusesImpossibleDependentSale(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := newInvestmentsTestFixture(t)
	buyOn(t, f, "2026-01-01", 10, 100000)
	written := writeOffOn(t, f, "2026-02-01", 4, "fifo")
	sold, err := f.investmentService.Sell(ctx, tradeOn(f, f.holdingAccountID, "2026-03-01", 6, 60000))
	require.NoError(t, err)
	before := buyReplacementPreviewSnapshot(t, f.database)

	_, err = acknowledgedReplaceWriteOff(ctx, f.investmentService, ReplaceInvestmentWriteOffInput{
		OwnerUserID: f.ownerUserID, TransactionID: written.Transaction.ID, Reason: "eight were delisted",
		Replacement: writeOffReplacement(f, "2026-02-01", 8, "fifo")})
	var dependency InvestmentSaleDependencyError
	require.ErrorAs(t, err, &dependency)
	assert.Equal(t, saleOperationIDForTest(t, f, sold.Transaction.ID), dependency.OperationID)
	assert.Equal(t, before, buyReplacementPreviewSnapshot(t, f.database))
}

// A specific-lot write-off is replaced by one electing the other lot. The
// election names the lot the user chose; a later specific-lot sale of the
// originally written-off lot now replays against it.
func TestReplaceSpecificLotWriteOffKeepsLineage(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := newInvestmentsTestFixture(t)
	first := buyOn(t, f, "2026-01-01", 5, 50000)
	second := buyOn(t, f, "2026-02-01", 5, 150000)
	input := backdatedWriteOff(f, "2026-03-01", 5)
	input.CostBasisMethod = "specific_lot"
	input.LotAllocations = []InvestmentLotAllocationInput{{LotID: *second.LotID, QuantityValue: exact.New(5)}}
	written, err := f.investmentService.WriteOff(ctx, input)
	require.NoError(t, err)

	replacement := writeOffReplacement(f, "2026-03-01", 5, "specific_lot")
	replacement.LotAllocations = []InvestmentLotAllocationInput{{LotID: *first.LotID, QuantityValue: exact.New(5)}}
	result, err := acknowledgedReplaceWriteOff(ctx, f.investmentService, ReplaceInvestmentWriteOffInput{
		OwnerUserID: f.ownerUserID, TransactionID: written.Transaction.ID, Reason: "the other fund closed",
		Replacement: replacement})
	require.NoError(t, err)
	require.Len(t, result.Replacement.Allocations, 1)
	assert.Equal(t, *first.LotID, result.Replacement.Allocations[0].LotID)

	sale := tradeOn(f, f.holdingAccountID, "2026-04-01", 5, 100000)
	sale.CostBasisMethod = "specific_lot"
	sale.LotAllocations = []InvestmentLotAllocationInput{{LotID: *second.LotID, QuantityValue: exact.New(5)}}
	_, err = f.investmentService.Sell(ctx, sale)
	require.NoError(t, err)
	assert.Equal(t, "0", positionQuantity(t, f, f.holdingAccountID, f.stockCommodityID))
	requireSelfCheckPasses(t, f)
}

// An already corrected write-off cannot be corrected again, and the sale and
// write-off commands stay in their own families.
func TestWriteOffCorrectionsStayInTheirFamilyAndRefuseAlreadyCorrected(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := newInvestmentsTestFixture(t)
	buyOn(t, f, "2026-01-01", 10, 100000)
	written := writeOffOn(t, f, "2026-02-01", 2, "fifo")
	sold, err := f.investmentService.Sell(ctx, tradeOn(f, f.holdingAccountID, "2026-03-01", 2, 30000))
	require.NoError(t, err)

	_, err = f.investmentService.ReverseSale(ctx, ReverseInvestmentSaleInput{OwnerUserID: f.ownerUserID,
		TransactionID: written.Transaction.ID, Reason: "wrong command"})
	require.ErrorIs(t, err, ErrInvestmentSaleNotFound)
	_, err = f.investmentService.ReverseWriteOff(ctx, ReverseInvestmentSaleInput{OwnerUserID: f.ownerUserID,
		TransactionID: sold.Transaction.ID, Reason: "wrong command"})
	require.ErrorIs(t, err, ErrInvestmentWriteOffNotFound)
	_, err = f.investmentService.ReplaceSale(ctx, ReplaceInvestmentSaleInput{OwnerUserID: f.ownerUserID,
		TransactionID: written.Transaction.ID, Reason: "wrong command", Replacement: tradeOn(f, f.holdingAccountID, "2026-02-01", 2, 1)})
	require.ErrorIs(t, err, ErrInvestmentSaleNotFound)

	_, err = acknowledgedReverseWriteOff(ctx, f.investmentService, ReverseInvestmentSaleInput{
		OwnerUserID: f.ownerUserID, TransactionID: written.Transaction.ID, Reason: "not delisted"})
	require.NoError(t, err)
	_, err = f.investmentService.ReverseWriteOff(ctx, ReverseInvestmentSaleInput{OwnerUserID: f.ownerUserID,
		TransactionID: written.Transaction.ID, Reason: "again"})
	require.ErrorIs(t, err, ErrInvestmentWriteOffAlreadyCorrected)
	_, err = f.investmentService.ReplaceWriteOff(ctx, ReplaceInvestmentWriteOffInput{OwnerUserID: f.ownerUserID,
		TransactionID: written.Transaction.ID, Reason: "again", Replacement: writeOffReplacement(f, "2026-02-01", 1, "fifo")})
	require.ErrorIs(t, err, ErrInvestmentWriteOffAlreadyCorrected)
}

// A preview's acknowledgement is bound to the history it saw. A sale entered
// after the preview changes the gain set, so the stale token is refused and
// nothing is written.
func TestReplaceWriteOffRefusesStalePreview(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := newInvestmentsTestFixture(t)
	buyOn(t, f, "2026-01-01", 10, 100000)
	buyOn(t, f, "2026-02-01", 10, 300000)
	written := writeOffOn(t, f, "2026-03-01", 10, "fifo")
	input := ReplaceInvestmentWriteOffInput{OwnerUserID: f.ownerUserID, TransactionID: written.Transaction.ID,
		Reason: "only four", Replacement: writeOffReplacement(f, "2026-03-01", 4, "fifo")}
	impact, err := f.investmentService.ReplaceWriteOffReconciliationImpact(ctx, input)
	require.NoError(t, err)
	input.GainImpactAcknowledgement = impact.GainImpact.Acknowledgement

	sale := tradeOn(f, f.holdingAccountID, "2026-04-01", 5, 200000)
	sale.CostBasisMethod = "fifo"
	_, err = f.investmentService.Sell(ctx, sale)
	require.NoError(t, err)
	before := buyReplacementPreviewSnapshot(t, f.database)
	_, err = f.investmentService.ReplaceWriteOff(ctx, input)
	require.ErrorIs(t, err, ErrGainImpactAcknowledgementStale)
	assert.Equal(t, before, buyReplacementPreviewSnapshot(t, f.database))
}

// A failure after the replacement and dependent replay are written rolls back
// the inverse, the corrected write-off, the replay revisions and the audit.
func TestReplaceWriteOffRollsBackLateFailure(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := newInvestmentsTestFixture(t)
	buyOn(t, f, "2026-01-01", 10, 100000)
	written := writeOffOn(t, f, "2026-02-01", 4, "fifo")
	_, err := f.investmentService.Sell(ctx, tradeOn(f, f.holdingAccountID, "2026-03-01", 3, 30000))
	require.NoError(t, err)
	before := buyReplacementPreviewSnapshot(t, f.database)
	injected := errors.New("injected after the corrected write-off")

	input := ReplaceInvestmentWriteOffInput{OwnerUserID: f.ownerUserID, TransactionID: written.Transaction.ID,
		Reason: "two shares", Replacement: writeOffReplacement(f, "2026-02-01", 2, "fifo")}
	if impact, err := f.investmentService.ReplaceWriteOffReconciliationImpact(ctx, input); err == nil && impact.GainImpact != nil {
		input.GainImpactAcknowledgement = impact.GainImpact.Acknowledgement
	}
	sale, err := writeOffReplacementAsSale(input)
	require.NoError(t, err)
	_, err = f.investmentService.replaceDisposal(ctx, sale, "browser_api", "investment.write_off.replace",
		func(tx *sql.Tx, _, _ int64) error {
			var decisions int
			require.NoError(t, tx.QueryRow(`SELECT count(*) FROM investment_disposal_decisions`).Scan(&decisions))
			require.Equal(t, 3, decisions, "the corrected write-off exists when the hook runs")
			return injected
		}, writeOffCorrectionFamily)
	require.ErrorIs(t, err, injected)
	assert.Equal(t, before, buyReplacementPreviewSnapshot(t, f.database))
	assert.Equal(t, "3", positionQuantity(t, f, f.holdingAccountID, f.stockCommodityID))
	requireSelfCheckPasses(t, f)
}

// A reconciled write-off date needs the explicit override; the preview names
// the checkpoint. The write-off has no cash leg, so the checkpoint is on the
// holding's security balance.
func TestReverseWriteOffRequiresReconciliationOverride(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := newInvestmentsTestFixture(t)
	buyOn(t, f, "2026-01-01", 10, 100000)
	written := writeOffOn(t, f, "2026-02-01", 10, "fifo")
	checkpointID := reconcileHolding(t, f, "2026-03-01", 0)
	input := ReverseInvestmentSaleInput{OwnerUserID: f.ownerUserID, TransactionID: written.Transaction.ID,
		Reason: "the fund reopened"}
	impact, err := f.investmentService.ReverseWriteOffReconciliationImpact(ctx, input)
	require.NoError(t, err)
	require.Len(t, impact.AffectedCheckpoints, 1)
	assert.Equal(t, checkpointID, impact.AffectedCheckpoints[0].CheckpointID)
	_, err = acknowledgedReverseWriteOff(ctx, f.investmentService, input)
	require.ErrorIs(t, err, ErrReconciliationOverrideRequired)
	input.ReconciliationOverride = true
	_, err = acknowledgedReverseWriteOff(ctx, f.investmentService, input)
	require.NoError(t, err)
	requireSelfCheckPasses(t, f)
}

// A write-off recorded against the wrong holding moves to the right one. It
// has no cash leg, so its cost currency is resolved from the new holding's
// lots on the corrected date; the source position gets its units back.
func TestReplaceWriteOffMovesHolding(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := newInvestmentsTestFixture(t)
	other := seedTestAccountWithClass(t, f.database, "active", true, "asset", "security_holding")
	buyOn(t, f, "2026-01-01", 10, 100000)
	_, err := f.investmentService.Buy(ctx, tradeOn(f, other, "2026-01-01", 4, 80000))
	require.NoError(t, err)
	written := writeOffOn(t, f, "2026-02-01", 4, "fifo")

	replacement := writeOffReplacement(f, "2026-02-15", 4, "fifo")
	replacement.HoldingAccountID = other
	result, err := acknowledgedReplaceWriteOff(ctx, f.investmentService, ReplaceInvestmentWriteOffInput{
		OwnerUserID: f.ownerUserID, TransactionID: written.Transaction.ID, Reason: "the other broker's fund closed",
		Replacement: replacement})
	require.NoError(t, err)
	requireZeroProceeds(t, f, result.Replacement.Transaction.ID)
	assert.Equal(t, "10", positionQuantity(t, f, f.holdingAccountID, f.stockCommodityID))
	assert.Equal(t, "0", positionQuantity(t, f, other, f.stockCommodityID))
	var cost int64
	require.NoError(t, f.database.QueryRow(`SELECT cost_commodity_id FROM investment_disposal_decisions
		WHERE transaction_id = ?`, result.Replacement.Transaction.ID).Scan(&cost))
	assert.Equal(t, f.eurCommodityID, cost)
	requireSelfCheckPasses(t, f)
}
