package app

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"rekenraam/backend/internal/exact"
)

func saleOperationIDForTest(t *testing.T, f *investmentsTestFixture, transactionID int64) int64 {
	t.Helper()
	var operationID int64
	require.NoError(t, f.database.QueryRow(`SELECT id FROM investment_operations WHERE id IN (SELECT link.operation_id FROM investment_operation_journal_links link JOIN transaction_versions version ON version.id = link.transaction_version_id WHERE version.transaction_id = ? AND link.role = 'primary')`,
		transactionID).Scan(&operationID))
	return operationID
}

func TestReverseManualSaleReplaysLaterSaleAndKeepsOriginalHistory(t *testing.T) {
	ctx := context.Background()
	f := newInvestmentsTestFixture(t)
	_, err := f.investmentService.Buy(ctx, InvestmentTradeInput{
		OwnerUserID: f.ownerUserID, TransactionDate: "2026-01-01",
		CommodityID: f.stockCommodityID, HoldingAccountID: f.holdingAccountID,
		CashAccountID: f.cashAccountID, CashCommodityID: f.eurCommodityID,
		QuantityValue: exact.New(10), CashAmountValue: 100000, CashAmountScale: 2,
	})
	require.NoError(t, err)
	firstSale, err := f.investmentService.Sell(ctx, InvestmentTradeInput{
		OwnerUserID: f.ownerUserID, TransactionDate: "2026-02-01",
		CommodityID: f.stockCommodityID, HoldingAccountID: f.holdingAccountID,
		CashAccountID: f.cashAccountID, CashCommodityID: f.eurCommodityID,
		QuantityValue: exact.New(4), CashAmountValue: 48000, CashAmountScale: 2,
	})
	require.NoError(t, err)
	_, err = f.investmentService.Sell(ctx, InvestmentTradeInput{
		OwnerUserID: f.ownerUserID, TransactionDate: "2026-03-01",
		CommodityID: f.stockCommodityID, HoldingAccountID: f.holdingAccountID,
		CashAccountID: f.cashAccountID, CashCommodityID: f.eurCommodityID,
		QuantityValue: exact.New(3), CashAmountValue: 39000, CashAmountScale: 2,
	})
	require.NoError(t, err)
	operationID := saleOperationIDForTest(t, f, firstSale.Transaction.ID)
	beforeChain, err := f.investmentService.CorrectionChain(ctx, f.ownerUserID, firstSale.Transaction.ID)
	require.NoError(t, err)
	require.Len(t, beforeChain.Operations, 1)
	require.True(t, beforeChain.CanReverseManualSale)
	require.Equal(t, firstSale.Transaction.ID, *beforeChain.EffectiveTransactionID)
	reversal, err := acknowledgedReverseSale(ctx, f.investmentService, ReverseInvestmentSaleInput{
		OwnerUserID: f.ownerUserID, OperationID: operationID,
		Reason: "broker canceled this fill",
	})
	require.NoError(t, err)
	require.Equal(t, "posted", reversal.Status)
	require.NotNil(t, reversal.CorrectionOfTransactionID)
	require.Equal(t, firstSale.Transaction.ID, *reversal.CorrectionOfTransactionID)
	require.Len(t, reversal.JournalEntries, len(firstSale.Transaction.JournalEntries))
	for entryIndex, originalEntry := range firstSale.Transaction.JournalEntries {
		inverseEntry := reversal.JournalEntries[entryIndex]
		require.Equal(t, originalEntry.EntryDate, inverseEntry.EntryDate)
		require.Len(t, inverseEntry.Postings, len(originalEntry.Postings))
		for postingIndex, originalPosting := range originalEntry.Postings {
			inversePosting := inverseEntry.Postings[postingIndex]
			require.Equal(t, originalPosting.AccountID, inversePosting.AccountID)
			require.Equal(t, originalPosting.CommodityID, inversePosting.CommodityID)
			require.Equal(t, originalPosting.QuantityScale, inversePosting.QuantityScale)
			require.Equal(t, originalPosting.QuantityValue.Negated(), inversePosting.QuantityValue)
		}
	}
	var correctionOfOperationID int64
	var auditID int64
	require.NoError(t, f.database.QueryRow(`SELECT correction_of_operation_id, created_audit_event_id
		FROM investment_operations WHERE id IN (SELECT link.operation_id FROM investment_operation_journal_links link JOIN transaction_versions version ON version.id = link.transaction_version_id WHERE version.transaction_id = ? AND link.role = 'primary')`, reversal.ID).Scan(&correctionOfOperationID, &auditID))
	require.Equal(t, operationID, correctionOfOperationID)
	var retiredAuditID int64
	require.NoError(t, f.database.QueryRow(`SELECT voided_audit_event_id FROM price_observations
		WHERE source_transaction_version_id = ?`, firstSale.Transaction.VersionID).Scan(&retiredAuditID))
	require.Equal(t, auditID, retiredAuditID, "price retirement must share the command audit event")
	chain, err := f.investmentService.CorrectionChain(ctx, f.ownerUserID, firstSale.Transaction.ID)
	require.NoError(t, err)
	require.Equal(t, operationID, chain.RootOperationID)
	require.Len(t, chain.Operations, 2)
	require.Nil(t, chain.EffectiveTransactionID)
	require.False(t, chain.CanReverseManualSale)
	require.Equal(t, "reverse", chain.Operations[1].CorrectionMode)
	require.Equal(t, "broker canceled this fill", chain.Operations[1].CorrectionReason)
	require.False(t, chain.Operations[0].Effective)
	require.False(t, chain.Operations[1].Effective)
	fromReversal, err := f.investmentService.CorrectionChain(ctx, f.ownerUserID, reversal.ID)
	require.NoError(t, err)
	require.Equal(t, chain, fromReversal)
	lots, err := f.investmentService.ListLots(ctx, f.holdingAccountID, f.stockCommodityID)
	require.NoError(t, err)
	require.Len(t, lots, 1)
	require.Equal(t, "7", lots[0].RemainingQuantityValue.String())
	gains, err := f.investmentService.ListRealizedGains(ctx, GainsReportParams{})
	require.NoError(t, err)
	require.Len(t, gains, 1, "only the later effective sale contributes gain")
	var originalPosted int
	require.NoError(t, f.database.QueryRow(`SELECT count(*) FROM current_transaction_versions
		WHERE transaction_id = ? AND status = 'posted'`, firstSale.Transaction.ID).Scan(&originalPosted))
	require.Equal(t, 1, originalPosted, "the original sale remains posted history")
	check, err := selfCheckOver(t, f.database).RunSelfCheck(ctx, "manual")
	require.NoError(t, err)
	require.Equal(t, SelfCheckPassed, check.Status)
	_, err = acknowledgedReverseSale(ctx, f.investmentService, ReverseInvestmentSaleInput{
		OwnerUserID: f.ownerUserID, OperationID: operationID, Reason: "again",
	})
	require.ErrorIs(t, err, ErrInvestmentSaleAlreadyCorrected)
}

