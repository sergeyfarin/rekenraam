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

	return database, nil
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

	if strings.HasPrefix(databaseURL, "file:") {
		trimmed := strings.TrimPrefix(databaseURL, "file:")
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

	if err := os.MkdirAll(filepath.Dir(backupPath), 0o755); err != nil {
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

	return nil
}

func VerifySQLiteBackup(ctx context.Context, backupPath string) error {
	backupPath = filepath.Clean(strings.TrimSpace(backupPath))
	if backupPath == "" {
		return fmt.Errorf("backup path is required")
	}

	backupDatabase, err := sql.Open(driverName, "file:"+backupPath+"?mode=ro")
	if err != nil {
		return fmt.Errorf("open sqlite backup: %w", err)
	}
	defer backupDatabase.Close()

	if err := backupDatabase.PingContext(ctx); err != nil {
		return fmt.Errorf("ping sqlite backup: %w", err)
	}

	var integrityResult string
	if err := backupDatabase.QueryRowContext(ctx, "PRAGMA integrity_check").Scan(&integrityResult); err != nil {
		return fmt.Errorf("run sqlite integrity_check: %w", err)
	}
	if !strings.EqualFold(integrityResult, "ok") {
		return fmt.Errorf("sqlite integrity_check returned %q", integrityResult)
	}

	return nil
}

func sqliteStringLiteral(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}
