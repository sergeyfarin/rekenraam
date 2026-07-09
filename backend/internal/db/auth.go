package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

type AuthRepository struct {
	database *sql.DB
}

type OwnerCredentialsRecord struct {
	ID           int64
	Username     string
	PasswordHash string
}

type AuthenticatedUserRecord struct {
	SessionID int64
	ID        int64
	Username  string
}

type CreateSessionParams struct {
	UserID    int64
	TokenHash string
	CreatedAt string
	ExpiresAt string
}

type UpdatePasswordHashParams struct {
	UserID       int64
	PasswordHash string
	UpdatedAt    string
}

type LoginThrottleRecord struct {
	ScopeType      string
	ScopeKey       string
	FailedAttempts int
	BlockedUntil   sql.NullString
	UpdatedAt      string
}

type UpsertLoginThrottleParams struct {
	ScopeType      string
	ScopeKey       string
	FailedAttempts int
	BlockedUntil   *string
	UpdatedAt      string
}

func NewAuthRepository(database *sql.DB) *AuthRepository {
	return &AuthRepository{database: database}
}

func (r *AuthRepository) OwnerExists(ctx context.Context) (bool, error) {
	var ownerCount int
	if err := r.database.QueryRowContext(ctx, `SELECT COUNT(1) FROM users WHERE is_owner = 1`).Scan(&ownerCount); err != nil {
		return false, fmt.Errorf("count owners: %w", err)
	}

	return ownerCount > 0, nil
}

func (r *AuthRepository) ReadOwnerCredentials(ctx context.Context, username string) (OwnerCredentialsRecord, error) {
	var record OwnerCredentialsRecord
	if err := r.database.QueryRowContext(ctx, `
		SELECT id, username, password_hash
		FROM users
		WHERE is_owner = 1 AND username = ?
	`, username).Scan(&record.ID, &record.Username, &record.PasswordHash); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return OwnerCredentialsRecord{}, ErrNotFound
		}
		return OwnerCredentialsRecord{}, fmt.Errorf("read owner credentials: %w", err)
	}

	return record, nil
}

func (r *AuthRepository) CreateSession(ctx context.Context, params CreateSessionParams) error {
	if _, err := r.database.ExecContext(ctx, `
		INSERT INTO auth_sessions (user_id, token_hash, created_at, expires_at, revoked_at)
		VALUES (?, ?, ?, ?, NULL)
	`, params.UserID, params.TokenHash, params.CreatedAt, params.ExpiresAt); err != nil {
		return fmt.Errorf("insert auth session: %w", err)
	}

	return nil
}

func (r *AuthRepository) ReadSessionUser(ctx context.Context, tokenHash string, now string) (AuthenticatedUserRecord, error) {
	var record AuthenticatedUserRecord
	if err := r.database.QueryRowContext(ctx, `
		SELECT auth_sessions.id, users.id, users.username
		FROM auth_sessions
		JOIN users ON users.id = auth_sessions.user_id
		WHERE auth_sessions.token_hash = ?
		  AND auth_sessions.revoked_at IS NULL
		  AND auth_sessions.expires_at > ?
	`, tokenHash, now).Scan(&record.SessionID, &record.ID, &record.Username); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return AuthenticatedUserRecord{}, ErrNotFound
		}
		return AuthenticatedUserRecord{}, fmt.Errorf("read auth session user: %w", err)
	}

	return record, nil
}

func (r *AuthRepository) RevokeSession(ctx context.Context, tokenHash string, revokedAt string) error {
	if _, err := r.database.ExecContext(ctx, `
		UPDATE auth_sessions
		SET revoked_at = ?
		WHERE token_hash = ? AND revoked_at IS NULL
	`, revokedAt, tokenHash); err != nil {
		return fmt.Errorf("revoke auth session: %w", err)
	}

	return nil
}

func (r *AuthRepository) DeleteExpiredOrRevokedSessions(ctx context.Context, now string) (int64, error) {
	result, err := r.database.ExecContext(ctx, `
		DELETE FROM auth_sessions
		WHERE revoked_at IS NOT NULL
		   OR expires_at <= ?
	`, now)
	if err != nil {
		return 0, fmt.Errorf("delete expired or revoked auth sessions: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("read deleted auth session rows affected: %w", err)
	}

	return rowsAffected, nil
}

func (r *AuthRepository) UpdatePasswordHash(ctx context.Context, params UpdatePasswordHashParams) error {
	result, err := r.database.ExecContext(ctx, `
		UPDATE users
		SET password_hash = ?, updated_at = ?
		WHERE id = ? AND is_owner = 1
	`, params.PasswordHash, params.UpdatedAt, params.UserID)
	if err != nil {
		return fmt.Errorf("update owner password hash: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read owner password rows affected: %w", err)
	}
	if rowsAffected != 1 {
		return ErrNotFound
	}

	return nil
}

func (r *AuthRepository) ReadLoginThrottle(ctx context.Context, scopeType string, scopeKey string) (LoginThrottleRecord, error) {
	var record LoginThrottleRecord
	if err := r.database.QueryRowContext(ctx, `
		SELECT scope_type, scope_key, failed_attempts, blocked_until, updated_at
		FROM login_throttles
		WHERE scope_type = ? AND scope_key = ?
	`, scopeType, scopeKey).Scan(&record.ScopeType, &record.ScopeKey, &record.FailedAttempts, &record.BlockedUntil, &record.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return LoginThrottleRecord{}, ErrNotFound
		}
		return LoginThrottleRecord{}, fmt.Errorf("read login throttle: %w", err)
	}

	return record, nil
}

func (r *AuthRepository) UpsertLoginThrottle(ctx context.Context, params UpsertLoginThrottleParams) error {
	if _, err := r.database.ExecContext(ctx, `
		INSERT INTO login_throttles (scope_type, scope_key, failed_attempts, blocked_until, updated_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(scope_type, scope_key) DO UPDATE SET
			failed_attempts = excluded.failed_attempts,
			blocked_until = excluded.blocked_until,
			updated_at = excluded.updated_at
	`, params.ScopeType, params.ScopeKey, params.FailedAttempts, params.BlockedUntil, params.UpdatedAt); err != nil {
		return fmt.Errorf("upsert login throttle: %w", err)
	}

	return nil
}

func (r *AuthRepository) DeleteLoginThrottle(ctx context.Context, scopeType string, scopeKey string) error {
	if _, err := r.database.ExecContext(ctx, `DELETE FROM login_throttles WHERE scope_type = ? AND scope_key = ?`, scopeType, scopeKey); err != nil {
		return fmt.Errorf("delete login throttle: %w", err)
	}

	return nil
}
