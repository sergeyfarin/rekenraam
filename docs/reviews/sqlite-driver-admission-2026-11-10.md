# SQLite driver admission — go-sqlite3 v1.14.53 / SQLite 3.54.1 — 2026-11-10

**Admission pending merge.** `github.com/mattn/go-sqlite3` v1.14.53 is the
latest released upstream driver and bundles SQLite **3.54.1**. It carries both
upstream correctness fixes retained by [#356](https://github.com/sergeyfarin/rekenraam/issues/356):
the stale-JSON-validation fix and the expression-index constant-propagation
fix. This review supersedes the admission hold in the
[2026-11-08 candidate review](sqlite-driver-candidate-2026-11-08.md) for the
environments measured below; the candidate's source and probe evidence remains
valid.

The 2026-11-08 review measured a 34% application-package slowdown and a race
deadline timeout on its reference host. That result was **not reproduced** in
the matched 2026-11-10 environments: the candidate is 12.8–18.7% slower than
main on the available six-CPU workspace, while two hosted runs measured
**-1.2%** and **+15.2%**; all full race gates pass below ADR 0016's 270-second
act threshold. The
[ADR 0016](../adrs/0016-native-backend-test-resources.md) investigate
threshold (>210 s) remains exceeded, as it already was before this candidate;
headroom is narrow and monitoring continues under #419/#445. No statement
cache, deadline, scheduling or durability policy changes with this admission.

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

- `GOFLAGS=-count=1 GO_PHASE_TIMINGS=1 ./scripts/test-backend.sh` passed twice.
  The application package ran in **245.012 s** and **257.674 s**; every other
  package passed in both runs. Both are below the 270 s act threshold.
- `COVERAGE=1 GO_PHASE_TIMINGS=1 ./scripts/test-backend.sh` passed with merged
  coverage **82.8%**; the application package ran in 198.720 s.
- `pnpm build` passed, including the embedded-asset test and the integrated
  Go binary build.
- The full race suite includes the native audit, search/NUL rejection,
  backup/restore, fee-policy, reference-book, self-check and planner tests.

The only test expectation that changed with the engine is the authorizer
counter in `TestTransactionResponseEnrichmentBatchesCompoundAndRepeatedJournals`.
SQLite 3.54.1 re-prepares a joined statement once when a bound equality takes
part in constant propagation, so the two bulk lookups may compile twice. The
test now requires one to two preparations and equal counts at batch sizes 2
and 100, which still rejects per-account or per-posting lookups.

## Performance recheck

Matched application-package timings (`internal/app`):

| Environment | v1.14.52 / 3.53.4 | v1.14.53 / 3.54.1 | Delta |
|---|---:|---:|---:|
| Local six-CPU workspace, `-race` full gate | 217.163 s | 245.012 s / 257.674 s | +12.8% / +18.7% |
| Hosted CI, `-race` full gate | 219.404 s (main `7d3d2059`) | 216.848 s / 252.718 s (PR #557) | -1.2% / +15.2% |

The local baseline and candidate runs used the same host and branch content
except for the driver, its test expectations and the documentation in this
change. The hosted comparison uses main `7d3d2059` and the admission branch's
own PR #557 runs; the two candidate runs show runner variance, with the final
run slightly faster than the baseline. The 2026-11-08
review's ten-CPU reference measurement remains the record of its own
environment; the difference is why the act decision is based on matched runs,
not a single cross-host comparison.

No production query plan or result changed in the tested consumers. The cost
is engine statement preparation, not the driver's Go bindings; the hybrid
measurement in the candidate review showed the Go changes are neutral.

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

Admit `github.com/mattn/go-sqlite3` v1.14.53 with bundled SQLite 3.54.1 under
the existing ADR 0014 driver decision and ADR 0015 durability policy. The
production module pin changes only with this admission. The #356 acceptance
items are met: both retained probes return their expected values on the exact
engine source, the full backend/integrated gates pass, and the advisory/FTS5
positions are reconciled above. The performance investigate threshold remains
open under #419/#445; that is a continuing monitoring item, not a correctness
or durability exception.
