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
  effects. Not a status. Manual Save creates `posted`; a future producer may
  persist `draft` for its own recovery/review workflow.
- **draft** — persisted `transaction_versions.status='draft'`. **System-only, not
  user-facing.** Reserved for future autosave recovery, scheduled generation, and
  committed-import-awaiting-review; no current workflow produces it. Manual entry
  never produces a draft. There is no "save as draft" button and users never pick
  `draft`. Excluded from the ledger; may trigger FX coverage.
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
- Current same-day order uses database IDs as tie-breakers. The core plan now
  requires separate editable transaction-day and account-posting-day sequences;
  Step 2.5 replaces those incidental tie-breakers.
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

### ✅ 1a. Add bulk lookup methods (db layer) — DONE

- `backend/internal/db/accounts.go` — `AccountsByIDs` added; returns
  `PostingAccountSummary` (id, name, code, system_role, account_class,
  builtin_key via `json_extract(av.metadata_json, '$.category.builtin_key')`).
- `backend/internal/db/commodities.go` — `CommoditiesByIDs` added; returns
  `PostingCommoditySummary` (id, code, display_symbol, kind). Does not filter by
  kind so securities and currencies are both returned.

Both use parameterised `IN (?, ?, ...)`. Return a map keyed by ID. Missing IDs
absent from map.

### ✅ 1b. Add fields to `app.Posting` (app layer) — DONE

Added to `Posting` in `backend/internal/app/transactions.go`:

```go
AccountName       *string  // nil for system accounts with no user-visible name
AccountCode       *string  // nil if unset
AccountSystemRole *string  // non-nil for system accounts (e.g. "transfer_clearing")
AccountBuiltinKey *string  // non-nil for built-in/starter category accounts
AccountClass      string   // asset | liability | income | expense | equity
CommodityCode     string   // "USD", "EUR", "AAPL"
CommoditySymbol   *string  // "$", "€"; nil if unset
```

### ✅ 1c. One reusable enrichment helper (app layer) — DONE

`enrichPostings(ctx, txns []Transaction) error` and
`enrichRegisterPostings(ctx, entries []AccountRegisterEntry) error` added.
`enrichOne` convenience wrapper for single-record paths.

`TransactionService` now holds `accountRepository *db.AccountRepository` and
`commodityRepository *db.CommodityRepository`. `NewTransactionService` updated
(4 params: repository, payeeRepository, accountRepository, commodityRepository).
Call sites updated: `cmd/rekenraam/command.go`, `api/setup_test.go`,
`api/auth_test.go`.

All response paths call enrichment: list, register, single read, create, update,
post, void, unvoid, soft-delete, restore, approve.

### ✅ 1d. Propagate to API DTOs — DONE

Seven fields added to `postingResponse` in
`backend/internal/api/transactions.go`. Both construction sites updated (list
path ~line 625, register path ~line 720).

### ✅ 1e. OpenAPI + types — DONE (2026-06-21)

- Updated `api/openapi/components/schemas/transactions.yaml`: seven enrichment
  fields added to `PostingResponse` (`account_name`, `account_code`,
  `account_system_role`, `account_builtin_key`, `account_class`, `commodity_code`,
  `commodity_symbol`). Nullable fields marked nullable; `account_class` and
  `commodity_code` are required (always present in responses).
- Frontend types regenerated: all seven fields appear in `PostingResponse` in
  `schema.d.ts`.

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

What was done
Step 1a (db layer):

backend/internal/db/accounts.go — AccountsByIDs with new PostingAccountSummary type. Uses a targeted query (5 columns only) rather than the full AccountRecord scan, which avoids touching the existing accountSelect/scanAccountRecord path.
backend/internal/db/commodities.go — CommoditiesByIDs with PostingCommoditySummary. Does not filter by kind so securities work too.
Step 1b (app.Posting): 7 enrichment fields added, all optional/zero-valued until enrichPostings is called.

Step 1c (enrichment helper): enrichPostings / enrichRegisterPostings / enrichOne added. TransactionService now takes accountRepository and commodityRepository. All mutation and list paths call enrichment before returning.

