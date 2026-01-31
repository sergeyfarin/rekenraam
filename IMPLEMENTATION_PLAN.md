# Rekenraam — Comprehensive Implementation Plan

**Goal:** Build a full-featured MS Money/Quicken analogue with Dutch (NL) tax support, multi-currency, investments, imports, and reporting.

---

## Current State Summary

### ✅ Completed Foundation
- **Tauri + Svelte architecture** with local SQLite
- **Comprehensive schema** (V1 + V2): books, accounts, transactions, splits, commodities, payees, categories, tags, people, projects, lots, corporate actions, prices, balancing
- **DB layer**: migrations, WAL, foreign keys, change tracking, triggers for invariants
- **Backend commands**:
  - Storage: `validate_and_set_storage_location`, `get_storage_location`, `get_db_path`, `db_health`, `get_schema_version`
  - Accounts: `create_account`, `get_account`, `list_accounts`, `update_account`, `delete_account`, `list_account_balances`
  - Balancing: `create_account_balancing`, `list_account_balancings`, `unlock_account_balancing`
  - Transactions: `create_transaction_with_splits`, `get_transaction_with_splits`, `list_transactions`, `update_transaction_with_splits`, `delete_transaction`, `list_account_register`
  - Prices: `create_commodity_price`, `list_commodity_prices`, `update_commodity_price`, `delete_commodity_price`
  - Price sources: CRUD for `price_sources` and `commodity_price_sources`
  - Corporate actions: `list_corporate_actions`, `apply_corporate_action_split_merge`
  - Categories/Payees: listing commands exist
- **Frontend shell**: navigation, accounts page with list/filter/sort/group, placeholder pages

### ❌ Missing for Feature Parity
- Transaction entry UI (split editor, running balance)
- Investments UI (holdings, performance, lots)
- Full CRUD UI for categories/payees/tags
- Budgets & scheduled transactions
- Reports (cash flow, net worth, spending)
- NL tax calculations (Box 1/2/3)
- Import (MS Money, QIF, OFX, CSV, PDF statements)
- Multi-currency with FX rates
- Online price fetching
- Backup/restore tooling

---

## Implementation Phases

### Phase 1: Core Transaction Workflow (4-6 weeks)

#### 1.1 Backend Enhancements
| Task | Priority | Effort |
|------|----------|--------|
| Categories CRUD commands | High | 2d |
| Payees CRUD commands | High | 2d |
| Tags CRUD + split_tags handling | Medium | 2d |
| Commodities CRUD (currencies, stocks) | High | 2d |
| Books management (multi-book support) | Medium | 2d |
| Transaction status bulk update | Medium | 1d |
| Running balance calculation endpoint | High | 2d |

#### 1.2 Transaction Register UI
| Task | Priority | Effort |
|------|----------|--------|
| Account register page with transaction list | High | 3d |
| Split editor component (multi-line entry) | High | 5d |
| Category/payee autocomplete dropdowns | High | 3d |
| Inline editing with validation | High | 3d |
| Running balance display | High | 2d |
| Filter bar (date range, status, category, payee) | Medium | 2d |
| Bulk status change (clear/reconcile) | Medium | 2d |

#### 1.3 Account Management UI
| Task | Priority | Effort |
|------|----------|--------|
| Create/edit account dialog | High | 2d |
| Account type configuration | Medium | 1d |
| Close/reopen account flow | Medium | 1d |
| Account hierarchy (parent/child) display | Low | 2d |

---

### Phase 2: Reconciliation & Scheduled Transactions (3-4 weeks)

#### 2.1 Reconciliation
| Task | Priority | Effort |
|------|----------|--------|
| Reconciliation wizard UI | High | 4d |
| Statement balance entry | High | 1d |
| Mark transactions cleared/reconciled | High | 2d |
| Balancing history view | Medium | 2d |
| Unlock/void balancing with audit trail | Medium | 2d |

#### 2.2 Scheduled Transactions
| Task | Priority | Effort |
|------|----------|--------|
| `scheduled_transactions` schema (V3 migration) | High | 1d |
| Recurrence rules (daily/weekly/monthly/yearly) | High | 2d |
| Scheduled transaction CRUD commands | High | 2d |
| Auto-post on due date | Medium | 2d |
| Scheduled bills list UI | High | 3d |
| Reminder notifications | Low | 2d |
| Skip/postpone functionality | Medium | 1d |

