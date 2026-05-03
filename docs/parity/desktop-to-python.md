# Desktop To Python Parity Matrix

Last updated: 2026-05-03

Status values:

- `not started`
- `in progress`
- `verified`
- `dropped`

This tracks migration status at the capability-group level first. As deeper slices
move, each row can be expanded into per-command or per-use-case entries.

| Desktop capability | Current Rust source | Target Python modules | Target endpoint(s) | Status | Notes |
|---|---|---|---|---|---|
| storage and migration bootstrap | `src-tauri/src/db.rs`, `src-tauri/src/db_storage.rs` | `apps/api/alembic`, `apps/api/src/rekenraam_api/config/settings.py`, `apps/api/src/rekenraam_api/db/*` | `/api/v1/health` | in progress | The empty app now uses one current initial Alembic schema. Tauri desktop runtime/session tables are intentionally excluded from the Postgres baseline; SQLite import and operational admin flows are not migrated yet. |
| books read-only | `src-tauri/src/db_currencies.rs`, `src-tauri/src/db_commodities.rs`, book bootstrap in `src-tauri/src/db.rs` | `apps/api/src/rekenraam_api/repositories/books.py`, `apps/api/src/rekenraam_api/services/books.py`, `apps/api/src/rekenraam_api/api/v1/books.py` | `/api/v1/books`, `/api/v1/books/{slug}` | verified | Covered by API tests, repository tests, and Docker smoke tests. |
| accounts read-only list/detail | `src-tauri/src/db_accounts.rs` | `apps/api/src/rekenraam_api/repositories/accounts.py`, `apps/api/src/rekenraam_api/services/accounts.py`, `apps/api/src/rekenraam_api/api/v1/accounts.py` | `/api/v1/accounts`, `/api/v1/accounts/{id}` | verified | Flat list/detail slice is migrated and tested. |
| account tree and balances | `src-tauri/src/db_accounts.rs`, `src/routes/accounts/AccountTreeItem.svelte` | `apps/api/src/rekenraam_api/services/accounts.py`, `apps/api/src/rekenraam_api/api/v1/accounts.py` | `/api/v1/accounts/tree` | in progress | Tree shape and transaction-backed seeded balances exist; richer metadata, filters, and parity coverage are still pending. |
| transactions and splits | `src-tauri/src/db_transactions.rs` | `apps/api/src/rekenraam_api/repositories/transactions.py`, `apps/api/src/rekenraam_api/services/transactions.py`, `apps/api/src/rekenraam_api/api/v1/transactions.py` | `/api/v1/transactions`, `/api/v1/transactions/{id}`, `/api/v1/accounts/{id}/register` | in progress | Read-only list/detail with nested splits and account register running balances are implemented and tested; filtering, richer register data, and mutation flows are still pending. |
| investments and lots | `src-tauri/src/db_transactions.rs`, `src-tauri/src/db_accounts.rs` | planned investment and lots services | planned `/api/v1/investments*`, `/api/v1/lots*` | not started | Legacy schema includes `lots`, `split_lot_allocations`, `corporate_actions`, and `dividend_income_categories`; none are in the Postgres baseline yet. |
| reports | `src-tauri/src/db_reports.rs` | planned `apps/api/src/rekenraam_api/services/reports.py`, `api/v1/reports.py` | planned `/api/v1/reports*` | not started | Read-only metadata and simple reports should move before calculation-heavy reports. |
| report state and caching | `src-tauri/src/db_reports.rs`, `src-tauri/migrations/V1__init.sql` | planned report services plus cache invalidation strategy | planned `/api/v1/reports*` | not started | Legacy schema includes `book_state`, `report_cache`, `report_definitions`, and `report_runs`. `book_state` and `report_cache` should be migrated even before a frontend depends on them. |
| commodities metadata including currencies | `src-tauri/src/db_currencies.rs`, `src-tauri/src/db_commodities.rs` | `apps/api/src/rekenraam_api/repositories/metadata.py`, `apps/api/src/rekenraam_api/services/metadata.py`, `apps/api/src/rekenraam_api/api/v1/metadata.py`, `apps/api/src/rekenraam_api/repositories/pricing.py`, `apps/api/src/rekenraam_api/services/pricing.py`, `apps/api/src/rekenraam_api/services/pricing_execution.py`, `apps/api/src/rekenraam_api/api/v1/pricing.py`, `apps/api/src/rekenraam_api/workers/pricing.py` | `/api/v1/commodities`, `/api/v1/commodities/{id}`, `/api/v1/currencies`, `/api/v1/currencies/{id}`, `/api/v1/currencies/{id}/default`, `/api/v1/currencies/{id}/activation`, `/api/v1/pricing/sources`, `/api/v1/pricing/policy`, `/api/v1/pricing/source-assignments`, `/api/v1/pricing/refresh-state`, `/api/v1/pricing/refresh/run`, `/api/v1/pricing/refresh/execution-status` | in progress | Commodity list/edit plus currency list/create/update/default/activation are now Python-backed. Active state is stored in `commodities.metadata_text` JSON, the default currency is always active, and inactive currencies remain visible in settings but are excluded from FX entry and refresh flows. The FX settings tab now uses Python-backed pricing endpoints for sources, persisted policy, source assignments, refresh-state reads, manual refresh, and execution status in web mode. Scheduled FX and market-price execution now runs in a backend-owned worker. Treat `commodities` as the canonical asset master. Currencies should be modeled as commodity rows with `kind='currency'` rather than reintroducing a separate `currencies` table in PostgreSQL. |
| reconciliation | `src-tauri/src/db_transactions.rs`, `src-tauri/src/db_accounts.rs` | planned reconciliation service slice | planned `/api/v1/reconciliation*` | not started | Must follow transaction correctness migration. |
| balance checks and adjustments | `src-tauri/src/db_accounts.rs`, `src-tauri/migrations/V1__init.sql` | planned reconciliation services | planned `/api/v1/reconciliation*` | not started | Legacy schema includes `balance_checks`, `balance_adjustments`, and `balance_constraints`; none are in the Postgres baseline yet. |
| imports | `src-tauri/src/import.rs` | planned import service and worker slice | planned `/api/v1/imports*` | not started | SQLite-to-Postgres import also depends on this area. |
| pricing and FX refresh | `src-tauri/src/fx_refresh.rs`, `src-tauri/src/fx_rates/*` | `apps/api/src/rekenraam_api/repositories/pricing.py`, `apps/api/src/rekenraam_api/services/pricing.py`, `apps/api/src/rekenraam_api/services/pricing_execution.py`, `apps/api/src/rekenraam_api/api/v1/pricing.py`, `apps/api/src/rekenraam_api/workers/pricing.py` | `/api/v1/pricing/sources`, `/api/v1/pricing/policy`, `/api/v1/pricing/source-assignments`, `/api/v1/pricing/refresh-state`, `/api/v1/pricing/refresh/run`, `/api/v1/pricing/refresh/execution-status` | in progress | Pricing configuration, source assignments, refresh-state reads, scheduled execution, and manual refresh are now Python-backed. Current provider coverage in the worker starts with ECB, Federal Reserve, and Bank of Canada; the remaining gap is expanding provider coverage and broader market-price ingestion beyond the initial FX execution slice. |
| pricing history and valuation state | `src-tauri/src/fx_refresh.rs`, `src-tauri/src/db_currencies.rs`, `src-tauri/migrations/V1__init.sql` | `apps/api/src/rekenraam_api/repositories/pricing.py`, `apps/api/src/rekenraam_api/services/pricing.py`, `apps/api/src/rekenraam_api/services/pricing_execution.py`, `apps/api/src/rekenraam_api/api/v1/pricing.py`, `apps/api/src/rekenraam_api/workers/pricing.py`, planned valuation slice | `/api/v1/pricing/sources`, `/api/v1/pricing/policy`, `/api/v1/pricing/source-assignments`, `/api/v1/pricing/refresh-state`, `/api/v1/pricing/refresh/run`, `/api/v1/pricing/refresh/execution-status`, planned valuation endpoints later | in progress | The Postgres baseline now includes `price_sources`, `pricing_policies`, `pricing_source_assignments`, `pricing_refresh_state`, and `price_observations` support for scheduled `fx_daily` rows. Valuation snapshots, `price_ingest_runs`, `commodity_price_sources`, and base-currency history are still pending. |
| settings and metadata | `src-tauri/src/db_accounts.rs`, `src-tauri/src/db_commodities.rs`, `src-tauri/src/db_storage.rs` | `apps/api/src/rekenraam_api/repositories/metadata.py`, `apps/api/src/rekenraam_api/services/metadata.py`, `apps/api/src/rekenraam_api/api/v1/metadata.py`, `apps/api/src/rekenraam_api/repositories/pricing.py`, `apps/api/src/rekenraam_api/services/pricing.py`, `apps/api/src/rekenraam_api/services/pricing_execution.py`, `apps/api/src/rekenraam_api/api/v1/pricing.py`, `apps/api/src/rekenraam_api/workers/pricing.py`, planned admin/settings modules later | `/api/v1/commodities`, `/api/v1/currencies`, `/api/v1/countries`, `/api/v1/institutions`, `/api/v1/categories`, `/api/v1/payees`, `/api/v1/tags`, `/api/v1/people`, `/api/v1/projects`, `/api/v1/pricing/sources`, `/api/v1/pricing/policy`, `/api/v1/pricing/source-assignments`, `/api/v1/pricing/refresh-state`, `/api/v1/pricing/refresh/run`, `/api/v1/pricing/refresh/execution-status` | in progress | Institutions, categories, payees, tags, people, projects, commodity metadata, currency list/create/update/default/activation, FX settings persistence, and backend-owned pricing execution are on Python endpoints. Remaining settings-side migration work is concentrated in app-level storage/admin flows plus broader provider and valuation coverage. |
| notes, events, and documents | `src-tauri/src/db_accounts.rs`, `src-tauri/src/db_transactions.rs`, `src-tauri/migrations/V1__init.sql` | planned note/event/document services | planned `/api/v1/notes*`, `/api/v1/events*`, `/api/v1/documents*` | not started | Legacy schema includes `notes`, `events`, and `documents`; they should be migrated even before a frontend depends on them. |
| backup and restore settings | `src-tauri/src/db_storage.rs`, `src-tauri/migrations/V1__init.sql` | planned server backup/restore replacement | planned admin or settings endpoints | not started | Legacy schema includes `backup_settings`; the web plan calls for replacement, not direct carry-over. |
| desktop runtime session state | `src-tauri/src/db.rs`, `src-tauri/src/session.rs` | FastAPI request context, later auth/session handling with persisted audit sessions | none | dropped | `app_runtime_session` is a desktop-process marker and is intentionally not ported as a PostgreSQL table. Replace it with web session handling plus persisted session/device audit context. |
| undo/redo | `src-tauri/src/session.rs` | future server-side mutation history or conscious removal | none yet | not started | `session_undo_stack`, `session_redo_stack`, and `session_reverts` are intentionally dropped from Stage 2. If undo/redo survives as a product feature, it needs a new server-safe design rather than a direct table-for-table port. |
| append-only immutability model | `src-tauri/migrations/V1__init.sql` | planned immutable/versioned row model in PostgreSQL plus services | none yet | not started | Preserve immutable history, but implement it idiomatically in PostgreSQL: domain tables keep append-only version rows, while request attribution and audit metadata move into explicit user/session/device audit tables instead of staying mixed into desktop-specific session mechanics. |
| database-side invariant triggers and current-state views | `src-tauri/migrations/V1__init.sql` | PostgreSQL constraints plus service validation, with SQL views as the default current-state projection | none yet | not started | Legacy SQLite uses triggers and `current_*` views for same-book checks, datetime validation, cache bumping, and latest-version projections. In PostgreSQL, favor SQL constraints for durable invariants, service validation for user-facing errors, and ordinary SQL views for reusable latest-version reads; only introduce materialized projections when measured workloads justify them. |
| audit identity | `src-tauri/src/db.rs` | FastAPI auth plus persistent audit session/device trail | none yet | not started | Replace `audit_user` with logged-in user identity and record not only who changed data, but also which session and which device initiated the change. |

