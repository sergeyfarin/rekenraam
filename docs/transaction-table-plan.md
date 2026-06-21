# Transaction Table Implementation Plan

This plan is written to be executed by an LLM agent step by step. Each step lists
exact files, the change to make, and a verification command that must pass before
moving on. Do not skip verification. Every step leaves the app runnable.

## Reading order before you start

Read these first; they are the source of truth and override anything here on
conflict:

- `docs/conventions.md` — cross-cutting rules (money precision, API envelope,
  frontend stack, lifecycle vs reconciliation separation, the period-scoped
  reconciliation guard).
- `docs/transaction-ledger-core-plan.md` — the ledger schema, the transaction
  lifecycle, the reconciliation guard, and the API slice.
- `docs/product-requirements.md` — product source of truth and sequencing.
- `docs/early-architecture-decisions.md` and `docs/adrs/` — long-lived
  architectural constraints and decision records.
- `docs/categories-design.md` — categories are income/expense accounts; the
  built-in key lives in `metadata_json.$.category.builtin_key`.

### Lifecycle terms used throughout (do not conflate)

These are NOT a single five-step ladder. There are **three persisted statuses**
(`draft`, `posted`, `voided`), plus **one pre-persistence condition** (unsaved
entry) and **one independent flag** (soft-delete) that is orthogonal to status.

- **Unsaved entry** — pre-persistence UI working copy, no database row, no side
  effects. Not a status. Becomes `posted` (manual entry) or `draft`
  (autosave/import) only on save.
- **draft** — persisted `transaction_versions.status='draft'`. **System-only, not
  user-facing.** Produced only by autosave recovery, scheduled generation, and
  committed-import-awaiting-review. Manual entry never produces a draft. There is
  no "save as draft" button and users never pick `draft`. Excluded from the
  ledger; may trigger FX coverage.
- **posted** — in the ledger, directly editable. The default and only outcome of
  manual entry.
- **voided** — excluded from ledger but **stays visible** in the table marked
  voided; reversible via unvoid.
- **soft-deleted** — nullable `transactions.deleted_at` flag (NOT a status),
  orthogonal to `draft`/`posted`/`voided`. **Hidden** from the table and all
  ordinary views, durable and recoverable; reversible via restore. A transaction
  can be e.g. posted+deleted, or voided+deleted.

**The user-facing maturity line is reconciled vs not reconciled — not draft vs
posted.** Before a reconciliation checkpoint locks a posting, the transaction is
freely editable and removable; after, changes are guarded (period-scoped
reconciliation rule). The table/editor UI must present this distinction, not
draft-vs-posted, as what gates editability.

## Current state of the codebase (verified)

Already implemented — **consume, do not rebuild**:

- Lifecycle endpoints and services: `POST .../post`, `/void`, `/unvoid`,
  `/soft-delete`, `/restore`, `/correct`, `/approve`, and `DELETE` (never-posted
  draft hard delete). Registered in `backend/internal/api/health.go`.
- Frontend API mutation helpers in `frontend/src/lib/api/transactions.ts`:
  `postTransaction`, `voidTransaction`, `unvoidTransaction`,
  `softDeleteTransaction`, `restoreTransaction`, `correctTransaction`,
  `approveTransaction`, `deleteDraftTransaction`, plus `createTransaction`,
  `updateTransaction`, `getTransactions`, `getTransaction`, `getAccountRegister`,
  `transactionsInfiniteQueryOptions`.
- Reconciliation guard in `app/transactions.go`
  (`reconciliationAffectingPostingChange`): **currently the narrow rule** —
  override required only when a *reconciled* posting's account, commodity, value,
  scale, or entry date changes. `reconciliation_override` flag plumbed through
  update, void, unvoid, soft-delete. **This plan broadens it** to the
  period-scoped rule (guard any posting entering/leaving a reconciled period; see
  Step 9a) per the updated `conventions.md`/core plan; restore currently has no
  guard at all.
- List/register queries already exclude soft-deleted rows
  (`deleted_at IS NULL`).
- Reusable frontend primitives: `frontend/src/lib/components/` has
  `status-badge.svelte`, `panel.svelte`, `state-panel.svelte`,
  `page-header.svelte`, `api-form-error.svelte`, `top-bar.svelte`. Editor
  patterns to mirror: `frontend/src/lib/accounts/account-editor.svelte`,
  `frontend/src/lib/categories/category-editor.svelte`.
- Account editing currently lives **inside** the account-list screen
  (`frontend/src/routes/app/accounts/+page.svelte`). There is **no**
  `accounts/[id]/` route yet — the register route in Step 7 is greenfield, and
  the "row link" navigation must be added to the existing list screen. The same
  is true for categories (list-screen editing, no `categories/[id]/` route yet).
- `payeesQueryOptions({ q })` exists in `frontend/src/lib/api/payees.ts`.

**Not yet implemented — this plan builds it**:

- Posting/register response enrichment. `postingResponse` in
  `backend/internal/api/transactions.go` still carries only `account_id` and
  `commodity_id` — no name/class/symbol fields. `app.Posting` likewise.
- `category_id` filter on `GET /transactions`.
- `accountRegisterInfiniteQueryOptions` and `categoryID` in
  `TransactionListOptions` (frontend).
- Every UI component: table primitive, editor, filter bar, row actions, labels,
  list/register/category wrappers, and the three routes.
- No `frontend/src/routes/app/transactions/` route yet.

---

## Architecture

### Composition over configuration

