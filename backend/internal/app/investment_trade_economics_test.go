package app

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"rekenraam/backend/internal/db"
	"rekenraam/backend/internal/exact"
)

func tradeMoney(value int64) *int64 { return &value }

func TestExactTradeEconomicsFeeTreatmentAndGrossPrice(t *testing.T) {
	for _, test := range []struct {
		name      string
		treatment string
		basis     int64
		proceeds  int64
		gain      int64
	}{
		{"capitalized", "clearing_included", 10200, 11800, 1600},
		{"expensed", "separately_expensed", 10000, 12000, 2000},
	} {
		t.Run(test.name, func(t *testing.T) {
			f := newInvestmentsTestFixture(t)
			ctx := context.Background()
			feeAccountID := seedTestAccountWithClass(t, f.database, "active", true, "expense", "expense")
			var accountID *int64
			if test.treatment == "separately_expensed" {
				accountID = &feeAccountID
			}
			base := InvestmentTradeInput{OwnerUserID: f.ownerUserID, CommodityID: f.stockCommodityID,
				HoldingAccountID: f.holdingAccountID, CashAccountID: f.cashAccountID,
				CashCommodityID: f.eurCommodityID, QuantityValue: exact.New(1),
				SettlementDate: "2026-01-03"}
			buy := base
			buy.TransactionDate = "2026-01-01"
			buy.GrossAmountValue = tradeMoney(-10000)
			buy.GrossAmountScale = 2
			buy.NetSettlementValue = tradeMoney(-10200)
			buy.NetSettlementScale = 2
			buy.Charges = []InvestmentTradeChargeInput{{Kind: "commission", AmountValue: -200,
				AmountScale: 2, CommodityID: f.eurCommodityID, Treatment: test.treatment, ChargeAccountID: accountID}}
			bought, err := f.investmentService.Buy(ctx, buy)
			require.NoError(t, err)
			require.Len(t, bought.Transaction.JournalEntries, 2)
			lots, err := f.investmentService.ListLots(ctx, f.holdingAccountID, f.stockCommodityID)
			require.NoError(t, err)
			require.Len(t, lots, 1)
			assertMoneyValue(t, test.basis, 2, lots[0].CostBasisValue, lots[0].CostBasisScale)
			var price int64
			var approximate int
			require.NoError(t, f.database.QueryRowContext(ctx, `SELECT price_value, is_approximate FROM price_observations WHERE source_transaction_version_id = ?`, bought.Transaction.VersionID).Scan(&price, &approximate))
			require.EqualValues(t, 0, approximate)
			require.EqualValues(t, 10000000000, price, "unit price must use gross 100, excluding commission")

			sell := base
			sell.TransactionDate = "2026-02-01"
			sell.SettlementDate = "2026-02-03"
			sell.GrossAmountValue = tradeMoney(12000)
			sell.GrossAmountScale = 2
			sell.NetSettlementValue = tradeMoney(11800)
			sell.NetSettlementScale = 2
			sell.Charges = buy.Charges
			preview, err := f.investmentService.PreviewSell(ctx, sell)
			require.NoError(t, err)
			assertMoneyValue(t, test.gain, 2, preview.RealizedGain, preview.RealizedGainScale)
			sold, err := f.investmentService.Sell(ctx, sell)
			require.NoError(t, err)
			require.NotNil(t, sold.DisposalDecision)
			require.Len(t, sold.DisposalDecision.Allocations, 1)
			assertMoneyValue(t, test.proceeds, 2, sold.DisposalDecision.Allocations[0].ProceedsValue, sold.DisposalDecision.Allocations[0].ProceedsScale)
			assertMoneyValue(t, test.proceeds, 2, preview.DisposalDecision.Allocations[0].ProceedsValue, preview.DisposalDecision.Allocations[0].ProceedsScale)
			gains, err := f.investmentService.ListRealizedGains(ctx, GainsReportParams{})
			require.NoError(t, err)
			require.Len(t, gains, 1)
			assertMoneyValue(t, test.proceeds, 2, gains[0].ProceedsValue, gains[0].ProceedsScale)
			assertMoneyValue(t, test.gain, 2, gains[0].RealizedGainValue, gains[0].RealizedGainScale)
			var netLinks, grossLinks, chargeLinks int
			require.NoError(t, f.database.QueryRowContext(ctx, `
				SELECT COUNT(*) FILTER (WHERE component_kind = 'net_settlement' AND posting_version_id IS NOT NULL),
					COUNT(*) FILTER (WHERE component_kind = 'gross_consideration' AND posting_version_id IS NOT NULL),
					COUNT(*) FILTER (WHERE component_kind = 'charge' AND posting_version_id IS NOT NULL)
				FROM investment_operation_components
			`).Scan(&netLinks, &grossLinks, &chargeLinks))
			require.Equal(t, 2, netLinks)
			require.Zero(t, grossLinks)
			if test.treatment == "separately_expensed" {
				require.Equal(t, 2, chargeLinks)
			} else {
				require.Zero(t, chargeLinks, "fees included in net clearing have no independent posting")
			}
			require.Equal(t, SelfCheckPassed, resultFor(t, mustRunInvestmentSelfCheck(t, f), CheckInvestmentFoundation).Status)
		})
	}
}

