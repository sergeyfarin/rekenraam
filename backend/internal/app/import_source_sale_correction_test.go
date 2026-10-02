package app

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"rekenraam/backend/internal/db"
	"rekenraam/backend/internal/exact"
)

type sourceSaleFixture struct {
	*investTestFixture
	connectionID int64
	original     db.ImportStagedRowRecord
	fill         trading212OrderFill
}

func newSourceSaleFixture(t *testing.T) sourceSaleFixture {
	t.Helper()
	f := newInvestTestFixture(t)
	conn := f.createConnection(t, &f.cashAccountID)
	buy := trading212OrderFill{FillType: "TRADE", FillID: "sale-buy", OrderID: "sale-buy-order", Ticker: "AAPL_US_EQ", ISIN: "US0378331005",
		Side: "BUY", Quantity: "10", Price: "100.00", Currency: "EUR", FilledAt: "2026-06-01T10:00:00Z", NetValue: "-1000.00", NetValueCurrency: "EUR"}
	commitSourceFill(t, f, conn.ID, buy)
	sale := buy
	sale.FillID, sale.OrderID, sale.Side = "sale-fill", "sale-order", "SELL"
	sale.Quantity, sale.NetValue, sale.Price, sale.FilledAt = "2", "300.00", "150.00", "2026-07-01T10:00:00Z"
	row := commitSourceFill(t, f, conn.ID, sale)
	require.True(t, row.SourceSaleOperation)
	require.False(t, row.SourceBuyOperation)
	return sourceSaleFixture{f, conn.ID, row, sale}
}

func commitSourceFill(t *testing.T, f *investTestFixture, connectionID int64, fill trading212OrderFill) db.ImportStagedRowRecord {
	t.Helper()
	batchID, _ := f.stageOrderFillRow(t, connectionID, fill)
	result, err := f.importService.CommitImportBatch(context.Background(), CommitImportBatchInput{OwnerUserID: f.ownerUserID, BatchID: batchID})
	require.NoError(t, err)
	require.Equal(t, 1, result.CommittedCount)
	rows, err := f.importRepo.ListAllImportStagedRows(context.Background(), batchID)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Len(t, rows[0].CommitEffects, 1)
	require.True(t, rows[0].CommitEffects[0].OperationID.Valid)
	return rows[0]
}

func (f sourceSaleFixture) revision(t *testing.T, fill trading212OrderFill) CorrectTrading212SaleInput {
	t.Helper()
	batchID, rowID := f.stageOrderFillRow(t, f.connectionID, fill)
	return CorrectTrading212SaleInput{OwnerUserID: f.ownerUserID, BatchID: batchID, RowID: rowID, Reason: "broker revised sale fill"}
}

