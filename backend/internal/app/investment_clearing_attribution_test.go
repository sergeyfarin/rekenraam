package app

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDisposalClearingAttributionGuardsAndRestoredDamage(t *testing.T) {
	t.Parallel()
	for _, test := range []struct{ name, mutation string }{
		{"missing", `DELETE FROM investment_disposal_clearing_allocations WHERE decision_id = ?`},
		{"amount", `UPDATE investment_disposal_clearing_allocations SET proceeds_value = '9999' WHERE decision_id = ?`},
		{"wrong posting account", `UPDATE investment_disposal_clearing_allocations SET posting_version_id =
			(SELECT c.posting_version_id FROM investment_operation_components c
			JOIN investment_disposal_decisions d ON d.operation_id = c.operation_id
			WHERE d.id = investment_disposal_clearing_allocations.decision_id AND c.component_kind = 'net_settlement') WHERE decision_id = ?`},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			f := newInvestmentsTestFixture(t)
			buyOn(t, f, "2026-01-01", 2, 2000)
			sold, err := f.investmentService.Sell(context.Background(), sellInput(f, "2026-02-01", 1))
			require.NoError(t, err)
			require.Equal(t, SelfCheckPassed, mustRunInvestmentSelfCheck(t, f).Status)
			_, err = f.database.Exec(test.mutation, *sold.DisposalDecision.ID)
			require.ErrorContains(t, err, "immutable")
			_, err = f.database.Exec(`DROP TRIGGER investment_disposal_clearing_allocations_no_update`)
			require.NoError(t, err)
			_, err = f.database.Exec(`DROP TRIGGER investment_disposal_clearing_allocations_no_delete`)
			require.NoError(t, err)
			_, err = f.database.Exec(test.mutation, *sold.DisposalDecision.ID)
			require.NoError(t, err)
			check := resultFor(t, mustRunInvestmentSelfCheck(t, f), CheckInvestmentFoundation)
			require.Equal(t, SelfCheckFailed, check.Status)
			require.Contains(t, check.Summary, "disposal clearing attribution")
			require.Contains(t, check.Sample, *sold.DisposalDecision.ID)
		})
	}
}

func TestDisposalClearingAttributionRejectsUnrelatedJournal(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	buyOn(t, f, "2026-01-01", 2, 2000)
	first, err := f.investmentService.Sell(context.Background(), sellInput(f, "2026-02-01", 1))
	require.NoError(t, err)
	second, err := f.investmentService.Sell(context.Background(), sellInput(f, "2026-03-01", 1))
	require.NoError(t, err)
	_, err = f.database.Exec(`INSERT INTO investment_disposal_clearing_allocations
		(book_id, decision_id, posting_version_id, proceeds_value, proceeds_scale)
		SELECT book_id, ?, posting_version_id, proceeds_value, proceeds_scale
		FROM investment_disposal_clearing_allocations WHERE decision_id = ?`,
		*first.DisposalDecision.ID, *second.DisposalDecision.ID)
	require.ErrorContains(t, err, "linked cost-currency clearing posting")
	require.Equal(t, SelfCheckPassed, mustRunInvestmentSelfCheck(t, f).Status)
	// A restored malformed relation must also be found without relying on the
	// INSERT guard or on its amount: both journals have equal clearing values.
	_, err = f.database.Exec(`DROP TRIGGER investment_disposal_clearing_allocations_no_update`)
	require.NoError(t, err)
	_, err = f.database.Exec(`UPDATE investment_disposal_clearing_allocations SET posting_version_id =
		(SELECT posting_version_id FROM investment_disposal_clearing_allocations WHERE decision_id = ?)
		WHERE decision_id = ?`, *second.DisposalDecision.ID, *first.DisposalDecision.ID)
	require.NoError(t, err)
	check := resultFor(t, mustRunInvestmentSelfCheck(t, f), CheckInvestmentFoundation)
	require.Equal(t, SelfCheckFailed, check.Status)
	require.Contains(t, check.Summary, "disposal clearing attribution")
}

func TestDisposalClearingAttributionFailureRollsBackSale(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	buyOn(t, f, "2026-01-01", 2, 2000)
	counts := func() []int {
		var values []int
		for _, table := range []string{"transactions", "audit_events", "investment_operations", "investment_lot_events", "investment_disposal_decisions", "investment_disposal_allocations", "investment_disposal_clearing_allocations"} {
			var count int
			require.NoError(t, f.database.QueryRow(`SELECT count(*) FROM `+table).Scan(&count))
			values = append(values, count)
		}
		return values
	}
	before := counts()
	_, err := f.database.Exec(`CREATE TRIGGER reject_disposal_clearing_fixture BEFORE INSERT ON investment_disposal_clearing_allocations
		BEGIN SELECT RAISE(ABORT, 'fixture attribution failure'); END`)
	require.NoError(t, err)
	_, err = f.investmentService.Sell(context.Background(), sellInput(f, "2026-02-01", 1))
	require.ErrorContains(t, err, "fixture attribution failure")
	require.Equal(t, before, counts())
	require.Equal(t, SelfCheckPassed, mustRunInvestmentSelfCheck(t, f).Status,
		"the rollback must restore the lot projection as well as all durable facts")
}

func TestDisposalClearingAttributionHandlesDatedFees(t *testing.T) {
	t.Parallel()
	for _, treatment := range []string{"clearing_included", "separately_expensed"} {
		t.Run(treatment, func(t *testing.T) {
			t.Parallel()
			f := newInvestmentsTestFixture(t)
			buyOn(t, f, "2026-01-01", 2, 2000)
			input := sellInput(f, "2026-02-01", 1)
			input.CashAmountValue = 0
			input.GrossAmountValue, input.GrossAmountScale = tradeMoney(10000), 2
			input.NetSettlementValue, input.NetSettlementScale = tradeMoney(10000), 2
			input.SettlementDate = "2026-02-03"
			var chargeAccount *int64
			if treatment == "separately_expensed" {
				feeAccount := seedTestAccountWithClass(t, f.database, "active", true, "expense", "expense")
				chargeAccount = &feeAccount
			}
			input.Charges = []InvestmentTradeChargeInput{{Kind: "commission", AmountValue: -200, AmountScale: 2,
				CommodityID: f.eurCommodityID, CashAccountID: &f.cashAccountID,
				ChargeAccountID: chargeAccount, Treatment: treatment, PaidOn: "2026-02-05"}}
			sold, err := f.investmentService.Sell(context.Background(), input)
			require.NoError(t, err)
			var count int
			require.NoError(t, f.database.QueryRow(`SELECT count(*) FROM investment_disposal_clearing_allocations WHERE decision_id = ?`, *sold.DisposalDecision.ID).Scan(&count))
			want := 1
			if treatment == "clearing_included" {
				want = 2
			}
			require.Equal(t, want, count)
			require.Equal(t, SelfCheckPassed, mustRunInvestmentSelfCheck(t, f).Status)
		})
	}
}
