# Roadmap

This is the one active, forward-looking plan for Rekenraam. It answers
**what to build next**, in order. It is governed by
`docs/product-requirements.md`; shipped scope is recorded in
`docs/implemented.md`; live technical debt is in `docs/backlog.md`.

Last reviewed: 2026-07-12.

## Direction

Build a polished, self-hosted personal-finance daily driver first. The order is:

1. Trust and visibility: reports and exports.
2. Lower manual effort: CSV import with reusable profiles.
3. Daily planning: budgets and recurring transactions.
4. Differentiate for multi-currency, cross-border households and investors.

The product is deliberately single-user and self-hosted. Connections are
bring-your-own-key adapters, never guaranteed coverage. Native apps,
small-business accounting, a hosted service, and broker trade execution are out
of current scope. The trade-execution decision and market analysis are retained
in `docs/competitive-analysis-2026-07.md`.

## Current plan

Do not start a new roadmap initiative until the current one has met its
acceptance criteria. Feature-specific design documents may clarify a slice, but
must not create a competing sequence.

### Now — R2: reports users can act on

Ship a reports route with net worth over time, spending by category/payee, and
cashflow. `docs/reports-plan.md` is the implementation reference.

Deliver in this order:

1. Shared report-query contract and accessible report shell; net-worth and
   spending views over the existing read models.
2. Cashflow read model and view, explicitly separating inflow, outflow,
   transfers, and net movement.
3. Date/account/category/payee/commodity filters, CSV and print-friendly tables,
   and summary charts that do not replace the accessible data table.

The first implementation must be a coherent vertical slice, but this is a
**sequencing constraint, not a deleted design decision**. The fuller report
contract—including saved definitions/runs, cross-currency valuation, investment
dimensions, and later snapshots—remains in `docs/reports-plan.md`. At the R2
acceptance review, explicitly decide which of those items is justified by real
use before moving to R3; do not let it disappear by omission.

### Next — R3: portable core data

Ship core-ledger CSV and QIF export. These are mandatory product requirements,
not optional polish. The first release should favour a documented, stable export
shape over a broad structured-backup format.

### R3a — core-workflow accessibility regression coverage

Add focused automated accessibility smoke checks for setup/auth, transaction
entry, reconciliation, reports, and import. Cover semantic controls, labels,
keyboard navigation, focus handling, and contrast violations; keep mobile
journeys in the broader Playwright suite.

### Then — R5: ordinary-bank CSV import

Ship CSV import plus saved mapping profiles. The user maps a bank statement once
and can reuse that profile for the next statement. Reuse the staged review and
commit pipeline; do not build another import path.

### After that — planning loop

1. **R8 Budgets:** period budgets with actual-versus-budget reporting.
2. **R9 Recurring transactions:** templates and due-entry generation into the
   reserved producer-owned draft workflow.
3. **R10 Projected balances:** per-currency projections first; converted totals
   only with explicit FX semantics. Loan helpers are optional follow-up work.

## Deliberately later

These are valuable, but they are not allowed to displace the current plan:

- R6: XLSX and OFX/QFX adapters, import matching, per-split mapping, batch
  rollback, and richer import history. `docs/import-plan.md` retains the detailed
  data, lifecycle, and acceptance design.
- Import rules and payee/category cleanup: persistent staged-pipeline rules that
  match payee/description/amount and set category, payee, or tags; duplicate
  payee merge, bulk recategorization, and the imported `needs_review` queue.
- A second provider, with IBKR Flex Query the likely investor candidate;
  GoCardless Bank Account Data (EU) and SimpleFIN Bridge (US) remain candidates
  for the broader bank-feed need. All remain bring-your-own-key adapters.
- R7a daily-entry convenience: transaction templates, payee defaults, saved
  views, and keyboard-first entry.
- R11 pricing/FX management UI.
- R13 investment return analytics (TWR/MWR, allocation, benchmark comparison).
- Multi-currency reporting, report snapshots, attachment storage, multi-user,
  and household features.

## Competitor and parity check

Keep `docs/competitor-comparison.md` as the maintained parity matrix and
`docs/competitive-analysis-2026-07.md` as the dated deep dive. Before declaring
each roadmap initiative complete, update the comparison's implication section
and record either the parity gained or the deliberate gap retained.

The current parity lens is:

- **Money/Quicken/Monarch:** reports, imports, exports, budgets, and recurring
  transactions determine whether the app feels like an everyday replacement.
- **Firefly III:** reusable import profiles and user-defined import rules drive
  import-heavy retention; bank coverage itself is not a promise.
- **PocketSmith:** per-currency cashflow forecasting is the differentiator to
  protect in R10, not an optional conversion afterthought.
- **Ghostfolio and Portfolio Performance:** returns analytics, allocation, and
  benchmark comparison remain the investment-expectation gap after gains.
- **Rekenraam's moat:** exact multi-currency double-entry and lot-level
  investments must stay coherent as parity features are added.

## Public-release gates

These are a parallel release-readiness track, not a reason to delay local
daily-driver work.

### Before making the repository public

1. Scan the complete Git history for secrets and remove scanner bait or real
   secrets before anyone clones it.
2. Keep AGPL-3.0, `SECURITY.md`, dependency scanning, and vulnerability scanning
   current; enable GitHub secret/push protection when available.

### Before public announcement or marketplace listings

1. Complete R2, R3, and R5 so a newcomer can migrate, inspect, and export data.
2. Produce signed release binaries with reproducibility notes.
3. Prepare adoption assets: seeded demo, README screenshots, and a short
   migration walkthrough.

Public internet deployment with real financial data remains blocked by the live
security gates in `docs/backlog.md`: lockout-safe login throttling, MFA, and
authentication-event visibility.

## Open product decisions

Resolve these only when their related slice becomes current work:

- Export scope beyond core ledger CSV/QIF, including a full structured JSON
  backup of settings and metadata.
- First non-English UI languages and the highest-priority mobile workflow.
- Attachment storage, retention, access-control, backup, and encryption model.
- I-03: whether gains reporting should offer read-only analytical methods in
  addition to the authoritative lot-disposal method.
- I-04: whether realized gains/losses should remain computed reporting values or
  become ledger postings with an explicit account convention.

## Completed milestones

Foundation, accounts, ledger transactions, reconciliation UI, QIF import,
Trading 212 ingestion (including investment lots), and investments UI/gains are
shipped. See `docs/implemented.md` for the capability ledger and the historical
design records for rationale.
