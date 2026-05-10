# Rekenraam Self-Hosted Migration Plan

Last updated: 2026-05-11

## Summary

Rekenraam is migrating from a desktop-first Tauri/Rust/SQLite application to a
self-hosted personal finance web app:

- FastAPI backend
- PostgreSQL database
- SvelteKit frontend
- Docker-based development and deployment
- architecture that can grow trusted plugin extension points after b1
- persisted theme preference kept as a future theming foothold

Milestones 1-9 are complete as baseline web slices, with Milestone 8 now
expanded as the investment-first v1 foundation. That means the core
self-hosted architecture, auth/request context, accounting writes,
reconciliation, import/export, reports, investments, pricing, and admin
foundations plus budgets, schedules, projected cash planning, and loans exist
in Python/HTTP/PostgreSQL. The remaining roadmap is about v1 feature breadth,
correctness hardening, operational readiness, and final Tauri removal.

The v1 product target is personal finance first. Small-business use cases should
shape the architecture, but invoices, customers, VAT workflows, and other
business-only features are not v1 release gates.

Tauri is no longer a product target. The old desktop code remains only as a
temporary parity reference until required behavior has been migrated or
consciously dropped.

## Sources Of Truth

- Live in-flight TODO dashboard: `TODO.md`
- Product scope: `docs/product/v1-scope.md`
- V1 gap analysis & fix plan: `docs/product/v1-gap-plan.md`
- PostgreSQL schema direction: `docs/architecture/postgres-schema.md`
- Post-b1 extensibility direction: `docs/architecture/post-b1-extensibility.md`
- Desktop-to-web parity tracking: `docs/parity/desktop-to-python.md`
- Deployment entrypoint: `compose.yaml`
- API source: `apps/api`
- Frontend source: `src` for now, with a later move to `apps/web`

Old desktop planning documents and TODO archives are intentionally removed once
their useful decisions are represented in these active docs.

## Completed Baseline

Working today:

- Docker Compose stack for `postgres`, `api`, and `frontend`
- FastAPI app under `/api/v1`
- PostgreSQL baseline managed by Alembic
- Svelte frontend served through nginx with same-origin `/api/v1` proxying
- first-admin bootstrap, password login/logout, sessions, device attribution,
  request context, and book membership enforcement
- Python-backed books, accounts, transactions, metadata, reconciliation,
  imports, exports, reports, investments, pricing settings, FX refresh
  status/history, admin runtime health, user administration, preferences,
  saved transaction views, transaction templates, payee defaults, notes,
  budgets, scheduled transactions, projected cash planning, loans, and audit
  slices, plus an extensible investment instrument master, cost-basis profiles,
  corporate-action records, non-currency market prices, source-health views,
  investment performance, account valuation, and currency exposure
- shared frontend API seam under `src/lib/api`
- frontend routes for dashboard, accounts, account registers, reconciliation,
  transactions, import/export, reports, investments, tax, planning, settings,
  and about
- PostgreSQL table families for users, devices, auth sessions, memberships,
  books, commodities/currencies, metadata, accounts, account balancings,
  reconciliation checks/adjustments/constraints, transactions, splits, lots,
  split-lot allocations, price observations, pricing configuration and refresh
  runs, imports, report state/cache, report definitions, report runs, budgets,
  budget targets, scheduled transactions, scheduled occurrences, loans, and
  loan terms
- backend tests for API, services, repositories, schema/migrations, auth,
  reconciliation, imports/exports, reports, investments, pricing, metadata, and
  admin workflows

Still transitional:

- frontend source remains in root `src`
- `src-tauri` remains as parity reference
- Tauri dependencies remain until the final deletion gate is met
- milestones 1-7 are baseline complete but still need broader v1 hardening,
  fixtures, and UX depth
- multi-currency transfers, tax-country modules, attachment/document uploads, backup/restore
  documentation, final CI/deployment gates, and post-b1 plugin/theme
  architecture remain to be completed or consciously deferred

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
- request attribution should include authenticated user, session, device,
  request id, and timestamp where writes are audited

## Milestone 1: Roadmap And Documentation Reset

Status: Complete.

Goal: make the self-hosted roadmap the only active plan.

Completed:

