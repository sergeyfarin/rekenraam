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

This builds the integrated app, starts a fresh local instance on `127.0.0.1:16889`, and uses a dedicated SQLite file at `backend/var/e2e.sqlite` for the test run. Set `E2E_PORT` to use a different self-managed test port. The harness also sets a throwaway `REKENRAAM_SECRET_KEY` for that instance, since the two-factor journey cannot enrol without one.

Run only the fast journeys — everything except the serial release preflight. This is what CI runs on every push:

```sh
./scripts/test-e2e-smoke.sh
```

For integrated app testing, run the single binary or Docker app on `16888`, then:

```sh
E2E_BASE_URL=http://localhost:16888 ./scripts/test-e2e.sh
```

If the environment cannot download the Chromium revision Playwright pins, point it at a browser that is already installed instead of patching the browser cache:

```sh
PLAYWRIGHT_CHROMIUM_EXECUTABLE=/opt/pw-browsers/chromium ./scripts/test-e2e.sh
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

### Scheduled backups

The app backs itself up nightly, using SQLite's online backup API while it keeps
running. Nothing needs to be scheduled outside it.

- **Where:** `BACKUP_DIR`, or `<database directory>/backups` when unset. Point it
  at a **different device** from the database — a backup on the same disk
  survives corruption and mistakes, not disk loss.
- **When:** the owner's local time, 03:15 by default, with retention of the last
  14. Both are changed in the app, under the backup settings.
- **What you get:** one self-contained `rekenraam-<date>.sqlite` file per run,
  mode `0600`, verified with `integrity_check` and `foreign_key_check` before it
  is given its final name. A `.part` file is always a failed attempt.
- **What a full backup covers:** the database **and**, once attachments exist,
  the attachments directory. Attachment storage is not implemented yet, so
  today there is nothing beside the database to copy — the procedure names both
  from the start so that "backed up nightly" never has to be walked back when
  files land outside SQLite, where a database copy cannot reach them.
- **Whether it worked:** the app shows the last successful backup, the next
  scheduled one, and any failures with their reason — distinguishing a backup
  that will try again from one that has spent its attempts. A backup that has
  been failing all week is visible without reading logs.
- **If a night fails repeatedly:** attempts are bounded, and once they are
  spent that night has no backup and no further automatic attempt — retrying a
  full disk every minute helps nobody. Fix the cause and press retry; the next
  night is scheduled as usual.

**`REKENRAAM_SECRET_KEY` is not in any backup, and must not be.** It seals
two-factor enrolment and connection credentials, and a key stored beside the
data it protects protects nothing. Keep it in a password manager or secret store
**outside the backup directory**. Restoring a database without it leaves an
intact ledger, unreadable two-factor enrolment, and unusable connection
credentials — the ledger survives, those two must be set up again.

Pruning only ever deletes a file that the app recorded, that matches its own
naming, and that resolves to a regular file inside `BACKUP_DIR`. Anything else
in that directory is left alone.

### Checking a backup

Before trusting a backup — and ideally on a quiet Sunday rather than during an
incident:

```sh
./rekenraam verify-backup --from data/backups/rekenraam-2026-08-24.sqlite
```

It reports the integrity checks, the schema version against what this build
knows, row counts you can sanity-check, and whether the sealed rows (two-factor
enrolment, connection credentials) still decrypt under the
`REKENRAAM_SECRET_KEY` currently in the environment. That last line is the one
worth reading: it is the question a restore only raises once it is too late.

### Restoring

Stop the app first — the restore refuses to run while the server holds its lock,
and names the process that does.

```sh
./rekenraam restore --from data/backups/rekenraam-2026-08-24.sqlite
```

What it does, in order: proves the server is stopped, refuses if the backup and
the database are the same file or if the backup's schema is newer than this
build, checkpoints the current database so its write-ahead log is folded in,
moves the whole set (`.sqlite`, `-wal`, `-shm`) into
`<database>.before-restore-<timestamp>/`, then installs the backup atomically
and syncs it to disk before reporting success.

The previous database is kept, not deleted. Start the app, check that the
restored data looks right, and delete that directory yourself.

Set the **original** `REKENRAAM_SECRET_KEY` before starting the restored app. The
ledger does not need it; two-factor enrolment and stored connection credentials
do, and without it they must be set up again.

### Operator backups

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

If you are restoring by hand rather than with `rekenraam restore`, stop the app
and move the **whole set** aside — a WAL-mode database is three files, and
moving only the first discards transactions that were committed but not yet
checkpointed:

```sh
mkdir -p data/before-restore
mv data/rekenraam.sqlite data/rekenraam.sqlite-wal data/rekenraam.sqlite-shm data/before-restore/ 2>/dev/null
cp data/rekenraam-backup.sqlite data/rekenraam.sqlite
chmod 600 data/rekenraam.sqlite
```

`rekenraam restore` does all of that, plus the checkpoint, the atomic install,
and the running-server check. Prefer it.

For Docker Compose, stop the app before restore and copy the verified backup into the mounted `rekenraam-data` volume. Keep the old database until the restored app has started and the setup/status endpoint responds successfully.

The local owner recovery command also creates and verifies a SQLite backup by default before changing the owner password.

## Two-Factor Authentication

Turn it on at **Settings → Security**: scan the setup key into any authenticator
app, confirm one code, and save the ten recovery codes it shows — they are
displayed once, work once each, and are the only way in if the authenticator is
lost. Enrolment needs `REKENRAAM_SECRET_KEY` (below), because the shared secret
is encrypted at rest rather than stored in the clear.

After that, signing in asks for a code after the password. Wrong codes count
against the same login throttle as wrong passwords, and turning two-factor off
or replacing the recovery codes asks for the password again.

**Enrol before putting real financial data on the public internet** — see
`docs/deployment-security.md`. If both the authenticator and the recovery codes
are gone, the last resort is the `recover-owner` command on the host, which
resets the owner password and clears the enrolment.

## Online Provider Secret Key

Online provider credentials (such as Trading 212 API keys) and the two-factor
shared secret are encrypted at rest with `REKENRAAM_SECRET_KEY`. The value must be base64 for exactly 32 random bytes.
Generate one with:

```sh
openssl rand -base64 32
```

Keep this value in the service environment or secret manager and back it up with
the same care as the SQLite database. Losing it does not delete ledger data, but
it makes stored online provider credentials unreadable, and a two-factor
enrolment can then only be satisfied with a recovery code (those are hashed, not
encrypted, so they keep working).

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