## Runtime/Session Table Decisions

| Tauri table | Decision | Replacement |
|---|---|---|
| `app_runtime_session` | dropped | request-scoped context now; authenticated web sessions plus persisted audit sessions later in Stage 8 |
| `session_undo_stack` | dropped from Stage 2 | no direct replacement; redesign only if undo remains a product requirement |
| `session_redo_stack` | dropped from Stage 2 | no direct replacement; redesign only if redo remains a product requirement |
| `session_reverts` | dropped from Stage 2 | no direct replacement; redesign only as part of a server-side undo/audit model |

## Deferred vs Unaccounted

### Intentionally Deferred To Later Web Stages

| Legacy schema family | Current disposition | Planned stage or area |
|---|---|---|
| `lots`, `split_lot_allocations`, `corporate_actions`, `dividend_income_categories` | deferred | Stage 7 complex domains: investments and lots |
| `price_ingest_runs`, `price_observations`, `price_sources`, `commodity_price_sources`, `pricing_policies`, `pricing_source_assignments`, `pricing_refresh_state`, `valuation_snapshots`, `valuation_snapshot_items`, `book_base_currency_history` | deferred | Stage 7 complex domains: pricing / FX refresh |
| `import_rules`, `import_sessions`, `import_session_transactions` | deferred | Stage 6 and Stage 7 import migration |
| `book_state`, `report_cache` | deferred | Stage 7 reports, even if no frontend depends on them yet |
| `report_definitions`, `report_runs` | deferred | Stage 7 reports |
| `balance_checks`, `balance_adjustments`, `balance_constraints` | deferred | Stage 6 reconciliation and correctness-driven write migration |
| `notes`, `events`, `documents` | deferred | later backend parity work, even if frontend is still absent |
| `backup_settings` | deferred | Stage 11 replacement of desktop-only backup model |
| `app_runtime_session`, `session_undo_stack`, `session_redo_stack`, `session_reverts` | intentionally dropped from Stage 2 | Stage 8 request/auth sessions for runtime context; separate redesign decision for undo/redo |
| append-only/version-chain model (`previous_*`, append-only semantics) | required architecture, not dropped | preserve time-machine behavior as the web write model rather than reverting to mutable rows; separate request/session/device audit context from the domain version chain |
| legacy `currencies` table | replace, not port 1:1 | fold currency semantics into `commodities(kind='currency')` so one immutable asset model covers cash currencies and non-currency instruments |

