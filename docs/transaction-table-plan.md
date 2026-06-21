# Transaction Table Implementation Plan

## Challenge: Is a "Customizable Component" the Right Model?

The instinct to build one configurable `TransactionTable` with mode switches for account/category/stock contexts is appealing but worth scrutinising before committing to it.

**What actually varies across contexts:**

| Context | API endpoint | Row unit | Unique columns | Filters available |
|---|---|---|---|---|
| Global transactions | `GET /transactions` | Transaction | — | All (status, kind, account, payee, search, date) |
| Account register | `GET /accounts/{id}/register` | Posting (one per account leg) | Running balance | Status, date only |
| Category view | `GET /transactions?category_id=X` | Transaction | Category total (optional) | Status, date, search |
| Commodity/stock view | `GET /transactions?account_id=X` (commodity account) | Posting | Quantity + commodity | Status, date |

The data shapes differ fundamentally: the account register returns a flat posting-per-row with a running balance. The transaction list returns a transaction-per-row with all postings nested. Merging these into one row model for a single component either forces an awkward normalisation step or a large `if (mode)` branch tree in the template.

**The better model: composition over configuration.**

Build one dumb display primitive (`TransactionTable`) that accepts column definitions and row data via typed props and Svelte 5 snippets. Each context gets a thin wrapper component (~80–100 lines) that owns: the query, the column set, and any context-specific row normalisation. The primitive stays ignorant of data sources.

This means:

- Adding a new context (stocks) is writing a new wrapper, not modifying the shared primitive.
- Each wrapper can evolve its column set independently.
- The primitive can still share sorting headers, infinite scroll, loading states, and row actions.

---

## Architecture

### Layer 1: Display Primitive — `transaction-table.svelte`

Accepts:

```typescript
type Column<R> = {
  key: string;
  header: string;
  width?: string;
  align?: 'left' | 'right';
  priority?: number;  // lower = hidden first on narrow viewports
  cell: Snippet<[R]>;  // Svelte 5 snippet
};

let {
  rows,         // R[] — generic row type
  columns,      // Column<R>[]
  isLoading,
  hasNextPage,
  onLoadMore,   // called by intersection observer at bottom; falls back to a "Load more" button if observer unavailable
  onRowClick,   // optional: (row: R) => void
  onError,      // optional: (err: Error) => void — surfaces retry UI when set
}: Props<R>
```

Handles: intersection observer for infinite scroll, loading skeleton rows, empty state, error/retry state, sticky header. Does not know about transactions.

**Keyboard interaction:** the primitive supports arrow-key row navigation and Enter to activate `onRowClick`. Tab moves focus to row action buttons within the focused row.

### Layer 2: Context Wrappers

#### `transaction-list.svelte` — Global & side panel

- Uses `createInfiniteQuery(transactionsInfiniteQueryOptions(filters))`
- Columns: Date, Payee/Description, Accounts involved (summarised), Amount, Status, Flags
- Full filter bar: status, kind, date range, account, search, needs_review

#### `account-register.svelte` — Account drill-down route

- Uses `createInfiniteQuery(accountRegisterInfiniteQueryOptions(accountID, filters))`
- Columns: Date, Payee/Description, Memo, Reconciliation, Amount, Running Balance
- Filter bar: status, date range only (matches what the API supports)
- Normalises `AccountRegisterEntryResponse` into display rows

#### `category-transactions.svelte` — Category drill-down route

- Uses `createInfiniteQuery(transactionsInfiniteQueryOptions({ categoryID, ...filters }))`
- Same columns as global list, minus the account column (implicit)
- Filter bar: status, date range, search

Future contexts (commodity/stock view) follow the same pattern.

### Layer 3: Route & Navigation

```
/app/transactions              ← full global page with side panel for editing
/app/accounts/[id]             ← account register (dedicated route)
/app/categories/[id]           ← category transactions (dedicated route)
```

