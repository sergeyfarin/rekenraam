# Frontend Parity & Refactor Tracker

Last updated: 2026-05-18

This is the live tracker for the aggressive frontend parity/refactor pass.
Backend `/api/v1` remains the source of truth; items below record what the
frontend now supports, what was intentionally deferred, and what must be fixed
when touching the listed area.

## Shipped 2026-05-18

- Added a typed API error boundary in `src/lib/api/client.ts`.
  `ApiError` now carries `status`, `detail`, FastAPI validation errors,
  request id, and raw response text. `formatApiError` standardizes common
  401/403/409/422/5xx/frontend-network messages.
- Added `src/lib/book-context.ts` for active-book initialization from user
  preferences, fallback to the first readable book, and persisted book
  switching.
- Added browser flows for self-service password reset and invite acceptance:
  `/reset-password` and `/accept-invite`.
- Added auth API clients for password reset, invite acceptance, profile update,
  and password change.
- Added admin invite client support and surfaced invite issuance in
  `UserSettings.svelte`.
- Removed the `window.prompt` password-reset flow in `UserSettings.svelte`;
  password resets now use visible inline form state.
- Added profile display-name and password-change controls to preferences.
- Started active-book adoption in planning/import-export flows and replaced
  touched `String(e)`/raw error displays with `formatApiError`.
- Added loan payment draft/post clients and a basic loan payment assistant in
  the planning page.
- Added Vitest coverage for API error parsing, book context, password reset,
  and invite acceptance.

## Deferred Findings

- `CommoditySettings.svelte` remains too large and should be split before more
  pricing work. Risk: hard to test and easy to regress manual FX/market-price
  flows. Recommended split: currency list, FX daily, FX official, market
  prices, pricing policy, refresh health/history.
- `investments/+page.svelte` still uses generic investment API clients and
  page-local payload shaping. Risk: backend contract drift is not caught by
  TypeScript. Recommended fix: typed instruments/profiles/corporate-actions/
  trade clients plus focused form components.
- `reports/+page.svelte` still exposes only part of the backend reports
  surface. Recommended fix: typed report request/response contracts, saved
  definitions/runs UI, net worth/account trends/performance/valuation/exposure
  tabs, and report CSV/print controls.
- Hard-coded fallback `book_id: 1` still exists in pages and API helpers not
  touched in this pass. Risk: multi-book users can write/read the wrong book.
  Recommended fix: migrate route-by-route to `bookContext` and make client
  functions require explicit book ids where possible.
- Many mutating flows still lack consistent confirmation/success/error states.
  Recommended fix: adopt a shared action-state helper and standardized
  destructive confirmation component when each page is refactored.

## Completion Criteria

- No loose generic API clients for backend-owned schemas.
- No route-level `book_id: 1` except documented test fixtures or fallback
  bootstrap paths.
- No `String(e)` or raw `error.message` user-facing API errors in touched code.
- Each new backend workflow has a browser path and at least one Vitest or
  Playwright regression test.
