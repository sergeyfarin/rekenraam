package db

import (
	"context"
	"database/sql"
	"testing"

	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"rekenraam/backend/migrations"
)

func TestThrottleGenerationMigrationPreservesBudgetsAndRestarts(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	database, _ := newMFATestRepository(t)
	provider, err := goose.NewProvider(goose.DialectSQLite3, database, migrations.FS)
	require.NoError(t, err)
	_, err = provider.DownTo(ctx, 19)
	require.NoError(t, err)
	_, err = database.Exec(`INSERT INTO login_throttles(scope_type,scope_key,failed_attempts,blocked_until,updated_at) VALUES
 ('username','owner',4,NULL,'2026-10-20T12:00:00Z'),
 ('client_ip','203.0.113.40',0,'2026-10-20T12:15:00Z','2026-10-20T12:00:00Z'),
 ('trusted_device','1',5,NULL,'2026-10-20T12:00:00Z'),
 ('reauth_session','1',3,NULL,'2026-10-20T12:00:00Z')`)
	require.NoError(t, err)
	before := captureLedgerState(t, database)
	budgets := func(database *sql.DB) []string {
		rows, err := database.Query(`SELECT scope_type || ':' || scope_key || ':' || failed_attempts || ':' || COALESCE(blocked_until,'') || ':' || updated_at FROM login_throttles ORDER BY scope_type`)
		require.NoError(t, err)
		defer rows.Close()
		result := []string{}
		for rows.Next() {
			var row string
			require.NoError(t, rows.Scan(&row))
			result = append(result, row)
		}
		require.NoError(t, rows.Err())
		return result
	}
	original := budgets(database)
	_, err = provider.Up(ctx)
	require.NoError(t, err)
	after := captureLedgerState(t, database)
	delete(before, "count:goose_db_version")
	delete(after, "count:goose_db_version")
	assert.Equal(t, before, after, "no financial, credential, event or other user row changes")
	assert.Equal(t, original, budgets(database), "no counter/deadline reset on upgrade")
	var distinct, populated int
	require.NoError(t, database.QueryRow(`SELECT COUNT(DISTINCT generation),SUM(length(generation)=32) FROM login_throttles`).Scan(&distinct, &populated))
	assert.Equal(t, 4, distinct)
	assert.Equal(t, 4, populated)
	var sequence int
	var name, path string
	require.NoError(t, database.QueryRow(`PRAGMA database_list`).Scan(&sequence, &name, &path))
	var generation string
	require.NoError(t, database.QueryRow(`SELECT generation FROM login_throttles WHERE scope_type='username'`).Scan(&generation))
	require.NoError(t, database.Close())
	reopened, err := Open(ctx, SQLiteFileURL(path))
	require.NoError(t, err)
	defer reopened.Close()
	require.NoError(t, Migrate(ctx, reopened))
	assert.Equal(t, original, budgets(reopened))
	var persisted string
	require.NoError(t, reopened.QueryRow(`SELECT generation FROM login_throttles WHERE scope_type='username'`).Scan(&persisted))
	assert.Equal(t, generation, persisted, "restart must preserve admission identity")
	// A local down/up preserves counts and deadlines, then assigns fresh IDs.
	provider, err = goose.NewProvider(goose.DialectSQLite3, reopened, migrations.FS)
	require.NoError(t, err)
	_, err = provider.DownTo(ctx, 19)
	require.NoError(t, err)
	assert.Equal(t, original, budgets(reopened))
	_, err = provider.Up(ctx)
	require.NoError(t, err)
	require.NoError(t, reopened.QueryRow(`SELECT generation FROM login_throttles WHERE scope_type='username'`).Scan(&persisted))
	assert.NotEqual(t, generation, persisted)
	assert.Equal(t, original, budgets(reopened))
}
