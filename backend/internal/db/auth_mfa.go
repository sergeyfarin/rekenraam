package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// ErrMFARecoveryCodeUsed distinguishes "this code exists but has already been
// spent" from "no such code", so the service can say so in the auth log
// without leaking the difference to the caller.
var ErrMFARecoveryCodeUsed = errors.New("mfa recovery code already used")

type MFATOTPRecord struct {
	UserID           int64
	SecretCiphertext string
	Status           string
	CreatedAt        string
	ActivatedAt      sql.NullString
	LastUsedStep     sql.NullInt64
}

type UpsertMFATOTPParams struct {
	UserID           int64
	SecretCiphertext string
	Status           string
	CreatedAt        string
}

type CreateMFAChallengeParams struct {
	UserID    int64
	TokenHash string
	CreatedAt string
	ExpiresAt string
	ClientIP  string
}

type MFAChallengeRecord struct {
	ID       int64
	UserID   int64
	ClientIP string
}

// ReadMFATOTP returns the user's enrollment, pending or active.
func (r *AuthRepository) ReadMFATOTP(ctx context.Context, userID int64) (MFATOTPRecord, error) {
	var record MFATOTPRecord
	if err := r.database.QueryRowContext(ctx, `
		SELECT user_id, secret_ciphertext, status, created_at, activated_at, last_used_step
		FROM user_mfa_totp
		WHERE user_id = ?
	`, userID).Scan(&record.UserID, &record.SecretCiphertext, &record.Status,
		&record.CreatedAt, &record.ActivatedAt, &record.LastUsedStep); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return MFATOTPRecord{}, ErrNotFound
		}
		return MFATOTPRecord{}, fmt.Errorf("read mfa totp enrollment: %w", err)
	}

	return record, nil
}

// UpsertMFATOTP replaces any existing enrollment. Starting a new enrollment
// discards the old secret and its replay counter together — a half-finished
// re-enrollment must never leave the previous secret partially in force.
func (r *AuthRepository) UpsertMFATOTP(ctx context.Context, params UpsertMFATOTPParams) error {
	if _, err := r.database.ExecContext(ctx, `
		INSERT INTO user_mfa_totp (user_id, secret_ciphertext, status, created_at, activated_at, last_used_step)
		VALUES (?, ?, ?, ?, NULL, NULL)
		ON CONFLICT(user_id) DO UPDATE SET
			secret_ciphertext = excluded.secret_ciphertext,
			status = excluded.status,
			created_at = excluded.created_at,
			activated_at = NULL,
			last_used_step = NULL
	`, params.UserID, params.SecretCiphertext, params.Status, params.CreatedAt); err != nil {
		return fmt.Errorf("upsert mfa totp enrollment: %w", err)
	}

	return nil
}

// ActivateMFATOTP promotes a pending enrollment once a code has proved the
// user really stored the secret, recording the step that code came from so it
// cannot be replayed.
func (r *AuthRepository) ActivateMFATOTP(ctx context.Context, userID int64, activatedAt string, usedStep int64) error {
	result, err := r.database.ExecContext(ctx, `
		UPDATE user_mfa_totp
		SET status = 'active', activated_at = ?, last_used_step = ?
		WHERE user_id = ? AND status = 'pending'
	`, activatedAt, usedStep, userID)
	if err != nil {
		return fmt.Errorf("activate mfa totp enrollment: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read activated mfa totp rows: %w", err)
	}
	if rowsAffected != 1 {
		return ErrNotFound
	}

	return nil
}

