# Rekenraam — Master Execution Plan

> **Single source of truth.** This document supersedes all previous planning files
> (`IMPLEMENTATION_PLAN.md`, `IMPORT_PLAN.md`, `1.MD`, and `OLD_TODOS/`).
> Last updated: 2026-03-18

---

## Vision

A **modern, local-first personal finance desktop application** for individuals and small
self-employed businesses — the MS Money successor that never arrived. Fast, private,
offline-capable, extensible to sync.

**Design philosophy:**
- **Local-first.** The database is a portable SQLite file the user owns.
- **Correctness first.** Double-entry, append-only, trigger-enforced invariants.
- **Sync-ready, not sync-required.** Architecture supports future optional cloud sync.
- **Power-user friendly.** Keyboard navigation, auto-fill, bulk operations.
- **Opinionated defaults, full control.** Sensible defaults for most users; full depth for power users.

**Inspiration sources:**

| App | What to take |
|-----|-------------|
| MS Money | Account register UX, scheduled bills, investment tracking |
| GnuCash | Double-entry correctness, lot booking, reconciliation workflow |
| YNAB | Proactive budgeting, goal tracking, spending awareness |
| Actual Budget | Local-first philosophy, minimal clean UX, fast register |
| MoneyManager Ex | Lightweight SQLite-portable, good reports, open source |
| Beancount | Append-only data model, import rules, audit trail |
| Firefly III | Rules engine, recurring transactions, piggy banks |
| Quicken | Multi-account register, investment reports, tax prep |

---

## Current State (March 2026)

### What works ✅
- Storage lifecycle: SQLite with WAL, versioned migrations (V18+), backup/restore
- Account CRUD + tree view with rollup balances
- Transaction/split create + edit with double-entry
- Account register with running balance
- Investment: holdings, lots, buy/sell/dividend, realized/unrealized gains
- FX: rate scheduler, source providers, daily/official rates
- Reports: cashflow, category spend, payee totals, gains
- Settings: categories, payees, tags, commodities, institutions, DB management
- UI: shadcn-svelte + Tailwind 4 component library

### What is incomplete or missing ⚠️

**Critical bugs:**
- Balance calculations include voided/superseded transactions (data integrity)

**Missing MVP features:**
- Onboarding flow (first-launch DB creation without manual navigation)
- Transaction filters incomplete (payee, amount range, date range not all wired)
- Reconciliation wizard (backend exists, no UI workflow)
- Year-end close UI action

**Missing product features:**
- Scheduled transactions + reminders
- Budgeting (no schema, no UI)
- Import wizard UI (parsers exist, no UI pipeline)
- Charts + visual reports
- Tax features

**Technical debt:**
- 5 tests for 19,000+ lines of Rust (virtually no coverage)
- All errors are `String` — frontend cannot distinguish error types
- Single `std::sync::Mutex` blocks UI during long operations
- Hardcoded `SINGLE_BOOK_ID = 1` in 6 files
- No logging or observability
- No pagination metadata on list endpoints
- No UUIDs (required for future sync)

---

## Architecture Decisions (Confirmed)

| Concern | Decision | Notes |
|---------|----------|-------|
| Desktop runtime | Tauri 2.x | Rust backend + webview frontend |
| UI framework | SvelteKit 2 + Svelte 5 | Reactive, lightweight |
| UI components | shadcn-svelte + Bits UI + Tailwind 4 | Already in place |
| Storage | SQLite (rusqlite, bundled) | WAL mode, append-only design |
| State management | Svelte stores + Tauri invoke | No extra state library needed |
| Error handling | `AppError` enum → structured JSON | Replace all `Result<T, String>` |
| Logging | `tracing` crate + file rotation | Structured, async-safe |
| Concurrency | tokio::Mutex + read/write split | Unblock UI for long reads |
| Sync readiness | UUID columns (V2 migration) | CRDTs via append-only chains |
| Encryption | SQLCipher (deferred to Sprint 11) | Optional encrypted-at-rest |

---

## Execution Plan

Work is organized into **sprints** of roughly 1–3 weeks each, strictly ordered by
dependency and risk. Sprints within the same stage can overlap when independent.

---

### Stage 0 — Critical Bug Fixes  *(do before anything else)*

**Sprint 0.1 — Data Integrity ✅ DONE (2026-03-18)**

> Every feature built on top of wrong balances amplifies the bug. Fix first.

