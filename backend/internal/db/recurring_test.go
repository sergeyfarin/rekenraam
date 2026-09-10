package db

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rekenraam/backend/internal/exact"
)

// One book, because the schema allows exactly one: books.id carries
// CHECK (id = 1). The same-book triggers are therefore exercised through the
// arm a cross-book row would hit anyway — the target is not in this book —
// with ids that exist nowhere.
func newRecurringTestDatabase(t testing.TB) *sql.DB {
	t.Helper()

	ctx := context.Background()
	database, err := Open(ctx, "file:"+filepath.Join(t.TempDir(), "recurring.sqlite"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, database.Close()) })
	require.NoError(t, Migrate(ctx, database))

	_, err = database.ExecContext(ctx, `
		INSERT INTO users (id, username, password_hash, is_owner, created_at, updated_at)
		VALUES (1, 'owner', 'x', 1, '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z');

		INSERT INTO books (id, code, name, owner_user_id, created_at, updated_at, updated_by_user_id)
		VALUES (1, 'personal', 'Personal', 1, '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z', 1);

		INSERT INTO commodities (id, book_id, code, kind, is_builtin, created_at, created_by_user_id)
		VALUES (1, 1, 'EUR', 'currency', 1, '2026-01-01T00:00:00Z', 1);

		INSERT INTO commodity_versions (
			commodity_id, version_seq, effective_from, recorded_at, changed_by_user_id, change_reason,
			status, symbol, display_symbol, name, standard_scale, max_quantity_scale
		) VALUES (1, 1, '2026-01-01', '2026-01-01T00:00:00Z', 1, 'seed', 'active', '€', '€', 'Euro', 2, 2);

		INSERT INTO accounts (id, book_id, created_at, created_by_user_id)
		VALUES (1, 1, '2026-01-01T00:00:00Z', 1), (2, 1, '2026-01-01T00:00:00Z', 1);

		INSERT INTO account_versions (
			account_id, version_seq, effective_from, recorded_at, changed_by_user_id, change_reason,
			status, opened_on, name, account_class, account_kind, allows_postings
		) VALUES
			(1, 1, '2026-01-01', '2026-01-01T00:00:00Z', 1, 'seed', 'active', '2026-01-01', 'Checking', 'asset', 'checking', 1),
			(2, 1, '2026-01-01', '2026-01-01T00:00:00Z', 1, 'seed', 'active', '2026-01-01', 'Rent', 'expense', 'expense', 1);

		INSERT INTO payees (id, book_id, created_at, created_by_user_id)
		VALUES (1, 1, '2026-01-01T00:00:00Z', 1);

		INSERT INTO payee_versions (
			payee_id, version_seq, effective_from, recorded_at, changed_by_user_id, change_reason,
			status, name, normalized_name
		) VALUES (1, 1, '2026-01-01', '2026-01-01T00:00:00Z', 1, 'seed', 'active', 'Landlord', 'landlord');

		INSERT INTO tags (id, book_id, name, kind, status, created_at, created_by_user_id, updated_at, updated_by_user_id)
		VALUES (1, 1, 'housing', 'custom', 'active', '2026-01-01T00:00:00Z', 1, '2026-01-01T00:00:00Z', 1);
	`)
	require.NoError(t, err)
	return database
}

func rentTemplateSpec() RecurringTemplateSpec {
	dayOfMonth := 1
	payeeID := int64(1)
	return RecurringTemplateSpec{
		Name:            "Rent",
		Enabled:         true,
		TransactionKind: "ordinary",
		PayeeID:         &payeeID,
		Description:     "Monthly rent",
		Frequency:       "monthly",
		IntervalCount:   1,
		DayOfMonth:      &dayOfMonth,
		StartsOn:        "2026-09-01",
		LeadDays:        5,
		GenerateFrom:    "2026-09-01",
		Postings: []RecurringTemplatePostingSpec{
			{LineKey: "rent", AccountID: 2, QuantityValue: exact.MustParse("120000"), QuantityScale: 2, CommodityID: 1, Memo: "rent"},
			{LineKey: "cash", AccountID: 1, QuantityValue: exact.MustParse("-120000"), QuantityScale: 2, CommodityID: 1},
		},
		TagIDs: []int64{1},
	}
}

