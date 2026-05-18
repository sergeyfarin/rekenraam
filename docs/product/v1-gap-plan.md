# V1 Gap Plan

V1 is SQLite-only. The default deployment is one app container on port `16888`
with the database at `/data/rekenraam.sqlite3`.

## Test Status Snapshot

- CI and local validation now gate on the SQLite runtime by default.
- Narrow confidence checks remain important: schema-contract tests,
  migration-smoke coverage, backup/restore smoke, and Playwright against the
  one-container stack.
- Explicit compatibility work should be treated as follow-up work rather than
  hidden inside the v1 release gate.

## Phase Status

- [x] Phase 0: end-to-end test seam.
- [ ] Phase 1: release-blocker scope items. Full Tauri deletion remains open.
- [ ] Phase 2: hardening of high-risk service code.
- [x] Phase 3: frontend tests and route carving. Detailed shipped history lives
  in [phase-3-plan.md](phase-3-plan.md).
- [ ] Phase 4: nice-to-have scope items.

## Key Findings Preserved From The Pre-Cleanup Plan

### Accounting Correctness

- Keep schema-contract coverage strong; it is the best guard against ORM and
  migration drift now that only one database dialect is supported.
- Future migrations should keep using explicit SQL defaults for numeric and
  boolean defaults.
- Reconciliation and locked-range behavior remain high-risk slices and should
  stay under both API and browser regression coverage.

### Tauri Parity

- Final `src-tauri/` deletion is not just a build cleanup. Product sign-off
  still needs to accept the desktop-only behaviors that were consciously
  dropped, deferred, or replaced.
- The parity audit remains the reference for Rust behaviors that are not
  one-for-one Python ports.

### Frontend Confidence

- The detailed Phase 3 workstream history matters because it records extracted
  pure modules, carved components, and the specific behaviors now pinned by
  Vitest and Playwright.
- Future parity work should build on those surfaces instead of collapsing them
  back into route-local logic.

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
- Product sign-off for final Tauri deletion still needs explicit acceptance of
  dropped or deferred desktop behaviors captured in the parity docs.

## Discovered Cleanup Notes

- Stale tests assumed optional `book_id` defaults and old repository stubs. They
  should be kept aligned with the SQLite schema instead of skipped.
- Keep schema-contract coverage; it is the best guard against ORM/migration
  drift now that only one database dialect is supported.
- Future migrations should use explicit SQL defaults via `text(...)` for
  numeric and boolean defaults.

## Historical Detail Still Worth Keeping

- Detailed shipped history for frontend testing and route carving:
  [phase-3-plan.md](phase-3-plan.md).
- Detailed desktop-to-web capability mapping and changed behavior list:
  [docs/parity/desktop-to-python.md](../parity/desktop-to-python.md).
- Product requirements and release gate:
  [v1-scope.md](v1-scope.md).