Build one dumb display primitive (`transaction-table.svelte`) that accepts column
definitions and row data via typed props and Svelte 5 snippets. Each context
(global list, account register, category) gets a thin wrapper (~80–120 lines)
owning its query, column set, and row normalisation. The primitive never knows
about transactions, so a new context (e.g. commodity/stock view) is a new
wrapper, not a primitive change.

This is justified because the data shapes differ fundamentally:

| Context | API endpoint | Row unit | Unique columns | Filters |
|---|---|---|---|---|
| Global transactions | `GET /transactions` | Transaction | — | status, kind, account, payee, search, date, needs_review |
| Account register | `GET /accounts/{id}/register` | Posting (one per account leg) | Running balance | status, date |
| Category view | `GET /transactions?category_id=X` | Transaction | Category total | status, date, search |
| Commodity/stock (future) | `GET /transactions?account_id=X` | Posting | Quantity + commodity | status, date |

The register returns a flat posting-per-row with a running balance; the
transaction list returns a transaction-per-row with nested postings. One row
model for both forces awkward normalisation or a large `if (mode)` template tree.

### Layers

- **Layer 1 — primitive** `transaction-table.svelte`: generic table, infinite
  scroll, loading/empty/error/retry states, sticky header, keyboard navigation,
  priority-based column hiding.
- **Layer 2 — context wrappers**: `transaction-list.svelte`,
  `account-register.svelte`, `category-transactions.svelte`. Each owns its query
  and columns.
- **Layer 3 — routes**:
  - `/app/transactions` — global page, table + editor side panel.
  - `/app/accounts/[id]/register` — account register, new dedicated route (no
    `/app/accounts/[id]` route exists today; account editing stays on the list
    screen). A dedicated route so Back/Forward and bookmarking work for the
    primary daily view.
  - `/app/categories/[id]` — category transactions, new dedicated route (no
    `/app/categories/[id]` route exists today either).

Dedicated routes for account/category (not a drawer) because the register is the
primary daily interaction — bookmarkable, shareable, Back/Forward must work. The
global list keeps a side panel because the list *is* the page; the panel is for
editing, mirroring the existing account/category editor pattern.

---

## Step 1 — Backend: posting & register response enrichment

**Goal:** every transaction response path returns display-ready account and
commodity metadata so the frontend never fan-outs to resolve names.

**Rule — one lookup per page, not per posting.** Collect the union of account and
commodity IDs across all postings in the result set, run one bulk account query
and one bulk commodity query, join in memory.

### 1a. Add bulk lookup methods (db layer)

- `backend/internal/db/accounts.go` — add
  `AccountsByIDs(ctx, bookID int64, ids []int64) (map[int64]AccountRecord, error)`.
  It must return, per account: current `name` (nullable), `code` (nullable),
  `system_role` (nullable), `account_class`, and the category built-in key. The
  built-in key is **not a column** — it is
  `json_extract(av.metadata_json, '$.category.builtin_key')` on the current
  account version (see `db/categories.go:136,147` for the exact path). Use the
  current-account-version resolution already used by `CurrentAccountByID`.
- `backend/internal/db/commodities.go` — add
  `CommoditiesByIDs(ctx, bookID int64, ids []int64) (map[int64]CommodityRecord, error)`
  returning `code`, `symbol`/`display_symbol`, and `kind`. Note commodities used
  by postings may be currencies or securities; do not filter by
  `kind='currency'` here (existing `ListCurrencies` does — do not reuse it).

Use parameterised `IN (?, ?, ...)` with a bounded batch; dedupe IDs first. Return
a map keyed by ID.

### 1b. Add fields to `app.Posting` (app layer)

In `backend/internal/app/transactions.go`, add to `Posting`:

```go
AccountName       *string  // nil for system accounts with no user-visible name
AccountCode       *string  // nil if unset
AccountSystemRole *string  // non-nil for system accounts (e.g. "transfer_clearing")
AccountBuiltinKey *string  // non-nil for built-in/starter category accounts
AccountClass      string   // asset | liability | income | expense | equity
CommodityCode     string   // "USD", "EUR", "AAPL"
CommoditySymbol   *string  // "$", "€"; nil if unset
```

### 1c. One reusable enrichment helper (app layer)

Add a single helper, e.g.
`enrichPostings(ctx, txns []Transaction) error` (and an
`enrichRegisterEntries` variant or a shared inner function), that:
1. Walks the transactions/entries and collects the union of account and
   commodity IDs.
2. Calls `AccountsByIDs` and `CommoditiesByIDs` once each.
3. Joins results into each `Posting`'s new fields in memory.

**Every** response path must call it: list, account register, single read,
create, update, post, void, unvoid, soft-delete, restore, approve, correct.
Single-record paths pass a one-element slice. No path may emit partially
populated metadata — if a referenced account/commodity is missing, that is an
internal error, not a silent null.

### 1d. Propagate to API DTOs

In `backend/internal/api/transactions.go`, add to `postingResponse` and to the
register's posting field:

```go
AccountName       *string `json:"account_name"`
AccountCode       *string `json:"account_code"`
AccountSystemRole *string `json:"account_system_role"`
AccountBuiltinKey *string `json:"account_builtin_key"`
AccountClass      string  `json:"account_class"`
CommodityCode     string  `json:"commodity_code"`
CommoditySymbol   *string `json:"commodity_symbol"`
```

Map from `app.Posting` in the existing `postingResponse` construction sites
(there are construction points for list, register at ~line 625, and single/
mutation at ~720 — update all).

