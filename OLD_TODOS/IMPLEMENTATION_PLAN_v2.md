# Implementation Plan — Backend Improvements

> This plan is ordered by priority: data-integrity fixes first, then developer-experience and extensibility, then future-readiness.

---

## Phase 1 — Critical Data Integrity & Safety (1–2 weeks)

### 1.1  Fix `get_account_balance_minor` (IMPROVEMENTS §7)

**What:** The function sums ALL splits without filtering voided/superseded transactions.

**Files:**
- `src-tauri/src/db_accounts.rs` — rewrite `get_account_balance_minor()` to join through `transactions` and apply void + supersede + session_reverts filters.

**Steps:**
1. Replace the current summation query with a filtered variant matching the pattern used in `list_account_register_with_balance`.
2. Add unit tests: create a transaction, void it, verify balance is zero.
3. Add unit test: create a transaction, update it (creating a revision), verify only the latest revision is counted.

**Verify:** `cargo test` in `src-tauri/`. Add `test_balance_excludes_voided_transactions` and `test_balance_excludes_superseded_transactions` to `db_accounts.rs`.

---

### 1.2  Input Validation Layer (IMPROVEMENTS §5)

**What:** Add domain-type validators to prevent invalid data from being stored.

**Files:**
- **[NEW]** `src-tauri/src/validation.rs` — validators for account types, transaction statuses, name lengths, amount bounds, commodity scales.
- `src-tauri/src/db_accounts.rs` — call validators before inserts.
- `src-tauri/src/db_transactions.rs` — call validators before inserts.
- `src-tauri/src/db_commodities.rs` — call validators before inserts.

**Steps:**
1. Define allowed values as `const` arrays (e.g. `ACCOUNT_TYPES`, `TX_STATUSES`).
2. Create `validate_account_type()`, `validate_status()`, `validate_name()`, `validate_amount()` functions.
3. Wire validators into `create_account`, `update_account`, `create_transaction_with_splits`, `update_transaction_with_splits`.
4. Write unit tests for each validator — both pass and fail cases.

**Verify:** `cargo test` — new tests in `validation.rs`.

---

### 1.3  LIKE Pattern Escaping (IMPROVEMENTS §6)

**What:** Escape `%` and `_` in user search strings.

**Files:**
- `src-tauri/src/db_transactions.rs` — add `escape_like()` helper, apply to search strings in `list_transactions`.
- `src-tauri/src/db_accounts.rs` — same for any LIKE searches.

**Steps:**
1. Add `fn escape_like(input: &str) -> String` to `validation.rs`.
2. Apply wherever `format!("%{}%", search)` is used.
3. Add `ESCAPE '\'` clause to the SQL.

**Verify:** Unit test: a search for `%` returns zero results on a clean DB.

---

## Phase 2 — Code Quality & Developer Experience (2–3 weeks)

### 2.1  Structured Error Types (IMPROVEMENTS §3)

**What:** Replace `Result<T, String>` with `Result<T, AppError>`.

**Files:**
- **[NEW]** `src-tauri/src/error.rs` — define `AppError` enum, implement `Serialize`, implement `Into<tauri::InvokeError>`.
- All command modules — migrate from `String` to `AppError`.

**Steps:**
1. Define `AppError` with variants: `Validation`, `NotFound`, `Conflict`, `Database`, `Internal`.
2. Implement `From<rusqlite::Error>` for `AppError`.
3. Implement `From<String>` for `AppError` as a migration shim.
4. Migrate one module at a time, starting with `db_transactions.rs` (smallest, best tested).
5. Update frontend IPC handlers to parse the structured error.

**Verify:** `cargo test` passes. Manually verify that a validation error (e.g. unbalanced splits) returns a structured JSON error to the frontend.

---

### 2.2  Extract Shared Session Helpers (IMPROVEMENTS §4)

**What:** Deduplicate `current_session_id`, `clear_redo_stack`, `record_insert_change`.

**Files:**
- **[NEW]** `src-tauri/src/session.rs` — shared module.
- `src-tauri/src/db_accounts.rs` — remove local definitions, import from `session.rs`.
- `src-tauri/src/db_transactions.rs` — same.
- `src-tauri/src/import.rs` — same.

**Steps:**
1. Move the three functions to `session.rs`.
2. Add `pub mod session;` to `lib.rs`.
3. Replace local definitions with `use crate::session::*;`.
4. Run existing tests to confirm no regression.

**Verify:** `cargo test` — all existing tests pass unchanged.

---

### 2.3  Add Logging (IMPROVEMENTS §9)

**What:** Add `tracing` crate for structured logging.

**Files:**
- `src-tauri/Cargo.toml` — add `tracing`, `tracing-subscriber`.
- `src-tauri/src/lib.rs` — initialize subscriber in `run()`.
- All command modules — add `info!`/`warn!`/`error!` at key points.

**Steps:**
1. Add dependencies.
2. Initialize a `fmt::Subscriber` with file rotation in the app data directory.
3. Add `info!` spans at command entry/exit.
4. Add `error!` at every error return path.
5. Add `info!` for FX refresh results, backup operations, migration steps.

**Verify:** `cargo build` succeeds. Manually verify log output by running the app and checking the log file.

---

### 2.4  Pagination Metadata (IMPROVEMENTS §10)

**What:** Return `{ items, total_count, has_more }` from list endpoints.

