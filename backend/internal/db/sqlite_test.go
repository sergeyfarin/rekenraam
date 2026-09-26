package db

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rekenraam/backend/internal/exact"
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

func TestEnforceSQLiteFilePermissionsRestrictsDatabaseAndSidecars(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "private.sqlite")
	databaseURL := "file:" + databasePath
	database, err := Open(context.Background(), databaseURL)
	require.NoError(t, err)
	defer database.Close()

	_, err = database.ExecContext(context.Background(), "CREATE TABLE permissions_test (id INTEGER PRIMARY KEY)")
	require.NoError(t, err)
	require.NoError(t, EnforceSQLiteFilePermissions(databaseURL))

	for _, path := range []string{databasePath, databasePath + "-wal", databasePath + "-shm"} {
		info, err := os.Stat(path)
		if os.IsNotExist(err) {
			continue
		}
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0o600), info.Mode().Perm(), path)
	}
}

// A recovery backup is a full copy of the ledger, so the directory holding it
// must not be more permissive than the database files themselves, which
// EnforceSQLiteFilePermissions pins to 0600. The container image makes the same
// assumption: deploy/docker/Dockerfile creates /app/data as 0700.
//
// This covers directories the backup itself creates. A directory that already
// exists keeps whatever mode it has — MkdirAll does not tighten one — so an
// operator pointing --backup-path into an existing world-readable directory
// still gets a 0600 backup file inside a 0755 directory. The file mode is what
// protects the ledger contents there.
func TestBackupSQLiteDatabaseCreatesPrivateDirectory(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "source.sqlite")
	database, err := Open(context.Background(), "file:"+databasePath)
	require.NoError(t, err)
	defer database.Close()

	_, err = database.ExecContext(context.Background(), "CREATE TABLE backup_perm_test (id INTEGER PRIMARY KEY)")
	require.NoError(t, err)

	// A nested path so the directory is created by the backup, not by t.TempDir.
	backupDir := filepath.Join(t.TempDir(), "backups", "nested")
	backupPath := filepath.Join(backupDir, "recovery.sqlite")
	require.NoError(t, BackupSQLiteDatabase(context.Background(), database, backupPath))

	info, err := os.Stat(backupDir)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o700), info.Mode().Perm(), backupDir)

	backupInfo, err := os.Stat(backupPath)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), backupInfo.Mode().Perm(), backupPath)
}

func TestVerifySQLiteBackupRejectsForeignKeyViolation(t *testing.T) {
	backupPath := filepath.Join(t.TempDir(), "invalid-backup.sqlite")
	backupDatabase, err := sql.Open(driverName, "file:"+backupPath)
	require.NoError(t, err)
	_, err = backupDatabase.ExecContext(context.Background(), `
		PRAGMA foreign_keys = OFF;
		CREATE TABLE parent_records (id INTEGER PRIMARY KEY);
		CREATE TABLE child_records (parent_id INTEGER REFERENCES parent_records(id));
		INSERT INTO child_records (parent_id) VALUES (1);
	`)
	require.NoError(t, err)
	require.NoError(t, backupDatabase.Close())

	err = VerifySQLiteBackup(context.Background(), backupPath)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "foreign_key_check found violation")
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
	assert.True(t, sqliteObjectExists(t, database, "table", "background_work_items"))
	assert.True(t, sqliteObjectExists(t, database, "trigger", "fx_work_after_account_version_insert"))
	assert.True(t, sqliteObjectExists(t, database, "trigger", "fx_work_after_posting_version_insert"))
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