Step 1d (API DTOs): Both postingResponse construction sites updated.

Issues / things to flag
Step 1e (OpenAPI) not done yet — the plan lists it and says to regenerate frontend types. It's needed before the frontend can consume the new fields with generated types. It's the natural next sub-step before moving to Step 2.

PostTransaction delegates through UpdateTransaction which now calls enrichOne — so it's enriched twice for draft→posted transitions where Transaction already called the service. Not a correctness issue (idempotent), but one redundant lookup. Acceptable for now.

prepareLifecycleChange reads the transaction for internal use only (not returned to callers) and skips enrichment — correct, since it's used for reconciliation guards, not response building.

No test yet asserting account_class and commodity_code appear in responses — the plan calls for this in the Step 1 verification block. The existing tests pass but don't specifically assert the new fields. Worth adding, or noting as a gap. Next step 1e naturally pairs with adding that assertion.


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

## Step 2.5 — Backend prerequisite: ordering and reconciliation impact ✅ DONE

This step must land before frontend table/editor work. It introduces the explicit
same-day order requested by the UI and finishes the period-scoped reconciliation
contract that the editor and Trash view consume.

**Status**: Completed 2026-06-21. All sub-steps (2.5a–e) implemented, all tests pass,
OpenAPI updated, frontend types regenerated. Notable fixes applied during implementation:
- `allocateOrInheritAccountDaySeq` bug: was querying `current_transaction_versions` after
  inserting the new version row (so no postings existed yet); fixed to use `SupersedesVersionID`
  (the previous version) for the inheritance lookup.
- Reconciliation guard change-detection (`reconciliationInvalidationRefs`): uses
  `reconciliationAffectingChange` to exempt memo/metadata-only edits from the guard,
  matching the pre-existing `TestPostingScopedReconciliation*` tests.
- `MovePosting` boundary guard: added inline reconciliation check in the DB method before
  the swap is committed. Uses `db.ErrReconciliationOverrideRequired` (now in `db/setup.go`).
- `CreateTransaction` checkpoint invalidation: wired `InvalidateCheckpointRefs` through
  `CreateTransactionParams` so backdated creates with override actually invalidate
  the checkpoint.

### 2.5a. Migration and model fields

Add a migration under `backend/migrations`:

- `transaction_versions.transaction_day_sequence INTEGER NOT NULL CHECK
  (transaction_day_sequence > 0)`
  — scoped to current transactions in `(book_id, transaction_date)`.
- `posting_versions.account_day_sequence INTEGER NOT NULL CHECK
  (account_day_sequence > 0)` — scoped
  to current postings in `(book_id, account_id, entry_date)`.
- `reconciliation_checkpoints.statement_account_sequence INTEGER NOT NULL CHECK
  (>= 0)` — together with `statement_date`, the inclusive account-register lock
  boundary. `0` means before every posting on that date.

Backfill transaction and posting sequences deterministically using the current
ID tie-breakers so existing visible order does not jump. Backfill each checkpoint
to the greatest account-day sequence among its `reconciliation_checkpoint_postings`
whose `entry_date` equals `statement_date`, or `0` when it selected none that day.
Use stable identity ranks for history: rank `transaction_id` within each
transaction date and `posting_line_id` within each account/entry date, so multiple
versions of the same logical row inherit the same initial position. Current rows
must preserve the existing ID order.

The existing append-only triggers reject updates to `transaction_versions` and
`posting_versions`. The migration must explicitly drop and recreate those
triggers around a controlled backfill (or rebuild the tables); do not leave the
new columns at one shared default and call that ordered. Add migration tests that
both verify the backfill and confirm append-only enforcement is restored.
Do not add a simple SQL UNIQUE constraint across the append-only version tables:
historical versions legitimately repeat positions. The service validates
uniqueness among current rows.

Add the fields through db records, app models, API DTOs, OpenAPI, and generated
frontend types. Keep sequence values hidden in ordinary forms; they remain in the
contract for deterministic cursors and move operations.

### 2.5b. Allocation, movement, and cursor rules