**Why dedicated routes for account/category, not a drawer?**

The account register is the primary daily interaction in a personal finance app — it is not a drill-down secondary view. A user will navigate there repeatedly and want browser Back/Forward to work. A URL also makes the view shareable/bookmarkable. The global transactions view uses a side panel because it is already the "list" — the panel there is for editing, not navigation.

The side panel (global transactions page) remains the right pattern for the transaction editor — consistent with how account and category editors work today.

---

## Backend Changes Required

### 1. Enrich posting and register responses with display metadata

`postingResponse` currently carries only IDs for account and commodity (`app.Posting` has `AccountID` and `CommodityID`, and `toTransactionResponse` maps these directly with no name resolution). The frontend cannot resolve names without fan-out requests or a warm client-side cache — both fragile.

**The enrichment must not be done per-posting.** The implementation must bulk-fetch all account and commodity records needed for a page in one query each (keyed by the union of IDs across all postings in the result set), then join in memory before constructing responses. One lookup per page, not one per posting.

**Add to `postingResponse`:**
```go
AccountName       *string `json:"account_name"`        // null for system accounts that have no user-visible name
AccountCode       *string `json:"account_code"`        // user-assigned account code, if set
AccountSystemRole *string `json:"account_system_role"` // non-null for built-in system accounts (e.g. "transfer_clearing")
AccountBuiltinKey *string `json:"account_builtin_key"` // non-null for built-in/starter category accounts
AccountClass      string  `json:"account_class"`       // asset | liability | income | expense | equity
CommodityCode     string  `json:"commodity_code"`      // e.g. "USD", "EUR", "AAPL"
CommoditySymbol   *string `json:"commodity_symbol"`    // e.g. "$", "€" — null if not set
```

System accounts (`account_system_role` non-null) have no `account_name`. Built-in and starter category accounts may also omit `account_name`; their localized label comes from `account_builtin_key`. The frontend resolves labels in this order: localized `account_system_role`, localized `account_builtin_key`, user-entered `account_name`, then `account_code` as a defensive fallback. It must not render a null name directly.

The same enrichment applies to `accountRegisterEntryResponse` posting rows.

Because `postingResponse` is shared by list, detail, and mutation responses, enrichment must cover every path that returns a transaction: list, account register, single read, create, update, post, void, approve, and correct. Implement one reusable application-service enrichment helper that collects the union of account and commodity IDs for the returned transaction or page, performs one bulk account query and one bulk commodity query, and joins the results in memory. Mutation and single-read paths call the same helper for their one returned transaction. No response path may emit partially populated display metadata.

**Version history for the detail panel is deferred.** The plan previously mentioned "status history / version info" in the detail panel. No corresponding API exists, and building one is out of scope. The detail panel will show current state only; a version history view is a future feature.

**Changes:**
- `db/accounts.go` / `db/commodities.go` — add bulk lookup methods by ID for one book; account lookup must expose category `builtin_key` metadata as well as system role, code, name, and class
- `app/transactions.go` — add one reusable enrichment helper used by list, register, single-read, and every mutation response; add `AccountName`, `AccountCode`, `AccountSystemRole`, `AccountBuiltinKey`, `AccountClass`, `CommodityCode`, `CommoditySymbol` to `app.Posting`
- `api/transactions.go` — propagate new fields into `postingResponse` and `accountRegisterEntryResponse`
- `api/schema.yaml` — reflect new fields
- Regenerate `schema.d.ts`

### 2. Category filtering on transactions (~30 lines)

`GET /api/v1/transactions` has no `category_id` param. Categories are income/expense accounts; `account_id` already filters by account using an EXISTS subquery against `posting_versions`. `category_id` uses the same EXISTS pattern but additionally validates that the resolved account has class `income` or `expense`, so callers get a clear error if they pass an asset account ID by mistake.

`category_id` and `account_id` compose with AND when both are supplied — they are not an OR branch.

