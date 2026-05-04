# Rekenraam Self-Hosted Migration Plan

Last updated: 2026-05-04

## Summary

Rekenraam is migrating from a desktop-first Tauri/Rust/SQLite application to a
self-hosted finance web app:

- FastAPI backend
- PostgreSQL database
- SvelteKit frontend
- Docker-based development and deployment
- trusted plugin extension points
- themeable frontend

The v1 product target is personal finance first. Small-business use cases should
shape the architecture, but invoices, customers, VAT workflows, and other
business-only features are not v1 release gates.

Tauri is no longer a product target. The old desktop code remains only as a
temporary parity reference until required behavior has been migrated or
consciously dropped.

## Sources Of Truth

- Product scope: `docs/product/v1-scope.md`
- PostgreSQL schema direction: `docs/architecture/postgres-schema.md`
- Desktop-to-web parity tracking: `docs/parity/desktop-to-python.md`
- Deployment entrypoint: `compose.yaml`
- API source: `apps/api`
- Frontend source: `src` for now, with a later move to `apps/web`

Old desktop planning documents and TODO archives are intentionally removed once
their useful decisions are represented in these active docs.

## Current State

Working today:

- Docker Compose stack for `postgres`, `api`, and `frontend`
- FastAPI app under `/api/v1`
- PostgreSQL baseline managed by Alembic
- Svelte frontend served through nginx with same-origin `/api/v1` proxying
- Python-backed books, accounts, transactions, metadata, reports, investments,
  pricing settings, FX refresh status/history, and admin runtime health slices
- Shared frontend API seam under `src/lib/api`
- Report state/cache foundation with `book_state`, `report_cache`,
  `report_definitions`, and `report_runs`

Still transitional:

- frontend source remains in root `src`
- `src-tauri` remains as parity reference
- Tauri dependencies remain until the final deletion gate is met
- auth/audit/session attribution is not complete
- import, reconciliation, budgets, schedules, plugins, and themes still need
  first-class web implementations

## Architecture Defaults

Backend:

- Python 3.13+
- FastAPI
- Pydantic v2
- SQLAlchemy 2.x async + Alembic
- Pyright strict mode
- Ruff
- pytest

Frontend:

- SvelteKit 2 + Svelte 5 + TypeScript
- shared HTTP client under `src/lib/api`
- same-origin `/api/v1` in containerized deployment
- root `src` retained until the web seam is stable enough to move to `apps/web`

Database and operations:

- PostgreSQL 16+
- Docker Compose as the primary deployment path
- Postgres volumes and `pg_dump`/`pg_restore` for backups
- no desktop file-picker storage model in the self-hosted product

Correctness:

- thin FastAPI routes
- request/response shapes as typed Pydantic models
- service layer owns use cases and business rules
- repositories encapsulate SQLAlchemy/database access
- double-entry integrity must not rely on frontend validation
- immutable/versioned accounting history should be preserved

## Milestone 1: Roadmap And Documentation Reset

Goal: make the self-hosted roadmap the only active plan.

Tasks:

1. Keep this document as the canonical execution roadmap.
2. Move PostgreSQL schema direction into `docs/architecture/postgres-schema.md`.
3. Move product release scope into `docs/product/v1-scope.md`.
4. Keep parity state in `docs/parity/desktop-to-python.md`.
5. Update README so Docker/self-hosting is the first workflow.
6. Delete obsolete desktop-era planning archives after their useful content is
   represented in the active docs.

Exit criteria:

- no active docs point readers to old desktop TODOs as planning sources
- README starts with Docker/self-hosted setup
- old TODO archives are removed

## Milestone 2: Web-Native Frontend Cleanup

Goal: remove stale desktop assumptions from the current Svelte app without
prematurely moving folders.

Tasks:

1. Remove stale Tauri copy from app title, about page, layout comments, and
   README.
2. Remove direct Tauri route-layer dependencies where HTTP endpoints exist.
3. Keep all route data access behind `src/lib/api`.
4. Keep root `src` until the HTTP seam is stable; move to `apps/web` later.
5. Keep the frontend build Docker-compatible through `Dockerfile.frontend`.

