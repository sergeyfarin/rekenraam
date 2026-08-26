package db

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite"

	"rekenraam/backend/migrations"
)

// rollbackTx rolls back tx and logs any rollback error. It is intended for use in
// deferred cleanup when a transaction has not been committed. A rollback failure is
// non-fatal (SQLite WAL self-recovers) but must not be silently discarded.
func rollbackTx(ctx context.Context, tx *sql.Tx) {
	if err := tx.Rollback(); err != nil && err != sql.ErrTxDone {
		slog.Default().ErrorContext(ctx, "db: rollback failed", slog.Any("err", err))
	}
}

const (
	DefaultSQLiteURL = "file:var/dev.sqlite"

	driverName            = "sqlite"
	expectedBusyTimeout   = 5000
	expectedJournalMode   = "wal"
	expectedSynchronous   = 1
	expectedWALCheckpoint = 1000
)

var requiredPragmas = []string{
	"busy_timeout(5000)",
	"foreign_keys(1)",
	"journal_mode(WAL)",
	"synchronous(NORMAL)",
	"wal_autocheckpoint(1000)",
}

// readOnlyPragmas are the subset a read-only connection can set. Journal mode,
// synchronous, and autocheckpoint are database-level settings owned by the
// writer; query_only is added so a bug on this pool fails loudly instead of
// writing.
var readOnlyPragmas = []string{
	"busy_timeout(5000)",
	"foreign_keys(1)",
	"query_only(1)",
}

type SQLiteState struct {
	ForeignKeys       int
	BusyTimeout       int
	JournalMode       string
	Synchronous       int
	WALAutoCheckpoint int
}

func Open(ctx context.Context, databaseURL string) (*sql.DB, error) {
	if databaseURL == "" {
		databaseURL = DefaultSQLiteURL
	}

	database, err := sql.Open(driverName, withRequiredPragmas(databaseURL))
	if err != nil {
		return nil, fmt.Errorf("open sqlite database: %w", err)
	}

	// Single connection is safe here: SQLite does not support concurrent writers,
	// and the background FX worker (pricing_worker.go) intentionally performs all
	// HTTP fetches before opening any DB transaction (see pricing_refresh.go), so
	// no transaction is ever held across a network call. User requests serialize
	// behind worker writes; busy_timeout=5000 handles the wait.
	database.SetMaxOpenConns(1)
	database.SetMaxIdleConns(1)

	if err := database.PingContext(ctx); err != nil {
		_ = database.Close()
		return nil, fmt.Errorf("ping sqlite database: %w", err)
	}

	if _, err := Check(ctx, database); err != nil {
		_ = database.Close()
		return nil, err
	}
	if err := EnforceSQLiteFilePermissions(databaseURL); err != nil {
		_ = database.Close()
		return nil, err
	}

	return database, nil
}

// OpenReadOnly opens a second pool over the same database file for long reads —
// exports and the ledger self-check — that must not hold the writer.
//
// The main pool is SetMaxOpenConns(1) (see Open), so a read transaction held
// there for the length of an export would stall every other request. WAL
// readers run concurrently with the single writer by design, so a separate
// read-only pool adds no write contention: it is the standard SQLite shape for
// this, not a workaround.
//
// The database must already exist and be migrated — mode=ro will not create it,
// which is deliberate: this pool is never the one that brings a book into being.
func OpenReadOnly(ctx context.Context, databaseURL string) (*sql.DB, error) {
	databasePath, err := ResolveSQLiteFilePath(databaseURL)
	if err != nil {
		return nil, err
	}

	parts := make([]string, 0, len(readOnlyPragmas)+1)
	parts = append(parts, "mode=ro")
	for _, pragma := range readOnlyPragmas {
		parts = append(parts, "_pragma="+pragma)
	}

	database, err := sql.Open(driverName, "file:"+databasePath+"?"+strings.Join(parts, "&"))
	if err != nil {
		return nil, fmt.Errorf("open read-only sqlite database: %w", err)
	}

	// Small but not single: concurrent reads are safe, and a stuck export
	// must not block the self-check or a second reader.
	database.SetMaxOpenConns(4)
	database.SetMaxIdleConns(2)

	if err := database.PingContext(ctx); err != nil {
		_ = database.Close()
		return nil, fmt.Errorf("ping read-only sqlite database: %w", err)
	}

	var queryOnly int
	if err := database.QueryRowContext(ctx, "PRAGMA query_only").Scan(&queryOnly); err != nil {
		_ = database.Close()
		return nil, fmt.Errorf("read PRAGMA query_only: %w", err)
	}
	if queryOnly != 1 {
		_ = database.Close()
		return nil, fmt.Errorf("sqlite query_only pragma is %d, want 1", queryOnly)
	}

	return database, nil
}

