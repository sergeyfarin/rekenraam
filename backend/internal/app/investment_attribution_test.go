package app

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"rekenraam/backend/internal/db"
	"rekenraam/backend/internal/exact"
)

// All diagnostic controls now use ordinary owner commands (#388).
func attributionFixturePayment(t *testing.T, f *investmentsTestFixture, input PaymentInLieuInput) Transaction {
	t.Helper()
	payment, err := f.investmentService.PaymentInLieu(context.Background(), input)
	require.NoError(t, err)
	return payment
}

func TestPaymentAttributionFullCoverWarningPreservesCashAndHistory(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := newInvestmentsTestFixture(t)
	expense := seedTestAccountWithClass(t, f.database, "active", true, "expense", "expense")
	shortSaleOn(t, f, "2026-03-02", 10, 100000)
	_, err := f.investmentService.ShortCover(ctx, coverInput(f, "2026-03-31", 10, 70000, "fifo"))
	require.NoError(t, err)
	payment := attributionFixturePayment(t, f, paymentInLieuInput(f, expense, 5000))
	require.Equal(t, int64(25000), accountBalance(t, f, f.cashAccountID), "1000 proceeds - 700 cover - 50 charge")
	require.Equal(t, int64(5000), accountBalance(t, f, expense))
	before := buyReplacementPreviewSnapshot(t, f.database)
	run := mustRunInvestmentSelfCheck(t, f)
	require.Equal(t, before, buyReplacementPreviewSnapshot(t, f.database), "diagnostic writes no ledger or investment facts")
	require.Equal(t, SelfCheckWarning, run.Status)
	require.Zero(t, run.FailedCheckCount)
	require.EqualValues(t, 1, run.WarningCheckCount)
	result := resultFor(t, run, CheckInvestmentAttribution)
	require.Equal(t, []SelfCheckWarningSample{{TransactionID: payment.ID, Reason: db.AttributionNoShort}}, result.WarningSample)
	require.Zero(t, result.FindingCount)
	require.Zero(t, result.UnavailableCount)
	require.Equal(t, SelfCheckPassed, resultFor(t, run, CheckInvestmentFoundation).Status, "old opening-existence check missed this case")
	require.Equal(t, SelfCheckPassed, resultFor(t, run, CheckInvestmentReplay).Status)
	chain, err := f.investmentService.CorrectionChain(ctx, f.ownerUserID, payment.ID)
	require.NoError(t, err)
	require.Equal(t, InvestmentAttributionDiagnostic{Status: SelfCheckWarning, Reasons: []string{db.AttributionNoShort}}, chain.EffectivePaymentInLieu.Attribution)
	latest, ok, err := NewSelfCheckService(db.NewSelfCheckRepository(f.database, f.database)).LatestSelfCheck(ctx)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, SelfCheckWarning, latest.Status)
	require.Equal(t, result.WarningSample, resultFor(t, latest, CheckInvestmentAttribution).WarningSample)
	_, err = f.investmentService.ReversePaymentInLieu(ctx, ReverseInvestmentPaymentInLieuInput{OwnerUserID: f.ownerUserID, TransactionID: payment.ID, Reason: "not owed"})
	require.NoError(t, err)
	requireInvestmentSelfCheckPasses(t, f)
	// Later checks do not restate the original saved warning.
	var oldStatus string
	require.NoError(t, f.database.QueryRow(`SELECT status FROM self_check_runs WHERE id = ?`, run.ID).Scan(&oldStatus))
	require.Equal(t, SelfCheckWarning, oldStatus)
}

