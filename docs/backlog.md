# Technical Backlog

This is the **live** technical-debt list: items that are actionable now and are
not already scheduled as product work. It is intentionally short.

- **Feature sequence:** `docs/roadmap.md`.
- **Short-horizon queue:** `docs/todo.md`.
- **Shipped capability:** `docs/implemented.md`.
- **Resolved and deliberate decisions:** `docs/reviews/resolved-backlog-2026-07.md`.

Status legend: `[ ]` open · `[~]` partly done, with the remainder stated ·
`[blocked]` open with a stated dependency that must land first.

## General

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
- **R16's lot-mutation design note**, for the structural half only. Splits,
  mergers, spin-offs, ticker changes, and delistings mutate historical lots;
  `AcceptSuggestion` rejects them today by design. Dividend suggestions need
  no such note and would work the day a producer exists.

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
correctly (fixed the same day — see below), but have no data to act on until
a producer exists. `docs/product-requirements.md` lists "provider events and
reviewable suggestions" as a real requirement. Needs: a chosen data source
(no candidate picked yet), a fetch/detection design, and — separately — a
lot-mutation design for structural corporate actions (split, merger,
spin_off, ticker_change, delisting, `corporate_action`), which
`AcceptSuggestion` currently rejects outright since only the
`dividend_income` proposed-transaction kind (dividend, distribution,
cash_in_lieu, return_of_capital) is implemented.

### G-02 Frontend money logic is effectively untested `[~]`

**Partly done — the transaction editor half landed 2026-08-08.** From
`docs/reviews/test-coverage-review-2026-07.md` G-02. The gap was never
"components lack tests"; it is that **money logic living inside `.svelte`
files cannot be tested at all**, so the work is extraction, not test volume.

The backend equivalent of this gap is closed — merged Go coverage is 75.2%
with a CI floor (G-07, closed 2026-08-07) — which is what made the frontend
the remaining blind spot.

**Done (2026-08-08).** `frontend/src/lib/money/amount.ts` is now the single
money-parsing and money-formatting layer: `parseDecimalAmount`,
`formatLedgerAmount`, `negateCoefficient`, `inflowPositiveAmount`, and the
scale-aware `commodityImbalance`. It replaced two divergent copies of the
same logic — the private helpers inside `transactions/transaction-editor.svelte`
and `reconcile/statement-balance.ts`, which is now a thin alias over it. This
is the frontend counterpart of the backend's `exact.ScaledInt` consolidation
(T-41): one place where scale alignment happens, so a fix lands once.
`money/amount.test.ts` table-tests it on all four cases that have burned this
repo — cross-commodity scale mismatch, values past
`Number.MAX_SAFE_INTEGER`, decimal-comma input, negative/zero amounts —
plus the backend's 38-digit and 24-scale ceilings. Two real defects fell out
of the extraction: T-45 and T-46 below.

**Remaining.** The investment forms still hold untested money math and, in
`dividend-form.svelte`, a **third and fourth** copy of the same helpers
(`rawLedgerToDisplay` at line 155, and `parseDecimalField`, which returns a
`bigint` rather than a coefficient string):

- `lib/investments/dividend-form.svelte` — retire both helpers onto
  `$lib/money/amount`; the `bigint` return needs its call sites checked.
- `lib/investments/buy-form.svelte`, `sell-form.svelte` — a fifth and sixth
  copy of `parseDecimalField`, each with its own `toSafeInt`. Note these
  parsers reject a leading `-` outright (the `/^\d+$/` check runs on the whole
  coefficient), which is correct for quantity and cash but means the shared
  parser cannot be dropped in unchanged — the forms need an explicit
  non-negative check where the sign rejection used to be implicit.

Surveyed 2026-08-08 while scoping the above, and **not** a defect: the
investment endpoints take `cash_amount_value` as an OpenAPI
`integer/int64` — a JSON number — while `quantity_value` and every other
money field on the wire is a coefficient string. That is why `toSafeInt`
exists: a JSON number is only exact to 2^53−1, about a thousand times tighter
than the int64 the backend accepts. Both forms handle it correctly and
visibly (`investments_form_amount_too_large`), and at realistic cash scales
the cap is ~90 trillion currency units, so there is nothing to fix urgently.
It is recorded because it is a real inconsistency in the API contract, and
the cap does bite at scale 10+; worth folding into R16 rather than a
standalone change.

E2E growth is tracked separately (T-20, R3a) and is not a substitute:
Playwright is still an intentionally unscheduled CI decision.

### G-08 Amount input is not locale-aware `[ ]`

Found while doing G-02's first half, 2026-08-08. `frontend/src/lib/money/amount.ts`
reads "," as a thousands separator, which is right for the only locale the app
ships (`en`, per `frontend/project.inlang/settings.json`) and wrong the moment a
decimal-comma locale is added: a user typing `1,50` means 1.5, not 150.

The immediate hazard is closed (see T-45) — malformed grouping is now rejected
rather than silently mangled, so the user gets a rejection instead of a 100×
error. What is missing is the real fix: resolve the separator from the active
locale, in both directions (parsing input and rendering an editable value back
into a form field), the way `app/import_locale.go` does for file imports.

Deliberately not done now: with one locale there is nothing to resolve
against, and guessing per-value is how T-36 happened. This blocks nothing
today but is a prerequisite for the multi-currency/expat direction in
`docs/roadmap.md`, and should be picked up with the first non-`en` locale.

Noted while confirming the above: **`dinero.js` 2.0.2 is a declared dependency
with zero imports anywhere in `frontend/src`.** `docs/conventions.md` claimed it
was used "for all frontend money arithmetic … input parsing", which was never
true — parsing has always been hand-rolled, correctly, because Dinero's default
calculator is JS numbers and breaks past `Number.MAX_SAFE_INTEGER`. The
convention was corrected 2026-08-08 to match reality: exact arithmetic in
`$lib/money/amount.ts`, Dinero reserved for locale-aware display. Whoever picks
up G-08 should decide whether Dinero (with its BigInt calculator) is actually
the display layer we want, or whether `Intl.NumberFormat` alone is enough and
the dependency should be dropped.

## Public-deployment security gates

**All closed as of 2026-08-07.** S-04 (lockout-safe throttle) and S-07
(authentication-event visibility) closed 2026-08-06; S-06 (multi-factor
authentication) closed 2026-08-07 — see
`docs/reviews/resolved-backlog-2026-07.md`.

What remains is an operator step rather than a backlog item: exposing real
financial data to the internet requires the owner account to actually be
**enrolled** in two-factor authentication, and `REKENRAAM_SECRET_KEY` to be
configured. `docs/deployment-security.md` is the checklist.

