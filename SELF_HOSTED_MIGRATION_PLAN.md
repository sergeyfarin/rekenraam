# Rekenraam Self-Hosted Migration Plan

Last updated: 2026-05-04

## Purpose

This document defines the migration path from the current desktop-first
Tauri + Rust + SQLite application to a self-hosted, multi-user finance
application with:

- Python backend
- PostgreSQL database
- SvelteKit frontend
- Docker-based local development and deployment

This plan replaces the previous Rust-backend direction. The new goal is a
Python-first server architecture with strong correctness guarantees enforced by:

- FastAPI
- Pydantic
- Pyright in strict mode
- Ruff
- frozen data structures for domain and DTO models

The end state is a Python + SvelteKit web application only. There is no target
desktop product in this migration plan. Tauri exists only as a temporary
migration reference and compatibility bridge until the required behavior has
been migrated or consciously dropped. Once that work is verified, the Tauri
path and related files should be removed.

Global instruction for all stages:

- do not preserve a desktop runtime as a product goal
- do not introduce new Tauri-dependent architecture or workflows
- migrate required behavior to Python + SvelteKit, then delete Tauri-specific code
- treat remaining Tauri usage only as transitional migration scaffolding

## Current Stage Status

- Stage 0: complete
- Stage 1: complete
- Stage 2: complete
- Stage 3: in progress
- Stage 4: in progress
- Stage 5: in progress
- Stage 6: not started
- Stage 7: not started
- Stage 8: not started
- Stage 9: in progress
- Stage 10: not started
- Stage 11: not started

Current evidence behind those statuses:

- Python FastAPI scaffold is active and validated in Docker.
- The empty app now uses one current initial Alembic schema rather than artificial historical migration steps.
- That single current baseline already carries the migrated investments tables plus the pricing tables, seeded providers, and persisted pricing refresh history used by the web app.
- Read-only books, accounts list/detail/tree/register, and transactions list/detail endpoints are implemented and covered by pytest plus Docker smoke checks.
- Read-only commodities, countries, and institutions endpoints now exist and are validated as frontend migration support metadata.
- Read-only categories, payees, tags, people, and projects endpoints now also exist and are validated as transaction-form/frontend migration support metadata.
- The Python transaction read slice now carries payee/reference, rich split metadata, and list filter/sort/pagination inputs needed by the current Svelte transaction views.
- Plain account balances now also have a Python endpoint and shared-client route usage for the accounts page and account detail page.
- Account-detail balancings, directives, and investment booking-policy reads now also have Python endpoints and shared-client route usage.
- Account-detail booking-policy updates now also use the shared client seam rather than direct Tauri writes.
- Account-detail register rows now load through the `/accounts/{id}/register` seam, with on-demand transaction detail reads for edit/status/delete actions.
- Account-detail transaction create/update/delete flows now also go through the shared client seam, the unlock-account-balancing helper now does as well, and the remaining create-category/payee/tag/person/project/account helpers on that page now also use shared client methods.
- Transaction create/update/delete plus duplicate/bulk-void/bulk-delete plus payee-default helpers now also have Python endpoints behind the shared client seam.
- Classic reports plus realized/unrealized gains now have Python endpoints, backed by Python lots, split-lot allocation, positions conversion, and price-observation schema.
- Investments now also have Python read/write coverage for positions, converted positions, lot holding periods, buy, sell, and cash dividend flows.
- Institution writes now also have Python-backed create/update/delete endpoints, and the institution settings screen no longer needs direct Tauri mutation calls.
- Commodity edit now also has a Python-backed update endpoint, and the commodity metadata editor no longer needs a direct Tauri mutation call.
- Currency list/create/update/default and activation now also have Python-backed endpoints, and the commodity settings read/write flows now use HTTP endpoints for pricing settings, source assignments, autocomplete, and manual daily/official FX entry.
- A shared frontend API seam now exists, and the accounts page account-tree plus account balances plus metadata lookups, the account-detail summary plus balances plus balancings plus directives plus booking-policy read/write plus form lookups plus register read plus on-demand transaction detail read plus transaction create/update/delete plus helper creation, the home/dashboard account plus balance plus payee plus recent-transaction reads, the settings categories/payees/tags and database/commodity read loads, and the transactions page form lookups plus transaction list read use it through HTTP.
- The dashboard startup and default-currency path now run as web-native HTTP behavior rather than desktop storage/file-picker behavior.
- The current Svelte frontend still lives in root `src/`, but direct frontend runtime imports of Tauri APIs are now out of the route layer and shared client seam; the remaining frontend cleanup is limited to explicit non-web guard paths and stale desktop-era copy/comments before final Tauri-path deletion.
- Parity tracking now lives in `docs/parity/desktop-to-python.md`.

