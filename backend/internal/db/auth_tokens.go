package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

var ErrTokenLimit = errors.New("active token limit reached")

type PersonalAccessTokenRecord struct {
	ID         int64
	UserID     int64
	Username   string
	Name       string
	ScopesJSON string
	CreatedAt  string
	ExpiresAt  string
	RevokedAt  sql.NullString
	LastUsedAt sql.NullString
}

type CreatePersonalAccessTokenParams struct {
	UserID       int64
	SessionID    int64
	PasswordHash string
	Name         string
	TokenHash    string
	ScopesJSON   string
	CreatedAt    string
	ExpiresAt    string
}

// Own both the pool connection and SQLite write admission before sampling the
// clock (#359). BEGIN is deferred, so sampling immediately after BeginTx still
// freezes expiry during busy_timeout. This empty UPDATE obtains the write lock
// without changing a row or running row triggers; all guards/events follow in
// that same transaction.
func tokenAdmissionTime(ctx context.Context, tx *sql.Tx, clock func() time.Time) (string, error) {
	if _, err := tx.ExecContext(ctx, `UPDATE personal_access_tokens SET last_used_at=last_used_at WHERE 0`); err != nil {
		return "", fmt.Errorf("acquire token write admission: %w", err)
	}
	return clock().UTC().Format(time.RFC3339), nil
}

// Token management rechecks the live session inside its write transaction.
func tokenManagementSession(ctx context.Context, tx *sql.Tx, userID, sessionID int64, now, passwordHash string) error {
	var id int64
	err := tx.QueryRowContext(ctx, `SELECT s.id FROM auth_sessions s JOIN users u ON u.id = s.user_id
		WHERE s.id = ? AND s.user_id = ? AND u.is_owner = 1
		AND s.revoked_at IS NULL AND s.expires_at > ? AND (? = '' OR u.password_hash = ?)`,
		sessionID, userID, now, passwordHash, passwordHash).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("read token management session: %w", err)
	}
	return nil
}

func (r *AuthRepository) CreatePersonalAccessToken(ctx context.Context, p CreatePersonalAccessTokenParams, event AuthenticationEventParams, now func() time.Time) (int64, error) {
	tx, err := r.database.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin token creation: %w", err)
	}
	defer rollbackTx(ctx, tx)
	checkedAt, err := tokenAdmissionTime(ctx, tx, now)
	if err != nil {
		return 0, err
	}
	event.OccurredAt = checkedAt
	if err := tokenManagementSession(ctx, tx, p.UserID, p.SessionID, checkedAt, p.PasswordHash); err != nil {
		return 0, err
	}
	var count int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM personal_access_tokens
		WHERE user_id = ? AND revoked_at IS NULL AND expires_at > ?`, p.UserID, checkedAt).Scan(&count); err != nil {
		return 0, fmt.Errorf("count active tokens: %w", err)
	}
	if count >= 50 {
		return 0, ErrTokenLimit
	}
	result, err := tx.ExecContext(ctx, `INSERT INTO personal_access_tokens
		(user_id, name, token_hash, scopes_json, created_at, expires_at) VALUES (?, ?, ?, ?, ?, ?)`,
		p.UserID, p.Name, p.TokenHash, p.ScopesJSON, p.CreatedAt, p.ExpiresAt)
	if err != nil {
		return 0, fmt.Errorf("insert personal access token: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("read token id: %w", err)
	}
	event.MetadataJSON = fmt.Sprintf(`{"token_id":%d}`, id)
	if err := insertAuthenticationEvent(ctx, tx, event); err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit token creation: %w", err)
	}
	return id, nil
}

func (r *AuthRepository) ListPersonalAccessTokens(ctx context.Context, userID, beforeID int64) ([]PersonalAccessTokenRecord, error) {
	rows, err := r.database.QueryContext(ctx, `SELECT id, user_id, name, scopes_json, created_at, expires_at, revoked_at, last_used_at
		FROM personal_access_tokens WHERE user_id = ? AND (? = 0 OR id < ?) ORDER BY id DESC LIMIT 51`, userID, beforeID, beforeID)
	if err != nil {
		return nil, fmt.Errorf("list personal access tokens: %w", err)
	}
	defer rows.Close()
	result := make([]PersonalAccessTokenRecord, 0)
	for rows.Next() {
		var v PersonalAccessTokenRecord
		if err := rows.Scan(&v.ID, &v.UserID, &v.Name, &v.ScopesJSON, &v.CreatedAt, &v.ExpiresAt, &v.RevokedAt, &v.LastUsedAt); err != nil {
			return nil, fmt.Errorf("scan personal access token: %w", err)
		}
		result = append(result, v)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate personal access tokens: %w", err)
	}
	return result, nil
}

func (r *AuthRepository) ReadPersonalAccessToken(ctx context.Context, hash, now string) (PersonalAccessTokenRecord, error) {
	var v PersonalAccessTokenRecord
	err := r.database.QueryRowContext(ctx, `SELECT t.id, t.user_id, u.username, t.name, t.scopes_json, t.created_at, t.expires_at
		FROM personal_access_tokens t JOIN users u ON u.id = t.user_id
		WHERE t.token_hash = ? AND t.revoked_at IS NULL AND t.expires_at > ? AND u.is_owner = 1`, hash, now).
		Scan(&v.ID, &v.UserID, &v.Username, &v.Name, &v.ScopesJSON, &v.CreatedAt, &v.ExpiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return v, ErrNotFound
	}
	if err != nil {
		return v, fmt.Errorf("read personal access token: %w", err)
	}
	return v, nil
}

// The second check establishes the revocation cutoff. The token's scopes are
// immutable; its lifecycle and the use event commit together before dispatch.
func (r *AuthRepository) RecordPersonalAccessTokenUse(ctx context.Context, id int64, clock func() time.Time, event AuthenticationEventParams) error {
	tx, err := r.database.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin token authentication: %w", err)
	}
	defer rollbackTx(ctx, tx)
	now, err := tokenAdmissionTime(ctx, tx, clock)
	if err != nil {
		return err
	}
	event.OccurredAt = now
	result, err := tx.ExecContext(ctx, `UPDATE personal_access_tokens SET last_used_at = CASE WHEN ? = 'success' THEN ? ELSE last_used_at END
		WHERE id = ? AND revoked_at IS NULL AND expires_at > ?`, event.Outcome, now, id, now)
	if err != nil {
		return fmt.Errorf("record token use: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return ErrNotFound
	}
	if err := insertAuthenticationEvent(ctx, tx, event); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit token authentication: %w", err)
	}
	return nil
}

func (r *AuthRepository) RevokePersonalAccessToken(ctx context.Context, userID, sessionID, id int64, clock func() time.Time, event AuthenticationEventParams) error {
	tx, err := r.database.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin token revocation: %w", err)
	}
	defer rollbackTx(ctx, tx)
	now, err := tokenAdmissionTime(ctx, tx, clock)
	if err != nil {
		return err
	}
	event.OccurredAt = now
	if err := tokenManagementSession(ctx, tx, userID, sessionID, now, ""); err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, `UPDATE personal_access_tokens SET revoked_at = ? WHERE id = ? AND user_id = ? AND revoked_at IS NULL`, now, id, userID)
	if err != nil {
		return fmt.Errorf("revoke personal access token: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return ErrNotFound
	}
	if err := insertAuthenticationEvent(ctx, tx, event); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit token revocation: %w", err)
	}
	return nil
}