func negativeProceedsSale(f *investmentsTestFixture, date string) InvestmentTradeInput {
	return InvestmentTradeInput{
		OwnerUserID: f.ownerUserID, TransactionDate: date,
		CommodityID: f.stockCommodityID, HoldingAccountID: f.holdingAccountID,
		CashAccountID: f.cashAccountID, CashCommodityID: f.eurCommodityID,
		CostBasisMethod: "fifo",
		QuantityValue:   exact.New(1), GrossAmountValue: tradeMoney(50), GrossAmountScale: 2,
		NetSettlementValue: tradeMoney(-50), NetSettlementScale: 2,
		Charges: []InvestmentTradeChargeInput{{Kind: "commission", AmountValue: -100,
			AmountScale: 2, CommodityID: f.eurCommodityID, Treatment: "clearing_included"}},
	}
}

func TestNegativeSaleProceedsSurviveLaterReplayAndReplacement(t *testing.T) {
	t.Run("later sale replays after earlier reversal", func(t *testing.T) {
		f := newInvestmentsTestFixture(t)
		ctx := context.Background()
		buyOn(t, f, "2026-01-01", 2, 2000)
		older, err := f.investmentService.Sell(ctx, sellInput(f, "2026-02-01", 1))
		require.NoError(t, err)
		later, err := f.investmentService.Sell(ctx, negativeProceedsSale(f, "2026-03-01"))
		require.NoError(t, err)
		require.EqualValues(t, -50, later.DisposalDecision.ProceedsValue)
		_, err = f.investmentService.ReverseSale(ctx, ReverseInvestmentSaleInput{
			OwnerUserID: f.ownerUserID, TransactionID: older.Transaction.ID, Reason: "duplicate sale",
		})
		require.NoError(t, err)
		var revisedProceeds string
		var revisedScale int
		require.NoError(t, f.database.QueryRow(`SELECT a.proceeds_value, a.proceeds_scale
			FROM investment_disposal_revision_allocations a
			JOIN investment_disposal_revisions r ON r.id = a.revision_id
			JOIN investment_disposal_decisions d ON d.id = r.decision_id
			WHERE d.transaction_id = ? ORDER BY r.revision_seq DESC LIMIT 1`,
			later.Transaction.ID).Scan(&revisedProceeds, &revisedScale))
		require.Zero(t, exact.ScaledIntFromCoefficient(exact.Coefficient(revisedProceeds), revisedScale).Cmp(
			exact.ScaledIntFromInt64(-50, 2)))
	})
	t.Run("sale replacement keeps negative proceeds", func(t *testing.T) {
		f := newInvestmentsTestFixture(t)
		ctx := context.Background()
		buyOn(t, f, "2026-01-01", 1, 1000)
		original, err := f.investmentService.Sell(ctx, sellInput(f, "2026-02-01", 1))
		require.NoError(t, err)
		replaced, err := f.investmentService.ReplaceSale(ctx, ReplaceInvestmentSaleInput{
			OwnerUserID: f.ownerUserID, TransactionID: original.Transaction.ID,
			Reason:      "minimum commission exceeded gross proceeds",
			Replacement: negativeProceedsSale(f, "2026-02-01"),
		})
		require.NoError(t, err)
		require.EqualValues(t, -50, replaced.Replacement.DisposalDecision.ProceedsValue)
		check, err := selfCheckOver(t, f.database).RunSelfCheck(ctx, "manual")
		require.NoError(t, err)
		require.Equal(t, SelfCheckPassed, check.Status)
	})
}

