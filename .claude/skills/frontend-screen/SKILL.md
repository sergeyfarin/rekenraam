---
name: frontend-screen
description: SvelteKit frontend rules for Rekenraam - static adapter constraints, Svelte 5 runes, Paraglide i18n, theme tokens, TanStack Query, required screen states, accessibility. Use when creating or modifying anything under frontend/src.
---

# Frontend Screen Work

## Hard constraints (violations break the production shape)

- Production is **static** SvelteKit output (`@sveltejs/adapter-static`) served
  by the Go binary. **Never add** `+page.server.ts`, `+layout.server.ts`,
  `+server.ts`, form actions, or production server hooks — that requires an ADR.
- All data access goes through `/api/v1` via the typed client
  (`openapi-fetch` pattern, types from `frontend/src/lib/api/schema.d.ts` —
  see `api-contract` skill).
- **TypeScript only** (`.ts`, `<script lang="ts">`). No `.js` under `frontend/src`.
- **Svelte 5 runes only** (`$state`, `$derived`, `$effect`, `$props`).
  Cross-component shared state = `$state` in `.svelte.ts` module files
  (example: `frontend/src/lib/theme.svelte.ts`). No Svelte 4 stores in new code.

## Structure

- Routes under `frontend/src/routes/app/<area>/` stay **thin entrypoints**;
  reusable workflow UI and helpers live in feature folders under
  `frontend/src/lib/<area>/` (examples: `lib/install-gate/`, `lib/reconcile/`,
  `lib/transactions/`).
- Build screens from the shared primitives (`PageHeader`, `Panel`,
  `StatePanel`, status badges — see `frontend/src/lib/components/`) before
  adding route-local styling.

## i18n (every user-visible string, no exceptions)

- Paraglide JS: `import { m } from '$lib/paraglide/messages.js';` then
  `m.some_message_id()`. Catalogs live in
  `frontend/messages/<domain>/<locale>.json`; keep message IDs flat, prefixed
  by domain/screen (`import_preview_dedupe_duplicate`).
- Never concatenate translated fragments into sentences; use message parameters.
- Formatting of money, dates, numbers is locale-aware (Dinero.js formatting
  layer, `Intl.DateTimeFormat`, `date-fns` v4) and separate from translation.
  Never use the `Date` constructor or luxon/moment for financial date logic.
- Built-in DB labels arrive as stable codes; resolve display text here, at
  render time.
- Regenerate outside dev/check/build with
  `pnpm --dir frontend run paraglide:compile`.

## Theming and design

- Semantic design tokens (CSS custom properties) are the source of truth;
  Tailwind utilities compose layout but never replace token roles. No one-off
  hard-coded colors/spacing in new UI.
- Light + dark must both work; critical financial states (positive, negative,
  warning, reconciled, locked) need a **non-color cue** as well.
- Tone: calm, trustworthy financial software — explicit totals, obvious
  destructive-action warnings, readable dense tables.

## Data layer

- `@tanstack/svelte-query` for all server state; `createInfiniteQuery` for
  cursor-paginated lists (consume `next_cursor` — silently showing page one
  only is a known bug class, backlog T-05); query keys include search strings.
- `minisearch` only for small in-memory sets (dropdowns, autocomplete); full
  transaction search is a backend FTS5 `search` param.

## Every new screen must define

1. Loading state, 2. Empty state, 3. Error state (translated, keyed on stable
error codes), 4. Success state — plus keyboard usability, labels/focus
handling, and deliberate responsive behavior (core workflows, including
transaction entry, must work on mobile).

## Validation

```sh
./scripts/test-frontend.sh    # openapi:generate + paraglide:compile + svelte-check
pnpm --dir frontend run test  # vitest (unit tests for pure logic)
pnpm build                    # if the production bundle/app shell changed
```

Add a vitest unit test when you write non-trivial pure logic (parsing,
label mapping, polling contracts — see `lib/api/imports.test.ts`).
