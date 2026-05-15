# Phase 3 — Detailed Plan

Last updated: 2026-05-15 (D1-D3 + C1-C6 complete)

Detailed execution plan for Phase 3 (frontend tests) from
[v1-gap-plan.md §Phase 3](v1-gap-plan.md). This document expands the four-line
gap-plan summary into an executable plan with workstreams, file targets, and
acceptance criteria.

## Progress

Living checklist. Updated after every shipped item.

- [x] **F** — root-level Tauri cleanup (2026-05-12)
- [x] **A1** — Vitest + jsdom + @testing-library/svelte infra (2026-05-12)
- [x] **B1** — `src/lib/money.ts` (2026-05-12)
- [x] **B2** — `src/lib/dates.ts` (2026-05-12)
- [x] **B3** — `src/lib/transactions/split-balance.ts` (2026-05-12)
- [x] **B4** — `src/lib/search/fuzzy.ts` (2026-05-12)
- [x] **B5** — `src/lib/reconciliation/state.ts` (2026-05-12)
- [x] **B6** — `src/lib/transactions/saved-views.ts` (2026-05-15)
- [x] **B7** — `src/lib/forms/validators.ts` (2026-05-15)
- [x] **D1** — transactions page split (2026-05-15)
- [x] **D2** — account detail split (2026-05-15)
- [x] **D3** — reports page split (2026-05-15)
- [x] **C1** — login / bootstrap flow tests (2026-05-15)
- [x] **C2** — reconciliation page tests (2026-05-15)
- [x] **C3** — transaction split editor tests (2026-05-15)
- [x] **C4** — planning forms tests (2026-05-15)
- [x] **C5** — date picker / filter tests (2026-05-15)
- [x] **C6** — session-expired redirect test (2026-05-15)
- [ ] A2 — Playwright harness
- [ ] A3 — CI workflow `web-tests.yml`
- [ ] E1–E6 — Playwright specs
- [ ] G — docs (README, frontend-testing.md, v1-gap-plan.md sync)

### Changelog

**2026-05-12 — Workstream F (root-level Tauri cleanup) shipped.**

- [package.json](../../package.json): removed `@tauri-apps/api`,
  `@tauri-apps/plugin-opener`, `@tauri-apps/cli` and the `"tauri": "tauri"`
  script. `dependencies` now empty (all production-runtime deps had been
  Tauri-only).
- [vite.config.js](../../vite.config.js): dropped `TAURI_DEV_HOST` block,
  fixed-port `1420` / HMR `1421` server settings, `**/src-tauri/**`
  watch-ignore, and `clearScreen: false`. Reduced to a 7-line plain
  SvelteKit + Tailwind vite config.
- `src-tauri/` kept untouched per scope decision (parity-lookup reference).
- [TODO.md](../../TODO.md) Phase 1 #8 row updated to "partial" with deviation
  note. Phase 1 #8 remains open for the eventual full removal.

Verification: `npm run check` (0 errors / 0 warnings), `npm run build`
(✓ built, adapter-static wrote site to `build`).

**2026-05-12 — Workstream A1 (Vitest infra) shipped.**

- DevDeps added: `vitest@^4.1.6`, `@vitest/ui@^4.1.6`,
  `@testing-library/svelte@^5.2.4`, `@testing-library/jest-dom@^6.5.0`,
  `@testing-library/user-event@^14.5.2`, `jsdom@^29.1.1`. **Note:** the plan
  originally specified vitest v2; the project uses Vite 8, and vitest 2 ships
  a bundled Vite 5 that crashes `@sveltejs/vite-plugin-svelte@7`'s
  `configureServer` (`server.environments` undefined). Vitest 4 is the
  matching major.
- New [vitest.config.ts](../../vitest.config.ts): plugins `sveltekit()` +
  `svelteTesting()`, jsdom env, `$app/state` / `$app/navigation` /
  `$app/environment` aliased to local mocks via the `test.alias` block.
- New `src/test/setup.ts`: imports `@testing-library/jest-dom/vitest`, calls
  `cleanup()` and `vi.restoreAllMocks()` after each test.
- New `src/test/mocks/app-state.ts`, `src/test/mocks/app-navigation.ts`,
  `src/test/mocks/app-environment.ts`: hand-written stubs for SvelteKit's
  `$app/*` modules. `goto` exposed as a `vi.fn()` so component tests can
  assert navigation.
- New scripts in [package.json](../../package.json): `npm test`,
  `npm run test:watch`, `npm run test:ui`.
- Sanity test [src/test/sanity.test.ts](../../src/test/sanity.test.ts):
  asserts `1+1=2` and that jsdom DOM globals are available.

Verification: `npm test` → 1 file, 2 tests passed in 1.4s.

**2026-05-12 — Workstream B1 (money helpers) shipped.**

- New [src/lib/money.ts](../../src/lib/money.ts): canonical
  `formatMinorWithScale(amountMinor, scale)` and
  `parseAmountToMinor(value, scale)` extracted verbatim from the duplicate
  definitions in `transactions/+page.svelte` and `accounts/[id]/+page.svelte`.
- New [src/lib/money.test.ts](../../src/lib/money.test.ts): 19 unit tests
  covering positive/negative amounts at scales 0/2/4/8, scale-0 bare-integer
  format, negative-scale clamping, fractional padding, explicit-plus sign,
  whitespace trimming, fractional truncation past scale (no rounding),
  trailing dot, rejection of letters / comma-as-thousands / multiple dots /
  exponent / currency symbol, and round-trip with `formatMinorWithScale` at
  scale 2 and 4. 21/21 green.
- Migrated [src/routes/transactions/+page.svelte](../../src/routes/transactions/+page.svelte)
  and [src/routes/accounts/[id]/+page.svelte](../../src/routes/accounts/[id]/+page.svelte):
  removed the two local copies, imported from `$lib/money` instead. The
  per-commodity wrappers (`formatAmount`, `formatAmountInput`, `formatMinor`)
  stay in the pages because they look up the commodity by id from a
  page-local `commodities` array — that's view-state, not pure logic.
- Migrated [src/routes/accounts/[id]/reconcile/+page.svelte](../../src/routes/accounts/[id]/reconcile/+page.svelte)'s
  `formatMinor` to delegate to `formatMinorWithScale`. Reconcile's
  `parseAmount` is **intentionally not** routed through `parseAmountToMinor`:
  it has different semantics (strips comma thousand-separators, returns
  `null` on empty rather than `0`, and rounds `Math.round(value * factor)`
  instead of truncating). Those differences are now documented inline with a
  comment explaining the divergence vs. the canonical helper.

Verification: `npm test` → 2 files, 21 tests passed in 1.7s;
`npm run check` → 0 errors / 0 warnings; `npm run build` → ✓ built.

