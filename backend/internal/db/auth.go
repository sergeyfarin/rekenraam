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

type AuthenticationEventParams struct {
	OccurredAt    string
	EventType     string
	Outcome       string
	Username      string
	UserID        int64
	AuthSessionID int64
	ClientIP      string
	FailureReason string
	RequestID     string
}

type AuthenticationEventRecord struct {
	ID            int64
	OccurredAt    string
	EventType     string
	Outcome       string
	Username      string
	UserID        sql.NullInt64
	AuthSessionID sql.NullInt64
	ClientIP      string
	FailureReason string
	RequestID     string
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

// CreateSession returns the new session id so an authentication event can be
// correlated with the session it created (S-07).
func (r *AuthRepository) CreateSession(ctx context.Context, params CreateSessionParams) (int64, error) {
	result, err := r.database.ExecContext(ctx, `
		INSERT INTO auth_sessions (user_id, token_hash, created_at, expires_at, revoked_at)
		VALUES (?, ?, ?, ?, NULL)
	`, params.UserID, params.TokenHash, params.CreatedAt, params.ExpiresAt)
	if err != nil {
		return 0, fmt.Errorf("insert auth session: %w", err)
	}
	sessionID, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("read auth session id: %w", err)
	}

	return sessionID, nil
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

// RecordAuthenticationEvent appends one row to the operational authentication
// log (S-07). It never stores password material or session tokens.
func (r *AuthRepository) RecordAuthenticationEvent(ctx context.Context, params AuthenticationEventParams) error {
	if _, err := r.database.ExecContext(ctx, `
		INSERT INTO authentication_events (
			occurred_at, event_type, outcome, username, user_id, auth_session_id,
			client_ip, failure_reason, request_id
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, params.OccurredAt, params.EventType, params.Outcome, params.Username,
		nullablePositiveInt64(params.UserID), nullablePositiveInt64(params.AuthSessionID),
		params.ClientIP, params.FailureReason, params.RequestID); err != nil {
		return fmt.Errorf("insert authentication event: %w", err)
	}

	return nil
}

// ListAuthenticationEvents returns the most recent events first. The caller
// asks for one more than it needs to detect a further page.
func (r *AuthRepository) ListAuthenticationEvents(ctx context.Context, limit int) ([]AuthenticationEventRecord, error) {
	rows, err := r.database.QueryContext(ctx, `
		SELECT id, occurred_at, event_type, outcome, username, user_id, auth_session_id,
			client_ip, failure_reason, request_id
		FROM authentication_events
		ORDER BY occurred_at DESC, id DESC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("list authentication events: %w", err)
	}
	defer rows.Close()

	var records []AuthenticationEventRecord
	for rows.Next() {
		var record AuthenticationEventRecord
		if err := rows.Scan(&record.ID, &record.OccurredAt, &record.EventType, &record.Outcome,
			&record.Username, &record.UserID, &record.AuthSessionID, &record.ClientIP,
			&record.FailureReason, &record.RequestID); err != nil {
			return nil, fmt.Errorf("scan authentication event: %w", err)
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate authentication events: %w", err)
	}

	return records, nil
}

// DeleteAuthenticationEventsBefore prunes the log to its retention window.
func (r *AuthRepository) DeleteAuthenticationEventsBefore(ctx context.Context, cutoff string) (int64, error) {
	result, err := r.database.ExecContext(ctx, `DELETE FROM authentication_events WHERE occurred_at < ?`, cutoff)
	if err != nil {
		return 0, fmt.Errorf("delete authentication events: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("read deleted authentication event rows affected: %w", err)
	}

	return rowsAffected, nil
}

// FailedLoginsSince counts failed attempts in a window, optionally narrowed to
// one client IP. It is the number an operator needs to answer "is this a
// brute-force run or one fat-fingered password?".
func (r *AuthRepository) FailedLoginsSince(ctx context.Context, since string, clientIP string) (int, error) {
	query := `
		SELECT COUNT(*)
		FROM authentication_events
		WHERE outcome = 'failure' AND occurred_at >= ?
	`
	args := []any{since}
	if clientIP != "" {
		query += ` AND client_ip = ?`
		args = append(args, clientIP)
	}
	var count int
	if err := r.database.QueryRowContext(ctx, query, args...).Scan(&count); err != nil {
		return 0, fmt.Errorf("count failed logins: %w", err)
	}

	return count, nil
}

// --- Approved-device records (S-04) ---

type TrustedDeviceRecord struct {
	ID              int64
	UserID          int64
	CreatedAt       string
	LastUsedAt      string
	ExpiresAt       string
	CreatedClientIP string
}

type CreateTrustedDeviceParams struct {
	UserID          int64
	TokenHash       string
	CreatedAt       string
	ExpiresAt       string
	CreatedClientIP string
}

func (r *AuthRepository) CreateTrustedDevice(ctx context.Context, params CreateTrustedDeviceParams) error {
	if _, err := r.database.ExecContext(ctx, `
		INSERT INTO login_trusted_devices (user_id, token_hash, created_at, last_used_at, expires_at, created_client_ip)
		VALUES (?, ?, ?, ?, ?, ?)
	`, params.UserID, params.TokenHash, params.CreatedAt, params.CreatedAt, params.ExpiresAt, params.CreatedClientIP); err != nil {
		return fmt.Errorf("insert login trusted device: %w", err)
	}

	return nil
}

// ReadTrustedDevice resolves an unexpired, unrevoked device by token hash.
func (r *AuthRepository) ReadTrustedDevice(ctx context.Context, tokenHash string, now string) (TrustedDeviceRecord, error) {
	var record TrustedDeviceRecord
	if err := r.database.QueryRowContext(ctx, `
		SELECT id, user_id, created_at, last_used_at, expires_at, created_client_ip
		FROM login_trusted_devices
		WHERE token_hash = ? AND revoked_at IS NULL AND expires_at > ?
	`, tokenHash, now).Scan(&record.ID, &record.UserID, &record.CreatedAt, &record.LastUsedAt,
		&record.ExpiresAt, &record.CreatedClientIP); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return TrustedDeviceRecord{}, ErrNotFound
		}
		return TrustedDeviceRecord{}, fmt.Errorf("read login trusted device: %w", err)
	}

	return record, nil
}

// TouchTrustedDevice records use and slides the expiry forward, so a device in
// regular use never lapses and an abandoned one does.
func (r *AuthRepository) TouchTrustedDevice(ctx context.Context, deviceID int64, now string, expiresAt string) error {
	if _, err := r.database.ExecContext(ctx, `
		UPDATE login_trusted_devices
		SET last_used_at = ?, expires_at = ?
		WHERE id = ? AND revoked_at IS NULL
	`, now, expiresAt, deviceID); err != nil {
		return fmt.Errorf("touch login trusted device: %w", err)
	}

	return nil
}

func (r *AuthRepository) ListTrustedDevices(ctx context.Context, userID int64, now string) ([]TrustedDeviceRecord, error) {
	rows, err := r.database.QueryContext(ctx, `
		SELECT id, user_id, created_at, last_used_at, expires_at, created_client_ip
		FROM login_trusted_devices
		WHERE user_id = ? AND revoked_at IS NULL AND expires_at > ?
		ORDER BY last_used_at DESC, id DESC
	`, userID, now)
	if err != nil {
		return nil, fmt.Errorf("list login trusted devices: %w", err)
	}
	defer rows.Close()

	var records []TrustedDeviceRecord
	for rows.Next() {
		var record TrustedDeviceRecord
		if err := rows.Scan(&record.ID, &record.UserID, &record.CreatedAt, &record.LastUsedAt,
			&record.ExpiresAt, &record.CreatedClientIP); err != nil {
			return nil, fmt.Errorf("scan login trusted device: %w", err)
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate login trusted devices: %w", err)
	}

	return records, nil
}

func (r *AuthRepository) RevokeTrustedDevice(ctx context.Context, userID int64, deviceID int64, revokedAt string) error {
	result, err := r.database.ExecContext(ctx, `
		UPDATE login_trusted_devices
		SET revoked_at = ?
		WHERE id = ? AND user_id = ? AND revoked_at IS NULL
	`, revokedAt, deviceID, userID)
	if err != nil {
		return fmt.Errorf("revoke login trusted device: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read revoked login trusted device rows: %w", err)
	}
	if rowsAffected == 0 {
		return ErrNotFound
	}

	return nil
}

func (r *AuthRepository) DeleteExpiredOrRevokedTrustedDevices(ctx context.Context, now string) (int64, error) {
	result, err := r.database.ExecContext(ctx, `
		DELETE FROM login_trusted_devices
		WHERE revoked_at IS NOT NULL OR expires_at <= ?
	`, now)
	if err != nil {
		return 0, fmt.Errorf("delete expired login trusted devices: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("read deleted login trusted device rows: %w", err)
	}

	return rowsAffected, nil
}

func (r *AuthRepository) DeleteLoginThrottle(ctx context.Context, scopeType string, scopeKey string) error {
	if _, err := r.database.ExecContext(ctx, `DELETE FROM login_throttles WHERE scope_type = ? AND scope_key = ?`, scopeType, scopeKey); err != nil {
		return fmt.Errorf("delete login throttle: %w", err)
	}

	return nil
}