func TestDisposalDecisionSequenceAllowsSharedJournalProvenance(t *testing.T) {
	f := newInvestmentsTestFixture(t)
	buyOn(t, f, "2026-01-01", 2, 2000)
	sold, err := f.investmentService.Sell(context.Background(), sellInput(f, "2026-02-01", 1))
	require.NoError(t, err)
	cloneDecision := `INSERT INTO investment_disposal_decisions
		(book_id, transaction_id, transaction_version_id, operation_id, decision_seq,
		 position_side, account_id, commodity_id, cost_commodity_id, event_date,
		 quantity_value, quantity_scale, disposed_basis_value, disposed_basis_scale,
		 proceeds_value, proceeds_scale, cost_basis_method, resolution_tier,
		 created_at, created_by_user_id, created_audit_event_id)
		SELECT book_id, transaction_id, transaction_version_id, operation_id, ?,
		 position_side, account_id, commodity_id, cost_commodity_id, event_date,
		 quantity_value, quantity_scale, disposed_basis_value, disposed_basis_scale,
		 proceeds_value, proceeds_scale, cost_basis_method, resolution_tier,
		 created_at, created_by_user_id, created_audit_event_id
		FROM investment_disposal_decisions WHERE transaction_id = ? AND decision_seq = 1`
	_, err = f.database.Exec(cloneDecision, 2, sold.Transaction.ID)
	require.NoError(t, err, "a compound operation can have another decision under the same journal version")
	_, err = f.database.Exec(cloneDecision, 2, sold.Transaction.ID)
	require.Error(t, err, "the operation and decision sequence still identify one decision")
}

func TestSelfCheckDetectsDisposalProceedsClearingMismatch(t *testing.T) {
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	buyOn(t, f, "2026-01-01", 2, 2000)
	sold, err := f.investmentService.Sell(ctx, sellInput(f, "2026-02-01", 1))
	require.NoError(t, err)
	check := resultFor(t, mustRunInvestmentSelfCheck(t, f), CheckInvestmentFoundation)
	require.Equal(t, SelfCheckPassed, check.Status)

	// Keep the cash entry balanced while changing its clearing value. A plain
	// journal-balance check cannot find the resulting gains divergence. Disable
	// the append-only guard only in this disposable test database to model a
	// damaged restore or manual SQL repair.
	_, err = f.database.ExecContext(ctx, `DROP TRIGGER posting_versions_no_update`)
	require.NoError(t, err)
	_, err = f.database.ExecContext(ctx, `
		UPDATE posting_versions SET quantity_value = '999999'
		WHERE transaction_version_id = ? AND commodity_id = ?
		AND account_id = (SELECT id FROM accounts WHERE book_id = 1 AND system_role = 'commodity_trading')
	`, sold.Transaction.VersionID, f.eurCommodityID)
	require.NoError(t, err)
	_, err = f.database.ExecContext(ctx, `
		UPDATE posting_versions SET quantity_value = '-999999'
		WHERE transaction_version_id = ? AND account_id = ? AND commodity_id = ?
	`, sold.Transaction.VersionID, f.cashAccountID, f.eurCommodityID)
	require.NoError(t, err)
	run := mustRunInvestmentSelfCheck(t, f)
	require.Equal(t, SelfCheckPassed, resultFor(t, run, CheckEntryBalance).Status)
	check = resultFor(t, run, CheckInvestmentFoundation)
	require.Equal(t, SelfCheckFailed, check.Status)
	require.Contains(t, check.Summary, "disposal proceeds disagree with posted clearing")
	require.Contains(t, check.Sample, *sold.DisposalDecision.ID)
}

