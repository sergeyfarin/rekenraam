# FTS5 integrity_check stale structure — standalone reproduction — 2026-11-08

[#541](https://github.com/sergeyfarin/rekenraam/issues/541) reduces the
[#403](https://github.com/sergeyfarin/rekenraam/issues/403) false finding to a
standalone reproduction that uses only official SQLite amalgamations, and
prepares the upstream report. **The engine bug is present in 3.53.4 (the
admitted driver) and 3.54.1 (the held candidate, #356), and on SQLite trunk
as of 2026-11-08.** `refreshFTS5StructuresTx` in
`backend/internal/db/self_check.go` stays load-bearing.

## The defect

`PRAGMA integrity_check` calls each virtual table's `xIntegrity` method. FTS5's
`fts5IntegrityMethod` checks the index against the connection's **cached**
structure record. It skips the reset that every other FTS5 entry point makes
at the start of a transaction: `fts5BeginMethod` and `fts5OpenMethod` call
`fts5NewTransaction()`, which calls `sqlite3Fts5StorageReset()` when no cursor
is open. Once a connection has loaded the structure, a write to the FTS5 table
from **another** connection leaves that cache stale. The reused connection's
next `integrity_check` then reports

```text
fts5: checksum mismatch for table "t"
```

for a file that is not damaged: a fresh connection reports `ok`, and so does
FTS5's own `INSERT INTO t(t) VALUES('integrity-check')` command on the same
connection. Any ordinary read of the table first (for example
`SELECT rowid FROM t LIMIT 1`) refreshes the cache and the false finding
disappears. That is the application's workaround.

The journal mode does not matter: WAL and rollback (`delete`) behave the same.

## Retained reproductions

| File | What it is |
|---|---|
| [`fixtures/fts5-integrity-stale-structure-probe.c.txt`](fixtures/fts5-integrity-stale-structure-probe.c.txt) | Self-checking C probe: two connections, three `integrity_check` runs with a write between each, fresh-connection and FTS5-command controls, then a real-damage control (a deleted `t_docsize` row). Exits 1 on a false finding or a missed damage finding. Optional `refresh` argument applies the application's workaround. |
| [`fixtures/fts5-integrity-stale-structure-probe.sql`](fixtures/fts5-integrity-stale-structure-probe.sql) | Shell-only version for the `sqlite3` CLI (`.connection`, 3.37+). This is the form used in the upstream report. |

## Results

Official amalgamations, built with
`cc -O1 -DSQLITE_THREADSAFE=1 -DSQLITE_ENABLE_FTS5`. The 3.54.1 `sqlite3.c`
has SHA3-256 `e2b869d3…b558ee81`, matching the
[release log](https://sqlite.org/releaselog/3_54_1.html).

| Engine | Mode | No refresh | `refresh` |
|---|---|---|---|
| 3.53.4 | wal / delete | **FAIL**: runs 2–3 `checksum mismatch` | PASS |
| 3.54.1 | wal / delete | **FAIL**: runs 2–3 `checksum mismatch` | PASS |
| 3.54.1 + candidate patch | wal / delete | PASS | PASS |

In every row, the fresh connection and the FTS5 `'integrity-check'` command
report `ok`. The damage control is reported (`malformed inverted index for
FTS5 table main.t`), including with the refresh and with the patch. The CLI
script prints `ok / checksum mismatch / ok` on 3.53.4 and 3.54.1, and
`ok / ok / ok` with the patch.

The candidate patch is one line in `fts5IntegrityMethod`, mirroring
`fts5BeginMethod`:

```diff
   pTab->p.pConfig->pzErrmsg = pzErr;
-  rc = sqlite3Fts5StorageIntegrity(pTab->pStorage, 0);
+  rc = fts5NewTransaction(pTab);
+  if( rc==SQLITE_OK ) rc = sqlite3Fts5StorageIntegrity(pTab->pStorage, 0);
```

This is evidence for the report, not something the application carries. The
application never patches the bundled engine (ADR 0014).

**Trunk.** `ext/fts5/fts5_main.c` on the SQLite GitHub mirror at `81ca4985`
(2026-11-08) has the same `fts5IntegrityMethod` body. The three most recent
FTS5 commits (`014644d8`, `2bcdb878`, `bedc3187`) do not touch this path.
Trunk was inspected, not built.

**Existing reports.** #403 found none. The SQLite forum and Fossil timeline
sit behind a browser check, so this review could not search them again
automatically. Whoever posts the report should search the forum for
`fts5 checksum mismatch integrity_check` first.

## Upstream report

Status: **drafted, awaiting owner approval** (#541). Once it is posted, the
thread link goes on #541 and in this section.

Draft for the [SQLite forum](https://sqlite.org/forum/forum):

> **Title:** FTS5: PRAGMA integrity_check reports false "checksum mismatch" on a connection whose cached structure is stale
>
> On a connection that has already loaded an FTS5 table's structure, `PRAGMA
> integrity_check` reports `fts5: checksum mismatch for table "t"` after
> another connection writes to that table. The database is fine: a fresh
> connection reports `ok`, and so does `INSERT INTO t(t)
> VALUES('integrity-check')` on the same connection. Seen with 3.53.4 and
> 3.54.1, in WAL and rollback mode.
>
> ```
> .open probe-cli.db
> CREATE VIRTUAL TABLE t USING fts5(x);
> INSERT INTO t(x) VALUES('alpha beta');
> .connection 1
> .open probe-cli.db
> PRAGMA integrity_check;          -- ok
> .connection 0
> INSERT INTO t(x) VALUES('gamma delta');
> .connection 1
> PRAGMA integrity_check;          -- fts5: checksum mismatch for table "t"
> .open probe-cli.db
> PRAGMA integrity_check;          -- ok
> ```
>
> It looks like `fts5IntegrityMethod()` uses the cached structure without the
> `fts5NewTransaction()` call that `fts5BeginMethod()` and `fts5OpenMethod()`
> make. Adding
>
> ```c
> rc = fts5NewTransaction(pTab);
> if( rc==SQLITE_OK ) rc = sqlite3Fts5StorageIntegrity(pTab->pStorage, 0);
> ```
>
> in place of the bare `sqlite3Fts5StorageIntegrity()` call makes the check
> report `ok` here, and genuine damage (e.g. a row deleted from `t_docsize`)
> is still reported. Reading one row from the table before the pragma also
> avoids it, which is what we do for now.
>
> We hit this in an application that keeps a long-lived read connection and
> runs `integrity_check` periodically while another connection writes.

## Workaround lifecycle

- Keep `refreshFTS5StructuresTx` until an admitted driver bundles an engine
  with the fix.
- **On every driver admission review** (#356 and later), build this probe
  against the candidate's engine source without `refresh` and record the
  result. Also run
  `TestSQLiteIntegrityOnAReusedReaderAfterSearchWrites`: its bare-pragma
  canary fails once the engine is fixed. When both show the fix, decide in
  that review whether to remove the refresh; keep the canary or invert it.

## Reproduction

```sh
DIR=sqlite-amalgamation-3540100   # unzip https://sqlite.org/2026/$DIR.zip
cp docs/reviews/fixtures/fts5-integrity-stale-structure-probe.c.txt /tmp/probe.c
cc -O1 -DSQLITE_THREADSAFE=1 -DSQLITE_ENABLE_FTS5 -I "$DIR" "$DIR/sqlite3.c" \
  /tmp/probe.c -o /tmp/fts5-probe -lm -lpthread -ldl
(cd "$(mktemp -d)" && /tmp/fts5-probe wal; /tmp/fts5-probe wal refresh)
./scripts/go-backend.sh test ./internal/api ./internal/db \
  -run 'TestRepeatedSelfCheckRunsPassAfterNewEntries|TestSQLiteIntegrityOnAReusedReaderAfterSearchWrites' -count=3
```

The Go tests pass on the admitted v1.14.52 / 3.53.4 (2026-11-08, three runs).
