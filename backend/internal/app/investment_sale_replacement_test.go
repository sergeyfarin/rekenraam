package app

import (
	"context"
	"fmt"
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
	result, err := f.investmentService.ReplaceSale(ctx, ReplaceInvestmentSaleInput{
		OwnerUserID: f.ownerUserID, TransactionID: original.Transaction.ID,
		Reason: "corrected broker fill",
		Replacement: InvestmentTradeInput{
			TransactionDate: "2026-02-01", CommodityID: f.stockCommodityID,
			HoldingAccountID: f.holdingAccountID, CashAccountID: f.cashAccountID,
			CashCommodityID: f.eurCommodityID, QuantityValue: exact.New(3),
			CashAmountValue: 39000, CashAmountScale: 2, CostBasisMethod: "fifo",
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
		WHERE (SELECT linked_version.transaction_id FROM investment_operation_journal_links journal_link
        JOIN transaction_versions linked_version ON linked_version.id = journal_link.transaction_version_id
        WHERE journal_link.operation_id = operation.id AND journal_link.role = 'primary' ORDER BY journal_link.link_seq LIMIT 1) = ? AND link.role = 'reversal'`, result.Replacement.Transaction.ID).Scan(&linkedInverse))
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
	result, err := f.investmentService.ReplaceSale(ctx, ReplaceInvestmentSaleInput{
		OwnerUserID: f.ownerUserID, TransactionID: sale.Transaction.ID, Reason: "fill was eight shares",
		Replacement: InvestmentTradeInput{
			TransactionDate: "2026-02-01", CommodityID: f.stockCommodityID,
			HoldingAccountID: f.holdingAccountID, CashAccountID: f.cashAccountID,
			CashCommodityID: f.eurCommodityID, QuantityValue: exact.New(8),
			CashAmountValue: 96000, CashAmountScale: 2, CostBasisMethod: "fifo",
		},
	})
	require.NoError(t, err)
	require.Len(t, result.Replacement.Allocations, 1)
	lots, err := f.investmentService.ListLots(ctx, f.holdingAccountID, f.stockCommodityID)
	require.NoError(t, err)
	require.Equal(t, "2", lots[0].RemainingQuantityValue.String())
}

func TestReplaceOlderManualSaleReplaysLaterPositionIntent(t *testing.T) {
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
	input := ReplaceInvestmentSaleInput{
		OwnerUserID: f.ownerUserID, TransactionID: sale.Transaction.ID, Reason: "correct fill",
		Replacement: InvestmentTradeInput{
			TransactionDate: "2026-02-01", CommodityID: f.stockCommodityID,
			HoldingAccountID: f.holdingAccountID, CashAccountID: f.cashAccountID,
			CashCommodityID: f.eurCommodityID, QuantityValue: exact.New(3),
			CashAmountValue: 39000, CashAmountScale: 2, CostBasisMethod: "fifo",
		},
	}
	input.Replacement.QuantityValue = exact.New(11)
	_, err = f.investmentService.ReplaceSaleReconciliationImpact(ctx, input)
	require.ErrorIs(t, err, ErrInvestmentLotsInsufficient)
	_, err = f.investmentService.ReplaceSale(ctx, input)
	require.ErrorIs(t, err, ErrInvestmentLotsInsufficient)
	input.Replacement.QuantityValue = exact.New(3)
	result, err := f.investmentService.ReplaceSale(ctx, input)
	require.NoError(t, err)
	require.Equal(t, "posted", result.Replacement.Transaction.Status)
	var correctionCount int
	require.NoError(t, f.database.QueryRow(`SELECT count(*) FROM investment_operations
		WHERE correction_of_operation_id IS NOT NULL`).Scan(&correctionCount))
	require.Equal(t, 1, correctionCount)
	var activePriceCount int
	require.NoError(t, f.database.QueryRow(`SELECT count(*) FROM price_observations
		WHERE source_transaction_version_id = ? AND voided_at IS NULL`, sale.Transaction.VersionID).Scan(&activePriceCount))
	require.Zero(t, activePriceCount)
	lots, err := f.investmentService.ListLots(ctx, f.holdingAccountID, f.stockCommodityID)
	require.NoError(t, err)
	var total = exact.NewScaledInt()
	for _, lot := range lots {
		total.AddCoefficient(lot.RemainingQuantityValue, lot.RemainingQuantityScale)
	}
	require.Zero(t, total.Cmp(exact.ScaledIntFromInt64(9, 0)))
	check, err := selfCheckOver(t, f.database).RunSelfCheck(ctx, "manual")
	require.NoError(t, err)
	require.Equal(t, SelfCheckPassed, check.Status)
}

func TestReplaceOlderManualSaleReplaysDependentSaleAndRefusesImpossibleCorrection(t *testing.T) {
	ctx := context.Background()
	f := newInvestmentsTestFixture(t)
	_, err := f.investmentService.Buy(ctx, InvestmentTradeInput{
		OwnerUserID: f.ownerUserID, TransactionDate: "2026-01-01",
		CommodityID: f.stockCommodityID, HoldingAccountID: f.holdingAccountID,
		CashAccountID: f.cashAccountID, CashCommodityID: f.eurCommodityID,
		QuantityValue: exact.New(10), CashAmountValue: 100000, CashAmountScale: 2,
	})
	require.NoError(t, err)
	older, err := f.investmentService.Sell(ctx, InvestmentTradeInput{
		OwnerUserID: f.ownerUserID, TransactionDate: "2026-02-01",
		CommodityID: f.stockCommodityID, HoldingAccountID: f.holdingAccountID,
		CashAccountID: f.cashAccountID, CashCommodityID: f.eurCommodityID,
		QuantityValue: exact.New(4), CashAmountValue: 48000, CashAmountScale: 2,
		CostBasisMethod: "fifo",
	})
	require.NoError(t, err)
	later, err := f.investmentService.Sell(ctx, InvestmentTradeInput{
		OwnerUserID: f.ownerUserID, TransactionDate: "2026-03-01",
		CommodityID: f.stockCommodityID, HoldingAccountID: f.holdingAccountID,
		CashAccountID: f.cashAccountID, CashCommodityID: f.eurCommodityID,
		QuantityValue: exact.New(6), CashAmountValue: 72000, CashAmountScale: 2,
		CostBasisMethod: "fifo",
	})
	require.NoError(t, err)
	input := ReplaceInvestmentSaleInput{
		OwnerUserID: f.ownerUserID, TransactionID: older.Transaction.ID,
		Reason: "broker corrected sale quantity",
		Replacement: InvestmentTradeInput{
			TransactionDate: "2026-02-01", CommodityID: f.stockCommodityID,
			HoldingAccountID: f.holdingAccountID, CashAccountID: f.cashAccountID,
			CashCommodityID: f.eurCommodityID, QuantityValue: exact.New(5),
			CashAmountValue: 65000, CashAmountScale: 2, CostBasisMethod: "fifo",
		},
	}
	var auditBefore, revisionsBefore int
	require.NoError(t, f.database.QueryRow(`SELECT count(*) FROM audit_events`).Scan(&auditBefore))
	require.NoError(t, f.database.QueryRow(`SELECT count(*) FROM investment_disposal_revisions`).Scan(&revisionsBefore))
	_, err = f.investmentService.ReplaceSaleReconciliationImpact(ctx, input)
	require.ErrorIs(t, err, ErrInvestmentSaleDependency)
	require.ErrorContains(t, err, fmt.Sprintf("operation %d", saleOperationIDForTest(t, f, later.Transaction.ID)))
	_, err = f.investmentService.ReplaceSale(ctx, input)
	require.ErrorIs(t, err, ErrInvestmentSaleDependency)
	var auditAfter, revisionsAfter int
	require.NoError(t, f.database.QueryRow(`SELECT count(*) FROM audit_events`).Scan(&auditAfter))
	require.NoError(t, f.database.QueryRow(`SELECT count(*) FROM investment_disposal_revisions`).Scan(&revisionsAfter))
	require.Equal(t, auditBefore, auditAfter)
	require.Equal(t, revisionsBefore, revisionsAfter)

	input.Replacement.QuantityValue = exact.New(3)
	input.Replacement.CashAmountValue = 39000
	_, err = f.investmentService.ReplaceSaleReconciliationImpact(ctx, input)
	require.NoError(t, err)
	result, err := f.investmentService.ReplaceSale(ctx, input)
	require.NoError(t, err)
	require.Equal(t, "posted", result.Replacement.Transaction.Status)
	require.Len(t, result.Replacement.Allocations, 1)
	var revisedLater int
	require.NoError(t, f.database.QueryRow(`SELECT count(*) FROM investment_disposal_revisions revision
		JOIN investment_disposal_decisions decision ON decision.id = revision.decision_id
		WHERE decision.transaction_id = ?`, later.Transaction.ID).Scan(&revisedLater))
	require.Equal(t, 1, revisedLater)
	gains, err := f.investmentService.ListRealizedGains(ctx, GainsReportParams{})
	require.NoError(t, err)
	require.Len(t, gains, 2)
	var foundCorrected, foundLater bool
	for _, gain := range gains {
		if gain.TransactionID != nil && *gain.TransactionID == result.Replacement.Transaction.ID {
			foundCorrected = true
			require.Equal(t, "3", gain.QuantityValue.String())
		}
		if gain.TransactionID != nil && *gain.TransactionID == later.Transaction.ID {
			foundLater = true
			require.Equal(t, "6", gain.QuantityValue.String())
		}
	}
	require.True(t, foundCorrected)
	require.True(t, foundLater)
	check, err := selfCheckOver(t, f.database).RunSelfCheck(ctx, "manual")
	require.NoError(t, err)
	require.Equal(t, SelfCheckPassed, check.Status)
}

func TestReplaceOlderManualSaleReplaysEveryBasisMethod(t *testing.T) {
	for _, method := range []string{"fifo", "lifo", "average_cost", "specific_lot"} {
		t.Run(method, func(t *testing.T) {
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
				OwnerUserID: f.ownerUserID, TransactionDate: "2026-02-01",
				CommodityID: f.stockCommodityID, HoldingAccountID: f.holdingAccountID,
				CashAccountID: f.cashAccountID, CashCommodityID: f.eurCommodityID,
				QuantityValue: exact.New(5), CashAmountValue: 75000, CashAmountScale: 2,
				CostBasisMethod: method,
			}
			if method == "specific_lot" {
				saleInput.LotAllocations = []InvestmentLotAllocationInput{{
					LotID: *first.LotID, QuantityValue: exact.New(5),
				}}
			}
			older, err := f.investmentService.Sell(ctx, saleInput)
			require.NoError(t, err)
			saleInput.TransactionDate = "2026-03-01"
			if method == "specific_lot" {
				saleInput.LotAllocations = []InvestmentLotAllocationInput{{
					LotID: *second.LotID, QuantityValue: exact.New(5),
				}}
			}
			later, err := f.investmentService.Sell(ctx, saleInput)
			require.NoError(t, err)
			replacement := saleInput
			replacement.TransactionDate = "2026-02-01"
			replacement.QuantityValue = exact.New(4)
			replacement.CashAmountValue = 64000
			if method == "specific_lot" {
				replacement.LotAllocations = []InvestmentLotAllocationInput{{
					LotID: *first.LotID, QuantityValue: exact.New(4),
				}}
			}
			result, err := f.investmentService.ReplaceSale(ctx, ReplaceInvestmentSaleInput{
				OwnerUserID: f.ownerUserID, TransactionID: older.Transaction.ID,
				Reason: "correct quantity and proceeds", Replacement: replacement,
			})
			require.NoError(t, err)
			require.Len(t, result.Replacement.Allocations, 1)
			var revisionCount int
			require.NoError(t, f.database.QueryRow(`SELECT count(*) FROM investment_disposal_revisions r
				JOIN investment_disposal_decisions d ON d.id = r.decision_id
				WHERE d.transaction_id = ?`, later.Transaction.ID).Scan(&revisionCount))
			require.Equal(t, 1, revisionCount)
			gains, err := f.investmentService.ListRealizedGains(ctx, GainsReportParams{})
			require.NoError(t, err)
			require.Len(t, gains, 2)
			check, err := selfCheckOver(t, f.database).RunSelfCheck(ctx, "manual")
			require.NoError(t, err)
			require.Equal(t, SelfCheckPassed, check.Status)
		})
	}
}

func TestReplaceOlderSpecificLotSaleAfterBuyCorrectionUsesEffectiveLot(t *testing.T) {
	ctx := context.Background()
	f := newInvestmentsTestFixture(t)
	bought, err := f.investmentService.Buy(ctx, InvestmentTradeInput{
		OwnerUserID: f.ownerUserID, TransactionDate: "2026-01-01",
		CommodityID: f.stockCommodityID, HoldingAccountID: f.holdingAccountID,
		CashAccountID: f.cashAccountID, CashCommodityID: f.eurCommodityID,
		QuantityValue: exact.New(10), CashAmountValue: 100000, CashAmountScale: 2,
	})
	require.NoError(t, err)
	saleInput := InvestmentTradeInput{
		OwnerUserID: f.ownerUserID, TransactionDate: "2026-02-01",
		CommodityID: f.stockCommodityID, HoldingAccountID: f.holdingAccountID,
		CashAccountID: f.cashAccountID, CashCommodityID: f.eurCommodityID,
		QuantityValue: exact.New(4), CashAmountValue: 48000, CashAmountScale: 2,
		CostBasisMethod: "specific_lot",
		LotAllocations: []InvestmentLotAllocationInput{{
			LotID: *bought.LotID, QuantityValue: exact.New(4),
		}},
	}
	older, err := f.investmentService.Sell(ctx, saleInput)
	require.NoError(t, err)
	saleInput.TransactionDate = "2026-03-01"
	saleInput.QuantityValue = exact.New(3)
	saleInput.CashAmountValue = 39000
	saleInput.LotAllocations[0].QuantityValue = exact.New(3)
	_, err = f.investmentService.Sell(ctx, saleInput)
	require.NoError(t, err)
	correctedBuy, err := f.investmentService.ReplaceBuy(ctx, ReplaceInvestmentBuyInput{
		OwnerUserID: f.ownerUserID, TransactionID: bought.Transaction.ID,
		Reason: "correct acquisition cost",
		Replacement: InvestmentTradeInput{
			TransactionDate: "2026-01-01", CommodityID: f.stockCommodityID,
			HoldingAccountID: f.holdingAccountID, CashAccountID: f.cashAccountID,
			CashCommodityID: f.eurCommodityID, QuantityValue: exact.New(10),
			CashAmountValue: 120000, CashAmountScale: 2,
		},
	})
	require.NoError(t, err)
	context, err := f.investmentService.TradeCorrectionContext(ctx, f.ownerUserID, older.Transaction.ID)
	require.NoError(t, err)
	require.True(t, context.CanReplaceSale)
	require.Equal(t, *correctedBuy.Replacement.LotID, context.AvailableLots[0].LotID)
	replacement := saleInput
	replacement.TransactionDate = "2026-02-01"
	replacement.LotAllocations = []InvestmentLotAllocationInput{{
		LotID: *correctedBuy.Replacement.LotID, QuantityValue: exact.New(3),
	}}
	result, err := f.investmentService.ReplaceSale(ctx, ReplaceInvestmentSaleInput{
		OwnerUserID: f.ownerUserID, TransactionID: older.Transaction.ID,
		Reason: "correct sale quantity", Replacement: replacement,
	})
	require.NoError(t, err)
	require.Equal(t, *correctedBuy.Replacement.LotID, result.Replacement.Allocations[0].LotID)
	check, err := selfCheckOver(t, f.database).RunSelfCheck(ctx, "manual")
	require.NoError(t, err)
	require.Equal(t, SelfCheckPassed, check.Status)
}

func TestReplaceSaleAfterLaterSaleReversalUsesHistoricalReplay(t *testing.T) {
	ctx := context.Background()
	f := newInvestmentsTestFixture(t)
	_, err := f.investmentService.Buy(ctx, InvestmentTradeInput{
		OwnerUserID: f.ownerUserID, TransactionDate: "2026-01-01",
		CommodityID: f.stockCommodityID, HoldingAccountID: f.holdingAccountID,
		CashAccountID: f.cashAccountID, CashCommodityID: f.eurCommodityID,
		QuantityValue: exact.New(10), CashAmountValue: 100000, CashAmountScale: 2,
	})
	require.NoError(t, err)
	older, err := f.investmentService.Sell(ctx, InvestmentTradeInput{
		OwnerUserID: f.ownerUserID, TransactionDate: "2026-02-01",
		CommodityID: f.stockCommodityID, HoldingAccountID: f.holdingAccountID,
		CashAccountID: f.cashAccountID, CashCommodityID: f.eurCommodityID,
		QuantityValue: exact.New(4), CashAmountValue: 48000, CashAmountScale: 2,
	})
	require.NoError(t, err)
	later, err := f.investmentService.Sell(ctx, InvestmentTradeInput{
		OwnerUserID: f.ownerUserID, TransactionDate: "2026-03-01",
		CommodityID: f.stockCommodityID, HoldingAccountID: f.holdingAccountID,
		CashAccountID: f.cashAccountID, CashCommodityID: f.eurCommodityID,
		QuantityValue: exact.New(3), CashAmountValue: 39000, CashAmountScale: 2,
	})
	require.NoError(t, err)
	_, err = f.investmentService.ReverseSale(ctx, ReverseInvestmentSaleInput{
		OwnerUserID: f.ownerUserID, TransactionID: later.Transaction.ID,
		Reason: "later fill was canceled",
	})
	require.NoError(t, err)
	result, err := f.investmentService.ReplaceSale(ctx, ReplaceInvestmentSaleInput{
		OwnerUserID: f.ownerUserID, TransactionID: older.Transaction.ID,
		Reason: "correct earlier quantity",
		Replacement: InvestmentTradeInput{
			TransactionDate: "2026-02-01", CommodityID: f.stockCommodityID,
			HoldingAccountID: f.holdingAccountID, CashAccountID: f.cashAccountID,
			CashCommodityID: f.eurCommodityID, QuantityValue: exact.New(3),
			CashAmountValue: 39000, CashAmountScale: 2, CostBasisMethod: "fifo",
		},
	})
	require.NoError(t, err)
	require.Equal(t, "posted", result.Replacement.Transaction.Status)
	check, err := selfCheckOver(t, f.database).RunSelfCheck(ctx, "manual")
	require.NoError(t, err)
	require.Equal(t, SelfCheckPassed, check.Status)
}

func TestReplaceOlderManualSaleRequiresReconciliationOverrideAtomically(t *testing.T) {
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
	_, err = f.investmentService.Sell(ctx, InvestmentTradeInput{
		OwnerUserID: f.ownerUserID, TransactionDate: "2026-04-01",
		CommodityID: f.stockCommodityID, HoldingAccountID: f.holdingAccountID,
		CashAccountID: f.cashAccountID, CashCommodityID: f.eurCommodityID,
		QuantityValue: exact.New(1), CashAmountValue: 13000, CashAmountScale: 2,
	})
	require.NoError(t, err)
	input := ReplaceInvestmentSaleInput{
		OwnerUserID: f.ownerUserID, TransactionID: sale.Transaction.ID, Reason: "correct fill",
		Replacement: InvestmentTradeInput{
			TransactionDate: "2026-02-01", CommodityID: f.stockCommodityID,
			HoldingAccountID: f.holdingAccountID, CashAccountID: f.cashAccountID,
			CashCommodityID: f.eurCommodityID, QuantityValue: exact.New(3),
			CashAmountValue: 39000, CashAmountScale: 2, CostBasisMethod: "fifo",
		},
	}
	impact, err := f.investmentService.ReplaceSaleReconciliationImpact(ctx, input)
	require.NoError(t, err)
	require.Len(t, impact.AffectedCheckpoints, 1)
	require.Equal(t, checkpointID, impact.AffectedCheckpoints[0].CheckpointID)
	_, err = f.investmentService.ReplaceSale(ctx, input)
	require.ErrorIs(t, err, ErrReconciliationOverrideRequired)
	var correctionCount int
	require.NoError(t, f.database.QueryRow(`SELECT count(*) FROM investment_operations
		WHERE correction_of_operation_id IS NOT NULL`).Scan(&correctionCount))
	require.Zero(t, correctionCount)
	require.Equal(t, []int64{checkpointID}, activeCheckpointIDs(t, f))
	input.ReconciliationOverride = true
	result, err := f.investmentService.ReplaceSale(ctx, input)
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
	_, err = f.investmentService.ReplaceSale(ctx, ReplaceInvestmentSaleInput{
		OwnerUserID: f.ownerUserID, TransactionID: sale.Transaction.ID,
		Reason: "broker fill exceeds balance",
		Replacement: InvestmentTradeInput{
			TransactionDate: "2026-02-01", CommodityID: f.stockCommodityID,
			HoldingAccountID: f.holdingAccountID, CashAccountID: f.cashAccountID,
			CashCommodityID: f.eurCommodityID, QuantityValue: exact.New(11),
			CashAmountValue: 132000, CashAmountScale: 2, CostBasisMethod: "fifo",
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

func TestReplaceManualSaleRequiresExplicitEconomicElections(t *testing.T) {
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
	base := InvestmentTradeInput{
		TransactionDate: "2026-02-01", CommodityID: f.stockCommodityID,
		HoldingAccountID: f.holdingAccountID, CashAccountID: f.cashAccountID,
		CashCommodityID: f.eurCommodityID, QuantityValue: exact.New(3),
		CashAmountValue: 39000, CashAmountScale: 2,
	}
	for _, test := range []struct {
		name string
		edit func(*InvestmentTradeInput)
		want string
	}{
		{name: "missing_cost_basis_method", edit: func(*InvestmentTradeInput) {}, want: "cost-basis method is required"},
		{name: "missing_charge_treatment", edit: func(trade *InvestmentTradeInput) {
			trade.CostBasisMethod = "fifo"
			trade.Charges = []InvestmentTradeChargeInput{{Kind: "commission", AmountValue: -200, AmountScale: 2, CommodityID: f.eurCommodityID}}
		}, want: "charge 1 treatment is required"},
	} {
		t.Run(test.name, func(t *testing.T) {
			replacement := base
			test.edit(&replacement)
			input := ReplaceInvestmentSaleInput{OwnerUserID: f.ownerUserID,
				TransactionID: sale.Transaction.ID, Reason: "correct fill", Replacement: replacement}
			_, err := f.investmentService.ReplaceSaleReconciliationImpact(ctx, input)
			require.ErrorContains(t, err, test.want)
			_, err = f.investmentService.ReplaceSale(ctx, input)
			require.ErrorContains(t, err, test.want)
		})
	}
	var corrections int
	require.NoError(t, f.database.QueryRow(`SELECT count(*) FROM investment_operations
		WHERE correction_of_operation_id IS NOT NULL`).Scan(&corrections))
	require.Zero(t, corrections)
}