**Schema addition (V3):**
```sql
CREATE TABLE scheduled_transactions (
  id INTEGER PRIMARY KEY,
  book_id INTEGER NOT NULL,
  name TEXT NOT NULL,
  payee_id INTEGER,
  memo TEXT,
  amount_minor INTEGER NOT NULL,
  account_id INTEGER NOT NULL,
  category_id INTEGER,
  recurrence_type TEXT NOT NULL CHECK (recurrence_type IN ('daily','weekly','monthly','yearly')),
  recurrence_interval INTEGER NOT NULL DEFAULT 1,
  next_date TEXT NOT NULL,
  end_date TEXT,
  last_posted_date TEXT,
  auto_post INTEGER NOT NULL DEFAULT 0,
  days_before_remind INTEGER DEFAULT 3,
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  FOREIGN KEY(book_id) REFERENCES books(id) ON DELETE CASCADE,
  FOREIGN KEY(payee_id) REFERENCES payees(id) ON DELETE SET NULL,
  FOREIGN KEY(account_id) REFERENCES accounts(id) ON DELETE CASCADE,
  FOREIGN KEY(category_id) REFERENCES categories(id) ON DELETE SET NULL
);
```

---

### Phase 3: Budgets & Planning (3-4 weeks)

#### 3.1 Budget Schema & Backend
| Task | Priority | Effort |
|------|----------|--------|
| `budgets` and `budget_items` schema (V4) | High | 1d |
| Budget CRUD commands | High | 2d |
| Budget vs actual calculation | High | 3d |
| Rollover logic | Medium | 2d |

**Schema addition (V4):**
```sql
CREATE TABLE budgets (
  id INTEGER PRIMARY KEY,
  book_id INTEGER NOT NULL,
  name TEXT NOT NULL,
  period_type TEXT NOT NULL CHECK (period_type IN ('monthly','quarterly','yearly')),
  start_date TEXT NOT NULL,
  end_date TEXT,
  rollover_enabled INTEGER NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  FOREIGN KEY(book_id) REFERENCES books(id) ON DELETE CASCADE
);

CREATE TABLE budget_items (
  id INTEGER PRIMARY KEY,
  budget_id INTEGER NOT NULL,
  category_id INTEGER NOT NULL,
  amount_minor INTEGER NOT NULL,
  notes TEXT,
  FOREIGN KEY(budget_id) REFERENCES budgets(id) ON DELETE CASCADE,
  FOREIGN KEY(category_id) REFERENCES categories(id) ON DELETE CASCADE,
  UNIQUE(budget_id, category_id)
);
```

#### 3.2 Budget UI
| Task | Priority | Effort |
|------|----------|--------|
| Budget planner grid (categories × months) | High | 4d |
| Planned vs actual visualization | High | 3d |
| Category goal progress bars | Medium | 2d |
| Variance alerts | Low | 1d |

---

### Phase 4: Investments & Portfolio (4-5 weeks)

#### 4.1 Holdings & Lots
| Task | Priority | Effort |
|------|----------|--------|
| Holdings summary endpoint | High | 3d |
| Lot tracking: FIFO/LIFO/Specific ID | High | 3d |
| Cost basis calculations | High | 3d |
| Realized gains report | High | 3d |
| Unrealized gains calculation | High | 2d |

#### 4.2 Online Price Fetching
| Task | Priority | Effort |
|------|----------|--------|
| Price provider abstraction | High | 2d |
| Yahoo Finance integration | High | 3d |
| Alpha Vantage integration | Medium | 2d |
| ECB/Fixer.io FX rates | High | 2d |
| Amsterdam/Euronext support | High | 2d |
| Price fetch scheduler | Medium | 2d |
| Manual price entry UI | High | 2d |

**Price providers to support:**
- Yahoo Finance (global stocks, ETFs, indices)
- Alpha Vantage (US stocks, crypto)
- ECB (Euro FX rates)
- Fixer.io (multi-currency FX)
- Euronext (Amsterdam stocks)
- Morningstar (mutual funds)

#### 4.3 Investment UI
| Task | Priority | Effort |
|------|----------|--------|
| Holdings dashboard | High | 4d |
| Position detail with lots | High | 3d |
| Performance chart (time-weighted return) | Medium | 3d |
| Asset allocation pie chart | Medium | 2d |
| Corporate actions list & entry | Medium | 2d |
| Dividend tracking | Medium | 2d |

---

### Phase 5: Multi-Currency (2-3 weeks)

#### 5.1 Backend
| Task | Priority | Effort |
|------|----------|--------|
| FX rate table enhancements | High | 2d |
| Cross-rate calculation | High | 2d |
| Per-transaction exchange rate | Medium | 2d |
| Multi-currency split support | Medium | 2d |
| Base currency conversion for reports | High | 2d |