func TestSelfCheckDetectsBalancedCashPostingComponentMismatch(t *testing.T) {
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	bought := buyOn(t, f, "2026-01-01", 1, 1000)
	require.Equal(t, SelfCheckPassed, resultFor(t, mustRunInvestmentSelfCheck(t, f), CheckInvestmentFoundation).Status)
	_, err := f.database.ExecContext(ctx, `DROP TRIGGER posting_versions_no_update`)
	require.NoError(t, err)
	_, err = f.database.ExecContext(ctx, `
		UPDATE posting_versions SET quantity_value = '-999999'
		WHERE transaction_version_id = ? AND account_id = ? AND commodity_id = ?
	`, bought.Transaction.VersionID, f.cashAccountID, f.eurCommodityID)
	require.NoError(t, err)
	_, err = f.database.ExecContext(ctx, `
		UPDATE posting_versions SET quantity_value = '999999'
		WHERE transaction_version_id = ? AND commodity_id = ?
		AND account_id = (SELECT id FROM accounts WHERE book_id = 1 AND system_role = 'commodity_trading')
	`, bought.Transaction.VersionID, f.eurCommodityID)
	require.NoError(t, err)
	run := mustRunInvestmentSelfCheck(t, f)
	require.Equal(t, SelfCheckPassed, resultFor(t, run, CheckEntryBalance).Status)
	check := resultFor(t, run, CheckInvestmentFoundation)
	require.Equal(t, SelfCheckFailed, check.Status)
	require.Contains(t, check.Summary, "source components disagree with posted journal legs")
}

func TestInvestmentFoundationChecksBuyThroughJournalLinkWithoutCompatibilityID(t *testing.T) {
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	bought := buyOn(t, f, "2026-01-01", 1, 1000)
	_, err := f.database.ExecContext(ctx, `DROP TRIGGER investment_operations_no_update`)
	require.NoError(t, err)
	_, err = f.database.ExecContext(ctx, `UPDATE investment_operations SET transaction_id = NULL
		WHERE id IN (SELECT operation_id FROM investment_operation_journal_links
			WHERE transaction_version_id = ?)`, bought.Transaction.VersionID)
	require.NoError(t, err)
	check := resultFor(t, mustRunInvestmentSelfCheck(t, f), CheckInvestmentFoundation)
	require.Equal(t, SelfCheckPassed, check.Status)
}

func mustRunInvestmentSelfCheck(t *testing.T, f *investmentsTestFixture) SelfCheckRun {
	t.Helper()
	run, err := selfCheckOver(t, f.database).RunSelfCheck(context.Background(), "manual")
	require.NoError(t, err)
	return run
}

