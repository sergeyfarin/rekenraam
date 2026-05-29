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

## Backend Validation

- Default validation is `cd backend && go test ./...`.
- If the change affects the integrated app shape or static asset serving, also run `pnpm build` from repo root.
