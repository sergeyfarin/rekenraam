package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"rekenraam/backend/migrations"
)

// A SQLite database in WAL mode is a *set* of files. Every step here treats it
// as one, because the alternative destroys data: moving only the main file
// aside and deleting -wal discards transactions that were committed and never
// checkpointed, and it leaves the "safety copy" an operator would reach for
// after a bad restore incomplete.

// ErrRestoreSourceIsDestination means the backup and the live database are the
// same file. Continuing would move the database aside and then look for it.
var ErrRestoreSourceIsDestination = errors.New("backup and database are the same file")

// ErrRestoreSchemaNewer means the backup was written by a newer binary than the
// one restoring it, whose migrations this build does not have.
var ErrRestoreSchemaNewer = errors.New("backup schema is newer than this build")

// BackupInspection is what verify-backup reports.
type BackupInspection struct {
	Path            string
	ByteSize        int64
	SchemaVersion   int64
	BinaryVersion   int64
	IntegrityOK     bool
	RowCounts       map[string]int64
	SealedRowCounts map[string]int64
}

// InspectBackup opens a backup read-only and answers the questions a person
// asks before trusting it: is it intact, what schema is it, and how much is in
// it.
func InspectBackup(ctx context.Context, backupPath string) (BackupInspection, error) {
	inspection := BackupInspection{
		Path:            backupPath,
		RowCounts:       map[string]int64{},
		SealedRowCounts: map[string]int64{},
	}

	info, err := os.Stat(backupPath)
	if err != nil {
		return inspection, fmt.Errorf("stat backup: %w", err)
	}
	inspection.ByteSize = info.Size()

	if err := VerifySQLiteBackup(ctx, backupPath); err != nil {
		return inspection, err
	}
	inspection.IntegrityOK = true

	backupDatabase, err := sql.Open(driverName, "file:"+backupPath+"?mode=ro")
	if err != nil {
		return inspection, fmt.Errorf("open backup: %w", err)
	}
	defer backupDatabase.Close()

	if err := backupDatabase.QueryRowContext(ctx, `SELECT COALESCE(MAX(version_id), 0) FROM goose_db_version`).Scan(&inspection.SchemaVersion); err != nil {
		return inspection, fmt.Errorf("read backup schema version: %w", err)
	}
	inspection.BinaryVersion, err = EmbeddedMigrationVersion()
	if err != nil {
		return inspection, err
	}

	// Counts a human can sanity-check against what they remember having.
	for _, table := range []string{"transactions", "posting_versions", "accounts", "commodities", "payees"} {
		var count int64
		if err := backupDatabase.QueryRowContext(ctx, `SELECT COUNT(*) FROM `+table).Scan(&count); err != nil {
			// A table absent from an older schema is not a failure of the
			// backup; it is a fact about it.
			continue
		}
		inspection.RowCounts[table] = count
	}

	// Sealed rows are the ones whose usefulness depends on a key that is not in
	// this file. Counting them is how verify-backup can say what a restore
	// without the key would cost.
	for table, column := range map[string]string{
		"user_mfa_totp":      "secret_ciphertext",
		"import_connections": "secret_ciphertext",
	} {
		var count int64
		if err := backupDatabase.QueryRowContext(ctx, `SELECT COUNT(*) FROM `+table+` WHERE length(trim(`+column+`)) > 0`).Scan(&count); err != nil {
			continue
		}
		inspection.SealedRowCounts[table] = count
	}

	return inspection, nil
}

// SealedSamples returns one sealed ciphertext per table that has any, so a
// caller holding the key can prove the data is still readable rather than
// assume it.
func SealedSamples(ctx context.Context, backupPath string) (map[string]string, error) {
	backupDatabase, err := sql.Open(driverName, "file:"+backupPath+"?mode=ro")
	if err != nil {
		return nil, fmt.Errorf("open backup: %w", err)
	}
	defer backupDatabase.Close()

	samples := map[string]string{}
	for table, column := range map[string]string{
		"user_mfa_totp":      "secret_ciphertext",
		"import_connections": "secret_ciphertext",
	} {
		var ciphertext string
		err := backupDatabase.QueryRowContext(ctx,
			`SELECT `+column+` FROM `+table+` WHERE length(trim(`+column+`)) > 0 LIMIT 1`).Scan(&ciphertext)
		if err != nil {
			continue
		}
		samples[table] = ciphertext
	}

	return samples, nil
}

