# Rekenraam — Personal Finance Tracker

Personal finance application in active migration from a Tauri desktop runtime to a
self-hosted SvelteKit + FastAPI web stack. Tauri remains only as transitional
compatibility scaffolding for slices that have not yet been moved.

## Current Status (May 2026)

See [SELF_HOSTED_MIGRATION_PLAN.md](SELF_HOSTED_MIGRATION_PLAN.md) for the active
self-hosted migration roadmap. [MASTER_PLAN.md](MASTER_PLAN.md) is now mainly
historical desktop/product planning context.

**Working:**
- Dockerized web stack with PostgreSQL, FastAPI, and a separately served Svelte frontend
- Books, accounts, balances, register, transactions, metadata, classic reports, investments, and FX settings on Python HTTP endpoints
- Backend-owned pricing refresh worker with persisted run history and UI polling
- Report cache/state foundation with `book_state`, `report_cache`, `report_definitions`, and `report_runs` in the web baseline
- Current report endpoints using persisted cache state plus invalidation bumps on report-relevant writes
- Shared frontend API seam with same-origin `/api/v1` support for the containerized web frontend

**Recently completed:**
- Full Compose path for `postgres`, `api`, and `frontend`
- Browser-safe API connectivity through same-origin `/api/v1` plus frontend nginx proxying
- Python-backed report metadata persistence and report invalidation/cache wiring
- Python-backed FX execution status, history, provider coverage, and web-mode settings integration

**Next up:**
- Finish retiring the remaining desktop-only frontend paths: admin/storage settings, commodity autocomplete, and manual price-entry helpers
- Start the thin audit/session/device attribution foundation needed before broader mutation parity work
- Continue transaction/account write parity and only then widen further into import and broader reporting/investment expansion

**Planning note:**
- [SELF_HOSTED_MIGRATION_PLAN.md](SELF_HOSTED_MIGRATION_PLAN.md) is the active migration roadmap.
- [docs/parity/desktop-to-python.md](docs/parity/desktop-to-python.md) tracks capability-by-capability parity state.
- `OLD_TODOS/` is historical reference material; keep it for background, not active prioritization.

## Architecture

| Layer | Technology |
|-------|-----------|
| Transitional desktop runtime | Tauri 2.x |
| Web runtime | Docker Compose + nginx |
| UI | SvelteKit 2 + Svelte 5 + TypeScript |
| UI components | shadcn-svelte + Bits UI + Tailwind 4 |
| Backend | FastAPI + SQLAlchemy + Pydantic |
| Web storage | PostgreSQL |
| Legacy desktop storage | SQLite |

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
docker compose up --build
```

That starts three containers by default:

- `postgres` for the database
- `api` for the FastAPI backend
- `frontend` for the built Svelte web app served as static files

The frontend is intentionally separate from the API container. The UI is a static
Svelte build, so bundling Node build output and static file serving into the Python
API image would mix two runtimes and make cache/build behavior worse without adding
useful coupling.

For host-side development outside the proxied frontend container, the frontend can still target the Python API through:

```bash
PUBLIC_API_BASE_URL=http://localhost:8080/api/v1
```

For the containerized web frontend, the default path is now same-origin `/api/v1`
through the nginx proxy in front of the static frontend.

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

Remaining web-frontend blockers before the Tauri fallback seam can be removed cleanly:

- `src/lib/api/admin.ts` and `src/routes/settings/DatabaseSettings.svelte` still depend on desktop-only storage, backup, restore, and DB-maintenance commands
- `src/routes/settings/+page.svelte` still uses the Tauri-backed fiscal year close helper from `src/lib/api/admin.ts`
- `src/lib/api/commodities.ts` still uses Tauri-only helpers for commodity autocomplete and manual daily/official price entry flows
- `src/lib/api/accounts.ts`, `src/lib/api/transactions.ts`, and `src/lib/api/metadata.ts` still expose HTTP-first plus Tauri-fallback wrappers, even where Python endpoints now exist, because desktop compatibility has not been formally retired yet

If you previously ran the old Rust API scaffold against Docker, the existing
Postgres volume may contain incompatible tables. The Python scaffold now uses a
fresh Compose volume, but if you want to reset the local test database entirely:

```bash
make api-reset-db
docker compose up --build
```

Then check:

```bash
curl http://localhost:8080/api/v1/health
curl http://localhost:3000
```

Expected response:

```json
{"status":"ok","service":"rekenraam-api","database":"ok"}
```

The API now runs one current initial Alembic schema automatically on startup.
That single `0001_initial_schema` baseline is the source of truth for fresh web
databases and already includes the current investments tables, pricing policy
tables, seeded pricing sources, persisted pricing refresh run history, and the
first report invalidation foundation tables: `book_state` and `report_cache`.
It now also includes report metadata persistence tables: `report_definitions`
and `report_runs`.
The Compose stack also includes an API healthcheck against `/api/v1/health`, so
container readiness reflects actual application startup rather than process spawn
alone.

Pricing execution is now backend-owned in the Python API as well. The FastAPI
app starts an in-process pricing worker on startup, and the FX settings page in
web mode now uses these endpoints:

```bash
curl http://localhost:8080/api/v1/pricing/refresh-state
curl http://localhost:8080/api/v1/pricing/refresh/execution-status
curl http://localhost:8080/api/v1/pricing/refresh/history
curl -X POST http://localhost:8080/api/v1/pricing/refresh/run \
  -H 'Content-Type: application/json' \
  -d '{"book_id":1}'
```

Current backend execution coverage includes ECB, Federal Reserve, Bank of
Canada, ExchangeRate.host, and Yahoo Finance, with compatibility aliases for
HMRC, IRS, Belastingdienst, RBA, and SNB. Completed runs are now persisted and
available through the history endpoint that the FX settings page shows in web
mode.

The repo also includes a small `Makefile` for common API tasks:

```bash
make api-check
make api-lint
make api-typecheck
make api-test
make api-test-docker
make api-test-postgres
make api-up
make web-up
make api-health
make api-books
make api-accounts
make api-accounts-tree
make api-account-register
make api-transactions
make api-smoke
make api-migrate-new NAME=add_accounts
make api-migrate-up
make api-migrate-down REV=base
make api-migrate-current
make api-migrate-smoke
```

For the hot-reload development stack, use:

```bash
make web-dev-up
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