func TestReverseManualSaleRequiresReconciliationOverrideAtomically(t *testing.T) {
	ctx := context.Background()
	f := newInvestmentsTestFixture(t)
	_, err := f.investmentService.Buy(ctx, InvestmentTradeInput{
		OwnerUserID: f.ownerUserID, TransactionDate: "2026-01-01",
		CommodityID: f.stockCommodityID, HoldingAccountID: f.holdingAccountID,
		CashAccountID: f.cashAccountID, CashCommodityID: f.eurCommodityID,
		QuantityValue: exact.New(10), CashAmountValue: 100000, CashAmountScale: 2,
	})
	require.NoError(t, err)
	sale, err := f.investmentService.Sell(ctx, InvestmentTradeInput{
		OwnerUserID: f.ownerUserID, TransactionDate: "2026-02-01",
		CommodityID: f.stockCommodityID, HoldingAccountID: f.holdingAccountID,
		CashAccountID: f.cashAccountID, CashCommodityID: f.eurCommodityID,
		QuantityValue: exact.New(4), CashAmountValue: 48000, CashAmountScale: 2,
	})
	require.NoError(t, err)
	checkpointID := reconcileCash(t, f, "2026-03-01", -520)
	input := ReverseInvestmentSaleInput{OwnerUserID: f.ownerUserID, TransactionID: sale.Transaction.ID, Reason: "broker canceled fill"}
	impact, err := f.investmentService.ReverseSaleReconciliationImpact(ctx, input)
	require.NoError(t, err)
	require.Len(t, impact.AffectedCheckpoints, 1)
	require.Equal(t, checkpointID, impact.AffectedCheckpoints[0].CheckpointID)
	_, err = acknowledgedReverseSale(ctx, f.investmentService, input)
	require.ErrorIs(t, err, ErrReconciliationOverrideRequired)
	var correctionCount int
	require.NoError(t, f.database.QueryRow(`SELECT count(*) FROM investment_operations WHERE correction_of_operation_id IS NOT NULL`).Scan(&correctionCount))
	require.Zero(t, correctionCount, "rejected write must roll back operation and replay")
	var remaining string
	require.NoError(t, f.database.QueryRow(`SELECT remaining_quantity_value FROM current_investment_lots WHERE book_id = 1 AND account_id = ? AND commodity_id = ?`,
		f.holdingAccountID, f.stockCommodityID).Scan(&remaining))
	require.Equal(t, "6", remaining)
	var activePriceCount int
	require.NoError(t, f.database.QueryRow(`SELECT count(*) FROM price_observations WHERE source_transaction_version_id = ? AND voided_at IS NULL`,
		sale.Transaction.VersionID).Scan(&activePriceCount))
	require.Equal(t, 1, activePriceCount)
	require.Equal(t, []int64{checkpointID}, activeCheckpointIDs(t, f))
	input.ReconciliationOverride = true
	_, err = acknowledgedReverseSale(ctx, f.investmentService, input)
	require.NoError(t, err)
	require.Empty(t, activeCheckpointIDs(t, f))
	var invalidatedAuditID, correctionAuditID int64
	require.NoError(t, f.database.QueryRow(`SELECT invalidated_audit_event_id FROM reconciliation_checkpoints WHERE id = ?`, checkpointID).Scan(&invalidatedAuditID))
	require.NoError(t, f.database.QueryRow(`SELECT created_audit_event_id FROM investment_operations WHERE correction_of_operation_id IS NOT NULL`).Scan(&correctionAuditID))
	require.Equal(t, correctionAuditID, invalidatedAuditID)
}