#### 5.2 UI
| Task | Priority | Effort |
|------|----------|--------|
| Currency selector in transaction entry | High | 2d |
| Exchange rate display/override | Medium | 2d |
| Currency formatting per account | High | 1d |
| FX gains/losses report | Medium | 2d |

---

### Phase 6: Reports & Analytics (4-5 weeks)

#### 6.1 Core Reports Backend
| Task | Priority | Effort |
|------|----------|--------|
| Net worth report (assets - liabilities over time) | High | 3d |
| Cash flow report (income vs expenses by period) | High | 3d |
| Spending by category report | High | 2d |
| Account trends report | Medium | 2d |
| Payee spending report | Medium | 2d |
| Tag-based analysis | Low | 2d |
| Report caching with `report_cache` table | Medium | 2d |

#### 6.2 Reports UI
| Task | Priority | Effort |
|------|----------|--------|
| Report dashboard with tiles | High | 3d |
| Date range picker (YTD, last 12mo, custom) | High | 2d |
| Interactive charts (Chart.js or similar) | High | 4d |
| Drill-down to transactions | Medium | 3d |
| Export to PDF/CSV | Medium | 3d |
| Saved report configurations | Low | 2d |

**Report Types:**
1. **Net Worth Statement** — Total assets minus liabilities with trend
2. **Cash Flow** — Income vs expenses, monthly breakdown
3. **Spending by Category** — Pie/bar chart with drill-down
4. **Budget Variance** — Planned vs actual by category
5. **Investment Performance** — Return calculations, benchmarking
6. **Tax Summary** — NL Box 1/2/3 breakdown (Phase 7)

---

### Phase 7: Dutch Tax Support (3-4 weeks)

#### 7.1 NL Tax Model
The Netherlands has a unique "box" system:
- **Box 1:** Income from work and home (salary, business income, home mortgage interest)
- **Box 2:** Income from substantial interest (>5% ownership in companies)
- **Box 3:** Income from savings and investments (deemed return on assets)

#### 7.2 Schema Additions (V5)
```sql
CREATE TABLE tax_categories (
  id INTEGER PRIMARY KEY,
  book_id INTEGER NOT NULL,
  category_id INTEGER NOT NULL,
  tax_box TEXT NOT NULL CHECK (tax_box IN ('box1_income','box1_deduction','box2','box3_exempt','box3_taxable','not_applicable')),
  tax_code TEXT,
  notes TEXT,
  FOREIGN KEY(book_id) REFERENCES books(id) ON DELETE CASCADE,
  FOREIGN KEY(category_id) REFERENCES categories(id) ON DELETE CASCADE,
  UNIQUE(book_id, category_id)
);

CREATE TABLE tax_reference_dates (
  id INTEGER PRIMARY KEY,
  book_id INTEGER NOT NULL,
  tax_year INTEGER NOT NULL,
  reference_date TEXT NOT NULL, -- Jan 1 for Box 3
  FOREIGN KEY(book_id) REFERENCES books(id) ON DELETE CASCADE,
  UNIQUE(book_id, tax_year)
);
```

#### 7.3 Implementation Tasks
| Task | Priority | Effort |
|------|----------|--------|
| Tax category mapping UI | High | 2d |
| Box 1 income/deduction report | High | 3d |
| Box 3 reference date balance (Jan 1) | High | 3d |
| Box 3 deemed return calculation (forfaitair rendement) | High | 2d |
| Box 2 substantial interest tracking | Medium | 2d |
| Capital gains report (verkoop aandelen) | High | 3d |
| Dividend withholding tracking | Medium | 2d |
| Tax year export (XML for Belastingdienst) | Medium | 3d |
| Hypotheekrente aftrek calculation | Medium | 2d |

#### 7.4 NL-Specific Features
- **Vermogensrendementsheffing (Box 3)**:
  - Calculate total assets on January 1st
  - Apply tiered deemed return rates (updated annually)
  - Subtract tax-free allowance (heffingsvrij vermogen)
  
- **Dividend tracking**:
  - Dutch dividendbelasting (15%)
  - Foreign withholding tax credits

- **Capital gains**:
  - Track purchase price per lot
  - Calculate taxable gain on sale
  - Distinguish Box 2 vs Box 3

---

### Phase 8: Import/Export (4-5 weeks)

#### 8.1 Import Formats