### 1e. OpenAPI + types

- Update the posting schema in
  `api/openapi/components/schemas/transactions.yaml` with the seven fields,
  marking `account_name`, `account_code`, `account_system_role`,
  `account_builtin_key`, `commodity_symbol` as nullable.
- Regenerate frontend types (see verification).

**Label resolution contract (document in the schema description and honour in the
frontend):** resolve the display label in this order — localized
`account_system_role`, localized `account_builtin_key`, user-entered
`account_name`, then `account_code` as a defensive fallback. Never render a null
name directly.

**Deferred:** version/status history in responses (no API exists; out of scope).

**Verify Step 1:**
```bash
cd backend && go build ./... && go test ./internal/app/ ./internal/db/ ./internal/api/
cd .. && pnpm --dir frontend run check
```
Add a backend test asserting a list response includes `account_class` and
`commodity_code` for a known seeded transaction, and that a system-account
posting (e.g. `transfer_clearing`) returns `account_system_role` non-null with
`account_name` null.

---

## Step 2 — Backend: `category_id` filter on `GET /transactions`

Categories are income/expense accounts. `account_id` already filters via an
EXISTS subquery on `posting_versions`. `category_id` uses the same EXISTS
pattern **plus** validates the resolved account has class `income` or `expense`,
so passing an asset account ID returns a clear `VALIDATION_FAILED` error.

`category_id` and `account_id` compose with **AND** when both are supplied — not
OR.

**Changes:**
- `db/transactions.go` — add `CategoryID int64` to `ListTransactionsParams`; add
  the EXISTS clause mirroring `AccountID`.
- `app/transactions.go` — add `CategoryID int64` to `ListTransactionsInput`;
  validate the account class (reuse the bulk/Current account lookup); pass
  through.
- `api/transactions.go` — parse `category_id` in `readTransactionListInput`.
- `api/openapi/...` — add the `category_id` query param.
- Regenerate types; add `categoryID` to `TransactionListOptions` in
  `frontend/src/lib/api/transactions.ts`.

**Verify Step 2:**
```bash
cd backend && go test ./internal/app/ ./internal/db/ ./internal/api/
```
Add tests: filtering by a valid income/expense `category_id` returns only
matching transactions; passing an asset account ID returns `VALIDATION_FAILED`;
`category_id` + `account_id` together AND-compose.

**Deferred to v2:** sort direction (`SortAsc`). The register and global page are
both most-recent-first; the cursor format can absorb ASC later without breaking.

---

## Step 3 — Frontend API layer

In `frontend/src/lib/api/transactions.ts`:

- Add `categoryID?: number` to `TransactionListOptions`; include it in the query
  string in `getTransactions` and in the infinite query key.
- Add `accountRegisterInfiniteQueryOptions(accountID, options)` mirroring
  `transactionsInfiniteQueryOptions`, calling the existing `getAccountRegister`,
  with `getNextPageParam` reading `next_cursor`.

**Verify Step 3:** `pnpm --dir frontend run check`.

---

## Step 4 — Display primitive & shared pieces

### 4a. `frontend/src/lib/transactions/transaction-table.svelte`

Generic, transaction-agnostic. Props:

```typescript
type Column<R> = {
  key: string;
  header: string;
  width?: string;
  align?: 'left' | 'right';
  priority?: number;       // 1 always shown; 2 hidden <600px; 3 hidden <900px
  cell: Snippet<[R]>;      // Svelte 5 snippet
};

let {
  rows,                // R[]
  columns,             // Column<R>[]
  isLoading,           // initial load
  isFetchingNextPage,  // a page fetch is in flight — gate the observer on this
  hasNextPage,
  onLoadMore,          // intersection observer at bottom; "Load more" button fallback
  error,               // Error | null — reactive value, drives persistent retry UI
  onRetry,             // () => void — re-runs the failed query
  onRowClick,          // optional (row: R) => void
}: Props<R>;
```

These map onto TanStack Query's `createInfiniteQuery` result fields; the wrapper
passes `error`, `isFetchingNextPage`, `fetchNextPage`, and `refetch` straight
through. `error` is a reactive value (not a callback) so retry UI persists while
the error stands. The observer must not call `onLoadMore` while
`isFetchingNextPage` is true, or it fires duplicate page requests.

Must handle: IntersectionObserver infinite scroll (gated on
`!isFetchingNextPage && hasNextPage`) with a button fallback, loading skeleton
rows, empty state (use `state-panel.svelte`), error state with a Retry button
wired to `onRetry`, sticky header, priority-based column hiding via CSS (no
horizontal scroll).

**Keyboard:** arrow keys move row focus; Enter activates `onRowClick`; Tab moves
into row action buttons within the focused row. Honour the conventions'
accessibility requirement.

Use Tailwind + existing semantic tokens; do not invent route-local colors.

### 4b. `frontend/src/lib/transactions/transaction-labels.ts`

