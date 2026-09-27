# Open backlog before GitHub issue migration — 2026-09-27

**Status:** The migration completed on 2026-09-27. Current status and priority
are in [GitHub Issues](https://github.com/sergeyfarin/rekenraam/issues), with
the local ID mapping in [docs/backlog.md](../backlog.md).

This is a fixed snapshot of the actionable issue descriptions after the
2026-09-27 recheck. `docs/backlog.md` remains live until the issues are
published and linked. Once migration completes, use the GitHub issues for
current status; this file preserves the source acceptance context.

# Technical Backlog

This is the active defect and technical-debt registry. Some entries are
scheduled product work and are marked with their dependency. Closed-item
evidence is preserved in the dated resolution records.

- **Feature sequence:** `docs/roadmap.md`.
- **Short-horizon queue:** `docs/todo.md`.
- **Shipped capability:** `docs/implemented.md`.
- **Resolved through July:** `docs/reviews/resolved-backlog-2026-07.md`.
- **Later closed-item evidence:** `docs/reviews/resolved-backlog-2026-09-27.md`.

Status legend: `[ ]` open · `[~]` partly done, with the remainder stated ·
`[blocked]` open with a stated dependency that must land first. Closed items
move to `docs/reviews/`.

**A note on IDs T-42–T-47.** Two long-diverged branches independently used
these six numbers for six *different* defects/features before merging. Rather
than silently pick one meaning and erase the other's history — both are real
and both are merged in — the branch that used them first in already-resolved,
frozen history (`docs/reviews/resolved-backlog-2026-07.md`, `docs/design/`)
keeps T-42–T-47 with its original meaning (the genesis-date fixes and the
frontend decimal-comma bugs, all closed). The other branch's same-numbered
items are renumbered **T-48–T-53** in this file and the
[resolved record](resolved-backlog-2026-09-27.md): T-42→T-48 (TS7), T-43→T-49 (gofmt),
T-44→T-50 (payee resolution), T-45→T-51 (net-worth perf), T-46→T-52 (CSP),
T-47→T-53 (investment reconciliation override).

Every code comment and test name has been rewritten to the renumbered ID, so
an ID in the tree now has exactly one meaning and needs no lookup. Note that
T-44 was overloaded across *both* sets: the holding-account opened-date
comments in `import_trading212_invest.go` are the surviving T-44, while the
payee-resolution comments are T-50. Only the frozen `docs/reviews/` and
`docs/design/` files still use the pre-merge numbering — that history is not
rewritten, and this mapping is how to read it.

## General

### T-108 Explicit short-sale positions need a named workflow `[ ]`

An intentional short sale creates a real negative share position. It must be
entered and displayed as a short sale, with its own position, cover, proceeds,
and cost-basis rules. The current ordinary-entry and investment-sale paths do
not identify that intent. Until a dedicated short-sale contract and workflow
exist, a negative countable position is reported as unclassified for review;
it is neither silently accepted as a valid short nor forcibly discarded.
ADR 0013 specifies named operations, separate long/short lot sides, cover
allocation, journal balancing, and dated classification. Deliver the operation
and replay foundation before opening this workflow in the UI.

### T-34 No producer of investment provider events/suggestions `[blocked]`

**Depends on** (assessed 2026-08-07 — this is scheduled product work, not a
fix that can be taken out of order):

- **R15, third slice.** The roadmap sequences the T-34 producer explicitly
  after IBKR Flex and GoCardless (`docs/roadmap.md` "Deliberately later";
  `docs/plans/connections-plan.md` § Sequencing). It is an adapter slice —
  provider package, credentials, prober, background fetch, cursors — not a
  defect with a local fix.
- **A provider decision that is still open.** Financial Modeling Prep is the
  leading free candidate for dividends and splits, but its EU coverage is
  unverified, and `connections-plan.md` records provider verification as a
  *blocking slice-start precondition*, not a task inside the slice.
- **R16's per-kind structural action specification**, for the structural half
  only. The shared operation/lot design is now in
  `docs/plans/investment-operation-refactor-plan.md`, but each split, merger,
  spin-off, ticker change, or delisting still needs its own posting, basis,
  date, and replay rules before acceptance. `AcceptSuggestion` rejects them
  today by design. Dividend suggestions need no structural rule and would
  work the day a producer exists.

Nothing in the app is broken by this: the review UI, accept/ignore, and the
automation rules all work, they simply have no data. Do not start it early.

**Files:** `backend/internal/app/investments.go` (`DividendProvider`,
`CorporateActionProvider` interfaces, declared but never implemented or
referenced); `investment_provider_events` / `investment_event_suggestions`
tables (`backend/migrations/0001_initial_schema.sql`).

Verified 2026-07-15 (Workstream 3 investment test coverage): nothing in the
codebase writes to `investment_provider_events` or
`investment_event_suggestions` — no fetcher, worker, or import path creates
them. The review UI and the accept/ignore/automation-rules endpoints all work
correctly, but have no data to act on until a producer exists.
`docs/product-requirements.md` lists "provider events and reviewable
suggestions" as a real requirement. Needs: a chosen data source (no candidate
picked yet), a fetch/detection design, and — separately — a per-kind posting,
basis, date, and replay specification for structural corporate actions (split,
merger, spin_off, ticker_change, delisting, `corporate_action`), which
`AcceptSuggestion`
currently rejects outright. Dividend and distribution income suggestions can
be accepted; cash-in-lieu and return-of-capital suggestions are refused until
their basis-aware workflows exist (T-109).

