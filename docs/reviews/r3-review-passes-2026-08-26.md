# R3 review passes 1–6 — 2026-08-26

The six passes `r3-verification-review-2026-08-24.md` listed as "not done" are
done. This is what they found.

**Status: all findings resolved 2026-08-26.** Seven defects and two useless
tests, across four commits. Nothing found here was a data-loss bug; two were
"the app tells the user something untrue", which in a backup-and-restore
feature is the same category of problem one step earlier.

## What each pass found

| Pass | Findings |
|---|---|
| 1 — claim audit | Seven dead pointers into migrations that no longer exist; two counts that had drifted; two ADR clauses that had drifted from the code |
| 2 — money-path coverage | QIF splits had never been exported at more than one scale |
| 3 — failure-branch walk | A failed restore gave one message for three different disk states, one of which it described wrongly |
| 4 — contract-vs-implementation | Nothing pinned `ledger.csv`'s published column list; the export-order test could not fail |
| 5 — concurrency | Check-then-insert on the backup occurrence; a crash-adopted backup recorded `page_count 0`; the work queue had no test of its own |
| 6 — frontend state | The backup form wrote every keystroke into the shared query cache |

## The findings, in the order they matter

### P-1 — an unsaved edit came back as the configured schedule

The Data screen's backup policy inputs were bound straight to
`backupQuery.data.policy`, which is the TanStack cache entry: shared with every
other reader of that key, and outliving the component. Typing in the form
rewrote the app's idea of what the server had said. Change the retention count,
leave without saving, come back, and the abandoned number is sitting there
reading as the configured one.

The first attempt to reproduce it navigated with `page.goto` and **passed** — a
full reload discards the cache the edit is hiding in. Only client-side
navigation, which is what a user does, shows it. That near-miss is the finding
behind the finding: a browser test can navigate in a way no user ever does.

Fixed by giving the form its own copy, re-seeded only when the server's answer
actually changes, so a background refetch no longer wipes an edit in progress.

### P-2 — a backup adopted after a crash claimed to hold no pages

When a crash lands between the rename and the run being recorded, `produceBackup`
verifies the file already in place and adopts it. It reported the size and left
the page count at zero, and the run was written as completed and verified. The
existing crash test asserted the run completed; it did not ask what the run said
about the file.

Fixed by measuring the adopted file. The online backup API's page count is the
source's count at the end of the copy, which is the destination's, so the copied
and adopted paths now agree by construction rather than by coincidence.

### P-3 — a failed restore described the wrong disk state

Three states are possible when `rekenraam restore` fails, and they need
different responses: nothing moved; the previous database moved aside and
**nothing is at the database path**; or the restored database already installed
and the failure afterwards. The command printed one sentence for all three,
from a field assigned before the first rename — so it claimed data had been
preserved when nothing had moved, and, in the case where the database path was
empty, never said the files had to be moved back.

Fixed with `RestoreResult.Installed` and three messages. Telling someone to move
files back over a restore that already succeeded would be worse than silence,
which is why the third case is separate rather than folded into the second.

### P-4 — two schedulers racing produced an unexplained error

`CreateBackupRunWithWork` did check-then-insert. Both processes read "no run for
tonight" and the loser's INSERT came back as a generic wrapped error —
so `scheduleBackupIfDue`'s branch for exactly this, commented "losing the race
is the normal outcome", never ran for the race it names. The unique index now
decides and the SELECT is gone.

### P-5 — nothing pinned the published column list

ADR 0011 clause 6 publishes `ledger.csv`'s columns as append-only. Every export
test reads columns *by name* through the header, which is precisely what makes
them survive the reorder that breaks a consumer parsing by position. A golden
list now fails on a rename, a reorder, a removal, or a duplicate.

### P-6 — QIF splits had only ever been tested at one scale

A posting is recorded at any scale its commodity permits, so one entry can hold
a scale-0 leg beside a scale-2 one, and each split must render at its own
counterpart's scale. Borrowing the record's scale looks like a tidy
simplification and is wrong by a factor of a hundred per decimal place — the bug
this project has shipped three times (T-36/T-45/T-47). Now pinned, and verified
by making that exact change and watching the test fail.

### P-7 — seven pointers to migrations that do not exist

T-64 collapsed every migration into `0001_initial_schema.sql` on 2026-08-24.
Two days later, six rows of `implemented.md` and three lines of `todo.md` still
sent a reader to "migration 0002" through "migration 0007". The numbers are
gone, the tables are named, and a line at the top of `implemented.md` states the
convention so the next person does not reinvent the broken pointer.

## Two tests that could not fail

Both were written *during* this review, which is the only reason they were
caught, and both are recorded because writing one while reviewing for exactly
this failure mode is worth remembering.

1. **"Two exports of one snapshot are byte-identical."** It held with the
   ORDER BY replaced by a deliberately partial one: SQLite returns the same scan
   order for the same query on unchanged data. Replaced with an assertion on the
   documented order itself, verified to fail when that order is broken.

2. **"Two workers never claim one item."** It held with the claim's status
   re-check removed. The main pool is `SetMaxOpenConns(1)` (ADR 0004), so each
   claim transaction holds the one connection for its whole length and two
   SELECTs in this process never interleave. The re-check exists for a second
   process on the same file, which no in-process test can stage. The test kept
   its other, real assertions — every item claimed exactly once, the queue then
   empty, the lease honoured and stealable only once expired — and says in its
   comment what it cannot reach.

The general rule this suggests, beyond the three the last review recorded:

4. **A new test is run against a deliberately broken version of the thing it
   names.** If it still passes, it is not a test yet. Both of the above passed
   that check only on the second attempt, and the export-determinism one could
   not be repaired at all — it had to be replaced with a different assertion.

## What these passes did *not* cover

The same limits as the last review, minus the two it named: concurrency is now
tested at the queue level, and the frontend has one browser test for cache
state. Still untouched: behaviour against a large or long-lived book, two
*processes* on one database file, and anything outside R3 except where the
reporting-currency work reached into it.

`app.scaledDivision` (int64-bound, `investments.go`) should still migrate onto
`exact.MulDivRound`, which the reporting currency added. That is a refactor with
a real correctness argument behind it — the int64 bound is a silent ceiling
where the big.Int path has none — and it is the largest known piece of debt in
the money paths.
