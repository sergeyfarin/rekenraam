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
  cell: Snippet<[R]>;  // Svelte 5 snippet
};

let {
  rows,         // R[] — generic row type
  columns,      // Column<R>[]
  isLoading,
  hasNextPage,
  onLoadMore,   // called by intersection observer at bottom
  onRowClick,   // optional: (row: R) => void
}: Props<R>
```

Handles: intersection observer for infinite scroll, loading skeleton rows, empty state, sticky header. Does not know about transactions.

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

### 1. Category filtering on transactions (~30 lines)

`GET /api/v1/transactions` currently has no `category_id` param. Categories are income/expense accounts. The account filter already uses an EXISTS subquery against `posting_versions`. Category filtering is identical — just an additional OR branch.

**Changes:**
- `db/transactions.go` — add `CategoryID int64` to `ListTransactionsParams`; add EXISTS clause (same pattern as AccountID)
- `app/transactions.go` — add `CategoryID int64` to `ListTransactionsInput`; pass through
- `api/transactions.go` — parse `category_id` query param in `readTransactionListInput`
- `api/schema.yaml` (or equivalent OpenAPI spec) — add `category_id` query param
- Regenerate `schema.d.ts` and add `categoryID` to `TransactionListOptions` in `transactions.ts`

### 2. Sort direction (~20 lines, optional for v1)

Current sort is always `transaction_date DESC, id DESC`. For the global transactions page it may be useful to flip to ASC. Cursor logic already uses `(date, id)` tuple — just invert the comparison.

Add `SortAsc bool` to `ListTransactionsParams`. When true: `ORDER BY transaction_date ASC, id ASC` and cursor flips to `date > ? OR (date = ? AND id > ?)`. This is the only sorting worth supporting without moving to offset pagination.

**Decision: defer to v2.** The register is always most-recent-first. The global page starting most-recent-first is reasonable. Sort direction can be added later without breaking the cursor format.

### 3. Account register infinite query options (~15 lines)

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
      transaction-labels.ts             ← display helpers (status labels, etc.)
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

| Column | Source | Notes |
|---|---|---|
| Date | `transaction_date` | |
| Payee / Description | `payee_name` or `description` | |
| Account(s) | Derived from `journal_entries[].postings[].account_id` | Show primary account(s), collapse if many |
| Amount | Primary posting `quantity_value / 10^quantity_scale` | Signed, formatted with commodity symbol |
| Status | `status` | Badge: draft/posted/voided |
| Flags | `needs_review` | Review badge |

### Account register

| Column | Source | Notes |
|---|---|---|
| Date | `entry_date` | Journal entry date, not transaction date |
| Payee / Description | `payee_name` or `description` | |
| Memo | `posting.memo` | |
| Reconciliation | `posting.reconciliation_status` | Icon: uncleared/cleared/reconciled |
| Amount | `posting.quantity_value` | Signed |
| Balance | `running_balance` | Right-aligned, shown in account's commodity |

### Category transactions

Same as global list, minus the Accounts column (replaced by a "counterpart account" column showing where the money went/came from — the non-category posting's account).

---

## Row Actions

Actions are context-sensitive based on transaction status:

| Status | Available actions |
|---|---|
| draft | Edit, Post, Delete |
| posted | View, Void (with reason), Correct (opens editor in correction mode) |
| voided | View only |
| any + needs_review | Approve (inline, no editor needed) |

The editor distinguishes between:
- **Draft edit** → `PATCH /api/v1/transactions/{id}`
- **Correction** → `POST /api/v1/transactions/{id}/correct` (creates new transaction replacing the posted one)

This distinction is owned by `transaction-editor.svelte`, not the table — the table just passes the transaction and whether it's a correction.

---

## Side Panel Pattern (Global Transactions Page)

The global transactions page has:
- Full-width table on the left/main area
- Side panel (same pattern as account-editor, category-editor) that slides in when a transaction is clicked or "New transaction" is pressed

The side panel is NOT used for navigation — it hosts `transaction-editor.svelte`. This is consistent with the existing account and category page patterns.

Account and category register pages do not use a side panel for the table itself — they navigate to a dedicated route. They also embed the editor side panel within the route for editing.

---

## Implementation Sequence

Each step leaves the app runnable.

**Step 1 — Backend: category filter**
Add `category_id` to `ListTransactionsParams`, `ListTransactionsInput`, API handler, and OpenAPI spec. Regenerate schema. No frontend changes yet.

**Step 2 — Frontend API layer**
- Add `categoryID` to `TransactionListOptions` in `transactions.ts`
- Add `accountRegisterInfiniteQueryOptions`

**Step 3 — Display primitive and shared pieces**
- `transaction-table.svelte` (generic table with infinite scroll)
- `transaction-labels.ts` (status/kind display helpers)
- `transaction-filter-bar.svelte`
- `transaction-row-actions.svelte`

**Step 4 — Transaction editor**
`transaction-editor.svelte` — create, draft edit, and correction mode. This is the largest piece and can be built independently of the table.

**Step 5 — Global transactions page**
- `transaction-list.svelte` wrapper
- `/app/transactions/+page.svelte`
- Wire editor side panel

**Step 6 — Account register route**
- `account-register.svelte` wrapper
- `/app/accounts/[id]/+page.svelte`
- Add "View register" link/click handler on account list rows

**Step 7 — Category transactions route**
- `category-transactions.svelte` wrapper
- `/app/categories/[id]/+page.svelte`
- Add click handler on category list rows

---

## Transaction Editor: Simple by Default

The editor has three tiers exposed progressively — the default view covers the vast majority of ordinary use.

### Tier 1 — Simple (default, always visible)

Single-form fields:
- Date
- Payee (autocomplete from existing payees)
- Description / memo
- Category (income or expense account, single select)
- Amount (signed: positive = income, negative = expense)
- Account (which asset/liability account the money moves from/to)

This covers the common case: one account leg + one category leg. The backend receives this as two postings in one journal entry. No posting structure is shown to the user.

### Tier 2 — Advanced (expandable section)

Reveals additional fields:
- Transaction kind (ordinary / transfer / adjustment — opening_balance is system-only)
- Note (markdown)
- Needs review toggle
- External reference / import hint
- Individual posting memos
- Reconciliation status per posting (for manual override)

### Tier 3 — Split transaction (button that expands the posting list)

Reveals the full posting list editor: multiple category/account legs, each with an amount. The sum must balance (backend enforces this; frontend shows a running total and highlights imbalance).

The backend already accepts the full `journal_entries[]` structure. The UI abstraction is purely cosmetic — Tiers 1 and 2 construct the same payload as Tier 3.

### Click-to-view vs. Click-to-edit

Clicking a transaction row opens a **detail panel** (read-only), not the editor directly. The detail panel shows:
- All fields in a clean read layout
- Full posting breakdown
- Status history / version info
- **Edit button** — opens editor in the same panel, replacing detail view
- **Action buttons** — Post, Approve, Void (status-appropriate)

If the transaction has any reconciled postings, the Edit button shows a warning modal first:
> "This transaction has reconciled postings. Editing it will require a corrective entry that may affect your reconciliation. Continue?"

Confirming opens the editor in **correction mode** (`POST /correct`), not draft edit mode. This is because `PATCH` is only valid for drafts; a posted transaction with reconciled postings requires the corrective workflow. The warning makes this consequential distinction visible to the user before they proceed.

For draft transactions with no reconciled postings, Edit opens the editor directly with no warning.

---

## Amount Display and FX Conversion

### The data model

Each posting carries: `quantity_value` (integer coefficient), `quantity_scale` (decimal places), and `commodity_id`. The book has a `default_currency_commodity_id` (the base currency). Historical FX rates are stored in the pricing system as `(base_commodity_id, quote_commodity_id, valuation_date) → price`.

### The display problem

In the transaction list, a single transaction may have postings in multiple commodities (e.g. a EUR purchase on a USD account involves a USD posting and a EUR posting plus potentially an exchange journal entry). The list needs one primary amount and optionally a base-currency equivalent.

### Strategy: backend-enriched amounts (preferred)

Doing this purely client-side requires: for each transaction row, identify the "primary" posting, look up the FX rate for that commodity on that date, and compute the base-currency equivalent. This means N price lookups per page load, or a bulk price fetch that then needs client-side joining. Both are fragile and add latency.

**Better: add a `ValuedAt` enrichment to the transaction list response.** The backend, when returning a transaction list, can attach the base-currency valuation to each transaction (the sum of all posting values converted to the book's base commodity at the transaction date's rate). This is a single backend computation at query time with access to all the pricing data.

This requires a new optional field on `TransactionResponse`:

```go
BaseCurrencyValue *valuationResponse `json:"base_currency_value,omitempty"`

