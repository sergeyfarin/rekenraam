# Desktop To Python Parity Matrix

Last updated: 2026-05-05

Status values:

- `not started`
- `baseline complete`
- `in progress`
- `verified`
- `dropped`

This tracks migration status at the capability-group level first. As deeper
slices move, each row can be expanded into per-command or per-use-case entries.

| Desktop capability | Current Rust source | Target Python modules | Target endpoint(s) | Status | Notes |
|---|---|---|---|---|---|
| storage and migration bootstrap | `src-tauri/src/db.rs`, `src-tauri/src/db_storage.rs` | `apps/api/alembic`, `apps/api/src/rekenraam_api/config/settings.py`, `apps/api/src/rekenraam_api/db/*` | `/api/v1/health`, `/api/v1/admin/runtime` | baseline complete | The empty app uses one current initial Alembic schema. Tauri desktop runtime/session tables are intentionally excluded from the Postgres baseline. Operational backup/restore docs and final deployment smoke remain. |
| books read-only | `src-tauri/src/db_currencies.rs`, `src-tauri/src/db_commodities.rs`, book bootstrap in `src-tauri/src/db.rs` | `apps/api/src/rekenraam_api/repositories/books.py`, `apps/api/src/rekenraam_api/services/books.py`, `apps/api/src/rekenraam_api/api/v1/books.py` | `/api/v1/books`, `/api/v1/books/{slug}` | verified | Covered by API tests, repository tests, and Docker smoke tests. |
| auth identity, sessions, and book access | `src-tauri/src/db.rs`, `src-tauri/src/session.rs` | `apps/api/src/rekenraam_api/repositories/access.py`, `apps/api/src/rekenraam_api/services/auth.py`, `apps/api/src/rekenraam_api/services/access.py`, `apps/api/src/rekenraam_api/services/request_context.py`, `apps/api/src/rekenraam_api/api/v1/auth.py` | `/api/v1/auth/*`, protected `/api/v1/*` routers | baseline complete | First-admin bootstrap, login/logout, user devices, auth sessions, request context, and book membership enforcement exist. Remaining work is user administration, password change/reset, role management UI/API, and user-visible audit trails. |
| accounts read-only list/detail | `src-tauri/src/db_accounts.rs` | `apps/api/src/rekenraam_api/repositories/accounts.py`, `apps/api/src/rekenraam_api/services/accounts.py`, `apps/api/src/rekenraam_api/api/v1/accounts.py` | `/api/v1/accounts`, `/api/v1/accounts/{id}` | verified | Flat list/detail slice is migrated and tested. |
| account tree and balances | `src-tauri/src/db_accounts.rs`, `src/routes/accounts/AccountTreeItem.svelte` | `apps/api/src/rekenraam_api/services/accounts.py`, `apps/api/src/rekenraam_api/api/v1/accounts.py` | `/api/v1/accounts/tree`, `/api/v1/accounts/balances` | baseline complete | Tree shape and transaction-backed balances exist. Remaining work is broader high-volume parity coverage and richer saved/filter views. |
| accounts writes and directives | `src-tauri/src/db_accounts.rs` | `apps/api/src/rekenraam_api/repositories/accounts.py`, `apps/api/src/rekenraam_api/services/accounts.py`, `apps/api/src/rekenraam_api/api/v1/accounts.py` | `/api/v1/accounts`, `/api/v1/accounts/{id}`, `/api/v1/accounts/{id}/directives`, `/api/v1/accounts/{id}/closing-validation`, `/api/v1/accounts/{id}/booking-policy` | baseline complete | Create/update/delete, close validation, directives, booking policy, and system-account protections exist. Remaining work is additional invariants and parity fixtures. |
| transactions and splits | `src-tauri/src/db_transactions.rs` | `apps/api/src/rekenraam_api/repositories/transactions.py`, `apps/api/src/rekenraam_api/services/transactions.py`, `apps/api/src/rekenraam_api/api/v1/transactions.py` | `/api/v1/transactions`, `/api/v1/transactions/{id}`, `/api/v1/accounts/{id}/register`, `/api/v1/transactions/bulk-void`, `/api/v1/transactions/bulk-delete`, `/api/v1/transactions/{id}/duplicate`, `/api/v1/transactions/payee-defaults` | baseline complete | Read/write list/detail/register flows, duplicate/bulk actions, locked-range checks, split support, cursor pagination, and payee-default helpers are on Python endpoints. Remaining work is deeper correctness parity, search/saved views, templates, memorized splits, and explicit cross-currency transfer workflows. |
| reconciliation | `src-tauri/src/db_transactions.rs`, `src-tauri/src/db_accounts.rs` | `apps/api/src/rekenraam_api/repositories/reconciliation.py`, `apps/api/src/rekenraam_api/services/reconciliation.py`, `apps/api/src/rekenraam_api/api/v1/reconciliation.py` | `/api/v1/reconciliation/accounts/{id}/start`, `/api/v1/reconciliation/accounts/{id}/finish`, `/api/v1/reconciliation/accounts/{id}/history`, `/api/v1/reconciliation/accounts/{id}/unlock` | baseline complete | Reconciliation start/finish/history/unlock flows exist and the account reconciliation page uses HTTP. Remaining work is broader statement fixtures, account-type coverage, and audit review depth. |
| balance checks and adjustments | `src-tauri/src/db_accounts.rs`, `src-tauri/migrations/V1__init.sql` | `apps/api/src/rekenraam_api/db/models/accounts.py`, `apps/api/src/rekenraam_api/repositories/reconciliation.py`, `apps/api/src/rekenraam_api/services/reconciliation.py` | `/api/v1/reconciliation/accounts/{id}/constraints`, `/api/v1/reconciliation/accounts/{id}/constraints/validation` | baseline complete | `balance_checks`, `balance_adjustments`, `balance_constraints`, and reconciliation preferences are in the Postgres baseline and Python reconciliation slice. Remaining work is policy edge-case coverage. |
| imports | `src-tauri/src/import.rs` | `apps/api/src/rekenraam_api/repositories/imports.py`, `apps/api/src/rekenraam_api/services/imports.py`, `apps/api/src/rekenraam_api/api/v1/imports.py` | `/api/v1/imports*` | baseline complete | CSV, XLS, XLSX, QIF, and OFX/QFX preview, validation, matching, import sessions, rules, audit trail, and commit workflow are Python-backed. Legacy desktop database import is deferred after v1. Remaining work is broader real-bank fixture coverage and import mapping template portability. |
| exports | `src-tauri/src/import.rs`, report/export helpers | `apps/api/src/rekenraam_api/services/exports.py`, `apps/api/src/rekenraam_api/api/v1/exports.py` | `/api/v1/exports/accounts.csv`, `/api/v1/exports/transactions.csv`, `/api/v1/exports/registers/{id}.qif`, `/api/v1/exports/reports/{kind}.csv` | baseline complete | CSV account/transaction export, QIF register export, and report CSV export exist. Remaining work is coverage review for v1 data portability. |
| reports | `src-tauri/src/db_reports.rs` | `apps/api/src/rekenraam_api/repositories/reports.py`, `apps/api/src/rekenraam_api/services/reports.py`, `apps/api/src/rekenraam_api/api/v1/reports.py` | `/api/v1/reports/cashflow`, `/api/v1/reports/category-spend`, `/api/v1/reports/payee-totals`, `/api/v1/reports/realized-gains`, `/api/v1/reports/unrealized-gains`, `/api/v1/reports/definitions`, `/api/v1/reports/runs` | baseline complete | Classic reports plus realized/unrealized gains, saved report definitions, and run metadata are on Python endpoints. Remaining work is v1 report breadth, saved/custom report UX, chart/print views, budget variance, and account statement/income-expense reports. |
| report state and caching | `src-tauri/src/db_reports.rs`, `src-tauri/migrations/V1__init.sql` | `apps/api/src/rekenraam_api/db/models/report_state.py`, `apps/api/src/rekenraam_api/db/models/report_metadata.py`, `apps/api/src/rekenraam_api/repositories/reports.py`, `apps/api/src/rekenraam_api/services/reports.py` | `/api/v1/reports*` | baseline complete | `book_state`, `report_cache`, `report_definitions`, and `report_runs` are in the web baseline. Report-relevant writes bump book change sequence. Remaining work is wider cache/report coverage. |
| commodities metadata including currencies | `src-tauri/src/db_currencies.rs`, `src-tauri/src/db_commodities.rs` | `apps/api/src/rekenraam_api/repositories/metadata.py`, `apps/api/src/rekenraam_api/services/metadata.py`, `apps/api/src/rekenraam_api/api/v1/metadata.py`, pricing modules | `/api/v1/commodities`, `/api/v1/commodities/{id}`, `/api/v1/currencies`, `/api/v1/currencies/{id}`, `/api/v1/currencies/{id}/default`, `/api/v1/currencies/{id}/activation`, `/api/v1/pricing/*` | baseline complete | Commodity list/edit plus currency list/create/update/default/activation are Python-backed. Treat `commodities` as the canonical asset master; currencies are commodity rows with `kind='currency'`. Remaining work is explicit cross-currency transfer UX and broader valuation/history behavior. |
| pricing and FX refresh | `src-tauri/src/fx_refresh.rs`, `src-tauri/src/fx_rates/*` | `apps/api/src/rekenraam_api/repositories/pricing.py`, `apps/api/src/rekenraam_api/services/pricing.py`, `apps/api/src/rekenraam_api/services/pricing_execution.py`, `apps/api/src/rekenraam_api/api/v1/pricing.py`, `apps/api/src/rekenraam_api/workers/pricing.py` | `/api/v1/pricing/sources`, `/api/v1/pricing/policy`, `/api/v1/pricing/source-assignments`, `/api/v1/pricing/refresh-state`, `/api/v1/pricing/refresh/run`, `/api/v1/pricing/refresh/execution-status`, `/api/v1/pricing/refresh/history`, `/api/v1/pricing/rates/*` | baseline complete | Pricing configuration, source assignments, manual rate entry, refresh-state reads, scheduled execution, manual refresh, and persisted run history are Python-backed. Remaining work is historical backfill, source health/status UX, broader market-price ingestion, and valuation snapshots if report workloads need them. |
| investments and lots | `src-tauri/src/db_transactions.rs`, `src-tauri/src/db_accounts.rs` | `apps/api/src/rekenraam_api/repositories/investments.py`, `apps/api/src/rekenraam_api/services/investments.py`, `apps/api/src/rekenraam_api/api/v1/investments.py` | `/api/v1/investments/positions`, `/api/v1/investments/positions/converted`, `/api/v1/investments/lots`, `/api/v1/investments/buy`, `/api/v1/investments/sell`, `/api/v1/investments/dividend` | baseline complete | Python read/write coverage exists for positions, converted positions, holding periods, and buy/sell/dividend flows. Remaining work is reinvested dividends, corporate actions, stricter booking validation, cost-basis policy, and performance reporting. |
| settings and metadata | `src-tauri/src/db_accounts.rs`, `src-tauri/src/db_commodities.rs`, `src-tauri/src/db_storage.rs` | `apps/api/src/rekenraam_api/repositories/metadata.py`, `apps/api/src/rekenraam_api/services/metadata.py`, `apps/api/src/rekenraam_api/api/v1/metadata.py`, pricing/admin modules | metadata routes, `/api/v1/pricing/*`, `/api/v1/admin/*` | baseline complete | Institutions, categories, payees, tags, people, projects, commodity metadata, currency settings, pricing settings, admin runtime, integrity checks, and fiscal-year close are Python-backed. Remaining work is per-user preferences, user admin, audit history, and final desktop-storage replacement docs. |
| notes, events, and documents | `src-tauri/src/db_accounts.rs`, `src-tauri/src/db_transactions.rs`, `src-tauri/migrations/V1__init.sql` | planned note/event/document services | planned `/api/v1/notes*`, `/api/v1/events*`, `/api/v1/documents*` | not started | Legacy schema includes `notes`, `events`, and `documents`. Notes/documents are v1 should-have if they do not delay core workflows; attachments OCR is deferred. |
| budgets | no strong desktop equivalent | planned budget services | planned `/api/v1/budgets*` | not started | V1 must include monthly/annual budgets, category targets, rollover, and planned-vs-actual reporting. |
| scheduled transactions and planning | desktop recurrence/planning references | planned schedule/planning services | planned `/api/v1/schedules*` | not started | V1 must include recurrence, reminders, skip/post flows, and projected cash balance from scheduled transactions. |
| loans and liabilities | account/transaction primitives | planned loan services on top of accounts/transactions | planned `/api/v1/loans*` | not started | V1 must include liability accounts, loans, mortgages, amortization schedules, and loan-payment assistant. |
| plugins and themes | none | planned plugin/theme services | planned `/api/v1/plugins*`, `/api/v1/themes*` | not started | V1 uses trusted server-installed plugin manifests and built-in frontend theme token packs with persisted selection. |
| backup and restore settings | `src-tauri/src/db_storage.rs`, `src-tauri/migrations/V1__init.sql` | server admin docs and operational smoke checks | admin/status endpoints plus docs | in progress | Legacy `backup_settings` is replaced by Postgres-native backup/restore docs, `pg_dump`/`pg_restore`, volume snapshots, and operational status views. |
| desktop runtime session state | `src-tauri/src/db.rs`, `src-tauri/src/session.rs` | FastAPI request context, auth sessions, and devices | `/api/v1/auth/*` | dropped | `app_runtime_session` is a desktop-process marker and is intentionally not ported as a PostgreSQL table. Web session/device/request context replaces it. |
| undo/redo | `src-tauri/src/session.rs` | future server-side mutation history or conscious removal | none yet | dropped | `session_undo_stack`, `session_redo_stack`, and `session_reverts` are intentionally not direct table ports. Undo/redo can return only with a server-safe design. |
| append-only immutability model | `src-tauri/migrations/V1__init.sql` | immutable/versioned row model in PostgreSQL plus services | domain endpoints | in progress | Preserve immutable history, but keep request/session/device audit context separate from domain version chains. Existing account and transaction version links are baseline work; broader documentation and constraints remain. |
| database-side invariant triggers and current-state views | `src-tauri/migrations/V1__init.sql` | PostgreSQL constraints plus service validation, with repository helpers/views where useful | domain endpoints | in progress | Legacy SQLite used triggers and `current_*` views. In PostgreSQL, favor durable constraints for simple facts, service validation for user-facing errors, and ordinary SQL views/query helpers for reusable latest-state reads. |

