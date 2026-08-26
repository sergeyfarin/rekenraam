package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// ErrBackupOccurrenceExists means a run already exists for this occurrence —
// the same scheduled day, asked for twice. It is a normal outcome, not a
// failure: the second caller adopts the first one's run.
var ErrBackupOccurrenceExists = errors.New("a backup run already exists for this occurrence")

type BackupPolicyRecord struct {
	BookID              int64
	Enabled             bool
	HourLocal           int
	MinuteLocal         int
	RetentionCount      int
	RetentionMaxAgeDays sql.NullInt64
	UpdatedAt           sql.NullString
}

type BackupRunRecord struct {
	ID                    int64
	BookID                int64
	Trigger               string
	OccurrenceKey         string
	Status                string
	TargetPath            string
	ScheduledForLocalDate sql.NullString
	ByteSize              sql.NullInt64
	PageCount             sql.NullInt64
	Verified              bool
	Attempts              int
	ErrorSummary          string
	WorkItemID            sql.NullInt64
	StartedAt             sql.NullString
	FinishedAt            sql.NullString
	PrunedAt              sql.NullString
	CreatedAt             string
	UpdatedAt             string
	// WorkStatus is the queue's view of this run: 'pending' while a retry is
	// waiting, 'failed' once the attempt cap is spent. The run row alone cannot
	// tell those apart — it says "failed" after every failed attempt — and a
	// screen that shows one word for both is telling a reader two things
	// (T-69).
	WorkStatus sql.NullString
}

type BackupRepository struct {
	database *sql.DB
}

func NewBackupRepository(database *sql.DB) *BackupRepository {
	return &BackupRepository{database: database}
}

// BackupPolicy returns the stored policy, or ErrNotFound when the book has
// never saved one. The service supplies the defaults rather than the schema, so
// "never configured" and "configured to the defaults" stay distinguishable.
func (r *BackupRepository) BackupPolicy(ctx context.Context, bookID int64) (BackupPolicyRecord, error) {
	var policy BackupPolicyRecord
	err := r.database.QueryRowContext(ctx, `
		SELECT book_id, enabled, hour_local, minute_local, retention_count, retention_max_age_days, updated_at
		FROM backup_policies
		WHERE book_id = ?
	`, bookID).Scan(
		&policy.BookID, &policy.Enabled, &policy.HourLocal, &policy.MinuteLocal,
		&policy.RetentionCount, &policy.RetentionMaxAgeDays, &policy.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return BackupPolicyRecord{}, ErrNotFound
	}
	if err != nil {
		return BackupPolicyRecord{}, fmt.Errorf("read backup policy: %w", err)
	}
	return policy, nil
}

type SaveBackupPolicyParams struct {
	BookID              int64
	Enabled             bool
	HourLocal           int
	MinuteLocal         int
	RetentionCount      int
	RetentionMaxAgeDays *int64
	UpdatedByUserID     int64
	Now                 string
}

func (r *BackupRepository) SaveBackupPolicy(ctx context.Context, params SaveBackupPolicyParams) (BackupPolicyRecord, error) {
	var maxAge any
	if params.RetentionMaxAgeDays != nil {
		maxAge = *params.RetentionMaxAgeDays
	}

	if _, err := r.database.ExecContext(ctx, `
		INSERT INTO backup_policies (
			book_id, enabled, hour_local, minute_local, retention_count,
			retention_max_age_days, created_at, updated_at, updated_by_user_id
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(book_id) DO UPDATE SET
			enabled = excluded.enabled,
			hour_local = excluded.hour_local,
			minute_local = excluded.minute_local,
			retention_count = excluded.retention_count,
			retention_max_age_days = excluded.retention_max_age_days,
			updated_at = excluded.updated_at,
			updated_by_user_id = excluded.updated_by_user_id
	`, params.BookID, params.Enabled, params.HourLocal, params.MinuteLocal,
		params.RetentionCount, maxAge, params.Now, params.Now, params.UpdatedByUserID); err != nil {
		return BackupPolicyRecord{}, fmt.Errorf("save backup policy: %w", err)
	}

	return r.BackupPolicy(ctx, params.BookID)
}

type CreateBackupRunParams struct {
	BookID                int64
	Trigger               string
	OccurrenceKey         string
	TargetPath            string
	ScheduledForLocalDate string
	RequestedByUserID     int64
	WorkKind              string
	Now                   string
}

