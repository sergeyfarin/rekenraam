# Early Architecture Decisions

Rekenraam is a personal, self-hosted finance app. The product should grow feature by feature, but a few decisions need to be explicit early so later work does not paint the app into a corner.

This document is intentionally lightweight. It records durable constraints and design defaults, not a full specification.

## Product Defaults

- Self-hosted first: the app must work as a single deployable service owned by the user.
- Private by default: financial data stays in the user's deployment unless a future feature explicitly exports it.
- Local-first operations: core budgeting, account, transaction, and reporting flows must not depend on third-party services.
- Incremental delivery: each feature should be useful on its own and should leave tests and migrations in a shippable state.
- English first, translation-ready: build initial UI copy in English, but avoid hard-coded text patterns that block localization later. Including any definitions in sql database

## Decisions To Keep

### Runtime Shape

Use the existing production shape: a Go backend serving a statically built SvelteKit frontend from one binary, with Docker packaging as a deployment convenience.

Reason: this is simple to self-host, easy to back up, and avoids splitting the app into multiple runtime services before the product needs that complexity.

### Database

Use SQLite as the primary database for the self-hosted app.

Rules:

- Every schema change must be represented by a migration.
- Tests should use temporary SQLite databases.
- Runtime database files belong in a persistent data directory, not inside the binary or container image.
- Avoid database features that would make a future PostgreSQL option unnecessarily painful, but do not design every query around a hypothetical migration.

### Money Representation

Never store money as floating point.

Store monetary values as integers in the smallest practical unit for the currency, together with the currency code. For most currencies this is minor units such as cents, but the data model should not assume that every currency has exactly two decimal places. Some tranactions in some accounts shall allow higher precision than default for the currency, e.g. investment accounts, fractional shares, etc., but do not make it more complicated than necessary.

Recommended early fields:

- `amount_minor`: signed integer amount.
- `currency_code`: ISO 4217 currency code such as `EUR` or `USD`.
- `occurred_on`: date for financial reporting.
- `created_at` and `updated_at`: timestamps for system history.

### Date And Time Semantics

Use dates and time (where available) for financial facts and timestamps for system facts.

- Transaction dates and local time, statement dates, and budget periods are calendar dates.
- Audit, import, sync, and edit events are timestamps.
- Store timestamps in UTC.
- Let display formatting be locale-aware in the frontend.

### Localization

Prepare for translation before the first real screens become large.

Rules:

- Route all user-facing UI copy through a translation boundary once real feature UI starts.
- Do not concatenate translated fragments to build sentences.
- Keep formatting of dates, numbers, percentages, and money separate from translated messages.
- Store stable codes in data, not translated labels.

Initial implementation may use English message files only. The architecture should still make adding another locale a mechanical task.

### API Boundaries

Keep `/api` as the backend API namespace and document public request and response shapes in OpenAPI as endpoints become real.

Rules:

- API DTOs belong near HTTP handlers.
- Business rules belong in application services, not directly inside handlers.
- Database access belongs behind repository-style functions or methods.
- Bruno requests should cover important API workflows as they appear.

### Data Ownership And Portability

The user must be able to leave with their data.

Plan early for:

- Database backup instructions.
- Export of transactions, accounts, categories, and budgets.
- Import workflows that preserve original source data where possible.
- Clear migration behavior during upgrades.

Do not require telemetry, cloud accounts, or external identity providers for core functionality.

### Authentication And Access

Assume a self-hosted personal app, but do not assume the network is trusted.

Early default:

- Support a single owner/user model first.
- Put all privileged operations behind authentication before real financial data is entered.
- Keep auth local to the deployment unless a future decision introduces external identity providers.

Multi-user households, sharing, and permissions can be added later, but the data model should avoid names that imply only one global user forever.

### Auditability

Financial apps need explainable changes.

Early behavior can be simple, but important records should be designed so future audit history is possible:

- Prefer correcting transactions with edits that update timestamps rather than silent destructive rewrites.
- Preserve import source metadata for imported transactions.
- Consider soft deletion or event history for records that affect reports.

### Testing Strategy

Keep the current layered test shape:

- Go tests for backend services, repositories, migrations, and API handlers.
- Svelte checks and component-level tests for frontend logic when added.
- Bruno for API examples and contract smoke tests.
- Playwright for critical user workflows.

When a feature changes financial calculations, include table-driven backend tests with named cases.

## Decisions To Make Before Specific Features

- Before transactions: account model, category model, transfer representation, recurring transaction approach, and import-source metadata.
- Before budgets: budget period semantics, category rollover rules, and whether budgets are per account, per household, or global.
- Before reports: cashflow basis, date range inclusivity, currency conversion policy, and whether multi-currency totals are allowed.
- Before file import: supported formats, duplicate detection, source retention, and reconciliation workflow.
- Before backups: backup location, restore UX, encryption expectations, and Docker volume guidance.
- Before multi-user support: owner model, roles, invitation flow, and whether per-user audit history is required.

## Deferred Until There Is Pressure

- PostgreSQL support.
- Cloud sync.
- Mobile apps.
- Bank integrations.
- Complex household permissions.
- Investment tracking.
- Multi-currency conversion rates.
- Plugin architecture.

These may become valuable later, but they should not complicate the first usable personal finance workflows.
