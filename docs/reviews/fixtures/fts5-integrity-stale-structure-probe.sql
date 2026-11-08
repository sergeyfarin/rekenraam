-- Shell-only reproduction for #403 / #541 (sqlite3 CLI 3.37+ for .connection).
-- Run from an empty directory:  sqlite3 < fts5-integrity-stale-structure-probe.sql
-- Expected: ok / ok / ok. SQLite 3.53.4 and 3.54.1 print:
--   ok
--   fts5: checksum mismatch for table "t"
--   ok
.open probe-cli.db
CREATE VIRTUAL TABLE t USING fts5(x);
INSERT INTO t(x) VALUES('alpha beta');
.connection 1
.open probe-cli.db
PRAGMA integrity_check;
.connection 0
INSERT INTO t(x) VALUES('gamma delta');
.connection 1
PRAGMA integrity_check;
.open probe-cli.db
PRAGMA integrity_check;
