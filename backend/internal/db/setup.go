package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

var ErrOwnerExists = errors.New("owner already exists")
var ErrNotFound = errors.New("record not found")

type SetupRepository struct {
	database *sql.DB
}

type SetupOverview struct {
	OwnerExists        bool
	OwnerStepCompleted bool
}

type SetupStepRecord struct {
	Key         string
	CompletedAt sql.NullString
}

type OwnerRecord struct {
	ID       int64
	Username string
}

type CompleteOwnerSetupParams struct {
	Username         string
	PasswordHash     string
	SessionTokenHash string
	CreatedAt        string
	SessionExpiresAt string
}

func NewSetupRepository(database *sql.DB) *SetupRepository {
	return &SetupRepository{database: database}
}

func (r *SetupRepository) ReadSetupOverview(ctx context.Context) (SetupOverview, error) {
	var ownerCount, ownerStepCount int
	err := r.database.QueryRowContext(ctx, `
		SELECT
			(SELECT COUNT(1) FROM users WHERE is_owner = 1),
			(SELECT COUNT(1) FROM setup_steps WHERE step_key = 'owner' AND completed_at IS NOT NULL)
	`).Scan(&ownerCount, &ownerStepCount)
	if err != nil {
		return SetupOverview{}, fmt.Errorf("read setup overview: %w", err)
	}

	return SetupOverview{
		OwnerExists:        ownerCount > 0,
		OwnerStepCompleted: ownerStepCount > 0,
	}, nil
}

func (r *SetupRepository) ListSetupSteps(ctx context.Context) ([]SetupStepRecord, error) {
	rows, err := r.database.QueryContext(ctx, `
		SELECT step_key, completed_at
		FROM setup_steps
		ORDER BY CASE step_key
			WHEN 'owner' THEN 1
			WHEN 'book' THEN 2
			WHEN 'currencies' THEN 3
			WHEN 'system_accounts' THEN 4
			WHEN 'categories' THEN 5
			ELSE 99
		END,
		step_key
	`)
	if err != nil {
		return nil, fmt.Errorf("list setup steps: %w", err)
	}
	defer rows.Close()

	var steps []SetupStepRecord
	for rows.Next() {
		var step SetupStepRecord
		if err := rows.Scan(&step.Key, &step.CompletedAt); err != nil {
			return nil, fmt.Errorf("scan setup step: %w", err)
		}

		steps = append(steps, step)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate setup steps: %w", err)
	}

	return steps, nil
}

func (r *SetupRepository) CompleteOwnerSetup(ctx context.Context, params CompleteOwnerSetupParams) (OwnerRecord, error) {
	tx, err := r.database.BeginTx(ctx, nil)
	if err != nil {
		return OwnerRecord{}, fmt.Errorf("begin owner setup transaction: %w", err)
	}

	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	var ownerCount int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(1) FROM users WHERE is_owner = 1`).Scan(&ownerCount); err != nil {
		return OwnerRecord{}, fmt.Errorf("count owners: %w", err)
	}
	if ownerCount > 0 {
		return OwnerRecord{}, ErrOwnerExists
	}

	result, err := tx.ExecContext(ctx, `
		INSERT INTO users (username, password_hash, is_owner, created_at, updated_at)
		VALUES (?, ?, 1, ?, ?)
	`, params.Username, params.PasswordHash, params.CreatedAt, params.CreatedAt)
	if err != nil {
		return OwnerRecord{}, fmt.Errorf("insert owner user: %w", err)
	}

	ownerID, err := result.LastInsertId()
	if err != nil {
		return OwnerRecord{}, fmt.Errorf("read owner id: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO auth_sessions (user_id, token_hash, created_at, expires_at, revoked_at)
		VALUES (?, ?, ?, ?, NULL)
	`, ownerID, params.SessionTokenHash, params.CreatedAt, params.SessionExpiresAt); err != nil {
		return OwnerRecord{}, fmt.Errorf("insert owner session: %w", err)
	}

	result, err = tx.ExecContext(ctx, `
		UPDATE setup_steps
		SET completed_at = ?
		WHERE step_key = 'owner'
	`, params.CreatedAt)
	if err != nil {
		return OwnerRecord{}, fmt.Errorf("mark owner setup step complete: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return OwnerRecord{}, fmt.Errorf("read owner setup step rows affected: %w", err)
	}
	if rowsAffected != 1 {
		return OwnerRecord{}, fmt.Errorf("mark owner setup step complete: updated %d rows", rowsAffected)
	}

	if err := tx.Commit(); err != nil {
		return OwnerRecord{}, fmt.Errorf("commit owner setup transaction: %w", err)
	}
	committed = true

	return OwnerRecord{
		ID:       ownerID,
		Username: params.Username,
	}, nil
}