func TestCorrectTrading212SaleKeepsIdentityAndRecordedMethodWithDependentReplay(t *testing.T) {
	for _, method := range []string{"fifo", "lifo", "average_cost"} {
		t.Run(method, func(t *testing.T) {
			t.Parallel()
			f := newSourceSaleFixture(t)
			ctx := context.Background()
			if method != "fifo" {
				source, err := f.investmentSvc.TradeCorrectionContext(ctx, f.ownerUserID, f.original.CommittedTransactionID.Int64)
				require.NoError(t, err)
				_, err = acknowledgedReplaceSale(ctx, f.investmentSvc, ReplaceInvestmentSaleInput{OwnerUserID: f.ownerUserID, TransactionID: source.TransactionID,
					Reason: "explicit method election", Replacement: InvestmentTradeInput{TransactionDate: source.EventDate,
						HoldingAccountID: source.HoldingAccountID, CommodityID: source.CommodityID, CashAccountID: source.CashAccountID,
						CashCommodityID: source.CostCommodityID, QuantityValue: exact.New(2), CashAmountValue: 30000, CashAmountScale: 2, CostBasisMethod: method}})
				require.NoError(t, err)
			}
			source, err := f.investmentSvc.TradeCorrectionContext(ctx, f.ownerUserID, f.original.CommittedTransactionID.Int64)
			require.NoError(t, err)
			_, err = f.investmentSvc.Sell(ctx, InvestmentTradeInput{OwnerUserID: f.ownerUserID, TransactionDate: "2026-07-03",
				HoldingAccountID: source.HoldingAccountID, CommodityID: source.CommodityID, CashAccountID: source.CashAccountID,
				CashCommodityID: source.CostCommodityID, QuantityValue: exact.New(1), CashAmountValue: 16000, CashAmountScale: 2, CostBasisMethod: method})
			require.NoError(t, err)
			stale := f.fill
			stale.NetValue = "301.00"
			staleInput := f.revision(t, stale)
			changed := f.fill
			changed.Quantity, changed.NetValue = "3", "450.00"
			input := f.revision(t, changed)
			impact, err := f.importService.Trading212SaleCorrectionReconciliationImpact(ctx, input)
			require.NoError(t, err)
			require.Empty(t, impact.AffectedCheckpoints)
			result, err := acknowledgedCorrectTrading212Sale(ctx, f.importService, input)
			require.NoError(t, err)
			corrected, err := f.investmentSvc.TradeCorrectionContext(ctx, f.ownerUserID, result.Replacement.Transaction.ID)
			require.NoError(t, err)
			require.Equal(t, "3", corrected.QuantityValue)
			require.Equal(t, "45000", corrected.NetValue)
			require.Equal(t, method, corrected.CostBasisMethod)
			require.True(t, corrected.CanReplaceSale)
			require.Equal(t, f.original.CommittedIdentityID.Int64, corrected.SourceIdentityID)
			positions, err := f.investmentSvc.Positions(ctx)
			require.NoError(t, err)
			require.Len(t, positions, 1)
			require.Zero(t, exact.ScaledIntFromCoefficient(positions[0].QuantityValue, positions[0].QuantityScale).Cmp(exact.ScaledIntFromInt64(6, 0)))
			staged, err := f.importRepo.ImportStagedRowByID(ctx, input.RowID)
			require.NoError(t, err)
			require.Equal(t, "committed", staged.CommitStatus)
			require.Equal(t, f.original.CommittedIdentityID, staged.CommittedIdentityID)
			require.Equal(t, f.original.CommittedTransactionID, staged.CommittedTransactionID)
			_, err = acknowledgedCorrectTrading212Sale(ctx, f.importService, input)
			require.ErrorIs(t, err, ErrImportSourceCorrectionConflict)
			_, err = acknowledgedCorrectTrading212Sale(ctx, f.importService, staleInput)
			require.ErrorIs(t, err, ErrImportSourceCorrectionConflict)
			newer := changed
			newer.NetValue = "451.00"
			_, err = acknowledgedCorrectTrading212Sale(ctx, f.importService, f.revision(t, newer))
			require.NoError(t, err)
			repeatBatch, _ := f.stageOrderFillRow(t, f.connectionID, newer)
			repeat, err := f.importRepo.ListAllImportStagedRows(ctx, repeatBatch)
			require.NoError(t, err)
			require.False(t, repeat[0].SourceChanged)
			require.Equal(t, "duplicate", repeat[0].DedupeStatus)
			var revisions int
			require.NoError(t, f.database.QueryRow(`SELECT count(*) FROM import_source_revisions WHERE identity_id = ?`, f.original.CommittedIdentityID.Int64).Scan(&revisions))
			require.Equal(t, 2, revisions)
			var origin, operation string
			require.NoError(t, f.database.QueryRow(`SELECT origin_type, operation FROM audit_events WHERE id = (SELECT created_audit_event_id FROM investment_operations WHERE id = ?)`, corrected.OperationID).Scan(&origin, &operation))
			require.Equal(t, "import", origin)
			require.Equal(t, "investment.sale.source_correct", operation)
			assertSourceSaleIntegrity(t, f)
		})
	}
}