## Runtime/Session Table Decisions

| Tauri table | Decision | Replacement |
|---|---|---|
| `app_runtime_session` | dropped | request-scoped context plus authenticated web sessions and user devices |
| `session_undo_stack` | dropped as direct port | no direct replacement; redesign only if undo remains a product requirement |
| `session_redo_stack` | dropped as direct port | no direct replacement; redesign only if redo remains a product requirement |
| `session_reverts` | dropped as direct port | no direct replacement; redesign only as part of a server-side undo/audit model |

## Deferred vs Unaccounted

### Intentionally Deferred After V1

| Legacy or product family | Current disposition | Planned area |
|---|---|---|
| legacy desktop database import | deferred | post-v1 migration tooling if still needed |
| PDF statement parsing | deferred | post-v1 import enrichment |
| attachments OCR | deferred | post-v1 document processing |
| PSD2/open banking and online bank connectivity | deferred | post-v1 connectivity |
| remote plugin marketplace or arbitrary downloaded plugin execution | deferred | post-v1 plugin distribution, if ever |
| encrypted-at-rest database packaging | deferred | post-v1 deployment/security packaging |
| country-specific tax filing exports | deferred | post-v1 tax localization |
| server-side undo/redo | deferred/dropped direct port | future mutation-history design only |

### V1 Gaps Without Desktop Equivalents

