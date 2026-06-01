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
docs/           Architecture notes and early product decisions
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

Active product requirements and staged feature sequencing are documented in [docs/product-requirements.md](docs/product-requirements.md).

Repo-wide conventions are documented in [docs/conventions.md](docs/conventions.md).

Archive requirement review and near-term package or approach locks are documented in [docs/archive-requirements-review.md](docs/archive-requirements-review.md).

Accepted decision records are documented in [docs/adrs/](docs/adrs/).

Developer workflow, commands, and commit conventions are documented in [docs/developer-workflow.md](docs/developer-workflow.md).

Early architecture decisions are documented in [docs/early-architecture-decisions.md](docs/early-architecture-decisions.md).

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

Backend tests are independent from Node and the frontend:

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

Run SvelteKit checks:

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

Copy `dist/rekenraam` to the target machine.

Create a data directory for SQLite:

```sh
mkdir -p data
```

Run the app:

```sh
HTTP_ADDR=:16888 DATABASE_URL=file:data/rekenraam.sqlite ./dist/rekenraam
```

If the app runs behind a trusted reverse proxy that rewrites forwarding headers, opt in explicitly and allowlist the proxy source ranges:

```sh
HTTP_ADDR=:16888 DATABASE_URL=file:data/rekenraam.sqlite TRUST_PROXY_HEADERS=1 TRUSTED_PROXY_CIDRS=127.0.0.1/32,10.0.0.0/8 ./dist/rekenraam
```

Open:

```text
http://localhost:16888
```

For a real server, run the binary under a process manager such as `systemd`, and keep the SQLite data directory on persistent storage.

## Backup And Restore

SQLite data lives wherever `DATABASE_URL` points. For the single-binary example above, that is:

```text
data/rekenraam.sqlite
```

For Docker Compose, SQLite data lives in the `rekenraam-data` Docker volume.

Prefer app-aware or stopped-app backups. Do not copy a live WAL-mode SQLite database file as the normal backup path.

Create a compact operator backup from the project root while the app is stopped or lightly used:

```sh
sqlite3 data/rekenraam.sqlite "VACUUM INTO 'data/rekenraam-backup.sqlite'"
sqlite3 data/rekenraam-backup.sqlite "PRAGMA integrity_check"
```

The integrity check should print:

```text
ok
```

Restore by stopping the app, moving the existing database aside, copying the verified backup into place, then starting the app again:

```sh
mv data/rekenraam.sqlite data/rekenraam.sqlite.before-restore
cp data/rekenraam-backup.sqlite data/rekenraam.sqlite
```

For Docker Compose, stop the app before restore and copy the verified backup into the mounted `rekenraam-data` volume. Keep the old database until the restored app has started and the setup/status endpoint responds successfully.

The local owner recovery command also creates and verifies a SQLite backup by default before changing the owner password.

## Deploy Docker

Build and run with Docker Compose:

```sh
docker compose -f deploy/docker/compose.yaml up --build
```

Open:

```text
http://localhost:16888
```

The Compose file mounts SQLite data into a Docker volume named `rekenraam-data`.
