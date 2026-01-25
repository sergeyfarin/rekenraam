## Rekenraam Database Schema

This document summarizes the current SQLite schema. Source of truth is the migration files in `src-tauri/migrations`.

### Migrations
- V1: `V1__init.sql` (core domain, reports, prices, invariants, seed data)
- V2: `V2__account_balancing.sql` (account balancing and locking)

### Core Entities
**books**
- Ledger container. One book per dataset.
- Key fields: `id`, `name`, `kind`, `base_commodity_id`.

**commodities**
- Currencies and instruments.
- Key fields: `id`, `book_id`, `kind`, `symbol`, `name`, `scale`.

**accounts**
- Accounts in a book.
- Key fields: `id`, `book_id`, `parent_id`, `type`, `name`, `commodity_id`, `institution`, `is_closed`.

**transactions**
- Top-level transaction header.
- Key fields: `id`, `book_id`, `txn_date`, `payee_id`, `memo`, `status`.

**splits**
- Line items for a transaction (double-entry style).
- Key fields: `id`, `tx_id`, `account_id`, `commodity_id`, `amount_minor`, `category_id`.

### Supporting Entities
**payees**
- Counterparties for transactions.

**categories**
- Income/expense/transfer taxonomy.

**tags**
- Freeform tags for splits.

**people**
- People metadata for allocations.

**projects**
- Project/cost center grouping.

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

**split_lot_allocations**
- Allocation of split quantities to lots.

**corporate_actions**
- Stock splits/merges and related adjustments.

### Reports + Cache
**report_cache**
- Cached report snapshots keyed by params and `as_of_seq`.

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

### Seed Data (V1)
- Default book (`Personal`), base currency (`USD`), starter accounts, categories, and manual price source.
