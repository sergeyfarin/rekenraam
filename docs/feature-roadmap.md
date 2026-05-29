# Feature Roadmap

This roadmap keeps Rekenraam incremental while preserving the foundations needed for a GnuCash or Microsoft Money successor. Each phase should leave the app runnable, tested, and documented.

## Phase 0: Foundation

Goal: make the empty app safe to evolve.

- SQLite migration runner and schema version table.
- SQLite connection setup with deliberate pragmas.
- Browser-based first-run owner setup with username and password before real financial data entry.
- Translation boundary with English messages and localization-ready built-in data.
- Versioned `/api/v1` route shape for real domain endpoints.
- Basic backup and restore documentation.
- OpenAPI and Bruno coverage for the first real endpoints.

## Phase 1: Books, Commodities, And Accounts

Goal: create the durable accounting skeleton.

- Single owner book.
- Commodity/currency table with exact decimal scale metadata.
- Account tree with account types.
- Opening balances through explicit equity/opening-balance transactions.
- Account list and account detail UI.

## Phase 2: Ledger Transactions

Goal: make daily transaction entry useful.

- Transactions with postings/splits.
- Transfers as ordinary balanced transactions.
- Friendly category UI mapped to income/expense accounts.
- Transaction create, edit, void/archive, and list flows.
- Backend balancing tests and API smoke tests.

## Phase 3: Reconciliation And Core Reports

Goal: make records trustworthy over time.

- Account reconciliation workflow.
- Reconciliation-safe edit/correction behavior.
- Net worth, cashflow, and spending reports.
- Export of core ledger data.

## Phase 4: Import And Cleanup

Goal: reduce manual entry without sacrificing trust.

- CSV import preview and commit.
- Duplicate detection.
- Source metadata retention.
- Import rollback or cleanup workflow.
- Payee/category cleanup helpers.

## Phase 5: Planning

Goal: support forward-looking personal finance.

- Budgets.
- Scheduled transactions.
- Projected balances.
- Simple loan/liability helpers if they fit the existing ledger model.

## Phase 6: Advanced Finance

Goal: add power-user workflows after the core ledger is stable.

- Multi-currency reporting.
- Price history.
- Investment accounts.
- Lots and realized gain/loss reporting.
- Report snapshots where reproducibility matters.

## Later

- Multi-user households.
- Public deployment hardening beyond the owner-user model.
- Bank integrations.
- Mobile apps.
- Plugin architecture.
- Small-business AR/AP and invoicing.

Later phases should remain optional. The earlier phases must still produce a complete self-hosted personal finance app.