type valuationResponse struct {
    CommodityID  int64  `json:"commodity_id"`
    Value        string `json:"value"`   // formatted decimal
    Scale        int    `json:"scale"`
    IsEstimated  bool   `json:"is_estimated"` // true if rate was approximated
}
```

The backend computes this by summing the absolute value of asset/liability postings (not the income/expense counterparts — they are the same transaction in double-entry) converted to base commodity using the nearest available price on the transaction date.

**For transfers between same-commodity accounts:** the posting amounts are in the same commodity — no conversion needed, value is exact.

**For FX transfers (e.g. USD→EUR):** the exchange journal entry postings already encode the rate implicitly. The backend can derive base-currency value from the asset/liability legs directly if both are in known commodities with prices on file.

**When no price is available** (missing historical rate): `IsEstimated: true` is set, and the UI shows the native amount only with a visual indicator (e.g. a `~` prefix or a muted "no rate" label in the converted column). This is better than showing nothing or erroring.

### Column layout in the global list

| Column | Content |
|---|---|
| Amount | Native amount of the primary posting (asset/liability leg), with commodity symbol |
| Base value | Base-currency equivalent from `base_currency_value`, shown in a narrower column with muted styling. Hidden if commodity = base commodity (no conversion needed). `~` prefix if `is_estimated`. |

The "primary posting" for display is the asset or liability leg (account_class = asset or liability). For income/expense-only transactions (unusual), fall back to the first posting.

### Implementation note

The `BaseCurrencyValue` enrichment is an optional backend addition — it can be deferred to a later step without blocking the table build. For v1, show native amount only; add the base-currency column in a follow-up once the enrichment endpoint is built.

---

## Open Questions

1. **Navigation from account list** — clicking an account row currently opens the editor. Adding "View register" navigation needs a UI decision: make the row itself a link to `/app/accounts/[id]`, or add a dedicated register icon button alongside the existing Edit/Close/Archive actions?

2. **Navigation from category list** — same question.

3. **Editor: payee autocomplete scope** — payee names are stored on transactions as free text (`payee_name`). The backend would need a `GET /api/v1/payees` or a suggestion endpoint to power autocomplete. Currently there is no such endpoint. Defer autocomplete to v2, or add a lightweight payee suggestion endpoint as part of this work?
