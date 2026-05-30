package db

import (
	"context"
	"database/sql"
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

	var message string
	err := database.QueryRowContext(
		context.Background(),
		"SELECT value FROM app_info WHERE key = ?",
		"hello",
	).Scan(&message)
	require.NoError(t, err)
	assert.Equal(t, "hello from sqlite", message)

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
