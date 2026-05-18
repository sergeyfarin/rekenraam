# SQLite Schema Notes

SQLite is the only V1 runtime database. The schema keeps `book_id` columns and a
multi-book shape so later product work can enable additional books without a
schema reset, but V1 runtime access remains guarded to the seeded book.

## Source Of Truth

The active schema source of truth is the Alembic baseline under
`apps/api/alembic/versions/0001_initial_schema.py` plus the SQLAlchemy models
under `apps/api/src/rekenraam_api/db/models`.

The old Tauri SQLite schema remains parity and deletion-reference material only.
The active runtime schema is the SQLAlchemy/Alembic sqlite model used by the app
container.

## Core Decisions

- SQLite is the only v1 runtime database.
- `book_id` remains in the schema even though v1 runtime access is intentionally
  gated to the seeded book.
- Currencies are modeled as commodity rows with `kind='currency'`; do not split
  them back into a separate canonical currency table.
- Immutable or recoverable accounting history should remain a design goal even
  where current v1 slices still use mutable or void-style semantics.
- Audit attribution is separate from business data: authenticated user,
  session, device, request id, and timestamp belong to the web request context
  and audit model.
- Latest-state reads should prefer ordinary queries or repository helpers first;
  extra projections are justified only by measured workloads.
- User-facing validation belongs in schemas and services so API errors stay
  clear, while sqlite-side triggers or constraints protect the invariants that
  should survive service-layer bypass.

## Runtime Pragmas

The API installs these pragmas on every SQLite connection:

- `foreign_keys=ON`
- `journal_mode=WAL`
- `busy_timeout=5000`
- `synchronous=NORMAL`

## Migrations

Alembic owns the schema. The baseline includes auth/session tables, books,
accounts, transactions, imports, reconciliation, reports, pricing, planning,
investments, audit log, and runtime lock rows.

SQLite-specific parity triggers remain in the baseline for cross-table
invariants that are hard to express with ordinary constraints.

## Active Table Families

Already represented in the sqlite web baseline:

- users, user devices, auth sessions, memberships, password-reset tokens, MFA
  secrets, recovery codes, and challenges
- books
- commodities/currencies metadata
- countries and institutions
- categories, payees, tags, people, and projects
- accounts, account balancings, balance checks, adjustments, constraints, and
  reconciliation preferences
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
- audit log
- budgets and budget lines
- scheduled transactions and recurrence state
- loan/amortization helpers where account and transaction tables are not enough

Planned schema families still consciously deferred:

- uploaded documents and attachment metadata
- user-facing events if they remain distinct from audit events and notes
- plugin/theme runtime tables, manifests, permissions, or registries

## Desktop Concepts Replaced Or Dropped

Replaced:

- OS-login identity with authenticated user/session/device context.
- Desktop database-folder selection with Docker sqlite configuration and server
  runtime status.
- Desktop backup scheduling with online-backup tooling plus restore-smoke
  validation.
- Desktop import/export assumptions with the web upload and export pipeline.

Dropped as direct table ports:

- `app_runtime_session`
- `session_undo_stack`
- `session_redo_stack`
- `session_reverts`

Undo/redo can return later only as a server-safe mutation-history design.

## Invariant Placement

Keep the split explicit:

- SQLite constraints and triggers protect invariants that should survive direct
  SQL mistakes or future service drift.
- Services and schemas own user-facing validation, authorization, and clearer
  error messages.
- Repositories should keep query shapes readable and not hide critical
  accounting rules inside convenience helpers.
- Schema-contract tests and migration-smoke checks are the main guard against
  ORM/schema drift in the sqlite-only path.

## Backups

Backups use Python's SQLite online backup API. Backup smoke validates:

- `PRAGMA integrity_check`
- an `alembic_version` row
- readability of the `books` table

Live file copies are discouraged because WAL databases may have committed data
outside the main database file.

## Compatibility Note

The schema intentionally preserves some future-facing shape, especially
multi-book columns and extension guardrails, but that should not be confused
with a second supported runtime. V1 release confidence gates on the sqlite
schema and the one-container deployment only.