func createRentTemplate(t *testing.T, repository *RecurringRepository) RecurringTemplateRecord {
	t.Helper()

	record, err := repository.CreateRecurringTemplate(context.Background(), CreateRecurringTemplateParams{
		BookID:      1,
		ActorUserID: 1,
		CreatedAt:   "2026-08-29T10:00:00Z",
		Spec:        rentTemplateSpec(),
	})
	require.NoError(t, err)
	return record
}

func TestCreateRecurringTemplateRoundTripsPostingsAndTags(t *testing.T) {
	t.Parallel()

	repository := NewRecurringRepository(newRecurringTestDatabase(t))
	record := createRentTemplate(t, repository)

	assert.Equal(t, "Rent", record.Name)
	assert.True(t, record.Enabled)
	assert.False(t, record.ArchivedAt.Valid)
	assert.Equal(t, "monthly", record.Frequency)
	assert.Equal(t, int64(1), record.DayOfMonth.Int64)
	assert.False(t, record.LastDayOfMonth)
	assert.Equal(t, "2026-09-01", record.StartsOn)
	assert.Equal(t, "2026-09-01", record.GenerateFrom)
	assert.Equal(t, 5, record.LeadDays)
	// An open-ended series stores NULL, not a sentinel date.
	assert.False(t, record.EndsOn.Valid)
	assert.False(t, record.MaxOccurrences.Valid)
	// The payee name comes from the payee's current version, not from the
	// free-text column, so a renamed payee is not stale here.
	assert.Equal(t, "Landlord", record.PayeeName.String)

	require.Len(t, record.Postings, 2)
	assert.Equal(t, 1, record.Postings[0].LineSeq)
	assert.Equal(t, "rent", record.Postings[0].LineKey)
	assert.Equal(t, "120000", record.Postings[0].QuantityValue.String())
	assert.Equal(t, 2, record.Postings[0].QuantityScale)
	assert.Equal(t, "-120000", record.Postings[1].QuantityValue.String())
	assert.Equal(t, []int64{1}, record.TagIDs)
}

func TestRecurringTemplateRefusesTargetsOutsideItsBook(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	repository := NewRecurringRepository(newRecurringTestDatabase(t))

	t.Run("account", func(t *testing.T) {
		spec := rentTemplateSpec()
		spec.Postings[0].AccountID = 404
		_, err := repository.CreateRecurringTemplate(ctx, CreateRecurringTemplateParams{
			BookID: 1, ActorUserID: 1, CreatedAt: "2026-08-29T10:00:00Z", Spec: spec,
		})
		require.ErrorContains(t, err, "must belong to the same book")
	})

	t.Run("commodity", func(t *testing.T) {
		spec := rentTemplateSpec()
		spec.Postings[0].CommodityID = 404
		_, err := repository.CreateRecurringTemplate(ctx, CreateRecurringTemplateParams{
			BookID: 1, ActorUserID: 1, CreatedAt: "2026-08-29T10:00:00Z", Spec: spec,
		})
		require.ErrorContains(t, err, "must belong to the same book")
	})

	t.Run("tag", func(t *testing.T) {
		spec := rentTemplateSpec()
		spec.TagIDs = []int64{404}
		_, err := repository.CreateRecurringTemplate(ctx, CreateRecurringTemplateParams{
			BookID: 1, ActorUserID: 1, CreatedAt: "2026-08-29T10:00:00Z", Spec: spec,
		})
		require.ErrorContains(t, err, "must belong to the same book")
	})

	t.Run("payee", func(t *testing.T) {
		spec := rentTemplateSpec()
		otherPayee := int64(404)
		spec.PayeeID = &otherPayee
		_, err := repository.CreateRecurringTemplate(ctx, CreateRecurringTemplateParams{
			BookID: 1, ActorUserID: 1, CreatedAt: "2026-08-29T10:00:00Z", Spec: spec,
		})
		require.ErrorContains(t, err, "must belong to the same book")
	})

	// A refused create leaves nothing behind: the whole thing is one
	// transaction, so a rejected child rolls the template back with it.
	remaining, err := repository.ListRecurringTemplates(ctx, ListRecurringTemplatesParams{BookID: 1, IncludeArchived: true})
	require.NoError(t, err)
	assert.Empty(t, remaining)
}

