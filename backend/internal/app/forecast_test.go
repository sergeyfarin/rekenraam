package app

import (
	"context"
	"database/sql"
	"errors"
	"math/rand"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rekenraam/backend/internal/db"
	"rekenraam/backend/internal/exact"
)

func forecastTestSnapshot() db.ForecastSnapshot {
	accounts := []db.ForecastAccountVersionRecord{
		forecastAccountVersion(1, "Checking", "asset", "checking", 1),
		forecastAccountVersion(2, "Savings", "asset", "savings", 1),
		forecastAccountVersion(3, "Expense", "expense", "expense", 1),
		forecastAccountVersion(4, "Card", "liability", "credit_card", 1),
		forecastAccountVersion(5, "USD cash", "asset", "cash", 2),
	}
	return db.ForecastSnapshot{TimeZone: "UTC", AccountVersions: accounts,
		CommodityVersions: []db.ForecastCommodityVersionRecord{
			{CommodityID: 1, BookID: 1, VersionID: 1, VersionSeq: 1, EffectiveFrom: "0001-01-01", Code: "EUR", Kind: "currency", Status: "active", StandardScale: 2, MaxQuantityScale: 12},
			{CommodityID: 2, BookID: 1, VersionID: 2, VersionSeq: 1, EffectiveFrom: "0001-01-01", Code: "USD", Kind: "currency", Status: "active", StandardScale: 2, MaxQuantityScale: 12},
			{CommodityID: 3, BookID: 1, VersionID: 3, VersionSeq: 1, EffectiveFrom: "0001-01-01", Code: "SEC", Kind: "security", Status: "active", StandardScale: 4, MaxQuantityScale: 12},
		}, PayeeNames: map[int64]string{}}
}

func forecastAccountVersion(id int64, name, class, kind string, commodityID int64) db.ForecastAccountVersionRecord {
	return db.ForecastAccountVersionRecord{AccountID: id, BookID: 1, VersionID: id, VersionSeq: 1, EffectiveFrom: "0001-01-01", Status: "active", OpenedOn: "0001-01-01", Name: sql.NullString{String: name, Valid: true}, AccountClass: class, AccountKind: kind, DefaultCommodityID: sql.NullInt64{Int64: commodityID, Valid: true}, AllowsPostings: true}
}

func forecastTestScope(snapshot db.ForecastSnapshot, ids ...int64) forecastScope {
	rules := accountRulesAt(snapshot.AccountVersions, "2026-08-31")
	accounts := map[int64]db.ForecastAccountVersionRecord{}
	for _, id := range ids {
		accounts[id] = rules[id]
	}
	return forecastScope{AccountIDs: ids, Accounts: accounts}
}

func forecastTestBuild(t *testing.T, snapshot db.ForecastSnapshot, ids ...int64) ForecastResult {
	t.Helper()
	result, err := buildForecast(context.Background(), forecastNormalizedInput{HorizonDays: 5}, forecastBounds{AsOf: "2026-08-31", First: "2026-09-01", Through: "2026-09-05"}, forecastTestScope(snapshot, ids...), snapshot, "2026-08-31T12:00:00Z")
	require.NoError(t, err)
	return result
}

func posting(id, tx, entry int64, date string, account, commodity int64, value string, scale int) db.ForecastPostingRecord {
	return db.ForecastPostingRecord{PostingID: id, TransactionID: tx, TransactionVersionID: tx, JournalEntryID: entry, EntryDate: date, AccountID: account, CommodityID: commodity, QuantityValue: exact.MustParse(value), QuantityScale: scale}
}

func TestForecastOpeningUsesPostedEntryDates(t *testing.T) {
	s := forecastTestSnapshot()
	s.PostedPostings = []db.ForecastPostingRecord{posting(1, 1, 1, "2026-08-31", 1, 1, "10000", 2), posting(2, 2, 2, "2026-09-01", 1, 1, "20000", 2)}
	r := forecastTestBuild(t, s, 1)
	assert.Equal(t, exact.Coefficient("10000"), r.Series[0].Opening.Value)
	assert.Equal(t, exact.Coefficient("30000"), r.Series[0].Points[0].RecordedBalance.Value)
}

