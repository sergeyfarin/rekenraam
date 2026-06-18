package db

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rekenraam/backend/migrations"
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
	assert.Subset(t, readTableColumns(t, database, "tags"), []string{
		"id",
		"book_id",
		"name",
		"kind",
		"status",
		"metadata_json",
		"created_audit_event_id",
		"updated_audit_event_id",
	})
	assert.True(t, sqliteObjectExists(t, database, "index", "tags_active_name_kind_idx"))
	assert.Contains(t, readTableColumns(t, database, "commodities"), "created_audit_event_id")
	assert.Contains(t, readTableColumns(t, database, "commodity_versions"), "change_audit_event_id")
	assert.Contains(t, readTableColumns(t, database, "institutions"), "created_audit_event_id")
	assert.Contains(t, readTableColumns(t, database, "institution_versions"), "change_audit_event_id")
	assert.Subset(t, readTableColumns(t, database, "account_kinds"), []string{
		"code",
		"account_class",
		"base_kind",
		"is_user_assignable",
	})
	assert.Contains(t, readTableColumns(t, database, "accounts"), "created_audit_event_id")
	assert.Contains(t, readTableColumns(t, database, "account_versions"), "change_audit_event_id")
	assert.True(t, sqliteObjectExists(t, database, "view", "current_account_versions"))
	assert.True(t, sqliteObjectExists(t, database, "view", "current_institution_versions"))
	assert.True(t, sqliteObjectExists(t, database, "view", "current_commodity_versions"))
	assert.Subset(t, readTableColumns(t, database, "payees"), []string{
		"id",
		"book_id",
		"created_audit_event_id",
	})
	assert.Subset(t, readTableColumns(t, database, "payee_versions"), []string{
		"id",
		"payee_id",
		"version_seq",
		"status",
		"normalized_name",
	})
	assert.True(t, sqliteObjectExists(t, database, "view", "current_payee_versions"))
	assert.Subset(t, readTableColumns(t, database, "transactions"), []string{
		"id",
		"book_id",
		"correction_of_transaction_id",
		"created_audit_event_id",
	})
	assert.Subset(t, readTableColumns(t, database, "transaction_versions"), []string{
		"id",
		"transaction_id",
		"version_seq",
		"status",
		"transaction_date",
	})
	assert.Subset(t, readTableColumns(t, database, "journal_entries"), []string{
		"id",
		"transaction_version_id",
		"entry_date",
		"entry_kind",
	})
	assert.Subset(t, readMarketDataSourceCodes(t, database), []string{
		"manual",
		"ecb_euro_reference_rates",
		"frankfurter",
		"exchangerate_api_open_access",
		"open_exchange_rates_free",
	})
	assert.Subset(t, readTableColumns(t, database, "posting_versions"), []string{
		"id",
		"posting_line_id",
		"account_id",
		"quantity_value",
		"quantity_scale",
		"commodity_id",
	})
	assert.True(t, sqliteObjectExists(t, database, "view", "current_transaction_versions"))
	assert.True(t, sqliteObjectExists(t, database, "table", "transaction_search"))
	assert.True(t, sqliteObjectExists(t, database, "table", "reconciliation_sessions"))
	assert.True(t, sqliteObjectExists(t, database, "table", "reconciliation_session_postings"))
	assert.True(t, sqliteObjectExists(t, database, "table", "reconciliation_checkpoints"))
	assert.True(t, sqliteObjectExists(t, database, "table", "reconciliation_checkpoint_postings"))
}

func TestCurrentVersionViewsUseConsistentSelectionRules(t *testing.T) {
	database := openTestDatabase(t)

	require.NoError(t, Migrate(context.Background(), database))

	for _, viewName := range []string{
		"current_commodity_versions",
		"current_institution_versions",
		"current_account_versions",
		"current_payee_versions",
	} {
		viewSQL := readSQLiteObjectSQL(t, database, "view", viewName)
		assert.Contains(t, viewSQL, "effective_from <= date('now')", viewName)
		assert.Contains(t, viewSQL, "ORDER BY", viewName)
		assert.Contains(t, viewSQL, "effective_from DESC", viewName)
		assert.Contains(t, viewSQL, "version_seq DESC", viewName)
	}

	transactionViewSQL := readSQLiteObjectSQL(t, database, "view", "current_transaction_versions")
	assert.NotContains(t, transactionViewSQL, "effective_from", "transaction versions are sequence-only and not effective-dated")
	assert.Contains(t, transactionViewSQL, "ORDER BY", "current_transaction_versions")
	assert.Contains(t, transactionViewSQL, "version_seq DESC", "current_transaction_versions")
	assert.Contains(t, transactionViewSQL, "id DESC", "current_transaction_versions")
}