func TestPaymentAttributionEffectiveHistoryOnReadOnlyPoolMatchesReplay(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	for _, tc := range []struct {
		name   string
		owed   int64
		change func(*testing.T, *investmentsTestFixture, InvestmentTradeResult)
	}{
		{"partial cover", 6, func(t *testing.T, f *investmentsTestFixture, _ InvestmentTradeResult) {
			_, err := f.investmentService.ShortCover(ctx, coverInput(f, "2026-03-20", 4, 28000, "fifo"))
			require.NoError(t, err)
		}},
		{"cover on ex-date", 10, func(t *testing.T, f *investmentsTestFixture, _ InvestmentTradeResult) {
			_, err := f.investmentService.ShortCover(ctx, coverInput(f, "2026-04-01", 10, 70000, "fifo"))
			require.NoError(t, err)
		}},
		{"opening reversed", 0, func(t *testing.T, f *investmentsTestFixture, opening InvestmentTradeResult) {
			_, err := f.investmentService.ReverseShortSale(ctx, ReverseInvestmentBuyInput{OwnerUserID: f.ownerUserID, TransactionID: opening.Transaction.ID, Reason: "not shorted"})
			require.NoError(t, err)
		}},
		{"opening replaced after ex-date", 0, func(t *testing.T, f *investmentsTestFixture, opening InvestmentTradeResult) {
			_, err := f.investmentService.ReplaceShortSale(ctx, ReplaceInvestmentBuyInput{OwnerUserID: f.ownerUserID, TransactionID: opening.Transaction.ID, Reason: "date", Replacement: shortInput(f, "2026-04-02", 10, 100000)})
			require.NoError(t, err)
		}},
		{"cover reversed", 10, func(t *testing.T, f *investmentsTestFixture, _ InvestmentTradeResult) {
			cover, err := f.investmentService.ShortCover(ctx, coverInput(f, "2026-03-20", 10, 70000, "fifo"))
			require.NoError(t, err)
			reverseShortCoverAcknowledged(t, f, cover.Transaction.ID, "not covered")
		}},
		{"cover replaced before ex-date", 0, func(t *testing.T, f *investmentsTestFixture, _ InvestmentTradeResult) {
			cover, err := f.investmentService.ShortCover(ctx, coverInput(f, "2026-04-02", 10, 70000, "fifo"))
			require.NoError(t, err)
			input := ReplaceInvestmentSaleInput{OwnerUserID: f.ownerUserID, TransactionID: cover.Transaction.ID, Reason: "date", Replacement: coverInput(f, "2026-03-31", 10, 70000, "fifo")}
			impact, err := f.investmentService.ReplaceShortCoverReconciliationImpact(ctx, input)
			require.NoError(t, err)
			if impact.GainImpact != nil {
				input.GainImpactAcknowledgement = impact.GainImpact.Acknowledgement
			}
			_, err = f.investmentService.ReplaceShortCover(ctx, input)
			require.NoError(t, err)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newInvestmentsTestFixture(t)
			expense := seedTestAccountWithClass(t, f.database, "active", true, "expense", "expense")
			opening := shortSaleOn(t, f, "2026-03-02", 10, 100000)
			tc.change(t, f, opening)
			payment := attributionFixturePayment(t, f, paymentInLieuInput(f, expense, 5000))
			// A separate replay oracle verifies the hand-derived owed quantity;
			// it does not use EffectiveLotMovements or the diagnostic helper.
			replayed, err := db.NewPricingRepository(f.database).DividendEntitlementHoldings(ctx, BookID, f.stockCommodityID, "2026-03-31")
			require.NoError(t, err)
			total := exact.NewScaledInt()
			for _, holding := range replayed.Short {
				total.AddCoefficient(holding.QuantityValue, holding.QuantityScale)
			}
			require.Zero(t, total.Cmp(exact.ScaledIntFromInt64(tc.owed, 0)))
			var sequence int
			var name, path string
			require.NoError(t, f.database.QueryRow(`PRAGMA database_list`).Scan(&sequence, &name, &path))
			readOnly, err := db.OpenReadOnly(ctx, db.SQLiteFileURL(path))
			require.NoError(t, err)
			defer readOnly.Close()
			repo := db.NewSelfCheckRepository(f.database, readOnly)
			snapshot, err := repo.Snapshot(ctx)
			require.NoError(t, err)
			_, err = snapshot.ExecContext(ctx, `CREATE TABLE forbidden_attribution_write (id INTEGER)`)
			require.Error(t, err, "the pool really refuses writes")
			records, err := repo.PaymentInLieuAttributions(ctx, snapshot, BookID)
			require.NoError(t, err)
			require.Len(t, records, 1)
			require.Equal(t, payment.ID, records[0].TransactionID)
			require.False(t, records[0].Unavailable)
			require.Equal(t, tc.owed == 0, len(records[0].WarningReasons) > 0)
			require.NoError(t, snapshot.Rollback())
			_, err = repo.PaymentInLieuAttributions(ctx, snapshot, BookID)
			require.Error(t, err, "execution failure never becomes unavailable or zero")
		})
	}
}