func TestForecastFuturePostedEntriesMoveOnEntryDate(t *testing.T) {
	s := forecastTestSnapshot()
	s.PostedPostings = []db.ForecastPostingRecord{posting(1, 1, 1, "2026-09-01", 1, 1, "100", 2), posting(2, 2, 2, "2026-09-05", 1, 1, "200", 2), posting(3, 3, 3, "2026-09-06", 1, 1, "900", 2)}
	r := forecastTestBuild(t, s, 1)
	assert.Equal(t, exact.Coefficient("100"), r.Series[0].Points[0].RecordedBalance.Value)
	assert.Equal(t, exact.Coefficient("300"), r.Series[0].Points[4].RecordedBalance.Value)
}

func TestForecastEditedDraftOverridesTemplateDateAndAmount(t *testing.T) {
	s := forecastTestSnapshot()
	s.Templates = []db.ForecastTemplateRecord{{ID: 10, BookID: 1, Name: "Rent", Enabled: true, TransactionKind: "ordinary", Frequency: "daily", IntervalCount: 1, StartsOn: "2026-09-02", GenerateFrom: "2026-09-02"}}
	s.TemplatePostings = []db.ForecastTemplatePostingRecord{{TemplateID: 10, PostingID: 1, AccountID: 1, CommodityID: 1, QuantityValue: exact.MustParse("-10000"), QuantityScale: 2}, {TemplateID: 10, PostingID: 2, AccountID: 3, CommodityID: 1, QuantityValue: exact.MustParse("10000"), QuantityScale: 2}}
	s.Occurrences = []db.ForecastOccurrenceRecord{{ID: 20, TemplateID: 10, OccurrenceDate: "2026-09-02", Status: "generated", TransactionID: sql.NullInt64{Int64: 30, Valid: true}, TransactionStatus: sql.NullString{String: "draft", Valid: true}}}
	for i, row := range []struct {
		account int64
		value   string
	}{{1, "-12500"}, {3, "12500"}} {
		s.DraftPostings = append(s.DraftPostings, db.ForecastDraftPostingRecord{OccurrenceID: 20, TemplateID: 10, OccurrenceDate: "2026-09-02", TransactionID: 30, TransactionVersionID: 30, TransactionKind: "ordinary", JournalEntryID: 40, EntryDate: "2026-09-03", PostingID: int64(i + 1), AccountID: row.account, CommodityID: 1, QuantityValue: exact.MustParse(row.value), QuantityScale: 2})
	}
	r := forecastTestBuild(t, s, 1)
	assert.Zero(t, r.Series[0].Points[1].Components.Template.Value.Sign())
	assert.Equal(t, exact.Coefficient("-12500"), r.Series[0].Points[2].Components.Draft.Value)
}

func TestForecastRecurringMaterializationDoesNotChangeAmounts(t *testing.T) {
	base := forecastTestSnapshot()
	base.Templates = []db.ForecastTemplateRecord{{ID: 10, BookID: 1, Enabled: true, TransactionKind: "ordinary", Frequency: "daily", IntervalCount: 1, StartsOn: "2026-09-01", GenerateFrom: "2026-09-01", EndsOn: sql.NullString{String: "2026-09-01", Valid: true}}}
	base.TemplatePostings = []db.ForecastTemplatePostingRecord{{TemplateID: 10, AccountID: 1, CommodityID: 1, QuantityValue: exact.MustParse("-100"), QuantityScale: 2}, {TemplateID: 10, AccountID: 3, CommodityID: 1, QuantityValue: exact.MustParse("100"), QuantityScale: 2}}
	templateResult := forecastTestBuild(t, base, 1)
	draft := base
	draft.Occurrences = []db.ForecastOccurrenceRecord{{ID: 20, TemplateID: 10, OccurrenceDate: "2026-09-01", Status: "generated", TransactionID: sql.NullInt64{Int64: 30, Valid: true}, TransactionStatus: sql.NullString{String: "draft", Valid: true}}}
	for i, row := range []struct {
		account int64
		value   string
	}{{1, "-100"}, {3, "100"}} {
		draft.DraftPostings = append(draft.DraftPostings, db.ForecastDraftPostingRecord{OccurrenceID: 20, TemplateID: 10, OccurrenceDate: "2026-09-01", TransactionID: 30, TransactionVersionID: 30, TransactionKind: "ordinary", JournalEntryID: 40, EntryDate: "2026-09-01", PostingID: int64(i + 1), AccountID: row.account, CommodityID: 1, QuantityValue: exact.MustParse(row.value), QuantityScale: 2})
	}
	draftResult := forecastTestBuild(t, draft, 1)
	assert.Equal(t, projectedQuantities(templateResult.Series[0]), projectedQuantities(draftResult.Series[0]))
	assert.Equal(t, "template", templateResult.Events[0].Source)
	assert.Equal(t, "draft", draftResult.Events[0].Source)
	posted := draft
	posted.DraftPostings = nil
	posted.Occurrences[0].TransactionStatus = sql.NullString{String: "posted", Valid: true}
	posted.PostedPostings = []db.ForecastPostingRecord{posting(1, 30, 40, "2026-09-01", 1, 1, "-100", 2)}
	postedResult := forecastTestBuild(t, posted, 1)
	assert.Equal(t, draftResult.Series[0].Points[0].ProjectedBalance, postedResult.Series[0].Points[0].ProjectedBalance)
	assert.Equal(t, "posted", postedResult.Events[0].Source)
}

