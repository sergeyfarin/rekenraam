# UI Implementation Plan (Aligning with Backend Capabilities)

This plan outlines UI work needed to surface the backend capabilities that have been implemented so far.

---

## 1) Navigation & Core Screens
- **Accounts**
  - Account tree view (use `get_account_tree`).
  - Account detail header with open/close directives and balances.
  - Booking policy selector (use `get_account_booking_policy` / `set_account_booking_policy`).
- **Register / Transactions**
  - Account register with running balance (use `list_account_register_with_balance`).
  - Transaction form with split editor (existing `create_transaction_with_splits`, `update_transaction_with_splits`).
  - Postings view for a single account (use `list_postings`).

## 2) Investments & Pricing
- **Investments**
  - Buy/Sell/Reinvest/Dividend forms (use `buy_commodity`, `sell_commodity`, `create_dividend_transaction`, `create_reinvest_dividend`).
  - Lot allocation UI for sells (FIFO/LIFO/AVERAGE/custom + STRICT enforcement).
  - Holdings view (use `get_positions`, `convert_positions`, `list_account_position_totals`, `list_account_position_totals_by_commodity`).
  - Holding period report (use `list_lots_with_holding_period`).
- **Prices**
  - Price list CRUD (use `create_commodity_price`, `list_commodity_prices`, `update_commodity_price`, `delete_commodity_price`).
  - Implicit price generation action (use `add_implicit_price`).
  - Latest price lookup and historical price display (use `get_latest_price`, `get_price_on_date`).

## 3) Validation & Controls
- **Account Closing Validation**
  - UI button to validate closing (use `validate_account_closing`).
- **Balance Constraints**
  - Constraint editor per account (use `create_balance_constraint`, `list_balance_constraints`, `delete_balance_constraint`, `validate_balance_constraints`).
- **Currency Trading Balances**
  - Action on transaction detail to auto-balance per commodity (use `ensure_currency_trading_balances`).

## 4) Notes, Events, Documents
- **Account & Transaction Detail**
  - Notes CRUD (use `create_note`, `list_notes`, `get_note`, `update_note`, `delete_note`).
  - Events CRUD (use `create_event`, `list_events`, `get_event`, `update_event`, `delete_event`).
  - Documents CRUD (use `create_document`, `list_documents`, `get_document`, `update_document`, `delete_document`).

## 5) Import / Ingest
- **Import Wizard**
  - File upload + format detection (`parse_import_file`).
  - Rules preview and apply (`apply_import_rules`).
  - Match duplicates (`match_import_transactions`).
  - Commit import (`import_transactions`).
  - Import sessions tracking (`start_import_session`, `commit_import_session`).
- **Import Rules UI**
  - Rule list + editor with priority, match type, amount/date/account filters (use `create_import_rule`, `list_import_rules`, `delete_import_rule`).

## 6) Reporting
- **Account Tree**
  - Hierarchical tree with rollups (use `get_account_tree`).
- **Gains Reports**
  - Realized gains report (use `realized_gains_report`).
  - Unrealized gains report (use `unrealized_gains_report`).

---

# Suggested Implementation Order

## Phase 1: Core Register & Accounts
1. Account tree view + account detail header.
2. Register view with running balance.
3. Transaction editor with splits.

## Phase 2: Import Workflow
1. Import wizard with parse + preview.
2. Rules application + duplicate matching.
3. Import sessions tracking.

## Phase 3: Investments & Prices
1. Buy/sell/reinvest/dividend forms.
2. Lot allocation UI and holdings view.
3. Prices CRUD + implicit price generation.

## Phase 4: Validation + Reports
1. Balance constraints UI + closing validation.
2. Account tree report.
3. Gains reports.

---

# UI/UX Notes
- Use existing Svelte routing structure under src/routes.
- Add a shared `services/api.ts` wrapper for Tauri invoke calls if not present.
- Reuse Carbon components for consistency with current UI.
- Keep single-book mode assumptions in the UI (no book picker).