## Executive Summary

The recommended path is still a staged monorepo migration, but the backend
implementation language changes from Rust to Python.

Recommended end state:

- `apps/web`: SvelteKit frontend
- `apps/api`: FastAPI backend
- `apps/worker`: background jobs and schedulers
- `packages/client`: generated or handwritten TypeScript API client
- `infra/`: Docker, Compose, reverse proxy, deployment files
- PostgreSQL as the only system of record

Do not delete the Tauri path before the required behavior has been migrated and
verified in Python + SvelteKit. Then remove `src-tauri/`, Tauri-specific frontend
code, and related packaging files in one controlled cleanup phase.

## What Changes In This Direction

The change is not only Rust to Python. It changes the implementation strategy:

- domain logic moves from Rust modules to Python packages
- Tauri command handlers are no longer the backend boundary
- the SQLite desktop schema becomes a migration source, not the target runtime
- correctness must be enforced through Python typing, validation, immutable data
  models, explicit service boundaries, and tests

## Architecture Goals

### Product Goals

- browser-based, self-hosted, multi-user finance app
- shared access across devices and users
- explicit auth and authorization
- correctness for accounting workflows
- incremental migration from current desktop implementation

### Technical Goals

- Python backend with clear internal layering
- PostgreSQL schema designed for long-term web use
- no hidden state in route handlers
- strict typing across backend and frontend interfaces
- explicit parity tracking against existing behavior that must survive the migration

## Recommended Stack

### Backend

- Python 3.13+
- FastAPI
- Pydantic v2
- SQLAlchemy 2.x with async support and Alembic
- `asyncpg` via SQLAlchemy async engine
- Pyright with `typeCheckingMode = strict`
- Ruff for linting and formatting
- pytest

### Frontend

- SvelteKit
- TypeScript
- adapter-node for self-hosting
- HTTP-only interaction with the backend
- infinite scrolling for long ledgers and registers; backend should support that UX with cursor or keyset-style fetches rather than page-number navigation

### Database and Infra

- PostgreSQL 16+
- Docker Compose for dev and initial production
- Caddy or Traefik for reverse proxy and TLS
- Redis optional later, not required initially

## Correctness Strategy For Python

Python is acceptable here only if the repo enforces discipline aggressively.
These rules are part of the architecture, not optional code style.

### Type Safety

- all backend packages must pass Pyright strict
- no untyped public service functions
- no `Any` in domain or service layers unless locally justified and isolated
- explicit return types on all exported functions

### Validation

- all request and response shapes defined as Pydantic models
- all command inputs validated before service execution
- all persistence outputs mapped into typed models before use in business logic

### Immutability

- domain objects and DTOs should be frozen Pydantic models where mutation is not required
- prefer tuples and frozen sets for internal immutable collections where useful
- avoid passing mutable dicts across service boundaries

### Repository Boundaries

- HTTP handlers do not write SQL
- service layer coordinates use cases and business rules
- repositories encapsulate database access
- domain rules do not depend on FastAPI or SQLAlchemy session objects

### Testing

- unit tests for pure domain logic
- repository tests against PostgreSQL
- API tests for HTTP behavior
- parity tests against known desktop behavior for important financial rules

## Recommended Repository Layout

Target structure:

```text
.
├── apps/
│   ├── api/
│   │   ├── pyproject.toml
│   │   ├── alembic.ini
│   │   ├── alembic/
│   │   ├── src/
│   │   │   └── rekenraam_api/
│   │   │       ├── app.py
│   │   │       ├── api/
│   │   │       ├── config/
│   │   │       ├── db/
│   │   │       ├── domain/
│   │   │       ├── services/
│   │   │       ├── repositories/
│   │   │       ├── schemas/
│   │   │       └── tests/
│   ├── worker/
│   └── web/
├── packages/
│   └── client/
├── docs/
│   ├── migration/
│   ├── parity/
│   └── architecture/
├── infra/
│   ├── compose/
│   ├── caddy/
│   └── scripts/
├── src/
├── src-tauri/
├── README.md
└── SELF_HOSTED_MIGRATION_PLAN.md
```

Transitional note:

- `src/` and `src-tauri/` remain until parity is achieved
- `apps/api/` becomes the new backend source of truth during migration
- once parity is verified, delete Tauri-related paths

## Backend Package Shape

Recommended backend package layout inside `apps/api/src/rekenraam_api/`:

