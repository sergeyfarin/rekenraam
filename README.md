# Rekenraam — Personal Finance Tracker

Modern MS Money–style desktop application built with Tauri (Rust) and Svelte (TypeScript).
Offline-first, multi-currency, multi-account, with optional FX refresh and a roadmap for
future sync, import, and plugins.

## Current Status (April 2026)

See [MASTER_PLAN.md](MASTER_PLAN.md) for the full execution roadmap.

**Working:**
- SQLite storage with WAL, versioned migrations, backup/restore
- Account CRUD with tree view and rollup balances
- Transaction/split create and edit with double-entry enforcement
- Account register with running balance
- Reconciliation wizard with balance verification and account locking
- Investment: holdings, lots, buy/sell/dividend, realized/unrealized gains
- FX: rate scheduler, source providers, daily/official rates
- Reports: cashflow, category spend, payee totals, gains
- Settings: categories, payees, tags, commodities, institutions, database management

**Recently completed (March 2026):**
- Onboarding: welcome screen on first launch (no DB path stored)
- Transaction register improvements: server-side filters and sorting, infinite scroll, smart date parsing, duplication, bulk actions
- Structured backend errors (`AppError`), frontend error formatting, logging, read/write DB split
- Year-end close UI in Settings → Year-End tab
- Reconciliation wizard and account-level reconciliation status

**Next up:**
- Import pipeline completion: backend hardening plus 3-step import wizard UI
- Reports page charts using existing data APIs
- Planning page implementation (scheduled transactions and budgeting remain placeholders)

**Planning note:**
- [MASTER_PLAN.md](MASTER_PLAN.md) is the active roadmap.
- `OLD_TODOS/` is historical reference material; keep it for background, not active prioritization.

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

## Self-Hosted API Scaffold

The first web migration slice now includes a standalone Rust API scaffold under
`apps/api` plus a Docker Compose stack with PostgreSQL.

```bash
cp .env.example .env
docker compose up --build api postgres
```

Then check:

```bash
curl http://localhost:8080/health
```

Expected response:

```json
{"status":"ok","service":"rekenraam-api","database":"ok","schema_version":"0002_accounts"}
```

The API now runs SQLx migrations automatically on startup from `apps/api/migrations`.
The Compose stack also includes an API healthcheck against `/health`, so container
readiness reflects actual application startup rather than process spawn alone.

The repo also includes a small `Makefile` for common API tasks:

```bash
make api-check
make api-up
make api-health
make api-books
make api-migrate-new NAME=add_accounts
```

If your Docker access requires elevation, override the Docker command:

```bash
make DOCKER="sudo docker compose" api-up
```

First real read endpoint:

```bash
curl http://localhost:8080/api/v1/books
```

Expected response:

```json
[{"id":1,"slug":"personal","name":"Personal","base_currency_code":"USD"}]
```

Book detail endpoint:

```bash
curl http://localhost:8080/api/v1/books/personal
```

Expected response:

```json
{"id":1,"slug":"personal","name":"Personal","base_currency_code":"USD"}
```

First accounts endpoint:

```bash
curl http://localhost:8080/api/v1/accounts
```

Expected response:

```json
[
  {"id":1,"book_id":1,"parent_id":null,"account_type":"asset","name":"Cash","currency_code":"USD","is_closed":false,"is_hidden":false},
  {"id":2,"book_id":1,"parent_id":null,"account_type":"asset","name":"Checking Account","currency_code":"USD","is_closed":false,"is_hidden":false}
]
```

**Linux / WSL2 prerequisites** (required for Tauri's native file dialogs):
```bash
sudo apt install -y libgtk-3-dev libglib2.0-dev libwebkit2gtk-4.1-dev \
  libjavascriptcoregtk-4.1-dev libsoup-3.0-dev
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
| [SELF_HOSTED_MIGRATION_PLAN.md](SELF_HOSTED_MIGRATION_PLAN.md) | Target architecture and staged migration plan for the self-hosted Rust + PostgreSQL + SvelteKit + Docker version |

## Contributing

- Rust domain model is the source of truth
- Prefer small, deterministic components; avoid hidden I/O in UI code
- Security and privacy over convenience; no network calls without explicit user consent
- Every new Tauri command must have at least one unit test
- All errors must use `AppError` (see MASTER_PLAN.md Sprint 2.1)
- Stack: Rust, Tauri, Svelte, TypeScript, SQLite
