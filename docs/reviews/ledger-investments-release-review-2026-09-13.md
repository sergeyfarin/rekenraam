# Ledger and investment release review — 2026-09-13

Reviewed revision: `b6207928`. Scope: readiness for v0.1 and one household's
real transactions, using one owner and one app process per database.

## Decision

**Hold the release and real-data onboarding.** Three confirmed P1 integrity
issues remain in supported write paths, plus one P2 fractional-investment
workflow defect. This supersedes the 2026-09-11 triage's statement that no known
supported workflow can silently corrupt financial state. No production fixes
are included in this review.

The existing accounting foundation is substantial: exact quantities,
per-entry/per-commodity balancing, append-only versions, atomic trade/lot
writes, disposal policy snapshots, and reconciliation/lifecycle tests. Passing
tests nevertheless missed the scenarios below. Neither statement coverage nor
this review can establish correctness for every possible edge case.

## Confirmed findings

### T-94 / P1 — reconciliation checks and commits are not atomic

Locations: `backend/internal/app/transactions_write.go:35` and
`backend/internal/db/transactions_write.go:72`.

`prepareCreateTransactionForWrite` reads checkpoint references before the write
transaction begins. The repository later trusts that list; an empty list skips
invalidation without checking whether a checkpoint now exists.

Deterministic reproduction using the actual service preparation and repository
commit phases:

1. Post 10 EUR on January 1.
2. Prepare another +10 EUR entry dated January 2, without an override.
3. Reconcile the first posting to a 10 EUR statement dated January 31.
4. Commit the already-prepared entry.

Observed: transaction 2 commits successfully and the checkpoint remains active.
A fresh preparation of the identical entry correctly returns
`ErrReconciliationOverrideRequired`. This is a logical interleaving, not a Go
memory race; the race detector and a single SQLite connection do not prevent
another request executing between preparation and `BeginTx`. Two app processes
are not required, so T-72's multi-instance deferral does not cover this.

The residue is also undetectable after the fact. `checkpointIntegrityCheck`
(`backend/internal/app/self_check.go:661`) sums only the postings *linked* to a
checkpoint and compares that to the statement balance, plus a superseded-version
count. The stale entry is linked to nothing, so both sums still agree and the
check reports "every active checkpoint still adds up". The write guard enforces
a stronger rule than the diagnostic does — any posting landing in a reconciled
window invalidates the checkpoint — so running a self-check after onboarding is
not a mitigation for T-94, and the diagnostic's own narrative overstates what it
verifies.

Required fix: resolve/enforce the reconciliation boundary against current state
inside the same transaction as the financial write. Cover create first, then
inspect update, lifecycle, reorder, import, and investment paths for the same
stale-validation pattern. Test both no-override rejection and deliberate
invalidation of every affected checkpoint. Test a checkpoint advancing after
preparation, too.

### T-95 / P1 — disposals consume lots acquired after the sale date

Locations: `backend/internal/db/investments.go:1300`, `:1387`, and `:2512`.

FIFO/LIFO and average-cost queries select current open lots without an
`opened_on <= event_date` restriction. Specific-lot disposal also does not check
the acquisition date.

Reproduced through `InvestmentService.Buy` and `Sell`: buy 10 shares on June 1,
then sell 10 on May 1. All four methods (`fifo`, `lifo`, `average_cost`,
`specific_lot`) commit the May sale and consume the June lot. This records a
negative historical holding and attributes basis to an acquisition that had not
yet occurred. With older eligible lots as well, future lots can also distort
LIFO selection and average-cost basis without causing a negative holding.

Every method funnels through `disposeLotTx` (`:2512`), which re-reads the lot
and already rejects mismatched account, commodity and status — but never
compares `opened_on` with the disposal date. That makes it the one choke point
where a date guard closes `specific_lot` and backstops the other three; the
per-method queries still need `opened_on <= ?` so FIFO/LIFO/average-cost
*select* eligible lots rather than failing at the sink.

Required fix: apply temporal eligibility consistently to simulation, committed
sales, specific allocations, and write-offs. Explicitly define safe handling
of out-of-order historical trades against the current operational projection:
filtering future acquisitions alone does not solve earlier sales entered after
later disposals, or backdated acquisitions that would change previous pooling.
Reject unsupported sequences or implement a deliberate replay/correction policy
that preserves disposal elections. Include same-day ordering and historical
imports in the tests.