**Changes:**
- `db/transactions.go` — add `CategoryID int64` to `ListTransactionsParams`; add EXISTS clause (same pattern as `AccountID`)
- `app/transactions.go` — add `CategoryID int64` to `ListTransactionsInput`; validate account class; pass through
- `api/transactions.go` — parse `category_id` query param in `readTransactionListInput`
- `api/schema.yaml` — add `category_id` query param
- Regenerate `schema.d.ts` and add `categoryID` to `TransactionListOptions` in `transactions.ts`

### 3. Sort direction (~20 lines, optional for v1)

Current sort is always `transaction_date DESC, id DESC`. For the global transactions page it may be useful to flip to ASC. Cursor logic already uses `(date, id)` tuple — just invert the comparison.

Add `SortAsc bool` to `ListTransactionsParams`. When true: `ORDER BY transaction_date ASC, id ASC` and cursor flips to `date > ? OR (date = ? AND id > ?)`. This is the only sorting worth supporting without moving to offset pagination.

**Decision: defer to v2.** The register is always most-recent-first. The global page starting most-recent-first is reasonable. Sort direction can be added later without breaking the cursor format.

### 4. Account register infinite query options (~15 lines)

Add `accountRegisterInfiniteQueryOptions` to `transactions.ts` following the same pattern as `transactionsInfiniteQueryOptions`. The `getAccountRegister` function already exists.

---

## Frontend File Plan

```
frontend/src/
  lib/
    transactions/
      transaction-table.svelte          ← dumb display primitive
      transaction-row-actions.svelte    ← post/void/approve/correct/delete menu
      transaction-editor.svelte         ← create/edit form (largest piece)
      transaction-filter-bar.svelte     ← reusable filter controls
      transaction-list.svelte           ← global list wrapper (uses /transactions)
      account-register.svelte           ← account register wrapper
      category-transactions.svelte      ← category wrapper
      transaction-labels.ts             ← display helpers (status labels, amount formatter, system account labels)
    api/
      transactions.ts                   ← add accountRegisterInfiniteQueryOptions
                                           add categoryID to TransactionListOptions
  routes/
    app/
      transactions/
        +page.svelte                    ← mounts TransactionList with side panel
      accounts/
        [id]/
          +page.svelte                  ← mounts AccountRegister
      categories/
        [id]/
          +page.svelte                  ← mounts CategoryTransactions
```

---

## Column Definitions by Context

### Global transaction list

The "primary posting" for display is the first asset or liability leg (`account_class = asset` or `liability`). For FX transfers, this leg's amount is in a single commodity — showing both legs would require two Amount columns, which is out of scope for v1. For income/expense-only transactions (unusual), fall back to the first posting. Account name and commodity symbol are available directly from the enriched `postingResponse` fields.

| Column | Source | Priority | Notes |
|---|---|---|---|
| Date | `transaction_date` | 1 (always shown) | |
| Payee / Description | `payee_name` or `description` | 1 (always shown) | |
| Amount | Primary posting, formatted | 1 (always shown) | See Amount Display Semantics |
| Account(s) | Primary posting `account_name` / `account_system_role` | 2 | Collapse to "N accounts" when more than two |
| Status | `status` | 3 | Badge: draft/posted/voided |
| Flags | `needs_review` | 3 | Review badge |

### Account register

| Column | Source | Priority | Notes |
|---|---|---|---|
| Date | `entry_date` | 1 (always shown) | Journal entry date, not transaction date |
| Payee / Description | `payee_name` or `description` | 1 (always shown) | |
| Amount | `posting.quantity_value` | 1 (always shown) | Inflow positive, outflow negative from this account's perspective |
| Balance | `running_balance` | 1 (always shown) | Right-aligned, in account's commodity |
| Reconciliation | `posting.reconciliation_status` | 2 | Icon: uncleared/cleared/reconciled |
| Memo | `posting.memo` | 3 | |

### Category transactions

