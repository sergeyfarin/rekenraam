## Rekenraam Database Schema

This document summarizes the current SQLite schema. Source of truth is the migration files in `src-tauri/migrations`.

### Migrations
- V1: `V1__init.sql` (core domain, reports, prices, invariants, seed data)
- V2: `V2__account_balancing.sql` (account balancing and locking)
- V3: `V3__dividend_income_categories.sql` (dividend income category mapping)
- V4: `V4__directives_and_documents.sql` (directives, balance checks, pad directives, notes/events/documents)
- V5: `V5__booking_policy_and_cost_basis.sql` (account booking policy + lot cost basis)
- V6: `V6__balance_constraints.sql` (per-account balance constraints)
- V7: `V7__import_rules_sessions.sql` (import rules + import sessions)
- V8: `V8__import_rules_extensions.sql` (import rule priority + match types + amount/date/account match)
- V9: `V9__reporting.sql` (report definitions + report runs cache)
- V10: `V10__institutions_countries.sql` (countries + institutions + account links)
- V11: `V11__currencies_seed.sql` (currencies table + country default currencies + seed data)
- V12: `V12__backup_settings.sql` (backup settings stored per book)
- V13: `V13__currency_management.sql` (currency activation/default + FX rate tables)
- V14: `V14__currency_display_symbols.sql` (display symbols for currencies)
- V15: `V15__fx_rate_settings.sql` (FX settings, source assignments, refresh state)
- V16: `V16__fx_rate_daily_provenance.sql` (daily FX provenance fields)
- V17: `V17__commodities_currency_sync.sql` (sync currencies to commodities)

### Core Entities
**books**
- Ledger container. One book per dataset.
- Key fields: `id`, `name`, `kind`, `base_commodity_id`.

**commodities**
- Currencies and instruments.
- Key fields: `id`, `book_id`, `kind`, `symbol`, `name`, `scale`, `is_active`, `is_default`, `display_symbol`.

**accounts**
- Accounts in a book.
- Key fields: `id`, `book_id`, `parent_id`, `type`, `name`, `commodity_id`, `institution_id`, `country_id`, `is_closed`, `booking_policy`.

**transactions**
- Top-level transaction header.
- Key fields: `id`, `book_id`, `txn_date`, `payee_id`, `memo`, `status`, `reference`, `import_id`.

**splits**
- Line items for a transaction (double-entry style).
- Key fields: `id`, `tx_id`, `account_id`, `commodity_id`, `amount_minor`, `category_id`, `memo`.

### Supporting Entities
**payees**
- Counterparties for transactions.

**countries**
- ISO-like country catalog per book with optional default currency.

**currencies**
- Currency catalog per book (used as seed/reference data).

**institutions**
- Banks/brokers/credit unions per book (linked to countries).

**categories**
- Income/expense/transfer taxonomy.

**tags**
- Freeform tags for splits.

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

**import_sessions**
- Import batch audit trail (`status`, timestamps, source).

### Relations and Join Tables
**split_tags**
- Many-to-many: `splits` ↔ `tags`.

**split_people**
- Many-to-many: `splits` ↔ `people` (with `share_bps`).

**split_projects**
- Many-to-many: `splits` ↔ `projects` (with `share_bps`).

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

**report_runs**
- Cached report results keyed by params and `as_of_seq`.

### System State
**schema_migrations**
- Applied migrations.

**book_state**
- Change sequence for invalidation and caching.

### Account Balancing / Locking (V2)
**account_balancings**
- Reconciliation/balancing checkpoints by account.
- Fields: `account_id`, `as_of_date`, `balance_minor`, `voided_at`, `void_reason`.
- When locked, older transactions are prevented from modification unless unlock/void is performed.

### Invariants (Triggers)
- Account commodity must belong to same book.
- Split commodity must match account commodity.
- Split account must belong to same book as transaction.
- Split category must belong to same book as transaction.
- Commodity scale bounds enforced.
- Price source validity checks.
- `book_state.change_seq` is bumped on inserts/updates/deletes for key tables.

### Seed Data (V1 + later)
- Default book (`Personal`), base currency (`USD`), starter accounts, categories, and manual price source.
- Currency seeds and FX sources (V11-V13) with display symbols (V14).