### T-96 / P1 — ordinary entries can create journal-only investments

Locations: `backend/internal/app/transactions_validate.go:337` and
`frontend/src/lib/transactions/transaction-editor.svelte:176`.

Generic posting validation checks account eligibility and commodity precision,
but does not require an investment-aware workflow for a security holding.
The ordinary editor offers holding accounts; only template mode excludes them.

The existing investment lifecycle fence checks links on an existing transaction,
so it cannot protect the creation of a transaction with no lot links.

Scope is wider than the account kind named above. The backend app layer has no
concept of holding account kinds at all, so `fund_holding` and `crypto_wallet`
are equally unprotected, and the frontend's single fence excludes only
`security_holding` — `fund_holding` is offered even in template mode. A fix
must key off the investment-account kind set (`buy-form.svelte:93`), not off
`security_holding` alone.

Reproduced: create a balanced ordinary transaction with +10 security units in a
`security_holding` account and -10 units in another posting-enabled account.
The service accepts it, without creating any lot. The register now has shares
that the investment positions/gains subledger cannot account for. This is not
fixed by a later diagnostic reporting the mismatch.

Required fix: reject generic security-holding quantity changes, or route them
through an atomic subledger-aware domain workflow. Protect creation, edits that
introduce holding postings, opening balances, transfers, corrections, and import
producers; keep legitimate investment commands working. An ordinary balanced
transaction must never be a substitute for a missing investment transfer or
corporate-action workflow.

### T-97 / P2 — fractional sales depend on purchase text precision

Location: `backend/internal/db/investments.go:1348` (also the scale restrictions
in average-cost and specific-lot disposal).

Reproduced: buy 10 shares at quantity scale 0, then sell 0.5 shares at scale 1.
The commodity permits six decimal places. FIFO rejects the sale with
`invalid disposal parameters: sale quantity is not representable at lot 1's
quantity scale 0`. Writing the purchase as `10.0` can therefore change which
future sales are possible despite representing the same economic quantity.

This fails safely rather than corrupting balances, but blocks normal fractional
broker activity. Average cost additionally requires identical quantity and
basis scales across lots; tests currently certify rejection of mismatches.

Required fix: align exact quantities to a sufficient common scale and preserve
acquisition evidence separately from remaining projection precision. Verify
partial/full disposal, all methods, mixed import precision, residual basis,
preview/commit parity, and coefficient limits. Do not fix this by rounding the
requested sale or by asking users to rewrite posted acquisitions.

## Reproduction artifact

`fixtures/v0.1-ledger-investment-probes.go.txt` contains executable Go tests using
the existing application fixture and an isolated migrated SQLite database.
All seven cases fail their intended safety/validity assertion on the reviewed
revision: four temporal methods, fractional sale, generic holding entry, and
reconciliation interleaving. Setup succeeds in the final probe run.

It is stored outside the normal suite because this is a review with unresolved
findings, not a fix; it does not turn incorrect behavior into passing tests or
leave the default test suite deliberately broken. To reproduce from repo root,
first ensure the destination does not already exist:

```sh
cp docs/reviews/fixtures/v0.1-ledger-investment-probes.go.txt backend/internal/app/release_review_probe_test.go
(cd backend && go test ./internal/app -run '^TestReleaseReview' -count=1 -v)
rm backend/internal/app/release_review_probe_test.go
```

Expected before fixes: nonzero test exit. Promote these assertions into named
permanent regression tests with each fix, expanding the coverage listed above.

## Test coverage assessment

