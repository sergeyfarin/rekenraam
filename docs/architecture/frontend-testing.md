# Frontend Testing

Last updated: 2026-05-18

How tests are structured in the Svelte frontend: what runs where, which tool
to reach for, and the conventions that keep the suite fast and stable. The
canonical execution plan for landing this infrastructure lives in
[docs/product/phase-3-plan.md](../product/phase-3-plan.md); this document is
the steady-state reference once that plan ships.

## Test pyramid

Three layers, each with a single-tool answer:

```
┌──────────────────────────────────────────┐
│  Playwright e2e   (e2e/*.spec.ts)        │  ← 12 specs, runs against compose stack
├──────────────────────────────────────────┤
│  Vitest component (src/.../*.test.ts)    │  ← ~60 tests, jsdom + @testing-library/svelte
├──────────────────────────────────────────┤
│  Vitest unit      (src/lib/**/*.test.ts) │  ← ~150 tests, pure functions
└──────────────────────────────────────────┘
```

Rule of thumb for "where does this test live":

| Question | Answer |
|---|---|
| Does it depend on a Svelte component rendering? | Vitest component (or Playwright if it needs real CSS/layout) |
| Is it pure logic on data? | Vitest unit (`src/lib/**/*.test.ts`) |
| Does it touch the FastAPI service, SQLite app container, or auth cookies? | Playwright (`e2e/*.spec.ts`) |
| Does it require a real browser layout engine (drag, scroll, pointer events)? | Playwright |

Tests at the lowest level that can prove the property. A pure parser doesn't
need a component test; a component's prop wiring doesn't need a Playwright
test.

## Vitest layer

Configured in [vitest.config.ts](../../vitest.config.ts):

- **Environment:** `jsdom`. Renders DOM nodes; does NOT do layout, paint, or
  real pointer events.
- **Setup:** [src/test/setup.ts](../../src/test/setup.ts) imports
  `@testing-library/jest-dom/vitest` and runs `cleanup()` +
  `vi.restoreAllMocks()` between tests.
- **`$app/*` aliases:** SvelteKit's `$app/state`, `$app/navigation`,
  `$app/environment` are mocked in
  [src/test/mocks/](../../src/test/mocks/) so route tests don't need a
  running SvelteKit server. The `goto` export is a `vi.fn()` — component
  tests can assert on its calls.

Run with `npm test` (one-shot) or `npm run test:watch` (TDD loop).

### Unit tests (`src/lib/**/*.test.ts`)

For pure functions that don't touch the DOM. Examples:

- [src/lib/money.test.ts](../../src/lib/money.test.ts) — parse/format money
- [src/lib/api/client.test.ts](../../src/lib/api/client.test.ts) — typed API
  error parsing and user-facing error messages
- [src/lib/book-context.test.ts](../../src/lib/book-context.test.ts) —
  active-book initialization and persistence
- [src/lib/dates.test.ts](../../src/lib/dates.test.ts) — smart-date parser
- [src/lib/transactions/saved-views.test.ts](../../src/lib/transactions/saved-views.test.ts)
  — filter-state ↔ API-shape round-trip
- [src/lib/forms/validators.test.ts](../../src/lib/forms/validators.test.ts)
  — validation result types

These are the cheapest tests in the suite (~5ms each). Add new ones liberally
when extracting logic out of `+page.svelte` files. See
[phase-3-plan.md §Workstream B](../product/phase-3-plan.md) for the
extraction pattern.

### Component tests (`*.test.ts` co-located with `*.svelte`)

For Svelte components. Render with
`render(Component, props)` from `@testing-library/svelte`; assert via
`screen.getByRole(...)` and the jest-dom matchers from
`@testing-library/jest-dom`.

Mocking strategy: **per-spec `vi.mock("$lib/api/<module>", …)`** at the top
of the file, then `vi.mocked(fn).mockResolvedValue(…)` per test. This was
chosen over a shared mock-API module so each spec declares only the API
surface it actually touches; see the C-workstream notes in
[phase-3-plan.md](../product/phase-3-plan.md) for the discussion.

Examples:

- [src/lib/components/TransactionSplitEditor.test.ts](../../src/lib/components/TransactionSplitEditor.test.ts)
  — direct component render, no API mocks needed.
- [src/routes/layout.test.ts](../../src/routes/layout.test.ts) — full layout
  render with `$lib/api/auth` mocked.
- [src/routes/planning/page.test.ts](../../src/routes/planning/page.test.ts)
  — full page render with 11 API modules mocked.
- [src/routes/reset-password/page.test.ts](../../src/routes/reset-password/page.test.ts)
  and [src/routes/accept-invite/page.test.ts](../../src/routes/accept-invite/page.test.ts)
  — auth lifecycle pages with API clients mocked.

### API error convention

