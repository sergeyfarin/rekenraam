# ADR 0016: Native Backend Test Resources

## Status

Accepted (2026-10-12); amended 2026-10-24 by #402 (monitoring thresholds and
escalation order); remedy 1 measured and closed by #404 the same day. Supersedes ADR 0015 decision 5's retained serial-package
schedule and 20-minute deadline. FULL durability and native-driver admission
remain unchanged.

## Context

The owner clarified that the post-migration review should update the old test
resource limits. The modernc-era 20-minute deadline and serial-package schedule
were responses to translated-engine race costs. Those costs are gone.

With native SQLite and FULL, three four-CPU serial-package runs put application
race execution at 109.964–114.740 seconds. The corresponding CI run takes
56.508 seconds. A 20-minute package deadline now delays detection of hangs and
large regressions unnecessarily. The earlier four-to-six-core scaling advice
also described modernc, not the native implementation.

## Decision

1. The shared local/CI backend wrapper uses a **five-minute per-package
   deadline** for both race and ordinary coverage tests. All financial,
   migration, recovery and concurrency assertions remain; no faster test-only
   database setting is introduced.
2. Run **two packages concurrently** (`-p 2`) in both modes. Tests inside each
   package retain their isolated fixtures and ordinary Go parallelism. A
   matched native scheduling comparison must retain at least 30% application
   headroom under the new 300-second bound before this schedule is admitted.
3. Do not set a fixed `GOMAXPROCS`, test `-parallel` value or CPU affinity in
   normal validation. Use available Go CPU resources. Four CPUs describe the
   controlled scheduling comparison and historical driver comparison, not a
   minimum deployment requirement or a normal development cap. Retire the
   modernc-specific recommendation to restrict test concurrency to four cores.
4. Let CI's integrated-build job run independently of backend and frontend
   checks. It is a separate validation job with its own build prerequisites;
   all CI jobs still have to pass. This removes the serial wait without
   changing validation frequency or publishing any artifact.
5. Keep the source/feature configuration identical to production: bundled
   native SQLite, CGO, FTS5/netgo/osusergo, FULL and all Go race instrumentation.
   The native C engine remains outside Go race coverage.

## Validation and consequences

The [native gate review](../reviews/native-test-gate-2026-10-12.md) records the
matched scheduling runs, case-name digest, resource limits and local/CI results.
Command wall time and package execution are separate metrics: concurrency
shortens the gate while increasing some individual package times.

Five minutes is a failure bound, not an execution target. Future suite growth
or sustained runtime pressure should trigger measured optimization before
raising it. A CPU limit can still be used for a declared diagnostic comparison;
it must not quietly become the default developer command.

## Amendment — race-suite cost thresholds, 2026-10-24