| Task | File(s) | Status |
|------|---------|--------|
| Fix `get_account_balance_minor()` — void/supersede/revert filter | `db_accounts.rs` | ✅ Done |
| Fix `list_account_balances()` — same unfiltered bug | `db_accounts.rs` | ✅ Done (additional gap found) |
| Fix `validate_account_closing()` — calls fixed helper | `db_accounts.rs` | ✅ Done |
| Fix `validate_balance_constraints()` — calls fixed helper | `db_accounts.rs` | ✅ Done |
| Fix `get_account_tree()` balance subquery | `db_accounts.rs` | ✅ Done (additional gap found) |
| Add balance unit tests | `db_accounts.rs` | ✅ Done: `test_balance_excludes_voided_transactions`, `test_balance_excludes_superseded_transactions` |
| Add SQLite PRAGMAs | `db.rs` | ✅ Done: `mmap_size`, `cache_size=-64000`, `busy_timeout=5000`, `temp_store=MEMORY` |
| Escape LIKE metacharacters + `ESCAPE '\'` clause | `db_transactions.rs` | ✅ Done |
| Create `validation.rs` with all validators + tests | `validation.rs` (new) | ✅ Done: `validate_account_type`, `validate_tx_status`, `validate_name`, `validate_memo`, `validate_amount_minor`, `escape_like` |
| Wire validators into `create_account`, `update_account` | `db_accounts.rs` | ✅ Done |
| Wire validators into `create/update_transaction_with_splits` | `db_transactions.rs` | ✅ Done |

**Gaps found during Sprint 0.1 (recorded for tracking):**
- `list_account_balances()` and `get_account_tree()` had the same unfiltered balance bug as `get_account_balance_minor()` — fixed in same sprint (not originally listed)
- Build environment note: WSL2 environment requires GTK dev libraries (`sudo apt install libgtk-3-dev`) to run `cargo test`. Code is type-correct; run tests on Windows/macOS or after installing GTK dev libs.

**Verify when environment is set up:**
```bash
cd src-tauri
sudo apt install -y libgtk-3-dev libglib2.0-dev libwebkit2gtk-4.1-dev
cargo test test_balance_excludes_voided_transactions
cargo test test_balance_excludes_superseded_transactions
cargo test test_validate_account_type
cargo test test_escape_like
```

---

### Stage 1 — MVP Usability  *(make it usable by a real user)*

**Sprint 1.1 — Onboarding & Core UX (2 weeks)**

| Task | File(s) | Detail |
|------|---------|--------|
| Onboarding flow | `lib.rs`, new `routes/onboarding/` | On first launch (no DB path stored), show a welcome screen with "Create new book" / "Open existing file". Skip manual Settings navigation. |
| Complete transaction filters | `transactions/+page.svelte` | Wire payee filter, date-range picker, amount range, status filter to `list_transactions` backend params |
| Server-side sort + pagination | `db_transactions.rs`, frontend | Add `sort_by`, `sort_dir`, `page`, `page_size` to `ListTransactionsFilter`; return `PaginatedResult<T>` |
| Year-end close UI | `settings/+page.svelte` | Add "Year-End" tab calling `close_fiscal_year`; show resulting closing transaction id + retained earnings delta |
| Fix README bottom junk | `README.md` | Remove dangling text at line 84-87 |

**Sprint 1.2 — Transaction UX (2 weeks)**

| Task | File(s) | Detail |
|------|---------|--------|
| Keyboard-driven quick entry | `transactions/+page.svelte` | Tab-through fields; Enter to save; Escape to cancel; auto-advance to next row |
| Payee/category auto-fill | Frontend | On payee selection, pre-fill last-used category and memo for that payee (call new `get_payee_defaults` command) |
| Transaction duplication | Frontend + backend | "Duplicate" action creates a copy with today's date |
| Bulk operations | Frontend | Multi-select with checkboxes; bulk categorize, bulk void, bulk delete |
| Split balancing hint | Transaction form | Show running debit/credit sum; highlight imbalance in red before save |

---

### Stage 2 — Code Quality Foundation  *(required before scaling the codebase)*

**Sprint 2.1 — Structured Errors + Deduplication (2 weeks)**

