package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

type SettingsRepository struct {
	database *sql.DB
}

type UserPreferencesRecord struct {
	ID        int64
	UserID    int64
	TimeZone  string
	CreatedAt string
	UpdatedAt string
}

type SaveUserPreferencesParams struct {
	UserID        int64
	AuthSessionID int64
	RequestID     string
	OriginType    string
	Operation     string
	TimeZone      string
	RecordedAt    string
	ChangeReason  string
}

func NewSettingsRepository(database *sql.DB) *SettingsRepository {
	return &SettingsRepository{database: database}
}

func (r *SettingsRepository) UserPreferences(ctx context.Context, userID int64) (UserPreferencesRecord, error) {
	var record UserPreferencesRecord
	if err := r.database.QueryRowContext(ctx, `
		SELECT id, user_id, time_zone, created_at, updated_at
		FROM user_preferences
		WHERE user_id = ?
	`, userID).Scan(&record.ID, &record.UserID, &record.TimeZone, &record.CreatedAt, &record.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return UserPreferencesRecord{}, ErrNotFound
		}
		return UserPreferencesRecord{}, fmt.Errorf("read user preferences: %w", err)
	}
	return record, nil
}

func (r *SettingsRepository) SaveUserPreferences(ctx context.Context, params SaveUserPreferencesParams) (UserPreferencesRecord, error) {
	tx, err := r.database.BeginTx(ctx, nil)
	if err != nil {
		return UserPreferencesRecord{}, fmt.Errorf("begin save user preferences: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			rollbackTx(ctx, tx)
		}
	}()

	auditEventID, err := insertAuditEvent(ctx, tx, AuditEventParams{
		ActorUserID:   params.UserID,
		AuthSessionID: params.AuthSessionID,
		OccurredAt:    params.RecordedAt,
		RequestID:     params.RequestID,
		OriginType:    params.OriginType,
		Operation:     params.Operation,
		Reason:        params.ChangeReason,
	})
	if err != nil {
		return UserPreferencesRecord{}, err
	}

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO user_preferences (
			user_id, time_zone, created_at, created_by_user_id, updated_at, updated_by_user_id,
			created_audit_event_id, updated_audit_event_id
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(user_id) DO UPDATE SET
			time_zone = excluded.time_zone,
			updated_at = excluded.updated_at,
			updated_by_user_id = excluded.updated_by_user_id,
			updated_audit_event_id = excluded.updated_audit_event_id
	`, params.UserID, params.TimeZone, params.RecordedAt, params.UserID, params.RecordedAt, params.UserID, auditEventID, auditEventID); err != nil {
		return UserPreferencesRecord{}, fmt.Errorf("upsert user preferences: %w", err)
	}

	record, err := userPreferencesByUserIDTx(ctx, tx, params.UserID)
	if err != nil {
		return UserPreferencesRecord{}, err
	}
	if err := tx.Commit(); err != nil {
		return UserPreferencesRecord{}, fmt.Errorf("commit save user preferences: %w", err)
	}
	committed = true
	return record, nil
}

func userPreferencesByUserIDTx(ctx context.Context, tx *sql.Tx, userID int64) (UserPreferencesRecord, error) {
	var record UserPreferencesRecord
	if err := tx.QueryRowContext(ctx, `
		SELECT id, user_id, time_zone, created_at, updated_at
		FROM user_preferences
		WHERE user_id = ?
	`, userID).Scan(&record.ID, &record.UserID, &record.TimeZone, &record.CreatedAt, &record.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return UserPreferencesRecord{}, ErrNotFound
		}
		return UserPreferencesRecord{}, fmt.Errorf("read user preferences: %w", err)
	}
	return record, nil
}
