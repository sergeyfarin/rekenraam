# Product Requirements

This document translates the archived Rust/Tauri and Python/FastAPI experiments into current requirements for the Go, SvelteKit, SQLite app.

This is the source of truth for product requirements in the active repo. The `.archive/` folder is reference material only.

## Product Intent

Rekenraam is a self-hosted personal finance app aimed at becoming a practical successor to Microsoft Money and similar desktop finance tools.

Primary deployment targets:

- Local-network self-hosting for a single household owner.
- VPS deployment for remote access under the same single-binary and Docker Compose app shape.

Primary product goal:

- Deliver the app incrementally, feature by feature, without requiring a one-shot migration from archived experiments.

## Product Principles

- Personal finance first.
- Accounting correctness over shortcut UX.
- Incremental delivery over big-bang rewrites.
- Self-hosted ownership, backup, restore, and export are core product capabilities.
- Same-origin deployment: one Go app serving API and frontend.
- Archive ideas must be translated to the current stack before adoption.

## Working Decisions

- Theme infrastructure starts as a token-based system with light and dark themes only.
- Runtime product scope is strictly single-owner and single-user for now; if household sharing ever becomes real scope, naming can be revised deliberately then.
- First-run setup is browser-based and guided. It creates the owner first, then grows to include the default book, default currency preference, system accounts, default categories, and optional additional categories as those feature slices are implemented.
- Public VPS deployments require HTTPS and local authentication.
- Public VPS deployment with real financial data requires MFA; public deployment may be delayed until MFA is implemented.
- Localhost development may use HTTP. LAN/private deployments should strongly prefer HTTPS through a reverse proxy or app-provided certificate and key configuration.
- Browser-warning-free LAN/private HTTPS requires either a trusted certificate for a real domain or installing a trusted local certificate authority on client devices.
- SQLite database encryption is deferred for early local use, but the product should document that encrypted-at-rest storage may be needed before recommending higher-risk deployments.
- The first non-negotiable report set is net worth, cashflow, and spending by category.
- The first mandatory export formats are CSV export of core ledger data and QIF export.
- Attachments such as statement PDFs and receipts are intentionally out of scope for now.
- The minimum mobile requirement is responsive support for full core workflows, including transaction entry.

## Decisions To Confirm Before Implementation

- No remaining product-wide decisions block Phase 0 foundation coding. Feature-specific decisions remain listed below and should be resolved before their related slices.

## Phase 0 Foundation Coding Gates

These are locked before implementation starts:

- First-run setup starts with `GET /api/v1/setup/status` and `POST /api/v1/setup/owner`.
- Setup progress is persisted as named steps, starting with `owner`, `book`, `currencies`, `system_accounts`, and `categories`, rather than a single boolean.
- Setup status must expose a derived install state so future seeded setup steps do not block current installs before their APIs and UI exist.
- Early password recovery has no unauthenticated browser or email reset flow. Use an operator-controlled local recovery path that requires server or database access, creates a verified backup by default, supports an explicit emergency override, and invalidates existing sessions.
- Stable `/api/v1` endpoints are OpenAPI-first with `api/openapi/openapi.yaml` as the checked source of truth.
- Initial API error codes are `VALIDATION_FAILED`, `UNAUTHENTICATED`, `FORBIDDEN`, `NOT_FOUND`, `CONFLICT`, `CSRF_INVALID`, `RATE_LIMITED`, `RESOURCE_BUSY`, `SETUP_REQUIRED`, `SETUP_ALREADY_COMPLETE`, and `INTERNAL_ERROR`.
- Request IDs are server-generated UUIDs, returned in `X-Request-ID`, and included in logs.
- Before the first non-placeholder UI screen, add Paraglide/Inlang message catalogs and semantic `light`/`dark` theme tokens.

## Pre-Domain Readiness Checklist

These decisions must be locked before the first real domain slice beyond setup/auth starts:

- Money representation limits: maximum `quantity_scale`, allowed `commodity_code` formats, custom commodity identifiers, and validation behavior for values that exceed limits.
- Exact OpenAPI type generation/check command and frontend generated-client path.
- Domain lifecycle status taxonomy for drafts, posted records, voided records, archived records, and corrective entries.

Already locked before domain work:

- Foundation coding gates for setup, password recovery, API contracts, error codes, request IDs, i18n scaffolding, and theme token scaffolding.
- Physical delete is allowed only for never-posted drafts; posted financial records use void, archive, or corrective-entry workflows.
- Static frontend routing: Go serves real assets, returns 404 for unknown `/api/` and missing asset-like paths, and falls back to the SvelteKit app shell for extensionless non-API routes.
- SQLite connection, migration, busy-timeout, WAL, and backup behavior.
- Docker runtime base image.
- Password hashing and staged first-run setup.

These can wait until the related feature slice:

- First non-English UI language.
- Full structured JSON export scope.
- Attachment storage.
- Report snapshots.
- Import formats beyond CSV.
- Multi-user or household model.

## Must-Have Cross-Cutting Requirements

These apply across all feature phases.

### Data Integrity

- The financial model must remain double-entry capable.
- Transactions with postings are the canonical ledger primitive. "Split" may be
  used as user-facing copy for split transaction entry, but "posting" is the
  canonical schema and service term.
- Transfers are ordinary balanced transactions.
- Financial quantities must never use floating point storage.
- Reconciled or closed-period data must prefer corrective workflows over silent in-place mutation.
- Posted financial data must not be hard-deleted.

### Self-Hosting And Operations

- The app must run as a single Go binary serving the static frontend.
- Docker Compose must package the same app shape, not a separate architecture.
- SQLite is the primary database.
- Backup and restore must be documented and eventually productized.
- Users must be able to export their data in durable formats.

### Security

- Local authentication is required before real financial data entry ships.
- The initial auth flow is browser first-run setup with owner username and password.
- Password hashing must use Argon2id with self-describing stored hashes and upgradeable parameters.
- Owner login must throttle repeated failed attempts by username and client IP in a way that survives process restarts and should upgrade stale Argon2id password hashes on successful login.
- Public VPS deployments must assume an untrusted network.
- Forwarded proxy headers must be ignored unless the deployment explicitly enables trusted proxy handling and limits trusted peers to an allowlisted proxy CIDR set.
- All privileged operations must require authentication.
- Session, audit, and request attribution should be designed in from the first real user flows.

### Frontend Quality

- UI copy must go through a translation boundary.
- The app must support multilingual UI from an early stage.
- English is the initial implementation language; translation boundaries must keep UI copy and built-in data ready to add other languages without reworking UI code or database schema.
- Built-in database records not entered by the user or imported from a source must use stable codes or keys and resolve display labels through localization assets.
- Built-in currency, category, account-class, account-kind, system-account, and other app-defined labels must be translated at frontend render time from stable identifiers, not stored as localized canonical database names during setup.
- The design system must support themes through semantic tokens rather than hard-coded colors.
- Theme support starts with light and dark only, but the token model should avoid blocking future named themes.
- Desktop and mobile layouts must both be intentional and usable.
- Accessibility is required: keyboard navigation, focus states, readable contrast, form labels, and screen-reader-friendly structure.
- Loading, empty, error, and success states must be designed explicitly rather than left implicit.

### Product Trust

- Financial changes should be explainable through audit-friendly data and UX.
- Import workflows must preserve source metadata where practical.
- Duplicate detection, reconciliation, and correction behavior must be explicit.
- Reports and exports must reflect durable accounting semantics rather than ad hoc UI summaries.

## Delivery Phases

These phases keep Rekenraam incremental while preserving the foundations needed for a GnuCash or Microsoft Money successor. Each phase should leave the app runnable, tested, and documented.

Current near-term implementation sequencing lives in
`docs/phase-1-implementation-plan.md`. This product requirements document keeps
the durable phase boundaries and product intent.

### Phase 0: Foundation

Goal: make the empty app safe to evolve.

- SQLite migration runner and schema version table.
- SQLite connection setup with deliberate per-connection PRAGMAs: foreign keys on, WAL, busy timeout, explicit synchronous mode, and WAL autocheckpoint.
- Automatic startup migrations before the HTTP server starts serving requests.
- Browser-based first-run owner setup with username and password before real financial data entry. This slice creates the owner only; later setup steps are added with books, accounts, and categories.
- Translation boundary with English messages and localization-ready built-in data.
- Versioned `/api/v1` route shape for real domain endpoints.
- Basic backup and restore documentation.
- OpenAPI and Bruno coverage for the first real endpoints.
- Phase 0 does not add books, commodities, institutions, accounts, opening balances, or system-account seeding. It leaves those setup steps visible but non-blocking until their domain slices exist.

