package app

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"rekenraam/backend/internal/onlinesource/trading212"
)

func TestFetchAdapterPreservesFillReviewEvidence(t *testing.T) {
	t.Parallel()
	payload, err := json.Marshal(trading212FetchPayload{ConnectionID: 1, OrderFills: toAdapterOrderFills([]trading212.OrderFill{
		{FillType: "FOP_CORRECTION", OrderStatus: "CANCELLED", FillID: "fill"},
	})})
	require.NoError(t, err)
	parsed, err := (&Trading212Adapter{}).Parse(context.Background(), RawInput{Bytes: payload}, nil)
	require.NoError(t, err)
	require.Len(t, parsed.Rows, 1)
	assert.Equal(t, "FOP_CORRECTION", parsed.Rows[0].Raw["fill_type"])
	assert.Equal(t, "CANCELLED", parsed.Rows[0].Raw["order_status"])
	require.Len(t, parsed.Warnings, 1)
	assert.Contains(t, parsed.Warnings[0].Message, "unsupported fill type")
}

func TestImportUnsupportedFillCannotPost(t *testing.T) {
	t.Parallel()
	for _, fillType := range []string{"STOCK_SPLIT", "STOCK_DISTRIBUTION", "FOP", "FOP_CORRECTION", "CUSTOM_STOCK_DISTRIBUTION", "EQUITY_RIGHTS", "SCRIP_STOCK_DIVIDENDS", "STOCK_DIVIDENDS", "STOCK_ACQUISITION", "CASH_AND_STOCK_ACQUISITION", "SPIN_OFF", "FUTURE_TYPE", ""} {
		name := fillType
		if name == "" {
			name = "missing"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f := newInvestTestFixture(t)
			conn := f.createConnection(t, &f.cashAccountID)
			batchID, rowID := f.stageOrderFillRow(t, conn.ID, trading212OrderFill{
				FillType: fillType, FillID: "unsupported", OrderID: "order", Side: "BUY",
				Ticker: "AAPL_US_EQ", ISIN: "US0378331005", Quantity: "2", Price: "100",
				Currency: "EUR", FilledAt: "2026-06-01T10:00:00Z", NetValue: "-200", NetValueCurrency: "EUR",
			})
			// Even explicit generic resolution and an includable status cannot
			// turn a corporate action into a purchase or cash transaction.
			f.resolveRowGeneric(t, batchID, rowID, f.cashAccountID)
			result, err := f.importService.CommitImportBatch(context.Background(), CommitImportBatchInput{
				OwnerUserID: f.ownerUserID, BatchID: batchID,
			})
			require.NoError(t, err)
			assert.Zero(t, result.CommittedCount)
			rows, err := f.importRepo.ListAllImportStagedRows(context.Background(), batchID)
			require.NoError(t, err)
			assert.False(t, rows[0].CommittedTransactionID.Valid)
			assert.False(t, rows[0].CommittedIdentityID.Valid)
			assert.Contains(t, rows[0].CommitError.String, "unsupported fill type")
			instruments, err := f.investmentSvc.ListInstruments(context.Background())
			require.NoError(t, err)
			assert.Empty(t, instruments)
		})
	}
}

func TestImportCancelledOrderPreservesExecutedTrade(t *testing.T) {
	t.Parallel()
	f := newInvestTestFixture(t)
	conn := f.createConnection(t, &f.cashAccountID)
	fill := trading212OrderFill{FillType: "TRADE", OrderStatus: "CANCELLED", FillID: "partial", OrderID: "order",
		Side: "BUY", Ticker: "AAPL_US_EQ", ISIN: "US0378331005", Quantity: "2", Price: "100",
		Currency: "EUR", FilledAt: "2026-06-01T10:00:00Z", NetValue: "-200", NetValueCurrency: "EUR"}
	commitSourceFill(t, f, conn.ID, fill)
	fill.OrderStatus = "FILLED"
	batchID, rowID := f.stageOrderFillRow(t, conn.ID, fill)
	row, err := f.importRepo.ImportStagedRowByID(context.Background(), rowID)
	require.NoError(t, err)
	assert.False(t, row.SourceChanged)
	assert.Equal(t, "duplicate", row.DedupeStatus)
	result, err := f.importService.CommitImportBatch(context.Background(), CommitImportBatchInput{OwnerUserID: f.ownerUserID, BatchID: batchID})
	require.NoError(t, err)
	assert.Zero(t, result.CommittedCount)
	positions, err := f.investmentSvc.Positions(context.Background())
	require.NoError(t, err)
	require.Len(t, positions, 1)
	assert.Equal(t, "2", positions[0].QuantityValue.String())
}

