package db

import (
	"context"
	"database/sql"
	"fmt"
)

type RecoveryRepository struct {
	database *sql.DB
}

type ResetOwnerAccessParams struct {
	PasswordHash string
	UpdatedAt    string
	RevokedAt    string
}

func NewRecoveryRepository(database *sql.DB) *RecoveryRepository {
	return &RecoveryRepository{database: database}
}

func (r *RecoveryRepository) ResetOwnerPasswordAndRevokeSessions(ctx context.Context, params ResetOwnerAccessParams) error {
	tx, err := r.database.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin recovery transaction: %w", err)
	}

	committed := false
	defer func() {
		if !committed {
			rollbackTx(ctx, tx)
		}
	}()

	result, err := tx.ExecContext(ctx, `
		UPDATE users
		SET password_hash = ?, updated_at = ?
		WHERE is_owner = 1
	`, params.PasswordHash, params.UpdatedAt)
	if err != nil {
		return fmt.Errorf("update owner password hash: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read owner password update rows affected: %w", err)
	}
	if rowsAffected != 1 {
		return ErrNotFound
	}

	if _, err := tx.ExecContext(ctx, `
		UPDATE auth_sessions
		SET revoked_at = ?
		WHERE revoked_at IS NULL
	`, params.RevokedAt); err != nil {
		return fmt.Errorf("revoke owner sessions: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit recovery transaction: %w", err)
	}
	committed = true

	return nil
}
