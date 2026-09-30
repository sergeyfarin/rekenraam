package app

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"rekenraam/backend/internal/exact"
)

func TestSelfCheckDetectsOffsettingDisposalProceedsDamage(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	buyOn(t, f, "2026-01-01", 2, 2000)
	sold, err := f.investmentService.Sell(context.Background(), sellInput(f, "2026-02-01", 1))
	require.NoError(t, err)
	first := *sold.DisposalDecision.ID
	_, err = f.database.Exec(`DROP TRIGGER investment_disposal_decisions_no_update`)
	require.NoError(t, err)
	_, err = f.database.Exec(`UPDATE investment_disposal_decisions SET proceeds_value = '4000',
		quantity_value = '5', quantity_scale = 1, disposed_basis_value = '500', disposed_basis_scale = 2 WHERE id = ?`, first)
	require.NoError(t, err)
	_, err = f.database.Exec(`DROP TRIGGER investment_disposal_allocations_no_update`)
	require.NoError(t, err)
	_, err = f.database.Exec(`DROP TRIGGER investment_lot_events_no_update`)
	require.NoError(t, err)
	_, err = f.database.Exec(`UPDATE investment_disposal_allocations SET proceeds_value = '4000', proceeds_scale = 2,
		quantity_value = '5', quantity_scale = 1, cost_basis_value = '500', cost_basis_scale = 2 WHERE decision_id = ?`, first)
	require.NoError(t, err)
	_, err = f.database.Exec(`UPDATE investment_lot_events SET quantity_value = '-5', quantity_scale = 1,
		cost_basis_value = '-500', cost_basis_scale = 2
		WHERE id = (SELECT lot_event_id FROM investment_disposal_allocations WHERE decision_id = ?)`, first)
	require.NoError(t, err)
	second := appendSharedDisposalDecision(t, f, first, "6000", 2, f.eurCommodityID)
	result, err := f.database.Exec(`INSERT INTO investment_lot_events
		(book_id, lot_id, event_kind, transaction_id, event_date, quantity_value, quantity_scale,
		cost_basis_value, cost_basis_scale, metadata_json, created_at, created_by_user_id, created_audit_event_id, cost_basis_method)
		SELECT book_id, lot_id, event_kind, transaction_id, event_date, quantity_value, quantity_scale,
		cost_basis_value, cost_basis_scale, metadata_json, created_at, created_by_user_id, created_audit_event_id, cost_basis_method
		FROM investment_lot_events WHERE id = (SELECT lot_event_id FROM investment_disposal_allocations WHERE decision_id = ?)`, first)
	require.NoError(t, err)
	eventID, err := result.LastInsertId()
	require.NoError(t, err)
	_, err = f.database.Exec(`INSERT INTO investment_disposal_allocations
		(book_id, decision_id, lot_event_id, lot_id, allocation_seq, quantity_value, quantity_scale,
		cost_basis_value, cost_basis_scale, proceeds_value, proceeds_scale)
		SELECT book_id, ?, ?, lot_id, 1, quantity_value, quantity_scale, cost_basis_value, cost_basis_scale, '6000', 2
		FROM investment_disposal_allocations WHERE decision_id = ?`, second, eventID, first)
	require.NoError(t, err)
	_, err = f.database.Exec(`INSERT INTO investment_operation_lot_effects (operation_id, effect_seq, lot_event_id)
		SELECT operation_id, 2, ? FROM investment_disposal_decisions WHERE id = ?`, eventID, first)
	require.NoError(t, err)
	attributeSharedDisposalClearing(t, f, first, second, "4000", 2)
	baseline := mustRunInvestmentSelfCheck(t, f)
	require.Equal(t, SelfCheckPassed, baseline.Status, "%+v", baseline.Results)
	// The shared journal total remains 100.00, so an aggregate check cannot
	// establish which disposal earned which portion of the clearing proceeds.
	_, err = f.database.Exec(`UPDATE investment_disposal_decisions SET proceeds_value = '4100' WHERE id = ?`, first)
	require.NoError(t, err)
	_, err = f.database.Exec(`UPDATE investment_disposal_decisions SET proceeds_value = '5900' WHERE id = ?`, second)
	require.NoError(t, err)
	_, err = f.database.Exec(`UPDATE investment_disposal_allocations SET proceeds_value = '4100' WHERE decision_id = ?`, first)
	require.NoError(t, err)
	_, err = f.database.Exec(`UPDATE investment_disposal_allocations SET proceeds_value = '5900' WHERE decision_id = ?`, second)
	require.NoError(t, err)
	run := mustRunInvestmentSelfCheck(t, f)
	require.Equal(t, SelfCheckPassed, resultFor(t, run, CheckLotReconciliation).Status)
	check := resultFor(t, run, CheckInvestmentFoundation)
	require.Equal(t, SelfCheckFailed, check.Status)
	require.EqualValues(t, 2, check.FindingCount)
	require.Contains(t, check.Summary, "disposal clearing attribution")
}

