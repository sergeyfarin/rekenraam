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
	ExpiresAt string
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
	// Origin defaults to browser_api; only owner recovery writes cli_recovery.
	Origin string
	// SecondFactor is totp or recovery_code on an MFA sign-in (#288).
	SecondFactor string
	// MetadataJSON is a JSON object of non-secret context; empty means {}.
	MetadataJSON string
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
	Origin        string
	SecondFactor  string
	MetadataJSON  string
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
	Generation     string
}

type UpsertLoginThrottleParams struct {
	ScopeType      string
	ScopeKey       string
	FailedAttempts int
	BlockedUntil   *string
	UpdatedAt      string
	Generation     string
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

// ReadOwnerCredentialsByID is the lookup for a caller that already holds a
// session (re-authentication before a security change) or a resolved MFA
// challenge, where the user id is known and the username is not.
func (r *AuthRepository) ReadOwnerCredentialsByID(ctx context.Context, userID int64) (OwnerCredentialsRecord, error) {
	var record OwnerCredentialsRecord
	if err := r.database.QueryRowContext(ctx, `
		SELECT id, username, password_hash
		FROM users
		WHERE is_owner = 1 AND id = ?
	`, userID).Scan(&record.ID, &record.Username, &record.PasswordHash); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return OwnerCredentialsRecord{}, ErrNotFound
		}
		return OwnerCredentialsRecord{}, fmt.Errorf("read owner credentials by id: %w", err)
	}

	return record, nil
}

// ThrottleScopeKey pins one generation of a login_throttles row (#366).
type ThrottleScopeKey struct {
	ScopeType  string
	ScopeKey   string
	Generation string
}

// CompleteLoginParams is everything a verified sign-in writes. Device is the
// approved device to extend, or NewDevice the one to create; at most one is
// set.
type CompleteLoginParams struct {
	Session          CreateSessionParams
	ClearThrottles   []ThrottleScopeKey
	RefundThrottles  []ThrottleScopeKey
	TouchDeviceID    int64
	DeviceUsedAt     string
	DeviceExpiresAt  string
	NewDevice        *CreateTrustedDeviceParams
	MFAChallengeHash string
}

// CompleteLogin writes the session, settles the attempt's throttle places and
// approves the device in one transaction (#287), and returns the session id
// so an authentication event can be correlated with it (S-07). A failure in
// any of them leaves no session behind.
func (r *AuthRepository) CompleteLogin(ctx context.Context, params CompleteLoginParams) (int64, error) {
	tx, err := r.database.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin login transaction: %w", err)
	}
	defer rollbackTx(ctx, tx)

	result, err := tx.ExecContext(ctx, `
		INSERT INTO auth_sessions (user_id, token_hash, created_at, expires_at, revoked_at)
		SELECT ?, ?, ?, ?, NULL
		WHERE ? = '' OR EXISTS (
			SELECT 1 FROM login_mfa_challenges c JOIN user_mfa_totp m ON m.user_id=c.user_id
			WHERE c.token_hash=? AND c.user_id=? AND c.consumed_at IS NOT NULL AND m.status='active'
		)
	`, params.Session.UserID, params.Session.TokenHash, params.Session.CreatedAt, params.Session.ExpiresAt,
		params.MFAChallengeHash, params.MFAChallengeHash, params.Session.UserID)
	if err != nil {
		return 0, fmt.Errorf("insert auth session: %w", err)
	}
	if rows, err := result.RowsAffected(); err != nil {
		return 0, fmt.Errorf("read inserted auth session rows: %w", err)
	} else if rows != 1 {
		return 0, ErrNotFound
	}
	sessionID, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("read auth session id: %w", err)
	}
	for _, scope := range params.ClearThrottles {
		if err := deleteLoginThrottleExec(ctx, tx, scope); err != nil {
			return 0, err
		}
	}
	for _, scope := range params.RefundThrottles {
		if err := refundLoginAttemptExec(ctx, tx, scope); err != nil {
			return 0, err
		}
	}
	// Extending a used device slides its expiry forward, so a device in
	// regular use never lapses and an abandoned one does.
	if params.TouchDeviceID > 0 {
		if _, err := tx.ExecContext(ctx, `
			UPDATE login_trusted_devices
			SET last_used_at = ?, expires_at = ?
			WHERE id = ? AND revoked_at IS NULL
		`, params.DeviceUsedAt, params.DeviceExpiresAt, params.TouchDeviceID); err != nil {
			return 0, fmt.Errorf("touch login trusted device: %w", err)
		}
	}
	if params.NewDevice != nil {
		if err := createTrustedDeviceExec(ctx, tx, *params.NewDevice); err != nil {
			return 0, err
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit login transaction: %w", err)
	}

	return sessionID, nil
}