[#402](https://github.com/sergeyfarin/rekenraam/issues/402) measured the
application package at `935ccdb7` on a ten-CPU host. The
[record](../reviews/race-suite-cost-2026-10-24.md) has the matched runs and the
profile. Results:

- Single package: median 225.7 s.
- Full `-p 2` gate: 276.6–277.7 s.
- CI per package: about 183 s.

Migration replay is 4.5% of the package's CPU. Statement compilation is 32.7%,
and per-connection schema loading is 19.4%: every test pays for schema size.
The earlier 200-second trigger did not name a measurement scope, so it is
replaced by these thresholds.

- **Investigate** when either median exceeds **210 s**: three interleaved,
  uncached single-package runs of `./internal/app` on the reference ten-CPU
  host, or CI's per-package time over the latest six successful `main` runs.
- **Act before the next feature slice** when two uncached full-gate runs
  (`GOFLAGS=-count=1 GO_PHASE_TIMINGS=1 scripts/test-backend.sh`) both exceed
  **270 s** (90% of the deadline) for any package.
- Single runs, and CI jobs without a matched base, are observations, not
  triggers. The wrapper does not bypass Go's test cache, so timing runs set
  `-count=1`.

Remedies are taken in this order:

1. Reduce statement compilation cost, measured
   ([#404](https://github.com/sergeyfarin/rekenraam/issues/404)). First reuse
   statements explicitly at the hot production sites, then evaluate the native
   statement cache. Prefer explicit reuse when it recovers most of the gain;
   adopt the global cache only after its correctness review.
2. If the full-gate threshold still fires, choose between reducing schema load
   cost and raising the deadline. Reducing schema load means fewer or smaller
   triggers and views, preferably before v0.1.0 while migrations can still be
   rewritten. A deadline change needs a new amendment with matched evidence.
   Schema slimming is already scheduled as a v0.1.0 release gate
   ([#405](https://github.com/sergeyfarin/rekenraam/issues/405)), after #404.

Do not recover headroom by:

- splitting packages to fit the per-package bound;
- trimming coverage or upgrade paths;
- pooled or reset fixtures;
- faster test-only database settings;
- lowering `-p` or test parallelism.

At `935ccdb7` the single-package and full-gate thresholds have fired; the CI
threshold has not. #404 is the response.

**#404 result (2026-10-24).** Remedy 1 is done. Explicit statement reuse in
five hot loops lowered the single-package median from 223.3 s to 219.4 s and
moved the full-gate pair from 280.2/276.6 s to 255.5/274.1 s. The act
threshold no longer fires, by a thin margin; the investigate threshold still
does. The native statement cache measured −4.4% here, about 2.7% beyond
explicit reuse, and the owner declined it: `_stmt_cache_size=0` stays. Remedy
2, schema slimming in #405, is next. See the
[#404 record](../reviews/statement-compilation-2026-10-24.md).

**#405 result (2026-10-26).** Remedy 2 is delivered (`f4ee790f`). Against the
R13-inclusive `9b414c08`, the slimmed baseline and obsolete-transition retirement
lowered the isolated application median 240.7 → 229.6 s. Both original full-gate
application runs exceed 270 s (271.8/289.0); both proposed runs are below it
(248.5/262.5). The act threshold no longer fires; investigate and the unmet 30%
headroom objective remain. These modest suite gains do not establish the same
gain for production requests. Concurrency, deadlines, FULL and cache policy
remain unchanged. The [review and evidence](../reviews/schema-baseline-review-2026-10-26.md#measurements-and-validation)
record native operations, spread, case changes and validation limits. Hosted
Backend Tests and Coverage pass at `4b3ef084`; hosted runners are not a timing
measurement. No further policy amendment is made. Reprofiling the new baseline
against the investigate threshold is #419.

**#419 result (2026-10-29).** At `d524eecf`, three uncached isolated
application runs have median **226.3 s**; interleaved fresh #405 runs have
median **219.5 s**, with a large same-source spread. Both unchanged full gates
now **time out at 300 s** in the application package; all other packages pass.
The durations are censored, not completed timings. Both investigate and act
fire; the response is required before the next feature slice. The complete
profiled application run passes, separately from the timing median.

Explicit preparation is **30.69%** of sampled CPU and driver connection setup
(including schema load) **17.79%**. Migration work is **0.82%**; 19.33% is
unattributed external code, not safely assignable to those buckets. Ranked
recommendation: (1) a bounded, equivalence-preserving compilation experiment
on foundation self-check, effective lot movements and replay-intent reads;
(2) further trigger/view slimming only with a specific semantic-equivalence
case and measured gain; (3) an explicit owner resource/deadline decision if
bounded production optimization cannot clear act. The first candidate's
preparation union is 9.11% of CPU: halving it suggests 4.55% CPU savings,
roughly 10 isolated seconds only if wall time scales proportionally. It does
not promise 30% headroom. No deadline, concurrency, durability, cache or
coverage policy changes here; the global cache remains declined.

The [report and evidence](../reviews/race-suite-reprofile-2026-10-29.md)
record phase/package timings, case digests, profile attribution and probes.
The owner authorized publication on 2026-10-29; the chosen remedy is filed as
[#445](https://github.com/sergeyfarin/rekenraam/issues/445) with measurable
acceptance. This completes #419's measurement scope, not the remedy. The act
rule remains in force before the next feature slice.

**#445 result (2026-10-29).** The bounded read-query experiment reduces
preparation of one transfer replay statement by **26.9–27.6%** across three
alternating native measurements. Query results and observed execution plans
are unchanged. All six isolated application race runs retain the same 2,476
leaf cases; original/proposed medians are **215.5 / 221.9 s**. This does not
demonstrate a suite speedup. Both fresh original full gates are already below
270 s (254.8 / 260.6 s), so the difference from #419's historical timeouts
cannot be attributed to the query change.

Both proposed uncached full gates pass at **263.4 / 269.2 s** in the
application package. The #445 clearance criterion (both below 270 s) is met
on this pair, with only **0.8 s** to the threshold in the slower run. The act
trigger no longer fires on the measured pair; investigate and the unmet
**30% headroom** objective remain. Keep monitoring under the existing
thresholds. No deadline, scheduling, cache, durability, coverage or schema
policy changes. The [report](../reviews/investment-read-preparation-2026-10-29.md)
retains SQL, fixture/semantic controls, exact sources, all matched timings and
their limits.

**#356 driver recheck (2026-11-10).** The SQLite 3.54.1 candidate was measured
with the explicit uncached gate (`GOFLAGS=-count=1 GO_PHASE_TIMINGS=1
./scripts/test-backend.sh`) on a six-CPU workspace: baseline **225.1 / 232.3 s**,
candidate **245.7 / 293.6 / 271.3 s** in the application package. Two candidate
runs exceed the **270 s act threshold** while neither baseline run does, so the
act rule fires; all runs still finish under the 300 s deadline. A provisional
uncached gate on the Go 1.27.4 security-patch worktree measured **279.089 s**,
so the toolchain patch does not change the conclusion. Hosted runs
(**216.8 s** `2eb5adb4`; **252.7 s** `b6655526`; **253.2 s** `faeedf20`) are
cached-gate observations, not threshold evidence. The investigate threshold
also remains exceeded. Production-query cost is separately measured at **+16%
to +63%** on representative affected paths. **Owner amendment (2026-11-11):**
the owner accepted this act-threshold breach and the production cost for the
#356 driver admission, which satisfies the act rule for this change; a
speed-up follow-up is separate. No deadline, scheduling, cache, durability,
coverage or schema policy changes are made here. The investigate threshold
and the 30% headroom objective remain open. See the
[admission review](../reviews/sqlite-driver-admission-2026-11-10.md#owner-acceptance-2026-11-11).

**#356 speed-up follow-up (2026-11-11).** The 3.54.1 cost is a second
compilation of every joined statement whose bound equality takes part in
constant propagation. Building the engine with `SQLITE_ENABLE_QPSG`
(ADR 0014 amendment) removes it. Isolated uncached `internal/app` race runs on
a four-CPU workspace, alternating builds, measured **306.5 / 297.2 s** without
QPSG and **255.2 / 250.0 s** with it (−16% to −17%). These are not
six-CPU `-p 2` full-gate threshold runs; the formal act/investigate
measurement on the threshold host remains to be repeated. Remedy order,
deadline, scheduling, cache, durability and coverage policy are unchanged. See
the [QPSG review](../reviews/sqlite-qpsg-2026-11-11.md).

## References

- [ADR 0014](0014-native-sqlite-driver.md), native driver admission.
- [ADR 0015](0015-sqlite-commit-durability.md), FULL durability.
- [FULL timing evidence](../reviews/sqlite-full-durability-2026-10-12.md).
- [#164](https://github.com/sergeyfarin/rekenraam/issues/164), development feedback.
- [Race-suite cost investigation](../reviews/race-suite-cost-2026-10-24.md), #402 amendment.
- [Statement compilation cost](../reviews/statement-compilation-2026-10-24.md), #404 result.
- [Post-baseline race reprofile](../reviews/race-suite-reprofile-2026-10-29.md), #419 result.
- [Investment read preparation](../reviews/investment-read-preparation-2026-10-29.md), #445 result.