func TestReverseImportedSaleRequiresSourceAwareCorrection(t *testing.T) {
	ctx := context.Background()
	f := newInvestmentsTestFixture(t)
	_, err := f.investmentService.Buy(ctx, InvestmentTradeInput{
		OwnerUserID: f.ownerUserID, TransactionDate: "2026-01-01",
		CommodityID: f.stockCommodityID, HoldingAccountID: f.holdingAccountID,
		CashAccountID: f.cashAccountID, CashCommodityID: f.eurCommodityID,
		QuantityValue: exact.New(5), CashAmountValue: 50000, CashAmountScale: 2,
	})
	require.NoError(t, err)
	sale, err := f.investmentService.Sell(ctx, InvestmentTradeInput{
		OwnerUserID: f.ownerUserID, OriginType: "import", TransactionDate: "2026-02-01",
		CommodityID: f.stockCommodityID, HoldingAccountID: f.holdingAccountID,
		CashAccountID: f.cashAccountID, CashCommodityID: f.eurCommodityID,
		QuantityValue: exact.New(2), CashAmountValue: 24000, CashAmountScale: 2,
	})
	require.NoError(t, err)
	chain, err := f.investmentService.CorrectionChain(ctx, f.ownerUserID, sale.Transaction.ID)
	require.NoError(t, err)
	require.Len(t, chain.Operations, 1)
	require.True(t, chain.Operations[0].Imported)
	require.False(t, chain.CanReverseManualSale)
	_, err = acknowledgedReverseSale(ctx, f.investmentService, ReverseInvestmentSaleInput{
		OwnerUserID: f.ownerUserID, TransactionID: sale.Transaction.ID, Reason: "source removed fill",
	})
	require.ErrorIs(t, err, ErrInvestmentImportedSale)
}
