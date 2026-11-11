package db

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	sqlite "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rekenraam/backend/internal/exact"
)

func TestNativeSQLiteEngineFeaturesAndFingerprint(t *testing.T) {
	t.Parallel()
	database := openTestDatabase(t)
	var version, sourceID string
	require.NoError(t, database.QueryRow("SELECT sqlite_version(),sqlite_source_id()").Scan(&version, &sourceID))
	assert.Equal(t, "3.54.1", version, "admit engine changes together with the pinned driver")
	assert.Equal(t, "2026-10-16 14:45:20 555e07ef33f69a0957d24d8e555b702464a131d6f1607f4c3c1e4551e8e6ca76", sourceID)
	rows, err := database.Query("PRAGMA compile_options")
	require.NoError(t, err)
	var options []string
	for rows.Next() {
		var option string
		require.NoError(t, rows.Scan(&option))
		options = append(options, option)
	}
	require.NoError(t, rows.Err())
	require.NoError(t, rows.Close())
	assert.Contains(t, options, "ENABLE_FTS5")
	assert.Contains(t, options, "THREADSAFE=1")
	assert.Contains(t, options, "MAX_SCHEMA=10000000", "3.54.1 adds this compile option")
	assert.Contains(t, options, "ENABLE_QPSG", "build through scripts/native-sqlite-env.sh (ADR 0014)")
	fingerprint, err := json.Marshal(map[string]any{"version": version, "source_id": sourceID, "compile_options": options})
	require.NoError(t, err)
	t.Log("SQLite fingerprint:", string(fingerprint))
}

// SQLite 3.54.1 marks a joined statement for re-preparation when a bound
// equality takes part in constant propagation, so the first bind recompiles it.
// The QPSG build keeps one compile per statement, as 3.53.4 did.
func TestNativeBoundJoinedEqualityCompilesOnce(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	database := openTestDatabase(t)
	database.SetMaxOpenConns(1)
	_, err := database.ExecContext(ctx, `CREATE TABLE qpsg_parent(id INTEGER PRIMARY KEY, book_id INTEGER NOT NULL);
		CREATE TABLE qpsg_child(id INTEGER PRIMARY KEY, parent_id INTEGER NOT NULL REFERENCES qpsg_parent(id), name TEXT NOT NULL);
		CREATE INDEX qpsg_child_parent_idx ON qpsg_child(parent_id);
		INSERT INTO qpsg_parent VALUES (1, 1), (2, 1), (3, 2);
		INSERT INTO qpsg_child VALUES (10, 1, 'a'), (11, 2, 'b'), (12, 3, 'c')`)
	require.NoError(t, err)
	conn, err := database.Conn(ctx)
	require.NoError(t, err)
	defer conn.Close()
	var compiles int
	require.NoError(t, conn.Raw(func(raw any) error {
		raw.(*sqlite.SQLiteConn).RegisterAuthorizer(func(operation int, _, _, _ string) int {
			if operation == sqlite.SQLITE_SELECT {
				compiles++
			}
			return sqlite.SQLITE_OK
		})
		return nil
	}))
	defer func() {
		require.NoError(t, conn.Raw(func(raw any) error { raw.(*sqlite.SQLiteConn).RegisterAuthorizer(nil); return nil }))
	}()
	for _, lookup := range []struct {
		query string
		args  []any
		want  []string
	}{
		{`SELECT c.name FROM qpsg_parent p JOIN qpsg_child c ON c.parent_id = p.id WHERE p.id = ? ORDER BY c.id`, []any{2}, []string{"b"}},
		{`SELECT c.name FROM qpsg_parent p JOIN qpsg_child c ON c.parent_id = p.id WHERE p.book_id = ? AND p.id IN (?, ?) ORDER BY c.id`, []any{1, 1, 2}, []string{"a", "b"}},
	} {
		compiles = 0
		rows, err := conn.QueryContext(ctx, lookup.query, lookup.args...)
		require.NoError(t, err)
		var names []string
		for rows.Next() {
			var name string
			require.NoError(t, rows.Scan(&name))
			names = append(names, name)
		}
		require.NoError(t, rows.Err())
		require.NoError(t, rows.Close())
		assert.Equal(t, lookup.want, names, lookup.query)
		assert.Equal(t, 1, compiles, "one compile, no re-prepare on first bind: %s", lookup.query)
	}
}

