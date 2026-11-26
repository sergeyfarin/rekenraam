package db

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Tool consumer for scripts/build-release-seed.sh. Installed fixtures remain
// frozen; this validates a proposed new fixture through the actual baseline.
func TestValidateReleaseSeedFixture(t *testing.T) {
	t.Parallel()
	path := os.Getenv("SEED_INPUT")
	if path == "" {
		t.Skip("SEED_INPUT is unset; invoked by scripts/build-release-seed.sh")
	}
	seed, err := os.ReadFile(path)
	require.NoError(t, err)
	database := openTestDatabase(t)
	require.NoError(t, Migrate(context.Background(), database))
	loadReleaseSeed(t, database, seed)
	before := captureLedgerState(t, database)
	if sourcePath := os.Getenv("SEED_SOURCE"); sourcePath != "" {
		source, err := OpenReadOnly(context.Background(), SQLiteFileURL(sourcePath))
		require.NoError(t, err)
		defer source.Close()
		assertReleaseSeedStatePreserved(t, captureLedgerState(t, source), database)
	}
	require.NoError(t, Migrate(context.Background(), database))
	assertReleaseSeedStatePreserved(t, before, database)
	assertUpgradedBookIsIntact(t, database)
}

func TestLedgerSnapshotPreservesTypesAndDuplicateRows(t *testing.T) {
	t.Parallel()
	database := openTestDatabase(t)
	_, err := database.Exec(`
		CREATE TABLE "audit""payload" ("key.part" TEXT PRIMARY KEY, data BLOB) WITHOUT ROWID;
		INSERT INTO "audit""payload" VALUES ('test', X'610062');
		CREATE TABLE duplicates (data TEXT);
		INSERT INTO duplicates VALUES ('same'), ('same');
	`)
	require.NoError(t, err)
	before := captureLedgerState(t, database)
	require.Len(t, before[`audit"payload`].rows, 1)
	require.Len(t, before["duplicates"].rows, 2)

	_, err = database.Exec(`
		ALTER TABLE "audit""payload" ADD COLUMN later TEXT NOT NULL DEFAULT 'new';
		CREATE TABLE later_table (id INTEGER PRIMARY KEY);
	`)
	require.NoError(t, err)
	assertReleaseSeedStatePreserved(t, before, database)

	_, err = database.Exec(`
		UPDATE "audit""payload" SET data = CAST(data AS TEXT);
		DELETE FROM duplicates WHERE rowid = 1;
	`)
	require.NoError(t, err)
	for _, table := range []string{`audit"payload`, "duplicates"} {
		old := before[table]
		require.NotEqual(t, old.rows, readLedgerRows(t, database, table, old.columns, ""), table)
	}
}

type ledgerTableState struct {
	columns []string
	rows    []string
}

// Capture application tables, including virtual tables, but skip SQLite's
// internal and shadow tables and Goose's migration bookkeeping.
func ledgerTableNames(t *testing.T, database *sql.DB) []string {
	t.Helper()
	return readLedgerNames(t, database, `SELECT name FROM pragma_table_list
		WHERE schema = 'main' AND type IN ('table', 'virtual')
		AND name NOT GLOB 'sqlite_*' AND name <> 'goose_db_version'
		ORDER BY name`)
}

func captureLedgerState(t *testing.T, database *sql.DB) map[string]ledgerTableState {
	t.Helper()
	state := make(map[string]ledgerTableState)
	for _, name := range ledgerTableNames(t, database) {
		columns := readLedgerNames(t, database, `SELECT name FROM pragma_table_xinfo(?, 'main')
			WHERE hidden <> 1 ORDER BY cid`, name)
		state[name] = ledgerTableState{columns: columns, rows: readLedgerRows(t, database, name, columns, "")}
	}
	return state
}

func readLedgerNames(t *testing.T, database *sql.DB, query string, args ...any) []string {
	t.Helper()
	rows, err := database.Query(query, args...)
	require.NoError(t, err)
	defer rows.Close()
	var names []string
	for rows.Next() {
		var name string
		require.NoError(t, rows.Scan(&name))
		names = append(names, name)
	}
	require.NoError(t, rows.Err())
	return names
}

func quoteLedgerIdentifier(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

func readLedgerRows(t *testing.T, database *sql.DB, table string, columns []string, where string, args ...any) []string {
	t.Helper()
	fields := make([]string, len(columns))
	values := make([]any, len(columns))
	destinations := make([]any, len(columns))
	for i, column := range columns {
		// Unary plus preserves SQLite's value but removes its declared type,
		// so the driver does not normalize dates or booleans while scanning.
		// Qualifying the column makes a removed column fail even on empty tables.
		fields[i] = `+data.` + quoteLedgerIdentifier(column)
		destinations[i] = &values[i]
	}
	query := `SELECT ` + strings.Join(fields, ", ") + ` FROM main.` + quoteLedgerIdentifier(table) + " AS data" + where
	rows, err := database.Query(query, args...)
	require.NoError(t, err, "read %s columns %v", table, columns)
	defer rows.Close()
	var snapshot []string
	for rows.Next() {
		require.NoError(t, rows.Scan(destinations...))
		for i, value := range values {
			fields[i] = fmt.Sprintf("%T:%#v", value, value)
		}
		snapshot = append(snapshot, strings.Join(fields, ", "))
	}
	require.NoError(t, rows.Err())
	// Compare a multiset of rows. A rebuild can reorder rows and some tables
	// have no rowid. Sorting retains duplicate rows without relying on either.
	slices.Sort(snapshot)
	return snapshot
}

func assertReleaseSeedStatePreserved(t *testing.T, before map[string]ledgerTableState, database *sql.DB) {
	t.Helper()
	tables := ledgerTableNames(t, database)
	for _, name := range tables {
		if old, existed := before[name]; existed {
			assert.Equal(t, old.rows, readLedgerRows(t, database, name, old.columns, ""),
				"stored rows in %s, columns %v", name, old.columns)
		} else {
			var hasRows bool
			require.NoError(t, database.QueryRow(`SELECT EXISTS (SELECT 1 FROM main.`+quoteLedgerIdentifier(name)+")").Scan(&hasRows))
			assert.False(t, hasRows, "new table %s must start empty", name)
		}
	}
	for name := range before {
		assert.Contains(t, tables, name, "the upgrade removed table %s", name)
	}
}