Exit criteria:

- primary UI text describes the self-hosted web product
- remaining Tauri references are only explicit migration/parity references
- no route depends on desktop storage/file-picker behavior

## Milestone 3: Auth, Audit, And Request Context Foundation

Goal: establish the server identity model before broad immutable writes.

Tasks:

1. Add request context carrying `user_id`, `session_id`, `device_id`,
   `request_id`, and request timestamp.
2. Add minimal auth/session/device tables.
3. Add first-admin bootstrap.
4. Add Argon2id password login/logout.
5. Enforce book membership in every book-scoped service.
6. Stamp immutable writes with authenticated audit context.

Public API target:

- `/api/v1/auth/*`

Exit criteria:

- multi-user login works
- book access is enforced server-side
- write services can attribute who changed data, from which session, and from
  which device

## Milestone 4: Core Accounting Write Parity

Goal: make the core personal accounting workflows correct through Python and
HTTP.

Tasks:

1. Finish account create/update/delete, directives, close/open validation, and
   system-account protections.
2. Harden transaction create/update/delete/duplicate/bulk flows with service
   invariants and durable PostgreSQL constraints where appropriate.
3. Preserve append-only/versioned accounting history.
4. Add parity fixtures for balances, splits, voided transactions, revisions,
   and locked ranges.
5. Add cursor/keyset pagination for registers and long transaction lists.

Exit criteria:

- double-entry integrity is enforced server-side
- locked ranges are respected
- register and transaction lists scale beyond small demo data
- parity fixtures cover the core desktop accounting scenarios worth preserving

## Milestone 5: Reconciliation And Balancing

Goal: migrate statement reconciliation and balancing controls to web-native
services and UI.

Tasks:

1. Add `/api/v1/reconciliation*` service slice.
2. Port balancing history, unlock/void flows, statement workflows, and balance
   adjustments.
3. Represent `balance_checks`, `balance_adjustments`, and
   `balance_constraints` in the PostgreSQL/Python model.
4. Build the reconciliation UI against HTTP only.

Exit criteria:

- users can reconcile accounts, inspect history, unlock/void where permitted,
  and create balancing adjustments without desktop code

## Milestone 6: Import, Export, And Migration

Goal: provide the migration and day-to-day import/export paths required for a
self-hosted finance app.

Tasks:

1. Build a unified import pipeline for CSV, QIF, and OFX/QFX.
2. Add import sessions, preview, duplicate detection, mapping templates, and
   commit workflow.
3. Add SQLite desktop-to-Postgres import as a special importer.
4. Add CSV/QIF export and report CSV export before PDF.
5. Defer PDF statement parsing and bank connectivity until after v1.

Public API targets:

- `/api/v1/imports/*`
- `/api/v1/exports/*`

Exit criteria:

- existing desktop data can be migrated into Postgres through a tested path
- common bank/card files can be previewed, matched, and committed
- users can export core data without direct database access

## Milestone 7: Personal Finance Feature Expansion

Goal: widen from core accounting into the feature breadth expected from Firefly
III, GnuCash, MS Money, Quicken, and Money Manager Ex style applications.

Tasks:

1. Budgets: monthly/annual budgets, category targets, rollover, planned-vs-actual
   reporting.
2. Scheduled transactions: recurrence, reminders, skip/post flows.
3. Reports: net worth over time, account trends, budget variance, saved report
   execution, exportable report data.
4. Investments: reinvested dividends, corporate actions, stricter lot/account
   validations, portfolio performance.
5. Tax: capital gains and dividend summaries plus configurable category tax
   codes; country-specific exports later.

Public API targets:

- `/api/v1/budgets/*`
- `/api/v1/schedules/*`
- wider `/api/v1/reports/*`
- wider `/api/v1/investments/*`

Exit criteria:

- v1 personal-finance workflows are useful without returning to the desktop app
- small-business expansion remains possible without reshaping the core ledger

