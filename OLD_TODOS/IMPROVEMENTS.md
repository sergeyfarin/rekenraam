# Backend Code & Schema Review — Observations and Improvements

> Reviewed: all 14 Rust source files (~16 000 lines), `V1__init.sql`, `Cargo.toml`, `SCHEMA.md`, and `README.md`.
> Date: 2026-02-17

---

## 1. Architecture — Hardcoded Single-Book Assumption

**Observation:** Every command module defines `const SINGLE_BOOK_ID: i64 = 1;` and silently overwrites the `book_id` parameter sent by the frontend:

```rust
let _ = book_id;
let book_id = SINGLE_BOOK_ID;
```

This pattern appears in `db_accounts.rs`, `db_commodities.rs`, `db_transactions.rs`, `db_storage.rs`, `import.rs`, `fx_refresh.rs` — at least 6 files.

**Impact:**
- Multi-book support is a single-line change in the schema but requires a sweep of every command.
- The silent discard of the user-supplied `book_id` masks bugs and makes API contracts misleading.
- Future sync/multi-tenant scenarios break.

**Recommendation:** Introduce a `resolve_book_id()` helper that validates the supplied book_id exists and is owned by the current user. Replace all hardcoded constants. This also prepares for multi-book and sync.

---

## 2. Concurrency — `std::sync::Mutex<Connection>` Bottleneck

**Observation:** `DbState` wraps the SQLite connection in `std::sync::Mutex`:

```rust
pub struct DbState {
    pub inner: Mutex<DbStateInner>,
}
```

Every Tauri command acquires this lock for the entire duration of its database work — including multi-step transactions, CSV parsing, FX HTTP requests, etc.

**Impact:**
- Any long-running command (CSV import of 10 000 rows, FX refresh with N HTTP calls) blocks the entire UI during the lock.
- Tauri invokes commands from the main thread pool; a poisoned mutex silently kills all subsequent commands.
- Running balance / register queries hold the lock during full-table scans, causing UI freezes with large datasets.

**Recommendation:**
1. **Short-term:** Use `tokio::sync::Mutex` or move to a connection pool (e.g. `r2d2` with `rusqlite`). WAL mode already supports concurrent readers.
2. **Medium-term:** Split read-only commands to use a separate read connection that never holds a write lock. Only write commands take the write mutex.
3. **Long-term:** Consider `async fn` commands with `spawn_blocking` for all SQLite work, avoiding Tauri IPC thread starvation.

---

## 3. Error Handling — No Structured Error Types

**Observation:** Every function returns `Result<T, String>` and uses `.map_err(|e| e.to_string())` extensively. Error messages are human-readable text with no machine-parseable codes.

```rust
pub fn create_account(db: State<DbState>, input: AccountCreate) -> Result<Account, String> {
    let guard = db.inner.lock().map_err(|_| "db state lock poisoned".to_string())?;
    ...
}
```

**Impact:**
- Frontend cannot distinguish between a validation error ("splits must balance") vs a system error ("db state lock poisoned").
- No error context chain — when a command fails, the user gets a SQLite error message with no indication of which operation failed.
- Impossible to implement retry logic, error reporting, or telemetry.

**Recommendation:**
```rust
#[derive(Debug, Serialize)]
pub enum AppError {
    Validation { field: String, message: String },
    NotFound { entity: String, id: i64 },
    Conflict { message: String },
    Database { message: String },
    Internal { message: String },
}
```
Map this to appropriate HTTP-style categories so the frontend can show contextual error dialogs.

---

## 4. Code Duplication — Repeated Helper Functions

**Observation:** The following functions are identically duplicated across `db_accounts.rs`, `db_transactions.rs`, and `import.rs`:

| Function | Files |
|---|---|
| `current_session_id()` | `db_accounts.rs`, `db_transactions.rs`, `import.rs` |
| `clear_redo_stack()` | `db_accounts.rs`, `db_transactions.rs` |
| `record_insert_change()` | `db_accounts.rs`, `db_transactions.rs` |

