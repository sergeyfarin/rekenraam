# R3 verification review — 2026-08-24

**Status: findings resolved 2026-08-24.** V-1 to V-7 below all landed —
T-65 to T-69 are closed in `backlog.md`, and V-7 is documented rather than
changed. The six further passes at the end were run on 2026-08-26 and are
written up in `r3-review-passes-2026-08-26.md`; they found seven more defects
and two tests that could not fail. This document is a point-in-time review, not
a tracker.

Resolving them turned up one defect the review had not seen: retrying a backup
that was still queued answered `404 "backup run not found"` about a run the
caller was looking at, because requeue matches only failed work items. Fixed
with T-68. That is the fourth wrong-reason error message this project has
found — T-63 is the open one — which is itself worth noticing.

R3 shipped in eight slices over two days, and a gap turned up at almost every
one. Most were caught before they shipped — by a test, by a reviewer, or by
checking a claim — but "caught most of them" is not the same as "there are no
more", and the pattern deserved measuring rather than reassuring.

This is that measurement: what the gaps actually were, what causes them, what a
first verification pass found, and what the remaining passes should look at
before R3a starts.

## What the gaps were

Every defect and correction during R3, by how it was found:

| Found by | Count | Examples |
|---|---|---|
| A test, before shipping | 6 | QIF header mismatch; sidecar orphans after rename; unbalanced-export blocked by trigger; e2e cross-spec pollution |
| A reviewer, before shipping | 8 | Trial-balance identities under scope and under transaction basis; ambiguous-QIF impossibility; WAL destruction in restore; the secret key |
| A reviewer, **after** shipping | 1 | QIF manifest reporting `rows: 0` for a non-empty file |
| Checking a claim while doing later work | 4 | `foreign_key_check` already present; comma-profile behaviour; append-only triggers; T-64's own trigger |
| A review pass looking for it | 1 | The attachments hook missing from backup documentation |

Two things stand out. The one defect that shipped and needed a follow-up commit
was in a **branch no test exercised** (the pre-rendered archive path). And all
four "checking a claim" corrections were *documentation* asserting behaviour
nobody had opened the file to confirm.

## Root causes, stated plainly

1. **Plausible behaviour written down as fact.** Four planning claims were
   wrong. Each was reasonable, none was checked. Cost: one duplicated
   implementation, one impossible promise made to a reviewer, two corrections.
2. **Arithmetic reasoned about rather than enumerated.** The trial balance was
   wrong twice — first under account scope, then under transaction basis —
   because each design considered the cases it had in mind rather than the
   cross-product of filter kinds.
3. **Branches without tests.** State threaded through a callback or a rarely
   taken branch was where the shipped defect lived, and the coverage numbers
   below say there are more such branches.
4. **Tests that assert their premise instead of their name.** Finding V-1.

## First pass: coverage-directed review of shipped R3 code

Measured with `go test ./... -coverpkg=./...`, which attributes cross-package
execution — per-package coverage reports 0% for export code that the API tests
exercise thoroughly, and reading that number without `-coverpkg` would have sent
this review in the wrong direction entirely.

### V-1 — a restore test does not test what its name says `[T-65]`

`TestRestorePreservesUncheckpointedWALContent` closes the database before
restoring. **Closing a SQLite database checkpoints and deletes its WAL**
(verified directly: 947 KB `-wal` before `Close()`, no file after). By the time
the restore runs there is no WAL, so `checkpointStoppedDatabase` returns at its
first line and the scenario the test is named for never occurs. It passes
because the data was already folded into the main file.

The behaviour may well be correct. Nothing verifies it, and the function sits at
**11.8% coverage** in the most safety-critical path R3 shipped. A test must
leave the WAL in place — hold a second connection open, or copy the file set
aside while a writer is live — and then assert the preserved copy carries what
only the WAL held.

### V-2 — the nightly schedule has no test at all `[T-66]`

`StartScheduler` and `scheduleBackupIfDue`: **0%**. The code that decides a
backup is due — local-time arithmetic across a DST boundary, the
already-ran-today check, the owner lookup, the lost-race path — is the whole of
"backed up nightly", and no test executes any of it. A regression here fails
silently in the direction that matters: no backup, and a screen that truthfully
says so to someone not looking.