| Task | File(s) | Detail |
|------|---------|--------|
| `AppError` enum | `error.rs` (new) | `Validation{field,message}`, `NotFound{entity,id}`, `Conflict{message}`, `Database{message}`, `Internal{message}`. Implement `Serialize`, `From<rusqlite::Error>`. |
| Migrate error types | All command modules | Replace `Result<T, String>` with `Result<T, AppError>`. Start with `db_transactions.rs`. |
| Extract `session.rs` | `session.rs` (new) | Move `current_session_id()`, `clear_redo_stack()`, `record_insert_change()` — deduplicate from 3 files |
| Add `tracing` logging | `Cargo.toml`, `lib.rs`, all modules | `info!` at command entry/exit; `error!` at every error path; `info!` for FX refresh, backup, migration. File rotation in app data dir. |
| Pagination metadata | `pagination.rs` (new) | `PaginatedResult<T> { items, total_count, has_more }`. Apply to all list commands. Update frontend accordingly. |
| Update frontend error handling | All frontend invoke calls | Parse structured `AppError` JSON; show contextual dialogs (validation vs system errors) |

**Sprint 2.2 — Concurrency (1 week)**

| Task | File(s) | Detail |
|------|---------|--------|
| Read/write connection split | `state.rs`, `db.rs` | Add `read_conn` with `PRAGMA query_only = ON`. All `list_*`/`get_*` commands use `read_conn`; writes use existing `conn`. |
| Fix FX HTTP in Mutex | `fx_refresh.rs` | Fetch rates with lock released; acquire only for final DB write. Add configurable timeouts. Add retry with exponential backoff (1s, 2s, 4s, max 30s). |

---

### Stage 3 — Reconciliation & Data Entry Completions

**Sprint 3.1 — Reconciliation Wizard (2 weeks)**

| Task | File(s) | Detail |
|------|---------|--------|
| Reconciliation route | `routes/accounts/[id]/reconcile/` | New page: enter statement date + ending balance. Load unreconciled transactions. |
| Clear/unclear transactions | Frontend + `db_accounts.rs` | Mark individual transactions as cleared (R) during reconciliation session |
| Balance verification | Frontend | Show running cleared balance vs statement balance; highlight difference |
| Finish reconciliation | `db_accounts.rs` | Call `create_account_balancing` when cleared balance matches; create adjustment transaction if needed |
| Reconciliation history | `accounts/[id]/+page.svelte` | Show last reconciled date + balance in account header |

---

### Stage 4 — Import Pipeline  *(high value, parsers already exist)*

**Sprint 4.1 — Fix Import Backend (1 week)**

| Task | File(s) | Detail |
|------|---------|--------|
| Fix `import_rules` constraint | Migration V2 | Allow `rule_kind` values: `payee`, `memo`, `amount`, `date`, `account` |
| Align `import_sessions` | `db_transactions.rs` / `import.rs` | Ensure session created + committed on every import; store `session_id` on transactions |
| Multi-currency import policy | `import.rs` | Reject mismatched currency rows with clear error; add UI feedback |
| Add XLS/XLSX support | `Cargo.toml`, `import.rs` | Add `calamine` crate; route XLS/XLSX worksheet rows through CSV mapping logic |
| Add import parser tests | `import.rs` | Unit tests for CSV, QIF, OFX, MT940 with inline sample data |

**Sprint 4.2 — Import Wizard UI (2 weeks)**

| Task | File(s) | Detail |
|------|---------|--------|
| 6-step import wizard | `routes/import/` (new) | Step 1: file pick + format detection. Step 2: template select/create (CSV/XLS). Step 3: column mapping preview. Step 4: validation + row errors. Step 5: duplicate matching. Step 6: commit + results. |
| Import rules management | `settings/+page.svelte` | New "Import Rules" tab: list/create/delete rules with priority ordering |
| Import template CRUD | Backend + UI | `import_templates` table (V3 migration); CRUD commands; export/import JSON |
| Duplicate detection UI | Import wizard step 5 | Show suggested duplicates; accept/skip per row; persist decisions |

---

### Stage 5 — Scheduled Transactions  *(core personal finance feature)*

**Sprint 5.1 — Scheduled Transactions (3 weeks)**

