package db

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNativeSQLiteAuditFeatureAndExtensionBoundaries(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "native-audit.sqlite")
	database, err := Open(ctx, SQLiteFileURL(path))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, database.Close()) })
	require.NoError(t, Migrate(ctx, database))
	reader, err := OpenReadOnly(ctx, SQLiteFileURL(path))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, reader.Close()) })
	auxiliary, err := sql.Open(driverName, sqliteAuxiliaryDSN(path, true))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, auxiliary.Close()) })

	check := func(t *testing.T, pool *sql.DB) {
		t.Helper()
		for option, want := range map[string]int{"ENABLE_FTS3": 1, "ENABLE_FTS5": 1, "ENABLE_RTREE": 1, "ENABLE_SESSION": 0, "ENABLE_DBSTAT_VTAB": 0} {
			var got int
			require.NoError(t, pool.QueryRowContext(ctx, "SELECT sqlite_compileoption_used(?)", option).Scan(&got))
			assert.Equal(t, want, got, option)
		}
		var extensions int
		require.NoError(t, pool.QueryRowContext(ctx, `SELECT count(*) FROM pragma_function_list WHERE name IN ('readfile','writefile','sqlar_uncompress')`).Scan(&extensions))
		assert.Zero(t, extensions)
		require.NoError(t, pool.QueryRowContext(ctx, `SELECT count(*) FROM pragma_module_list WHERE name IN ('zipfile','sqlite_dbpage')`).Scan(&extensions))
		assert.Zero(t, extensions)
		var value any
		err := pool.QueryRowContext(ctx, "SELECT load_extension(?)", filepath.Join(t.TempDir(), "synthetic-not-a-library.so")).Scan(&value)
		require.ErrorContains(t, err, "not authorized", "refusal must happen before looking for a library")
		for _, malformed := range []string{`{"value":`, `{"value":"unterminated}`, "["} {
			err = pool.QueryRowContext(ctx, "SELECT json_extract(?, '$.value')", malformed).Scan(&value)
			require.ErrorContains(t, err, "malformed JSON")
		}
		var exact string
		require.NoError(t, pool.QueryRowContext(ctx, "SELECT json_extract(?, '$.value')", `{"value":"99999999999999999999999999999999999999"}`).Scan(&exact))
		assert.Equal(t, "99999999999999999999999999999999999999", exact, "the connection remains usable and exact text is preserved")
	}
	for name, pool := range map[string]*sql.DB{"writer": database, "reader": reader, "auxiliary": auxiliary} {
		t.Run(name, func(t *testing.T) { check(t, pool) })
		connection, err := pool.Conn(ctx)
		require.NoError(t, err)
		require.ErrorIs(t, connection.Raw(func(any) error { return driver.ErrBadConn }), driver.ErrBadConn)
		_ = connection.Close() // Raw's ErrBadConn already discarded this physical connection.
		t.Run(name+"_replacement", func(t *testing.T) { check(t, pool) })
	}
}
