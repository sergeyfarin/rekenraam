package app

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"rekenraam/backend/internal/exact"
)

func TestReplaceLatestManualSalePostsOneAuditedCompoundCorrection(t *testing.T) {
	ctx := context.Background()
	f := newInvestmentsTestFixture(t)
	_, err := f.investmentService.Buy(ctx, InvestmentTradeInput{
		OwnerUserID: f.ownerUserID, TransactionDate: "2026-01-01",
		CommodityID: f.stockCommodityID, HoldingAccountID: f.holdingAccountID,
		CashAccountID: f.cashAccountID, CashCommodityID: f.eurCommodityID,
		QuantityValue: exact.New(10), CashAmountValue: 100000, CashAmountScale: 2,
	})
	require.NoError(t, err)
	original, err := f.investmentService.Sell(ctx, InvestmentTradeInput{
		OwnerUserID: f.ownerUserID, TransactionDate: "2026-02-01",
		CommodityID: f.stockCommodityID, HoldingAccountID: f.holdingAccountID,
		CashAccountID: f.cashAccountID, CashCommodityID: f.eurCommodityID,
		QuantityValue: exact.New(4), CashAmountValue: 48000, CashAmountScale: 2,
	})
	require.NoError(t, err)
	result, err := f.investmentService.ReplaceLatestSale(ctx, ReplaceInvestmentSaleInput{
		OwnerUserID: f.ownerUserID, TransactionID: original.Transaction.ID,
		Reason: "corrected broker fill",
		Replacement: InvestmentTradeInput{
			TransactionDate: "2026-02-01", CommodityID: f.stockCommodityID,
			HoldingAccountID: f.holdingAccountID, CashAccountID: f.cashAccountID,
			CashCommodityID: f.eurCommodityID, QuantityValue: exact.New(3),
			CashAmountValue: 39000, CashAmountScale: 2,
		},
	})
	require.NoError(t, err)
	require.Equal(t, "posted", result.Inverse.Status)
	require.Equal(t, "posted", result.Replacement.Transaction.Status)
	require.Equal(t, original.Transaction.ID, *result.Replacement.Transaction.CorrectionOfTransactionID)
	var inverseAudit, replacementAudit, linkedInverse int64
	require.NoError(t, f.database.QueryRow(`SELECT created_audit_event_id FROM transactions WHERE id = ?`, result.Inverse.ID).Scan(&inverseAudit))
	require.NoError(t, f.database.QueryRow(`SELECT created_audit_event_id FROM transactions WHERE id = ?`, result.Replacement.Transaction.ID).Scan(&replacementAudit))
	require.Equal(t, inverseAudit, replacementAudit)
	var retiredAudit, activePriceCount int64
	require.NoError(t, f.database.QueryRow(`SELECT voided_audit_event_id FROM price_observations
		WHERE source_transaction_version_id = ?`, original.Transaction.VersionID).Scan(&retiredAudit))
	require.Equal(t, inverseAudit, retiredAudit)
	require.NoError(t, f.database.QueryRow(`SELECT count(*) FROM price_observations
		WHERE source_transaction_version_id = ? AND voided_at IS NULL`, result.Replacement.Transaction.VersionID).Scan(&activePriceCount))
	require.EqualValues(t, 1, activePriceCount)
	require.NoError(t, f.database.QueryRow(`SELECT link.transaction_version_id FROM investment_operation_journal_links link
		JOIN investment_operations operation ON operation.id = link.operation_id
		WHERE operation.transaction_id = ? AND link.role = 'reversal'`, result.Replacement.Transaction.ID).Scan(&linkedInverse))
	require.Equal(t, result.Inverse.VersionID, linkedInverse)
	lots, err := f.investmentService.ListLots(ctx, f.holdingAccountID, f.stockCommodityID)
	require.NoError(t, err)
	require.Len(t, lots, 1)
	require.Equal(t, "7", lots[0].RemainingQuantityValue.String())
	gains, err := f.investmentService.ListRealizedGains(ctx, GainsReportParams{})
	require.NoError(t, err)
	require.Len(t, gains, 1)
	chain, err := f.investmentService.CorrectionChain(ctx, f.ownerUserID, original.Transaction.ID)
	require.NoError(t, err)
	require.Len(t, chain.Operations, 2)
	require.Equal(t, result.Replacement.Transaction.ID, *chain.EffectiveTransactionID)
	require.Equal(t, "replace", chain.Operations[1].CorrectionMode)
	fromInverse, err := f.investmentService.CorrectionChain(ctx, f.ownerUserID, result.Inverse.ID)
	require.NoError(t, err)
	require.Equal(t, chain, fromInverse)
	check, err := selfCheckOver(t, f.database).RunSelfCheck(ctx, "manual")
	require.NoError(t, err)
	require.Equal(t, SelfCheckPassed, check.Status)
}

func TestReplaceLatestManualSaleCanUseSharesRestoredByItsOwnInverse(t *testing.T) {
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
	result, err := f.investmentService.ReplaceLatestSale(ctx, ReplaceInvestmentSaleInput{
		OwnerUserID: f.ownerUserID, TransactionID: sale.Transaction.ID, Reason: "fill was eight shares",
		Replacement: InvestmentTradeInput{
			TransactionDate: "2026-02-01", CommodityID: f.stockCommodityID,
			HoldingAccountID: f.holdingAccountID, CashAccountID: f.cashAccountID,
			CashCommodityID: f.eurCommodityID, QuantityValue: exact.New(8),
			CashAmountValue: 96000, CashAmountScale: 2,
		},
	})
	require.NoError(t, err)
	require.Len(t, result.Replacement.Allocations, 1)
	lots, err := f.investmentService.ListLots(ctx, f.holdingAccountID, f.stockCommodityID)
	require.NoError(t, err)
	require.Equal(t, "2", lots[0].RemainingQuantityValue.String())
}

