package app

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"rekenraam/backend/internal/exact"
)

func TestReplaceOldManualBuyReplaysDependentSameDaySale(t *testing.T) {
	ctx := context.Background()
	f := newInvestmentsTestFixture(t)
	original, err := f.investmentService.Buy(ctx, InvestmentTradeInput{
		OwnerUserID: f.ownerUserID, TransactionDate: "2026-01-01",
		CommodityID: f.stockCommodityID, HoldingAccountID: f.holdingAccountID,
		CashAccountID: f.cashAccountID, CashCommodityID: f.eurCommodityID,
		QuantityValue: exact.New(10), CashAmountValue: 100000, CashAmountScale: 2,
	})
	require.NoError(t, err)
	_, err = f.investmentService.Sell(ctx, InvestmentTradeInput{
		OwnerUserID: f.ownerUserID, TransactionDate: "2026-01-01",
		CommodityID: f.stockCommodityID, HoldingAccountID: f.holdingAccountID,
		CashAccountID: f.cashAccountID, CashCommodityID: f.eurCommodityID,
		QuantityValue: exact.New(4), CashAmountValue: 48000, CashAmountScale: 2,
		CostBasisMethod: "fifo",
	})
	require.NoError(t, err)
	result, err := f.investmentService.ReplaceBuy(ctx, ReplaceInvestmentBuyInput{
		OwnerUserID: f.ownerUserID, TransactionID: original.Transaction.ID,
		Reason: "correct broker cost",
		Replacement: InvestmentTradeInput{
			TransactionDate: "2026-01-01", CommodityID: f.stockCommodityID,
			HoldingAccountID: f.holdingAccountID, CashAccountID: f.cashAccountID,
			CashCommodityID: f.eurCommodityID, QuantityValue: exact.New(10),
			CashAmountValue: 200000, CashAmountScale: 2,
		},
	})
	require.NoError(t, err)
	require.Equal(t, "posted", result.Inverse.Status)
	require.Equal(t, "posted", result.Replacement.Transaction.Status)
	var inverseAudit, replacementAudit, retiredAudit int64
	require.NoError(t, f.database.QueryRow(`SELECT created_audit_event_id FROM transactions WHERE id = ?`,
		result.Inverse.ID).Scan(&inverseAudit))
	require.NoError(t, f.database.QueryRow(`SELECT created_audit_event_id FROM transactions WHERE id = ?`,
		result.Replacement.Transaction.ID).Scan(&replacementAudit))
	require.Equal(t, inverseAudit, replacementAudit)
	require.NoError(t, f.database.QueryRow(`SELECT voided_audit_event_id FROM price_observations
		WHERE source_transaction_version_id = ?`, original.Transaction.VersionID).Scan(&retiredAudit))
	require.Equal(t, inverseAudit, retiredAudit)
	chain, err := f.investmentService.CorrectionChain(ctx, f.ownerUserID, result.Inverse.ID)
	require.NoError(t, err)
	require.Equal(t, result.Replacement.Transaction.ID, *chain.EffectiveTransactionID)
	lots, err := f.investmentService.ListLots(ctx, f.holdingAccountID, f.stockCommodityID)
	require.NoError(t, err)
	require.Len(t, lots, 2)
	var activeLotCount int
	require.NoError(t, f.database.QueryRow(`SELECT count(*) FROM investment_lots
		WHERE book_id = 1 AND account_id = ? AND commodity_id = ?
		AND status = 'open' AND remaining_quantity_value = '6'`,
		f.holdingAccountID, f.stockCommodityID).Scan(&activeLotCount))
	require.Equal(t, 1, activeLotCount)
	gains, err := f.investmentService.ListRealizedGains(ctx, GainsReportParams{})
	require.NoError(t, err)
	require.Len(t, gains, 1)
	require.Zero(t, exact.ScaledIntFromInt64(gains[0].DisposedBasisValue, gains[0].DisposedBasisScale).
		Cmp(exact.ScaledIntFromInt64(-80000, 2)))
	require.Zero(t, exact.ScaledIntFromInt64(gains[0].RealizedGainValue, gains[0].RealizedGainScale).
		Cmp(exact.ScaledIntFromInt64(-32000, 2)))
	check, err := selfCheckOver(t, f.database).RunSelfCheck(ctx, "manual")
	require.NoError(t, err)
	require.Equal(t, SelfCheckPassed, check.Status)
}

