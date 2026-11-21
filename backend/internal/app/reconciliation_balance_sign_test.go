package app

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rekenraam/backend/internal/exact"
)

func TestReconciliationBalanceQuantitiesUseAccountNormalSign(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		class, kind string
		ledgerSign  int64
	}{
		{"liability", "credit_card", -1},
		{"liability", "mortgage", -1},
		{"asset", "checking", 1},
	} {
		t.Run(tc.kind, func(t *testing.T) {
			ctx := context.Background()
			f := newInvestmentsTestFixture(t)
			service := f.transactionService
			accountID := seedTestAccountWithClass(t, f.database, "active", true, tc.class, tc.kind)
			post := func(date string, amount int64) {
				t.Helper()
				spec := ordinaryEntry(accountID, f.cashAccountID, f.eurCommodityID, tc.ledgerSign*amount)
				spec.TransactionDate = date
				_, err := service.CreateTransaction(ctx, CreateTransactionInput{OriginType: "browser_api", OwnerUserID: f.ownerUserID, Spec: spec})
				require.NoError(t, err)
			}
			assertBalance := func(label string, balance BalanceQuantity, amount int64) {
				t.Helper()
				assert.Equal(t, f.eurCommodityID, balance.CommodityID, label)
				assert.Equal(t, 0, balance.QuantityScale, label)
				assert.Equal(t, exact.New(tc.ledgerSign*amount), balance.QuantityValue, label)
				assert.Equal(t, exact.New(amount), balance.NormalQuantityValue, label)
			}

			post("2026-03-10", 400)
			reconcileAccount(t, f, accountID, f.eurCommodityID, "2026-03-31", tc.ledgerSign*400)
			post("2026-04-10", 100)
			started, err := service.StartReconciliation(ctx, StartReconciliationInput{
				OriginType: "browser_api", OwnerUserID: f.ownerUserID, AccountID: accountID, CommodityID: f.eurCommodityID,
				StatementDate: "2026-04-30", StatementBalanceValue: exact.New(tc.ledgerSign * 500),
			})
			require.NoError(t, err)
			assertBalance("statement", started.StatementBalance, 500)
			assertBalance("starting", started.StartingBalance, 400)
			assertBalance("unselected difference", started.Difference, 100)
			require.Len(t, started.Candidates, 1)

			selection := ReconciliationSelectionInput{
				OriginType: "browser_api", OwnerUserID: f.ownerUserID, SessionID: started.ID,
				PostingVersionIDs: []int64{started.Candidates[0].PostingID},
			}
			selected, err := service.UpdateReconciliationSelection(ctx, selection)
			require.NoError(t, err)
			assertBalance("selected", selected.SelectedBalance, 100)
			assertBalance("balanced difference", selected.Difference, 0)

			finished, err := service.FinishReconciliation(ctx, FinishReconciliationInput{
				OriginType: "browser_api", OwnerUserID: f.ownerUserID, SessionID: started.ID, ChangeReason: "statement verified",
			})
			require.NoError(t, err)
			assertBalance("finished statement", finished.StatementBalance, 500)
			read, err := service.ReconciliationSession(ctx, started.ID)
			require.NoError(t, err)
			assertBalance("reloaded statement", read.StatementBalance, 500)
			assertBalance("reloaded selection", read.SelectedBalance, 100)

			checkpoints, err := service.ReconciliationCheckpoints(ctx, ListReconciliationCheckpointsInput{
				AccountID: accountID, CommodityID: f.eurCommodityID,
			})
			require.NoError(t, err)
			require.Len(t, checkpoints, 2)
			assertBalance("listed checkpoint", checkpoints[0].StatementBalance, 500)
			var stored exact.Coefficient
			require.NoError(t, f.database.QueryRowContext(ctx,
				"SELECT statement_balance_value FROM reconciliation_checkpoints WHERE id = ?", checkpoints[0].ID).Scan(&stored))
			assert.Equal(t, exact.New(tc.ledgerSign*500), stored)

			voided, err := service.VoidReconciliationCheckpoint(ctx, VoidReconciliationInput{
				OriginType: "browser_api", OwnerUserID: f.ownerUserID, ID: checkpoints[0].ID, ChangeReason: "wrong statement",
			})
			require.NoError(t, err)
			assertBalance("voided checkpoint", voided.StatementBalance, 500)
		})
	}
}
