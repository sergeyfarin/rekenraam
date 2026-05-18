# Rekenraam

Rekenraam is a self-hosted personal finance app with a SvelteKit frontend and
FastAPI backend. V1 is SQLite-only and runs as one Docker container on port
`16888` by default.

## What Works

- Double-entry accounts, transactions, splits, reconciliation, imports, exports,
  reports, pricing, planning, investments, auth, MFA, audit logging, and admin
  runtime checks.
- SQLite via `aiosqlite`, managed by Alembic, stored at
  `/data/rekenraam.sqlite3` in the `rekenraam_data` volume.
- SQLite connection pragmas are set by the API: foreign keys, WAL,
  `busy_timeout`, and `synchronous=NORMAL`.
- One-container deployment with optional Caddy HTTPS proxy overlay.
- Online SQLite backup tooling with integrity, migration, and books-table smoke
  validation.

## Run

```bash
docker compose up -d --build
curl --fail http://localhost:16888/api/v1/health
```

Override the host port with `APP_PORT`:

```bash
APP_PORT=18080 docker compose up -d --build
```

The app listens inside the container on `16888`.

## Public HTTPS

For a public deployment, set `.env` from `.env.production.example`, create the
first-admin password secret, and run:

```bash
docker compose -f compose.yaml -f compose.public.yaml -f compose.proxy.yaml up -d --build
```

See [docs/deployment/self-hosting.md](docs/deployment/self-hosting.md).

## Development

```bash
uv lock
make api-check
make api-lint
make api-format-check
make api-typecheck
make api-test-fast
make api-test
make api-migrate-smoke
npm run check
npm test
```

Playwright runs against the one-container stack:

```bash
docker compose up -d --build --wait
PLAYWRIGHT_BASE_URL=http://localhost:16888 npm run e2e
```

## Backups

```bash
make backup-now
make backup-smoke
make restore-smoke BACKUP=backups/rekenraam-YYYYmmdd-HHMMSS.sqlite3
```

Backups are created with Python's SQLite online backup API and validated with
`PRAGMA integrity_check`, `alembic_version`, and `books`.

## Key Docs

| Doc | Purpose |
| --- | --- |
| [docs/deployment/self-hosting.md](docs/deployment/self-hosting.md) | Single-container and HTTPS deployment |
| [docs/architecture/sqlite-schema.md](docs/architecture/sqlite-schema.md) | SQLite schema/runtime notes |
| [docs/parity/desktop-to-python.md](docs/parity/desktop-to-python.md) | Desktop-to-web parity status |
| [docs/product/v1-gap-plan.md](docs/product/v1-gap-plan.md) | Current v1 gaps and cleanup notes |
