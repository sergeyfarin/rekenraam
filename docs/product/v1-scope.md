# Rekenraam V1 Product Scope

Last updated: 2026-05-04

## Product Target

V1 is a personal finance web app for self-hosted use. It should feel comparable
in breadth to Firefly III, GnuCash, MS Money, Quicken, and Money Manager Ex for
core personal workflows, while leaving room for later small-business features.

Small-business support is an architectural future option, not a v1 release gate.

## V1 Must Have

- Docker deployment with PostgreSQL, API, and frontend
- first-admin bootstrap, login/logout, sessions, and book authorization
- audit context for user/session/device/request attribution
- account tree, account detail, balances, and single-account/multi-account
  registers
- transaction create/edit/delete/duplicate/bulk actions with split support
- server-side accounting invariants and locked-range validation
- category, payee, tag, person, project, institution, commodity, and currency
  management
- reconciliation workflow with balancing history and unlock/void controls
- CSV, XLS/XLSX, QIF, and OFX/QFX import with preview, validation, matching,
  duplicate detection, import audit trail, and commit
- CSV/QIF export and report data export
- core reports: net worth, cash flow, spending by category/payee, account
  trends, realized/unrealized gains
- investment basics: holdings, lots, buys, sells, dividends, reinvested
  dividends, corporate actions, cost basis, and performance
- FX/pricing settings, manual refresh, scheduled refresh state/history, and
  manual price entry
- budgets with monthly/annual targets, rollover, and planned-vs-actual reporting
- scheduled transactions with recurrence, reminders, skip, and post flows
- basic tax summaries for capital gains and dividends with configurable category
  tax codes
- trusted server-installed plugin manifests for importers, reports, pricing
  providers, and transaction enrichment rules
- frontend plugin manifest slots for navigation, settings panels, and report
  panels
- built-in theme token packs and persisted per-user theme selection
- documented backup/restore with Postgres-native operations
- liability accounts, loans, mortgages, amortization schedules, and loan-payment
  assistant
- transaction finder, advanced register filtering/search, and saved views
- memorized transactions, payee defaults, memorized splits, and transaction
  templates
- explicit multi-currency account and cross-currency transfer support
- explicit uncleared, cleared, and reconciled transaction states with running
  balances
- historical FX/price backfill, per-currency or per-commodity source
  assignment, and price source health/status views


## V1 Should Have

- report cache widening for heavier report families
- report CSV/print-friendly views
- import mapping template export/import
- production Compose example with reverse proxy/TLS guidance
- saved/customizable reports, chart views, and account statement/
  income-expense style reports
- basic projected cash balance from scheduled transactions
- persistent import cleanup and matching rules for payee normalization,
  category/account mapping, categorization cleanup, and duplicate handling
  preferences
- user-visible notes/documents on accounts and transactions if they do not delay
  core workflows
- per-user date and number format preferences
- keyboard-first quick entry and shortcut support
- admin operational status views for database health, integrity checks, and
  schema/migration version
- payee merge/dedup tooling
- advanced investment corporate actions beyond buys and sells, such as splits,
  mergers, and write-offs

## Deferred After V1

- invoices, customers, vendors, VAT workflows, and small-business AR/AP
- PDF statement parsing
- attachments OCR
- advanced forecasting / projected cashflow using machine learning
- PSD2/open banking and online bank connectivity
- remote plugin marketplace or arbitrary downloaded plugin execution
- encrypted-at-rest database packaging
- full country-specific tax filing exports
- advanced forecasting and scenario planning beyond scheduled-transaction-based
  projections
- server-side undo/redo unless redesigned around mutation history

## Release Gate

V1 can release when:

- self-hosted Docker deployment works from fresh setup instructions
- authenticated users can safely manage at least one personal book
- core account, register, transaction, reconciliation, import/export, report,
  budget, schedule, investment, pricing, plugin, and theme workflows are usable
  without Tauri
- backups and restores are documented and smoke-tested
- frontend no longer needs Tauri runtime APIs
- deployment, frontend, and core workflows no longer depend on Tauri runtime or
  build artifacts
