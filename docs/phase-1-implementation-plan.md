# Phase 1 Implementation Plan (historical — COMPLETE)

Status: **historical.** Phase 1 (books, commodities, accounts, institutions,
account/institution UI) is complete. This document is kept for the design
rationale only; it is no longer the active tracker.

- Active "what's next": `docs/roadmap.md`.
- Active "what's done": `docs/implemented.md`.
- Product source of truth: `docs/product-requirements.md`.

## Summary

Phase 1 creates the durable accounting skeleton: one runtime book, exact
commodity/currency metadata, institutions, accounts, required system accounts,
starter cash account for the default setup currency, tags, and
account/institution UI. Account creation never creates balances. Opening
balances require posted transactions.

The backend is ahead of the original Phase 1 boundary: transaction,
reconciliation, payee, category, and ledger-summary foundations already exist in
the consolidated initial schema and services. Those foundations do not change
the Phase 1 acceptance gate, which remains account/institution skeleton plus UI
contract closeout.

## Current Status

Implemented backend foundation:

- Single runtime book guarded by `books.id CHECK (id = 1)`.
- Embedded currency catalog, currency setup, and book default-currency
  preference. This is not a book base/reporting currency.
- Commodity, institution, account, system-account, tag, category, payee,
  transaction, posting, ledger-summary, and reconciliation foundations.
- Current-state SQL views for commodity, institution, account, payee, and
  transaction versions.
- Hybrid audit model with `audit_events` referenced by setup, identity, version,
  lifecycle, transaction, and reconciliation rows.
- OpenAPI path/schema files and typed frontend API helpers for the current API
  surface.
- Account list route and supporting frontend components exist.

Remaining Phase 1 work:

- Verify OpenAPI and generated frontend types match the current backend surface.
- Finish or verify account management UI on desktop and mobile.
- Add or verify institution management UI on desktop and mobile.
- Keep all user-facing copy and built-in labels behind Paraglide/localization
  boundaries.
- Run full validation before declaring Phase 1 complete.

## Immediate Next Slice: UI Contract Closeout

This is the next implementation focus.

- Treat `api/openapi/openapi.yaml` and referenced files as the API source of
  truth.
- Compare the registered backend routes, OpenAPI paths, generated
  `frontend/src/lib/api/schema.d.ts`, and typed frontend API helpers.
- Fix drift only where the backend already implements the behavior or where an
  existing frontend helper points at a documented endpoint.
- Do not add new domain endpoints in this slice. Generic non-currency commodity
  CRUD, account-tree-specific endpoints, account closing-validation endpoints,
  and opening-balance endpoints are not required for Phase 1 closeout unless a
  separate product decision promotes them.
- Regenerate frontend OpenAPI types if the checked OpenAPI artifact changes.

## Next Product Slice: Account And Institution UI

After contract closeout, finish the user-facing Phase 1 skeleton.

- Account UI must support listing, filtering/grouping, creation, editing,
  close/reopen, archive/restore, and clear empty/loading/error/success states.
- Institution UI must support listing, creation, editing, archive/restore, and
  clear empty/loading/error/success states.
- UI labels for account classes, account kinds, system roles, statuses,
  institution kinds, currencies, and fields must resolve through localization
  boundaries rather than English-only constants in route code.
- Mobile layouts must remain usable for the core management workflows.
- No book selector or additional-book creation flow should be added.

## Rules To Preserve

- Default currency is a preference for setup/account defaults, not a base,
  home, or reporting currency. Reports choose reporting currency and FX method
  later.
- Income, expense, equity, and system accounts must not assume a single book
  currency and may receive postings in multiple commodities unless a later
  account policy explicitly restricts them.
- Account precision derives from commodity metadata or an explicit account
  quantity-scale override.
- Account and institution current state should use current-state views and
  repository helpers rather than handler-local latest-version joins.
- Multiple versions on the same `effective_from` are allowed for correction or
  reseeding; the greatest `version_seq` is canonical for current/as-of queries.
- Version/lifecycle tables explain what changed; `audit_events` explains who,
  when, how, why, and under which request/session the operation happened.
- Account setup and account editing must not create balances. Opening balances
  require explicit posted transactions.

## Phase 1 Completion Gate

Phase 1 is complete when:

- Fresh setup can create owner, book, default currency preference, optional
  system accounts, a starter cash account for the default currency, and
  categories as configured by the setup gate.
- Institutions can be managed through the UI.
- Accounts can be managed through the UI as an append-only versioned tree.
- Account creation creates no balances and exposes no opening-balance shortcut.
- Current runtime uses only book `1`; no book selector or extra-book creation UI
  exists.
- OpenAPI, generated frontend types, backend routes, and frontend API helpers
  agree.
- Backend validation, frontend validation, and integrated build pass.
- Manual smoke checks cover setup, account management, and institution
  management on desktop and mobile viewport sizes.

Validation commands before declaring completion:

```sh
./scripts/test-backend.sh
./scripts/test-frontend.sh
pnpm build
git diff --check
```

## Reference Documents

- `docs/product-requirements.md`: high-level product requirements and phase
  boundaries.
- `docs/accounts-system-design.md`: account/institution design reference.
- `docs/account-hierarchy.md`: account taxonomy and hierarchy guidance.
- `docs/categories-design.md`: category-as-account design reference.
- `docs/transaction-ledger-core-plan.md`: ledger transaction reference for the
  later transaction UI/reporting work; not the immediate Phase 1 focus.