```text
rekenraam_api/
├── api/
│   ├── dependencies.py
│   ├── errors.py
│   ├── router.py
│   └── v1/
│       ├── health.py
│       ├── books.py
│       ├── accounts.py
│       ├── transactions.py
│       ├── reports.py
│       └── ...
├── config/
│   ├── settings.py
│   └── logging.py
├── db/
│   ├── base.py
│   ├── session.py
│   ├── models/
│   └── migrations/
├── domain/
│   ├── accounts.py
│   ├── transactions.py
│   ├── reports.py
│   └── ...
├── repositories/
│   ├── books.py
│   ├── accounts.py
│   ├── transactions.py
│   └── ...
├── schemas/
│   ├── books.py
│   ├── accounts.py
│   ├── transactions.py
│   └── common.py
├── services/
│   ├── books.py
│   ├── accounts.py
│   ├── transactions.py
│   └── ...
└── tests/
```

## Standards To Enforce From Day One

### FastAPI

- thin route handlers only
- dependency injection only for auth, db session, config, request context
- all endpoints versioned under `/api/v1`

### Pydantic

- request and response models live in `schemas/`
- domain models that represent immutable business entities should be frozen
- use constrained types and validators rather than loose strings where practical

### Pyright

- strict mode enabled at project root for the backend
- fail CI on any type error
- no ignored files except generated or explicitly isolated integration glue

### Ruff

- lint + format enforced in CI
- import sorting included
- ban unused noqa clutter and dead imports

### Frozen Data Structures

Recommended defaults:

- Pydantic models: `ConfigDict(frozen=True)` unless mutation is necessary
- prefer dataclass `frozen=True` for internal pure domain helpers where Pydantic is unnecessary
- repository models are mapped into immutable domain or schema objects before leaving the repository layer

## Functional Parity Rule

The migration is not finished when the new stack is running. It is finished when
the important existing behavior has been migrated and verified in Python +
SvelteKit.

This means:

- every major Tauri command group must map to a Python service/module or be consciously dropped
- no Tauri path deletion until parity checklist is complete
- parity must be tracked in a document, not kept in memory

Recommended parity categories:

1. storage and migrations
2. books and currencies
3. accounts and account tree
4. transactions and splits
5. reconciliation
6. imports
7. reports
8. investments and lots
9. pricing / FX
10. settings and metadata
11. backup / restore
12. undo / redo policy replacement or conscious removal

Schema parity audit rule:

- treat table families, views, triggers, append-only/versioning rules, and audit/runtime state as separate checklist items
- when a Tauri schema item is absent from the current Postgres baseline, mark it as exactly one of: deferred, dropped, or explicit decision required
- do not mark a stage as full Tauri parity unless all three buckets are empty for that stage's intended scope

## Tauri Deletion Rule

Delete `src-tauri/` and related files only after all of the following are true:

1. Python backend covers all required MVP and currently used desktop functionality
2. frontend no longer imports Tauri APIs
3. parity checklist is signed off for core workflows
4. any required import or migration path from legacy SQLite data exists and is tested
5. production Docker deployment for the web app is working

Files to delete only at the end:

- `src-tauri/`
- Tauri dependencies in root `package.json`
- Tauri-specific Vite/Svelte config adjustments
- desktop-specific README sections

## Step-By-Step Concrete Implementation Plan

This is the concrete sequence to execute.

### Stage 0: Reset The Migration Direction

Status: complete

Goal:

- stop deepening the Rust-backend migration path
- make Python the official target backend

Tasks:

1. Update this migration plan to the Python direction.
2. Freeze further feature work in the Rust API scaffold except where needed for migration support.
3. Treat the current FastAPI migration as the only active backend direction.
4. Keep the legacy Tauri path only as temporary migration scaffolding while migrating behavior away from it.

Exit criteria:

- docs reflect Python backend direction
- implementation team follows Python plan, not Rust API expansion

### Stage 1: Scaffold Python Backend Correctly

Status: complete

Goal:

- create the long-term backend foundation with correctness tooling from day one

Tasks:

1. Create `apps/api/pyproject.toml`.
2. Add dependencies for FastAPI, Pydantic, SQLAlchemy, Alembic, asyncpg, uvicorn.
3. Add dev dependencies for Pyright, Ruff, pytest, pytest-asyncio, httpx.
4. Add `pyrightconfig.json` with strict mode.
5. Add Ruff config in `pyproject.toml`.
6. Create `src/rekenraam_api/` package structure.
7. Add settings management using Pydantic settings.
8. Add app factory, top-level router, and `/api/v1/health` endpoint.
9. Add Dockerfile for the Python API.
10. Add Compose service for the Python API beside Postgres.

Exit criteria:

- backend starts locally and in Docker
- `/api/v1/health` works
- Pyright strict passes
- Ruff passes

### Stage 2: Establish PostgreSQL Schema And Migration Workflow

Status: complete

Goal:

- make Alembic the only schema migration path for the new backend

Tasks:

1. Create Alembic config and migrations environment.
2. Create initial PostgreSQL schema for:
  - users
  - books
  - book memberships
  - currencies or base currency support