func TestNativeConnectionPoliciesSurviveReplacement(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	uri := SQLiteFileURL(filepath.Join(t.TempDir(), "policies.sqlite")) + "?cache=private&_fk=0&_foreign_keys=0&_timeout=1&_sync=OFF&_journal=DELETE&_txlock=exclusive&_stmt_cache_size=100&_query_only=1&_pragma=foreign_keys(0)"
	writer, err := Open(ctx, uri)
	require.NoError(t, err)
	defer writer.Close()
	_, err = writer.ExecContext(ctx, "CREATE TABLE policy_test(id INTEGER PRIMARY KEY)")
	require.NoError(t, err)
	for range 2 {
		state, err := Check(ctx, writer)
		require.NoError(t, err)
		assert.Equal(t, 2, state.Synchronous, "acknowledged financial writes require FULL")
		conn, err := writer.Conn(ctx)
		require.NoError(t, err)
		require.ErrorIs(t, conn.Raw(func(any) error { return driver.ErrBadConn }), driver.ErrBadConn)
		_ = conn.Close()
	}
	reader, err := OpenReadOnly(ctx, uri)
	require.NoError(t, err)
	defer reader.Close()
	assert.Equal(t, 4, reader.Stats().MaxOpenConnections)
	for range 2 {
		var connections []*sql.Conn
		for range 4 {
			conn, err := reader.Conn(ctx)
			require.NoError(t, err)
			connections = append(connections, conn)
			var foreignKeys, busyTimeout, queryOnly, autocheckpoint, synchronous int
			require.NoError(t, conn.QueryRowContext(ctx, `SELECT foreign_keys,timeout,query_only FROM pragma_foreign_keys,pragma_busy_timeout,pragma_query_only`).Scan(&foreignKeys, &busyTimeout, &queryOnly))
			require.NoError(t, conn.QueryRowContext(ctx, "PRAGMA wal_autocheckpoint").Scan(&autocheckpoint))
			require.NoError(t, conn.QueryRowContext(ctx, "PRAGMA synchronous").Scan(&synchronous))
			assert.Equal(t, 1, foreignKeys)
			assert.Equal(t, 5000, busyTimeout)
			assert.Equal(t, 1, queryOnly)
			assert.Equal(t, 1000, autocheckpoint)
			assert.Equal(t, 2, synchronous)
			_, err = conn.ExecContext(ctx, "INSERT INTO policy_test DEFAULT VALUES")
			require.Error(t, err)
		}
		for _, conn := range connections {
			require.ErrorIs(t, conn.Raw(func(any) error { return driver.ErrBadConn }), driver.ErrBadConn)
			_ = conn.Close()
		}
	}
}

func TestNativeDriverRequiresFullSynchronousOnEveryConnection(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	// Direct opens must share the durability policy too, even when they bypass
	// the app DSN or attempt to supply the driver's weaker defaults.
	database, err := sql.Open(driverName, SQLiteFileURL(filepath.Join(t.TempDir(), "raw-sync.sqlite"))+"?_synchronous=OFF")
	require.NoError(t, err)
	defer database.Close()
	database.SetMaxOpenConns(1)
	for range 2 {
		conn, err := database.Conn(ctx)
		require.NoError(t, err)
		var synchronous int
		require.NoError(t, conn.QueryRowContext(ctx, "PRAGMA synchronous").Scan(&synchronous))
		assert.Equal(t, 2, synchronous)
		require.ErrorIs(t, conn.Raw(func(any) error { return driver.ErrBadConn }), driver.ErrBadConn)
		assert.ErrorIs(t, conn.Close(), sql.ErrConnDone)
	}
}

