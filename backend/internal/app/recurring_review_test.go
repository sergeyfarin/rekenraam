package app

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"rekenraam/backend/internal/db"
	"rekenraam/backend/internal/exact"
)

func TestSkippingAFutureOccurrenceThroughReviewStopsItGenerating(t *testing.T) {
	f, s, input := recurringFixture(t)
	ctx := context.Background()
	template, err := s.CreateTemplate(ctx, input)
	require.NoError(t, err)
	before := recurringCounts(t, f.database)
	review := ReviewRecurringInput{OwnerUserID: input.OwnerUserID, TemplateID: template.ID, OccurrenceDate: "2026-09-01", Reason: "Not this month"}
	require.NoError(t, s.SkipOccurrence(ctx, review))
	after := recurringCounts(t, f.database)
	assert.Equal(t, before["transactions"], after["transactions"])
	assert.Equal(t, before["audit_events"]+1, after["audit_events"])
	next, err := s.Template(ctx, input.OwnerUserID, template.ID)
	require.NoError(t, err)
	assert.Equal(t, "2026-10-01", next.NextDueOn)
	all, err := s.ListTemplates(ctx, input.OwnerUserID, false)
	require.NoError(t, err)
	assert.Equal(t, next.NextDueOn, all[0].NextDueOn)
	result, err := s.GenerateDue(ctx, GenerateRecurringInput{OwnerUserID: input.OwnerUserID})
	require.NoError(t, err)
	assert.Zero(t, result.Generated)
	require.ErrorIs(t, s.SkipOccurrence(ctx, review), ErrRecurringOccurrenceMaterialized)
	rows, err := s.repository.ListRecurringOccurrences(ctx, BookID, template.ID, 10)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, "skipped", rows[0].Status)
	assert.Equal(t, review.Reason, rows[0].SkipReason)
	var operation, reason string
	require.NoError(t, f.database.QueryRow(`SELECT operation,reason FROM audit_events WHERE id=?`, rows[0].LastAuditEventID.Int64).Scan(&operation, &reason))
	assert.Equal(t, "recurring.occurrence.skip", operation)
	assert.Equal(t, review.Reason, reason)
}

func TestRecurringSkipRejectsNonScheduleAndPreCreationDates(t *testing.T) {
	f, s, input := recurringFixture(t)
	ctx := context.Background()
	template, err := s.CreateTemplate(ctx, input)
	require.NoError(t, err)
	before := recurringCounts(t, f.database)
	for _, date := range []string{"2026-09-02", "2026-02-30", "0000-01-01", "2026-08-01"} {
		require.ErrorIs(t, s.SkipOccurrence(ctx, ReviewRecurringInput{OwnerUserID: input.OwnerUserID, TemplateID: template.ID, OccurrenceDate: date, Reason: "skip"}), ErrRecurringScheduleInvalid)
	}
	require.Error(t, s.SkipOccurrence(ctx, ReviewRecurringInput{OwnerUserID: input.OwnerUserID, TemplateID: template.ID, OccurrenceDate: "2026-09-01"}))
	assert.Equal(t, before, recurringCounts(t, f.database))
}

