package app

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"rekenraam/backend/internal/exact"
)

func TestSelfCheckDetectsDisposalAllocationConservationDamage(t *testing.T) {
	for _, replayed := range []bool{false, true} {
		for _, mutation := range []struct {
			name, assignment string
		}{
			{"quantity", "quantity_value = '2'"},
			{"basis", "cost_basis_value = '9999'"},
			{"proceeds", "proceeds_value = '9999'"},
			{"negative quantity", "quantity_value = '-1'"},
			{"negative basis", "cost_basis_value = '-1'"},
			{"wide basis", "cost_basis_value = '100000000000000000000'"},
			{"missing allocations", ""},
		} {
			t.Run(fmt.Sprintf("replayed=%t/%s", replayed, mutation.name), func(t *testing.T) {
				t.Parallel()
				f := newInvestmentsTestFixture(t)
				ctx := context.Background()
				buy := buyOn(t, f, "2026-01-01", 2, 2000)
				sold, err := f.investmentService.Sell(ctx, sellInput(f, "2026-02-01", 1))
				require.NoError(t, err)
				table := "investment_disposal_allocations"
				where := "decision_id"
				id := *sold.DisposalDecision.ID
				if replayed {
					replaced, err := f.investmentService.ReplaceBuy(ctx, ReplaceInvestmentBuyInput{
						OwnerUserID: f.ownerUserID, TransactionID: buy.Transaction.ID, Reason: "correct basis",
						Replacement: InvestmentTradeInput{TransactionDate: "2026-01-01", CommodityID: f.stockCommodityID,
							HoldingAccountID: f.holdingAccountID, CashAccountID: f.cashAccountID,
							CashCommodityID: f.eurCommodityID, QuantityValue: exact.New(2), CashAmountValue: 4000, CashAmountScale: 2},
					})
					require.NoError(t, err)
					require.Positive(t, replaced.Replacement.Transaction.ID)
					table = "investment_disposal_revision_allocations"
					where = "revision_id"
					require.NoError(t, f.database.QueryRow(`SELECT id FROM investment_disposal_revisions WHERE decision_id = ? ORDER BY revision_seq DESC LIMIT 1`, id).Scan(&id))
				}
				require.Equal(t, SelfCheckPassed, mustRunInvestmentSelfCheck(t, f).Status)
				if mutation.assignment == "" {
					_, err = f.database.Exec(`DROP TRIGGER ` + table + `_no_delete`)
					require.NoError(t, err)
					_, err = f.database.Exec(`DELETE FROM `+table+` WHERE `+where+` = ?`, id)
				} else {
					_, err = f.database.Exec(`DROP TRIGGER ` + table + `_no_update`)
					require.NoError(t, err)
					_, err = f.database.Exec(`UPDATE `+table+` SET `+mutation.assignment+` WHERE `+where+` = ?`, id)
				}
				require.NoError(t, err)
				check := resultFor(t, mustRunInvestmentSelfCheck(t, f), CheckLotReconciliation)
				require.Equal(t, SelfCheckFailed, check.Status)
				require.Contains(t, check.Summary, "disposal allocation sets disagree with quantity, basis or proceeds")
				require.Contains(t, check.Summary, fmt.Sprintf("decision #%d", *sold.DisposalDecision.ID))
				require.Contains(t, check.Sample, *sold.DisposalDecision.ID)
			})
		}
	}
}

func TestSelfCheckAllocationConservationAcceptsMixedScalesAndNegativeProceeds(t *testing.T) {
	for _, negative := range []bool{false, true} {
		t.Run(fmt.Sprintf("negative=%t", negative), func(t *testing.T) {
			t.Parallel()
			f := newInvestmentsTestFixture(t)
			buyOn(t, f, "2026-01-01", 1, 2000)
			buyOn(t, f, "2026-01-02", 1, 4000)
			input := sellInput(f, "2026-02-01", 2)
			if negative {
				input = negativeProceedsSale(f, "2026-02-01")
				input.QuantityValue = exact.New(2)
			}
			sold, err := f.investmentService.Sell(context.Background(), input)
			require.NoError(t, err)
			require.Len(t, sold.DisposalDecision.Allocations, 2)
			_, err = f.database.Exec(`DROP TRIGGER investment_disposal_allocations_no_update`)
			require.NoError(t, err)
			_, err = f.database.Exec(`UPDATE investment_disposal_allocations
				SET quantity_value = quantity_value || '0', quantity_scale = quantity_scale + 1,
				cost_basis_value = cost_basis_value || '0', cost_basis_scale = cost_basis_scale + 1,
				proceeds_value = proceeds_value || '0', proceeds_scale = proceeds_scale + 1
				WHERE decision_id = ? AND allocation_seq = 2`, *sold.DisposalDecision.ID)
			require.NoError(t, err)
			require.Equal(t, SelfCheckPassed, mustRunInvestmentSelfCheck(t, f).Status,
				"equivalent scales and negative proceeds preserve a valid two-lot snapshot")
		})
	}
}