func TestNativeTransactionsRemainDeferred(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	uri := SQLiteFileURL(filepath.Join(t.TempDir(), "deferred.sqlite"))
	writer, err := Open(ctx, uri+"?_txlock=immediate")
	require.NoError(t, err)
	defer writer.Close()
	_, err = writer.ExecContext(ctx, "CREATE TABLE deferred_test(id INTEGER PRIMARY KEY)")
	require.NoError(t, err)
	other, err := Open(ctx, uri)
	require.NoError(t, err)
	defer other.Close()
	tx, err := writer.BeginTx(ctx, nil)
	require.NoError(t, err)
	defer tx.Rollback()
	_, err = other.ExecContext(ctx, "INSERT INTO deferred_test DEFAULT VALUES")
	require.NoError(t, err, "BEGIN must not acquire the write lock before the first write")
	var count int
	require.NoError(t, tx.QueryRowContext(ctx, "SELECT count(*) FROM deferred_test").Scan(&count))
	assert.Equal(t, 1, count)
}

func TestNativeCancellationLeavesConnectionUsable(t *testing.T) {
	t.Parallel()
	database := openTestDatabase(t)
	connection, err := database.Conn(context.Background())
	require.NoError(t, err)
	defer connection.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	var sum int64
	err = connection.QueryRowContext(ctx, `WITH RECURSIVE numbers(n) AS (VALUES(1) UNION ALL SELECT n+1 FROM numbers WHERE n<1000000000) SELECT sum(n) FROM numbers`).Scan(&sum)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	_, err = connection.ExecContext(context.Background(), "CREATE TABLE after_cancel(id INTEGER PRIMARY KEY)")
	require.NoError(t, err)
	result, err := connection.ExecContext(context.Background(), "INSERT INTO after_cancel DEFAULT VALUES")
	require.NoError(t, err)
	id, err := result.LastInsertId()
	require.NoError(t, err)
	assert.Equal(t, int64(1), id)
	count, err := result.RowsAffected()
	require.NoError(t, err)
	assert.Equal(t, int64(1), count)
}

func TestNativeValuesJSONAndFTSRemainCompatible(t *testing.T) {
	t.Parallel()
	database := openTestDatabase(t)
	ctx := context.Background()
	_, err := database.ExecContext(ctx, `CREATE TABLE native_values(coefficient TEXT,scale INTEGER,unknown TEXT,payload BLOB,day TEXT,stamp TEXT); CREATE VIRTUAL TABLE native_search USING fts5(body); INSERT INTO native_search VALUES('exact decimal accounting')`)
	require.NoError(t, err, "FTS5 must be enabled in tests and production")
	want := exact.MustParse("-99999999999999999999999999999999999999")
	blob := []byte{0, 1, 128, 255}
	_, err = database.ExecContext(ctx, "INSERT INTO native_values VALUES(?,?,?,?,?,?)", want, 24, nil, blob, "0001-01-01", "2026-10-12T01:02:03.123456789Z")
	require.NoError(t, err)
	var coefficient exact.Coefficient
	var scale int
	var unknown sql.NullString
	var payload []byte
	var day, stamp, kind string
	require.NoError(t, database.QueryRowContext(ctx, "SELECT coefficient,scale,unknown,payload,day,stamp,typeof(coefficient) FROM native_values").Scan(&coefficient, &scale, &unknown, &payload, &day, &stamp, &kind))
	assert.Equal(t, want, coefficient)
	assert.Equal(t, 24, scale)
	assert.False(t, unknown.Valid)
	assert.Equal(t, blob, payload)
	assert.Equal(t, "0001-01-01", day)
	assert.Equal(t, "2026-10-12T01:02:03.123456789Z", stamp)
	assert.Equal(t, "text", kind)
	var count int
	require.NoError(t, database.QueryRowContext(ctx, "SELECT count(*) FROM native_search WHERE native_search MATCH 'decimal'").Scan(&count))
	assert.Equal(t, 1, count)
	var jsonCoefficient string
	require.NoError(t, database.QueryRowContext(ctx, `SELECT json_extract(json_object('value',?), '$.value')`, want).Scan(&jsonCoefficient))
	assert.Equal(t, want.String(), jsonCoefficient)
}

