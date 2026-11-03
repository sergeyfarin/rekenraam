# CI and test performance profile — 2026-11-03

**Status:** Measurement and analysis only. No production code, schema, test,
workflow, cache or resource limit is changed by this review.

**Source:** `da2563b43d1f610b52d9865b62a1b1c4e1e05524` (`upstream/main` at the
time of measurement, 2026-11-03).

## Answer

There is **no measured Go-language slowdown or memory leak that a Rust rewrite
would demonstrably fix**. The backend spends most of its sampled CPU in native
calls: 86.6% of the large-book profile and 58.7% of the application test
profile are `runtime.cgocall`. That bucket includes both the Go-to-C transition
and the native execution beneath it; these profiles do **not** separate them,
so they cannot establish how much a different binding or language could
recover. Within the test profile, statement preparation is 28.7% and driver
connection/schema loading is 16.9%. The remaining Go work — HTTP, JSON, exact
arithmetic, CSV/zip writing, orchestration — is a minority and has no profile
signature that justifies a second language. No controlled Go-versus-Rust
binding comparison was run.

CI is not backend-bound either. The two browser jobs take roughly 705–750 s
each by median, while the backend race step is about 270 s, coverage about
151 s, the frontend check about 70 s and the integrated build about 40 s. The
browser `Evaluate` work (axe/layout measurements across route and sub-view
matrices) is the largest single cost. Rust would not change it.

Memory is bounded. The full backend race gate peaked at about **1,989 MiB RSS**
on this six-vCPU host; the isolated application profile peaked at about
**1,579 MiB**; the 250k-posting large-book test peaked at about **301 MiB**. The
frontend check/build peaked at about **1,718 MiB** and **1,731 MiB**, which is
Node/Vite memory, not Go. The application heap at the end of the profile was
about **4 MiB live**. There is no retained-heap or leak signal.

## Evidence and conditions

| Item | Value |
|---|---|
| Evidence | [ci-test-profile-2026-11-03.json](fixtures/ci-test-profile-2026-11-03.json) |
| Collector | [ci-test-profile-2026-11-03.py](fixtures/ci-test-profile-2026-11-03.py) |
| CI source | Successful `CI` push runs on `main`, last eight before 2026-11-03 |
| E2E artifacts | `browser-timings-app` and `browser-timings-audits` from run `41530724953` |
| Local host | 6 vCPU AMD EPYC 3251, 5.2 GiB RAM, Linux 7.0.0-42 |
| Toolchain | Go 1.27.3, GCC 15.2.0, Node 24.22.1, pnpm 12.11.2 |
| Native configuration | CGO, `fts5,netgo,osusergo`, FULL synchronous, statement cache 0 |
| CI runner (from artifact) | 4 vCPU AMD EPYC 7763, 16 GiB, three Playwright workers |
| Race configuration | `-race -p 2 -timeout=5m`; native SQLite C engine outside Go race detection |

The local machine is not the ten-vCPU reference host used by earlier race-suite
reviews, so absolute times are not directly interchangeable with those records.
The CI step medians are the cross-run comparison; the local runs are attribution
and memory measurements.

## CI timings

Job wall time includes GitHub runner queueing. One retained run waited about an
hour before its first step (`4378 s` workflow wall) while its browser step still
took `733 s`. Compare **step** durations, not job/workflow wall time, when
judging execution cost.

| Run | Commit | Workflow wall | E2E app | E2E audits | Backend tests | Coverage | Frontend | Build |
|---|---:|---:|---:|---:|---:|---:|---:|---:|
| 41530724953 | `da2563b4` | 823 s | 683 s | 765 s | 257 s | 131 s | 55 s | 37 s |
| 41358032132 | `2c9f0ac4` | 783 s | 699 s | 724 s | 154 s | 128 s | 75 s | 41 s |
| 41354953044 | `21faac9f` | 932 s | 843 s | 558 s | 253 s | 127 s | 72 s | 34 s |
| 41328464687 | `14f20ec7` | 853 s | 802 s | 747 s | 297 s | 183 s | 70 s | 50 s |
| 41318999728 | `f399b65b` | 893 s | 834 s | 732 s | 306 s | 171 s | 74 s | 53 s |
| 41311802795 | `20088756` | 701 s | 644 s | 511 s | 332 s | 189 s | 68 s | 40 s |
| 41286530486 | `c78d7712` | 4378 s* | 634 s | 733 s | 191 s | 121 s | 54 s | 33 s |
| 41283856463 | `d96d805f` | 878 s | 711 s | 826 s | 284 s | 250 s | 69 s | 125 s |

