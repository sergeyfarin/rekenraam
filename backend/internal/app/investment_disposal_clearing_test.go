package app

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

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
			appendSharedDisposalDecision(t, f, *sold.DisposalDecision.ID, test.second, test.secondScale, f.eurCommodityID)
			run := mustRunInvestmentSelfCheck(t, f)
			require.Equal(t, SelfCheckPassed, resultFor(t, run, CheckEntryBalance).Status)
			check := resultFor(t, run, CheckInvestmentFoundation)
			require.Equal(t, test.want, check.Status)
			if test.want == SelfCheckFailed {
				require.EqualValues(t, 1, check.FindingCount)
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
	require.EqualValues(t, 1, check.FindingCount)
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