func TestManualAndGrossPricesOutrankLaterApproximateTradeOnSameDate(t *testing.T) {
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	pricing := NewPricingService(db.NewPricingRepository(f.database))
	_, err := pricing.CreatePrice(ctx, PriceObservationInput{OwnerUserID: f.ownerUserID,
		BaseCommodityID: f.stockCommodityID, QuoteCommodityID: f.eurCommodityID,
		QuoteType: "manual", PriceValue: 11000000000, PriceScale: 8,
		ValuationDate: "2026-01-01", IsManual: true, MetadataJSON: `{}`})
	require.NoError(t, err)
	_, err = f.investmentService.Buy(ctx, InvestmentTradeInput{OwnerUserID: f.ownerUserID,
		TransactionDate: "2026-01-01", CommodityID: f.stockCommodityID,
		HoldingAccountID: f.holdingAccountID, CashAccountID: f.cashAccountID,
		CashCommodityID: f.eurCommodityID, QuantityValue: exact.New(1),
		CashAmountValue: 10000, CashAmountScale: 2})
	require.NoError(t, err)
	positions, err := f.investmentService.Positions(ctx)
	require.NoError(t, err)
	require.Len(t, positions, 1)
	require.Equal(t, int64(11000000000), *positions[0].LatestPriceValue)
	require.False(t, positions[0].LatestPriceApproximate)

	// On a date without a manual quote, a gross-derived quote outranks a
	// later-arriving net-only estimate.
	_, err = f.investmentService.Buy(ctx, InvestmentTradeInput{OwnerUserID: f.ownerUserID,
		TransactionDate: "2026-01-02", CommodityID: f.stockCommodityID,
		HoldingAccountID: f.holdingAccountID, CashAccountID: f.cashAccountID,
		CashCommodityID: f.eurCommodityID, QuantityValue: exact.New(1),
		GrossAmountValue: tradeMoney(-12000), GrossAmountScale: 2,
		NetSettlementValue: tradeMoney(-12000), NetSettlementScale: 2})
	require.NoError(t, err)
	_, err = f.investmentService.Buy(ctx, InvestmentTradeInput{OwnerUserID: f.ownerUserID,
		TransactionDate: "2026-01-02", CommodityID: f.stockCommodityID,
		HoldingAccountID: f.holdingAccountID, CashAccountID: f.cashAccountID,
		CashCommodityID: f.eurCommodityID, QuantityValue: exact.New(1),
		CashAmountValue: 13000, CashAmountScale: 2})
	require.NoError(t, err)
	positions, err = f.investmentService.Positions(ctx)
	require.NoError(t, err)
	require.Equal(t, int64(12000000000), *positions[0].LatestPriceValue)
	require.False(t, positions[0].LatestPriceApproximate)
}

func TestExactTradeEconomicsRejectsUnbalancedNet(t *testing.T) {
	f := newInvestmentsTestFixture(t)
	input := InvestmentTradeInput{OwnerUserID: f.ownerUserID, TransactionDate: "2026-01-01",
		CommodityID: f.stockCommodityID, HoldingAccountID: f.holdingAccountID,
		CashAccountID: f.cashAccountID, CashCommodityID: f.eurCommodityID,
		QuantityValue: exact.New(1), GrossAmountValue: tradeMoney(-10000), GrossAmountScale: 2,
		NetSettlementValue: tradeMoney(-10100), NetSettlementScale: 2,
		Charges: []InvestmentTradeChargeInput{{Kind: "commission", AmountValue: -200, AmountScale: 2, CommodityID: f.eurCommodityID}}}
	_, err := f.investmentService.Buy(context.Background(), input)
	require.ErrorContains(t, err, "gross plus charges does not equal net settlement")
	require.Zero(t, f.transactionCount(t))
}

