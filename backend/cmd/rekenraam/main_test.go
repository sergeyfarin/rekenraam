package main

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rekenraam/backend/internal/app"
	"rekenraam/backend/internal/db"
)

func TestRecoverOwnerCommandCreatesBackupAndRevokesSessions(t *testing.T) {
	databaseURL := setupRecoveryCommandDatabase(t, "old-password")
	backupPath := filepath.Join(t.TempDir(), "recovery.sqlite")
	t.Setenv("DATABASE_URL", databaseURL)
	t.Setenv("APP_ENV", "development")

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	exitCode := run(context.Background(), []string{"recover-owner", "--password-stdin", "--backup-path", backupPath}, bytes.NewBufferString("new-password\n"), stdout, stderr)

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Contains(t, stdout.String(), backupPath)

	_, err := os.Stat(backupPath)
	require.NoError(t, err)
	require.NoError(t, db.VerifySQLiteBackup(context.Background(), backupPath))

	database := openRecoveryCommandDatabase(t, databaseURL)
	defer database.Close()

	authService := app.NewAuthService(db.NewAuthRepository(database))
	loginResult, err := authService.Login(context.Background(), app.LoginInput{Username: "owner", Password: "new-password"})
	require.NoError(t, err)
	assert.NotEmpty(t, loginResult.SessionToken)

	_, err = authService.Login(context.Background(), app.LoginInput{Username: "owner", Password: "old-password"})
	require.Error(t, err)
	assert.True(t, errors.Is(err, app.ErrInvalidCredentials))

	var activeSessionCount int
	err = database.QueryRowContext(context.Background(), `SELECT COUNT(1) FROM auth_sessions WHERE revoked_at IS NULL`).Scan(&activeSessionCount)
	require.NoError(t, err)
	assert.Equal(t, 1, activeSessionCount)

	var revokedSessionCount int
	err = database.QueryRowContext(context.Background(), `SELECT COUNT(1) FROM auth_sessions WHERE revoked_at IS NOT NULL`).Scan(&revokedSessionCount)
	require.NoError(t, err)
	assert.Equal(t, 1, revokedSessionCount)
}

func TestRecoverOwnerCommandAbortsWhenBackupFailsWithoutOverride(t *testing.T) {
	databaseURL := setupRecoveryCommandDatabase(t, "old-password")
	backupPath := filepath.Join(t.TempDir(), "existing.sqlite")
	require.NoError(t, os.WriteFile(backupPath, []byte("already exists"), 0o644))
	t.Setenv("DATABASE_URL", databaseURL)
	t.Setenv("APP_ENV", "development")

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	exitCode := run(context.Background(), []string{"recover-owner", "--password-stdin", "--backup-path", backupPath}, bytes.NewBufferString("new-password\n"), stdout, stderr)

	require.Equal(t, 1, exitCode)
	assert.Contains(t, stderr.String(), "backup path already exists")

	database := openRecoveryCommandDatabase(t, databaseURL)
	defer database.Close()

	authService := app.NewAuthService(db.NewAuthRepository(database))
	_, err := authService.Login(context.Background(), app.LoginInput{Username: "owner", Password: "old-password"})
	require.NoError(t, err)

	var revokedSessionCount int
	err = database.QueryRowContext(context.Background(), `SELECT COUNT(1) FROM auth_sessions WHERE revoked_at IS NOT NULL`).Scan(&revokedSessionCount)
	require.NoError(t, err)
	assert.Equal(t, 0, revokedSessionCount)
}

func TestRecoverOwnerCommandAllowsNoBackupOverride(t *testing.T) {
	databaseURL := setupRecoveryCommandDatabase(t, "old-password")
	t.Setenv("DATABASE_URL", databaseURL)
	t.Setenv("APP_ENV", "development")

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	exitCode := run(context.Background(), []string{"recover-owner", "--password-stdin", "--allow-no-backup"}, bytes.NewBufferString("new-password\n"), stdout, stderr)

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Contains(t, stdout.String(), "without backup")

	database := openRecoveryCommandDatabase(t, databaseURL)
	defer database.Close()

	authService := app.NewAuthService(db.NewAuthRepository(database))
	_, err := authService.Login(context.Background(), app.LoginInput{Username: "owner", Password: "new-password"})
	require.NoError(t, err)

	var revokedSessionCount int
	err = database.QueryRowContext(context.Background(), `SELECT COUNT(1) FROM auth_sessions WHERE revoked_at IS NOT NULL`).Scan(&revokedSessionCount)
	require.NoError(t, err)
	assert.Equal(t, 1, revokedSessionCount)
}

func setupRecoveryCommandDatabase(t *testing.T, password string) string {
	t.Helper()

	databasePath := filepath.Join(t.TempDir(), "rekenraam.sqlite")
	databaseURL := "file:" + databasePath
	database := openRecoveryCommandDatabase(t, databaseURL)
	defer database.Close()

	setupService := app.NewSetupService(db.NewSetupRepository(database))
	_, err := setupService.CreateOwner(context.Background(), app.CreateOwnerInput{
		Username: "owner",
		Password: password,
	})
	require.NoError(t, err)

	return databaseURL
}

func openRecoveryCommandDatabase(t *testing.T, databaseURL string) *sql.DB {
	t.Helper()

	database, err := db.Open(context.Background(), databaseURL)
	require.NoError(t, err)
	require.NoError(t, db.Migrate(context.Background(), database))
	return database
}