- `formatQuantity(value: string, scale: number, locale, commodity): string` — the
  exact formatter. `quantity_value` is a JSON **string**; never coerce it to
  `number` at any step (loses 53-bit+ precision). **Do not pass a decimal string
  to `Intl.NumberFormat.format()`** — that coerces the argument to `Number` and
  destroys precision just as badly. Two safe paths:
  - **Preferred:** Dinero.js v2 (mandated by conventions) constructed from the
    integer coefficient + scale, using its formatting layer for locale output.
  - **Exact manual fallback:** split the coefficient with `BigInt` into integer
    and fractional parts by string-index arithmetic (`"12345"`, scale 2 →
    int `"123"`, frac `"45"`). Group the integer part using
    `Intl.NumberFormat(locale).formatToParts(BigInt(intPart))` (`Intl` handles a
    `BigInt` integer losslessly) to obtain the locale group/decimal separators,
    then reassemble with the untouched fractional string. Never build a JS
    `number` from the coefficient.
  Both paths must round-trip a value beyond `Number.MAX_SAFE_INTEGER` unchanged.
- `resolveAccountLabel(posting)` — applies the label resolution chain
  (`account_system_role` → `account_builtin_key` → `account_name` →
  `account_code`) through the Paraglide i18n boundary for the localized
  system-role and built-in-key labels. Never return a raw null.
- `statusLabel(status)` and `statusTone(status)` — map draft/posted/voided to
  localized copy and a `status-badge` tone (e.g. draft→neutral, posted→accent,
  voided→warning). All copy through Paraglide.
- `formatSignedAmount(posting, perspectiveClass)` — applies the sign convention
  table below.
- `commodityDisplay(posting)` — symbol if present, else `commodity_code` suffix.

### 4c. `frontend/src/lib/transactions/transaction-filter-bar.svelte`

Reusable filter controls; each context passes which filters to show. Controls:
status (draft/posted/voided), kind, date range, account, payee (autocomplete via
`payeesQueryOptions`), search (`q`), needs_review. Emits a typed filter object.
Use `minisearch` only for small in-memory dropdowns (accounts/payees), never for
the transaction list itself (server FTS5 handles `q`).

### 4d. `frontend/src/lib/transactions/transaction-row-actions.svelte`

Light row-level actions only; consequential actions (void, soft-delete, restore)
live on the detail panel (Step 6). Actions are driven by **status AND whether the
transaction is reconciliation-locked** — not by a draft-vs-posted maturity idea.
The main transactions table shows `posted` and `voided` rows; `draft` is
system-only and surfaces in its producing workflow (import-review tray), not the
general list. Each row's actions:

| Row state | Row actions | Notes |
|---|---|---|
| posted, not locked | Edit, Create correction | Freely editable — "not locked" = no affected posting falls in a reconciled period |
| posted, reconciliation-locked | Edit (warns on save), Create correction | Edit opens normally; the period-guard warning fires at save only if the change actually shifts a reconciled balance |
| voided | Unvoid, View | Unvoid re-enters the ledger and is period-guarded |
| any + needs_review | Approve (inline, no editor) | Import-set flag only |

Notes:
- "Reconciliation-locked" is a per-row hint: any posting whose `entry_date` is on
  or before the latest active checkpoint for its account/commodity. Use the
  backend-provided per-row flag from Step 1/Step 9 enrichment rather than
  computing it client-side; if the flag is not yet available, treat the row as
  potentially locked and let the save-time guard decide.
- The row menu never shows `Post` (no user-facing draft promotion) or a hard
  `Delete` (hard delete is only for never-posted system drafts, exposed in the
  import-review tray, not here). Removal of a posted/voided transaction is
  Soft-delete on the detail panel.
- Soft-delete and Restore are detail-panel actions (Step 6), available on both
  posted and voided rows since the flag is independent of status.

**Verify Step 4:** `pnpm --dir frontend run check`, then `pnpm --dir frontend run test`.

**Prerequisite (do this before adding the test):** the repo has **no frontend
test runner configured** — there is no `test` script in `frontend/package.json`
and no Vitest setup. Conventions allow "focused component or unit tests when
introduced," so introducing Vitest is in scope but is its own small task: add
`vitest` (+ `@testing-library/svelte` if component tests are wanted) as dev deps,
add a `"test": "vitest run"` script, and a minimal `vitest.config.ts`. Treat this
as Step 4.0. If you cannot set up Vitest in this pass, do not silently drop the
test — flag it and leave the formatter test as a tracked TODO rather than writing
a verification line that never runs.

Then add a `formatQuantity` unit test covering scale 0, scale 2, a value beyond
`Number.MAX_SAFE_INTEGER` (must round-trip exactly), and a negative value.

---

## Step 5 — Transaction editor `transaction-editor.svelte`

Largest piece; can be built independently of the table. Mirror the structure of
`account-editor.svelte`/`category-editor.svelte` (panel, runes, `api-form-error`).

### Three progressive tiers

**Tier 1 — Simple (default):** Date, Payee (autocomplete), Description/memo,
Category (income/expense account only — must not select asset/liability),
Amount, Account (the asset/liability leg). Covers one asset/liability leg + one
category leg. Builds a balanced two-posting `journal_entries[]` payload at save.

**Tier 2 — Advanced (expandable):** transaction kind (ordinary / adjustment —
`opening_balance` is system-only; `transfer` is set by the explicit Transfer
workflow, not selectable), note (markdown), external reference/import hint,
per-posting memos.

- Do **not** expose `needs_review` (always false for manual entries; only the
  import pipeline sets it).
- Do **not** expose reconciliation status (auditable workflow, never a manual
  form field).

**Tier 3 — Split (button):** full posting-list editor; multiple legs each with an
amount; show a running total and highlight imbalance (balance enforced by
backend; check client-side with Dinero before submit). Tiers 1/2 construct the
same `journal_entries[]` payload as Tier 3 — the abstraction is cosmetic.

