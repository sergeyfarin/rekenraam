# Rekenraam – Tauri Backend

Rust backend for the Rekenraam desktop app. Provides local SQLite storage, migrations, and Tauri commands used by the Svelte UI.

## Current State
- SQLite database created on first run; migration runner loads versioned SQL files.
- Storage location persisted per user, with basic recovery if the DB is missing.
- Storage commands: `validate_and_set_storage_location`, `get_storage_location`, `get_db_path`, `db_health`.
- Schema version command: `get_schema_version`.

## Data Storage
- DB file: `Rekenraam-data.rekenraam` stored under a user-selected folder.
- On first run, the user is asked to choose a folder (defaults to Documents/rekenraam on Windows).
- Migrations are stored as SQL files in `src-tauri/migrations` and applied in order.

## Migration Policy
- Naming and ordering: migrations use the `V{n}__description.sql` convention and are stored in `src-tauri/migrations`.
- The runner (in `src-tauri/src/db.rs`) contains a static `MIGRATIONS` list that is applied in order; each entry is a tuple `(version, sql)` and the runner skips versions already recorded in the `schema_migrations` table.
- Transactionality: each migration is run inside a transaction; the runner enables `PRAGMA foreign_keys = ON` before applying migrations.
- How to add a migration (current workflow):
	1. Add a new SQL file named `V{n}__short_description.sql` in `src-tauri/migrations`.
	2. Add an entry to the `MIGRATIONS` array in `src-tauri/src/db.rs` with the next integer `n` and `include_str!("../migrations/V{n}__short_description.sql")`.
	3. Verify the SQL is idempotent where possible and that it runs cleanly inside a transaction.
- Recording: successful migrations are inserted into `schema_migrations (version INTEGER PRIMARY KEY)` so applied versions are tracked and skipped on subsequent runs.

## Backend Milestones (Merged from TODO)
### M1 — Storage + Migrations
- [x] Open rusqlite on startup and run migrations.
- [x] Persist chosen storage location.
- [x] Document migration policy in README (naming, versioning, ordering).
- [x] Enable WAL and core SQLite pragmas.
- [x] Add command: `get_schema_version` (reads from `schema_migrations`).
- [x] Add seed data migration (default book + base currency + starter accounts).

Acceptance criteria
- App boots on a new machine, creates DB, applies migrations, and returns `db_health = ok`.
- `get_schema_version` returns the latest applied version.

### M2 — Storage Location (except UX)
- [X] Add command to validate and set storage location (runs migrations, returns effective DB path).
- [X] Add command to return current DB path (if not already exposed in UI).

Acceptance criteria
- Changing storage location validates write access and results in a usable DB.

### M3 — Domain Schema + Invariants
- [X] Add core domain tables: accounts, commodities, transactions, splits.
- [X] Add invariants (e.g., account commodity rules) via constraints or validation.
- [ ] Add report cache table migrations if still needed.

Acceptance criteria
- Schema supports minimal account/transaction workflows without violating invariants.

### M4 — Backend Commands (MVP)
- [X] CRUD commands for accounts.
- [X] Command to create a transaction with splits.
- [X] Read commands for account list and register.

Acceptance criteria
- UI can show accounts and a read-only register using these commands.

## Backlog (Post‑MVP)
- [ ] Business extensions (customers/vendors, invoices, taxes).
- [ ] Tighten invariants as domain expands.