func TestUpdateRecurringTemplateReplacesPostingsAndTags(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	repository := NewRecurringRepository(newRecurringTestDatabase(t))
	created := createRentTemplate(t, repository)

	spec := rentTemplateSpec()
	spec.Name = "Rent (increased)"
	spec.Postings = []RecurringTemplatePostingSpec{
		{LineKey: "rent", AccountID: 2, QuantityValue: exact.MustParse("135000"), QuantityScale: 2, CommodityID: 1},
		{LineKey: "cash", AccountID: 1, QuantityValue: exact.MustParse("-135000"), QuantityScale: 2, CommodityID: 1},
	}
	spec.TagIDs = nil

	updated, err := repository.UpdateRecurringTemplate(ctx, UpdateRecurringTemplateParams{
		BookID: 1, TemplateID: created.ID, ActorUserID: 1, UpdatedAt: "2026-08-30T10:00:00Z", Spec: spec,
	})
	require.NoError(t, err)

	assert.Equal(t, "Rent (increased)", updated.Name)
	require.Len(t, updated.Postings, 2, "the replacement must not accumulate old lines")
	assert.Equal(t, "135000", updated.Postings[0].QuantityValue.String())
	assert.Empty(t, updated.TagIDs, "clearing the tags must actually clear them")
}

