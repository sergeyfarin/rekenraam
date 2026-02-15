## Rekenraam Database Schema

This file summarizes the current SQLite schema.
Source of truth: `src-tauri/migrations/V1__init.sql`.

## Migration Model

- Single consolidated migration: `V1__init.sql` (includes historical V1..V18 schema changes, triggers, indexes, and seed data).

## Schema Objects

### Tables

`schema_migrations`  
`books`  
`commodities`  
`price_ingest_runs`  
`price_observations`  
`payees`  
`categories`  
`tags`  
`people`  
`projects`  
`accounts`  
`transactions`  
`splits`  
`lots`  
`split_lot_allocations`  
`book_state`  
`report_cache`  
`corporate_actions`  
`price_sources`  
`commodity_price_sources`  
`account_balancings`  
`dividend_income_categories`  
`balance_checks`  
`balance_adjustments`  
`notes`  
`events`  
`documents`  
`balance_constraints`  
`import_rules`  
`import_sessions`  
`import_session_transactions`  
`report_definitions`  
`report_runs`  
`countries`  
`institutions`  
`currencies`  
`backup_settings`  
`pricing_policies`  
`pricing_source_assignments`  
`book_base_currency_history`  
`pricing_refresh_state`  
`valuation_snapshots`  
`valuation_snapshot_items`  
`app_runtime_session`  
`session_undo_stack`  
`session_redo_stack`  
`session_reverts`

### Views

- `current_commodities`: rows in `commodities` with no newer revision (`newer.previous_commodity_id = c.id`).
- `current_price_observations`: rows in `price_observations` with no newer correction (`newer.supersedes_observation_id = p.id`).

## Core Domain Notes

- `books`: ledger container with append-only revision linkage via `previous_book_id`.
- `commodities`: currencies + instruments (`kind` enum), with precision (`scale`) and active/default flags.
- `accounts`: append-only account lifecycle (`lifecycle_event` in `open|close|reopen|update`) and ownership metadata (`institution_id`, `country_id`, `is_system`, `system_role`).
- `transactions`: append-only header model with date/time fields: `occurred_date`, optional `occurred_at_utc` + `occurred_tz`, `posted_date`, optional `posted_at_utc` + `posted_tz`.
- `splits`: append-only line items with embedded tagging/allocation columns (`tag_id`, `person_id`, `project_id`, `share_bps`), plus revision linkage via `previous_split_id`.
- `price_observations`: append-only unified pricing fact table for market prices and FX (`observation_kind` in `commodity_market|fx_daily|fx_official`), including derivation and correction linkage (`derived_via_commodity_id`, `supersedes_observation_id`).

## Supporting Domains

- Classification and counterparties: `payees`, `categories`, `tags`, `people`, `projects`.
- Investment and pricing: `price_observations`, `price_sources`, `commodity_price_sources`, `price_ingest_runs`, `pricing_policies`, `pricing_source_assignments`, `book_base_currency_history`, `pricing_refresh_state`, `valuation_snapshots`, `valuation_snapshot_items`, `lots`, `split_lot_allocations`, `corporate_actions`, `dividend_income_categories`.
- Reconciliation and controls: `account_balancings`, `balance_checks`, `balance_adjustments`, `balance_constraints`.
- Attachments/annotations: `notes`, `events`, `documents`.
- Import pipeline: `import_rules`, `import_sessions`, `import_session_transactions`.
- Reporting: `report_cache`, `report_definitions`, `report_runs`.
- Geography/reference: `countries`, `institutions`, `currencies`.
- FX stack: represented via `price_observations` (`observation_kind='fx_daily'|'fx_official'`) plus policy and source-assignment tables.
- Runtime/session state: `book_state`, `app_runtime_session`, `session_undo_stack`, `session_redo_stack`, `session_reverts`, `schema_migrations`.

## Mutability Model

### Append-only tables (revisioned)

`books`, `commodities`, `payees`, `categories`, `tags`, `people`, `projects`, `accounts`, `transactions`, `splits`, `lots`, `split_lot_allocations`, `corporate_actions`, `price_sources`, `commodity_price_sources`, `price_observations`, `price_ingest_runs`, `account_balancings`, `notes`, `import_rules`, `report_definitions`, `report_runs` (delete allowed for retention pruning), `countries`, `institutions`, `currencies`, `pricing_policies`, `pricing_source_assignments`, `book_base_currency_history`, `valuation_snapshots`, `valuation_snapshot_items`.