### Phase 1: Books, Commodities, And Accounts

Goal: create the durable accounting skeleton.

- Single owner book. The `books` table remains a future extension point, but current runtime creates and uses only book `1` with no book selector.
- Commodity/currency table with exact decimal scale metadata.
- First-run setup extension for default book, default currency preference,
  required system accounts, and a starter cash account for the default currency.
- The default currency is a UI/account-default preference, not a book base currency or reporting currency. Reports choose their reporting currency and FX method later without changing the book.
- Active currencies are derived from active accounts that use a currency. Creating
  or editing an account can introduce another embedded-catalog currency; that
  currency becomes active when an active account uses it.
- Commodity and currency setup comes before accounts. Account precision must derive from commodity metadata or an explicit account-level quantity scale override, not from floating-point or free-text currency fields.
- Institution logo and backdrop support is limited to optional URL/reference metadata in Phase 1. File upload, attachment storage, statement PDFs, and receipts remain out of scope.
- Account tree with fixed accounting classes and catalog-backed account kinds.
- Account creation does not create balances. Opening balances require posted transactions and arrive with the ledger transaction slice.
- Account list and account detail UI.

### Phase 2: Ledger Transactions

Goal: make daily transaction entry useful.

- Transactions with postings.
- Transfers as ordinary balanced transactions.
- Opening balances through explicit equity/opening-balance transactions.
- Friendly category UI mapped to income/expense accounts.
- First-run setup extension for choosing default categories and optional additional categories.
- Transaction create, edit, void/archive, and list flows.
- Transaction list uses cursor-based pagination and supports server-side FTS5 search.
- Backend balancing tests and API smoke tests.

### Phase 3: Reconciliation And Core Reports

Goal: make records trustworthy over time.

- Account reconciliation workflow.
- Reconciliation-safe edit/correction behavior.
- Net worth, cashflow, and spending reports.
- Export of core ledger data.

### Phase 4: Import And Cleanup

Goal: reduce manual entry without sacrificing trust.

- CSV import preview and commit.
- Duplicate detection.
- Source metadata retention.
- Import rollback or cleanup workflow.
- Payee/category cleanup helpers.

### Phase 5: Planning

Goal: support forward-looking personal finance.

- Budgets.
- Account budget treatment as a separate account-facing planning axis, not an
  account kind.
- Scheduled transactions.
- Projected balances.
- Simple loan/liability helpers if they fit the existing ledger model.

### Phase 6: Advanced Finance

Goal: add power-user workflows after the core ledger is stable.

- Multi-currency reporting.
- Price history with exact manual, provider, FX, and trade-implied observations
  grouped into source/quote-type/adjustment-basis series. Price observations
  preserve valuation date separately from recorded-at time so historical reports
  can be reproduced with the price knowledge available at report time.
  Corrections supersede or void prior observations rather than mutating them in
  place.
- FX coverage is demand-driven and restart-safe. Activating a currency or durably
  entering an older transaction, including a manual draft, extends required
  provider history from the earliest needed date through today. Import previews do
  not trigger downloads until selected rows are committed as transactions. Manual,
  daily, and domain-triggered refreshes use durable background work that resumes
  after network loss or an app restart.
- Investment accounts for stocks and ETFs using the existing commodity,
  account, and ledger model.
- Security identity and provider matching for listed instruments, including
  symbol, exchange/MIC, identifiers, issuer, quote/trading commodity, provider
  metadata, and effective-dated version history.
- Lots and cost-basis foundations from the first investment slice. FIFO is the
  first implemented default, but durable policy values include FIFO, LIFO,
  average cost, and specific lot.
- Dividend and reinvested-dividend workflows with optional per-security income
  and withholding defaults.
- Provider events and reviewable suggestions for dividends, distributions,
  splits, mergers, spin-offs, ticker changes, delistings, cash-in-lieu, return
  of capital, and other corporate actions.
- Explicit automation rules are required before trusted provider events may
  auto-post; otherwise provider data remains a suggestion.
- Market-data extensibility starts with built-in adapters for trusted sources,
  then declarative HTTP adapters for simple source mappings, and only later
  external process plugins for complex/community providers.
- Realized gain/loss reporting.
- Report snapshots where reproducibility matters.

### Later