func TestUnsupportedSaleFillCannotUseCashFallback(t *testing.T) {
	t.Parallel()
	f := newInvestTestFixture(t)
	conn := f.createConnection(t, nil)
	batchID, rowID := f.stageOrderFillRow(t, conn.ID, trading212OrderFill{
		FillType: "FOP_CORRECTION", FillID: "unsupported-sale", Side: "SELL", Quantity: "2",
		FilledAt: "2026-06-01T10:00:00Z", NetValue: "200", NetValueCurrency: "EUR",
	})
	f.resolveRowGeneric(t, batchID, rowID, f.cashAccountID)
	preview, err := f.importService.PreviewCommit(context.Background(), PreviewCommitInput{OwnerUserID: f.ownerUserID, BatchID: batchID})
	require.NoError(t, err)
	assert.Zero(t, preview.IncludableCount)
	result, err := f.importService.CommitImportBatch(context.Background(), CommitImportBatchInput{OwnerUserID: f.ownerUserID, BatchID: batchID})
	require.NoError(t, err)
	assert.Zero(t, result.CommittedCount)
	row, err := f.importRepo.ImportStagedRowByID(context.Background(), rowID)
	require.NoError(t, err)
	assert.False(t, row.CommittedTransactionID.Valid)
	assert.False(t, row.CommittedIdentityID.Valid)
}

func TestUnsupportedFillCannotCorrectAcceptedBuyOrSale(t *testing.T) {
	t.Parallel()
	f := newSourceSaleFixture(t)
	for _, side := range []string{"BUY", "SELL"} {
		fill := f.fill
		if side == "BUY" {
			fill.FillID, fill.OrderID, fill.Side = "sale-buy", "sale-buy-order", "BUY"
			fill.Quantity, fill.Price, fill.NetValue, fill.FilledAt = "10", "100.00", "-1000.00", "2026-06-01T10:00:00Z"
		}
		fill.FillType = "FOP_CORRECTION"
		batchID, rowID := f.stageOrderFillRow(t, f.connectionID, fill)
		row, err := f.importRepo.ImportStagedRowByID(context.Background(), rowID)
		require.NoError(t, err)
		require.True(t, row.SourceChanged)
		if side == "BUY" {
			input := CorrectTrading212BuyInput{OwnerUserID: f.ownerUserID, BatchID: batchID, RowID: rowID, Reason: "provider revision"}
			_, err = f.importService.Trading212BuyCorrectionReconciliationImpact(context.Background(), input)
			require.ErrorIs(t, err, ErrImportSourceCorrectionConflict)
			_, err = acknowledgedCorrectTrading212Buy(context.Background(), f.importService, input)
		} else {
			input := CorrectTrading212SaleInput{OwnerUserID: f.ownerUserID, BatchID: batchID, RowID: rowID, Reason: "provider revision"}
			_, err = f.importService.Trading212SaleCorrectionReconciliationImpact(context.Background(), input)
			require.ErrorIs(t, err, ErrImportSourceCorrectionConflict)
			_, err = acknowledgedCorrectTrading212Sale(context.Background(), f.importService, input)
		}
		require.ErrorIs(t, err, ErrImportSourceCorrectionConflict)
	}
	positions, err := f.investmentSvc.Positions(context.Background())
	require.NoError(t, err)
	require.Len(t, positions, 1)
	assert.Equal(t, "8", positions[0].QuantityValue.String())
}
