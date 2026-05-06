# Rekenraam

Self-hosted personal finance app in active migration to a FastAPI,
PostgreSQL, SvelteKit, and Docker web stack.

Tauri/Rust/SQLite code remains in the repository only as temporary migration and
parity reference. It is not the target product architecture.

## Current Status

Working today:

- Docker Compose stack with PostgreSQL, FastAPI, and a separately served Svelte
  frontend
- same-origin `/api/v1` frontend-to-backend proxying in the containerized web
  stack
- first-admin bootstrap, login/logout, sessions, device attribution, request
  context, and protected API routes
- Python HTTP endpoints for books, accounts, balances, registers,
  transactions, metadata, reconciliation, imports, exports, reports,
  investments, pricing settings, FX refresh state/history, and admin runtime
  health
- local user administration, book roles, preferences, saved transaction views,
  transaction templates, markdown notes, and audit-event visibility
- backend-owned pricing refresh worker
- PostgreSQL Alembic baseline with auth/session, reconciliation, import,
  report state/cache, investment, and pricing foundations
- shared frontend API seam under `src/lib/api`

Still in migration:

- budgets and scheduled transactions
- loans, mortgages, amortization, and liability workflow helpers
- richer daily-use polish for transaction search, saved views, templates,
  memorized splits, and payee defaults
- explicit multi-currency transfer workflows
- broader reports, tax, investment, pricing, and valuation depth
- attachment/document uploads and email-based invites/password reset
- production deployment guidance, backup/restore smoke checks, and final CI
  gates
- final Tauri dependency and `src-tauri/` deletion

Deferred after b1/v1:

- plugin execution, frontend plugin slots, granular permissions,
  GitHub-sourced manifests, and WebAssembly/WASI or Extism-style runtime
  evaluation
- arbitrary-language plugins, which should use isolated sidecars rather than
  in-process execution
- built-in/custom theme token packs beyond semantic CSS tokens and the existing
  persisted `theme` preference

## Run The Self-Hosted Stack

```bash
cp .env.example .env
docker compose up --build
```

For faster local testing, `.env` may include `FIRST_ADMIN_EMAIL`,
`FIRST_ADMIN_PASSWORD`, and `FIRST_ADMIN_DISPLAY_NAME`; the API seeds that admin
only when no users exist.

Default services:

- frontend: <http://localhost:3000>
- API: <http://localhost:8080/api/v1>
- API health: <http://localhost:8080/api/v1/health>
- PostgreSQL: `localhost:5432`

Expected health response:

```json
{"status":"ok","service":"rekenraam-api","database":"ok"}
```

## Development

Install frontend dependencies:

```bash
npm install
```

Run the frontend against a host-side API:

```bash
PUBLIC_API_BASE_URL=http://localhost:8080/api/v1 npm run dev
```

Run the hot-reload Docker development stack:

```bash
make web-dev-up
```

Common backend tasks:

```bash
make api-check
make api-lint
make api-typecheck
make api-test
make api-test-postgres
make api-migrate-smoke
```

Common Docker smoke checks:

```bash
make api-health
make api-books
make api-accounts
make api-accounts-tree
make api-account-register
make api-transactions
make api-smoke
```

If Docker needs elevation on your host:

```bash
make DOCKER="sudo docker compose" api-up
make CONTAINER_RUNTIME="sudo docker" api-test-docker
make DEV_DOCKER="sudo docker compose -f compose.yaml -f compose.dev.yaml" api-dev-up
```

## Architecture

| Layer | Technology |
| --- | --- |
| Frontend | SvelteKit 2, Svelte 5, TypeScript |
| Frontend UI | shadcn-svelte, Bits UI, Tailwind 4 |
| Backend | FastAPI, SQLAlchemy, Pydantic |
| Database | PostgreSQL 16+ |
| Deployment | Docker Compose, nginx frontend proxy |
| Migration reference | Tauri/Rust/SQLite |

Current layout:

- `apps/api`: FastAPI backend, Alembic migrations, tests
- `src`: Svelte frontend, retained here until the web seam is stable enough to
  move to `apps/web`
- `docker/nginx`: frontend nginx config
- `docs`: active architecture, product, and parity docs
- `src-tauri`: temporary desktop migration reference

## API Examples

```bash
curl http://localhost:8080/api/v1/health
curl http://localhost:8080/api/v1/books
curl http://localhost:8080/api/v1/accounts
curl http://localhost:8080/api/v1/accounts/tree
curl http://localhost:8080/api/v1/accounts/2/register
curl http://localhost:8080/api/v1/transactions
curl http://localhost:8080/api/v1/reconciliation/accounts/2/start
curl http://localhost:8080/api/v1/imports/sessions
curl http://localhost:8080/api/v1/reports/definitions
curl http://localhost:8080/api/v1/pricing/refresh-state
curl http://localhost:8080/api/v1/pricing/refresh/execution-status
curl http://localhost:8080/api/v1/pricing/refresh/history
curl http://localhost:8080/api/v1/admin/runtime
```

Manual pricing refresh:

```bash
curl -X POST http://localhost:8080/api/v1/pricing/refresh/run \
  -H 'Content-Type: application/json' \
  -d '{"book_id":1}'
```

## Data And Backups

The self-hosted runtime uses PostgreSQL. Backups should use Postgres-native
operations such as `pg_dump`, `pg_restore`, and volume snapshots. Desktop-style
database folder selection and file-picker storage management are not part of the
target web product.

The active migration plan includes a dedicated operational milestone for
production Compose examples, backup jobs, restore smoke checks, and admin status
views.

## Active Documents

| Document | Purpose |
| --- | --- |
| [SELF_HOSTED_MIGRATION_PLAN.md](SELF_HOSTED_MIGRATION_PLAN.md) | Canonical migration and execution roadmap |
| [docs/product/v1-scope.md](docs/product/v1-scope.md) | Personal-first v1 product scope |
| [docs/architecture/postgres-schema.md](docs/architecture/postgres-schema.md) | PostgreSQL schema direction |
| [docs/architecture/post-b1-extensibility.md](docs/architecture/post-b1-extensibility.md) | Future plugin and theme architecture guardrails |
| [docs/parity/desktop-to-python.md](docs/parity/desktop-to-python.md) | Desktop-to-web parity matrix |

## Contribution Notes

- Prefer Python/FastAPI/PostgreSQL/Svelte web implementations for new work.
- Do not add new Tauri-dependent product features.
- Keep API behavior under `/api/v1`.
- Use typed Pydantic request/response schemas.
- Keep route components behind the shared frontend API seam.
- Preserve financial correctness with server-side validation and tests.