### V-3 — the work-queue path around the backup has no test `[T-66]`

`StartBackgroundWorker`, `runDueBackups`, `processBackupWork`: **0%**.
`RunBackup` is tested directly, but claim → complete, claim → retry with
backoff, and claim → fail at the cap are not. One thing I did verify by reading:
`ClaimBackgroundWork` does `attempts = attempts + 1`, so the cap can actually
trigger and this is **not** a repeat of T-39. Everything else there is unproven.

### V-4 — `lots.csv` and `prices.csv` have never been written with a row `[T-67]`

Both at **46.7%** — the loop bodies never execute, because no bundle fixture has
an investment lot or a price observation. These files carry money: cost basis at
its own scale, prices at theirs. Their column order, their scale handling, and
their `exact.Decimal` calls are unverified. An export that silently writes cost
basis in the wrong column would pass every test in the suite.

### V-5 — the two recovery paths are barely covered `[T-68]`

`RetryBackupRun` (**20%**) is the documented way back from an exhausted attempt
cap — a cap is only safe with one. `reportSealedData` (**20%**) covers only its
"no sealed data" branch; the branch that matters, where a key is configured and
a sample either opens or does not, is exercised only at service level, not
through the command an operator actually runs.

### V-6 — a failed run is indistinguishable from a given-up one `[T-69]`

`RunBackup` marks the run `failed` on every attempt, while the work item may
still be queued for retry. The Data screen shows "failed" for a backup that is
about to succeed, and the same word for one that has exhausted its cap. No data
is at risk; the screen is telling a reader two different things with one word.

### V-7 — a night that exhausts its retries produces no backup, silently

The scheduler skips a day when *any* scheduled run exists for it, whatever its
status. If a night's run fails five times, no further attempt is made that day
and the only way back is the retry button. That is defensible — retrying a full
disk every minute helps nobody — but it is not written down anywhere, and the
"nightly" promise quietly has an exception.

## What the remaining passes should look at

Ordered by expected value, and each is a pass over shipped code rather than a
plan to write more of it.

| # | Pass | What it checks | Method |
|---|---|---|---|
| 1 | **Claim audit** | Every factual assertion in `data-portability-plan.md`, ADR 0011, `implemented.md`, and `README.md`'s backup sections | Open the file it names. Four of these were wrong; the rest have never been checked either |
| 2 | **Money-path coverage** | Every place R3 renders or sums a monetary value: `lots.csv`, `prices.csv`, trial balance under crypto scales, QIF splits at scale 0 | Coverage-directed, plus one fixture per money-bearing file |
| 3 | **Failure-branch walk** | Each error return in the backup, restore, and export paths: what state is left behind, and can the next run recover from it | Read each `return err`, ask "what is on disk now", write the case that produces it |
| 4 | **Contract-vs-implementation diff** | ADR 0011's stated guarantees against what the code does, clause by clause | The ADR is what a consumer builds against; nothing currently cross-checks it |
| 5 | **Concurrency review** | Two workers, two schedulers, a manual backup during a scheduled one, a restore racing a running server | Read for TOCTOU; the occurrence key and the lock are the defences, and neither has a concurrent test |
| 6 | **Frontend state review** | The Data screen's four states per panel, and what it shows while a mutation is in flight | It shipped with no component tests; the repo has no component harness (G-02), so this is a read-and-check pass |

## What should change about how the next slice is built

Three cheap rules, each aimed at a root cause above:

1. **A claim about existing behaviour is not written until the file is open.**
   Cost: seconds. It would have prevented four corrections.
2. **An arithmetic contract is written as a table of cases before it is written
   as prose** — filter kinds × date bases × empty/non-empty. Both trial-balance
   errors were missing rows in a table nobody drew.
3. **A branch introduced for a specific scenario ships with the test for that
   scenario, or with a comment saying why it cannot have one.** The one defect
   that reached `main` was a branch added for the ambiguous-QIF case that no
   test entered.

## What this review does not claim

It is a coverage-directed read of R3's Go code and a re-check of its documents.
It has not: exercised the app against a large or long-lived book, tested
concurrent workers, verified the frontend beyond type-checking and two browser
cases, or examined anything outside R3. Passes 1 to 6 above are what would.
