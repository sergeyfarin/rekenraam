# Desktop To Python Parity Matrix

Last updated: 2026-05-12

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
| auth identity, sessions, and book access | `src-tauri/src/db.rs`, `src-tauri/src/session.rs` | `apps/api/src/rekenraam_api/repositories/access.py`, `apps/api/src/rekenraam_api/services/auth.py`, `apps/api/src/rekenraam_api/services/access.py`, `apps/api/src/rekenraam_api/services/request_context.py`, `apps/api/src/rekenraam_api/api/v1/auth.py` | `/api/v1/auth/*`, protected `/api/v1/*` routers | baseline complete | First-admin bootstrap, login/logout, user devices, auth sessions, request context, book membership enforcement, password change for the authenticated user, admin password reset, deactivate session revocation, MFA (TOTP + recovery codes), self-service password reset (`/api/v1/auth/password-reset/{request,confirm}` with 24h single-use tokens, anti-enumeration response, and session sweep on confirm), and admin-issued user invites (`/api/v1/admin/invites` + `/api/v1/auth/invite/accept` with 7-day single-use tokens, idempotent re-invite, optional role memberships) exist. Temporary v1 single-book support for the web runtime is centralized in `services/access.py` via `SUPPORTED_V1_BOOK_ID`; do not reintroduce scattered `book_id = 1` literals in migrated Python paths. Remaining work is explicit deactivate/reactivate semantics distinct from pending state, role management UI polish, and user-visible audit trails. |
| accounts read-only list/detail | `src-tauri/src/db_accounts.rs` | `apps/api/src/rekenraam_api/repositories/accounts.py`, `apps/api/src/rekenraam_api/services/accounts.py`, `apps/api/src/rekenraam_api/api/v1/accounts.py` | `/api/v1/accounts`, `/api/v1/accounts/{id}` | verified | Flat list/detail slice is migrated and tested. |
| account tree and balances | `src-tauri/src/db_accounts.rs`, `src/routes/accounts/AccountTreeItem.svelte` | `apps/api/src/rekenraam_api/services/accounts.py`, `apps/api/src/rekenraam_api/api/v1/accounts.py` | `/api/v1/accounts/tree`, `/api/v1/accounts/balances` | baseline complete | Tree shape and transaction-backed balances exist. Remaining work is broader high-volume parity coverage and richer saved/filter views. |
| accounts writes and directives | `src-tauri/src/db_accounts.rs` | `apps/api/src/rekenraam_api/repositories/accounts.py`, `apps/api/src/rekenraam_api/services/accounts.py`, `apps/api/src/rekenraam_api/api/v1/accounts.py` | `/api/v1/accounts`, `/api/v1/accounts/{id}`, `/api/v1/accounts/{id}/directives`, `/api/v1/accounts/{id}/closing-validation`, `/api/v1/accounts/{id}/booking-policy` | baseline complete | Create/update/delete, close validation, directives, booking policy, and system-account protections exist. Remaining work is additional invariants and parity fixtures. |
| transactions and splits | `src-tauri/src/db_transactions.rs` | `apps/api/src/rekenraam_api/repositories/transactions.py`, `apps/api/src/rekenraam_api/services/transactions.py`, `apps/api/src/rekenraam_api/api/v1/transactions.py` | `/api/v1/transactions`, `/api/v1/transactions/{id}`, `/api/v1/accounts/{id}/register`, `/api/v1/transactions/bulk-void`, `/api/v1/transactions/bulk-delete`, `/api/v1/transactions/{id}/duplicate`, `/api/v1/transactions/payee-defaults` | baseline complete | Read/write list/detail/register flows, duplicate/bulk actions, locked-range checks, split support, cursor pagination, payee-default helpers, zero-sum validation, and account/commodity matching are on Python endpoints. Remaining work is deeper correctness parity, search/saved views, templates, memorized splits, and explicit cross-currency transfer workflows. |
| reconciliation | `src-tauri/src/db_transactions.rs`, `src-tauri/src/db_accounts.rs` | `apps/api/src/rekenraam_api/repositories/reconciliation.py`, `apps/api/src/rekenraam_api/services/reconciliation.py`, `apps/api/src/rekenraam_api/api/v1/reconciliation.py` | `/api/v1/reconciliation/accounts/{id}/start`, `/api/v1/reconciliation/accounts/{id}/finish`, `/api/v1/reconciliation/accounts/{id}/history`, `/api/v1/reconciliation/accounts/{id}/unlock` | baseline complete | Reconciliation start/finish/history/unlock flows exist and the account reconciliation page uses HTTP. Remaining work is broader statement fixtures, account-type coverage, and audit review depth. |
| balance checks and adjustments | `src-tauri/src/db_accounts.rs`, `src-tauri/migrations/V1__init.sql` | `apps/api/src/rekenraam_api/db/models/accounts.py`, `apps/api/src/rekenraam_api/repositories/reconciliation.py`, `apps/api/src/rekenraam_api/services/reconciliation.py` | `/api/v1/reconciliation/accounts/{id}/constraints`, `/api/v1/reconciliation/accounts/{id}/constraints/validation` | baseline complete | `balance_checks`, `balance_adjustments`, `balance_constraints`, and reconciliation preferences are in the Postgres baseline and Python reconciliation slice. Remaining work is policy edge-case coverage. |
| imports | `src-tauri/src/import.rs` | `apps/api/src/rekenraam_api/repositories/imports.py`, `apps/api/src/rekenraam_api/services/imports.py`, `apps/api/src/rekenraam_api/api/v1/imports.py` | `/api/v1/imports*` | baseline complete | CSV, XLS, XLSX, QIF, OFX/QFX, and HBCI/MT940 preview, validation, matching, import sessions, rules, audit trail, and commit workflow are Python-backed. Legacy desktop database import is deferred after v1. Remaining work is broader real-bank fixture coverage and import mapping template portability. |
| exports | `src-tauri/src/import.rs`, report/export helpers | `apps/api/src/rekenraam_api/services/exports.py`, `apps/api/src/rekenraam_api/api/v1/exports.py` | `/api/v1/exports/accounts.csv`, `/api/v1/exports/transactions.csv`, `/api/v1/exports/registers/{id}.qif`, `/api/v1/exports/reports/{kind}.csv` | baseline complete | CSV account/transaction export, QIF register export, and report CSV export exist. Remaining work is coverage review for v1 data portability. |
| reports | `src-tauri/src/db_reports.rs` | `apps/api/src/rekenraam_api/repositories/reports.py`, `apps/api/src/rekenraam_api/services/reports.py`, `apps/api/src/rekenraam_api/api/v1/reports.py` | `/api/v1/reports/cashflow`, `/api/v1/reports/category-spend`, `/api/v1/reports/payee-totals`, `/api/v1/reports/net-worth`, `/api/v1/reports/account-trends`, `/api/v1/reports/realized-gains`, `/api/v1/reports/unrealized-gains`, `/api/v1/reports/investment-performance`, `/api/v1/reports/account-valuation`, `/api/v1/reports/currency-exposure`, `/api/v1/reports/corporate-actions`, `/api/v1/reports/definitions`, `/api/v1/reports/runs` | baseline complete | Classic reports, net worth, account trends, policy-aware realized/unrealized gains, investment performance, converted account valuation, unconverted currency exposure, saved report definitions, and run metadata are on Python endpoints. Remaining work is saved/custom report UX, chart/print views, budget variance, and account statement/income-expense reports. |
| report state and caching | `src-tauri/src/db_reports.rs`, `src-tauri/migrations/V1__init.sql` | `apps/api/src/rekenraam_api/db/models/report_state.py`, `apps/api/src/rekenraam_api/db/models/report_metadata.py`, `apps/api/src/rekenraam_api/repositories/reports.py`, `apps/api/src/rekenraam_api/services/reports.py` | `/api/v1/reports*` | baseline complete | `book_state`, `report_cache`, `report_definitions`, and `report_runs` are in the web baseline. Report-relevant writes bump book change sequence. Remaining work is wider cache/report coverage. |
| commodities metadata including currencies | `src-tauri/src/db_currencies.rs`, `src-tauri/src/db_commodities.rs` | `apps/api/src/rekenraam_api/repositories/metadata.py`, `apps/api/src/rekenraam_api/services/metadata.py`, `apps/api/src/rekenraam_api/api/v1/metadata.py`, pricing modules | `/api/v1/commodities`, `/api/v1/commodities/{id}`, `/api/v1/currencies`, `/api/v1/currencies/{id}`, `/api/v1/currencies/{id}/default`, `/api/v1/currencies/{id}/activation`, `/api/v1/pricing/*` | baseline complete | Commodity list/edit plus currency list/create/update/default/activation are Python-backed. Treat `commodities` as the canonical asset master; currencies are commodity rows with `kind='currency'`. Remaining work is explicit cross-currency transfer UX and broader valuation/history behavior. |
| pricing and FX refresh | `src-tauri/src/fx_refresh.rs`, `src-tauri/src/fx_rates/*` | `apps/api/src/rekenraam_api/repositories/pricing.py`, `apps/api/src/rekenraam_api/services/pricing.py`, `apps/api/src/rekenraam_api/services/pricing_execution.py`, `apps/api/src/rekenraam_api/api/v1/pricing.py`, `apps/api/src/rekenraam_api/workers/pricing.py` | `/api/v1/pricing/sources`, `/api/v1/pricing/policy`, `/api/v1/pricing/source-assignments`, `/api/v1/pricing/refresh-state`, `/api/v1/pricing/source-health`, `/api/v1/pricing/refresh/run`, `/api/v1/pricing/refresh/execution-status`, `/api/v1/pricing/refresh/history`, `/api/v1/pricing/rates/*`, `/api/v1/pricing/market-prices` | baseline complete | Pricing configuration, source assignments, manual FX/market-price entry, refresh-state reads, source health, scheduled execution, manual refresh, and persisted run history are Python-backed. Remaining work is broader automated market-price providers and valuation snapshots if report workloads need them. |
| investments and lots | `src-tauri/src/db_transactions.rs`, `src-tauri/src/db_accounts.rs` | `apps/api/src/rekenraam_api/repositories/investments.py`, `apps/api/src/rekenraam_api/services/investments.py`, `apps/api/src/rekenraam_api/api/v1/investments.py` | `/api/v1/investments/instruments`, `/api/v1/investments/cost-basis-profiles`, `/api/v1/investments/corporate-actions`, `/api/v1/investments/positions`, `/api/v1/investments/positions/converted`, `/api/v1/investments/lots`, `/api/v1/investments/buy`, `/api/v1/investments/sell`, `/api/v1/investments/short-sell`, `/api/v1/investments/short-cover`, `/api/v1/investments/dividend`, `/api/v1/investments/reinvested-dividend`, `/api/v1/investments/events`, `/api/v1/investments/performance`, `/api/v1/investments/account-valuation`, `/api/v1/investments/currency-exposure` | baseline complete | Python read/write coverage exists for instrument master data, cost-basis profiles, positions, converted positions, holding periods, buy/sell/short/dividend/reinvested-dividend flows, structured corporate actions, generic derivative/private investment events, performance, account valuation, and unconverted currency exposure. Remaining work is deeper automated accounting for complex derivative/private/corporate-action scenarios. |
| settings and metadata | `src-tauri/src/db_accounts.rs`, `src-tauri/src/db_commodities.rs`, `src-tauri/src/db_storage.rs` | `apps/api/src/rekenraam_api/repositories/metadata.py`, `apps/api/src/rekenraam_api/services/metadata.py`, `apps/api/src/rekenraam_api/api/v1/metadata.py`, pricing/admin modules | metadata routes, `/api/v1/pricing/*`, `/api/v1/admin/*` | baseline complete | Institutions, categories, payees, tags, people, projects, commodity metadata, currency settings, pricing settings, admin runtime, integrity checks, and fiscal-year close are Python-backed. Remaining work is per-user preferences, user admin, audit history, and final desktop-storage replacement docs. |
| notes, events, and documents | `src-tauri/src/db_accounts.rs`, `src-tauri/src/db_transactions.rs`, `src-tauri/migrations/V1__init.sql` | `apps/api/src/rekenraam_api/services/ergonomics.py`, notes routes | `/api/v1/notes*` | baseline complete for notes; deferred for documents/events | Markdown notes for accounts and transactions are Python-backed. The old desktop `create_event`/`list_events` and `create_document`/`list_documents` command families are not direct Python ports; audit events replace system history, while user documents, attachment storage, OCR, and any separate event model remain deferred. |
| search, saved views, templates, and preferences | no direct single desktop equivalent | `apps/api/src/rekenraam_api/services/ergonomics.py`, search/template/preferences routes | `/api/v1/search/transaction-views*`, `/api/v1/templates/transactions*`, `/api/v1/preferences*` | baseline complete | Saved transaction views, transaction templates, persisted payee defaults, user preferences, and compact frontend controls exist. Remaining work is daily-use polish. |
| budgets | no strong desktop equivalent | `apps/api/src/rekenraam_api/services/planning.py`, planning models/repository | `/api/v1/budgets*` | baseline complete | Monthly/annual budgets, category targets, rollover calculation, and planned-vs-actual reporting are Python-backed. Remaining work is UX polish, broader validation, and fixture coverage. |
| scheduled transactions and planning | desktop recurrence/planning references | `apps/api/src/rekenraam_api/services/planning.py`, planning models/repository | `/api/v1/schedules*` | baseline complete | Recurrence, reminders, projected instances, skip/post flows, and projected cash balance from scheduled transactions are Python-backed. Remaining work is richer recurrence edge cases and editing future occurrences. |
| loans and liabilities | account/transaction primitives | `apps/api/src/rekenraam_api/services/planning.py`, planning models/repository | `/api/v1/loans*` | baseline complete | Fixed-rate monthly loans/mortgages, amortization schedules, and loan-payment assistant are Python-backed. Remaining work is richer liability validation and advanced loan scenarios. |
| plugins and themes | none | future plugin/theme architecture | reserved future `/api/v1/plugins*`, `/api/v1/themes*` | deferred | Post-b1 work may add sandboxed WASM plugins, isolated arbitrary-language sidecars, granular permission grants, manifest-driven frontend slots, and theme token packs. B1 keeps compatibility guardrails, semantic CSS tokens, negative namespace tests, and the persisted `theme` preference. |
| backup and restore settings | `src-tauri/src/db_storage.rs`, `src-tauri/migrations/V1__init.sql` | server admin docs and operational smoke checks | admin/status endpoints plus docs | baseline complete | Legacy `backup_settings` is replaced by Postgres-native backup/restore docs, `pg_dump`/`pg_restore`, optional volume snapshots, a Compose backup profile, restore smoke commands, and operational status views. |
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