func TestSelfCheckDetectsDisposalRevisionBasisHeaderDamage(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	buy := buyOn(t, f, "2026-01-01", 2, 2000)
	sold, err := f.investmentService.Sell(ctx, sellInput(f, "2026-02-01", 1))
	require.NoError(t, err)
	_, err = f.investmentService.ReplaceBuy(ctx, ReplaceInvestmentBuyInput{
		OwnerUserID: f.ownerUserID, TransactionID: buy.Transaction.ID, Reason: "correct basis",
		Replacement: InvestmentTradeInput{TransactionDate: "2026-01-01", CommodityID: f.stockCommodityID,
			HoldingAccountID: f.holdingAccountID, CashAccountID: f.cashAccountID,
			CashCommodityID: f.eurCommodityID, QuantityValue: exact.New(2), CashAmountValue: 4000, CashAmountScale: 2},
	})
	require.NoError(t, err)
	require.Equal(t, SelfCheckPassed, mustRunInvestmentSelfCheck(t, f).Status)
	_, err = f.database.Exec(`DROP TRIGGER investment_disposal_revisions_no_update`)
	require.NoError(t, err)
	_, err = f.database.Exec(`UPDATE investment_disposal_revisions SET disposed_basis_value = '9999' WHERE decision_id = ?`, *sold.DisposalDecision.ID)
	require.NoError(t, err)
	check := resultFor(t, mustRunInvestmentSelfCheck(t, f), CheckLotReconciliation)
	require.Equal(t, SelfCheckFailed, check.Status)
	require.EqualValues(t, 1, check.FindingCount)
	require.Contains(t, check.Summary, "disposal allocation sets disagree with quantity, basis or proceeds")
}

func TestSelfCheckChecksEveryDisposalRevisionSnapshot(t *testing.T) {
	for _, original := range []bool{false, true} {
		t.Run(fmt.Sprintf("original=%t", original), func(t *testing.T) {
			t.Parallel()
			f := newInvestmentsTestFixture(t)
			ctx := context.Background()
			buy := buyOn(t, f, "2026-01-01", 2, 2000)
			sold, err := f.investmentService.Sell(ctx, sellInput(f, "2026-02-01", 1))
			require.NoError(t, err)
			for _, basis := range []int64{4000, 6000} {
				replaced, err := f.investmentService.ReplaceBuy(ctx, ReplaceInvestmentBuyInput{
					OwnerUserID: f.ownerUserID, TransactionID: buy.Transaction.ID, Reason: "correct basis",
					Replacement: InvestmentTradeInput{TransactionDate: "2026-01-01", CommodityID: f.stockCommodityID,
						HoldingAccountID: f.holdingAccountID, CashAccountID: f.cashAccountID,
						CashCommodityID: f.eurCommodityID, QuantityValue: exact.New(2), CashAmountValue: basis, CashAmountScale: 2},
				})
				require.NoError(t, err)
				buy = replaced.Replacement
			}
			require.Equal(t, SelfCheckPassed, mustRunInvestmentSelfCheck(t, f).Status)
			table, where := "investment_disposal_revision_allocations", "revision_id = (SELECT id FROM investment_disposal_revisions WHERE decision_id = ? AND revision_seq = 2)"
			if original {
				table, where = "investment_disposal_allocations", "decision_id = ?"
			}
			_, err = f.database.Exec(`DROP TRIGGER ` + table + `_no_update`)
			require.NoError(t, err)
			_, err = f.database.Exec(`UPDATE `+table+` SET proceeds_value = '9999' WHERE `+where, *sold.DisposalDecision.ID)
			require.NoError(t, err)
			check := resultFor(t, mustRunInvestmentSelfCheck(t, f), CheckLotReconciliation)
			require.Equal(t, SelfCheckFailed, check.Status)
			require.EqualValues(t, 1, check.FindingCount)
			require.Contains(t, check.Summary, "disposal allocation sets disagree with quantity, basis or proceeds")
			var stillDamaged string
			require.NoError(t, f.database.QueryRow(`SELECT proceeds_value FROM `+table+` WHERE `+where, *sold.DisposalDecision.ID).Scan(&stillDamaged))
			require.Equal(t, "9999", stillDamaged, "self-check must not repair immutable evidence")
		})
	}
}
