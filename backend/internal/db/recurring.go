package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"rekenraam/backend/internal/exact"
)

// ErrRecurringTemplateNotFound is the repository's answer for a template that
// does not exist in this book, kept distinct from ErrNotFound so a handler can
// say which thing was missing.
var ErrRecurringTemplateNotFound = errors.New("recurring template not found")

var ErrRecurringTemplateArchived = errors.New("recurring template is archived")
var ErrRecurringTemplateConflict = errors.New("recurring template changed concurrently")

// ErrRecurringOccurrenceExists means the template already has a row for this
// date. Like ErrBackupOccurrenceExists it is a normal outcome — two schedulers
// agreeing on the same due date, or a restart mid-tick — and the caller adopts
// the existing row rather than treating it as a failure.
var ErrRecurringOccurrenceExists = errors.New("a recurring occurrence already exists for this date")

// isRecurringOccurrenceConflict recognizes the unique index on (template_id,
// occurrence_date) refusing a second row for one occurrence. Named by its
// columns rather than by "any unique violation", so an unrelated constraint on
// this table is never silently reported as a duplicate occurrence.
func isRecurringOccurrenceConflict(err error) bool {
	return err != nil && strings.Contains(err.Error(),
		"UNIQUE constraint failed: recurring_occurrences.template_id, recurring_occurrences.occurrence_date")
}

// RecurringTemplateRecord is a template with its postings and tags. The
// schedule fields mirror recur.ScheduleSpec; the app layer converts between
// them, because the repository holds no business rules and the enumerator
// holds no SQL.
type RecurringTemplateRecord struct {
	ID              int64
	Revision        int64
	BookID          int64
	Name            string
	Enabled         bool
	ArchivedAt      sql.NullString
	TransactionKind string
	PayeeID         sql.NullInt64
	PayeeName       sql.NullString
	Description     string
	NoteMarkdown    string

	Frequency      string
	IntervalCount  int
	ByWeekday      sql.NullInt64
	DayOfMonth     sql.NullInt64
	LastDayOfMonth bool
	MonthOfYear    sql.NullInt64
	StartsOn       string
	EndsOn         sql.NullString
	MaxOccurrences sql.NullInt64
	LeadDays       int
	GenerateFrom   string

	CreatedAt string
	UpdatedAt string

	Postings []RecurringTemplatePostingRecord
	TagIDs   []int64
}

type RecurringTemplatePostingRecord struct {
	ID            int64
	TemplateID    int64
	LineSeq       int
	LineKey       string
	AccountID     int64
	QuantityValue exact.Coefficient
	QuantityScale int
	CommodityID   int64
	Memo          string
}

// RecurringTemplateSpec is the writable shape of a template. Optional schedule
// fields are pointers so an omitted one is distinguishable from a zero — the
// PATCH-omission bug class this repo keeps finding (T-36/T-45/T-47 for the
// money variant) starts one layer up, and the repository does not undo it.
type RecurringTemplateSpec struct {
	Name            string
	Enabled         bool
	TransactionKind string
	PayeeID         *int64
	PayeeName       string
	Description     string
	NoteMarkdown    string

	Frequency      string
	IntervalCount  int
	ByWeekday      *int
	DayOfMonth     *int
	LastDayOfMonth bool
	MonthOfYear    *int
	StartsOn       string
	EndsOn         string
	MaxOccurrences *int
	LeadDays       int
	GenerateFrom   string

	Postings []RecurringTemplatePostingSpec
	TagIDs   []int64
}

type RecurringTemplatePostingSpec struct {
	LineKey       string
	AccountID     int64
	QuantityValue exact.Coefficient
	QuantityScale int
	CommodityID   int64
	Memo          string
}

type CreateRecurringTemplateParams struct {
	BookID        int64
	ActorUserID   int64
	AuthSessionID int64
	RequestID     string
	CreatedAt     string
	Spec          RecurringTemplateSpec
}

type UpdateRecurringTemplateParams struct {
	BookID           int64
	TemplateID       int64
	ActorUserID      int64
	AuthSessionID    int64
	RequestID        string
	UpdatedAt        string
	Spec             RecurringTemplateSpec
	ExpectedRevision *int64
}

