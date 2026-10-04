# Race gate runtime review (2026-10-04)

[T-125 #140](https://github.com/sergeyfarin/rekenraam/issues/140) follows
the 2026-09-30 review (`test-runtime-review-2026-09-30.md`). After T-134/T-135
the application package's race run took 868 seconds on this workspace,
against its explicit 900-second bound.

## Where the time went

`go test -race -p 1 -json ./internal/app` passed in 865.6 seconds. Of its 753
top-level tests only 160 called `t.Parallel()`; the other **594 ran one at a
time and accounted for 654.5 seconds**. `-p 1` runs packages one after
another, so those tests used one core of the machine.
The database package had the same shape: 120 of 140 top-level tests serial.
The API package was already mostly parallel (50 of 392 serial).

## Safety audit before adding `t.Parallel()`

- No application or database test uses `t.Setenv`, `os.Setenv`, `os.Chdir`,
  `slog.SetDefault`, `time.Local`, HTTP default clients or fixed ports. They
  write no package-level variable.
- Every fixture opens its own database through `testdb.Open`, a copy of a
  `sync.Once` template in the test's own `t.TempDir()`. Migration tests use
  goose's `NewProvider`, which keeps no global state.
- `t.Setenv` appears only in `cmd/rekenraam` and `internal/config`, which were
  not changed.
- Across packages, `go test` runs each package as its own process with
  process-unique temporary paths, and no fixed port is bound (the fixed
  addresses in tests are configuration strings).

## Change

`t.Parallel()` was added as the first statement of all 594 serial top-level
application tests and all 120 serial database tests. No test, subtest or
assertion was removed: the application package passed the same 1,375
tests and subtests before and after. `TestIntegrationSuitesRunTestsInParallel` (in
`internal/testdb`) fails if a new top-level test in either package omits it,
unless a `// serial: <reason>` comment explains why. A mutation check
(removing one call) fails it by name. The race detector, every package and the
15-minute per-package bound are unchanged.

## Measurements (4 cores, `taskset -c 0-3`, same workspace)

The 4-core pin matches the CI runner's size; the workspace itself has 10.

| Package | Before `-p 1` (s) | After `-p 1` (s) | After `-p 2` (s) |
|---|---:|---:|---:|
| Application | 871.1 | 461.5 | 695.7 |
| Database | 330.8 | 141.7 | 289.1 |
| API | 234.7 | 234.7 | 346.3 |
| cmd/rekenraam | 46.1 | 47.2 | 48.7 |
| **Whole gate wall** | **1,557** | **921** | **747** |
| Peak RSS (MB) | 669 | 790 | 790 |

The full gate falls by 41% at `-p 1`; the application package now uses 51% of
its bound instead of 97%. Three repeated non-race runs of each changed package
passed (application 38–42 s, database 7.5–8.3 s on 4 cores).

**`-p 2` was not adopted.** It saves a further 19% of wall time, but two
packages competing for four cores stretched the application package to 77% of
its bound — the margin problem this review set out to remove. Revisit when the
slower packages are smaller.

## Remaining costs

SQLite statement preparation and race instrumentation still dominate CPU
(T-111). The API package (235 s) is now the second-largest cost and is mostly
parallel already. CI runners were faster per core than this workspace before
the change (application 384–516 s in CI vs 868 s here).