The category amount is the **sum of all postings to the selected category account** within the transaction. A split transaction may have multiple postings to the same category; these are summed exactly. To add values with different scales, rescale each integer coefficient to the greatest scale present using `BigInt`, sum the rescaled coefficients, and retain that common scale for formatting. Never sum JavaScript numbers or already-formatted decimal strings.

Category views use a category-activity convention rather than cashflow wording: normal expense activity (a debit to an expense category) and normal income activity (a credit to an income category) both display as positive. Reversals display as negative. The counterpart column lists localized labels for non-category postings, collapsed to "N accounts" when more than one.

| Column | Source | Priority | Notes |
|---|---|---|---|
| Date | `transaction_date` | 1 (always shown) | |
| Payee / Description | `payee_name` or `description` | 1 (always shown) | |
| Amount | Sum of postings to selected category account | 1 (always shown) | |
| Counterpart | Non-category posting `account_name`(s) | 2 | |
| Status | `status` | 3 | |
| Flags | `needs_review` | 3 | |

---

## Amount Display Semantics

### Exact formatting

`quantity_value` is serialized as a JSON string (not a number) to preserve lossless precision across Go's and JavaScript's integer ranges. The frontend must **never convert it to a JavaScript `number`** — doing so silently truncates values beyond 53-bit precision (possible for crypto amounts with large scales).

`transaction-labels.ts` must export a `formatQuantity(value: string, scale: number): string` function that:
1. Inserts the decimal point by string index arithmetic (e.g. `"12345"` at scale 2 → `"123.45"`), using `BigInt` if arithmetic is needed.
2. Passes the resulting decimal string to `Intl.NumberFormat` with `{ minimumFractionDigits: scale, maximumFractionDigits: scale }` for locale-aware display.
3. Never coerces the coefficient string to `number` at any step.

The commodity symbol from `postingResponse.commodity_symbol` is appended/prepended as appropriate for the commodity's locale convention. When `commodity_symbol` is null, fall back to `commodity_code` as a suffix.

### User-facing sign convention

The user-facing amount is always expressed as **inflow/outflow from the account's perspective**: inflow is positive, outflow is negative. This applies in both the table and the simple editor.

This differs from the ledger's internal debit/credit convention. The translation is deterministic by account class:

| Account class | Debit posting | Credit posting |
|---|---|---|
| Asset | Inflow (+) | Outflow (−) |
| Liability | Outflow (−) | Inflow (+) |
| Income | Outflow (−) | Inflow (+) |
| Expense | Inflow (+) | Outflow (−) |

In the global list, the sign convention applies to the primary posting's account class. In the account register, it always applies to this account's class (already known from context). Category views instead use the category-activity convention defined above so ordinary expense and income activity are both positive and reversals are negative.

For the simple editor, the Amount field is always labelled relative to the selected **Account** field (the asset/liability account): positive means money arrived in that account, negative means it left. The backend posting signs are derived from the account class at save time, not stored from the field value directly.

**Transfers are a distinct workflow.** Tier 1 assumes one asset/liability account and one category (income/expense) account, and the Category control only offers income/expense accounts. The editor provides an explicit "Transfer" entry point beside transaction creation; choosing it opens Tier 3 in transfer mode (or a dedicated transfer form) with From account and To account controls. Transfer selection is never inferred from an invalid Category choice.

### FX and multi-commodity transactions

For v1, each context shows the **native amount of the relevant posting** only — no synthetic base-currency equivalent:

- Global list: native amount of the primary asset/liability posting, with commodity symbol
- Account register: native amount of this account's posting, in the account's commodity
- Category transactions: sum of postings to the selected category, in the category's commodity

A synthetic `base_currency_value` enrichment (summing asset/liability legs converted to book currency) is **deferred to a later step**. Summing absolute values of asset/liability postings double-counts same-currency transfers (two asset legs in a transfer), and the semantics become ambiguous for split transactions and FX pairs. The correct definition depends on transaction kind and requires per-kind rules before it can be implemented safely.