// Fixture only: no compound command ships yet. Assign the first decision its
// elected portion of the first leg, and the second the rest of every leg.
func attributeSharedDisposalClearing(t *testing.T, f *investmentsTestFixture, first, second int64, value string, scale int) {
	t.Helper()
	_, err := f.database.Exec(`DROP TRIGGER IF EXISTS investment_disposal_clearing_allocations_no_delete`)
	require.NoError(t, err)
	_, err = f.database.Exec(`DELETE FROM investment_disposal_clearing_allocations WHERE decision_id = ?`, first)
	require.NoError(t, err)
	rows, err := f.database.Query(`SELECT pv.id, pv.quantity_value, pv.quantity_scale
		FROM investment_disposal_decisions d JOIN posting_versions pv ON pv.transaction_version_id = d.transaction_version_id
			AND pv.commodity_id = d.cost_commodity_id
		JOIN accounts a ON a.id = pv.account_id AND a.system_role = 'commodity_trading'
		WHERE d.id = ? ORDER BY pv.id`, first)
	require.NoError(t, err)
	type leg struct {
		id    int64
		value exact.Coefficient
		scale int
	}
	var legs []leg
	for rows.Next() {
		var item leg
		require.NoError(t, rows.Scan(&item.id, &item.value, &item.scale))
		legs = append(legs, item)
	}
	require.NoError(t, rows.Err())
	require.NoError(t, rows.Close())
	firstValue, err := exact.Parse(value)
	require.NoError(t, err)
	for i, item := range legs {
		portion := exact.NewScaledInt()
		if i == 0 {
			portion.AddCoefficient(firstValue, scale)
		}
		remainder := exact.ScaledIntFromCoefficient(item.value.Negated(), item.scale)
		remainder.SubScaled(portion)
		for _, allocated := range []struct {
			decision int64
			amount   *exact.ScaledInt
		}{{first, portion}, {second, remainder}} {
			coefficient, err := allocated.amount.Coefficient()
			require.NoError(t, err)
			_, err = f.database.Exec(`INSERT INTO investment_disposal_clearing_allocations
				(book_id, decision_id, posting_version_id, proceeds_value, proceeds_scale) VALUES (1, ?, ?, ?, ?)`,
				allocated.decision, item.id, coefficient, allocated.amount.Scale())
			require.NoError(t, err)
		}
	}
}

// No compound sale command ships yet. These fixtures exercise the admitted
// multi-decision journal contract directly; allocation checks remain separate.
func appendSharedDisposalDecision(t *testing.T, f *investmentsTestFixture, originalID int64, value string, scale int, costCommodityID int64) int64 {
	t.Helper()
	result, err := f.database.Exec(`INSERT INTO investment_disposal_decisions
		(book_id, transaction_id, transaction_version_id, operation_id, decision_seq,
		 position_side, account_id, commodity_id, cost_commodity_id, event_date,
		 quantity_value, quantity_scale, disposed_basis_value, disposed_basis_scale,
		 proceeds_value, proceeds_scale, cost_basis_method, resolution_tier,
		 created_at, created_by_user_id, created_audit_event_id)
		SELECT book_id, transaction_id, transaction_version_id, operation_id, 2,
		 position_side, account_id, commodity_id, ?, event_date,
		 quantity_value, quantity_scale, disposed_basis_value, disposed_basis_scale,
		 ?, ?, cost_basis_method, resolution_tier,
		 created_at, created_by_user_id, created_audit_event_id
		FROM investment_disposal_decisions WHERE id = ?`, costCommodityID, value, scale, originalID)
	require.NoError(t, err)
	id, err := result.LastInsertId()
	require.NoError(t, err)
	return id
}

