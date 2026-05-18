# Phase 3 Plan

Phase 3 focuses on frontend parity and release confidence against the
one-container SQLite stack.

## Current Test Shape

- Unit/component tests with `npm test`.
- Svelte checks with `npm run check`.
- Playwright against `docker compose up -d --build --wait`.
- API tests and migration smoke against real temporary SQLite databases.

## E2E Reset Strategy

The Playwright harness snapshots the migrated SQLite database inside the app
volume, stops the app between specs, restores the baseline file, restarts the
app, and polls `http://localhost:16888/api/v1/health`.

## Remaining Work

- Keep broadening E2E coverage for imports, reconciliation, reports, planning,
  investments, auth, and settings.
- Add visual and accessibility checks where they catch real regressions.
- Keep deployment smoke aligned with `compose.yaml`.
