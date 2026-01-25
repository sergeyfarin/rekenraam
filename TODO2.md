## Comprehensive Implementation Plan — Rekenraam (MS Money/Quicken Parity)

### Phase 0 — Baseline Alignment (Inventory + Gaps)
- [ ] Inventory schema, commands, and UI coverage across backend and frontend.
- [ ] Document current command surface and missing CRUD endpoints (transactions, categories, payees, tags, budgets).
- [ ] Confirm migration policy and versioning process for all new schema work.

### Phase 1 — Core Accounting Workflows (MVP+)
**Backend**
- [ ] Add transaction CRUD: list/search (by account/date/payee), update, delete.
- [ ] Enforce account balancing locks on create/update/delete flows.
- [ ] Add transaction-level listing API with pagination/filtering.
- [ ] Add category/payee/tag CRUD commands.

**Frontend**
- [ ] Account detail/register page with split editor, running balance, filters.
- [ ] Transaction entry UX: split rows, category/payee selectors, memo, status.
- [ ] Edit/delete flows with lock-aware error handling.

**Acceptance**
- [ ] Can create, edit, and delete transactions with splits.
- [ ] Locked ranges are respected across all transaction mutations.

### Phase 2 — Reconciliation + Balancing Workflow
**Backend**
- [ ] Expand account balancing: list, create, unlock/void with audit reason.
- [ ] Add reconciliation summary API for statement date range.

**Frontend**
- [ ] Reconciliation screen (statement balance, cleared vs uncleared).
- [ ] Balancing history view and unlock confirmation flow.

**Acceptance**
- [ ] Balancing locks historical transactions; unlock voids future balancings.

### Phase 3 — Scheduled Transactions + Reminders
**Backend**
- [ ] Add scheduled transactions schema: recurrence rules, next_run, status.
- [ ] Commands to create/modify/skip/post scheduled items.

**Frontend**
- [ ] Scheduled bills list + editor.
- [ ] Reminders UI with upcoming schedule.

### Phase 4 — Budgets + Planning
**Backend**
- [ ] Add budget tables: period, category targets, rollover settings.
- [ ] Budget calculation commands (planned vs actual by period).

**Frontend**
- [ ] Budget planner UI + variance reporting.
- [ ] Scenario planning and category goals.

### Phase 5 — Investments + Prices
**Backend**
- [ ] Holdings/positions commands using lots and commodity_prices.
- [ ] Performance and gains (realized/unrealized) reports.
- [ ] Corporate actions UI wiring.

**Frontend**
- [ ] Investment accounts view with lots and performance.
- [ ] Price management UI + import flows.

### Phase 6 — Reports + Tax
**Backend**
- [ ] Report generators: cash flow, net worth, category spend, account trends.
- [ ] Tax reporting: capital gains, tax categories, year-end exports.
- [ ] Leverage report_cache for snapshots.

**Frontend**
- [ ] Reports dashboard with filters and export options.
- [ ] Tax summary screens.

### Phase 7 — Import/Export + Backup
**Backend**
- [ ] Import pipelines: CSV/QIF/OFX with mapping and validation.
- [ ] Export pipelines: CSV/QIF/PDF.
- [ ] Backup/restore operations and storage management.

**Frontend**
- [ ] Import wizard with mapping UI and preview.
- [ ] Backup/restore UI.

### Phase 8 — Multi‑Currency + Advanced Features
**Backend**
- [ ] FX rate ingestion and conversion endpoints.
- [ ] Per-transaction currency handling and base conversions.

**Frontend**
- [ ] Currency-aware entry and display.

### Optional Enhancements
- [ ] Attachments (receipts/statements) with secure storage.
- [ ] Rules engine for auto-categorization and payee normalization.
- [ ] Advanced analytics: forecasting, scenario planning.

### Milestone Map (From Backend TODO + README)
- [x] M1: Storage + migrations (done)
- [x] M2: Storage location (done)
- [x] M3: Core schema + invariants (done)
- [x] M4: Accounts CRUD + transaction create (done)
- [ ] M5: Transaction CRUD + register UI
- [ ] M6: Reconciliation + balancing UX
- [ ] M7: Budgets + categories/payees/tags CRUD
- [ ] M8: Investments + pricing + reports
- [ ] M9: Import/export + tax + backup