### Gaps That Still Need Explicit Decision

| Legacy schema or invariant | Why it is still unresolved |
|---|---|
| `current_*` views | The default direction is now ordinary SQL views mapped into repository/query helpers for reusable latest-version reads. Materialized projections remain optional only for measured performance hotspots. |
| invariant trigger placement | The target direction is mixed PostgreSQL controls plus service-layer validation for better UX, but the exact split is still not documented trigger family by trigger family. |

## current_* View Decision Table

| Legacy view | Current purpose in SQLite | Likely web migration target | Decision status |
|---|---|---|---|
| `current_commodities` | latest-version projection over append-only `commodities` rows via `previous_commodity_id` | ordinary SQL view mapped read-only in SQLAlchemy/repositories; this is the canonical latest-commodity surface, including currencies | decided |
| `current_price_observations` | latest active price observation after supersession chaining | ordinary SQL view first, with repository helpers for common pricing lookups; materialize only if valuation workloads prove it necessary | decided |
| `current_pricing_policies` | current policy projection over append-only pricing-policy versions | ordinary SQL view mapped read-only in SQLAlchemy/repositories | decided |
| `current_pricing_source_assignments` | current effective pricing-source assignment over version chain | ordinary SQL view plus repository filters for effective-date resolution | decided |
| `current_book_base_currency_history` | current base-currency assignment over append-only history | ordinary SQL view plus repository helper that resolves the active base commodity for a requested date | decided |