- Multi-user households.
- Public deployment hardening beyond the owner-user model.
- Bank integrations.
- Mobile apps.
- Plugin architecture.
- Small-business AR/AP and invoicing.

Later phases should remain optional. The earlier phases must still produce a complete self-hosted personal finance app.

## UX And Design Requirements To Lock Early

These should be fixed before many screens accumulate inconsistent patterns.

- Localization strategy: message catalogs, locale selection, fallback behavior, target languages, and number or date formatting boundaries.
- Built-in data localization strategy: stable keys for seeded categories, account classes, account kinds, currencies, commodities, system accounts, and other app-defined labels.
- Theme strategy: light and dark themes first, semantic token naming, persisted preference, and chart color policy.
- Responsive layout rules: breakpoints, dense-table behavior, sidebar or drawer behavior, and mobile transaction entry expectations.
- Form conventions: dirty-state handling, autosave policy, cancel behavior, optimistic updates, and server-validation display.
- Error conventions: friendly message formatting, request identifiers, retry guidance, and conflict handling.
- Empty-state conventions: what every major screen should explain when there is no data yet.
- Audit visibility: where users can see who changed what and why.
- Reconciliation UX: lock semantics, mismatch resolution, and corrective-entry paths.
- Import UX: preview, validation, duplicate review, commit, and rollback behavior.
- Report UX: filters, saved views, export, print, and reproducibility rules.

## Additional Areas To Predefine Early

Beyond multilingual support and themes, these areas should be deliberately defined before the app grows:

- Authentication scope: single owner only versus future household users, and MFA timing for public real-data deployments.
- Authorization model: whether multi-user language should be avoided or prepared in table and API naming.
- Audit model: mutation attribution fields, change reasons, and audit visibility.
- Data lifecycle rules: void, archive, correct, restore, and retention semantics.
- Import priorities: which file formats arrive first after CSV.
- Export guarantees: minimum export formats and whether exports should include settings, reports, and metadata.
- API contract workflow: exact OpenAPI type generation command, frontend generated-client path, and CI check behavior.
- Money representation limits: maximum scale and commodity code validation.
- Backup policy: backup location, restore expectations, and Docker volume guidance.
- SQLite backup implementation: online backup API first, `VACUUM INTO` where useful, and no raw live file-copy guidance as the normal path.
- Database encryption policy: encryption is deferred, but risks and future requirements must be documented.
- Attachment policy: whether statement files or receipts are in scope at all.
- Notification policy: whether the app will ever send email, and what remains strictly local-only.
- Public deployment stance: reverse proxy assumptions, HTTPS requirements, and operator warnings for internet exposure.
- Performance targets: acceptable latency for register screens, imports, reports, and startup.
- Privacy stance: no telemetry by default, and explicit rules for any future optional external integrations.

## Locked Scope Decisions For Early Releases

- Export must support CSV export of core ledger data and QIF export.
- SQLite backup remains an operational safeguard, but it does not replace user-facing export formats.
- Attachments are out of scope until core ledger, reconciliation, import, and reporting workflows are stable.
- Mobile support must cover full core workflows responsively, including transaction entry rather than read-only access.
- User and permission language may stay explicitly single-user for now; future household support, if ever adopted, should be introduced as a deliberate product change rather than implied prematurely.

## Conventions Carried Forward From Archive Analysis

- `.archive/` ideas are useful input but not source of truth.
- Keep a single active book in the UI for now; do not introduce a visible multi-book UX prematurely.
- Keep runtime UX explicitly single-owner and single-user for now.
- Use browser-based setup for initial local authentication. Implement owner creation first, then extend the setup workflow with default book, currency, system account, and category choices when those domains exist.
- Keep app-defined database labels localization-ready; do not seed English-only category, currency, commodity, or system labels as the only source of truth.
- Prefer void or corrective workflows over destructive deletion.
- Avoid desktop-only concepts such as native file pickers, session undo stacks, or local database-path selection UX.
- Keep the web deployment model same-origin and self-contained.

## Open Product Decisions

These are important and should be resolved before the related feature slices begin.

- Which export scope should be available from the first export milestone beyond core ledger CSV and QIF, such as full structured JSON export of settings and metadata?
- Which first-class UI languages should ship after the English translation boundary is in place?
- When attachments eventually enter scope, should they live in SQLite, the filesystem, or pluggable object storage?
- Which mobile-heavy workflows deserve dedicated optimization first after the baseline responsive experience ships?