type ArchiveRecurringTemplateParams struct {
	BookID        int64
	TemplateID    int64
	ActorUserID   int64
	AuthSessionID int64
	RequestID     string
	ArchivedAt    string
}

type ListRecurringTemplatesParams struct {
	BookID          int64
	IncludeArchived bool
	// EnabledOnly narrows to what the generator sweeps: enabled and
	// unarchived. It is a separate flag rather than a second method so the
	// generator and the UI read the same rows through the same SQL.
	EnabledOnly bool
}

type RecurringRepository struct {
	database *sql.DB
}

func NewRecurringRepository(database *sql.DB) *RecurringRepository {
	return &RecurringRepository{database: database}
}

func (r *RecurringRepository) CreateRecurringTemplate(ctx context.Context, params CreateRecurringTemplateParams) (RecurringTemplateRecord, error) {
	tx, err := r.database.BeginTx(ctx, nil)
	if err != nil {
		return RecurringTemplateRecord{}, fmt.Errorf("begin create recurring template: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			rollbackTx(ctx, tx)
		}
	}()

	if _, err := readBookForUpdate(ctx, tx, params.BookID); err != nil {
		return RecurringTemplateRecord{}, err
	}
	if _, err := insertAuditEvent(ctx, tx, AuditEventParams{
		BookID:        params.BookID,
		ActorUserID:   params.ActorUserID,
		AuthSessionID: params.AuthSessionID,
		OccurredAt:    params.CreatedAt,
		RequestID:     params.RequestID,
		OriginType:    "browser_api",
		Operation:     "recurring.template.create",
		Reason:        "recurring template created",
	}); err != nil {
		return RecurringTemplateRecord{}, err
	}

	spec := params.Spec
	result, err := tx.ExecContext(ctx, `
		INSERT INTO recurring_templates (
			book_id, name, enabled, transaction_kind, payee_id, payee_name,
			description, note_markdown, frequency, interval_count, by_weekday,
			day_of_month, last_day_of_month, month_of_year, starts_on, ends_on,
			max_occurrences, lead_days, generate_from,
			created_at, created_by_user_id, updated_at, updated_by_user_id
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		params.BookID, spec.Name, spec.Enabled, spec.TransactionKind,
		nullableOptionalInt64(spec.PayeeID), nullableNonEmptyText(spec.PayeeName),
		spec.Description, spec.NoteMarkdown, spec.Frequency, spec.IntervalCount,
		nullableOptionalInt(spec.ByWeekday), nullableOptionalInt(spec.DayOfMonth),
		spec.LastDayOfMonth, nullableOptionalInt(spec.MonthOfYear),
		spec.StartsOn, nullableNonEmptyText(spec.EndsOn), nullableOptionalInt(spec.MaxOccurrences),
		spec.LeadDays, spec.GenerateFrom,
		params.CreatedAt, params.ActorUserID, params.CreatedAt, params.ActorUserID,
	)
	if err != nil {
		return RecurringTemplateRecord{}, fmt.Errorf("insert recurring template: %w", err)
	}
	templateID, err := result.LastInsertId()
	if err != nil {
		return RecurringTemplateRecord{}, fmt.Errorf("read recurring template id: %w", err)
	}

	if err := replaceRecurringTemplateChildren(ctx, tx, params.BookID, templateID, spec); err != nil {
		return RecurringTemplateRecord{}, err
	}
	if err := tx.Commit(); err != nil {
		return RecurringTemplateRecord{}, fmt.Errorf("commit create recurring template: %w", err)
	}
	committed = true
	return r.RecurringTemplateByID(ctx, params.BookID, templateID)
}

func (r *RecurringRepository) UpdateRecurringTemplate(ctx context.Context, params UpdateRecurringTemplateParams) (RecurringTemplateRecord, error) {
	tx, err := r.database.BeginTx(ctx, nil)
	if err != nil {
		return RecurringTemplateRecord{}, fmt.Errorf("begin update recurring template: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			rollbackTx(ctx, tx)
		}
	}()

	if _, err := readBookForUpdate(ctx, tx, params.BookID); err != nil {
		return RecurringTemplateRecord{}, err
	}
	if err := requireRecurringTemplate(ctx, tx, params.BookID, params.TemplateID); err != nil {
		return RecurringTemplateRecord{}, err
	}
	var revision int64
	var archived sql.NullString
	if err := tx.QueryRowContext(ctx, `SELECT revision, archived_at FROM recurring_templates WHERE book_id = ? AND id = ?`, params.BookID, params.TemplateID).Scan(&revision, &archived); err != nil {
		return RecurringTemplateRecord{}, fmt.Errorf("read recurring template revision: %w", err)
	}
	if archived.Valid {
		return RecurringTemplateRecord{}, ErrRecurringTemplateArchived
	}
	if params.ExpectedRevision != nil && revision != *params.ExpectedRevision {
		return RecurringTemplateRecord{}, ErrRecurringTemplateConflict
	}
	if _, err := insertAuditEvent(ctx, tx, AuditEventParams{
		BookID:        params.BookID,
		ActorUserID:   params.ActorUserID,
		AuthSessionID: params.AuthSessionID,
		OccurredAt:    params.UpdatedAt,
		RequestID:     params.RequestID,
		OriginType:    "browser_api",
		Operation:     "recurring.template.update",
		Reason:        "recurring template updated",
		MetadataJSON:  fmt.Sprintf(`{"template_id":%d}`, params.TemplateID),
	}); err != nil {
		return RecurringTemplateRecord{}, err
	}

	spec := params.Spec
	if _, err := tx.ExecContext(ctx, `
		UPDATE recurring_templates SET
			name = ?, enabled = ?, transaction_kind = ?, payee_id = ?, payee_name = ?,
			description = ?, note_markdown = ?, frequency = ?, interval_count = ?,
			by_weekday = ?, day_of_month = ?, last_day_of_month = ?, month_of_year = ?,
			starts_on = ?, ends_on = ?, max_occurrences = ?, lead_days = ?,
			generate_from = ?, updated_at = ?, updated_by_user_id = ?, revision = revision + 1
		WHERE book_id = ? AND id = ?
	`,
		spec.Name, spec.Enabled, spec.TransactionKind,
		nullableOptionalInt64(spec.PayeeID), nullableNonEmptyText(spec.PayeeName),
		spec.Description, spec.NoteMarkdown, spec.Frequency, spec.IntervalCount,
		nullableOptionalInt(spec.ByWeekday), nullableOptionalInt(spec.DayOfMonth),
		spec.LastDayOfMonth, nullableOptionalInt(spec.MonthOfYear),
		spec.StartsOn, nullableNonEmptyText(spec.EndsOn), nullableOptionalInt(spec.MaxOccurrences),
		spec.LeadDays, spec.GenerateFrom, params.UpdatedAt, params.ActorUserID,
		params.BookID, params.TemplateID,
	); err != nil {
		return RecurringTemplateRecord{}, fmt.Errorf("update recurring template: %w", err)
	}

	if err := replaceRecurringTemplateChildren(ctx, tx, params.BookID, params.TemplateID, spec); err != nil {
		return RecurringTemplateRecord{}, err
	}
	if err := tx.Commit(); err != nil {
		return RecurringTemplateRecord{}, fmt.Errorf("commit update recurring template: %w", err)
	}
	committed = true
	return r.RecurringTemplateByID(ctx, params.BookID, params.TemplateID)
}

// ArchiveRecurringTemplate retires a template without touching what it already
// produced. Generated drafts are the user's transactions from the moment they
// exist, and the conventions refuse hard deletes of business records; a
// template with occurrences therefore has no delete path at all.
func (r *RecurringRepository) ArchiveRecurringTemplate(ctx context.Context, params ArchiveRecurringTemplateParams) (RecurringTemplateRecord, error) {
	tx, err := r.database.BeginTx(ctx, nil)
	if err != nil {
		return RecurringTemplateRecord{}, fmt.Errorf("begin archive recurring template: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			rollbackTx(ctx, tx)
		}
	}()

	if _, err := readBookForUpdate(ctx, tx, params.BookID); err != nil {
		return RecurringTemplateRecord{}, err
	}
	if err := requireRecurringTemplate(ctx, tx, params.BookID, params.TemplateID); err != nil {
		return RecurringTemplateRecord{}, err
	}
	var archived sql.NullString
	if err := tx.QueryRowContext(ctx, `SELECT archived_at FROM recurring_templates WHERE book_id = ? AND id = ?`, params.BookID, params.TemplateID).Scan(&archived); err != nil {
		return RecurringTemplateRecord{}, fmt.Errorf("read recurring archive state: %w", err)
	}
	if archived.Valid {
		records, err := listRecurringTemplates(ctx, tx, ListRecurringTemplatesParams{BookID: params.BookID, IncludeArchived: true}, params.TemplateID)
		if err != nil {
			return RecurringTemplateRecord{}, err
		}
		return records[0], nil
	}
	if _, err := insertAuditEvent(ctx, tx, AuditEventParams{
		BookID:        params.BookID,
		ActorUserID:   params.ActorUserID,
		AuthSessionID: params.AuthSessionID,
		OccurredAt:    params.ArchivedAt,
		RequestID:     params.RequestID,
		OriginType:    "browser_api",
		Operation:     "recurring.template.archive",
		Reason:        "recurring template archived",
		MetadataJSON:  fmt.Sprintf(`{"template_id":%d}`, params.TemplateID),
	}); err != nil {
		return RecurringTemplateRecord{}, err
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE recurring_templates
		SET archived_at = ?, enabled = 0, updated_at = ?, updated_by_user_id = ?, revision = revision + 1
		WHERE book_id = ? AND id = ?
	`, params.ArchivedAt, params.ArchivedAt, params.ActorUserID, params.BookID, params.TemplateID); err != nil {
		return RecurringTemplateRecord{}, fmt.Errorf("archive recurring template: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return RecurringTemplateRecord{}, fmt.Errorf("commit archive recurring template: %w", err)
	}
	committed = true
	return r.RecurringTemplateByID(ctx, params.BookID, params.TemplateID)
}

// SetRecurringTemplateGenerateFrom advances the generation watermark. It is
// the generator's only write to the template row, kept separate from the
// user-facing update so a tick never rewrites a field the user owns.
func (r *RecurringRepository) SetRecurringTemplateGenerateFrom(ctx context.Context, bookID int64, templateID int64, generateFrom string, updatedAt string) error {
	result, err := r.database.ExecContext(ctx, `
		UPDATE recurring_templates
		SET generate_from = ?, updated_at = ?, revision = revision + 1
		WHERE book_id = ? AND id = ? AND generate_from < ?
	`, generateFrom, updatedAt, bookID, templateID, generateFrom)
	if err != nil {
		return fmt.Errorf("advance recurring template watermark: %w", err)
	}
	// No rows means the watermark is already at or past this date, which is
	// what a re-run of the same tick looks like. Not an error.
	if _, err := result.RowsAffected(); err != nil {
		return fmt.Errorf("read recurring template watermark result: %w", err)
	}
	return nil
}

func (r *RecurringRepository) RecurringTemplateByID(ctx context.Context, bookID int64, templateID int64) (RecurringTemplateRecord, error) {
	records, err := listRecurringTemplates(ctx, r.database, ListRecurringTemplatesParams{BookID: bookID, IncludeArchived: true}, templateID)
	if err != nil {
		return RecurringTemplateRecord{}, err
	}
	if len(records) == 0 {
		return RecurringTemplateRecord{}, ErrRecurringTemplateNotFound
	}
	return records[0], nil
}

func (r *RecurringRepository) ListRecurringTemplates(ctx context.Context, params ListRecurringTemplatesParams) ([]RecurringTemplateRecord, error) {
	return listRecurringTemplates(ctx, r.database, params, 0)
}

func listRecurringTemplates(ctx context.Context, queryer queryer, params ListRecurringTemplatesParams, templateID int64) ([]RecurringTemplateRecord, error) {
	where := "template.book_id = ?"
	arguments := []any{params.BookID}
	if templateID > 0 {
		where += " AND template.id = ?"
		arguments = append(arguments, templateID)
	}
	if !params.IncludeArchived {
		where += " AND template.archived_at IS NULL"
	}
	if params.EnabledOnly {
		where += " AND template.enabled = 1 AND template.archived_at IS NULL"
	}

	rows, err := queryer.QueryContext(ctx, `
		SELECT template.id, template.revision, template.book_id, template.name, template.enabled,
		       template.archived_at, template.transaction_kind, template.payee_id,
		       COALESCE(payee_version.name, template.payee_name),
		       template.description, template.note_markdown,
		       template.frequency, template.interval_count, template.by_weekday,
		       template.day_of_month, template.last_day_of_month, template.month_of_year,
		       template.starts_on, template.ends_on, template.max_occurrences,
		       template.lead_days, template.generate_from,
		       template.created_at, template.updated_at
		FROM recurring_templates template
		LEFT JOIN payees payee ON payee.id = template.payee_id AND payee.book_id = template.book_id
		LEFT JOIN current_payee_versions payee_version ON payee_version.payee_id = payee.id
		WHERE `+where+`
		ORDER BY template.name ASC, template.id ASC
	`, arguments...)
	if err != nil {
		return nil, fmt.Errorf("list recurring templates: %w", err)
	}
	defer rows.Close()

	records := make([]RecurringTemplateRecord, 0)
	for rows.Next() {
		var record RecurringTemplateRecord
		if err := rows.Scan(
			&record.ID, &record.Revision, &record.BookID, &record.Name, &record.Enabled,
			&record.ArchivedAt, &record.TransactionKind, &record.PayeeID,
			&record.PayeeName, &record.Description, &record.NoteMarkdown,
			&record.Frequency, &record.IntervalCount, &record.ByWeekday,
			&record.DayOfMonth, &record.LastDayOfMonth, &record.MonthOfYear,
			&record.StartsOn, &record.EndsOn, &record.MaxOccurrences,
			&record.LeadDays, &record.GenerateFrom,
			&record.CreatedAt, &record.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan recurring template: %w", err)
		}
		record.Postings = make([]RecurringTemplatePostingRecord, 0)
		record.TagIDs = make([]int64, 0)
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate recurring templates: %w", err)
	}
	if len(records) == 0 {
		return records, nil
	}

	byID := make(map[int64]*RecurringTemplateRecord, len(records))
	for index := range records {
		byID[records[index].ID] = &records[index]
	}

	postingRows, err := queryer.QueryContext(ctx, `
		SELECT id, template_id, line_seq, line_key, account_id,
		       quantity_value, quantity_scale, commodity_id, memo
		FROM recurring_template_postings
		WHERE book_id = ?
		ORDER BY template_id, line_seq
	`, params.BookID)
	if err != nil {
		return nil, fmt.Errorf("list recurring template postings: %w", err)
	}
	defer postingRows.Close()
	for postingRows.Next() {
		var posting RecurringTemplatePostingRecord
		if err := postingRows.Scan(
			&posting.ID, &posting.TemplateID, &posting.LineSeq, &posting.LineKey,
			&posting.AccountID, &posting.QuantityValue, &posting.QuantityScale,
			&posting.CommodityID, &posting.Memo,
		); err != nil {
			return nil, fmt.Errorf("scan recurring template posting: %w", err)
		}
		if record := byID[posting.TemplateID]; record != nil {
			record.Postings = append(record.Postings, posting)
		}
	}
	if err := postingRows.Err(); err != nil {
		return nil, fmt.Errorf("iterate recurring template postings: %w", err)
	}

	tagRows, err := queryer.QueryContext(ctx, `
		SELECT template_id, tag_id FROM recurring_template_tags
		WHERE book_id = ?
		ORDER BY template_id, tag_id
	`, params.BookID)
	if err != nil {
		return nil, fmt.Errorf("list recurring template tags: %w", err)
	}
	defer tagRows.Close()
	for tagRows.Next() {
		var templateRowID, tagID int64
		if err := tagRows.Scan(&templateRowID, &tagID); err != nil {
			return nil, fmt.Errorf("scan recurring template tag: %w", err)
		}
		if record := byID[templateRowID]; record != nil {
			record.TagIDs = append(record.TagIDs, tagID)
		}
	}
	if err := tagRows.Err(); err != nil {
		return nil, fmt.Errorf("iterate recurring template tags: %w", err)
	}
	return records, nil
}

func replaceRecurringTemplateChildren(ctx context.Context, tx *sql.Tx, bookID int64, templateID int64, spec RecurringTemplateSpec) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM recurring_template_postings WHERE book_id = ? AND template_id = ?`, bookID, templateID); err != nil {
		return fmt.Errorf("delete recurring template postings: %w", err)
	}
	for index, posting := range spec.Postings {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO recurring_template_postings (
				template_id, book_id, line_seq, line_key, account_id,
				quantity_value, quantity_scale, commodity_id, memo
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, templateID, bookID, index+1, posting.LineKey, posting.AccountID,
			posting.QuantityValue, posting.QuantityScale, posting.CommodityID, posting.Memo); err != nil {
			return fmt.Errorf("insert recurring template posting: %w", err)
		}
	}

	if _, err := tx.ExecContext(ctx, `DELETE FROM recurring_template_tags WHERE book_id = ? AND template_id = ?`, bookID, templateID); err != nil {
		return fmt.Errorf("delete recurring template tags: %w", err)
	}
	for _, tagID := range spec.TagIDs {
		if _, err := tx.ExecContext(ctx, `INSERT INTO recurring_template_tags (template_id, book_id, tag_id) VALUES (?, ?, ?)`, templateID, bookID, tagID); err != nil {
			return fmt.Errorf("insert recurring template tag: %w", err)
		}
	}
	return nil
}

func requireRecurringTemplate(ctx context.Context, tx *sql.Tx, bookID int64, templateID int64) error {
	var exists int
	err := tx.QueryRowContext(ctx, `SELECT 1 FROM recurring_templates WHERE book_id = ? AND id = ?`, bookID, templateID).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrRecurringTemplateNotFound
	}
	if err != nil {
		return fmt.Errorf("read recurring template: %w", err)
	}
	return nil
}

// RecurringOccurrenceRecord is one date the generator has acted on. There is
// no 'pending' status: a row exists because something happened to that date.
type RecurringOccurrenceRecord struct {
	ID             int64
	BookID         int64
	TemplateID     int64
	OccurrenceDate string
	Status         string
	TransactionID  sql.NullInt64
	ErrorSummary   string
	SkipReason     string
	MaterializedAt string
	CreatedAt      string
	UpdatedAt      string
}

type CreateRecurringOccurrenceParams struct {
	BookID         int64
	TemplateID     int64
	OccurrenceDate string
	Status         string
	TransactionID  *int64
	ErrorSummary   string
	SkipReason     string
	MaterializedAt string
}

// CreateRecurringOccurrence records one occurrence. A conflict on
// (template_id, occurrence_date) returns ErrRecurringOccurrenceExists so the
// caller can adopt the existing row; the generator's combined
// draft-plus-occurrence write lands in slice 3, where both halves share one
// transaction and a crash between them is impossible.
func (r *RecurringRepository) CreateRecurringOccurrence(ctx context.Context, params CreateRecurringOccurrenceParams) (RecurringOccurrenceRecord, error) {
	result, err := r.database.ExecContext(ctx, `
		INSERT INTO recurring_occurrences (
			book_id, template_id, occurrence_date, status, transaction_id,
			error_summary, skip_reason, materialized_at, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, params.BookID, params.TemplateID, params.OccurrenceDate, params.Status,
		nullableOptionalInt64(params.TransactionID), params.ErrorSummary, params.SkipReason,
		params.MaterializedAt, params.MaterializedAt, params.MaterializedAt)
	if isRecurringOccurrenceConflict(err) {
		return RecurringOccurrenceRecord{}, ErrRecurringOccurrenceExists
	}
	if err != nil {
		return RecurringOccurrenceRecord{}, fmt.Errorf("insert recurring occurrence: %w", err)
	}
	occurrenceID, err := result.LastInsertId()
	if err != nil {
		return RecurringOccurrenceRecord{}, fmt.Errorf("read recurring occurrence id: %w", err)
	}
	return r.RecurringOccurrenceByID(ctx, params.BookID, occurrenceID)
}