## Invariant Trigger Decision Table

| Legacy trigger family | Trigger names | Current SQLite responsibility | Likely web migration target | Decision status |
|---|---|---|---|---|
| account lifecycle/date guard | `trg_accounts_effective_date_format_ins` | validate `effective_at` date shape and lifecycle/is_closed consistency | service prevalidation plus PostgreSQL constraints/checks where expressible | explicit decision required |
| transaction datetime guard | `trg_transactions_datetime_format_ins` | validate transaction date, UTC timestamp, timezone, and created-at shapes | service/schema validation first, with PostgreSQL constraints only where durable and clear | explicit decision required |
| import session datetime guard | `trg_import_sessions_datetime_format_ins`, `trg_import_sessions_datetime_format_upd`, `trg_import_session_transactions_datetime_format_ins` | validate import-session and import-session-transaction timestamp shapes | service/schema validation first, with PostgreSQL constraints only where useful | explicit decision required |
| account commodity same-book invariant | `trg_accounts_commodity_book_ins`, `trg_accounts_commodity_book_upd` | ensure an account's commodity belongs to the same book | prefer PostgreSQL-enforced relational integrity if schema allows it, otherwise transactional service validation | explicit decision required |
| split commodity/account invariant | `trg_splits_commodity_matches_account_ins`, `trg_splits_commodity_matches_account_upd` | ensure split commodity matches account commodity | prefer PostgreSQL-enforced integrity if feasible, otherwise transactional service validation | explicit decision required |
| split account/book invariant | `trg_splits_book_matches_txn_ins`, `trg_splits_book_matches_txn_upd` | ensure split account belongs to the same book as the transaction | prefer PostgreSQL-enforced integrity if feasible, otherwise transactional service validation | explicit decision required |
| split category/book invariant | `trg_splits_category_book_matches_txn_ins`, `trg_splits_category_book_matches_txn_upd` | ensure split category belongs to the same book as the transaction | prefer PostgreSQL-enforced integrity if feasible, otherwise transactional service validation | explicit decision required |
| split-lot consistency invariant | `trg_split_lot_allocations_match_split_lot_ins`, `trg_split_lot_allocations_match_split_lot_upd` | ensure lot allocations match split account and commodity | prefer PostgreSQL-enforced integrity if feasible, otherwise transactional service validation | explicit decision required |
| commodity scale bounds | `trg_currency_scale_bounds_ins`, `trg_currency_scale_bounds_upd`, `trg_non_currency_scale_bounds_ins`, `trg_non_currency_scale_bounds_upd` | enforce commodity precision bounds | PostgreSQL check constraints plus service prevalidation for better error reporting | explicit decision required |
| price source existence guard | `trg_prices_source_valid` | ensure referenced price source exists | direct PostgreSQL foreign-key style enforcement in the target schema | mostly decided |
| report state bootstrap | `trg_books_insert_state` | create initial `book_state` row for new books | migrate with the `book_state` design, likely in service-layer book creation or database default/bootstrap logic | explicit decision required |
| report cache invalidation bumpers | `trg_bump_accounts_*`, `trg_bump_transactions_*`, `trg_bump_categories_*`, `trg_bump_tags_*`, `trg_bump_payees_*`, `trg_bump_people_*`, `trg_bump_projects_*`, `trg_bump_commodities_*`, `trg_bump_prices_*`, `trg_bump_lots_*`, `trg_bump_splits_*`, `trg_bump_split_lot_alloc_*`, `trg_bump_balance_checks_*`, `trg_bump_balance_adjustments_*`, `trg_bump_notes_*`, `trg_bump_events_*`, `trg_bump_documents_*`, `trg_bump_balance_constraints_*`, `trg_bump_import_rules_*` | bump `book_state.change_seq` so reports and caches can detect stale data | migrate with `book_state`/`report_cache`; exact placement between PostgreSQL triggers and service-layer invalidation is still open | explicit decision required |
| account system-role guards | `trg_accounts_system_role_insert_guard`, `trg_accounts_system_role_update_guard`, `trg_accounts_system_role_requires_system_insert`, `trg_accounts_system_role_requires_system_update` | restrict valid `system_role` values and require `is_system=1` when role is set | PostgreSQL constraints/checks plus service prevalidation for cleaner UX | explicit decision required |

Note:
Append-only triggers are tracked separately under the append-only immutability model because that is now a required architecture decision, not part of the still-open invariant-trigger bucket.

## PostgreSQL/SQLAlchemy Direction

- Model currencies as commodity rows with `kind='currency'`; do not carry forward a separate canonical `currencies` table.
- Keep immutable domain history in append-only versioned tables. The exact linking mechanism can stay `previous_*` compatible or evolve to a clearer versioned-identity pattern, but the write model should remain history-preserving rather than mutable.
- Expose latest-state reads through ordinary SQL views for cross-cutting "current" projections that many services share. Map those views read-only in SQLAlchemy or query them through repository selectables.
- Keep audit attribution separate from business version chains. Add explicit audit session and device tables, then stamp immutable writes with authenticated user, session, request, and device context.
- Use PostgreSQL constraints, foreign keys, and partial indexes for invariants the database can own safely; keep richer cross-row UX validation in service code so API errors remain understandable.