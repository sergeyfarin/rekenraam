package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"rekenraam/backend/internal/db"
)

var ErrRecoveryOwnerNotFound = errors.New("recovery owner not found")

type RecoveryService struct {
	database    *sql.DB
	databaseURL string
	repository  *db.RecoveryRepository
	now         func() time.Time
}

type RecoverOwnerInput struct {
	Password      string
	BackupPath    string
	AllowNoBackup bool
}

type RecoverOwnerResult struct {
	BackupPath string
}

func NewRecoveryService(database *sql.DB, databaseURL string, repository *db.RecoveryRepository) *RecoveryService {
	return &RecoveryService{
		database:    database,
		databaseURL: databaseURL,
		repository:  repository,
		now:         time.Now,
	}
}

func (s *RecoveryService) RecoverOwnerAccess(ctx context.Context, input RecoverOwnerInput) (RecoverOwnerResult, error) {
	if input.Password == "" {
		return RecoverOwnerResult{}, ValidationError{Message: "password is required"}
	}

	backupPath := strings.TrimSpace(input.BackupPath)
	if !input.AllowNoBackup {
		var err error
		if backupPath == "" {
			backupPath, err = defaultRecoveryBackupPath(s.databaseURL, s.now().UTC())
			if err != nil {
				return RecoverOwnerResult{}, fmt.Errorf("build recovery backup path: %w", err)
			}
		}

		if err := db.BackupSQLiteDatabase(ctx, s.database, backupPath); err != nil {
			return RecoverOwnerResult{}, fmt.Errorf("create recovery backup: %w", err)
		}
		if err := db.VerifySQLiteBackup(ctx, backupPath); err != nil {
			return RecoverOwnerResult{}, fmt.Errorf("verify recovery backup: %w", err)
		}
	}

	passwordHash, err := hashPassword(input.Password)
	if err != nil {
		return RecoverOwnerResult{}, fmt.Errorf("hash recovery password: %w", err)
	}

	now := s.now().UTC().Format(time.RFC3339)
	if err := s.repository.ResetOwnerPasswordAndRevokeSessions(ctx, db.ResetOwnerAccessParams{
		PasswordHash: passwordHash,
		UpdatedAt:    now,
		RevokedAt:    now,
	}); err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return RecoverOwnerResult{}, ErrRecoveryOwnerNotFound
		}

		return RecoverOwnerResult{}, fmt.Errorf("reset owner access: %w", err)
	}

	return RecoverOwnerResult{BackupPath: backupPath}, nil
}

func defaultRecoveryBackupPath(databaseURL string, now time.Time) (string, error) {
	databasePath, err := db.ResolveSQLiteFilePath(databaseURL)
	if err != nil {
		return "", err
	}

	ext := filepath.Ext(databasePath)
	baseName := strings.TrimSuffix(filepath.Base(databasePath), ext)
	if ext == "" {
		ext = ".sqlite"
	}

	backupFileName := fmt.Sprintf("%s.recovery-%s%s", baseName, now.Format("20060102T150405Z"), ext)
	return filepath.Join(filepath.Dir(databasePath), backupFileName), nil
}