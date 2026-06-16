package app

import (
	"context"
	"errors"
	"fmt"
	"time"

	"rekenraam/backend/internal/db"
)

type SettingsService struct {
	repository *db.SettingsRepository
	now        func() time.Time
}

type UserPreferences struct {
	ID        int64
	UserID    int64
	TimeZone  string
	CreatedAt string
	UpdatedAt string
}

type SaveUserPreferencesInput struct {
	UserID        int64
	AuthSessionID int64
	RequestID     string
	TimeZone      string
	ChangeReason  string
}

func NewSettingsService(repository *db.SettingsRepository) *SettingsService {
	return &SettingsService{repository: repository, now: time.Now}
}

func (s *SettingsService) Preferences(ctx context.Context, userID int64) (UserPreferences, error) {
	record, err := s.repository.UserPreferences(ctx, userID)
	if err == nil {
		return toUserPreferences(record), nil
	}
	if !errors.Is(err, db.ErrNotFound) {
		return UserPreferences{}, fmt.Errorf("read user preferences: %w", err)
	}
	return UserPreferences{UserID: userID, TimeZone: "UTC"}, nil
}

func (s *SettingsService) SavePreferences(ctx context.Context, input SaveUserPreferencesInput) (UserPreferences, error) {
	timeZone, err := cleanTimeZone(input.TimeZone)
	if err != nil {
		return UserPreferences{}, err
	}
	recordedAt := s.now().UTC().Format(time.RFC3339)
	record, err := s.repository.SaveUserPreferences(ctx, db.SaveUserPreferencesParams{
		UserID:        input.UserID,
		AuthSessionID: input.AuthSessionID,
		RequestID:     input.RequestID,
		OriginType:    "browser_api",
		Operation:     "settings.preferences.update",
		TimeZone:      timeZone,
		RecordedAt:    recordedAt,
		ChangeReason:  input.ChangeReason,
	})
	if err != nil {
		return UserPreferences{}, fmt.Errorf("save user preferences: %w", err)
	}
	return toUserPreferences(record), nil
}

func toUserPreferences(record db.UserPreferencesRecord) UserPreferences {
	return UserPreferences{
		ID:        record.ID,
		UserID:    record.UserID,
		TimeZone:  record.TimeZone,
		CreatedAt: record.CreatedAt,
		UpdatedAt: record.UpdatedAt,
	}
}