func TestBlockedOccurrenceRetriesOnlyWhenAskedTo(t *testing.T) {
	f, s, input := recurringFixture(t)
	ctx := context.Background()
	template, err := s.CreateTemplate(ctx, input)
	require.NoError(t, err)
	_, err = f.database.Exec(`UPDATE recurring_template_postings SET quantity_value='1' WHERE template_id=? AND line_seq=1`, template.ID)
	require.NoError(t, err)
	result, err := s.GenerateDue(ctx, GenerateRecurringInput{OwnerUserID: input.OwnerUserID})
	require.NoError(t, err)
	assert.Equal(t, 1, result.Blocked)
	original, err := s.repository.ListRecurringOccurrences(ctx, BookID, template.ID, 10)
	require.NoError(t, err)
	require.Len(t, original, 1)
	review := ReviewRecurringInput{OwnerUserID: input.OwnerUserID, TemplateID: template.ID, OccurrenceDate: "2026-09-01"}
	// Explicit failed retry is one new audited attempt, without a new identity.
	result, err = s.RetryOccurrence(ctx, review)
	require.NoError(t, err)
	assert.Equal(t, 1, result.Blocked)
	failed, err := s.repository.ListRecurringOccurrences(ctx, BookID, template.ID, 10)
	require.NoError(t, err)
	assert.Equal(t, original[0].ID, failed[0].ID)
	assert.NotEqual(t, original[0].LastAuditEventID, failed[0].LastAuditEventID)
	require.NotEmpty(t, failed[0].ErrorSummary)
	_, err = s.UpdateTemplate(ctx, WriteRecurringTemplateInput{OwnerUserID: input.OwnerUserID, TemplateID: template.ID, Patch: RecurringTemplatePatch{Postings: input.Patch.Postings}})
	require.NoError(t, err)
	result, err = s.GenerateDue(ctx, GenerateRecurringInput{OwnerUserID: input.OwnerUserID})
	require.NoError(t, err)
	assert.Zero(t, result.Generated)
	result, err = s.RetryOccurrence(ctx, review)
	require.NoError(t, err)
	assert.Equal(t, 1, result.Generated)
	generated, err := s.repository.ListRecurringOccurrences(ctx, BookID, template.ID, 10)
	require.NoError(t, err)
	require.Len(t, generated, 1)
	assert.Equal(t, original[0].ID, generated[0].ID)
	assert.Equal(t, original[0].CreatedAt, generated[0].CreatedAt)
	assert.Empty(t, generated[0].ErrorSummary)
	assert.True(t, generated[0].TransactionID.Valid)
	_, err = s.RetryOccurrence(ctx, review)
	require.ErrorIs(t, err, ErrRecurringOccurrenceMaterialized)
	require.ErrorIs(t, s.SkipOccurrence(ctx, ReviewRecurringInput{OwnerUserID: input.OwnerUserID, TemplateID: template.ID, OccurrenceDate: review.OccurrenceDate, Reason: "skip"}), ErrRecurringOccurrenceMaterialized)
	assert.Equal(t, 1, recurringCounts(t, f.database)["transactions"])
}

func TestRecurringRetryRollsBackDraftAndAuditOnOccurrenceFailure(t *testing.T) {
	f, s, input := recurringFixture(t)
	ctx := context.Background()
	template, err := s.CreateTemplate(ctx, input)
	require.NoError(t, err)
	_, err = f.database.Exec(`UPDATE recurring_template_postings SET quantity_value='1' WHERE template_id=? AND line_seq=1`, template.ID)
	require.NoError(t, err)
	_, err = s.GenerateDue(ctx, GenerateRecurringInput{OwnerUserID: input.OwnerUserID})
	require.NoError(t, err)
	_, err = s.UpdateTemplate(ctx, WriteRecurringTemplateInput{OwnerUserID: input.OwnerUserID, TemplateID: template.ID, Patch: RecurringTemplatePatch{Postings: input.Patch.Postings}})
	require.NoError(t, err)
	before := recurringCounts(t, f.database)
	original, err := s.repository.ListRecurringOccurrences(ctx, BookID, template.ID, 10)
	require.NoError(t, err)
	_, err = f.database.Exec(`CREATE TRIGGER fail_retry BEFORE UPDATE ON recurring_occurrences BEGIN SELECT RAISE(ABORT,'injected failure'); END`)
	require.NoError(t, err)
	_, err = s.RetryOccurrence(ctx, ReviewRecurringInput{OwnerUserID: input.OwnerUserID, TemplateID: template.ID, OccurrenceDate: "2026-09-01"})
	require.Error(t, err)
	after, err := s.repository.ListRecurringOccurrences(ctx, BookID, template.ID, 10)
	require.NoError(t, err)
	assert.Equal(t, original, after)
	assert.Equal(t, before, recurringCounts(t, f.database))
}

