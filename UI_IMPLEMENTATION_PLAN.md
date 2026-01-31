# UI Implementation Plan (Aligning with Backend Capabilities)

Feature-complete personal finance app comparable to GnuCash, MS Money, Quicken.

**Backend Commands Available:** ~139 commands  
**Currently Used in UI:** ~70+ commands  
**Target:** Expose all meaningful backend functionality

---

## Implementation Status Legend
- ✅ Complete
- 🔄 In Progress
- ⏳ Not Started
- ❌ Blocked (backend needed)

---

# Phase 0: UI Framework ✅

## 0.1 Adopt shadcn-svelte UI Framework ✅
- [x] Install Tailwind CSS v4 (`tailwindcss`, `@tailwindcss/vite`)
- [x] Initialize shadcn-svelte (`npx shadcn-svelte@latest init`)
- [x] Configure `$lib` alias in tsconfig.json and svelte.config.js
- [x] Install core components:
  - Button, Card, Tabs, Table, Dialog
  - Input, Label, Select, Badge, Alert, Separator
- [ ] Migrate existing pages to use shadcn components

---

# Phase 1: Settings & Configuration (Foundation)

## 1.1 Database & Backup Settings ✅
- [x] Database path display and management
- [x] Create/move/copy database
- [x] Backup configuration (interval, retention, folder)
- [x] Manual backup and restore
- [x] Database maintenance (vacuum, integrity check)
- [x] Schema version and migration status

## 1.2 Default Currency Setting ⏳
- [ ] Display current default currency
- [ ] Change default currency dropdown
- [ ] Commands: Use settings storage or add dedicated command

## 1.3 Commodity/Currency Management ✅
- [x] List all commodities (`list_commodities`)
- [x] View commodity details (`get_commodity`)
- [x] Rename commodity symbol (`rename_commodity_symbol`)
- [x] Filter by type (currency vs security)

## 1.4 Category Management ✅
- [x] List categories (`list_categories`)
- [x] Create category (`create_category`)
- [x] Edit category (`update_category`)
- [x] Delete category (`delete_category`)
- [x] Category hierarchy (parent selection)

## 1.5 Tag Management ✅
- [x] List tags (`list_tags`)
- [x] Create tag (`create_tag`)
- [x] Edit tag (`update_tag`)
- [x] Delete tag (`delete_tag`)

## 1.6 Payee Management ✅
- [x] List payees (`list_payees`)
- [x] Create payee (`create_payee`)
- [x] Edit payee (`update_payee`)
- [x] Delete payee (`delete_payee`)
- [ ] Merge duplicate payees

## 1.7 Institution Management ✅
- [x] List institutions (`list_institutions`)
- [x] Create institution (`create_institution`)
- [x] Edit institution (`update_institution`)
- [x] Delete institution (`delete_institution`)

## 1.8 Country Management ⏳
- [ ] List countries (`list_countries`)
- [ ] Create country (`create_country`)
- [ ] Edit country (`update_country`)
- [ ] Delete country (`delete_country`)

---

# Phase 2: Accounts (Core)

## 2.1 Account List ✅
- [x] List all accounts with grouping
- [x] Search/filter accounts
- [x] Account tree view with rollups
- [x] Create new account
- [x] Edit account
- [x] Include/exclude closed accounts

## 2.2 Account Detail ✅
- [x] Account header with balance
- [x] Open/close directives display
- [x] Booking policy selector
- [x] Transaction list for account

## 2.3 Account Operations ✅
- [x] Delete account (`delete_account`)
- [x] Account closing validation (`validate_account_closing`)
- [ ] Create open directive (`create_open_directive`)
- [ ] Create close directive (`create_close_directive`)

## 2.4 Account Reconciliation ✅
- [x] Mark transactions as reconciled
- [x] Unlock reconciled transactions (`unlock_balancing`)
- [ ] Reconciliation wizard with statement balance

## 2.5 Balance Constraints ⏳
- [ ] List balance constraints (`list_balance_constraints`)
- [ ] Create balance constraint (`create_balance_constraint`)
- [ ] Delete balance constraint (`delete_balance_constraint`)
- [ ] Validate constraints (`validate_balance_constraints`)
- [ ] Balance checks (`create_balance_check`, `list_balance_checks`)
- [ ] Pad directives (`create_pad_directive`, `list_pad_directives`)

---

# Phase 3: Transactions

## 3.1 Transaction List ✅
- [x] Full transaction list with filtering
- [x] Column sorting and resizing
- [x] Create/edit/delete transactions
- [x] Split transaction editor
- [x] Transaction status (flag, void, reconcile)

## 3.2 Transaction Enhancements ⏳
- [ ] Running balance column (`list_account_register_with_balance`)
- [ ] Postings view (`list_postings`)
- [ ] Currency trading balance (`ensure_currency_trading_balances`)
- [ ] Duplicate detection
- [ ] Bulk operations

## 3.3 Notes, Events, Documents ⏳
- [ ] Notes tab on transaction detail
  - `create_note`, `list_notes`, `update_note`, `delete_note`
- [ ] Events tab (scheduled/recurring)
  - `create_event`, `list_events`, `update_event`, `delete_event`
- [ ] Documents/attachments tab
  - `create_document`, `list_documents`, `update_document`, `delete_document`

---

# Phase 4: Import Workflow

## 4.1 Import Wizard 🔄
- [ ] File upload and format detection (`parse_import_file`)
- [ ] Preview parsed transactions
- [ ] Account mapping for import
- [ ] Apply import rules (`apply_import_rules`)
- [ ] Match against existing (`match_import_transactions`)
- [ ] Review matches and conflicts
- [ ] Commit import (`import_transactions`)

