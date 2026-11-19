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

// #605, after #566 review. A service pins the owner day on its first clock
// reading; validation, defaults and the response all use that day. These
// tests arm a clock whose first reading is the last second of the 15th and
// every later reading is past midnight, so a second "today" would be the 16th.
const pinnedTestDay = "2026-10-15"

func crossOwnerMidnight(set func(func() time.Time)) {
	calls := 0
	set(func() time.Time {
		calls++
		if calls == 1 {
			return time.Date(2026, 10, 15, 23, 59, 59, 0, time.UTC)
		}
		return time.Date(2026, 10, 16, 0, 0, 1, 0, time.UTC)
	})
}

func requireFutureDateRefused(t *testing.T, err error, what string) {
	t.Helper()
	require.Error(t, err, "%s: the 16th is in the future on the pinned day", what)
	var invalid ValidationError
	require.ErrorAs(t, err, &invalid, what)
}

func TestCategoryWritesUseThePinnedOwnerDay(t *testing.T) {
	t.Parallel()
	f := newReferenceBackdatingFixture(t, "category")
	ctx := context.Background()
	current := func() Category {
		t.Helper()
		category, err := f.categories.Category(db.WithReferenceDate(ctx, pinnedTestDay), f.id)
		require.NoError(t, err)
		return category
	}
	rename := func(name, date string) (Category, error) {
		crossOwnerMidnight(f.categories.SetNowForTest)
		return f.categories.UpdateCategory(ctx, UpdateCategoryInput{OwnerUserID: f.ownerUserID, OriginType: "browser_api", Operation: "category.update", CategoryID: f.id, Name: &name, EffectiveFrom: date})
	}
	archive := func(date string) (Category, error) {
		crossOwnerMidnight(f.categories.SetNowForTest)
		return f.categories.DisableCategory(ctx, CategoryLifecycleInput{OwnerUserID: f.ownerUserID, OriginType: "browser_api", CategoryID: f.id, EffectiveFrom: date})
	}

	_, err := rename("Tomorrow", "2026-10-16")
	requireFutureDateRefused(t, err, "rename")
	renamed, err := rename("Renamed", "")
	require.NoError(t, err)
	assert.Equal(t, pinnedTestDay, renamed.EffectiveFrom, "an omitted date defaults to the pinned day")
	assert.Equal(t, current(), renamed)

	_, err = archive("2026-10-16")
	requireFutureDateRefused(t, err, "archive")
	archived, err := archive("")
	require.NoError(t, err)
	assert.Equal(t, "archived", archived.Status)
	assert.Equal(t, pinnedTestDay, archived.EffectiveFrom)
	assert.Equal(t, pinnedTestDay, archived.ClosedOn)
	assert.Equal(t, current(), archived)
}

func TestPayeeWritesUseThePinnedOwnerDay(t *testing.T) {
	t.Parallel()
	f := newReferenceBackdatingFixture(t, "payee")
	ctx := context.Background()
	current := func(id int64) Payee {
		t.Helper()
		payee, err := f.payees.Payee(db.WithReferenceDate(ctx, pinnedTestDay), id)
		require.NoError(t, err)
		return payee
	}
	create := func(name, date string) (Payee, error) {
		crossOwnerMidnight(f.payees.SetNowForTest)
		return f.payees.CreatePayee(ctx, CreatePayeeInput{OwnerUserID: f.ownerUserID, OriginType: "browser_api", Operation: "payee.create", Name: name, EffectiveFrom: date})
	}
	rename := func(name, date string) (Payee, error) {
		crossOwnerMidnight(f.payees.SetNowForTest)
		return f.payees.UpdatePayee(ctx, UpdatePayeeInput{OwnerUserID: f.ownerUserID, OriginType: "browser_api", Operation: "payee.update", PayeeID: f.id, Name: &name, EffectiveFrom: date})
	}
	archive := func(date string) (Payee, error) {
		crossOwnerMidnight(f.payees.SetNowForTest)
		return f.payees.ArchivePayee(ctx, PayeeLifecycleInput{OwnerUserID: f.ownerUserID, OriginType: "browser_api", PayeeID: f.id, EffectiveFrom: date})
	}

	_, err := create("Tomorrow", "2026-10-16")
	requireFutureDateRefused(t, err, "create")
	created, err := create("Created at midnight", "")
	require.NoError(t, err)
	assert.Equal(t, pinnedTestDay, created.EffectiveFrom, "an omitted date defaults to the pinned day")
	assert.Equal(t, current(created.ID), created)

	_, err = rename("Tomorrow", "2026-10-16")
	requireFutureDateRefused(t, err, "rename")
	renamed, err := rename("Renamed", "")
	require.NoError(t, err)
	assert.Equal(t, pinnedTestDay, renamed.EffectiveFrom)
	assert.Equal(t, current(f.id), renamed)

	_, err = archive("2026-10-16")
	requireFutureDateRefused(t, err, "archive")
	archived, err := archive("")
	require.NoError(t, err)
	assert.Equal(t, "archived", archived.Status)
	assert.Equal(t, pinnedTestDay, archived.EffectiveFrom)
	assert.Equal(t, current(f.id), archived)
}