| Area | Existing evidence | Remaining risk / required coverage |
|---|---|---|
| Exact arithmetic and balance | 40 named `Test` functions in `internal/exact`; API validation, mixed-commodity and ledger math tests | Add generated/property cases over coefficient limits, scales, cancellation, sign and serialization. No `Fuzz` entrypoints found in the reviewed exact/app/db test files. |
| Transaction lifecycle | API tests cover void/unvoid, soft-delete/restore, draft promotion, tags, historic dates and balance-bypass prevention | Cross-workflow/state sequences and generic security-holding writes; assert failed operations leave versions, postings and audit history unchanged. |
| Reconciliation | Named checkpoint invalidation, category edit, transfer-leg, override, draft and boundary tests | Deterministic validation/commit interleavings; memory race tests cannot establish transaction isolation. T-94 is a concrete missing case. |
| Investment basis | 138 named `Test` functions across app/db/api investment-named files; conservation across methods, interleaved acquisitions/disposals, oversell rollback, average-cost residual, method-family lock and disposal provenance | Temporal eligibility, out-of-order events, fractional precision and generic entry bypass. Counts are inventory, not branch coverage. |
| Import and atomicity | Trading 212 service tests and post-write transaction hooks; named oversell rollback tests | Replay/duplicate imports with historical trades and mixed decimal precision, plus failures after each durable-write stage. |
| Diagnostics and durability | Self-check corruption cases, read-only check, interrupted-run recovery, migration/freeze and upgrade sentinel tests | Check historical negative holdings and event ordering, not just current quantity/basis totals. A diagnostic is not a write guard. |
| Frontend/browser | Money/form helper tests; release preflight has five financial/mobile journeys | Component interactions are not covered by plain TypeScript unit tests. Add browser cases for holding-account eligibility, decimal input through actual saves, historical trades and fractional sale. |

The seven probes demonstrate behavioral gaps in otherwise well-tested code.
Prioritize invariant/state-transition coverage over a larger global percentage.

## Validation performed

- `./scripts/test-backend.sh`: passed, including formatting, vet and the full
  race-enabled suite. Some package results were cached. This was the baseline
  run before introducing temporary probes.
- `./scripts/test-frontend.sh`: passed; Svelte checking reported zero errors and
  warnings, and 24 Vitest files / 377 tests passed.
- Application probes: ran with `-count=1`; all seven intended assertions failed,
  confirming the findings above. Temporary active test file removed afterward.
- `COVERAGE=1 ./scripts/test-backend.sh`: **corrected 2026-09-13.** This review
  first recorded a coverage-build failure blamed on a missing `covdata` tool.
  That does not reproduce. Go 1.27 no longer ships `covdata` as a separate
  binary under `$(go env GOROOT)/pkg/tool` — it is built into the `go` command,
  so `go tool covdata` works and an `ls` of the tool directory is not evidence
  that it is absent. A clean re-run passes and reports **78.9% of statements**
  merged across the backend. Coverage tooling is not a release gate.
- `pnpm test:release-preflight`: **corrected 2026-09-13.** A clean re-run passes:
  the managed web server builds and starts, and **7 of 7 tests pass with 0
  skipped** (2 bootstrap + 5 preflight journeys) in 40s. The earlier exit 1 was
  local, not a defect in the harness or the build; `pnpm build` also succeeds on
  its own.
- No production database was inspected or altered. Tests used isolated
  fixtures; the standard browser harness targets its disposable e2e database.

## Gates before household onboarding

1. Fix T-94–T-96 and pass permanent regression cases plus backend race checks.
2. Resolve T-97 before claiming fractional-investment support or onboarding a
   household whose broker trades fractions.
3. ~~Restore coverage tooling and run the browser release preflight
   successfully; check skipped counts because its journey group is serial.~~
   **Satisfied 2026-09-13:** coverage reports 78.9% and the preflight passes
   7/7 with 0 skipped. Keep both green, but neither is an open blocker.
4. Exercise a representative disposable household book: salary, bills, refund,
   credit card, transfer, split, reconciliation and correction, plus broker buys,
   partial sales, dividend, fees/withholding and historical import. Compare
   balances, shares and basis with source statements, export CSV/QIF, then
   verify a backup restore into a separate instance.
5. Acknowledge existing investment limits: native correction/reversal,
   splits/reverse splits, holding transfers and other corporate actions are
   not a complete onboarding story yet (R16). Do not improvise them with generic
   postings. Current gains are an operational view, not reproducible tax reports
   (ADR 0012). The app remains one owner, not shared family logins.
6. Start durable data only on the frozen baseline or its supported upgrades;
   do not reuse a database predating the final baseline consolidation.

These are release/onboarding gates, not a claim that broad feature expansion
must precede a small first release.