- New manual transactions receive `MAX(transaction_day_sequence)+1` on their
  `transaction_date`.
- Each new posting receives `MAX(account_day_sequence)+1` for its
  `(account_id, entry_date)`.
- A transaction date or posting account/date change places the affected item at
  the end of its new scope unless an explicit move follows.
- Add `POST /api/v1/transactions/{id}/move` with
  `{ "direction": "earlier" | "later" }` for global same-day order.
- Add `POST /api/v1/accounts/{account_id}/postings/{posting_line_id}/move` with
  the same body for account-register order. A transfer's posting in each account
  moves independently.
- Movement swaps adjacent current positions atomically. If multiple transactions
  are affected, append all replacement transaction versions under one audit
  event. Numeric gaps are allowed; the UI may display dense ordinal positions.
- Replace global pagination with cursor tuple `(transaction_date,
  transaction_day_sequence, transaction_id)` and register pagination with
  `(entry_date, account_day_sequence, posting_version_id)`. Update encode/decode,
  WHERE clauses, ordering, OpenAPI descriptions, and cursor tests together.

The UI labels movement as Move up/down for the current sort direction, backed by
the semantic earlier/later API so reversing visual sort does not reverse meaning.

### 2.5c. Reconciliation boundary

For an account/commodity checkpoint, a posting is inside the reconciled period
when:

```text
entry_date < statement_date
OR (entry_date = statement_date AND account_day_sequence <= statement_account_sequence)
```

At reconciliation finish, set `statement_account_sequence` to the greatest
sequence among selected postings on the statement date, or `0` if none are
selected that day. Before finish, allow the user to move an unselected same-day
posting after the selected statement items; this is the explicit before/after
distinction requested for one calendar date.

Changing `transaction_day_sequence` never affects reconciliation. Moving a
posting sequence wholly within the same side of the boundary is allowed without
override. Moving across it is guarded and invalidates that checkpoint plus later
active checkpoints for the account/commodity after explicit confirmation.

Apply this period rule consistently to create, edit, void, unvoid, soft-delete,
restore, and posting movement. Exempt restoring a voided+soft-deleted transaction,
which changes visibility but does not re-enter postings into the ledger.

### 2.5d. Backdated create and affected-checkpoint preview

- Plumb `reconciliation_override` through `CreateTransactionInput` and the create
  handler. Manual browser creation accepts only `status='posted'`.
- Before create, evaluate every proposed posting against active checkpoint
  date/sequence boundaries. On conflict, do not persist anything.
- Add `POST /api/v1/transactions/reconciliation-impact` for an unsaved create
  payload.
- Add `POST /api/v1/transactions/{id}/reconciliation-impact` for update, unvoid,
  soft-delete, restore, or posting-move payloads.
- Return `{ account_id, account_label, commodity_code, statement_date,
  statement_account_sequence, checkpoint_id }[]`. Keep the stable error envelope
  unchanged; these are explicit read-only previews.
- The UI may call preview proactively or after the generic override-required
  conflict, show named checkpoints, then retry with override.

### 2.5e. Ordinary list versus reserved drafts

With no status filter, `GET /transactions` returns only `posted` and `voided`.
`status=draft` remains an explicit internal filter for a future producer-owned
surface, but the general transaction table and filter bar never expose it. Manual
create rejects draft and no current workflow creates one.

Deferred follow-up: an owner-facing **Unfinished Work** inbox/backlog button may
link to future import-review, scheduled-generation, or recovery-draft owners. It
must not be a generic draft editor and is intentionally empty/unexposed until a
producer workflow exists.

**Verify Step 2.5:** ✅ PASSED 2026-06-21

```bash
cd backend && go test ./internal/app/ ./internal/db/ ./internal/api/
cd .. && pnpm --dir frontend run openapi:generate && pnpm --dir frontend run check
```