## Final Deletion Audit, 2026-05-18

- Registered Rust desktop command surface checked from
  `src-tauri/src/lib.rs`: 209 commands total, split across 182 `commands::*`,
  25 `db_currencies::*`, and 2 `fx_refresh::*` commands.
- A second-pass Rust function/helper audit is captured in
  [tauri-rust-function-audit-2026-05-18.md](./tauri-rust-function-audit-2026-05-18.md).
  That pass reviewed private helpers and tests in addition to command names.
- Active web runtime checked: no `@tauri`, `invoke`, `__TAURI__`, or Tauri API
  imports remain outside `src-tauri`, docs/TODO, `.vscode`, `.dockerignore`,
  and an unreferenced `static/tauri.svg` starter asset.
  `package.json`, `package-lock.json`, `vite.config.js`, and
  `svelte.config.js` are Tauri-free.
- Directly replaced command families: accounts, balances, reconciliation,
  transactions/splits, imports by uploaded content, import rules/sessions,
  exports, metadata settings, commodities/currencies, manual FX and pricing
  refresh, investments/lots/dividends/corporate-action records, built-in
  reports, saved report definitions/runs, notes, auth/session/admin context,
  health/runtime status, backup/restore docs and smoke workflows.
- Intentionally dropped or deferred command families: desktop storage location
  management and file pickers, desktop SQLite backup scheduling, desktop
  undo/redo tables, event CRUD, document/attachment CRUD, legacy desktop DB
  import/path helpers, transaction timestamp/timezone API fields, generic
  SQL/template `run_report`, report-run pruning, country create/update/delete,
  commodity price-source override CRUD, dividend income category defaults, and
  the richer legacy pricing-history fields listed in the V1 gap plan.
