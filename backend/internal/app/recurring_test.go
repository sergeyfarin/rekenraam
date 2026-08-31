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

func recurringTestPtr[T any](value T) *T { return &value }

func recurringFixture(t *testing.T) (*investmentsTestFixture, *RecurringService, WriteRecurringTemplateInput) {
	t.Helper()
	f := newInvestmentsTestFixture(t)
	s := NewRecurringService(db.NewRecurringRepository(f.database), f.transactionService, NewSettingsService(db.NewSettingsRepository(f.database)))
	s.now = func() time.Time { return time.Date(2026, 8, 30, 12, 0, 0, 0, time.UTC) }
	postings := []db.RecurringTemplatePostingSpec{
		{AccountID: f.cashAccountID, CommodityID: f.eurCommodityID, QuantityValue: exact.New(12500), QuantityScale: 2},
		{AccountID: f.incomeAccountID, CommodityID: f.eurCommodityID, QuantityValue: exact.New(-1250), QuantityScale: 1},
	}
	return f, s, WriteRecurringTemplateInput{OwnerUserID: f.ownerUserID, Patch: RecurringTemplatePatch{
		Name: recurringTestPtr("Salary"), Frequency: recurringTestPtr("monthly"),
		StartsOn: recurringTestPtr("2019-01-01"), DayOfMonth: NullablePatch[int]{Set: true, Value: recurringTestPtr(1)},
		Postings: &postings,
	}}
}

func TestUpdateRecurringTemplateLeavesOmittedFieldsAlone(t *testing.T) {
	_, service, input := recurringFixture(t)
	input.Patch.Enabled = recurringTestPtr(false)
	input.Patch.LeadDays = recurringTestPtr(0)
	input.Patch.Description = recurringTestPtr("Keep this description")
	input.Patch.NoteMarkdown = recurringTestPtr("Keep this note")
	input.Patch.PayeeName = recurringTestPtr("Example employer")
	input.Patch.EndsOn = NullablePatch[string]{Set: true, Value: recurringTestPtr("2028-12-31")}
	created, err := service.CreateTemplate(context.Background(), input)
	require.NoError(t, err)
	updated, err := service.UpdateTemplate(context.Background(), WriteRecurringTemplateInput{
		OwnerUserID: input.OwnerUserID, TemplateID: created.ID, Patch: RecurringTemplatePatch{Name: recurringTestPtr("Renamed")},
	})
	require.NoError(t, err)
	expected := created.Spec
	expected.Name = "Renamed"
	assert.Equal(t, expected, updated.Spec)
	assert.Equal(t, created.Revision+1, updated.Revision)
}

