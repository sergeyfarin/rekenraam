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

func TestMigrateAppliesCurrentRowSchemaAndAuditTables(t *testing.T) {
	database := openTestDatabase(t)

	require.NoError(t, Migrate(context.Background(), database))
	require.NoError(t, Migrate(context.Background(), database))

	rows, err := database.QueryContext(context.Background(), "SELECT step_key FROM setup_steps ORDER BY step_key")
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
	assert.Subset(t, readTableColumns(t, database, "audit_event_targets"), []string{
		"audit_event_id",
		"book_id",
		"target_type",
		"target_id",
		"target_role",
	})
	assert.Subset(t, readTableColumns(t, database, "audit_event_changes"), []string{
		"audit_event_id",
		"book_id",
		"table_name",
		"row_id",
		"operation",
		"before_json",
		"after_json",
		"changed_fields_json",
	})

	assert.Subset(t, readTableColumns(t, database, "commodities"), []string{"status", "symbol", "display_symbol", "name", "standard_scale", "max_quantity_scale", "updated_audit_event_id"})
	assert.Subset(t, readTableColumns(t, database, "institutions"), []string{"status", "name", "kind", "metadata_json", "change_reason", "updated_audit_event_id"})
	assert.Subset(t, readTableColumns(t, database, "accounts"), []string{"status", "opened_on", "closed_on", "account_class", "account_kind", "allows_postings", "change_reason", "updated_audit_event_id"})
	assert.Subset(t, readTableColumns(t, database, "payees"), []string{"status", "name", "normalized_name", "default_account_id", "change_reason", "updated_audit_event_id"})
	assert.Subset(t, readTableColumns(t, database, "transactions"), []string{"status", "transaction_kind", "transaction_date", "payee_id", "description", "change_reason", "updated_audit_event_id"})
	assert.Subset(t, readTableColumns(t, database, "journal_entries"), []string{"id", "book_id", "transaction_id", "entry_date", "entry_kind"})
	assert.Subset(t, readTableColumns(t, database, "postings"), []string{"id", "transaction_id", "posting_line_id", "account_id", "quantity_value", "quantity_scale", "commodity_id", "reconciliation_status"})
	assert.Subset(t, readTableColumns(t, database, "reconciliation_session_postings"), []string{"session_id", "posting_id", "transaction_id", "posting_line_id"})
	assert.Subset(t, readTableColumns(t, database, "reconciliation_checkpoint_postings"), []string{"checkpoint_id", "posting_id", "account_id", "commodity_id"})

	for _, tableName := range []string{
		"commodity_versions",
		"institution_versions",
		"account_versions",
		"payee_versions",
		"transaction_versions",
		"posting_versions",
	} {
		assert.False(t, sqliteObjectExists(t, database, "table", tableName), tableName)
	}
	for _, viewName := range []string{
		"current_commodity_versions",
		"current_institution_versions",
		"current_account_versions",
		"current_payee_versions",
		"current_transaction_versions",
	} {
		assert.False(t, sqliteObjectExists(t, database, "view", viewName), viewName)
	}

	assert.True(t, sqliteObjectExists(t, database, "table", "transaction_search"))
	assert.True(t, sqliteObjectExists(t, database, "index", "tags_active_name_kind_idx"))
}

func TestMigrationsEnforceAccountValidityDates(t *testing.T) {
	database := openTestDatabase(t)

	require.NoError(t, Migrate(context.Background(), database))
	insertMinimalFinancialFixture(t, database)

	_, err := database.ExecContext(context.Background(), `
		UPDATE accounts
		SET opened_on = '2026-06-06',
			closed_on = '2026-06-05'
		WHERE id = 1;
	`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "CHECK constraint failed")
}

func TestMigrationsEnforceTagIconFormat(t *testing.T) {
	database := openTestDatabase(t)

	require.NoError(t, Migrate(context.Background(), database))
	insertMinimalFinancialFixture(t, database)

	_, err := database.ExecContext(context.Background(), `
		INSERT INTO tags (
			book_id,
			name,
			kind,
			icon,
			status,
			metadata_json,
			created_at,
			created_by_user_id,
			updated_at,
			updated_by_user_id
		)
		VALUES (1, 'Bad Icon', 'custom', 'Plane!', 'active', '{}', '2026-06-01T00:00:00Z', 1, '2026-06-01T00:00:00Z', 1);
	`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "CHECK constraint failed")
}