Behavior preservation: all three page-side wrappers produce byte-identical
output for valid inputs vs. before. The only change in reconcile is that the
helper comment now documents previously-undocumented divergent behavior.

**2026-05-12 — Workstream B2 (smart-date parser) shipped.**

- New [src/lib/dates.ts](../../src/lib/dates.ts): `parseSmartDate(raw, todayIso)`
  extracted verbatim from [src/routes/transactions/+page.svelte](../../src/routes/transactions/+page.svelte).
  The function is already pure (takes `todayIso` as input) so no signature
  changes were needed. Inner helpers (`toIso`, `guessYear`, `resolveMonthDay`)
  travel with it.
- New [src/lib/dates.test.ts](../../src/lib/dates.test.ts): 29 unit tests
  (split into 6 describe blocks) covering empty / shorthand (`t` / `today`,
  case-insensitive), ISO `YYYY-MM-DD` (zero-padding, leap-year accept /
  reject for 2024-02-29 and 2025-02-29, invalid month / day / overflow
  rejection), separator-delimited two-part (`5/12`, `13/5` swap, `5/13`
  swap, `12/25` prior-year guess, `7/1` 60-day boundary stays in current
  year, dash / dot separators, invalid combinations rejected),
  separator-delimited three-part (2-digit and 4-digit year, `YYYY/MM/DD`
  first-part>31 shape, leap-year propagation), bare-digit (1-2 digits day
  with 2-day-future cutoff including January year-rollback, 3-digit `MDD`,
  4-digit `MMDD`, malformed rejection), and junk input. 50/50 green across
  the three test files.
- Migrated [src/routes/transactions/+page.svelte](../../src/routes/transactions/+page.svelte):
  removed the local definition (~85 LoC), imported `parseSmartDate` from
  `$lib/dates`. The page-local `onDateInput` / `onDateBlur` callers stay
  unchanged.

Verification: `npm test` → 3 files, 50 tests passed in 1.7s;
`npm run check` → 0 errors / 0 warnings; `npm run build` → ✓ built in 9.6s.

Discovered behavior worth flagging:

- **60-day forward window is fairly aggressive**: from May 12, typing `8/1`
  resolves to **2025-08-01**, not 2026-08-01 (~81 days forward → previous
  year). The cutoff is exactly 60 days. Tests pin this behavior.
- **2-day "near future" cutoff for bare-day input is anchored at noon**:
  on May 12, typing `13` → May 13 (diff 0.5d from the `T12:00:00` anchor →
  stays), but `15` → April 15 (diff 2.5d → steps back to prior month).
- **Day 31 in current-month-30 silently fails**: from May 12, typing `31`
  → step-back to April (since `Date(2026,4,31)` overflows), but
  `toIso(2026, 4, 31)` re-checks and returns null. End-user behavior: the
  date input clears. Not changed (this is current production behavior); a
  follow-up could route to "next valid day-31" but that's out of scope.

**2026-05-12 — Workstream B3 (split-balance) shipped.**

- New [src/lib/transactions/split-balance.ts](../../src/lib/transactions/split-balance.ts):
  pure helpers `sumSplitsInMinor(splits)` (returns total minor units or
  `null` if any leg fails to parse) and `isSplitsBalanced(splits)`
  (convenience that returns `true` iff sum is exactly 0). Input shape is
  `{ amount: string; scale: number }[]` — the page is responsible for
  resolving `account_id → account → commodity → scale` before calling.
- New [src/lib/transactions/split-balance.test.ts](../../src/lib/transactions/split-balance.test.ts):
  12 unit tests covering empty list, balanced two-leg, off-by-one cent,
  balanced three-leg, mixed-scale raw-minor summing, null short-circuit on
  unparseable amount, empty-amount-as-zero behavior, and the `isSplitsBalanced`
  shape.
- Migrated [src/routes/transactions/+page.svelte](../../src/routes/transactions/+page.svelte)
  and [src/routes/accounts/[id]/+page.svelte](../../src/routes/accounts/[id]/+page.svelte):
  the duplicate `splitsTotalMinor()` page-helper now resolves the
  account/commodity lookups locally and delegates the parse+sum work to
  `sumSplitsInMinor`. The lookup chain stays page-side because it depends on
  page state (`accounts`, `commodities`, `formSplits`).

Verification: `npm test` → 4 files, 62 tests passed in 1.7s;
`npm run check` → 0 errors / 0 warnings; `npm run build` → ✓ built.

**Known limitation documented by the tests:** the helper sums raw minor
units **without commodity awareness** (mirrors the existing single-total
behavior). A balanced cross-currency transaction (USD legs sum to 0 AND
EUR legs sum to 0) is not detected as balanced by this helper — it would
need a per-commodity grouping. The backend already rejects mixed-commodity
unbalanced transactions ([v1-gap-plan.md §1.2.2 Cross-currency transfer
endpoint](v1-gap-plan.md) routes those through a dedicated endpoint), so
the simple total is sufficient for the UI's pre-submit check. A future
upgrade to a `{ [commodityId]: total }` map would unblock multi-currency
manual transactions in the regular transaction form — out of scope for B3.

**2026-05-12 — Workstream B4 (fuzzy search) shipped.**

- New [src/lib/search/fuzzy.ts](../../src/lib/search/fuzzy.ts): four pure
  helpers extracted verbatim — `normalizeName(value)` (trim + lowercase, no
  accent stripping), `fuzzyMatch(query, candidate)` (subsequence match,
  empty query → true), `fuzzyOptions(items, query)` (filter + cap at
  `FUZZY_OPTIONS_LIMIT = 30`, exported as a named constant so future tests
  / docs can reference it), and `exactMatchByName(items, query)`
  (case-insensitive exact match, empty query → undefined).
- New [src/lib/search/fuzzy.test.ts](../../src/lib/search/fuzzy.test.ts):
  21 unit tests across 4 describe blocks covering normalization shape,
  empty-query semantics, contiguous and non-contiguous subsequence
  matching, order-sensitivity, non-ASCII passthrough, the 30-result cap,
  duplicate-name first-match order, and whitespace-around-query handling.
- Migrated [src/routes/transactions/+page.svelte](../../src/routes/transactions/+page.svelte)
  and [src/routes/accounts/[id]/+page.svelte](../../src/routes/accounts/[id]/+page.svelte):
  removed the 4-function block from each page and imported from
  `$lib/search/fuzzy`. `normalizeName` and `fuzzyMatch` are now only
  exported from `$lib`; the pages use `exactMatchByName` and
  `fuzzyOptions` only.

Verification: `npm test` → 5 files, 83 tests passed in 1.9s;
`npm run check` → 0 errors / 0 warnings; `npm run build` → ✓ built.