**Impact:** Maintenance burden. Any fix to these functions must be applied in 2-3 places. The functions are small now, but as undo/redo logic evolves, divergence is likely.

**Recommendation:** Move these to a shared `session.rs` module and import them into each command module.

---

## 5. Input Validation — Insufficient Guard-Rails

**Observation:** Most commands skip validation beyond date format checks:

- **No account-type validation:** `create_account` accepts any string for `account_type` — typos like `"epxense"` are silently accepted.
- **No status enum:** Transaction `status` is an unbounded string; nothing prevents `"foo"` from being written.
- **No name length limits:** Account names, payee names, category names, memos — all unbounded. A 10 MB memo would be stored.
- **No amount bounds:** `amount_minor` is `i64` with no guard against values like `i64::MAX` causing overflow in balance calculations.
- **No commodity validation on splits:** The split's `commodity_id` is stored but only the trigger catches commodity-account mismatches — the error message is a raw SQLite trigger error.

**Recommendation:** Add an `input_validation.rs` module with validators for each bounded domain type:
```rust
fn validate_account_type(t: &str) -> Result<(), AppError>;
fn validate_status(s: &str) -> Result<(), AppError>;
fn validate_name_length(name: &str, max: usize) -> Result<(), AppError>;
```

---

## 6. SQL Construction — Dynamic SQL Vulnerabilities

**Observation:** `list_transactions` and similar commands build SQL via string concatenation:

```rust
sql.push_str(" AND (t.memo LIKE ? OR t.reference LIKE ? OR p.name LIKE ?)");
```

While the parameters are bound, the pattern `format!("%{}%", search)` passes user input into a `LIKE` pattern without escaping `%` and `_` wildcards. A user searching for `%` would match all records.

**Impact:** Not an SQL injection risk (params are bound), but leads to incorrect query results.

**Recommendation:** Escape LIKE metacharacters in user-supplied search strings:
```rust
fn escape_like(input: &str) -> String {
    input.replace('\\', r"\\").replace('%', r"\%").replace('_', r"\_")
}
```

---

## 7. Missing `get_account_balance_minor` Filtering

**Observation:** The balance calculation function sums ALL splits without filtering for void/superseded transactions:

```rust
fn get_account_balance_minor(conn: &Connection, account_id: i64) -> Result<i64, String> {
    conn.query_row(
        "SELECT COALESCE(SUM(amount_minor), 0) FROM splits WHERE account_id = ?1",
        [account_id], |row| row.get(0),
    ).map_err(|e| e.to_string())
}
```

There is no filter for `t.status != 'void'`, no `NOT EXISTS` for superseded transactions, and no session_reverts exclusion.

**Impact:** Balance calculations used for balance adjustments and balance checks will include voided and superseded transactions, producing incorrect balances. This is a **data integrity issue**.

**Recommendation:** Replace with a query that joins through `transactions` and applies the standard void/supersede/revert filters.

---

## 8. Unsafe `transmute` in Tests

**Observation:** The test helper `as_state` uses `std::mem::transmute` to convert `&DbState` into `State<'_, DbState>`:

```rust
fn as_state<'a>(db: &'a DbState) -> State<'a, DbState> {
    unsafe { std::mem::transmute::<&'a DbState, State<'a, DbState>>(db) }
}
```

**Impact:** This relies on Tauri's internal layout of `State<T>` being identical to `&T`, which is an implementation detail that can break on any Tauri upgrade.

**Recommendation:** Use `tauri::test` utilities or restructure commands to accept `&DbState` directly (extract the business logic from the Tauri command wrappers).

---

## 9. Observability — No Logging

**Observation:** There is zero logging anywhere in the backend. No `log`, `tracing`, or `env_logger` dependency.

**Impact:**
- Debugging production issues requires code changes and recompilation.
- FX rate refresh failures, migration errors, and lock contention are invisible.
- No audit trail for destructive operations (backup, restore, vacuum).

**Recommendation:** Add `tracing` crate with structured logging. Key points: migration runner, command entry/exit, FX refresh results, backup lifecycle, error paths.