func TestRecurringSkipAndGenerationRacePreservesOneAuditedIdentity(t *testing.T) {
	f, s, input := recurringFixture(t)
	ctx := context.Background()
	template, err := s.CreateTemplate(ctx, input)
	require.NoError(t, err)
	start := make(chan struct{})
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	wg.Go(func() {
		<-start
		errs <- s.SkipOccurrence(ctx, ReviewRecurringInput{OwnerUserID: input.OwnerUserID, TemplateID: template.ID, OccurrenceDate: "2026-09-01", Reason: "skip"})
	})
	wg.Go(func() {
		<-start
		_, err := s.GenerateDue(ctx, GenerateRecurringInput{OwnerUserID: input.OwnerUserID})
		errs <- err
	})
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			assert.True(t, errors.Is(err, ErrRecurringOccurrenceMaterialized) || errors.Is(err, ErrRecurringTemplateConflict), err)
		}
	}
	rows, err := s.repository.ListRecurringOccurrences(ctx, BookID, template.ID, 10)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	counts := recurringCounts(t, f.database)
	if rows[0].Status == "generated" {
		assert.Equal(t, 1, counts["transactions"])
	} else {
		assert.Equal(t, "skipped", rows[0].Status)
		assert.Zero(t, counts["transactions"])
	}
	assert.True(t, rows[0].LastAuditEventID.Valid)
}

func TestRecurringDuePaginationUsesCurrentDraftAmountsAndKeepsArchivedDrafts(t *testing.T) {
	f, s, input := recurringFixture(t)
	ctx := context.Background()
	input.Patch.Frequency = recurringTestPtr("daily")
	input.Patch.DayOfMonth = NullablePatch[int]{}
	input.Patch.LeadDays = recurringTestPtr(2)
	input.Patch.PayeeName = recurringTestPtr("Original employer")
	template, err := s.CreateTemplate(ctx, input)
	require.NoError(t, err)
	_, err = s.GenerateDue(ctx, GenerateRecurringInput{OwnerUserID: input.OwnerUserID})
	require.NoError(t, err)
	first, err := s.Due(ctx, input.OwnerUserID, "", 1)
	require.NoError(t, err)
	require.Len(t, first.Items, 1)
	require.NotEmpty(t, first.NextCursor)
	assert.Equal(t, "12500", first.Items[0].Amounts[0].DebitValue.String())
	assert.Equal(t, "-12500", first.Items[0].Amounts[0].CreditValue.String())
	current, err := f.transactionService.Transaction(ctx, first.Items[0].TransactionID.Int64)
	require.NoError(t, err)
	spec := transactionInputFromTransaction(current)
	spec.Description = "Changed draft"
	spec.PayeeName = ""
	spec.PayeeID = nil
	spec.JournalEntries[0].Postings[0].QuantityValue = exact.New(9999)
	_, err = f.transactionService.UpdateTransaction(ctx, UpdateTransactionInput{OwnerUserID: input.OwnerUserID, TransactionID: current.ID, OriginType: "browser_api", Spec: spec})
	require.NoError(t, err)
	_, err = s.ArchiveTemplate(ctx, WriteRecurringTemplateInput{OwnerUserID: input.OwnerUserID, TemplateID: template.ID})
	require.NoError(t, err)
	first, err = s.Due(ctx, input.OwnerUserID, "", 1)
	require.NoError(t, err)
	assert.True(t, first.Items[0].TemplateArchived)
	assert.Equal(t, "Changed draft", first.Items[0].Description)
	assert.Empty(t, first.Items[0].PayeeName, "clearing the draft payee must not inherit the template payee")
	assert.Equal(t, "9999", first.Items[0].Amounts[0].DebitValue.String())
	assert.Equal(t, "-12500", first.Items[0].Amounts[0].CreditValue.String())
	// Removing an earlier row does not shift the cursor or skip remaining rows.
	require.NoError(t, f.transactionService.DeleteDraftTransaction(ctx, DeleteDraftTransactionInput{OwnerUserID: input.OwnerUserID, TransactionID: current.ID, OriginType: "browser_api"}))
	second, err := s.Due(ctx, input.OwnerUserID, first.NextCursor, 1)
	require.NoError(t, err)
	require.Len(t, second.Items, 1)
	assert.Equal(t, "2026-08-31", second.Items[0].OccurrenceDate)
	_, err = f.transactionService.PostTransaction(ctx, PostTransactionInput{OwnerUserID: input.OwnerUserID, TransactionID: second.Items[0].TransactionID.Int64, OriginType: "browser_api"})
	require.NoError(t, err)
	last, err := s.Due(ctx, input.OwnerUserID, second.NextCursor, 1)
	require.NoError(t, err)
	require.Len(t, last.Items, 1)
	assert.Empty(t, last.NextCursor)
	assert.Equal(t, "2026-09-01", last.Items[0].OccurrenceDate)
	remaining, err := s.Due(ctx, input.OwnerUserID, "", 200)
	require.NoError(t, err)
	require.Len(t, remaining.Items, 1)
}