// Institution creation pins nothing and reads the clock once: its response is
// the version it inserted, so only update and lifecycle are covered here.
func TestInstitutionWritesUseThePinnedOwnerDay(t *testing.T) {
	t.Parallel()
	f := newReferenceBackdatingFixture(t, "institution")
	ctx := context.Background()
	current := func() Institution {
		t.Helper()
		institution, err := f.institutions.Institution(db.WithReferenceDate(ctx, pinnedTestDay), f.id)
		require.NoError(t, err)
		return institution
	}
	rename := func(name, date string) (Institution, error) {
		crossOwnerMidnight(f.institutions.SetNowForTest)
		return f.institutions.UpdateInstitution(ctx, UpdateInstitutionInput{OwnerUserID: f.ownerUserID, OriginType: "browser_api", Operation: "institution.update", InstitutionID: f.id, Name: new(name), EffectiveFrom: date})
	}
	archive := func(date string) (Institution, error) {
		crossOwnerMidnight(f.institutions.SetNowForTest)
		return f.institutions.ArchiveInstitution(ctx, InstitutionLifecycleInput{OwnerUserID: f.ownerUserID, OriginType: "browser_api", InstitutionID: f.id, EffectiveFrom: date})
	}

	_, err := rename("Tomorrow", "2026-10-16")
	requireFutureDateRefused(t, err, "rename")
	renamed, err := rename("Renamed", "")
	require.NoError(t, err)
	assert.Equal(t, pinnedTestDay, renamed.EffectiveFrom, "an omitted date defaults to the pinned day")
	assert.Equal(t, current(), renamed)

	_, err = archive("2026-10-16")
	requireFutureDateRefused(t, err, "archive")
	archived, err := archive("")
	require.NoError(t, err)
	assert.Equal(t, "archived", archived.Status)
	assert.Equal(t, pinnedTestDay, archived.EffectiveFrom)
	assert.Equal(t, current(), archived)
}

