package db_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"rekenraam/backend/internal/db"
	"rekenraam/backend/internal/testdb"
)

// #403: the server's read pool outlives every self-check run. A connection
// that already ran integrity_check must not report FTS5 damage the file does
// not have after the writer adds search rows, and real damage must still fail.
func TestSQLiteIntegrityOnAReusedReaderAfterSearchWrites(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	writer, databaseURL := testdb.Open(t)
	readOnly, err := db.OpenReadOnly(ctx, databaseURL)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, readOnly.Close()) })
	readOnly.SetMaxOpenConns(1) // every step below uses the same reader connection
	repository := db.NewSelfCheckRepository(writer, readOnly)
	insertSearchRow := func(id int) {
		t.Helper()
		_, err := writer.ExecContext(ctx, `INSERT INTO transaction_search
			(transaction_version_id, transaction_id, payee_name, description, external_ref_hint, note_markdown)
			VALUES (?, ?, 'Payee', 'Groceries', '', '')`, id, id)
		require.NoError(t, err)
	}

	for run := 1; run <= 3; run++ {
		outcome, err := repository.SQLiteIntegrity(ctx)
		require.NoError(t, err)
		require.Equal(t, "ok", outcome, "run %d", run)
		insertSearchRow(run)
	}

	// Canary: the bare pragma on this connection still sees the stale FTS5
	// structure, so the runs above did exercise the case. If a SQLite upgrade
	// fixes the engine, this fails and the refresh can be reconsidered; rerun
	// docs/reviews/fixtures/fts5-integrity-stale-structure-probe.c.txt too (#541).
	var bare string
	require.NoError(t, readOnly.QueryRowContext(ctx, `PRAGMA integrity_check`).Scan(&bare))
	insertSearchRow(4)
	require.NoError(t, readOnly.QueryRowContext(ctx, `PRAGMA integrity_check`).Scan(&bare))
	require.Contains(t, bare, `fts5: checksum mismatch for table "transaction_search"`)

	outcome, err := repository.SQLiteIntegrity(ctx)
	require.NoError(t, err)
	require.Equal(t, "ok", outcome)

	_, err = writer.ExecContext(ctx, `DELETE FROM transaction_search_docsize WHERE id = 2`)
	require.NoError(t, err)
	outcome, err = repository.SQLiteIntegrity(ctx)
	require.NoError(t, err)
	require.NotEqual(t, "ok", outcome, "real FTS5 damage still fails")
	require.Contains(t, outcome, "transaction_search")
}
