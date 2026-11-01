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
  Type props by annotation (`let { ... }: Props = $props()`), not `$props<T>()`. An `$effect` that resets a form tracks only the edited record's identity; read query data inside it with `untrack`, or a refetch wipes unsaved input (see `docs/conventions.md`).
  Focus-on-mount is `{@attach autofocus}` (`#lib/components/autofocus.ts`), not `bind:this` + `$effect`; never-mutated server snapshots use `$state.raw`; context uses `createContext`.
- **SvelteKit 3 imports**: shared code is `#lib/...` with an explicit extension
  (`#lib/money/amount.ts`, `#lib/components/panel.svelte`; the generated
  `schema.d.ts` is `#lib/api/schema.js`). Use the repository's `#lib/*`
  subpath imports consistently. Route state
  comes from `$app/state` (`page.url`, `page.params`); `$app/stores` is gone and
  `$app/environment` is now `$app/env`. `page.url.searchParams` is
  `ReadonlyURLSearchParams`, so URL helpers accept that type and copy via
  `new URLSearchParams(params.toString())` before writing. `goto` takes
  `reset: false` (not `keepFocus`/`noScroll`) to keep scroll and focus.
  Shallow URL selections (`replaceState`) read `page.shallow?.url ?? page.url`;
  SvelteKit 3 leaves `page.url` at the last full navigation. Use shared
  transaction URL helpers and real calendar-date validation (#326/#377).
- SvelteKit/adapter config lives in `sveltekit({...})` in
  `frontend/vite.config.ts`; there is no `svelte.config.js`.

## Structure

- Routes under `frontend/src/routes/app/<area>/` stay **thin entrypoints**;
  reusable workflow UI and helpers live in feature folders under
  `frontend/src/lib/<area>/` (examples: `lib/install-gate/`, `lib/reconcile/`,
  `lib/transactions/`).
- Build screens from the shared primitives (`PageHeader`, `Panel`,
  `StatePanel`, status badges — see `frontend/src/lib/components/`) before
  adding route-local styling.
- **Shell and navigation (#436):** a new `/app` page gets an entry in the
  route registry `#lib/navigation/routes.ts` (section, title key, icon, width, `tab`, `parent`
  for detail pages, `ownHeader` if it renders `PageHeader` itself) and a row in
  `e2e/playwright/route-sweep.spec.ts`; both have tests that fail otherwise.
  The shell draws the header, the section's tab row and the page `h1`, so a
  page never adds its own top-level navigation or horizontal page padding.
  A page is a route; a view inside it reads `page.url` on every navigation
  (never once with `untrack`). Moving a page keeps the old path as a `legacy`
  entry redirected by `canonicalURL`, keeping all query parameters and the
  fragment and replacing the history entry. Canonical Ledger and Planning paths
  live under `/app/ledger/…` and `/app/planning/…` (#460); internal links and
  browser journeys use those paths. Rules: `docs/conventions.md` → Frontend.
- **Page header (#459):** `PageHeader` slots are `breadcrumbs`, `heading`
  (object switcher), `scope` and `actions`. A page's main action is a visible
  button in `actions`, never only a shortcut; transaction pages pass
  `title`/`breadcrumbs` to `TransactionWorkspace`, which puts *Add transaction*
  there. The registry `width` sizes the page column from the shared left edge.
  Shortcuts: search owns `/` and Ctrl/⌘ K, the shell owns `N` and `?`; none acts
  in an editable field or under a modal, and every shortcut is listed in
  `shortcutList()` so `?` help shows it.
- Shell counts come from `#lib/attention/context.ts`, one composed attention
  query for header, tabs, Home and Review (#458). Unknown/error sources never
  become zero or all-clear, and a failed refresh is an error even while TanStack
  still holds the last data. Counting rules live in the design's attention contract.
- Home reads one composed `GET /api/v1/home` (`#lib/api/home.ts`, keyed by
  owner-local today); the shell's write middleware invalidates it with the
  attention count. Briefing rules live in `#lib/home/briefing.ts`; Home
  amounts use `formatHomeMoney`, nothing else does (#476). My Home widgets
  are view models in `#lib/home/widgets.ts` built from that same read; a
  widget never fetches on its own, and a failed summary stays in its card (#478). More to explore
  (`#lib/home/explore.ts`, #479) renders the same `home-widget-card.svelte`; a card may also read the
  shell's attention query, never a request of its own, and a widget invents no threshold or ranking:
  it reads the owning feature's rule (or that rule is built there first). "Out of date" is the
  pricing policy's staleness, the word Prices and Review use; a link to a
  filtered destination must list what Home named (`?status=unusable|stale`).
- Global search is `#lib/search/search-field.svelte` (#457). Mount one shortcut
  owner; reuse `scope`/`onSelect` and `variant="home"` for placement. Keep page
  filters independent, derive page hits from the route registry and consume every
  catalog cursor before claiming complete search coverage. Search resets when
  leaving a pathname, never on navigation completion or same-page URL repair;
  a completed selection must not close a newly opened search (#472). The design's
  [search contract](../../../docs/design/home-and-navigation.md#shared-search-foundation-457)
  records retained #437/#439/#442 scope. Home mounts it as its command bar and
  the header omits its own instance there (#477).
- Adding a transaction from anywhere is `addTransaction()`
  (`#lib/transactions/add-transaction.svelte.ts`): the page's own editor when
  it provides one (`providePageAdd`), otherwise the shared
  `TransactionEntryDialog` the shell mounts once (#477). Never add another
  entry dialog or navigate away to add.
- Every modal is `ModalDialog` (`#lib/components/modal-dialog.svelte`, #327):
  wrap it in `{#if}`, pass `onCancel` and `pending`, and keep Cancel before a
  destructive confirm. Never hand-roll a `fixed inset-0` + `aria-modal`
  overlay — it leaves focus on the page behind it. A side panel that covers
  the page only below `lg` attaches `phoneModal` from
  `#lib/components/modal-focus.ts` (#342).

## i18n (every user-visible string, no exceptions)

- Paraglide JS: `import { m } from '#lib/paraglide/messages.js';` then
  `m.some_message_id()`. Catalogs live in
  `frontend/messages/<domain>/<locale>.json`; keep message IDs flat, prefixed
  by domain/screen (`import_preview_dedupe_duplicate`).
- Add every new key to the maintained locales (`en`, `nl`, `ru`) with the same
  placeholder names. `es`, `fr` and `de` are partial (#493): a new or changed
  key may be left out there and falls back to English. Until #493 lands,
  `pnpm --dir frontend run catalogs:check` still requires all six; it gates
  dev/check/build and CI. Native terminology review remains separate (#102).
- Never concatenate translated fragments into sentences; use message parameters.
- A count-dependent message is one plural variant with a `countPlural=*`
  catch-all, called as `m.key({ count, n })` (`n` locale-formatted); never pick
  between singular/plural keys in code (`docs/conventions.md`, #476).
- Formatting of money, dates, numbers is locale-aware (`Intl.NumberFormat`,
  `Intl.DateTimeFormat`, `date-fns` v4) and separate from translation.
  Never use the `Date` constructor or luxon/moment for financial date logic.
- **All money parsing and exact arithmetic goes through `#lib/money/amount.ts`**
  (`parseDecimalAmount`, `formatEditableAmount`, `negateCoefficient`,
  `inflowPositiveAmount`, `commodityImbalance`) — string/BigInt over the ledger's
  `{ value, scale }` pair, never a `Number`. Never write a private amount helper
  inside a `.svelte` file: every copy that existed had drifted, and two of them
  shipped a silent 100× error (T-45).
- Editable fields parse and prefill in the active locale with `parseDecimalAmount`
  and `formatEditableAmount`; `formatLedgerAmount` is locale-independent output
  for CSV/machines, never form rehydration (#100).
- **All money display goes through `#lib/money/format.ts`** (`formatQuantity`)
  — locale-aware presentation of read-only figures. There is no money-formatting
  dependency: G-08 settled on `Intl.NumberFormat` and dropped `dinero.js`.
  Reports, tables, and CSV export use these two modules; they never grow their
  own inline formatting.
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
- Investment checkpoint/gain review reuses a lifecycle controller (#400):
  `previewReview` (`#lib/investments/preview-review.ts`) for preview-then-record
  forms, `commitReview` (`commit-review.ts`, `protocol: 'gains'` or
  `'checkpoints'`) for forms that ask for the impact at commit. Do not copy the
  stale-refusal/re-preview transitions into a new form; see `docs/conventions.md`.

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
