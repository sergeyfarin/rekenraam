package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rekenraam/backend/internal/db"
	"rekenraam/backend/internal/lockfile"
)

// backupOf makes a verified backup of a fresh migrated database and returns
// both paths.
func backupOf(t *testing.T) (databaseURL string, backupPath string) {
	t.Helper()

	ctx := context.Background()
	root := t.TempDir()
	databaseURL = "file:" + filepath.Join(root, "rekenraam.sqlite")

	database, err := db.Open(ctx, databaseURL)
	require.NoError(t, err)
	require.NoError(t, db.Migrate(ctx, database))

	readOnly, err := db.OpenReadOnly(ctx, databaseURL)
	require.NoError(t, err)
	backupPath = filepath.Join(root, "backups", "rekenraam-2026-08-24.sqlite")
	_, err = db.OnlineBackupSQLiteDatabase(ctx, readOnly, backupPath, db.OnlineBackupOptions{})
	require.NoError(t, err)
	require.NoError(t, readOnly.Close())
	require.NoError(t, database.Close())

	return databaseURL, backupPath
}

// verify-backup answers what a person asks before trusting a file, including
// the question a restore only raises once it is too late.
func TestVerifyBackupCommandReportsIntegritySchemaAndSealedData(t *testing.T) {
	databaseURL, backupPath := backupOf(t)
	t.Setenv("DATABASE_URL", databaseURL)
	t.Setenv("APP_ENV", "development")

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	exitCode := run(context.Background(), []string{"verify-backup", "--from", backupPath}, strings.NewReader(""), stdout, stderr)

	require.Equalf(t, 0, exitCode, "stderr: %s", stderr.String())
	output := stdout.String()
	assert.Contains(t, output, "integrity:       ok")
	assert.Contains(t, output, "schema version:")
	assert.Contains(t, output, "rows transactions:")
	assert.Contains(t, output, "sealed data:     none",
		"a book with no sealed rows should say so rather than warn about a key it does not need")
}

func TestVerifyBackupCommandRejectsAnUnusableFile(t *testing.T) {
	root := t.TempDir()
	notABackup := filepath.Join(root, "notes.txt")
	require.NoError(t, os.WriteFile(notABackup, []byte("this is not a database"), 0o600))
	t.Setenv("DATABASE_URL", "file:"+filepath.Join(root, "rekenraam.sqlite"))
	t.Setenv("APP_ENV", "development")

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	exitCode := run(context.Background(), []string{"verify-backup", "--from", notABackup}, strings.NewReader(""), stdout, stderr)

	assert.Equal(t, 1, exitCode)
	assert.Contains(t, stderr.String(), "backup is not usable")
}

// The restore refuses while a server holds the lock, and says who holds it.
func TestRestoreCommandRefusesWhileTheServerIsRunning(t *testing.T) {
	databaseURL, backupPath := backupOf(t)
	databasePath, err := db.ResolveSQLiteFilePath(databaseURL)
	require.NoError(t, err)
	t.Setenv("DATABASE_URL", databaseURL)
	t.Setenv("APP_ENV", "development")

	held, err := lockfile.Acquire(databasePath)
	require.NoError(t, err)
	defer held.Close()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	exitCode := run(context.Background(), []string{"restore", "--from", backupPath}, strings.NewReader(""), stdout, stderr)

	assert.Equal(t, 1, exitCode)
	assert.Contains(t, stderr.String(), "the server is still running")
	assert.Contains(t, stderr.String(), "Stop it before restoring")
}

// With the server stopped, the restore installs the backup, keeps the previous
// database whole, and says both things plainly.
func TestRestoreCommandInstallsAndPreservesThePreviousDatabase(t *testing.T) {
	databaseURL, backupPath := backupOf(t)
	databasePath, err := db.ResolveSQLiteFilePath(databaseURL)
	require.NoError(t, err)
	t.Setenv("DATABASE_URL", databaseURL)
	t.Setenv("APP_ENV", "development")

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	exitCode := run(context.Background(), []string{"restore", "--from", backupPath}, strings.NewReader(""), stdout, stderr)

	require.Equalf(t, 0, exitCode, "stderr: %s", stderr.String())
	output := stdout.String()
	assert.Contains(t, output, "restored:")
	assert.Contains(t, output, "previous data:")
	assert.Contains(t, output, "attachments:     nothing to restore")
	assert.Contains(t, output, "secret key:")

	assert.FileExists(t, databasePath)
	entries, err := os.ReadDir(filepath.Dir(databasePath))
	require.NoError(t, err)
	var preserved int
	for _, entry := range entries {
		if strings.Contains(entry.Name(), ".before-restore-") {
			preserved++
		}
	}
	assert.Equal(t, 1, preserved, "the previous database is kept until a human deletes it")

	// The lock is free afterwards, so the server can start again.
	assert.NoError(t, lockfile.CheckAvailable(databasePath))
}

func TestRestoreCommandRequiresAFromFlag(t *testing.T) {
	databaseURL, _ := backupOf(t)
	t.Setenv("DATABASE_URL", databaseURL)
	t.Setenv("APP_ENV", "development")

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	exitCode := run(context.Background(), []string{"restore"}, strings.NewReader(""), stdout, stderr)

	assert.Equal(t, 2, exitCode)
	assert.Contains(t, stderr.String(), "restore requires --from")
}
