package app

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"rekenraam/backend/internal/db"
)

func TestReversalWriterRollsBackLateCheckpointFailure(t *testing.T) {
	t.Parallel()
	for _, kind := range []string{"buy", "sale"} {
		t.Run(kind, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			f := newInvestmentsTestFixture(t)
			buyOn(t, f, "2026-01-01", 10, 10000)
			target := buyOn(t, f, "2026-02-01", 10, 30000).Transaction
			saleInput := sellInput(f, "2026-03-01", 3)
			saleInput.CostBasisMethod = "lifo"
			sale, err := f.investmentService.Sell(ctx, saleInput)
			require.NoError(t, err)
			balance := int64(-300)
			if kind == "sale" {
				target = sale.Transaction
				_, err = f.investmentService.Sell(ctx, sellInput(f, "2026-04-01", 2))
				require.NoError(t, err)
				balance = -200
			}
			checkpointID := reconcileCash(t, f, "2026-05-01", balance)
			beforeLots, err := f.investmentService.ListLots(ctx, f.holdingAccountID, f.stockCommodityID)
			require.NoError(t, err)
			beforeGains, err := f.investmentService.ListRealizedGains(ctx, GainsReportParams{})
			require.NoError(t, err)
			counts := func() []int {
				var transactions, audits, operations, revisions int
				require.NoError(t, f.database.QueryRow(`SELECT
					(SELECT count(*) FROM transactions), (SELECT count(*) FROM audit_events),
					(SELECT count(*) FROM investment_operations), (SELECT count(*) FROM investment_disposal_revisions)`).
					Scan(&transactions, &audits, &operations, &revisions))
				return []int{transactions, audits, operations, revisions}
			}
			beforeCounts := counts()
			_, err = f.database.Exec(`CREATE TRIGGER reject_reversal_checkpoint AFTER UPDATE ON reconciliation_checkpoints
				WHEN NEW.status = 'invalidated' BEGIN SELECT RAISE(ABORT, 'forced reversal checkpoint failure'); END`)
			require.NoError(t, err)
			reverse := func() (Transaction, error) {
				if kind == "buy" {
					return f.investmentService.ReverseBuy(ctx, ReverseInvestmentBuyInput{
						OwnerUserID: f.ownerUserID, TransactionID: target.ID, Reason: "reverse duplicate acquisition", ReconciliationOverride: true,
					})
				}
				return f.investmentService.ReverseSale(ctx, ReverseInvestmentSaleInput{
					OwnerUserID: f.ownerUserID, TransactionID: target.ID, Reason: "reverse erroneous disposal", ReconciliationOverride: true,
				})
			}
			_, err = reverse()
			require.ErrorContains(t, err, "forced reversal checkpoint failure")
			require.Equal(t, beforeCounts, counts())
			afterLots, err := f.investmentService.ListLots(ctx, f.holdingAccountID, f.stockCommodityID)
			require.NoError(t, err)
			require.Equal(t, beforeLots, afterLots)
			afterGains, err := f.investmentService.ListRealizedGains(ctx, GainsReportParams{})
			require.NoError(t, err)
			require.Equal(t, beforeGains, afterGains)
			require.Equal(t, []int64{checkpointID}, activeCheckpointIDs(t, f))
			var activePrice int
			require.NoError(t, f.database.QueryRow(`SELECT count(*) FROM price_observations
				WHERE source_transaction_version_id = ? AND voided_at IS NULL`, target.VersionID).Scan(&activePrice))
			require.Equal(t, 1, activePrice)
			_, err = f.database.Exec(`DROP TRIGGER reject_reversal_checkpoint`)
			require.NoError(t, err)
			_, err = reverse()
			require.NoError(t, err)
			require.Empty(t, activeCheckpointIDs(t, f))
			check, err := selfCheckOver(t, f.database).RunSelfCheck(ctx, "manual")
			require.NoError(t, err)
			require.Equal(t, SelfCheckPassed, check.Status)
		})
	}
}

func TestReversalWriterRechecksStaleSourceBeforeJournal(t *testing.T) {
	t.Parallel()
	for _, kind := range []string{"buy", "sale"} {
		t.Run(kind, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			f := newInvestmentsTestFixture(t)
			target := buyOn(t, f, "2026-01-01", 10, 10000).Transaction
			if kind == "sale" {
				sale, err := f.investmentService.Sell(ctx, sellInput(f, "2026-02-01", 3))
				require.NoError(t, err)
				target = sale.Transaction
			}
			var expectedID int64
			var planned CreateTransactionInput
			var buy db.BuyOperationRecord
			var sale db.SaleOperationRecord
			var err error
			if kind == "buy" {
				buy, planned, err = f.investmentService.reverseBuyPlan(ctx, ReverseInvestmentBuyInput{
					OwnerUserID: f.ownerUserID, TransactionID: target.ID, Reason: "duplicate acquisition",
				})
				expectedID = buy.OperationID
			} else {
				sale, planned, err = f.investmentService.reverseSalePlan(ctx, ReverseInvestmentSaleInput{
					OwnerUserID: f.ownerUserID, TransactionID: target.ID, Reason: "erroneous disposal",
				})
				expectedID = sale.OperationID
			}
			require.NoError(t, err)
			params, err := f.investmentService.transactionService.prepareInvestmentTransactionForWrite(ctx, planned, nil)
			require.NoError(t, err)
			params.InvestmentCorrectionOfOperationID = expectedID
			params.InvestmentCorrectionMode, params.InvestmentCorrectionReason = "reverse", planned.ChangeReason
			reverse := func() error {
				if kind == "buy" {
					_, err := f.investmentService.repository.ReverseBuy(ctx, params, buy)
					return err
				}
				_, err := f.investmentService.repository.ReverseSale(ctx, params, sale)
				return err
			}
			require.NoError(t, reverse())
			beforeTransactions := f.transactionCount(t)
			var beforeAudits int
			require.NoError(t, f.database.QueryRow(`SELECT count(*) FROM audit_events`).Scan(&beforeAudits))
			require.ErrorIs(t, reverse(), db.ErrInvestmentOperationAlreadyCorrected)
			require.Equal(t, beforeTransactions, f.transactionCount(t))
			var afterAudits int
			require.NoError(t, f.database.QueryRow(`SELECT count(*) FROM audit_events`).Scan(&afterAudits))
			require.Equal(t, beforeAudits, afterAudits)
		})
	}
}
