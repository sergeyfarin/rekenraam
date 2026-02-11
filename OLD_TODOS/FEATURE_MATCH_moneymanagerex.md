
# MMEX Feature Match Review (Backend + Schema)

Date: 2026-01-31

Sources:
- MMEX README feature list (https://github.com/moneymanagerex/moneymanagerex?tab=readme-ov-file)
- Current backend schema and commands (see [SCHEMA.md](SCHEMA.md))

## 1) Current Backend Capabilities (Summary)

Based on [SCHEMA.md](SCHEMA.md) and existing Rust commands, the backend supports:

- Books, commodities (multi-currency), accounts, transactions, splits.
- Account types: cash, checking, savings, credit, loan, investment, asset, liability, income, expense, equity.
- Multi-level categories; tags on splits; people/projects allocations.
- Payees, notes, events, documents.
- Account balancing/locking, booking policies, balance constraints, directives.
- Investment flows (buy/sell), lots, corporate actions, price history, price sources.
- Import rules + import sessions; parsing for QIF/OFX/HBCI.
- Report cache and gains reports (realized/unrealized).
- Storage move/copy/backup/restore + integrity checks.

## 2) Feature Match Matrix (MMEX → Rekenraam)

Legend: ✅ Supported | ⚠️ Partial | ❌ Missing

| MMEX Feature | Status | Backend Evidence / Gap |
| --- | --- | --- |
| Checking/credit/savings/stock/asset accounts | ✅ | Account types are modeled in [SCHEMA.md](SCHEMA.md). |
| Unlimited nested categories | ✅ | Categories are hierarchical (parent_id). |
| Multiple tags per split | ✅ | split_tags join table. |
| Reminders for scheduled bills/deposits | ❌ | No schedule/reminder tables or commands. |
| Budgeting | ❌ | No budget tables or budget logic. |
| Cash flow forecasting | ❌ | No forecast model; no scheduled tx rollup. |
| One-click reporting with graphs/charts | ⚠️ | Gains reports + report_cache exist; no generic reports or chart data APIs. |
| Import CSV | ❌ | Import pipeline exists, but CSV parsing is not present. |
| Import QIF | ✅ | QIF parsing + import pipeline present. |
| Import OFX | ✅ | OFX parsing + import pipeline present. |
| Custom reports (general reports) | ❌ | No report definitions or templating. |
| Portable SQLite file | ⚠️ | Single SQLite file with move/copy/backup; no explicit “portable mode” UX. |
| AES-encrypted database | ❌ | No encryption layer or key management. |
| Multi-currency | ✅ | commodities + commodity_prices + price_sources. |
| Investments/stock price tracking | ✅ | lots, corporate_actions, commodity_prices. |
| Reconciliation | ⚠️ | account_balancings/locking supports reconciled checkpoints, but no reconciled workflow/reporting API. |
| Attachments (documents) | ⚠️ | documents table exists; UI/workflows unclear. |
| Payees | ✅ | payees table and relationships. |
| Projects/people allocations | ✅ | split_projects + split_people. |

## 3) Missing or Partial Functionality (Key Gaps)

1) **Scheduled transactions & reminders**
	- No schedule rules, next-run tracking, or reminder model.

2) **Budgeting & cash-flow forecasting**
	- No budget entities, periodization, or rollups.
	- No forecast logic combining scheduled transactions with historical trends.

3) **Reporting system (general + charts)**
	- Only gains reports exist; no generic report definitions or chart-friendly aggregations.

4) **CSV import/export**
	- Import pipeline exists but lacks CSV parser and mapping schema.
	- No export commands for CSV/QIF/etc.

5) **Database encryption**
	- No encrypted-at-rest support; no key management or migration to encrypted DB.

6) **Custom reports (MMEX general-reports)**
	- No report templating, parameters, or external report definitions.

7) **Portability UX**
	- File-based storage exists but there is no “portable mode” concept or profile switching workflow.

## 4) Comprehensive Implementation Plan

### Phase 0 — Assessment + Design (1–2 weeks)
- Define parity targets: which MMEX features are essential for MVP parity.
- Specify data models and API contracts for schedules, budgets, reports, and encryption.
- Decide on report engine approach (SQL-based templates vs. query DSL).

### Phase 1 — Scheduled Transactions + Reminders (2–4 weeks)
**Schema**
- Add tables:
  - `schedules` (name, type: bill/deposit/transfer, interval, start_date, end_date, next_run_date, status).
  - `scheduled_transactions` (schedule_id, template_tx_id, amount rules, payee, category, account, memo).
  - `reminders` (schedule_id, remind_days_before, last_reminded_at, channel).
  - `schedule_runs` (schedule_id, run_date, created_tx_id, status, error).

**Backend**
- CRUD commands for schedules and templates.
- Command to “preview due” items.
- Command to run schedule generator (manual trigger + background on app start).

**Tests**
- Due-date calculation, creation of transactions, idempotent runs, and reminder timing.

### Phase 2 — Budgeting + Forecasting (3–5 weeks)
**Schema**
- Add tables:
  - `budgets` (book_id, name, period: monthly/annual, start/end).
  - `budget_lines` (budget_id, category_id, amount_minor, rollover rules).
  - `budget_actuals` (budget_line_id, period_start, actual_minor).

**Backend**
- Budget CRUD + period rollups.
- Compute actuals from transactions and split allocations.
- Forecast endpoints combining scheduled transactions + historical averages.

**Tests**
- Category rollups, period boundaries, forecast calculations.

### Phase 3 — Reporting + Charts (3–6 weeks)
**Schema**
- `report_definitions` (name, kind, sql_or_template, params_schema).
- `report_runs` (definition_id, params_hash, result_cache, created_at).

**Backend**
- Generic report runner with parameter validation.
- Predefined reports: cashflow, category spend, payee totals, account balances.
- Chart data endpoints (timeseries, pie, bar) derived from report results.

**Tests**
- Report parameter validation, caching, invalidation using `book_state.change_seq`.

### Phase 4 — CSV Import/Export (2–4 weeks)
**Import**
- CSV parser with user-mapped columns → `ImportDraft`.
- Reuse existing import rules and sessions.

**Export**
- Export transactions/splits to CSV/QIF.

**Tests**
- Parsing edge cases, locale decimals, time formats, and mapping.

### Phase 5 — Encryption at Rest (2–4 weeks)
**Design**
- Decide between SQLCipher or file-level encryption.
- Key storage strategy (OS keychain, optional passphrase).

**Implementation**
- Storage initialization workflow for encrypted DBs.
- Migration path from unencrypted → encrypted.
- Backup/restore compatibility for encrypted files.

**Tests**
- Encrypted DB open/close, wrong key errors, backup restore.

### Phase 6 — Portability + Profiles (1–2 weeks)
- Add “profiles” (multiple db paths) with quick switching.
- Expose a “portable mode” setting that stores db alongside app.

## 5) Recommended Next Steps (Immediate)

1) Confirm scope: which MMEX features are required for parity vs. future.
2) Start Phase 1 (schedules/reminders) and Phase 4 (CSV import) in parallel.
3) Define report catalog and a minimal reporting framework.

