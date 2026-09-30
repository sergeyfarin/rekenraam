package app

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"rekenraam/backend/internal/exact"
)

func TestInvestmentCorrectionReadsAndCommandsUseJournalLinks(t *testing.T) {
	for _, kind := range []string{"buy", "sell"} {
		t.Run(kind, func(t *testing.T) {
			ctx := context.Background()
			f := newInvestmentsTestFixture(t)
			clearInvestmentHeadersOnInsert(t, f)
			trade := InvestmentTradeInput{
				OwnerUserID: f.ownerUserID, TransactionDate: "2026-01-01",
				CommodityID: f.stockCommodityID, HoldingAccountID: f.holdingAccountID,
				CashAccountID: f.cashAccountID, CashCommodityID: f.eurCommodityID,
				QuantityValue: exact.New(10), CashAmountValue: 100000, CashAmountScale: 2,
			}
			original, err := f.investmentService.Buy(ctx, trade)
			require.NoError(t, err)
			if kind == "sell" {
				trade.TransactionDate = "2026-02-01"
				trade.QuantityValue, trade.CashAmountValue, trade.CostBasisMethod = exact.New(3), 45000, "fifo"
				original, err = f.investmentService.Sell(ctx, trade)
				require.NoError(t, err)
			}
			facts, err := f.investmentService.TradeCorrectionContext(ctx, f.ownerUserID, original.Transaction.ID)
			require.NoError(t, err)
			before, err := f.investmentService.CorrectionChain(ctx, f.ownerUserID, original.Transaction.ID)
			require.NoError(t, err)
			// Isolated mutation proves these consumers do not consult the
			// transitional header. Durable posted journals and links stay intact.
			_, err = f.database.ExecContext(ctx, "UPDATE investment_operations SET transaction_id = NULL")
			require.NoError(t, err)
			afterFacts, err := f.investmentService.TradeCorrectionContext(ctx, f.ownerUserID, original.Transaction.ID)
			require.NoError(t, err)
			require.Equal(t, facts, afterFacts)
			after, err := f.investmentService.CorrectionChain(ctx, f.ownerUserID, original.Transaction.ID)
			require.NoError(t, err)
			require.Equal(t, before, after)

			var replacement, inverse Transaction
			trade.CashAmountValue += 100
			if kind == "buy" {
				input := ReplaceInvestmentBuyInput{OwnerUserID: f.ownerUserID, TransactionID: original.Transaction.ID, Reason: "correct linked buy", Replacement: trade}
				_, err = f.investmentService.ReplaceBuyReconciliationImpact(ctx, input)
				require.NoError(t, err)
				result, err := f.investmentService.ReplaceBuy(ctx, input)
				require.NoError(t, err)
				replacement, inverse = result.Replacement.Transaction, result.Inverse
			} else {
				input := ReplaceInvestmentSaleInput{OwnerUserID: f.ownerUserID, TransactionID: original.Transaction.ID, Reason: "correct linked sale", Replacement: trade}
				_, err = f.investmentService.ReplaceSaleReconciliationImpact(ctx, input)
				require.NoError(t, err)
				result, err := f.investmentService.ReplaceSale(ctx, input)
				require.NoError(t, err)
				replacement, inverse = result.Replacement.Transaction, result.Inverse
			}
			_, err = f.database.ExecContext(ctx, "UPDATE investment_operations SET transaction_id = NULL")
			require.NoError(t, err)
			chain, err := f.investmentService.CorrectionChain(ctx, f.ownerUserID, original.Transaction.ID)
			require.NoError(t, err)
			require.Len(t, chain.Operations, 2, "inverse journal must not duplicate the replacement node")
			require.Equal(t, replacement.ID, *chain.EffectiveTransactionID)
			for _, id := range []int64{inverse.ID, replacement.ID} {
				fromJournal, err := f.investmentService.CorrectionChain(ctx, f.ownerUserID, id)
				require.NoError(t, err)
				require.Equal(t, chain, fromJournal)
			}
			_, err = f.investmentService.TradeCorrectionContext(ctx, f.ownerUserID, inverse.ID)
			require.ErrorIs(t, err, ErrInvestmentOperationNotFound, "inverse is not a replacement trade")
			var reversal Transaction
			if kind == "buy" {
				reversal, err = f.investmentService.ReverseBuy(ctx, ReverseInvestmentBuyInput{OwnerUserID: f.ownerUserID, TransactionID: replacement.ID, Reason: "cancel linked buy"})
			} else {
				reversal, err = f.investmentService.ReverseSale(ctx, ReverseInvestmentSaleInput{OwnerUserID: f.ownerUserID, TransactionID: replacement.ID, Reason: "cancel linked sale"})
			}
			require.NoError(t, err)
			_, err = f.database.ExecContext(ctx, "UPDATE investment_operations SET transaction_id = NULL")
			require.NoError(t, err)
			terminal, err := f.investmentService.CorrectionChain(ctx, f.ownerUserID, reversal.ID)
			require.NoError(t, err)
			require.Len(t, terminal.Operations, 3)
			require.Nil(t, terminal.EffectiveTransactionID)
			require.Equal(t, reversal.ID, *terminal.Operations[2].TransactionID)
			require.Equal(t, SelfCheckPassed, mustRunInvestmentSelfCheck(t, f).Status)
		})
	}
}