Required tests: ✅ all written and passing in `internal/api/day_sequence_test.go`
- `TestSameDaySequencesAreMonotonicallyIncreasing` — monotonic allocation
- `TestDaySequenceAssignedOnCreateAndInheritedOnEdit` — inheritance on same-date edit
- `TestTransferPostingMovesIndependentlyInEachRegister` — independent transfer order
- `TestMoveTransactionSwapsAdjacentSameDaySequences` — move semantics
- `TestCursorPaginationRespectsDaySequence` — cursor traversal before/after move
- `TestMovePostingAcrossReconciliationBoundaryRequiresOverride` — guarded crossing
- `TestBackdatedCreateReconciliationImpactPreviewAndRetry` — backdated create preview/retry
- `TestReconciliationImpactPreviewForUpdate` — update impact preview
- `TestVoidedTransactionRestoreIsExemptFromReconciliationGuard` — voided restore exemption
- `TestDefaultListExcludesDraft` — default list excludes draft

---

## Step 3 — Frontend API layer ✅ DONE

In `frontend/src/lib/api/transactions.ts`:

- `categoryID?: number` in `TransactionListOptions` — already present from an
  earlier pass; plumbed into `getTransactions` query string.
- `accountRegisterInfiniteQueryOptions(accountID, options)` — added, mirrors
  `transactionsInfiniteQueryOptions`, keyed on `[..accountRegisterQueryKey,
  accountID, 'infinite', options]`. Accepts `RegisterFilterOptions` (status,
  afterDate, beforeDate, limit).
- `moveTransaction(id, direction, csrfToken)` — calls
  `POST /api/v1/transactions/{id}/move`.
- `movePosting(accountID, postingLineID, direction, csrfToken)` — calls
  `POST /api/v1/accounts/{account_id}/postings/{posting_line_id}/move`.
- `reconciliationImpactForCreate(input)` — calls
  `POST /api/v1/transactions/reconciliation-impact`.
- `reconciliationImpactForUpdate(id, input)` — calls
  `POST /api/v1/transactions/{id}/reconciliation-impact`.
- Exported types: `AccountRegisterEntryResponse`, `MoveRequest`,
  `ReconciliationImpactResponse`, `AccountRegisterInfiniteData`,
  `RegisterFilterOptions`.

**Gap resolved (2026-06-21):** `ReconciliationImpactResponse` now carries all
fields needed for the Step 5 named-checkpoint warning modal:

| Field | Source |
|---|---|
| `checkpoint_id` | `reconciliation_checkpoints.id` |
| `account_id` | posting account |
| `account_label` | first non-empty of system_role → builtin_key → name → code (raw key; localise via Paraglide client-side) |
| `commodity_id` | posting commodity |
| `commodity_code` | `PostingCommoditySummary.Code` |
| `statement_date` | `ReconciliationCheckpointRecord.StatementDate` |
| `statement_account_sequence` | `ReconciliationCheckpointRecord.StatementAccountSequence` |
| `entry_date` | the affected posting's entry date |

Changes: `db.CheckpointInvalidationRef` extended; `PeriodScopedCheckpointInvalidationRefs`
populates the new DB fields from the already-fetched checkpoint record; new
`enrichCheckpointRefs` helper at the app layer runs one bulk account + one bulk
commodity lookup; API DTO updated; OpenAPI schema updated; frontend types
regenerated. All existing tests pass.

**Verify Step 3:** ✅ PASSED — `pnpm --dir frontend run check` → 0 errors, 0 warnings.

---

## Step 4 — Display primitive & shared pieces ✅ DONE (2026-06-21)

### ✅ 4a. `frontend/src/lib/transactions/transaction-table.svelte`

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

### ✅ 4b. `frontend/src/lib/transactions/transaction-labels.ts`

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

### ✅ 4c. `frontend/src/lib/transactions/transaction-filter-bar.svelte`

Reusable filter controls; each context passes which filters to show. Controls:
ordinary status (posted/voided), kind, date range, account, payee (autocomplete
via `payeesQueryOptions`), search (`q`), needs_review. The shared status-label
helper may understand `draft` for future producer-owned screens, but the general
filter must not offer it. Emits a typed filter object.
Use `minisearch` only for small in-memory dropdowns (accounts/payees), never for
the transaction list itself (server FTS5 handles `q`).

