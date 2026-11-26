---
name: backend-slice
description: How to add or change a Go backend feature in Rekenraam - layering, migrations, SQLite rules, error envelope, service wiring, and test patterns. Use when creating or modifying endpoints, services, repositories, or schema under backend/.
---

# Backend Feature Slice

Read `docs/conventions.md` for the touched area first. If the change affects
money/ledger/reconciliation, apply the `ledger-invariants` skill. If it adds or
changes an endpoint, apply the `api-contract` skill in the same change.

## Layering (strict)

```
backend/internal/api/   HTTP handlers, DTOs, middleware — thin, no business rules
backend/internal/app/   application services — ALL business rules live here
backend/internal/db/    repository methods over database/sql — ALL SQL lives here
backend/internal/exact/ exact decimal arithmetic (do not duplicate)
backend/migrations/     numbered goose SQL migrations
backend/cmd/rekenraam/  binary entrypoint + service wiring (command.go)
```

- Handlers decode/validate the request shape, call one service method, map the
  result/error to the envelope. Nothing else.
- Services own invariants and orchestrate repositories; repositories never
  contain business rules.
- New service → add a field to `api.Services` (`internal/api/server.go`),
  register routes in `RegisterRoutesWithAuth`, construct and wire it in
  `cmd/rekenraam/command.go`.

## Go conventions

- `context.Context` first parameter on anything doing I/O.
- Wrap errors: `fmt.Errorf("doing x: %w", err)`. Never swallow; never panic in
  handlers/services.
- Sentinel errors in the service (`var ErrThing = errors.New(...)`), mapped to
  API codes in the handler layer (pattern: `api/transactions_errors.go`).
- Structured logging via `log/slog` only; **never log financial content**
  (amounts, payees, account names) at any level.
- Run `"$(go env GOROOT)/bin/gofmt" -w` and `./scripts/go-backend.sh vet ./...` on touched files.
  Use the GOROOT path, not a bare `gofmt`: a version manager can leave an older
  gofmt first on PATH while `go` switches to the newer toolchain go.mod asks
  for, and the two disagree about formatting.

## Error envelope

Every API error is `{"error":{"code":"STABLE_CODE","message":"..."}}` via
`writeAPIError(w, status, code, message)` (`internal/api/middleware.go`).
The complete code list is the `ErrorBody.code` enum in
`api/openapi/components/schemas/errors.yaml`; read it before inventing a code
and reuse a generic one (`VALIDATION_FAILED`, `NOT_FOUND`, `CONFLICT`,
`RESOURCE_BUSY`, `LEDGER_OVERFLOW`) when it fits. A new code is added to that
enum in the same change. Never return raw Go error strings to the client.

## Request pipeline and platform rules (source: `docs/conventions.md`)

- Middleware order is fixed: request ID → panic recovery → request logging →
  auth → CSRF for mutating authenticated requests → handler. Each layer is a
  plain `http.Handler` wrapper; the request ID is logged and echoed as
  `X-Request-ID`.
- Sessions are opaque `HttpOnly`, `SameSite=Strict` cookie tokens stored
  hashed; mutating requests need the session-bound CSRF token. The app never
  terminates TLS: a reverse proxy does.
- Pass the `slog` logger by parameter or context, never a package-level
  global. `APP_ENV` is `development` or `production`, and unset means
  production.
- SQL is handwritten over `database/sql`. There is no `sqlc` and no generated
  query code; do not assume generated files exist.
- Live backups use SQLite's online backup API or `VACUUM INTO`, never a raw
  copy of the live file.

## SQLite rules (source: ADRs 0004/0014/0015, conventions § Data)

- ADR 0014 selects bundled native SQLite through `github.com/mattn/go-sqlite3`
  (CGO), delivered in #195. Use `./scripts/go-backend.sh` for focused Go
  commands; it checks CGO/compiler prerequisites and enables
  `fts5,netgo,osusergo`. Follow `docs/plans/native-sqlite-driver-migration-plan.md`;
  use one driver and consistent features in runtime and tests. Install required
  settings on every physical connection, including replacements.
- `db.Open` (`internal/db/sqlite.go`) installs PRAGMAs per connection and
  validates PRAGMA state. `db.Migrate` applies goose migrations at startup;
  ordinary tests copy the migrated template. Use these paths everywhere.
- **The pool is `SetMaxOpenConns(1)`.** One `BeginTx` is a full mutual-exclusion
  lock — this is why single-transaction guard+insert patterns work (see
  `background-work` skill). Don't "fix" this without an ADR.
- Runtime, migrations, tests and auxiliary connections use FULL synchronous
  mode (ADR 0015); the writer uses WAL/FULL. The central DSN and native driver
  hook install FULL on initial and replacement connections, including direct
  driver opens. Keep the separate read-only pool capped at four connections;
  this is a pool limit, not a node or CPU requirement.