func sourceSaleWriteSnapshot(t *testing.T, f sourceSaleFixture) map[string]string {
	t.Helper()
	result := map[string]string{}
	for _, table := range []string{"audit_events", "transactions", "investment_operations", "investment_lot_events", "investment_disposal_decisions", "investment_disposal_allocations", "investment_disposal_clearing_allocations", "investment_disposal_revisions", "import_source_revisions", "price_observations"} {
		var count string
		require.NoError(t, f.database.QueryRow("SELECT count(*) FROM "+table).Scan(&count))
		result[table] = count
	}
	var resultValue string
	require.NoError(t, f.database.QueryRow(`SELECT remaining_quantity_value || ':' || remaining_quantity_scale || ':' || remaining_cost_basis_value || ':' || remaining_cost_basis_scale FROM investment_lot_state`).Scan(&resultValue))
	result["lot-state"] = resultValue
	require.NoError(t, f.database.QueryRow(`SELECT count(*) FROM price_observations WHERE voided_at IS NOT NULL`).Scan(&resultValue))
	result["voided-prices"] = resultValue
	return result
}

func assertSourceSaleIntegrity(t *testing.T, f sourceSaleFixture) {
	t.Helper()
	run, err := selfCheckOver(t, f.database).RunSelfCheck(context.Background(), "manual")
	require.NoError(t, err)
	for _, check := range []string{CheckInvestmentFoundation, CheckLotReconciliation, CheckCommodityPositionSign, CheckEntryBalance, CheckTransactionBalance} {
		result := resultFor(t, run, check)
		require.Equal(t, SelfCheckPassed, result.Status, "%s: %s", check, result.Summary)
	}
}

func TestCorrectTrading212SalePreviewAndSourceAcceptanceRollback(t *testing.T) {
	t.Parallel()
	f := newSourceSaleFixture(t)
	ctx := context.Background()
	checkpointID := f.createCashReconciliationCheckpoint(t, "2026-07-10")
	changed := f.fill
	changed.NetValue = "301.00"
	input := f.revision(t, changed)
	before := sourceSaleWriteSnapshot(t, f)
	impact, err := f.importService.Trading212SaleCorrectionReconciliationImpact(ctx, input)
	require.NoError(t, err)
	require.Len(t, impact.AffectedCheckpoints, 1)
	require.Equal(t, checkpointID, impact.AffectedCheckpoints[0].CheckpointID)
	require.Equal(t, before, sourceSaleWriteSnapshot(t, f))
	_, err = acknowledgedCorrectTrading212Sale(ctx, f.importService, input)
	require.ErrorIs(t, err, ErrReconciliationOverrideRequired)
	require.Equal(t, before, sourceSaleWriteSnapshot(t, f))
	_, err = f.database.Exec(`CREATE TRIGGER abort_sale_source_revision BEFORE INSERT ON import_source_revisions BEGIN SELECT RAISE(ABORT, 'injected source acceptance failure'); END`)
	require.NoError(t, err)
	input.ReconciliationOverride = true
	_, err = acknowledgedCorrectTrading212Sale(ctx, f.importService, input)
	require.ErrorContains(t, err, "injected source acceptance failure")
	require.Equal(t, before, sourceSaleWriteSnapshot(t, f))
	staged, err := f.importRepo.ImportStagedRowByID(ctx, input.RowID)
	require.NoError(t, err)
	require.Equal(t, "pending", staged.CommitStatus)
	var status string
	require.NoError(t, f.database.QueryRow(`SELECT status FROM reconciliation_checkpoints WHERE id = ?`, checkpointID).Scan(&status))
	require.Equal(t, "active", status)
	_, err = f.database.Exec(`DROP TRIGGER abort_sale_source_revision`)
	require.NoError(t, err)
	result, err := acknowledgedCorrectTrading212Sale(ctx, f.importService, input)
	require.NoError(t, err)
	require.Contains(t, append(result.Inverse.InvalidatedCheckpointIDs, result.Replacement.Transaction.InvalidatedCheckpointIDs...), checkpointID)
	assertSourceSaleIntegrity(t, f)
}

