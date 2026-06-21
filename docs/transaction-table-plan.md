# Transaction Table Implementation Plan

This plan is written to be executed by an LLM agent step by step. Each step lists
exact files, the change to make, and a verification command that must pass before
moving on. Do not skip verification. Every step leaves the app runnable.

## Reading order before you start

Read these first; they are the source of truth and override anything here on
conflict:

- `docs/conventions.md` — cross-cutting rules (money precision, API envelope,
  frontend stack, lifecycle vs reconciliation separation).
- `docs/transaction-ledger-core-plan.md` — the ledger schema, the five-state
  lifecycle (unsaved entry, draft, posted, voided, soft-deleted), the
  reconciliation guard, and the API slice.
- `docs/categories-design.md` — categories are income/expense accounts; the
  built-in key lives in `metadata_json.$.category.builtin_key`.

### Lifecycle terms used throughout (do not conflate)

- **Unsaved entry** — in-progress UI working copy, no database row, no side
  effects. Not a status. Becomes draft/posted only on save.
- **draft** — persisted `transaction_versions.status='draft'`. Excluded from the
  ledger; may trigger FX coverage. A real row with row actions.
- **posted** — in the ledger, directly editable.
- **voided** — excluded from ledger but **stays visible** in the table marked
  voided; reversible via unvoid.
- **soft-deleted** — nullable `transactions.deleted_at` flag (NOT a status),
  **hidden** from the table and all ordinary views, durable and recoverable;
  reversible via restore. Independent of voided.

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
  (`reconciliationAffectingPostingChange`): override required only when a
  reconciled posting's account, commodity, quantity value, quantity scale, or
  entry date changes. `reconciliation_override` flag plumbed through update,
  void, unvoid, soft-delete.
- List/register queries already exclude soft-deleted rows
  (`deleted_at IS NULL`).
- Reusable frontend primitives: `frontend/src/lib/components/` has
  `status-badge.svelte`, `panel.svelte`, `state-panel.svelte`,
  `page-header.svelte`, `api-form-error.svelte`, `top-bar.svelte`. Editor
  patterns to mirror: `frontend/src/lib/accounts/account-editor.svelte`,
  `frontend/src/lib/categories/category-editor.svelte`.
- `frontend/src/routes/app/accounts/[id]/+page.svelte` exists (currently an
  account editor view, not a register).
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
  - `/app/accounts/[id]/register` — account register, dedicated route (the
    existing `/app/accounts/[id]` editor route is left intact; the register is a
    sibling so Back/Forward and bookmarking work for the primary daily view).
  - `/app/categories/[id]` — category transactions, dedicated route.

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
cd ../frontend && npm run check
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