**Files:**
- **[NEW]** `src-tauri/src/pagination.rs` — `PaginatedResult<T>` struct.
- `src-tauri/src/db_transactions.rs` — update `list_transactions` return type.
- `src-tauri/src/db_accounts.rs` — update list commands.
- Frontend stores that call these commands — update to expect the wrapper.

**Steps:**
1. Define `PaginatedResult<T: Serialize>`.
2. Add a parallel `COUNT(*)` query in each list command.
3. Update return types.
4. Update frontend.

**Verify:** `cargo test` — add a test that creates N transactions and verifies `total_count`.

---

## Phase 3 — Performance & Concurrency (1–2 weeks)

### 3.1  SQLite PRAGMA Tuning (IMPROVEMENTS §17)

**What:** Add performance PRAGMAs to `open_and_migrate()`.

**Files:**
- `src-tauri/src/db.rs` — add PRAGMAs.

**Steps:**
1. Add `mmap_size`, `cache_size`, `busy_timeout`, `temp_store` PRAGMAs after connection open.
2. Benchmark before/after with a 50K-row test DB.

**Verify:** Existing test `test_report_cache_invalidation_by_book_state` still passes. Manually measure query time improvement.

---

### 3.2  Read/Write Connection Split (IMPROVEMENTS §2)

**What:** Use a separate read-only connection for queries.

**Files:**
- `src-tauri/src/state.rs` — add `read_conn: Option<Connection>` field.
- `src-tauri/src/db.rs` — open a second connection with `PRAGMA query_only = ON`.
- All read-only commands — use `read_conn` instead of `conn`.

**Steps:**
1. Open a second connection in `open_and_migrate`.
2. Add helper `read_conn()` and `write_conn()` to `DbState`.
3. Migrate `list_*` and `get_*` commands to use `read_conn()`.
4. Write commands continue using `write_conn()`.

**Verify:** `cargo test` — all tests pass. Manually verify that a long-running list query does not block a simultaneous write.

---

## Phase 4 — Extensibility & Future Readiness (3–4 weeks)

### 4.1  Book ID Abstraction (IMPROVEMENTS §1)

**What:** Replace hardcoded `SINGLE_BOOK_ID` with validated parameter.

**Files:**
- All 6 command modules — remove `const SINGLE_BOOK_ID` and `let _ = book_id;` patterns.
- **[NEW]** `src-tauri/src/book.rs` — `resolve_book_id()` function.

**Steps:**
1. Add `fn resolve_book_id(conn, book_id) -> Result<i64, AppError>` that validates the book exists.
2. Replace all `SINGLE_BOOK_ID` usages.
3. Test with `book_id = 1` (current behavior) and verify error for `book_id = 999`.

**Verify:** `cargo test` — add `test_resolve_book_id_valid` and `test_resolve_book_id_invalid`.

---

### 4.2  Sync Readiness — UUIDs (IMPROVEMENTS §14)

**What:** Add UUID columns to sync-eligible tables.

**Files:**
- **[NEW]** `src-tauri/migrations/V2__add_uuids.sql` — `ALTER TABLE ... ADD COLUMN uuid TEXT`.
- Schema documentation update in `SCHEMA.md`.

**Steps:**
1. Write migration V2 that adds `uuid TEXT NOT NULL DEFAULT (lower(hex(randomblob(16))))` to: `books`, `accounts`, `commodities`, `transactions`, `splits`, `payees`, `categories`, `tags`.
2. Add `updated_at` and `device_id` columns for conflict resolution.
3. Update Rust structs to include `uuid` in their mappings.
4. Update `SCHEMA.md`.

**Verify:** `cargo test` — migration applies cleanly on a fresh DB and on an existing DB with data.

---

### 4.3  Test Harness Expansion (IMPROVEMENTS §11)

**What:** Prioritized test suite expansion.

**Files:**
- `src-tauri/src/import.rs` — add `#[cfg(test)] mod tests` with sample CSV/QIF/OFX/MT940 data.
- `src-tauri/src/db_accounts.rs` — add account CRUD and fiscal-year-close tests.
- `src-tauri/src/db_commodities.rs` — add commodity buy/sell/lot tests.

**Steps:**
1. **Import parsers:** Create inline test CSV strings and verify parsed output.
2. **Balance adjustment:** Test that voided transactions don't affect balance (links to Phase 1).
3. **Fiscal year close:** Test multi-account close with income + expense zeroing.
4. **Undo/redo:** Test the full undo→redo→undo cycle.

**Verify:** `cargo test` — new test count should exceed 20 total (up from current 5).

---

## Phase 5 — Future Features (scoped but not yet implemented)

| Feature | Dependency | Notes |
|---|---|---|
| Backup checksum verification | Phase 1 | SHA-256 after copy |
| Append-only compaction (§12) | Phase 4.2 UUIDs | Archive old revisions |
| FX resilience (§15) | Phase 3.2 read/write split | Separate HTTP fetching from DB lock |
| Business schema extensions (§13) | Phase 4.1 book ID | Tax codes, invoices |
| Module layout cleanup (§16) | Any | Remove `#[path = "..."]` hacks |
| Remove test `transmute` (§8) | Phase 2.1 error types | Restructure commands |

---

## Running Existing Tests

```bash
cd src-tauri
cargo test
```

Current tests (5 total):
- `db::tests::test_report_cache_invalidation_by_book_state`
- `db_transactions::tests::test_create_update_register`
- `db_transactions::tests::test_normalize_date_or_utc_timestamp`
- `fx_refresh::tests::test_adjust_for_weekend`
- `fx_refresh::tests::test_prepare_refresh_tasks_uses_active_currencies_and_last_success`
