# Reports Plan

This plan scopes roadmap **R2 Reports UI**. It covers the first daily-driver
reports: net worth over time, spending by category/payee, and cashflow.

Reports are not just one route. The same calculations must power full report
pages, account-detail summaries, category/payee drill-downs, and later
multi-currency/investment views without each screen redefining financial
semantics.

## Principles

- Backend-composed read models are the source of truth for report values.
  Frontend code renders, filters, and formats returned values; it does not
  recalculate balances.
- Exact integer-plus-scale quantities remain grouped by commodity unless a
  request explicitly asks for conversion through a named FX/pricing method.
- Filters are part of the report contract, not route-local UI state. A saved or
  shared report definition should be able to reuse the same filter shape later.
- Net worth is a reusable valuation series, not only a "Reports" page chart. It
  should support account-level history, account groups, and filtered slices.
- First milestone is live reports, not immutable report snapshots. Report input
  parameters should be explicit enough that snapshots can be added later without
  changing user-facing semantics.

## Shared Report Query Shape

All R2 report endpoints should converge on one filter vocabulary:

- `book_id`: implicit `1` at runtime, still present in backend internals.
- Date range: inclusive `start_date` and `end_date` calendar dates.
- Bucket/granularity: at least day, week, month, quarter, and year where the
  report is a time series.
- Accounts: include/exclude account ids, with an option to include descendants.
- Account classes/kinds: useful for assets, liabilities, spending accounts, and
  investment holdings.
- Categories: include/exclude category accounts, with descendants.
- Payees: include/exclude payee ids.
- Commodities/currencies: include/exclude commodity ids or codes.
- Countries/jurisdictions: deferred until account, institution, security, or
  tax-residency metadata exists; reserve the filter concept so reports do not
  need a new model later.
- Tags: deferred unless tag filtering already exists for transaction lists when
  R2 is implemented.
- System-account policy: default exclusions should match ledger semantics, for
  example excluding `commodity_trading` from ordinary net-worth reports.
- Reporting currency and valuation method: optional in R2 unless enough FX data
  exists. Until conversion is implemented, return grouped commodity values and
  make "combined total unavailable" explicit.

## Core Reports

### Net Worth Over Time

Purpose: show balance-sheet value over time, with reusable slices.

Backend:

- Add a series endpoint or extend the current `GET /api/v1/ledger/net-worth`
  beyond one as-of date.
- Compute periodic asset/liability totals from posted, non-deleted,
  non-voided ledger state.
- Support account filters so the same endpoint can power full net worth, one
  account's historical balance on account detail, selected groups such as cash
  accounts/investments/liabilities, and future country/currency/investment
  filtered views.
- Return per-bucket totals grouped by commodity, plus normal/display quantity
  values using existing sign conventions.
- Keep `transfer_clearing` included and `commodity_trading` excluded by default,
  matching `docs/transaction-ledger-core-plan.md`.

UI:

- Reports route: overview chart/table with date range, bucket, account, and
  commodity filters.
- Account detail/register: compact account-balance-over-time view using the same
  endpoint and an account filter.
- Empty state explains that posted transactions are needed.
- Multi-commodity state shows grouped values and avoids fake combined totals.

### Spending By Category And Payee

Purpose: answer where money went and who received it.

Backend:

- Extend category totals or add a report endpoint that can group by category,
  payee, or category then payee.
- Use income/expense account mappings; categories remain accounts, not a new
  ledger primitive.
- Exclude transfers from spending totals unless the user deliberately includes
  transfer/system accounts.
- Preserve exact grouped-by-commodity totals.

UI:

- Show category and payee rankings for a selected date range.
- Provide drill-down links to the underlying category/payee transaction lists.
- Offer chart plus dense table; the table is the accessible source of truth.

### Cashflow

Purpose: show inflows, outflows, and net change over time.

Backend:

- Add the missing cashflow read model.
- Define basis explicitly before implementation: actual posted ledger activity,
  inclusive date range, grouped into buckets.
- Separate income, expenses, transfers, and net movement so users can inspect
  what changed cash without confusing transfer churn with spending.
- Support account filters for checking-account cashflow and future portfolio or
  country-specific cashflow.

UI:

- Time-series bars or table for inflow, outflow, and net by bucket.
- Date range, bucket, account, category, payee, and commodity filters.
- Make transfer inclusion/exclusion explicit.

## Delivery Slices

1. **Report query contract:** define shared filter DTOs, date inclusivity, bucket
   semantics, default system-account policy, and OpenAPI shapes.
2. **Net-worth series:** backend series endpoint plus reports-route chart/table;
   reuse it on account detail for account-level balance history.
3. **Spending report:** category/payee grouping, table, chart, drill-down links,
   and CSV/print-friendly output.
4. **Cashflow report:** backend read model, table/chart UI, and transfer policy.
5. **Polish and trust:** loading/empty/error states, mobile layout, accessible
   tables, print styles, CSV export for report outputs, and regression tests.

## Validation

- Backend tests must cover exact arithmetic, sign normalization, date-range
  boundaries, system-account default exclusions, multi-commodity grouping, and
  overflow returning `LEDGER_OVERFLOW`.
- API tests must cover the shared filter shape and invalid date/filter handling.
- Frontend tests should cover query construction and visible states for empty,
  error, loading, multi-commodity, and populated reports.
- Manual validation should include a seeded multi-account file with transfers,
  income, expenses, liabilities, and at least two commodities.