func TestPaymentAttributionTwoCostCurrencies(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := newInvestmentsTestFixture(t)
	expense := seedTestAccountWithClass(t, f.database, "active", true, "expense", "expense")
	shortSaleOn(t, f, "2026-03-02", 10, 100000)
	_, err := f.investmentService.ShortCover(ctx, coverInput(f, "2026-03-20", 10, 70000, "fifo"))
	require.NoError(t, err)
	usd := seedTestCurrencyCommodity(t, f.database, "USD")
	input := shortInput(f, "2026-03-21", 5, 50000)
	input.CashCommodityID = usd
	_, err = f.investmentService.ShortSale(ctx, input)
	require.NoError(t, err)
	attributionFixturePayment(t, f, paymentInLieuInput(f, expense, 5000))
	replayed, err := db.NewPricingRepository(f.database).DividendEntitlementHoldings(ctx, BookID, f.stockCommodityID, "2026-03-31")
	require.NoError(t, err)
	require.Len(t, replayed.Short, 1, "EUR group is zero; USD group independently owes 5")
	require.Equal(t, usd, replayed.Short[0].CostCommodityID)
	require.Zero(t, exact.ScaledIntFromCoefficient(replayed.Short[0].QuantityValue, replayed.Short[0].QuantityScale).Cmp(exact.ScaledIntFromInt64(5, 0)))
	require.Equal(t, SelfCheckPassed, resultFor(t, mustRunInvestmentSelfCheck(t, f), CheckInvestmentAttribution).Status)
}

func TestSelfCheckWarningsRemainVisibleAlongsideFailureAndUnavailable(t *testing.T) {
	t.Parallel()
	result := investmentAttributionResult([]db.InvestmentAttributionDiagnostic{
		{TransactionID: 1, WarningReasons: []string{db.AttributionNoShort}},
		{TransactionID: 2, Unavailable: true},
	})
	require.Equal(t, SelfCheckWarning, result.Status)
	require.Equal(t, []int64{2}, result.UnavailableSample)
	status, failed, warnings, incomplete := aggregateSelfCheck([]SelfCheckResult{result})
	require.Equal(t, SelfCheckWarning, status)
	require.Zero(t, failed)
	require.EqualValues(t, 1, warnings)
	require.EqualValues(t, 1, incomplete)
	result.FindingCount = 1
	result.Status = resultStatus(result)
	status, failed, warnings, incomplete = aggregateSelfCheck([]SelfCheckResult{result})
	require.Equal(t, SelfCheckFailed, status)
	require.EqualValues(t, 1, failed)
	require.EqualValues(t, 1, warnings)
	require.EqualValues(t, 1, incomplete)
}

func TestPaymentAttributionDoesNotReclassifyBrokenFactsAsWarnings(t *testing.T) {
	t.Parallel()
	for name, damage := range map[string]string{
		"missing facts": `DROP TRIGGER investment_payment_in_lieu_facts_no_delete; DELETE FROM investment_payment_in_lieu_facts`,
		"malformed date": `
			DROP TRIGGER investment_payment_in_lieu_facts_no_update;
			PRAGMA ignore_check_constraints = ON;
			UPDATE investment_payment_in_lieu_facts SET ex_date = '2026-02-31';
			PRAGMA ignore_check_constraints = OFF;
		`,
		"wrong object kind": `DROP TRIGGER investment_payment_in_lieu_facts_no_update; UPDATE investment_payment_in_lieu_facts SET commodity_id = (SELECT id FROM commodities WHERE kind = 'currency' LIMIT 1)`,
	} {
		t.Run(name, func(t *testing.T) {
			f := newInvestmentsTestFixture(t)
			expense := seedTestAccountWithClass(t, f.database, "active", true, "expense", "expense")
			shortSaleOn(t, f, "2026-03-02", 10, 100000)
			_, err := f.investmentService.PaymentInLieu(context.Background(), paymentInLieuInput(f, expense, 5000))
			require.NoError(t, err)
			_, err = f.database.Exec(damage)
			require.NoError(t, err)
			run := mustRunInvestmentSelfCheck(t, f)
			require.Equal(t, SelfCheckFailed, run.Status)
			require.Equal(t, SelfCheckFailed, resultFor(t, run, CheckInvestmentFoundation).Status)
			attribution := resultFor(t, run, CheckInvestmentAttribution)
			require.Zero(t, attribution.WarningCount)
			require.Zero(t, attribution.UnavailableCount, "broken facts are not missing owner context")
		})
	}
}
