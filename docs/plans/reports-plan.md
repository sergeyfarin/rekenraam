# Reports Plan

Status: **active implementation plan for roadmap R2**. Shipped so far: the
net-worth-series backend/OpenAPI foundation (`GET /api/v1/reports/net-worth`)
and the `/app/reports` route presenting it as an accessible per-commodity
table with URL-addressable date/bucket filters. Still pending: the spending
and cashflow read models, account/category/payee/commodity filters, CSV and
print output, and charts. Last verified against the codebase: 2026-08-07.

This plan delivers the first daily-driver reports: net worth over time,
spending by category or payee, and cashflow. It is governed by
`docs/product-requirements.md`, sequenced by `docs/roadmap.md`, and must follow
the ledger invariants in `docs/plans/transaction-ledger-core-plan.md`.

## Verified starting point

The app already has authenticated, OpenAPI-described single-point read models:

- `GET /api/v1/ledger/net-worth` — an as-of asset/liability total grouped by
  commodity; it excludes `commodity_trading`.
- `GET /api/v1/ledger/account-balances` — direct and subtree balances per
  account as of a date.
- `GET /api/v1/ledger/category-totals` — category totals for an inclusive date
  range, optionally restricted to income or expense.

They are useful building blocks, not the reports API. They do not provide time
series, a shared filter contract, cashflow semantics, a reports route, or
report-level drill-downs. Keep them stable for their existing consumers; add
new `/api/v1/reports/*` read models rather than overloading each endpoint into a
different contract.

## Outcomes and non-negotiables

1. A user can answer, from one reports route: “What is my net worth?”, “Where
   did money go?”, and “What changed my cash?” over a chosen period.
2. The server is the source of truth for all calculations. The frontend formats
   and presents returned values; it never recomputes balances from rows.
3. Money remains exact. Results are grouped by commodity unless the caller
   explicitly selects a later FX valuation method; the UI must never invent a
   combined total across unlike commodities.
4. Posted, non-voided, non-soft-deleted ledger data is the sole R2 reporting
   basis. Drafts, voids, and deleted records never leak into a report.
5. A table is the accessible source of truth. Charts summarize that table and
   never carry information unavailable to keyboard or screen-reader users.
6. Every result returns the effective query and any default/excluded-system
   policy so that an exported or printed report can be understood later.

## R2 boundary and preserved follow-ups

R2 ships three useful reports, not a generic report builder. This is an
implementation sequence, not a decision to discard deeper work.

R2 includes shared filters, URL-addressable report views, tables, modest chart
summaries, CSV/print-friendly output, drill-down where an existing route can
honour the filter, and all loading/empty/error states.

The following remain designed follow-ups and must be reconsidered explicitly at
the R2 acceptance review:

- named saved report definitions and live report runs;
- immutable/reproducible report snapshots;
- a reporting-currency selector and named FX/price valuation method;
- country, jurisdiction, tax, investment, and benchmark dimensions;
- a user-configurable report builder.

## Shared contract

### Common semantics

- `start_date` and `end_date` are ISO calendar dates, inclusive. Series reports
  require both; a client may offer presets but must put the resolved dates in the
  URL and request.
- `bucket` is `day`, `week`, `month`, `quarter`, or `year`. Week boundaries use
  ISO weeks; month/quarter/year buckets use calendar boundaries. The response
  labels the actual inclusive bucket start/end dates.
- Repeated query parameters represent an OR-set within that filter, for example
  `account_id=12&account_id=14`. Different filter types combine with AND.
- Account filters use account IDs plus `include_descendants`. The backend
  resolves descendants as of each reporting date; the frontend must not flatten
  the account tree itself.
- Category and payee filters are IDs. Commodity filtering is by commodity ID.
  Account-class/kind, tag, country, and jurisdiction filters are deliberately
  absent from the first public contract until a concrete report needs them.
- System accounts are excluded by a named, endpoint-specific default policy;
  callers cannot obtain a silently different result by relying on a UI default.
- Invalid dates, empty/inverted ranges, invalid buckets, inaccessible account or
  filter IDs, and unsupported filter combinations return `VALIDATION_FAILED`.
  Arithmetic overflow returns `LEDGER_OVERFLOW`.

### New API shape

Add these OpenAPI-first endpoints under `/api/v1/reports`:

| Endpoint | Purpose | First-release filters |
| --- | --- | --- |
| `GET /reports/net-worth` | Asset/liability value at each bucket end | dates, bucket, account IDs, descendants, commodity IDs |
| `GET /reports/spending` | Expense or income totals ranked by one dimension | dates, `group_by=category|payee`, category IDs, payee IDs, account IDs, descendants, commodity IDs |
| `GET /reports/cashflow` | Cash movement classified as inflow, outflow, transfer, and net movement | dates, bucket, cash account IDs, descendants, category IDs, payee IDs, commodity IDs, transfer policy |

Every response must contain `query`, `buckets`, `commodity_totals`, and
`excluded_system_roles` (or an equally explicit endpoint-specific policy). Do
not reuse presentation DTOs from the existing ledger endpoints merely because
they have similar quantity fields. Reuse the lossless quantity schema, but give
reports stable names and response types of their own.

## Report definitions

### Net worth over time

**Question:** What did I own less what I owed at each point in time?

- Include assets and liabilities only. `commodity_trading` is excluded by
  default; `transfer_clearing` remains included. The response names that policy.
- Take each bucket’s balance as of the bucket end, using account versions and
  postings valid through that date. Do not derive a time series by summing
  current account balances backward in the frontend.
- Return separate exact totals per commodity. A combined display is unavailable
  until the caller chooses a future reporting-currency/valuation method.
- An account filter may power full net worth, one account’s historical balance,
  or a selected account group. The same endpoint can later power compact account
  detail history without changing the financial calculation.

UI:

- `/app/reports` opens on net worth with a sensible, URL-visible recent date
  preset. It offers date range, bucket, account, and commodity filters.
- Show a chart only as a summary of a bucket table. The table provides bucket,
  commodity, asset total, liability total, and net-worth total.
- Explain empty state (“post transactions to see history”) and multi-commodity
  state (“totals are shown separately; no valuation method is selected”).

### Spending by category or payee

**Question:** Where did money go, and who received it?

- R2 has one grouping per request: `category` or `payee`. A nested
  category-then-payee pivot is follow-up work.
- Expense values are presented as positive spending magnitudes; refunds and
  reversals reduce the total. Income mode is supported only when explicitly
  selected and is labelled as income, not spending.
- Transfers and system-account activity are excluded by default. The report
  operates over income/expense category postings, not an inferred bank-statement
  classification.
- Each group row contains exact per-commodity totals, its share only within the
  same commodity, and a drill-down query. A ranking must not compare unlike
  commodities as one number.

UI:

- A category/payee switch preserves compatible filters and changes only
  `group_by` in the URL.
- Dense table first, then a companion bar/donut summary when a single commodity
  is selected. Drill-down links use the existing category or transactions route
  only when it can represent the same date/filter semantics; otherwise show the
  filter summary rather than a misleading link.

### Cashflow

**Question:** What changed my liquid cash, without treating transfers as
spending?

R2 must lock this semantic before writing SQL:

1. Default cash scope is active accounts of kinds `cash`, `checking`, `savings`,
   and `brokerage_cash`, excluding system accounts. The UI shows this default
   and allows an account/tree selection; it is not an invisible “all assets”
   shortcut.
2. For selected cash postings, counterpart income is **inflow** and counterpart
   expense is **outflow**. A transfer between two selected accounts is eliminated
   because it changes neither the selected cash total nor the user's cashflow.
3. A movement between selected cash and an unselected asset, liability, or equity
   account is **transfer/financing movement**, shown separately with signed
   incoming/outgoing totals. It never becomes income or spending.
4. `net_movement` is the signed sum of all selected cash postings for the
   bucket. It must reconcile exactly to the selected cash balance change between
   bucket boundaries. `operating_net` is inflow minus outflow and excludes
   transfer/financing movement.
5. A transaction with multiple counterpart postings must be classified from its
   posting relationships; no “first counterpart wins” heuristic is permitted.
   The backend may emit allocation rows internally, but the public bucket totals
   must reconcile exactly and be auditable through drill-down.

UI:

- Show inflow, outflow, operating net, transfers/financing, and net movement for
  every bucket. Make the transfer inclusion policy visible.
- The default table/chart uses the default liquid-cash scope. A selected account
  tree is clearly named in the result summary.
- Category/payee filters constrain the relevant counterpart dimensions; the UI
  must explain when a transfer-only movement is excluded by such a filter.