// EnforceSQLiteFilePermissions restricts the database and any WAL sidecars to
// the process owner. SQLite inherits the process umask when it creates these
// files, so this explicit correction is required for financial data on shared
// hosts. Memory databases have no filesystem paths and are intentionally skipped.
func EnforceSQLiteFilePermissions(databaseURL string) error {
	databasePath, err := ResolveSQLiteFilePath(databaseURL)
	if err != nil {
		return err
	}
	if databasePath == ":memory:" || strings.HasPrefix(databasePath, "file::memory:") {
		return nil
	}

	for _, path := range []string{databasePath, databasePath + "-wal", databasePath + "-shm"} {
		if err := os.Chmod(path, 0o600); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return fmt.Errorf("set sqlite file permissions for %s: %w", path, err)
		}
	}

	return nil
}

func Migrate(ctx context.Context, database *sql.DB) error {
	provider, err := goose.NewProvider(goose.DialectSQLite3, database, migrations.FS)
	if err != nil {
		return fmt.Errorf("create sqlite migration provider: %w", err)
	}
	if _, err := provider.Up(ctx); err != nil {
		return fmt.Errorf("run sqlite migrations: %w", err)
	}
	return nil
}

func Check(ctx context.Context, database *sql.DB) (SQLiteState, error) {
	var state SQLiteState

	if err := database.QueryRowContext(ctx, "PRAGMA foreign_keys").Scan(&state.ForeignKeys); err != nil {
		return state, fmt.Errorf("read PRAGMA foreign_keys: %w", err)
	}
	if err := database.QueryRowContext(ctx, "PRAGMA busy_timeout").Scan(&state.BusyTimeout); err != nil {
		return state, fmt.Errorf("read PRAGMA busy_timeout: %w", err)
	}
	if err := database.QueryRowContext(ctx, "PRAGMA journal_mode").Scan(&state.JournalMode); err != nil {
		return state, fmt.Errorf("read PRAGMA journal_mode: %w", err)
	}
	if err := database.QueryRowContext(ctx, "PRAGMA synchronous").Scan(&state.Synchronous); err != nil {
		return state, fmt.Errorf("read PRAGMA synchronous: %w", err)
	}
	if err := database.QueryRowContext(ctx, "PRAGMA wal_autocheckpoint").Scan(&state.WALAutoCheckpoint); err != nil {
		return state, fmt.Errorf("read PRAGMA wal_autocheckpoint: %w", err)
	}

	if state.ForeignKeys != 1 {
		return state, fmt.Errorf("sqlite foreign_keys pragma is %d, want 1", state.ForeignKeys)
	}
	if state.BusyTimeout != expectedBusyTimeout {
		return state, fmt.Errorf("sqlite busy_timeout pragma is %d, want %d", state.BusyTimeout, expectedBusyTimeout)
	}
	if !strings.EqualFold(state.JournalMode, expectedJournalMode) {
		return state, fmt.Errorf("sqlite journal_mode pragma is %q, want %q", state.JournalMode, expectedJournalMode)
	}
	if state.Synchronous != expectedSynchronous {
		return state, fmt.Errorf("sqlite synchronous pragma is %d, want %d", state.Synchronous, expectedSynchronous)
	}
	if state.WALAutoCheckpoint != expectedWALCheckpoint {
		return state, fmt.Errorf("sqlite wal_autocheckpoint pragma is %d, want %d", state.WALAutoCheckpoint, expectedWALCheckpoint)
	}

	return state, nil
}

func withRequiredPragmas(databaseURL string) string {
	separator := "?"
	if strings.Contains(databaseURL, "?") {
		separator = "&"
	}

	parts := make([]string, 0, len(requiredPragmas))
	for _, pragma := range requiredPragmas {
		parts = append(parts, "_pragma="+pragma)
	}

	return databaseURL + separator + strings.Join(parts, "&")
}

func ResolveSQLiteFilePath(databaseURL string) (string, error) {
	if databaseURL == "" {
		databaseURL = DefaultSQLiteURL
	}

	parsedURL, err := url.Parse(databaseURL)
	if err == nil && parsedURL.Scheme == "file" {
		switch {
		case parsedURL.Opaque != "":
			return parsedURL.Opaque, nil
		case parsedURL.Host != "":
			return "//" + parsedURL.Host + parsedURL.Path, nil
		case parsedURL.Path != "":
			return parsedURL.Path, nil
		}
	}

	if trimmed, ok := strings.CutPrefix(databaseURL, "file:"); ok {
		if idx := strings.Index(trimmed, "?"); idx >= 0 {
			trimmed = trimmed[:idx]
		}
		if trimmed != "" {
			return trimmed, nil
		}
	}

	return "", fmt.Errorf("sqlite backup requires a file database URL")
}