func TestCommodityPrecisionCeilingAllowsCryptoOnlyThroughTwentyFour(t *testing.T) {
	database := openTestDatabase(t)
	require.NoError(t, Migrate(context.Background(), database))
	insertMinimalFinancialFixture(t, database)

	_, err := database.ExecContext(context.Background(), `
		INSERT INTO commodities (id, book_id, code, kind, is_builtin, created_at, created_by_user_id)
		VALUES (2, 1, 'ETH', 'crypto', 0, '2026-06-18T00:00:00Z', 1);
		INSERT INTO commodity_versions (
			commodity_id, version_seq, effective_from, recorded_at, changed_by_user_id,
			change_reason, status, symbol, display_symbol, name, standard_scale, max_quantity_scale
		) VALUES (2, 1, '2026-06-18', '2026-06-18T00:00:00Z', 1,
			'crypto precision', 'active', 'ETH', 'ETH', 'Ether', 18, 24);
	`)
	require.NoError(t, err)

	_, err = database.ExecContext(context.Background(), `
		INSERT INTO commodity_versions (
			commodity_id, version_seq, effective_from, recorded_at, changed_by_user_id,
			change_reason, status, symbol, display_symbol, name, standard_scale, max_quantity_scale
		) VALUES (1, 1, '2026-06-18', '2026-06-18T00:00:00Z', 1,
			'invalid currency precision', 'active', 'USD', '$', 'US Dollar', 2, 13)
	`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "commodity precision is invalid for commodity kind")

	var columnType string
	require.NoError(t, database.QueryRowContext(context.Background(), `
		SELECT type FROM pragma_table_info('posting_versions') WHERE name = 'quantity_value'
	`).Scan(&columnType))
	assert.Equal(t, "TEXT", columnType)
}

func TestLosslessQuantityMigrationPreservesExistingLedgerRows(t *testing.T) {
	database := openTestDatabase(t)
	provider, err := goose.NewProvider(goose.DialectSQLite3, database, migrations.FS)
	require.NoError(t, err)
	_, err = provider.UpTo(context.Background(), 6)
	require.NoError(t, err)
	insertMinimalFinancialFixture(t, database)

	_, err = database.ExecContext(context.Background(), `
		INSERT INTO commodity_versions (
			commodity_id, version_seq, effective_from, recorded_at, changed_by_user_id,
			change_reason, status, symbol, display_symbol, name, standard_scale, max_quantity_scale
		) VALUES (1, 1, '2026-01-01', '2026-01-01T00:00:00Z', 1,
			'initial', 'active', 'USD', '$', 'US Dollar', 2, 6);
		INSERT INTO account_versions (
			account_id, version_seq, effective_from, recorded_at, changed_by_user_id,
			change_reason, status, opened_on, name, account_class, account_kind,
			default_commodity_id, quantity_scale_override, allows_postings
		) VALUES (1, 1, '2026-01-01', '2026-01-01T00:00:00Z', 1,
			'initial', 'active', '2026-01-01', 'Cash', 'asset', 'cash', 1, 6, 1);
		INSERT INTO transactions (id, book_id, created_at, created_by_user_id)
		VALUES (1, 1, '2026-01-01T00:00:00Z', 1);
		INSERT INTO transaction_versions (
			id, book_id, transaction_id, version_seq, status, transaction_kind,
			transaction_date, description, note_markdown, recorded_at,
			changed_by_user_id, change_reason
		) VALUES (1, 1, 1, 1, 'posted', 'ordinary', '2026-01-01', '', '',
			'2026-01-01T00:00:00Z', 1, 'initial');
		INSERT INTO journal_entries (id, book_id, transaction_version_id, entry_seq, entry_date, entry_kind)
		VALUES (1, 1, 1, 1, '2026-01-01', 'ordinary');
		INSERT INTO posting_lines (id, book_id, transaction_id, line_key, created_at, created_by_user_id)
		VALUES (1, 1, 1, 'line-1', '2026-01-01T00:00:00Z', 1);
		INSERT INTO posting_versions (
			id, book_id, transaction_version_id, journal_entry_id, posting_line_id,
			line_seq, account_id, quantity_value, quantity_scale, commodity_id,
			reconciliation_status
		) VALUES (1, 1, 1, 1, 1, 1, 1, 12345, 2, 1, 'uncleared');
	`)
	require.NoError(t, err)

	_, err = provider.Up(context.Background())
	require.NoError(t, err)

	var value string
	var storageType string
	require.NoError(t, database.QueryRowContext(context.Background(), `
		SELECT quantity_value, typeof(quantity_value) FROM posting_versions WHERE id = 1
	`).Scan(&value, &storageType))
	assert.Equal(t, "12345", value)
	assert.Equal(t, "text", storageType)
}