// RecordMFATOTPUse advances the replay counter. The WHERE clause makes the
// check-and-set atomic: two concurrent logins presenting the same code cannot
// both pass, because only one UPDATE can move the counter past that step.
func (r *AuthRepository) RecordMFATOTPUse(ctx context.Context, userID int64, usedStep int64) (bool, error) {
	result, err := r.database.ExecContext(ctx, `
		UPDATE user_mfa_totp
		SET last_used_step = ?
		WHERE user_id = ?
		  AND status = 'active'
		  AND (last_used_step IS NULL OR last_used_step < ?)
	`, usedStep, userID, usedStep)
	if err != nil {
		return false, fmt.Errorf("record mfa totp use: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("read mfa totp use rows: %w", err)
	}

	return rowsAffected == 1, nil
}

// DeleteMFATOTP removes the enrollment and every recovery code with it, in one
// transaction: an enrollment without codes, or codes without an enrollment,
// are both states the login path should never have to reason about.
func (r *AuthRepository) DeleteMFATOTP(ctx context.Context, userID int64) error {
	tx, err := r.database.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin mfa disable transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `DELETE FROM user_mfa_recovery_codes WHERE user_id = ?`, userID); err != nil {
		return fmt.Errorf("delete mfa recovery codes: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM user_mfa_totp WHERE user_id = ?`, userID); err != nil {
		return fmt.Errorf("delete mfa totp enrollment: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM login_mfa_challenges WHERE user_id = ?`, userID); err != nil {
		return fmt.Errorf("delete mfa challenges: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit mfa disable transaction: %w", err)
	}

	return nil
}

// ReplaceMFARecoveryCodes swaps the whole set atomically. Codes are issued and
// replaced as a set so the count the owner is shown is always the truth.
func (r *AuthRepository) ReplaceMFARecoveryCodes(ctx context.Context, userID int64, codeHashes []string, createdAt string) error {
	tx, err := r.database.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin mfa recovery code transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `DELETE FROM user_mfa_recovery_codes WHERE user_id = ?`, userID); err != nil {
		return fmt.Errorf("delete previous mfa recovery codes: %w", err)
	}
	for _, codeHash := range codeHashes {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO user_mfa_recovery_codes (user_id, code_hash, created_at)
			VALUES (?, ?, ?)
		`, userID, codeHash, createdAt); err != nil {
			return fmt.Errorf("insert mfa recovery code: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit mfa recovery code transaction: %w", err)
	}

	return nil
}

// ConsumeMFARecoveryCode spends a code if it is this user's and unused. The
// single UPDATE is the check: under the one-connection pool it cannot race
// with a second attempt using the same code.
func (r *AuthRepository) ConsumeMFARecoveryCode(ctx context.Context, userID int64, codeHash string, usedAt string, clientIP string) error {
	result, err := r.database.ExecContext(ctx, `
		UPDATE user_mfa_recovery_codes
		SET used_at = ?, used_client_ip = ?
		WHERE user_id = ? AND code_hash = ? AND used_at IS NULL
	`, usedAt, clientIP, userID, codeHash)
	if err != nil {
		return fmt.Errorf("consume mfa recovery code: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read consumed mfa recovery code rows: %w", err)
	}
	if rowsAffected == 1 {
		return nil
	}

	var usedCount int
	if err := r.database.QueryRowContext(ctx, `
		SELECT COUNT(1) FROM user_mfa_recovery_codes
		WHERE user_id = ? AND code_hash = ? AND used_at IS NOT NULL
	`, userID, codeHash).Scan(&usedCount); err != nil {
		return fmt.Errorf("check spent mfa recovery code: %w", err)
	}
	if usedCount > 0 {
		return ErrMFARecoveryCodeUsed
	}

	return ErrNotFound
}

// CountUnusedMFARecoveryCodes is the "you have N codes left" number.
func (r *AuthRepository) CountUnusedMFARecoveryCodes(ctx context.Context, userID int64) (int, error) {
	var count int
	if err := r.database.QueryRowContext(ctx, `
		SELECT COUNT(1) FROM user_mfa_recovery_codes WHERE user_id = ? AND used_at IS NULL
	`, userID).Scan(&count); err != nil {
		return 0, fmt.Errorf("count unused mfa recovery codes: %w", err)
	}

	return count, nil
}

func (r *AuthRepository) CreateMFAChallenge(ctx context.Context, params CreateMFAChallengeParams) error {
	if _, err := r.database.ExecContext(ctx, `
		INSERT INTO login_mfa_challenges (user_id, token_hash, created_at, expires_at, client_ip)
		VALUES (?, ?, ?, ?, ?)
	`, params.UserID, params.TokenHash, params.CreatedAt, params.ExpiresAt, params.ClientIP); err != nil {
		return fmt.Errorf("insert mfa challenge: %w", err)
	}

	return nil
}

// ReadMFAChallenge resolves an unconsumed, unexpired challenge.
func (r *AuthRepository) ReadMFAChallenge(ctx context.Context, tokenHash string, now string) (MFAChallengeRecord, error) {
	var record MFAChallengeRecord
	if err := r.database.QueryRowContext(ctx, `
		SELECT id, user_id, client_ip
		FROM login_mfa_challenges
		WHERE token_hash = ? AND consumed_at IS NULL AND expires_at > ?
	`, tokenHash, now).Scan(&record.ID, &record.UserID, &record.ClientIP); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return MFAChallengeRecord{}, ErrNotFound
		}
		return MFAChallengeRecord{}, fmt.Errorf("read mfa challenge: %w", err)
	}

	return record, nil
}

// ConsumeMFAChallenge marks a challenge spent. It reports whether it won: a
// challenge is single-use, so a replayed one must not produce a second
// session.
func (r *AuthRepository) ConsumeMFAChallenge(ctx context.Context, challengeID int64, consumedAt string) (bool, error) {
	result, err := r.database.ExecContext(ctx, `
		UPDATE login_mfa_challenges
		SET consumed_at = ?
		WHERE id = ? AND consumed_at IS NULL
	`, consumedAt, challengeID)
	if err != nil {
		return false, fmt.Errorf("consume mfa challenge: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("read consumed mfa challenge rows: %w", err)
	}

	return rowsAffected == 1, nil
}

func (r *AuthRepository) DeleteExpiredOrConsumedMFAChallenges(ctx context.Context, now string) (int64, error) {
	result, err := r.database.ExecContext(ctx, `
		DELETE FROM login_mfa_challenges
		WHERE consumed_at IS NOT NULL OR expires_at <= ?
	`, now)
	if err != nil {
		return 0, fmt.Errorf("delete expired mfa challenges: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("read deleted mfa challenge rows: %w", err)
	}

	return rowsAffected, nil
}