### T-48 TypeScript 7 upgrade blocked by `openapi-typescript` `[blocked]`

**Files:** `frontend/package.json` (`typescript`, `openapi-typescript`);
`frontend/src/lib/api/schema.d.ts` generation step.

TypeScript 7.0.2 is released, but `openapi-typescript` 7.13.0 (latest) still
declares `peerDependencies.typescript: ^5.x` and emits `schema.d.ts` through
the TypeScript JS compiler API (`ts.factory`). TS 7 no longer exposes it, so
`pnpm run openapi:generate` fails immediately with
`TypeError: Cannot read properties of undefined (reading 'createKeywordTypeNode')`,
taking `dev`, `check`, and `build` down with it. The frontend is held on
TypeScript 5.9.3 — inside `openapi-typescript`'s declared peer range — until
it ships TS 7 support; re-check on each `openapi-typescript` release. The
`typescript@7.0.2` release-age excludes have since been dropped from
`pnpm-workspace.yaml`, so the retry needs them added back alongside the
version bump.

### G-09 `.svelte` files carrying private money math `[~]`

Found 2026-08-08 by sweeping every `.svelte` file for inline coefficient
arithmetic, once G-02's investment forms were done.

**Done — `lib/transactions/category-transactions.svelte` (2026-08-08).** It
held a private `rescaleUp` plus per-commodity scale-aware summing with its own
income/liability/equity sign switch. That switch is the ledger's **normal-sign
convention** — the same rule `inflowPositiveAmount` applies to a single posting
and `balanceMapToQuantities` applies on the backend — so the component was
carrying a third copy of a rule that already existed twice. Retired onto a new
`sumByCommodity` in `$lib/money/amount.ts`, table-tested for the sign rule per
account class, scale alignment, commodity separation, refund netting, and
exactness past `Number.MAX_SAFE_INTEGER`.

**Not a defect after all — `lib/investments/gains-report.svelte`.** Recorded
initially as the highest-priority item of the three, on the strength of a
`BigInt(Math.trunc(Number(value)))` at line ~55. Checked properly: that
conversion feeds **only `gainClass`**, whose entire output is a CSS colour
chosen by a sign comparison, and `Number()` rounding preserves sign.
The original audit also accepted `proceeds_scale` for realized gains; that
assessment was superseded by T-101. Gains now carry `realized_gain_scale`
so subtraction preserves the deeper basis precision. The 2026-09-20 follow-up
uses shared `formatMoney` for currency-standard summary display while retaining
exact arithmetic and quantity formatting. The numeric investment API contract
still needs its separate string-coefficient migration.


**Open, and larger than first recorded —
`routes/app/settings/currencies/+page.svelte`.** Two separate issues in
`scaledRatioToDecimalText` (lines ~391–419), which renders an FX rate as a
ratio of two scaled values:

1. **Ad-hoc truncating division on an implied price.** It does integer division
   and strips trailing zeros, truncating at 8 decimals. `ledger-invariants`
   states that division and implied prices round **half-up via the shared
   helpers** and that ad-hoc rounding is never to be written. Truncation on a
   *displayed* rate is defensible, but it is inconsistent with the stated rule
   and with `scaledDivision` on the backend, and the difference is visible to
   the user at the 8th decimal.
2. **`Number(decimalText)` before formatting.** The function computes an exact
   decimal string and then throws the exactness away to hand a `double` to
   `Intl`. Harmless at realistic FX magnitudes, but it is the one remaining
   place in the frontend where a `Number` touches a computed money value, and
   `formatQuantity` exists precisely so it does not have to.

Issue 2 is a mechanical fix. Issue 1 needs a decision first — whether a
displayed rate truncates or rounds half-up — so this was **not** folded into
the 2026-08-08 sweep silently. The investment money-field inconsistency was
closed by T-107; the FX display conversion remains independent of R16.

### G-08 Amount input is not locale-aware `[~]`

**Priority P1 — cross-border entry correctness.** Display formatting already
uses the active locale, and malformed grouping is rejected, so the earlier
100× interpretation bug is fenced. Editable amount parsing still uses an
English-style separator regardless of the active locale. A user entering
`1,50` in a decimal-comma locale cannot use the expected input form.

**Current evidence (2026-09-27):** `frontend/src/lib/money/amount.ts` owns
`parseDecimalAmount` and `formatLedgerAmount`; locale-aware display lives in
`frontend/src/lib/money/format.ts`. Five drafted non-English locales are already
selectable. The old historical investigation is in Git history; T-45/T-47 in
`docs/reviews/resolved-backlog-2026-07.md` cover the silent misparse that is
already fixed.

**Done when:** resolve separators from the active locale for both editable
input and form rehydration, reject ambiguous grouping, and test `en` plus all
five non-English locales with exact coefficients beyond JavaScript's safe
integer range. Preserve the locale-independent API/storage format and the
existing import-file-level separator decision.

### T-54 Price derivation lives in opaque JSON, not an indexed table `[ ]`

**Files:** `backend/internal/db/pricing.go`
(`activeDerivedObservationIDsTx`, `VoidPriceObservation`);
`backend/internal/app/pricing_refresh.go` (`fxDerivationJSON`).

Which observations a derived price came from is recorded only inside
`price_observations.derivation_json`. Following the graph therefore means a
`json_each(json_extract(...))` scan of the whole table **per node**, with no
index available, and the cascade issues one such scan per level. The void
cascade was made to abort rather than commit partially when it cannot reach the
end of the graph (fixed 2026-08-23, `ErrPriceVoidCascadeTooDeep`), so the
correctness hole is closed — this item is the cost and the ceiling that remain.

Replace with an explicit `price_observation_dependencies (observation_id,
source_observation_id)` table written in the same transaction that inserts a
derived observation, indexed both ways. That makes the cascade a recursive CTE,
makes "what depends on this?" answerable directly, and lets an integrity check
find orphans. **Do this before derived pricing grows past triangulated FX** —
the current graph is two or three levels deep, so nothing is urgent today.

### T-56 No post-merge audit checklist `[ ]`

**Files:** would live in `scripts/` plus a section in
`docs/developer-workflow.md`.

The 2026-08 merge of two long-diverged branches shipped several defects that
share one shape — nobody structurally reviewed the *resolved* conflicts, only
the code. Found afterwards: a duplicated reconciliation paragraph in
`docs/conventions.md`, duplicated `case` arms in `writePricingServiceError`,
a duplicate `codeql2.yml` workflow, and colliding backlog IDs T-42–T-47 (see
the note at the top of this file).

A script or checklist should mechanically flag, after any non-trivial merge:
migration-number collisions; `pnpm --dir frontend run openapi:generate` leaving
the tree dirty; adjacent duplicate paragraphs in `docs/*.md`; duplicate
workflow files analysing the same thing; backlog/todo IDs used twice; and tests
deleted or renamed by the merge. Cheap to write, and each item on that list has
already cost something once.

### T-60 Report test fixtures are rebuilt per suite `[ ]`

