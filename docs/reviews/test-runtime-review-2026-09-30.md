# Backend test runtime and duplication review (2026-09-30)

T-111 [#126](https://github.com/sergeyfarin/rekenraam/issues/126) follows the
application race gate's 598.347-second pass against Go's default 600-second
package timeout. The owner subsequently authorized a timeout extension or
test review, explicitly requiring duplication review rather than cutting tests.

## Measurement

The complete application package was run with race detection, JSON test timing
and a CPU profile. A 15-minute diagnostic limit let the measurement finish:

```sh
cd backend
go test -race -p 1 ./internal/app -count=1 -timeout=15m -json \
  -cpuprofile=/tmp/rekenraam-app-runtime-before.cpu \
  -o /tmp/rekenraam-app-runtime-before.test \
  > /tmp/rekenraam-app-runtime-before.json
go tool pprof -top -cum /tmp/rekenraam-app-runtime-before.test \
  /tmp/rekenraam-app-runtime-before.cpu
```

It passed in **591.939 seconds**: 591 top-level tests and 1,132 named passing
tests/subtests. The profile sampled 790.34 CPU seconds. SQLite statement
preparation/parser stacks accounted for about 53% of sampled CPU time;
pointer validation and race instrumentation were large contributors within
those stacks. These are overlapping stack measurements, not additive shares.
The earlier normal full gate took 598.347 seconds for the application package,
201.129 seconds for API and 516.151 seconds for DB.

## Duplication review

The duplication review focused on measured expensive families and nearby
financial mutation fixtures.

| Area | Repeated work | Coverage decision |
|---|---|---|
| Trade sequence property matrix | Four methods × three seeds, each opening the same four acquisitions | Keep all 12 cases: seeds produce different partial-disposal sequences and specific-lot elections; every method follows a distinct allocation path. Every command checks exact cash, quantity, basis and gains; preview and rejected-write snapshots check audit/child-row rollback. |
| Closed-position precision matrix | Four methods × four acquisition/disposal precision combinations × loss/zero/gain | Keep all 48 cases: precision changes on acquisition and disposal are independent boundaries; sign outcomes and method paths are distinct. These exact expected gains complement sequence conservation rather than duplicate it. |
| Allocation mutations and clearing attribution | Similar sale fixtures, different damaged evidence | Keep original/replayed/superseded allocation coverage and per-decision/per-posting attribution. A sound journal or current projection cannot substitute for those checks. |
| Balanced posting mutation regressions | Similar balanced cash/trading edits in `TestSelfCheckDetectsDisposalProceedsClearingMismatch` and `TestSelfCheckDetectsBalancedCashPostingComponentMismatch` | Keep both: one exercises a sale's disposal clearing, the other a buy's cash source component. Their distinct diagnoses and operation families remain important; they took 2.10 and 1.86 seconds respectively in the profile. |
| Backup/restore | Repeated fresh schema creation and backup inspection | Keep real migration/restore paths. WAL preservation, equal source/destination refusal, future-schema refusal, retained-key decryption, separate safety copies and crash publication stages are different failures. The slower restore cases took roughly 15–27 seconds each in the concurrent portion of the profile. |
| Ordinary integration fixtures | Repeated schema preparation on copied databases | The migrated template already removes repeated migration execution while preserving isolated files and real PRAGMA checks. Sharing live mutable databases would weaken isolation. |

No test, seed, precision combination, random step, assertion or migration check
was removed. The two property matrices previously ran serially and took
**55.04 + 47.76 = 102.80 seconds**. Each case owns its database, services,
random generator and expected-value state; none changes process environment.
Their parent tests and individual cases now call `t.Parallel()`, retaining
Go's bounded in-package test scheduling and serial package execution.

The focused race rerun passed all **60 identical named matrix cases** in
**32.301 seconds** including package setup. Its case names were compared with
the complete before-run JSON. This focused timing is evidence for the
scheduling improvement, not a substitute for the full gate or a guaranteed
full-suite speedup under different load.

## Deadline and remaining limits

The shared local/CI race wrapper now explicitly uses `-timeout=15m`, while
retaining `-race -p 1`, formatting and vet. The non-race coverage pass is
unchanged. The owner authorized this headroom after the review; it is not a
runtime target. No production code, schema or financial behavior changes.

SQLite parsing/instrumentation costs remain. Reassess repeated fixture work
using measured hotspots as the suite grows, without hiding distinct financial,
concurrency or restore coverage.

## Full-gate result

The updated `./scripts/test-backend.sh` passed formatting, vet and the entire
race suite. No production code or test assertions changed between these gates.

| Package | Before (seconds) | After (seconds) |
|---|---:|---:|
| API | 201.129 | 200.347 |
| Application | 598.347 | 513.745 |
| Database | 516.151 | 516.399 |

The observed application reduction was **84.602 seconds (about 14%)**.
The application retains 86.255 seconds below the old 600-second deadline and
386.255 seconds below the explicit 900-second deadline. These are individual
run measurements on this workspace, not a guaranteed runtime on every runner.
The focused case-name comparison retained all 60 matrix cases; the full gate
retained every package and regression. T-111's reviewed scheduling/deadline
change restores headroom for further P0 slices.