func TestCorrectTrading212SaleRefusesUnsupportedSourceChangesAtomically(t *testing.T) {
	for _, test := range []struct{ name, field, value string }{
		{"date", "date", "2026-07-02T10:00:00Z"}, {"instrument", "ticker", "OTHER"},
		{"currency", "currency", "USD"}, {"cancellation quantity", "quantity", "-2"},
		{"negative proceeds", "net", "-300.00"}, {"zero proceeds", "net", "0.00"},
		{"insufficient holdings", "quantity", "11"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			f := newSourceSaleFixture(t)
			changed := f.fill
			switch test.field {
			case "date":
				changed.FilledAt = test.value
			case "ticker":
				changed.Ticker = test.value
			case "currency":
				changed.NetValueCurrency = test.value
			case "quantity":
				changed.Quantity = test.value
			case "net":
				changed.NetValue = test.value
			}
			input := f.revision(t, changed)
			before := sourceSaleWriteSnapshot(t, f)
			_, err := f.importService.Trading212SaleCorrectionReconciliationImpact(context.Background(), input)
			require.Error(t, err)
			_, err = acknowledgedCorrectTrading212Sale(context.Background(), f.importService, input)
			require.Error(t, err)
			require.Equal(t, before, sourceSaleWriteSnapshot(t, f))
			row, err := f.importRepo.ImportStagedRowByID(context.Background(), input.RowID)
			require.NoError(t, err)
			require.Equal(t, "pending", row.CommitStatus)
		})
	}
}

func TestCorrectTrading212SalePreservesSpecificElectionWithoutGuessingQuantityChanges(t *testing.T) {
	t.Parallel()
	f := newSourceSaleFixture(t)
	ctx := context.Background()
	source, err := f.investmentSvc.TradeCorrectionContext(ctx, f.ownerUserID, f.original.CommittedTransactionID.Int64)
	require.NoError(t, err)
	require.Len(t, source.AvailableLots, 1)
	_, err = acknowledgedReplaceSale(ctx, f.investmentSvc, ReplaceInvestmentSaleInput{OwnerUserID: f.ownerUserID, TransactionID: source.TransactionID,
		Reason: "explicit lot election", Replacement: InvestmentTradeInput{TransactionDate: source.EventDate,
			HoldingAccountID: source.HoldingAccountID, CommodityID: source.CommodityID, CashAccountID: source.CashAccountID,
			CashCommodityID: source.CostCommodityID, QuantityValue: exact.New(2), CashAmountValue: 30000, CashAmountScale: 2,
			CostBasisMethod: "specific_lot", LotAllocations: []InvestmentLotAllocationInput{{LotID: source.AvailableLots[0].LotID, QuantityValue: exact.New(2)}}}})
	require.NoError(t, err)
	changed := f.fill
	changed.NetValue = "310.00"
	result, err := acknowledgedCorrectTrading212Sale(ctx, f.importService, f.revision(t, changed))
	require.NoError(t, err)
	corrected, err := f.investmentSvc.TradeCorrectionContext(ctx, f.ownerUserID, result.Replacement.Transaction.ID)
	require.NoError(t, err)
	require.Equal(t, "specific_lot", corrected.CostBasisMethod)
	require.Len(t, corrected.EffectiveElectedLots, 1)
	require.Equal(t, source.AvailableLots[0].LotID, corrected.EffectiveElectedLots[0].LotID)
	changed.NetValue = "311.00"
	_, err = acknowledgedCorrectTrading212Sale(ctx, f.importService, f.revision(t, changed))
	require.NoError(t, err, "a source correction descendant must retain its effective specific-lot election")
	changed.Quantity = "3"
	input := f.revision(t, changed)
	before := sourceSaleWriteSnapshot(t, f)
	_, err = acknowledgedCorrectTrading212Sale(ctx, f.importService, input)
	require.ErrorContains(t, err, "explicit lot election")
	require.Equal(t, before, sourceSaleWriteSnapshot(t, f))
	assertSourceSaleIntegrity(t, f)
}