**Files:** `backend/internal/api/reports_test.go` (`newSpendingFixture`),
`backend/internal/api/cashflow_test.go` (`newCashflowFixture`),
`e2e/playwright/reports.spec.ts`.

Three test suites build the same reporting shape three times — accounts under a
parent, an expense and income category, a refund, a transfer, a voided posting,
a second commodity — each in its own dialect. They already drifted: the backend
fixtures carry a EUR expense the browser suite had to grow separately when the
multi-currency E2E case landed (2026-08-23).

The shape worth sharing is the *scenario*, not the code: a declared set of
transactions with the exact per-commodity figures every report must produce from
them, consumed by the Go suites through a seeding helper and by Playwright
through the same JSON. Then a classification change that breaks one report's
answer breaks it in both places at once, instead of in whichever suite happened
to encode that case. Not urgent — the coverage exists today — but it is the
cheapest way to stop the two layers asserting different numbers about the same
ledger.

### T-63 A posting rejected for a version gap says the account is invalid `[ ]`

**Files:** `backend/internal/app/transactions_validate.go:335-356`.

`PostingAccountRule` is an as-of lookup. When an account's earliest version is
effective *after* the posting's entry date, it misses — and the fallback branch
below it only produces a specific message when `entry_date < opened_on`. An
account whose `opened_on` is earlier than its first version's `effective_from`
satisfies neither, so the write is refused with "posting account is invalid".

Reproduced 2026-08-23: create an account with `opened_on` 2020-01-01 and
`effective_from` 2026-01-01, then post on 2020-06-01 →
`400 VALIDATION_FAILED "posting account is invalid"`. The account exists, is
active, and the date is after it opened; the real reason is that no *version* of
it is effective at that date. The message sends the reader to check the wrong
thing.

Fix is to name the actual reason (no account version is effective on that date,
and the earliest one starts on <date>), reached from the same miss the code
already detects.

**Do not "fix" it by letting the write through.** This rejection is what keeps
the ledger export's account-version fallback defensive rather than load-bearing
(ADR 0011, `db/exports.go`): if a posting can exist before its account's first
version, the export starts relying on a fallback instead of a guarantee, and
`docs/plans/data-portability-plan.md` slice 6 gains a real failure to report
rather than a counter that should always read zero.

### T-72 Two-process concurrency is reasoned about, never exercised `[ ]`

**Files:** `backend/internal/db/background_work.go` (`ClaimBackgroundWork`),
`backend/internal/db/backups.go` (`CreateBackupRunWithWork`),
`backend/internal/lockfile/`.

Three defences exist for two processes sharing one database file: the claim's
re-check of the status its SELECT chose on, the unique occurrence key, and the
restore lock. None has a test that runs two *processes*, and one of them
provably cannot get one in-process — the main pool is `SetMaxOpenConns(1)`
(ADR 0004), so each claim transaction holds the single connection for its whole
length and two SELECTs never interleave. `TestClaimHandsOneItemToOneWorker`
says so in its own comment after the re-check was removed and it kept passing.

What is needed is a test that spawns two `rekenraam` processes against one
database file and asserts: one scheduled occurrence, no work item claimed
twice, and a restore refused while the other holds the lock. `cmd/rekenraam`
already has command-level tests to build on.

Worth doing before anyone runs two instances deliberately — a second container
against a shared volume is exactly the deployment this would break, and today
nothing would catch it.

### T-73 No export or self-check is measured against a large book `[ ]`

**Files:** `backend/internal/app/exports_bundle.go` (`computeTrialBalance`),
`backend/internal/app/exports.go` (account paths), `backend/internal/app/self_check.go`.

`ledger.csv` streams, which is the part that matters. Around it, several
structures are accumulated whole: the trial balance is a map of one accumulator
set per account-and-commodity, account paths are built for every account, and
the self-check folds every posted posting in memory. Each is bounded by the
book's size rather than by anything chosen, and no test or benchmark states
what "large" costs — so the first person to find the limit will be a user
exporting their own ledger.

Wanted: a generated book of a realistic upper bound (say 250k postings across a
few thousand accounts), and a bounded assertion — peak allocation and wall time
for a bundle export and a self-check — that fails when either regresses sharply
rather than when either is merely slow. `testing.B` with `ReportAllocs` is
enough; this does not need to run on every push.

