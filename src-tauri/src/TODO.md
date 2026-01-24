# Rekenraam – Tauri Backend TODO

Note: This checklist is now merged into src-tauri/README.md. Keep updates there and sync back here only if needed.

This file tracks backend work by milestone, with acceptance criteria for each item.

## M1 — Storage + Migrations (Current)
- [x] Open rusqlite on startup and run migrations.
- [x] Persist chosen storage location.
- [x] Document migration policy in README (naming, versioning, ordering).
- [x] Enable WAL and core SQLite pragmas.
- [x] Add command: `get_schema_version` (reads from `schema_migrations`).
- [x] Add seed data migration (default book + base currency + starter accounts).

Acceptance criteria
- App boots on a new machine, creates DB, applies migrations, and returns `db_health = ok`.
- `get_schema_version` returns the latest applied version.

## M2 — Storage Location (except UX)
- [X] Add command to validate and set storage location (runs migrations, returns effective DB path).
- [X] Add command to return current DB path (if not already exposed in UI).

Acceptance criteria
- Changing storage location validates write access and results in a usable DB.

## M3 — Domain Schema + Invariants
- [X] Add core domain tables: accounts, commodities, transactions, splits.
- [X] Add invariants (e.g., account commodity rules) via constraints or validation.
- [X] Add report cache table migrations if still needed.

Acceptance criteria
- Schema supports minimal account/transaction workflows without violating invariants.

## M4 — Backend Commands (MVP)
- [X] CRUD commands for accounts.
- [X] Command to create a transaction with splits.
- [X] Read commands for account list and register.

Acceptance criteria
- UI can show accounts and a read-only register using these commands.

## Backlog (Post‑MVP)
- [ ] Business extensions (customers/vendors, invoices, taxes).
- [ ] Tighten invariants as domain expands.