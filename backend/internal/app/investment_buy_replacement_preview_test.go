package app

import (
	"context"
	"database/sql"
	"testing"

	"github.com/stretchr/testify/require"
	"rekenraam/backend/internal/db"
	"rekenraam/backend/internal/exact"
)

func TestBuyReplacementPreviewRejectsDependentDisposalWithoutWriting(t *testing.T) {
	t.Parallel()
	for _, method := range []string{"fifo", "lifo", "average_cost", "specific_lot"} {
		t.Run(method, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			f := newInvestmentsTestFixture(t)
			first := buyOn(t, f, "2026-01-01", 10, 10000)
			target := buyOn(t, f, "2026-02-01", 10, 30000)
			saleInput := sellInput(f, "2026-03-01", 15)
			saleInput.CostBasisMethod = method
			if method == "specific_lot" {
				saleInput.LotAllocations = []InvestmentLotAllocationInput{
					{LotID: *target.LotID, QuantityValue: exact.New(10)},
					{LotID: *first.LotID, QuantityValue: exact.New(5)},
				}
			}
			sale, err := f.investmentService.Sell(ctx, saleInput)
			require.NoError(t, err)
			checkpointID := reconcileCash(t, f, "2026-04-01", -300)
			input := ReplaceInvestmentBuyInput{OwnerUserID: f.ownerUserID, TransactionID: target.Transaction.ID,
				Reason: "correct acquisition quantity", Replacement: InvestmentTradeInput{
					TransactionDate: "2026-02-01", CommodityID: f.stockCommodityID, HoldingAccountID: f.holdingAccountID,
					CashAccountID: f.cashAccountID, CashCommodityID: f.eurCommodityID,
					QuantityValue: exact.New(2), CashAmountValue: 20000, CashAmountScale: 2,
				}}
			before := buyReplacementPreviewSnapshot(t, f.database)
			beforeLots, err := f.investmentService.ListLots(ctx, f.holdingAccountID, f.stockCommodityID)
			require.NoError(t, err)
			beforeGains, err := f.investmentService.ListRealizedGains(ctx, GainsReportParams{})
			require.NoError(t, err)
			beforeCount := f.transactionCount(t)
			_, err = f.investmentService.ReplaceBuyReconciliationImpact(ctx, input)
			require.ErrorIs(t, err, ErrInvestmentBuyDependency)
			var dependency InvestmentBuyDependencyError
			require.ErrorAs(t, err, &dependency)
			require.Equal(t, saleOperationIDForTest(t, f, sale.Transaction.ID), dependency.OperationID)
			require.Positive(t, dependency.DecisionID)
			afterLots, err := f.investmentService.ListLots(ctx, f.holdingAccountID, f.stockCommodityID)
			require.NoError(t, err)
			require.Equal(t, beforeLots, afterLots)
			afterGains, err := f.investmentService.ListRealizedGains(ctx, GainsReportParams{})
			require.NoError(t, err)
			require.Equal(t, beforeGains, afterGains)
			require.Equal(t, beforeCount, f.transactionCount(t))
			require.Equal(t, before, buyReplacementPreviewSnapshot(t, f.database))
			// A feasible preview must also restore audits, journal/order rows, prices,
			// replay revisions, queues and checkpoint evidence, even after repetition.
			input.Replacement.QuantityValue, input.Replacement.QuantityScale = exact.New(1000), 2
			for range 2 {
				impact, err := f.investmentService.ReplaceBuyReconciliationImpact(ctx, input)
				require.NoError(t, err)
				require.Len(t, impact.AffectedCheckpoints, 1)
				require.Equal(t, checkpointID, impact.AffectedCheckpoints[0].CheckpointID)
				require.Equal(t, before, buyReplacementPreviewSnapshot(t, f.database))
			}
			prepared, err := f.investmentService.prepareBuyReplacementWrite(ctx,
				ReplaceInvestmentBuyInput{OwnerUserID: input.OwnerUserID, TransactionID: input.TransactionID,
					Reason: input.Reason, Replacement: input.Replacement, ReconciliationOverride: true}, "browser_api", "investment.buy.replace")
			require.NoError(t, err)
			_, err = f.investmentService.ReplaceBuy(ctx, input)
			require.ErrorIs(t, err, ErrReconciliationOverrideRequired)
			require.Equal(t, before, buyReplacementPreviewSnapshot(t, f.database))
			input.ReconciliationOverride = true
			_, err = f.investmentService.ReplaceBuy(ctx, input)
			require.NoError(t, err)
			gains, err := f.investmentService.ListRealizedGains(ctx, GainsReportParams{})
			require.NoError(t, err)
			require.Len(t, gains, 1)
			expectedBasis := map[string]int64{"fifo": 20000, "lifo": 25000, "average_cost": 22500, "specific_lot": 25000}[method]
			require.Zero(t, exact.ScaledIntFromInt64(gains[0].DisposedBasisValue, gains[0].DisposedBasisScale).Cmp(exact.ScaledIntFromInt64(-expectedBasis, 2)))
			require.Zero(t, exact.ScaledIntFromInt64(gains[0].RealizedGainValue, gains[0].RealizedGainScale).Cmp(exact.ScaledIntFromInt64(10000-expectedBasis, 2)))
			committed := buyReplacementPreviewSnapshot(t, f.database)
			require.ErrorIs(t, f.investmentService.repository.SimulateBuyReplacement(ctx, prepared.Operation, prepared.Inverse, prepared.Replacement, prepared.Lot), db.ErrInvestmentOperationAlreadyCorrected)
			require.Equal(t, committed, buyReplacementPreviewSnapshot(t, f.database))
			check, err := selfCheckOver(t, f.database).RunSelfCheck(ctx, "manual")
			require.NoError(t, err)
			require.Equal(t, SelfCheckPassed, check.Status)

		})
	}
}