### Save and cancel routing (the editor owns this, not the table)

**There is no user-facing "save as draft."** Manual entry always saves to
`posted` (`POST /transactions` with `status:"posted"`). `draft` is system-only
(autosave/import) and the editor never offers it as a choice. The editor's
primary save action is a single "Save" / "Add transaction" that posts.

The backend permits `PATCH` on any transaction that is not voided and not
soft-deleted. Route by case:

| Case | Trigger | Call | Behaviour |
|---|---|---|---|
| New entry (unsaved) | Composing a new transaction | `POST /transactions` (`status:"posted"`) on Save | Working copy only — no row, no FX — until Save. No draft created |
| Posted edit — not in a reconciled period | No affected posting falls on/before a latest active checkpoint | `PATCH /transactions/{id}` | Opens and saves directly, no warning |
| Posted edit — non-financial only | Only category/description/payee/note/tags change | `PATCH /transactions/{id}` | Always allowed, reconciliation intact, no override — even inside a reconciled period |
| Posted edit — changes a reconciled balance | Adds/removes/re-dates a posting in a reconciled period, or changes a reconciled posting's account/commodity/amount/scale/date | `PATCH /transactions/{id}` with `reconciliation_override: true` | Warning modal **at save time** naming the affected checkpoint(s); proceed only on confirm |
| Voided edit | Transaction is voided | Unvoid first, then `PATCH` | Editor offers Unvoid (which re-enters the ledger and is itself period-guarded); editing blocked until unvoided |
| Draft edit (system) | A persisted draft in its producing workflow (import-review tray) | `PATCH /transactions/{id}` | Out of scope for the general editor; the import-review surface owns draft editing/promotion |
| Corrective transaction | "Create correction" | `POST /transactions/{id}/correct` | Opens in correction mode |

The guard uses the **period-scoped** rule (see `docs/conventions.md` and the core
plan): a change is guarded when it would alter a reconciled balance — either it
touches a reconciled posting's facts, or it adds/removes/re-dates a posting whose
`entry_date` is on or before the latest active checkpoint `statement_date` for an
affected account/commodity, even if that posting is not itself flagged
reconciled. Whether a specific edit crosses that line is only knowable after the
user edits, so the warning fires at save, not on open. If the backend returns
`reconciliation override is required`, fetch the affected-checkpoint details
(Step 9's preview/conflict read model) and show the warning naming them, then
retry with `reconciliation_override: true` on confirm.

**Cancel semantics:**
- Cancel on a new (unsaved) entry: discard the working copy entirely. No row was
  created, nothing to clean up. No draft is left behind.
- Cancel on an edit of an existing transaction: discard pending field changes and
  return to the read-only detail view; the persisted transaction is unchanged
  (no `PATCH` was sent).
- Autosave (if/when implemented) persists a `draft` row for crash recovery only;
  it is never the user's "save." A future autosave design must define its own
  discard path and must not surface the recovery draft as a user-chosen status.

**Create correction** makes a *new* adjusting transaction linked via
`correction_of_transaction_id`; it does not void or replace the original — both
exist and net to the corrected state. The label/UI must make clear it is an
*additional* transaction.

### Amount sign convention

User-facing amount is **inflow positive / outflow negative from the account's
perspective**, translated from the ledger's debit/credit by account class:

| Account class | Debit | Credit |
|---|---|---|
| Asset | Inflow (+) | Outflow (−) |
| Liability | Outflow (−) | Inflow (+) |
| Income | Outflow (−) | Inflow (+) |
| Expense | Inflow (+) | Outflow (−) |

In the editor, Amount is relative to the selected **Account** (asset/liability):
positive = money arrived. Backend posting signs are derived from account class at
save, not stored from the field directly.

**Transfers are a distinct workflow.** Tier 1's Category control offers only
income/expense accounts. An explicit "Transfer" entry point opens Tier 3 in
transfer mode (From/To account controls). Never infer transfer from an invalid
Category choice.

**Verify Step 5:** `pnpm --dir frontend run check`. Manually: create a new
transaction (saves directly to posted — confirm there is no "save as draft"
choice), edit a posted transaction, edit only the category of a reconciled
transaction (no warning), then change a reconciled posting's amount (warning +
override required, naming the checkpoint), and Cancel an in-progress new entry
(no row left behind).

---

## Step 6 — Global transactions page

### `frontend/src/lib/transactions/transaction-list.svelte`

- `createInfiniteQuery(transactionsInfiniteQueryOptions(filters))`.
- Columns (primary posting = first asset/liability leg; for income/expense-only
  transactions fall back to the first posting):

| Column | Source | Priority | Notes |
|---|---|---|---|
| Date | `transaction_date` | 1 | |
| Payee / Description | `payee_name` or `description` | 1 | |
| Amount | Primary posting, signed-formatted | 1 | Sign convention by primary posting class |
| Account(s) | Primary posting label (resolution chain) | 2 | "N accounts" when more than two |
| Status | `status` | 3 | Badge draft/posted/voided (soft-deleted are excluded from the list, not badged) |
| Flags | `needs_review` | 3 | Review badge |

- Full filter bar (status, kind, date, account, payee, search, needs_review).

### `frontend/src/routes/app/transactions/+page.svelte`

