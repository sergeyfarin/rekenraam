# V1 Scope

V1 ships as a SQLite-only self-hosted web app in a single Docker container.

## Product Target

V1 is a personal-finance web app for self-hosted use. It should cover the core
daily workflows expected from tools such as Firefly III, GnuCash, Quicken, and
Money Manager Ex for personal books.

Small-business support may influence architecture, but it is not a v1 release
gate.

## V1 Must Have

- Docker deployment with a SQLite-backed single app container serving both API
  and frontend.
- First-admin bootstrap, login/logout, sessions, device attribution, request
  context, book authorization, write attribution, password reset, invites, and
  MFA.
- Account tree, account detail, balances, and single-account or multi-account
  registers.
- Transaction create/edit/delete or void/duplicate/bulk actions with split
  support and server-side accounting invariants.
- Category, payee, tag, person, project, institution, commodity, and currency
  management.
- Reconciliation workflow with balancing history, constraints, unlock or void
  controls, and audit visibility.
- CSV, XLS/XLSX, QIF, OFX/QFX, and HBCI/MT940 import with preview, validation,
  matching, duplicate detection, import audit trail, rules, and commit.
- CSV and QIF exports plus report-data export.
- Core reports: net worth, cash flow, spending by category or payee, account
  trends, realized or unrealized gains, and saved report execution.
- Investment workflows: instrument or security master, holdings, lots,
  buy/sell/short/dividend flows, structured corporate actions, valuation, and
  performance.
- FX and pricing settings, manual refresh, scheduled refresh state/history,
  manual FX and market price entry, historical backfill, source assignment, and
  source health or status views.
- Budgets, scheduled transactions, projected cash balance, loans, mortgages,
  amortization schedules, and loan-payment helpers.
- Transaction finder, advanced register filtering or search, saved views,
  transaction templates, memorized splits, and payee defaults.
- Explicit multi-currency account and cross-currency transfer support.
- Per-user preferences for default book, date format, number format, locale,
  and theme.
- Admin runtime views for database health, integrity checks,
  migration/schema version, backup guidance, and audit/import history.
- Documented SQLite backup/restore guidance and restore-smoke validation.
- Public VPS hardening guidance with HTTPS cookies, private data volume,
  failed-login throttling, and TOTP MFA support.

## V1 Should Have

- Report cache widening for heavier report families.
- Report CSV/print-friendly views.
- Saved/customizable reports, chart views, and account statement or
  income-expense style reports.
- Import mapping template export/import.
- Persistent import cleanup and matching rules for payee normalization,
  category/account mapping, categorization cleanup, and duplicate-handling
  preferences.
- User-visible notes/documents on accounts and transactions if they do not slow
  core workflows.
- Keyboard-first quick entry and shortcut support.
- Payee merge/dedup tooling.
- Deeper automated accounting for complex corporate actions and derivative
  lifecycle events beyond the current structured event records.
- Production Compose example with reverse proxy/TLS guidance.

## Deferred After V1

- Invoices, customers, vendors, VAT workflows, and small-business AR/AP.
- Plugin execution, frontend plugin slots, granular plugin permissions,
  WebAssembly/sidecar runtimes, and plugin marketplaces.
- Built-in/custom theme packs beyond the persisted `theme` preference.
- Attachment OCR.
- Advanced forecasting and scenario planning beyond scheduled-transaction-based
  projections.
- PSD2/open banking and online bank connectivity.
- Hosted service operations.
- Server-side undo/redo unless redesigned around mutation history.

## Release Gate

V1 can release when:

- Self-hosted Docker deployment works from fresh setup instructions.
- Public deployments have documented HTTPS, secure-cookie, backup, and MFA
  requirements.
- Authenticated users can safely manage at least one personal book.
- Core account, register, transaction, reconciliation, import/export, report,
  budget, schedule, loan/liability, investment, pricing, preference, and
  administration workflows are usable without Tauri.
- Backups and restores are documented and smoke-tested.
- Frontend and deployment no longer need Tauri runtime APIs or build artifacts.

## Post-B1 Compatibility Guardrails

Plugin and theme implementation is deferred after b1/v1, but b1 should avoid
blocking later extension work:

- Core behavior stays behind typed `/api/v1` APIs.
- Backend behavior remains separated across routes, services, and repositories.
- Frontend data access continues through `src/lib/api`.
- The existing persisted `theme` preference remains stable for later theme
  token packs.
- Semantic CSS tokens should remain the compatibility layer for future theme
  packs.
- `/api/v1/plugins/*` and `/api/v1/themes/*` stay reserved as future additive
  namespaces only; no placeholder endpoints or runtime are required for b1.
- Future arbitrary-language plugins should run out of process as isolated
  sidecars; constrained trusted plugins should prefer a sandboxed WebAssembly
  host when post-b1 runtime work begins.
