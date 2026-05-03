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
| reports | `src-tauri/src/db_reports.rs` | planned `apps/api/src/rekenraam_api/services/reports.py`, `api/v1/reports.py` | planned `/api/v1/reports*` | not started | Read-only metadata and simple reports should move before calculation-heavy reports. |
| currencies and commodities metadata | `src-tauri/src/db_currencies.rs`, `src-tauri/src/db_commodities.rs` | planned `apps/api/src/rekenraam_api/services/commodities.py`, `api/v1/commodities.py` | planned `/api/v1/commodities*` | not started | Books currently expose only `base_currency_code`. |
| reconciliation | `src-tauri/src/db_transactions.rs`, `src-tauri/src/db_accounts.rs` | planned reconciliation service slice | planned `/api/v1/reconciliation*` | not started | Must follow transaction correctness migration. |
| imports | `src-tauri/src/import.rs` | planned import service and worker slice | planned `/api/v1/imports*` | not started | SQLite-to-Postgres import also depends on this area. |
| pricing and FX refresh | `src-tauri/src/fx_refresh.rs`, `src-tauri/src/fx_rates/*` | planned worker + API slice | planned `/api/v1/fx*` | not started | Likely split across API and background worker. |
| settings and metadata | `src-tauri/src/db_accounts.rs`, `src-tauri/src/db_commodities.rs`, `src-tauri/src/db_storage.rs` | planned metadata services | planned `/api/v1/settings*` | not started | Includes institutions, tags, payees, categories, and app-level configuration. |
| desktop runtime session state | `src-tauri/src/db.rs`, `src-tauri/src/session.rs` | FastAPI request context, later auth/session handling | none | dropped | `app_runtime_session` is a desktop-process marker and is intentionally not ported as a PostgreSQL table. |
| undo/redo | `src-tauri/src/session.rs` | future server-side mutation history or conscious removal | none yet | not started | `session_undo_stack`, `session_redo_stack`, and `session_reverts` are intentionally dropped from Stage 2. If undo/redo survives as a product feature, it needs a new server-safe design rather than a direct table-for-table port. |

## Runtime/Session Table Decisions

| Tauri table | Decision | Replacement |
|---|---|---|
| `app_runtime_session` | dropped | request-scoped context now; authenticated web sessions later in Stage 8 |
| `session_undo_stack` | dropped from Stage 2 | no direct replacement; redesign only if undo remains a product requirement |
| `session_redo_stack` | dropped from Stage 2 | no direct replacement; redesign only if redo remains a product requirement |
| `session_reverts` | dropped from Stage 2 | no direct replacement; redesign only as part of a server-side undo/audit model |