func TestReplaceOldBuyRollsBackWhenDependentSaleNeedsMoreShares(t *testing.T) {
	ctx := context.Background()
	f := newInvestmentsTestFixture(t)
	original, err := f.investmentService.Buy(ctx, InvestmentTradeInput{
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
		QuantityValue: exact.New(8), CashAmountValue: 96000, CashAmountScale: 2,
	})
	require.NoError(t, err)
	_, err = f.investmentService.ReplaceBuy(ctx, ReplaceInvestmentBuyInput{
		OwnerUserID: f.ownerUserID, TransactionID: original.Transaction.ID,
		Reason: "broker recorded five shares",
		Replacement: InvestmentTradeInput{
			TransactionDate: "2026-01-01", CommodityID: f.stockCommodityID,
			HoldingAccountID: f.holdingAccountID, CashAccountID: f.cashAccountID,
			CashCommodityID: f.eurCommodityID, QuantityValue: exact.New(5),
			CashAmountValue: 50000, CashAmountScale: 2,
		},
	})
	require.ErrorIs(t, err, ErrInvestmentBuyDependency)
	require.ErrorContains(t, err, "operation "+fmt.Sprint(saleOperationIDForTest(t, f, sale.Transaction.ID)))
	var transactionCount, correctionCount, activePriceCount int
	require.NoError(t, f.database.QueryRow(`SELECT count(*) FROM transactions`).Scan(&transactionCount))
	require.Equal(t, 2, transactionCount)
	require.NoError(t, f.database.QueryRow(`SELECT count(*) FROM investment_operations
		WHERE correction_of_operation_id IS NOT NULL`).Scan(&correctionCount))
	require.Zero(t, correctionCount)
	require.NoError(t, f.database.QueryRow(`SELECT count(*) FROM price_observations
		WHERE source_transaction_version_id = ? AND voided_at IS NULL`,
		original.Transaction.VersionID).Scan(&activePriceCount))
	require.Equal(t, 1, activePriceCount)
	lots, err := f.investmentService.ListLots(ctx, f.holdingAccountID, f.stockCommodityID)
	require.NoError(t, err)
	require.Len(t, lots, 1)
	require.Equal(t, "2", lots[0].RemainingQuantityValue.String())
}

func TestReplaceOldBuyRequiresReconciliationOverrideAtomically(t *testing.T) {
	ctx := context.Background()
	f := newInvestmentsTestFixture(t)
	original, err := f.investmentService.Buy(ctx, InvestmentTradeInput{
		OwnerUserID: f.ownerUserID, TransactionDate: "2026-01-01",
		CommodityID: f.stockCommodityID, HoldingAccountID: f.holdingAccountID,
		CashAccountID: f.cashAccountID, CashCommodityID: f.eurCommodityID,
		QuantityValue: exact.New(10), CashAmountValue: 100000, CashAmountScale: 2,
	})
	require.NoError(t, err)
	_, err = f.investmentService.Sell(ctx, InvestmentTradeInput{
		OwnerUserID: f.ownerUserID, TransactionDate: "2026-02-01",
		CommodityID: f.stockCommodityID, HoldingAccountID: f.holdingAccountID,
		CashAccountID: f.cashAccountID, CashCommodityID: f.eurCommodityID,
		QuantityValue: exact.New(4), CashAmountValue: 48000, CashAmountScale: 2,
	})
	require.NoError(t, err)
	checkpointID := reconcileCash(t, f, "2026-03-01", -520)
	input := ReplaceInvestmentBuyInput{
		OwnerUserID: f.ownerUserID, TransactionID: original.Transaction.ID,
		Reason: "correct acquisition cost",
		Replacement: InvestmentTradeInput{
			TransactionDate: "2026-01-01", CommodityID: f.stockCommodityID,
			HoldingAccountID: f.holdingAccountID, CashAccountID: f.cashAccountID,
			CashCommodityID: f.eurCommodityID, QuantityValue: exact.New(10),
			CashAmountValue: 200000, CashAmountScale: 2,
		},
	}
	impact, err := f.investmentService.ReplaceBuyReconciliationImpact(ctx, input)
	require.NoError(t, err)
	require.Len(t, impact.AffectedCheckpoints, 1)
	require.Equal(t, checkpointID, impact.AffectedCheckpoints[0].CheckpointID)
	_, err = f.investmentService.ReplaceBuy(ctx, input)
	require.ErrorIs(t, err, ErrReconciliationOverrideRequired)
	var correctionCount int
	require.NoError(t, f.database.QueryRow(`SELECT count(*) FROM investment_operations
		WHERE correction_of_operation_id IS NOT NULL`).Scan(&correctionCount))
	require.Zero(t, correctionCount)
	require.Equal(t, []int64{checkpointID}, activeCheckpointIDs(t, f))
	input.ReconciliationOverride = true
	result, err := f.investmentService.ReplaceBuy(ctx, input)
	require.NoError(t, err)
	require.Empty(t, activeCheckpointIDs(t, f))
	var checkpointAudit, replacementAudit int64
	require.NoError(t, f.database.QueryRow(`SELECT invalidated_audit_event_id FROM reconciliation_checkpoints
		WHERE id = ?`, checkpointID).Scan(&checkpointAudit))
	require.NoError(t, f.database.QueryRow(`SELECT created_audit_event_id FROM transactions
		WHERE id = ?`, result.Replacement.Transaction.ID).Scan(&replacementAudit))
	require.Equal(t, replacementAudit, checkpointAudit)
}

