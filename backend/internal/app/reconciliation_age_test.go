package app

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rekenraam/backend/internal/db"
	"rekenraam/backend/internal/exact"
)

// #482: one reconciliation out-of-date rule for the account list, Home and
// any other place that judges reconciliation age.

func TestReconciliationAgeStateAtWindowBoundary(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		fact db.ReconciliationCommodityFact
		want string
	}{
		{"statement exactly the window ago is within it", db.ReconciliationCommodityFact{LatestStatementDate: "2026-04-01", HasPostedPostings: true}, ReconciliationAgeReconciled},
		{"one day older is out of date", db.ReconciliationCommodityFact{LatestStatementDate: "2026-03-31", HasPostedPostings: true}, ReconciliationAgeOutOfDate},
		{"statement today", db.ReconciliationCommodityFact{LatestStatementDate: "2026-05-31", HasPostedPostings: true}, ReconciliationAgeReconciled},
		{"posted activity and no checkpoint", db.ReconciliationCommodityFact{HasPostedPostings: true}, ReconciliationAgeNeverReconciled},
		{"no activity and no checkpoint is never out of date", db.ReconciliationCommodityFact{}, ReconciliationAgeNothingToReconcile},
		{"an old checkpoint without later activity is still judged by its date", db.ReconciliationCommodityFact{LatestStatementDate: "2026-01-01"}, ReconciliationAgeOutOfDate},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := reconciliationAgeState(tc.fact, "2026-05-31", 60)
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func commodityFacts(facts ...db.ReconciliationCommodityFact) []db.ReconciliationCommodityFact {
	return facts
}

func TestReconciliationAgesAreWeakestFirst(t *testing.T) {
	t.Parallel()
	ages, err := judgeReconciliationAges([]db.ReconciliationAccountFact{
		{AccountID: 1, Commodities: commodityFacts(db.ReconciliationCommodityFact{CommodityID: 1, LatestStatementDate: "2026-05-01", HasPostedPostings: true})},
		{AccountID: 2, Commodities: commodityFacts()},
		{AccountID: 3, Commodities: commodityFacts(db.ReconciliationCommodityFact{CommodityID: 1, LatestStatementDate: "2026-01-01", HasPostedPostings: true})},
		{AccountID: 4, Commodities: commodityFacts(db.ReconciliationCommodityFact{CommodityID: 1, HasPostedPostings: true})},
	}, "2026-05-31", 60)
	require.NoError(t, err)
	ids := []int64{}
	for _, account := range ages.Accounts {
		ids = append(ids, account.AccountID)
	}
	assert.Equal(t, []int64{4, 3, 1, 2}, ids, "never reconciled, out of date, reconciled, then nothing to reconcile")
	assert.Equal(t, ReconciliationAgeNothingToReconcile, ages.ByAccount()[2].State)
}

// #594 review: a checkpoint covers one commodity. Reconciling EUR says
// nothing about USD in the same account.
func TestReconciledCurrencyNeverVouchesForAnother(t *testing.T) {
	t.Parallel()
	ages, err := judgeReconciliationAges([]db.ReconciliationAccountFact{{AccountID: 1, Commodities: commodityFacts(
		db.ReconciliationCommodityFact{CommodityID: 1, CommodityCode: "EUR", LatestStatementDate: "2026-05-20", HasPostedPostings: true},
		db.ReconciliationCommodityFact{CommodityID: 2, CommodityCode: "USD", HasPostedPostings: true},
		db.ReconciliationCommodityFact{CommodityID: 3, CommodityCode: "GBP", LatestStatementDate: "2026-05-01", HasPostedPostings: true},
	)}}, "2026-05-31", 60)
	require.NoError(t, err)
	account := ages.Accounts[0]
	assert.Equal(t, ReconciliationAgeNeverReconciled, account.State, "the weakest commodity decides")
	codes := []string{}
	for _, commodity := range account.Commodities {
		codes = append(codes, commodity.CommodityCode+":"+commodity.State)
	}
	assert.Equal(t, []string{"USD:never_reconciled", "GBP:reconciled", "EUR:reconciled"}, codes)

	ages, err = judgeReconciliationAges([]db.ReconciliationAccountFact{{AccountID: 1, Commodities: commodityFacts(
		db.ReconciliationCommodityFact{CommodityID: 1, LatestStatementDate: "2026-05-20", HasPostedPostings: true},
		db.ReconciliationCommodityFact{CommodityID: 3, LatestStatementDate: "2026-05-01", HasPostedPostings: true},
	)}}, "2026-05-31", 60)
	require.NoError(t, err)
	assert.Equal(t, ReconciliationAgeReconciled, ages.Accounts[0].State)
	assert.Equal(t, "2026-05-01", ages.Accounts[0].LastStatementDate, "coverage reaches only as far as the oldest commodity")
}

func TestReconciliationPolicyDefaultsAndValidates(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := newInvestmentsTestFixture(t)

	policy, err := f.transactionService.ReconciliationPolicy(ctx)
	require.NoError(t, err)
	assert.Equal(t, DefaultReconciliationOutOfDateAfterDays, policy.OutOfDateAfterDays)
	assert.Empty(t, policy.UpdatedAt, "a book that never saved the window reads the default")

	for _, days := range []int{0, -1, MaxReconciliationOutOfDateAfterDays + 1} {
		_, err := f.transactionService.SaveReconciliationPolicy(ctx, SaveReconciliationPolicyInput{OwnerUserID: f.ownerUserID, OutOfDateAfterDays: days})
		var validation ValidationError
		require.ErrorAs(t, err, &validation, "%d days", days)
	}

	saved, err := f.transactionService.SaveReconciliationPolicy(ctx, SaveReconciliationPolicyInput{OwnerUserID: f.ownerUserID, OutOfDateAfterDays: 90})
	require.NoError(t, err)
	assert.Equal(t, 90, saved.OutOfDateAfterDays)
	assert.NotEmpty(t, saved.UpdatedAt)
	_, err = f.transactionService.SaveReconciliationPolicy(ctx, SaveReconciliationPolicyInput{OwnerUserID: f.ownerUserID, OutOfDateAfterDays: MaxReconciliationOutOfDateAfterDays})
	require.NoError(t, err)

	var rows, audits int
	require.NoError(t, f.database.QueryRowContext(ctx, `SELECT COUNT(*) FROM reconciliation_policies`).Scan(&rows))
	require.NoError(t, f.database.QueryRowContext(ctx, `SELECT COUNT(*) FROM audit_events WHERE operation = 'reconciliation.policy.save'`).Scan(&audits))
	assert.Equal(t, 1, rows, "one window per book")
	assert.Equal(t, 2, audits, "each save is audited")
}

// The judgement reads the stored window, the latest active checkpoint and
// posted activity, on owner-local today, and covers only accounts that can be
// reconciled.
func TestAccountReconciliationAgesJudgeTheBookOnOwnerLocalToday(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := newInvestmentsTestFixture(t)
	settings := NewSettingsService(db.NewSettingsRepository(f.database))
	f.transactionService.SetSettings(settings)
	_, err := settings.SavePreferences(ctx, SaveUserPreferencesInput{UserID: f.ownerUserID, TimeZone: "Europe/Amsterdam", ChangeReason: "test"})
	require.NoError(t, err)

	postEUR(t, f, "2026-03-15", 100)
	reconcileCash(t, f, "2026-04-01", 100)
	savings := seedTestAccountWithClass(t, f.database, "active", true, "asset", "savings")
	_, err = f.transactionService.CreateTransaction(ctx, CreateTransactionInput{
		OwnerUserID: f.ownerUserID, OriginType: "browser_api",
		Spec: TransactionInput{TransactionDate: "2026-03-20", JournalEntries: []JournalEntryInput{{Postings: []PostingInput{
			{AccountID: savings, CommodityID: f.eurCommodityID, QuantityValue: exact.New(5)},
			{AccountID: f.incomeAccountID, CommodityID: f.eurCommodityID, QuantityValue: exact.New(-5)},
		}}}},
	})
	require.NoError(t, err)
	empty := seedTestAccountWithClass(t, f.database, "active", true, "liability", "credit_card")
	// A starter account has no name; the judgement must still read (e2e found
	// the NULL name failing the whole account list).
	_, err = f.database.ExecContext(ctx, `
		INSERT INTO account_versions (
			account_id, version_seq, effective_from, recorded_at, changed_by_user_id, change_reason, status, opened_on, closed_on,
			code, name, account_class, account_kind, allows_postings)
		SELECT account_id, version_seq + 1, effective_from, '2026-01-02T00:00:00Z', 1, 'unnamed', status, opened_on, closed_on,
			'CARD', NULL, account_class, account_kind, allows_postings
		FROM account_versions WHERE account_id = ? ORDER BY version_seq DESC LIMIT 1`, empty)
	require.NoError(t, err)
	seedTestAccountWithClass(t, f.database, "closed", true, "asset", "checking")
	seedTestAccountWithClass(t, f.database, "active", false, "asset", "checking")

	judge := func(now time.Time) map[int64]AccountReconciliationAge {
		t.Helper()
		f.transactionService.SetNowForTest(func() time.Time { return now })
		ages, err := f.transactionService.AccountReconciliationAges(ctx, f.ownerUserID)
		require.NoError(t, err)
		return ages.ByAccount()
	}

	// 2026-05-31 22:30 UTC is 2026-06-01 in Amsterdam: one day past the window.
	before := judge(time.Date(2026, 5, 31, 21, 30, 0, 0, time.UTC))
	assert.Equal(t, ReconciliationAgeReconciled, before[f.cashAccountID].State, "2026-05-31 local: exactly 60 days")
	after := judge(time.Date(2026, 5, 31, 22, 30, 0, 0, time.UTC))
	assert.Equal(t, ReconciliationAgeOutOfDate, after[f.cashAccountID].State, "2026-06-01 local: owner-local today, not UTC")
	assert.Equal(t, "2026-04-01", after[f.cashAccountID].LastStatementDate)
	assert.Equal(t, ReconciliationAgeNeverReconciled, after[savings].State)
	assert.Equal(t, ReconciliationAgeNothingToReconcile, after[empty].State, "an account without postings is not out of date")
	assert.Equal(t, "", after[empty].Name)
	assert.Equal(t, "CARD", after[empty].Code, "an unnamed account is labelled by its code")
	assert.Len(t, after, 3, "holdings, categories, closed and non-posting accounts cannot be reconciled")
	assert.NotContains(t, after, f.holdingAccountID)
	assert.NotContains(t, after, f.incomeAccountID)

	_, err = f.transactionService.SaveReconciliationPolicy(ctx, SaveReconciliationPolicyInput{OwnerUserID: f.ownerUserID, OutOfDateAfterDays: 61})
	require.NoError(t, err)
	assert.Equal(t, ReconciliationAgeReconciled, judge(time.Date(2026, 5, 31, 22, 30, 0, 0, time.UTC))[f.cashAccountID].State, "the setting moves the judgement")

	// #594 review: an account closed on the owner's today (local 06-01,
	// still 05-31 in UTC) can no longer be reconciled, so it is not judged.
	closing := seedTestAccountWithClass(t, f.database, "active", true, "asset", "checking")
	_, err = f.database.ExecContext(ctx, `
		INSERT INTO account_versions (
			account_id, version_seq, effective_from, recorded_at, changed_by_user_id, change_reason, status, opened_on, closed_on,
			code, name, account_class, account_kind, allows_postings)
		SELECT account_id, version_seq + 1, '2026-06-01', '2026-05-31T22:00:00Z', 1, 'closed', 'closed', opened_on, '2026-06-01',
			code, name, account_class, account_kind, allows_postings
		FROM account_versions WHERE account_id = ? ORDER BY version_seq DESC LIMIT 1`, closing)
	require.NoError(t, err)
	assert.Contains(t, judge(time.Date(2026, 5, 31, 21, 30, 0, 0, time.UTC)), closing, "still open on local 05-31")
	assert.NotContains(t, judge(time.Date(2026, 5, 31, 22, 30, 0, 0, time.UTC)), closing, "closed on local 06-01, though UTC is still 05-31")

	checkpoints := activeCheckpointIDs(t, f)
	require.Len(t, checkpoints, 1)
	voidCashCheckpoint(t, f, checkpoints[0])
	assert.Equal(t, ReconciliationAgeNeverReconciled, judge(time.Date(2026, 5, 31, 22, 30, 0, 0, time.UTC))[f.cashAccountID].State, "a voided checkpoint never counts")
}

// Home's widget is the same judgement counted: accounts with nothing to
// reconcile are in neither figure, and the oldest come first.
func TestHomeReconciliationWidgetCountsTheAccountListRule(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := newHomeWidgetFixture(t)
	savings := f.account(t, "Savings", "asset", "savings")
	f.account(t, "Card", "liability", "credit_card")
	f.move(t, "2026-08-01", f.cashAccountID, f.eurCommodityID, 10000)
	f.move(t, "2026-08-02", savings, f.eurCommodityID, 500)
	reconcileCash(t, f.investmentsTestFixture, "2026-09-30", 100)

	home := f.read(t)
	require.NoError(t, home.Reconciliation.Err)
	summary := home.Reconciliation.Summary
	require.NotNil(t, summary)
	assert.Equal(t, 60, summary.OutOfDateAfterDays)
	assert.Equal(t, 2, summary.AccountCount, "the empty card has nothing to reconcile")
	assert.Equal(t, 1, summary.ReconciledCount)
	assert.Equal(t, 1, summary.NeverReconciledCount)
	assert.Zero(t, summary.OutOfDateCount)
	require.Len(t, summary.Accounts, 2)
	assert.Equal(t, "Savings", summary.Accounts[0].Name)
	assert.Equal(t, ReconciliationAgeNeverReconciled, summary.Accounts[0].State)
	assert.Equal(t, "Checking", summary.Accounts[1].Name)

	_, err := f.transactionService.SaveReconciliationPolicy(ctx, SaveReconciliationPolicyInput{OwnerUserID: 1, OutOfDateAfterDays: 14})
	require.NoError(t, err)
	summary = f.read(t).Reconciliation.Summary
	require.NotNil(t, summary)
	assert.Equal(t, 14, summary.OutOfDateAfterDays)
	assert.Zero(t, summary.ReconciledCount, "2026-09-30 is 15 days before 2026-10-15")
	assert.Equal(t, 1, summary.OutOfDateCount)
}

// #594 review: an account with USD activity is not reconciled by its EUR
// statement, and a statement dated after today never vouches for today.
func TestHomeReconciliationJudgesEachCurrencyAndIgnoresLaterStatements(t *testing.T) {
	t.Parallel()
	f := newHomeWidgetFixture(t)
	f.move(t, "2026-09-01", f.cashAccountID, f.eurCommodityID, 10000)
	f.move(t, "2026-09-02", f.cashAccountID, f.usd, 2500)
	reconcileCash(t, f.investmentsTestFixture, "2026-09-30", 100)

	summary := f.read(t).Reconciliation.Summary
	require.NotNil(t, summary)
	assert.Zero(t, summary.ReconciledCount, "USD was never reconciled")
	assert.Equal(t, 1, summary.NeverReconciledCount)
	require.Len(t, summary.Accounts, 1)
	require.Len(t, summary.Accounts[0].Commodities, 2)
	assert.Equal(t, "USD", summary.Accounts[0].Commodities[0].CommodityCode)
	assert.Equal(t, ReconciliationAgeNeverReconciled, summary.Accounts[0].Commodities[0].State)
	assert.Equal(t, ReconciliationAgeReconciled, summary.Accounts[0].Commodities[1].State)

	later := newHomeWidgetFixture(t)
	later.move(t, "2026-09-01", later.cashAccountID, later.eurCommodityID, 10000)
	reconcileCash(t, later.investmentsTestFixture, "2027-12-31", 100)
	summary = later.read(t).Reconciliation.Summary
	require.NotNil(t, summary)
	assert.Zero(t, summary.ReconciledCount, "a statement dated 2027-12-31 does not count on 2026-10-15")
	assert.Equal(t, 1, summary.NeverReconciledCount)
}

// #594 review / #592: a brokerage's folded cash account has no row of its
// own, so its judgement travels on the brokerage row.
func TestLedgerOverviewCarriesFoldedAccountReconciliation(t *testing.T) {
	t.Parallel()
	f := newHomeWidgetFixture(t)
	broker := f.account(t, "Broker", "asset", "brokerage")
	cashChild := f.account(t, "Broker cash", "asset", "brokerage_cash")
	reviseHomeAccount(t, f.homeFixture, cashChild, "Broker cash", &broker)
	f.move(t, "2026-09-10", cashChild, f.eurCommodityID, 50000)
	service := NewLedgerService(f.transactionService, f.accountService, NewInstitutionService(db.NewInstitutionRepository(f.database)))
	result, err := service.Overview(context.Background(), LedgerOverviewInput{OwnerUserID: 1})
	require.NoError(t, err)
	for _, row := range result.Accounts {
		assert.NotEqual(t, cashChild, row.Account.ID, "the cash account is folded into the brokerage")
		if row.Account.ID != broker {
			continue
		}
		require.Len(t, row.FoldedReconciliations, 1)
		assert.Equal(t, cashChild, row.FoldedReconciliations[0].AccountID)
		assert.Equal(t, ReconciliationAgeNeverReconciled, row.FoldedReconciliations[0].State)
		return
	}
	t.Fatal("brokerage row not found")
}

// #594 review: the Reconcile form reads one account's judgement through that
// account alone; it must agree with the book-wide judgement.
func TestAccountReconciliationAgeReadsOneAccountAsTheBookDoes(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := newHomeWidgetFixture(t)
	savings := f.account(t, "Savings", "asset", "savings")
	f.move(t, "2026-09-01", f.cashAccountID, f.eurCommodityID, 10000)
	f.move(t, "2026-09-02", f.cashAccountID, f.usd, 2500)
	f.move(t, "2026-09-03", savings, f.eurCommodityID, 500)
	reconcileCash(t, f.investmentsTestFixture, "2026-09-30", 100)

	book, err := f.transactionService.AccountReconciliationAges(ctx, 1)
	require.NoError(t, err)
	for _, accountID := range []int64{f.cashAccountID, savings} {
		one, window, err := f.transactionService.AccountReconciliationAge(ctx, 1, accountID)
		require.NoError(t, err)
		require.NotNil(t, one)
		assert.Equal(t, 60, window)
		assert.Equal(t, book.ByAccount()[accountID], *one)
	}
	cash, _, err := f.transactionService.AccountReconciliationAge(ctx, 1, f.cashAccountID)
	require.NoError(t, err)
	require.Len(t, cash.Commodities, 2, "the form offers both currencies the account has activity in")

	holding, _, err := f.transactionService.AccountReconciliationAge(ctx, 1, f.holdingAccountID)
	require.NoError(t, err)
	assert.Nil(t, holding, "a holding exists but is not judged")
	_, _, err = f.transactionService.AccountReconciliationAge(ctx, 1, 999999)
	require.ErrorIs(t, err, ErrAccountNotFound)
}

// #594 review: an account opened on the owner's today, while UTC is still on
// the day before, exists for the Reconcile lookup exactly as it does for the
// book-wide judgement. The clock is set in 2030 so the database's own date
// is always earlier than the account's first version.
func TestAccountReconciliationAgeFindsAnAccountOpenedOnOwnerLocalToday(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := newInvestmentsTestFixture(t)
	settings := NewSettingsService(db.NewSettingsRepository(f.database))
	f.transactionService.SetSettings(settings)
	_, err := settings.SavePreferences(ctx, SaveUserPreferencesInput{UserID: f.ownerUserID, TimeZone: "Europe/Amsterdam", ChangeReason: "test"})
	require.NoError(t, err)
	f.transactionService.SetNowForTest(func() time.Time { return time.Date(2030, 5, 31, 22, 30, 0, 0, time.UTC) })

	result, err := f.database.ExecContext(ctx, `INSERT INTO accounts (book_id, created_at, created_by_user_id) VALUES (?, '2030-05-31T22:30:00Z', 1)`, BookID)
	require.NoError(t, err)
	opened, err := result.LastInsertId()
	require.NoError(t, err)
	_, err = f.database.ExecContext(ctx, `
		INSERT INTO account_versions (
			account_id, version_seq, effective_from, recorded_at, changed_by_user_id,
			change_reason, status, opened_on, code, name, account_class, account_kind, allows_postings
		) VALUES (?, 1, '2030-06-01', '2030-05-31T22:30:00Z', 1, 'opened today', 'active', '2030-06-01', 'NEW', 'Opened today', 'asset', 'checking', 1)
	`, opened)
	require.NoError(t, err)

	book, err := f.transactionService.AccountReconciliationAges(ctx, f.ownerUserID)
	require.NoError(t, err)
	require.Contains(t, book.ByAccount(), opened, "the book-wide judgement sees the account on local 2030-06-01")
	one, _, err := f.transactionService.AccountReconciliationAge(ctx, f.ownerUserID, opened)
	require.NoError(t, err, "the lookup must not report the account missing")
	require.NotNil(t, one)
	assert.Equal(t, book.ByAccount()[opened], *one)
}