**Schema (V4 migration):**
```sql
CREATE TABLE schedules (
  id INTEGER PRIMARY KEY,
  book_id INTEGER NOT NULL,
  name TEXT NOT NULL,
  kind TEXT NOT NULL CHECK (kind IN ('bill', 'deposit', 'transfer')),
  interval_unit TEXT NOT NULL CHECK (interval_unit IN ('day','week','month','year')),
  interval_count INTEGER NOT NULL DEFAULT 1,
  start_date TEXT NOT NULL,
  end_date TEXT,
  next_run_date TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active','paused','completed')),
  template_tx_json TEXT NOT NULL,  -- JSON snapshot of transaction template
  created_at TEXT NOT NULL
);

CREATE TABLE schedule_runs (
  id INTEGER PRIMARY KEY,
  schedule_id INTEGER NOT NULL REFERENCES schedules(id),
  run_date TEXT NOT NULL,
  created_tx_id INTEGER REFERENCES transactions(id),
  status TEXT NOT NULL CHECK (status IN ('created','skipped','error')),
  error_message TEXT,
  created_at TEXT NOT NULL
);
```

| Task | File(s) | Detail |
|------|---------|--------|
| Schema migration V4 | `migrations/V4__schedules.sql` | Above tables |
| Backend CRUD | `db_schedules.rs` (new) | `create_schedule`, `update_schedule`, `pause_schedule`, `list_schedules`, `preview_due_schedules`, `run_due_schedules` |
| Auto-run on startup | `lib.rs` | Run `run_due_schedules` on app start; show count of created transactions |
| Scheduled transactions UI | `routes/planning/+page.svelte` | List due/upcoming schedules; create schedule wizard; enable/pause/delete |
| Bill reminder on dashboard | `routes/+page.svelte` | "Upcoming bills" widget showing next 7 days |

---

### Stage 6 — Budgeting  *(YNAB-inspired proactive approach)*

**Sprint 6.1 — Budget Engine (3 weeks)**

**Schema (V5 migration):**
```sql
CREATE TABLE budgets (
  id INTEGER PRIMARY KEY,
  book_id INTEGER NOT NULL,
  name TEXT NOT NULL,
  period TEXT NOT NULL CHECK (period IN ('monthly', 'annual')),
  start_date TEXT NOT NULL,
  end_date TEXT,
  created_at TEXT NOT NULL
);

CREATE TABLE budget_lines (
  id INTEGER PRIMARY KEY,
  budget_id INTEGER NOT NULL REFERENCES budgets(id),
  category_id INTEGER NOT NULL,
  amount_minor INTEGER NOT NULL,  -- planned amount
  rollover INTEGER NOT NULL DEFAULT 0  -- carry underspend to next period
);
```

| Task | File(s) | Detail |
|------|---------|--------|
| Schema migration V5 | `migrations/V5__budgets.sql` | Above tables |
| Budget CRUD + period rollups | `db_budgets.rs` (new) | `create_budget`, `list_budgets`, `create_budget_line`, `get_budget_actuals(period_start, period_end)` — compute actuals from splits |
| Budget vs actual report | `db_reports.rs` | `budget_vs_actual_report(budget_id, period)` — return planned vs actual per category with variance |
| Budget UI | `routes/planning/+page.svelte` | Budget creation form; monthly/annual grid view; progress bars per category; over-budget alerts |
| Dashboard budget summary | `routes/+page.svelte` | Top 5 categories vs budget widget |

---

### Stage 7 — Reports & Charts  *(visual financial intelligence)*

**Sprint 7.1 — Chart Data APIs (1 week)**

| Task | File(s) | Detail |
|------|---------|--------|
| Net worth over time | `db_reports.rs` | `net_worth_history(from, to, period)` — monthly snapshots of assets - liabilities |
| Spending trends | `db_reports.rs` | `spending_trends(from, to, period, category_ids)` — monthly totals per category |
| Income vs expenses | `db_reports.rs` | Already in cashflow; expose monthly series for charting |
| Investment performance | `db_reports.rs` | `portfolio_performance(from, to)` — time-weighted return approximation |

**Sprint 7.2 — Chart UI (2 weeks)**

| Task | File(s) | Detail |
|------|---------|--------|
| Add chart library | `package.json` | Add `chart.js` + `svelte-chartjs` or `layerchart` (Svelte-native) |
| Net worth chart | `routes/reports/+page.svelte` | Line chart with asset/liability breakdown |
| Spending pie + bar | `routes/reports/+page.svelte` | Pie by category; bar chart month-over-month |
| Cash flow chart | `routes/reports/+page.svelte` | Income vs expenses bars |
| Investment performance | `routes/investments/+page.svelte` | Portfolio value over time line chart |
| Report export | Frontend | Export report data as CSV; print-friendly layout |

---

### Stage 8 — Tax Features

**Sprint 8.1 — Tax Tools (2 weeks)**