## 4.2 Import Sessions ⏳
- [ ] Start import session (`start_import_session`)
- [ ] Track session progress
- [ ] Commit session (`commit_import_session`)

## 4.3 Import Rules ⏳
- [ ] List import rules (`list_import_rules`)
- [ ] Create import rule (`create_import_rule`)
- [ ] Delete import rule (`delete_import_rule`)
- [ ] Rule priority ordering
- [ ] Match type configuration

---

# Phase 5: Investments & Trading

## 5.1 Holdings View ✅
- [x] Current positions (`get_positions`)
- [x] Convert to base currency (`convert_positions`)
- [x] Account portfolio totals
- [x] Holdings summary with value/cost basis/gain

## 5.2 Investment Transactions ✅
- [x] Buy commodity form (`buy_commodity`)
- [x] Sell commodity form (`sell_commodity`)
- [x] Lot selection for sells (FIFO default)
- [ ] Preview gains before sell (`preview_gains`)
- [x] Dividend transaction (`create_dividend_transaction`)
- [ ] Reinvest dividend / DRIP (`create_reinvest_dividend`)

## 5.3 Lots & Cost Basis ✅
- [x] View lots with holding periods (`list_lots_with_holding_period`)
- [x] Tax lot details (long-term/short-term)
- [x] Cost basis display

## 5.4 Corporate Actions ⏳
- [ ] List corporate actions (`list_corporate_actions`)
- [ ] Apply stock split/merge (`apply_corporate_action`)

## 5.5 Dividend Income Categories ⏳
- [ ] List dividend categories (`list_dividend_income_categories`)
- [ ] Create/edit/delete dividend categories

---

# Phase 6: Prices & Market Data

## 6.1 Price Management ⏳
- [ ] List commodity prices (`list_commodity_prices`)
- [ ] Create price (`create_commodity_price`)
- [ ] Edit price (`update_commodity_price`)
- [ ] Delete price (`delete_commodity_price`)
- [ ] Latest price display (`get_latest_price`)
- [ ] Historical price lookup (`get_price_on_date`)

## 6.2 Price Sources ⏳
- [ ] List price sources (`list_price_sources`)
- [ ] Create price source (`create_price_source`)
- [ ] Edit price source (`update_price_source`)
- [ ] Delete price source (`delete_price_source`)

## 6.3 Commodity-Price Source Mapping ⏳
- [ ] Link commodity to source (`create_commodity_price_source`)
- [ ] View mappings (`list_commodity_price_sources`)
- [ ] Update mapping (`update_commodity_price_source`)
- [ ] Remove mapping (`delete_commodity_price_source`)

## 6.4 Implicit Prices ⏳
- [ ] Generate from transactions (`add_implicit_price`)

---

# Phase 7: Reports

## 7.1 Report Infrastructure ⏳
- [ ] Built-in reports list (`list_builtin_reports`)
- [ ] Custom report definitions CRUD
- [ ] Run report (`run_report`)
- [ ] Report caching and history

## 7.2 Account Reports ⏳
- [x] Account balances report (`account_balances_report`) ✅ (via dashboard)
- [x] Account tree with rollups (`get_account_tree`) ✅
- [ ] Net worth over time chart

## 7.3 Cash Flow Reports ✅
- [x] Cash flow statement (`report_cashflow`)
- [x] Income vs expenses by period

## 7.4 Spending Reports ✅
- [x] Spending by category (`report_category_spend`)
- [x] Spending by payee (`report_payee_totals`)
- [ ] Budget vs actual

## 7.5 Investment Reports ✅
- [x] Realized gains report (`realized_gains_report`)
- [x] Unrealized gains report (`unrealized_gains_report`)
- [ ] Portfolio performance over time

---

# Phase 8: Dashboard & Home

## 8.1 Dashboard ✅
- [x] Net worth summary (assets - liabilities)
- [x] Recent transactions list
- [x] Quick action buttons (new transaction, import)
- [x] Account balances overview (assets/liabilities breakdown)
- [ ] Upcoming scheduled transactions

---

# Phase 9: Additional Features

## 9.1 Tax Features ⏳
- [ ] Capital gains summary
- [ ] Dividend income summary
- [ ] Tax year filtering
- [ ] Export for tax software

## 9.2 Planning/Budgets ❌
- [ ] Budget creation and tracking (needs backend)
- [ ] Forecasting (needs backend)
- [ ] Goals (needs backend)

---

# Implementation Order (Priority)

## Sprint 1: Settings Foundation
1. Default currency setting
2. Category management CRUD
3. Payee management CRUD  
4. Tag management CRUD
5. Full institution/country management

## Sprint 2: Account Enhancements
1. Delete account
2. Account closing validation
3. Balance constraints UI

## Sprint 3: Transaction Enhancements  
1. Running balance in register
2. Notes/Events/Documents tabs

## Sprint 4: Import Workflow
1. Import wizard with file parsing
2. Import rules management
3. Match and commit flow

## Sprint 5: Investments
1. Holdings view
2. Buy/sell forms with lot selection
3. Dividend transactions

## Sprint 6: Prices & Reports
1. Price management
2. Core reports (balances, cash flow, spending)
3. Gains reports

## Sprint 7: Dashboard & Polish
1. Home dashboard
2. UI polish and consistency
3. Performance optimization

---

# Technical Notes

- Use existing Svelte routing structure under `src/routes`
- Tauri commands via `invoke()` from `@tauri-apps/api/core`
- Follow existing patterns from transactions/accounts pages
- Consistent error handling with try/catch and user feedback
- Keep single-book mode assumptions (no book picker)
