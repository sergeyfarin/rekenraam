package app

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"rekenraam/backend/internal/exact"
)

func TestReleaseReviewProbe(t *testing.T) {
	for _, method := range []string{"fifo", "lifo", "average_cost", "specific_lot"} {
		t.Run("future_lot_"+method, func(t *testing.T) {
			f := newInvestmentsTestFixture(t)
			ctx := context.Background()
			input := InvestmentTradeInput{OwnerUserID: f.ownerUserID, TransactionDate: "2026-06-01", CommodityID: f.stockCommodityID, HoldingAccountID: f.holdingAccountID, CashAccountID: f.cashAccountID, QuantityValue: exact.New(10), CashAmountValue: 10000, CashAmountScale: 2, CashCommodityID: f.eurCommodityID}
			buy, err := f.investmentService.Buy(ctx, input)
			require.NoError(t, err)
			input.TransactionDate = "2026-05-01"
			input.CostBasisMethod = method
			if method == "specific_lot" {
				input.LotAllocations = []InvestmentLotAllocationInput{{LotID: *buy.LotID, QuantityValue: exact.New(10)}}
			}
			result, err := f.investmentService.Sell(ctx, input)
			t.Logf("sale=%d allocations=%+v err=%v", result.Transaction.ID, result.Allocations, err)
			require.Error(t, err, "sale before acquisition must be rejected")
		})
	}
	t.Run("fractional_sale", func(t *testing.T) {
		f := newInvestmentsTestFixture(t)
		ctx := context.Background()
		input := InvestmentTradeInput{OwnerUserID: f.ownerUserID, TransactionDate: "2026-01-01", CommodityID: f.stockCommodityID, HoldingAccountID: f.holdingAccountID, CashAccountID: f.cashAccountID, QuantityValue: exact.New(10), CashAmountValue: 10000, CashAmountScale: 2, CashCommodityID: f.eurCommodityID}
		_, err := f.investmentService.Buy(ctx, input)
		require.NoError(t, err)
		input.TransactionDate = "2026-02-01"
		input.QuantityValue = exact.New(5)
		input.QuantityScale = 1
		_, err = f.investmentService.Sell(ctx, input)
		require.NoError(t, err, "a half-share sale is within commodity precision")
	})
	t.Run("generic_holding_entry", func(t *testing.T) {
		f := newInvestmentsTestFixture(t)
		ctx := context.Background()
		f.holdingAccountID = seedTestAccountWithClass(t, f.database, "active", true, "asset", "security_holding")
		_, err := f.transactionService.CreateTransaction(ctx, CreateTransactionInput{OwnerUserID: f.ownerUserID, OriginType: "browser_api", Spec: TransactionInput{TransactionDate: "2026-01-01", JournalEntries: []JournalEntryInput{{Postings: []PostingInput{{AccountID: f.holdingAccountID, CommodityID: f.stockCommodityID, QuantityValue: exact.New(10)}, {AccountID: f.cashAccountID, CommodityID: f.stockCommodityID, QuantityValue: exact.New(-10)}}}}}})
		require.Error(t, err, "generic entry must not create an untracked security position")
	})
}

func TestReleaseReviewReconciliationInterleave(t *testing.T) {
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	input := CreateTransactionInput{OwnerUserID: f.ownerUserID, OriginType: "browser_api", Spec: TransactionInput{TransactionDate: "2026-01-01", JournalEntries: []JournalEntryInput{{Postings: []PostingInput{{AccountID: f.cashAccountID, CommodityID: f.eurCommodityID, QuantityValue: exact.New(10)}, {AccountID: f.incomeAccountID, CommodityID: f.eurCommodityID, QuantityValue: exact.New(-10)}}}}}}
	_, err := f.transactionService.CreateTransaction(ctx, input)
	require.NoError(t, err)
	input.Spec.TransactionDate = "2026-01-02"
	params, err := f.transactionService.prepareCreateTransactionForWrite(ctx, input)
	require.NoError(t, err)
	session, err := f.transactionService.StartReconciliation(ctx, StartReconciliationInput{OriginType: "browser_api", OwnerUserID: f.ownerUserID, AccountID: f.cashAccountID, CommodityID: f.eurCommodityID, StatementDate: "2026-01-31", StatementBalanceValue: exact.New(10)})
	require.NoError(t, err)
	require.Len(t, session.Candidates, 1)
	_, err = f.transactionService.UpdateReconciliationSelection(ctx, ReconciliationSelectionInput{OriginType: "browser_api", OwnerUserID: f.ownerUserID, SessionID: session.ID, PostingVersionIDs: []int64{session.Candidates[0].PostingID}})
	require.NoError(t, err)
	_, err = f.transactionService.FinishReconciliation(ctx, FinishReconciliationInput{OriginType: "browser_api", OwnerUserID: f.ownerUserID, SessionID: session.ID, ChangeReason: "statement verified"})
	require.NoError(t, err)
	_, err = f.transactionService.prepareCreateTransactionForWrite(ctx, input)
	require.ErrorIs(t, err, ErrReconciliationOverrideRequired)
	record, err := f.transactionService.repository.CreateTransaction(ctx, params)
	var active int
	require.NoError(t, f.database.QueryRowContext(ctx, `SELECT count(*) FROM reconciliation_checkpoints WHERE status='active'`).Scan(&active))
	t.Logf("stale write transaction=%d err=%v active checkpoints=%d", record.ID, err, active)
	require.Error(t, err, "commit must recheck checkpoint created after preparation")
}