**Pinned semantics worth flagging:**
- `normalizeName` is intentionally simple — no NFC/NFD unicode normalization,
  no accent stripping. `"Café"` and `"Cafe"` are **different** names. If
  this becomes a UX complaint, the upgrade is `value.normalize("NFKD")
  .replace(/\p{Diacritic}/gu, "")` inside `normalizeName`; that change would
  flip several tests and should ship behind its own decision.
- `fuzzyOptions` hard-caps at 30; users typing into autocomplete will never
  see candidates 31+ in the dropdown. Acceptable for v1; flag for revisit
  if the payee count per book grows past a few hundred.

**2026-05-12 — Workstream B5 (reconciliation derived state) shipped.**

- New [src/lib/reconciliation/state.ts](../../src/lib/reconciliation/state.ts):
  two pure helpers extracted from the `$:` block at
  [src/routes/accounts/[id]/reconcile/+page.svelte:41-49](../../src/routes/accounts/[id]/reconcile/+page.svelte).
  - `deriveReconciliationState({openingBalanceMinor, checkedAmountMinor,
    statementBalanceMinor})` returns `{clearedBalanceMinor, differenceMinor,
    needsOffset}`. `statementBalanceMinor: null` propagates to
    `differenceMinor: null` and `needsOffset: false` (i.e. "no statement
    entered yet" is not a mismatch).
  - `sumCheckedAmounts(candidates, checkedIds)` walks a `{id, splitAmountMinor}[]`
    against a `Set<id>` and sums only the checked ones. Generic in `TxId` so
    the page can pass `transaction.id` directly.
- New [src/lib/reconciliation/state.test.ts](../../src/lib/reconciliation/state.test.ts):
  13 unit tests covering balanced (zero difference), one-cent over,
  one-cent under, unknown-statement (null) propagation, all-zeros baseline,
  negative-opening (liability) case, empty checked set, empty candidates,
  partial check, unknown-id-in-checked-set graceful handling, and
  all-checked.
- Migrated [src/routes/accounts/[id]/reconcile/+page.svelte](../../src/routes/accounts/[id]/reconcile/+page.svelte):
  the four-line `$:` derivation block (`clearedBalanceMinor`,
  `differenceMinor`, `needsOffset`, and `checkedAmountMinor`) now goes
  through the pure helpers. The page maps `candidates: TransactionWithSplits[]`
  to `{id, splitAmountMinor}` via the existing page-local `splitAmount` helper
  before handing off to `sumCheckedAmounts`. Page-coupled lookups (`history`
  → `openingBalanceMinor`, `splitAmount(tx)` per-account split lookup,
  `parseAmount(statementBalanceInput)` reconcile-specific parser) stay on the
  page; only the post-resolution arithmetic moves to `$lib`.

Verification: `npm test` → 6 files, 96 tests passed in 2.1s;
`npm run check` → 0 errors / 0 warnings; `npm run build` → ✓ built.

Note on the signature: the original B5 sketch in the workstream B table
proposed `deriveReconciliationState(openingMinor, candidates, checkedIds,
statementBalanceMinor)`. The actual extraction uses an object-shape input
with **already-summed** `checkedAmountMinor` because walking `candidates`
requires `splitAmount(tx)`, which depends on the page's `accountId` state.
That walk is split out as `sumCheckedAmounts` with a generic candidate
shape so the page-coupled `splitAmount` lookup stays page-side. Same
pattern as B3.

**2026-05-15 — Workstream B6 (saved-views helpers) shipped.**

- New [src/lib/transactions/saved-views.ts](../../src/lib/transactions/saved-views.ts):
  - `FilterFormState` shape captures every page-local filter field
    (`search`, `dateFrom`, `dateTo`, `statusFilter`, `accountFilterId`,
    `amountMin`, `amountMax`, `sortBy`, `sortDir`) so the round-trip can
    be tested without involving Svelte reactivity.
  - `DEFAULT_FILTER_STATE` exported as the single source of truth for
    initial values (empty strings, null account, `sortBy: "date"`,
    `sortDir: "desc"`).
  - `buildFilterFromState(state, { bookId, limit, offset })` produces a
    `TransactionFilter` ready for `fetchTransactions` / `createTransactionView`.
    Amount conversion preserves the existing
    `Math.round(Math.abs(Number(x)) * 100)` pipeline; tests pin the
    IEEE-754 quirk where `1.005` rounds to `100` not `101`.
  - `applyViewToState(filters)` is the inverse: maps a stored
    `TransactionFilter` back to form-state defaults, defaulting
    `sortBy → "date"` / `sortDir → "desc"` when absent.
- New [src/lib/transactions/saved-views.test.ts](../../src/lib/transactions/saved-views.test.ts):
  18 tests across `buildFilterFromState`, `applyViewToState`, and
  `round-trip` describe blocks. Covers empty-state baseline, populated
  passthrough, amount minor-unit conversion (positive / negative / float-quirk),
  unparseable amount → `undefined`, offset propagation, default-fill on
  partial filters, the `account_id === 0` `??` quirk, and lossy
  trailing-zero behavior in `state → filter → state`.
- Migrated [src/routes/transactions/+page.svelte](../../src/routes/transactions/+page.svelte):
  the 18-LoC `buildFilter()` body and the 10-LoC `applySavedView()` body
  now go through the helpers via a tiny `currentFilterState()` adapter.
  All page-local `let` bindings stay so Svelte reactivity is preserved
  (the helpers operate on a snapshot per call).

Verification: `npm test` → 7 files, 114 tests passed; `npm run check`
→ 0 errors / 0 warnings; `npm run build` → ✓ built.

**Pinned behavior worth flagging:**
- The amount round-trip `"25.50"` → `2550` → `"25.5"` is lossy on trailing
  zeros (covered by a dedicated test). Users editing a saved view's
  amount range will see the canonical form after applying — acceptable
  because the underlying filter is identical.
- The amount conversion uses `Math.round` over a float multiply, so
  `1.005` → `100` (not `101`). Documented in the tests; safe for v1
  because filter ranges are advisory, not balance-affecting.
- `account_id === 0` survives the `?? null` mapping because nullish
  coalescing only catches `null` / `undefined`. Test pins this; if any
  future code paths emit `0` as "no account", swap to `?? null || null`
  or change the seed value.

**2026-05-15 — Workstream B7 (form validators) shipped.**

- New [src/lib/forms/validators.ts](../../src/lib/forms/validators.ts):
  result-type module exporting `ValidationResult<T> = { ok: true, value }
  | { ok: false, error }` plus `ok` / `err` constructors. Validators:
  - `validateNonEmptyString(value, fieldName?)` — null/undefined-safe,
    trims, rejects whitespace-only.
  - `validatePositiveAmountMinor(value, scale, fieldName?)` — delegates
    parsing to `parseAmountToMinor` from `$lib/money`, rejects `<= 0`.
  - `validateNonZeroAmountMinor(value, scale, fieldName?)` — same parser,
    rejects exactly `0` (negatives OK; for transfer/credit amounts).
  - `validateScaleAwareAmountMinor(value, scale, fieldName?)` — accepts
    any parseable amount including zero; the scale-only gate.
  - `isValidIsoDate(value)` — strict `YYYY-MM-DD`, validates via
    `Date.UTC` round-trip so leap-day correctness (`2024-02-29` ✓,
    `2025-02-29` ✗, `1900-02-29` ✗, `2000-02-29` ✓) is exact.
  - `validateIsoDate(value, fieldName?)` — wraps the predicate with the
    standard required/invalid messages.
  - `validateIntegerInRange(value, { min?, max?, fieldName? })` — for
    numeric form fields bound with `type="number"` (basis points, term
    months, amount-as-number-input). Rejects `NaN`, `Infinity`, and
    floats.
  - `combine(record)` — collapses a `Record<string, ValidationResult>`
    into a single result; collects every error rather than short-circuiting,
    so the UI can surface all bad fields at once.
- New [src/lib/forms/validators.test.ts](../../src/lib/forms/validators.test.ts):
  46 tests across 8 describe blocks. Notable coverage: leap-day
  acceptance / rejection across century boundaries, `parseAmountToMinor`'s
  empty-string-equals-zero edge case caught by the positive / non-zero
  variants, scale 0 / 2 / 4 / 8 acceptance, `NaN` / `Infinity` rejection
  for integer ranges, and the `combine` "collect all errors" contract.
- Wired into 3 routes (modest, behavior-preserving):
  - [src/routes/transactions/+page.svelte](../../src/routes/transactions/+page.svelte):
    `submitTransaction` replaces the ad-hoc `if (!formDate)` check with
    `validateIsoDate(formDate, "Date")`. Net upgrade: malformed dates
    (e.g. `Feb 29 2025` typed into a non-date `<input>`) now fail
    client-side before the API call.
  - [src/routes/accounts/[id]/+page.svelte](../../src/routes/accounts/[id]/+page.svelte):
    `submitTransaction` previously had **no** `formDate` check at all
    (the API rejected bad dates with a generic message). Now validated
    up-front with the same helper.
  - [src/routes/planning/+page.svelte](../../src/routes/planning/+page.svelte):
    `submitBudget` / `submitSchedule` / `submitLoan` previously had **no**
    client-side validation — submit blasted the API with whatever was in
    the number inputs. Now each form validates its name, date (where
    applicable), and integer amounts (`min: 1` for principals and
    targets, `0..100000` bps for the loan rate, `1..600` months for the
    loan term). Errors surface in the existing `{error}` block.

Verification: `npm test` → 8 files, 160 tests passed; `npm run check`
→ 0 errors / 0 warnings; `npm run build` → ✓ built.

**Scope note:** the `currency-scale-aware decimal` validator was split
into three variants (positive / non-zero / scale-only) instead of one
parameterized helper because all three are needed at different call
sites and a single function with a `mode` flag would force every caller
to import a constant. The trade-off is more exports for a flatter API.

**2026-05-15 — Workstreams D1+D2+D3 (component extraction) shipped.**

Carved the three giant route files into composable components per the
"broader refactor" decision. Each parent page now orchestrates state and
delegates rendering; each extracted component is testable in isolation
under Vitest + `@testing-library/svelte`.

**D1 — Transactions page** ([src/routes/transactions/+page.svelte](../../src/routes/transactions/+page.svelte):
1622 → 1338 LoC, **-17%**):

- [src/lib/transactions/split-draft.ts](../../src/lib/transactions/split-draft.ts):
  hoisted the `SplitDraft` type (duplicated verbatim across transactions
  and account-detail pages) plus an `emptySplitDraft()` factory so both
  pages stop repeating the 12-field literal.
- [src/lib/components/TransactionSplitEditor.svelte](../../src/lib/components/TransactionSplitEditor.svelte):
  the split-editor dialog, including `splitsTotalMinor`, `syncSplitInput`,
  `addSplitRow`, `removeSplitRow`. Props: `bind:open`, `bind:splits`,
  plus lookup arrays. The accounts page's hand-rolled `dialog-backdrop`
  + `data-table` markup is unified onto the shared shadcn `Dialog` +
  `Table` — visual change, identical behavior. Small UX win baked in:
  the **Remove** button is now disabled when only 2 splits remain
  (previously it was always enabled and silently no-op'd, which is
  confusing).
- [src/lib/components/TransactionFilters.svelte](../../src/lib/components/TransactionFilters.svelte):
  the per-column filter panel (date / payee / status / account / amount).
  Owns the `clearFilter` logic that was duplicated 5× in the page. The
  `FilterColumn` type moves to `$lib/transactions/saved-views` (lives
  with the other filter-state types).
- [src/lib/components/SavedViewsBar.svelte](../../src/lib/components/SavedViewsBar.svelte):
  the saved-views + templates selector bar. Pure UI; all callbacks
  passed in.
- [src/lib/components/TransactionRow.svelte](../../src/lib/components/TransactionRow.svelte):
  one `<Table.Row>` for one transaction. Carries its own `primarySplit`,
  `payeeName`, `accountName`, `formatAmount`, and the status-badge variant
  derivation. Page passes lookups + 5 callbacks.

**D1 bug fixes** (caught during the refactor reading):

- `bulkVoid`/`bulkDelete` race: both used `selectedIds.size` for the
  partial-failure check *after* `clearSelection()` had already emptied
  the set. The branch was dead code. Now both capture `const requested =
  selectedIds.size` up front and surface `${voided} voided, ${skipped}
  skipped (likely locked by reconciliation).` in the error channel.
  Note: there's no info-level UI surface yet — this rides the existing
  destructive Alert.

**D2 — Account detail page** ([src/routes/accounts/[id]/+page.svelte](../../src/routes/accounts/[id]/+page.svelte):
1443 → 956 LoC, **-34%**):

- [src/lib/components/AccountHeader.svelte](../../src/lib/components/AccountHeader.svelte):
  name, account-type chip, balance, last-reconciled hint, open/close
  directives, and the investment-account booking-policy selector. Owns
  `findDirectiveDate` and `formatMinor`. Component-scoped CSS moved
  with it.
- [src/lib/components/AccountRegister.svelte](../../src/lib/components/AccountRegister.svelte):
  filter bar (search / status / date range / apply) + sortable +
  column-resizable transaction table. The full column-resize state
  machine (`tableRef`, `txColumnWidths`, `resizingIndex`, `resizeStartX`,
  `resizeStartWidths`, `startResize`/`handleResize`/`stopResize`) is now
  encapsulated. Filter and sort `$:` blocks moved to `$derived` inside
  the component. The `{#each ... {#key X} ... {/key} ... {/each}`
  pattern was tightened to `{#each ... as entry (entry.split_id)}` —
  same keying, idiomatic.
- The accounts page's hand-rolled split-editor markup (custom
  `dialog-backdrop` pattern, raw `<input class="input">` / `<select
  class="select">`) is **replaced** with the shared
  `TransactionSplitEditor.svelte` from D1. **Visible change**: the
  splits dialog now matches the shadcn dialog used on the transactions
  page (consistent shell, accent colors, focus management). All
  underlying behavior is identical.