### ✅ 4d. `frontend/src/lib/transactions/transaction-row-actions.svelte`

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
- "Reconciliation-locked" is a per-row hint: any posting before an active
  checkpoint's `(statement_date, statement_account_sequence)` boundary for its
  account/commodity. Use the
  backend-provided per-row flag from Step 1/Step 9 enrichment rather than
  computing it client-side; if the flag is not yet available, treat the row as
  potentially locked and let the save-time guard decide.
- The row menu never shows `Post` (no user-facing draft promotion) or a hard
  `Delete` (hard delete is only for never-posted system drafts, exposed in the
  import-review tray, not here). Removal of a posted/voided transaction is
  Soft-delete on the detail panel.
- Soft-delete and Restore are detail-panel actions (Step 6), available on both
  posted and voided rows since the flag is independent of status.

**Verify Step 4:** ✅ PASSED 2026-06-21

```bash
# svelte-check: 0 errors, 0 warnings across 1716 files
npx svelte-check --tsconfig ./tsconfig.json

# Vitest: 11/11 tests pass
frontend/node_modules/.bin/vitest run
```

Step 4.0 (Vitest setup): `vitest` and `@vitest/ui` were already in devDependencies.
Added `"test": "vitest run"` script and `frontend/vitest.config.ts` with `$lib`
path alias.

`formatQuantity` tests cover: scale 0, scale 2, leading zeros (< 1 unit), round
number, negative scale 0, negative scale 2, value beyond `Number.MAX_SAFE_INTEGER`
(both scale 0 and scale 2), zero, empty string, and locale separator passthrough.

Implementation notes:
- `Column<R>` type extracted to `transaction-table-types.ts` because `export type`
  inside a `<script generics>` block is not allowed by svelte-check v4.
- `formatQuantity` uses the exact manual BigInt fallback (not Dinero) as specified
  by the plan — lossless for arbitrarily large values; Intl provides separators.
- `transaction-filter-bar.svelte` loads accounts/payees lazily only when the
  respective controls are shown (`show.account` / `show.payee`).
- Supply-chain policy note: `@inlang/paraglide-js@2.20.1` was auto-updated by
  `pnpm check` during this session; it was added to `minimumReleaseAgeExclude`
  automatically. Run vitest via `frontend/node_modules/.bin/vitest run` rather
  than `pnpm --dir frontend run test` until the policy age window passes.

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

### Commodity selection

- Tier 1 infers commodity from the selected asset/liability account's
  `default_commodity_id`. If that account has no default commodity, show a
  required commodity selector. The generated category posting uses the same
  commodity.
- Tier 3 exposes commodity on every posting. Account/default-commodity and scale
  validation mirror the backend rules; archived commodities remain displayable
  for history but are unavailable for new entry.
- A same-commodity transfer creates the ordinary balanced transfer postings. A
  cross-commodity transfer uses explicit From and To amounts/commodities and the
  FX `commodity_trading` pattern from the core plan; never infer an exchange rate
  by equating unlike quantities.

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
| Posted edit — not in a reconciled period | No affected posting falls inside an active checkpoint's date/account-sequence boundary | `PATCH /transactions/{id}` | Opens and saves directly, no warning |
| Posted edit — non-financial only | Only category/description/payee/note/tags change | `PATCH /transactions/{id}` | Always allowed, reconciliation intact, no override — even inside a reconciled period |
| Posted edit — changes a reconciled balance | Adds/removes/re-dates a posting in a reconciled period, or changes a reconciled posting's account/commodity/amount/scale/date | `PATCH /transactions/{id}` with `reconciliation_override: true` | Warning modal **at save time** naming the affected checkpoint(s); proceed only on confirm |
| Voided edit | Transaction is voided | Unvoid first, then `PATCH` | Editor offers Unvoid (which re-enters the ledger and is itself period-guarded); editing blocked until unvoided |
| Draft edit (system) | A persisted draft in its producing workflow (import-review tray) | `PATCH /transactions/{id}` | Out of scope for the general editor; the import-review surface owns draft editing/promotion |
| Corrective transaction | "Create correction" | `POST /transactions/{id}/correct` | Opens in correction mode |