func TestRecurringOccurrenceRangeKeepsSkippedDatesAcrossScheduleEdits(t *testing.T) {
	f, s, input := recurringFixture(t)
	ctx := context.Background()
	template, err := s.CreateTemplate(ctx, input)
	require.NoError(t, err)
	require.NoError(t, s.SkipOccurrence(ctx, ReviewRecurringInput{OwnerUserID: input.OwnerUserID, TemplateID: template.ID, OccurrenceDate: "2026-09-01", Reason: "skip"}))
	_, err = s.UpdateTemplate(ctx, WriteRecurringTemplateInput{OwnerUserID: input.OwnerUserID, TemplateID: template.ID, Patch: RecurringTemplatePatch{DayOfMonth: NullablePatch[int]{Set: true, Value: recurringTestPtr(2)}}})
	require.NoError(t, err)
	before := recurringCounts(t, f.database)
	items, err := s.Occurrences(ctx, input.OwnerUserID, template.ID, "2026-08-01", "2026-10-31")
	require.NoError(t, err)
	require.Len(t, items, 3)
	assert.Equal(t, "skipped", items[0].Status)
	assert.Equal(t, "2026-09-01", items[0].Date)
	assert.Equal(t, "scheduled", items[1].Status)
	assert.Equal(t, "2026-09-02", items[1].Date)
	assert.Nil(t, items[1].Record)
	assert.Equal(t, "2026-10-02", items[2].Date)
	past, err := s.Occurrences(ctx, input.OwnerUserID, template.ID, "2026-01-01", "2026-02-01")
	require.NoError(t, err)
	assert.Empty(t, past)
	assert.Equal(t, before, recurringCounts(t, f.database))
	_, err = s.Occurrences(ctx, input.OwnerUserID, template.ID, "2026-01-01", "2028-01-01")
	require.Error(t, err)
}

func TestArchivedBlockedOccurrenceCanBeSkippedButNotRetried(t *testing.T) {
	f, s, input := recurringFixture(t)
	ctx := context.Background()
	template, err := s.CreateTemplate(ctx, input)
	require.NoError(t, err)
	_, err = f.database.Exec(`UPDATE recurring_template_postings SET quantity_value='1' WHERE template_id=? AND line_seq=1`, template.ID)
	require.NoError(t, err)
	_, err = s.GenerateDue(ctx, GenerateRecurringInput{OwnerUserID: input.OwnerUserID})
	require.NoError(t, err)
	_, err = s.ArchiveTemplate(ctx, WriteRecurringTemplateInput{OwnerUserID: input.OwnerUserID, TemplateID: template.ID})
	require.NoError(t, err)
	page, err := s.Due(ctx, input.OwnerUserID, "", 50)
	require.NoError(t, err)
	require.Len(t, page.Items, 1)
	assert.Equal(t, "blocked", page.Items[0].Status)
	review := ReviewRecurringInput{OwnerUserID: input.OwnerUserID, TemplateID: template.ID, OccurrenceDate: "2026-09-01", Reason: "Retired template"}
	_, err = s.RetryOccurrence(ctx, review)
	require.ErrorIs(t, err, ErrRecurringTemplateArchived)
	require.NoError(t, s.SkipOccurrence(ctx, review))
	page, err = s.Due(ctx, input.OwnerUserID, "", 50)
	require.NoError(t, err)
	assert.Empty(t, page.Items)
}