1. Keep this document as the canonical execution roadmap.
2. Move PostgreSQL schema direction into `docs/architecture/postgres-schema.md`.
3. Move product release scope into `docs/product/v1-scope.md`.
4. Keep parity state in `docs/parity/desktop-to-python.md`.
5. Update README so Docker/self-hosting is the first workflow.
6. Delete obsolete desktop-era planning archives after their useful content is
   represented in the active docs.

## Milestone 2: Web-Native Frontend Cleanup

Status: Complete.

Goal: remove stale desktop assumptions from the current Svelte app without
prematurely moving folders.

Completed:

1. Primary UI text describes the self-hosted web product.
2. Remaining Tauri references are explicit migration/parity references.
3. Route data access is behind `src/lib/api` for migrated HTTP flows.
4. Root `src` is retained until the HTTP seam is stable; move to `apps/web`
   later.
5. The frontend build remains Docker-compatible through `Dockerfile.frontend`.

## Milestone 3: Auth, Audit, And Request Context Foundation

Status: Complete as baseline.

Goal: establish the server identity model before broad immutable writes.

Completed:

1. Request context carries user, session, device, request id, and timestamp.
2. Users, book memberships, user devices, and auth sessions are represented in
   PostgreSQL.
3. First-admin bootstrap exists.
4. Password login/logout exists.
5. Protected routers require request context.
6. Book-scoped services have server-side access-policy enforcement.

Remaining hardening:

- user/admin management beyond first-admin bootstrap
- self-service password reset (token-based, single-use, 24h TTL) shipped
  2026-05-09 via `/api/v1/auth/password-reset/{request,confirm}`; SMTP delivery
  remains deferred and admins retrieve issued tokens from the audit log
- user invite/create/deactivate flows
- book role management UI and API
- user-visible audit trail for writes, imports, and admin actions

Public API:

- existing `/api/v1/auth/*`
- planned `/api/v1/admin/users/*` or `/api/v1/users/*`

## Milestone 4: Core Accounting Write Parity

Status: Complete as baseline.

Goal: make core personal accounting workflows correct through Python and HTTP.

Completed:

1. Account create/update/delete, directives, close/open validation, booking
   policy, system-account protections, tree, balances, and registers exist.
2. Transaction create/update/delete/duplicate/bulk flows exist with split
   support and service validation.
3. Locked/reconciled range checks are enforced in write paths.
4. Cursor/keyset pagination exists for register and transaction list flows.
5. Shared frontend routes use HTTP for core account/register/transaction flows.

Remaining hardening:

- broaden parity fixtures for balances, splits, voided transactions, revisions,
  locked ranges, and high-volume registers
- continue tightening durable PostgreSQL constraints where service validation is
  not enough
- preserve and document the append-only/versioned accounting history model
- finish advanced register filtering/search, saved views, memorized
  transactions, memorized splits, templates, and payee defaults
- add explicit multi-currency account and cross-currency transfer workflows

## Milestone 5: Reconciliation And Balancing

Status: Complete as baseline.

Goal: migrate statement reconciliation and balancing controls to web-native
services and UI.

Completed:

1. `/api/v1/reconciliation*` service slice exists.
2. Start, finish, history, unlock, constraint, and validation flows exist.
3. `balance_checks`, `balance_adjustments`, `balance_constraints`, and
   reconciliation preferences are represented in the PostgreSQL/Python model.
4. Account reconciliation UI uses HTTP.

Remaining hardening:

- broaden statement fixtures and account-type coverage
- improve balancing history review and audit visibility
- expand unlock/void policy tests and locked-range edge cases

## Milestone 6: Import, Export, And Migration

Status: Complete as baseline.

Goal: provide migration and day-to-day import/export paths required for a
self-hosted finance app.

Completed:

1. CSV, XLS/XLSX, QIF, and OFX/QFX preview pipelines exist.
2. Import sessions, preview, validation, duplicate matching, import rules, audit
   trail, and commit workflow exist.
3. CSV account export, CSV transaction export, QIF register export, and report
   CSV export exist.
4. SQLite desktop-to-Postgres import, PDF statement parsing, and bank
   connectivity remain deferred after v1.

Remaining hardening:

- broaden real-bank/card fixture coverage
- add import mapping template export/import
- add persistent cleanup/matching rules for payee normalization,
  category/account mapping, categorization cleanup, and duplicate preferences
- ensure import/export coverage is sufficient for data portability without
  direct database access

Public API:

- existing `/api/v1/imports/*`
- existing `/api/v1/exports/*`