- The statement cache is off (`_stmt_cache_size=0`, #404). A loop that runs
  the same SQL per row inside one transaction goes through a `statementSet`
  (`internal/db/statement_reuse.go`) so it compiles once per call; prove it
  with an authorizer count and a negative control, as
  `TestTransactionVersionCompilesItsPostingInsertOnce` does.
- Treat `SQLITE_BUSY` as possible even with `busy_timeout`; exhausted timeout →
  retryable server error (`RESOURCE_BUSY`), never a partial apply.
- Mutations that must be atomic go in **one** DB transaction, including their
  `audit_events` row and any enqueued background work.
- Calendar-date CHECKs use `CASE WHEN length(col) = 10 THEN date(col, '+0 days') END IS col`
  to enforce the canonical text and preserve SQL `NULL` for optional dates.

## Migrations

- New schema = new sequential file `backend/migrations/00NN_short_name.sql`
  (goose format: `-- +goose Up` / `-- +goose Down`). Embedded via
  `migrations/embed.go`; they run automatically at startup and in every test
  through `db.Open`.
- Migrations on `main` are immutable from the household freeze (#576). Add
  the next sequential file and its SHA-256 in
  `backend/migrations/freeze_test.go` in the same PR. Preserve the baseline
  generation and historical-upgrade fixtures. Read *Project Lifecycle And
  Migration Immutability* in `docs/conventions.md` and extend upgrade coverage
  when changing the schema.
- Migration numbers collide across branches. The branch merged second renumbers
  its own file; renumbering one already on `main` is a rewrite.
- IDs are `INTEGER PRIMARY KEY` auto-increment. No UUID keys without an ADR.

## Test patterns

- Every ordinary integration test gets an isolated copy of the migrated test
  template:
  ```go
  database, databaseURL := testdb.Open(t)
  ```
  `internal/testdb` migrates once per Go test process, copies the closed
  template into `t.TempDir()`, and opens the copy through `db.Open`, so tests
  retain real PRAGMA validation and file isolation without replaying the full
  DDL. Migration, restore, and fresh-start tests keep using `db.Open` plus
  `db.Migrate` directly because migration behavior is their subject.
  Never touch `backend/var/dev.sqlite` from tests.
- Build file URIs from paths with `db.SQLiteFileURL` (`SQLiteFileURL` inside
  the db package), then append query options such as `?mode=ro`. Reuse this
  constructor for fresh migration, recovery and restore fixtures as well as
  auxiliary readers; raw `file:` concatenation breaks on `#`, `?` and `%`.
- Every top-level test in `internal/app` and `internal/db` starts with
  `t.Parallel()` (T-125: serial tests were 654 of the app package's 866
  race-gate seconds). Opt out only with a `// serial: <reason>` comment above
  the function for process-wide state such as `t.Setenv`;
  `TestIntegrationSuitesRunTestsInParallel` fails otherwise.
- Use `testify`: `require` for fatal, `assert` for non-fatal. No manual
  `if got != want` blocks.
- Good reference tests: `app/import_fetch_worker_test.go` (worker + concurrency),
  `app/investments_test.go` (financial cases), `api/transactions_test.go`
  (HTTP-level with auth/CSRF helpers).
- Lot fixtures use posted operations through the shared investment writer or
  a frozen release seed (#148, #576). `investment_lots.operation_id` and
  `source_transaction_id` are required; do not insert unmodeled openings.
- Financial invariants, reconciliation behavior, imports, and calculations
  require **named** test cases — a regression must fail loudly by name.
- Bug fixes: write the failing test first, confirm it fails, then fix
  (the project history records this in commit messages; keep doing it).

## Validation

```sh
./scripts/test-backend.sh            # go test -race -p 2 -timeout=5m ./... — the default gate
./scripts/go-backend.sh vet ./...
cd backend && "$(go env GOROOT)/bin/gofmt" -l .
pnpm build                           # only if the integrated binary shape changed
```

## Common mistakes seen in this repo's history (check for them)

- Repository list methods that **silently clamp `limit`** while a caller
  assumes it got everything (caused a real stuck-import bug; fix was a
  dedicated `ListAll*` method for internal full reads).
- `PATCH` handlers using plain `bool`/`string` for optional fields — an omitted
  field silently overwrites. Use pointer types (`*bool`) end-to-end.
- Guard-check + insert as two statements = TOCTOU race; do both in one
  transaction (repo-layer method), which under `SetMaxOpenConns(1)` is a lock.
- Writing a row in transaction A and its idempotency/identity marker in
  transaction B — a crash between them creates duplicates on retry (T-06 and
  T-26, both closed; the pattern recurs).

The full entries, with their named regression tests, are in
`.claude/skills/validate-and-ship/bug-classes/backend.md`.