func TestPostingCommodityScaleTriggersValidateInsertAndUpdate(t *testing.T) {
	database := openTestDatabase(t)

	require.NoError(t, Migrate(context.Background(), database))
	insertMinimalFinancialFixture(t, database)
	insertMinimalPostedTransactionFixture(t, database)

	_, err := database.ExecContext(context.Background(), `
		INSERT INTO postings (
			book_id,
			transaction_id,
			journal_entry_id,
			posting_line_id,
			line_seq,
			account_id,
			quantity_value,
			quantity_scale,
			commodity_id,
			reconciliation_status
		)
		VALUES (1, 1, 1, 1, 1, 1, 1000, 3, 1, 'uncleared');
	`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "posting commodity scale is invalid")

	_, err = database.ExecContext(context.Background(), `
		INSERT INTO postings (
			book_id,
			transaction_id,
			journal_entry_id,
			posting_line_id,
			line_seq,
			account_id,
			quantity_value,
			quantity_scale,
			commodity_id,
			reconciliation_status
		)
		VALUES (1, 1, 1, 1, 1, 1, 1000, 2, 1, 'uncleared');
	`)
	require.NoError(t, err)

	_, err = database.ExecContext(context.Background(), `
		UPDATE postings
		SET quantity_scale = 3
		WHERE id = 1;
	`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "posting commodity scale is invalid")
}

func TestBooksRemainSingleBookUntilScopeChanges(t *testing.T) {
	database := openTestDatabase(t)

	require.NoError(t, Migrate(context.Background(), database))

	_, err := database.ExecContext(context.Background(), `
		INSERT INTO users (id, username, password_hash, is_owner, created_at, updated_at)
		VALUES (1, 'owner', 'hash', 1, '2026-06-01T00:00:00Z', '2026-06-01T00:00:00Z');
	`)
	require.NoError(t, err)

	_, err = database.ExecContext(context.Background(), `
		INSERT INTO books (
			id,
			owner_user_id,
			code,
			name,
			updated_by_user_id,
			created_at,
			updated_at
		)
		VALUES (2, 1, 'second', 'Second', 1, '2026-06-01T00:00:00Z', '2026-06-01T00:00:00Z');
	`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "CHECK constraint failed")
}

func TestBookDefaultCurrencyMustReferenceExistingCurrency(t *testing.T) {
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
	assert.Contains(t, err.Error(), "FOREIGN KEY constraint failed")
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

func insertMinimalFinancialFixture(t *testing.T, database *sql.DB) {
	t.Helper()

	_, err := database.ExecContext(context.Background(), `
		INSERT INTO users (id, username, password_hash, is_owner, created_at, updated_at)
		VALUES (1, 'owner', 'hash', 1, '2026-06-01T00:00:00Z', '2026-06-01T00:00:00Z');

		INSERT INTO books (
			id,
			owner_user_id,
			code,
			name,
			updated_by_user_id,
			created_at,
			updated_at
		)
		VALUES (1, 1, 'personal', 'Personal', 1, '2026-06-01T00:00:00Z', '2026-06-01T00:00:00Z');

		INSERT INTO commodities (
			id,
			book_id,
			code,
			kind,
			is_builtin,
			status,
			symbol,
			display_symbol,
			name,
			standard_scale,
			max_quantity_scale,
			created_at,
			created_by_user_id,
			updated_at,
			updated_by_user_id
		)
		VALUES (1, 1, 'USD', 'currency', 1, 'active', '$', '$', 'US Dollar', 2, 2, '2026-06-01T00:00:00Z', 1, '2026-06-01T00:00:00Z', 1);

		INSERT INTO accounts (
			id,
			book_id,
			status,
			opened_on,
			name,
			account_class,
			account_kind,
			default_commodity_id,
			quantity_scale_override,
			allows_postings,
			created_at,
			created_by_user_id,
			updated_at,
			updated_by_user_id
		)
		VALUES (1, 1, 'active', '2026-01-01', 'Checking', 'asset', 'checking', 1, 4, 1, '2026-06-01T00:00:00Z', 1, '2026-06-01T00:00:00Z', 1);
	`)
	require.NoError(t, err)
}

func insertMinimalPostedTransactionFixture(t *testing.T, database *sql.DB) {
	t.Helper()

	_, err := database.ExecContext(context.Background(), `
		INSERT INTO transactions (
			id,
			book_id,
			status,
			transaction_kind,
			transaction_date,
			description,
			note_markdown,
			created_at,
			created_by_user_id,
			updated_at,
			updated_by_user_id
		)
		VALUES (1, 1, 'posted', 'ordinary', '2026-06-06', '', '', '2026-06-06T00:00:00Z', 1, '2026-06-06T00:00:00Z', 1);

		INSERT INTO journal_entries (id, book_id, transaction_id, entry_seq, entry_date, entry_kind)
		VALUES (1, 1, 1, 1, '2026-06-06', 'ordinary');

		INSERT INTO posting_lines (id, book_id, transaction_id, line_key, created_at, created_by_user_id)
		VALUES (1, 1, 1, 'line-1', '2026-06-06T00:00:00Z', 1);
	`)
	require.NoError(t, err)
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

func sqliteObjectExists(t *testing.T, database *sql.DB, objectType string, objectName string) bool {
	t.Helper()

	var exists int
	err := database.QueryRowContext(
		context.Background(),
		`SELECT EXISTS(SELECT 1 FROM sqlite_master WHERE type = ? AND name = ?)`,
		objectType,
		objectName,
	).Scan(&exists)
	require.NoError(t, err)

	return exists == 1
}