Related: T-70, which is about the *test suite's* runtime rather than the
app's.

## Investment integrity work (R12a correctness gate, then scheduled follow-up)

These three findings were opened by the 2026-08-29 ledger/subledger boundary
review. T-75a and T-74 are the narrow R12a gate ahead of the remaining R9
slices because they can already create or preserve incorrect financial state.
T-76 closed 2026-09-10 before the v0.1 schema/export contract freeze. It no
longer blocks R16 or R18.
T-75b is investment lifecycle feature work in R16, not a prerequisite for safely
continuing unrelated work while the generic fence remains. ADR 0012 governs the
correction; R18 owns the later multi-basis reporting engine.

### T-75 Generic transaction lifecycle can strand investment lots `[~]`

**Priority P0 for T-75b — current R16 correction gate.** T-75a closed on
2026-08-30: generic update, correction, promotion, void/unvoid, soft-delete,
restore and draft deletion now reject investment-linked transactions with
`INVESTMENT_WORKFLOW_REQUIRED`. Investment creation is posted-only, unsafe UI
actions are hidden, and self-check compares journal and lot positions. These
checks prevent the original corruption paths while the native command is built.

**Current evidence (2026-09-27):** R16 slices 4a–4e read immutable long-position
intents, simulate replay reversibly, store numbered effective allocation
revisions, select the latest revision for gains and self-check, and export the
revision history. Investment-native correction and backdated writes remain
fenced. See `docs/implemented.md` and
`docs/plans/investment-operation-refactor-plan.md` slice 4.

**T-75b done when:** a native correction/reversal posts audited reversal and
replacement journal rows (or reversal alone), updates lot projections and
effective allocations, handles dependent disposals, price provenance, imports,
and reconciliation impact in one transaction, and proves rollback across all
four basis methods. Preserve original source facts and the generic fence.

## Documentation/code audit follow-ups (2026-08-31)

### T-80 Shipped non-English catalogs lack 65 current message keys each `[ ]`

**Files:** `frontend/messages/app/{en,es,fr,nl,de,ru}.json`,
`frontend/messages/settings/{en,es,fr,nl,de,ru}.json`.

Recounted 2026-09-27 against the English catalogs: 623 app keys and
1,019 settings keys (1,642 total, excluding `$schema`). Each non-English locale
still lacks the same 36 app keys and 29 settings keys. The missing 36 app
keys are `import_csv_*`; the missing 29 settings keys cover MFA/security plus
`investments_form_negative_number`. Each also retains the obsolete
`categories_field_opened_on` settings key. Shared-key placeholder sets match.

English fallback keeps the screens usable, but the docs' former claim that
all current messages were translated was false. Translation completion and
native terminology review are separate tasks; neither is complete.

Acceptance: all five locales match the current English key sets, preserve
placeholder sets, and render the CSV-mapping and MFA/security workflows in the
selected language. Add a catalog-parity validation gate so future English-only
additions cannot silently reintroduce the gap. Do not count English fallback
as a completed translation.

## R9 acceptance follow-up

### T-87 Browser-derived “today” can disagree with the owner-local financial date `[ ]`

**Files:** `frontend/src/lib/transactions/transaction-editor.svelte`,
`frontend/src/lib/investments/buy-form.svelte`, `sell-form.svelte`,
`dividend-form.svelte`, `frontend/src/lib/reports/reports-screen.svelte`, and
`frontend/src/routes/app/settings/currencies/+page.svelte`. These screens seed
transaction dates, report ranges or pricing-assignment effective dates from the
browser clock; four use UTC `toISOString()`. Around midnight, or when the browser
and configured owner time zones differ, a default can be one calendar day away
from the owner-local date used by recurring and forecast services.

**Required fix:** expose/reuse one server-owned owner-local current date through
an existing composed page/session contract, then remove the private browser-date
helpers from every affected financial screen. Preserve explicit user-selected
dates. Add pure contract tests plus a browser case with a browser zone on the
opposite side of midnight from the configured owner zone, proving transaction,
investment, report and pricing defaults all use the same owner-local date.