func TestForecastServiceUsesRealRecurringGenerationAndPromotion(t *testing.T) {
	f, recurring, input := recurringFixture(t)
	ctx := context.Background()
	_, err := recurring.CreateTemplate(ctx, input)
	require.NoError(t, err)
	var databaseFile string
	require.NoError(t, f.database.QueryRow(`SELECT file FROM pragma_database_list WHERE name = 'main'`).Scan(&databaseFile))
	readOnly, err := db.OpenReadOnly(ctx, "file:"+databaseFile)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, readOnly.Close()) })
	forecast := NewForecastService(db.NewForecastRepository(readOnly))
	forecast.SetNowForTest(func() time.Time { return time.Date(2026, 8, 30, 12, 0, 0, 0, time.UTC) })
	recipe := ForecastInput{OwnerUserID: f.ownerUserID, HorizonDays: 5, AccountIDs: []int64{f.cashAccountID}}
	computed, err := forecast.Balances(ctx, recipe)
	require.NoError(t, err)
	_, err = recurring.GenerateDue(ctx, GenerateRecurringInput{OwnerUserID: f.ownerUserID})
	require.NoError(t, err)
	draft, err := forecast.Balances(ctx, recipe)
	require.NoError(t, err)
	assert.Equal(t, projectedQuantities(computed.Series[0]), projectedQuantities(draft.Series[0]))
	page, err := recurring.Due(ctx, f.ownerUserID, "", 10)
	require.NoError(t, err)
	require.Len(t, page.Items, 1)
	_, err = f.transactionService.PostTransaction(ctx, PostTransactionInput{OwnerUserID: f.ownerUserID, TransactionID: page.Items[0].TransactionID.Int64, OriginType: "browser_api"})
	require.NoError(t, err)
	posted, err := forecast.Balances(ctx, recipe)
	require.NoError(t, err)
	assert.Equal(t, projectedQuantities(draft.Series[0]), projectedQuantities(posted.Series[0]))
	assert.NotEqual(t, draft.Series[0].Points[1].RecordedBalance, posted.Series[0].Points[1].RecordedBalance)
}

func projectedQuantities(series ForecastSeries) []ForecastQuantity {
	result := make([]ForecastQuantity, 0, len(series.Points))
	for _, point := range series.Points {
		result = append(result, point.ProjectedBalance)
	}
	return result
}

func TestForecastTerminalOccurrencesNeverReappear(t *testing.T) {
	s := forecastTestSnapshot()
	s.Templates = []db.ForecastTemplateRecord{{ID: 10, BookID: 1, Enabled: true, TransactionKind: "ordinary", Frequency: "daily", IntervalCount: 1, StartsOn: "2026-09-01", GenerateFrom: "2026-09-01"}}
	s.TemplatePostings = []db.ForecastTemplatePostingRecord{{TemplateID: 10, AccountID: 1, CommodityID: 1, QuantityValue: exact.MustParse("-1")}, {TemplateID: 10, AccountID: 3, CommodityID: 1, QuantityValue: exact.MustParse("1")}}
	s.Occurrences = []db.ForecastOccurrenceRecord{{ID: 1, TemplateID: 10, OccurrenceDate: "2026-09-01", Status: "skipped"}, {ID: 2, TemplateID: 10, OccurrenceDate: "2026-09-02", Status: "generated", TransactionID: sql.NullInt64{Int64: 99, Valid: true}, TransactionStatus: sql.NullString{String: "voided", Valid: true}}}
	r := forecastTestBuild(t, s, 1)
	assert.NotContains(t, []string{r.Events[0].OccurrenceDate, r.Events[1].OccurrenceDate, r.Events[2].OccurrenceDate}, "2026-09-01")
	assert.True(t, r.Assumptions.Complete)
}