// EmbeddedMigrationVersion is the highest migration this binary carries.
func EmbeddedMigrationVersion() (int64, error) {
	entries, err := migrations.FS.ReadDir(".")
	if err != nil {
		return 0, fmt.Errorf("read embedded migrations: %w", err)
	}

	var highest int64
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasSuffix(name, ".sql") {
			continue
		}
		var version int64
		if _, err := fmt.Sscanf(name, "%d_", &version); err != nil {
			continue
		}
		if version > highest {
			highest = version
		}
	}

	return highest, nil
}

// RestoreResult records what a restore did, so its output is a report rather
// than a claim.
type RestoreResult struct {
	DatabasePath string
	PreservedDir string
	// PreservedFiles is what was actually moved aside, which is not the same
	// question as whether PreservedDir exists: the directory is created before
	// the first rename, so an empty list means the previous database is still
	// where it was.
	PreservedFiles []string
	// Installed is true once the restored file is under the real name. Between
	// that rename and this function returning there is still work that can
	// fail, and "the restore failed" means something different on either side
	// of it: before, the previous database is only in PreservedDir; after, the
	// restored one is already live.
	Installed     bool
	SchemaVersion int64
}

// RestoreSQLiteDatabase installs a verified backup over the live database.
//
// The caller must already have proved the server is stopped; this function
// cannot, and will not pretend to. What it does guarantee: the previous
// database is preserved *whole* before anything is replaced, the new file is
// durable before it is visible, and a failure part-way leaves the preserved
// copy intact.
func RestoreSQLiteDatabase(ctx context.Context, backupPath string, databaseURL string) (RestoreResult, error) {
	databasePath, err := ResolveSQLiteFilePath(databaseURL)
	if err != nil {
		return RestoreResult{}, err
	}

	backupPath = filepath.Clean(backupPath)
	databasePath = filepath.Clean(databasePath)
	if sameFile(backupPath, databasePath) {
		return RestoreResult{}, ErrRestoreSourceIsDestination
	}

	inspection, err := InspectBackup(ctx, backupPath)
	if err != nil {
		return RestoreResult{}, err
	}
	if inspection.SchemaVersion > inspection.BinaryVersion {
		return RestoreResult{}, fmt.Errorf("%w: backup is at migration %d, this build knows %d",
			ErrRestoreSchemaNewer, inspection.SchemaVersion, inspection.BinaryVersion)
	}

	result := RestoreResult{DatabasePath: databasePath, SchemaVersion: inspection.SchemaVersion}

	// Preserve whatever is there now, as a set. A WAL holds committed
	// transactions that are not in the main file yet, so checkpointing first is
	// what makes the preserved copy self-contained rather than torn.
	if _, err := os.Stat(databasePath); err == nil {
		if err := checkpointStoppedDatabase(ctx, databasePath); err != nil {
			return result, err
		}

		preservedDir, err := createPreservedDir(databasePath)
		if err != nil {
			return result, err
		}
		result.PreservedDir = preservedDir

		for _, suffix := range []string{"", "-wal", "-shm"} {
			source := databasePath + suffix
			if _, err := os.Stat(source); err != nil {
				continue
			}
			destination := filepath.Join(preservedDir, filepath.Base(source))
			if err := os.Rename(source, destination); err != nil {
				return result, fmt.Errorf("preserve %s: %w", filepath.Base(source), err)
			}
			result.PreservedFiles = append(result.PreservedFiles, destination)
		}
		if err := SyncDirectory(preservedDir); err != nil {
			return result, err
		}
	} else if !os.IsNotExist(err) {
		return result, fmt.Errorf("stat database: %w", err)
	}

	// Install atomically: a temp file in the destination directory, made
	// durable, then renamed. A crash mid-copy leaves the temp file and the
	// preserved set, never a half-written database under the real name.
	temporaryPath := databasePath + ".restoring"
	if err := copyFile(backupPath, temporaryPath); err != nil {
		return result, err
	}
	if err := os.Chmod(temporaryPath, 0o600); err != nil {
		return result, fmt.Errorf("set restored permissions: %w", err)
	}
	if err := SyncFileAndParent(temporaryPath); err != nil {
		return result, err
	}
	if err := os.Rename(temporaryPath, databasePath); err != nil {
		return result, fmt.Errorf("install restored database: %w", err)
	}
	result.Installed = true
	if err := SyncDirectory(filepath.Dir(databasePath)); err != nil {
		return result, err
	}

	// Only now: the sidecars of the database that is no longer there. Removing
	// them earlier would have thrown away the WAL this restore preserved.
	for _, suffix := range []string{"-wal", "-shm"} {
		if err := os.Remove(databasePath + suffix); err != nil && !os.IsNotExist(err) {
			return result, fmt.Errorf("remove stale sidecar: %w", err)
		}
	}

	return result, nil
}

