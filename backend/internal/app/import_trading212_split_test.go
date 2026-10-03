package app

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// splitImportFixture holds an imported Trading 212 position with a manually
// recorded 2-for-1 split, and a staged STOCK_SPLIT fill for it.
type splitImportFixture struct {
	*investTestFixture
	connectionID int64
	holdingID    int64
	splitOpID    int64
	splitFill    trading212OrderFill
}

func newSplitImportFixture(t *testing.T) splitImportFixture {
	t.Helper()
	f := newInvestTestFixture(t)
	conn := f.createConnection(t, &f.cashAccountID)
	commitSourceFill(t, f, conn.ID, trading212OrderFill{
		FillType: "TRADE", FillID: "buy-1", OrderID: "order-1", Side: "BUY",
		Ticker: "AAPL_US_EQ", ISIN: "US0378331005", Quantity: "10", Price: "100",
		Currency: "EUR", FilledAt: "2026-05-01T10:00:00Z", NetValue: "-1000", NetValueCurrency: "EUR",
	})
	positions, err := f.investmentSvc.Positions(context.Background())
	require.NoError(t, err)
	require.Len(t, positions, 1)
	_, err = f.investmentSvc.Split(context.Background(), InvestmentSplitInput{
		OwnerUserID: f.ownerUserID, EffectiveOn: "2026-06-01", HoldingAccountID: positions[0].AccountID,
		CommodityID: positions[0].CommodityID, RatioNumerator: 2, RatioDenominator: 1,
	})
	require.NoError(t, err)
	var splitOpID int64
	require.NoError(t, f.database.QueryRow(`SELECT operation_id FROM investment_split_facts`).Scan(&splitOpID))
	return splitImportFixture{investTestFixture: f, connectionID: conn.ID, holdingID: positions[0].AccountID,
		splitOpID: splitOpID, splitFill: trading212OrderFill{
			FillType: "STOCK_SPLIT", FillID: "split-1", OrderID: "split-order", Side: "BUY",
			Ticker: "AAPL_US_EQ", ISIN: "US0378331005", Quantity: "10", Price: "0",
			Currency: "EUR", FilledAt: "2026-06-01T08:00:00Z", NetValue: "0", NetValueCurrency: "EUR",
		}}
}

func (f splitImportFixture) holding(t *testing.T) string {
	t.Helper()
	positions, err := f.investmentSvc.Positions(context.Background())
	require.NoError(t, err)
	require.Len(t, positions, 1)
	return positions[0].QuantityValue.String()
}

func TestTrading212SplitRowLinksToRecordedSplitWithoutDoublePosting(t *testing.T) {
	t.Parallel()
	f := newSplitImportFixture(t)
	ctx := context.Background()
	batchID, rowID := f.stageOrderFillRow(t, f.connectionID, f.splitFill)
	rowInput := Trading212SplitRowInput{OwnerUserID: f.ownerUserID, BatchID: batchID, RowID: rowID}

	candidates, err := f.importService.Trading212SplitCandidates(ctx, rowInput)
	require.NoError(t, err)
	require.Len(t, candidates, 1)
	assert.Equal(t, f.splitOpID, candidates[0].OperationID)
	assert.Equal(t, "2026-06-01", candidates[0].EffectiveOn)
	assert.False(t, candidates[0].Linked)

	var transactionsBefore, auditsBefore int
	require.NoError(t, f.database.QueryRow(`SELECT COUNT(*) FROM transactions`).Scan(&transactionsBefore))
	require.NoError(t, f.database.QueryRow(`SELECT COUNT(*) FROM audit_events`).Scan(&auditsBefore))
	require.NoError(t, f.importService.LinkTrading212Split(ctx, LinkTrading212SplitInput{
		Trading212SplitRowInput: rowInput, OperationID: f.splitOpID}))
	var transactionsAfter, auditsAfter int
	require.NoError(t, f.database.QueryRow(`SELECT COUNT(*) FROM transactions`).Scan(&transactionsAfter))
	require.NoError(t, f.database.QueryRow(`SELECT COUNT(*) FROM audit_events`).Scan(&auditsAfter))
	assert.Equal(t, transactionsBefore, transactionsAfter, "linking posts nothing")
	assert.Equal(t, auditsBefore+1, auditsAfter)

	row, err := f.importRepo.ImportStagedRowByID(ctx, rowID)
	require.NoError(t, err)
	assert.Equal(t, "committed", row.CommitStatus)
	require.Len(t, row.CommitEffects, 1)
	assert.Equal(t, f.splitOpID, row.CommitEffects[0].OperationID.Int64)

	// Committing the batch afterwards cannot post the split row again.
	result, err := f.importService.CommitImportBatch(ctx, CommitImportBatchInput{OwnerUserID: f.ownerUserID, BatchID: batchID})
	require.NoError(t, err)
	assert.Equal(t, 1, result.CommittedCount, "the linked row is reported as committed")
	var transactionsAfterCommit int
	require.NoError(t, f.database.QueryRow(`SELECT COUNT(*) FROM transactions`).Scan(&transactionsAfterCommit))
	assert.Equal(t, transactionsBefore, transactionsAfterCommit)
	assert.Equal(t, "20", f.holding(t))

	// A re-fetch of the same fill is a duplicate of the linked identity, and
	// can be linked neither again nor to the same split from another row.
	retryBatch, retryRow := f.stageOrderFillRow(t, f.connectionID, f.splitFill)
	retried, err := f.importRepo.ImportStagedRowByID(ctx, retryRow)
	require.NoError(t, err)
	assert.Equal(t, "duplicate", retried.DedupeStatus)
	err = f.importService.LinkTrading212Split(ctx, LinkTrading212SplitInput{
		Trading212SplitRowInput: Trading212SplitRowInput{OwnerUserID: f.ownerUserID, BatchID: retryBatch, RowID: retryRow},
		OperationID:             f.splitOpID})
	require.ErrorIs(t, err, ErrImportSplitLinkUnavailable)
	other := f.splitFill
	other.FillID = "split-2"
	otherBatch, otherRow := f.stageOrderFillRow(t, f.connectionID, other)
	err = f.importService.LinkTrading212Split(ctx, LinkTrading212SplitInput{
		Trading212SplitRowInput: Trading212SplitRowInput{OwnerUserID: f.ownerUserID, BatchID: otherBatch, RowID: otherRow},
		OperationID:             f.splitOpID})
	require.ErrorIs(t, err, ErrImportSplitLinkUnavailable)
	candidates, err = f.importService.Trading212SplitCandidates(ctx,
		Trading212SplitRowInput{OwnerUserID: f.ownerUserID, BatchID: otherBatch, RowID: otherRow})
	require.NoError(t, err)
	require.Len(t, candidates, 1)
	assert.True(t, candidates[0].Linked)
	assert.Equal(t, "20", f.holding(t))
	requireInvestSelfCheckPasses(t, f.investTestFixture)
}

