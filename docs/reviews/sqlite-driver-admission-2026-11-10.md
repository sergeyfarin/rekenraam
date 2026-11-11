# SQLite driver admission — go-sqlite3 v1.14.53 / SQLite 3.54.1 — 2026-11-10

**Admitted; the owner accepted the performance cost on 2026-11-11.**
`github.com/mattn/go-sqlite3`
v1.14.53 is the latest released upstream driver and bundles SQLite **3.54.1**.
It carries both upstream correctness fixes retained by
[#356](https://github.com/sergeyfarin/rekenraam/issues/356): the
stale-JSON-validation fix and the expression-index constant-propagation fix.
This review supersedes the admission hold in the
[2026-11-08 candidate review](sqlite-driver-candidate-2026-11-08.md) for the
environments measured below; the candidate's source and probe evidence remains
valid.

The 2026-11-08 review measured a 34% application-package slowdown and a race
deadline timeout on its reference host. The clean 2026-11-10 uncached pair below
now shows the [ADR 0016](../adrs/0016-native-backend-test-resources.md) **act
threshold firing**: candidate runs of **245.657 s / 293.578 s / 271.254 s**
against baseline **225.121 s / 232.339 s** on the same six-CPU host. Two
candidate runs exceed 270 s while the baseline does not; all still finish under
the 300 s deadline. The hosted runs are cached-gate observations, not threshold
evidence. Production-query measurements also show a **+16% to +63%** cost on
representative affected paths. The implementation and correctness acceptance
are met; the performance acceptance is not. Per the 2026-11-08 #356 decision,
merging required an explicit owner amendment accepting both the act-threshold
breach and the production cost (or a mitigation). The owner gave that
acceptance on 2026-11-11 and asked for a follow-up that looks for speed-ups;
see [Owner acceptance](#owner-acceptance-2026-11-11). No statement cache,
deadline, scheduling or durability policy changes.

## Exact source

| Item | Value |
|---|---|
| Module | `github.com/mattn/go-sqlite3 v1.14.53` |
| Module checksum | `h1:mnTM0owzUMrKuLo+0w51NUiB0ir2Fo7kFXlPt/l3BGs=` |
| Bundled `sqlite3-binding.c` git blob | `2cff93269e94e4dbae0fffd29032362d4a8c0e85` |
| Bundled `sqlite3-binding.c` SHA-256 | `758343f1464f3ee8c7c601d5b0cde415b52dcdf43a44ed884a62d19ec416e824` |
| `sqlite_version()` | `3.54.1` |
| `sqlite_source_id()` | `2026-10-16 14:45:20 555e07ef33f69a0957d24d8e555b702464a131d6f1607f4c3c1e4551e8e6ca76` |

The [structured evidence](sqlite-driver-admission-2026-11-10.json) records the
check identities, probe output and matched timings in machine-readable form.

The [official 3.54.1 amalgamation](https://sqlite.org/2026/sqlite-amalgamation-3540100.zip)
`sqlite3.c` has SHA3-256
`e2b869d3003b33c942b7dd7063604fbc53553e1b66609570df39b9b7b558ee81`, matching
the [release log](https://sqlite.org/releaselog/3_54_1.html). Diffing it against
the bundled binding shows only the driver's `USE_LIBSQLITE3` wrapper: the
engine source is byte-identical. The driver's `#cgo CFLAGS` are unchanged from
v1.14.52. Relative to the admitted 3.53.4 fingerprint, `PRAGMA compile_options`
adds only `MAX_SCHEMA=10000000` and removes nothing; the fingerprint test now
pins that option.

## Correctness probes

The retained [Go-driver probe](fixtures/native-sqlite-upstream-probe.go.txt)
was run against this module:

```json
{"index_count_actual":1,"index_count_expected":1,"json_rendered":"[31,2]","json_valid_actual":1,"json_valid_expected":1,"not_indexed_control":1,"source_id":"2026-10-16 14:45:20 555e07ef33f69a0957d24d8e555b702464a131d6f1607f4c3c1e4551e8e6ca76","update_actual":"new","update_expected":"new","version":"3.54.1"}
```

The independent `NOT INDEXED` control remains 1. The retained C probe's
negative control on official 3.53.4 still returns 0/0/0/`old`; official 3.54.1
returns 1/1/1/`new` (recorded in the
[candidate review](sqlite-driver-candidate-2026-11-08.md#correctness-probes--pass)).
The application expression index is still the one reviewed for #356; the
`#405` planner regression `TestBaselineIndexesKeepOrderedAndForeignKeyLookups`
passes against this engine.

## Application validation

Run on the six-CPU workspace described below, from the admission branch:

- The earlier provisional uncached runs passed twice
  (`GOFLAGS=-count=1 GO_PHASE_TIMINGS=1 ./scripts/test-backend.sh`), with the
  application package at **245.012 s** and **257.674 s**. The clean uncached
  pair under [Race-suite performance](#race-suite-performance) supersedes them
  and fires the 270 s act threshold.
- `COVERAGE=1 GO_PHASE_TIMINGS=1 ./scripts/test-backend.sh` passed with merged
  coverage **82.8%**; the application package ran in 198.720 s.
- `pnpm build` passed, including the embedded-asset test and the integrated
  Go binary build.
- The branch also moves the Go minimum to **1.27.4** for GO-2026-6859
  (`net/http` MaxBytesReader header race); `govulncheck ./...` reports **no
  vulnerabilities**. A provisional uncached full gate on that toolchain
  measured **279.089 s** in the application package, so the act-threshold
  finding below is unchanged.
- The full race suite includes the native audit, search/NUL rejection,
  backup/restore, fee-policy, reference-book, self-check and planner tests.

The only test expectation that changed with the engine is the authorizer
counter in `TestTransactionResponseEnrichmentBatchesCompoundAndRepeatedJournals`.
SQLite 3.54.1 re-prepares a joined statement once when a bound equality takes
part in constant propagation, so the two bulk lookups may compile twice. The
test now requires one to two preparations and equal counts at batch sizes 2
and 100, which still rejects per-account or per-posting lookups.

## Race-suite performance

### Hosted observations (cached gate)

The CI workflow runs `./scripts/test-backend.sh` with `-race -p 2 -timeout=5m`,
the shared feature tags `fts5,netgo,osusergo`, `CGO_ENABLED=1` and
`GO_PHASE_TIMINGS=1`. It does **not** set `GOFLAGS=-count=1`: Go test caching is
enabled, and the logs show `(cached)` packages. The `internal/app` package
executed in these runs, but the gate as a whole is not cache-free, so these are
observations, not formal ADR 0016 threshold runs. The `setup-go-cache` exact-hit
flag is the build/module cache, separate from Go's test cache.

| Measurement | Commit | Run | `internal/app` | vs baseline | Build-cache exact hit |
|---|---|---|---:|---:|---|
| Main baseline | `7d3d2059` | [42760293595 / 129644138504](https://github.com/sergeyfarin/rekenraam/actions/runs/42760293595/job/129644138504) | 219.404 s | — | false |
| Candidate | `2eb5adb4` | [42873724976 / 130031379151](https://github.com/sergeyfarin/rekenraam/actions/runs/42873724976/job/130031379151) | 216.848 s | -1.2% | false |
| Candidate | `b6655526` | [42767669769 / 129667989356](https://github.com/sergeyfarin/rekenraam/actions/runs/42767669769/job/129667989356) | 252.718 s | +15.2% | false |
| Candidate, code head measured | `faeedf20` | [42876220590 / 130040105987](https://github.com/sergeyfarin/rekenraam/actions/runs/42876220590/job/130040105987) | 253.242 s | +15.4% | true |

The latest measured code-head result is **253.242 s**, not the earlier
**216.848 s**; the latter belongs to `2eb5adb4`. The later `33f94129` and
`1e3fd127` commits add this evidence and the retained probe; they do not change
the backend or frontend.

### Clean uncached threshold pair

Configuration: `GOFLAGS=-count=1 GO_PHASE_TIMINGS=1 ./scripts/test-backend.sh`
on the same six-CPU host, clean commits, `-race -p 2 -timeout=5m`.

| Tree | Commit | `internal/app` runs |
|---|---|---:|
| Main baseline, v1.14.52 / 3.53.4 | `7d3d2059` | 225.121 s, 232.339 s |
| Candidate, v1.14.53 / 3.54.1 | `35ee1bf5` | 245.657 s, **293.578 s**, **271.254 s** |

Two candidate runs exceed ADR 0016's 270 s act threshold; neither baseline run
does. All runs still finish under the 300 s deadline, but the act rule fires and
requires an owner amendment or mitigation before the next feature slice. A
provisional uncached gate on the Go 1.27.4 security-patch worktree measured
**279.089 s**, so the toolchain patch does not change the conclusion. The
earlier provisional runs (baseline 217.163 s; candidate 245.012 s / 257.674 s on
the worktree that became `3c52b99b`) are superseded by this clean pair and are
not used for the threshold. The 2026-11-08 review's ten-CPU reference
measurement remains the record of its own environment; this six-CPU host is the
available threshold evidence.

## Production-query cost

The [retained probe](fixtures/sqlite-driver-production-query-cost.go.txt) uses
the real repository methods, the production connection policy
(`_stmt_cache_size=0`, WAL/FULL) and one fixture with 20 buy transactions,
three accounts and two commodities. Copy it to
`backend/internal/app/sqlite_driver_production_query_cost_test.go` and run:

```sh
SQLITE_DRIVER_COST=1 ./scripts/go-backend.sh test ./internal/app \
  -run '^TestSQLiteDriverProductionQueryCost$' -count=1 -v
```

Medians of three matched runs on the same six-CPU host and fixture:

| Production path | v1.14.52 / 3.53.4 | v1.14.53 / 3.54.1 | Delta |
|---|---:|---:|---:|
| Accounts batch, 2 IDs | 127.28 µs | 189.99 µs | **+49.3%** |
| Commodities batch, 2 IDs | 95.79 µs | 156.54 µs | **+63.4%** |
| Enrich 2 transaction records | 240.19 µs | 345.50 µs | **+43.8%** |
| Transaction by ID | 335.62 µs | 528.37 µs | **+57.4%** |
| List transactions, page 50 | 3,879.05 µs | 5,020.96 µs | **+29.4%** |
| Account register, page 50 | 1,333.16 µs | 1,541.78 µs | **+15.6%** |

These are same-host measurements, not a production load test or a whole-book
latency SLA; the three raw samples per path are in the structured evidence.
They confirm that the 3.54.1 re-preparation cost reaches the application's real
query paths rather than only the test suite. No query result or observed plan
changed in the tested consumers; the cost is engine statement preparation, not
the driver's Go bindings (the hybrid measurement in the candidate review showed
the Go changes are neutral).

## FTS5 and advisory reconciliation

The [#403 workaround](../implemented.md) is **still required**. Under the
admission rule in [developer workflow](../developer-workflow.md), the FTS5
integrity probe built against the authenticated official 3.54.1 amalgamation
and run as `probe wal` exits **1** with a false checksum mismatch; every
control passes. [#541](https://github.com/sergeyfarin/rekenraam/issues/541)
tracks the standalone reproduction and draft upstream report.
`refreshFTS5StructuresTx` stays in place.

SQLite's [CVE catalogue](https://sqlite.org/cves.html) was rechecked on
2026-11-10: no entry names a fix in or an introduction by 3.54.x. The 3.54.1
patch fixes a 32-bit-only heap-buffer overflow introduced by 3.54.0. The
compile-option delta is only `MAX_SCHEMA=10000000`; FTS3, FTS5, RTREE,
`THREADSAFE=1`, FULL durability and the connection policies are unchanged. The
existing [native advisory audit](native-sqlite-advisory-audit-2026-10-20.md)
and its matrix therefore retain their application-consumer conclusions; no new
advisory or feature exposure was identified.

## Decision

The implementation and correctness acceptance items are met: both retained
probes return their expected values on the exact engine source, the full
backend/integrated gates pass, the advisory/FTS5 positions are reconciled, and
the measured production results match. The performance acceptance is **not**
met by this review alone. The clean uncached pair fires ADR 0016's 270 s act
threshold (candidate 245.657 s / 293.578 s / 271.254 s; baseline 225.121 s /
232.339 s), and production-query cost is **+16% to +63%** on the representative
affected paths above. Per the 2026-11-08 [#356 recheck](https://github.com/sergeyfarin/rekenraam/issues/356#issuecomment-6726921647),
merging requires an explicit owner amendment accepting both the act-threshold
breach and the production cost, or a mitigation. If the owner accepts them,
admit `github.com/mattn/go-sqlite3` v1.14.53 with bundled SQLite 3.54.1 under
the existing ADR 0014 driver decision and ADR 0015 durability policy. This
review records the evidence; it does not itself grant the performance
acceptance. The investigate threshold remains open under #419/#445.

### Owner acceptance (2026-11-11)

The owner accepted both the ADR 0016 act-threshold breach measured above and
the **+16% to +63%** production-query cost, and admitted
`github.com/mattn/go-sqlite3` v1.14.53 with bundled SQLite 3.54.1 under the
existing ADR 0014 driver decision and ADR 0015 durability policy. This is the
explicit owner amendment the #356 recheck required; it satisfies ADR 0016's
act rule for this driver change only. The owner also asked for a separate
follow-up that looks for ways to recover the speed. That follow-up does not
gate this admission.
