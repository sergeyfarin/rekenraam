# Rekenraam – Personal Finance Tracker

Modern MS Money–style desktop app built with Tauri (Rust backend) and Svelte (UI). Offline-first, multi-currency, multi-account, with optional FX refresh and a roadmap for future sync, import, and plugins.

## Current Status (Feb 10, 2026)

### Backend
- Storage: SQLite with WAL, migrations, and per-user storage path persisted in OS app data.
- Migrations: versioned SQL files through V17, applied in a transaction, tracked in `schema_migrations`.
- Core domain: accounts, commodities/currencies, categories, payees, tags, institutions, transactions, splits.
- Investments: lots, cost basis, buy/sell/dividend flows, gains reports, positions and lot holding periods.
- FX: daily and official rates, sources, assignments, refresh settings, scheduler, and provider integrations.
- Reports: cashflow, category spend, payee totals, gains, and report caching by book change sequence.
- Storage maintenance: backups, restore, integrity check, vacuum, migration status, DB stats.
- Single-book assumption: commands and seed data target book id 1.

### Frontend
- Home dashboard: net worth, assets/liabilities, recent transactions, quick actions.
- Accounts: tree view, balances, create/edit/close, institutions, currency assignment.
- Transactions: list, filters (partial), create/edit with splits and transfers.
- Investments: holdings, lots, buy/sell/dividend dialogs, gains views.
- Reports: cashflow, category spend, payee totals, gains.
- Settings: storage + backups + maintenance, categories, payees, tags, currencies, FX settings, institutions.
- Planning/Tax: placeholder pages (no functional UI yet).

## MVP Scope (Target)
- Local-first bookkeeping for a single book.
- Accounts, categories, payees, transactions (with splits) and a usable register.
- Basic reports: cashflow and category spend.
- Essential settings: DB location, backup, currencies.

## Nearest Steps to a Fully Functioning MVP
1. Account register view per account with running balance and drill-through to edit transactions.
2. Complete transaction filters (payee, memo, status, amount) and server-side sorting/paging.
3. Harden transaction creation and editing flows with validation messages and split balancing hints.
4. Onboarding flow that creates or opens the database on first launch without manual navigation.
5. Reduce scope in non-MVP areas (investments/FX/reports) or clearly label them as beta.
6. Basic smoke tests for storage, account CRUD, and transaction create/edit.

## Architecture (Short)
- UI: Svelte + Vite + TypeScript in Tauri webview.
- Backend: Rust + Tauri commands.
- Storage: SQLite with versioned SQL migrations.

## Running
- `npm install`
- `npm run dev`
- `npm run tauri dev`

## Data Storage
- DB file: `Rekenraam-data.rekenraam` stored under a user-selected folder.
- On first run, the user is asked to choose a folder (defaults to Documents/rekenraam on Windows).
- Migrations are stored as SQL files in `src-tauri/migrations` and applied in order.

## Migration Policy

- Naming and ordering: migrations use the `V{n}__description.sql` convention and are stored in `src-tauri/migrations`.
- The runner (in `src-tauri/src/db.rs`) contains a static `MIGRATIONS` list applied in order; each entry is a tuple `(version, sql)` and the runner skips versions already recorded in the `schema_migrations` table.
- Transactionality: each migration is run inside a transaction; the runner enables `PRAGMA foreign_keys = ON` before applying migrations.
- How to add a migration (current workflow):
	1. Add a new SQL file named `V{n}__short_description.sql` in `src-tauri/migrations`.
	2. Add an entry to the `MIGRATIONS` array in `src-tauri/src/db.rs` with the next integer `n` and `include_str!("../migrations/V{n}__short_description.sql")`.
	3. Verify the SQL is idempotent where possible and that it runs cleanly inside a transaction.
- Recording: successful migrations are inserted into `schema_migrations (version INTEGER PRIMARY KEY)` so applied versions are tracked and skipped on subsequent runs.

Current schema version (as of this commit): 17.

## Future (Post‑MVP)
- Encryption/key management for local DB.
- Import flows (CSV/QIF/OFX) and rules UI.
- Budgeting, rules, reports, and projections.
- Plugin system and bank connectivity.
- Sync and cloud service (optional).
- Business extensions (customers/vendors, invoices, taxes).
- Tighten invariants as domain expands.

## Contributing Notes
- Stack: Rust, Tauri, Svelte, TypeScript, SQLite.
- Rust domain model is the source of truth.
- Prefer small, deterministic components; avoid hidden I/O in UI code.
- Security and privacy over convenience; no network calls without explicit consent.


add a small frontend action in Settings/Year-End that calls close_fiscal_year and shows the resulting closing transaction id + retained earnings delta.



Checked V1__init.sql for tables that lack append-only immutability guards (no BEFORE UPDATE/DELETE trigger that aborts writes, unlike the section starting at V1__init.sql:2293).

account_balancings, account_directives, app_runtime_session, backup_settings, balance_checks, balance_constraints, book_state, books
commodity_prices, corporate_actions, countries, currencies, dividend_income_categories, documents, events
fx_rate_refresh_state, fx_rate_settings, fx_rate_source_assignments, fx_rate_sources, fx_rates_daily, fx_rates_official
import_rules, import_session_transactions, import_sessions, institutions, notes, pad_directives
report_cache, report_definitions, report_runs, schema_migrations, session_redo_stack, session_reverts, session_undo_stack
split_lot_allocations, split_people, split_projects, split_tags
If you want, I can also generate the inverse list (tables that are immutable-enforced) as a quick verification checklist.