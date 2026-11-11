package db

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBaselineIndexesKeepOrderedAndForeignKeyLookups(t *testing.T) {
	t.Parallel()
	database := openTestDatabase(t)
	require.NoError(t, Migrate(context.Background(), database))
	for _, test := range []struct{ name, query, index string }{
		{"journal order", "SELECT id FROM journal_entries WHERE transaction_version_id=1 ORDER BY entry_seq", "sqlite_autoindex_journal_entries_1"},
		{"posting order", "SELECT id FROM posting_versions WHERE journal_entry_id=1 ORDER BY line_seq", "sqlite_autoindex_posting_versions_1"},
		{"posting version", "SELECT posting_line_id FROM posting_versions WHERE transaction_version_id=1", "posting_versions_line_once_per_version_idx"},
		{"instruments", "SELECT commodity_id FROM investment_instruments WHERE book_id=1", "sqlite_autoindex_investment_instruments_1"},
		{"staged row order", "SELECT id FROM import_staged_rows WHERE batch_id=1 ORDER BY row_index", "sqlite_autoindex_import_staged_rows_1"},
		{"connection holdings", "SELECT commodity_id FROM import_connection_holdings WHERE connection_id=1", "sqlite_autoindex_import_connection_holdings_1"},
		{"commodity_versions backwards sequence", "SELECT id FROM commodity_versions WHERE commodity_id=1 ORDER BY version_seq DESC", "sqlite_autoindex_commodity_versions_1"},
		{"institution_versions backwards sequence", "SELECT id FROM institution_versions WHERE institution_id=1 ORDER BY version_seq DESC", "sqlite_autoindex_institution_versions_1"},
		{"account_versions backwards sequence", "SELECT id FROM account_versions WHERE account_id=1 ORDER BY version_seq DESC", "sqlite_autoindex_account_versions_1"},
		{"payee_versions backwards sequence", "SELECT id FROM payee_versions WHERE payee_id=1 ORDER BY version_seq DESC", "sqlite_autoindex_payee_versions_1"},
		{"transaction_versions backwards sequence", "SELECT id FROM transaction_versions WHERE transaction_id=1 ORDER BY version_seq DESC", "sqlite_autoindex_transaction_versions_1"},
		{"investment_instrument_versions backwards sequence", "SELECT id FROM investment_instrument_versions WHERE instrument_id=1 ORDER BY version_seq DESC", "sqlite_autoindex_investment_instrument_versions_1"},
		{"split facts by instrument and date", "SELECT operation_id FROM investment_split_facts WHERE book_id=1 AND commodity_id=1 AND effective_on > '2026-01-01' ORDER BY effective_on", "investment_split_facts_lookup_idx"},
	} {
		t.Run(test.name, func(t *testing.T) {
			// Force the retained key to prove its capability independently of
			// planner tie-breaking, then check the real unforced access path.
			forced := strings.Replace(test.query, " WHERE", " INDEXED BY "+test.index+" WHERE", 1)
			for _, query := range []string{forced, test.query} {
				rows, err := database.Query("EXPLAIN QUERY PLAN " + query)
				require.NoError(t, err)
				var plans []string
				for rows.Next() {
					var id, parent, unused int
					var detail string
					require.NoError(t, rows.Scan(&id, &parent, &unused, &detail))
					plans = append(plans, detail)
				}
				require.NoError(t, rows.Err())
				require.NoError(t, rows.Close())
				require.Len(t, plans, 1)
				assert.Contains(t, plans[0], "SEARCH")
				assert.NotContains(t, plans[0], "TEMP B-TREE")
				if query == forced {
					assert.Contains(t, plans[0], test.index)
				}
			}

		})
	}
}

func TestMigrateRefusesIncompatibleDevelopmentBaselinesWithoutWrites(t *testing.T) {
	t.Parallel()
	for _, version := range []int{1, 26, 27} {
		t.Run(strconv.Itoa(version), func(t *testing.T) {
			database := openTestDatabase(t)
			require.NoError(t, Migrate(context.Background(), database))
			_, err := database.Exec(`PRAGMA user_version=0; DELETE FROM goose_db_version; INSERT INTO goose_db_version(version_id,is_applied) VALUES(0,1); INSERT INTO goose_db_version(version_id,is_applied) VALUES(?,1)`, version)
			require.NoError(t, err)
			_, err = database.Exec(`CREATE TABLE preserved(value TEXT); INSERT INTO preserved VALUES('owner data')`)
			require.NoError(t, err)
			before := schemaFingerprint(t, database)
			err = Migrate(context.Background(), database)
			require.ErrorContains(t, err, "incompatible pre-release baseline")
			require.ErrorContains(t, err, "reset")
			assert.Equal(t, before, schemaFingerprint(t, database))
			var retainedVersion int
			require.NoError(t, database.QueryRow(`SELECT MAX(version_id) FROM goose_db_version`).Scan(&retainedVersion))
			assert.Equal(t, version, retainedVersion)
			var retained string
			require.NoError(t, database.QueryRow(`SELECT value FROM preserved`).Scan(&retained))
			assert.Equal(t, "owner data", retained)
		})
	}
}

