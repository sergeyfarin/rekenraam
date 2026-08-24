package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// ErrBackgroundWorkAlreadyActive is returned by RequeueBackgroundWork when an
// equivalent (same book/kind/payload) item is already pending or running. The
// partial unique index background_work_items_active_unique_idx would reject the
// transition anyway; this turns it into a meaningful error for the caller.
var ErrBackgroundWorkAlreadyActive = errors.New("equivalent background work is already active")

type BackgroundWorkItemRecord struct {
	ID             int64
	BookID         int64
	Kind           string
	PayloadVersion int
	PayloadJSON    string
	Status         string
	Attempts       int
	AvailableAt    string
	LeaseOwner     sql.NullString
	LeaseExpiresAt sql.NullString
	LastError      sql.NullString
	CreatedAt      string
	UpdatedAt      string
	CompletedAt    sql.NullString
}

type BackgroundWorkRepository struct {
	database *sql.DB
}

func NewBackgroundWorkRepository(database *sql.DB) *BackgroundWorkRepository {
	return &BackgroundWorkRepository{database: database}
}

func (r *BackgroundWorkRepository) EnqueueBackgroundWork(ctx context.Context, bookID int64, kind string, payloadJSON string, availableAt string) (BackgroundWorkItemRecord, error) {
	tx, err := r.database.BeginTx(ctx, nil)
	if err != nil {
		return BackgroundWorkItemRecord{}, fmt.Errorf("begin background work enqueue: %w", err)
	}
	defer tx.Rollback()

	item, err := EnqueueBackgroundWorkTx(ctx, tx, bookID, kind, payloadJSON, availableAt)
	if err != nil {
		return BackgroundWorkItemRecord{}, err
	}
	if err := tx.Commit(); err != nil {
		return BackgroundWorkItemRecord{}, fmt.Errorf("commit background work enqueue: %w", err)
	}
	return item, nil
}

