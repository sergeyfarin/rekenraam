package db

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite"

	"rekenraam/backend/migrations"
)

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