// The v0.1 database starts at migration 1. This test builds that released
// state from the frozen baseline plus a frozen seed — a real book written
// through the real API and dumped — then upgrades it to HEAD and checks that
// both the schema and the data came through.
//
// The data half is the point. Schema convergence alone says a migration
// produced the right shape; it says nothing about whether it carried the
// ledger across. While every migration only adds tables that distinction is
// academic, but the first migration that rewrites one — SQLite's twelve-step
// table rebuild, which is how a post-v0.1 schema redesign has to happen — makes
// this test the only thing standing between a redesign and silent data loss.
// It needs to be watching the ledger by then, not just the DDL.
func TestMigrateUpgradesV01DatabaseToFreshHeadSchema(t *testing.T) {
	ctx := context.Background()
	upgraded := openTestDatabase(t)

	baseline, err := migrations.FS.ReadFile("0001_initial_schema.sql")
	require.NoError(t, err)
	baselineFS := fstest.MapFS{
		"0001_initial_schema.sql": {Data: baseline},
	}
	provider, err := goose.NewProvider(goose.DialectSQLite3, upgraded, baselineFS)
	require.NoError(t, err)
	_, err = provider.Up(ctx)
	require.NoError(t, err)

	seed, err := os.ReadFile(filepath.Join("testdata", "v01_seed.sql"))
	require.NoError(t, err)
	// The dump is in table order, not dependency order, so a row can reference
	// one that has not been inserted yet. defer_foreign_keys holds every check
	// until COMMIT — which still enforces them, just once the whole book is
	// present. Turning foreign keys off outright would not.
	loadTx, err := upgraded.BeginTx(ctx, nil)
	require.NoError(t, err)
	_, err = loadTx.ExecContext(ctx, `PRAGMA defer_foreign_keys = ON`)
	require.NoError(t, err)
	_, err = loadTx.ExecContext(ctx, string(seed))
	require.NoError(t, err, "the frozen seed must load into the frozen baseline")
	require.NoError(t, loadTx.Commit(), "the frozen seed must satisfy every foreign key")

	before := captureLedgerState(t, upgraded)
	require.NoError(t, Migrate(ctx, upgraded))
	after := captureLedgerState(t, upgraded)

	assert.Equal(t, before, after, "upgrading must not change a single durable figure")

	// Stated separately from the snapshot comparison so a failure names what
	// broke rather than dumping two large maps side by side.
	assertUpgradedBookIsIntact(t, upgraded)

	fresh := openTestDatabase(t)
	require.NoError(t, Migrate(ctx, fresh))
	assert.Equal(t, schemaFingerprint(t, fresh), schemaFingerprint(t, upgraded))
}