func (r *RecurringRepository) RecurringOccurrenceByID(ctx context.Context, bookID int64, occurrenceID int64) (RecurringOccurrenceRecord, error) {
	var record RecurringOccurrenceRecord
	err := r.database.QueryRowContext(ctx, `
		SELECT id, book_id, template_id, occurrence_date, status, transaction_id,
		       error_summary, skip_reason, materialized_at, created_at, updated_at
		FROM recurring_occurrences
		WHERE book_id = ? AND id = ?
	`, bookID, occurrenceID).Scan(
		&record.ID, &record.BookID, &record.TemplateID, &record.OccurrenceDate,
		&record.Status, &record.TransactionID, &record.ErrorSummary,
		&record.SkipReason, &record.MaterializedAt, &record.CreatedAt, &record.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return RecurringOccurrenceRecord{}, ErrNotFound
	}
	if err != nil {
		return RecurringOccurrenceRecord{}, fmt.Errorf("read recurring occurrence: %w", err)
	}
	return record, nil
}

// RecurringOccurrenceDates returns the dates in [from, to] that already have a
// row for this template, whatever became of them. It is what the generator
// subtracts from the enumerator's answer, so a skipped or blocked date is
// never retried by accident.
func (r *RecurringRepository) RecurringOccurrenceDates(ctx context.Context, bookID int64, templateID int64, from string, to string) (map[string]string, error) {
	rows, err := r.database.QueryContext(ctx, `
		SELECT occurrence_date, status
		FROM recurring_occurrences
		WHERE book_id = ? AND template_id = ? AND occurrence_date BETWEEN ? AND ?
		ORDER BY occurrence_date
	`, bookID, templateID, from, to)
	if err != nil {
		return nil, fmt.Errorf("list recurring occurrence dates: %w", err)
	}
	defer rows.Close()

	dates := map[string]string{}
	for rows.Next() {
		var date, status string
		if err := rows.Scan(&date, &status); err != nil {
			return nil, fmt.Errorf("scan recurring occurrence date: %w", err)
		}
		dates[date] = status
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate recurring occurrence dates: %w", err)
	}
	return dates, nil
}

