package app

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"rekenraam/backend/internal/exact"
)

func TestTradeCorrectionContextReadsImmutableSourceFacts(t *testing.T) {
	ctx := context.Background()
	f := newInvestmentsTestFixture(t)
	bought, err := f.investmentService.Buy(ctx, InvestmentTradeInput{
		OwnerUserID: f.ownerUserID, TransactionDate: "2026-01-01",
		CommodityID: f.stockCommodityID, HoldingAccountID: f.holdingAccountID,
		CashAccountID: f.cashAccountID, CashCommodityID: f.eurCommodityID,
		QuantityValue: exact.New(10), GrossAmountValue: tradeMoney(-100000), GrossAmountScale: 2,
		NetSettlementValue: tradeMoney(-100200), NetSettlementScale: 2,
		SettlementDate: "2026-01-03", Memo: "original broker fill", Charges: []InvestmentTradeChargeInput{{
			Kind: "commission", AmountValue: -200, AmountScale: 2,
			CommodityID: f.eurCommodityID, Treatment: "clearing_included",
		}},
	})
	require.NoError(t, err)
	before, err := f.investmentService.TradeCorrectionContext(ctx, f.ownerUserID, bought.Transaction.ID)
	require.NoError(t, err)
	require.Equal(t, "buy", before.OperationKind)
	require.Equal(t, f.holdingAccountID, before.HoldingAccountID)
	require.Equal(t, f.cashAccountID, before.CashAccountID)
	require.Equal(t, "10", before.QuantityValue)
	require.Equal(t, "-100200", before.NetValue)
	require.Equal(t, "2026-01-03", before.SettlementDate)
	require.Equal(t, "original broker fill", before.Memo)
	require.Equal(t, "-100000", *before.GrossValue)
	require.Len(t, before.Charges, 1)
	require.Equal(t, "clearing_included", before.Charges[0].Treatment)
	require.False(t, before.AlreadyCorrected)
	_, err = acknowledgedReplaceBuy(ctx, f.investmentService, ReplaceInvestmentBuyInput{
		OwnerUserID: f.ownerUserID, TransactionID: bought.Transaction.ID,
		Reason: "correct broker cost",
		Replacement: InvestmentTradeInput{
			TransactionDate: "2026-01-01", CommodityID: f.stockCommodityID,
			HoldingAccountID: f.holdingAccountID, CashAccountID: f.cashAccountID,
			CashCommodityID: f.eurCommodityID, QuantityValue: exact.New(10),
			CashAmountValue: 200000, CashAmountScale: 2,
		},
	})
	require.NoError(t, err)
	after, err := f.investmentService.TradeCorrectionContext(ctx, f.ownerUserID, bought.Transaction.ID)
	require.NoError(t, err)
	require.True(t, after.AlreadyCorrected)
	require.Equal(t, before.NetValue, after.NetValue)
	require.Equal(t, before.Charges, after.Charges)
}

func TestTradeCorrectionContextIncludesSaleElection(t *testing.T) {
	ctx := context.Background()
	f := newInvestmentsTestFixture(t)
	bought, err := f.investmentService.Buy(ctx, InvestmentTradeInput{
		OwnerUserID: f.ownerUserID, TransactionDate: "2026-01-01",
		CommodityID: f.stockCommodityID, HoldingAccountID: f.holdingAccountID,
		CashAccountID: f.cashAccountID, CashCommodityID: f.eurCommodityID,
		QuantityValue: exact.New(10), CashAmountValue: 100000, CashAmountScale: 2,
	})
	require.NoError(t, err)
	sold, err := f.investmentService.Sell(ctx, InvestmentTradeInput{
		OwnerUserID: f.ownerUserID, TransactionDate: "2026-02-01",
		CommodityID: f.stockCommodityID, HoldingAccountID: f.holdingAccountID,
		CashAccountID: f.cashAccountID, CashCommodityID: f.eurCommodityID,
		QuantityValue: exact.New(4), CashAmountValue: 48000, CashAmountScale: 2,
		CostBasisMethod: "specific_lot",
		LotAllocations:  []InvestmentLotAllocationInput{{LotID: *bought.LotID, QuantityValue: exact.New(4)}},
	})
	require.NoError(t, err)
	var eventsBefore, auditBefore int
	require.NoError(t, f.database.QueryRow(`SELECT count(*) FROM investment_lot_events`).Scan(&eventsBefore))
	require.NoError(t, f.database.QueryRow(`SELECT count(*) FROM audit_events`).Scan(&auditBefore))
	context, err := f.investmentService.TradeCorrectionContext(ctx, f.ownerUserID, sold.Transaction.ID)
	require.NoError(t, err)
	var eventsAfter, auditAfter int
	require.NoError(t, f.database.QueryRow(`SELECT count(*) FROM investment_lot_events`).Scan(&eventsAfter))
	require.NoError(t, f.database.QueryRow(`SELECT count(*) FROM audit_events`).Scan(&auditAfter))
	require.Equal(t, eventsBefore, eventsAfter)
	require.Equal(t, auditBefore, auditAfter)
	require.Equal(t, "sell", context.OperationKind)
	require.Equal(t, "specific_lot", context.CostBasisMethod)
	require.Len(t, context.ElectedLots, 1)
	require.Equal(t, *bought.LotID, context.ElectedLots[0].LotID)
	require.Equal(t, "4", context.ElectedLots[0].QuantityValue)
	require.True(t, context.CanReplaceSale)
	require.Len(t, context.AvailableLots, 1)
	require.Equal(t, *bought.LotID, context.AvailableLots[0].LotID)
	require.Equal(t, "10", context.AvailableLots[0].QuantityValue)
	require.Len(t, context.EffectiveElectedLots, 1)
	require.Equal(t, *bought.LotID, context.EffectiveElectedLots[0].LotID)
	require.Equal(t, "48000", context.NetValue)
	require.Nil(t, context.GrossValue)
	require.Empty(t, context.Charges)
	replaced, err := acknowledgedReplaceBuy(ctx, f.investmentService, ReplaceInvestmentBuyInput{
		OwnerUserID: f.ownerUserID, TransactionID: bought.Transaction.ID,
		Reason: "correct original basis",
		Replacement: InvestmentTradeInput{
			TransactionDate: "2026-01-01", CommodityID: f.stockCommodityID,
			HoldingAccountID: f.holdingAccountID, CashAccountID: f.cashAccountID,
			CashCommodityID: f.eurCommodityID, QuantityValue: exact.New(10),
			CashAmountValue: 120000, CashAmountScale: 2,
		},
	})
	require.NoError(t, err)
	context, err = f.investmentService.TradeCorrectionContext(ctx, f.ownerUserID, sold.Transaction.ID)
	require.NoError(t, err)
	require.Equal(t, *bought.LotID, context.ElectedLots[0].LotID)
	require.Equal(t, *replaced.Replacement.LotID, context.EffectiveElectedLots[0].LotID)
	require.Equal(t, *replaced.Replacement.LotID, context.AvailableLots[0].LotID)
}