// captureLedgerState reads every durable figure the seed carries, keyed so a
// diff points at the row that moved. Counting rows is not enough: a migration
// that rebuilt a table and mangled a coefficient, a scale, or a lifecycle flag
// would leave every count identical.
func captureLedgerState(t *testing.T, database *sql.DB) map[string]string {
	t.Helper()
	ctx := context.Background()
	state := map[string]string{}

	// Row counts across every table that carries user data, so a table that
	// loses or gains rows is caught even where no query below inspects it.
	rows, err := database.QueryContext(ctx, `
		SELECT name FROM sqlite_master
		WHERE type = 'table' AND name NOT LIKE 'sqlite_%' AND name NOT LIKE 'transaction_search%'
		ORDER BY name
	`)
	require.NoError(t, err)
	var tables []string
	for rows.Next() {
		var name string
		require.NoError(t, rows.Scan(&name))
		tables = append(tables, name)
	}
	require.NoError(t, rows.Err())
	require.NoError(t, rows.Close())
	for _, table := range tables {
		var count int64
		require.NoError(t, database.QueryRowContext(ctx, `SELECT count(*) FROM "`+table+`"`).Scan(&count))
		state["count:"+table] = strconv.FormatInt(count, 10)
	}

	// Exact figures, as strings, so a rescaled or rounded value is a diff
	// rather than an equal number at a different precision.
	for key, query := range map[string]string{
		"postings": `SELECT pl.id || '=' || pv.account_id || '/' || pv.commodity_id || '/' ||
				pv.quantity_value || 'e-' || pv.quantity_scale || '/' || pv.reconciliation_status
			FROM posting_versions pv JOIN posting_lines pl ON pl.id = pv.posting_line_id
			ORDER BY pv.id`,
		"transactions": `SELECT t.id || '=' || tv.version_seq || '/' || tv.status || '/' ||
				tv.transaction_date || '/' || coalesce(t.deleted_at, '-')
			FROM transaction_versions tv JOIN transactions t ON t.id = tv.transaction_id
			ORDER BY tv.id`,
		"lots": `SELECT id || '=' || opened_on || '/' || status || '/' ||
				quantity_value || 'e-' || quantity_scale || '/' ||
				remaining_quantity_value || 'e-' || remaining_quantity_scale || '/' ||
				cost_basis_value || 'e-' || cost_basis_scale || '/' ||
				remaining_cost_basis_value || 'e-' || remaining_cost_basis_scale
			FROM investment_lots ORDER BY id`,
		"lot_events": `SELECT id || '=' || lot_id || '/' || event_kind || '/' || event_date || '/' ||
				quantity_value || 'e-' || quantity_scale || '/' || cost_basis_value || 'e-' || cost_basis_scale ||
				'/' || coalesce(cost_basis_method, '-')
			FROM investment_lot_events ORDER BY id`,
		"disposal_decisions": `SELECT id || '=' || event_date || '/' || cost_basis_method || '/' || resolution_tier || '/' ||
				quantity_value || 'e-' || quantity_scale || '/' || disposed_basis_value || 'e-' || disposed_basis_scale
			FROM investment_disposal_decisions ORDER BY id`,
		"disposal_allocations": `SELECT id || '=' || lot_id || '/' || quantity_value || 'e-' || quantity_scale || '/' ||
				cost_basis_value || 'e-' || cost_basis_scale
			FROM investment_disposal_allocations ORDER BY id`,
		"checkpoints": `SELECT id || '=' || account_id || '/' || commodity_id || '/' || status || '/' ||
				statement_date || '/' || statement_account_sequence || '/' ||
				statement_balance_value || 'e-' || statement_balance_scale
			FROM reconciliation_checkpoints ORDER BY id`,
		"checkpoint_postings": `SELECT checkpoint_id || '=' || posting_version_id FROM reconciliation_checkpoint_postings
			ORDER BY checkpoint_id, posting_version_id`,
		"accounts": `SELECT a.id || '=' || coalesce(a.system_role, '-') || '/' || av.version_seq || '/' ||
				av.account_class || '/' || av.account_kind || '/' || av.status || '/' || av.opened_on || '/' ||
				coalesce(av.default_commodity_id, 0) || '/' || coalesce(av.quantity_scale_override, -1)
			FROM account_versions av JOIN accounts a ON a.id = av.account_id ORDER BY av.id`,
		"commodities": `SELECT c.id || '=' || c.code || '/' || c.kind || '/' || cv.standard_scale || '/' || cv.max_quantity_scale
			FROM commodity_versions cv JOIN commodities c ON c.id = cv.commodity_id ORDER BY cv.id`,
		"prices": `SELECT id || '=' || series_id || '/' || valuation_date || '/' ||
				price_value || 'e-' || price_scale || '/' || coalesce(voided_at, '-')
			FROM price_observations ORDER BY id`,
		"budget_targets": `SELECT id || '=' || category_account_id || '/' || commodity_id || '/' || period_start || '/' ||
				quantity_value || 'e-' || quantity_scale
			FROM budget_targets ORDER BY id`,
		"recurring": `SELECT rt.id || '=' || rt.name || '/' || rt.frequency || '/' || rt.starts_on || '/' ||
				rtp.account_id || '/' || rtp.quantity_value || 'e-' || rtp.quantity_scale
			FROM recurring_template_postings rtp JOIN recurring_templates rt ON rt.id = rtp.template_id
			ORDER BY rtp.id`,
		"tag_links": `SELECT 'txn:' || transaction_id || '=' || tag_id FROM transaction_tags ORDER BY transaction_id, tag_id`,
		"audit":     `SELECT id || '=' || operation || '/' || origin_type || '/' || occurred_at FROM audit_events ORDER BY id`,
		"deletions": `SELECT id || '=' || transaction_id || '/' || action || '/' || occurred_at
			FROM transaction_deletion_events ORDER BY id`,
	} {
		state[key] = joinQueryRows(t, database, query)
	}

	return state
}

func joinQueryRows(t *testing.T, database *sql.DB, query string) string {
	t.Helper()
	rows, err := database.QueryContext(context.Background(), query)
	require.NoError(t, err, query)
	defer rows.Close()
	var lines []string
	for rows.Next() {
		var line string
		require.NoError(t, rows.Scan(&line))
		lines = append(lines, line)
	}
	require.NoError(t, rows.Err())
	return strings.Join(lines, "\n")
}

