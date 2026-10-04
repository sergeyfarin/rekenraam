package app

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"rekenraam/backend/internal/db"
	"rekenraam/backend/internal/exact"
)

func TestOlderSaleReplacementSimulationReplaysLaterSaleWithoutWrites(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := newInvestmentsTestFixture(t)
	bought, err := f.investmentService.Buy(ctx, InvestmentTradeInput{
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
	_, err = f.investmentService.Sell(ctx, InvestmentTradeInput{
		OwnerUserID: f.ownerUserID, TransactionDate: "2026-03-01",
		CommodityID: f.stockCommodityID, HoldingAccountID: f.holdingAccountID,
		CashAccountID: f.cashAccountID, CashCommodityID: f.eurCommodityID,
		QuantityValue: exact.New(6), CashAmountValue: 72000, CashAmountScale: 2,
		CostBasisMethod: "fifo",
	})
	require.NoError(t, err)
	source, err := f.investmentService.repository.SaleOperationByTransactionID(ctx, BookID, older.Transaction.ID)
	require.NoError(t, err)
	var auditBefore, eventsBefore, decisionsBefore, revisionsBefore int
	require.NoError(t, f.database.QueryRow(`SELECT count(*) FROM audit_events`).Scan(&auditBefore))
	require.NoError(t, f.database.QueryRow(`SELECT count(*) FROM investment_lot_events`).Scan(&eventsBefore))
	require.NoError(t, f.database.QueryRow(`SELECT count(*) FROM investment_disposal_decisions`).Scan(&decisionsBefore))
	require.NoError(t, f.database.QueryRow(`SELECT count(*) FROM investment_disposal_revisions`).Scan(&revisionsBefore))

	proposed := InvestmentTradeInput{
		OwnerUserID: f.ownerUserID, TransactionDate: "2026-02-01",
		CommodityID: f.stockCommodityID, HoldingAccountID: f.holdingAccountID,
		CashAccountID: f.cashAccountID, CashCommodityID: f.eurCommodityID,
		QuantityValue: exact.New(3), CashAmountValue: 39000, CashAmountScale: 2,
		CostBasisMethod: "fifo",
	}
	_, disposal, err := f.investmentService.prepareSellWrite(ctx, proposed)
	require.NoError(t, err)
	projection, err := f.investmentService.repository.SimulateSaleReplacement(ctx, source, disposal)
	require.NoError(t, err)
	require.Len(t, projection.Disposals, 2)
	require.Equal(t, "3", projection.Disposals[0].Allocations[0].QuantityValue.String())
	require.Equal(t, "6", projection.Disposals[1].Allocations[0].QuantityValue.String())
	require.Len(t, projection.Lots, 1)
	require.Equal(t, *bought.LotID, projection.Lots[0].LotID)
	require.Equal(t, "1", projection.Lots[0].RemainingQuantityValue.String())

	proposed.CostBasisMethod = "specific_lot"
	proposed.LotAllocations = []InvestmentLotAllocationInput{{
		LotID: *bought.LotID, QuantityValue: exact.New(3),
	}}
	_, disposal, err = f.investmentService.prepareSellWrite(ctx, proposed)
	require.NoError(t, err)
	projection, err = f.investmentService.repository.SimulateSaleReplacement(ctx, source, disposal)
	require.NoError(t, err)
	require.Equal(t, *bought.LotID, projection.Disposals[0].Allocations[0].LotID)

	proposed.CostBasisMethod = "fifo"
	proposed.LotAllocations = nil
	proposed.QuantityValue = exact.New(5)
	_, disposal, err = f.investmentService.prepareSellWrite(ctx, proposed)
	require.NoError(t, err)
	_, err = f.investmentService.repository.SimulateSaleReplacement(ctx, source, disposal)
	var dependency *db.InvestmentReplayDependencyError
	require.ErrorAs(t, err, &dependency)
	require.ErrorIs(t, err, db.ErrInsufficientLots)
	require.NotZero(t, dependency.DecisionID)
	require.NotEqual(t, source.OperationID, dependency.OperationID)

	var auditAfter, eventsAfter, decisionsAfter, revisionsAfter int
	require.NoError(t, f.database.QueryRow(`SELECT count(*) FROM audit_events`).Scan(&auditAfter))
	require.NoError(t, f.database.QueryRow(`SELECT count(*) FROM investment_lot_events`).Scan(&eventsAfter))
	require.NoError(t, f.database.QueryRow(`SELECT count(*) FROM investment_disposal_decisions`).Scan(&decisionsAfter))
	require.NoError(t, f.database.QueryRow(`SELECT count(*) FROM investment_disposal_revisions`).Scan(&revisionsAfter))
	require.Equal(t, auditBefore, auditAfter)
	require.Equal(t, eventsBefore, eventsAfter)
	require.Equal(t, decisionsBefore, decisionsAfter)
	require.Equal(t, revisionsBefore, revisionsAfter)
}