func TestTradeCorrectionContextPreservesOlderSalePrefixForReplacement(t *testing.T) {
	ctx := context.Background()
	f := newInvestmentsTestFixture(t)
	bought, err := f.investmentService.Buy(ctx, InvestmentTradeInput{
		OwnerUserID: f.ownerUserID, TransactionDate: "2026-01-01",
		CommodityID: f.stockCommodityID, HoldingAccountID: f.holdingAccountID,
		CashAccountID: f.cashAccountID, CashCommodityID: f.eurCommodityID,
		QuantityValue: exact.New(10), CashAmountValue: 100000, CashAmountScale: 2,
	})
	require.NoError(t, err)
	sold, err := f.investmentService.Sell(ctx, InvestmentTradeInput{
		OwnerUserID: f.ownerUserID, TransactionDate: "2026-02-01",
		CommodityID: f.stockCommodityID, HoldingAccountID: f.holdingAccountID,
		CashAccountID: f.cashAccountID, CashCommodityID: f.eurCommodityID,
		QuantityValue: exact.New(4), CashAmountValue: 48000, CashAmountScale: 2,
		CostBasisMethod: "specific_lot",
		LotAllocations: []InvestmentLotAllocationInput{{LotID: *bought.LotID,
			QuantityValue: exact.New(4)}},
	})
	require.NoError(t, err)
	_, err = f.investmentService.Sell(ctx, InvestmentTradeInput{
		OwnerUserID: f.ownerUserID, TransactionDate: "2026-03-01",
		CommodityID: f.stockCommodityID, HoldingAccountID: f.holdingAccountID,
		CashAccountID: f.cashAccountID, CashCommodityID: f.eurCommodityID,
		QuantityValue: exact.New(6), CashAmountValue: 72000, CashAmountScale: 2,
	})
	require.NoError(t, err)
	replaced, err := acknowledgedReplaceBuy(ctx, f.investmentService, ReplaceInvestmentBuyInput{
		OwnerUserID: f.ownerUserID, TransactionID: bought.Transaction.ID,
		Reason: "correct acquisition basis",
		Replacement: InvestmentTradeInput{
			TransactionDate: "2026-01-01", CommodityID: f.stockCommodityID,
			HoldingAccountID: f.holdingAccountID, CashAccountID: f.cashAccountID,
			CashCommodityID: f.eurCommodityID, QuantityValue: exact.New(10),
			CashAmountValue: 110000, CashAmountScale: 2,
		},
	})
	require.NoError(t, err)
	var eventsBefore, auditBefore int
	require.NoError(t, f.database.QueryRow(`SELECT count(*) FROM investment_lot_events`).Scan(&eventsBefore))
	require.NoError(t, f.database.QueryRow(`SELECT count(*) FROM audit_events`).Scan(&auditBefore))
	context, err := f.investmentService.TradeCorrectionContext(ctx, f.ownerUserID, sold.Transaction.ID)
	require.NoError(t, err)
	require.True(t, context.CanReplaceSale)
	require.Len(t, context.AvailableLots, 1)
	require.Equal(t, *replaced.Replacement.LotID, context.AvailableLots[0].LotID)
	require.Equal(t, "10", context.AvailableLots[0].QuantityValue)
	require.Len(t, context.ElectedLots, 1)
	require.Equal(t, *bought.LotID, context.ElectedLots[0].LotID)
	require.Len(t, context.EffectiveElectedLots, 1)
	require.Equal(t, *replaced.Replacement.LotID, context.EffectiveElectedLots[0].LotID)
	var eventsAfter, auditAfter int
	require.NoError(t, f.database.QueryRow(`SELECT count(*) FROM investment_lot_events`).Scan(&eventsAfter))
	require.NoError(t, f.database.QueryRow(`SELECT count(*) FROM audit_events`).Scan(&auditAfter))
	require.Equal(t, eventsBefore, eventsAfter)
	require.Equal(t, auditBefore, auditAfter)
}