// Both write paths check the template exists in this book before they write
// anything, including the audit event: an update that cannot happen must not
// leave a record saying it did.
func TestUpdateAndArchiveRefuseAnUnknownTemplate(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	database := newRecurringTestDatabase(t)
	repository := NewRecurringRepository(database)

	_, err := repository.UpdateRecurringTemplate(ctx, UpdateRecurringTemplateParams{
		BookID: 1, TemplateID: 999, ActorUserID: 1, UpdatedAt: "2026-08-30T10:00:00Z", Spec: rentTemplateSpec(),
	})
	require.ErrorIs(t, err, ErrRecurringTemplateNotFound)

	_, err = repository.ArchiveRecurringTemplate(ctx, ArchiveRecurringTemplateParams{
		BookID: 1, TemplateID: 999, ActorUserID: 1, ArchivedAt: "2026-08-30T10:00:00Z",
	})
	require.ErrorIs(t, err, ErrRecurringTemplateNotFound)

	var auditEvents int
	require.NoError(t, database.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM audit_events WHERE operation LIKE 'recurring.%'`).Scan(&auditEvents))
	assert.Zero(t, auditEvents)
}

// Archiving retires the template and stops the generator, and leaves every
// occurrence it already produced exactly where it is. Those drafts are the
// user's transactions from the moment they exist.
func TestArchiveRecurringTemplateKeepsItsOccurrences(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	repository := NewRecurringRepository(newRecurringTestDatabase(t))
	created := createRentTemplate(t, repository)

	_, err := repository.CreateRecurringOccurrence(ctx, CreateRecurringOccurrenceParams{
		BookID: 1, TemplateID: created.ID, OccurrenceDate: "2026-09-01", Status: "skipped",
		SkipReason: "paid by hand", MaterializedAt: "2026-08-29T10:00:00Z",
	})
	require.NoError(t, err)

	archived, err := repository.ArchiveRecurringTemplate(ctx, ArchiveRecurringTemplateParams{
		BookID: 1, TemplateID: created.ID, ActorUserID: 1, ArchivedAt: "2026-08-30T10:00:00Z",
	})
	require.NoError(t, err)
	assert.True(t, archived.ArchivedAt.Valid)
	assert.False(t, archived.Enabled, "archiving must also stop the generator")

	occurrences, err := repository.ListRecurringOccurrences(ctx, 1, created.ID, 0)
	require.NoError(t, err)
	require.Len(t, occurrences, 1)
	assert.Equal(t, "2026-09-01", occurrences[0].OccurrenceDate)
}

func TestListRecurringTemplatesExcludesArchivedUnlessAsked(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	repository := NewRecurringRepository(newRecurringTestDatabase(t))
	created := createRentTemplate(t, repository)
	_, err := repository.ArchiveRecurringTemplate(ctx, ArchiveRecurringTemplateParams{
		BookID: 1, TemplateID: created.ID, ActorUserID: 1, ArchivedAt: "2026-08-30T10:00:00Z",
	})
	require.NoError(t, err)

	visible, err := repository.ListRecurringTemplates(ctx, ListRecurringTemplatesParams{BookID: 1})
	require.NoError(t, err)
	assert.Empty(t, visible)

	all, err := repository.ListRecurringTemplates(ctx, ListRecurringTemplatesParams{BookID: 1, IncludeArchived: true})
	require.NoError(t, err)
	assert.Len(t, all, 1)

	// The generator's own sweep never sees it either, and an archived template
	// is disabled, so both halves of the filter agree.
	due, err := repository.ListRecurringTemplates(ctx, ListRecurringTemplatesParams{BookID: 1, EnabledOnly: true})
	require.NoError(t, err)
	assert.Empty(t, due)
}

// The unique constraint is the whole idempotency story: two schedulers
// agreeing on the same due date, or a restart mid-tick, converge on one row.
func TestRecurringOccurrenceIsUniquePerTemplateAndDate(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	repository := NewRecurringRepository(newRecurringTestDatabase(t))
	created := createRentTemplate(t, repository)

	params := CreateRecurringOccurrenceParams{
		BookID: 1, TemplateID: created.ID, OccurrenceDate: "2026-09-01", Status: "blocked",
		ErrorSummary: "account is archived", MaterializedAt: "2026-08-29T10:00:00Z",
	}
	first, err := repository.CreateRecurringOccurrence(ctx, params)
	require.NoError(t, err)
	assert.Equal(t, "blocked", first.Status)

	_, err = repository.CreateRecurringOccurrence(ctx, params)
	require.ErrorIs(t, err, ErrRecurringOccurrenceExists,
		"a second row for one date must be reported as the normal conflict it is")
}

// Each status carries what makes it meaningful, refused at the schema so no
// caller can write a 'generated' row that generated nothing.
func TestRecurringOccurrenceStatusMustCarryItsEvidence(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	repository := NewRecurringRepository(newRecurringTestDatabase(t))
	created := createRentTemplate(t, repository)
	strayTransaction := int64(1)

	_, err := repository.CreateRecurringOccurrence(ctx, CreateRecurringOccurrenceParams{
		BookID: 1, TemplateID: created.ID, OccurrenceDate: "2026-09-01", Status: "generated",
		MaterializedAt: "2026-08-29T10:00:00Z",
	})
	require.Error(t, err, "a generated occurrence without a transaction is not generated")

	_, err = repository.CreateRecurringOccurrence(ctx, CreateRecurringOccurrenceParams{
		BookID: 1, TemplateID: created.ID, OccurrenceDate: "2026-09-02", Status: "blocked",
		MaterializedAt: "2026-08-29T10:00:00Z",
	})
	require.Error(t, err, "a blocked occurrence must say why")

	_, err = repository.CreateRecurringOccurrence(ctx, CreateRecurringOccurrenceParams{
		BookID: 1, TemplateID: created.ID, OccurrenceDate: "2026-09-03", Status: "skipped",
		TransactionID: &strayTransaction, MaterializedAt: "2026-08-29T10:00:00Z",
	})
	require.Error(t, err, "a skipped occurrence produced nothing, so it points at nothing")
}

// What the generator subtracts from the enumerator's answer. A skipped or
// blocked date is in here too, so neither is silently retried.
func TestRecurringOccurrenceDatesReportsEveryMaterializedDate(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	repository := NewRecurringRepository(newRecurringTestDatabase(t))
	created := createRentTemplate(t, repository)

	for _, occurrence := range []CreateRecurringOccurrenceParams{
		{OccurrenceDate: "2026-09-01", Status: "skipped", SkipReason: "paid by hand"},
		{OccurrenceDate: "2026-10-01", Status: "blocked", ErrorSummary: "account is archived"},
		{OccurrenceDate: "2026-12-01", Status: "skipped", SkipReason: "moving out"},
	} {
		occurrence.BookID = 1
		occurrence.TemplateID = created.ID
		occurrence.MaterializedAt = "2026-08-29T10:00:00Z"
		_, err := repository.CreateRecurringOccurrence(ctx, occurrence)
		require.NoError(t, err)
	}

	dates, err := repository.RecurringOccurrenceDates(ctx, 1, created.ID, "2026-09-01", "2026-11-30")
	require.NoError(t, err)
	assert.Equal(t, map[string]string{
		"2026-09-01": "skipped",
		"2026-10-01": "blocked",
	}, dates, "the window bounds are inclusive and December is outside them")
}

// The watermark only ever moves forward. A tick that runs twice, or two
// processes racing on the same template, must not rewind generation into
// dates that were already handled.
func TestAdvanceRecurringGenerationNeverMovesBackwards(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	repository := NewRecurringRepository(newRecurringTestDatabase(t))
	created := createRentTemplate(t, repository)

	require.NoError(t, repository.AdvanceRecurringGeneration(ctx, 1, created.ID, created.Revision, "2026-10-01", "2026-09-30T10:00:00Z"))
	advanced, err := repository.RecurringTemplateByID(ctx, 1, created.ID)
	require.NoError(t, err)
	assert.Equal(t, "2026-10-01", advanced.GenerateFrom)

	require.NoError(t, repository.AdvanceRecurringGeneration(ctx, 1, created.ID, advanced.Revision, "2026-09-01", "2026-09-30T11:00:00Z"))
	unchanged, err := repository.RecurringTemplateByID(ctx, 1, created.ID)
	require.NoError(t, err)
	assert.Equal(t, "2026-10-01", unchanged.GenerateFrom, "an earlier watermark is ignored, not applied")
}

// The per-frequency field requirements are refused by the schema as well as by
// the service, so a template the enumerator cannot read can never be stored —
// whatever writes it.
func TestSchemaRefusesUnusableRecurrenceCombinations(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	database := newRecurringTestDatabase(t)

	insert := func(columns string, values string) error {
		_, err := database.ExecContext(ctx, `
			INSERT INTO recurring_templates (
				book_id, name, transaction_kind, starts_on, generate_from,
				created_at, created_by_user_id, updated_at, `+columns+`
			) VALUES (
				1, 'T', 'ordinary', '2026-09-01', '2026-09-01',
				'2026-01-01T00:00:00Z', 1, '2026-01-01T00:00:00Z', `+values+`
			)
		`)
		return err
	}

	require.NoError(t, insert("frequency, interval_count, day_of_month", "'monthly', 1, 1"),
		"the shape every other case here is a deviation from")

	assert.Error(t, insert("frequency, interval_count", "'weekly', 1"),
		"a weekly schedule with no weekday cannot be enumerated")
	assert.Error(t, insert("frequency, interval_count", "'monthly', 1"),
		"a monthly schedule needs a day rule")
	assert.Error(t, insert("frequency, interval_count, day_of_month, last_day_of_month", "'monthly', 1, 15, 1"),
		"a nominal day and the last-day flag are mutually exclusive")
	assert.Error(t, insert("frequency, interval_count, day_of_month", "'yearly', 1, 15"),
		"a yearly schedule needs a month")
	assert.Error(t, insert("frequency, interval_count, day_of_month", "'monthly', 0, 1"),
		"an interval of zero is an infinite loop, not a schedule")
	assert.Error(t, insert("frequency, interval_count, day_of_month, ends_on", "'monthly', 1, 1, '2026-08-01'"),
		"a series cannot end before it starts")
	assert.Error(t, insert("frequency, interval_count, by_weekday", "'daily', 1, 3"),
		"a daily schedule carrying a weekday means two different things at once")
	assert.Error(t, insert("frequency, interval_count, by_weekday, day_of_month", "'weekly', 1, 3, 15"),
		"a weekly schedule carrying a day of the month does too")
	assert.Error(t, insert("frequency, interval_count, day_of_month, month_of_year", "'monthly', 1, 15, 3"),
		"a monthly schedule has no month of the year")
	assert.Error(t, insert("frequency, interval_count, day_of_month, lead_days", "'monthly', 1, 1, 365"),
		"a lead time longer than the schedule itself is not a lead time")
}

func TestRecurringTemplateByIDReportsAMissingTemplate(t *testing.T) {
	t.Parallel()

	repository := NewRecurringRepository(newRecurringTestDatabase(t))
	_, err := repository.RecurringTemplateByID(context.Background(), 1, 999)
	require.ErrorIs(t, err, ErrRecurringTemplateNotFound)
}