- Mounts `transaction-list.svelte` as the main area.
- Side panel (mirror account/category page pattern) that slides in on row click
  or "New transaction". Clicking a row opens a **read-only detail panel** first:
  - All fields in a clean read layout.
  - Full posting breakdown with resolved account labels and commodity symbols.
  - **Edit** — opens the editor in the panel, replacing the detail view (posted
    is directly editable; voided must be unvoided first).
  - **Create correction** — opens editor in correction mode (posted only).
  - **Action buttons (status-appropriate):** Post, Approve, Void with reason,
    Unvoid, Soft-delete with reason, Restore. Void keeps the row visible marked
    voided; Soft-delete hides it from the table but keeps it recoverable. Void
    and Soft-delete are independent and each requires a reason.
- No version/audit history in the panel for v1 (no API).
- **Mobile:** side panel becomes a full-screen overlay (no split). The editor on
  mobile is a full-screen form / bottom sheet below the breakpoint.
- The side panel hosts the editor; it is never used for navigation.

**Soft-delete recovery:** soft-deleted transactions never appear in the list or
for any `status` value. Restore from the detail panel works once a transaction is
opened by id. A dedicated trash/recovery view for browsing and restoring them is
built in **Step 9** as a settings subpage.

**Verify Step 6:** `pnpm --dir frontend run check`; run the app and confirm list
loads, row opens detail, edit/post/void/soft-delete round-trip.

---

## Step 7 — Account register route

### `frontend/src/lib/transactions/account-register.svelte`

- `createInfiniteQuery(accountRegisterInfiniteQueryOptions(accountID, filters))`.
- Normalises `accountRegisterEntryResponse` (posting-per-row) into display rows.
- Columns:

| Column | Source | Priority | Notes |
|---|---|---|---|
| Date | `entry_date` | 1 | Journal entry date, not transaction date |
| Payee / Description | `payee_name` or `description` | 1 | |
| Amount | `posting.quantity_value`, signed | 1 | Inflow +/outflow − from *this* account's class (known from context) |
| Balance | `running_balance` | 1 | Right-aligned, account's commodity |
| Reconciliation | `posting.reconciliation_status` | 2 | Icon uncleared/cleared/reconciled |
| Memo | `posting.memo` | 3 | |

- Filter bar: status, date range only (matches the register API).
- Mobile: priority-1 columns in a responsive grid; Payee may wrap; Amount and
  Balance stay right-aligned.

### `frontend/src/routes/app/accounts/[id]/register/+page.svelte`

- Mounts `account-register.svelte` for the route's account id.
- Embeds the editor side panel for editing a clicked entry's transaction.
- **Navigation:** make the account list row navigate to
  `/app/accounts/[id]/register` as its default action (register is the primary
  daily destination, not edit). Use the **stretched-link** pattern, not a
  row-level `<a>` wrapping the action buttons: a single `<a>` for the register
  link with an absolutely-positioned `::after` covering the row, and the
  Edit/Close/Archive `<button>`s as siblings at a higher stacking context (so
  they remain valid, focusable, and keyboard-reachable). Do **not** nest
  interactive elements inside the `<a>` — that is invalid HTML and breaks assistive
  tech. The action buttons sit above the stretched link, so a click on them does
  not trigger navigation without relying on `stopPropagation`.

**Verify Step 7:** `pnpm --dir frontend run check`; confirm register loads with a
running balance and respects status/date filters.

---

## Step 8 — Category transactions route

### `frontend/src/lib/transactions/category-transactions.svelte`

- `createInfiniteQuery(transactionsInfiniteQueryOptions({ categoryID, ...filters }))`.
- **Category amount = per-commodity sums; never combine commodities.** A category
  is an income/expense account and may hold postings in different commodities
  (e.g. a EUR and a USD grocery purchase). **Group postings to the category
  account by `commodity_id` first**, then sum exactly within each commodity group
  (rescale each integer coefficient to the greatest scale present in that group
  using `BigInt`, sum, retain the common scale). EUR and USD must never be
  rescaled together or added — that produces a meaningless number. A row with
  postings in two commodities shows two amounts (e.g. "€120.00" and "$40.00",
  stacked or "+N more"); never one fused total. Never sum JS numbers or formatted
  strings. (This per-commodity rule applies anywhere postings are aggregated —
  running balances and any future totals included.)
- **Category-activity sign convention** (not cashflow), applied within each
  commodity group: a debit to an expense category and a credit to an income
  category both display **positive**; reversals display negative.
- Columns:

| Column | Source | Priority | Notes |
|---|---|---|---|
| Date | `transaction_date` | 1 | |
| Payee / Description | `payee_name` or `description` | 1 | |
| Amount | Sum of postings to the category | 1 | Activity-positive; reversals negative |
| Counterpart | Non-category posting label(s) | 2 | "N accounts" when more than one |
| Status | `status` | 3 | |
| Flags | `needs_review` | 3 | |

- Filter bar: status, date range, search.

### `frontend/src/routes/app/categories/[id]/+page.svelte`

- Mounts `category-transactions.svelte`; embeds the editor side panel.
- **Navigation:** make the category list row navigate to `/app/categories/[id]`
  using the same **stretched-link** pattern as the account register (single `<a>`
  with a covering `::after`; action `<button>`s as higher-stacked siblings, never
  nested inside the `<a>`).

**Verify Step 8:** `pnpm --dir frontend run check`; confirm category totals match a
known split transaction.

---

## Step 9 — Trash / recovery (settings subpage)

A dedicated view to browse and restore soft-deleted transactions, with a
recovery rule that protects reconciled periods. This step has both backend and
frontend work.

### Recovery rule (the safety contract)

