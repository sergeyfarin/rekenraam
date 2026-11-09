# FTS5 integrity_check stale structure — standalone reproduction — 2026-11-08

[#541](https://github.com/sergeyfarin/rekenraam/issues/541) reduces the
[#403](https://github.com/sergeyfarin/rekenraam/issues/403) false finding to a
standalone reproduction that uses only official SQLite amalgamations, and
prepares the upstream report. **The engine bug reproduces on 3.53.4 (the
admitted driver) and 3.54.1 (the held candidate, #356).** Trunk was not run.
Its `fts5IntegrityMethod` source is unchanged as of 2026-11-08, so the bug is
probably still there, but that is an inference from the source, not a
result. `refreshFTS5StructuresTx` in `backend/internal/db/self_check.go`
stays load-bearing.

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
connection. An ordinary read of the table (for example
`SELECT rowid FROM t LIMIT 1`) refreshes the cache. If the pragma then runs
in the **same read transaction**, the false finding disappears.

That transaction boundary is part of the workaround, not a detail. A refresh
in its own autocommit statement only shows the cache as it stood at that
moment. A write that commits between the refresh and an autocommit pragma
makes the cache stale again. `SQLiteIntegrity` therefore opens one read
transaction and runs the refresh, `integrity_check` and `foreign_key_check`
inside it. The probe's `refresh` mode does the same: `BEGIN`, the one-row
read, the pragma, `COMMIT`. The shell script and the forum draft show the
bare defect, so neither one refreshes.

The journal mode does not matter: WAL and rollback (`delete`) behave the same.

## Retained reproductions

| File | What it is |
|---|---|
| [`fixtures/fts5-integrity-stale-structure-probe.c.txt`](fixtures/fts5-integrity-stale-structure-probe.c.txt) | Self-checking C probe. Two connections run three `integrity_check` checks, with a write on the other connection between each. Then come three controls: a fresh-connection check, the FTS5 `'integrity-check'` command, and real damage (a deleted `t_docsize` row) that both a fresh connection and the reader must report as an FTS5 corruption finding for `t`. The optional `refresh` argument applies the application's workaround in one read transaction. Exit status: **0** pass, **1** false finding (bug present, controls passed), **2** invalid run, including invalid arguments. |
| [`fixtures/fts5-integrity-stale-structure-probe.sql`](fixtures/fts5-integrity-stale-structure-probe.sql) | Shell-only version for the `sqlite3` CLI (`.connection`, 3.37+). `.open --new` makes it rerunnable in place. This is the form used in the upstream report. |
| [`fixtures/fts5-integrity-stale-structure-matrix.py`](fixtures/fts5-integrity-stale-structure-matrix.py) | Matrix driver. It builds the probe against each given amalgamation and against a copy with the candidate patch applied. It runs the base grid, invalid arguments and every source mutation. Each case requires an exit code and the stdout line that shows which branch decided the run. |
| [`fixtures/fts5-integrity-stale-structure-2026-11-09.json`](fixtures/fts5-integrity-stale-structure-2026-11-09.json) | That driver's output for 3.53.4 and 3.54.1: 120 runs, all as expected. For every run it records the required lines and the full probe stdout, including the run 2–3 findings. It also records the probe and driver hashes and the official and patched `sqlite3.c` hashes. Rerunning with the same inputs gives a byte-identical file. |
| [`fixtures/fts5-integrity-stale-structure-2026-11-09.environment.json`](fixtures/fts5-integrity-stale-structure-2026-11-09.environment.json) | The volatile half of the run record: timestamp, compiler and platform. |

## Results

Official amalgamations, built with
`cc -O1 -DSQLITE_THREADSAFE=1 -DSQLITE_ENABLE_FTS5`. Each `sqlite3.c`
SHA3-256 matches its release log:

| Engine | `sqlite3.c` SHA3-256 |
|---|---|
| [3.53.4](https://sqlite.org/releaselog/3_53_4.html) | `67f423e9ebbbdc473cbc4772c872ee6b89f31fde4ed0279a5c25d5f65c043a16` |
| [3.54.1](https://sqlite.org/releaselog/3_54_1.html) | `e2b869d3003b33c942b7dd7063604fbc53553e1b66609570df39b9b7b558ee81` |

| Engine | Mode | No refresh | `refresh` |
|---|---|---|---|
| 3.53.4 | wal / delete | **exit 1**: runs 2–3 `checksum mismatch` | exit 0 |
| 3.54.1 | wal / delete | **exit 1**: runs 2–3 `checksum mismatch` | exit 0 |
| 3.53.4 or 3.54.1 + candidate patch | wal / delete | exit 0 | exit 0 |

In every row, all three controls passed: the fresh connection and the FTS5
`'integrity-check'` command reported `ok`, and both the fresh connection and
the reader reported the damage as `malformed inverted index for FTS5 table
main.t`. The CLI script prints `ok / checksum mismatch / ok` on 3.53.4 and
3.54.1, and `ok / ok / ok` with the patch.

**The probe's own failure modes.** The first version of the probe treated any
non-`ok` damage result as detection, even a failure to run the check, and it
only printed its two independent controls. Review of PR #546 (summarized in
the PR description) showed that an injected authorization failure and broken
controls both still produced exit 0, and that a mistyped `refresh` silently
ran the other experiment. The probe now separates execution status from
findings, rejects unknown arguments, and lets every open, statement and
control fail the run. The matrix driver applies each failure below to all
four engines (3.53.4 and 3.54.1, with and without the patch), with and
without `refresh`. Every one returns **exit 2**, and the required line shows
the branch that decided it. Four invalid argument lists also return exit 2:
`wal refesh`, `wal refresh extra`, `wl` and `wal ''`.

| Injected failure | Reported as |
|---|---|
| Authorizer denies the reader's pragma at the damage check | `reader PRAGMA integrity_check: prepare: not authorized` |
| Authorizer denies every reader pragma | same |
| Fresh-connection control returns something other than `ok` | `fresh-connection control` |
| FTS5 `'integrity-check'` command fails | `FTS5 'integrity-check' control` |
| Damage `DELETE` matches no row | `damage control: no t_docsize row deleted` |
| Damage `DELETE` fails | the statement and its error |
| A writer `INSERT` between checks fails | the statement and its error |
| The fresh connection cannot open | `open fresh connection: no such vfs` |
| Fresh damage-control expected text changed to `"main"."t"` quoting (simulated engine rewording) | `fresh-connection damage control: no FTS5 corruption finding for t` |
| Reader damage-control expected text changed the same way; the fresh check still passes | `reader damage control: no FTS5 corruption finding for t` |

The two expected-text cases change the probe's expectation, not the engine's
message. They stand in for an engine that rewords its finding, and exercise
each exact-match branch on its own.

**A stale reader still reports real damage.** One more mutation replaces the
FTS5 `'integrity-check'` command with a bare reader `integrity_check` that
only prints. On the unpatched engines that check prints `checksum mismatch`,
so the reader's cache is provably stale when the damage is checked. The
reader still reports `malformed inverted index for FTS5 table main.t`,
because FTS5 checks docsize before it compares checksums. The exit status
matches the base probe in all 8 runs. An earlier version of the probe also
accepted `checksum mismatch` from a reader that had shown the defect. This
case shows that branch could not be reached with this kind of damage, so it
was removed. The reader must now report the same corruption as the fresh
connection.

**Message matching is asymmetric, on purpose.**

- **The undamaged file.** Any result other than `ok` counts as the false
  finding (exit 1), whatever its wording. That keeps the admission signal
  conservative, but exit 1 alone does not prove this defect caused it. The
  admission rule therefore also requires reading the printed finding.
- **The damage controls.** These match exact English engine messages, because
  SQLite has no code for an `integrity_check` row. A damage message that no
  longer matches makes the probe exit 2 and print the new text. The matrix
  covers the fresh and reader branches separately by changing the expected
  text, as described above.

Exact matching is a strictness choice, not the only reasonable one. A prefix
such as `malformed inverted index for FTS5 table` would survive cosmetic
changes like that quoting change. The cost of exact matching is a noisy
exit 2 after such a change, never a wrong 0 or 1.

The candidate patch is one logical change in `fts5IntegrityMethod`: call
`fts5NewTransaction()` before the check, as `fts5BeginMethod` does.

```diff
   pTab->p.pConfig->pzErrmsg = pzErr;
-  rc = sqlite3Fts5StorageIntegrity(pTab->pStorage, 0);
+  rc = fts5NewTransaction(pTab);
+  if( rc==SQLITE_OK ) rc = sqlite3Fts5StorageIntegrity(pTab->pStorage, 0);
```

This patch is a hypothesis offered with the report, not a validated fix. The
only evidence for it is this probe's small matrix: two engine versions, two
journal modes, one table and one kind of damage. It has not been run against
SQLite's own test suite, and it does not show that resetting the cache inside
`xIntegrity` is safe with open cursors, nested transactions or other schemas.
Whether and how to fix this is upstream's call. The application never patches
the bundled engine (ADR 0014).

**Trunk: not run; probably affected, judging from unchanged source.**
`ext/fts5/fts5_main.c` on the SQLite GitHub mirror at `81ca4985` (2026-11-08)
has the same `fts5IntegrityMethod` body. The three most recent FTS5 commits
(`014644d8`, `2bcdb878`, `bedc3187`) do not touch this path. A fix elsewhere,
such as in `sqlite3Fts5StorageIntegrity` or the structure loader, would not
show up in this comparison. Building trunk needs its `configure`/`make`
toolchain, which this review did not run.

**Existing reports.** #403 found none. The SQLite forum and Fossil timeline
sit behind a browser check, so this review could not search them again
automatically. The pre-post forum search belongs with the posting record on
#541.

## Upstream report

This section keeps the draft as evidence. #541 is the only record of
approval, the pre-post search, posting and the thread link.

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
> Run in the `sqlite3` shell (3.37+ for `.connection`). It creates or
> overwrites `probe-cli.db` in the current directory:
>
> ```
> .open --new probe-cli.db
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
> is still reported. That is only a sketch, checked against this one case.
> Until there is a fix, we read one row from the table before the pragma,
> inside the same read transaction.
>
> We hit this in an application that keeps a long-lived read connection and
> runs `integrity_check` periodically while another connection writes.

## Workaround lifecycle

`docs/developer-workflow.md` (SQLite driver section) holds the normative
rule: when the probe runs at a driver admission, how its exit status reads,
and what removing `refreshFTS5StructuresTx` requires. This review only
supplies the probe and the evidence.

## Reproduction

```sh
# From https://sqlite.org/2026/sqlite-amalgamation-3540100.zip, unzipped here
DIR=sqlite-amalgamation-3540100
cp docs/reviews/fixtures/fts5-integrity-stale-structure-probe.c.txt /tmp/probe.c
cc -O1 -DSQLITE_THREADSAFE=1 -DSQLITE_ENABLE_FTS5 -I "$DIR" "$DIR/sqlite3.c" \
  /tmp/probe.c -o /tmp/fts5-probe -lm -lpthread -ldl
(cd "$(mktemp -d)" && { /tmp/fts5-probe wal; echo "exit=$?"; /tmp/fts5-probe wal refresh; echo "exit=$?"; })
# Full matrix (both engines, patched copies, invalid arguments, injected failures):
python3 -I docs/reviews/fixtures/fts5-integrity-stale-structure-matrix.py \
  docs/reviews/fixtures/fts5-integrity-stale-structure-probe.c.txt /tmp/fts5-matrix.json \
  3.53.4=sqlite-amalgamation-3530400 3.54.1=sqlite-amalgamation-3540100
cmp /tmp/fts5-matrix.json docs/reviews/fixtures/fts5-integrity-stale-structure-2026-11-09.json
./scripts/go-backend.sh test ./internal/api ./internal/db \
  -run 'TestRepeatedSelfCheckRunsPassAfterNewEntries|TestSQLiteIntegrityOnAReusedReaderAfterSearchWrites' -count=3
```

The Go tests pass on the admitted v1.14.52 / 3.53.4 (2026-11-08, rerun
2026-11-09, three runs each).