func TestRecurringTemplateValidatesBalancePerCommodityAndScale(t *testing.T) {
	for _, test := range []struct {
		name           string
		first, second  string
		scale1, scale2 int
		otherCommodity bool
		wantError      bool
	}{
		{"different scales balance", "12500", "-1250", 2, 1, false, false},
		{"38 digit quantities remain exact", "99999999999999999999999999999999999999", "-99999999999999999999999999999999999999", 2, 2, false, false},
		{"mismatch refused", "12501", "-1250", 2, 1, false, true},
		{"unlike commodities cannot cancel", "12500", "-12500", 2, 2, true, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			f, service, input := recurringFixture(t)
			postings := *input.Patch.Postings
			postings[0].QuantityValue, postings[1].QuantityValue = exact.MustParse(test.first), exact.MustParse(test.second)
			postings[0].QuantityScale, postings[1].QuantityScale = test.scale1, test.scale2
			if test.otherCommodity {
				postings[1].CommodityID = f.eurCommodityID + 10000
			}
			_, err := service.CreateTemplate(context.Background(), input)
			if test.wantError {
				require.ErrorIs(t, err, ErrRecurringTemplateUnbalanced)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestRecurringTemplateSaveHasNoLedgerSideEffects(t *testing.T) {
	f, service, input := recurringFixture(t)
	tables := []string{"transactions", "transaction_versions", "posting_versions", "investment_lots", "investment_lot_events", "background_work_items", "reconciliation_checkpoints"}
	before := map[string]int{}
	for _, table := range tables {
		var count int
		require.NoError(t, f.database.QueryRow("SELECT COUNT(*) FROM "+table).Scan(&count))
		before[table] = count
	}
	created, err := service.CreateTemplate(context.Background(), input)
	require.NoError(t, err)
	assert.Equal(t, "2026-08-30", created.Spec.GenerateFrom, "phase anchor must not backfill history")
	assert.Equal(t, "2026-09-01", created.NextDueOn)
	for _, table := range tables {
		var count int
		require.NoError(t, f.database.QueryRow("SELECT COUNT(*) FROM "+table).Scan(&count))
		assert.Equal(t, before[table], count, table)
	}
}

func TestRecurringTemplateWatermarkUsesOwnerLocalDate(t *testing.T) {
	_, service, input := recurringFixture(t)
	_, err := service.settings.SavePreferences(context.Background(), SaveUserPreferencesInput{UserID: input.OwnerUserID, TimeZone: "Europe/Amsterdam"})
	require.NoError(t, err)
	service.now = func() time.Time { return time.Date(2026, 8, 30, 23, 30, 0, 0, time.UTC) }
	created, err := service.CreateTemplate(context.Background(), input)
	require.NoError(t, err)
	assert.Equal(t, "2026-08-31", created.Spec.GenerateFrom)
}

func TestRecurringTemplateRejectsInvalidSchedules(t *testing.T) {
	for _, test := range []struct {
		name  string
		patch func(*RecurringTemplatePatch)
	}{
		{"invalid date", func(p *RecurringTemplatePatch) { p.StartsOn = recurringTestPtr("2026-02-30") }},
		{"wrong frequency fields", func(p *RecurringTemplatePatch) { p.Frequency = recurringTestPtr("daily") }},
		{"day and last day together", func(p *RecurringTemplatePatch) { p.LastDayOfMonth = recurringTestPtr(true) }},
		{"zero maximum is not unlimited", func(p *RecurringTemplatePatch) {
			p.MaxOccurrences = NullablePatch[int]{Set: true, Value: recurringTestPtr(0)}
		}},
		{"lead limit", func(p *RecurringTemplatePatch) { p.LeadDays = recurringTestPtr(91) }},
		{"interval overflow input", func(p *RecurringTemplatePatch) { p.IntervalCount = recurringTestPtr(10001) }},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, service, input := recurringFixture(t)
			test.patch(&input.Patch)
			_, err := service.CreateTemplate(context.Background(), input)
			require.ErrorIs(t, err, ErrRecurringScheduleInvalid)
		})
	}
}

func TestRecurringTemplateRejectsInvestmentPostingsDespiteOrdinaryKind(t *testing.T) {
	f, service, input := recurringFixture(t)
	// A kind label alone cannot make a security holding safe for a producer
	// that has no investment lot workflow.
	*input.Patch.Postings = []db.RecurringTemplatePostingSpec{
		{AccountID: f.holdingAccountID, CommodityID: f.stockCommodityID, QuantityValue: exact.New(1)},
		{AccountID: f.holdingAccountID, CommodityID: f.stockCommodityID, QuantityValue: exact.New(-1)},
	}
	_, err := service.CreateTemplate(context.Background(), input)
	var validation ValidationError
	require.ErrorAs(t, err, &validation)
}

func TestRecurringTemplateScheduleEditPreservesMaterializedOccurrences(t *testing.T) {
	_, service, input := recurringFixture(t)
	ctx := context.Background()
	created, err := service.CreateTemplate(ctx, input)
	require.NoError(t, err)
	occurrence, err := service.repository.CreateRecurringOccurrence(ctx, db.CreateRecurringOccurrenceParams{
		BookID: BookID, TemplateID: created.ID, OccurrenceDate: "2026-09-01", Status: "skipped", SkipReason: "Example", MaterializedAt: "2026-08-30T12:00:00Z",
	})
	require.NoError(t, err)
	require.NoError(t, service.repository.AdvanceRecurringGeneration(ctx, BookID, created.ID, created.Revision, "2026-10-01", "2026-08-30T12:00:01Z"))
	updated, err := service.UpdateTemplate(ctx, WriteRecurringTemplateInput{OwnerUserID: input.OwnerUserID, TemplateID: created.ID,
		Patch: RecurringTemplatePatch{DayOfMonth: NullablePatch[int]{Set: true, Value: recurringTestPtr(15)}}})
	require.NoError(t, err)
	assert.Equal(t, "2026-08-30", updated.Spec.GenerateFrom)
	assert.Equal(t, "2026-09-15", updated.NextDueOn)
	after, err := service.repository.RecurringOccurrenceByID(ctx, BookID, occurrence.ID)
	require.NoError(t, err)
	assert.Equal(t, occurrence, after)
}

func TestRecurringTemplateProducesInputAcceptedByRealTransactionService(t *testing.T) {
	f, service, input := recurringFixture(t)
	ctx := context.Background()
	created, err := service.CreateTemplate(ctx, input)
	require.NoError(t, err)
	postings := []PostingInput{}
	for _, posting := range created.Spec.Postings {
		postings = append(postings, PostingInput{AccountID: posting.AccountID, CommodityID: posting.CommodityID, QuantityValue: posting.QuantityValue, QuantityScale: posting.QuantityScale, Memo: posting.Memo})
	}
	transaction, err := f.transactionService.CreateTransaction(ctx, CreateTransactionInput{OwnerUserID: input.OwnerUserID, OriginType: "scheduled", Spec: TransactionInput{
		Status: "draft", TransactionDate: created.NextDueOn, TransactionKind: created.Spec.TransactionKind, PayeeID: created.Spec.PayeeID,
		PayeeName: created.Spec.PayeeName, Description: created.Spec.Description, NoteMarkdown: created.Spec.NoteMarkdown,
		TagIDs: created.Spec.TagIDs, JournalEntries: []JournalEntryInput{{EntryKind: "ordinary", Postings: postings}},
	}})
	require.NoError(t, err)
	assert.Equal(t, "draft", transaction.Status)
}

func TestRecurringTemplateNextDueIncludesUnmaterializedOverdueDates(t *testing.T) {
	_, service, input := recurringFixture(t)
	created, err := service.CreateTemplate(context.Background(), input)
	require.NoError(t, err)
	service.now = func() time.Time { return time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC) }
	read, err := service.Template(context.Background(), input.OwnerUserID, created.ID)
	require.NoError(t, err)
	assert.Equal(t, "2026-09-01", read.NextDueOn, "reading must not hide overdue work or advance the watermark")
	assert.Equal(t, created.Spec.GenerateFrom, read.Spec.GenerateFrom)
}

func TestRecurringTemplateAllowsCurrencyClearingWithoutInvestmentEffects(t *testing.T) {
	f, service, input := recurringFixture(t)
	tradingID, err := db.NewInvestmentRepository(f.database).CommodityTradingAccountID(context.Background(), BookID)
	require.NoError(t, err)
	input.Patch.TransactionKind = recurringTestPtr("transfer")
	(*input.Patch.Postings)[1].AccountID = tradingID
	_, err = service.CreateTemplate(context.Background(), input)
	require.NoError(t, err, "currency exchange clearing is not an investment lot effect")
}
