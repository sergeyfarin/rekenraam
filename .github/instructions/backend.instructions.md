---
applyTo: "backend/**/*.go,backend/go.mod,backend/migrations/**"
description: "Use when editing Go backend code, migrations, repositories, services, handlers, or backend tests."
---

# Backend Instructions

- Keep HTTP handlers thin.
- Put business rules in `backend/internal/app/`.
- Keep database access in `backend/internal/db/`.
- Keep request and response DTOs near API handlers.
- Real endpoints belong under `/api/v1`.
- New schema changes require explicit migrations in `backend/migrations/`.
- Backend tests should prefer temporary SQLite databases.
- Preserve exact financial arithmetic and reconciliation-safe behavior.
- Built-in database records must use stable keys/codes for localized display labels instead of English text as the only source of truth.
- First-run setup creates the single owner with a username and password.
- Avoid introducing assumptions that depend on cloud services or external identity providers.
- If a backend change adds a durable domain rule, update `docs/product-requirements.md`, `docs/conventions.md`, or `docs/early-architecture-decisions.md` as appropriate.

## Go Conventions

- Thread `context.Context` as the first parameter through every function that does I/O, database access, or calls a service.
- Return errors explicitly; do not swallow them. Wrap with `fmt.Errorf("...: %w", err)` to preserve the chain.
- Package names should match the directory name and be a single lowercase word.
- Do not panic in handlers or services; return errors and let the caller decide.

## Error And Logging Conventions

- API error responses use the envelope shape `{"error": {"code": "STABLE_CODE", "message": "human-readable"}}`. The `code` field is a stable uppercase string. Never return raw Go error strings to the client.
- Use `log/slog` for all structured logging. Pass the logger via context or as a parameter; do not use a package-level global.
- Do not log financial values (amounts, payee names, account names) at any level.

## Infrastructure Conventions

- The `APP_ENV` environment variable controls mode: `development` (text logs, dev middleware) or `production` (JSON logs). Default to `production` when unset.
- HTTP middleware layers run in this order: request ID injection → request logging → auth check → handler. Each layer is a plain `http.Handler` wrapper.
- Inject a request-scoped UUID as a request ID on every inbound request. Include it in log entries and echo it as `X-Request-ID` in the response.
- Use `testify/assert` (non-fatal) and `testify/require` (fatal) in all Go tests. Do not write verbose manual assertion blocks.
- `sqlc`-generated code lives in `backend/internal/db/`. Do not edit generated files by hand; regenerate with `sqlc generate`.
- Backend tests use a temporary in-memory or temp-file SQLite database. Never use the development database file in tests.
- The server must handle `SIGTERM` and `SIGINT` with `http.Server.Shutdown(ctx)` and a short grace period.

## Backend Validation

- Default validation is `./scripts/test-backend.sh`.
- If the change affects the integrated app shape or static asset serving, also run `pnpm build` from repo root.