func TestForeignTradeFeeKeepsCostCurrencyBasisAndPostsOwnCashLegs(t *testing.T) {
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	result, err := f.database.ExecContext(ctx, `INSERT INTO commodities (book_id, code, kind, is_builtin, created_at, created_by_user_id) VALUES (1, 'USD', 'currency', 1, '2026-01-01T00:00:00Z', 1)`)
	require.NoError(t, err)
	usdID, err := result.LastInsertId()
	require.NoError(t, err)
	_, err = f.database.ExecContext(ctx, `INSERT INTO commodity_versions (commodity_id, version_seq, effective_from, recorded_at, changed_by_user_id, change_reason, status, symbol, display_symbol, name, standard_scale, max_quantity_scale) VALUES (?, 1, '2026-01-01', '2026-01-01T00:00:00Z', 1, 'test', 'active', 'USD', '$', 'US dollar', 2, 6)`, usdID)
	require.NoError(t, err)
	foreignCash := seedTestAccount(t, f.database, "active", true)
	feeAccount := seedTestAccountWithClass(t, f.database, "active", true, "expense", "expense")
	input := InvestmentTradeInput{OwnerUserID: f.ownerUserID, TransactionDate: "2026-01-01",
		CommodityID: f.stockCommodityID, HoldingAccountID: f.holdingAccountID,
		CashAccountID: f.cashAccountID, CashCommodityID: f.eurCommodityID,
		QuantityValue: exact.New(1), GrossAmountValue: tradeMoney(-10000), GrossAmountScale: 2,
		NetSettlementValue: tradeMoney(-10000), NetSettlementScale: 2,
		Charges: []InvestmentTradeChargeInput{{Kind: "commission", AmountValue: -500, AmountScale: 2,
			CommodityID: usdID, ChargeAccountID: &feeAccount, CashAccountID: &foreignCash, PaidOn: "2026-01-04"}}}
	trade, err := f.investmentService.Buy(ctx, input)
	require.NoError(t, err)
	require.Len(t, trade.Transaction.JournalEntries, 2)
	lots, err := f.investmentService.ListLots(ctx, f.holdingAccountID, f.stockCommodityID)
	require.NoError(t, err)
	assertMoneyValue(t, 10000, 2, lots[0].CostBasisValue, lots[0].CostBasisScale)
	var cashLegs, expenseLegs int
	for _, entry := range trade.Transaction.JournalEntries {
		for _, posting := range entry.Postings {
			if posting.CommodityID != usdID {
				continue
			}
			if posting.AccountID == foreignCash && posting.QuantityValue.String() == "-500" {
				cashLegs++
			}
			if posting.AccountID == feeAccount && posting.QuantityValue.String() == "500" {
				expenseLegs++
			}
		}
	}
	require.Equal(t, 1, cashLegs)
	require.Equal(t, 1, expenseLegs)
	var treatment, tier string
	require.NoError(t, f.database.QueryRowContext(ctx, `SELECT charge_treatment, resolution_tier FROM investment_operation_components WHERE component_kind = 'charge'`).Scan(&treatment, &tier))
	require.Equal(t, "separately_expensed", treatment)
	require.Equal(t, "fallback", tier)
	var separatelyPaid int
	var postingID int64
	require.NoError(t, f.database.QueryRowContext(ctx, `
		SELECT separately_paid, posting_version_id FROM investment_operation_components WHERE component_kind = 'charge'
	`).Scan(&separatelyPaid, &postingID))
	require.Equal(t, 1, separatelyPaid)
	require.Positive(t, postingID)
	require.Equal(t, SelfCheckPassed, resultFor(t, mustRunInvestmentSelfCheck(t, f), CheckInvestmentFoundation).Status)
}

func TestSeparatelyPaidSameCurrencyCommissionPostsOnPaymentDate(t *testing.T) {
	for _, test := range []struct {
		treatment string
		basis     int64
	}{
		{"clearing_included", 10200}, {"separately_expensed", 10000},
	} {
		t.Run(test.treatment, func(t *testing.T) {
			f := newInvestmentsTestFixture(t)
			ctx := context.Background()
			feeAccount := seedTestAccountWithClass(t, f.database, "active", true, "expense", "expense")
			var accountID *int64
			if test.treatment == "separately_expensed" {
				accountID = &feeAccount
			}
			trade, err := f.investmentService.Buy(ctx, InvestmentTradeInput{
				OwnerUserID: f.ownerUserID, TransactionDate: "2026-01-01", CommodityID: f.stockCommodityID,
				HoldingAccountID: f.holdingAccountID, CashAccountID: f.cashAccountID,
				CashCommodityID: f.eurCommodityID, QuantityValue: exact.New(1),
				GrossAmountValue: tradeMoney(-10000), GrossAmountScale: 2,
				NetSettlementValue: tradeMoney(-10000), NetSettlementScale: 2,
				Charges: []InvestmentTradeChargeInput{{Kind: "commission", AmountValue: -200,
					AmountScale: 2, CommodityID: f.eurCommodityID, Treatment: test.treatment,
					ChargeAccountID: accountID, CashAccountID: &f.cashAccountID, PaidOn: "2026-01-03"}},
			})
			require.NoError(t, err)
			require.Len(t, trade.Transaction.JournalEntries, 2)
			require.Equal(t, "2026-01-03", trade.Transaction.JournalEntries[1].EntryDate)
			lots, err := f.investmentService.ListLots(ctx, f.holdingAccountID, f.stockCommodityID)
			require.NoError(t, err)
			assertMoneyValue(t, test.basis, 2, lots[0].CostBasisValue, lots[0].CostBasisScale)
			var cashAccountID int64
			require.NoError(t, f.database.QueryRowContext(ctx, `SELECT cash_account_id FROM investment_operation_components WHERE component_kind = 'charge'`).Scan(&cashAccountID))
			require.Equal(t, f.cashAccountID, cashAccountID)
			var settlementCount int
			require.NoError(t, f.database.QueryRowContext(ctx, `SELECT count(*) FROM investment_operation_components WHERE component_kind = 'net_settlement'`).Scan(&settlementCount))
			require.Equal(t, 2, settlementCount, "main settlement and separately paid fee retain distinct source cash facts")
		})
	}
}