**Verify Step 3:** `cd frontend && npm run check`.

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
  rows,          // R[]
  columns,       // Column<R>[]
  isLoading,
  hasNextPage,
  onLoadMore,    // intersection observer at bottom; "Load more" button fallback
  onRowClick,    // optional (row: R) => void
  onError,       // optional (err: Error) => void — show retry UI when set
}: Props<R>;
```

Must handle: IntersectionObserver infinite scroll with a button fallback,
loading skeleton rows, empty state (use `state-panel.svelte`), error/retry state,
sticky header, priority-based column hiding via CSS (no horizontal scroll).

**Keyboard:** arrow keys move row focus; Enter activates `onRowClick`; Tab moves
into row action buttons within the focused row. Honour the conventions'
accessibility requirement.

Use Tailwind + existing semantic tokens; do not invent route-local colors.

### 4b. `frontend/src/lib/transactions/transaction-labels.ts`

- `formatQuantity(value: string, scale: number): string` — the exact formatter.
  `quantity_value` is a JSON **string**; never coerce to `number` (loses 53-bit+
  precision). Insert the decimal point by string/`BigInt` arithmetic
  (`"12345"`, scale 2 → `"123.45"`), then pass the decimal string to
  `Intl.NumberFormat` with `{ minimumFractionDigits: scale, maximumFractionDigits: scale }`.
  Convention also mandates Dinero.js v2 for money — prefer constructing a Dinero
  amount from the integer coefficient + scale and using its formatting layer; if
  Dinero is not yet wired, the `BigInt` + `Intl` path above is the fallback, but
  still never via `number`.
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

Status-driven action menu. Light row-level actions only; consequential actions
(void, soft-delete) live on the detail panel (Step 6).

| Status | Row actions |
|---|---|
| draft | Edit, Post, Delete (hard delete — never-posted draft, via `deleteDraftTransaction`) |
| posted | Edit, Create correction |
| voided | Unvoid, View |
| any + needs_review | Approve (inline, no editor) |

**Verify Step 4:** `cd frontend && npm run check`. Add a focused unit test for
`formatQuantity` covering scale 0, scale 2, a value beyond `Number.MAX_SAFE_INTEGER`,
and a negative value.

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

### Save routing (the editor owns this, not the table)

The backend permits `PATCH` on any transaction that is not voided and not
soft-deleted. Route by case:

| Case | Trigger | Call | Behaviour |
|---|---|---|---|
| New entry (unsaved) | Composing a new transaction | `POST /transactions` on first save | Working copy only; no row, no FX, until saved |
| Draft edit | Persisted draft | `PATCH /transactions/{id}` | Opens directly, no warning |
| Posted edit — safe | No reconciled posting affected | `PATCH /transactions/{id}` | Opens directly, no warning (posted is directly editable) |
| Posted edit — non-financial on reconciled | Only category/description/payee/note/tags change | `PATCH /transactions/{id}` | Allowed, reconciliation stays intact, no override |
| Posted edit — reconciliation-invalidating | Reconciled posting's account/commodity/amount/scale/date changes | `PATCH /transactions/{id}` with `reconciliation_override: true` | Warning modal **at save time**: "This will invalidate a reconciliation checkpoint. Continue?" naming the affected checkpoint |
| Voided edit | Transaction is voided | Unvoid first, then `PATCH` | Editor offers Unvoid; editing blocked until unvoided |
| Corrective transaction | "Create correction" | `POST /transactions/{id}/correct` | Opens in correction mode |

The safe vs reconciliation-invalidating distinction is only knowable after edits,
so the warning fires at save, not on open. If the backend returns
`reconciliation override is required`, surface the warning modal and retry with
`reconciliation_override: true` on confirm.

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

**Verify Step 5:** `cd frontend && npm run check`. Manually: create, save a draft,
post it, edit a posted transaction.

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

**Verify Step 6:** `cd frontend && npm run check`; run the app and confirm list
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
- **Navigation:** make the account list row itself a link to
  `/app/accounts/[id]/register`. The whole row navigates; keep the existing
  Edit/Close/Archive actions as explicit buttons within the row (stop event
  propagation so an action click does not also trigger navigation). The register
  is the primary daily destination, so the row's default action is "open
  register," not "edit."

**Verify Step 7:** `cd frontend && npm run check`; confirm register loads with a
running balance and respects status/date filters.

---

## Step 8 — Category transactions route

### `frontend/src/lib/transactions/category-transactions.svelte`

- `createInfiniteQuery(transactionsInfiniteQueryOptions({ categoryID, ...filters }))`.
- **Category amount** = sum of all postings to the selected category account
  within each transaction (a split may have several). Sum exactly: rescale each
  integer coefficient to the greatest scale present using `BigInt`, sum, retain
  the common scale. Never sum JS numbers or formatted strings.
- **Category-activity sign convention** (not cashflow): a debit to an expense
  category and a credit to an income category both display **positive**;
  reversals display negative.
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
- **Navigation:** make the category list row itself a link to
  `/app/categories/[id]`, consistent with the account-register row-link pattern.
  Keep any per-row action buttons explicit and stop their propagation so they do
  not also trigger navigation.

**Verify Step 8:** `cd frontend && npm run check`; confirm category totals match a
known split transaction.

---

## Step 9 — Trash / recovery (settings subpage)

A dedicated view to browse and restore soft-deleted transactions, with a
recovery rule that protects reconciled periods. This step has both backend and
frontend work.

### Recovery rule (the safety contract)

Restoring a soft-deleted transaction puts its postings back into the ledger. If
those postings fall within an already-reconciled period, restoring would silently
change a reconciled balance. So:

- **Easy restore (default):** a soft-deleted transaction is restorable with no
  warning **only when none of its postings sit on or before the latest active
  reconciliation checkpoint** for their `(account_id, commodity_id)`. In
  practice: every affected account's posting `entry_date` is strictly after that
  account/commodity's most recent active checkpoint `statement_date` (or the
  account/commodity has no active checkpoint at all). This is the common case —
  recovering something deleted since the last reconciliation.
- **Guarded restore:** if any affected posting is on or before a latest active
  checkpoint, restore requires explicit `reconciliation_override: true` and
  invalidates that checkpoint plus all later active checkpoints for the same
  account/commodity — the same override-and-invalidate mechanism already used by
  edit, void, and soft-delete. The UI must warn, naming the affected
  checkpoint(s), exactly like the reconciliation-invalidating edit path in
  Step 5.

This mirrors how the reconciled posting itself works: the checkpoint is the lock
floor; crossing it is allowed but never silent.

### 9a. Backend — restore guard

Today `RestoreTransaction` (`app/transactions.go`,
`setTransactionDeleted(..., false)`) performs **no** reconciliation check. Add
one symmetric to the soft-delete path:

- Compute the affected checkpoint refs for the transaction being restored. Unlike
  soft-delete (which uses `reconciliationRefsFromTransaction` keyed on postings
  whose `reconciliation_status='reconciled'`), restore must compare each
  posting's `entry_date` against the **latest active checkpoint** for its
  `(account_id, commodity_id)`, because a soft-deleted transaction's postings may
  not themselves be marked reconciled yet still fall inside a reconciled period.
- Add a repository query, e.g.
  `LatestActiveCheckpointByAccountCommodity(ctx, bookID, accountID, commodityID)`
  reading `reconciliation_checkpoints` (filter `status='active'`,
  `ORDER BY statement_date DESC, id DESC LIMIT 1`; the
  `reconciliation_checkpoints_account_idx` index already supports this).
- If any posting's `entry_date <= latest active checkpoint.statement_date` and
  `!input.ReconciliationOverride`, return `ErrReconciliationOverrideRequired`.
- When override is given, pass the affected checkpoint refs through
  `InvalidateCheckpointRefs` (the plumbing already exists on
  `SetTransactionDeletedParams`).

Add backend tests: restore of a post-checkpoint transaction succeeds with no
override; restore of a transaction dated on/before the latest active checkpoint
fails without override and, with override, succeeds and invalidates that
checkpoint and later ones.

### 9b. Backend — list soft-deleted

Add an `include_deleted` / deleted-only listing path so the trash view can
enumerate soft-deleted transactions (the list query currently hard-excludes
`deleted_at IS NULL`; only single-record `TransactionByIDIncludingDeleted`
exists).

- `db/transactions.go` — add a `Deleted` filter mode to `ListTransactionsParams`
  (e.g. `DeletedOnly bool`) that flips the `deleted_at IS NULL` clause to
  `deleted_at IS NOT NULL` and orders by `deleted_at DESC, id DESC`. Include the
  `delete_reason`, `deleted_at`, and `deleted_by_user_id` snapshot columns in the
  response so the trash view can show when/why.
- `app/transactions.go` — surface it on `ListTransactionsInput`; still run the
  Step 1 enrichment so rows render with names. Also compute, per row, a
  `restore_blocked_by_reconciliation` boolean (using 9a's checkpoint comparison)
  so the UI can show which rows are easy-restore vs guarded **without** a
  per-row probe request.
- `api/transactions.go` + OpenAPI — expose the filter (e.g.
  `GET /api/v1/transactions?deleted=true`) and the extra response fields.
  Regenerate types.

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
cd ../frontend && npm run check
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
- `cd frontend && npm run check` passes (0 errors, 0 warnings).
- All response paths return enriched posting metadata (verified by test).
- `category_id` filter works and validates account class.
- Global list, account register, and category routes render, paginate, filter,
  and open the editor.
- Editor handles create, draft edit, posted edit (incl. reconciliation-override
  at save), correction, and the transfer entry point.
- Detail panel exposes Post/Approve/Void/Unvoid/Soft-delete/Restore correctly by
  status, each consequential action requiring a reason.
- Soft-deleted transactions never appear in the main list; voided transactions
  appear marked voided. The Settings → Trash subpage lists soft-deleted
  transactions and restores them, with the reconciliation-aware recovery guard.
- New copy goes through Paraglide; new components use Svelte 5 runes and semantic
  tokens; money is never coerced to `number`.

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