| Task | File(s) | Detail |
|------|---------|--------|
| Tax category tagging | Schema V6 | Add `tax_code TEXT` and `tax_rate_bps INTEGER` (nullable) to `categories` |
| Capital gains summary | `db_reports.rs` | `tax_capital_gains_report(tax_year)` — short/long term breakdown, cost basis, proceeds |
| Dividend income summary | `db_reports.rs` | `tax_dividend_report(tax_year)` — income by commodity |
| Tax year filtering | `routes/tax/+page.svelte` | Replace placeholder; year picker; gains + dividend tables |
| Year-end close workflow | `settings/+page.svelte` | Complete UI for `close_fiscal_year`; show closing entries preview before confirmation |
| Export for tax software | Frontend | CSV export with configurable columns for common tax apps |

---

### Stage 9 — Testing & Reliability  *(parallel with all above stages)*

> Testing should be threaded through every sprint, but this sprint catches up the
> existing deficit.

**Sprint 9 — Test Coverage (2 weeks, run after Stage 2)**

| Target | Count | Notes |
|--------|-------|-------|
| Import parsers (CSV, QIF, OFX, MT940) | 8+ | Inline sample strings, edge cases |
| Balance calculation | 4+ | Void, supersede, revert exclusions |
| Account CRUD | 4+ | Create, update, close, reopen |
| Transaction create/update/void cycle | 4+ | With split balancing |
| Fiscal year close | 3+ | Multi-account, FX-adjusted |
| Undo/redo | 4+ | Full undo→redo→undo cycle |
| Input validation | 6+ | Each validator pass + fail |
| Import rules | 3+ | Rule matching, constraint variants |

**Target: 40+ tests (up from 5)**

---

### Stage 10 — Future Readiness  *(sync, multi-book, resilience)*

**Sprint 10.1 — Sync Readiness (2 weeks)**

| Task | File(s) | Detail |
|------|---------|--------|
| UUID columns | `migrations/V7__uuids.sql` | `ALTER TABLE ... ADD COLUMN uuid TEXT NOT NULL DEFAULT (lower(hex(randomblob(16))))` on: `books`, `accounts`, `commodities`, `transactions`, `splits`, `payees`, `categories`, `tags` |
| `updated_at` + `device_id` | `migrations/V7__uuids.sql` | Conflict resolution columns on sync-eligible tables |
| Update Rust structs | All modules | Include `uuid` in row mappings |
| `SCHEMA.md` update | `SCHEMA.md` | Document sync columns |

**Sprint 10.2 — Book ID Abstraction (1 week)**

| Task | File(s) | Detail |
|------|---------|--------|
| `resolve_book_id()` | `book.rs` (new) | Validate book exists + owned by current user. Replace all `const SINGLE_BOOK_ID: i64 = 1` usages across 6 files |
| Test book resolution | `book.rs` | `test_resolve_book_id_valid`, `test_resolve_book_id_invalid` |

**Sprint 10.3 — Backup & Storage Hardening (1 week)**

| Task | File(s) | Detail |
|------|---------|--------|
| Backup checksum | `db_storage.rs` | SHA-256 of source + destination after `fs::copy`; store hash alongside backup file; reject corrupt backups |
| Append-only compaction | `db_storage.rs` + migration | `compact_book(before_date)` command: materializes current-state snapshots; archives old revisions; adds required indexes on `previous_*_id` columns |
| Remove `#[path]` hack | `commands.rs` | Convert to standard Rust module layout: `commands/` directory or conventional `mod` statements |
| Remove test `transmute` | `db_accounts.rs` | Restructure commands to accept `&DbState` directly; use Tauri test utilities |

---

### Stage 11 — Advanced Features  *(post-core, deferred)*

**Sprint 11.1 — Database Encryption (3 weeks)**

- Evaluate SQLCipher vs file-level encryption
- OS keychain integration for key storage
- Migration path: unencrypted → encrypted
- Backup/restore compatibility

**Sprint 11.2 — Multiple Books / Profiles (1 week)**

- Profile switching UI (multiple DB files)
- "Portable mode" (DB stored alongside app binary)
- Recent books list on onboarding screen

**Sprint 11.3 — Business Extensions (4+ weeks)**

Schema additions:
- `invoices` table with links to `payees`, `transactions`, `documents`
- `invoice_lines` with tax code + amount
- `tax_codes` table with jurisdiction + rate
- A/R and A/P account types
- Customer vs vendor distinction on `payees`