3. Add a backend schema version mechanism if operationally useful.
4. Add Makefile or task commands for:
  - create migration
  - apply migration
  - downgrade in dev
  - reset local database
5. Add migration smoke tests in CI.

Exit criteria:

- empty database can be created from scratch
- migrations are reproducible in Docker and CI

Progress note:

- Docker reproducibility is working and covered by smoke validation.
- Repository integration tests now run against ephemeral PostgreSQL.
- The empty-app schema has been squashed into a single current initial migration to avoid fake historical version churn.
- Migration tests now assert the full Stage 2 schema contract for every migrated table, index, foreign key, unique constraint, and named check constraint in the initial Alembic migration, including the investments and pricing tables that were added after the first web slices.
- ORM coverage and direct parity tests now cover the full Stage 2 table set so the SQLAlchemy model layer matches the initial Alembic schema rather than only the original `users` and `book_memberships` slice.
- Migration automation now includes explicit upgrade, downgrade, current-version, and Docker-backed smoke targets.
- Migration smoke is now wired into CI so upgrade/downgrade/re-upgrade validation runs automatically for API changes.
- The Tauri-only runtime/session tables `app_runtime_session`, `session_undo_stack`, `session_redo_stack`, and `session_reverts` are intentionally excluded from Stage 2 because they model desktop-process-local state, not shared server data. `app_runtime_session` is replaced by request/auth session context later in the web stack; the undo/redo tables have no direct PostgreSQL port and require a separate server-safe policy decision.

Stage 2 completion note:

- Stage 2 is complete for the narrowed web baseline schema, not for full legacy SQLite parity.
- Full Tauri-schema parity now requires tracking three separate buckets explicitly: intentionally deferred schema families, intentionally dropped desktop-only runtime state, and unresolved parity decisions that still need a web-side replacement or conscious removal.

Tauri-vs-Postgres schema gap checklist:

- Deferred to later web stages:
  - investments and lots: `lots`, `split_lot_allocations`, `corporate_actions`, `dividend_income_categories`
  - pricing and valuation: `price_ingest_runs`, `price_observations`, `price_sources`, `commodity_price_sources`, `pricing_policies`, `pricing_source_assignments`, `pricing_refresh_state`, `valuation_snapshots`, `valuation_snapshot_items`, `book_base_currency_history`
  - imports: `import_rules`, `import_sessions`, `import_session_transactions`
  - report state and caching even before frontend adoption: `book_state`, `report_cache`
  - reports beyond the current baseline: `report_definitions`, `report_runs`
  - reconciliation support tables: `balance_checks`, `balance_adjustments`, `balance_constraints`
  - note and attachment domain even before frontend adoption: `notes`, `events`, `documents`
  - backup replacement: `backup_settings`
- Intentionally dropped from the Stage 2 baseline:
  - desktop runtime/session tables: `app_runtime_session`, `session_undo_stack`, `session_redo_stack`, `session_reverts`
- Still requires explicit product or architecture decision:
  - SQLite trigger-enforced invariants such as same-book checks, datetime shape guards, system-role guards, and cache-bump triggers
  - per-trigger placement between PostgreSQL controls and service-layer validation for graceful user-facing errors

Required architecture decisions already made:

- preserve append-only immutability/version-chain behavior using `previous_*`, `session_id`, and append-only semantics because it supports time-machine behavior and is preferred over mutable-row replacement
- replace legacy `audit_user` / OS-login identity with authenticated user attribution plus persistent user-session-device audit trails
- treat `commodities` as the canonical asset master and represent currencies as commodity rows with `kind='currency'` rather than maintaining a separate canonical `currencies` table
- keep `current_*` latest-version projections, but implement them as ordinary PostgreSQL SQL views plus repository helpers by default instead of copying SQLite-specific trigger/view mechanics verbatim
- keep audit attribution separate from immutable business history: domain version chains preserve time-machine semantics, while explicit audit session/device records capture who, from which session, and from which device a change came

### Stage 3: Define The Python Domain Rules Before Porting Features

Status: in progress

Goal:

- avoid porting Tauri handlers directly into FastAPI route files

Tasks:

1. Create frozen domain models for books, accounts, transactions, splits, payees, categories.
2. Define canonical schema objects for inbound/outbound API payloads.
3. Define repository interfaces and service responsibilities.
4. Define error taxonomy:
  - validation
  - not found
  - conflict
  - authorization
  - infrastructure
5. Define request context shape with authenticated user and correlation id.

Exit criteria:

- service boundaries are explicit
- no feature port begins without target domain shape

Progress note:

- Frozen outbound schema models and repository/service separation exist for books and accounts.
- Account tree response shape now exists so frontend migration can start against a stable HTTP contract before transaction balances migrate.
- Minimal transaction and split read models now exist and drive account balances plus rollups.
- Account register reads with running balances now exist as a better finance-oriented read surface than raw transaction lists alone.
- Domain models, shared error taxonomy, and request context shape are still incomplete.

### Stage 4: Build A Parity Matrix Against Tauri

Status: in progress

Goal:

- ensure no desktop functionality is forgotten

Tasks:

1. Create `docs/parity/desktop-to-python.md`.
2. Inventory Tauri commands by module:
  - storage
  - accounts
  - transactions
  - reports
  - commodities / currencies
  - pricing / FX
  - imports
  - reconciliation
  - settings
3. For each command or feature, record:
  - current file/function
  - target Python module/service
  - target endpoint
  - status: not started / in progress / verified / dropped
4. Mark conscious removals explicitly.

Exit criteria:

- every meaningful Tauri capability is accounted for

Progress note:

- Initial parity matrix exists at `docs/parity/desktop-to-python.md`.
- The matrix still needs deeper per-command expansion for high-risk areas like transactions, reconciliation, and imports.

### Stage 5: Migrate Read-Only Slices First

Status: in progress

Goal:

- move low-risk functionality first and validate architecture

Recommended order:

1. books
2. accounts list/detail/tree
3. payees/categories/tags/institutions
4. reports metadata and simple read-only reports

Per slice tasks:

1. create SQLAlchemy models or table mappings
2. create repository module
3. create service module
4. create Pydantic response models
5. create FastAPI endpoints
6. add repository tests
7. add API tests
8. wire frontend client to new HTTP endpoint

Exit criteria per slice:

- route works
- tests pass
- frontend can read data through HTTP

Progress note:

- Books list/detail are verified.
- Accounts list/detail are verified.
- Accounts tree shape is now available and verified.
- Account register reads are now available and verified.
- Transaction-backed balances and rollups are now verified on seeded data.
- Transactions list filters are now available and verified.
- Read-only commodities, countries, and institutions are now available and verified as backend support for frontend migration.
- Read-only categories, payees, tags, people, and projects are now available and verified as backend support for transaction-form and register migration.
- The accounts page account tree plus balances plus create-edit metadata lookups, account detail summary plus balances plus balancings plus directives plus booking-policy read/write plus form lookups plus register read plus on-demand transaction detail read plus transaction create/update/delete, and transactions page form lookups plus transaction list read now read through the shared frontend client seam, proving the incremental HTTP migration path.
- Broader frontend HTTP integration is still pending, and that remains the highest-leverage next step before adding many more backend-only slices.

### Stage 6: Migrate Write Slices With Accounting Correctness First

Goal:

- preserve financial correctness while moving mutation logic to Python

Priority order:

1. account create/update/delete
2. transaction create/update/delete with splits
3. bulk transaction actions
4. reconciliation actions
5. imports

Per mutation slice tasks:

1. identify existing Rust validations and invariants
2. encode them in Python service layer
3. decide which invariants also belong in PostgreSQL constraints or triggers
4. implement transactional repository operations
5. add parity tests against known desktop scenarios
6. expose mutation endpoints
7. wire frontend mutation flows

Critical requirement:

- double-entry integrity must not rely only on frontend validation

### Stage 7: Migrate Complex Domains

Goal:

- port the most stateful and correctness-sensitive areas after the core is stable

Priority order:

1. reports calculations
2. imports and import rules
3. investments and lots
4. pricing / FX refresh
5. backup / restore strategy replacement for web
6. notes, events, and documents backend parity

Tasks for each complex domain:

1. document current Rust behavior
2. port rules into Python services
3. create regression fixtures from current app behavior
4. implement endpoints and jobs
5. validate on realistic data

### Stage 8: Introduce Audit Context, Auth, And Multi-User Access

Goal:

- move from personal desktop state to real shared server state

Tasks:

1. define request context shape for authenticated user, session, device, and correlation id
2. add persistent auth-session and device records so every immutable change can be attributed correctly
3. add audit trail fields and request context logging to mutation flows
4. add users table and membership model if not already present
5. add password hashing with Argon2id
6. add login, logout, session management
7. add access checks on every book-scoped service
8. add first-admin bootstrap flow

Exit criteria:

- multiple users can log in
- book access is enforced server-side
- audit trails can attribute changes to user, session, and device

Planning note:

- The audit-aware immutable write foundation should start before the full auth rollout is complete. Mutation design for append-only rows should not proceed without a clear request/session/device attribution model.

### Stage 9: Move Frontend Off Tauri Completely

Status: in progress

Goal:

- make the SvelteKit frontend web-native

Tasks:

1. create shared API client layer for HTTP calls
2. add a migration seam so existing route components can switch from direct `invoke()` calls to backend client methods incrementally
3. replace direct Tauri `invoke()` usage throughout frontend starting with home, transactions, accounts, and settings read flows
4. preserve infinite-scroll UX for registers and long transaction lists, backed by cursor or keyset pagination semantics at the API layer
5. move frontend to `apps/web` if not already done
6. switch from SPA/Tauri assumptions to adapter-node deployment
7. add auth-aware session handling in SvelteKit

Exit criteria:

- frontend runs only against HTTP API
- no runtime dependency on Tauri remains in the web frontend

Progress note:

- A shared frontend client seam now exists under `src/lib/api/`, and the migrated route flows now use it through HTTP rather than direct Tauri imports.
- The accounts page account-tree plus balance plus metadata lookup loads, the account-detail summary plus balance plus balancings plus directives plus booking-policy read/write plus form lookup loads plus register read plus on-demand transaction detail read plus transaction create/update/delete plus duplicate/bulk helpers plus unlock helper, and the transactions page form lookup loads plus transaction list read are now on that seam.
- The dashboard page now uses HTTP-only health and book metadata reads, and the old desktop storage/file-picker onboarding flow has been removed from that route.
- The reports page now uses Python HTTP endpoints for cashflow, category-spend, payee totals, and realized/unrealized gains, and the investments page now also has Python HTTP coverage for read models plus buy/sell/dividend mutations.
- The frontend is still rooted in `src/`, but the remaining migration cleanup is no longer the core read seam itself. Direct Tauri runtime imports are out of the frontend route layer, commodity autocomplete and manual price entry are HTTP-backed, and the remaining frontend work is to retire explicit non-web guard paths plus stale desktop-era assumptions before the final Tauri cleanup stage.
- The report-state/cache foundation is now landed in the web baseline and service layer: `book_state`, `report_cache`, `report_definitions`, and `report_runs` exist in the single current Alembic baseline, and current report endpoints now use persisted cache state plus invalidation bumps on report-relevant writes.
- The remaining report-area gap is no longer the cache foundation itself; it is widening that foundation into broader saved-report execution, broader cache coverage, and adjacent reporting features after the core frontend seam and write-path work are cleaner.
- The next frontend migration step should continue incremental route conversion, not a big-bang folder move.

## Recommended Immediate Next Execution Order

To complete the Rust/Tauri-era migration cleanly, the next work should be organized around four major milestones rather than around broad, overlapping stage names.

### Milestone A: Frontend Off The Critical Tauri Read Paths

Target outcome:

- the highest-traffic user journeys read through the shared HTTP client seam rather than direct `invoke()` calls

Priority work:

1. migrate the remaining dashboard/setup reads and read-only settings surfaces beyond categories/payees/tags
2. retire the remaining explicit non-web guard paths and stale desktop-era assumptions in the commodity/FX settings seam now that the read flows themselves are HTTP-backed
3. keep the register UX as infinite scroll while introducing backend cursor semantics for large result sets
4. continue route-by-route frontend conversion rather than moving `src/` to `apps/web` prematurely

Immediate route-by-route execution checklist:

1. Complete: `src/routes/+layout.svelte`: the layout no longer anchors the web app to desktop runtime behavior; remaining Tauri coupling is now below the shared client seam and admin/storage helpers rather than in the global shell.
2. Complete: `src/routes/+page.svelte`: dashboard health and default-currency reads are now HTTP-only, and desktop storage onboarding has been removed from that route.
3. Complete: `src/routes/settings/+page.svelte`: the settings shell is now thin, and the database plus year-end surfaces use the HTTP admin seam rather than desktop-only storage helpers.
4. Complete: `src/routes/settings/InstitutionSettings.svelte`: institution writes now use Python-backed endpoints and no longer need direct Tauri mutation calls.
5. `src/routes/settings/CommoditySettings.svelte`: currency activation now has an explicit Python-backed model: store `is_active` in `commodities.metadata_text` JSON, keep the default currency always active, and exclude inactive currencies from FX entry/refresh selectors while still showing them in settings. The FX settings tab now reads and writes Python-backed pricing configuration and status over `/api/v1/pricing/*` in web mode. Scheduled execution and manual refresh now run in a backend-owned Python worker with `/api/v1/pricing/refresh/run` and `/api/v1/pricing/refresh/execution-status`. The remaining cleanup here is no longer read-path migration; it is removing the explicit non-web guard paths and stale desktop-era assumptions around browser-owned FX automation.
6. `src/routes/reports/+page.svelte`: keep this on the Python seam and use it as the reference shape for future report invalidation/cache work.
7. `src/routes/investments/+page.svelte`: keep this on the Python seam and widen next into reinvested dividends or stricter account-type/booking validations rather than new Tauri wrappers.