## Milestone 7: Budgets, Schedules, Loans, And Planning

Status: Complete as baseline.

Goal: complete the personal finance planning workflows expected in v1.

Completed:

1. Budgets: monthly/annual budgets, category targets, rollover, and
   planned-vs-actual reporting.
2. Scheduled transactions: recurrence, reminders, projected instances, skip,
   edit, and post flows.
3. Planning: projected cash balance based on scheduled transactions.
4. Loans and mortgages: liability account helpers, amortization schedules,
   payoff views, and loan-payment assistant.
5. Add schema, services, HTTP APIs, and frontend routes without reintroducing
   desktop storage assumptions.

Remaining hardening:

- broaden recurrence coverage for end-of-month, yearly, and edited future
  occurrences
- add richer account/category selection validation and non-USD commodity support
- polish planning UX for editing/deleting targets, schedules, and loans
- expand loan workflows beyond fixed-rate monthly amortization

Public API targets:

- existing `/api/v1/budgets/*`
- existing `/api/v1/schedules/*`
- existing `/api/v1/loans/*`

Exit criteria:

- users can budget, schedule recurring activity, project near-term balances,
  and manage personal liabilities without returning to the desktop app

## Milestone 8: Investments, Pricing, Reports, And Valuation Expansion

Status: Complete as baseline.

Goal: widen the already migrated investment, pricing, and reporting
foundations into v1-complete personal finance valuation workflows while keeping
country-specific tax calculations deferred.

Completed:

1. Instrument/security master exists on top of commodities for stocks, ETFs,
   funds, bonds, options, futures, crypto spot assets, private investments, and
   generic instruments.
2. Investment valuation separates account currency, instrument quantity, quote
   currency, and book/report currency.
3. Cost-basis profiles exist per book with FIFO, LIFO, average-cost, and
   specific-lot modes so personal accounting, broker reconciliation, and future
   tax interpretations can use different policies.
4. Reinvested dividends, short sale/cover endpoints, generic derivative/private
   investment event recording, and structured corporate-action records exist
   for splits, spin-offs, mergers/acquisitions, conversions, return of capital,
   delisting, and write-offs.
5. Pricing supports manual market-price observations for non-currency
   instruments, per-instrument source assignment through existing pricing
   assignments, historical/as-of lookup, and source health/status summaries.
6. Investment/reporting surfaces include portfolio performance, account
   valuation, book-level currency exposure without forced conversion,
   policy-aware realized/unrealized gains, corporate-action history, net worth,
   and account trends.
7. Tax-specific calculations are consciously deferred after v1 because tax
   treatment is country-specific and may later be implemented through trusted
   plugins or optional modules.

Remaining hardening:

- broaden fixtures for multi-currency brokerage accounts where quote currency,
  account currency, and book currency differ
- deepen derivative lifecycle posting beyond event records where accounting
  mechanics are clear
- add richer private-investment valuation workflows and audit references
- extend corporate-action generated posting for complex actions such as
  mixed-consideration mergers and cash-in-lieu
- improve investment/report chart UX and print-friendly exports
- measure whether persisted valuation snapshots are needed for heavy reports

Public API targets:

- existing wider `/api/v1/investments/*`
- existing wider `/api/v1/pricing/*`
- existing wider `/api/v1/reports/*`
- no v1 country-specific `/api/v1/tax/*` calculation surface

Exit criteria:

- v1 investment, valuation, pricing, and investment-focused reporting workflows
  are useful without desktop parity fallbacks

## Milestone 9: Search, Templates, Preferences, Notes, And Administration

Status: Complete as baseline.

Goal: add the everyday ergonomics and administration expected from a self-hosted
personal finance product.

Completed:

1. Transaction finder, advanced register filtering/search, saved views, and
   keyboard-first quick entry foundations.
2. Memorized transactions, payee defaults, memorized splits, and transaction
   templates baseline.
3. Per-user preferences: default book, date format, number format, locale,
   theme selection, and other display preferences baseline.
4. Markdown notes on accounts and transactions.
5. Admin account management: create/invite/deactivate users, change/reset
   passwords, manage book roles, inspect integrity checks, view migration
   version, and review audit/import history.

Remaining hardening:

- broaden saved-view and template UX from baseline controls into polished daily
  workflows
