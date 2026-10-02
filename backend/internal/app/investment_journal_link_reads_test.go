package app

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"bytes"
	"rekenraam/backend/internal/db"
	"rekenraam/backend/internal/exact"
)

func TestInvestmentCorrectionReadsAndCommandsUseJournalLinks(t *testing.T) {
	for _, kind := range []string{"buy", "sell"} {
		t.Run(kind, func(t *testing.T) {
			ctx := context.Background()
			f := newInvestmentsTestFixture(t)
			requireInvestmentHeaderRetired(t, f)
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
			// Reads and commands run with the compatibility column absent.
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
				result, err := acknowledgedReplaceBuy(ctx, f.investmentService, input)
				require.NoError(t, err)
				replacement, inverse = result.Replacement.Transaction, result.Inverse
			} else {
				input := ReplaceInvestmentSaleInput{OwnerUserID: f.ownerUserID, TransactionID: original.Transaction.ID, Reason: "correct linked sale", Replacement: trade}
				_, err = f.investmentService.ReplaceSaleReconciliationImpact(ctx, input)
				require.NoError(t, err)
				result, err := acknowledgedReplaceSale(ctx, f.investmentService, input)
				require.NoError(t, err)
				replacement, inverse = result.Replacement.Transaction, result.Inverse
			}
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
				reversal, err = acknowledgedReverseBuy(ctx, f.investmentService, ReverseInvestmentBuyInput{OwnerUserID: f.ownerUserID, TransactionID: replacement.ID, Reason: "cancel linked buy"})
			} else {
				reversal, err = acknowledgedReverseSale(ctx, f.investmentService, ReverseInvestmentSaleInput{OwnerUserID: f.ownerUserID, TransactionID: replacement.ID, Reason: "cancel linked sale"})
			}
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

// Every writer regression runs against the actual retired-header schema.
func requireInvestmentHeaderRetired(t *testing.T, f *investmentsTestFixture) {
	t.Helper()
	var count int
	require.NoError(t, f.database.QueryRow(`SELECT count(*) FROM pragma_table_info('investment_operations') WHERE name = 'transaction_id'`).Scan(&count))
	require.Zero(t, count)
}

func TestInvestmentTransferAndReinvestmentWritersWithoutCompatibilityHeader(t *testing.T) {
	ctx := context.Background()
	f := newInvestmentsTestFixture(t)
	requireInvestmentHeaderRetired(t, f)
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
	_, err = acknowledgedReinvestedDividend(ctx, f.investmentService, ReinvestedDividendInput{
		OwnerUserID: f.ownerUserID, TransactionDate: "2026-08-01",
		CommodityID: f.stockCommodityID, HoldingAccountID: destinationID,
		IncomeAccountID: &f.incomeAccountID, QuantityValue: exact.New(2),
		AmountValue: 5000, AmountScale: 2, CashCommodityID: f.eurCommodityID,
	})
	require.NoError(t, err)
	require.Equal(t, SelfCheckPassed, mustRunInvestmentSelfCheck(t, f).Status)
}

func TestInvestmentCorrectionReadsRequireJournalLink(t *testing.T) {
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
	_, err = acknowledgedReverseBuy(ctx, f.investmentService, ReverseInvestmentBuyInput{OwnerUserID: f.ownerUserID, TransactionID: bought.Transaction.ID, Reason: "refuse unlinked buy"})
	require.ErrorIs(t, err, ErrInvestmentBuyNotFound)
	require.Equal(t, SelfCheckFailed, resultFor(t, mustRunInvestmentSelfCheck(t, f), CheckInvestmentFoundation).Status)
}

func TestInvestmentOperationSchemaRetiresTransactionHeader(t *testing.T) {
	f := newInvestmentsTestFixture(t)
	var count int
	require.NoError(t, f.database.QueryRow(`SELECT count(*) FROM pragma_table_info('investment_operations') WHERE name = 'transaction_id'`).Scan(&count))
	require.Zero(t, count, "journal links are the only operation-to-transaction relationship")
}

func TestLotSelfCheckReportsMissingAndCorruptState(t *testing.T) {
	ctx := context.Background()
	f := newInvestmentsTestFixture(t)
	bought := buyOn(t, f, "2026-01-01", 3, 30000)
	_, err := f.database.ExecContext(ctx, `UPDATE investment_lot_state SET remaining_quantity_value = '4' WHERE lot_id = ?`, *bought.LotID)
	require.NoError(t, err)
	require.Equal(t, SelfCheckFailed, resultFor(t, mustRunInvestmentSelfCheck(t, f), CheckLotReconciliation).Status)
	_, err = f.database.ExecContext(ctx, `DELETE FROM investment_lot_state WHERE lot_id = ?`, *bought.LotID)
	require.NoError(t, err)
	missing := resultFor(t, mustRunInvestmentSelfCheck(t, f), CheckLotReconciliation)
	require.Equal(t, SelfCheckFailed, missing.Status)
	require.Contains(t, missing.Summary, "missing their current state projection")
	require.EqualValues(t, 1, missing.FindingCount)
	require.Contains(t, missing.Sample, *bought.LotID)
	var quantity string
	require.NoError(t, f.database.QueryRow(`SELECT quantity_value FROM investment_lots WHERE id = ?`, *bought.LotID).Scan(&quantity))
	require.Equal(t, "3", quantity)
	var bundle bytes.Buffer
	require.Error(t, NewExportService(db.NewExportRepository(f.database)).WriteBundle(ctx, &bundle, ExportFilter{}), "missing state must not export a fabricated zero")
}

func TestBuyLotStateInitializationFailureRollsBackCommand(t *testing.T) {
	ctx := context.Background()
	f := newInvestmentsTestFixture(t)
	counts := func() map[string]int {
		result := map[string]int{}
		for _, table := range []string{"audit_events", "transactions", "investment_operations", "investment_lots", "investment_lot_state", "investment_lot_events", "price_observations"} {
			var count int
			require.NoError(t, f.database.QueryRow("SELECT count(*) FROM "+table).Scan(&count))
			result[table] = count
		}
		return result
	}
	before := counts()
	_, err := f.database.ExecContext(ctx, `CREATE TRIGGER reject_lot_state BEFORE INSERT ON investment_lot_state
 BEGIN SELECT RAISE(ABORT, 'lot state rejected'); END`)
	require.NoError(t, err)
	_, err = f.investmentService.Buy(ctx, InvestmentTradeInput{
		OwnerUserID: f.ownerUserID, TransactionDate: "2026-01-01",
		CommodityID: f.stockCommodityID, HoldingAccountID: f.holdingAccountID,
		CashAccountID: f.cashAccountID, CashCommodityID: f.eurCommodityID,
		QuantityValue: exact.New(3), CashAmountValue: 30000, CashAmountScale: 2,
	})
	require.ErrorContains(t, err, "lot state rejected")
	require.Equal(t, before, counts())
}