All API calls should throw `ApiError` from
[src/lib/api/client.ts](../../src/lib/api/client.ts). Components and pages
should render `formatApiError(error)` from [src/lib/utils.ts](../../src/lib/utils.ts)
instead of `String(error)` or raw `error.message`. Add a focused unit test
whenever a new error shape is introduced at the client boundary.

### What jsdom can NOT test

If you're tempted to test any of these in Vitest, escalate to Playwright:

- **Layout & sizing:** jsdom returns `0` for `getBoundingClientRect`,
  `clientWidth`, etc. Column-resize state machines need Playwright.
- **Real pointer events:** `pointerdown`/`pointermove`/`pointerup` don't
  propagate through the layout engine.
- **CSS-conditional rendering:** styles aren't evaluated; selectors that
  depend on computed styles will silently no-op.
- **Cross-tab cookies / session lifecycle:** the auth seam works fine with
  mocks at the API client level; the real cookie-domain behavior is
  Playwright territory.

## Playwright layer

Configured in [playwright.config.ts](../../playwright.config.ts):

- **Single `chromium` project.** Firefox/WebKit deferred — we have one
  rendered target (the static SvelteKit build behind nginx); compatibility
  testing is out of v1 scope.
- **No `webServer`.** CI brings up the compose stack itself
  ([.github/workflows/web-e2e.yml](../../.github/workflows/web-e2e.yml));
  local dev is `docker compose -f compose.sqlite.yaml up -d --wait && npm run e2e`.
- **`workers: 1`.** All specs share the single compose-deployed SQLite volume;
  serial execution is required by the per-spec reset fixture (below).
- **`PLAYWRIGHT_BASE_URL`** defaults to `http://localhost:3000`, but the v1 CI
  job sets it to the SQLite app at `http://localhost:8080`.
- **`PLAYWRIGHT_DB_BACKEND`** defaults to `sqlite`;
  **`PLAYWRIGHT_COMPOSE_FILES`** defaults to `compose.sqlite.yaml`. Both can be
  overridden for post-v1 Postgres compatibility work.

Run with `npm run e2e` (one-shot) or `npm run e2e:ui` (the Playwright UI
runner). First-time setup: `npm run e2e:install` to grab the chromium
browser + system deps.

### Fixtures

All in [e2e/fixtures.ts](../../e2e/fixtures.ts). The Playwright `test` is
re-exported from this file with three extensions:

| Fixture | Lifetime | What it gives you |
|---|---|---|
| `cleanDatabase` | auto, per-test | Resets the DB before the test body runs |
| `authedApi` | per-test | `APIRequestContext` bootstrapped + authed as admin |
| `loggedIn` | per-test | `Page` already at `/` as an authenticated user |

The simplest spec is one parameter: `test("...", async ({ loggedIn: page }) => { … })`.
For specs that need to drive the auth flow itself (E1), take `{ page }` and
do the form-filling explicitly.

### DB reset strategy

**SQLite baseline copy**, not a `/api/v1/test/reset` endpoint. First call to
`resetDatabase()` stops the app container and snapshots the post-migration
`/data/rekenraam.sqlite3` file as `/data/rekenraam.e2e-baseline.sqlite3`.
Each subsequent call:

1. `docker compose -f compose.sqlite.yaml stop app` to release SQLite file
   handles.
2. Run a one-off app container against the same volume to delete
   `rekenraam.sqlite3` plus any `-wal`, `-shm`, or `-journal` sidecars.
3. Copy `rekenraam.e2e-baseline.sqlite3` back to `rekenraam.sqlite3`.
4. `docker compose -f compose.sqlite.yaml start app` and poll
   `/api/v1/health` until ready.

The seeded `personal` book (book_id=1), USD commodity (commodity_id=1), Cash
account (account_id=2), and the $5,000.00 opening balance are all preserved
exactly as the migrations left them.

**Why not the plan's `/api/v1/test/reset` endpoint?** It would ship test
code in the production image, gated by `REKENRAAM_E2E_RESET=1`. The
volume-copy approach avoids that surface entirely. Documented in
[phase-3-plan.md §A2 decision record](../product/phase-3-plan.md).

### Specs

| Spec | Coverage |
|---|---|
| [e2e/smoke.spec.ts](../../e2e/smoke.spec.ts) | Login form renders on fresh stack; `loggedIn` fixture lands authenticated |
| [e2e/auth.spec.ts](../../e2e/auth.spec.ts) (E1) | Bootstrap → logout → re-login → session-clear; wrong password error |
| [e2e/transactions.spec.ts](../../e2e/transactions.spec.ts) (E2) | New transaction dialog → list row → balance reflects on `/accounts/{id}` |
| [e2e/reconcile.spec.ts](../../e2e/reconcile.spec.ts) (E3) | Matched-balance finish (no offset); mismatched balance requires offset |
| [e2e/cross-currency.spec.ts](../../e2e/cross-currency.spec.ts) (E4) | USD→EUR transfer via the cross-currency dialog; same-account validation |
| [e2e/import-ofx.spec.ts](../../e2e/import-ofx.spec.ts) (E5) | OFX preview → commit → re-upload flags duplicates |
| [e2e/reports.spec.ts](../../e2e/reports.spec.ts) (E6) | Cashflow auto-loads with totals; Spending-by-Category tab |