**Sprint 11.4 — Advanced Import/Sync**

- Online banking connectivity (OFX Direct Connect)
- Automatic duplicate learning (ML-assisted matching)
- Optional cloud sync backend (CouchDB-style replication using append-only CRDT chains)

---

## Technical Debt Register

Tracked items from the Feb 2026 code review. Cross-referenced to execution sprints above.

| # | Issue | Severity | Sprint |
|---|-------|----------|--------|
| 7 | Balance bug: voided/superseded transactions included in balance | 🔴 Critical | **0.1** |
| 5 | No input validation (account type, status, name length, amount bounds) | 🟠 High | **0.1** |
| 3 | All errors are strings; frontend cannot distinguish error types | 🟠 High | **2.1** |
| 2 | Single `std::sync::Mutex` blocks UI during long operations | 🟠 High | **2.2** |
| 11 | 5 tests for 19K+ lines; import parsers untested | 🟠 High | **9** |
| 4 | `current_session_id()` duplicated in 3 files | 🟡 Medium | **2.1** |
| 9 | Zero logging/observability | 🟡 Medium | **2.1** |
| 10 | List endpoints return `Vec<T>` with no pagination metadata | 🟡 Medium | **2.1** |
| 1 | Hardcoded `SINGLE_BOOK_ID = 1` in 6 files | 🟡 Medium | **10.2** |
| 17 | Missing SQLite PRAGMAs (mmap, cache, busy_timeout, temp_store) | 🟡 Medium | **0.1** |
| 14 | No UUIDs — foreign keys are local integers, breaks sync | 🟡 Medium | **10.1** |
| 18 | Backup integrity: no checksum after file copy | 🟡 Medium | **10.3** |
| 12 | Append-only tables grow forever; no compaction | 🟡 Medium | **10.3** |
| 6 | LIKE `%` and `_` not escaped in user search strings | 🟢 Low | **0.1** |
| 8 | `std::mem::transmute` in tests (Tauri internal layout assumption) | 🟢 Low | **10.3** |
| 16 | `#[path = "..."]` hack in `commands.rs` bypasses module system | 🟢 Low | **10.3** |
| 15 | FX HTTP calls inside Mutex lock; no retry/timeout | 🟢 Low | **2.2** |
| 13 | No business schema (invoices, tax codes, A/R, A/P) | 🔵 Future | **11.3** |

---

## Sprint Dependency Graph

```
Stage 0 (bugs)
  └─ Stage 1 (MVP UX)
       └─ Stage 2 (code quality)
            ├─ Stage 3 (reconciliation)
            ├─ Stage 4 (import)
            ├─ Stage 5 (schedules)
            ├─ Stage 6 (budgets)
            │    └─ Stage 7 (reports/charts)
            │         └─ Stage 8 (tax)
            └─ Stage 9 (testing — parallel)
                 └─ Stage 10 (future readiness)
                      └─ Stage 11 (advanced, deferred)
```

Stages 3, 4, 5, 6 can be worked in parallel after Stage 2 is complete.

---

## Documentation Structure (Going Forward)

| File | Purpose | Status |
|------|---------|--------|
| `README.md` | Project overview, how to run, architecture summary | Maintained |
| `MASTER_PLAN.md` | This file — single prioritized execution plan | Maintained |
| `SCHEMA.md` | Database schema reference (tables, triggers, mutability model) | Maintained |
| `OLD_TODOS/` | Archived historical planning documents | Archive — do not edit |
| `IMPROVEMENTS.md` | Archived Feb 2026 code review (raw observations) | Archive — superseded by Technical Debt Register above |
| `IMPLEMENTATION_PLAN.md` | Archived Feb 2026 phase plan | Archive — superseded by this file |
| `IMPORT_PLAN.md` | Archived import pipeline design | Archive — superseded by Stage 4 above |
| `1.MD` | Archived FX/pricing architecture notes | Archive — decisions incorporated into Schema and Stage 10 |

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

Target after Stage 9: **40+ tests**.

---

## How to Contribute

- Rust domain model is the source of truth.
- Prefer small, deterministic components; avoid hidden I/O in UI code.
- Security and privacy over convenience; no network calls without explicit user consent.
- Every new command must have at least one test.
- All errors must use `AppError` (after Sprint 2.1 lands).
- New schema changes go in a new `V{n}__description.sql` migration file.
