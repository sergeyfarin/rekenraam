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

The first web migration slice now includes a standalone Python FastAPI scaffold under
`apps/api` plus a Docker Compose stack with PostgreSQL.

```bash
cp .env.example .env
docker compose up --build api postgres
```

The frontend migration seam can now target the Python API through:

```bash
PUBLIC_API_BASE_URL=http://localhost:8080/api/v1
```

Frontend routes that have been migrated to the shared client layer will prefer
HTTP and fall back to Tauri commands if the Python API is not yet configured or
does not provide the needed slice.

Current migrated frontend read slices:

- accounts page account tree
- accounts page account balances
- accounts page commodities, countries, and institutions lookups
- account detail page account summary
- account detail page account balances
- account detail page balancings, directives, and booking policy
- account detail page booking policy update
- account detail page categories, payees, tags, people, projects, and commodities lookups
- account detail page register rows and on-demand transaction detail read
- account detail page transaction create, update, and delete flows via shared seam
- account detail page unlock-account-balancing helper via shared seam
- account detail page supporting create-entity/account helpers via shared seam
- home/dashboard account, balance, payee, and recent-transaction reads via shared seam
- settings categories, payees, and tags read loads via shared seam
- transactions page categories, payees, tags, people, projects, and commodities lookups
- transactions page transaction list read

If you previously ran the old Rust API scaffold against Docker, the existing
Postgres volume may contain incompatible tables. The Python scaffold now uses a
fresh Compose volume, but if you want to reset the local test database entirely:

```bash
make api-reset-db
docker compose up --build api postgres
```

Then check:

```bash
curl http://localhost:8080/api/v1/health
```

Expected response:

```json
{"status":"ok","service":"rekenraam-api","database":"ok"}
```

The API now runs one current initial Alembic schema automatically on startup.
The Compose stack also includes an API healthcheck against `/api/v1/health`, so
container readiness reflects actual application startup rather than process spawn
alone.

The repo also includes a small `Makefile` for common API tasks:

```bash
make api-check
make api-lint
make api-typecheck
make api-test
make api-test-docker
make api-test-postgres
make api-up
make api-health
make api-books
make api-accounts
make api-accounts-tree
make api-account-register
make api-transactions
make api-smoke
make api-migrate-new NAME=add_accounts
```

If your Docker access requires elevation, override the Docker command:

```bash
make DOCKER="sudo docker compose" api-up
make CONTAINER_RUNTIME="sudo docker" api-test-docker
make DEV_DOCKER="sudo docker compose -f compose.yaml -f compose.dev.yaml" api-dev-up
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

Accounts endpoints:

```bash
curl http://localhost:8080/api/v1/accounts
curl http://localhost:8080/api/v1/accounts/1
curl http://localhost:8080/api/v1/accounts/tree
curl http://localhost:8080/api/v1/accounts/2/register
curl http://localhost:8080/api/v1/transactions
```

Example response fields:

```json
{
  "id": 1,
  "book_id": 1,
  "parent_id": null,
  "account_type": "asset",
  "name": "Assets",
  "is_closed": false,
  "is_hidden": false,
  "is_system": false,
  "system_role": null,
  "created_at": "2026-05-03T00:00:00+00:00"
}
```

Account tree response shape for frontend migration:

```json
[
  {
    "id": 1,
    "parent_id": null,
    "name": "Assets",
    "account_type": "asset",
    "commodity_id": 1,
    "commodity_name": "USD",
    "commodity_scale": 2,
    "institution_name": null,
    "country_name": null,
    "balance_minor": 0,
    "rollup_balance_minor": 500000,
    "children": [
      {
        "id": 2,
        "parent_id": 1,
        "name": "Cash",
        "account_type": "asset",
        "commodity_id": 1,
        "commodity_name": "USD",
        "commodity_scale": 2,
        "institution_name": null,
        "country_name": null,
        "balance_minor": 500000,
        "rollup_balance_minor": 500000,
        "children": []
      }
    ]
  }
]
```

Balances and rollups are now backed by seeded transaction/split data. They are
still minimal and will need to grow with the full transaction parity work, but
they are no longer structural placeholders.

Transactions endpoint:

```bash
curl http://localhost:8080/api/v1/transactions
curl 'http://localhost:8080/api/v1/transactions?account_id=2&status=cleared&occurred_from=2026-05-01&occurred_to=2026-05-31'
```

Supported transaction list filters:

- `book_id`
- `account_id`
- `status`
- `occurred_from`
- `occurred_to`

Example response shape:

```json
[
  {
    "id": 1,
    "book_id": 1,
    "occurred_date": "2026-05-01",
    "posted_date": "2026-05-01",
    "memo": "Initial opening balance",
    "status": "cleared",
    "splits": [
      {"id": 1, "tx_id": 1, "account_id": 2, "amount_minor": 500000, "memo": "Opening cash balance"},
      {"id": 2, "tx_id": 1, "account_id": 3, "amount_minor": -500000, "memo": "Opening equity offset"}
    ]
  }
]
```

Account register endpoint:

```bash
curl http://localhost:8080/api/v1/accounts/2/register
```

Example response shape:

```json
[
  {
    "tx_id": 1,
    "split_id": 1,
    "account_id": 2,
    "occurred_date": "2026-05-01",
    "posted_date": "2026-05-01",
    "memo": "Initial opening balance",
    "status": "cleared",
    "amount_minor": 500000,
    "running_balance_minor": 500000
  }
]
```

## Dev Container Workflow

For day-to-day backend work, prefer the dev compose workflow over rebuilding the
production API image on every code change.

```bash
make api-dev-up
make api-dev-logs
```

This starts an `api-dev` container with:

- bind-mounted `apps/api` source
- `uvicorn --reload`
- Dockerized Postgres
- no rebuild required for normal Python code edits

Real repository tests against ephemeral PostgreSQL are available via:

```bash
make api-test-postgres
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
| [SELF_HOSTED_MIGRATION_PLAN.md](SELF_HOSTED_MIGRATION_PLAN.md) | Target architecture and staged migration plan for the self-hosted Python + PostgreSQL + SvelteKit + Docker version |

## Contributing

- Rust domain model is the source of truth
- Prefer small, deterministic components; avoid hidden I/O in UI code
- Security and privacy over convenience; no network calls without explicit user consent
- Every new Tauri command must have at least one unit test
- All errors must use `AppError` (see MASTER_PLAN.md Sprint 2.1)
- Stack: Rust, Tauri, Svelte, TypeScript, SQLite