func TestRecurringRetryRejectsStaleAttemptsAndTemplateRevisions(t *testing.T) {
	f, s, input := recurringFixture(t)
	ctx := context.Background()
	template, err := s.CreateTemplate(ctx, input)
	require.NoError(t, err)
	_, err = f.database.Exec(`UPDATE recurring_template_postings SET quantity_value='1' WHERE template_id=? AND line_seq=1`, template.ID)
	require.NoError(t, err)
	_, err = s.GenerateDue(ctx, GenerateRecurringInput{OwnerUserID: input.OwnerUserID})
	require.NoError(t, err)
	template, err = s.Template(ctx, input.OwnerUserID, template.ID)
	require.NoError(t, err)
	rows, err := s.repository.ListRecurringOccurrences(ctx, BookID, template.ID, 10)
	require.NoError(t, err)
	_, err = s.RetryOccurrence(ctx, ReviewRecurringInput{OwnerUserID: input.OwnerUserID, TemplateID: template.ID, OccurrenceDate: rows[0].OccurrenceDate})
	require.NoError(t, err)
	before := recurringCounts(t, f.database)
	_, err = s.materializeOccurrenceAttempt(ctx, GenerateRecurringInput{OwnerUserID: input.OwnerUserID}, template, rows[0].OccurrenceDate, &rows[0].LastAuditEventID.Int64)
	require.ErrorIs(t, err, db.ErrRecurringOccurrenceExists)
	assert.Equal(t, before, recurringCounts(t, f.database))
	// An edit cannot be overwritten by a retry based on an old template snapshot.
	_, err = s.UpdateTemplate(ctx, WriteRecurringTemplateInput{OwnerUserID: input.OwnerUserID, TemplateID: template.ID, Patch: RecurringTemplatePatch{Postings: input.Patch.Postings}})
	require.NoError(t, err)
	_, err = s.materializeOccurrenceAttempt(ctx, GenerateRecurringInput{OwnerUserID: input.OwnerUserID}, template, rows[0].OccurrenceDate, &rows[0].LastAuditEventID.Int64)
	require.ErrorIs(t, err, db.ErrRecurringTemplateConflict)
}

func TestRecurringDuePreservesMoreThanOnePage(t *testing.T) {
	_, s, input := recurringFixture(t)
	ctx := context.Background()
	input.Patch.Frequency = recurringTestPtr("daily")
	input.Patch.DayOfMonth = NullablePatch[int]{}
	input.Patch.LeadDays = recurringTestPtr(0)
	_, err := s.CreateTemplate(ctx, input)
	require.NoError(t, err)
	s.now = func() time.Time { return time.Date(2027, 4, 1, 12, 0, 0, 0, time.UTC) }
	for range 5 {
		_, err = s.GenerateDue(ctx, GenerateRecurringInput{OwnerUserID: input.OwnerUserID})
		require.NoError(t, err)
	}
	page, err := s.Due(ctx, input.OwnerUserID, "", 200)
	require.NoError(t, err)
	require.Len(t, page.Items, 200)
	require.NotEmpty(t, page.NextCursor)
	rest, err := s.Due(ctx, input.OwnerUserID, page.NextCursor, 200)
	require.NoError(t, err)
	require.Len(t, rest.Items, 15)
	assert.Empty(t, rest.NextCursor)
	assert.NotEqual(t, page.Items[199].ID, rest.Items[0].ID)
}

