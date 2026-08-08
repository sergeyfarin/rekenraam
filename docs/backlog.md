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

### G-02 Frontend money logic is effectively untested `[x]`

**Done 2026-08-08** (editor half, then the investment forms). From
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

**Done (2026-08-08), and it turned up a live 100x bug — see T-47.** The three
investment forms held **seven** copies of three helpers, one more than the
survey above counted: `dividend-form.svelte` also carried its own `toSafeInt`
beside `rawLedgerToDisplay` and `parseDecimalField`.

The non-negative note above was exactly right and was the shape of the work.
The old parsers rejected a leading `-` only as a side effect — they
concatenated the integer and fractional digits and ran `/^\d+$/` on the
result, so `-5.00` built `"-500"` and failed the digit check.
`parseDecimalAmount` handles signs correctly, so the swap needed an explicit,
named non-negative guard per form or negative quantities would have started
being accepted.

Because there is **no component-test harness** in this project (no
`testing-library`, no `jsdom`/`happy-dom` — vitest runs plain `.ts` only), a
per-form behaviour change could not be tested in place. The validation was
extracted to `lib/investments/form-amounts.ts` (`parseMagnitude`,
`parseMoneyMagnitude`, `parseTradeAmounts`, `parseDividendAmounts`) with 35
named tests, and the components now just call it.

`toSafeInt` deliberately did **not** move onto `$lib/money`. It is not money
arithmetic that drifted; it is the adapter for the `integer/int64` contract
quirk described below. It now lives once as `toInt64Coefficient` in
`lib/api/investments.ts`, next to the contract it adapts — putting a
`BigInt`→`Number` converter into `amount.ts` would contradict that module's
invariant that no `Number` ever touches a coefficient.

Also fixed in passing: `dividend-form` and `sell-form` returned silently when
an amount failed to parse, so a typo left the submit button doing nothing with
no explanation. Both now surface an error, as `buy-form` already did.
`sell-form`'s debounced preview keeps its silence for half-typed values but
reports `too_large` immediately, since that will not become valid by typing
more.

G-02's stated scope — the editor, the reconcile form, and the investment
forms — is now closed. A sweep done afterwards found three further `.svelte`
files with private money math that were never part of that scope; they are
tracked separately as G-09 rather than reopening this item.

E2E growth is tracked separately (T-20, R3a) and is not a substitute:
Playwright is still an intentionally unscheduled CI decision.

### G-09 Three more `.svelte` files carry private money math `[ ]`

Found 2026-08-08 by sweeping every `.svelte` file for inline coefficient
arithmetic, once G-02's investment forms were done. **None of these is a
defect** — each aligns scales correctly today. They are recorded because they
are private copies that can drift, which is the entire reason
`docs/conventions.md` requires one module: every copy that has drifted in this
repo so far shipped a bug (T-45, T-46, T-47).

- `lib/transactions/category-transactions.svelte` — a private `rescaleUp`
  plus scale-aware per-commodity summing (lines ~48–99). `commodityImbalance`
  in `$lib/money/amount.ts` already encapsulates exactly this.
- `routes/app/settings/currencies/+page.svelte` — inline `10n ** BigInt(...)`
  price scaling (lines ~373–419).
- `lib/investments/gains-report.svelte` — `BigInt(Math.trunc(Number(value)))`
  (line ~55). The one remaining place a `Number` briefly touches a
  coefficient, so it is the only one of the three with a real precision
  ceiling rather than only a drift risk. Take this one first.

Lower priority than G-02 was: that item was chasing a live 100× error, this
one is preventing a future one.

### G-08 Amount input is not locale-aware `[~]`

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

**The display-layer half of this item is now resolved; only input parsing is
still open.** Noted while confirming the above: `dinero.js` 2.0.2 was a declared
dependency with zero imports anywhere in `frontend/src`. `docs/conventions.md`
claimed it was used "for all frontend money arithmetic … input parsing", which
was never true — parsing has always been hand-rolled, correctly, because
Dinero's default calculator is JS numbers and breaks past
`Number.MAX_SAFE_INTEGER`.

Decided 2026-08-08, at the start of R2's remaining reports work, because
cashflow and CSV output both needed a named display formatter and deferring
again would have meant a third round of inline formatting:

- **`Intl.NumberFormat` alone is enough; `dinero.js` was removed** from
  `frontend/package.json`. Dinero formats through `Intl` regardless, so it adds
  no capability; reaching it safely would mean wiring its BigInt calculator to
  get arithmetic we already do and test directly; and it models money as an
  amount in a currency with one fixed exponent, which cannot represent a book
  holding a 24-scale crypto quantity beside a 2-scale euro.
- The formatter that already existed — `formatQuantity`, previously buried in
  `$lib/transactions/transaction-labels.ts` and imported by seven unrelated
  modules including reports — moved to **`frontend/src/lib/money/format.ts`**,
  the read-only display half of `$lib/money`. `format.test.ts` pins how it
  differs from `amount.ts`'s editable `formatLedgerAmount` so neither half
  drifts into the other's job.

What remains under G-08 is only the input direction: resolving the separator
from the active locale when parsing, and when rendering an editable value back
into a form field. That is still gated on the first non-`en` locale.

## Public-deployment security gates

**All closed as of 2026-08-07.** S-04 (lockout-safe throttle) and S-07
(authentication-event visibility) closed 2026-08-06; S-06 (multi-factor
authentication) closed 2026-08-07 — see
`docs/reviews/resolved-backlog-2026-07.md`.

What remains is an operator step rather than a backlog item: exposing real
financial data to the internet requires the owner account to actually be
**enrolled** in two-factor authentication, and `REKENRAAM_SECRET_KEY` to be
configured. `docs/deployment-security.md` is the checklist.