---

## Row Actions

Actions are context-sensitive based on transaction status:

| Status | Available actions |
|---|---|
| draft | Edit, Post, Delete |
| posted | Edit, Create correction |
| voided | View only |
| any + needs_review | Approve (inline, no editor needed) |

Void is an action on the **detail panel**, not the row action menu — it requires a reason and is consequential enough to warrant the extra step.

### Edit/correction lifecycle

The backend (`app/transactions.go`) permits `PATCH` on any non-voided transaction. The UI must distinguish four cases and route accordingly:

| Case | Trigger | API call | UI behaviour |
|---|---|---|---|
| Draft edit | Transaction is draft | `PATCH /api/v1/transactions/{id}` | Editor opens directly, no warning |
| Posted edit — safe | No reconciled postings are affected by the change | `PATCH /api/v1/transactions/{id}` | Editor opens directly, no warning |
| Posted edit — reconciliation-invalidating | Change affects quantity, account, commodity, or date of a reconciled posting | `PATCH /api/v1/transactions/{id}` with `reconciliation_override: true` | Warning modal at save time: "This will invalidate a reconciliation checkpoint. Continue?" |
| Corrective transaction | User chooses "Create correction" | `POST /api/v1/transactions/{id}/correct` | Editor opens in correction mode |

Cases 2 and 3 are only distinguishable after the user makes edits, so the reconciliation warning fires at save time, not when the editor opens.

**"Create correction"** creates a new adjusting transaction linked to the original via `correction_of_transaction_id`. It does not void or replace the original — both transactions exist in the ledger and the net effect of both represents the corrected state. The action label and editor UI must make this clear: the correction is an *additional* transaction, not an edit of the existing one.

Reconciliation override is a destructive acknowledgement, not a routine path — the warning must be explicit about which checkpoint is affected.

This distinction is owned by `transaction-editor.svelte`, not the table — the table passes the transaction and opens the editor.

---

## Side Panel Pattern (Global Transactions Page)

The global transactions page has:
- Full-width table on the left/main area
- Side panel (same pattern as account-editor, category-editor) that slides in when a transaction is clicked or "New transaction" is pressed

Clicking a row opens a **detail panel** (read-only) first. The detail panel shows:
- All fields in a clean read layout
- Full posting breakdown (with account names/codes and commodity symbols from enriched response; system account labels resolved from `account_system_role`)
- **Edit button** — opens editor in the same panel, replacing detail view
- **Create correction button** — opens editor in correction mode (posted transactions only)
- **Action buttons** — Post, Approve, Void with reason (status-appropriate)

Version history / audit trail is not included in the detail panel for v1 (no API exists for it).

On **mobile**, the side panel becomes a full-screen overlay (no split layout).

The side panel is NOT used for navigation — it hosts `transaction-editor.svelte`. This is consistent with the existing account and category page patterns.

Account and category register pages do not use a side panel for the table itself — they navigate to a dedicated route. They also embed the editor side panel within the route for editing.

---

## Mobile Presentation

The app is primarily desktop but register use on mobile is a realistic workflow. The table adapts to narrow viewports using the `priority` values defined in each column set:

- **Priority 1** columns are always shown.
- **Priority 2** columns are hidden below ~600px.
- **Priority 3** columns are hidden below ~900px.

This gives a mobile global list of Date + Payee + Amount, and a compact mobile register of Date + Payee + Amount + Balance, which covers the essential read use case. The four priority-1 register columns remain visible in a responsive grid; Payee may wrap, while Amount and Balance remain right-aligned. Priority-2 and priority-3 columns are hidden rather than forcing horizontal scrolling.

The transaction **editor** on mobile is a full-screen form rather than a side panel. The side panel component detects viewport width and switches to a bottom-sheet or full-page presentation below the breakpoint.

---

## Implementation Sequence

Each step leaves the app runnable.

