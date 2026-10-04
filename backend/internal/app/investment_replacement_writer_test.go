package app

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"rekenraam/backend/internal/db"
	"rekenraam/backend/internal/exact"
)

func TestReplacementWriterRollsBackLateFailure(t *testing.T) {
	t.Parallel()
	for _, test := range []struct{ kind, failure string }{{"buy", "checkpoint"}, {"sale", "checkpoint"}, {"buy", "source acceptance"}, {"sale", "source acceptance"}} {
		kind := test.kind
		t.Run(kind+"/"+test.failure, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			buyQuantity := exact.New(10)
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
				var transactions, audits, operations, revisions, lots, events, prices, links int
				require.NoError(t, f.database.QueryRow(`SELECT
					(SELECT count(*) FROM transactions), (SELECT count(*) FROM audit_events),
					(SELECT count(*) FROM investment_operations), (SELECT count(*) FROM investment_disposal_revisions),
					(SELECT count(*) FROM investment_lots), (SELECT count(*) FROM investment_lot_events),
					(SELECT count(*) FROM price_observations), (SELECT count(*) FROM investment_operation_journal_links)`).
					Scan(&transactions, &audits, &operations, &revisions, &lots, &events, &prices, &links))
				return []int{transactions, audits, operations, revisions, lots, events, prices, links}
			}
			beforeCounts := counts()
			buyReplacement := ReplaceInvestmentBuyInput{
				OwnerUserID: f.ownerUserID, TransactionID: target.ID, Reason: "correct acquisition", ReconciliationOverride: true,
				Replacement: InvestmentTradeInput{TransactionDate: target.TransactionDate,
					HoldingAccountID: f.holdingAccountID, CommodityID: f.stockCommodityID,
					CashAccountID: f.cashAccountID, CashCommodityID: f.eurCommodityID,
					QuantityValue: buyQuantity, CashAmountValue: 20000, CashAmountScale: 2},
			}
			corrected := sellInput(f, target.TransactionDate, 4)
			corrected.CostBasisMethod = "fifo"
			saleReplacement := ReplaceInvestmentSaleInput{
				OwnerUserID: f.ownerUserID, TransactionID: target.ID, Reason: "correct disposal", ReconciliationOverride: true,
				Replacement: corrected,
			}
			// Review the gain changes before forcing a failure: the preview runs
			// the same checkpoint invalidation, so it must happen first.
			if kind == "buy" {
				impact, err := f.investmentService.ReplaceBuyReconciliationImpact(ctx, buyReplacement)
				require.NoError(t, err)
				buyReplacement.GainImpactAcknowledgement = impact.GainImpact.Acknowledgement
			} else {
				impact, err := f.investmentService.ReplaceSaleReconciliationImpact(ctx, saleReplacement)
				require.NoError(t, err)
				saleReplacement.GainImpactAcknowledgement = impact.GainImpact.Acknowledgement
			}
			require.Equal(t, beforeCounts, counts())
			if test.failure == "checkpoint" {
				_, err = f.database.Exec(`CREATE TRIGGER reject_replacement_checkpoint AFTER UPDATE ON reconciliation_checkpoints
				WHEN NEW.status = 'invalidated' BEGIN SELECT RAISE(ABORT, 'forced checkpoint failure'); END`)
				require.NoError(t, err)
			}
			failAcceptance := test.failure == "source acceptance"
			callbackRan := false
			postWrite := func(tx *sql.Tx, operationID, auditEventID int64) error {
				callbackRan = true
				var journals, invalidated int
				require.NoError(t, tx.QueryRowContext(ctx, `SELECT count(*) FROM transactions WHERE created_audit_event_id = ?`, auditEventID).Scan(&journals))
				require.Equal(t, 2, journals)
				require.Positive(t, operationID)
				require.NoError(t, tx.QueryRowContext(ctx, `SELECT count(*) FROM reconciliation_checkpoints WHERE id = ? AND status = 'invalidated'`, checkpointID).Scan(&invalidated))
				require.Equal(t, 1, invalidated)
				if failAcceptance {
					return errors.New("forced source acceptance failure")
				}
				return nil
			}
			replace := func() error {
				if kind == "buy" {
					_, err := f.investmentService.replaceBuyWithPostWrite(ctx, buyReplacement, postWrite)
					return err
				}
				_, err := f.investmentService.replaceSaleWithPostWriteOrigin(ctx, saleReplacement,
					"browser_api", "investment.sale.replace", postWrite)
				return err
			}
			err = replace()
			require.ErrorContains(t, err, "forced "+map[string]string{"checkpoint": "checkpoint", "source acceptance": "source acceptance"}[test.failure]+" failure")
			require.Equal(t, test.failure == "source acceptance", callbackRan)

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
			if test.failure == "checkpoint" {
				_, err = f.database.Exec(`DROP TRIGGER reject_replacement_checkpoint`)
				require.NoError(t, err)
			}
			failAcceptance = false
			err = replace()
			require.NoError(t, err)
			require.Empty(t, activeCheckpointIDs(t, f))
			check, err := selfCheckOver(t, f.database).RunSelfCheck(ctx, "manual")
			require.NoError(t, err)
			require.Equal(t, SelfCheckPassed, check.Status)
		})
	}
}