func TestReplaceImportedBuyRequiresSourceAwareCorrection(t *testing.T) {
	ctx := context.Background()
	f := newInvestmentsTestFixture(t)
	original, err := f.investmentService.Buy(ctx, InvestmentTradeInput{
		OwnerUserID: f.ownerUserID, OriginType: "import", TransactionDate: "2026-01-01",
		CommodityID: f.stockCommodityID, HoldingAccountID: f.holdingAccountID,
		CashAccountID: f.cashAccountID, CashCommodityID: f.eurCommodityID,
		QuantityValue: exact.New(10), CashAmountValue: 100000, CashAmountScale: 2,
	})
	require.NoError(t, err)
	_, err = f.investmentService.ReplaceBuy(ctx, ReplaceInvestmentBuyInput{
		OwnerUserID: f.ownerUserID, TransactionID: original.Transaction.ID,
		Reason: "source corrected acquisition",
	})
	require.ErrorIs(t, err, ErrInvestmentImportedBuy)
}

func TestReplaceBuyRequiresExplicitChargeTreatment(t *testing.T) {
	ctx := context.Background()
	f := newInvestmentsTestFixture(t)
	original, err := f.investmentService.Buy(ctx, InvestmentTradeInput{
		OwnerUserID: f.ownerUserID, TransactionDate: "2026-01-01",
		CommodityID: f.stockCommodityID, HoldingAccountID: f.holdingAccountID,
		CashAccountID: f.cashAccountID, CashCommodityID: f.eurCommodityID,
		QuantityValue: exact.New(10), CashAmountValue: 100000, CashAmountScale: 2,
	})
	require.NoError(t, err)
	input := ReplaceInvestmentBuyInput{
		OwnerUserID: f.ownerUserID, TransactionID: original.Transaction.ID,
		Reason: "correct commission",
		Replacement: InvestmentTradeInput{
			TransactionDate: "2026-01-01", CommodityID: f.stockCommodityID,
			HoldingAccountID: f.holdingAccountID, CashAccountID: f.cashAccountID,
			CashCommodityID: f.eurCommodityID, QuantityValue: exact.New(10),
			CashAmountValue: 102000, CashAmountScale: 2,
			Charges: []InvestmentTradeChargeInput{{
				Kind: "commission", AmountValue: -2000, AmountScale: 2,
				CommodityID: f.eurCommodityID,
			}},
		},
	}
	_, err = f.investmentService.ReplaceBuyReconciliationImpact(ctx, input)
	require.ErrorContains(t, err, "charge 1 treatment is required")
	_, err = f.investmentService.ReplaceBuy(ctx, input)
	require.ErrorContains(t, err, "charge 1 treatment is required")
}

