# SQLite driver candidate — go-sqlite3 v1.14.53 / SQLite 3.54.1 — 2026-11-08

**#356 remains OPEN. The candidate exists but is held.** Upstream released
[`github.com/mattn/go-sqlite3` v1.14.53](https://github.com/mattn/go-sqlite3/releases/tag/v1.14.53),
which bundles SQLite **3.54.1** ("Upgrade SQLite to version 3540100",
mattn/go-sqlite3#1472). That clears the prerequisite recorded in the
[2026-10-20 blocker review](sqlite-driver-upgrade-blocker-2026-10-20.md). The
candidate passes source authentication and both correctness probes. During
the 2026-11-08 bulk dependency update, however, it made the application test
package about **34% slower**, which pushes the race gate past its deadline.
The production pin stays at **v1.14.52 / SQLite 3.53.4**.

## Exact source

| Item | Value |
|---|---|
| Module | `github.com/mattn/go-sqlite3 v1.14.53` |
| Module checksum | `h1:mnTM0owzUMrKuLo+0w51NUiB0ir2Fo7kFXlPt/l3BGs=` |
| Bundled `sqlite3-binding.c` git blob | `2cff93269e94e4dbae0fffd29032362d4a8c0e85` (v1.14.52: `4a3437f5…`) |
| Bundled `sqlite3-binding.c` SHA-256 | `758343f1464f3ee8c7c601d5b0cde415b52dcdf43a44ed884a62d19ec416e824` |
| `sqlite_version()` | `3.54.1` |
| `sqlite_source_id()` | `2026-10-16 14:45:20 555e07ef33f69a0957d24d8e555b702464a131d6f1607f4c3c1e4551e8e6ca76` |

The [official 3.54.1 amalgamation](https://sqlite.org/2026/sqlite-amalgamation-3540100.zip)
`sqlite3.c` has SHA3-256
`e2b869d3003b33c942b7dd7063604fbc53553e1b66609570df39b9b7b558ee81`, matching
the [release log](https://sqlite.org/releaselog/3_54_1.html). It also matches
the hash the blocker review authenticated. Diffing it against the bundled
`sqlite3-binding.c`, and `sqlite3.h` against `sqlite3-binding.h`, shows only
the driver's outer `#ifndef USE_LIBSQLITE3` … `#else … #endif` wrapper. That
is **byte-identical engine source**, the standard #354 applied to 3.53.4.

The driver's `#cgo CFLAGS` are unchanged from v1.14.52. Relative to the
admitted fingerprint, `PRAGMA compile_options` adds only
`MAX_SCHEMA=10000000` and removes nothing.

## Correctness probes — pass

The retained [Go-driver probe](fixtures/native-sqlite-upstream-probe.go.txt),
run against the candidate module, gave these results:

| Result | Expected | v1.14.52 / 3.53.4 | v1.14.53 / 3.54.1 |
|---|---|---|---|
| Edited canonical JSON `[31,2]` is valid | 1 | **0** | **1** |
| Indexed join/subquery matches its sole eligible row | 1 | **0** | **1** |
| Independent `NOT INDEXED` control | 1 | 1 | 1 |
| Eligible row changed from `old` to `new` | `new` | **`old`** | **`new`** |

With the fingerprint test updated to 3.54.1, the native audit, backup/restore,
fee-policy, search and self-check tests pass. On 2026-11-08 SQLite's
[CVE catalogue](https://sqlite.org/cves.html) lists no entry fixed in or
introduced by 3.54.x. The 3.54.1 patch fixes a 32-bit-only heap overflow that
3.54.0 introduced.

**#403 is not fixed upstream.** With `refreshFTS5StructuresTx` disabled,
`TestRepeatedSelfCheckRunsPassAfterNewEntries` still fails on run 2 under
3.54.1. [#541](https://github.com/sergeyfarin/rekenraam/issues/541) tracks a
standalone reproduction and the upstream report; the
[standalone review](fts5-integrity-stale-structure-2026-11-08.md) reproduces
it on official 3.53.4 and 3.54.1 amalgamations. Under the admission rule in
`docs/developer-workflow.md`, the FTS5 probe built against this candidate's
engine (the official 3.54.1 amalgamation authenticated above) and run as
`probe wal` **exits 1** (2026-11-09): the workaround is still needed.

## Blocking finding — statement re-preparation

3.54.1 **re-prepares a joined statement when a bound equality on it takes
part in constant propagation.** This plausibly comes from the same
`fee60d93e9fb3723` fix that #356 wants. The
[retained C probe](fixtures/native-sqlite-reprepare-probe.c.txt) uses only
official amalgamations and runs three rebinds per statement:

| Statement shape | 3.53.4 reprepares | 3.54.1 reprepares |
|---|---|---|
| `… FROM a JOIN k ON k.code=a.code WHERE a.id = ?` | 0 | **2** |
| `… JOIN … WHERE a.id IN (?,?)` | 0 | 0 |
| `… JOIN … WHERE a.book = ? AND a.id IN (?,?)` | 0 | **2** |
| Single-table `WHERE id = ?` / `book = ?` / `code = ?` | 0 | 0 |

The application opens connections with `_stmt_cache_size=0`. Every execution
of an affected query therefore compiles its statement twice: prepare, bind
(the plan expires), then re-prepare on the first step. Joined queries
filtered by `book_id = ?` are the norm here. ADR 0016's 2026-10-24 amendment
measured statement compilation as 32.7% of the application package's CPU.

Matched, uncached `go test ./internal/app -count=1 -p 1` on the same host,
run sequentially with nothing else running:

| Tree | Driver Go code | Engine | `internal/app` |
|---|---|---|---|
| `main` `cf0384d8` | v1.14.52 | 3.53.4 | **186 s** |
| `main` + driver bump | v1.14.53 | 3.54.1 | **250 s** |
| `main` + hybrid | v1.14.53 | 3.53.4 (v1.14.52 bundled source) | **185 s** |
| bump branch (`go fix` modernization) | v1.14.53 | 3.54.1 | **250 s** |

The driver's new fused bind/step CGO paths are neutral. **The whole
regression comes from the 3.54.1 engine.** Under `-race`,
`./scripts/test-backend.sh` failed: `internal/app` hit Go's 5-minute package
deadline. ADR 0016's last matched full gates were already 263–269 s, so this
is the act threshold firing outright, not a near miss.

A second effect is visible in tests: authorizer-based query counters see two
prepares for one statement.
`TestTransactionResponseEnrichmentBatchesCompoundAndRepeatedJournals` counted
2 `account_kinds.base_kind` reads where it expects 1. That test needs to
allow one re-prepare while still rejecting per-account lookups: assert at
most 2 reads, and equal reads at batch sizes 2 and 100. A per-account
mutation then reports 6.

## What would unblock admission

Any one of the following, re-measured against ADR 0016's thresholds:

1. An upstream SQLite release that no longer re-prepares these shapes, or
   re-prepares only when the bound value changes the plan.
2. Statement-compilation reductions on the #404/#405 track that leave enough
   headroom to absorb the extra prepares under `-race`, with production
   query cost measured as well as test time.
3. An owner decision, recorded as an ADR 0016 amendment with matched
   evidence, that the correctness fixes justify the cost and the deadline.

Until then the two reproduced defects remain as #354 classified them: real
engine bugs with no affected application financial consumer reproduced.

## Reproduction

```sh
cp docs/reviews/fixtures/native-sqlite-upstream-probe.go.txt /tmp/native-sqlite-upstream-probe.go
cp docs/reviews/fixtures/native-sqlite-reprepare-probe.c.txt /tmp/reprepare-probe.c
# In a scratch checkout, bump only the driver:
(cd backend && go get github.com/mattn/go-sqlite3@v1.14.53)
./scripts/go-backend.sh run /tmp/native-sqlite-upstream-probe.go
./scripts/go-backend.sh test ./internal/app -count=1 -p 1 -timeout=20m
```

Compile the C probe once against each official amalgamation (`3530400`,
`3540100`), using the command in its header. For the hybrid row, copy the
v1.14.53 module, replace its `sqlite3-binding.{c,h}` with v1.14.52's, and
point a `replace` directive at the copy.
