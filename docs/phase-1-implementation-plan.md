# Phase 1 Implementation Plan

Status: active Phase 1 status tracker. Early backend slices have landed in the
consolidated initial schema and services; this document now records what is
done, what remains, and the constraints that must not drift.

## Phase 1 Goal

Create the durable accounting skeleton:

1. Single owner book.
2. Commodity/currency model with exact precision metadata.
3. Institutions.
4. Accounts and account tree.
5. System account seeding.
6. Tags for transaction/posting context.
7. Account/institution UI.

Account setup must not create balances. Opening balances require posted
transactions. Transaction and reconciliation work has started in code, but it is
tracked separately from the Phase 1 accounting-skeleton acceptance criteria.

## Current Status

Implemented backend foundation:

- Single runtime book guarded by `books.id CHECK (id = 1)`.
- Embedded currency catalog plus currency/default-currency setup.
- Commodity, institution, account, payee, transaction, posting, tag, category,
  and reconciliation schema in the consolidated initial migration.
- Current-state SQL views for commodity, institution, account, payee, and
  transaction versions.
- Hybrid audit model with `audit_events` rows referenced by setup, identity,
  version, lifecycle, transaction, and reconciliation rows.
- Backend APIs and tests for books, currencies, institutions, accounts, system
  accounts, tags, categories, payees, transactions, ledger summaries, and
  reconciliation.

Remaining Phase 1/product cleanup:

- Confirm OpenAPI and generated frontend API types match the current backend
  surface.
- Finish or verify the account and institution UI on desktop and mobile.
- Keep user-facing copy and built-in labels behind Paraglide/localization
  boundaries.
- Run full backend, frontend, and integrated build validation before declaring
  Phase 1 complete.

## Audit And Current-State Rules

- Domain version/lifecycle tables remain the source of truth for state history.
- `audit_events` explains the initiating operation: actor, session/request,
  origin, operation code, reason, and grouped workflow metadata.
- Create one audit event inside the same database transaction as the domain
  changes it explains.
- Repository current-state helpers should query current-state views instead of
  repeating latest-version logic in handlers.
- Effective-dated current views choose the greatest `(effective_from,
  version_seq)` where `effective_from <= today`.
- Multiple versions on the same `effective_from` are allowed for correction or
  reseeding; the greatest `version_seq` is canonical for current/as-of queries.

## Slice 1: Book Setup

Status: implemented.

Backend schema:

- Add `books` table with `id`, `owner_user_id`, `code`, `name`,
  `default_currency_commodity_id`, `created_at`, and `updated_at`.
- Keep the `books` table as the future extension point, but current runtime has
  exactly one book with `id = 1`.
- Do not add book selection UI or any flow for creating another book.

Backend behavior:

- Add `POST /api/v1/setup/book`.
- Create exactly one book for the owner.
- Mark `setup_steps.book` complete.
- Require authenticated owner session.
- Add `GET /api/v1/books/current`.

Frontend:

- Extend setup gate to show book setup after owner setup and sign-in.
- Book form fields: book name and optional internal code defaulting to
  `personal`.

Tests:

- Creating first book succeeds.
- Creating second book returns conflict.
- The created book always has `id = 1`.
- Unauthenticated book setup is rejected.
- Setup status advances from `book` to `currencies`.

## Slice 2: Commodities And Currencies

Status: implemented for currencies/default-currency setup and commodity
identity/version storage. Generic non-currency commodity UX/API can remain a
later expansion unless a current product flow needs it.

This must come before accounts.

Backend schema:

- Add `commodities` identity table.
- Add append-only `commodity_versions` table.
- Validate `standard_scale`, `max_quantity_scale`, and version ordering.
- Reject `UPDATE` and `DELETE` on commodity versions.

Planned generic commodity APIs:

- `GET /api/v1/commodities`
- `POST /api/v1/commodities`
- `GET /api/v1/commodities/{commodity_id}`
- `PATCH /api/v1/commodities/{commodity_id}`
- `GET /api/v1/commodities/{commodity_id}/versions`
- `GET /api/v1/currencies`
- `POST /api/v1/currencies`
- `POST /api/v1/currencies/{commodity_id}/default`

Implemented setup behavior:

- Currency setup selects a default currency preference from the embedded
  currency catalog and may add optional additional currencies.
- Mark `setup_steps.currencies` complete.
- Update `books.default_currency_commodity_id`.
- The default currency is not a base/reporting currency. Reports choose their
  reporting currency and FX method later without changing the book.

## Slice 3: Institutions

Status: implemented on the backend.

Backend schema:

- Add `institutions` identity table.
- Add append-only `institution_versions` table.
- Use stable institution kind codes.
- Reject `UPDATE` and `DELETE` on institution versions.

Backend APIs:

- `GET /api/v1/institutions`
- `POST /api/v1/institutions`
- `GET /api/v1/institutions/{institution_id}`
- `PATCH /api/v1/institutions/{institution_id}`
- `POST /api/v1/institutions/{institution_id}/archive`
- `POST /api/v1/institutions/{institution_id}/restore`
- `GET /api/v1/institutions/{institution_id}/versions`

## Slice 4: Accounts

Status: implemented on the backend.

Backend schema:

- Add `accounts` identity table.
- Add append-only `account_versions` table.
- Store nullable `system_role` on identity rows. A non-null role marks the
  account as system-owned.
- Do not store balances, `normal_balance`, `display_order`, or `archived_on`.
- Store account validity dates as `opened_on` and `closed_on`; keep
  `effective_from` for account-version/as-of history.

Backend APIs:

- `GET /api/v1/accounts`
- `POST /api/v1/accounts`
- `GET /api/v1/accounts/{account_id}`
- `PATCH /api/v1/accounts/{account_id}`
- `POST /api/v1/accounts/{account_id}/close`
- `POST /api/v1/accounts/{account_id}/reopen`
- `POST /api/v1/accounts/{account_id}/archive`
- `POST /api/v1/accounts/{account_id}/restore`
- `GET /api/v1/accounts/{account_id}/versions`

Tree rules:

- Parent must be same book.
- Active account cannot be its own ancestor.
- Use Go service plus recursive CTE in the same write transaction.
- Active account cannot be parented under archived account.
- Phase 1 default: enforce same `account_class` under parent.

## Slice 5: System Account Seeding

Status: implemented on the backend.

Required roles:

- `opening_balance`
- `import_imbalance`
- `retained_earnings`
- `unassigned_income`
- `unassigned_expense`

Behavior:

- Add `POST /api/v1/setup/system-accounts`.
- Create required system account identities and versions.
- Mark `setup_steps.system_accounts` complete.
- Make seeding idempotent.
- Do not add opening-balance transaction UI or API in Phase 1.
- System accounts must not assume a single book base currency. Income, expense,
  equity, and system accounts should be able to receive postings in multiple
  commodities unless a later account policy explicitly restricts them.

## Slice 6: Tags

Status: implemented on the backend. Transaction/posting tag junctions now exist
with transaction/posting schema.

Backend schema:

- Add book-scoped `tags` table.
- Store `name`, `kind`, optional `color`, optional `icon`, `status`, and
  `metadata_json`.
- Enforce active tag uniqueness per `(book_id, kind, name)`.
- Archive tags instead of hard deleting them.
Backend APIs:

- `GET /api/v1/tags`
- `POST /api/v1/tags`
- `GET /api/v1/tags/{tag_id}`
- `PATCH /api/v1/tags/{tag_id}`
- `POST /api/v1/tags/{tag_id}/archive`
- `POST /api/v1/tags/{tag_id}/restore`

## Slice 7: Frontend Account/Institution Experience

Status: remaining Phase 1 acceptance item unless verified complete in the
current frontend.

Suggested routes:

- `/app/settings/institutions`
- `/app/accounts`
- `/app/accounts/:id`

UI requirements:

- Account tree grouped by accounting class.
- Institution create/edit/archive/restore.
- Account create/edit/close/archive/restore.
- Loading, empty, validation, conflict, archived, closed, and success states.
- Stable localized labels for account classes, account kinds, institution kinds,
  system roles, statuses, and fields.

## OpenAPI And Client Work

For every backend API slice:

- Update `api/openapi/openapi.yaml` first.
- Regenerate `frontend/src/lib/api/schema.d.ts`.
- Add typed API helpers under `frontend/src/lib/api/`.
- Avoid English fallbacks in API helper files.

## Validation Plan

After each backend slice:

```sh
./scripts/test-backend.sh
```

After each frontend slice:

```sh
./scripts/test-frontend.sh
```

Before considering Phase 1 complete:

```sh
pnpm build
```

Also verify:

- Fresh setup can create owner, book, default currency preference, optional
  additional currencies, and system accounts.
- Categories are implemented as a later setup step; keep them non-blocking
  unless the setup gate explicitly requires them.
- Account creation never creates balances.
- Current runtime uses only book `1`; future multi-book support must be a
  deliberate extension rather than an accidental UI affordance.
- No floating-point monetary or quantity storage is introduced.

## Phase 1 Done Criteria

Phase 1 is complete when:

- Setup can create owner book.
- Default currency preference exists.
- Commodities/currencies are modeled with exact scale metadata.
- Institutions can be managed.
- Accounts can be managed as an append-only versioned tree.
- Required system accounts are seeded.
- Account and institution UI is usable on desktop and mobile.
- OpenAPI, generated frontend types, backend tests, frontend checks, and
  integrated build all pass.
- Opening balances remain explicitly deferred to Phase 2.