func BackupSQLiteDatabase(ctx context.Context, database *sql.DB, backupPath string) error {
	backupPath = filepath.Clean(strings.TrimSpace(backupPath))
	if backupPath == "" {
		return fmt.Errorf("backup path is required")
	}

	// 0700, not 0755: the backup is a full copy of the ledger, and
	// EnforceSQLiteFilePermissions below pins the file itself to 0600. A
	// world-readable directory would leave the containing path more permissive
	// than everything in it. The container image assumes the same — see
	// deploy/docker/Dockerfile, which creates /app/data as 0700.
	if err := os.MkdirAll(filepath.Dir(backupPath), 0o700); err != nil {
		return fmt.Errorf("create backup directory: %w", err)
	}
	if _, err := os.Stat(backupPath); err == nil {
		return fmt.Errorf("backup path already exists")
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("stat backup path: %w", err)
	}

	statement := fmt.Sprintf("VACUUM INTO %s", sqliteStringLiteral(backupPath))
	if _, err := database.ExecContext(ctx, statement); err != nil {
		return fmt.Errorf("vacuum into backup: %w", err)
	}
	if err := EnforceSQLiteFilePermissions("file:" + backupPath); err != nil {
		return fmt.Errorf("set sqlite backup permissions: %w", err)
	}

	return nil
}

// VerifySQLiteBackup reports whether a copy is a usable database. It is the
// error-only form of VerifySQLiteBackupStats, for callers with nothing to
// record.
func VerifySQLiteBackup(ctx context.Context, backupPath string) error {
	_, err := VerifySQLiteBackupStats(ctx, backupPath)
	return err
}

// VerifySQLiteBackupStats verifies a copy and reports its size in pages and
// bytes.
//
// The page count is read from the file rather than taken from whatever produced
// it, so a backup this process *adopted* — one a crash left correctly in place
// between the rename and the run being recorded — is described by the same
// number a backup this process copied would carry. The online backup API's page
// count is the source's page count at the end of the copy, which is the
// destination's, so the two paths agree by construction rather than by
// coincidence. Returning zero there instead recorded a completed, verified
// backup that claimed to hold no pages at all.
func VerifySQLiteBackupStats(ctx context.Context, backupPath string) (OnlineBackupResult, error) {
	backupPath = filepath.Clean(strings.TrimSpace(backupPath))
	if backupPath == "" {
		return OnlineBackupResult{}, fmt.Errorf("backup path is required")
	}

	backupDatabase, err := sql.Open(driverName, "file:"+backupPath+"?mode=ro")
	if err != nil {
		return OnlineBackupResult{}, fmt.Errorf("open sqlite backup: %w", err)
	}
	defer backupDatabase.Close()

	if err := backupDatabase.PingContext(ctx); err != nil {
		return OnlineBackupResult{}, fmt.Errorf("ping sqlite backup: %w", err)
	}

	var integrityResult string
	if err := backupDatabase.QueryRowContext(ctx, "PRAGMA integrity_check").Scan(&integrityResult); err != nil {
		return OnlineBackupResult{}, fmt.Errorf("run sqlite integrity_check: %w", err)
	}
	if !strings.EqualFold(integrityResult, "ok") {
		return OnlineBackupResult{}, fmt.Errorf("sqlite integrity_check returned %q", integrityResult)
	}

	foreignKeyRows, err := backupDatabase.QueryContext(ctx, "PRAGMA foreign_key_check")
	if err != nil {
		return OnlineBackupResult{}, fmt.Errorf("run sqlite foreign_key_check: %w", err)
	}
	defer foreignKeyRows.Close()
	if foreignKeyRows.Next() {
		var table string
		var rowID int64
		var parentTable string
		var foreignKeyID int64
		if err := foreignKeyRows.Scan(&table, &rowID, &parentTable, &foreignKeyID); err != nil {
			return OnlineBackupResult{}, fmt.Errorf("read sqlite foreign_key_check result: %w", err)
		}
		return OnlineBackupResult{}, fmt.Errorf("sqlite foreign_key_check found violation in %s row %d referencing %s (foreign key %d)", table, rowID, parentTable, foreignKeyID)
	}
	if err := foreignKeyRows.Err(); err != nil {
		return OnlineBackupResult{}, fmt.Errorf("iterate sqlite foreign_key_check: %w", err)
	}

	var pageCount int64
	if err := backupDatabase.QueryRowContext(ctx, "PRAGMA page_count").Scan(&pageCount); err != nil {
		return OnlineBackupResult{}, fmt.Errorf("read sqlite page_count: %w", err)
	}

	info, err := os.Stat(backupPath)
	if err != nil {
		return OnlineBackupResult{}, fmt.Errorf("stat sqlite backup: %w", err)
	}

	return OnlineBackupResult{PageCount: int(pageCount), ByteSize: info.Size()}, nil
}

func sqliteStringLiteral(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}