func TestForecastPausedAndArchivedTemplatesKeepSavedDrafts(t *testing.T) {
	s := forecastTestSnapshot()
	s.Templates = []db.ForecastTemplateRecord{{ID: 10, BookID: 1, Enabled: false, ArchivedAt: sql.NullString{String: "x", Valid: true}, TransactionKind: "ordinary", Frequency: "daily", IntervalCount: 1, StartsOn: "2026-09-01", GenerateFrom: "2026-09-01"}}
	s.Occurrences = []db.ForecastOccurrenceRecord{{ID: 1, TemplateID: 10, OccurrenceDate: "2026-09-01", Status: "generated", TransactionID: sql.NullInt64{Int64: 2, Valid: true}, TransactionStatus: sql.NullString{String: "draft", Valid: true}}}
	for i, row := range []struct {
		account int64
		value   string
	}{{1, "-10"}, {3, "10"}} {
		s.DraftPostings = append(s.DraftPostings, db.ForecastDraftPostingRecord{OccurrenceID: 1, TemplateID: 10, OccurrenceDate: "2026-09-01", TransactionID: 2, TransactionVersionID: 2, TransactionKind: "ordinary", JournalEntryID: 3, EntryDate: "2026-09-01", PostingID: int64(i + 1), AccountID: row.account, CommodityID: 1, QuantityValue: exact.MustParse(row.value)})
	}
	r := forecastTestBuild(t, s, 1)
	require.Len(t, r.Events, 1)
	assert.Equal(t, "draft", r.Events[0].Source)
}

func TestForecastCarriesOverdueAndTodayToTomorrow(t *testing.T) {
	s := forecastTestSnapshot()
	s.Templates = []db.ForecastTemplateRecord{{ID: 10, BookID: 1, Enabled: true, TransactionKind: "ordinary", Frequency: "daily", IntervalCount: 1, StartsOn: "2026-08-30", GenerateFrom: "2026-08-30", EndsOn: sql.NullString{String: "2026-08-31", Valid: true}}}
	s.TemplatePostings = []db.ForecastTemplatePostingRecord{{TemplateID: 10, AccountID: 1, CommodityID: 1, QuantityValue: exact.MustParse("-10")}, {TemplateID: 10, AccountID: 3, CommodityID: 1, QuantityValue: exact.MustParse("10")}}
	r := forecastTestBuild(t, s, 1)
	require.Len(t, r.Events, 2)
	assert.Equal(t, "2026-09-01", r.Events[0].ProjectedDate)
	assert.True(t, r.Events[0].CarriedForward)
	assert.Equal(t, 2, r.Assumptions.Total)
}

func TestForecastUsesWatermarkButNotLeadWindow(t *testing.T) {
	s := forecastTestSnapshot()
	s.Templates = []db.ForecastTemplateRecord{{ID: 10, BookID: 1, Enabled: true, TransactionKind: "ordinary", Frequency: "daily", IntervalCount: 1, StartsOn: "2020-01-01", GenerateFrom: "2026-09-03", MaxOccurrences: sql.NullInt64{Int64: 2440, Valid: true}}}
	s.TemplatePostings = []db.ForecastTemplatePostingRecord{{TemplateID: 10, AccountID: 1, CommodityID: 1, QuantityValue: exact.MustParse("-1")}, {TemplateID: 10, AccountID: 3, CommodityID: 1, QuantityValue: exact.MustParse("1")}}
	r := forecastTestBuild(t, s, 1)
	require.NotEmpty(t, r.Events)
	assert.Equal(t, "2026-09-03", r.Events[0].SourceDate)
}

