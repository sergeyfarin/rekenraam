package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
)

type RecoveryRepository struct {
	database *sql.DB
}

type ResetOwnerAccessParams struct {
	PasswordHash string
	UpdatedAt    string
	RevokedAt    string
	// BackupWritten records whether a verified backup preceded the reset.
	BackupWritten bool
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

	var ownerID int64
	var ownerUsername string
	if err := tx.QueryRowContext(ctx, `SELECT id, username FROM users WHERE is_owner = 1`).Scan(&ownerID, &ownerUsername); err != nil {
		return fmt.Errorf("read recovered owner: %w", err)
	}

	revoked, err := tx.ExecContext(ctx, `
		UPDATE auth_sessions
		SET revoked_at = ?
		WHERE revoked_at IS NULL
	`, params.RevokedAt)
	if err != nil {
		return fmt.Errorf("revoke owner sessions: %w", err)
	}
	sessionsRevoked, err := revoked.RowsAffected()
	if err != nil {
		return fmt.Errorf("read revoked owner session rows: %w", err)
	}
	tokensRevokedResult, err := tx.ExecContext(ctx, `UPDATE personal_access_tokens SET revoked_at = ? WHERE revoked_at IS NULL`, params.RevokedAt)
	if err != nil {
		return fmt.Errorf("revoke owner API credentials: %w", err)
	}
	tokensRevoked, err := tokensRevokedResult.RowsAffected()
	if err != nil {
		return fmt.Errorf("count revoked API credentials: %w", err)
	}

	var mfaStatus string
	if err := tx.QueryRowContext(ctx, `
		SELECT COALESCE((SELECT status FROM user_mfa_totp WHERE user_id = ?), '')
	`, ownerID).Scan(&mfaStatus); err != nil {
		return fmt.Errorf("read owner mfa enrollment: %w", err)
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

	// The command's stdout is not a record (#288): the reset, the sessions it
	// ended and the MFA it cleared are one attributed event, written in this
	// transaction. It shares the reset's failure domain — the same database,
	// the same commit — so it cannot turn a working recovery into a lockout.
	metadata, err := json.Marshal(struct {
		SessionsRevoked   int64  `json:"sessions_revoked"`
		PreviousMFAStatus string `json:"previous_mfa_status,omitempty"`
		BackupWritten     bool   `json:"backup_written"`
		TokensRevoked     int64  `json:"tokens_revoked"`
	}{sessionsRevoked, mfaStatus, params.BackupWritten, tokensRevoked})
	if err != nil {
		return fmt.Errorf("encode owner recovery event metadata: %w", err)
	}
	if err := insertAuthenticationEvent(ctx, tx, AuthenticationEventParams{
		OccurredAt:   params.UpdatedAt,
		EventType:    "owner_access_recovered",
		Outcome:      "success",
		Username:     ownerUsername,
		UserID:       ownerID,
		Origin:       "cli_recovery",
		MetadataJSON: string(metadata),
	}); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit recovery transaction: %w", err)
	}
	committed = true

	return nil
}