This applies the **period-scoped reconciliation guard** (now the source-of-truth
rule in `docs/conventions.md` and the core plan). Restoring a soft-deleted
*posted* transaction puts its postings back into the ledger; if any fall within
an already-reconciled period, restoring changes a reconciled balance and must be
guarded.

- **Voided transactions are exempt.** A voided transaction is already out of the
  ledger, so restoring it from soft-delete only restores *visibility* of an
  out-of-ledger record and changes no balance. Restoring a voided+soft-deleted
  transaction is **always easy, never reconciliation-guarded.** (Re-entering it
  into the ledger would be a separate unvoid, which is itself guarded.)
- **Easy restore (default for posted):** restorable with no warning when no
  affected posting falls on or before the latest active reconciliation checkpoint
  for its `(account_id, commodity_id)` — i.e. every affected posting `entry_date`
  is strictly after that account/commodity's most recent active checkpoint
  `statement_date`, or there is no active checkpoint. The common case.
- **Guarded restore (posted in a reconciled period):** if any affected posting is
  on or before a latest active checkpoint, restore requires explicit
  `reconciliation_override: true` and invalidates that checkpoint plus all later
  active checkpoints for the same account/commodity — the same
  override-and-invalidate mechanism used by edit/void/unvoid/soft-delete. The UI
  warns, naming the affected checkpoint(s), like the guarded edit path in Step 5.

The checkpoint is the lock floor; crossing it is allowed but never silent.

### 9a. Backend — restore guard (period-scoped)

Today `RestoreTransaction` (`app/transactions.go`,
`setTransactionDeleted(..., false)`) performs **no** reconciliation check. Add
the period-scoped guard:

- **Exempt voided transactions first:** if `current.Status == "voided"`, skip the
  guard entirely — restore changes no balance.
- Otherwise compute affected checkpoint refs by the **period** rule, not the
  reconciled-posting-facts rule. For each posting, look up the latest active
  checkpoint for its `(account_id, commodity_id)` and flag it when the posting's
  `entry_date <= checkpoint.statement_date`. This differs from soft-delete's
  `reconciliationRefsFromTransaction` (which keys on
  `reconciliation_status='reconciled'`): a soft-deleted posting may not itself be
  flagged reconciled yet still fall inside a reconciled period.
- Add a repository query, e.g.
  `LatestActiveCheckpointByAccountCommodity(ctx, bookID, accountID, commodityID)`
  reading `reconciliation_checkpoints` (filter `status='active'`,
  `ORDER BY statement_date DESC, id DESC LIMIT 1`; the
  `reconciliation_checkpoints_account_idx` index already supports this).
- If any posting trips the period check and `!input.ReconciliationOverride`,
  return `ErrReconciliationOverrideRequired`. With override, pass the affected
  checkpoint refs through `InvalidateCheckpointRefs` (plumbing already exists on
  `SetTransactionDeletedParams`).

**Apply the same period rule to the other in-ledger mutations** so the guard is
consistent everywhere (this is the rule change agreed in conventions/core plan,
not restore-only): create/edit (`reconciliationInvalidationRefs`) and unvoid must
also flag postings that enter a reconciled period, not only changes to
already-reconciled postings. Update `reconciliationAffectingPostingChange` and its
callers accordingly. Soft-delete of a posted transaction in a reconciled period
is likewise guarded.

Add backend tests: restore of a post-checkpoint posted transaction succeeds with
no override; restore of a posted transaction dated on/before the latest active
checkpoint fails without override and, with override, succeeds and invalidates
that checkpoint and later ones; restore of a voided+soft-deleted transaction in a
reconciled period succeeds with no override (exempt); create/edit/unvoid that
places a posting in a reconciled period is guarded.

### 9a-bis. Backend — affected-checkpoint preview (so warnings can name them)

The generic `ErrReconciliationOverrideRequired` cannot tell the UI *which*
checkpoints are affected, but Step 5 and Step 9 both require warnings that name
them. Add a read-only preview the editor/trash UI can call before confirming:

- A dry-run/preview endpoint or read model that, given a pending operation
  (restore, unvoid, or an edit payload) for a transaction, returns the list of
  affected active checkpoints: `{ account_id, account_label, commodity_code,
  statement_date, checkpoint_id }[]`. Prefer a dedicated preview endpoint (e.g.
  `POST /api/v1/transactions/{id}/reconciliation-impact`) over enriching the
  error envelope, because the error envelope is a stable `{code, message}` shape
  per conventions and must not grow structured payloads.
- The UI calls preview when it knows an override may be required (or after
  receiving `ErrReconciliationOverrideRequired`), renders the named checkpoints in
  the warning modal, then retries the real mutation with
  `reconciliation_override: true`.
- Update OpenAPI and regenerate types.

### 9b. Backend — list soft-deleted

Add a deleted-only listing path so the trash view can enumerate soft-deleted
transactions (the list query currently hard-excludes `deleted_at IS NULL`; only
single-record `TransactionByIDIncludingDeleted` exists).

**Use a dedicated trash endpoint, not a `deleted=true` flag on `/transactions`.**
The existing list cursor encodes and filters by `(transaction_date, id)` (see
`EncodeTransactionCursor`/`DecodeTransactionCursor` in `db/transactions.go`).
The trash view must order by deletion recency (`deleted_at DESC, id DESC`), which
the date cursor cannot express — overloading `/transactions?deleted=true` would
silently break pagination. A separate endpoint with its own cursor avoids that.