// ListRecurringOccurrences returns a template's materialized occurrences,
// newest first, for the template detail view.
func (r *RecurringRepository) ListRecurringOccurrences(ctx context.Context, bookID int64, templateID int64, limit int) ([]RecurringOccurrenceRecord, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := r.database.QueryContext(ctx, `
		SELECT id, book_id, template_id, occurrence_date, status, transaction_id,
		       error_summary, skip_reason, materialized_at, created_at, updated_at
		FROM recurring_occurrences
		WHERE book_id = ? AND template_id = ?
		ORDER BY occurrence_date DESC, id DESC
		LIMIT ?
	`, bookID, templateID, limit)
	if err != nil {
		return nil, fmt.Errorf("list recurring occurrences: %w", err)
	}
	defer rows.Close()
	return scanRecurringOccurrences(rows)
}

func scanRecurringOccurrences(rows *sql.Rows) ([]RecurringOccurrenceRecord, error) {
	records := make([]RecurringOccurrenceRecord, 0)
	for rows.Next() {
		var record RecurringOccurrenceRecord
		if err := rows.Scan(
			&record.ID, &record.BookID, &record.TemplateID, &record.OccurrenceDate,
			&record.Status, &record.TransactionID, &record.ErrorSummary,
			&record.SkipReason, &record.MaterializedAt, &record.CreatedAt, &record.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan recurring occurrence: %w", err)
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate recurring occurrences: %w", err)
	}
	return records, nil
}

// Optional-field binders. The repository's own, because the shared helpers in
// accounts.go take sql.Null* values while a write spec carries pointers: an
// omitted optional field must reach SQLite as NULL, not as a zero the CHECK
// constraints would then have to distinguish.
func nullableOptionalInt64(value *int64) any {
	if value == nil {
		return nil
	}
	return *value
}

func nullableOptionalInt(value *int) any {
	if value == nil {
		return nil
	}
	return *value
}

func nullableNonEmptyText(value string) any {
	if value == "" {
		return nil
	}
	return value
}