// createPreservedDir makes the directory this restore moves the previous
// database into, and never adopts one that already exists.
//
// The name carries a timestamp with one-second resolution, so two restores
// inside the same second compute the same path. MkdirAll accepts an existing
// directory and os.Rename silently replaces its destination, so the second
// restore would move the database *it* is replacing on top of the copy the
// first one preserved — destroying the only remaining copy of the original,
// inside the feature whose whole promise is that it does not. Each generation
// gets its own directory instead.
//
// Failing here is safe: nothing has been moved yet, so the caller reports that
// nothing was replaced and the database is untouched.
func createPreservedDir(databasePath string) (string, error) {
	base := databasePath + ".before-restore-" + time.Now().UTC().Format("20060102T150405Z")
	for attempt := 1; attempt <= 100; attempt++ {
		candidate := base
		if attempt > 1 {
			candidate = fmt.Sprintf("%s-%d", base, attempt)
		}
		err := os.Mkdir(candidate, 0o700)
		if err == nil {
			return candidate, nil
		}
		if !os.IsExist(err) {
			return "", fmt.Errorf("create pre-restore directory: %w", err)
		}
	}
	return "", fmt.Errorf("create pre-restore directory: %s and 99 numbered siblings already exist", base)
}

// checkpointStoppedDatabase folds a WAL back into the main file so the
// preserved copy is complete. It opens the database, which is safe precisely
// because the caller has established that nothing else has it open.
func checkpointStoppedDatabase(ctx context.Context, databasePath string) error {
	if _, err := os.Stat(databasePath + "-wal"); err != nil {
		return nil
	}

	database, err := sql.Open(driverName, "file:"+databasePath+"?_pragma=busy_timeout(5000)")
	if err != nil {
		return fmt.Errorf("open database for checkpoint: %w", err)
	}
	defer database.Close()
	database.SetMaxOpenConns(1)

	var busy, logPages, checkpointed int
	if err := database.QueryRowContext(ctx, "PRAGMA wal_checkpoint(TRUNCATE)").Scan(&busy, &logPages, &checkpointed); err != nil {
		return fmt.Errorf("checkpoint database before restore: %w", err)
	}
	if busy != 0 {
		return fmt.Errorf("database is still in use; stop the server before restoring")
	}
	return database.Close()
}

func copyFile(sourcePath string, destinationPath string) error {
	source, err := os.Open(sourcePath)
	if err != nil {
		return fmt.Errorf("open backup for copy: %w", err)
	}
	defer source.Close()

	destination, err := os.OpenFile(destinationPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("create restored database: %w", err)
	}
	if _, err := io.Copy(destination, source); err != nil {
		destination.Close()
		_ = os.Remove(destinationPath)
		return fmt.Errorf("copy backup: %w", err)
	}
	if err := destination.Close(); err != nil {
		return fmt.Errorf("close restored database: %w", err)
	}
	return nil
}

func sameFile(left string, right string) bool {
	leftInfo, leftErr := os.Stat(left)
	rightInfo, rightErr := os.Stat(right)
	if leftErr != nil || rightErr != nil {
		// Fall back to the paths themselves, resolved as far as they can be:
		// two names for one file must not slip through because one of them
		// does not exist yet.
		leftResolved, err := filepath.EvalSymlinks(left)
		if err != nil {
			leftResolved = left
		}
		rightResolved, err := filepath.EvalSymlinks(right)
		if err != nil {
			rightResolved = right
		}
		return leftResolved == rightResolved
	}
	return os.SameFile(leftInfo, rightInfo)
}