##### 8.1.1 MS Money Import
| Task | Priority | Effort |
|------|----------|--------|
| Parse .mny file (OLE structured storage) | High | 5d |
| Map MS Money accounts to Rekenraam | High | 2d |
| Import transactions with splits | High | 3d |
| Import categories and payees | High | 2d |
| Import investment lots | Medium | 3d |
| Handle currency conversion records | Medium | 2d |

##### 8.1.2 QIF Import
| Task | Priority | Effort |
|------|----------|--------|
| QIF parser (Quicken Interchange Format) | High | 3d |
| Account type mapping | High | 1d |
| Split transaction support | High | 2d |
| Investment transaction types | Medium | 2d |

##### 8.1.3 OFX Import
| Task | Priority | Effort |
|------|----------|--------|
| OFX/QFX parser | High | 3d |
| Bank statement import | High | 2d |
| Credit card statement import | High | 2d |
| Duplicate detection | High | 2d |

##### 8.1.4 CSV Import
| Task | Priority | Effort |
|------|----------|--------|
| Generic CSV import wizard | High | 3d |
| Column mapping UI | High | 3d |
| Preview and validation | High | 2d |
| Save mapping templates | Medium | 2d |
| Date format detection | Medium | 1d |

##### 8.1.5 PDF Statement Import
| Task | Priority | Effort |
|------|----------|--------|
| PDF text extraction (pdf-extract crate) | Medium | 3d |
| Bank statement templates | Medium | 4d |
| Dutch bank support (ING, ABN AMRO, Rabobank) | High | 4d |
| Brokerage statement parsing | Medium | 3d |
| Manual correction UI | Medium | 2d |

**Dutch Banks PDF Support:**
- ING (Mijn ING exports)
- ABN AMRO (Internet Bankieren)
- Rabobank (Rabo Internetbankieren)
- SNS Bank
- ASN Bank
- Triodos Bank

#### 8.2 Import UI
| Task | Priority | Effort |
|------|----------|--------|
| Import wizard (file type detection) | High | 3d |
| Account mapping step | High | 2d |
| Transaction preview/edit | High | 3d |
| Duplicate detection UI | High | 2d |
| Import progress/summary | Medium | 1d |

#### 8.3 Export
| Task | Priority | Effort |
|------|----------|--------|
| QIF export | Medium | 2d |
| CSV export (customizable columns) | High | 2d |
| PDF report export | Medium | 3d |
| Tax year summary XML (NL format) | Medium | 3d |

---

### Phase 9: Bank Connectivity (Optional, 4-6 weeks)

#### 9.1 Open Banking / PSD2 (EU)
| Task | Priority | Effort |
|------|----------|--------|
| Research Dutch bank APIs | Medium | 2d |
| Nordigen/GoCardless integration | Medium | 4d |
| Account consent flow | Medium | 3d |
| Transaction sync | Medium | 3d |
| Balance sync | Medium | 2d |

**Note:** Most Dutch banks support PSD2 through aggregators like Nordigen.

#### 9.2 Manual Entry Enhancements
| Task | Priority | Effort |
|------|----------|--------|
| Quick-entry mode | High | 2d |
| Keyboard shortcuts | High | 2d |
| Auto-fill from history | Medium | 2d |
| Receipt scanning (future) | Low | - |

---

### Phase 10: Settings & Polish (2-3 weeks)

#### 10.1 Settings UI
| Task | Priority | Effort |
|------|----------|--------|
| Storage location management | High | 2d |
| Backup/restore functionality | High | 3d |
| Default currency setting | High | 1d |
| Date format preferences | Medium | 1d |
| Number format (Dutch: 1.234,56) | High | 1d |
| Theme selection | Low | 1d |
| Data export options | Medium | 2d |

#### 10.2 Backup & Recovery
| Task | Priority | Effort |
|------|----------|--------|
| One-click backup to file | High | 2d |
| Scheduled backups | Medium | 2d |
| Restore from backup | High | 2d |
| Data integrity check | Medium | 2d |

#### 10.3 Performance & Polish
| Task | Priority | Effort |
|------|----------|--------|
| Large dataset optimization | Medium | 3d |
| Lazy loading for registers | Medium | 2d |
| Search performance | Medium | 2d |
| UI polish and accessibility | Medium | 3d |
| Keyboard navigation | Medium | 2d |

---

## Technical Architecture

### Price Fetching Service (Rust)