func TestForecastReusesClampedCalendarSchedules(t *testing.T) {
	s := forecastTestSnapshot()
	s.Templates = []db.ForecastTemplateRecord{{ID: 10, BookID: 1, Enabled: true, TransactionKind: "ordinary", Frequency: "monthly", IntervalCount: 1, DayOfMonth: sql.NullInt64{Int64: 31, Valid: true}, StartsOn: "2026-01-31", GenerateFrom: "2026-08-01"}}
	s.TemplatePostings = []db.ForecastTemplatePostingRecord{{TemplateID: 10, AccountID: 1, CommodityID: 1, QuantityValue: exact.MustParse("-10")}, {TemplateID: 10, AccountID: 3, CommodityID: 1, QuantityValue: exact.MustParse("10")}}
	r, err := buildForecast(context.Background(), forecastNormalizedInput{HorizonDays: 35}, forecastBounds{AsOf: "2026-08-31", First: "2026-09-01", Through: "2026-10-05"}, forecastTestScope(s, 1), s, "2026-08-31T12:00:00Z")
	require.NoError(t, err)
	require.Len(t, r.Events, 2)
	assert.Equal(t, "2026-08-31", r.Events[0].SourceDate)
	assert.Equal(t, "2026-09-30", r.Events[1].SourceDate)
}

func TestForecastInvalidDraftExcludesWholeTransaction(t *testing.T) {
	s := forecastTestSnapshot()
	s.DraftPostings = []db.ForecastDraftPostingRecord{{OccurrenceID: 1, TemplateID: 10, TransactionID: 2, TransactionVersionID: 2, TransactionKind: "ordinary", JournalEntryID: 3, EntryDate: "2026-09-01", AccountID: 1, CommodityID: 1, QuantityValue: exact.MustParse("-10")}, {OccurrenceID: 1, TemplateID: 10, TransactionID: 2, TransactionVersionID: 2, TransactionKind: "ordinary", JournalEntryID: 3, EntryDate: "2026-09-01", AccountID: 3, CommodityID: 1, QuantityValue: exact.MustParse("9")}}
	r := forecastTestBuild(t, s, 1)
	assert.Empty(t, r.Events)
	assert.False(t, r.Assumptions.Complete)
}

func TestForecastLifecycleEligibilityUsesSnapshotDates(t *testing.T) {
	s := forecastTestSnapshot()
	closed := s.AccountVersions[0]
	closed.VersionID = 20
	closed.VersionSeq = 2
	closed.EffectiveFrom = "2026-09-01"
	closed.ClosedOn = sql.NullString{String: "2026-08-31", Valid: true}
	s.AccountVersions = append(s.AccountVersions, closed)
	s.PostedPostings = []db.ForecastPostingRecord{posting(1, 1, 1, "2026-08-31", 1, 1, "100", 2)}
	s.Templates = []db.ForecastTemplateRecord{{ID: 10, BookID: 1, Enabled: true, TransactionKind: "ordinary", Frequency: "daily", IntervalCount: 1, StartsOn: "2026-09-01", GenerateFrom: "2026-09-01", EndsOn: sql.NullString{String: "2026-09-01", Valid: true}}}
	s.TemplatePostings = []db.ForecastTemplatePostingRecord{{TemplateID: 10, AccountID: 1, CommodityID: 1, QuantityValue: exact.MustParse("-10")}, {TemplateID: 10, AccountID: 3, CommodityID: 1, QuantityValue: exact.MustParse("10")}}
	r := forecastTestBuild(t, s, 1)
	assert.Equal(t, exact.Coefficient("100"), r.Series[0].Opening.Value)
	assert.Empty(t, r.Events)
	assert.False(t, r.Assumptions.Complete)
}

func TestForecastScopeDefaultsAndDescendants(t *testing.T) {
	s := forecastTestSnapshot()
	parent := forecastAccountVersion(6, "Group", "asset", "group", 1)
	parent.AllowsPostings = false
	child := forecastAccountVersion(7, "Child", "asset", "checking", 1)
	child.ParentAccountID = sql.NullInt64{Int64: 6, Valid: true}
	s.AccountVersions = append(s.AccountVersions, parent, child)
	defaultScope, err := resolveForecastScope(s.AccountVersions, "2026-08-31", forecastNormalizedInput{})
	require.NoError(t, err)
	assert.Equal(t, []int64{1, 2, 5, 7}, defaultScope.AccountIDs)
	explicit, err := resolveForecastScope(s.AccountVersions, "2026-08-31", forecastNormalizedInput{AccountIDs: []int64{6}, IncludeDescendants: true})
	require.NoError(t, err)
	assert.Equal(t, []int64{7}, explicit.AccountIDs)
	empty, err := resolveForecastScope(s.AccountVersions, "2026-08-31", forecastNormalizedInput{AccountIDs: []int64{6}})
	require.NoError(t, err)
	assert.Empty(t, empty.AccountIDs)
}