func TestMigrationsEnforceVersionDatesAndPositiveSequences(t *testing.T) {
	database := openTestDatabase(t)

	require.NoError(t, Migrate(context.Background(), database))
	insertMinimalFinancialFixture(t, database)

	_, err := database.ExecContext(context.Background(), `
		INSERT INTO commodity_versions (
			commodity_id,
			version_seq,
			effective_from,
			recorded_at,
			changed_by_user_id,
			change_reason,
			status,
			symbol,
			display_symbol,
			name,
			standard_scale,
			max_quantity_scale
		)
		VALUES (1, 1, '2026-06-06T00:00:00Z', '2026-06-06T00:00:00Z', 1, 'test', 'active', 'USD', '$', 'US Dollar', 2, 2);
	`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "CHECK constraint failed")

	_, err = database.ExecContext(context.Background(), `
		INSERT INTO institution_versions (
			institution_id,
			version_seq,
			effective_from,
			recorded_at,
			changed_by_user_id,
			change_reason,
			status,
			name,
			kind
		)
		VALUES (1, 0, '2026-06-06', '2026-06-06T00:00:00Z', 1, 'test', 'active', 'Bank', 'bank');
	`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "CHECK constraint failed")

	_, err = database.ExecContext(context.Background(), `
		INSERT INTO account_versions (
			account_id,
			version_seq,
			effective_from,
			recorded_at,
			changed_by_user_id,
			change_reason,
			status,
			opened_on,
			name,
			account_class,
			account_kind,
			allows_postings
		)
		VALUES (1, 1, '2026-06-06T00:00:00Z', '2026-06-06T00:00:00Z', 1, 'test', 'active', '2026-06-06', 'Checking', 'asset', 'checking', 1);
	`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "CHECK constraint failed")
}