// Capture durable rows, not just counts: a preview must also preserve every
// existing amount, lifecycle flag and attribution timestamp.
func buyReplacementPreviewSnapshot(t *testing.T, database *sql.DB) map[string][][]any {
	t.Helper()
	snapshot := make(map[string][][]any)
	for _, table := range []string{
		"audit_events", "transactions", "transaction_versions", "journal_entries", "posting_lines", "posting_versions",
		"investment_operations", "investment_operation_journal_links", "investment_operation_dates", "investment_operation_components",
		"investment_lots", "investment_lot_state", "investment_lot_facts", "investment_lot_events", "investment_operation_lot_effects",
		"investment_position_basis_state", "investment_disposal_decisions", "investment_disposal_allocations", "investment_disposal_clearing_allocations",
		"investment_disposal_revisions", "investment_disposal_revision_allocations", "investment_transfer_facts", "investment_transfer_lot_links",
		"price_series", "price_observations", "reconciliation_checkpoints", "reconciliation_checkpoint_postings",
		"background_work_items", "import_batches", "import_staged_rows", "import_commit_identities", "import_commit_identity_effects", "import_source_revisions",
	} {
		rows, err := database.Query("SELECT * FROM " + table + " ORDER BY rowid")
		require.NoError(t, err)
		columns, err := rows.Columns()
		require.NoError(t, err)
		snapshot[table] = nil
		for rows.Next() {
			values := make([]any, len(columns))
			destinations := make([]any, len(columns))
			for index := range values {
				destinations[index] = &values[index]
			}
			require.NoError(t, rows.Scan(destinations...))
			for index, value := range values {
				if bytes, ok := value.([]byte); ok {
					values[index] = string(bytes)
				}
			}
			snapshot[table] = append(snapshot[table], values)
		}
		require.NoError(t, rows.Err())
		require.NoError(t, rows.Close())
	}
	return snapshot
}

func TestBuyReplacementPreviewPreservesSameDayRootOrder(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := newInvestmentsTestFixture(t)
	target := buyOn(t, f, "2026-01-01", 10, 10000)
	_, err := f.investmentService.Sell(ctx, sellInput(f, "2026-01-01", 4))
	require.NoError(t, err)
	buyOn(t, f, "2026-01-01", 10, 30000)
	input := ReplaceInvestmentBuyInput{OwnerUserID: f.ownerUserID, TransactionID: target.Transaction.ID,
		Reason: "correct first same-day acquisition", Replacement: InvestmentTradeInput{
			TransactionDate: "2026-01-01", CommodityID: f.stockCommodityID, HoldingAccountID: f.holdingAccountID,
			CashAccountID: f.cashAccountID, CashCommodityID: f.eurCommodityID,
			QuantityValue: exact.New(10), CashAmountValue: 20000, CashAmountScale: 2,
		}}
	before := buyReplacementPreviewSnapshot(t, f.database)
	_, err = f.investmentService.ReplaceBuyReconciliationImpact(ctx, input)
	require.NoError(t, err)
	require.Equal(t, before, buyReplacementPreviewSnapshot(t, f.database))
	_, err = f.investmentService.ReplaceBuy(ctx, input)
	require.NoError(t, err)
	gains, err := f.investmentService.ListRealizedGains(ctx, GainsReportParams{})
	require.NoError(t, err)
	require.Len(t, gains, 1)
	require.Zero(t, exact.ScaledIntFromInt64(gains[0].DisposedBasisValue, gains[0].DisposedBasisScale).Cmp(exact.ScaledIntFromInt64(-8000, 2)))
}

