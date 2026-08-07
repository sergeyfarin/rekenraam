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

	// Clear any MFA enrollment in the same transaction (S-06). This command is
	// the documented last resort for an owner locked out of their own install,
	// and it already requires filesystem access to the database — an
	// authenticator that is gone along with the recovery codes must not be able
	// to outlast it. Leaving the enrollment would hand back an account the new
	// password cannot open.
	for _, statement := range []string{
		`DELETE FROM user_mfa_recovery_codes`,
		`DELETE FROM login_mfa_challenges`,
		`DELETE FROM user_mfa_totp`,
	} {
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("clear owner mfa enrollment: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit recovery transaction: %w", err)
	}
	committed = true

	return nil
}