### Mutable tables

`schema_migrations`, `book_state`, `report_cache`, `dividend_income_categories`, `balance_checks`, `balance_adjustments`, `events`, `documents`, `balance_constraints`, `import_sessions`, `import_session_transactions`, `backup_settings`, `pricing_refresh_state`, `app_runtime_session`, `session_undo_stack`, `session_redo_stack`, `session_reverts`.

## Trigger-Enforced Invariants

- Account commodity must belong to same book as account (`trg_accounts_commodity_book_*`).
- Split commodity must match account commodity (`trg_splits_commodity_matches_account_*`).
- Split account book must match transaction book (`trg_splits_book_matches_txn_*`).
- Split category book must match transaction book when category set (`trg_splits_category_book_matches_txn_*`).
- Split-lot allocation requires matching account and commodity between split and lot (`trg_split_lot_allocations_match_split_lot_*`).
- Commodity precision bounds: `scale` must be 0..9 (`trg_currency_scale_bounds_*`, `trg_non_currency_scale_bounds_*`).
- `price_observations.source_id` must reference an existing `price_sources` row (`trg_prices_source_valid`).
- `accounts.system_role` restricted to `income_summary|expense_summary|retained_earnings` and requires `is_system=1` (`trg_accounts_system_role_*`).

## Date/Time Format Guards

- `accounts.effective_at` must be `YYYY-MM-DD` (`trg_accounts_effective_date_format_ins`).
- `transactions` guards:
	- `occurred_date` and `posted_date` must be `YYYY-MM-DD`.
	- `occurred_at_utc` / `posted_at_utc` (if set) must be UTC ISO-8601 with `Z`.
	- `occurred_tz` / `posted_tz` required and must look like IANA TZ (`*/*`) when corresponding UTC value is set.
	- `created_at` must be UTC ISO-8601 with `Z`.
- `import_sessions.started_at`, optional `committed_at`, and `import_session_transactions.created_at` are validated as UTC ISO-8601 with `Z`.
- Runtime session tables validate UTC ISO-8601 timestamps (`trg_app_runtime_session_datetime_format_ins`, `trg_session_*_datetime_format_ins`).

## Book State / Change Sequence

- `trg_books_insert_state` ensures `book_state` row exists for each book.
- `book_state.change_seq` auto-bumps on insert/update/delete for key tables, including:
	- core: `accounts`, `transactions`, `splits`, `commodities`, `lots`, `split_lot_allocations`
	- classification/support: `categories`, `tags`, `payees`, `people`, `projects`
	- pricing/reconciliation/docs: `price_observations`, `balance_checks`, `balance_adjustments`, `notes`, `events`, `documents`, `balance_constraints`
	- import rules: `import_rules`

## Append-only Enforcement

- Direct `UPDATE`/`DELETE` is blocked by triggers for append-only tables:
	`transactions`, `payees`, `categories`, `tags`, `commodities`, `people`, `projects`, `accounts`, `lots`, `splits`, `price_sources`, `commodity_price_sources`, `books`, `account_balancings`, `currencies`, `corporate_actions`, `countries`, `institutions`, `report_definitions`, `import_rules`, `notes`.
- New versions are inserted and linked via the relevant `previous_*` column.
- Partial unique indexes on `previous_*` columns enforce one-to-one revision chains (where applicable).

## Seed Data (Consolidated V1)

- Inserts default `books` row (`Personal`) and initial `commodities` (`USD`).
- Seeds starter categories (`Groceries`, `Salary`) and starter accounts (`Cash`, `Checking Account`) plus hidden system accounts (`income_summary`, `expense_summary`, `retained_earnings`).
- Seeds `price_sources` with `Manual` and providers (ECB, IRS, HMRC, etc.).
- Seeds `currencies` and `countries` reference data and initializes `backup_settings`, `pricing_policies`, and `book_base_currency_history` for book 1.
- Includes commodity normalization/sync updates (`display_symbol`, `is_default`, `is_active`) across consolidated migration sections.
