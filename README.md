# Rekenraam — Personal Finance Tracker

Modern MS Money–style desktop application built with Tauri (Rust) and Svelte (TypeScript).
Offline-first, multi-currency, multi-account, with optional FX refresh and a roadmap for
future sync, import, and plugins.

## Current Status (March 2026)

See [MASTER_PLAN.md](MASTER_PLAN.md) for the full execution roadmap.

**Working:**
- SQLite storage with WAL, versioned migrations, backup/restore
- Account CRUD with tree view and rollup balances
- Transaction/split create and edit with double-entry enforcement
- Account register with running balance
- Investment: holdings, lots, buy/sell/dividend, realized/unrealized gains
- FX: rate scheduler, source providers, daily/official rates
- Reports: cashflow, category spend, payee totals, gains
- Settings: categories, payees, tags, commodities, institutions, database management

**In progress / next up:**
- Onboarding flow (first-launch DB creation)
- Complete transaction filters and server-side pagination
- Reconciliation wizard
- Import wizard UI (parsers already exist for QIF/OFX/CSV/MT940)

## Architecture

| Layer | Technology |
|-------|-----------|
| Desktop runtime | Tauri 2.x |
| UI | SvelteKit 2 + Svelte 5 + TypeScript |
| UI components | shadcn-svelte + Bits UI + Tailwind 4 |
| Backend | Rust (Tauri commands) |
| Storage | SQLite (rusqlite, WAL mode, versioned migrations) |

## Running

```bash
npm install
npm run tauri dev
```

## Data Storage

- Database file: `Rekenraam-data.rekenraam` in a user-selected folder
- Default location: `Documents/rekenraam` on Windows
- First run: user is prompted to choose or create the storage folder
- Migrations: SQL files in `src-tauri/migrations`, applied in version order

## Adding a Migration

1. Create `src-tauri/migrations/V{n}__short_description.sql`
2. Add an entry to the `MIGRATIONS` array in `src-tauri/src/db.rs`
3. Verify the SQL runs cleanly inside a transaction

Current schema version: **18** (consolidated in `V1__init.sql`).

## Key Documents

| Document | Purpose |
|----------|---------|
| [MASTER_PLAN.md](MASTER_PLAN.md) | Prioritized execution plan — start here |
| [SCHEMA.md](SCHEMA.md) | Database schema reference |

## Contributing

- Rust domain model is the source of truth
- Prefer small, deterministic components; avoid hidden I/O in UI code
- Security and privacy over convenience; no network calls without explicit user consent
- Every new Tauri command must have at least one unit test
- All errors must use `AppError` (see MASTER_PLAN.md Sprint 2.1)
- Stack: Rust, Tauri, Svelte, TypeScript, SQLite