// assertUpgradedBookIsIntact restates the book's own invariants against the
// upgraded database, so a migration that preserved every value but broke a
// relationship between them still fails. These are the same properties the
// app's self-check asserts; a released database that upgrades into one the
// self-check would reject has not been upgraded.
func assertUpgradedBookIsIntact(t *testing.T, database *sql.DB) {
	t.Helper()
	ctx := context.Background()

	// The seed is not empty. A migration that dropped everything would
	// otherwise satisfy every equality above it.
	var postings int64
	require.NoError(t, database.QueryRowContext(ctx, `SELECT count(*) FROM posting_versions`).Scan(&postings))
	require.Greater(t, postings, int64(20), "the frozen seed must still be a real book")

	// SQLite's own view of the file, including every foreign key the rebuild
	// would have had to re-point.
	var integrity string
	require.NoError(t, database.QueryRowContext(ctx, `PRAGMA integrity_check`).Scan(&integrity))
	assert.Equal(t, "ok", integrity)
	foreignKeyRows, err := database.QueryContext(ctx, `PRAGMA foreign_key_check`)
	require.NoError(t, err)
	defer foreignKeyRows.Close()
	assert.False(t, foreignKeyRows.Next(), "upgrade left a dangling foreign key")

	// Double entry, per commodity, over the current posted ledger. Coefficients
	// are strings and scales differ, so this sums in Go rather than in SQL.
	balanceRows, err := database.QueryContext(ctx, `
		SELECT pv.commodity_id, pv.quantity_value, pv.quantity_scale
		FROM posting_versions pv
		JOIN journal_entries je ON je.id = pv.journal_entry_id
		JOIN current_transaction_versions tv ON tv.id = je.transaction_version_id
		JOIN transactions t ON t.id = tv.transaction_id
		WHERE tv.status = 'posted' AND t.deleted_at IS NULL
	`)
	require.NoError(t, err)
	defer balanceRows.Close()
	totals := map[int64]*exact.ScaledInt{}
	for balanceRows.Next() {
		var commodityID int64
		var value exact.Coefficient
		var scale int
		require.NoError(t, balanceRows.Scan(&commodityID, &value, &scale))
		if totals[commodityID] == nil {
			totals[commodityID] = exact.NewScaledInt()
		}
		totals[commodityID].AddCoefficient(value, scale)
	}
	require.NoError(t, balanceRows.Err())
	require.NotEmpty(t, totals)
	for commodityID, total := range totals {
		assert.Zerof(t, total.Sign(), "commodity %d does not sum to zero after upgrade", commodityID)
	}

	// Lot conservation: every open lot still holds something, no lot holds
	// more than it was acquired with, and the closed one holds nothing.
	lotRows, err := database.QueryContext(ctx, `
		SELECT id, status, quantity_value, quantity_scale, remaining_quantity_value, remaining_quantity_scale
		FROM investment_lots ORDER BY id
	`)
	require.NoError(t, err)
	defer lotRows.Close()
	seenLot := false
	for lotRows.Next() {
		var id int64
		var status string
		var acquired, remaining exact.Coefficient
		var acquiredScale, remainingScale int
		require.NoError(t, lotRows.Scan(&id, &status, &acquired, &acquiredScale, &remaining, &remainingScale))
		seenLot = true
		acquiredTotal := exact.ScaledIntFromCoefficient(acquired, acquiredScale)
		remainingTotal := exact.ScaledIntFromCoefficient(remaining, remainingScale)
		assert.LessOrEqualf(t, remainingTotal.Cmp(acquiredTotal), 0, "lot %d holds more than it acquired", id)
		assert.GreaterOrEqualf(t, remainingTotal.Sign(), 0, "lot %d went negative", id)
		if status == "closed" {
			assert.Zerof(t, remainingTotal.Sign(), "closed lot %d still holds something", id)
		} else {
			assert.Positivef(t, remainingTotal.Sign(), "open lot %d holds nothing", id)
		}
	}
	require.NoError(t, lotRows.Err())
	require.True(t, seenLot, "the frozen seed must still carry investment lots")

	// The reconciliation checkpoint still sums to the statement it recorded,
	// from the postings it named.
	checkpointRows, err := database.QueryContext(ctx, `
		SELECT c.id, c.statement_balance_value, c.statement_balance_scale
		FROM reconciliation_checkpoints c WHERE c.status = 'active' ORDER BY c.id
	`)
	require.NoError(t, err)
	defer checkpointRows.Close()
	type checkpoint struct {
		id        int64
		statement *exact.ScaledInt
	}
	var checkpoints []checkpoint
	for checkpointRows.Next() {
		var id int64
		var value exact.Coefficient
		var scale int
		require.NoError(t, checkpointRows.Scan(&id, &value, &scale))
		checkpoints = append(checkpoints, checkpoint{id: id, statement: exact.ScaledIntFromCoefficient(value, scale)})
	}
	require.NoError(t, checkpointRows.Err())
	require.NotEmpty(t, checkpoints, "the frozen seed must still carry an active checkpoint")
	for _, entry := range checkpoints {
		cleared := exact.NewScaledInt()
		postingRows, err := database.QueryContext(ctx, `
			SELECT pv.quantity_value, pv.quantity_scale
			FROM reconciliation_checkpoint_postings cp
			JOIN posting_versions pv ON pv.id = cp.posting_version_id
			WHERE cp.checkpoint_id = ?
		`, entry.id)
		require.NoError(t, err)
		for postingRows.Next() {
			var value exact.Coefficient
			var scale int
			require.NoError(t, postingRows.Scan(&value, &scale))
			cleared.AddCoefficient(value, scale)
		}
		require.NoError(t, postingRows.Err())
		require.NoError(t, postingRows.Close())
		assert.Zerof(t, cleared.Cmp(entry.statement), "checkpoint %d no longer sums to its statement balance", entry.id)
	}

	// Lifecycle states the seed deliberately carries, so a migration cannot
	// quietly resurrect a voided or deleted transaction.
	var voided, deleted, superseded int64
	require.NoError(t, database.QueryRowContext(ctx,
		`SELECT count(*) FROM current_transaction_versions WHERE status = 'voided'`).Scan(&voided))
	require.NoError(t, database.QueryRowContext(ctx,
		`SELECT count(*) FROM transactions WHERE deleted_at IS NOT NULL`).Scan(&deleted))
	require.NoError(t, database.QueryRowContext(ctx,
		`SELECT count(*) FROM transaction_versions tv
		 WHERE NOT EXISTS (SELECT 1 FROM current_transaction_versions c WHERE c.id = tv.id)`).Scan(&superseded))
	assert.Equal(t, int64(1), voided, "the seeded voided transaction must stay voided")
	assert.Equal(t, int64(1), deleted, "the seeded soft-deleted transaction must stay deleted")
	assert.Positive(t, superseded, "the seeded edit must leave a superseded version behind")
}