func TestCorrectTrading212SaleRefusesImpossibleDependentReplay(t *testing.T) {
	t.Parallel()
	f := newSourceSaleFixture(t)
	ctx := context.Background()
	source, err := f.investmentSvc.TradeCorrectionContext(ctx, f.ownerUserID, f.original.CommittedTransactionID.Int64)
	require.NoError(t, err)
	_, err = f.investmentSvc.Sell(ctx, InvestmentTradeInput{OwnerUserID: f.ownerUserID, TransactionDate: "2026-07-03",
		HoldingAccountID: source.HoldingAccountID, CommodityID: source.CommodityID, CashAccountID: source.CashAccountID,
		CashCommodityID: source.CostCommodityID, QuantityValue: exact.New(8), CashAmountValue: 120000, CashAmountScale: 2, CostBasisMethod: "fifo"})
	require.NoError(t, err)
	changed := f.fill
	changed.Quantity, changed.NetValue = "3", "450.00"
	input := f.revision(t, changed)
	before := sourceSaleWriteSnapshot(t, f)
	_, err = f.importService.Trading212SaleCorrectionReconciliationImpact(ctx, input)
	require.ErrorIs(t, err, ErrInvestmentSaleDependency)
	_, err = acknowledgedCorrectTrading212Sale(ctx, f.importService, input)
	require.ErrorIs(t, err, ErrInvestmentSaleDependency)
	require.Equal(t, before, sourceSaleWriteSnapshot(t, f))
	row, err := f.importRepo.ImportStagedRowByID(ctx, input.RowID)
	require.NoError(t, err)
	require.Equal(t, "pending", row.CommitStatus)
	assertSourceSaleIntegrity(t, f)
}

func TestCorrectTrading212SaleDoesNotOfferCashFallback(t *testing.T) {
	t.Parallel()
	f := newInvestTestFixture(t)
	ctx := context.Background()
	connection := f.createConnection(t, nil)
	fill := trading212OrderFill{FillType: "TRADE", FillID: "fallback-sale", OrderID: "fallback-order", Ticker: "AAPL_US_EQ", ISIN: "US0378331005",
		Side: "SELL", Quantity: "2", Price: "150.00", Currency: "EUR", FilledAt: "2026-07-01T10:00:00Z", NetValue: "300.00", NetValueCurrency: "EUR"}
	batchID, rowID := f.stageOrderFillRow(t, connection.ID, fill)
	f.resolveRowGeneric(t, batchID, rowID, f.cashAccountID)
	result, err := f.importService.CommitImportBatch(ctx, CommitImportBatchInput{OwnerUserID: f.ownerUserID, BatchID: batchID})
	require.NoError(t, err)
	require.Equal(t, 1, result.CommittedCount)
	fill.NetValue = "301.00"
	batchID, rowID = f.stageOrderFillRow(t, connection.ID, fill)
	row, err := f.importRepo.ImportStagedRowByID(ctx, rowID)
	require.NoError(t, err)
	require.True(t, row.SourceChanged)
	require.False(t, row.SourceSaleOperation)
	_, err = acknowledgedCorrectTrading212Sale(ctx, f.importService, CorrectTrading212SaleInput{OwnerUserID: f.ownerUserID, BatchID: batchID, RowID: rowID, Reason: "revised cash fallback"})
	require.ErrorIs(t, err, ErrImportSourceCorrectionConflict)
}
