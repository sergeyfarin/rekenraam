package db

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"testing"
	"testing/fstest"

	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rekenraam/backend/migrations"
)

// #573: the single runtime book keeps id 1 through two triggers instead of a
// CHECK, so a later scope change can drop the rule in an ordinary migration.

const (
	bookInsertWithID = `INSERT INTO books (id, owner_user_id, code, name, updated_by_user_id, created_at, updated_at)
		VALUES (?, 1, ?, 'Book', 1, '2026-06-01T00:00:00Z', '2026-06-01T00:00:00Z')`
	bookInsertWithoutID = `INSERT INTO books (owner_user_id, code, name, updated_by_user_id, created_at, updated_at)
		VALUES (1, ?, 'Book', 1, '2026-06-01T00:00:00Z', '2026-06-01T00:00:00Z')`
)

func TestBooksKeepTheSingleBookAtIDOne(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	database := openTestDatabase(t)
	require.NoError(t, Migrate(ctx, database))
	_, err := database.ExecContext(ctx, `
		INSERT INTO users (id, username, password_hash, is_owner, created_at, updated_at)
		VALUES (1, 'owner', 'hash', 1, '2026-06-01T00:00:00Z', '2026-06-01T00:00:00Z')`)
	require.NoError(t, err)

	bookIDs := func(t *testing.T, tx *sql.Tx) []int64 {
		t.Helper()
		rows, err := tx.QueryContext(ctx, `SELECT id FROM books ORDER BY id`)
		require.NoError(t, err)
		defer rows.Close()
		ids := []int64{}
		for rows.Next() {
			var id int64
			require.NoError(t, rows.Scan(&id))
			ids = append(ids, id)
		}
		require.NoError(t, rows.Err())
		return ids
	}

	const (
		refusedInsert = "books_single_id_insert: the single book must have id 1"
		refusedUpdate = "books_id_no_update: a book id cannot change"
	)
	// Each case runs in its own rolled-back transaction on one migrated file.
	for _, test := range []struct {
		name      string
		seed      bool
		statement string
		args      []any
		refuse    string // empty means the statement is accepted
		want      []int64
	}{
		{name: "first book with id 1 is accepted", statement: bookInsertWithID, args: []any{1, "first"}, want: []int64{1}},
		// AFTER INSERT sees the stored id: an omitted id on an empty table
		// becomes 1, exactly as the former CHECK (id = 1) allowed.
		{name: "first book with an omitted id becomes id 1", statement: bookInsertWithoutID, args: []any{"first"}, want: []int64{1}},
		// A guard that refused only a second book would accept this, and
		// CurrentBook would never find it.
		{name: "first book with id 2 is refused", statement: bookInsertWithID, args: []any{2, "first"}, refuse: refusedInsert, want: []int64{}},
		{name: "first book inserted through rowid 2 is refused", statement: `INSERT INTO books (rowid, owner_user_id, code, name, updated_by_user_id, created_at, updated_at)
			VALUES (2, 1, 'first', 'Book', 1, '2026-06-01T00:00:00Z', '2026-06-01T00:00:00Z')`, refuse: refusedInsert, want: []int64{}},
		{name: "second book with id 1 is refused by the primary key", seed: true, statement: bookInsertWithID, args: []any{1, "second"}, refuse: "UNIQUE constraint failed: books.id", want: []int64{1}},
		{name: "second book with id 2 is refused", seed: true, statement: bookInsertWithID, args: []any{2, "second"}, refuse: refusedInsert, want: []int64{1}},
		{name: "second book with an omitted id is refused", seed: true, statement: bookInsertWithoutID, args: []any{"second"}, refuse: refusedInsert, want: []int64{1}},
		{name: "changing the book id is refused", seed: true, statement: `UPDATE books SET id = 2 WHERE id = 1`, refuse: refusedUpdate, want: []int64{1}},
		// id is the rowid alias, which an UPDATE OF id trigger would miss.
		{name: "changing the book id through rowid is refused", seed: true, statement: `UPDATE books SET rowid = 2 WHERE id = 1`, refuse: refusedUpdate, want: []int64{1}},
		{name: "changing the book id through oid is refused", seed: true, statement: `UPDATE books SET oid = 2 WHERE id = 1`, refuse: refusedUpdate, want: []int64{1}},
		{name: "rewriting the same id and other columns is accepted", seed: true, statement: `UPDATE books SET id = 1, name = 'Renamed' WHERE id = 1`, want: []int64{1}},
	} {
		t.Run(test.name, func(t *testing.T) {
			tx, err := database.BeginTx(ctx, nil)
			require.NoError(t, err)
			defer func() { require.NoError(t, tx.Rollback()) }()
			if test.seed {
				_, err := tx.ExecContext(ctx, bookInsertWithID, 1, "first")
				require.NoError(t, err)
			}
			_, err = tx.ExecContext(ctx, test.statement, test.args...)
			if test.refuse == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, test.refuse)
			}
			assert.Equal(t, test.want, bookIDs(t, tx))
		})
	}
}