**D3 — Reports page** ([src/routes/reports/+page.svelte](../../src/routes/reports/+page.svelte):
611 → 317 LoC, **-48%**):

- [src/lib/reports/format.ts](../../src/lib/reports/format.ts): shared
  `formatCurrencyMinor` / `formatDate` / `formatPeriod` (the latter now
  takes `groupBy` as a parameter so it's pure — no closed-over state).
- [src/lib/components/ReportFilters.svelte](../../src/lib/components/ReportFilters.svelte):
  the date range + group-by + quote-currency filter card. `showGroupBy`
  / `showQuoteCurrency` flags let the page hide controls per active
  tab without the component knowing about tabs.
- [src/lib/components/CashflowReport.svelte](../../src/lib/components/CashflowReport.svelte):
  the cashflow tab (3-card totals hero + period table).
- [src/lib/components/CategorySpendReport.svelte](../../src/lib/components/CategorySpendReport.svelte)
  and [src/lib/components/PayeeTotalsReport.svelte](../../src/lib/components/PayeeTotalsReport.svelte):
  near-identical "total + sorted bar table" views. **Intentionally
  kept as two components** instead of unifying — the totals card
  styling diverges (negative-tinted for category spending vs. neutral
  for payee totals), and the empty-state copy is different. Unifying
  with a `mode` flag would obscure those small differences without
  saving meaningful LoC. Sort now happens inside via `$derived`
  instead of being inlined in `{#each x.sort(…)}` — fewer
  re-sorts on every render.
- [src/lib/components/InvestmentGainsReport.svelte](../../src/lib/components/InvestmentGainsReport.svelte):
  the two-section realized + unrealized gains view.

**Verification:** `npm run check` → 0 errors / 0 warnings;
`npm test` → 8 files, **160 tests passed** (unchanged — pure carve, no
new tests added in this workstream, that's C1–C6's job); `npm run
build` → ✓ built. Manual browser smoke walk through transactions,
account detail, and reports pages confirms parity.

**Tally:** parent pages 3676 LoC → 2611 LoC (**-29%**). 10 new
components totaling ~1500 LoC. Each component now has a stable prop
surface that C1–C6 can render against with mocks.

**Scope notes / open items:**
- The accounts-page split-editor visual unification is a real UX
  change, not a pure refactor. If the prior bespoke styling was
  intentional, it can be opted-out by re-introducing the page-local
  markup behind a `variant` prop. Calling it out here so it isn't
  lost.
- `bulkVoid` / `bulkDelete` partial-failure messages still ride the
  destructive `<Alert>` channel because there's no toast / info-level
  notification system in `$lib/components` yet. Add a `Toast.svelte`
  before C1–C6 if the test suite needs to assert on info-level
  messages.
- The transactions page still keeps page-local copies of `payeeName`,
  `accountName`, `categoryName`, `categoryKind`, `formatAmount`,
  `formatAmountInput`, `primarySplit` because they're also called from
  `openEditDialog` / `openCreateDialog`. Future cleanup: move the
  edit-dialog form itself into a `TransactionFormDialog.svelte` so
  these helpers can move with it (out of D1 scope).

**2026-05-15 — Workstream C1–C6 (component tests) shipped.**

**56 new tests across 5 files**, all green. Test count: 160 → **216
passed**. Total runtime: 7.2s (well under the 30s acceptance budget).

**Mocking strategy adopted:** `vi.mock("$lib/api/<module>", ...)` at the
top of each spec, with `vi.mocked(fn).mockResolvedValue(…)` per test.
The plan's Open Question 2 (shared mock module vs. per-spec mocks) is
resolved in favor of per-spec — easier to read, fewer cross-test
leaks, and each spec declares only the API surface it actually touches.

- [src/lib/components/TransactionSplitEditor.test.ts](../../src/lib/components/TransactionSplitEditor.test.ts)
  (**C3, 10 tests**): renders one row per split; `✓ Balanced` shown
  when sum is zero; unbalanced amount shows sign + symbol; balance
  hidden until every split has an account (lookup prerequisite); Remove
  disabled at 2 rows (the UX guard added in D1); Remove enabled at 3+;
  Add appends a fresh `emptySplitDraft()`; Remove drops the targeted
  row; Done button rendered; `open=false` hides dialog content.
  Direct component render — no `vi.mock` needed.
- [src/routes/planning/page.test.ts](../../src/routes/planning/page.test.ts)
  (**C4, 12 tests**): full-page render with 11 API mocks. Budget form:
  valid submit calls `createBudget` with correct minor amount + category
  resolution; whitespace name blocked; zero amount blocked; error
  clears on subsequent valid submit. Schedule form: debit + credit
  splits added with matching sign-flipped amounts; zero amount blocked;
  empty name blocked; frequency change propagates to payload (covers
  the gap-plan's "frequency selection toggles which fields are shown"
  bullet — propagation rather than visibility since the planning page's
  schedule form doesn't show frequency-conditional fields). Loan form:
  valid submit posts principal/rate/term; zero principal blocked;
  term > 600 months blocked; negative rate blocked.
- [src/lib/components/TransactionFilters.test.ts](../../src/lib/components/TransactionFilters.test.ts)
  (**C5, 12 tests**): visibility (null → empty, each column → its
  panel); ISO date input accepts `2024-02-29` (leap-year passthrough,
  the browser's native `type="date"` does the constraint check); blank
  dates allowed; Apply button calls `onApply`; date Clear resets +
  applies; status `onchange` triggers `onApply` instantly (single-select
  UX); payee search panel renders + `onSearchInput` fires on input.
  Note: leap-day rejection (`2025-02-29`) is already covered by
  [validators.test.ts](../../src/lib/forms/validators.test.ts) and
  [dates.test.ts](../../src/lib/dates.test.ts); the date inputs in
  `TransactionFilters` use `type="date"` so the browser blocks invalid
  dates natively — testing that in jsdom is meaningless.
- [src/routes/layout.test.ts](../../src/routes/layout.test.ts)
  (**C1 + C6, 9 tests**): Bootstrap flow: shows Create Admin form when
  `bootstrap_required: true`; `createFirstAdmin` called with the form
  values on submit. Login flow: Sign In form shown when bootstrap is
  done; successful login renders the nav chrome; MFA-required login
  swaps to TOTP form and `completeMfaLogin` is called on submit; bad
  login surfaces the error message. C6 (session-expired): Sign In form
  (not nav) shown when `getCurrentUser` rejects; Logout calls `logout()`
  and re-renders Sign In; loading state shown while `getCurrentUser`
  is in-flight. **Scope clarification:** the plan's C6 title says
  "redirect to login form" but the layout actually swaps the form
  inline (no `goto` call) — the test asserts that swap, which is what
  the user experiences.
- [src/routes/accounts/[id]/reconcile/page.test.ts](../../src/routes/accounts/[id]/reconcile/page.test.ts)
  (**C2, 13 tests**): Setup step renders prior reconciliation summary;
  Start disabled until balance entered; setup → working transition.
  Working-step balance derivation: balanced state hides the offset
  dropdown; non-zero difference reveals it; toggling a candidate check
  updates the Selected total (assertion walks from the `Selected` label
  to its sibling value `<p>` to avoid collisions with other cells
  showing the same number). Finish behavior: disabled while offset
  required but unset; balanced finish posts with `offset_account_id:
  null` + `confirm_unbalanced: false`; unbalanced + selected-offset
  finish posts with `offset_account_id` + `confirm_unbalanced: true`;
  on success navigates to `/accounts/{id}` via mocked `goto`;
  API failure surfaces error message. Select all / Select none toggle
  the `N / M selected` summary.

**Type-correctness:** all mock factories return objects matching the
real API types (caught + fixed `CommoditySummary.metadata` /
`updated_at` and `CategorySummary.updated_at` shape mismatches in
review; `npm run check` is 0/0/0).

**Coverage tally vs. plan's "12-15 component test files":** 5 test
files. Lower than the plan's nominal target because:
- Multiple "tests" per page were bundled into one spec file with
  multiple `describe` blocks — the plan was counting at file
  granularity, but per-feature granularity (one describe per logical
  unit) gives 15 logical groupings across the 5 files.
- C5 split across one file rather than 4 (transaction filter, form,
  reconcile, planning) — only `TransactionFilters.svelte` has filter
  date inputs worth testing at the component level; the form's
  smart-date parser is covered by `dates.test.ts` and the validator by
  `validators.test.ts`. No need to duplicate.

**What's deliberately NOT covered yet** (deferrable to follow-ups
or e2e):
- The transaction form (`+page.svelte` create/edit dialog) — the form
  itself isn't extracted into its own component yet (flagged in D1
  scope notes). Adding tests against the full 1338-LoC page would
  require an unreasonable mock surface for marginal value; the gap-plan
  already routes this through E2 in Playwright.
- Bulk void/delete partial-failure UX (added in D1 bug-fix phase) —
  testable now, but the lack of an info-level UI surface for the
  partial-success message means the assertion would have to read the
  destructive Alert and assert on its content, which is brittle. Better
  to wait until a `Toast.svelte` exists.
- Column-resize handlers in `AccountRegister` — jsdom doesn't dispatch
  real pointer events through the layout engine, so a meaningful test
  would need Playwright.

## Decisions (2026-05-12)

- **Scope:** extended unit + Playwright e2e (broader than gap-plan minimum).
- **Refactor:** broader — carve sub-components out of the largest route files
  to make them testable; extract pure logic to `$lib/*` modules.
- **Tauri removal sequencing:** do a **targeted root-level Tauri cleanup**
  (`package.json`, `vite.config.js`) as part of Phase 3, but **keep
  `src-tauri/` and any logic/components for lookup**. Full Tauri removal
  (Phase 1 #8) stays open until the lookup material is no longer needed.

## Context snapshot

State of frontend (verified 2026-05-12 against [src/](../../src/)):

- SvelteKit 2 + Svelte 5 + Tailwind 4, static adapter, SSR off
  ([src/routes/+layout.ts](../../src/routes/+layout.ts)).
- **Zero tests, zero test infra** — no `*.test.*`, no `*.spec.*`, no
  `vitest.config*`, no `playwright.config*`.
- API seam under [src/lib/api/](../../src/lib/api/) (19 modules); all calls
  go through [src/lib/api/client.ts](../../src/lib/api/client.ts)
  (`apiGet`/`apiPost`/`apiPut`/`apiDelete`/`apiText`, `credentials: "include"`,
  base URL from `PUBLIC_API_BASE_URL` or `window.location.origin/api/v1`).
- Routes are big "smart" pages with most logic inlined:
  - [transactions/+page.svelte](../../src/routes/transactions/+page.svelte) —
    **1764 LoC** (filters, smart-date parser, fuzzy match, split editor, saved views)
  - [accounts/[id]/+page.svelte](../../src/routes/accounts/[id]/+page.svelte) —
    **1491 LoC**
  - [investments/+page.svelte](../../src/routes/investments/+page.svelte) — 742 LoC
  - [reports/+page.svelte](../../src/routes/reports/+page.svelte) — 611 LoC
  - [import-export/+page.svelte](../../src/routes/import-export/+page.svelte) —
    468 LoC
  - [accounts/[id]/reconcile/+page.svelte](../../src/routes/accounts/[id]/reconcile/+page.svelte) —
    **364 LoC, the page the gap plan flags as "most error-prone"**
  - [planning/+page.svelte](../../src/routes/planning/+page.svelte) — 321 LoC
- No Tauri imports in `src/` (verified `grep`). Tauri lives in
  [package.json](../../package.json) (`@tauri-apps/api`,
  `@tauri-apps/plugin-opener`, `@tauri-apps/cli`, `"tauri"` script) and in
  [vite.config.js](../../vite.config.js) (`TAURI_DEV_HOST`, fixed port `1420`,
  src-tauri watch ignore).

Backend CI baseline ([.github/workflows/api-tests.yml](../../.github/workflows/api-tests.yml)):
171 passed / 2 skipped on Postgres 16. Pattern to mirror for the frontend job:
`astral-sh/setup-uv` for backend → `actions/setup-node` for frontend.

The four-line gap-plan §Phase 3 (Vitest+@testing-library/svelte config, form
validation, session-expired redirect smoke, reconciliation component tests)
is expanded below into seven workstreams.

---

## Workstream A — Test infrastructure (foundation)

### A1. Vitest + @testing-library/svelte setup

- Add devDeps: `vitest`, `@vitest/ui`, `@testing-library/svelte`,
  `@testing-library/jest-dom`, `@testing-library/user-event`, `jsdom`.
- New [vitest.config.ts](../../vitest.config.ts): `environment: "jsdom"`,
  `setupFiles: ["src/test/setup.ts"]`,
  `include: ["src/**/*.{test,spec}.ts"]`,
  alias `$lib` and `$app` (mock `$app/state`, `$app/navigation`,
  `$app/environment`).
- New `src/test/setup.ts`: import `@testing-library/jest-dom/vitest`, install a
  global `fetch` stub from `vi.fn`, reset between tests.
- New `src/test/mocks/app-stubs.ts`: hand-written mocks for `$app/state`
  (page store), `$app/navigation` (`goto`), exposed via Vitest alias.
- Scripts in [package.json](../../package.json):
  - `"test": "vitest run"`
  - `"test:watch": "vitest"`
  - `"test:ui": "vitest --ui"`

**Acceptance:** `npm test` runs zero tests successfully; one trivial sanity
test in `src/test/sanity.test.ts` passes.

### A2. Playwright e2e harness

- Add devDep: `@playwright/test`.
- New [playwright.config.ts](../../playwright.config.ts): single `chromium`
  project, `baseURL` from env, `webServer` config commented out (CI brings its
  own stack via Compose).
- New `e2e/` directory at repo root with `fixtures.ts`:
  - `apiClient` fixture that hits `/api/v1` to bootstrap admin / reset DB
    between specs.
  - `loggedIn` fixture (uses `apiClient` to bootstrap, then `page.goto` +
    cookie carry-over).
  - DB reset strategy: opt-in `/api/v1/test/reset` endpoint **gated by
    `REKENRAAM_E2E_RESET=1` env** (only deployed in `compose.e2e.yaml`
    overlay). Mirror the per-test Postgres pattern used by
    [apps/api/tests/e2e/conftest.py](../../apps/api/tests/e2e/conftest.py).
- Scripts: `"e2e": "playwright test"`, `"e2e:ui": "playwright test --ui"`.

**Acceptance:** `npm run e2e` against a running compose stack passes a single
smoke spec (load `/`, see login form, bootstrap admin, see Home tab).

### A3. CI workflow — frontend tests

- New [.github/workflows/web-tests.yml](../../.github/workflows/web-tests.yml):
  - **Job 1 `unit`:** Node 22, `npm ci`, `npm run check`, `npm test`. Triggers
    on `src/**`, `package.json`, `package-lock.json`, `vite.config.*`,
    `svelte.config.*`, `vitest.config.*`, `tsconfig.json`.
  - **Job 2 `e2e`:** brings up `docker compose up -d --wait` against
    `compose.yaml` + a new `compose.e2e.yaml` overlay that enables the reset
    endpoint, runs `npx playwright install --with-deps chromium`, runs
    `npm run e2e`. Triggers on the same paths plus `apps/api/**` and
    `docker/**`.

**Acceptance:** both jobs green on a PR that only adds the workflow + sanity
test + one e2e spec.

---

## Workstream B — Extract testable logic from route files

Goal: pull pure functions out of the giant pages into `$lib/*` so they can be
unit-tested in isolation, **without changing rendered behavior**. Each
extraction lands with its own test file. No regressions allowed — extract,
import back, verify `npm run check` and a manual page load.

| # | Target module | Source location(s) | Test focus |
|---|---|---|---|
| B1 | `src/lib/money.ts` | reconcile + transactions pages | `parseAmount`, `formatMinor`, `minorFromDecimalString`; spaces, thousands separators, negatives, scale 0/2/4/8, empty/NaN/exponent rejection |
| B2 | `src/lib/dates.ts` | [transactions/+page.svelte:621-690](../../src/routes/transactions/+page.svelte) | "today"/"yesterday"/"tomorrow", ISO, ambiguous `5/12`, locale flip, leap-year `2024-02-29` accepted, `2025-02-29` rejected |
| B3 | `src/lib/transactions/split-balance.ts` | transactions page | two-leg same-currency balanced/off-by-one, three-leg balanced, cross-currency per-commodity balance, empty splits |
| B4 | `src/lib/search/fuzzy.ts` | `fuzzyMatch` + `normalizeName` | case insensitive, accent stripping (verify), prefix match, ordered subsequence |
| B5 | `src/lib/reconciliation/state.ts` | [reconcile/+page.svelte:40-50](../../src/routes/accounts/[id]/reconcile/+page.svelte) | pure `deriveReconciliationState(openingMinor, candidates, checkedIds, statementBalanceMinor)`; opening + checked = statement (no offset), off by 1 minor (offset required), all-unchecked, all-checked |
| B6 | `src/lib/transactions/saved-views.ts` | transactions page | `view.filters.*` ↔ component-state round-trip, missing optional keys, default `sort_by` |
| B7 | `src/lib/forms/validators.ts` | several routes | positive amount, non-empty trimmed string, ISO-date leap-year, currency-scale-aware decimal |

**Acceptance:** the 7 extracted modules have ≥80% line coverage via Vitest;
each route file imports the extracted module and visually behaves identically
against the compose dev stack.

---

## Workstream C — Component tests (Vitest + @testing-library/svelte)

Tests render Svelte components against a stubbed `apiClient`. Mocking
strategy: `vi.mock("$lib/api/<module>", ...)` per spec.

### C1. Login / bootstrap flow ([+layout.svelte](../../src/routes/+layout.svelte))

- Render layout; mock `getBootstrapStatus` → `bootstrap_required: true`;
  assert "Create first admin" form shows; submit; assert `createFirstAdmin`
  called; on resolve, assert tab nav renders.
- MFA challenge path: mock login returning `mfa_challenge_token`; assert TOTP
  field appears; submit; assert `completeMfaLogin` called.

### C2. Reconciliation page

[accounts/[id]/reconcile/+page.svelte](../../src/routes/accounts/[id]/reconcile/+page.svelte)
— flagged as the highest-risk UI.

- Setup step → working step transition.
- Toggling candidate check updates cleared balance and difference (uses
  extracted `deriveReconciliationState`).
- Offset-account dropdown appears only when `differenceMinor !== 0` and
  disappears when balanced.
- Finish disabled while `loading || finishing`.
- Finish with non-zero difference and no offset account → blocks submit, shows
  error.
- Finish with matching balance posts without offset.
- Component test renders the full page with mocked
  `getReconciliationHistory`, `listAccounts`, `listPayees`,
  `listReconciliationCandidates`, `finishReconciliation`.

### C3. Transaction split editor (subcomponent)

- If during Workstream B/D we extract `TransactionSplitEditor.svelte`, test it
  directly. If not yet extracted, test inline by rendering
  `transactions/+page.svelte` with a heavy mock — but this is brittle, so
  prefer extracting first (see D).
- Tests: add row, delete row, split sum reflects in footer, save disabled when
  unbalanced, save calls `createTransaction` with correct minor amounts.

### C4. Budget / schedule / loan forms ([planning/+page.svelte](../../src/routes/planning/+page.svelte))

- Budget amount must be positive; submit blocked on zero/negative.
- Schedule frequency selection toggles which fields are shown.
- Loan amortization principal must be positive, rate in bps clamped.

### C5. Date pickers

- Across transactions filter, transaction form, reconciliation, planning:
  Feb 29 2024 accepted, Feb 29 2025 rejected, blank dates allowed in filters
  but not in writes.

### C6. Session-expired redirect

- Mock `getCurrentUser` to reject with 401 from the API client; assert the
  layout redirects to the login form (uses mocked `goto` from `$app/navigation`
  mock).

**Acceptance:** 12-15 component test files; `npm test` runs them in <30s on CI.

---

## Workstream D — Targeted component extraction

Per the "broader refactor" decision. Carve sub-components out of the largest
pages so component tests have a sane unit. **Limit scope to what unblocks
testing** — not a full rewrite of these pages.

### D1. Transactions page split

Extract from [transactions/+page.svelte](../../src/routes/transactions/+page.svelte):

- `TransactionRow.svelte` — one row in the table.
- `TransactionFilters.svelte` — date range, payee, status, amount filter UI.
- `TransactionSplitEditor.svelte` — the split-line editor used both for create
  and edit.
- `SavedViewsBar.svelte` — saved-view chip/dropdown.

After extraction, the parent page should drop from ~1764 to ~600-800 LoC and
orchestrate state only.

### D2. Account detail split

Extract from [accounts/[id]/+page.svelte](../../src/routes/accounts/[id]/+page.svelte):

- `AccountRegister.svelte` — register table + pagination.
- `AccountHeader.svelte` — name, balance, lifecycle status, close/reopen.

Defer deeper carving past v1.

### D3. Reports page split

Extract from [reports/+page.svelte](../../src/routes/reports/+page.svelte):

- `ReportDefinitionForm.svelte`, `ReportRunViewer.svelte`. Lighter touch than
  D1/D2.

**Acceptance:** parent pages still render and behave identically on a manual
compose-dev walkthrough; extracted components import cleanly and are testable
in isolation.

---

## Workstream E — Playwright e2e specs

Each spec runs against the full compose stack (`api` + `postgres` +
`frontend`). Reuse the `loggedIn` fixture from A2.

| # | Spec | Coverage |
|---|---|---|
| E1 | Auth round-trip | bootstrap admin, login, logout, re-login, 401 on protected page when session cookie cleared, session-expired redirect |
| E2 | Post a transaction | open transactions, click "New", fill payee + amount + account + category, save, verify it appears in list and updates account balance on the accounts page |
| E3 | Reconcile | create starting balance, post a few txns, open reconcile, statement date + balance, check matching candidates, finish without offset; second pass with mismatched balance + offset account |
| E4 | Cross-currency transfer | gap-plan §1.2.2 — set up two accounts in different currencies, call `POST /api/v1/transactions/transfer` via UI, verify two legs visible and FX gain/loss split present |
| E5 | Import OFX | upload canned OFX file (from [apps/api/tests/data/](../../apps/api/tests/data/) — verify available), preview, commit, verify transactions land; re-import same file and verify no duplicates (gap-plan §1.4.3) |
| E6 | Reports smoke | run cashflow report for a seeded book, verify rows |

**Acceptance:** all 6 specs green on CI against a fresh compose stack.

---

## Workstream F — Targeted Tauri cleanup

Scope limited to root-level files. Keep `src-tauri/` and any Tauri parity
lookup material untouched.

- Remove from [package.json](../../package.json): `@tauri-apps/api`,
  `@tauri-apps/plugin-opener`, `@tauri-apps/cli` from deps/devDeps; remove
  `"tauri": "tauri"` script.
- Edit [vite.config.js](../../vite.config.js): drop `TAURI_DEV_HOST` block,
  fixed-port `1420`/HMR `1421`, and `**/src-tauri/**` watch-ignore.
- Run `npm install` to regenerate `package-lock.json`.
- Verify `npm run check` and `npm run build` still pass.
- Do **not** touch [src-tauri/](../../src-tauri/) directory itself, do **not**
  edit `SELF_HOSTED_MIGRATION_PLAN.md` parity sections.
- This is narrower than gap-plan §1.8 (which calls for full removal); document
  the deviation in [TODO.md](../../TODO.md) so Phase 1 #8 stays open for the
  full removal later.

---

## Workstream G — Docs & status updates

- Update [TODO.md](../../TODO.md): mark Phase 3 in flight; check off the
  Workstream B/C/E items as they ship.
- Update [v1-gap-plan.md](v1-gap-plan.md) §Phase 3 with delivered scope
  (Vitest+Playwright, refactor, what was extracted, what specs landed).
- Update [README.md](../../README.md) "Development" section with `npm test`
  and `npm run e2e` commands.
- Add a short [docs/architecture/frontend-testing.md](../architecture/frontend-testing.md):
  test pyramid, fixture conventions, when to use Vitest vs Playwright.

---

## Suggested execution order

1. **F** (Tauri cleanup) — tiny, isolated, lowers diff noise for the rest.
2. **A1** (Vitest infra) — gates everything else; sanity test only.
3. **B1–B7** (extract pure logic + unit tests) — each ships as its own
   commit/PR; cumulative coverage growth on each extraction.
4. **D1–D3** (component carving) — interleaved with C as needed.
5. **C1–C6** (component tests) — depends on D being far enough along to have
   stable surface.
6. **A2** (Playwright infra) — can run in parallel with C.
7. **A3** (CI workflow) — once both unit and at least one e2e spec are green
   locally.
8. **E1–E6** (e2e specs) — incremental.
9. **G** (docs) — at the end.

## Estimate

Gap-plan budgeted 3-4 days for the minimum. With extended scope + refactor:

| Workstream | Estimate (eng-days) |
|---|---|
| A — infra | 1 |
| B — logic extraction + unit tests | 1.0-1.5 |
| C — component tests | 1.5-2 |
| D — component extraction | 1.5-2 |
| E — Playwright specs | 1.5-2 |
| F — targeted Tauri cleanup | 0.5 |
| G — docs | 0.5 |
| **Total** | **7.5-9.5** |

Largest risk is D (component extraction) introducing regressions in two
1500+ LoC pages; mitigation is to land each extracted component in its own
commit and walk the UI manually after each.

## Open questions

1. **Playwright DB reset:** OK with a test-only `/api/v1/test/reset` endpoint
   gated by `REKENRAAM_E2E_RESET=1`, or prefer an ephemeral compose stack with
   a per-job Postgres volume?
2. **Component test mocking:** OK with `vi.mock("$lib/api/*")` per spec, or
   want a single shared mock-API module that specs override entries on?
3. ~~**`fuzzyMatch` semantics:** accent stripping / unicode normalization
   present today, or ASCII-only?~~ **Answered 2026-05-12 in B4:** the
   current implementation is **case-only** (lowercase + trim). No accent
   stripping, no unicode NFC/NFD normalization. `normalizeName("Café")` →
   `"café"` (the accented `é` survives). Multi-byte code-point iteration in
   `fuzzyMatch` means non-ASCII characters work as match targets, but they
   must match exactly. Pinned by tests in
   [src/lib/search/fuzzy.test.ts](../../src/lib/search/fuzzy.test.ts).
