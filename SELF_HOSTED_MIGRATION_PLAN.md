# Rekenraam Self-Hosted Migration Plan

Last updated: 2026-05-02

## Purpose

This document defines the recommended migration path from the current desktop-first
Tauri + SQLite application to a self-hosted, multi-user finance application with:

- Rust backend
- PostgreSQL database
- SvelteKit frontend
- Docker-based local development and production deployment

This is a product and platform migration, not just a framework swap. The core
requirements change from local single-user desktop usage to shared server-side
state, authenticated users, and browser-based access across devices.

## Executive Summary

The best path is a staged monorepo migration that preserves the existing Rust
domain logic while replacing Tauri-only runtime assumptions with a transport-
agnostic backend service layer.

Recommended end state:

- `apps/web`: SvelteKit frontend for browser clients
- `apps/api`: Rust HTTP API server
- `packages/types`: shared API contracts / DTO schemas
- `infra/`: Docker, Compose, reverse proxy, deployment files
- PostgreSQL as the system of record
- Background workers inside the Rust backend or a separate worker binary
- Tauri retained only temporarily or as an optional future desktop shell

Do not create a separate repository yet. Keep a single repo until the new web
product is stable. The current codebase contains substantial reusable Rust domain
logic, tests, and schema knowledge that would be unnecessarily fragmented by an
early repo split.

## Why The Current Architecture Must Change

The current repo is optimized for a desktop app:

- Frontend calls Tauri commands directly via `invoke(...)`
- SvelteKit is configured as a static SPA with `ssr = false`
- Storage lifecycle depends on local file dialogs and local filesystem ownership
- Database state is in-process and built around a single local SQLite file
- Audit identity is derived from the operating system login instead of app users

Those are all acceptable for desktop, but not for a self-hosted multi-user app.

## Target Architecture

## 1. Product Shape

Deploy a browser-based web application with a separate API backend and a shared
PostgreSQL database.

### Frontend

- SvelteKit
- SSR enabled for authenticated app shell and faster first load
- Client-side interactivity for register, reports, filters, and long forms
- All data access through HTTP API, never direct database access

### Backend

- Rust with Axum
- Clear layering:
  - HTTP handlers
  - application services
  - domain logic
  - repositories / persistence adapters
- Authentication, authorization, audit logging, import processing, report jobs,
  and background refresh handled server-side

### Database

- PostgreSQL 16+
- SQL migrations with a real migration tool
- Row ownership and access rules designed for multi-user books
- Explicit indexes for register, reports, imports, and pricing queries

### Deployment

- Docker Compose for local development and single-host deployment
- Reverse proxy in front of app containers
- Persistent Postgres volume
- Optional object storage later for documents / attachments

## 2. Recommended Repository Layout

Target layout:

```text
.
├── apps/
│   ├── api/                # Rust Axum API server
│   ├── worker/             # Optional Rust worker for scheduled jobs
│   ├── web/                # SvelteKit web frontend
│   └── desktop/            # Optional temporary Tauri shell during migration
├── crates/
│   ├── domain/             # Business rules, validation, invariants
│   ├── application/        # Use cases / service layer
│   ├── persistence/        # DB adapters, repository implementations
│   ├── api-contracts/      # Shared DTOs / serde schemas / OpenAPI support
│   └── test-support/       # Fixtures, builders, integration helpers
├── packages/
│   └── types/              # Generated TS API types if needed
├── infra/
│   ├── docker/
│   ├── compose/
│   ├── nginx/              # or caddy/traefik
│   └── scripts/
├── docs/
│   ├── architecture/
│   ├── operations/
│   └── migration/
├── migrations/             # PostgreSQL migrations
└── README.md
```

Transitional layout is acceptable at first. You do not need to move everything
on day one. But the destination should be explicit.

## 3. Core Architectural Decisions

### Keep Rust

Keep Rust for the backend. The repo already contains substantial domain logic in
Rust for:

- accounts
- transactions and splits
- investments and lots
- reporting
- import pipeline
- pricing / FX
- validation and invariants

Rewriting this into TypeScript would slow the migration, reduce confidence, and
discard existing test coverage.

### Replace Tauri Commands With Service Calls

Tauri command handlers should stop being the business logic boundary.

Move to:

- thin HTTP handlers in `apps/api`
- thin Tauri handlers if desktop is retained temporarily
- shared application services in reusable crates

This is the most important refactor in the entire migration.

### Move From SQLite-Centric Design To Postgres-Ready Design

The current code relies on:

- SQLite PRAGMAs
- SQLite triggers and append-only enforcement
- `rusqlite`
- local connection management assumptions

That needs a deliberate persistence redesign. Some concepts carry over cleanly,
but the implementation should target Postgres first, not emulate SQLite forever.

### Add Real User And Access Model

The current schema is book-centric, which is useful. The self-hosted model should
introduce:

- `users`
- `organizations` or `households` if shared spaces are needed
- `book_memberships`
- roles such as `owner`, `editor`, `viewer`
- audit attribution by authenticated user id

## 4. Recommended Stack

### Backend

- Rust stable
- Axum
- Tokio
- SQLx or SeaORM for PostgreSQL access
- `serde` / `serde_json`
- `tracing` and `tracing-subscriber`
- `utoipa` or equivalent for OpenAPI generation
- `tower-http` for CORS, compression, trace layers

Recommendation: prefer SQLx over an ORM-heavy stack. This app is query-heavy and
financially sensitive. Explicit SQL is usually clearer and easier to optimize.

### Frontend

- SvelteKit
- adapter-node for self-hosting behind reverse proxy
- TypeScript
- existing component stack can be retained if still suitable

### Database and Infra

- PostgreSQL 16+
- Redis only if later needed for queues, rate limits, or cache invalidation
- Docker Compose for local dev and initial production
- Caddy or Traefik for TLS termination and reverse proxy on a single host

## 5. Required Domain Changes For Multi-User Web

These are not optional if the app becomes shared and self-hosted.

### Authentication

Support one of:

- email + password with secure reset flow
- OIDC only
- both

Recommendation: start with local auth plus optional OIDC later.

Requirements:

- password hashing with Argon2id
- session or token strategy defined explicitly
- CSRF protection if cookie auth is used
- secure password reset tokens
- email verification optional for self-hosted setups

### Authorization

Every data access path must validate book access. Do not rely on frontend route
protection. Authorization belongs in the backend service layer.

### Auditability

Every mutation should record:

- authenticated user id
- timestamp
- request id
- originating book id
- relevant entity ids

### Attachments And Files

Current document flows likely assume local filesystem access. For web:

- start with server-managed local storage volume
- later support S3-compatible object storage if needed
- store only metadata in Postgres

### Background Jobs

Server-side jobs will be needed for:

- FX refresh
- backup jobs
- report cache pruning
- import processing if large files are supported

Avoid tying these jobs to web request lifecycles.

## 6. Migration Strategy

Use staged migration. Do not attempt a single big-bang rewrite.

### Phase 0: Freeze The Direction

Goals:

- keep `main` usable
- create the future target shape in docs
- avoid mixing old Tauri assumptions into new work

Tasks:

- create this migration plan
- create a dedicated long-lived refactor branch when implementation starts
- keep the current desktop app buildable until the web path is stable

Exit criteria:

- architecture accepted
- repo strategy accepted
- implementation sequence agreed

### Phase 1: Introduce Backend Boundaries Without Changing Product Behavior

Goals:

- isolate business logic from Tauri
- reduce direct frontend coupling to `invoke`

Tasks:

- create a frontend API client abstraction
- move `src/lib/api/*` toward a backend-agnostic interface
- extract service modules from Tauri command files
- make Tauri commands thin wrappers around services
- define request / response DTOs independent of Tauri

Exit criteria:

- at least one vertical slice works through shared services
- no new business logic added directly to Tauri command handlers

### Phase 2: Create The Web Backend Skeleton

Goals:

- stand up a real Rust HTTP server
- prove one vertical slice end-to-end

Tasks:

- add `apps/api`
- add health endpoint, auth placeholder, structured error model, tracing setup
- expose one slice first, preferably `accounts` or `commodities`
- add OpenAPI generation
- add integration tests against Postgres

Exit criteria:

- browser client can call the new API for at least one slice
- backend starts under Docker Compose

### Phase 3: Introduce PostgreSQL

Goals:

- establish the long-term database model
- stop deepening SQLite-only dependencies

Tasks:

- design Postgres schema equivalent to current core model
- create migration toolchain
- port seed/reference data intentionally, not by blind SQL translation
- decide which append-only invariants belong in DB constraints, triggers, or app services
- define UUID strategy for externally visible ids

Important:

Do not carry forward every SQLite trigger mechanically. Re-evaluate each invariant.
Some belong in the database, some in the service layer, and some in both.

Exit criteria:

- core entities exist in Postgres
- schema migration story is reliable in dev and CI
- one end-to-end write flow works against Postgres

### Phase 4: Add Auth And Multi-User Model

Goals:

- make shared usage real, not theoretical

Tasks:

- add users, sessions, memberships, roles
- add audit identity based on authenticated user
- protect all routes and API handlers
- add first-run admin bootstrap flow

Exit criteria:

- multiple users can authenticate
- permissions are enforced per book

### Phase 5: Migrate Frontend From Tauri SPA To Web App

Goals:

- make SvelteKit a real server-hosted web frontend

Tasks:

- move frontend to `apps/web`
- switch from `adapter-static` to `adapter-node`
- enable SSR where beneficial
- replace all remaining `@tauri-apps/api` imports
- add auth-aware route guards and session handling

Exit criteria:

- app works in browser without Tauri runtime
- login, core account views, transaction flows work end-to-end

### Phase 6: Migrate Remaining Vertical Slices

Priority order:

1. accounts
2. transactions/register
3. categories/payees/tags/institutions
4. reports
5. commodities/investments
6. import pipeline
7. pricing / FX refresh
8. backups / storage / documents

Rationale:

- accounts and transactions establish the app's core value
- reports depend on clean transaction and classification data
- investments and imports are higher complexity and should not be the first slice

### Phase 7: Operations Hardening

Tasks:

- Docker production images
- Compose deployment profile
- reverse proxy and TLS
- backups for Postgres and uploaded documents
- metrics and log aggregation guidance
- health checks and readiness checks
- restore test procedure

Exit criteria:

- single-host deployment is documented and reproducible
- backup and restore are tested

### Phase 8: Retire Or Minimize Desktop-Specific Code

Options:

- fully retire Tauri
- keep Tauri as a thin desktop shell pointed at the same API
- keep a local-only desktop edition temporarily

Recommendation: do not decide this too early. First get the web product stable.

## 7. Docker Strategy

### Development Compose

Use Docker Compose for local development with at least:

- `postgres`
- `api`
- `web`
- optional `mailpit` for auth email flows

Recommended dev flow:

- run Postgres in Docker
- optionally run `api` and `web` outside Docker for faster edit cycles
- keep full Compose support for onboarding and CI parity

Example services:

- `postgres`: persistent named volume, init env vars, healthcheck
- `api`: mounted source in dev or built image in prod
- `web`: SvelteKit node server or dev server depending on mode
- `proxy`: only needed in production-like local tests

### Production Compose

For a single VPS / home server deployment:

- `caddy` or `traefik`
- `web`
- `api`
- `postgres`
- optional `worker`

Requirements:

- no database exposed publicly
- secrets injected through environment or mounted secret files
- persistent volumes for database and uploads
- automated backups outside the running container filesystem

### Container Build Guidance

- multi-stage Docker builds
- minimal runtime images
- non-root user in runtime containers
- pinned base image families
- healthcheck endpoints for `api` and `web`