func TestReplaceBuyReplacesApproximatePriceWithGrossPrice(t *testing.T) {
	ctx := context.Background()
	f := newInvestmentsTestFixture(t)
	original, err := f.investmentService.Buy(ctx, InvestmentTradeInput{
		OwnerUserID: f.ownerUserID, TransactionDate: "2026-01-01",
		CommodityID: f.stockCommodityID, HoldingAccountID: f.holdingAccountID,
		CashAccountID: f.cashAccountID, CashCommodityID: f.eurCommodityID,
		QuantityValue: exact.New(1), CashAmountValue: 10200, CashAmountScale: 2,
	})
	require.NoError(t, err)
	result, err := f.investmentService.ReplaceBuy(ctx, ReplaceInvestmentBuyInput{
		OwnerUserID: f.ownerUserID, TransactionID: original.Transaction.ID,
		Reason: "add broker gross and commission",
		Replacement: InvestmentTradeInput{
			TransactionDate: "2026-01-01", CommodityID: f.stockCommodityID,
			HoldingAccountID: f.holdingAccountID, CashAccountID: f.cashAccountID,
			CashCommodityID: f.eurCommodityID, QuantityValue: exact.New(1),
			GrossAmountValue: tradeMoney(-10000), GrossAmountScale: 2,
			NetSettlementValue: tradeMoney(-10200), NetSettlementScale: 2,
			Charges: []InvestmentTradeChargeInput{{Kind: "commission", AmountValue: -200,
				AmountScale: 2, CommodityID: f.eurCommodityID, Treatment: "clearing_included"}},
		},
	})
	require.NoError(t, err)
	var originalApproximate, replacementApproximate int
	var retiredAudit, replacementAudit int64
	var grossPrice int64
	require.NoError(t, f.database.QueryRow(`SELECT is_approximate, voided_audit_event_id FROM price_observations
		WHERE source_transaction_version_id = ?`, original.Transaction.VersionID).Scan(&originalApproximate, &retiredAudit))
	require.NoError(t, f.database.QueryRow(`SELECT is_approximate, price_value, created_audit_event_id FROM price_observations
		WHERE source_transaction_version_id = ? AND voided_at IS NULL`, result.Replacement.Transaction.VersionID).
		Scan(&replacementApproximate, &grossPrice, &replacementAudit))
	require.Equal(t, 1, originalApproximate)
	require.Zero(t, replacementApproximate)
	require.EqualValues(t, 10000000000, grossPrice)
	require.Equal(t, retiredAudit, replacementAudit)
	check, err := selfCheckOver(t, f.database).RunSelfCheck(ctx, "manual")
	require.NoError(t, err)
	require.Equal(t, SelfCheckPassed, check.Status)
}

func TestReplaceBuyTwiceKeepsOriginalSpecificLotElection(t *testing.T) {
	ctx := context.Background()
	f := newInvestmentsTestFixture(t)
	first, err := f.investmentService.Buy(ctx, InvestmentTradeInput{
		OwnerUserID: f.ownerUserID, TransactionDate: "2026-01-01",
		CommodityID: f.stockCommodityID, HoldingAccountID: f.holdingAccountID,
		CashAccountID: f.cashAccountID, CashCommodityID: f.eurCommodityID,
		QuantityValue: exact.New(10), CashAmountValue: 100000, CashAmountScale: 2,
	})
	require.NoError(t, err)
	_, err = f.investmentService.Sell(ctx, InvestmentTradeInput{
		OwnerUserID: f.ownerUserID, TransactionDate: "2026-02-01",
		CommodityID: f.stockCommodityID, HoldingAccountID: f.holdingAccountID,
		CashAccountID: f.cashAccountID, CashCommodityID: f.eurCommodityID,
		QuantityValue: exact.New(4), CashAmountValue: 48000, CashAmountScale: 2,
		CostBasisMethod: "specific_lot",
		LotAllocations:  []InvestmentLotAllocationInput{{LotID: *first.LotID, QuantityValue: exact.New(4)}},
	})
	require.NoError(t, err)
	replace := func(transactionID int64, basis int64) ReplaceInvestmentBuyResult {
		t.Helper()
		result, err := f.investmentService.ReplaceBuy(ctx, ReplaceInvestmentBuyInput{
			OwnerUserID: f.ownerUserID, TransactionID: transactionID,
			Reason: "correct purchase amount",
			Replacement: InvestmentTradeInput{
				TransactionDate: "2026-01-01", CommodityID: f.stockCommodityID,
				HoldingAccountID: f.holdingAccountID, CashAccountID: f.cashAccountID,
				CashCommodityID: f.eurCommodityID, QuantityValue: exact.New(10),
				CashAmountValue: basis, CashAmountScale: 2,
			},
		})
		require.NoError(t, err)
		return result
	}
	second := replace(first.Transaction.ID, 200000)
	third := replace(second.Replacement.Transaction.ID, 300000)
	gains, err := f.investmentService.ListRealizedGains(ctx, GainsReportParams{})
	require.NoError(t, err)
	require.Len(t, gains, 1)
	require.Zero(t, exact.ScaledIntFromInt64(gains[0].DisposedBasisValue, gains[0].DisposedBasisScale).
		Cmp(exact.ScaledIntFromInt64(-120000, 2)))
	chain, err := f.investmentService.CorrectionChain(ctx, f.ownerUserID, first.Transaction.ID)
	require.NoError(t, err)
	require.Len(t, chain.Operations, 3)
	require.Equal(t, third.Replacement.Transaction.ID, *chain.EffectiveTransactionID)
	check, err := selfCheckOver(t, f.database).RunSelfCheck(ctx, "manual")
	require.NoError(t, err)
	require.Equal(t, SelfCheckPassed, check.Status)
}

