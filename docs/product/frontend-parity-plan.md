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
- Added `src/lib/book-context.ts` for active-book initialization that now
  prefers book `1`, falls back to the preferred/default readable book when
  needed, and still preserves the underlying persisted-book mechanism for a
  later multi-book rollout.
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
- Refactored `CommoditySettings.svelte` into focused currency, commodity,
  FX daily, FX official, and FX settings sections; adopted active-book/error
  handling in the touched flows; and added Vitest coverage for the route and
  manual FX refresh client behavior.
- Refactored `investments/+page.svelte` to use typed investment clients,
  focused trade/dividend form components, explicit active-book/error handling,
  and Vitest coverage for the route, form payload builders, and API seam.
- Refactored `reports/+page.svelte` onto typed report contracts and
  active-book/error handling; added net worth, account trends, performance,
  account valuation, currency exposure, and saved definitions/runs surfaces;
  added CSV export and print controls for active report output; and added
  Vitest coverage for the route and reports API seam.
- Removed the visible frontend book selector for the temporary v1 single-book
  UX; live routes now resolve the active book through `bookContext`; remaining
  frontend API helpers no longer default `bookId` to `1`; and the Python API
  now accepts explicit `book_id` query parameters on accounts/metadata list
  endpoints while returning HTTP 501 with a clear message for non-`1` book
  requests.
- Added Vitest coverage for API error parsing, book context, password reset,
  and invite acceptance.

## Deferred Findings

- Many mutating flows still lack consistent confirmation/success/error states.
  Recommended fix: adopt a shared action-state helper and standardized
  destructive confirmation component when each page is refactored.

## Completion Criteria

- No loose generic API clients for backend-owned schemas.
- No route-level `book_id: 1` except documented test fixtures or temporary
  migration-only code outside the web runtime.
- No `String(e)` or raw `error.message` user-facing API errors in touched code.
- Each new backend workflow has a browser path and at least one Vitest or
  Playwright regression test.