## 8. Data Migration Strategy

There are two separate migrations:

1. architecture migration
2. existing user data migration from SQLite to Postgres

Treat them separately.

### Initial Recommendation

Do not block backend architecture migration on perfect automated data import.
First make the new system correct for fresh Postgres data. Then build a one-way
import from the current SQLite format.

### Data Migration Approach

- define canonical entity mapping from SQLite schema to Postgres schema
- build import command in Rust
- run import idempotently into an empty target book or workspace
- verify with reconciliation checks and row-count sanity checks

Do not attempt live bi-directional sync between desktop SQLite and Postgres.
That is a separate product problem.

## 9. Testing Strategy

### Backend

- unit tests for pure domain logic
- repository tests against ephemeral Postgres
- integration tests for HTTP handlers
- migration tests on empty and semi-realistic databases

### Frontend

- component tests for critical interactions
- end-to-end tests for login, accounts, transaction creation, reconciliation,
  and import wizard

### Contract

- OpenAPI generation checked in CI
- frontend type generation from API schema where useful

## 10. CI/CD Recommendations

Add CI before the refactor gets deep.

Minimum CI jobs:

- frontend typecheck and build
- Rust fmt, clippy, test
- Postgres-backed integration tests
- Docker image build smoke test

Later:

- dependency scanning
- image scanning
- migration drift detection

## 11. Security And Operations Baseline

Minimum baseline for a self-hosted finance app:

- HTTPS by default
- secure headers at reverse proxy
- rate limits on auth endpoints
- Argon2id password hashing
- encrypted backups where appropriate
- audit logs for sensitive mutations
- documented secret rotation process
- no direct DB admin endpoints in app UI

## 12. What To Preserve From The Current System

Preserve these concepts during migration:

- strong domain validation
- double-entry integrity
- append-only thinking where it protects auditability
- test coverage for core finance rules
- explicit reporting logic rather than opaque ORM behavior

## 13. What To Drop Or Rework

Do not carry these assumptions forward unchanged:

- local filesystem pickers as storage setup
- OS login as audit user identity
- direct Tauri `invoke` coupling from route components
- SQLite-specific concurrency tuning as a system design foundation
- SPA-only deployment assumptions

## 14. Recommended First Implementation Sprint

The first implementation sprint should be small and architecture-heavy.

### Sprint A

- create `docs/` and move this plan there later if repo restructuring begins
- add `apps/api` Rust server skeleton
- add `apps/web` skeleton or prepare current frontend for later move
- extract one service slice from Tauri command handlers
- define shared error model and API DTOs
- run Postgres in Docker Compose
- implement `GET /health` and one real read endpoint

### Success Condition

You can boot:

- Postgres in Docker
- Rust API locally or in Docker
- SvelteKit frontend locally

and load one real screen using HTTP instead of Tauri.

## 15. Recommended Order Of Repository Changes

1. Add docs and target layout plan
2. Add Docker Compose and Postgres dev setup
3. Add `apps/api`
4. Extract shared Rust service layer
5. Move first vertical slice to HTTP
6. Add auth foundation
7. Move frontend to web-first setup
8. Port remaining slices
9. Add production deployment and backup docs

This order reduces the risk of a half-migrated repo with no stable execution path.

## 16. Non-Goals For The First Refactor Wave

Avoid these early:

- Kubernetes
- microservices
- event sourcing rewrite
- real-time collaboration
- mobile apps
- bi-directional desktop/web sync

They add complexity before the core web product is stable.

## 17. Final Recommendation

Build a monorepo self-hosted web app with:

- Rust Axum backend
- PostgreSQL
- SvelteKit frontend
- Docker Compose deployment

Keep the current repo, preserve the Rust domain logic, and perform the migration
in staged vertical slices. The controlling technical task is not "switch to web".
It is "separate business logic from Tauri and redesign persistence for Postgres
and multi-user access."