func (r *AuthRepository) ReadSessionUser(ctx context.Context, tokenHash string, now string) (AuthenticatedUserRecord, error) {
	var record AuthenticatedUserRecord
	if err := r.database.QueryRowContext(ctx, `
		SELECT auth_sessions.id, users.id, users.username, auth_sessions.expires_at
		FROM auth_sessions
		JOIN users ON users.id = auth_sessions.user_id
		WHERE auth_sessions.token_hash = ?
		  AND auth_sessions.revoked_at IS NULL
		  AND auth_sessions.expires_at > ?
	`, tokenHash, now).Scan(&record.SessionID, &record.ID, &record.Username, &record.ExpiresAt); err != nil {
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

// UpdateLoginThrottle reads one throttle row and writes what decide returns,
// in one transaction (#285). Throttle admission and settlement are each a
// read-modify-write; done as separate statements, two overlapping attempts
// read the same count and one of them is lost. found is false when no row
// exists; a nil result writes nothing. decide must not touch the database.
func (r *AuthRepository) UpdateLoginThrottle(ctx context.Context, scopeType string, scopeKey string, decide func(current LoginThrottleRecord, found bool) (*UpsertLoginThrottleParams, error)) error {
	tx, err := r.database.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin login throttle transaction: %w", err)
	}
	defer rollbackTx(ctx, tx)

	var record LoginThrottleRecord
	// A deferred transaction has not acquired SQLite's writer yet. Establish
	// admission before reading the row or sampling a decision clock (#364).
	if _, err := tx.ExecContext(ctx, `UPDATE login_throttles SET updated_at=updated_at WHERE 0`); err != nil {
		return fmt.Errorf("acquire login throttle admission: %w", err)
	}
	found := true
	if err := tx.QueryRowContext(ctx, `
		SELECT scope_type, scope_key, failed_attempts, blocked_until, updated_at, generation
		FROM login_throttles
		WHERE scope_type = ? AND scope_key = ?
	`, scopeType, scopeKey).Scan(&record.ScopeType, &record.ScopeKey, &record.FailedAttempts, &record.BlockedUntil, &record.UpdatedAt, &record.Generation); err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("read login throttle: %w", err)
		}
		found = false
	}
	next, err := decide(record, found)
	if err != nil {
		return err
	}
	if next != nil {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO login_throttles (scope_type, scope_key, failed_attempts, blocked_until, updated_at, generation)
			VALUES (?, ?, ?, ?, ?, ?)
			ON CONFLICT(scope_type, scope_key) DO UPDATE SET
				failed_attempts = excluded.failed_attempts,
				blocked_until = excluded.blocked_until,
				updated_at = excluded.updated_at,
				generation = excluded.generation
		`, scopeType, scopeKey, next.FailedAttempts, next.BlockedUntil, next.UpdatedAt, next.Generation); err != nil {
			return fmt.Errorf("upsert login throttle: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit login throttle transaction: %w", err)
	}

	return nil
}

// RefundLoginAttempt gives back one admitted attempt's place. A blocked row
// is left alone: a block is never shortened by an attempt that settled after
// it. A replacement generation is never refunded.
func (r *AuthRepository) RefundLoginAttempt(ctx context.Context, scope ThrottleScopeKey) error {
	return refundLoginAttemptExec(ctx, r.database, scope)
}

func refundLoginAttemptExec(ctx context.Context, exec execContexter, scope ThrottleScopeKey) error {
	if _, err := exec.ExecContext(ctx, `
		UPDATE login_throttles
		SET failed_attempts = MAX(failed_attempts - 1, 0)
		WHERE scope_type = ? AND scope_key = ? AND generation = ? AND blocked_until IS NULL
	`, scope.ScopeType, scope.ScopeKey, scope.Generation); err != nil {
		return fmt.Errorf("refund login attempt: %w", err)
	}

	return nil
}

// RecordAuthenticationEvent appends one row to the operational authentication
// log (S-07). It never stores password material or session tokens.
func (r *AuthRepository) RecordAuthenticationEvent(ctx context.Context, params AuthenticationEventParams) error {
	return insertAuthenticationEvent(ctx, r.database, params)
}

// insertAuthenticationEvent is the one writer of authentication_events, so a
// security change can append its event inside its own transaction (#288).
// An empty username is filled from the user row, so every attributed event
// names the account even when the caller only knew its id. A session that
// cleanup deleted after the request authenticated is recorded as absent, the
// same outcome as the column's ON DELETE SET NULL, rather than failing the
// change it describes.
func insertAuthenticationEvent(ctx context.Context, exec execContexter, params AuthenticationEventParams) error {
	origin := params.Origin
	if origin == "" {
		origin = "browser_api"
	}
	metadata := params.MetadataJSON
	if metadata == "" {
		metadata = "{}"
	}
	if _, err := exec.ExecContext(ctx, `
		INSERT INTO authentication_events (
			occurred_at, event_type, outcome, username, user_id, auth_session_id,
			client_ip, failure_reason, request_id, origin, second_factor, metadata_json
		)
		VALUES (?, ?, ?, COALESCE(NULLIF(?, ''), (SELECT username FROM users WHERE id = ?), ''),
			?, (SELECT id FROM auth_sessions WHERE id = ?), ?, ?, ?, ?, ?, ?)
	`, params.OccurredAt, params.EventType, params.Outcome, params.Username, params.UserID,
		nullablePositiveInt64(params.UserID), nullablePositiveInt64(params.AuthSessionID),
		params.ClientIP, params.FailureReason, params.RequestID, origin, params.SecondFactor,
		metadata); err != nil {
		return fmt.Errorf("insert authentication event: %w", err)
	}

	return nil
}

// ListAuthenticationEvents returns the most recent events first. The caller
// asks for one more than it needs to detect a further page.
func (r *AuthRepository) ListAuthenticationEvents(ctx context.Context, limit int) ([]AuthenticationEventRecord, error) {
	rows, err := r.database.QueryContext(ctx, `
		SELECT id, occurred_at, event_type, outcome, username, user_id, auth_session_id,
			client_ip, failure_reason, request_id, origin, second_factor, metadata_json
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
			&record.FailureReason, &record.RequestID, &record.Origin, &record.SecondFactor,
			&record.MetadataJSON); err != nil {
			return nil, fmt.Errorf("scan authentication event: %w", err)
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate authentication events: %w", err)
	}

	return records, nil
}

