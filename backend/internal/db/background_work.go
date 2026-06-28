package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

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
	if err := tx.Commit(); err != nil {
		return BackgroundWorkItemRecord{}, fmt.Errorf("commit background work enqueue: %w", err)
	}
	return r.BackgroundWorkByID(ctx, id)
}

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

func (r *BackgroundWorkRepository) BackgroundWorkByID(ctx context.Context, id int64) (BackgroundWorkItemRecord, error) {
	var item BackgroundWorkItemRecord
	err := r.database.QueryRowContext(ctx, `
		SELECT id, book_id, kind, payload_version, payload_json, status, attempts,
			available_at, lease_owner, lease_expires_at, last_error, created_at,
			updated_at, completed_at
		FROM background_work_items WHERE id = ?
	`, id).Scan(&item.ID, &item.BookID, &item.Kind, &item.PayloadVersion, &item.PayloadJSON,
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