// CreateBackupRunWithWork creates the run and the work item that will execute
// it in ONE transaction.
//
// Two failures this prevents, both seen before in this codebase: a run row with
// no work item is a backup that never happens and never says so, and a work
// item with no run row is a backup nobody can find afterwards. The unique
// occurrence key is what makes a *completed* run idempotent, which the work
// queue cannot do on its own — its uniqueness covers pending and running items
// only.
func (r *BackupRepository) CreateBackupRunWithWork(ctx context.Context, params CreateBackupRunParams) (BackupRunRecord, BackgroundWorkItemRecord, error) {
	tx, err := r.database.BeginTx(ctx, nil)
	if err != nil {
		return BackupRunRecord{}, BackgroundWorkItemRecord{}, fmt.Errorf("begin backup run: %w", err)
	}
	defer rollbackTx(ctx, tx)

	var existing int64
	err = tx.QueryRowContext(ctx, `
		SELECT id FROM backup_runs WHERE book_id = ? AND occurrence_key = ?
	`, params.BookID, params.OccurrenceKey).Scan(&existing)
	if err == nil {
		return BackupRunRecord{}, BackgroundWorkItemRecord{}, ErrBackupOccurrenceExists
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return BackupRunRecord{}, BackgroundWorkItemRecord{}, fmt.Errorf("check backup occurrence: %w", err)
	}

	var scheduledFor any
	if params.ScheduledForLocalDate != "" {
		scheduledFor = params.ScheduledForLocalDate
	}
	var requestedBy any
	if params.RequestedByUserID > 0 {
		requestedBy = params.RequestedByUserID
	}

	result, err := tx.ExecContext(ctx, `
		INSERT INTO backup_runs (
			book_id, trigger, occurrence_key, status, target_path,
			scheduled_for_local_date, requested_by_user_id, created_at, updated_at
		) VALUES (?, ?, ?, 'pending', ?, ?, ?, ?, ?)
	`, params.BookID, params.Trigger, params.OccurrenceKey, params.TargetPath,
		scheduledFor, requestedBy, params.Now, params.Now)
	if err != nil {
		return BackupRunRecord{}, BackgroundWorkItemRecord{}, fmt.Errorf("insert backup run: %w", err)
	}
	runID, err := result.LastInsertId()
	if err != nil {
		return BackupRunRecord{}, BackgroundWorkItemRecord{}, fmt.Errorf("read backup run id: %w", err)
	}

	payload := fmt.Sprintf(`{"run_id":%d}`, runID)
	item, err := EnqueueBackgroundWorkTx(ctx, tx, params.BookID, params.WorkKind, payload, params.Now)
	if err != nil {
		return BackupRunRecord{}, BackgroundWorkItemRecord{}, err
	}

	if _, err := tx.ExecContext(ctx, `
		UPDATE backup_runs SET work_item_id = ?, updated_at = ? WHERE id = ?
	`, item.ID, params.Now, runID); err != nil {
		return BackupRunRecord{}, BackgroundWorkItemRecord{}, fmt.Errorf("link backup run to work: %w", err)
	}

	run, err := backupRunByIDTx(ctx, tx, runID)
	if err != nil {
		return BackupRunRecord{}, BackgroundWorkItemRecord{}, err
	}
	if err := tx.Commit(); err != nil {
		return BackupRunRecord{}, BackgroundWorkItemRecord{}, fmt.Errorf("commit backup run: %w", err)
	}

	return run, item, nil
}

const backupRunColumns = `
	backup_runs.id, backup_runs.book_id, backup_runs.trigger, backup_runs.occurrence_key,
	backup_runs.status, backup_runs.target_path, backup_runs.scheduled_for_local_date,
	backup_runs.byte_size, backup_runs.page_count, backup_runs.verified, backup_runs.attempts,
	backup_runs.error_summary, backup_runs.work_item_id, backup_runs.started_at,
	backup_runs.finished_at, backup_runs.pruned_at, backup_runs.created_at,
	backup_runs.updated_at,
	(SELECT status FROM background_work_items WHERE id = backup_runs.work_item_id)`

func scanBackupRun(scan func(dest ...any) error) (BackupRunRecord, error) {
	var run BackupRunRecord
	err := scan(
		&run.ID, &run.BookID, &run.Trigger, &run.OccurrenceKey, &run.Status, &run.TargetPath,
		&run.ScheduledForLocalDate, &run.ByteSize, &run.PageCount, &run.Verified, &run.Attempts,
		&run.ErrorSummary, &run.WorkItemID, &run.StartedAt, &run.FinishedAt, &run.PrunedAt,
		&run.CreatedAt, &run.UpdatedAt, &run.WorkStatus,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return BackupRunRecord{}, ErrNotFound
	}
	if err != nil {
		return BackupRunRecord{}, fmt.Errorf("scan backup run: %w", err)
	}
	return run, nil
}

func backupRunByIDTx(ctx context.Context, tx *sql.Tx, id int64) (BackupRunRecord, error) {
	return scanBackupRun(tx.QueryRowContext(ctx, `SELECT `+backupRunColumns+` FROM backup_runs WHERE id = ?`, id).Scan)
}

func (r *BackupRepository) BackupRunByID(ctx context.Context, id int64) (BackupRunRecord, error) {
	return scanBackupRun(r.database.QueryRowContext(ctx, `SELECT `+backupRunColumns+` FROM backup_runs WHERE id = ?`, id).Scan)
}