func TestTradeFeePolicySnapshotsAccountVersionAndOldTradeDoesNotChange(t *testing.T) {
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	feeAccount := seedTestAccountWithClass(t, f.database, "active", true, "expense", "expense")
	result, err := f.database.ExecContext(ctx, `INSERT INTO investment_fee_policies (book_id, account_id, charge_kind, created_at) VALUES (1, ?, 'commission', '2026-01-01T00:00:00Z')`, f.holdingAccountID)
	require.NoError(t, err)
	policyID, err := result.LastInsertId()
	require.NoError(t, err)
	result, err = f.database.ExecContext(ctx, `INSERT INTO investment_fee_policy_versions (policy_id, version_seq, effective_from, treatment, charge_account_id, recorded_at) VALUES (?, 1, '2026-01-01', 'separately_expensed', ?, '2026-01-01T00:00:00Z')`, policyID, feeAccount)
	require.NoError(t, err)
	versionID, err := result.LastInsertId()
	require.NoError(t, err)
	trade, err := f.investmentService.Buy(ctx, InvestmentTradeInput{OwnerUserID: f.ownerUserID,
		TransactionDate: "2026-01-02", CommodityID: f.stockCommodityID,
		HoldingAccountID: f.holdingAccountID, CashAccountID: f.cashAccountID,
		CashCommodityID: f.eurCommodityID, QuantityValue: exact.New(1),
		GrossAmountValue: tradeMoney(-10000), GrossAmountScale: 2,
		NetSettlementValue: tradeMoney(-10200), NetSettlementScale: 2,
		Charges: []InvestmentTradeChargeInput{{Kind: "commission", AmountValue: -200, AmountScale: 2, CommodityID: f.eurCommodityID}}})
	require.NoError(t, err)
	var treatment, tier string
	var storedVersionID int64
	require.NoError(t, f.database.QueryRowContext(ctx, `SELECT charge_treatment, resolution_tier, fee_policy_version_id FROM investment_operation_components WHERE component_kind = 'charge'`).Scan(&treatment, &tier, &storedVersionID))
	require.Equal(t, "separately_expensed", treatment)
	require.Equal(t, "account", tier)
	require.Equal(t, versionID, storedVersionID)
	_, err = f.database.ExecContext(ctx, `INSERT INTO investment_fee_policy_versions (policy_id, version_seq, effective_from, treatment, recorded_at) VALUES (?, 2, '2026-01-03', 'clearing_included', '2026-01-03T00:00:00Z')`, policyID)
	require.NoError(t, err)
	lots, err := f.investmentService.ListLots(ctx, f.holdingAccountID, f.stockCommodityID)
	require.NoError(t, err)
	require.Len(t, lots, 1)
	assertMoneyValue(t, 10000, 2, lots[0].CostBasisValue, lots[0].CostBasisScale)
	require.NotNil(t, trade.LotID)
}