func TestForecastTransfersAndLiabilitySigns(t *testing.T) {
	s := forecastTestSnapshot()
	s.PostedPostings = []db.ForecastPostingRecord{posting(1, 1, 1, "2026-08-31", 1, 1, "10000", 2), posting(2, 2, 2, "2026-09-02", 1, 1, "-3000", 2), posting(3, 2, 2, "2026-09-02", 2, 1, "3000", 2), posting(4, 3, 3, "2026-08-31", 4, 1, "-30000", 2), posting(5, 4, 4, "2026-09-03", 4, 1, "10000", 2)}
	r := forecastTestBuild(t, s, 1, 2, 4)
	assert.Equal(t, exact.Coefficient("-20000"), r.Aggregates[0].Opening.Value)
	assert.Equal(t, exact.Coefficient("-20000"), r.Series[2].Points[2].ProjectedBalance.Value)
	assert.Empty(t, r.Series[2].FirstNegativeDate)
}

func TestForecastExactDailyIdentities(t *testing.T) {
	s := forecastTestSnapshot()
	s.PostedPostings = []db.ForecastPostingRecord{posting(1, 1, 1, "2026-08-31", 1, 1, "100", 2), posting(2, 2, 2, "2026-09-01", 1, 1, "20", 2)}
	first := forecastTestBuild(t, s, 1)
	rand.New(rand.NewSource(3)).Shuffle(len(s.PostedPostings), func(i, j int) { s.PostedPostings[i], s.PostedPostings[j] = s.PostedPostings[j], s.PostedPostings[i] })
	second := forecastTestBuild(t, s, 1)
	assert.Equal(t, first.Series, second.Series)
	for _, point := range first.Series[0].Points {
		assert.Equal(t, point.RecordedBalance, point.ProjectedBalance)
	}
}

func TestForecastPreservesLargeAndMixedScaleAmounts(t *testing.T) {
	s := forecastTestSnapshot()
	s.PostedPostings = []db.ForecastPostingRecord{posting(1, 1, 1, "2026-08-31", 1, 1, "9007199254740993", 2), posting(2, 2, 2, "2026-08-31", 1, 1, "4", 3), posting(3, 3, 3, "2026-09-01", 1, 1, "1", 3)}
	r := forecastTestBuild(t, s, 1)
	assert.Equal(t, exact.Coefficient("90071992547409934"), r.Series[0].Opening.Value)
	assert.Equal(t, exact.Coefficient("90071992547409935"), r.Series[0].Points[0].ProjectedBalance.Value)
	overflow := forecastTestSnapshot()
	overflow.PostedPostings = []db.ForecastPostingRecord{posting(1, 1, 1, "2026-08-31", 1, 1, "99999999999999999999999999999999999999", 0), posting(2, 2, 2, "2026-09-01", 1, 1, "1", 0)}
	_, err := buildForecast(context.Background(), forecastNormalizedInput{HorizonDays: 5}, forecastBounds{AsOf: "2026-08-31", First: "2026-09-01", Through: "2026-09-05"}, forecastTestScope(overflow, 1), overflow, "now")
	var ledgerOverflow LedgerOverflowError
	assert.ErrorAs(t, err, &ledgerOverflow)
}

func TestForecastFlatSeriesAndMinimumTieDates(t *testing.T) {
	s := forecastTestSnapshot()
	s.PostedPostings = []db.ForecastPostingRecord{posting(1, 1, 1, "2026-08-31", 1, 1, "100", 2), posting(2, 2, 2, "2026-09-02", 1, 1, "-120", 2)}
	r := forecastTestBuild(t, s, 1)
	assert.Equal(t, "2026-09-02", r.Series[0].MinimumDate)
	assert.Equal(t, "2026-09-02", r.Series[0].FirstNegativeDate)
	assert.Len(t, r.Series[0].Points, 5)
}

func TestForecastDiagnosticsDoNotHideExclusions(t *testing.T) {
	s := forecastTestSnapshot()
	for i := 0; i < 60; i++ {
		s.Occurrences = append(s.Occurrences, db.ForecastOccurrenceRecord{ID: int64(i + 1), TemplateID: 10, OccurrenceDate: "2026-09-01", Status: "blocked"})
	}
	r := forecastTestBuild(t, s, 1)
	assert.Equal(t, 60, r.Assumptions.Total)
	assert.Equal(t, 10, r.Assumptions.Hidden)
	assert.Len(t, r.Assumptions.Diagnostics, 50)
	assert.False(t, r.Assumptions.Complete)
}