| Capability | Why it matters |
|---|---|
| budgets and scheduled transactions | expected personal finance breadth and release gate |
| loans, mortgages, and amortization | needed for liability workflows beyond basic accounts |
| search, saved views, and transaction templates | daily-use ergonomics and data-entry speed |
| per-user preferences | required for self-hosted multi-user comfort and theme persistence |
| user administration and book role management | required beyond first-admin bootstrap |
| plugin/theme manifests | v1 extension and theming model |
| backup/restore smoke checks | production self-hosting confidence |

## current_* View Direction

| Legacy view | Current purpose in SQLite | Web migration target | Decision status |
|---|---|---|---|
| `current_commodities` | latest-version projection over commodity rows | ordinary SQL view or repository helper; commodities remain the canonical asset/currency surface | decided |
| `current_price_observations` | latest active price observation after supersession chaining | ordinary SQL view or repository helper first; materialize only if valuation workloads prove it necessary | decided |
| `current_pricing_policies` | current policy projection over append-only pricing-policy versions | ordinary SQL view or repository helper | decided |
| `current_pricing_source_assignments` | current effective pricing-source assignment over version chain | ordinary SQL view or repository filters for effective-date resolution | decided |
| `current_book_base_currency_history` | current base-currency assignment over append-only history | repository helper or future view that resolves active base commodity for a requested date | decided |