- add richer audit coverage for every write family
- defer file attachments/document uploads until after v1
- keep email invite/reset delivery deferred after v1

Public API targets:

- existing `/api/v1/search/transaction-views/*`
- existing `/api/v1/templates/transactions/*`
- existing `/api/v1/preferences/*`
- `/api/v1/notes/*` and `/api/v1/documents/*` if included in v1
- existing `/api/v1/notes/*`
- existing `/api/v1/admin/users/*`
- existing `/api/v1/admin/audit-events`

Exit criteria:

- repeated data-entry, review, and administration workflows are efficient and
  auditable for a self-hosted household or small trusted group

## Milestone 10: Post-B1 Plugin And Theme Architecture

Goal: keep b1/v1 unblocked by plugin/theme implementation while preserving an
additive path for trusted extensions after b1.

Status: Ready for b1 as an extensibility guardrail.

Compatibility guardrails for b1:

- core behavior stays behind typed `/api/v1` APIs
- backend logic remains separated across routes, services, and repositories
- frontend routes continue using `src/lib/api` rather than direct backend
  assumptions
- user preferences keep the existing `theme` string so built-in themes can
  arrive later without breaking preference data
- semantic CSS tokens cover core surfaces, status states, charts, money colors,
  and account/transaction concepts so later theme packs can swap token values
  instead of rewriting route markup
- `/api/v1/plugins/*` and `/api/v1/themes/*` remain unimplemented and covered
  by negative route tests

Future plugin candidates:

- import providers
- report providers
- pricing providers
- transaction enrichment rules
- static plugin assets
- Docker image layer or mounted-volume installation for trusted local plugins

Future plugin host model:

- prefer sandboxed WebAssembly plugins for constrained extension logic after
  evaluating the Component Model, WASI capabilities, Wasmtime, and
  Extism-style host functions
- support arbitrary-language plugins through isolated sidecar containers over a
  narrow local HTTP/gRPC contract rather than in-process Python package loading
- expose typed host capabilities instead of database credentials, repositories,
  or internal SQLAlchemy models
- enforce permissions at the host-function/API boundary with per-book grants,
  network allowlists, plugin storage quotas, timeout/memory limits, explicit
  secret grants, disabled/failed-plugin isolation, and full audit attribution

Future frontend candidates:

- manifest-driven navigation items
- manifest-driven settings panels
- manifest-driven report panels
- constrained plugin asset references served by the backend

Future runtime/security questions:

- evaluate Extism/WASM for server-side plugins and any limited
  frontend-compatible execution after b1
- design granular permission manifests before executing plugin code
- consider GitHub-sourced manifests or plugin bundles only after local trusted
  installation and permission review are designed
- do not allow arbitrary downloaded plugin execution in b1/v1

Future theme candidates:

- built-in CSS token packs in the frontend
- custom theme manifests through admin-managed files or mounted volumes
- deterministic token fallback
- existing per-user theme preference remains the stable selection field
- custom CSS, if ever allowed, requires admin enablement, validation, strict CSP
  guidance, deterministic fallback, and no arbitrary remote CSS loading

Public API reservation:

- no `/api/v1/plugins/*` or `/api/v1/themes/*` endpoints in b1
- reserve `/api/v1/plugins/*` and `/api/v1/themes/*` as future additive
  namespaces only
- do not introduce plugin tables, theme tables, manifest schemas, permission
  models, Extism/WASM dependencies, or no-op registries before b1

Implementation notes:

- the b1 reference architecture is documented in
  `docs/architecture/post-b1-extensibility.md`
- tests assert reserved namespaces stay absent, the `theme` preference remains
  future-compatible, and docs keep plugin/theme execution deferred

## Milestone 11: Operational Self-Hosting

Goal: make the Docker deployment reliable for real self-hosting.

Tasks:

1. Keep `compose.yaml` as the primary deployment entrypoint.
2. Add production Compose example with Postgres volume, API, frontend/reverse
   proxy, backups, file-backed secrets, and environment docs for both VPS and
   home/LAN installs.
3. Replace desktop storage UI with server admin status: DB host/name/version,
   migration status, writable check, health check, backup guidance, and
   structured integrity check.
4. Add backup/restore docs and commands using `pg_dump`, `pg_restore`, and
   volume snapshots.
5. Smoke-test backup and restore docs.
6. Add CI gates for API lint/typecheck/tests/migrations, frontend check/build,
   Docker build, and Compose health.
