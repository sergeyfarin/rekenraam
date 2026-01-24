# Rekenraam – Personal Finance Tracker

Modern MS Money–style desktop app built with Tauri (Rust backend) and Svelte (UI). Offline-first, multi-currency, multi-account, with a roadmap for optional cloud sync, bank connectivity, and a plugin system.

## Current State
- Tauri app shell with Svelte frontend.
- SQLite database created on first run; migration runner loads versioned SQL files.
- Storage location persisted per user, with basic recovery if the DB is missing.
- Storage commands: `validate_and_set_storage_location`, `get_storage_location`, `get_db_path`, `db_health`.

## Goals (MVP)
- Reliable local storage with migrations, WAL, and seed data.
- Minimal accounting domain: accounts, commodities, transactions, splits.
- Basic UI: accounts list, read-only register, create transaction.

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
- The current runner (in `src-tauri/src/db.rs`) contains a static `MIGRATIONS` list that is applied in order; each entry is a tuple `(version, sql)` and the runner skips versions already recorded in the `schema_migrations` table.
- Transactionality: each migration is run inside a transaction; the runner enables `PRAGMA foreign_keys = ON` before applying migrations.
- How to add a migration (current workflow):
	1. Add a new SQL file named `V{n}__short_description.sql` in `src-tauri/migrations`.
	2. Add an entry to the `MIGRATIONS` array in `src-tauri/src/db.rs` with the next integer `n` and `include_str!("../migrations/V{n}__short_description.sql")`.
	3. Verify the SQL is idempotent where possible and that it runs cleanly inside a transaction.
- Recording: successful migrations are inserted into `schema_migrations (version INTEGER PRIMARY KEY)` so applied versions are tracked and skipped on subsequent runs.

Current schema version (as of this commit): 1 (single consolidated schema in V1__init.sql).

## Milestone Checklist (Backend)
### M1 — Storage + Migrations
- [x] Document migration policy and current schema version.
- [X] Ensure WAL and core SQLite pragmas are enabled.
- [X] Add a Tauri command to return the current schema version.
- [X] Add seed data migration (base currency + starter accounts).

### M2 — Storage Location (except UX)
- [X] Add command to validate and set storage location (runs migrations, returns effective DB path).
- [X] Add command to return current DB path (if not already exposed in UI).

### M2 — Domain MVP + Commands
- [ ] Define domain model: accounts, commodities, transactions, splits.
- [ ] Add invariants (e.g., account commodity rules).
- [X] Implement CRUD commands for accounts.
- [X] Implement command to add a transaction with splits.

### M3 — UI MVP
- [ ] Accounts list and account detail.
- [ ] Read-only transaction register.
- [ ] Basic “add transaction” flow.

### M4 — Quality
- [ ] Minimal test coverage for core DB commands.
- [ ] Smoke test for UI shell and command wiring.

## Future (Post‑MVP)
- Encryption/key management for local DB.
- Import flows (CSV/QIF/OFX).
- Budgeting, rules, reports, and projections.
- Plugin system and bank connectivity.
- Sync and cloud service (optional).

## Contributing Notes
- Stack: Rust, Tauri, Svelte, TypeScript, SQLite.
- Rust domain model is the source of truth.
- Prefer small, deterministic components; avoid hidden I/O in UI code.
- Security and privacy over convenience; no network calls without explicit consent.