## Frontend shape

- Add `frontend/src/routes/app/reports/+page.svelte` and report-specific
  components under `frontend/src/lib/reports/`. Keep the route as a composed
  screen, not one monolithic component.
- Add typed API helpers through generated OpenAPI types. Do not hand-maintain
  report DTO copies or issue one request per table row.
- Make query state shareable: the URL is the source of filter state; changing a
  control updates the URL, then the typed query. Named saved reports later store
  this same query shape.
- Use the translation boundary for every label and empty/error explanation;
  use locale-aware money/date formatting and semantic design tokens.
- Define loading, empty, error, overflow, single-commodity, and
  multi-commodity states for each report. Meet mobile layout and keyboard
  navigation requirements before chart polish.

## Delivery slices and acceptance

1. **Contract and fixtures**
   - Define shared report DTOs/OpenAPI paths, date/bucket semantics, system
     policy, and a deterministic multi-account/multi-commodity fixture.
   - Add a reports navigation entry and URL-driven empty/loading/error shell.
   - Acceptance: generated frontend types compile; invalid shared query cases
     have API tests before individual report queries exist.

   **Progress (2026-07-13):** `GET /api/v1/reports/net-worth` now establishes
   the date-range/bucket contract, calendar bucket boundaries, exact
   commodity-grouped bucket-end totals, effective-query echo, explicit
   `commodity_trading` exclusion, OpenAPI types, and named API/application
   tests. `/app/reports` now provides the shared report shell, navigation,
   URL-addressable date/bucket filters, and loading/empty/error states for the
   net-worth view. Account/commodity filters and the common fixture remain in
   this slice.
2. **Net-worth series**
   - Implement the series read model, API, table/chart, and account/commodity
     filters. Reuse the typed result later for account-detail history.
   - Acceptance: bucket-end balances match the existing as-of net-worth result
     for each bucket end; grouped commodities never yield a fake total.

   **Progress (2026-07-13):** the route renders the exact series in an
   accessible, responsive table with currency labels. It deliberately keeps
   unlike commodities on separate rows and explains why they cannot be summed;
   charts and account/commodity filters remain pending.
3. **Spending**
   - Implement category/payee grouping, exact totals, filters, table/chart, and
     safe drill-down.
   - Acceptance: expense/refund/transfer fixture results match manual ledger
     arithmetic, and category versus payee changes grouping rather than source
     data.
4. **Cashflow**
   - Implement the locked liquid-cash selection and counterpart-classification
     model, then the API and UI.
   - Acceptance: for every commodity and bucket, `net_movement` reconciles to
     the selected-cash balance delta; transfers within scope net to zero; a
     split transaction is not misclassified.
5. **Trust and release quality**
   - Add print-friendly and CSV views, accessibility smoke coverage, responsive
     review, and backend/API/frontend/E2E regression tests.
   - Acceptance: all three reports have loading, empty, error, populated, and
     multi-commodity states; their tables are usable without a pointing device.

## Validation matrix

- **Backend:** exact arithmetic and scale alignment; inclusive boundaries;
  account hierarchy as-of dates; bucket boundaries; system exclusions;
  refunds; void/deleted exclusion; transfer elimination; split counterpart
  classification; net-movement reconciliation; and overflow.
- **API/OpenAPI:** authentication, all invalid common filters, repeated filter
  semantics, default-policy fields, generated type check, and stable error
  codes.
- **Frontend:** URL/query construction, date presets, locale formatting,
  accessible tables, chart/table equivalence, and all explicit screen states.
- **E2E:** a seeded owner records income, an expense, an internal transfer, a
  refund, and a multi-currency transaction; reports show each in the correct
  view and cashflow reconciles to the visible cash-account change.

## Competitor and parity review

Before closing R2, update `docs/competitor-comparison.md` with the result:

- Money/Quicken/Monarch parity: visible net worth, spending, cashflow, and
  export-ready reports.
- Firefly III parity: category/payee insight without compromising ledger
  semantics.
- PocketSmith differentiation groundwork: exact per-currency cashflow, ready
  for R10 forecasting rather than a single fabricated base-currency number.
- Ghostfolio/Portfolio Performance gap retained: returns, allocation, and
  benchmarks remain R13 work, not an accidental partial R2 promise.