\* Queueing outlier; its steps are in the normal range and are retained rather
than silently dropped.

Medians over the last six normal runs:

| Step | Median | Range |
|---|---:|---:|
| E2E Smoke (app) — browser journeys | 750.5 s | 644–843 s |
| E2E Smoke (audits) — browser journeys | 728.0 s | 511–765 s |
| Backend Tests — run backend tests | 277.0 s | 154–332 s |
| Backend Coverage — run with coverage | 151.0 s | 127–189 s |
| Frontend Check — run frontend check | 71.0 s | 55–75 s |
| Integrated Build — build single binary | 40.5 s | 34–53 s |
| E2E — install Chromium | 20–22 s | 17–41 s |
| Go-cache setup | 12–14 s | 10–17 s |

The latest run (`41530724953`) follows the same shape: browser jobs 683/765 s,
backend 257 s, coverage 131 s, frontend 55 s, build 37 s. The E2E pair is the
critical path; the backend and frontend jobs finish well before it.

## Browser cost breakdown

Latest E2E artifacts, source `da2563b4`:

| Suite | Wall | Setup/build | Selected cases | Worker case-seconds | Main cost |
|---|---:|---:|---:|---:|---|
| `app` | 667.9 s | 50.8 s | 455 | 1,765.8 s over 3 workers | Ordinary journeys: click, navigate, evaluate |
| `audits` | 751.8 s | 42.2 s | 14 | 1,997.9 s over 3 workers | Route/sub-view matrices: axe, layout, locales |

The audit suite runs only 14 cases but takes longer because each case is a
serial matrix. The slowest single cases are:

- route sweep, every route fits the phone and declares its language in every
  locale: **277.4 s**;
- sub-view sweeps by locale/theme: **161–176 s** each;
- route sweeps by theme: **171–172 s** each;
- route error-state sweep: **66.5 s**.

Playwright action totals in the audit suite are dominated by `Evaluate`
(**1,195 s**), then `Navigate` (134.5 s), `Wait for selector` (84.1 s) and
`Click` (61.1 s). In the app suite, `Evaluate` is **430.7 s**, then `Click`
(329.2 s) and `Navigate` (119.0 s). These are browser-side costs: axe scans,
layout/overflow checks, focus traversal and the serial route matrices. A Go or
Rust backend change does not remove them.

The three-worker parallel efficiency is already close to the case-second lower
bound. The audit suite's 1,997.9 case-seconds divide to about 666 s across three
workers; its measured wall is 751.8 s including setup. The remaining lever is
matrix sharding or fewer redundant browser evaluations, not a faster backend.

## Local backend gate

Command:

```sh
GOFLAGS=-count=1 GO_PHASE_TIMINGS=1 /usr/bin/time -v ./scripts/test-backend.sh
```

Result: **pass** in **315.2 s** wall, **1,989 MiB** max RSS. Race execution was
249.4 s; vet and race compile/vet were 18.1 s and 47.2 s. Package times:

| Package | Wall |
|---|---:|
| `internal/app` | 188.4 s |
| `internal/api` | 130.5 s |
| `internal/db` | 95.5 s |
| `cmd/rekenraam` | 54.9 s |
| `internal/testdb` | 4.5 s |
| all others | 1–3 s each |

This local app time is below ADR 0016's 210 s investigate threshold, and the
full gate is below the 270 s act threshold. CI's backend step is a whole-suite
step, not a per-package measurement, so it is not compared directly with those
thresholds.

## Application CPU profile

Isolated command:

```sh
cd backend
source ../scripts/native-sqlite-env.sh
go test -race -count=1 -p 1 -timeout=15m ./internal/app \
  -cpuprofile=app.cpu -memprofile=app.mem -json > app-profile.json
```

Result: **pass**, **201.7 s** wall, **195.3 s** profile duration, **717.23
CPU-seconds**, **1,579 MiB** max RSS, **2,622 passing**, **0 failing**, **4
skipped** leaf cases.

Selected cumulative samples (overlapping; a cgo call can contain the deeper
SQLite frames). `runtime.cgocall` includes the Go-to-C transition **and** the
native execution beneath it; it is not a pure SQLite-engine measurement:

| Frame | CPU seconds | Share |
|---|---:|---:|
| `runtime.cgocall` (flat) | 418.25 | 58.31% |
| `runtime.cgocall` (cumulative) | 421.09 | 58.71% |
| `sqlite3_prepare_v2_internal` | 205.60 | 28.67% |
| `SQLiteDriver.Open` (driver connection/schema load) | 120.98 | 16.87% |
| `SelfCheckService.RunSelfCheck` | 66.84 | 9.32% |
| `testdb.Open` (test fixture setup) | 70.94 | 9.89% |
| `SQLiteConn.Close` | 32.50 | 4.53% |
| `argon2.processBlockSSE` (test authentication) | 26.74 | 3.73% |
| `goose` migration SQL parsing | 7.73 | 1.08% |

`database/sql` and the native driver account for the majority of the sampled
time. The application's own Go functions are mostly callers of SQL; there is no
large pure-Go arithmetic, JSON or serialization hotspot in this profile.

The allocation profile is also test-harness dominated: 65.4% of allocated
bytes come from Goose's SQL parser initialization, 20.7% from Argon2 test
password hashing, and about 7.7% from zip/flate bundle work. At the end of the
run only about **4.2 MiB** was live. That is a setup/allocation pattern, not a
retained-heap problem.

## Large-book runtime

`./scripts/measure-large-book.sh` generates 250,000 posted postings over 3,020
accounts in two currencies across five years.

| Operation | Wall | Allocated | Peak live heap |
|---|---:|---:|---:|
| Bundle export | 12.851 s | 433 MiB | 13 MiB |
| Self-check | 12.808 s | 137 MiB | 40 MiB |
| Attention read | 2.9 s | 101 MiB | 3 MiB |
| Home briefing | 4.247 s | 290 MiB | 113 MiB |

The whole test passed in 47.6 s with **301 MiB** max RSS. A non-race profile of
the same test sampled **43.95 CPU-seconds**, of which **38.07 s (86.62%)** was
`runtime.cgocall`. `sqlite3_step` alone was 55.0% cumulative, the export writer
26.2% and the self-check 25.9% (overlapping callers). The seed also contributes
cgo time.

This is strong evidence that the upper-bound workload is dominated by native
calls, but it is not a controlled language/binding comparison. The 86.6%
`runtime.cgocall` bucket includes the Go-to-C boundary as well as native
execution, so it cannot prove that a different Go layer or SQLite binding would
recover or preserve a specific fraction. It also cannot show that replacing
SQLite itself would help; that would be a data/product decision with much
larger correctness and migration risk. The defensible conclusion is **no
demonstrated rewrite benefit**, not an unrecoverable 86.6%.

## Frontend and build memory

| Command | Wall | Max RSS | Detail |
|---|---:|---:|---|
| `./scripts/test-frontend.sh` | 96.7 s | 1,718 MiB | 79 Vitest files, 979 tests, 27.9 s test time |
| `pnpm build` | 62.5 s | 1,731 MiB | 93% of Vite build time in JavaScript callbacks; Svelte compile 61%, Tailwind 30% |

These are the largest transient memory users measured here, but they are
Node/Vite costs. A Rust backend would not reduce them; moving the frontend
toolchain would be a separate UI/build decision. On a 5 GiB development host,
running the frontend check, build and backend race gate concurrently can create
memory pressure; CI has 16 GiB and does not show it. The remedy is scheduling,
not a language rewrite.

## Would Rust help anywhere?

| Candidate | Measured today | Rust potential | Verdict |
|---|---|---|---|
| SQLite statement preparation | 28.7% of test CPU | None if SQLite stays; `rusqlite` also calls C | Optimize SQL/schema, not language |
| Connection/schema load | 16.9% of test CPU | None; same engine and schema | Schema slimming/connection policy |
| Investment/ledger arithmetic | Not a top profile frame | Possible for a pure function | No measured need |
| CSV/QIF/import parsing | Minor in these profiles; I/O and SQL dominate | Possible for a bounded parser | Profile the real import path first |
| Bundle export zip/flate | About 7.7% of allocations in tests; SQLite read dominates large book | Possible, but Go's stdlib is adequate and the limit is SQLite | Not justified |
| Argon2 hashing | 3.7% of test CPU, already optimized assembly | No clear gain | Keep `x/crypto` |
| E2E/axe/layout | 705–750 s per CI browser step | None | Browser/tooling concern |
| Frontend check/build | 70 s check, 40 s build, ~1,720 MiB | A Rust bundler might help a different stack, not this backend | Separate frontend decision |

