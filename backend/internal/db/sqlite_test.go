package db

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOpenAppliesRequiredPragmas(t *testing.T) {
	database := openTestDatabase(t)

	state, err := Check(context.Background(), database)
	require.NoError(t, err)

	assert.Equal(t, 1, state.ForeignKeys)
	assert.Equal(t, expectedBusyTimeout, state.BusyTimeout)
	assert.Equal(t, expectedJournalMode, state.JournalMode)
	assert.Equal(t, expectedSynchronous, state.Synchronous)
	assert.Equal(t, expectedWALCheckpoint, state.WALAutoCheckpoint)
	assert.Equal(t, 1, database.Stats().MaxOpenConnections)
}

func TestMigrateAppliesEmbeddedMigrations(t *testing.T) {
	database := openTestDatabase(t)

	require.NoError(t, Migrate(context.Background(), database))
	require.NoError(t, Migrate(context.Background(), database))

	rows, err := database.QueryContext(
		context.Background(),
		"SELECT step_key FROM setup_steps ORDER BY step_key",
	)
	require.NoError(t, err)
	defer rows.Close()

	var stepKeys []string
	for rows.Next() {
		var stepKey string
		require.NoError(t, rows.Scan(&stepKey))
		stepKeys = append(stepKeys, stepKey)
	}
	require.NoError(t, rows.Err())
	assert.Equal(t, []string{"book", "categories", "currencies", "owner", "system_accounts"}, stepKeys)
	assert.Equal(t, []string{"id", "user_id", "token_hash", "created_at", "expires_at", "revoked_at"}, readTableColumns(t, database, "auth_sessions"))
	assert.Equal(t, []string{"scope_type", "scope_key", "failed_attempts", "blocked_until", "updated_at"}, readTableColumns(t, database, "login_throttles"))
	assert.Contains(t, readTableColumns(t, database, "setup_steps"), "completed_audit_event_id")
	assert.Subset(t, readTableColumns(t, database, "audit_events"), []string{
		"id",
		"book_id",
		"actor_user_id",
		"auth_session_id",
		"occurred_at",
		"request_id",
		"origin_type",
		"operation",
		"reason",
		"metadata_json",
	})
	assert.Contains(t, readTableColumns(t, database, "books"), "updated_by_user_id")
	assert.Contains(t, readTableColumns(t, database, "books"), "created_audit_event_id")
	assert.Contains(t, readTableColumns(t, database, "books"), "updated_audit_event_id")
	assert.Contains(t, readTableColumns(t, database, "commodities"), "created_audit_event_id")
	assert.Contains(t, readTableColumns(t, database, "commodity_versions"), "change_audit_event_id")
	assert.Contains(t, readTableColumns(t, database, "institutions"), "created_audit_event_id")
	assert.Contains(t, readTableColumns(t, database, "institution_versions"), "change_audit_event_id")
	assert.Contains(t, readTableColumns(t, database, "accounts"), "created_audit_event_id")
	assert.Contains(t, readTableColumns(t, database, "account_versions"), "change_audit_event_id")
}

func TestBookDefaultCurrencyInsertMustReferenceSameBookCurrency(t *testing.T) {
	database := openTestDatabase(t)

	require.NoError(t, Migrate(context.Background(), database))

	_, err := database.ExecContext(context.Background(), `
		INSERT INTO users (id, username, password_hash, is_owner, created_at, updated_at)
		VALUES (1, 'owner', 'hash', 1, '2026-06-01T00:00:00Z', '2026-06-01T00:00:00Z');

		INSERT INTO books (
			id,
			owner_user_id,
			code,
			name,
			default_currency_commodity_id,
			updated_by_user_id,
			created_at,
			updated_at
		)
		VALUES (1, 1, 'personal', 'Personal', 999, 1, '2026-06-01T00:00:00Z', '2026-06-01T00:00:00Z');
	`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "book default currency must reference a currency in the same book")
}

func TestMigrateUpgradesHistoricalSetupOwnerSchema(t *testing.T) {
	database := openTestDatabase(t)

	_, err := database.ExecContext(context.Background(), `
		CREATE TABLE goose_db_version (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			version_id INTEGER NOT NULL,
			is_applied INTEGER NOT NULL,
			tstamp TIMESTAMP DEFAULT (datetime('now'))
		);
		INSERT INTO goose_db_version (version_id, is_applied) VALUES (0, 1), (1, 1), (2, 1);

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
			created_at TEXT NOT NULL
		);

		CREATE TABLE setup_steps (
			step_key TEXT PRIMARY KEY,
			completed_at TEXT
		);

		INSERT INTO setup_steps (step_key, completed_at)
		VALUES
			('owner', NULL),
			('book', NULL),
			('currencies', NULL),
			('system_accounts', NULL),
			('categories', NULL);

		INSERT INTO users (id, username, password_hash, is_owner, created_at, updated_at)
		VALUES (1, 'owner', 'hash', 1, '2026-05-31T06:07:00Z', '2026-05-31T06:07:00Z');

		INSERT INTO auth_sessions (id, user_id, token_hash, created_at)
		VALUES (1, 1, 'token-hash', '2026-05-31T06:07:00Z');
	`)
	require.NoError(t, err)

	require.NoError(t, Migrate(context.Background(), database))

	assert.Equal(t, []string{"id", "user_id", "token_hash", "created_at", "expires_at", "revoked_at"}, readTableColumns(t, database, "auth_sessions"))
	assert.Equal(t, []string{"scope_type", "scope_key", "failed_attempts", "blocked_until", "updated_at"}, readTableColumns(t, database, "login_throttles"))

	var expiresAt string
	var revokedAt sql.NullString
	err = database.QueryRowContext(context.Background(), `SELECT expires_at, revoked_at FROM auth_sessions WHERE id = 1`).Scan(&expiresAt, &revokedAt)
	require.NoError(t, err)
	assert.Equal(t, "2026-06-30T06:07:00Z", expiresAt)
	assert.False(t, revokedAt.Valid)
}

func TestWithRequiredPragmasPreservesExistingQuery(t *testing.T) {
	got := withRequiredPragmas("file:var/dev.sqlite?mode=rwc")

	assert.Contains(t, got, "mode=rwc")
	assert.Contains(t, got, "_pragma=busy_timeout(5000)")
	assert.Contains(t, got, "_pragma=foreign_keys(1)")
	assert.Contains(t, got, "_pragma=journal_mode(WAL)")
	assert.Contains(t, got, "_pragma=synchronous(NORMAL)")
	assert.Contains(t, got, "_pragma=wal_autocheckpoint(1000)")
}

func openTestDatabase(t *testing.T) *sql.DB {
	t.Helper()

	database, err := Open(context.Background(), "file:"+filepath.Join(t.TempDir(), "rekenraam.sqlite"))
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, database.Close())
	})

	return database
}

func readTableColumns(t *testing.T, database *sql.DB, tableName string) []string {
	t.Helper()

	rows, err := database.QueryContext(context.Background(), fmt.Sprintf("PRAGMA table_info(%s)", tableName))
	require.NoError(t, err)
	defer rows.Close()

	var columnNames []string
	for rows.Next() {
		var (
			columnID   int
			columnName string
			columnType string
			notNull    int
			defaultVal sql.NullString
			primaryKey int
		)
		require.NoError(t, rows.Scan(&columnID, &columnName, &columnType, &notNull, &defaultVal, &primaryKey))
		columnNames = append(columnNames, columnName)
	}
	require.NoError(t, rows.Err())

	return columnNames
}
