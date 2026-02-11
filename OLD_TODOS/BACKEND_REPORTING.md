
# Backend Reporting Plan

Date: 2026-01-31

## Implementation Steps (Status)
- [x] Add reporting schema migration (report_definitions, report_runs).
- [x] Wire migration into runner.
- [x] Implement report definition CRUD commands.
- [x] Implement report runner with cache keyed by params hash + `book_state.change_seq`.
- [x] Implement SQL-only guard (SELECT-only).
- [x] Add basic template support (JSON with `sql` + optional `param_order`).
- [x] Implement built-in report commands (account balances, cashflow, category spend, payee totals).
- [x] Add backend tests for CRUD, caching, negative paths, and built-in reports.
- [x] Update schema documentation.

This document outlines the backend reporting system plan: schema, commands, caching, and test strategy. It targets a general-purpose reporting pipeline plus a small set of built-in reports (cashflow, category spend, payee totals, account balances), aligned with current schema and `book_state.change_seq` invalidation.

## 1) Goals

- Provide a consistent API for running parameterized reports.
- Cache report results based on params and `book_state.change_seq`.
- Support both predefined reports and custom definitions.
- Keep reporting fully offline and deterministic.

## 2) Data Model (Schema Additions)

### 2.1 `report_definitions`
- `id` INTEGER PRIMARY KEY
- `book_id` INTEGER NOT NULL
- `name` TEXT NOT NULL
- `kind` TEXT NOT NULL (e.g. `builtin`, `custom`)
- `query_type` TEXT NOT NULL (`sql`, `template`)
- `query_text` TEXT NOT NULL (SQL string or template JSON)
- `params_schema` TEXT (JSON schema / constraints)
- `created_at`, `updated_at`
- UNIQUE(`book_id`, `name`)

### 2.2 `report_runs`
- `id` INTEGER PRIMARY KEY
- `book_id` INTEGER NOT NULL
- `definition_id` INTEGER NOT NULL
- `params_hash` TEXT NOT NULL
- `as_of_seq` INTEGER NOT NULL
- `result_json` TEXT NOT NULL (cached JSON)
- `created_at`
- INDEX(`book_id`, `definition_id`, `params_hash`, `as_of_seq`)

### 2.3 Optional `report_catalog` (built-ins)
- Option A: Seed built-in definitions into `report_definitions` migration.
- Option B: Keep built-ins in code and expose via `list_builtin_reports`.

## 3) Report Parameter Contract

### 3.1 Common Parameters
- `book_id` (implicit for single-book mode)
- `date_from`, `date_to`
- `as_of_date` (optional)
- `account_ids`, `category_ids`, `payee_ids`
- `include_closed` (bool)
- `group_by` (enum: `month`, `quarter`, `year`, `none`)

### 3.2 Param Hashing
- Canonicalize JSON params (sorted keys) and hash (e.g., SHA-256).
- `params_hash` + `as_of_seq` determines cache key.

## 4) Backend Commands

### 4.1 Definitions
- `create_report_definition(input)`
- `update_report_definition(input)`
- `delete_report_definition(id)`
- `get_report_definition(id)`
- `list_report_definitions(book_id)`

### 4.2 Running Reports
- `run_report(definition_id, params)`
	- Validate params against `params_schema`.
	- Resolve `as_of_seq` from `book_state`.
	- Check cache in `report_runs`.
	- Execute query (SQL or template engine).
	- Store `result_json` and return.

### 4.3 Built-in Reports (initial set)
- `report_account_balances(params)`
- `report_cashflow(params)`
- `report_category_spend(params)`
- `report_payee_totals(params)`

These can be implemented as:
- Code-native queries (Rust + SQL).
- Or seeded `report_definitions` with SQL templates.

## 5) Report Query Engine

### 5.1 SQL-based Execution
- Use parameterized SQL only; no dynamic string concatenation beyond safe clauses.
- Convert query results to JSON arrays (Vec<HashMap<String, Value>>).

### 5.2 Template-based Execution (optional)
- Template JSON defines base SQL plus allowed filters.
- Engine composes SQL from whitelisted clauses.

## 6) Cache Invalidation

- Use `book_state.change_seq` (already incremented by triggers) to invalidate cached report results.
- Cache key = (`definition_id`, `params_hash`, `as_of_seq`).
- Optionally prune old report_runs periodically.

## 7) Built-in Report Specs (initial)

### 7.1 Account Balances
- Output: account_id, account_name, commodity_id, balance_minor.
- Filters: date_to, account_ids, include_closed.

### 7.2 Cashflow
- Output: period_start, inflow_minor, outflow_minor, net_minor.
- Filters: date_from/date_to, group_by.

### 7.3 Category Spend
- Output: category_id, category_name, total_minor.
- Filters: date_from/date_to, category_ids.

### 7.4 Payee Totals
- Output: payee_id, payee_name, total_minor.
- Filters: date_from/date_to, payee_ids.

## 8) Security + Validation

- Validate `params_schema` using a strict JSON parser (no unknown keys).
- Reject non-SELECT queries if using SQL-based definitions.
- Apply default limits to row count (configurable).

## 9) Tests

### 9.1 Definition CRUD
- Create/list/update/delete report definitions.
- Enforce uniqueness by book/name.

### 9.2 Report Execution
- Validate required params.
- Cache hit/miss with `as_of_seq` change.
- Built-in reports return expected totals.

### 9.3 Negative Cases
- Invalid SQL (non-SELECT) rejected.
- Invalid params schema errors.

## 10) Phased Delivery Plan

### Phase A — Foundations (1–2 weeks)
- Add `report_definitions` and `report_runs` tables + migrations.
- Implement CRUD commands for definitions.
- Implement `run_report` with caching.

### Phase B — Built-in Reports (2–3 weeks)
- Add account balances, cashflow, category spend, payee totals.
- Seed built-in definitions or implement as native commands.

### Phase C — Reporting UX/Extensibility (2–4 weeks)
- Add template-based reports (optional).
- Add pruning/maintenance for report_runs.
- Expand reports (budget vs actual, net worth, investment performance).

## 11) Open Decisions

- SQL definitions vs. template engine for user-defined reports.
- Should report definitions be shared across books?
- JSON schema validator library selection.

---

## 12) Remaining Work (Not Implemented Yet)

1) ✅ **Row limit enforcement for `run_report`**
	- Default limit enforced via `REKENRAAM_REPORT_ROW_LIMIT` (clamped) with optional `limit` param.

2) ✅ **Built-in report catalog option**
	- Added `list_builtin_reports` command with built-in templates and schemas.

3) ✅ **Template engine enhancements**
	- Added whitelist-based `filters` support for safe clause composition.

4) ✅ **Param contract enforcement beyond required/allowed keys**
	- Added type, enum, and date-format validation in `params_schema`.

5) ✅ **Invalid params schema tests**
	- Added tests for malformed schema and type/enum validation failures.

6) ✅ **Report runs maintenance**
	- Added `prune_report_runs` command to keep latest N per definition.