Routes already sufficiently advanced for Milestone A and not the next priority:

1. `src/routes/accounts/+page.svelte`
2. `src/routes/accounts/[id]/+page.svelte`
3. `src/routes/transactions/+page.svelte`

Milestone A completion gate:

- the main layout, dashboard, classic reports landing flows, investments landing flows, and remaining settings read surfaces no longer need direct `@tauri-apps/api/core` imports and no longer depend on Tauri-backed read helpers in the shared frontend seam

### Milestone B: Audit-Aware Immutable Write Foundation

Target outcome:

- immutable write paths have a defined attribution model before broad mutation parity work starts

Priority work:

1. define request context for user, session, device, and correlation id
2. design the persistent audit-session/device model that replaces desktop runtime identity
3. decide how immutable rows are stamped with audit context during writes
4. implement the minimal backend infrastructure needed so transaction/account mutations do not have to guess later

Immediate design deliverables:

1. define a backend request-context object with `user_id`, `session_id`, `device_id`, `request_id`, and request timestamp semantics
2. define the persistent tables needed for web audit attribution, at minimum one auth-session table and one device-registration or device-fingerprint table
3. decide which immutable domain tables need audit stamp columns directly versus which can rely on related audit-event records
4. define how service-layer writes receive request context without coupling domain logic directly to FastAPI request objects
5. decide the first thin vertical slice that will prove the model, preferably transaction create/update/delete rather than a synthetic example

Milestone B completion gate:

- transaction/account write implementation can start without unresolved questions about who performed a change, from which session it came, or how that attribution is persisted

### Milestone C: Transaction/Account Write Parity

Target outcome:

- the core accounting write flows behave correctly through the Python backend and shared frontend seam

Priority work:

1. finish the remaining backend parity slices needed by forms and account/transaction mutations
2. implement transaction and account write-path invariants in services plus PostgreSQL where appropriate
3. add parity tests for known accounting scenarios and immutable-write behavior
4. migrate the corresponding frontend write flows fully off Tauri

### Milestone D: Report/Pricing/Investment Expansion

Target outcome:

- the complex domains expand only after the core read and write model is stable

Priority work:

1. widen the already-landed `book_state` and `report_cache` foundation into broader saved-report execution and adjacent reporting capabilities
2. migrate commodities and pricing-history foundations using the append-only plus `current_*` view approach
3. widen the now-landed lots/pricing foundation into reinvested dividends, corporate actions, and stricter investment booking validations before broader FX/report expansion
4. move the frontend from root `src/` into `apps/web` only after the client seam is stable enough to justify the move

This means the immediate execution order is:

1. finish Milestone A first
2. start the minimum viable Milestone B infrastructure before broad write migration
3. execute Milestone C next as the main correctness milestone
4. widen into Milestone D only after the core write model is proven

### Stage 10: Replace Or Retire Desktop-Only Concepts

Goal:

- remove behavior that only made sense in the old local Tauri application model

Candidates to replace or retire:

- local file chooser storage selection
- OS login derived audit identity
- desktop backup scheduling model
- Tauri undo/redo session implementation if not suitable for server model
- append-only/version-chain row model enforced through `previous_*`, `session_id`, and append-only triggers
- SQLite `current_*` views and trigger-driven latest-version projections
- report invalidation infrastructure built around `book_state` and `report_cache`
- notes, events, and documents if they are not part of the intended web MVP

Explicit runtime/session table decision:

- `app_runtime_session`: dropped as a database table. This was a singleton desktop-process marker and should be replaced by explicit request context plus authenticated web session handling, not by a PostgreSQL bootstrap table.
- `session_undo_stack`: dropped from the Stage 2 schema. Do not port this table directly; if undo remains a product requirement, replace it later with a server-safe mutation-history design.
- `session_redo_stack`: dropped from the Stage 2 schema for the same reason as `session_undo_stack`; no direct table port is planned.
- `session_reverts`: dropped from the Stage 2 schema. Any future replacement belongs with a redesigned server-side undo/audit subsystem, not the initial migration baseline.
- `audit_user` / OS-login audit identity: do not port the SQLite function model. Replace it with authenticated user identity plus persistent session-and-device-aware audit context.
- append-only/version-chain model: preserve it deliberately. The web architecture should keep immutable versioned rows or an equivalent append-only design rather than falling back to ordinary mutable rows.
- `current_*` views and trigger-driven invariants: still undecided. Each item should be reassigned explicitly to PostgreSQL constraints, repository queries, service-layer validation, or conscious removal.
- `book_state` / `report_cache`: migrate them even before a frontend depends on them, but redesign the implementation for a web-safe cache invalidation model.
- `notes`, `events`, `documents`: migrate them as backend parity work even if the frontend arrives later.