func TestConcurrentRecurringRetriesAcrossPoolsCreateOneDraft(t *testing.T) {
	f, first, input := recurringFixture(t)
	ctx := context.Background()
	template, err := first.CreateTemplate(ctx, input)
	require.NoError(t, err)
	_, err = f.database.Exec(`UPDATE recurring_template_postings SET quantity_value='1' WHERE template_id=? AND line_seq=1`, template.ID)
	require.NoError(t, err)
	_, err = first.GenerateDue(ctx, GenerateRecurringInput{OwnerUserID: input.OwnerUserID})
	require.NoError(t, err)
	_, err = first.UpdateTemplate(ctx, WriteRecurringTemplateInput{OwnerUserID: input.OwnerUserID, TemplateID: template.ID, Patch: RecurringTemplatePatch{Postings: input.Patch.Postings}})
	require.NoError(t, err)
	var file string
	require.NoError(t, f.database.QueryRow(`SELECT file FROM pragma_database_list WHERE name='main'`).Scan(&file))
	other, err := db.Open(ctx, "file:"+file)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, other.Close()) })
	second := NewRecurringService(db.NewRecurringRepository(other), NewTransactionService(db.NewTransactionRepository(other), db.NewPayeeRepository(other), db.NewAccountRepository(other), db.NewCommodityRepository(other)), NewSettingsService(db.NewSettingsRepository(other)))
	second.now = first.now
	start := make(chan struct{})
	errs := make(chan error, 8)
	var wg sync.WaitGroup
	for i := range 8 {
		service := first
		if i%2 == 1 {
			service = second
		}
		wg.Go(func() {
			<-start
			_, err := service.RetryOccurrence(ctx, ReviewRecurringInput{OwnerUserID: input.OwnerUserID, TemplateID: template.ID, OccurrenceDate: "2026-09-01"})
			errs <- err
		})
	}
	close(start)
	wg.Wait()
	close(errs)
	successes := 0
	for err := range errs {
		if err == nil {
			successes++
		} else {
			require.ErrorIs(t, err, ErrRecurringOccurrenceMaterialized)
		}
	}
	assert.Equal(t, 1, successes)
	counts := recurringCounts(t, f.database)
	assert.Equal(t, 1, counts["transactions"])
	assert.Equal(t, 1, counts["recurring_occurrences"])
}

func TestRecurringSkipRollsBackAuditOnWriteFailure(t *testing.T) {
	f, s, input := recurringFixture(t)
	ctx := context.Background()
	template, err := s.CreateTemplate(ctx, input)
	require.NoError(t, err)
	_, err = f.database.Exec(`CREATE TRIGGER fail_skip BEFORE INSERT ON recurring_occurrences BEGIN SELECT RAISE(ABORT,'injected failure'); END`)
	require.NoError(t, err)
	before := recurringCounts(t, f.database)
	require.Error(t, s.SkipOccurrence(ctx, ReviewRecurringInput{OwnerUserID: input.OwnerUserID, TemplateID: template.ID, OccurrenceDate: "2026-09-01", Reason: "Skip"}))
	assert.Equal(t, before, recurringCounts(t, f.database))
}

func TestRecurringDueAmountsKeepCommoditiesSeparateAndRejectOverflow(t *testing.T) {
	result, err := recurringAmounts([]db.RecurringDuePosting{
		{CommodityID: 1, CommodityCode: "EUR", QuantityValue: exact.MustParse("9007199254740993"), QuantityScale: 2},
		{CommodityID: 1, CommodityCode: "EUR", QuantityValue: exact.New(7), QuantityScale: 3},
		{CommodityID: 1, CommodityCode: "EUR", QuantityValue: exact.MustParse("-90071992547409937"), QuantityScale: 3},
		{CommodityID: 2, CommodityCode: "USD", QuantityValue: exact.New(100), QuantityScale: 2},
		{CommodityID: 2, CommodityCode: "USD", QuantityValue: exact.New(-100), QuantityScale: 2},
	})
	require.NoError(t, err)
	require.Len(t, result, 2)
	assert.Equal(t, "90071992547409937", result[0].DebitValue.String())
	assert.Equal(t, "-90071992547409937", result[0].CreditValue.String())
	assert.Equal(t, 3, result[0].QuantityScale)
	assert.Equal(t, "100", result[1].DebitValue.String())
	assert.Equal(t, 2, result[1].QuantityScale)
	_, err = recurringAmounts([]db.RecurringDuePosting{
		{CommodityID: 1, QuantityValue: exact.MustParse("99999999999999999999999999999999999999"), QuantityScale: 2},
		{CommodityID: 1, QuantityValue: exact.New(1), QuantityScale: 2},
	})
	var overflow LedgerOverflowError
	require.ErrorAs(t, err, &overflow)
}
