# GnuCash Feature Gap Review (Backend)

This document compares current Rekenraam backend capabilities against the GnuCash feature set (based on https://www.gnucash.org/features.phtml) and lists missing or partial features.

## Current Backend Capabilities (Rekenraam)
- Double-entry transactions with splits and validation.
- Accounts, categories, payees, tags, balances, and account tree rollups.
- Account balancing/locking and balance checks.
- Commodities, prices, price sources, and implicit prices.
- Investment lots, FIFO/LIFO/AVERAGE/STRICT booking, short sales, corporate actions.
- Dividends + reinvestments.
- Inventory/positions and conversions to base commodity.
- Gains reports (realized/unrealized) and posting ledger queries.
- Import rules (payee/memo/amount/date/account matching) and import sessions.
- Notes/events/documents metadata.
- Single-book mode (book_id forced to 1).

## Missing or Partial vs GnuCash Features

### Main Features
- **Checkbook-style register UX** (multi-account register, autofill, reconciled summaries): **UI missing**.
- **Scheduled transactions**: **missing** (no scheduler, templates, reminders).
- **Statement reconciliation UI**: **missing** (backend supports balances/locks, no reconciliation workflow).
- **Reports/graphs**: **partial** (backend reporting endpoints exist; no charting/templated report suite).

### Advanced Features
- **Small business (A/R, A/P, customers, vendors, jobs, invoices, bills, payment terms)**: **missing**.
- **Budgeting tools**: **missing**.
- **Payroll support**: **missing**.
- **Multi-currency trading account automation**: **partial** (balancing command exists; no full FX workflow).
- **Online quotes retrieval**: **missing** (no quote downloader; only manual/implicit prices).

### Data Storage & Exchange
- **QIF/OFX/HBCI import + transaction matching**: **missing** (rules exist but no parsers or matching engine).
- **Export formats (QIF/OFX/CSV)**: **missing**.
- **Online banking (HBCI/AqBanking)**: **missing**.

### Other Goodies
- **Transaction finder/query UI**: **missing**.
- **Check printing**: **missing**.
- **Mortgage/loan repayment assistant**: **missing**.
- **Localization + multi-language UI**: **missing**.
- **User manual/tutorial integration**: **missing**.

## Notes
- Backend already exceeds the GnuCash feature list in some areas (e.g., lot allocation strategies, inventory conversions). Most remaining gaps are **UI workflows** and **import/export pipelines** rather than core ledger primitives.