func TestSelfCheckMultipleDisposalsShareClearingOnce(t *testing.T) {
	for _, test := range []struct {
		name         string
		first        string
		firstScale   int
		second       string
		secondScale  int
		negativeSale bool
		separateFee  bool
		want         string
	}{
		{name: "balanced mixed scales", first: "40", second: "6000", secondScale: 2, want: SelfCheckPassed},
		{name: "mismatched aggregate", first: "40", second: "6001", secondScale: 2, want: SelfCheckFailed},
		{name: "negative proceeds", first: "-25", firstScale: 2, second: "-2500", secondScale: 4, negativeSale: true, want: SelfCheckPassed},
		{name: "negative proceeds mismatch", first: "-25", firstScale: 2, second: "-2501", secondScale: 4, negativeSale: true, want: SelfCheckFailed},
		{name: "wide coefficients cancel exactly", first: "100000000000000000000", firstScale: 2, second: "-99999999999999990000", secondScale: 2, want: SelfCheckPassed},
		{name: "two shared clearing legs", first: "40", second: "5800", secondScale: 2, separateFee: true, want: SelfCheckPassed},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			f := newInvestmentsTestFixture(t)
			buyOn(t, f, "2026-01-01", 2, 2000)
			input := sellInput(f, "2026-02-01", 1)
			if test.negativeSale {
				input = negativeProceedsSale(f, "2026-02-01")
			}
			if test.separateFee {
				feeCashAccount := seedTestAccount(t, f.database, "active", true)
				input.CashAmountValue = 0
				input.GrossAmountValue, input.GrossAmountScale = tradeMoney(10000), 2
				input.NetSettlementValue, input.NetSettlementScale = tradeMoney(10000), 2
				input.Charges = []InvestmentTradeChargeInput{{Kind: "commission", AmountValue: -200,
					AmountScale: 2, CommodityID: f.eurCommodityID, CashAccountID: &feeCashAccount,
					Treatment: "clearing_included", PaidOn: "2026-02-01"}}
			}
			sold, err := f.investmentService.Sell(context.Background(), input)
			require.NoError(t, err)
			_, err = f.database.Exec(`DROP TRIGGER investment_disposal_decisions_no_update`)
			require.NoError(t, err)
			_, err = f.database.Exec(`UPDATE investment_disposal_decisions SET proceeds_value = ?, proceeds_scale = ? WHERE id = ?`,
				test.first, test.firstScale, *sold.DisposalDecision.ID)
			require.NoError(t, err)
			second := appendSharedDisposalDecision(t, f, *sold.DisposalDecision.ID, test.second, test.secondScale, f.eurCommodityID)
			attributeSharedDisposalClearing(t, f, *sold.DisposalDecision.ID, second, test.first, test.firstScale)
			run := mustRunInvestmentSelfCheck(t, f)
			require.Equal(t, SelfCheckPassed, resultFor(t, run, CheckEntryBalance).Status)
			check := resultFor(t, run, CheckInvestmentFoundation)
			require.Equal(t, test.want, check.Status)
			if test.want == SelfCheckFailed {
				require.EqualValues(t, 2, check.FindingCount, "group and individual attribution checks both detect the discrepancy")
				require.Contains(t, check.Summary, "disposal proceeds disagree with posted clearing")
				require.Contains(t, check.Sample, *sold.DisposalDecision.ID)
			}
		})
	}
}