// A later scope change can drop the single-book rule in an ordinary,
// transactional Goose migration on a populated book: no table rebuild, no
// NO TRANSACTION, nothing else in the book touched. The migration exists only
// in this test and is never shipped.
func TestLaterMigrationCanDropTheSingleBookTriggersInATransaction(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	seeded := seededBaselineDatabase(t)
	require.NoError(t, Migrate(ctx, seeded))
	head, err := EmbeddedMigrationVersion()
	require.NoError(t, err)
	before := captureLedgerState(t, seeded)

	withLaterMigration := func(body string) fs.FS {
		chain := fstest.MapFS{}
		names, err := fs.Glob(migrations.FS, "*.sql")
		require.NoError(t, err)
		for _, name := range names {
			contents, err := migrations.FS.ReadFile(name)
			require.NoError(t, err)
			chain[name] = &fstest.MapFile{Data: contents}
		}
		chain[fmt.Sprintf("%04d_drop_single_book_triggers.sql", head+1)] = &fstest.MapFile{Data: []byte(
			"-- +goose Up\nDROP TRIGGER books_single_id_insert;\nDROP TRIGGER books_id_no_update;\n" + body +
				"-- +goose Down\nSELECT 1;\n")}
		return chain
	}
	guardsPresent := func() []bool {
		return []bool{
			sqliteObjectExists(t, seeded, "trigger", "books_single_id_insert"),
			sqliteObjectExists(t, seeded, "trigger", "books_id_no_update"),
		}
	}
	appliedVersion := func() int64 {
		var version int64
		require.NoError(t, seeded.QueryRowContext(ctx, `SELECT MAX(version_id) FROM goose_db_version WHERE is_applied = 1`).Scan(&version))
		return version
	}

	// A failure after both drops rolls them back: the migration really runs
	// inside one transaction.
	failing, err := goose.NewProvider(goose.DialectSQLite3, seeded, withLaterMigration("INSERT INTO no_such_table VALUES (1);\n"))
	require.NoError(t, err)
	_, err = failing.Up(ctx)
	require.ErrorContains(t, err, "no_such_table")
	assert.Equal(t, []bool{true, true}, guardsPresent())
	assert.Equal(t, head, appliedVersion())

	provider, err := goose.NewProvider(goose.DialectSQLite3, seeded, withLaterMigration(""))
	require.NoError(t, err)
	_, err = provider.Up(ctx)
	require.NoError(t, err)
	assert.Equal(t, []bool{false, false}, guardsPresent())
	assert.Equal(t, head+1, appliedVersion())
	assert.Equal(t, before, captureLedgerState(t, seeded), "dropping the triggers must not change a durable figure")
	assertUpgradedBookIsIntact(t, seeded)

	// The rule is gone; only the primary key is left.
	_, err = seeded.ExecContext(ctx, bookInsertWithID, 2, "second")
	require.NoError(t, err)
}
