# Early Architecture Decisions

Rekenraam is a personal, self-hosted finance app. The long-term ambition is a modern successor to tools such as GnuCash and Microsoft Money: friendly enough for daily personal finance, but built on accounting foundations that can survive imports, reconciliation, investments, multi-currency data, and long-lived records.

This document records active decisions for the current Go, SvelteKit, and SQLite app. Archived documents can inform these decisions, but they are not source of truth and should not be copied forward without translating them to the current stack.

## Product Target

- Build personal finance first.
- Keep the data model strong enough for accounting-style correctness.
- Prefer steady feature slices over a large all-at-once rewrite.
- Optimize for self-hosted ownership, backup, restore, and export.
- Do not require cloud accounts, telemetry, bank integrations, or external identity providers for core use.

Small-business accounting, tax filing, hosted service operations, bank sync, and plugin systems may influence naming and boundaries, but they are not first-release goals.

## Runtime Shape

Use the existing production shape: a Go backend serving a statically built SvelteKit frontend from one binary. Docker should package the same app shape rather than introducing a second production architecture.

The frontend runtime is static SvelteKit output built with `@sveltejs/adapter-static` and embedded into the Go binary. Go owns all production server behavior: API routes, authentication, cookies, headers, persistence, logging, and static-file serving. The Go static handler serves real frontend assets directly, returns 404 for missing asset files, and falls back to the SvelteKit `index.html` shell for extensionless browser routes so deep links and refreshes work.

Do not introduce SvelteKit production server routes, server `load` functions, form actions, or hooks unless a later ADR changes the runtime shape. Frontend code should call Go endpoints under `/api/v1` through the typed API client. Unknown `/api/` routes must remain API 404s rather than falling through to the frontend shell.

Reasons:

- Simple to self-host.
- Easy to back up.
- One origin for frontend and API.
- Fewer moving parts while the product model is still forming.

## Database And Migrations

Use SQLite as the primary database.

Rules:

- Every schema change must be represented by a migration under `backend/migrations`.
- The app must track applied migrations in the database.
- Migrations should run transactionally where SQLite allows it.
- Tests should use temporary SQLite databases.
- Runtime database files belong in a persistent data directory, not inside the binary or container image.
- Install SQLite connection pragmas deliberately: foreign keys on, WAL mode for normal runtime use, a busy timeout, and an explicit synchronous setting.

Do not optimize the first schema around a hypothetical PostgreSQL migration. Avoid gratuitous SQLite-only tricks in ordinary queries, but use SQLite constraints and triggers when they protect financial invariants that should survive application bugs.

## Ledger Model

Use a double-entry-capable ledger from the first real transaction schema.

Rules:

- A book is the top-level accounting boundary.
- Support one user-owned book first, but keep `book_id` in core domain tables so multi-book support does not require a schema reset.
- Accounts form a tree.
- Account types should include at least asset, liability, equity, income, and expense.
- Categories are presented in the UI as friendly budgeting/reporting concepts, but they should map to income or expense accounts instead of becoming a separate transaction primitive.
- Transactions contain postings, also called splits, against accounts.
- Transfers are ordinary transactions with postings to two or more accounts.
- A posted transaction should balance by commodity/currency unless it is an explicit opening balance, equity adjustment, or other named system workflow.

This is more structure than the simplest personal-finance app needs, but it avoids a painful rewrite when split transactions, transfers, reconciliation, investment lots, or proper income/expense reports arrive.

## Money, Commodities, And Precision

Never store financial quantities as floating point.

Use exact decimal storage for money and security quantities. The default representation should be an integer value plus an explicit decimal scale, associated with a commodity.

Recommended early fields:

- `quantity_value`: signed integer.
- `quantity_scale`: decimal scale for the stored value.
- `commodity_code`: stable code such as `EUR`, `USD`, or a security symbol/internal code.

Examples:

- `12345` with scale `2` means `123.45`.
- `125000` with scale `6` means `0.125000`.

Rules:

- Currencies, securities, and other valued things are commodities.
- A commodity has a default display scale, but individual stored quantities may use a higher scale when needed.
- UI formatting decides how values are displayed; storage preserves exactness.
- Exchange rates and prices must also use exact decimal storage.

This keeps normal currency handling simple while leaving room for fractional shares, crypto-like assets, and high-precision calculated allocations.

## Date And Time Semantics

Use calendar dates for financial facts and timestamps for system facts.

- Transaction dates, statement dates, reconciliation dates, and budget periods are dates.
- Optional local transaction time can be recorded when the source provides it, but reporting must not depend on every transaction having a time of day.
- Audit, import, backup, sync, and edit events are timestamps.
- Store timestamps in UTC.
- Let frontend formatting be locale-aware.

## Localization

Prepare for translation before real screens become large.

Rules:

- Route user-facing UI copy through a translation boundary once feature UI starts.
- Initial message files may contain English only.
- Do not concatenate translated fragments to build sentences.
- Keep formatting of dates, numbers, percentages, and money separate from translated messages.
- Store stable codes in data, not translated labels.
- Do not store built-in definitions in the database only as English display text; store stable keys/codes and resolve display labels through localization.
- Seeded categories, account types, currencies, commodities, system accounts, and other app-defined labels must remain localization-ready.

## API Boundaries

Keep API behavior behind explicit HTTP endpoints and document public request and response shapes in OpenAPI as endpoints become real.

Rules:

- The scaffold `/api/hello` endpoint is disposable.
- Real domain endpoints should use a versioned prefix, starting with `/api/v1`.
- API DTOs belong near HTTP handlers.
- Business rules belong in application services, not directly inside handlers.
- Database access belongs behind repository-style functions or methods.
- Bruno requests should cover important API workflows as they appear.

## Authentication And Access

Assume a self-hosted personal app, but do not assume the network is trusted.

Early default:

- Support a single owner user first.
- Add local authentication before real financial data entry ships.
- Use browser-based first-run setup to create the owner username and password.
- Put all privileged operations behind authentication.
- Keep auth local to the deployment unless a future decision introduces external identity providers.
- Use same-origin browser sessions with `HttpOnly` cookies, hashed session tokens in SQLite, and CSRF protection on mutating API requests.
- Public deployments require HTTPS. Localhost development may use HTTP. LAN/private deployments should use HTTPS through either a reverse proxy or app-provided certificate and key configuration.

Multi-user households, sharing, and book permissions can be added later. Avoid names that imply there can only ever be one global user or one global book.

## Auditability And Correction

Financial apps need explainable changes.

Early rules:

- Do not hard-delete posted financial records.
- Allow ordinary edits while records are unreconciled and in an open period.
- Once a transaction is reconciled or belongs to a closed period, prefer corrective transactions over in-place mutation.
- Preserve import source metadata for imported transactions.
- Track `created_at` and `updated_at` on mutable records.
- Plan for actor, request, and reason fields before multi-user or public deployment features are added.

The first implementation can be modest, but table names and APIs should not assume destructive rewriting is the normal long-term behavior.

## Reconciliation And Periods

Reconciliation is a first-class financial workflow, not only a report filter.

Decisions:

- Accounts can be reconciled against statement balances.
- Reconciliation status belongs to postings or account-specific ledger state, not only to the transaction header.
- A reconciled posting should not be silently changed by a later edit.
- Period close is deferred, but the transaction schema should leave room for closed-period correction workflows.

## Import, Export, And Portability

The user must be able to leave with their data.

Plan early for:

- Database backup instructions.
- Full export of books, accounts, commodities, transactions, postings, categories, budgets, and settings.
- CSV import first unless a feature slice deliberately chooses another format.
- Preservation of original import source data where practical.
- Duplicate detection that can improve over time without changing the transaction schema.
- Clear migration behavior during upgrades.

## Backup And Restore

Backups are a product feature, not only an operator task.

Rules:

- Document where the SQLite database lives in single-binary and Docker deployments.
- Prefer SQLite online backup or a stopped-app backup over copying a live WAL database file.
- Provide a restore path before recommending the app for real financial records.
- Add backup smoke checks once backup tooling exists.
- Defer SQLite database encryption for early local use, but document the risk and revisit encrypted-at-rest storage before recommending higher-risk deployments.

## Testing Strategy

Keep the current layered test shape:

- Go tests for backend services, repositories, migrations, and API handlers.
- Svelte checks and component-level tests for frontend logic when added.
- Bruno for API examples and contract smoke tests.
- Playwright for critical user workflows.

When a feature changes ledger posting, balancing, reconciliation, import matching, or financial calculations, include table-driven backend tests with named cases.

## Decisions To Make Before Specific Features

- Before accounts: account type list, account tree rules, opening-balance behavior, and default book setup.
- Before commodities: currency metadata source, custom commodity codes, display scale, and precision limits.
- Before transactions: posting schema, balancing rules, transfer representation, split editing, correction behavior, and import-source metadata.
- Before reconciliation: statement model, lock semantics, undo/correction behavior, and balance tolerance rules.
- Before budgets: period semantics, category/account mapping, rollover rules, and whether budgets are book-wide or account-scoped.
- Before reports: cashflow basis, date range inclusivity, multi-currency totals, and whether report runs need snapshots.
- Before file import: supported formats, duplicate detection, source retention, preview/commit workflow, and rollback behavior.
- Before backups: backup location, restore UX, encryption expectations, Docker volume guidance, and smoke validation.
- Before multi-user support: owner model, roles, invitation flow, per-user audit history, and book permission rules.

## Deferred Until There Is Pressure

- PostgreSQL support.
- Cloud sync.
- Mobile apps.
- Bank integrations.
- Hosted service operations.
- Complex household permissions.
- Full small-business AR/AP and invoicing.
- Tax filing exports.
- Advanced investment tax-lot accounting.
- Plugin architecture.

These may become valuable later, but they should not complicate the first usable personal finance workflows.