func TestRestoreRefusesIncompatibleBaselineBeforePreservingDestination(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	database := openTestDatabase(t)
	require.NoError(t, Migrate(ctx, database))
	_, err := database.Exec(`PRAGMA user_version=0`)
	require.NoError(t, err)
	source := filepath.Join(t.TempDir(), "old.sqlite")
	require.NoError(t, BackupSQLiteDatabase(ctx, database, source))
	destination := filepath.Join(t.TempDir(), "live.sqlite")
	live, err := Open(ctx, SQLiteFileURL(destination))
	require.NoError(t, err)
	require.NoError(t, Migrate(ctx, live))
	require.NoError(t, live.Close())
	before, err := os.ReadFile(destination)
	require.NoError(t, err)
	result, err := RestoreSQLiteDatabase(ctx, source, SQLiteFileURL(destination))
	require.ErrorContains(t, err, "incompatible pre-release baseline")
	assert.False(t, result.Installed)
	assert.Empty(t, result.PreservedFiles)
	assert.Empty(t, result.PreservedDir)
	after, err := os.ReadFile(destination)
	require.NoError(t, err)
	assert.Equal(t, before, after)
}

func TestMigrateResumesEmptyGooseHistory(t *testing.T) {
	t.Parallel()
	database := openTestDatabase(t)
	_, err := database.Exec(`CREATE TABLE goose_db_version (id INTEGER PRIMARY KEY,version_id INTEGER NOT NULL,is_applied BOOLEAN NOT NULL,tstamp TIMESTAMP DEFAULT CURRENT_TIMESTAMP);
INSERT INTO goose_db_version(version_id,is_applied) VALUES(0,1)`)
	require.NoError(t, err)
	require.NoError(t, Migrate(context.Background(), database))
	var identity int
	require.NoError(t, database.QueryRow(`PRAGMA user_version`).Scan(&identity))
	assert.Equal(t, BaselineIdentity, identity)
}

func TestMigrateRefusesUnversionedPopulatedFilesWithoutCreatingHistory(t *testing.T) {
	t.Parallel()
	database := openTestDatabase(t)
	_, err := database.Exec(`CREATE TABLE users(id INTEGER PRIMARY KEY, password_hash TEXT); INSERT INTO users VALUES(1,'retained')`)
	require.NoError(t, err)
	before := schemaFingerprint(t, database)
	require.ErrorIs(t, Migrate(context.Background(), database), ErrIncompatibleBaseline)
	require.Equal(t, before, schemaFingerprint(t, database))
	var value string
	require.NoError(t, database.QueryRow(`SELECT password_hash FROM users WHERE id=1`).Scan(&value))
	require.Equal(t, "retained", value)
}

func TestPricingSourceAssignmentGuardRefusesMissingCommodity(t *testing.T) {
	t.Parallel()
	database, ownerID, currencyID := migratedInvestmentTestDatabase(t)
	var sourceID int64
	require.NoError(t, database.QueryRow(`SELECT id FROM market_data_sources WHERE code='manual'`).Scan(&sourceID))
	for _, baseID := range []int64{0, currencyID} {
		_, err := database.Exec(`INSERT INTO pricing_source_assignments(book_id,base_commodity_id,quote_commodity_id,source_id,status,effective_from,created_at,created_by_user_id)
VALUES(1,?, ?, ?, 'active','2026-01-01','2026-01-01T00:00:00Z',?)`, baseID, currencyID, sourceID, ownerID)
		if baseID == 0 {
			require.ErrorContains(t, err, "pricing source assignment commodities must belong to one book")
		} else {
			require.NoError(t, err)
		}
	}
}

func TestBaselineTagLinksRefuseMissingOrArchivedTags(t *testing.T) {
	t.Parallel()
	database := seedReplayTestBook(t)
	var tagID int64
	require.NoError(t, database.QueryRow(`SELECT id FROM tags WHERE status='active' ORDER BY id LIMIT 1`).Scan(&tagID))
	for _, test := range []struct{ table, parent, key, message string }{
		{"transaction_tags", "transactions", "transaction_id", "transaction tag must reference an active tag in the same book"},
		{"posting_tags", "posting_lines", "posting_line_id", "posting tag must reference an active tag in the same book"},
	} {
		t.Run(test.table, func(t *testing.T) {
			var parentID int64
			require.NoError(t, database.QueryRow("SELECT id FROM "+test.parent+" p WHERE NOT EXISTS(SELECT 1 FROM "+test.table+" t WHERE t."+test.key+"=p.id AND t.tag_id=?) ORDER BY id LIMIT 1", tagID).Scan(&parentID))
			insert := "INSERT INTO " + test.table + "(book_id," + test.key + ",tag_id,created_at,created_by_user_id) VALUES(1,?,?,'2026-01-01T00:00:00Z',1)"
			before := captureLedgerState(t, database)
			_, err := database.Exec(insert, parentID, 0)
			require.ErrorContains(t, err, test.message)
			require.Equal(t, before, captureLedgerState(t, database))
			_, err = database.Exec(insert, parentID, tagID)
			require.NoError(t, err)
			tx, err := database.BeginTx(context.Background(), nil)
			require.NoError(t, err)
			_, err = tx.Exec(`UPDATE tags SET status='archived' WHERE id=?`, tagID)
			require.NoError(t, err)
			_, err = tx.Exec(insert, parentID, tagID)
			require.ErrorContains(t, err, test.message)
			require.NoError(t, tx.Rollback())
		})
	}
}
