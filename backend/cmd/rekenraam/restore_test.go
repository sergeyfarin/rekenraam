package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rekenraam/backend/internal/db"
	"rekenraam/backend/internal/lockfile"
	"rekenraam/backend/internal/secretbox"
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

// --- T-68: the sealed-data report, through the command an operator runs ---
//
// The branch that matters is the one where a key is configured and a sample
// either opens or does not — the question a restore otherwise raises far too
// late. It was covered at service level and never through verify-backup itself.

// sealedBackupOf returns a backup containing one sealed row, plus the key it
// was sealed with.
func sealedBackupOf(t *testing.T) (databaseURL string, backupPath string, key string) {
	t.Helper()

	ctx := context.Background()
	root := t.TempDir()
	databaseURL = "file:" + filepath.Join(root, "rekenraam.sqlite")

	database, err := db.Open(ctx, databaseURL)
	require.NoError(t, err)
	require.NoError(t, db.Migrate(ctx, database))

	key = "0123456789abcdef0123456789abcdef"
	box, err := secretbox.New([]byte(key))
	require.NoError(t, err)
	sealed, err := box.Seal([]byte("JBSWY3DPEHPK3PXP"))
	require.NoError(t, err)

	_, err = database.ExecContext(ctx, `
		INSERT INTO users (id, username, password_hash, is_owner, created_at, updated_at)
		VALUES (1, 'owner', 'x', 1, '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z');
		INSERT INTO user_mfa_totp (user_id, secret_ciphertext, status, created_at)
		VALUES (1, ?, 'active', '2026-01-01T00:00:00Z');
	`, sealed)
	require.NoError(t, err)

	readOnly, err := db.OpenReadOnly(ctx, databaseURL)
	require.NoError(t, err)
	backupPath = filepath.Join(root, "backups", "rekenraam-2026-08-24.sqlite")
	_, err = db.OnlineBackupSQLiteDatabase(ctx, readOnly, backupPath, db.OnlineBackupOptions{})
	require.NoError(t, err)
	require.NoError(t, readOnly.Close())
	require.NoError(t, database.Close())

	return databaseURL, backupPath, key
}

func TestVerifyBackupReportsSealedDataOpeningWithTheRetainedKey(t *testing.T) {
	databaseURL, backupPath, key := sealedBackupOf(t)
	t.Setenv("DATABASE_URL", databaseURL)
	t.Setenv("APP_ENV", "development")
	t.Setenv("REKENRAAM_SECRET_KEY", base64.StdEncoding.EncodeToString([]byte(key)))

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	exitCode := run(context.Background(), []string{"verify-backup", "--from", backupPath}, strings.NewReader(""), stdout, stderr)

	require.Equalf(t, 0, exitCode, "stderr: %s", stderr.String())
	output := stdout.String()
	assert.Contains(t, output, "sealed rows user_mfa_totp:")
	assert.Contains(t, output, "decrypts with the configured REKENRAAM_SECRET_KEY")
}

// The case that matters most: the operator kept a key, but not *the* key.
// Silence here would be the worst possible answer.
func TestVerifyBackupSaysSoWhenTheConfiguredKeyIsTheWrongOne(t *testing.T) {
	databaseURL, backupPath, _ := sealedBackupOf(t)
	t.Setenv("DATABASE_URL", databaseURL)
	t.Setenv("APP_ENV", "development")
	t.Setenv("REKENRAAM_SECRET_KEY",
		base64.StdEncoding.EncodeToString([]byte("fedcba9876543210fedcba9876543210")))

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	run(context.Background(), []string{"verify-backup", "--from", backupPath}, strings.NewReader(""), stdout, stderr)

	assert.Contains(t, stdout.String(), "does NOT decrypt with the configured key",
		"a key that cannot open the data must be reported, not passed over")
}

// No key configured: the report says what a restore would cost rather than
// staying quiet about it.
func TestVerifyBackupExplainsTheCostWhenNoKeyIsConfigured(t *testing.T) {
	databaseURL, backupPath, _ := sealedBackupOf(t)
	t.Setenv("DATABASE_URL", databaseURL)
	t.Setenv("APP_ENV", "development")
	t.Setenv("REKENRAAM_SECRET_KEY", "")

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	run(context.Background(), []string{"verify-backup", "--from", backupPath}, strings.NewReader(""), stdout, stderr)

	output := stdout.String()
	assert.Contains(t, output, "is not set here")
	assert.Contains(t, output, "would have to be set up again")
}

// A restore that fails after moving the previous database aside leaves nothing
// at the database path. What the operator is told at that moment decides
// whether their data is recoverable or merely present somewhere they will not
// look, so the message has to state which of the three states this is.
func TestRestoreCommandSaysWhereTheDataIsWhenItFailsPartWay(t *testing.T) {
	databaseURL, backupPath := backupOf(t)
	databasePath, err := db.ResolveSQLiteFilePath(databaseURL)
	require.NoError(t, err)
	t.Setenv("DATABASE_URL", databaseURL)
	t.Setenv("APP_ENV", "development")

	// A directory where the install wants to write its temporary file: the copy
	// fails, and it fails *after* the preserve step, which is the state worth
	// describing.
	require.NoError(t, os.MkdirAll(databasePath+".restoring", 0o700))

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	exitCode := run(context.Background(), []string{"restore", "--from", backupPath}, strings.NewReader(""), stdout, stderr)

	require.Equal(t, 1, exitCode)
	output := stderr.String()
	assert.Contains(t, output, "restore failed:")
	assert.Contains(t, output, "nothing is at "+databasePath)
	assert.Contains(t, output, "move those files beside the database path")
	assert.NotContains(t, output, "nothing was replaced")

	// And the claim is true: the files really are in the preserved directory.
	entries, err := os.ReadDir(filepath.Dir(databasePath))
	require.NoError(t, err)
	var preservedDir string
	for _, entry := range entries {
		if strings.Contains(entry.Name(), ".before-restore-") {
			preservedDir = filepath.Join(filepath.Dir(databasePath), entry.Name())
		}
	}
	require.NotEmpty(t, preservedDir, "the previous database must be somewhere")
	preserved, err := os.ReadDir(preservedDir)
	require.NoError(t, err)
	assert.NotEmpty(t, preserved)
	assert.NoFileExists(t, databasePath)
}

// The other side of the same decision: a failure before anything moved must not
// send the operator looking for a preserved copy that does not exist.
func TestRestoreCommandSaysNothingWasReplacedWhenItRefusesEarly(t *testing.T) {
	databaseURL, _ := backupOf(t)
	t.Setenv("DATABASE_URL", databaseURL)
	t.Setenv("APP_ENV", "development")

	notABackup := filepath.Join(t.TempDir(), "not-a-database.sqlite")
	require.NoError(t, os.WriteFile(notABackup, []byte("this is not a database"), 0o600))

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	exitCode := run(context.Background(), []string{"restore", "--from", notABackup}, strings.NewReader(""), stdout, stderr)

	require.Equal(t, 1, exitCode)
	assert.Contains(t, stderr.String(), "nothing was replaced; the database is untouched.")
}
