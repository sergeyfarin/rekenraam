package app

import (
	"context"
	"database/sql"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"rekenraam/backend/internal/db"
)

func recurringCounts(t *testing.T, database *sql.DB) map[string]int {
	t.Helper()
	counts := map[string]int{}
	for _, table := range []string{"transactions", "transaction_versions", "posting_versions", "audit_events", "background_work_items", "recurring_occurrences", "reconciliation_checkpoints"} {
		var count int
		require.NoError(t, database.QueryRow("SELECT COUNT(*) FROM "+table).Scan(&count))
		counts[table] = count
	}
	return counts
}

func TestGenerationIsIdempotentAcrossTicks(t *testing.T) {
	f, s, input := recurringFixture(t)
	ctx := context.Background()
	template, err := s.CreateTemplate(ctx, input)
	require.NoError(t, err)
	before := recurringCounts(t, f.database)
	result, err := s.GenerateDue(ctx, GenerateRecurringInput{OwnerUserID: input.OwnerUserID})
	require.NoError(t, err)
	assert.Equal(t, RecurringGenerationResult{Generated: 1}, result)
	after := recurringCounts(t, f.database)
	assert.Equal(t, before["background_work_items"], after["background_work_items"], "draft generation must not request FX coverage")
	assert.Equal(t, before["reconciliation_checkpoints"], after["reconciliation_checkpoints"])
	assert.Equal(t, before["audit_events"]+1, after["audit_events"], "one audit for the atomic generation operation")
	result, err = s.GenerateDue(ctx, GenerateRecurringInput{OwnerUserID: input.OwnerUserID})
	require.NoError(t, err)
	assert.Zero(t, result.Generated)
	assert.Equal(t, after, recurringCounts(t, f.database))
	rows, err := s.repository.ListRecurringOccurrences(ctx, BookID, template.ID, 100)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, "2026-09-01", rows[0].OccurrenceDate)
	draft, err := f.transactionService.repository.TransactionByID(ctx, BookID, rows[0].TransactionID.Int64)
	require.NoError(t, err)
	assert.Equal(t, "draft", draft.Status)
	assert.Equal(t, "2026-09-01", draft.TransactionDate)
	list, err := f.transactionService.repository.ListTransactions(ctx, db.ListTransactionsParams{BookID: BookID, ExcludeDraft: true, Limit: 100})
	require.NoError(t, err)
	assert.Empty(t, list)
}

func TestCreatingATemplateWithAPastStartDateBackfillsNothing(t *testing.T) {
	_, s, input := recurringFixture(t)
	template, err := s.CreateTemplate(context.Background(), input)
	require.NoError(t, err)
	_, err = s.GenerateDue(context.Background(), GenerateRecurringInput{OwnerUserID: input.OwnerUserID})
	require.NoError(t, err)
	rows, err := s.repository.ListRecurringOccurrences(context.Background(), BookID, template.ID, 1000)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, "2026-09-01", rows[0].OccurrenceDate)
}

func TestConcurrentGeneratorsProduceOneOccurrence(t *testing.T) {
	f, s, input := recurringFixture(t)
	_, err := s.CreateTemplate(context.Background(), input)
	require.NoError(t, err)
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for range 8 {
		wg.Go(func() {
			_, err := s.GenerateDue(context.Background(), GenerateRecurringInput{OwnerUserID: input.OwnerUserID})
			errs <- err
		})
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
	counts := recurringCounts(t, f.database)
	assert.Equal(t, 1, counts["transactions"])
	assert.Equal(t, 1, counts["recurring_occurrences"])
}

func TestConcurrentRecurringGeneratorsWithIndependentDatabasePools(t *testing.T) {
	f, first, input := recurringFixture(t)
	_, err := first.CreateTemplate(context.Background(), input)
	require.NoError(t, err)
	var file string
	require.NoError(t, f.database.QueryRow(`SELECT file FROM pragma_database_list WHERE name = 'main'`).Scan(&file))
	other, err := db.Open(context.Background(), "file:"+file)
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
			_, err := service.GenerateDue(context.Background(), GenerateRecurringInput{OwnerUserID: input.OwnerUserID})
			errs <- err
		})
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
	counts := recurringCounts(t, f.database)
	assert.Equal(t, 1, counts["transactions"])
	assert.Equal(t, 1, counts["recurring_occurrences"])
}