---

## 10. Missing Pagination Metadata

**Observation:** All list commands return `Vec<T>` with no total count, page size, or has-more indicator:

```rust
pub fn list_transactions(db: State<DbState>, filter: ListTransactionsFilter)
    -> Result<Vec<TransactionWithSplits>, String>
```

**Impact:** The frontend cannot show proper pagination controls without issuing a separate count query. For large datasets (10K+ transactions), this forces the UI into infinite-scroll guessing.

**Recommendation:** Return a wrapper struct:
```rust
pub struct PaginatedResult<T> {
    pub items: Vec<T>,
    pub total_count: i64,
    pub has_more: bool,
}
```

---

## 11. Testing Gaps

**Observation:**
- Only 5 tests exist across the entire backend (`db.rs`: 1, `db_transactions.rs`: 2, `fx_refresh.rs`: 2).
- No tests for: account CRUD, commodity operations, import parsing, report generation, balance adjustments, fiscal year close, undo/redo.
- The CSV/QIF/OFX/MT940 parsers in `import.rs` (2266 lines) have **zero tests**.

**Impact:** Regressions are caught only by manual testing. The import parser is especially risky — it handles 4 formats with locale-sensitive number and date parsing.

**Recommendation:** Priority test targets (by risk):
1. Import parsers (CSV, QIF, OFX, MT940) — unit tests with sample files
2. Transaction create/update/delete cycle with balance verification
3. Fiscal year close with multi-account scenarios
4. Undo/redo stack operations
5. Balance adjustment accuracy (the bug in §7)

---

## 12. Append-Only Table Growth

**Observation:** The append-only model keeps every revision forever. There is no retention policy, archival, or compaction mechanism.

**Impact:**
- After a few years of active use with frequent edits, the `transactions` and `splits` tables will grow significantly (every edit doubles the row count for that transaction).
- SQLite performance degrades with table sizes in the millions; the current queries use `NOT EXISTS (SELECT 1 FROM ... WHERE newer.previous_*_id = ...)` anti-join patterns that become expensive.
- Database file size grows unbounded.

**Recommendation:**
1. Add a `compact` command that materializes current-state snapshots and moves old revisions to an archive table.
2. Add indexes for the `previous_*_id` columns if not already present (check migration SQL).
3. Consider periodic `VACUUM` as part of backups.

---

## 13. Schema Readiness — Self-Employed / Business

**Observation:** The schema has `payees` with a `kind` field, `people` with roles, and `projects` — which is a good start. However, for self-employed use:

- **Missing:** Invoices, receivables/payables aging, tax categories with jurisdiction metadata, VAT/GST tracking, customer vs. vendor distinction at the payee level.
- **`categories.kind`** only allows `income`/`expense` — business creates need for `revenue`, `cost_of_goods`, and multi-tier tax categories.
- **No multi-currency profit/loss:** The fiscal year close enforces single-commodity; self-employed users with foreign clients will hit this wall.

**Recommendation:** Define extension points now:
- Add `tax_code` and `tax_rate_bps` columns to `categories` (nullable, additive migration).
- Add an `invoice` table with links to `payees`, `transactions`, and `documents`.
- Allow fiscal year close to produce FX-adjusted P&L entries.

---

## 14. Schema Readiness — Sync

**Observation:** The current schema uses SQLite `INTEGER PRIMARY KEY` (rowid aliases) as foreign keys. These are auto-incrementing, storage-local integers.

**Impact:** Any sync mechanism will produce key collisions between two separate databases.

**Recommendation:**
1. Add a `uuid TEXT NOT NULL DEFAULT (lower(hex(randomblob(16))))` column to every sync-eligible table.
2. Use UUIDs as the sync-layer identity; keep rowids for local performance.
3. Add `updated_at`, `synced_at`, and `device_id` columns for conflict resolution.
4. The append-only model already provides natural CRDT properties — leverage revision chains for merge operations.

---

## 15. FX Rate Providers — Resilience