func TestInstrumentWritesUseThePinnedOwnerDay(t *testing.T) {
	t.Parallel()
	f := newReferenceVersionFixture(t)
	ctx := context.Background()
	createInput := InvestmentInstrumentInput{
		OwnerUserID: f.ownerUserID, OriginType: "browser_api", Operation: "investment.instrument.create",
		CommodityCode: "MIDNIGHT", InstrumentType: "stock", DisplayName: "Midnight instrument",
		QuantityScale: 6, PriceScale: 4,
	}

	crossOwnerMidnight(f.investmentService.SetNowForTest)
	tomorrow := createInput
	tomorrow.EffectiveFrom = "2026-10-16"
	_, err := f.investmentService.CreateInstrument(ctx, tomorrow)
	requireFutureDateRefused(t, err, "create")
	crossOwnerMidnight(f.investmentService.SetNowForTest)
	instrument, err := f.investmentService.CreateInstrument(ctx, createInput)
	require.NoError(t, err)

	rename := func(name, date string) (InvestmentInstrument, error) {
		input := instrumentUpdateInput(t, f, instrument.ID, name, instrument.QuoteCommodityID, "", "", date)
		crossOwnerMidnight(f.investmentService.SetNowForTest)
		return f.investmentService.UpdateInstrument(ctx, input)
	}
	_, err = rename("Tomorrow", "2026-10-16")
	requireFutureDateRefused(t, err, "rename")
	renamed, err := rename("Renamed", "")
	require.NoError(t, err)
	var effectiveFrom string
	require.NoError(t, f.database.QueryRow(`SELECT MAX(effective_from) FROM investment_instrument_versions WHERE instrument_id = ?`, instrument.ID).Scan(&effectiveFrom))
	assert.Equal(t, pinnedTestDay, effectiveFrom, "an omitted date defaults to the pinned day")
	onPinnedDay, err := f.investmentService.Instrument(db.WithReferenceDate(ctx, pinnedTestDay), instrument.ID)
	require.NoError(t, err)
	assert.Equal(t, "Renamed", onPinnedDay.DisplayName)
	assert.Equal(t, onPinnedDay, renamed)
}

// A transaction with no date takes the day its reference checks ran on.
func TestTransactionDefaultDateUsesThePinnedOwnerDay(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	crossOwnerMidnight(f.transactionService.SetNowForTest)
	spec, err := f.transactionService.cleanTransactionSpec(context.Background(), TransactionInput{
		JournalEntries: []JournalEntryInput{{Postings: []PostingInput{
			{AccountID: f.cashAccountID, CommodityID: f.eurCommodityID, QuantityValue: exact.New(-100), QuantityScale: 2},
			{AccountID: f.incomeAccountID, CommodityID: f.eurCommodityID, QuantityValue: exact.New(100), QuantityScale: 2},
		}}},
	}, cleanTransactionOptions{OwnerUserID: f.ownerUserID})
	require.NoError(t, err)
	assert.Equal(t, pinnedTestDay, spec.TransactionDate)
}

// Reads that pin the owner day answer "as of today" on that same day, so the
// as-of date they report is the day their reference reads used.
func TestLedgerReadsDefaultAsOfToThePinnedOwnerDay(t *testing.T) {
	t.Parallel()
	f := newReferenceVersionFixture(t)
	ctx := context.Background()
	arm := func() { crossOwnerMidnight(f.transactionService.SetNowForTest) }

	arm()
	balances, err := f.transactionService.AccountBalances(ctx, AccountBalancesInput{OwnerUserID: f.ownerUserID})
	require.NoError(t, err)
	assert.Equal(t, pinnedTestDay, balances.AsOf, "account balances")

	arm()
	netWorth, err := f.transactionService.NetWorth(ctx, NetWorthInput{OwnerUserID: f.ownerUserID})
	require.NoError(t, err)
	assert.Equal(t, pinnedTestDay, netWorth.AsOf, "net worth")

	arm()
	home, err := f.transactionService.HomeBriefing(ctx, f.ownerUserID)
	require.NoError(t, err)
	assert.Equal(t, pinnedTestDay, home.AsOf, "home briefing")

	arm()
	overview, err := NewLedgerService(f.transactionService, f.accountService, f.institutions).Overview(ctx, LedgerOverviewInput{OwnerUserID: f.ownerUserID})
	require.NoError(t, err)
	assert.Equal(t, pinnedTestDay, overview.AsOf, "ledger overview")
}

// The ledger overview judges reconciliation ages inside its own pinned read;
// the judgement takes the caller's day, not a fresh clock reading after
// midnight.
func TestReconciliationAgesJudgeOnTheCallersPinnedDay(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	f.transactionService.SetNowForTest(func() time.Time { return time.Date(2026, 10, 16, 0, 0, 1, 0, time.UTC) })
	ages, err := f.transactionService.AccountReconciliationAges(db.WithReferenceDate(context.Background(), pinnedTestDay), f.ownerUserID)
	require.NoError(t, err)
	assert.Equal(t, pinnedTestDay, ages.Today)
}