// Remove the compatibility identity before the writer can read it, rather than
// only clearing it after commit. This mutation is confined to the test database.
func clearInvestmentHeadersOnInsert(t *testing.T, f *investmentsTestFixture) {
	t.Helper()
	_, err := f.database.Exec("DROP TRIGGER investment_operations_no_update")
	require.NoError(t, err)
	_, err = f.database.Exec(`CREATE TRIGGER test_clear_investment_header
		AFTER INSERT ON investment_operations BEGIN
		UPDATE investment_operations SET transaction_id = NULL WHERE id = NEW.id;
		END`)
	require.NoError(t, err)
}

func TestInvestmentTransferAndReinvestmentWritersWithoutCompatibilityHeader(t *testing.T) {
	ctx := context.Background()
	f := newInvestmentsTestFixture(t)
	clearInvestmentHeadersOnInsert(t, f)
	seedExternalTransferEquity(t, f.database)
	destinationID := seedTestAccountWithClass(t, f.database, "active", true, "asset", "security_holding")
	inbound, err := f.investmentService.ExternalTransferIn(ctx, knownTransferInput(f))
	require.NoError(t, err)
	require.NotNil(t, inbound.LotID)
	transfer := internalTransferFromLot(f, destinationID, *inbound.LotID, exact.New(1), 0)
	transfer.EffectiveOn = "2026-07-01"
	moved, err := f.investmentService.InternalTransfer(ctx, transfer)
	require.NoError(t, err)
	require.Len(t, moved.DestinationLotIDs, 1)
	_, err = f.investmentService.ReinvestedDividend(ctx, ReinvestedDividendInput{
		OwnerUserID: f.ownerUserID, TransactionDate: "2026-08-01",
		CommodityID: f.stockCommodityID, HoldingAccountID: destinationID,
		IncomeAccountID: &f.incomeAccountID, QuantityValue: exact.New(2),
		AmountValue: 5000, AmountScale: 2, CashCommodityID: f.eurCommodityID,
	})
	require.NoError(t, err)
	require.Equal(t, SelfCheckPassed, mustRunInvestmentSelfCheck(t, f).Status)
}

func TestInvestmentCorrectionReadsRequireJournalLinkEvenWithCompatibilityHeader(t *testing.T) {
	ctx := context.Background()
	f := newInvestmentsTestFixture(t)
	bought, err := f.investmentService.Buy(ctx, InvestmentTradeInput{
		OwnerUserID: f.ownerUserID, TransactionDate: "2026-01-01",
		CommodityID: f.stockCommodityID, HoldingAccountID: f.holdingAccountID,
		CashAccountID: f.cashAccountID, CashCommodityID: f.eurCommodityID,
		QuantityValue: exact.New(2), CashAmountValue: 20000, CashAmountScale: 2,
	})
	require.NoError(t, err)
	_, err = f.database.ExecContext(ctx, "DROP TRIGGER investment_operation_journal_links_no_delete")
	require.NoError(t, err)
	_, err = f.database.ExecContext(ctx, "DELETE FROM investment_operation_journal_links WHERE transaction_version_id = ?", bought.Transaction.VersionID)
	require.NoError(t, err)
	_, err = f.investmentService.TradeCorrectionContext(ctx, f.ownerUserID, bought.Transaction.ID)
	require.ErrorIs(t, err, ErrInvestmentOperationNotFound)
	_, err = f.investmentService.CorrectionChain(ctx, f.ownerUserID, bought.Transaction.ID)
	require.ErrorIs(t, err, ErrInvestmentOperationNotFound)
	_, err = f.investmentService.ReverseBuy(ctx, ReverseInvestmentBuyInput{OwnerUserID: f.ownerUserID, TransactionID: bought.Transaction.ID, Reason: "refuse unlinked buy"})
	require.ErrorIs(t, err, ErrInvestmentBuyNotFound)
	require.Equal(t, SelfCheckFailed, resultFor(t, mustRunInvestmentSelfCheck(t, f), CheckInvestmentFoundation).Status)
}
