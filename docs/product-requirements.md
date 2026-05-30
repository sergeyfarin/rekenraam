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
- First-run setup is browser-based and creates the single owner with a username and password.
- Public VPS deployments require HTTPS and local authentication.
- Public VPS deployment with real financial data requires MFA; public deployment may be delayed until MFA is implemented.
- SQLite database encryption is deferred for early local use, but the product should document that encrypted-at-rest storage may be needed before recommending higher-risk deployments.
- The first non-negotiable report set is net worth, cashflow, and spending by category.
- The first mandatory export formats are CSV export of core ledger data and QIF export.
- Attachments such as statement PDFs and receipts are intentionally out of scope for now.
- The minimum mobile requirement is responsive support for full core workflows, including transaction entry.

## Decisions To Confirm Before Implementation

- Initial first-class UI language set beyond English.
- Password reset approach for a single-owner self-hosted app.
- Whether export scope should include full structured JSON in the first export milestone.
- Whether future attachments should use SQLite, filesystem storage, or pluggable object storage.

## Must-Have Cross-Cutting Requirements

These apply across all feature phases.

### Data Integrity

- The financial model must remain double-entry capable.
- Transactions with postings or splits are the canonical ledger primitive.
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
- Public VPS deployments must assume an untrusted network.
- All privileged operations must require authentication.
- Session, audit, and request attribution should be designed in from the first real user flows.

### Frontend Quality

- UI copy must go through a translation boundary.
- The app must support multilingual UI from an early stage.
- English is the initial implementation language; translation boundaries must keep UI copy and built-in data ready to add other languages without reworking UI code or database schema.
- Built-in database records not entered by the user or imported from a source must use stable codes or keys and resolve display labels through localization assets.
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

### Phase 0: Foundation

Goal: make the empty app safe to evolve.

- SQLite migration runner and schema version table.
- SQLite connection setup with deliberate pragmas.
- Browser-based first-run owner setup with username and password before real financial data entry.
- Translation boundary with English messages and localization-ready built-in data.
- Versioned `/api/v1` route shape for real domain endpoints.
- Basic backup and restore documentation.
- OpenAPI and Bruno coverage for the first real endpoints.

### Phase 1: Books, Commodities, And Accounts

Goal: create the durable accounting skeleton.

- Single owner book.
- Commodity/currency table with exact decimal scale metadata.
- Account tree with account types.
- Opening balances through explicit equity/opening-balance transactions.
- Account list and account detail UI.

### Phase 2: Ledger Transactions

Goal: make daily transaction entry useful.

- Transactions with postings/splits.
- Transfers as ordinary balanced transactions.
- Friendly category UI mapped to income/expense accounts.
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
- Scheduled transactions.
- Projected balances.
- Simple loan/liability helpers if they fit the existing ledger model.

### Phase 6: Advanced Finance

Goal: add power-user workflows after the core ledger is stable.

- Multi-currency reporting.
- Price history.
- Investment accounts.
- Lots and realized gain/loss reporting.
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
- Built-in data localization strategy: stable keys for seeded categories, account types, currencies, commodities, system accounts, and other app-defined labels.
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

- Authentication scope: single owner only versus future household users, MFA timing, and password-reset rules.
- Authorization model: whether multi-user language should be avoided or prepared in table and API naming.
- Audit model: mutation attribution fields, change reasons, and audit visibility.
- Data lifecycle rules: void, archive, correct, restore, and retention semantics.
- Import priorities: which file formats arrive first after CSV.
- Export guarantees: minimum export formats and whether exports should include settings, reports, and metadata.
- Backup policy: backup location, restore expectations, and Docker volume guidance.
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
- Use browser-based owner setup with username and password for initial local authentication.
- Keep app-defined database labels localization-ready; do not seed English-only category, currency, commodity, or system labels as the only source of truth.
- Prefer void or corrective workflows over destructive deletion.
- Avoid desktop-only concepts such as native file pickers, session undo stacks, or local database-path selection UX.
- Keep the web deployment model same-origin and self-contained.

## Open Product Decisions

These are important and should be resolved before the related feature slices begin.

- Which export scope should be available from the first export milestone beyond core ledger CSV and QIF, such as full structured JSON export of settings and metadata?
- Which first-class UI languages should ship after the English translation boundary is in place?
- What password-reset flow makes sense for a self-hosted single-owner deployment?
- When attachments eventually enter scope, should they live in SQLite, the filesystem, or pluggable object storage?
- Which mobile-heavy workflows deserve dedicated optimization first after the baseline responsive experience ships?
