# V1 Gap Plan

V1 is SQLite-only. The default deployment is one app container on port `16888`
with the database at `/data/rekenraam.sqlite3`.

## Done In This Cleanup

- Removed alternate database runtime configuration and dependency surface.
- Simplified SQL helpers and reconciliation locking to SQLite paths.
- Kept SQLite WAL, foreign keys, busy timeout, and normal synchronous mode.
- Converted repository, migration, API, and E2E fixtures to SQLite-only.
- Made `compose.yaml` the canonical one-container stack.
- Replaced database backup/restore scripts with SQLite online backup tooling.
- Updated CI and docs around the one-container deployment.

## Remaining Product Gaps

- Create-books workflow: schema is multi-book, but V1 runtime access remains
  intentionally guarded to the seeded book.
- More admin warnings for public-host misconfiguration would be useful.
- Reconciliation and imports should keep getting load tests as real-world data
  grows.
- Rate-limit state is currently process memory; durable throttling can be added
  if multi-process deployment becomes a V1 requirement.

## Discovered Cleanup Notes

- Stale tests assumed optional `book_id` defaults and old repository stubs. They
  should be kept aligned with the SQLite schema instead of skipped.
- Keep schema-contract coverage; it is the best guard against ORM/migration
  drift now that only one database dialect is supported.
- Future migrations should use explicit SQL defaults via `text(...)` for
  numeric and boolean defaults.
