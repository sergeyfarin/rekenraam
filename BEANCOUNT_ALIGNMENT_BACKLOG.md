# Beancount Alignment Backlog (Mapped to Rekenraam Schema + Commands)

This backlog translates Beancount-style capabilities into concrete tasks aligned with the current SQLite schema and Tauri command surface.

---

## A. Accounting Directives Layer (Beancount-style "directives")
**Goal:** Add Beancount-like structural records for open/close/balance/pad/documents/events.

### A1. Open/Close Directives (Accounts)
**Schema mapping:** `accounts` already exists. Add open/close directives for audit trail.
- Add `account_directives` table (open/close dates, metadata)
- Backend: `create_account_open`, `create_account_close`, `list_account_directives`
- UI: show open/close dates in account detail

### A2. Balance Checks
**Schema mapping:** existing `account_balancings` can be extended to support balance assertions.
- Add `balance_checks` table or reuse `account_balancings` with `kind = 'check'`
- Backend: `create_balance_check`, `list_balance_checks`
- Enforce optional validation (block transaction insert if check fails)

### A3. Pad Transactions
**Schema mapping:** use `transactions` + `splits` to insert balancing "pad" entries.
- Backend: `create_pad_transaction(account_id, target_balance, date)`
- Link to `account_balancings` or `balance_checks`

### A4. Notes / Events / Documents
**Schema mapping:** add a table for metadata linked to accounts or transactions.
- Add `notes`, `events`, `documents`
- Backend CRUD: `create_note`, `create_event`, `create_document`
- UI: attachments and notes in account & transaction detail

---

## B. Booking Engine (Lot + Cost Handling)
**Goal:** replicate Beancount booking rules with lots, costs, and strict validation.

### B1. Lot Booking Policies
**Schema mapping:** `lots`, `split_lot_allocations`
- Add booking policy per account (`FIFO`, `LIFO`, `STRICT`, `AVERAGE`)
- Backend: enforce booking when selling
- Already partially implemented: `sell_commodity` with `FIFO/LIFO/custom`

### B2. Cost Basis + Price Inference
**Schema mapping:** `lots`, `commodity_prices`
- Add `cost_basis_minor` on `lots` or derived from purchase splits
- Validate sell proceeds against cost basis (Beancount sellgains plugin)
- Backend: `validate_sell_gains(tx_id)`

### B3. Short/Long Term Lot Flags
**Schema mapping:** `lots.opened_date`
- Add reporting of holding period
- Backend: `list_lots_with_holding_period`

---

## C. Inventory + Positions (Beancount Inventory)
**Goal:** provide inventory/position reporting like Beancount’s Inventory object.

### C1. Position Aggregation
**Schema mapping:** `splits`, `lots`, `commodity_prices`
- Backend: `get_positions(book_id, as_of_date)`
- Return per account positions with lots + cost basis

### C2. Inventory Conversions
**Schema mapping:** `commodity_prices`
- Backend: `convert_positions(base_commodity_id, date)`
- Mimic Beancount’s price map conversion

---

## D. Price + FX Enhancements
**Goal:** align with Beancount’s implicit price generation and price map.

### D1. Implicit Prices
**Schema mapping:** `commodity_prices`
- When posting a transaction with price annotation, auto-insert price record
- Backend: `add_implicit_price(tx_id)`

### D2. Price Map / Latest Price
- Backend: `get_latest_price(commodity_id, quote_commodity_id)`
- Backend: `get_price_on_date(commodity_id, quote_commodity_id, date)`

---

## E. Validation Plugins (Beancount plugin equivalents)
**Goal:** implement validation checks as built-in commands or plugin-like services.

### E1. Check Closing
- When an account is marked closed, enforce balance = 0
- Backend: `validate_account_closing(account_id)`

### E2. Currency Trading Accounts
- Insert balancing postings for multi-currency transactions
- Backend: `ensure_currency_trading_balances(tx_id)`

### E3. Balance Constraints
- Per-account constraints (min/max, sign enforcement)
- Backend: `create_balance_constraint(account_id, rule)`

---

## F. Reporting Pipeline
**Goal:** align with Beancount’s reporting and account tree realization.

### F1. Account Tree Realization
**Schema mapping:** `accounts`, `splits`
- Backend: `get_account_tree(book_id)`
- Return tree with balances and rollups

### F2. Posting Ledger + Transaction Ledger
- Backend: `list_postings(account_id, range)`
- Backend: `list_transactions(filter)` (already exists)

### F3. Gains & Performance Reports
- Use `lots` + `commodity_prices`
- Backend: `realized_gains_report`, `unrealized_gains_report`

---

## G. Import / Ingest Alignment
**Goal:** align with Beancount ingest model (rules + plugins).

### G1. Import Rules
- Create `import_rules` table (payee normalization, category mapping, account mapping)
- Backend: `apply_import_rules` for CSV/OFX/QIF

### G2. Import Sessions
- Add `import_sessions` table for audit trail
- Backend: `start_import_session`, `commit_import_session`

---

## H. Mapping to Existing Commands (Summary)

| Beancount Capability | Current Rekenraam Commands | Gap |
|----------------------|----------------------------|-----|
| Transactions + splits | `create_transaction_with_splits` | ✅ Done |
| Lot selection | `sell_commodity` (FIFO/LIFO/custom) | ⚠ needs strict booking + cost rules |
| Corporate actions | `apply_corporate_action_split_merge` | ✅ Done |
| Prices | `create_commodity_price` | ⚠ missing implicit price generation |
| Balances / closing | `account_balancings` | ⚠ no closing check |
| Reports | none | ❌ |
| Inventory positions | none | ❌ |
| Plugin validations | none | ❌ |

---

## Suggested Priority (Next 4 Milestones)

### M1 (Now)
- Add balance checks + account closing validation
- Add implicit price generation

### M2
- Add positions/holdings endpoint
- Add realized/unrealized gains reports

### M3
- Add booking strict mode + cost basis enforcement
- Add currency trading balance adjustments

### M4
- Add import rules + import sessions

---

## Notes
- Single-book mode already enforced; most tasks assume book_id = 1.
- Most Beancount features map naturally to `transactions`, `splits`, `lots`, `commodity_prices`.
- Directive-layer tables can be added incrementally without breaking current flows.
