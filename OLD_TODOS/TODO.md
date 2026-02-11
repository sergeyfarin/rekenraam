## Product Plan — MS Money/Quicken Core Parity

### Phase 1 — Foundation and Core Data Workflows
- [ ] Inventory current schema/commands and map gaps against requirements.
- [ ] Transaction CRUD: list/search, update, delete, and bulk import support.
- [ ] Account register UI with split editor, filters, and running balance.
- [ ] Enforce lock rules for edits after account balancing.

### Phase 2 — Reconciliation and Scheduled Activity
- [ ] Reconciliation UX: match/clear, statement balance, and history view.
- [ ] Lock/unlock flows for account balancings (void forward records).
- [ ] Scheduled bills/transactions with recurrence and reminders.

### Phase 3 — Budgets and Categories
- [ ] Budget model (monthly/annual, rollover, category targets).
- [ ] Category/Payee/Tag CRUD UI and backend commands.
- [ ] Budget reports (planned vs actual, variance by category).

### Phase 4 — Investments
- [ ] Holdings and lots UI (basis, lots, realized/unrealized gains).
- [ ] Price management UX and import pipeline.
- [ ] Corporate actions UX (splits/merges) and performance views.

### Phase 5 — Reporting and Tax
- [ ] Core reports: cash flow, net worth, spending, account trends.
- [ ] Report cache service and UI filters.
- [ ] Tax summaries: capital gains, tax categories, year-end exports.

### Phase 6 — Import/Export, Backup, and Multi-Currency
- [ ] Import formats: QIF/OFX/CSV with mapping tools.
- [ ] Export to QIF/CSV and PDF reports.
- [ ] Backup/restore and storage location tooling.
- [ ] Multi-currency: FX rates, per-transaction currency, conversions.

### Optional Enhancements
- [ ] Attachments (receipts, statements) with secure storage.
- [ ] Rules engine for auto-categorization and payee normalization.
- [ ] Advanced analytics (forecasting, scenario planning).