- `db/transactions.go` — add a deleted-only query ordered by
  `deleted_at DESC, id DESC`, with a **deletion cursor** `(deleted_at, id)`
  (add `Encode/DecodeDeletionCursor`; do not reuse the date cursor). Select the
  `delete_reason`, `deleted_at`, and `deleted_by_user_id` snapshot columns.
- `app/transactions.go` — a `ListDeletedTransactions` service; still run Step 1
  enrichment so rows render with names. Per row, compute a
  `restore_blocked_by_reconciliation` boolean using 9a's **period** check
  (and honouring the voided exemption: a voided row is never blocked) so the UI
  shows easy-restore vs guarded **without** a per-row probe request.
- `api/transactions.go` + OpenAPI — add `GET /api/v1/transactions/deleted`
  (cursor-paginated) returning the deleted rows plus the snapshot fields and the
  per-row flag. Regenerate types.

### 9c. Frontend — settings subpage

- Route: `frontend/src/routes/app/settings/trash/+page.svelte` (sibling of the
  existing `settings/currencies`, `settings/appearance`). Add it to the settings
  navigation.
- Reuse `transaction-table.svelte` with a trash column set: Deleted date, Date,
  Payee/Description, Amount, Delete reason, and a Restore action. Query the
  deleted-only list via a new `deletedTransactionsInfiniteQueryOptions` in
  `transactions.ts`.
- Restore action calls the existing `restoreTransaction` helper. Rows where
  `restore_blocked_by_reconciliation` is false restore in one click. Rows where
  it is true show the warning modal (naming the checkpoint) and only then retry
  with `reconciliation_override: true`, consistent with Step 5's override flow.
- Empty state via `state-panel.svelte`. All copy through Paraglide.

**Verify Step 9:**
```bash
cd backend && go test ./internal/app/ ./internal/db/ ./internal/api/
cd .. && pnpm --dir frontend run check
```
Manually: soft-delete a recent transaction, open Settings → Trash, restore it in
one click; soft-delete a transaction inside a reconciled period and confirm the
restore warning fires and override is required.

---

## FX and multi-commodity display (all contexts)

For v1 each context shows the **native amount of the relevant posting** only — no
synthetic base-currency equivalent:

- Global list: native amount of the primary asset/liability posting, with symbol.
- Account register: native amount of this account's posting, account's commodity.
- Category: sum of postings to the category, in the category's commodity.

A synthetic `base_currency_value` (summing legs converted to book currency) is
**deferred** — naive summation double-counts same-currency transfers and is
ambiguous for splits/FX pairs; it needs per-kind rules first.

For FX transfers in the global list, the primary leg's amount is a single
commodity; do not attempt two Amount columns in v1.

---

## Mobile presentation

The table adapts via column `priority`:
- Priority 1 always shown.
- Priority 2 hidden below ~600px.
- Priority 3 hidden below ~900px.

Mobile global list = Date + Payee + Amount. Mobile register = Date + Payee +
Amount + Balance (responsive grid; Payee wraps; Amount/Balance right-aligned).
Hide lower-priority columns rather than horizontal-scroll. The editor is a
full-screen form / bottom sheet below the breakpoint.

---

## Definition of done

- `cd backend && go build ./... && go test ./...` passes.
- `pnpm --dir frontend run check` passes (0 errors, 0 warnings), and
  `pnpm --dir frontend run test` passes (Vitest set up per Step 4.0).
- All response paths return enriched posting metadata (verified by test).
- `category_id` filter works and validates account class.
- Category and any aggregate amounts are **per-commodity** — different commodities
  are never rescaled or summed together.
- Money is never coerced to a JS `number` at any step, including formatting
  (no decimal string passed to `Intl.NumberFormat.format()`).
- Global list, account register, and category routes render, paginate, filter,
  and open the editor; list rows navigate via the stretched-link pattern (no
  nested interactive elements inside an `<a>`).
- Editor saves manual entry directly to `posted` (no "save as draft"), handles
  posted edit (incl. period-scoped reconciliation override at save with named
  checkpoints), correction, the transfer entry point, and well-defined Cancel
  semantics.
- The period-scoped reconciliation guard is enforced consistently across
  create/edit/void/unvoid/soft-delete/restore; restoring a voided+soft-deleted
  transaction is exempt (visibility only).
- Detail panel exposes Post/Approve/Void/Unvoid/Soft-delete/Restore correctly by
  status and the soft-delete flag, each consequential action requiring a reason.
- Soft-deleted transactions never appear in the main list; voided transactions
  appear marked voided. The Settings → Trash subpage (its own deleted endpoint +
  deletion cursor) lists soft-deleted transactions and restores them with the
  period-scoped recovery guard and per-row impact flag.
- New copy goes through Paraglide; new components use Svelte 5 runes and semantic
  tokens.

---

## Resolved decisions

These were previously open; they are now settled and reflected in the steps
above:

1. **Account list → register navigation:** the account list row itself links to
   `/app/accounts/[id]/register`; per-row action buttons stop propagation.
   (Step 7.)
2. **Category list → navigation:** the category list row itself links to
   `/app/categories/[id]`, same pattern. (Step 8.)
3. **Trash/recovery:** a Settings → Trash subpage
   (`/app/settings/trash`) lists and restores soft-deleted transactions.
   Easy one-click restore is allowed only for transactions after the latest
   active reconciliation checkpoint; older ones require reconciliation override
   and invalidate affected checkpoints. (Step 9.)