The only defensible Rust experiment would be an **isolated component** after a
fresh profile shows a pure-CPU function above roughly 20–30% of the relevant
wall time and after Go/SQL optimization has been tried. It would need a
differential-test harness against the Go implementation, a stable FFI boundary
and a measured end-to-end gain. None of those conditions holds today.

## Recommendations

1. **Keep Go.** Do not schedule a backend rewrite. The current language is not
   the measured constraint.
2. **Continue the SQL/query work** tracked by #445. Earlier profiling put the
   investment-foundation, effective-lot and replay-intent preparation union at
   about 9.1% of sampled test CPU; that is a real, bounded target and it also
   affects production reads.
3. **If CI wall time becomes a priority, target the browser matrices.** Shard
   the route/sub-view locale/theme matrices across workers, or remove genuinely
   redundant axe/layout evaluations while retaining the complete assertions.
   The backend and build jobs are not the critical path.
4. **Keep the current thresholds.** ADR 0016's per-package investigate/act
   thresholds still apply. This run's app package (188.4 s under the full gate,
   201.7 s isolated) does not fire them.
5. **Re-profile before any language decision.** Use the same source, native
   configuration, case inventory and worker/race settings. A comparison of
   unrelated runs is not evidence of a language speedup.

## Reproduction

```sh
# CI and browser evidence (requires an authenticated gh CLI)
python3 docs/reviews/fixtures/ci-test-profile-2026-11-03.py \
  --runs 8 \
  --browser-dir /path/to/browser-timings-app \
  --browser-dir /path/to/browser-timings-audits \
  --output /tmp/rekenraam-ci-profile.json

# Full backend gate; timing runs must bypass the test cache
GOFLAGS=-count=1 GO_PHASE_TIMINGS=1 /usr/bin/time -v ./scripts/test-backend.sh

# Isolated application CPU/heap profile; the subshell keeps the repo root current
(
  cd backend
  source ../scripts/native-sqlite-env.sh
  go test -race -count=1 -p 1 -timeout=15m ./internal/app \
    -cpuprofile=/tmp/app.cpu -memprofile=/tmp/app.mem -json >/tmp/app.json
  go tool pprof -top -cum -nodecount=50 /tmp/app.cpu
  go tool pprof -top -sample_index=alloc_space -nodecount=25 /tmp/app.mem
)

# Large-book runtime and profile (from the repo root)
./scripts/measure-large-book.sh
(
  cd backend
  source ../scripts/native-sqlite-env.sh
  REKENRAAM_LARGE_BOOK=1 go test -count=1 -timeout=15m \
    -run '^TestLargeBookExportAndSelfCheckStayBounded$' ./internal/app \
    -cpuprofile=/tmp/large-book.cpu -memprofile=/tmp/large-book.mem
)

# Frontend checks and build (from the repo root)
/usr/bin/time -v ./scripts/test-frontend.sh
/usr/bin/time -v pnpm build
```

The raw CPU/heap profiles, full JSON test stream and browser artifacts are not
committed. Their SHA-256 values and the reduced reports are retained in the
[evidence JSON](fixtures/ci-test-profile-2026-11-03.json). Re-run the commands
above to regenerate them.

## Limitations

- One CI runner class and one local host. Hosted step times include cache,
  runner-image and scheduling variance; the queueing outlier is retained and
  labelled.
- Race instrumentation and in-package test parallelism inflate CPU and memory
  relative to production. This profile attributes test cost; it is not a
  production latency benchmark.
- Go's CPU profile attributes native calls to their Go caller where a stack is
  available, but it does not resolve all SQLite C internals. The 16.9%
  connection-open bucket is not a pure schema-parser measurement.
- The native SQLite engine is outside Go's race detector (ADR 0014/0016).
- No controlled Python or Rust implementation was benchmarked. The conclusion
  follows the measured location of cost in this codebase, not a claim that Go
  wins every language benchmark.

## References

- [Backend language choice review — 2026-11-03](language-choice-review-2026-11-03.md).
- [ADR 0014: Native SQLite Driver](../adrs/0014-native-sqlite-driver.md).
- [ADR 0016: Native Backend Test Resources](../adrs/0016-native-backend-test-resources.md).
- [Race-suite reprofile after #419](race-suite-reprofile-2026-10-29.md) and
  [race-suite cost](race-suite-cost-2026-10-24.md) — earlier profiles with the
  same SQLite/schema attribution.
- [Large-book measurement](../developer-workflow.md#large-book-measurement).
- [E2E timing contract](../developer-workflow.md#e2e).