// EnqueueBackgroundWorkTx enqueues inside a caller's transaction, so a domain
// row and the work item that acts on it are created together or not at all —
// the transactional-outbox rule from ADR 0010, and the fix for the
// guard-then-create race that stranded imports (T-14).
func EnqueueBackgroundWorkTx(ctx context.Context, tx *sql.Tx, bookID int64, kind string, payloadJSON string, availableAt string) (BackgroundWorkItemRecord, error) {
	result, err := tx.ExecContext(ctx, `
		INSERT INTO background_work_items (
			book_id, kind, payload_json, available_at, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(book_id, kind, payload_json) WHERE status IN ('pending', 'running') DO NOTHING
	`, bookID, kind, payloadJSON, availableAt, availableAt, availableAt)
	if err != nil {
		return BackgroundWorkItemRecord{}, fmt.Errorf("enqueue background work: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return BackgroundWorkItemRecord{}, fmt.Errorf("read background work enqueue rows: %w", err)
	}
	var id int64
	if rows == 1 {
		id, err = result.LastInsertId()
		if err != nil {
			return BackgroundWorkItemRecord{}, fmt.Errorf("read background work id: %w", err)
		}
	} else {
		err = tx.QueryRowContext(ctx, `
			SELECT id
			FROM background_work_items
			WHERE book_id = ? AND kind = ? AND payload_json = ?
			  AND status IN ('pending', 'running')
			ORDER BY id
			LIMIT 1
		`, bookID, kind, payloadJSON).Scan(&id)
		if errors.Is(err, sql.ErrNoRows) {
			return BackgroundWorkItemRecord{}, ErrNotFound
		}
		if err != nil {
			return BackgroundWorkItemRecord{}, fmt.Errorf("read existing background work id: %w", err)
		}
	}

	return backgroundWorkByIDTx(ctx, tx, id)
}

func backgroundWorkByIDTx(ctx context.Context, tx *sql.Tx, id int64) (BackgroundWorkItemRecord, error) {
	var item BackgroundWorkItemRecord
	err := tx.QueryRowContext(ctx, backgroundWorkByIDQuery, id).Scan(
		&item.ID, &item.BookID, &item.Kind, &item.PayloadVersion, &item.PayloadJSON,
		&item.Status, &item.Attempts, &item.AvailableAt, &item.LeaseOwner,
		&item.LeaseExpiresAt, &item.LastError, &item.CreatedAt, &item.UpdatedAt,
		&item.CompletedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return BackgroundWorkItemRecord{}, ErrNotFound
	}
	if err != nil {
		return BackgroundWorkItemRecord{}, fmt.Errorf("read background work: %w", err)
	}
	return item, nil
}

const backgroundWorkByIDQuery = `
	SELECT id, book_id, kind, payload_version, payload_json, status, attempts,
		available_at, lease_owner, lease_expires_at, last_error, created_at,
		updated_at, completed_at
	FROM background_work_items WHERE id = ?`

func (r *BackgroundWorkRepository) ClaimBackgroundWork(ctx context.Context, kind string, leaseOwner string, now string, leaseExpiresAt string) (BackgroundWorkItemRecord, error) {
	tx, err := r.database.BeginTx(ctx, nil)
	if err != nil {
		return BackgroundWorkItemRecord{}, fmt.Errorf("begin background work claim: %w", err)
	}
	defer tx.Rollback()

	var id int64
	err = tx.QueryRowContext(ctx, `
		SELECT id
		FROM background_work_items
		WHERE kind = ?
		  AND available_at <= ?
		  AND (status = 'pending' OR (status = 'running' AND lease_expires_at <= ?))
		ORDER BY available_at, id
		LIMIT 1
	`, kind, now, now).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return BackgroundWorkItemRecord{}, ErrNotFound
	}
	if err != nil {
		return BackgroundWorkItemRecord{}, fmt.Errorf("find due background work: %w", err)
	}
	result, err := tx.ExecContext(ctx, `
		UPDATE background_work_items
		SET status = 'running', attempts = attempts + 1, lease_owner = ?,
			lease_expires_at = ?, updated_at = ?
		WHERE id = ?
		  AND (status = 'pending' OR (status = 'running' AND lease_expires_at <= ?))
	`, leaseOwner, leaseExpiresAt, now, id, now)
	if err != nil {
		return BackgroundWorkItemRecord{}, fmt.Errorf("claim background work: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return BackgroundWorkItemRecord{}, fmt.Errorf("claim background work rows affected: %w", err)
	}
	if rows != 1 {
		return BackgroundWorkItemRecord{}, ErrNotFound
	}
	if err := tx.Commit(); err != nil {
		return BackgroundWorkItemRecord{}, fmt.Errorf("commit background work claim: %w", err)
	}
	return r.BackgroundWorkByID(ctx, id)
}

func (r *BackgroundWorkRepository) CompleteBackgroundWork(ctx context.Context, id int64, leaseOwner string, now string) error {
	result, err := r.database.ExecContext(ctx, `
		UPDATE background_work_items
		SET status = 'completed', completed_at = ?, updated_at = ?,
			lease_owner = NULL, lease_expires_at = NULL, last_error = NULL
		WHERE id = ? AND status = 'running' AND lease_owner = ?
	`, now, now, id, leaseOwner)
	if err != nil {
		return fmt.Errorf("complete background work: %w", err)
	}
	return requireOneBackgroundWorkRow(result)
}

func (r *BackgroundWorkRepository) RetryBackgroundWork(ctx context.Context, id int64, leaseOwner string, availableAt string, now string, workError string) error {
	result, err := r.database.ExecContext(ctx, `
		UPDATE background_work_items
		SET status = 'pending', available_at = ?, updated_at = ?, last_error = ?,
			lease_owner = NULL, lease_expires_at = NULL
		WHERE id = ? AND status = 'running' AND lease_owner = ?
	`, availableAt, now, workError, id, leaseOwner)
	if err != nil {
		return fmt.Errorf("retry background work: %w", err)
	}
	return requireOneBackgroundWorkRow(result)
}

func (r *BackgroundWorkRepository) FailBackgroundWork(ctx context.Context, id int64, leaseOwner string, now string, workError string) error {
	result, err := r.database.ExecContext(ctx, `
		UPDATE background_work_items
		SET status = 'failed', updated_at = ?, last_error = ?,
			lease_owner = NULL, lease_expires_at = NULL
		WHERE id = ? AND status = 'running' AND lease_owner = ?
	`, now, workError, id, leaseOwner)
	if err != nil {
		return fmt.Errorf("fail background work: %w", err)
	}
	return requireOneBackgroundWorkRow(result)
}

// RequeueBackgroundWork moves a failed item back to pending with its attempt
// counter reset, so the exponential backoff starts over rather than resuming at
// the 6h cap. Only 'failed' items are eligible: pending/running work is already
// on its way, and completed work must be re-enqueued as a fresh item.
func (r *BackgroundWorkRepository) RequeueBackgroundWork(ctx context.Context, bookID int64, id int64, availableAt string, now string) (BackgroundWorkItemRecord, error) {
	tx, err := r.database.BeginTx(ctx, nil)
	if err != nil {
		return BackgroundWorkItemRecord{}, fmt.Errorf("begin background work requeue: %w", err)
	}
	defer tx.Rollback()

	var kind, payloadJSON string
	err = tx.QueryRowContext(ctx, `
		SELECT kind, payload_json
		FROM background_work_items
		WHERE id = ? AND book_id = ? AND status = 'failed'
	`, id, bookID).Scan(&kind, &payloadJSON)
	if errors.Is(err, sql.ErrNoRows) {
		return BackgroundWorkItemRecord{}, ErrNotFound
	}
	if err != nil {
		return BackgroundWorkItemRecord{}, fmt.Errorf("read failed background work: %w", err)
	}

	var active int
	err = tx.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM background_work_items
		WHERE book_id = ? AND kind = ? AND payload_json = ?
		  AND status IN ('pending', 'running')
	`, bookID, kind, payloadJSON).Scan(&active)
	if err != nil {
		return BackgroundWorkItemRecord{}, fmt.Errorf("check active background work: %w", err)
	}
	if active > 0 {
		return BackgroundWorkItemRecord{}, ErrBackgroundWorkAlreadyActive
	}

	result, err := tx.ExecContext(ctx, `
		UPDATE background_work_items
		SET status = 'pending', attempts = 0, available_at = ?, updated_at = ?,
			lease_owner = NULL, lease_expires_at = NULL, completed_at = NULL
		WHERE id = ? AND book_id = ? AND status = 'failed'
	`, availableAt, now, id, bookID)
	if err != nil {
		return BackgroundWorkItemRecord{}, fmt.Errorf("requeue background work: %w", err)
	}
	if err := requireOneBackgroundWorkRow(result); err != nil {
		return BackgroundWorkItemRecord{}, err
	}
	if err := tx.Commit(); err != nil {
		return BackgroundWorkItemRecord{}, fmt.Errorf("commit background work requeue: %w", err)
	}
	return r.BackgroundWorkByID(ctx, id)
}

// ListBackgroundWorkByStatus returns items of one kind in one status, newest
// update first. Used to surface permanently failed work to the operator.
func (r *BackgroundWorkRepository) ListBackgroundWorkByStatus(ctx context.Context, bookID int64, kind string, status string, limit int) ([]BackgroundWorkItemRecord, error) {
	rows, err := r.database.QueryContext(ctx, `
		SELECT id, book_id, kind, payload_version, payload_json, status, attempts,
			available_at, lease_owner, lease_expires_at, last_error, created_at,
			updated_at, completed_at
		FROM background_work_items
		WHERE book_id = ? AND kind = ? AND status = ?
		ORDER BY updated_at DESC, id DESC
		LIMIT ?
	`, bookID, kind, status, limit)
	if err != nil {
		return nil, fmt.Errorf("list background work by status: %w", err)
	}
	defer rows.Close()

	items := make([]BackgroundWorkItemRecord, 0)
	for rows.Next() {
		var item BackgroundWorkItemRecord
		if err := rows.Scan(&item.ID, &item.BookID, &item.Kind, &item.PayloadVersion, &item.PayloadJSON,
			&item.Status, &item.Attempts, &item.AvailableAt, &item.LeaseOwner,
			&item.LeaseExpiresAt, &item.LastError, &item.CreatedAt, &item.UpdatedAt,
			&item.CompletedAt); err != nil {
			return nil, fmt.Errorf("scan background work by status: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate background work by status: %w", err)
	}
	return items, nil
}

func (r *BackgroundWorkRepository) BackgroundWorkByID(ctx context.Context, id int64) (BackgroundWorkItemRecord, error) {
	var item BackgroundWorkItemRecord
	err := r.database.QueryRowContext(ctx, backgroundWorkByIDQuery, id).Scan(
		&item.ID, &item.BookID, &item.Kind, &item.PayloadVersion, &item.PayloadJSON,
		&item.Status, &item.Attempts, &item.AvailableAt, &item.LeaseOwner,
		&item.LeaseExpiresAt, &item.LastError, &item.CreatedAt, &item.UpdatedAt,
		&item.CompletedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return BackgroundWorkItemRecord{}, ErrNotFound
	}
	if err != nil {
		return BackgroundWorkItemRecord{}, fmt.Errorf("read background work: %w", err)
	}
	return item, nil
}

func requireOneBackgroundWorkRow(result sql.Result) error {
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read background work rows: %w", err)
	}
	if rows != 1 {
		return ErrNotFound
	}
	return nil
}
