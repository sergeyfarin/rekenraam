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

func TestRecoverOwnerCommandBacksUpDatabaseBeforeMigration(t *testing.T) {
	databaseURL := setupPreMigrationRecoveryDatabase(t)
	backupPath := filepath.Join(t.TempDir(), "recovery.sqlite")
	t.Setenv("DATABASE_URL", databaseURL)
	t.Setenv("APP_ENV", "development")

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	exitCode := run(context.Background(), []string{"recover-owner", "--password-stdin", "--backup-path", backupPath}, bytes.NewBufferString("new-password\n"), stdout, stderr)

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Contains(t, stdout.String(), backupPath)

	backupDatabase := openSQLiteReadOnly(t, backupPath)
	defer backupDatabase.Close()
	assert.False(t, sqliteTableExists(t, backupDatabase, "setup_steps"))
	assert.False(t, sqliteTableExists(t, backupDatabase, "login_throttles"))

	database := openRecoveryCommandDatabase(t, databaseURL)
	defer database.Close()
	assert.True(t, sqliteTableExists(t, database, "setup_steps"))
	assert.True(t, sqliteTableExists(t, database, "login_throttles"))

	var passwordHash string
	err := database.QueryRowContext(context.Background(), `SELECT password_hash FROM users WHERE username = 'owner'`).Scan(&passwordHash)
	require.NoError(t, err)
	assert.NotEqual(t, "old-password-hash", passwordHash)

	var revokedSessionCount int
	err = database.QueryRowContext(context.Background(), `SELECT COUNT(1) FROM auth_sessions WHERE revoked_at IS NOT NULL`).Scan(&revokedSessionCount)
	require.NoError(t, err)
	assert.Equal(t, 1, revokedSessionCount)
}

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

func setupPreMigrationRecoveryDatabase(t *testing.T) string {
	t.Helper()

	databasePath := filepath.Join(t.TempDir(), "pre-migration.sqlite")
	databaseURL := "file:" + databasePath
	database, err := db.Open(context.Background(), databaseURL)
	require.NoError(t, err)
	defer database.Close()

	_, err = database.ExecContext(context.Background(), `
		CREATE TABLE users (
			id INTEGER PRIMARY KEY,
			username TEXT NOT NULL UNIQUE,
			password_hash TEXT NOT NULL,
			is_owner INTEGER NOT NULL CHECK (is_owner IN (0, 1)),
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		);
		CREATE UNIQUE INDEX users_one_owner_idx
			ON users (is_owner)
			WHERE is_owner = 1;
		CREATE TABLE auth_sessions (
			id INTEGER PRIMARY KEY,
			user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			token_hash TEXT NOT NULL UNIQUE,
			created_at TEXT NOT NULL,
			expires_at TEXT NOT NULL,
			revoked_at TEXT
		);
		INSERT INTO users (id, username, password_hash, is_owner, created_at, updated_at)
		VALUES (1, 'owner', 'old-password-hash', 1, '2026-05-30T00:00:00Z', '2026-05-30T00:00:00Z');
		INSERT INTO auth_sessions (id, user_id, token_hash, created_at, expires_at, revoked_at)
		VALUES (1, 1, 'old-session-hash', '2026-05-30T00:00:00Z', '2026-06-29T00:00:00Z', NULL);
	`)
	require.NoError(t, err)

	return databaseURL
}

func sqliteTableExists(t *testing.T, database *sql.DB, tableName string) bool {
	t.Helper()

	var found int
	err := database.QueryRowContext(context.Background(), `SELECT COUNT(1) FROM sqlite_master WHERE type = 'table' AND name = ?`, tableName).Scan(&found)
	require.NoError(t, err)
	return found == 1
}

func openSQLiteReadOnly(t *testing.T, databasePath string) *sql.DB {
	t.Helper()

	database, err := sql.Open("sqlite", "file:"+databasePath+"?mode=ro")
	require.NoError(t, err)
	require.NoError(t, database.PingContext(context.Background()))
	return database
}