For each such feature:

1. define server equivalent
2. migrate if still valuable
3. otherwise explicitly drop and document

### Stage 11: Delete Tauri Path

Goal:

- remove the old desktop backend and packaging once parity is proven

Tasks:

1. verify parity checklist completion
2. verify frontend no longer uses Tauri APIs
3. remove `src-tauri/`
4. remove Tauri dependencies from root frontend config and package files
5. remove Tauri-specific docs and scripts
6. update README and onboarding docs to web-first instructions

Exit criteria:

- repo contains only the self-hosted web architecture

## Detailed First 10 Implementation Tasks

These are the next concrete execution steps to start the Python migration.

1. Complete: remove `apps/api` Rust backend scaffold or archive it as inactive.
2. Complete: create `apps/api` Python package scaffold with `pyproject.toml`.
3. Complete: add FastAPI app with `/api/v1/health`.
4. Complete: add Pyright strict and Ruff configuration.
5. Complete: add Dockerfile and Compose service for Python API.
6. Complete: add Alembic and initial Postgres migration for users/books/book_memberships.
7. Complete: add frozen Pydantic models for books.
8. Complete: add repository + service + endpoint for books list/detail.
9. Complete: add parity tracking document for Tauri-to-Python migration.
10. Complete: inventory `src-tauri/src/db_accounts.rs` and start the accounts read-only slice.

## Detailed Accounts Migration Checklist

Because accounts are the first real finance slice, use this sequence:

1. Complete: map current Rust `Account` fields to a Python target model.
2. Complete: decide which fields are required for the first read-only endpoint.
3. Complete: create Postgres accounts table and indexes.
4. Complete: create SQLAlchemy model and repository.
5. Complete: create frozen Pydantic output model.
6. Complete: create accounts service.
7. Complete: add `/api/v1/accounts`.
8. Complete: add `/api/v1/accounts/{id}`.
9. Complete: add tests against seeded data.
10. In progress: compare output to current desktop behavior on sample data and extend current transaction-backed balances toward richer metadata and parity cases.

## Testing Plan

### Backend

- pytest unit tests for domain logic
- repository tests against PostgreSQL
- API tests using HTTPX test client
- migration tests using ephemeral databases

Current implemented coverage:

- pytest service tests for books, accounts, and transactions mapping behavior, missing-record handling, and frozen outputs
- pytest API tests for health, books list/detail, accounts list/detail/tree/register, transactions list/detail, and 404 behavior using dependency overrides
- pytest repository tests against ephemeral PostgreSQL for books, accounts, transaction repositories, and seeded account balances
- pytest migration tests for upgrade/downgrade/re-upgrade plus full Stage 2 schema-contract assertions across tables, indexes, foreign keys, unique constraints, and named check constraints
- pytest ORM parity tests that compare the SQLAlchemy metadata against the Stage 2 Alembic schema contract across all modeled tables
- Docker smoke coverage for schema boot plus live `/api/v1/health`, `/api/v1/books`, `/api/v1/accounts`, `/api/v1/accounts/tree`, `/api/v1/accounts/{id}`, `/api/v1/accounts/2/register`, and `/api/v1/transactions`

Still missing:

- parity fixtures comparing desktop and Python outputs on the same sample data

### Frontend

- component tests for HTTP-driven views
- end-to-end tests for login, accounts, transactions, reconciliation, imports

### Parity Tests

- create fixtures from desktop SQLite files where useful
- compare balances, account lists, and report outputs between old and new implementations

## CI/CD Plan

Minimum CI for the Python backend:

- Ruff check
- Ruff format check
- Pyright strict
- pytest
- Alembic migration smoke test
- Docker image build

Frontend CI remains:

- Svelte typecheck
- build
- tests

## Security Baseline

Minimum requirements:

- HTTPS in production
- secure session or token handling
- Argon2id password hashing
- no secrets in source control
- explicit authorization checks in services
- audit logging for sensitive mutations

## Open Technical Defaults

Unless you want to override them, this plan assumes:

- SQLAlchemy 2.x + Alembic, not Django ORM or Tortoise
- async FastAPI backend
- frozen Pydantic v2 models for immutable schemas and domain DTOs
- pytest as the test runner
- one monorepo, not split repositories

## Final Recommendation

Build the self-hosted finance app as:

- Python backend with FastAPI
- PostgreSQL
- SvelteKit frontend
- Docker Compose deployment

Use strict typing and immutable models aggressively so the Python backend remains
predictable and safe. Migrate the current Tauri functionality slice by slice,
track parity explicitly, and delete the Tauri path only after the Python web
stack has proven functional coverage.