- Delete decision: the Rust tree is no longer required by the active web
  runtime. It is safe to delete only after product sign-off accepts the
  dropped/deferred command families above and the tiny non-runtime cleanup
  references are removed.

## Deferred vs Unaccounted

### Intentionally Deferred After V1

| Legacy or product family | Current disposition | Planned area |
|---|---|---|
| legacy desktop database import | deferred | post-v1 migration tooling if still needed |
| PDF statement parsing | deferred | post-v1 import enrichment |
| attachments OCR | deferred | post-v1 document processing |
| PSD2/open banking and online bank connectivity | deferred | post-v1 connectivity |
| trusted plugin execution and frontend plugin slots | deferred | post-b1 extension architecture |
| theme token packs beyond persisted preference | deferred | post-b1 theming architecture |
| granular plugin permissions, GitHub-sourced manifests, and Extism/WASM evaluation | deferred | post-b1 plugin runtime design |
| remote plugin marketplace or arbitrary downloaded plugin execution | deferred | post-v1 plugin distribution, if ever |
| encrypted-at-rest database packaging | deferred | post-v1 deployment/security packaging |
| country-specific tax summaries and filing exports | deferred | post-v1 tax localization, likely via trusted plugins or optional modules |
| server-side undo/redo | deferred/dropped direct port | future mutation-history design only |

### V1 Gaps Without Desktop Equivalents

| Capability | Why it matters |
|---|---|
| budgets and scheduled transactions | expected personal finance breadth and release gate |
| loans, mortgages, and amortization | needed for liability workflows beyond basic accounts |
| search, saved views, and transaction templates | daily-use ergonomics and data-entry speed |
| per-user preferences | required for self-hosted multi-user comfort; the persisted `theme` field also preserves a later theming path |
| user administration and book role management | required beyond first-admin bootstrap |
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