## Invariant Placement Direction

| Legacy trigger family | Current SQLite responsibility | Web migration target | Decision status |
|---|---|---|---|
| account lifecycle/date guard | validate account effective dates and lifecycle/is-closed consistency | service prevalidation plus PostgreSQL constraints/checks where expressible | decided direction |
| transaction datetime guard | validate transaction dates, UTC timestamps, timezone, and created-at shapes | schema/service validation first, with PostgreSQL constraints where durable and clear | decided direction |
| import session datetime guard | validate import-session and import-row timestamp shapes | schema/service validation first, with PostgreSQL constraints where useful | decided direction |
| account/split/category/lot same-book invariants | keep cross-table rows inside the same book | prefer PostgreSQL-enforced relationships where feasible, otherwise transactional service validation | decided direction |
| split commodity/account invariant | ensure split commodity matches account commodity rules | service validation plus durable constraints where schema allows | decided direction |
| commodity scale bounds | enforce precision bounds | PostgreSQL check constraints plus service prevalidation for better error reporting | decided direction |
| price source existence guard | ensure referenced price source exists | direct PostgreSQL foreign-key style enforcement | decided direction |
| report state bootstrap and cache invalidation | maintain `book_state.change_seq` and report cache freshness | explicit service invalidation first; database triggers only if correctness risk warrants them | decided direction |
| account system-role guards | restrict valid system roles and protect system accounts | PostgreSQL constraints/checks plus service prevalidation | decided direction |

## PostgreSQL/SQLAlchemy Direction

- Model currencies as commodity rows with `kind='currency'`; do not carry
  forward a separate canonical `currencies` table.
- Keep immutable domain history in append-only versioned tables or an equivalent
  history-preserving model.
- Expose latest-state reads through ordinary SQL views or repository query
  helpers; materialized projections require measured need.
- Keep audit attribution separate from business version chains. Stamp writes
  with authenticated user, session, request, and device context where the domain
  records audit details.
- Use PostgreSQL constraints, foreign keys, and partial indexes for invariants
  the database can own safely; keep richer cross-row UX validation in service
  code so API errors remain understandable.