// DeleteAuthenticationEventsBefore prunes sign-in attempts and logouts to
// their retention window. Account-security changes (#288) are few and are
// the record of who weakened or reset protection, so they are kept.
func (r *AuthRepository) DeleteAuthenticationEventsBefore(ctx context.Context, cutoff string) (int64, error) {
	result, err := r.database.ExecContext(ctx, `
		DELETE FROM authentication_events
		WHERE occurred_at < ?
		  AND event_type IN ('login_succeeded', 'login_failed', 'login_blocked', 'logout', 'token_authenticated', 'token_denied')
	`, cutoff)
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
	return createTrustedDeviceExec(ctx, r.database, params)
}

func createTrustedDeviceExec(ctx context.Context, exec execContexter, params CreateTrustedDeviceParams) error {
	if _, err := exec.ExecContext(ctx, `
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

// RevokeTrustedDevice revokes one device and appends its event in the same
// transaction (#288).
func (r *AuthRepository) RevokeTrustedDevice(ctx context.Context, userID int64, deviceID int64, revokedAt string, event AuthenticationEventParams) error {
	tx, err := r.database.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin trusted device revocation transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	result, err := tx.ExecContext(ctx, `
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
	if err := insertAuthenticationEvent(ctx, tx, event); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit trusted device revocation transaction: %w", err)
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

func (r *AuthRepository) DeleteLoginThrottle(ctx context.Context, scope ThrottleScopeKey) error {
	return deleteLoginThrottleExec(ctx, r.database, scope)
}

func deleteLoginThrottleExec(ctx context.Context, exec execContexter, scope ThrottleScopeKey) error {
	if _, err := exec.ExecContext(ctx, `DELETE FROM login_throttles WHERE scope_type = ? AND scope_key = ? AND generation = ?`, scope.ScopeType, scope.ScopeKey, scope.Generation); err != nil {
		return fmt.Errorf("delete login throttle: %w", err)
	}

	return nil
}
