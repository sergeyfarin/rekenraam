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
- Run `"$(go env GOROOT)/bin/gofmt" -w` and `go vet ./...` on touched files.
  Use the GOROOT path, not a bare `gofmt`: a version manager can leave an older
  gofmt first on PATH while `go` switches to the newer toolchain go.mod asks
  for, and the two disagree about formatting.

## Error envelope

Every API error is `{"error":{"code":"STABLE_CODE","message":"..."}}` via
`writeAPIError(w, status, code, message)` (`internal/api/middleware.go`).
Existing codes: `VALIDATION_FAILED`, `UNAUTHENTICATED`, `FORBIDDEN`,
`NOT_FOUND`, `CONFLICT`, `CSRF_INVALID`, `RATE_LIMITED`, `RESOURCE_BUSY`,
`LEDGER_OVERFLOW`, `SETUP_REQUIRED`, `SETUP_ALREADY_COMPLETE`,
`CONFIG_REQUIRED`, `PROVIDER_ERROR`, `INTERNAL_ERROR`. Adding a code means
updating the OpenAPI `ErrorBody.code` enum too. Never return raw Go error
strings to the client.

## SQLite rules (source: ADR 0004, conventions § Data)

- Driver is `modernc.org/sqlite` (pure Go). Never `mattn/go-sqlite3`.
- `db.Open` (`internal/db/sqlite.go`) installs PRAGMAs per connection, runs
  goose migrations, and validates PRAGMA state. Use it everywhere, tests included.
- **The pool is `SetMaxOpenConns(1)`.** One `BeginTx` is a full mutual-exclusion
  lock — this is why single-transaction guard+insert patterns work (see
  `background-work` skill). Don't "fix" this without an ADR.
- Treat `SQLITE_BUSY` as possible even with `busy_timeout`; exhausted timeout →
  retryable server error (`RESOURCE_BUSY`), never a partial apply.
- Mutations that must be atomic go in **one** DB transaction, including their
  `audit_events` row and any enqueued background work.

## Migrations

- New schema = new sequential file `backend/migrations/00NN_short_name.sql`
  (goose format: `-- +goose Up` / `-- +goose Down`). Embedded via
  `migrations/embed.go`; they run automatically at startup and in every test
  through `db.Open`. Never edit an already-committed migration.
- IDs are `INTEGER PRIMARY KEY` auto-increment. No UUID keys without an ADR.

## Test patterns

- Every test gets a fresh migrated temp DB:
  ```go
  database, err := db.Open(ctx, "file:"+filepath.Join(t.TempDir(), "test.sqlite"))
  require.NoError(t, err)
  ```
  Never touch `backend/var/dev.sqlite` from tests.
- Use `testify`: `require` for fatal, `assert` for non-fatal. No manual
  `if got != want` blocks.
- Good reference tests: `app/import_fetch_worker_test.go` (worker + concurrency),
  `app/investments_test.go` (financial cases), `api/transactions_test.go`
  (HTTP-level with auth/CSRF helpers).
- Financial invariants, reconciliation behavior, imports, and calculations
  require **named** test cases — a regression must fail loudly by name.
- Bug fixes: write the failing test first, confirm it fails, then fix
  (the project history records this in commit messages; keep doing it).

## Validation

```sh
./scripts/test-backend.sh            # go test ./... — the default gate
cd backend && go vet ./... && "$(go env GOROOT)/bin/gofmt" -l .
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
  transaction B — a crash between them creates duplicates on retry (open
  backlog item T-06 documents this trap).
