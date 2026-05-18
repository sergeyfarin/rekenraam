# Phase 3 Plan

Phase 3 focuses on frontend parity and release confidence against the
one-container SQLite stack.

## Shipped Scope

Phase 3 is already complete. The short version is not enough on its own because
the detailed workstream history records what was extracted, tested, and pinned
for later parity work.

- Root-level Tauri cleanup shipped while keeping `src-tauri/` as parity lookup.
- Vitest + jsdom + `@testing-library/svelte` infra shipped.
- Pure helpers were extracted into `$lib/*` modules for money, dates,
    split-balance, fuzzy search, reconciliation state, saved views, and
    validators.
- Large routes were carved into testable components.
- Component tests shipped for login/bootstrap, reconciliation, split editor,
    planning forms, date/filter behavior, and session-expired handling.
- Playwright harness and e2e specs shipped for auth, transactions,
    reconciliation, cross-currency transfer, OFX import, and reports.
- README, testing docs, and gap-plan references were updated to reflect the
    shipped test pyramid.

## Why This Detail Still Matters

- The extracted helpers and carved components are the stable surfaces future
    parity/refactor work should extend rather than fold back into giant route
    files.
- Several behavior quirks are intentionally pinned by tests, especially around
    date parsing, amount parsing, fuzzy search semantics, and reconciliation
    state derivation.
- The browser-e2e coverage documents what the current sqlite-only stack is
    expected to keep working end-to-end.

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

## Detailed Historical Reference

The pre-cleanup detailed Phase 3 execution log remains useful as historical
reference for shipped workstreams, extracted modules, carved components, and
test acceptance notes. It should stay in git history even if this live file
remains shorter than the original execution document.