func TestReplacementWriterRechecksStaleSourceBeforeJournals(t *testing.T) {
	t.Parallel()
	for _, kind := range []string{"buy", "sale"} {
		t.Run(kind, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			f := newInvestmentsTestFixture(t)
			target := buyOn(t, f, "2026-01-01", 10, 10000).Transaction
			replacement := InvestmentTradeInput{OwnerUserID: f.ownerUserID, TransactionDate: target.TransactionDate,
				HoldingAccountID: f.holdingAccountID, CommodityID: f.stockCommodityID,
				CashAccountID: f.cashAccountID, CashCommodityID: f.eurCommodityID,
				QuantityValue: exact.New(10), CashAmountValue: 20000, CashAmountScale: 2}
			var buy db.BuyOperationRecord
			var sale db.SaleOperationRecord
			var inversePlan CreateTransactionInput
			var operationID int64
			var err error
			if kind == "buy" {
				buy, inversePlan, err = f.investmentService.buyReplacementPlan(ctx, ReplaceInvestmentBuyInput{
					OwnerUserID: f.ownerUserID, TransactionID: target.ID, Reason: "correct acquisition"})
				operationID = buy.OperationID
			} else {
				sold, sellErr := f.investmentService.Sell(ctx, sellInput(f, "2026-02-01", 3))
				require.NoError(t, sellErr)
				target = sold.Transaction
				replacement = sellInput(f, target.TransactionDate, 4)
				replacement.CostBasisMethod = "fifo"
				sale, inversePlan, err = f.investmentService.reverseSalePlan(ctx, ReverseInvestmentSaleInput{
					OwnerUserID: f.ownerUserID, TransactionID: target.ID, Reason: "correct disposal"}, saleCorrectionFamily)
				operationID = sale.OperationID
				inversePlan.Spec.InvestmentOperationKind = ""
			}
			require.NoError(t, err)
			inverse, err := f.investmentService.transactionService.prepareInvestmentTransactionForWrite(ctx, inversePlan, nil)
			require.NoError(t, err)
			var corrected db.CreateTransactionParams
			var lot db.CreateInvestmentLotParams
			var disposal db.DisposeLotsParams
			if kind == "buy" {
				corrected, lot, err = f.investmentService.prepareBuyWrite(ctx, replacement)
			} else {
				corrected, disposal, err = f.investmentService.prepareSellWrite(ctx, replacement)
			}
			require.NoError(t, err)
			corrected.CorrectionOfTransactionID = inverse.CorrectionOfTransactionID
			corrected.InvestmentCorrectionOfOperationID = operationID
			corrected.InvestmentCorrectionMode, corrected.InvestmentCorrectionReason = "replace", inversePlan.ChangeReason
			corrected.CreatedAt = inverse.CreatedAt
			callbackCalls := 0
			postWrite := func(*sql.Tx, int64, int64) error { callbackCalls++; return nil }
			replace := func() error {
				if kind == "buy" {
					_, err := f.investmentService.repository.ReplaceBuyWithPostWrite(ctx, buy, inverse, corrected, lot, postWrite)
					return err
				}
				_, _, _, _, err := f.investmentService.repository.ReplaceSaleWithPostWrite(ctx, sale, inverse, corrected, disposal, postWrite)
				return err
			}
			require.NoError(t, replace())
			beforeTransactions := f.transactionCount(t)
			var beforeAudits int
			require.NoError(t, f.database.QueryRow(`SELECT count(*) FROM audit_events`).Scan(&beforeAudits))
			// If the guard ran after journal insertion, even the first write above
			// would reject its own successor. A stale retry must not reach acceptance.
			require.ErrorIs(t, replace(), db.ErrInvestmentOperationAlreadyCorrected)
			require.Equal(t, 1, callbackCalls)
			require.Equal(t, beforeTransactions, f.transactionCount(t))
			var afterAudits int
			require.NoError(t, f.database.QueryRow(`SELECT count(*) FROM audit_events`).Scan(&afterAudits))
			require.Equal(t, beforeAudits, afterAudits)
		})
	}
}
