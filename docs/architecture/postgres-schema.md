# PostgreSQL Schema Direction

Last updated: 2026-05-17

PostgreSQL is a post-v1 compatibility and hardening target. The v1 runtime and
CI release gate are SQLite-first; keep this document as the design map for
reintroducing PostgreSQL production support after v1 without weakening the
shared SQLAlchemy/Alembic model.

## Source Of Truth

The active schema source of truth is the Alembic baseline under
`apps/api/alembic/versions/0001_initial_schema.py`, plus the SQLAlchemy models
under `apps/api/src/rekenraam_api/db/models`.

The old Tauri SQLite schema remains a parity reference only. The active v1
runtime is the Docker SQLite app container backed by the SQLAlchemy/Alembic
schema in this repository.

## Core Decisions

- SQLite is the v1 target runtime database; PostgreSQL remains a post-v1 target.
- Currencies are modeled as commodity rows with `kind='currency'`; do not
  reintroduce a separate canonical `currencies` table.
- Immutable accounting history should be preserved with append-only/versioned
  rows or an equivalent history-preserving model.
- Audit attribution is separate from business version chains: authenticated
  user, session, device, request id, and timestamp belong to the web request
  context and audit model.
- Latest-state reads should use ordinary SQL views or repository query helpers
  first. Materialized projections are only justified by measured workloads.
- Durable invariants should be enforced with PostgreSQL constraints, foreign
  keys, partial indexes, and transaction-safe service validation.
- User-facing validation belongs in schemas/services so API errors stay clear.

## Active Table Families

Already represented in the web baseline:

- users, user devices, auth sessions, book memberships, MFA TOTP secrets,
  MFA recovery codes, MFA challenges, and password-reset tokens
- books
- commodities/currencies metadata
- countries and institutions
- categories, payees, tags, people, and projects
- accounts, account balancings, balance checks, balance adjustments, balance
  constraints, and reconciliation preferences
- transactions and splits
- investments, lots, split-lot allocations, and price observations
- investment instruments/security master, cost-basis profiles, and structured
  corporate actions
- pricing sources, policies, assignments, refresh state, and refresh runs
- import rules, import sessions, and import session transactions
- report state/cache, definitions, and runs
- user preferences
- saved transaction views, transaction templates, template splits, and payee
  defaults
- markdown notes
- audit events
- budgets and budget lines
- scheduled transactions and recurrence state
- loan/mortgage amortization helpers where account and transaction tables are
  not sufficient

Planned schema families:

- uploaded documents and attachment metadata
- events, if they remain distinct from audit events and notes
- operational backup metadata only where useful for server administration

Future reserved schema families:

- plugin manifests/status, permission grants, and runtime state only after the
  post-b1 plugin design is settled
- theme manifests only after token-pack theming is implemented; the existing
  user preference `theme` string remains the b1 compatibility foothold
- b1 must not add plugin/theme tables, no-op registries, or placeholder
  manifest tables; future plugin storage should be introduced only with a
  permissioned host/runtime design

## Desktop Concepts To Replace Or Drop

Replace:

- OS-login audit identity with authenticated user/session/device context.
- desktop database folder selection with Docker SQLite configuration and
  server admin status.
- desktop backup scheduling with SQLite volume/database backup guidance for v1;
  Postgres backups using `pg_dump`, `pg_restore`, and volume snapshots return
  with the post-v1 PostgreSQL runtime.
- desktop import/export needs with the web import/export pipeline.
  SQLite-to-PostgreSQL migration is deferred after v1 rather than a v1 release
  gate.

Drop as direct table ports:

- `app_runtime_session`
- `session_undo_stack`
- `session_redo_stack`
- `session_reverts`

Undo/redo can return later only as a server-safe mutation-history design.

## Invariant Placement

Prefer this split:

- Database constraints for simple durable facts such as enum-like values,
  precision ranges, required relationships, uniqueness, and ownership links.
  Prefer portable SQLAlchemy/Alembic constructs while v1 gates on SQLite.
- Service validation for cross-row checks that need helpful API errors or depend
  on richer domain rules.
- Repository transactions for operations that must update multiple table
  families atomically.
- Report invalidation through explicit service calls first; introduce database
  triggers only if they reduce real correctness risk.

Important invariant families to preserve:

- account, category, split, lot, commodity, price, import, reconciliation, and
  transaction rows must remain inside the same book where relevant
- transaction splits must balance according to the book/account commodity rules
- locked/reconciled ranges must reject unsafe mutations
- system accounts and system roles must be protected
- commodity scale/precision must stay inside supported bounds
- price observations must keep source/correction/derivation integrity
- investment valuation must keep instrument quantity, quote currency, account
  currency, and book/report currency distinct
- cost-basis profiles must be reusable by reports without embedding
  country-specific tax rules into core accounting
- import commits must be auditable and idempotent enough to avoid accidental
  duplicate posting

## Operational Schema Rules

- Alembic migrations are the only schema migration path.
- Tests should assert migration upgrade/downgrade smoke and ORM/schema parity.
- Do not create fake historical migration chains for already-squashed baseline
  work.
- New schema work should add explicit tests for table shape, indexes, foreign
  keys, unique constraints, and named checks where practical.