## Milestone 8: Plugins And Themes

Goal: make extension points explicit without allowing arbitrary remote code
execution.

Plugin model:

- trusted server-installed plugins only for v1
- install through Docker image layers or mounted volumes
- backend plugin manifests declare extension points
- frontend receives a constrained plugin manifest from the backend

Backend extension points:

- import providers
- report providers
- pricing providers
- transaction enrichment rules

Frontend extension points:

- navigation item
- settings panel
- report panel
- static assets owned by installed plugins

Theme model:

- built-in CSS token packs in the frontend
- custom theme manifests later through admin-managed files or mounted volumes
- theme selection persisted per user

Public API targets:

- `/api/v1/plugins/*`
- `/api/v1/themes/*`

Exit criteria:

- plugin manifests validate
- disabled or failing plugins do not break core startup
- frontend renders only allowed plugin slots
- theme token fallback is deterministic

## Milestone 9: Operational Self-Hosting

Goal: make the Docker deployment reliable for real self-hosting.

Tasks:

1. Keep `compose.yaml` as the primary deployment entrypoint.
2. Add production Compose example with Postgres volume, API, frontend/reverse
   proxy, backups, and environment docs.
3. Replace desktop storage UI with server admin status: DB host/name/version,
   migration status, writable check, and health check.
4. Add backup/restore docs and commands using `pg_dump`, `pg_restore`, and
   volume snapshots.
5. Add CI gates for API lint/typecheck/tests/migrations, frontend check/build,
   and Docker build.

Exit criteria:

- a fresh server can be deployed from documented Docker steps
- backup and restore are documented and smoke-tested
- CI covers the deployment-critical path

## Milestone 10: Final Tauri Deletion

Goal: remove the old desktop runtime once it is no longer needed as a parity
reference.

Deletion gate:

1. parity checklist is signed off for core workflows
2. no frontend runtime imports Tauri APIs
3. SQLite desktop-to-Postgres import is tested
4. Docker deployment is working
5. auth/audit context is in place for writes

Deletion tasks:

1. Delete `src-tauri/`.
2. Remove `@tauri-apps/*` dependencies and `tauri` scripts from `package.json`.
3. Remove Tauri-specific Vite/Svelte config.
4. Remove desktop-only README/contributing instructions.

Exit criteria:

- repo contains only the self-hosted web architecture and parity docs for
  historical context

## Testing Strategy

Backend:

- Ruff check
- Ruff format check
- Pyright strict
- pytest service/repository/API tests
- Alembic upgrade/downgrade smoke

Frontend:

- Svelte check
- production build
- route-level tests for API-driven flows as they are added

Parity:

- compare old SQLite fixtures against Python outputs for balances, registers,
  reports, investments, FX, and imports

Docker:

- Compose health smoke for Postgres, API, frontend, migrations, and documented
  backup/restore commands

Plugin/theme:

- manifest validation
- disabled-plugin behavior
- failed-plugin isolation
- theme token fallback

## Public Interface Roadmap

Keep all backend endpoints under `/api/v1`.

Current and planned slices:

- `/api/v1/books/*`
- `/api/v1/accounts/*`
- `/api/v1/transactions/*`
- `/api/v1/metadata/*`
- `/api/v1/reports/*`
- `/api/v1/investments/*`
- `/api/v1/pricing/*`
- `/api/v1/admin/*`
- `/api/v1/auth/*`
- `/api/v1/reconciliation/*`
- `/api/v1/imports/*`
- `/api/v1/exports/*`
- `/api/v1/budgets/*`
- `/api/v1/schedules/*`
- `/api/v1/plugins/*`
- `/api/v1/themes/*`

Every request and response shape should be represented by typed Pydantic models.

## Assumptions

- v1 is personal finance first.
- small-business features are deferred but not blocked architecturally.
- plugins are trusted and server-installed in v1.
- themes are token packs, not arbitrary frontend code.
- Tauri code is deleted only after the final deletion gate.