// ScheduledRunExistsForLocalDate answers the scheduler's only question: has
// today's backup already been arranged? Any status counts — a failed run is
// still an attempt that happened, and retrying it is the work queue's job, not
// the scheduler's.
func (r *BackupRepository) ScheduledRunExistsForLocalDate(ctx context.Context, bookID int64, localDate string) (bool, error) {
	var exists int
	err := r.database.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM backup_runs
			WHERE book_id = ? AND trigger = 'scheduled' AND scheduled_for_local_date = ?
		)
	`, bookID, localDate).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check scheduled backup run: %w", err)
	}
	return exists == 1, nil
}

type UpdateBackupRunParams struct {
	ID           int64
	Status       string
	Verified     bool
	ByteSize     *int64
	PageCount    *int64
	ErrorSummary string
	StartedAt    string
	FinishedAt   string
	Now          string
	BumpAttempts bool
}

func (r *BackupRepository) UpdateBackupRun(ctx context.Context, params UpdateBackupRunParams) error {
	var byteSize, pageCount any
	if params.ByteSize != nil {
		byteSize = *params.ByteSize
	}
	if params.PageCount != nil {
		pageCount = *params.PageCount
	}
	var startedAt, finishedAt any
	if params.StartedAt != "" {
		startedAt = params.StartedAt
	}
	if params.FinishedAt != "" {
		finishedAt = params.FinishedAt
	}

	_, err := r.database.ExecContext(ctx, `
		UPDATE backup_runs SET
			status = ?,
			verified = ?,
			byte_size = COALESCE(?, byte_size),
			page_count = COALESCE(?, page_count),
			error_summary = ?,
			started_at = COALESCE(?, started_at),
			finished_at = COALESCE(?, finished_at),
			attempts = attempts + ?,
			updated_at = ?
		WHERE id = ?
	`, params.Status, params.Verified, byteSize, pageCount, params.ErrorSummary,
		startedAt, finishedAt, boolToInt(params.BumpAttempts), params.Now, params.ID)
	if err != nil {
		return fmt.Errorf("update backup run: %w", err)
	}
	return nil
}

// ListBackupRuns returns recent runs newest first, for the read model the Data
// screen polls.
func (r *BackupRepository) ListBackupRuns(ctx context.Context, bookID int64, limit int) ([]BackupRunRecord, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := r.database.QueryContext(ctx, `
		SELECT `+backupRunColumns+`
		FROM backup_runs
		WHERE book_id = ?
		ORDER BY created_at DESC, id DESC
		LIMIT ?
	`, bookID, limit)
	if err != nil {
		return nil, fmt.Errorf("list backup runs: %w", err)
	}
	defer rows.Close()

	var runs []BackupRunRecord
	for rows.Next() {
		run, err := scanBackupRun(rows.Scan)
		if err != nil {
			return nil, err
		}
		runs = append(runs, run)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate backup runs: %w", err)
	}
	return runs, nil
}

// PrunableBackupRuns returns completed, unpruned runs oldest first — the only
// files pruning is allowed to consider. A file the app never recorded is not
// its to delete.
func (r *BackupRepository) PrunableBackupRuns(ctx context.Context, bookID int64) ([]BackupRunRecord, error) {
	rows, err := r.database.QueryContext(ctx, `
		SELECT `+backupRunColumns+`
		FROM backup_runs
		WHERE book_id = ? AND status = 'completed' AND pruned_at IS NULL
		ORDER BY created_at ASC, id ASC
	`, bookID)
	if err != nil {
		return nil, fmt.Errorf("list prunable backup runs: %w", err)
	}
	defer rows.Close()

	var runs []BackupRunRecord
	for rows.Next() {
		run, err := scanBackupRun(rows.Scan)
		if err != nil {
			return nil, err
		}
		runs = append(runs, run)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate prunable backup runs: %w", err)
	}
	return runs, nil
}

func (r *BackupRepository) MarkBackupRunPruned(ctx context.Context, id int64, now string) error {
	if _, err := r.database.ExecContext(ctx, `
		UPDATE backup_runs SET pruned_at = ?, updated_at = ? WHERE id = ?
	`, now, now, id); err != nil {
		return fmt.Errorf("mark backup run pruned: %w", err)
	}
	return nil
}

func (r *BackupRepository) BookOwnerTimeZone(ctx context.Context, bookID int64) (string, error) {
	var timeZone string
	if err := r.database.QueryRowContext(ctx, `
		SELECT COALESCE(up.time_zone, 'UTC')
		FROM books b
		LEFT JOIN user_preferences up ON up.user_id = b.owner_user_id
		WHERE b.id = ?
	`, bookID).Scan(&timeZone); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "UTC", nil
		}
		return "", fmt.Errorf("read book owner time zone: %w", err)
	}
	return timeZone, nil
}

func (r *BackupRepository) CurrentBookOwnerID(ctx context.Context, bookID int64) (int64, error) {
	var ownerID int64
	if err := r.database.QueryRowContext(ctx, `SELECT owner_user_id FROM books WHERE id = ?`, bookID).Scan(&ownerID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, ErrNotFound
		}
		return 0, fmt.Errorf("read book owner: %w", err)
	}
	return ownerID, nil
}