func TestNativeSQLiteURIPunctuationAndBackup(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "ledger ?#%&' café.sqlite")
	uri := SQLiteFileURL(path)
	resolved, err := ResolveSQLiteFilePath(uri)
	require.NoError(t, err)
	assert.Equal(t, path, resolved)
	writer, err := Open(ctx, uri)
	require.NoError(t, err)
	defer writer.Close()
	_, err = writer.ExecContext(ctx, "CREATE TABLE punctuation(id INTEGER PRIMARY KEY); INSERT INTO punctuation VALUES(42)")
	require.NoError(t, err)
	reader, err := OpenReadOnly(ctx, uri)
	require.NoError(t, err)
	defer reader.Close()
	backup := filepath.Join(t.TempDir(), "copy ?#%&' café.sqlite")
	result, err := OnlineBackupSQLiteDatabase(ctx, reader, backup, OnlineBackupOptions{PagesPerStep: 1})
	require.NoError(t, err)
	assert.Positive(t, result.PageCount)
	require.NoError(t, VerifySQLiteBackup(ctx, backup))
	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
}

func TestNativeSQLiteRejectsUnsupportedModerncOption(t *testing.T) {
	t.Parallel()
	_, err := sqliteDSN("file:test.sqlite?_pragma=trusted_schema(0)", false)
	require.ErrorContains(t, err, "unsupported modernc")
}

func TestNativeConstraintClassificationUsesCodes(t *testing.T) {
	t.Parallel()
	database := openTestDatabase(t)
	_, err := database.Exec("CREATE TABLE constraints(id INTEGER PRIMARY KEY, name TEXT UNIQUE, amount INTEGER CHECK(amount>0)); INSERT INTO constraints VALUES(1,'first',1)")
	require.NoError(t, err)
	_, err = database.Exec("INSERT INTO constraints VALUES(2,'first',1)")
	require.Error(t, err)
	assert.True(t, IsSQLiteUniqueConstraint(fmt.Errorf("wrapped: %w", err)))
	_, err = database.Exec("INSERT INTO constraints VALUES(2,'second',0)")
	require.Error(t, err)
	assert.False(t, IsSQLiteUniqueConstraint(err))
	assert.False(t, IsSQLiteUniqueConstraint(errors.New("UNIQUE constraint failed")))
	assert.True(t, IsSQLiteBusy(fmt.Errorf("wrapped: %w", sqlite.Error{Code: sqlite.ErrBusy, ExtendedCode: sqlite.ErrBusySnapshot})))
	assert.True(t, IsSQLiteBusy(sqlite.Error{Code: sqlite.ErrLocked}))
	assert.False(t, IsSQLiteBusy(errors.New("database is locked")))
}

func TestNativeSQLiteFileURLPreservesRelativePaths(t *testing.T) {
	t.Parallel()
	uri := SQLiteFileURL("var/backup ?#%.sqlite")
	path, err := ResolveSQLiteFilePath(uri)
	require.NoError(t, err)
	assert.Equal(t, "var/backup ?#%.sqlite", path)
	assert.NotContains(t, uri, "file://var/")
}

func TestNativeReadOnlyRetainsFullSynchronous(t *testing.T) {
	t.Parallel()
	path := SQLiteFileURL(filepath.Join(t.TempDir(), "read-sync.sqlite"))
	writer, err := Open(context.Background(), path)
	require.NoError(t, err)
	defer writer.Close()
	reader, err := OpenReadOnly(context.Background(), path)
	require.NoError(t, err)
	defer reader.Close()
	var synchronous int
	require.NoError(t, reader.QueryRow("PRAGMA synchronous").Scan(&synchronous))
	assert.Equal(t, 2, synchronous, "retain modernc FULL for read-only/auxiliary connections")
}

func TestNativeAuxiliaryConnectionsRetainFullSynchronous(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "aux.sqlite")
	for _, readOnly := range []bool{false, true} {
		database, err := sql.Open(driverName, sqliteAuxiliaryDSN(path, readOnly))
		require.NoError(t, err)
		var synchronous int
		require.NoError(t, database.QueryRow("PRAGMA synchronous").Scan(&synchronous))
		assert.Equal(t, 2, synchronous, "backup and maintenance must retain FULL")
		require.NoError(t, database.Close())
	}
}
