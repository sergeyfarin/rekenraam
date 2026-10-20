package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rekenraam/backend/internal/db"
)

func TestNativeSQLiteAuditBackupRefusalsPreserveBothFiles(t *testing.T) {
	for _, kind := range []string{"truncated", "invalid_btree", "invalid_schema", "extension_view", "unbounded_view"} {
		t.Run(kind, func(t *testing.T) {
			databaseURL, backupPath := backupOf(t)
			databasePath, err := db.ResolveSQLiteFilePath(databaseURL)
			require.NoError(t, err)
			t.Setenv("DATABASE_URL", databaseURL)
			t.Setenv("APP_ENV", "development")
			data, err := os.ReadFile(backupPath)
			require.NoError(t, err)
			switch kind {
			case "truncated":
				require.NoError(t, os.WriteFile(backupPath, data[:100], 0o600))
			case "invalid_btree":
				data[100] = 0xff // Page one's b-tree header has no such page type.
				require.NoError(t, os.WriteFile(backupPath, data, 0o600))
			default:
				database, err := db.Open(context.Background(), db.SQLiteFileURL(backupPath))
				require.NoError(t, err)
				switch kind {
				case "invalid_schema":
					_, err = database.Exec(`PRAGMA writable_schema=ON; UPDATE sqlite_schema SET sql='CREATE TABLE ???' WHERE name='transactions'; PRAGMA writable_schema=OFF`)
				case "extension_view":
					_, err = database.Exec(`ALTER TABLE goose_db_version RENAME TO audit_saved_migrations;
					CREATE VIEW goose_db_version AS SELECT load_extension('synthetic-never-load.so') AS version_id`)
				case "unbounded_view":
					_, err = database.Exec(`ALTER TABLE goose_db_version RENAME TO audit_saved_migrations;
					CREATE VIEW goose_db_version AS WITH RECURSIVE n(x) AS (VALUES(0) UNION ALL SELECT x+1 FROM n) SELECT x AS version_id FROM n`)
				}
				require.NoError(t, err)
				require.NoError(t, database.Close())
			}
			liveBefore, err := os.ReadFile(databasePath)
			require.NoError(t, err)
			sourceBefore, err := os.ReadFile(backupPath)
			require.NoError(t, err)
			for _, command := range []string{"verify-backup", "restore"} {
				ctx, cancel := context.Background(), func() {}
				if kind == "unbounded_view" {
					ctx, cancel = context.WithTimeout(ctx, 200*time.Millisecond)
				}
				stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
				exitCode := run(ctx, []string{command, "--from", backupPath}, strings.NewReader(""), stdout, stderr)
				cancel()
				assert.Equal(t, 1, exitCode, "%s must refuse %s: stdout=%s stderr=%s", command, kind, stdout, stderr)
				if kind == "extension_view" {
					assert.Contains(t, stderr.String(), "unsafe use of load_extension()", "schema code must not reach the dynamic loader")
				}
				if kind == "unbounded_view" {
					assert.Contains(t, stderr.String(), "context deadline exceeded", "verification must honor cancellation while reading a crafted schema")
				}
				liveAfter, err := os.ReadFile(databasePath)
				require.NoError(t, err)
				sourceAfter, err := os.ReadFile(backupPath)
				require.NoError(t, err)
				assert.Equal(t, sha256.Sum256(liveBefore), sha256.Sum256(liveAfter), "live database must be untouched")
				assert.Equal(t, sha256.Sum256(sourceBefore), sha256.Sum256(sourceAfter), "read-only verification must preserve source bytes")
				preserved, err := filepath.Glob(databasePath + ".before-restore-*")
				require.NoError(t, err)
				assert.Empty(t, preserved, "refusal must precede preservation or replacement")
				assert.NoFileExists(t, databasePath+".restoring")
			}
		})
	}
}