func TestBuyReplacementPreviewNamesInternalTransferDependency(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := newInvestmentsTestFixture(t)
	destination := seedTestAccountWithClass(t, f.database, "active", true, "asset", "security_holding")
	target := buyOn(t, f, "2026-05-01", 3, 1000)
	transfer, err := f.investmentService.InternalTransfer(ctx, internalTransferFromLot(f, destination, *target.LotID, exact.New(1), 0))
	require.NoError(t, err)
	input := ReplaceInvestmentBuyInput{OwnerUserID: f.ownerUserID, TransactionID: target.Transaction.ID,
		Reason: "correct transferred acquisition", Replacement: InvestmentTradeInput{
			TransactionDate: "2026-05-01", CommodityID: f.stockCommodityID, HoldingAccountID: f.holdingAccountID,
			CashAccountID: f.cashAccountID, CashCommodityID: f.eurCommodityID,
			QuantityValue: exact.New(3), CashAmountValue: 1100, CashAmountScale: 2,
		}}
	before := buyReplacementPreviewSnapshot(t, f.database)
	_, err = f.investmentService.ReplaceBuyReconciliationImpact(ctx, input)
	require.ErrorIs(t, err, ErrInvestmentBuyDependency)
	var dependency InvestmentBuyDependencyError
	require.ErrorAs(t, err, &dependency)
	var operationID int64
	require.NoError(t, f.database.QueryRow(`SELECT link.operation_id FROM investment_operation_journal_links link
  JOIN transaction_versions version ON version.id = link.transaction_version_id
  WHERE version.transaction_id = ? AND link.role = 'primary'`, transfer.Transaction.ID).Scan(&operationID))
	require.Equal(t, operationID, dependency.OperationID)
	require.Zero(t, dependency.DecisionID)
	require.Equal(t, before, buyReplacementPreviewSnapshot(t, f.database))
}

func TestTrading212BuyReplacementPreviewChecksReplayWithoutAcceptingSource(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := newSourceSaleFixture(t)
	changed := f.fill
	changed.FillID, changed.OrderID, changed.Side = "sale-buy", "sale-buy-order", "BUY"
	changed.FilledAt, changed.Quantity, changed.Price, changed.NetValue = "2026-06-01T10:00:00Z", "1", "100.00", "-100.00"
	batchID, rowID := f.stageOrderFillRow(t, f.connectionID, changed)
	input := CorrectTrading212BuyInput{OwnerUserID: f.ownerUserID, BatchID: batchID, RowID: rowID, Reason: "broker revised acquisition"}
	before := buyReplacementPreviewSnapshot(t, f.database)
	_, err := f.importService.Trading212BuyCorrectionReconciliationImpact(ctx, input)
	require.ErrorIs(t, err, ErrInvestmentBuyDependency)
	require.Equal(t, before, buyReplacementPreviewSnapshot(t, f.database))
	changed.Quantity, changed.NetValue = "3", "-300.00"
	input.BatchID, input.RowID = f.stageOrderFillRow(t, f.connectionID, changed)
	before = buyReplacementPreviewSnapshot(t, f.database)
	_, err = f.importService.Trading212BuyCorrectionReconciliationImpact(ctx, input)
	require.NoError(t, err)
	require.Equal(t, before, buyReplacementPreviewSnapshot(t, f.database))
	result, err := f.importService.CorrectTrading212Buy(ctx, input)
	require.NoError(t, err)
	require.Positive(t, result.Replacement.Transaction.ID)
	staged, err := f.importRepo.ImportStagedRowByID(ctx, input.RowID)
	require.NoError(t, err)
	require.Equal(t, "committed", staged.CommitStatus)
	check, err := selfCheckOver(t, f.database).RunSelfCheck(ctx, "manual")
	require.NoError(t, err)
	require.Equal(t, SelfCheckPassed, check.Status)
}

func TestBuyReplacementPreviewRollsBackLateCheckpointFailure(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := newInvestmentsTestFixture(t)
	target := buyOn(t, f, "2026-01-01", 10, 10000)
	reconcileCash(t, f, "2026-02-01", -100)
	input := ReplaceInvestmentBuyInput{OwnerUserID: f.ownerUserID, TransactionID: target.Transaction.ID,
		Reason: "correct purchase cost", Replacement: InvestmentTradeInput{
			TransactionDate: "2026-01-01", CommodityID: f.stockCommodityID, HoldingAccountID: f.holdingAccountID,
			CashAccountID: f.cashAccountID, CashCommodityID: f.eurCommodityID,
			QuantityValue: exact.New(10), CashAmountValue: 20000, CashAmountScale: 2,
		}}
	before := buyReplacementPreviewSnapshot(t, f.database)
	_, err := f.database.Exec(`CREATE TRIGGER reject_preview_checkpoint AFTER UPDATE ON reconciliation_checkpoints
  WHEN NEW.status = 'invalidated' BEGIN SELECT RAISE(ABORT, 'forced preview checkpoint failure'); END`)
	require.NoError(t, err)
	_, err = f.investmentService.ReplaceBuyReconciliationImpact(ctx, input)
	require.ErrorContains(t, err, "forced preview checkpoint failure")
	require.Equal(t, before, buyReplacementPreviewSnapshot(t, f.database))
	_, err = f.database.Exec(`DROP TRIGGER reject_preview_checkpoint`)
	require.NoError(t, err)
	_, err = f.investmentService.ReplaceBuyReconciliationImpact(ctx, input)
	require.NoError(t, err)
	require.Equal(t, before, buyReplacementPreviewSnapshot(t, f.database))
}
