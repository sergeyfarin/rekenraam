## Rekenraam Database Schema

This document summarizes the current SQLite schema. Source of truth is `src-tauri/migrations/V1__init.sql`.

### Migrations
- V1: `V1__init.sql` (single consolidated migration containing all schema, triggers, and seed data)

### Core Entities
**books**
- Ledger container. One book per dataset.
- Append-only versioned rows.
- Key fields: `id`, `previous_book_id`, `session_id`, `name`, `kind`, `base_commodity_id`.

**commodities**
- Currencies and instruments.
- Key fields: `id`, `book_id`, `kind`, `symbol`, `name`, `scale`, `is_active`, `is_default`, `display_symbol`.

**accounts**
- Accounts in a book.
- Key fields: `id`, `book_id`, `parent_id`, `type`, `name`, `commodity_id`, `booking_policy`, `institution_id`, `country_id`, `is_hidden`, `is_system`, `system_role`, `is_closed`.

**transactions**
- Top-level transaction header (append-only revision model).
- Key fields: `id`, `book_id`, `previous_tx_id`, `txn_date`, `happened_at_utc`, `posted_at_utc`, `edited_at_utc`, `payee_id`, `memo`, `status`, `reference`, `import_id`, `import_session_id`, `is_deleted`, `session_id`.

**splits**
- Line items for a transaction (double-entry style).
- Key fields: `id`, `tx_id`, `account_id`, `commodity_id`, `amount_minor`, `category_id`, `memo`.

### Supporting Entities
**payees**
- Counterparties for transactions (append-only versioned rows).
- Key fields: `previous_payee_id`, `modified_at_utc`, `effective_at_utc`, `is_deleted`, `session_id`.

**countries**
- ISO-like country catalog per book with optional default currency.
- Append-only versioned rows via `previous_country_id`.

**currencies**
- Currency catalog per book (used as seed/reference data).
- Append-only versioned rows via `previous_currency_id`.

**institutions**
- Banks/brokers/credit unions per book (linked to countries).
- Append-only versioned rows via `previous_institution_id`.

**categories**
- Income/expense/transfer taxonomy (append-only versioned rows).
- Key fields: `previous_category_id`, `modified_at_utc`, `effective_at_utc`, `is_deleted`, `session_id`.

**tags**
- Freeform tags for splits (append-only versioned rows).
- Key fields: `previous_tag_id`, `modified_at_utc`, `effective_at_utc`, `is_deleted`, `session_id`.

**people**
- People metadata for allocations.

**projects**
- Project/cost center grouping.

**dividend_income_categories**
- Maps dividend income categories (optionally per commodity) for reporting and tax.

**account_directives**
- Open/close directives for accounts (audit trail).

**balance_checks**
- Balance assertion records (Beancount-style balance directives).

**pad_directives**
- Pad directives linking target balance and optional pad transaction.

**notes**
- Freeform notes attached to accounts or transactions.
- Append-only versioned rows via `previous_note_id`.

**events**
- Dated events attached to accounts or transactions.

**documents**
- Document references attached to accounts or transactions.

**backup_settings**
- Backup schedule and retention configuration per book.

**balance_constraints**
- Per-account balance rules (min/max, sign enforcement).

**import_rules**
- Import matching rules for payee/memo normalization and mapping.
- Extended with `priority`, `match_type`, amount/date ranges, and `match_account_id`.
- Append-only versioned rows via `previous_import_rule_id`.

**import_sessions**
- Import batch audit trail (`status`, timestamps, source).

**import_session_transactions**
- Per-session linkage to affected transactions (`created`, `updated`, `validated`).

### Relations and Join Tables
Split associations are now embedded directly in `splits` via:
- `tag_id`
- `person_id`
- `project_id`
- `share_bps`

### Investments and Pricing
**commodity_prices**
- Price history for commodities (quoted in another commodity).

**price_sources**
- Price providers.

**commodity_price_sources**
- Mapping of commodity ↔ source with symbol overrides.

**lots**
- Investment lots.
- Adds `cost_basis_minor` for lot cost basis tracking.

**split_lot_allocations**
- Allocation of split quantities to lots.

**corporate_actions**
- Stock splits/merges and related adjustments.
- Append-only versioned rows via `previous_corporate_action_id`.

### Currency + FX
**fx_rates_daily**
- Daily market rates between currencies.
- Key fields: `from_currency_id`, `to_currency_id`, `rate_date`, `rate`, `source`, `source_id`, `is_derived`, `derived_via_currency_id`.

**fx_rates_official**
- Monthly/yearly tax authority rates.
- Key fields: `period_type`, `period_year`, `period_month`, `rate`, `source_name`, `source_url`.

**fx_rate_sources**
- Reference list of FX sources (ECB, IRS, etc.).

**fx_rate_settings**
- Per-book refresh settings (base currency, default source, schedule, weekend policy).

**fx_rate_source_assignments**
- Source assignment per currency pair and effective date range.

**fx_rate_refresh_state**
- Refresh status and errors per currency pair and source.

### Reports + Cache
**report_cache**
- Cached report snapshots keyed by params and `as_of_seq`.

**report_definitions**
- Stored report definitions (SQL or templates).
- Append-only versioned rows via `previous_report_definition_id`.

**report_runs**
- Cached report results keyed by params and `as_of_seq`.

### System State
**schema_migrations**
- Applied migrations.

**book_state**
- Change sequence for invalidation and caching.

**app_runtime_session / session_undo_stack / session_redo_stack / session_reverts**
- Runtime/session bookkeeping for undo/redo and local revert visibility.

### Account Balancing / Locking
**account_balancings**
- Reconciliation/balancing checkpoints by account.
- Fields: `account_id`, `as_of_date`, `balance_minor`, `voided_at`, `void_reason`.
- When locked, older transactions are prevented from modification unless unlock/void is performed.
- Append-only versioned rows via `previous_account_balancing_id`.

### Append-only Guards
- Immutability triggers now guard these append-only tables from direct `UPDATE`/`DELETE`:
	`transactions`, `splits`, `accounts`, `books`, `commodities`, `payees`, `categories`, `tags`,
	`people`, `projects`, `lots`, `price_sources`, `commodity_price_sources`,
	`account_balancings`, `currencies`, `corporate_actions`, `countries`, `institutions`,
	`report_definitions`, `import_rules`, and `notes`.
- New revisions are represented by inserting new rows linked via `previous_*` columns.

### Invariants (Triggers)
- Account commodity must belong to same book.
- Split commodity must match account commodity.
- Split account must belong to same book as transaction.
- Split category must belong to same book as transaction.
- Transaction date/time guards enforce `txn_date` as `YYYY-MM-DD` and require UTC ISO-8601 `Z` format for `happened_at_utc`, `edited_at_utc`, and optional `posted_at_utc`.
- Commodity scale bounds enforced.
- Price source validity checks.
- `book_state.change_seq` is bumped on inserts/updates/deletes for key tables.

### Double-entry Enforcement
- Command-path transaction creation/update enforces at least two splits and balanced split totals (sum to zero) for user-entered journal transactions.
- System-generated transaction flows (imports, trading/dividend helpers) write explicit paired debit/credit splits and still pass split-level schema invariants.

### Seed Data (V1 + later)
- Default book (`Personal`), base currency (`USD`), starter accounts, categories, and manual price source.
- Currency seeds, country defaults, and FX sources/settings are included in V1.
- Starter `accounts` seed inserts are placed after `countries`/`institutions` are created so the consolidated V1 script validates in a single SQLite pass.