```rust
// src-tauri/src/price_providers/mod.rs
pub trait PriceProvider: Send + Sync {
    fn name(&self) -> &'static str;
    fn fetch_price(&self, symbol: &str, date: Option<NaiveDate>) -> Result<Price, Error>;
    fn fetch_historical(&self, symbol: &str, start: NaiveDate, end: NaiveDate) -> Result<Vec<Price>, Error>;
    fn supports_symbol(&self, symbol: &str) -> bool;
}

pub struct YahooFinance { /* ... */ }
pub struct AlphaVantage { api_key: String }
pub struct ECBRates { /* ... */ }
pub struct Euronext { /* ... */ }
```

### Import Architecture

```rust
// src-tauri/src/import/mod.rs
pub trait ImportFormat: Send + Sync {
    fn detect(data: &[u8]) -> bool;
    fn parse(&self, data: &[u8]) -> Result<ImportData, Error>;
    fn name(&self) -> &'static str;
}

pub struct ImportData {
    pub accounts: Vec<ImportAccount>,
    pub transactions: Vec<ImportTransaction>,
    pub payees: Vec<String>,
    pub categories: Vec<ImportCategory>,
}

pub struct QifImporter;
pub struct OfxImporter;
pub struct CsvImporter { mapping: ColumnMapping }
pub struct MsMoneyImporter;
pub struct PdfImporter { template: BankTemplate }
```

### Report Generation

```rust
// src-tauri/src/reports/mod.rs
pub trait Report: Send + Sync {
    fn report_type(&self) -> &'static str;
    fn generate(&self, conn: &Connection, params: &ReportParams) -> Result<ReportData, Error>;
}

pub struct NetWorthReport;
pub struct CashFlowReport;
pub struct SpendingByCategoryReport;
pub struct InvestmentPerformanceReport;
pub struct TaxSummaryReport { tax_year: i32, country: Country }
```

---

## Database Migrations Summary

| Version | Description | Status |
|---------|-------------|--------|
| V1 | Core schema, invariants, seed data | ✅ Done |
| V2 | Account balancing | ✅ Done |
| V3 | Scheduled transactions | 📋 Planned |
| V4 | Budgets | 📋 Planned |
| V5 | Tax categories (NL) | 📋 Planned |
| V6 | Import history | 📋 Planned |
| V7 | Report preferences | 📋 Planned |

---

## UI Component Library

Using **Carbon Design System** (IBM) via `carbon-components-svelte`:

### Key Components to Build
- `SplitEditor` — Multi-line transaction entry
- `AccountSelector` — Searchable account dropdown
- `CategoryPicker` — Hierarchical category selection
- `DateRangePicker` — For reports and filters
- `AmountInput` — Currency-aware number input
- `ReconciliationWizard` — Step-by-step reconciliation
- `ImportWizard` — File upload and mapping
- `ReportChart` — Chart.js wrapper for reports

---

## Priority Summary

### Must-Have (MVP+)
1. Transaction register with split editor
2. Full account management
3. Categories/payees/tags CRUD
4. Basic reconciliation
5. Net worth and cash flow reports
6. CSV import
7. Manual price entry

### Should-Have (v1.0)
8. Investments with lots
9. Online price fetching
10. QIF/OFX import
11. Budget planning
12. NL tax Box 3 calculation
13. Scheduled transactions

### Nice-to-Have (v1.x)
14. MS Money import
15. PDF statement parsing
16. PSD2 bank connectivity
17. Full NL tax integration
18. Advanced analytics

---

## Estimated Timeline

| Phase | Duration | Cumulative |
|-------|----------|------------|
| Phase 1: Core Transactions | 5 weeks | 5 weeks |
| Phase 2: Reconciliation | 3 weeks | 8 weeks |
| Phase 3: Budgets | 3 weeks | 11 weeks |
| Phase 4: Investments | 5 weeks | 16 weeks |
| Phase 5: Multi-Currency | 2 weeks | 18 weeks |
| Phase 6: Reports | 4 weeks | 22 weeks |
| Phase 7: NL Tax | 4 weeks | 26 weeks |
| Phase 8: Import/Export | 5 weeks | 31 weeks |
| Phase 9: Bank Connect | 4 weeks | 35 weeks |
| Phase 10: Polish | 3 weeks | 38 weeks |

**Total: ~9-10 months for full feature parity**

---

## Next Steps (Immediate)

1. **Complete Phase 1.1**: Add missing CRUD commands for categories, payees, tags, commodities
2. **Build transaction register UI**: This is the core feature users interact with daily
3. **Add category CRUD UI**: Essential for transaction categorization
4. **Implement running balance**: Critical for reconciliation accuracy

Start with:
```
npm run tauri dev
```

Focus first on making the accounts → transactions flow work end-to-end.
