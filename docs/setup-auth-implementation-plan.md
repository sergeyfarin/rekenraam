# Phase 0 Setup And Auth Completion Plan

Status: historical completed Phase 0 implementation plan. It is not the active
next-step tracker.

This document records the completed Phase 0 setup/auth baseline and the original
handoff constraints for the next domain slice. It is an implementation
checklist, not a replacement for accepted ADRs or product requirements.

Current note: Phase 0 is complete. Later slices have since implemented book,
currency, institution, account, system-account, category, and audit/current-state
foundations; see `docs/phase-1-implementation-plan.md` for current Phase 1
status.

## Locked Decisions

- Keep `setup_steps` as durable backend progress state.
- Do not use a UI setup table as the primary setup experience.
- Early forgotten-password recovery is operator-controlled, local, and backup-first.
- Cookie session plus CSRF stays same-origin and follows ADR 0002.
- Owner creation stays Argon2id-based and upgradeable.
- Phase 0 does not create books, commodities, institutions, accounts, opening balances, or system accounts.
- Phase 1 starts with setup for the single runtime book, then commodities/currencies, then institutions and accounts.

## Completed Baseline

Phase 0 foundation now includes:

- SQLite goose migrations with durable `setup_steps`.
- Required SQLite PRAGMAs installed through the modernc DSN and validated at startup.
- Browser first-run owner creation at `POST /api/v1/setup/owner`.
- Setup status at `GET /api/v1/setup/status` with derived install state.
- Owner login, logout, and session introspection.
- Same-origin session cookies, Origin validation, and CSRF validation for authenticated mutations.
- Login throttling that persists in SQLite.
- Operator-controlled owner recovery with backup-first behavior and session revocation.
- OpenAPI source for the first `/api/v1` endpoints and Bruno smoke requests.
- Frontend Paraglide message catalogs, generated OpenAPI types, theme tokens, setup/login gate, and authenticated shell.

## Setup State Contract

Goal: future setup steps stay visible without blocking the current app.

Current runtime derivation:

- `fresh`: no owner exists and the owner step is not marked complete.
- `configured`: owner exists and the owner step is marked complete.
- `recovery_required`: owner rows and owner-step completion disagree.

Notes:

- `owner` is the only currently implemented setup step.
- Future pending steps must not block the app until their APIs and UI exist.
- Keep seeded step keys in this order: `owner`, `book`, `currencies`, `system_accounts`, `categories`.
- Only `owner` belongs to Phase 0. Book, currency, system-account, and category completion belong to later domain slices.

## Validation Matrix

Backend:

- Setup status by install state.
- Owner bootstrap success and rejection paths.
- Login and logout behavior.
- Session revocation and CSRF enforcement.
- Recovery-command backup and reset behavior.

Frontend:

- Install gate state switching.
- Owner setup form states.
- Login form states.
- Shared translated API error rendering.

E2E:

- Fresh database bootstrap path.
- Existing owner login path.
- Recovery-required warning path.

Build:

- Frontend OpenAPI generation is covered by `pnpm --dir frontend run check` and `pnpm --dir frontend run build`.
- Integrated single-binary build is covered by `pnpm build`.

## Phase 1 Handoff

Historical handoff note: these items have since been implemented or superseded
by the current Phase 1 tracker. See `docs/phase-1-implementation-plan.md` before
starting new work.

1. Add the single owner book setup step. The `books` table remains the future extension point, but current runtime creates only book `1`.
2. Add commodity and currency identity/version tables before account tables.
3. Add institution and account management after commodities exist.
4. Seed required system accounts only after the account model exists.
5. Do not add opening-balance amounts to account creation. Opening balances require Phase 2 posted transactions.
6. Do not add book selection UI or a flow to create additional books until a later multi-book decision is accepted.