func TestTrading212SplitLinkRefusesOtherSecurityAndNonSplitRows(t *testing.T) {
	t.Parallel()
	f := newSplitImportFixture(t)
	ctx := context.Background()
	foreign := f.splitFill
	foreign.FillID, foreign.Ticker, foreign.ISIN = "split-msft", "MSFT_US_EQ", "US5949181045"
	batchID, rowID := f.stageOrderFillRow(t, f.connectionID, foreign)
	input := Trading212SplitRowInput{OwnerUserID: f.ownerUserID, BatchID: batchID, RowID: rowID}
	candidates, err := f.importService.Trading212SplitCandidates(ctx, input)
	require.NoError(t, err)
	assert.Empty(t, candidates, "an unknown security has no recorded split")
	err = f.importService.LinkTrading212Split(ctx, LinkTrading212SplitInput{Trading212SplitRowInput: input, OperationID: f.splitOpID})
	require.ErrorIs(t, err, ErrImportSplitLinkUnavailable)
	var instruments int
	require.NoError(t, f.database.QueryRow(`SELECT COUNT(*) FROM investment_instruments`).Scan(&instruments))
	assert.Equal(t, 1, instruments, "resolving a split row never creates an instrument")

	trade := f.splitFill
	trade.FillType, trade.FillID = "TRADE", "trade-not-split"
	tradeBatch, tradeRow := f.stageOrderFillRow(t, f.connectionID, trade)
	err = f.importService.LinkTrading212Split(ctx, LinkTrading212SplitInput{
		Trading212SplitRowInput: Trading212SplitRowInput{OwnerUserID: f.ownerUserID, BatchID: tradeBatch, RowID: tradeRow},
		OperationID:             f.splitOpID})
	require.ErrorIs(t, err, ErrImportSplitLinkUnavailable)
	// The fill type alone never posts a split.
	result, err := f.importService.CommitImportBatch(ctx, CommitImportBatchInput{OwnerUserID: f.ownerUserID, BatchID: batchID})
	require.NoError(t, err)
	assert.Zero(t, result.CommittedCount)
	assert.Equal(t, "20", f.holding(t))
}

func requireInvestSelfCheckPasses(t *testing.T, f *investTestFixture) {
	t.Helper()
	run, err := selfCheckOver(t, f.database).RunSelfCheck(context.Background(), "manual")
	require.NoError(t, err)
	for _, check := range []string{CheckLotReconciliation, CheckInvestmentFoundation} {
		result := resultFor(t, run, check)
		assert.Equal(t, SelfCheckPassed, result.Status, "%s: %s", check, result.Summary)
	}
}