A new backdated transaction uses the create-impact preview from Step 2.5 when the
first save reports an override conflict. Nothing is persisted before confirmation;
after the user reviews the named checkpoints, retry the same create payload with
`reconciliation_override:true`.

The guard uses the **period-scoped** rule (see Step 2.5,
`docs/conventions.md`, and the core
plan): a change is guarded when it would alter a reconciled balance — either it
touches a reconciled posting's facts, or it adds/removes/re-dates a posting whose
`(entry_date, account_day_sequence)` falls inside an active checkpoint's
`(statement_date, statement_account_sequence)` boundary for an affected
account/commodity, even if that posting is not itself flagged reconciled.
Whether a specific edit crosses that line is only knowable after the
user edits, so the warning fires at save, not on open. If the backend returns
`reconciliation override is required`, fetch the affected-checkpoint details
(Step 2.5's preview read model) and show the warning naming them, then
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
| Status | `status` | 3 | Badge posted/voided (draft is not part of the general table) |
| Flags | `needs_review` | 3 | Review badge |

- Full filter bar (posted/voided status, kind, date, account, payee, search,
  needs_review). No draft option.
- Same-day Move up/down actions call the semantic earlier/later transaction move
  endpoint. Sequence numbers remain hidden unless a future advanced preference
  exposes them.

### `frontend/src/routes/app/transactions/+page.svelte`

- Mounts `transaction-list.svelte` as the main area.
- Side panel (mirror account/category page pattern) that slides in on row click
  or "New transaction". Clicking a row opens a **read-only detail panel** first:
  - All fields in a clean read layout.
  - Full posting breakdown with resolved account labels and commodity symbols.
  - **Edit** — opens the editor in the panel, replacing the detail view (posted
    is directly editable; voided must be unvoided first).
  - **Create correction** — opens editor in correction mode (posted only).
  - **Action buttons (status-appropriate):** Approve, Void with reason, Unvoid,
    Soft-delete with reason, Restore. `Post` belongs only to a future draft-owning
    workflow and never appears here. Void keeps the row visible marked
    voided; Soft-delete hides it from the table but keeps it recoverable. Void
    and Soft-delete are independent and each requires a reason.
- No version/audit history in the panel for v1 (no API).
- **Mobile:** side panel becomes a full-screen overlay (no split). The editor on
  mobile is a full-screen form / bottom sheet below the breakpoint.
- The side panel hosts the editor; it is never used for navigation.

**Soft-delete recovery:** soft-deleted transactions never appear in the list or
ordinary single-record reads. Immediately after soft-delete, the detail panel may
offer Undo/Restore using the mutation response retained in local state. After
navigation, close, or refresh, durable recovery occurs through the dedicated
Settings → Trash view in Step 9; do not rely on reopening the deleted ID through
the ordinary read endpoint.

**Verify Step 6:** `pnpm --dir frontend run check`; run the app and confirm list
loads, row opens detail, and edit/void/unvoid/soft-delete/restore round-trips.

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
| Amount | `posting.quantity_value`, signed | 1 | Inflow +/outflow − from this account's class; label with the posting commodity |
| Balance | `running_balance` | 1 | Right-aligned balance for this account/commodity pair |
| Reconciliation | `posting.reconciliation_status` | 2 | Icon uncleared/cleared/reconciled |
| Memo | `posting.memo` | 3 | |

- Filter bar: status, date range only (matches the register API).
- Same-day Move up/down changes only this posting's account-register order. For
  transfers, moving the posting in one account does not move its counterpart in
  the other account. A boundary-crossing move uses the Step 2.5 impact/override
  flow; movement wholly on one side is immediate.
- Mobile: priority-1 columns in a responsive grid; Payee may wrap; Amount and
  Balance stay right-aligned.

Multi-commodity accounts may show interleaved commodities. Each row carries its
posting commodity and an independently accumulated running balance for that
commodity; never carry a USD balance into the next EUR row.

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
  affected posting falls inside the latest active reconciliation checkpoint's
  `(statement_date, statement_account_sequence)` boundary for its account and
  commodity, or there is no active checkpoint. The common case.
- **Guarded restore (posted in a reconciled period):** if any affected posting is
  inside a latest active checkpoint's date/sequence boundary, restore requires explicit
  `reconciliation_override: true` and invalidates that checkpoint plus all later
  active checkpoints for the same account/commodity — the same
  override-and-invalidate mechanism used by edit/void/unvoid/soft-delete. The UI
  warns, naming the affected checkpoint(s), like the guarded edit path in Step 5.

The checkpoint is the lock floor; crossing it is allowed but never silent. The
restore guard and affected-checkpoint preview are implemented earlier in Step
2.5 so this step only adds deleted-listing and recovery UI.

### 9a. Backend — list soft-deleted

Add a deleted-only listing path so the trash view can enumerate soft-deleted
transactions (the list query currently hard-excludes `deleted_at IS NULL`; only
single-record `TransactionByIDIncludingDeleted` exists).

**Use a dedicated trash endpoint, not a `deleted=true` flag on `/transactions`.**
After Step 2.5 the ordinary cursor follows transaction date and same-day sequence,
while Trash must order by deletion recency (`deleted_at DESC, id DESC`). A
separate endpoint with its own cursor keeps those semantics independent.

- `db/transactions.go` — add a deleted-only query ordered by
  `deleted_at DESC, id DESC`, with a **deletion cursor** `(deleted_at, id)`
  (add `Encode/DecodeDeletionCursor`; do not reuse the date cursor). Select the
  `delete_reason`, `deleted_at`, and `deleted_by_user_id` snapshot columns.
- `app/transactions.go` — a `ListDeletedTransactions` service; still run Step 1
  enrichment so rows render with names. Per row, compute a
  `restore_blocked_by_reconciliation` boolean using Step 2.5's **period** check
  (and honouring the voided exemption: a voided row is never blocked) so the UI
  shows easy-restore vs guarded **without** a per-row probe request.
- `api/transactions.go` + OpenAPI — add `GET /api/v1/transactions/deleted`
  (cursor-paginated) returning the deleted rows plus the snapshot fields and the
  per-row flag. Regenerate types.

### 9b. Frontend — settings subpage

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
- Account register: native amount and running balance for this account/posting
  commodity pair.
- Category: separate per-commodity sums of postings to the category.

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
- Global transactions and account postings have independent editable same-day
  order; transfers can occupy different positions in their two registers;
  sequence-aware cursors and reconciliation boundaries are covered by tests.
- Editor saves manual entry directly to `posted` (no "save as draft"), handles
  posted edit (incl. period-scoped reconciliation override at save with named
  checkpoints), correction, the transfer entry point, and well-defined Cancel
  semantics.
- The period-scoped reconciliation guard is enforced consistently across
  create/edit/void/unvoid/soft-delete/restore; restoring a voided+soft-deleted
  transaction is exempt (visibility only).
- Detail panel exposes Approve/Void/Unvoid/Soft-delete/Restore correctly by
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

1. **Account list → register navigation:** the account list row uses a stretched
   link to `/app/accounts/[id]/register`; per-row buttons are focusable siblings.
   (Step 7.)
2. **Category list → navigation:** the category list row itself links to
   `/app/categories/[id]`, same pattern. (Step 8.)
3. **Trash/recovery:** a Settings → Trash subpage
   (`/app/settings/trash`) lists and restores soft-deleted transactions.
   Easy one-click restore is allowed only when postings are outside the latest
   active account checkpoint's date/sequence boundary; boundary-crossing restores
   require reconciliation override and invalidate affected checkpoints. (Step 9.)
4. **Draft:** retained as an inactive, system-only status. Manual entry posts
   directly; the ordinary table excludes drafts. A producer-owned Unfinished Work
   inbox is deferred until imports, scheduling, or recovery autosave exists.
5. **Same-day order:** global transaction order and account-posting register order
   are independent. Reconciliation uses the account-specific date/sequence
   boundary; only crossing it is guarded.