func TestRecurringCatchUpCapHoldsWatermarkUntilWindowComplete(t *testing.T) {
	f, s, input := recurringFixture(t)
	input.Patch.Frequency = recurringTestPtr("daily")
	input.Patch.DayOfMonth = NullablePatch[int]{}
	input.Patch.LeadDays = recurringTestPtr(0)
	template, err := s.CreateTemplate(context.Background(), input)
	require.NoError(t, err)
	s.now = func() time.Time { return time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC) }
	result, err := s.GenerateDue(context.Background(), GenerateRecurringInput{OwnerUserID: input.OwnerUserID})
	require.NoError(t, err)
	assert.Equal(t, 50, result.Generated)
	partial, err := s.Template(context.Background(), input.OwnerUserID, template.ID)
	require.NoError(t, err)
	assert.Equal(t, template.Spec.GenerateFrom, partial.Spec.GenerateFrom)
	result, err = s.GenerateDue(context.Background(), GenerateRecurringInput{OwnerUserID: input.OwnerUserID})
	require.NoError(t, err)
	assert.Equal(t, 14, result.Generated)
	complete, err := s.Template(context.Background(), input.OwnerUserID, template.ID)
	require.NoError(t, err)
	assert.Equal(t, "2026-11-02", complete.Spec.GenerateFrom)
	assert.Equal(t, 64, recurringCounts(t, f.database)["transactions"])
}

func TestRecurringValidationFailureIsBlockedOnceAndDoesNotStopOtherTemplates(t *testing.T) {
	f, s, input := recurringFixture(t)
	ctx := context.Background()
	bad, err := s.CreateTemplate(ctx, input)
	require.NoError(t, err)
	input.Patch.Name = recurringTestPtr("Second template")
	_, err = s.CreateTemplate(ctx, input)
	require.NoError(t, err)
	// Simulate stored corruption; creation itself refuses imbalance.
	_, err = f.database.Exec(`UPDATE recurring_template_postings SET quantity_value = '1' WHERE template_id = ? AND line_seq = 1`, bad.ID)
	require.NoError(t, err)
	result, err := s.GenerateDue(ctx, GenerateRecurringInput{OwnerUserID: input.OwnerUserID})
	require.NoError(t, err)
	assert.Equal(t, RecurringGenerationResult{Generated: 1, Blocked: 1}, result)
	rows, err := s.repository.ListRecurringOccurrences(ctx, BookID, bad.ID, 100)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, "blocked", rows[0].Status)
	assert.Contains(t, rows[0].ErrorSummary, "balanced")
	assert.False(t, rows[0].TransactionID.Valid)
	before := recurringCounts(t, f.database)
	_, err = s.GenerateDue(ctx, GenerateRecurringInput{OwnerUserID: input.OwnerUserID})
	require.NoError(t, err)
	assert.Equal(t, before, recurringCounts(t, f.database))
}

func TestRecurringGenerationRevalidatesAccountsAtOccurrenceDate(t *testing.T) {
	f, s, input := recurringFixture(t)
	_, err := s.CreateTemplate(context.Background(), input)
	require.NoError(t, err)
	_, err = f.accountService.CloseAccount(context.Background(), AccountLifecycleInput{OwnerUserID: input.OwnerUserID, OriginType: "browser_api", AccountID: f.cashAccountID, EffectiveFrom: "2026-08-31", ClosedOn: "2026-08-31"})
	require.NoError(t, err)
	result, err := s.GenerateDue(context.Background(), GenerateRecurringInput{OwnerUserID: input.OwnerUserID})
	require.NoError(t, err)
	assert.Equal(t, RecurringGenerationResult{Blocked: 1}, result)
	assert.Zero(t, recurringCounts(t, f.database)["transactions"])
}

func TestRecurringGenerationRollsBackDraftAuditAndOccurrenceOnFailure(t *testing.T) {
	f, s, input := recurringFixture(t)
	template, err := s.CreateTemplate(context.Background(), input)
	require.NoError(t, err)
	_, err = f.database.Exec(`CREATE TRIGGER fail_occurrence BEFORE INSERT ON recurring_occurrences BEGIN SELECT RAISE(ABORT, 'injected failure'); END`)
	require.NoError(t, err)
	before := recurringCounts(t, f.database)
	_, err = s.GenerateDue(context.Background(), GenerateRecurringInput{OwnerUserID: input.OwnerUserID})
	require.Error(t, err)
	assert.Equal(t, before, recurringCounts(t, f.database))
	unchanged, err := s.Template(context.Background(), input.OwnerUserID, template.ID)
	require.NoError(t, err)
	assert.Equal(t, template.Spec.GenerateFrom, unchanged.Spec.GenerateFrom)
	_, err = f.database.Exec(`DROP TRIGGER fail_occurrence`)
	require.NoError(t, err)
	result, err := s.GenerateDue(context.Background(), GenerateRecurringInput{OwnerUserID: input.OwnerUserID})
	require.NoError(t, err)
	assert.Equal(t, 1, result.Generated)
}