**Step 1 — Backend: posting response enrichment**
Add bulk account/commodity lookup and reusable enrichment to all transaction response paths: list, register, single read, create, update, post, void, approve, and correct. Add `account_name`, `account_code`, `account_system_role`, `account_builtin_key`, `account_class`, `commodity_code`, `commodity_symbol` to `app.Posting` and propagate them to API response structs. Update schema and regenerate types.

**Step 2 — Backend: category filter**
Add `category_id` to `ListTransactionsParams`, `ListTransactionsInput`, API handler, and OpenAPI spec. Validate account class in app layer. Regenerate schema. No frontend changes yet.

**Step 3 — Frontend API layer**
- Add `categoryID` to `TransactionListOptions` in `transactions.ts`
- Add `accountRegisterInfiniteQueryOptions`

**Step 4 — Display primitive and shared pieces**
- `transaction-table.svelte` (generic table with infinite scroll, error/retry, keyboard navigation, priority-based column hiding)
- `transaction-labels.ts` (status/kind display helpers, `formatQuantity` exact formatter, system account label resolver)
- `transaction-filter-bar.svelte`
- `transaction-row-actions.svelte`

**Step 5 — Transaction editor**
`transaction-editor.svelte` — create, draft edit, posted edit (with reconciliation-override path at save time), correction mode, and an explicit transfer entry point. This is the largest piece and can be built independently of the table.

**Step 6 — Global transactions page**
- `transaction-list.svelte` wrapper
- `/app/transactions/+page.svelte` with mobile-responsive side panel
- Wire editor side panel

**Step 7 — Account register route**
- `account-register.svelte` wrapper (priority-based compact grid on mobile)
- `/app/accounts/[id]/+page.svelte`
- Add "View register" link/click handler on account list rows

**Step 8 — Category transactions route**
- `category-transactions.svelte` wrapper
- `/app/categories/[id]/+page.svelte`
- Add click handler on category list rows

---

## Transaction Editor: Simple by Default

The editor has three tiers exposed progressively — the default view covers the vast majority of ordinary use.

### Tier 1 — Simple (default, always visible)

Single-form fields:
- Date
- Payee (autocomplete using `payeesQueryOptions({ q: inputValue })` — endpoint and query helper already exist)
- Description / memo
- Category (income or expense account, single select)
- Amount (inflow positive, outflow negative — from the selected Account's perspective; see Amount Display Semantics)
- Account (which asset/liability account the money moves from/to)

This covers the common case: one asset/liability account leg + one income/expense category leg. The Category field cannot select asset/liability accounts. An explicit "Transfer" action opens Tier 3 in transfer mode (or a dedicated transfer form) for account-to-account movement.

### Tier 2 — Advanced (expandable section)

Reveals additional fields:
- Transaction kind (ordinary / transfer / adjustment — opening_balance is system-only)
- Note (markdown)
- External reference / import hint
- Individual posting memos

`needs_review` is **not** exposed here. Per the API contract, `needs_review` is always false for manually entered transactions and is only set by the import pipeline. Exposing a toggle would contradict the product contract.

Reconciliation status is also **not** exposed here. Reconciliation is an auditable workflow; per-posting status must not be settable as a manual form field during ordinary transaction entry.

### Tier 3 — Split transaction (button that expands the posting list)

Reveals the full posting list editor: multiple category/account legs, each with an amount. The sum must balance (backend enforces this; frontend shows a running total and highlights imbalance).

The backend already accepts the full `journal_entries[]` structure. The UI abstraction is purely cosmetic — Tiers 1 and 2 construct the same payload as Tier 3.

---

## Open Questions

1. **Navigation from account list** — clicking an account row currently opens the editor. Adding "View register" navigation needs a UI decision: make the row itself a link to `/app/accounts/[id]`, or add a dedicated register icon button alongside the existing Edit/Close/Archive actions?

2. **Navigation from category list** — same question.