**Observation:** `fx_refresh.rs` calls HTTP APIs synchronously within a Mutex-locked context:

```rust
fn refresh_pair(db: &DbState, ...) -> Result<(usize, usize), String> {
    let guard = db.inner.lock()...;
    // HTTP call while lock is held?
}
```

**Impact:**
- A slow or offline provider stalls the entire app.
- No retry logic, backoff, or timeout configuration.
- Provider errors are silently swallowed in the scheduler loop.

**Recommendation:**
1. Fetch rates with the lock released; only acquire it for the final DB write.
2. Add configurable timeouts per provider.
3. Add retry with exponential backoff (1s, 2s, 4s, max 30s).
4. Emit structured log events for each provider call.

---

## 16. `commands.rs` Module Layout — Path Hack

**Observation:** `commands.rs` re-exports modules using `#[path = "..."]`:

```rust
#[path = "db_accounts.rs"]
pub mod db_accounts;
```

This is an unusual Rust idiom that bypasses the standard module system. The same files are also imported directly via `mod db_currencies` in `lib.rs`.

**Impact:** Cargo/rust-analyzer may produce confusing diagnostics. New developers will struggle with the non-standard layout.

**Recommendation:** Use standard module organization: either a `commands/` directory with `mod.rs`, or keep all modules at the same level and import them conventionally.

---

## 17. Missing PRAGMA Optimizations

**Observation:** The DB is opened with `journal_mode = WAL` and `synchronous = NORMAL`, which is good. But several useful PRAGMAs are missing:

- `PRAGMA mmap_size = 268435456` — memory-mapped I/O for faster reads
- `PRAGMA cache_size = -64000` — 64MB page cache (default is 2MB)
- `PRAGMA busy_timeout = 5000` — avoid immediate `SQLITE_BUSY` errors
- `PRAGMA temp_store = MEMORY` — keep temp tables in memory

**Recommendation:** Add these to `open_and_migrate()` for better performance with growing datasets.

---

## 18. Backup Integrity — No Checksum Verification

**Observation:** Backups are created via file copy (`fs::copy`), but there is no checksum verification after copy:

```rust
fn create_backup_internal(...) -> Result<String, String> {
    ...
    fs::copy(&src_db, &dest)?;
    ...
}
```

**Impact:** A corrupted backup (disk error, partial write) is silently accepted. The user thinks they have a good backup.

**Recommendation:** After copy, compute SHA-256 of both source and destination and compare. Store the hash in `backup_settings` or alongside the backup file.

---

## Summary Priority Matrix

| # | Area | Severity | Effort | Value |
|---|---|---|---|---|
| 7 | Balance calculation bug | 🔴 Critical | Small | Data integrity |
| 3 | Structured error types | 🟠 High | Medium | User experience |
| 5 | Input validation | 🟠 High | Medium | Data integrity |
| 2 | Mutex bottleneck | 🟠 High | Medium | Performance |
| 11 | Testing gaps | 🟠 High | Large | Reliability |
| 4 | Code deduplication | 🟡 Medium | Small | Maintainability |
| 9 | Logging | 🟡 Medium | Small | Operability |
| 10 | Pagination metadata | 🟡 Medium | Small | UX |
| 1 | Book ID abstraction | 🟡 Medium | Medium | Extensibility |
| 17 | SQLite PRAGMAs | 🟡 Medium | Small | Performance |
| 14 | Sync readiness (UUIDs) | 🟡 Medium | Medium | Future sync |
| 18 | Backup checksums | 🟡 Medium | Small | Reliability |
| 12 | Append-only compaction | 🟡 Medium | Large | Performance |
| 6 | LIKE escaping | 🟢 Low | Tiny | Correctness |
| 8 | Transmute in tests | 🟢 Low | Tiny | Safety |
| 16 | Module layout | 🟢 Low | Small | DX |
| 15 | FX resilience | 🟢 Low | Medium | Reliability |
| 13 | Business schema | 🔵 Future | Large | Extensibility |