func schemaFingerprint(t *testing.T, database *sql.DB) []string {
	t.Helper()
	rows, err := database.QueryContext(context.Background(), `
		SELECT type || ':' || name || ':' || coalesce(sql, '')
		FROM sqlite_schema
		WHERE name NOT LIKE 'sqlite_%' AND name NOT LIKE 'goose_%'
		ORDER BY type, name
	`)
	require.NoError(t, err)
	defer rows.Close()

	var fingerprint []string
	for rows.Next() {
		var object string
		require.NoError(t, rows.Scan(&object))
		fingerprint = append(fingerprint, object)
	}
	require.NoError(t, rows.Err())
	return fingerprint
}

func TestInitialMigrationDownRemovesTheConsolidatedSchema(t *testing.T) {
	database := openTestDatabase(t)
	provider, err := goose.NewProvider(goose.DialectSQLite3, database, migrations.FS)
	require.NoError(t, err)

	_, err = provider.Up(context.Background())
	require.NoError(t, err)
	_, err = provider.Down(context.Background())
	require.NoError(t, err)

	var objectCount int
	require.NoError(t, database.QueryRowContext(context.Background(), `
		SELECT COUNT(*) FROM sqlite_schema
		WHERE name NOT LIKE 'sqlite_%' AND name NOT LIKE 'goose_%'
	`).Scan(&objectCount))
	assert.Zero(t, objectCount)
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

func TestBaselineStoresLosslessLedgerQuantitiesAsText(t *testing.T) {
	database := openTestDatabase(t)
	require.NoError(t, Migrate(context.Background(), database))
	insertMinimalFinancialFixture(t, database)

	_, err := database.ExecContext(context.Background(), `
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

func TestMigrationsEnforceTransactionAndVersionIntegrity(t *testing.T) {
	database := openTestDatabase(t)

	require.NoError(t, Migrate(context.Background(), database))
	insertMinimalFinancialFixture(t, database)

	// A transaction cannot correct itself. The same-book trigger fires first
	// because a self-reference can never satisfy "target already exists" at
	// BEFORE INSERT time, but the row is rejected either way.
	_, err := database.ExecContext(context.Background(), `
		INSERT INTO transactions (id, book_id, correction_of_transaction_id, created_at, created_by_user_id)
		VALUES (2, 1, 2, '2026-06-06T00:00:00Z', 1);
	`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "correction target must belong to the same book")

	// A superseding version must belong to the same transaction.
	_, err = database.ExecContext(context.Background(), `
		INSERT INTO transactions (id, book_id, created_at, created_by_user_id) VALUES
			(3, 1, '2026-06-06T00:00:00Z', 1),
			(4, 1, '2026-06-06T00:00:00Z', 1);

		INSERT INTO transaction_versions (
			id, book_id, transaction_id, version_seq, status, transaction_kind,
			transaction_date, recorded_at, changed_by_user_id, change_reason
		)
		VALUES (10, 1, 3, 1, 'posted', 'ordinary', '2026-06-06', '2026-06-06T00:00:00Z', 1, 'test');
	`)
	require.NoError(t, err)

	_, err = database.ExecContext(context.Background(), `
		INSERT INTO transaction_versions (
			id, book_id, transaction_id, version_seq, supersedes_version_id, status,
			transaction_kind, transaction_date, recorded_at, changed_by_user_id, change_reason
		)
		VALUES (11, 1, 4, 1, 10, 'posted', 'ordinary', '2026-06-06', '2026-06-06T00:00:00Z', 1, 'test');
	`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "superseded transaction version must belong to the same transaction")

	// version_seq must be unique per transaction.
	_, err = database.ExecContext(context.Background(), `
		INSERT INTO transaction_versions (
			id, book_id, transaction_id, version_seq, status, transaction_kind,
			transaction_date, recorded_at, changed_by_user_id, change_reason
		)
		VALUES (12, 1, 3, 1, 'posted', 'ordinary', '2026-06-07', '2026-06-07T00:00:00Z', 1, 'test');
	`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "UNIQUE constraint failed")
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

func TestOpenReadOnlyRefusesWrites(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	databaseURL := "file:" + filepath.Join(t.TempDir(), "readonly.sqlite")
	writer, err := Open(ctx, databaseURL)
	require.NoError(t, err)
	defer writer.Close()
	require.NoError(t, Migrate(ctx, writer))

	reader, err := OpenReadOnly(ctx, databaseURL)
	require.NoError(t, err)
	defer reader.Close()

	var books int
	require.NoError(t, reader.QueryRowContext(ctx, "SELECT COUNT(*) FROM books").Scan(&books))

	_, err = reader.ExecContext(ctx, "INSERT INTO tags (book_id, name, kind, status, created_at, created_by_user_id, updated_at, updated_by_user_id) VALUES (1, 'x', 'custom', 'active', '2026-01-01T00:00:00Z', 1, '2026-01-01T00:00:00Z', 1)")
	require.Error(t, err, "the read pool must refuse writes rather than quietly competing with the writer")
}

// A snapshot read must not see rows committed after it began, which is what
// makes a multi-file export internally consistent.
func TestReadOnlySnapshotDoesNotSeeLaterWrites(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	databaseURL := "file:" + filepath.Join(t.TempDir(), "snapshot.sqlite")
	writer, err := Open(ctx, databaseURL)
	require.NoError(t, err)
	defer writer.Close()
	require.NoError(t, Migrate(ctx, writer))

	reader, err := OpenReadOnly(ctx, databaseURL)
	require.NoError(t, err)
	defer reader.Close()

	snapshot, err := reader.BeginTx(ctx, nil)
	require.NoError(t, err)
	defer func() { _ = snapshot.Rollback() }()

	var before int
	require.NoError(t, snapshot.QueryRowContext(ctx, "SELECT COUNT(*) FROM audit_events").Scan(&before))

	_, err = writer.ExecContext(ctx, "INSERT INTO audit_events (occurred_at, origin_type, operation) VALUES ('2026-01-01T00:00:00Z', 'internal', 'test.write')")
	require.NoError(t, err)

	var after int
	require.NoError(t, snapshot.QueryRowContext(ctx, "SELECT COUNT(*) FROM audit_events").Scan(&after))
	require.Equal(t, before, after, "the snapshot must not grow while it is open")
}

// The schema is one migration file now (T-64), which means nothing else
// re-derives it and nothing checks it against a previous shape. What can still
// be checked is that the file produces what the code expects: every table,
// index, and trigger the app reads, created exactly once, with the constraints
// that make the ledger's invariants enforceable rather than aspirational.
func TestMigrationsProduceTheExpectedSchema(t *testing.T) {
	t.Parallel()

	version, err := EmbeddedMigrationVersion()
	require.NoError(t, err)
	assert.Equal(t, int64(1), version, "the v0.1.0 baseline must remain a single migration")

	ctx := context.Background()
	database, err := Open(ctx, "file:"+filepath.Join(t.TempDir(), "schema.sqlite"))
	require.NoError(t, err)
	defer database.Close()
	require.NoError(t, Migrate(ctx, database))

	objects := map[string]string{}
	rows, err := database.QueryContext(ctx, `
		SELECT type, name FROM sqlite_master
		WHERE name NOT LIKE 'sqlite_%' AND name NOT LIKE 'goose_%'
	`)
	require.NoError(t, err)
	defer rows.Close()
	for rows.Next() {
		var kind, name string
		require.NoError(t, rows.Scan(&kind, &name))
		_, duplicate := objects[name]
		require.Falsef(t, duplicate, "%s %s was created twice", kind, name)
		objects[name] = kind
	}
	require.NoError(t, rows.Err())

	objectCounts := map[string]int{}
	for _, kind := range objects {
		objectCounts[kind]++
	}
	assert.Equal(t, map[string]int{
		"index":   100,
		"table":   84,
		"trigger": 56,
		"view":    6,
	}, objectCounts, "the consolidated baseline must retain every schema object")

	// A sample across every area of the schema. The exact object counts above
	// catch omissions; these names make a failure identify the missing feature.
	for name, kind := range map[string]string{
		"account_budget_treatment_versions":       "table",
		"transactions":                            "table",
		"posting_versions":                        "table",
		"current_transaction_versions":            "view",
		"budget_targets":                          "table",
		"cost_basis_profile_versions":             "table",
		"cost_basis_profiles_current_version_idx": "index",
		"import_rules":                            "table",
		"import_rule_tags":                        "table",
		"import_rule_tags_same_book":              "trigger",
		"investment_disposal_allocations":         "table",
		"investment_disposal_decisions":           "table",
		"investment_disposal_decisions_event_idx": "index",
		"investment_lots":                         "table",
		"investment_lots_side_position_idx":       "index",
		"investment_operations":                   "table",
		"investment_operations_same_book":         "trigger",
		"investment_operations_no_update":         "trigger",
		"investment_operations_no_delete":         "trigger",
		"investment_lot_events_transaction_idx":   "index",
		"investment_position_basis_state":         "table",
		"price_observations":                      "table",
		"background_work_items":                   "table",
		"authentication_events":                   "table",
		"login_trusted_devices":                   "table",
		"user_mfa_totp":                           "table",
		"user_mfa_recovery_codes":                 "table",
		"login_mfa_challenges":                    "table",
		"backup_policies":                         "table",
		"backup_runs":                             "table",
		"self_check_runs":                         "table",
		"self_check_results":                      "table",
		"recurring_templates":                     "table",
		"recurring_template_postings":             "table",
		"recurring_occurrences":                   "table",
		"recurring_occurrences_status_idx":        "index",
		"recurring_template_postings_same_book":   "trigger",
		"price_observations_voided_idx":           "index",
		"posting_versions_no_update":              "trigger",
		"posting_versions_account_version_valid":  "trigger",
	} {
		assert.Equalf(t, kind, objects[name], "%s %s is missing from the schema", kind, name)
	}

	// Columns formerly added by ALTER TABLE are part of the baseline definitions
	// now. Keep this list explicit so a later cleanup cannot silently lose one.
	for table, column := range map[string]string{
		"cost_basis_profiles":   "current_version_id",
		"investment_lot_events": "cost_basis_method",
		"price_observations":    "voided_audit_event_id",
		"recurring_occurrences": "last_audit_event_id",
		"recurring_templates":   "revision",
	} {
		var columnCount int
		require.NoError(t, database.QueryRowContext(ctx, `
			SELECT COUNT(*) FROM pragma_table_info(?) WHERE name = ?
		`, table, column).Scan(&columnCount))
		assert.Equalf(t, 1, columnCount, "%s.%s is missing from the schema", table, column)
	}
}