func TestMigrationsEnforceAccountValidityDates(t *testing.T) {
	database := openTestDatabase(t)

	require.NoError(t, Migrate(context.Background(), database))
	insertMinimalFinancialFixture(t, database)

	_, err := database.ExecContext(context.Background(), `
		INSERT INTO account_versions (
			account_id,
			version_seq,
			effective_from,
			recorded_at,
			changed_by_user_id,
			change_reason,
			status,
			opened_on,
			closed_on,
			name,
			account_class,
			account_kind,
			allows_postings
		)
		VALUES (1, 1, '2026-06-06', '2026-06-06T00:00:00Z', 1, 'test', 'active', '2026-06-06', '2026-06-07', 'Checking', 'asset', 'checking', 1);
	`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "CHECK constraint failed")

	_, err = database.ExecContext(context.Background(), `
		INSERT INTO account_versions (
			account_id,
			version_seq,
			effective_from,
			recorded_at,
			changed_by_user_id,
			change_reason,
			status,
			opened_on,
			closed_on,
			name,
			account_class,
			account_kind,
			allows_postings
		)
		VALUES (1, 1, '2026-06-06', '2026-06-06T00:00:00Z', 1, 'test', 'closed', '2026-06-06', '2026-06-05', 'Checking', 'asset', 'checking', 1);
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

func TestPostingCommodityScaleTriggerUsesEntryDate(t *testing.T) {
	database := openTestDatabase(t)

	require.NoError(t, Migrate(context.Background(), database))
	insertMinimalFinancialFixture(t, database)

	_, err := database.ExecContext(context.Background(), `
		INSERT INTO commodity_versions (
			commodity_id,
			version_seq,
			effective_from,
			recorded_at,
			changed_by_user_id,
			change_reason,
			status,
			symbol,
			display_symbol,
			name,
			standard_scale,
			max_quantity_scale
		)
		VALUES
			(1, 1, '2026-01-01', '2026-06-01T00:00:00Z', 1, 'initial', 'active', 'USD', '$', 'US Dollar', 2, 2),
			(1, 2, '2026-06-07', '2026-06-07T00:00:00Z', 1, 'more precision', 'active', 'USD', '$', 'US Dollar', 2, 4);

		INSERT INTO account_versions (
			account_id,
			version_seq,
			effective_from,
			recorded_at,
			changed_by_user_id,
			change_reason,
			status,
			opened_on,
			name,
			account_class,
			account_kind,
			default_commodity_id,
			quantity_scale_override,
			allows_postings
		)
		VALUES (1, 1, '2026-01-01', '2026-06-01T00:00:00Z', 1, 'test', 'active', '2026-01-01', 'Checking', 'asset', 'checking', 1, 4, 1);

		INSERT INTO transactions (id, book_id, created_at, created_by_user_id)
		VALUES (1, 1, '2026-06-01T00:00:00Z', 1);

		INSERT INTO transaction_versions (
			id,
			book_id,
			transaction_id,
			version_seq,
			status,
			transaction_kind,
			transaction_date,
			description,
			note_markdown,
			recorded_at,
			changed_by_user_id,
			change_reason
		)
		VALUES (1, 1, 1, 1, 'posted', 'ordinary', '2026-06-06', '', '', '2026-06-06T00:00:00Z', 1, 'test');

		INSERT INTO journal_entries (id, book_id, transaction_version_id, entry_seq, entry_date, entry_kind)
		VALUES (1, 1, 1, 1, '2026-06-06', 'ordinary');

		INSERT INTO posting_lines (id, book_id, transaction_id, line_key, created_at, created_by_user_id)
		VALUES (1, 1, 1, 'line-1', '2026-06-06T00:00:00Z', 1);
	`)
	require.NoError(t, err)

	_, err = database.ExecContext(context.Background(), `
		INSERT INTO posting_versions (
			book_id,
			transaction_version_id,
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
}

func TestCurrentVersionViewsUseEffectiveDateBeforeVersionSequence(t *testing.T) {
	database := openTestDatabase(t)

	require.NoError(t, Migrate(context.Background(), database))
	insertMinimalFinancialFixture(t, database)

	_, err := database.ExecContext(context.Background(), `
		INSERT INTO account_versions (
			account_id,
			version_seq,
			effective_from,
			recorded_at,
			changed_by_user_id,
			change_reason,
			status,
			opened_on,
			name,
			account_class,
			account_kind,
			allows_postings
		)
		VALUES
			(1, 1, '2024-01-01', '2026-06-06T00:00:00Z', 1, 'test', 'active', '2024-01-01', 'Current Name', 'asset', 'checking', 1),
			(1, 2, '2023-01-01', '2026-06-06T00:00:00Z', 1, 'historical correction', 'active', '2024-01-01', 'Old Name', 'asset', 'checking', 1),
			(1, 3, '2024-01-01', '2026-06-06T00:01:00Z', 1, 'same-date correction', 'active', '2024-01-01', 'Corrected Current Name', 'asset', 'checking', 1);
	`)
	require.NoError(t, err)

	var name string
	err = database.QueryRowContext(context.Background(), `
		SELECT name
		FROM current_account_versions
		WHERE account_id = 1
	`).Scan(&name)
	require.NoError(t, err)
	assert.Equal(t, "Corrected Current Name", name)
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

		INSERT INTO commodities (id, book_id, code, kind, is_builtin, created_at, created_by_user_id)
		VALUES (1, 1, 'USD', 'currency', 1, '2026-06-01T00:00:00Z', 1);

		INSERT INTO institutions (id, book_id, created_at, created_by_user_id)
		VALUES (1, 1, '2026-06-01T00:00:00Z', 1);

		INSERT INTO accounts (id, book_id, created_at, created_by_user_id)
		VALUES (1, 1, '2026-06-01T00:00:00Z', 1);
	`)
	require.NoError(t, err)
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

func readMarketDataSourceCodes(t *testing.T, database *sql.DB) []string {
	t.Helper()

	rows, err := database.QueryContext(context.Background(), `SELECT code FROM market_data_sources ORDER BY code`)
	require.NoError(t, err)
	defer rows.Close()

	var codes []string
	for rows.Next() {
		var code string
		require.NoError(t, rows.Scan(&code))
		codes = append(codes, code)
	}
	require.NoError(t, rows.Err())
	return codes
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

func readSQLiteObjectSQL(t *testing.T, database *sql.DB, objectType string, objectName string) string {
	t.Helper()

	var objectSQL string
	err := database.QueryRowContext(
		context.Background(),
		`SELECT sql FROM sqlite_master WHERE type = ? AND name = ?`,
		objectType,
		objectName,
	).Scan(&objectSQL)
	require.NoError(t, err)

	return strings.Join(strings.Fields(objectSQL), " ")
}
