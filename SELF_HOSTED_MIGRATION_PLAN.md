# Self-Hosted Migration Plan

Rekenraam V1 has moved to the Python/HTTP web stack with SQLite as the only
runtime database. The remaining migration work is product depth, polish, and
operational hardening around the one-container deployment.

## Summary

- SvelteKit frontend served by the app container.
- FastAPI backend in the same container.
- SQLite database at `/data/rekenraam.sqlite3`.
- Docker Compose default on `http://localhost:16888`.
- Optional Caddy HTTPS overlay.
- Online SQLite backup plus restore-smoke validation.

Tauri is no longer a product target. The old desktop tree remains only as a
parity and deletion-reference surface until product sign-off accepts the
remaining dropped or deferred desktop behaviors.

## Sources Of Truth

- [TODO.md](TODO.md)
- [docs/product/v1-scope.md](docs/product/v1-scope.md)
- [docs/product/v1-gap-plan.md](docs/product/v1-gap-plan.md)
- [docs/product/frontend-parity-plan.md](docs/product/frontend-parity-plan.md)
- [docs/parity/desktop-to-python.md](docs/parity/desktop-to-python.md)
- [docs/deployment/self-hosting.md](docs/deployment/self-hosting.md)

## Current Target

- SvelteKit frontend served by the app container.
- FastAPI backend in the same container.
- SQLite database at `/data/rekenraam.sqlite3`.
- Docker Compose default on `http://localhost:16888`.
- Optional Caddy HTTPS overlay.
- Online SQLite backup and restore-smoke validation.

## Important Runtime Choices

- Keep `book_id` columns and the multi-book schema shape.
- Keep the V1 runtime guard to the seeded book until create-books UX and
  authorization flows are ready.
- Keep SQLite parity triggers and ORM/schema drift tests.
- Keep the plugin/theme runtime deferred: no `/api/v1/plugins/*` or `/api/v1/themes/*` endpoints in b1.
  Extension work still requires semantic
  CSS tokens, WebAssembly or sidecar isolation, manifest-declared capabilities,
  admin review, disabled/failed-plugin isolation, deterministic fallback, and no
  arbitrary remote CSS loading.

## Completed Baseline

Working today:

- Docker Compose stack with SQLite by default: one container serves FastAPI and
  the built Svelte frontend from the same origin.
- First-admin bootstrap, login/logout, sessions, request context, protected API
  routes, password reset, MFA, invites, and audit visibility.
- Python-backed books, accounts, balances, transactions, metadata,
  reconciliation, imports, exports, reports, investments, pricing, planning,
  preferences, notes, and admin runtime checks.
- Shared frontend API seam under `src/lib/api` plus browser flows for the web
  product areas above.
- Root-level Tauri package, script, and Vite hooks already removed.

Still transitional:

- `src-tauri/` remains only as parity reference and deletion-audit material.
- Temporary v1 single-book mode remains enforced in the backend policy layer;
  migration work should not reintroduce scattered `book_id = 1` literals.
- The remaining roadmap is product breadth, correctness hardening,
  deployment/operator polish, and final Tauri deletion sign-off.

## Operational Checklist

```bash
uv lock
make api-check
make api-lint
make api-format-check
make api-typecheck
make api-test-fast
make api-test
make api-migrate-smoke
make prod-config-check
docker compose up -d --build --wait
curl --fail http://localhost:16888/api/v1/health
npm run check
npm test
PLAYWRIGHT_BASE_URL=http://localhost:16888 npm run e2e
make backup-smoke
make restore-smoke BACKUP=backups/rekenraam-YYYYmmdd-HHMMSS.sqlite3
```

## Follow-Up Log

- Keep watching SQLite write contention around long reconciliation flows; the
  current `database_locks` upsert path is the V1 serialization mechanism.
- Consider a CI lint for raw `server_default` strings so future migrations keep
  using explicit SQL defaults.
- Rename `scheduled_transactions.interval` to `interval_count` before broader
  external integrations depend on raw SQL column names.
- Keep the parity audit updated until `src-tauri/` is physically deleted so
  consciously dropped desktop behaviors do not disappear from the docs.