func TestReplaceOlderManualSaleRefusesLaterPositionIntentWithoutPartialWrite(t *testing.T) {
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
	_, err = f.investmentService.Buy(ctx, InvestmentTradeInput{
		OwnerUserID: f.ownerUserID, TransactionDate: "2026-03-01",
		CommodityID: f.stockCommodityID, HoldingAccountID: f.holdingAccountID,
		CashAccountID: f.cashAccountID, CashCommodityID: f.eurCommodityID,
		QuantityValue: exact.New(2), CashAmountValue: 20000, CashAmountScale: 2,
	})
	require.NoError(t, err)
	_, err = f.investmentService.ReplaceLatestSale(ctx, ReplaceInvestmentSaleInput{
		OwnerUserID: f.ownerUserID, TransactionID: sale.Transaction.ID, Reason: "correct fill",
		Replacement: InvestmentTradeInput{
			TransactionDate: "2026-02-01", CommodityID: f.stockCommodityID,
			HoldingAccountID: f.holdingAccountID, CashAccountID: f.cashAccountID,
			CashCommodityID: f.eurCommodityID, QuantityValue: exact.New(3),
			CashAmountValue: 39000, CashAmountScale: 2,
		},
	})
	require.ErrorIs(t, err, ErrInvestmentSaleNotLatest)
	var correctionCount int
	require.NoError(t, f.database.QueryRow(`SELECT count(*) FROM investment_operations
		WHERE correction_of_operation_id IS NOT NULL`).Scan(&correctionCount))
	require.Zero(t, correctionCount)
	var activePriceCount int
	require.NoError(t, f.database.QueryRow(`SELECT count(*) FROM price_observations
		WHERE source_transaction_version_id = ? AND voided_at IS NULL`, sale.Transaction.VersionID).Scan(&activePriceCount))
	require.Equal(t, 1, activePriceCount)
}

func TestReplaceLatestManualSaleRequiresReconciliationOverrideAtomically(t *testing.T) {
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
	input := ReplaceInvestmentSaleInput{
		OwnerUserID: f.ownerUserID, TransactionID: sale.Transaction.ID, Reason: "correct fill",
		Replacement: InvestmentTradeInput{
			TransactionDate: "2026-02-01", CommodityID: f.stockCommodityID,
			HoldingAccountID: f.holdingAccountID, CashAccountID: f.cashAccountID,
			CashCommodityID: f.eurCommodityID, QuantityValue: exact.New(3),
			CashAmountValue: 39000, CashAmountScale: 2,
		},
	}
	impact, err := f.investmentService.ReplaceLatestSaleReconciliationImpact(ctx, input)
	require.NoError(t, err)
	require.Len(t, impact.AffectedCheckpoints, 1)
	require.Equal(t, checkpointID, impact.AffectedCheckpoints[0].CheckpointID)
	_, err = f.investmentService.ReplaceLatestSale(ctx, input)
	require.ErrorIs(t, err, ErrReconciliationOverrideRequired)
	var correctionCount int
	require.NoError(t, f.database.QueryRow(`SELECT count(*) FROM investment_operations
		WHERE correction_of_operation_id IS NOT NULL`).Scan(&correctionCount))
	require.Zero(t, correctionCount)
	require.Equal(t, []int64{checkpointID}, activeCheckpointIDs(t, f))
	input.ReconciliationOverride = true
	result, err := f.investmentService.ReplaceLatestSale(ctx, input)
	require.NoError(t, err)
	require.Empty(t, activeCheckpointIDs(t, f))
	var checkpointAudit, replacementAudit int64
	require.NoError(t, f.database.QueryRow(`SELECT invalidated_audit_event_id FROM reconciliation_checkpoints WHERE id = ?`,
		checkpointID).Scan(&checkpointAudit))
	require.NoError(t, f.database.QueryRow(`SELECT created_audit_event_id FROM transactions WHERE id = ?`,
		result.Replacement.Transaction.ID).Scan(&replacementAudit))
	require.Equal(t, replacementAudit, checkpointAudit)
}

func TestReplaceLatestManualSaleRollsBackImpossibleQuantity(t *testing.T) {
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
	_, err = f.investmentService.ReplaceLatestSale(ctx, ReplaceInvestmentSaleInput{
		OwnerUserID: f.ownerUserID, TransactionID: sale.Transaction.ID,
		Reason: "broker fill exceeds balance",
		Replacement: InvestmentTradeInput{
			TransactionDate: "2026-02-01", CommodityID: f.stockCommodityID,
			HoldingAccountID: f.holdingAccountID, CashAccountID: f.cashAccountID,
			CashCommodityID: f.eurCommodityID, QuantityValue: exact.New(11),
			CashAmountValue: 132000, CashAmountScale: 2,
		},
	})
	require.ErrorIs(t, err, ErrInvestmentLotsInsufficient)
	var correctionCount, transactionCount int
	require.NoError(t, f.database.QueryRow(`SELECT count(*) FROM investment_operations
		WHERE correction_of_operation_id IS NOT NULL`).Scan(&correctionCount))
	require.Zero(t, correctionCount)
	require.NoError(t, f.database.QueryRow(`SELECT count(*) FROM transactions`).Scan(&transactionCount))
	require.Equal(t, 2, transactionCount)
	lots, err := f.investmentService.ListLots(ctx, f.holdingAccountID, f.stockCommodityID)
	require.NoError(t, err)
	require.Equal(t, "6", lots[0].RemainingQuantityValue.String())
}