7. Harden public VPS login with secure cookie settings, trusted proxy handling,
   failed-login rate limiting, and optional/enforced TOTP MFA with recovery
   codes.

Exit criteria:

- a fresh server can be deployed from documented Docker steps
- backup and restore are documented and smoke-tested
- CI covers the deployment-critical path
- public VPS docs require HTTPS, secure cookies, private Postgres, and MFA

## Milestone 12: Final Tauri Deletion

Goal: remove the old desktop runtime once it is no longer needed as a parity
reference.

Deletion gate:

1. Parity checklist is signed off for core workflows.
2. No frontend runtime imports Tauri APIs.
3. Import/export parity is tested without a desktop migration requirement.
4. Docker deployment is working from fresh setup docs.
5. Auth/audit context is in place for writes.
6. Operational backup/restore path is documented and smoke-tested.

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
- Ruff format check (not yet enforced; bulk reformat tracked in TODO.md)
- Pyright strict
- pytest service/repository/API tests
- Alembic upgrade/downgrade smoke
- ORM-vs-migrated-Postgres schema-drift contract (rebuilt 2026-05-11 to be
  derived from `Base.metadata`; covers all tables, columns, indexes, FKs,
  unique constraints, CHECK names, and `server_default` values)
- auth/session/device/request-context tests
- book access-policy tests
- reconciliation/import/export/report/investment/pricing/admin tests

Frontend:

- Svelte check
- production build
- route-level tests for API-driven flows as they are added
- smoke checks for auth, account/register, reconciliation, import/export,
  reports, settings, and admin flows

Parity:

- compare old SQLite fixtures against Python outputs for balances, registers,
  reports, investments, FX/pricing, reconciliation, imports, and exports
- explicitly mark desktop behaviors as verified, migrated with changed shape,
  deferred, or dropped

Docker:

- Compose health smoke for Postgres, API, frontend, migrations, and documented
  backup/restore commands

Post-b1 plugin/theme:

- docs consistency scan confirms plugins/themes are not b1/v1 release gates
- future implementation must add manifest validation, disabled/failed-plugin
  isolation, permission checks, and theme token fallback before execution

Docs:

- scan active docs for stale milestone/status wording after each roadmap update
- keep README, product scope, schema direction, parity matrix, and this roadmap
  aligned

## Public Interface Roadmap

Keep all backend endpoints under `/api/v1`.

Existing slices:

- `/api/v1/health`
- `/api/v1/auth/*`
- `/api/v1/books/*`
- `/api/v1/accounts/*`
- `/api/v1/transactions/*`
- `/api/v1/metadata/*` through metadata routes such as commodities,
  currencies, countries, institutions, categories, payees, tags, people, and
  projects
- `/api/v1/reconciliation/*`
- `/api/v1/imports/*`
- `/api/v1/exports/*`
- `/api/v1/reports/*`
- `/api/v1/investments/*`
- `/api/v1/pricing/*`
- `/api/v1/admin/*`

Planned or widened slices:

- `/api/v1/admin/users/*` or `/api/v1/users/*`
- `/api/v1/budgets/*`
- `/api/v1/schedules/*`
- `/api/v1/loans/*`
- `/api/v1/search/*` or saved-view endpoints under existing slices
- `/api/v1/templates/*`
- `/api/v1/preferences/*`
- `/api/v1/notes/*`
- `/api/v1/documents/*`
- wider `/api/v1/reports/*`
- wider `/api/v1/investments/*`
- wider `/api/v1/pricing/*`

Future additive reservations:

- `/api/v1/plugins/*`
- `/api/v1/themes/*`

Every request and response shape should be represented by typed Pydantic models.

## Assumptions

- v1 is personal finance first.
- small-business features are deferred but not blocked architecturally.
- milestones 1-6 are baseline complete, not final v1 feature-complete.
- SQLite desktop import remains deferred after v1.
- b1 is the first beta/release-readiness milestone before a fuller post-b1
  extension system.
- plugins, plugin execution, granular permissions, GitHub-sourced manifests,
  and Extism/WASM evaluation are deferred until after b1/v1.
- built-in theme implementation is not required for b1; the existing persisted
  `theme` preference is the compatibility foothold.
- notes/documents are useful v1 candidates but should not delay core release
  gates.
- Tauri code is deleted only after the final deletion gate.