func TestReplaceOldBuyRecalculatesDependentSaleUnderEveryBasisMethod(t *testing.T) {
	for _, test := range []struct {
		method string
		basis  int64
	}{
		{method: "fifo", basis: 300000},
		{method: "lifo", basis: 200000},
		{method: "average_cost", basis: 250000},
		{method: "specific_lot", basis: 300000},
	} {
		t.Run(test.method, func(t *testing.T) {
			ctx := context.Background()
			f := newInvestmentsTestFixture(t)
			first, err := f.investmentService.Buy(ctx, InvestmentTradeInput{
				OwnerUserID: f.ownerUserID, TransactionDate: "2026-01-01",
				CommodityID: f.stockCommodityID, HoldingAccountID: f.holdingAccountID,
				CashAccountID: f.cashAccountID, CashCommodityID: f.eurCommodityID,
				QuantityValue: exact.New(10), CashAmountValue: 100000, CashAmountScale: 2,
			})
			require.NoError(t, err)
			second, err := f.investmentService.Buy(ctx, InvestmentTradeInput{
				OwnerUserID: f.ownerUserID, TransactionDate: "2026-01-02",
				CommodityID: f.stockCommodityID, HoldingAccountID: f.holdingAccountID,
				CashAccountID: f.cashAccountID, CashCommodityID: f.eurCommodityID,
				QuantityValue: exact.New(10), CashAmountValue: 200000, CashAmountScale: 2,
			})
			require.NoError(t, err)
			saleInput := InvestmentTradeInput{
				OwnerUserID: f.ownerUserID, TransactionDate: "2026-01-03",
				CommodityID: f.stockCommodityID, HoldingAccountID: f.holdingAccountID,
				CashAccountID: f.cashAccountID, CashCommodityID: f.eurCommodityID,
				QuantityValue: exact.New(5), CashAmountValue: 150000, CashAmountScale: 2,
				CostBasisMethod: test.method,
			}
			if test.method == "specific_lot" {
				saleInput.LotAllocations = []InvestmentLotAllocationInput{{LotID: *first.LotID, QuantityValue: exact.New(5)}}
			}
			sale, err := f.investmentService.Sell(ctx, saleInput)
			require.NoError(t, err)
			saleInput.TransactionDate = "2026-01-04"
			_, err = f.investmentService.Sell(ctx, saleInput)
			require.NoError(t, err)
			replaced, err := f.investmentService.ReplaceBuy(ctx, ReplaceInvestmentBuyInput{
				OwnerUserID: f.ownerUserID, TransactionID: first.Transaction.ID,
				Reason: "correct purchase cost",
				Replacement: InvestmentTradeInput{
					TransactionDate: "2026-01-01", CommodityID: f.stockCommodityID,
					HoldingAccountID: f.holdingAccountID, CashAccountID: f.cashAccountID,
					CashCommodityID: f.eurCommodityID, QuantityValue: exact.New(10),
					CashAmountValue: 300000, CashAmountScale: 2,
				},
			})
			require.NoError(t, err)
			gains, err := f.investmentService.ListRealizedGains(ctx, GainsReportParams{})
			require.NoError(t, err)
			require.Len(t, gains, 2)
			totalBasis := exact.NewScaledInt()
			for _, gain := range gains {
				totalBasis.AddInt64(gain.DisposedBasisValue, gain.DisposedBasisScale)
			}
			require.Zero(t, totalBasis.Cmp(exact.ScaledIntFromInt64(-test.basis, 2)))
			var originalLotID, effectiveLotID int64
			require.NoError(t, f.database.QueryRow(`SELECT lot_id FROM investment_disposal_allocations
				WHERE decision_id = ? ORDER BY allocation_seq LIMIT 1`, *sale.DisposalDecision.ID).Scan(&originalLotID))
			require.NoError(t, f.database.QueryRow(`SELECT allocation.lot_id
				FROM investment_disposal_revision_allocations allocation
				JOIN investment_disposal_revisions revision ON revision.id = allocation.revision_id
				WHERE revision.decision_id = ? ORDER BY revision.revision_seq DESC, allocation.allocation_seq LIMIT 1`,
				*sale.DisposalDecision.ID).Scan(&effectiveLotID))
			if test.method == "fifo" || test.method == "specific_lot" {
				require.Equal(t, *first.LotID, originalLotID)
				require.Equal(t, *replaced.Replacement.LotID, effectiveLotID)
			} else if test.method == "lifo" {
				require.Equal(t, *second.LotID, effectiveLotID)
			}
			check, err := selfCheckOver(t, f.database).RunSelfCheck(ctx, "manual")
			require.NoError(t, err)
			require.Equal(t, SelfCheckPassed, check.Status)
		})
	}
}
