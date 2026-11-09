-- Shell-only reproduction for #403 / #541 (sqlite3 CLI 3.37+ for .connection).
-- Run:  sqlite3 < fts5-integrity-stale-structure-probe.sql
-- It creates or overwrites probe-cli.db in the current directory (.open --new
-- truncates it), so it can be rerun in place.
-- Expected: ok / ok / ok. SQLite 3.53.4 and 3.54.1 print:
--   ok
--   fts5: checksum mismatch for table "t"
--   ok
.open --new probe-cli.db
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
