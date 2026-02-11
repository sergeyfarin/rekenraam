## Backend Test Plan (Rust)

This plan is based on a review of the backend Rust modules in src-tauri/src. It organizes test coverage by module and prioritizes risk‑critical behavior first.

---

## 1) Test Strategy Overview

### Test Types
- **Unit tests**: Pure logic in db_* modules (calculations, query helpers, validation helpers).
- **DB integration tests**: SQLite‑backed tests that run migrations and verify CRUD + invariants.
- **Command/API tests**: Invoke the same functions used by Tauri commands to ensure error paths + side effects.
- **Filesystem tests**: Storage/backup/move/copy/restore behavior with temp folders.

### Test Harness & Fixtures
- **Test DB factory**: Helper to create a temporary directory, run migrations, seed defaults, and return `(DbState, PathBuf)`.
- **Seed helpers**: Create book, commodities, accounts, categories, payees, tags, and sample transactions.
- **Deterministic timestamps**: Use fixed timestamps for reports + time-sensitive behaviors.
- **Cleanup helpers**: Delete temp directories and files after each test.

### Priority Levels
- **P0**: Data safety, migrations, balances/locks, storage/backup/restore.
- **P1**: Domain correctness (transactions, splits, postings, lots, gains, constraints).
- **P2**: Importing, pricing, reports, directives/notes/events/docs.

---

## 2) Module‑by‑Module Coverage

### db.rs (storage/migrations core)
**P0**
- Migration runner applies all versions in order; re-run is idempotent.
- `open_and_migrate` sets WAL and foreign_keys pragmas.
- `normalize_db_path` maps directory → file path.
- `db_accessible` handles nonexistent path and directory creation.
- `resolve_accessible_db` picks fallback and returns accessible path.

### db_storage.rs (storage switching + backups)
**P0**
- `validate_and_set_storage_location` migrates and updates DbState.
- `create_new_storage` creates new DB (fails if exists).
- `move_storage` moves db file; error when destination exists; reopen works.
- `copy_storage` copies db file; source retained; reopen uses destination.
- `restore_from_backup` overwrites current db safely.
- `create_backup` creates file; respects retention count.
- `list_backups` returns sorted list; empty dir returns empty array.
- `db_stats` returns size/writable/journal/foreign_keys.
- `db_integrity_check` returns “ok” for clean DB; non‑ok when intentionally corrupted (optional).
- `db_vacuum` succeeds and updates file size (optional).

**P1**
- Scheduler start/stop respects settings (enabled, interval).
- Backup on close is executed and generates a file in expected folder.

### db_accounts.rs (accounts, categories, tags, directives, constraints)
**P0**
- CRUD for accounts, categories, payees, tags.
- Account tree computes rollups correctly.
- Account balancing: create/list/unlock/void; locks prevent changes pre‑date.

**P1**
- Booking policy updates persisted and validated.
- Balance constraints: create/list/delete; validation rejects violations.
- Directives: open/close, balance check, pad directives create and list.

**P2**
- Notes/events/documents CRUD (account/tx attachment, optional fields).
- Dividend income categories CRUD (book/category/commodity uniqueness).

### db_transactions.rs (transactions, splits, register, imports)
**P0**
- Create/update/delete transaction with splits; splits integrity preserved.
- Register with balance: running balance accurate (including ordering).
- Posting list correct for account/date filters.

**P1**
- Currency trading balances enforcement.
- Import rules: priority, match types, amount/date/account matching.
- Import sessions: start/commit status transitions.

**P2**
- Import parsing (QIF/OFX/HBCI) happy paths + malformed input errors.
- Matching engine: exact match vs contains; duplicate handling.

### db_commodities.rs (commodities, prices, investments)
**P0**
- Commodities CRUD; rename respects uniqueness.
- Price sources and commodity price sources CRUD.
- Commodity prices CRUD; latest price lookup.

**P1**
- Buy/sell/reinvest/dividend flows create correct transactions + splits.
- Lot allocation logic: FIFO/LIFO/strict/average policy enforcement.
- Gains validation for sells; holding period calculation.
- Positions and conversions: totals by account/commodity, implicit prices.

**P2**
- Corporate actions (split/merge) adjust lots/positions correctly.
- Gains reports (realized/unrealized) match expected basis and prices.

### lib.rs (app wiring)
**P1**
- Tauri command registration smoke tests (optional: compile‑time only).
- Window state save/restore behavior (optional, OS‑specific).

### state.rs
**P2**
- Backup scheduler state starts/stops without panic; task aborts cleanly.

---

## 3) Test Data Scenarios

### Minimal book baseline
- Book + base commodity + two accounts (cash + checking) + two categories.

### Balanced transaction
- Split across cash + expense category; validate register + postings.

### Investment scenario
- Buy, dividend, reinvest, sell; validate lots, gains, positions.

### Import scenario
- Create rules, parse dummy import lines, match vs create, verify session.

---

## 4) Test Infrastructure Tasks

1. **Create test helpers module** in src-tauri/src (or tests/):
	- `create_temp_db()` → (DbState, PathBuf)
	- `seed_default_book()`
	- `insert_account`, `insert_category`, `insert_payee`
	- `insert_transaction_with_splits`

2. **Centralize cleanup** to remove temp dirs even on failure.

3. **Fixture data builders** for investments/imports.

4. **Optional**: add feature flag to skip heavy tests in CI.

---

## 5) Phased Implementation Order

### Phase A (P0 – Safety & Core)
- db.rs migrations + path handling tests
- db_storage.rs move/copy/backup/restore tests
- db_accounts.rs account balancing + CRUD tests
- db_transactions.rs create/update/register tests

### Phase B (P1 – Domain Correctness)
- Booking policy, balance constraints, directives
- Prices + positions + gains validations
- Import rules + sessions

### Phase C (P2 – Extended Features)
- ✅ Corporate actions
- ✅ Reports (realized/unrealized gains)
- ✅ Parsing pipelines (QIF/OFX/HBCI)

---

## TODOs (Missing Coverage)

### Reports + Cache
- [ ] Add tests that exercise `report_cache` population and invalidation (`book_state.change_seq`).
- [ ] Add tests for account tree/report cache interactions (future: only if cached account tree reports are introduced beyond gains).

### db_accounts.rs (P2)
- ✅ Notes/events/documents CRUD tests (create/update/delete + account/tx attachment paths).
- ✅ Dividend income categories CRUD tests (uniqueness by book/category/commodity).

### db_storage.rs (P1/P2)
- ✅ Scheduler timing test using a short interval and verifying multiple backups.
- ✅ Integrity check negative case (intentionally corrupt DB) if feasible on CI.

### db_transactions.rs (P2)
- ✅ Import matching negative paths (invalid date/amount, missing required fields) for `import_transactions`.
- ✅ Duplicate detection paths (existing `import_id` or same date+amount+memo) in `match_import_transactions`.

### db_commodities.rs (P2)
- ✅ Price sources CRUD tests (create/update/delete + primary source flag enforcement).
- ✅ Commodity price sources CRUD tests (symbol override + is_primary behavior).

---

## 6) Acceptance Criteria Mapping

- **No data loss on switching**: covered by move/copy + register readback tests.
- **Backups reliable**: backup creation + retention + restore tests.
- **Restore safe**: restore test validates pre‑backup state.
- **Clear errors**: negative‑path tests for invalid inputs across storage and CRUD commands.

---

## 7) Notes

- Favor integration tests that run against a temp SQLite file created via `open_and_migrate`.
- Keep tests deterministic; use explicit timestamps for sorting in reports and register.
- Add regression tests when bugs are fixed.