func TestSelfCheckDisposalClearingKeepsCurrenciesSeparate(t *testing.T) {
	f := newInvestmentsTestFixture(t)
	buyOn(t, f, "2026-01-01", 2, 2000)
	sold, err := f.investmentService.Sell(context.Background(), sellInput(f, "2026-02-01", 1))
	require.NoError(t, err)
	result, err := f.database.Exec(`INSERT INTO commodities
		(book_id, code, kind, is_builtin, created_at, created_by_user_id)
		VALUES (1, 'USD', 'currency', 1, '2026-01-01T00:00:00Z', 1)`)
	require.NoError(t, err)
	usd, err := result.LastInsertId()
	require.NoError(t, err)
	secondID := appendSharedDisposalDecision(t, f, *sold.DisposalDecision.ID, "1", 0, usd)
	check := resultFor(t, mustRunInvestmentSelfCheck(t, f), CheckInvestmentFoundation)
	require.Equal(t, SelfCheckFailed, check.Status)
	require.EqualValues(t, 2, check.FindingCount, "the new currency has neither balanced clearing nor attributed proceeds")
	require.Contains(t, check.Sample, secondID)
	require.Contains(t, check.Summary, "disposal proceeds disagree with posted clearing")
}

func TestSelfCheckDisposalDecisionRequiresOperationJournalLink(t *testing.T) {
	f := newInvestmentsTestFixture(t)
	buyOn(t, f, "2026-01-01", 2, 2000)
	sold, err := f.investmentService.Sell(context.Background(), sellInput(f, "2026-02-01", 1))
	require.NoError(t, err)
	other, err := f.investmentService.Sell(context.Background(), sellInput(f, "2026-03-01", 1))
	require.NoError(t, err)
	// Equal-valued, balanced journals must not make unrelated provenance valid.
	_, err = f.database.Exec(`DROP TRIGGER investment_disposal_decisions_no_update`)
	require.NoError(t, err)
	_, err = f.database.Exec(`UPDATE investment_disposal_decisions
		SET transaction_id = ?, transaction_version_id = ? WHERE id = ?`,
		other.Transaction.ID, other.Transaction.VersionID, *sold.DisposalDecision.ID)
	require.NoError(t, err)
	check := resultFor(t, mustRunInvestmentSelfCheck(t, f), CheckInvestmentFoundation)
	require.Equal(t, SelfCheckFailed, check.Status)
	require.Contains(t, check.Summary, "disposal decision has no matching operation journal link")
	require.Contains(t, check.Sample, *sold.DisposalDecision.ID)
}

func TestSelfCheckDisposalClearingKeepsJournalVersionsSeparate(t *testing.T) {
	f := newInvestmentsTestFixture(t)
	buyOn(t, f, "2026-01-01", 2, 2000)
	first, err := f.investmentService.Sell(context.Background(), sellInput(f, "2026-02-01", 1))
	require.NoError(t, err)
	second, err := f.investmentService.Sell(context.Background(), sellInput(f, "2026-03-01", 1))
	require.NoError(t, err)
	// Model two decisions and two linked journals under one operation. Other
	// foundation checks still report the original source-effect provenance;
	// the proceeds check must independently find both mismatched journal totals.
	_, err = f.database.Exec(`DROP TRIGGER investment_disposal_decisions_no_update`)
	require.NoError(t, err)
	_, err = f.database.Exec(`DROP TRIGGER investment_operation_journal_links_no_update`)
	require.NoError(t, err)
	_, err = f.database.Exec(`UPDATE investment_operation_journal_links
		SET operation_id = (SELECT operation_id FROM investment_disposal_decisions WHERE id = ?), link_seq = 2
		WHERE transaction_version_id = ?`, *first.DisposalDecision.ID, second.Transaction.VersionID)
	require.NoError(t, err)
	_, err = f.database.Exec(`UPDATE investment_disposal_decisions
		SET operation_id = (SELECT operation_id FROM investment_disposal_decisions WHERE id = ?),
		decision_seq = 2, proceeds_value = '15000' WHERE id = ?`, *first.DisposalDecision.ID, *second.DisposalDecision.ID)
	require.NoError(t, err)
	_, err = f.database.Exec(`UPDATE investment_disposal_decisions SET proceeds_value = '5000' WHERE id = ?`, *first.DisposalDecision.ID)
	require.NoError(t, err)
	check := resultFor(t, mustRunInvestmentSelfCheck(t, f), CheckInvestmentFoundation)
	require.Equal(t, SelfCheckFailed, check.Status)
	require.Contains(t, check.Summary, "2 disposal proceeds disagree with posted clearing",
		"opposite discrepancies must not cancel between journals in the same operation")
	require.Contains(t, check.Sample, *first.DisposalDecision.ID)
	require.Contains(t, check.Sample, *second.DisposalDecision.ID)
}