func TestConcurrentTemplateEditCannotAdvanceAStaleGenerationWatermark(t *testing.T) {
	for _, change := range []string{"schedule", "disable", "archive"} {
		t.Run(change, func(t *testing.T) {
			f, s, input := recurringFixture(t)
			ctx := context.Background()
			captured, err := s.CreateTemplate(ctx, input)
			require.NoError(t, err)
			// Complete an edit after the generator captured its old snapshot,
			// before it enters either guarded repository write.
			edit := WriteRecurringTemplateInput{OwnerUserID: input.OwnerUserID, TemplateID: captured.ID}
			switch change {
			case "schedule":
				edit.Patch.DayOfMonth = NullablePatch[int]{Set: true, Value: recurringTestPtr(2)}
			case "disable":
				edit.Patch.Enabled = recurringTestPtr(false)
			}
			var updated RecurringTemplate
			if change == "archive" {
				updated, err = s.ArchiveTemplate(ctx, edit)
			} else {
				updated, err = s.UpdateTemplate(ctx, edit)
			}
			require.NoError(t, err)
			before := recurringCounts(t, f.database)
			_, err = s.materializeOccurrence(ctx, GenerateRecurringInput{OwnerUserID: input.OwnerUserID}, captured, "2026-09-01")
			require.ErrorIs(t, err, db.ErrRecurringTemplateConflict)
			err = s.repository.AdvanceRecurringGeneration(ctx, BookID, captured.ID, captured.Revision, "2026-09-05", "2026-08-30T12:00:00Z")
			require.ErrorIs(t, err, db.ErrRecurringTemplateConflict)
			assert.Equal(t, before, recurringCounts(t, f.database))
			after, err := s.Template(ctx, input.OwnerUserID, captured.ID)
			require.NoError(t, err)
			assert.Equal(t, updated, after)
			result, err := s.GenerateDue(ctx, GenerateRecurringInput{OwnerUserID: input.OwnerUserID})
			require.NoError(t, err)
			if change == "schedule" {
				assert.Equal(t, 1, result.Generated)
			} else {
				assert.Zero(t, result.Generated)
			}
		})
	}
}

func TestRecurringSchedulerUsesOwnerLocalDateAndUTCAttribution(t *testing.T) {
	f, s, input := recurringFixture(t)
	ctx := context.Background()
	input.Patch.Frequency = recurringTestPtr("daily")
	input.Patch.DayOfMonth = NullablePatch[int]{}
	input.Patch.LeadDays = recurringTestPtr(0)
	_, err := s.settings.SavePreferences(ctx, SaveUserPreferencesInput{UserID: input.OwnerUserID, TimeZone: "Europe/Amsterdam"})
	require.NoError(t, err)
	s.now = func() time.Time { return time.Date(2026, 8, 30, 23, 30, 0, 0, time.UTC) }
	_, err = s.CreateTemplate(ctx, input)
	require.NoError(t, err)
	s.scheduleRecurringIfDue(ctx, slog.New(slog.NewTextHandler(io.Discard, nil)))
	var date, at, origin string
	var actor int64
	require.NoError(t, f.database.QueryRow(`SELECT o.occurrence_date, o.materialized_at, a.actor_user_id, a.origin_type FROM recurring_occurrences o JOIN audit_events a ON a.id = o.last_audit_event_id`).Scan(&date, &at, &actor, &origin))
	assert.Equal(t, "2026-08-31", date)
	assert.Equal(t, "2026-08-30T23:30:00Z", at)
	assert.Equal(t, input.OwnerUserID, actor)
	assert.Equal(t, "scheduled", origin)
}

func TestSkippingAFutureOccurrenceStopsItGenerating(t *testing.T) {
	_, s, input := recurringFixture(t)
	ctx := context.Background()
	template, err := s.CreateTemplate(ctx, input)
	require.NoError(t, err)
	_, err = s.repository.CreateRecurringOccurrence(ctx, db.CreateRecurringOccurrenceParams{BookID: BookID, TemplateID: template.ID, OccurrenceDate: "2026-09-01", Status: "skipped", SkipReason: "not this month", MaterializedAt: "2026-08-30T12:00:00Z"})
	require.NoError(t, err)
	result, err := s.GenerateDue(ctx, GenerateRecurringInput{OwnerUserID: input.OwnerUserID})
	require.NoError(t, err)
	assert.Zero(t, result.Generated)
}

func TestArchivingATemplateLeavesItsGeneratedDraftsAlone(t *testing.T) {
	f, s, input := recurringFixture(t)
	ctx := context.Background()
	template, err := s.CreateTemplate(ctx, input)
	require.NoError(t, err)
	_, err = s.GenerateDue(ctx, GenerateRecurringInput{OwnerUserID: input.OwnerUserID})
	require.NoError(t, err)
	_, err = s.ArchiveTemplate(ctx, WriteRecurringTemplateInput{OwnerUserID: input.OwnerUserID, TemplateID: template.ID})
	require.NoError(t, err)
	s.now = func() time.Time { return time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC) }
	before := recurringCounts(t, f.database)
	result, err := s.GenerateDue(ctx, GenerateRecurringInput{OwnerUserID: input.OwnerUserID})
	require.NoError(t, err)
	assert.Zero(t, result.Generated)
	assert.Equal(t, before, recurringCounts(t, f.database))
	assert.Equal(t, 1, before["transactions"])
}
