# Rekenraam

Rekenraam is a self-hosted personal finance app with a SvelteKit frontend and
FastAPI backend. V1 is SQLite-only and runs as one Docker container on port
`16888` by default.

## Current Status

Working today:

- Docker Compose stack with SQLite by default: one container serves FastAPI and
  the built Svelte frontend from the same origin.
- Same-origin `/api/v1` frontend-to-backend traffic in the containerized web
  stack.
- First-admin bootstrap, login/logout, sessions, device attribution, request
  context, protected API routes, password reset, MFA, and invites.
- Python HTTP endpoints for books, accounts, balances, registers,
  transactions, metadata, reconciliation, imports, exports, reports,
  investments, pricing settings, FX refresh state/history, and admin runtime
  health.
- Local user administration, book roles, preferences, saved transaction views,
  transaction templates, markdown notes, audit-event visibility, budgets,
  schedules, projected cash, loans, and cross-currency transfer UI/API.
- Vitest unit/component tests and Playwright e2e coverage for the migrated web
  flows.

Still being tightened:

- Multi-book runtime access remains intentionally gated to the seeded book until
  create-books UX and authorization flows are ready.
- Daily-use polish for transaction search, saved views, templates, memorized
  splits, and payee defaults.
- Broader operator warnings and load coverage for public-host deployments,
  reconciliation, and imports.

Deferred after v1:

- Plugin execution, frontend plugin slots, granular permissions, and any
  WebAssembly or sidecar runtime.
- Arbitrary remote CSS loading and richer built-in theme packs beyond semantic
  tokens plus the persisted `theme` preference.
- Attachment/document uploads, OCR, open banking, and server-side undo/redo.

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
make ci
```

`make ci` runs the always-on GitHub checks locally: Python compile/lint/format/
typecheck/coverage, Svelte typecheck, Vitest, frontend build, and the
self-hosting Docker smoke test.

For narrower loops during development:

```bash
make api-test-fast
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
| [docs/product/v1-scope.md](docs/product/v1-scope.md) | Detailed v1 requirements and release gate |
| [docs/product/phase-3-plan.md](docs/product/phase-3-plan.md) | Detailed frontend test and refactor history |
