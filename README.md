# Rekenraam

Rekenraam is structured as a monorepo with separate backend, frontend, API-test, e2e-test, and deployment areas.

## Ports

- App/frontend development URL: `http://localhost:1888`
- Backend development API URL: `http://localhost:16888`
- Production single binary and Docker URL: `http://localhost:16888`

The port numbers echo the app's wealth-tracking purpose: `1888` for the frontend dev server and `16888` for the backend, both carrying the lucky `8` pattern.

## Layout

```text
backend/        Go backend, SQLite access, migrations, backend tests, and embedded frontend assets
frontend/       SvelteKit app compiled to static files
api/            Bruno API tests and OpenAPI description
e2e/            End-to-end browser tests
docs/           Product and architecture documentation (see docs/README.md)
scripts/        Build and developer workflow scripts
deploy/         Docker and deployment notes
dist/           Local release output
```

Area notes:

- `backend/cmd/rekenraam/` is the server binary entrypoint.
- `backend/internal/api/` holds HTTP handlers, middleware, and DTOs.
- `backend/internal/app/` holds application/service logic.
- `backend/internal/config/` holds configuration loading and validation.
- `backend/internal/db/` holds SQLite connection and repository code.
- `backend/migrations/` holds SQLite schema migrations.
- `backend/testdata/` holds Go test fixtures.
- `backend/var/` is for local SQLite databases and is ignored by Git.
- `backend/internal/web/dist/` is copied SvelteKit output for Go embedding.
- `frontend/src/` holds the SvelteKit app; `frontend/static/` holds static assets.
- `api/bruno/` holds Bruno requests and environments.
- `api/openapi/` holds the OpenAPI description.
- `e2e/` holds Playwright browser tests.
- `deploy/docker/` holds Dockerfile and Compose assets.
- `dist/` is for generated release artifacts and is ignored by Git.

## Architecture Notes

[docs/README.md](docs/README.md) is the documentation map. The four
current-state files are:

- [docs/roadmap.md](docs/roadmap.md) — the one prioritized plan for what is next.
- [docs/todo.md](docs/todo.md) — the short-horizon working queue and pending decisions.
- [docs/backlog.md](docs/backlog.md) — tracked defects, technical debt, and public-deployment gates.
- [docs/implemented.md](docs/implemented.md) — the feature ledger of what ships today.

Reference material is sorted by kind: governance in
[docs/product-requirements.md](docs/product-requirements.md) and
[docs/conventions.md](docs/conventions.md); accepted decision records in
[docs/adrs/](docs/adrs/) and [docs/early-architecture-decisions.md](docs/early-architecture-decisions.md);
feature plans in [docs/plans/](docs/plans/); durable design documents in
[docs/design/](docs/design/); dated audits, reviews, and analyses in
[docs/reviews/](docs/reviews/); superseded trackers and pre-Go-stack reviews
in [docs/archive/](docs/archive/). Developer workflow, commands, and commit
conventions are in [docs/developer-workflow.md](docs/developer-workflow.md).

Keep these documents current when a feature introduces a durable product or technical constraint.

The production frontend is static SvelteKit output built with `@sveltejs/adapter-static`. The Go binary embeds those files, serves real assets directly, returns API 404s under `/api/`, and falls back to the SvelteKit app shell for browser routes such as `/accounts` or `/transactions/import`.

## Documentation Shape

The repo keeps durable product and architecture decisions in `docs/` and keeps local README files only when they describe generated or ignored directories. Day-to-day commands live here and in [docs/developer-workflow.md](docs/developer-workflow.md), so area folders do not each need their own repeated command notes.

## Run Development Servers

Install workspace dependencies once from the repo root:

```sh
pnpm install
```

Start the backend and frontend together from the repo root:

```sh
pnpm dev
```

This uses `concurrently` to run both processes in one terminal with labeled `backend` and `frontend` logs. Stop both with `Ctrl+C`.

Open:

```text
http://localhost:1888
```

During development, SvelteKit serves the app on `1888` and proxies `/api` requests to the Go backend on `16888`.

You can still run each side separately when needed:

```sh
pnpm dev:backend
pnpm dev:frontend
```

## Test Backend

Backend tests are independent from Node and the frontend and run with Go's race
detector:

```sh
./scripts/test-backend.sh
```

Run the backend manually:

```sh
cd backend
go run ./cmd/rekenraam
```

Check the health API:

```sh
curl http://localhost:16888/api/v1/health
```

## Local Owner Recovery

Reset the owner password locally with a verified SQLite backup first:

```sh
printf '%s\n' 'new-password' | DATABASE_URL=file:backend/var/dev.sqlite go run ./backend/cmd/rekenraam recover-owner --password-stdin
```

Write the verified backup to an explicit path when needed:

```sh
printf '%s\n' 'new-password' | DATABASE_URL=file:backend/var/dev.sqlite go run ./backend/cmd/rekenraam recover-owner --password-stdin --backup-path dist/recovery.sqlite
```

Only use the emergency override when backup creation or verification is impossible and you accept the risk of proceeding without a fresh verified backup:

```sh
printf '%s\n' 'new-password' | DATABASE_URL=file:backend/var/dev.sqlite go run ./backend/cmd/rekenraam recover-owner --password-stdin --allow-no-backup
```

## Test Frontend

Install dependencies once:

```sh
pnpm install
```

Run SvelteKit checks and the Vitest unit suite:

```sh
./scripts/test-frontend.sh
```

Build the static frontend:

```sh
pnpm --dir frontend run build
```

## Test API With Bruno

Open `api/bruno/` in Bruno.

Use the `local` environment when the backend is running directly on `16888`.

Use the `app` environment when testing the integrated binary or Docker app on `16888`.

Keep Bruno requests grouped by feature area once real API routes exist.

## Test E2E

Install e2e dependencies once:

```sh
pnpm install
```

Run the self-contained e2e path from the repo root:

```sh
./scripts/test-e2e.sh
```

This builds the integrated app, starts a fresh local instance on `127.0.0.1:16889`, and uses a dedicated SQLite file at `backend/var/e2e.sqlite` for the test run. Set `E2E_PORT` to use a different self-managed test port.

For integrated app testing, run the single binary or Docker app on `16888`, then:

```sh
E2E_BASE_URL=http://localhost:16888 ./scripts/test-e2e.sh
```

E2E tests should cover browser-level user journeys. Prefer backend or frontend unit tests for logic that does not need a real browser.

Run the critical browser release preflight separately:

```sh
pnpm test:release-preflight
```

It serially covers split transfer entry, reconciliation to zero, QIF preview and
commit, investment buy plus sell preview and commit, and a mobile
transaction-entry flow. It is local-only until the journeys have proven stable.

## Build Single Binary

Install frontend dependencies first if needed:

```sh
pnpm install
```

Build the static frontend, copy it into the Go embed directory, and compile the binary:

```sh
./scripts/build-single-binary.sh
```

The output is:

```text
dist/rekenraam
```

## Deploy Single Binary

Use the [deployment-security guide](docs/deployment-security.md) for LAN and
reverse-proxy deployments. It covers the required first-run setup token,
HTTPS/TLS termination, proxy-header allowlisting, SQLite permissions, backups,
and secret-key custody. Do not expose the app's HTTP listener directly to the
internet.

## Backup And Restore

SQLite data lives wherever `DATABASE_URL` points. For the single-binary example above, that is:

```text
data/rekenraam.sqlite
```

For Docker Compose, SQLite data lives in the `rekenraam-data` Docker volume.

Prefer app-aware or stopped-app backups. Do not copy a live WAL-mode SQLite database file as the normal backup path.

Create a compact operator backup from the project root while the app is stopped or lightly used:

```sh
umask 077
sqlite3 data/rekenraam.sqlite "VACUUM INTO 'data/rekenraam-backup.sqlite'"
sqlite3 data/rekenraam-backup.sqlite "PRAGMA integrity_check"
sqlite3 data/rekenraam-backup.sqlite "PRAGMA foreign_key_check"
chmod 600 data/rekenraam-backup.sqlite
```

The integrity check should print `ok`; the foreign-key check should return no rows.

`umask 077` protects files created during the procedure and the explicit `chmod`
keeps the backup at mode `0600` even if the operator's umask differs.

Restore by stopping the app, moving the existing database aside, copying the verified backup into place, then starting the app again:

```sh
mv data/rekenraam.sqlite data/rekenraam.sqlite.before-restore
cp data/rekenraam-backup.sqlite data/rekenraam.sqlite
```

For Docker Compose, stop the app before restore and copy the verified backup into the mounted `rekenraam-data` volume. Keep the old database until the restored app has started and the setup/status endpoint responds successfully.

The local owner recovery command also creates and verifies a SQLite backup by default before changing the owner password.

## Online Provider Secret Key

Online provider credentials, such as Trading 212 API keys, are encrypted at rest
with `REKENRAAM_SECRET_KEY`. The value must be base64 for exactly 32 random bytes.
Generate one with:

```sh
openssl rand -base64 32
```

Keep this value in the service environment or secret manager and back it up with
the same care as the SQLite database. Losing it does not delete ledger data, but
it makes stored online provider credentials unreadable.

If `REKENRAAM_SECRET_KEY` is lost, restore it from backup if possible. If it
cannot be restored, stop the app, take and verify a SQLite backup, start the app
with a new key, delete the affected online import connections, and re-add them
with fresh provider credentials. Existing imported transactions, lots, batches,
and audit history remain in the database.

There is currently no in-place `REKENRAAM_SECRET_KEY` rotation command. To rotate
the key intentionally, use the same backup-first delete-and-re-add procedure for
stored online import connections. Do not change the key while expecting existing
connection secrets to keep working.

## Deploy Docker

Build and run with Docker Compose:

```sh
docker compose -f deploy/docker/compose.yaml up --build
```

The example binds only to `127.0.0.1:16888` and requires `SETUP_TOKEN`; put TLS
at a reverse proxy and follow the [deployment-security guide](docs/deployment-security.md)
before exposing it beyond the host. The Compose file mounts SQLite data into a
Docker volume named `rekenraam-data`.