### Adding a new spec

1. Create `e2e/<feature>.spec.ts`.
2. Import from `./fixtures`, not directly from `@playwright/test`.
3. Pick a fixture (`{ loggedIn: page }` is the right default).
4. Use the public API (`/api/v1/*`) via `page.evaluate(...)` to seed any
   data the spec needs — keeps tests robust to UI changes for seed steps
   that aren't part of the assertion path.
5. Drive the UI via accessibility-friendly locators (`getByRole`,
   `getByLabel`, `getByText`) before falling back to CSS / id selectors.

### What Playwright is NOT for

- Logic that can be unit-tested. A spec that boots compose just to verify a
  function's output is wasteful.
- Visual regression. We don't run that today; if it lands, it gets a
  separate Percy/Chromatic-style runner, not Playwright snapshots.
- Cross-browser. See "Single chromium project" above.

## CI shape

Two workflows, [split](.../.github/workflows/) by cost:

- **[web-unit.yml](../../.github/workflows/web-unit.yml)** — always-on.
  `npm ci` → `npm run check` → `npm test`. ~30s including
  install. Triggers on `src/**` and config-file changes.
- **[web-e2e.yml](../../.github/workflows/web-e2e.yml)** — label-gated on
  PRs (apply the `run-e2e` label), always-on for pushes to main. Brings up
  the full compose stack, runs `npm run e2e`, uploads
  `playwright-report/` on failure. ~5-10 min.

Why split? Most FE PRs don't touch the compose stack at all; running the
expensive e2e job on every PR would burn CI budget. The `run-e2e` label is
a one-click opt-in when an e2e regression is actually plausible.

## Common patterns and pitfalls

### Pattern: `vi.mock` ordering matters

```ts
// CORRECT — vi.mock is hoisted; declare it before any imports that pull
// in the mocked module.
vi.mock("$lib/api/auth", () => ({ getCurrentUser: vi.fn() /* ... */ }));

import { getCurrentUser } from "$lib/api/auth";
import Layout from "./+layout.svelte";
```

If you `import Layout` before `vi.mock(...)` lexically, Vitest still
hoists the `vi.mock` call, so it works — but reading top-to-bottom is
clearer when the mock comes first.

### Pattern: assert on the parent label, not the value cell

DOM text can be ambiguous. In the reconcile spec, multiple cells render the
same dollar amount when the user toggles a checkbox. Walk from a label to
its sibling value instead of grepping the document:

```ts
const selectedLabel = screen.getByText("Selected");
const selectedValue = selectedLabel.nextElementSibling as HTMLElement;
expect(selectedValue.textContent).toMatch(/80\.00/);
```

### Pitfall: `getByRole("combobox")` over-matches

`<input list="…">` carries `role="combobox"` in modern browsers because of
the implicit datalist binding. Counting `getByRole("combobox")` will
include both the actual `<select>` elements AND any datalist-backed
`<input>`. Use `getByPlaceholderText` or `getByLabel` for inputs with
unambiguous labels.

### Pitfall: Svelte 5 `derived_inert` warnings on cleanup

When a test unmounts a component, `bits-ui`'s focus trap effects can read
derived state from the just-destroyed scope. Svelte logs
`[svelte] derived_inert` to stderr. Harmless; ignore unless the test
actually fails.

### Pitfall: type-mismatch in mock factories

API mock factories (e.g. for `CommoditySummary` or `CategorySummary`) must
match the real type shape exactly. Missing fields like `updated_at` won't
fail the test at runtime — tests use the value structurally — but
`npm run check` will fail in CI. Always assign through the declared type:

```ts
function mockCommodity(id: number): CommoditySummary {
  return { id, /* full shape */ };
}
```

## Per-feature checklist for new components

When you carve a new component out of a route page or add one from scratch:

- [ ] Pure logic extracted into `$lib/**` with its own `*.test.ts` (unit
  layer).
- [ ] Component test in the same directory as the `.svelte` file, with
  per-spec `vi.mock` of any API modules it imports.
- [ ] If the component is reachable from a navigated route and exercises a
  user-facing flow, add or extend a Playwright spec.
- [ ] Skip the e2e if Vitest can prove the property — every Playwright
  test is 100x slower.

## Reference numbers (2026-05-15)

| Layer | Files | Tests | Runtime |
|---|---:|---:|---:|
| Vitest unit + component | 13 | 216 | ~8s |
| Playwright e2e | 7 | 12 | ~3 min (CI) |

Acceptance criteria for the suite from
[phase-3-plan.md](../product/phase-3-plan.md) — both met as of 2026-05-15.