func TestForecastIgnoresNonRecurringDraftsAndSecurityValues(t *testing.T) {
	s := forecastTestSnapshot()
	s.PostedPostings = []db.ForecastPostingRecord{posting(1, 1, 1, "2026-08-31", 1, 3, "999", 2), posting(2, 1, 1, "2026-08-31", 1, 1, "100", 2)}
	r := forecastTestBuild(t, s, 1)
	require.Len(t, r.Series, 1)
	assert.Equal(t, int64(1), r.Series[0].CommodityID)
}

func TestForecastLongCatchUpIsBoundedWithoutTruncation(t *testing.T) {
	s := forecastTestSnapshot()
	s.Templates = []db.ForecastTemplateRecord{{ID: 10, BookID: 1, Enabled: true, TransactionKind: "ordinary", Frequency: "daily", IntervalCount: 1, StartsOn: "1900-01-01", GenerateFrom: "1900-01-01"}}
	s.TemplatePostings = []db.ForecastTemplatePostingRecord{{TemplateID: 10, AccountID: 1, CommodityID: 1, QuantityValue: exact.MustParse("-1")}, {TemplateID: 10, AccountID: 3, CommodityID: 1, QuantityValue: exact.MustParse("1")}}
	_, err := buildForecast(context.Background(), forecastNormalizedInput{HorizonDays: 5}, forecastBounds{AsOf: "2026-08-31", First: "2026-09-01", Through: "2026-09-05"}, forecastTestScope(s, 1), s, "now")
	assert.ErrorIs(t, err, ErrForecastTooLarge)
}

func TestForecastEmptyChunksAndExhaustedSchedules(t *testing.T) {
	s := forecastTestSnapshot()
	s.Templates = []db.ForecastTemplateRecord{{ID: 10, BookID: 1, Enabled: true, TransactionKind: "ordinary", Frequency: "yearly", IntervalCount: 10, DayOfMonth: sql.NullInt64{Int64: 1, Valid: true}, MonthOfYear: sql.NullInt64{Int64: 1, Valid: true}, StartsOn: "2000-01-01", GenerateFrom: "2026-01-01", MaxOccurrences: sql.NullInt64{Int64: 2, Valid: true}}}
	s.TemplatePostings = []db.ForecastTemplatePostingRecord{{TemplateID: 10, AccountID: 1, CommodityID: 1, QuantityValue: exact.MustParse("-1")}, {TemplateID: 10, AccountID: 3, CommodityID: 1, QuantityValue: exact.MustParse("1")}}
	r := forecastTestBuild(t, s, 1)
	assert.Empty(t, r.Events)
}

func TestForecastInputAndOutputBudgets(t *testing.T) {
	rootIDs := make([]int64, forecastMaxRootAccounts+1)
	for index := range rootIDs {
		rootIDs[index] = int64(index + 1)
	}
	_, err := normalizeForecastInput(ForecastInput{HorizonDays: 5, AccountIDs: rootIDs})
	assert.Error(t, err)
	s := forecastTestSnapshot()
	scope := forecastScope{Accounts: map[int64]db.ForecastAccountVersionRecord{}}
	for id := int64(1); id <= 200; id++ {
		account := forecastAccountVersion(id, "A", "asset", "checking", 1)
		scope.AccountIDs = append(scope.AccountIDs, id)
		scope.Accounts[id] = account
		s.AccountVersions = append(s.AccountVersions, account)
		for commodity := int64(1); commodity <= 3; commodity++ {
			s.PostedPostings = append(s.PostedPostings, posting(id*10+commodity, id, id, "2026-08-31", id, commodity, "1", 0))
		}
	}
	s.CommodityVersions[2].Kind = "currency"
	_, err = buildForecast(context.Background(), forecastNormalizedInput{HorizonDays: 366}, forecastBounds{AsOf: "2026-08-31", First: "2026-09-01", Through: "2027-09-01"}, scope, s, "now")
	assert.ErrorIs(t, err, ErrForecastTooLarge)
}

func TestForecastHonorsContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := buildForecast(ctx, forecastNormalizedInput{HorizonDays: 5}, forecastBounds{AsOf: "2026-08-31", First: "2026-09-01", Through: "2026-09-05"}, forecastTestScope(forecastTestSnapshot(), 1), forecastTestSnapshot(), "now")
	assert.True(t, errors.Is(err, context.Canceled))
}
