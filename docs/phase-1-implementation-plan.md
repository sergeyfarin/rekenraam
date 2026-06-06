# Phase 1 Implementation Plan

Status: implementation plan for the Phase 1 accounting skeleton.

## Phase 1 Goal

Create the durable accounting skeleton:

1. Single owner book.
2. Commodity/currency model with exact precision metadata.
3. Institutions.
4. Accounts and account tree.
5. System account seeding.
6. Account/institution UI.

No transaction posting yet. No opening balances yet. No balances created by
account setup.

## Slice 1: Book Setup

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

This must come before accounts.

Backend schema:

- Add `commodities` identity table.
- Add append-only `commodity_versions` table.
- Validate `standard_scale`, `max_quantity_scale`, and version ordering.
- Reject `UPDATE` and `DELETE` on commodity versions.

Backend APIs:

- `GET /api/v1/commodities`
- `POST /api/v1/commodities`
- `GET /api/v1/commodities/{commodity_id}`
- `PATCH /api/v1/commodities/{commodity_id}`
- `GET /api/v1/commodities/{commodity_id}/versions`
- `GET /api/v1/currencies`
- `POST /api/v1/currencies`
- `POST /api/v1/currencies/{commodity_id}/default`

Setup behavior:

- Currency setup selects a default currency preference from the embedded
  currency catalog and may add optional additional currencies.
- Mark `setup_steps.currencies` complete.
- Update `books.default_currency_commodity_id`.
- The default currency is not a base/reporting currency. Reports choose their
  reporting currency and FX method later without changing the book.

## Slice 3: Institutions

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

Backend schema:

- Add `accounts` identity table.
- Add append-only `account_versions` table.
- Store nullable `system_role` on identity rows. A non-null role marks the
  account as system-owned.
- Do not store balances, `normal_balance`, `display_order`, `closed_on`, or
  `archived_on`.

Backend APIs:

- `GET /api/v1/accounts`
- `GET /api/v1/accounts/tree`
- `POST /api/v1/accounts`
- `GET /api/v1/accounts/{account_id}`
- `PATCH /api/v1/accounts/{account_id}`
- `POST /api/v1/accounts/{account_id}/close`
- `POST /api/v1/accounts/{account_id}/reopen`
- `POST /api/v1/accounts/{account_id}/archive`
- `POST /api/v1/accounts/{account_id}/restore`
- `GET /api/v1/accounts/{account_id}/versions`
- `GET /api/v1/accounts/{account_id}/closing-validation`

Tree rules:

- Parent must be same book.
- Active account cannot be its own ancestor.
- Use Go service plus recursive CTE in the same write transaction.
- Active account cannot be parented under archived account.
- Phase 1 default: enforce same `account_class` under parent.

## Slice 5: System Account Seeding

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

## Slice 6: Frontend Account/Institution Experience

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
- Future categories step remains pending but non-blocking if not implemented.
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
