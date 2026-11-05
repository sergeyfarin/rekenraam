# Frontend framework choice review — 2026-11-03

**Status:** Analysis only. This document revalidates the current SvelteKit/Svelte
choice; it does not propose a rewrite and does not change an ADR, requirement or
roadmap item. The governed runtime shape remains in
[Early Architecture Decisions](../early-architecture-decisions.md#runtime-shape).

## Question

Rekenraam uses SvelteKit with Svelte 5, compiled by Vite to static assets that
the Go binary embeds and serves. The app is a browser-based single-page
application: there is no SvelteKit production server, server `load`, form action
or production hook. Is Svelte still the right frontend framework, and what would
React, Vue, Solid, Angular or another option add or take away?

This is a "keep or revisit" review, not a migration plan. It uses the same
constraints as the backend review: single-user, self-hosted, one Go binary,
SQLite, a browser UI that must work on desktop and phone, and a pre-release
product with one maintainer.

## Answer

**Keep SvelteKit with Svelte 5.** The framework matches the runtime shape
unusually well: it compiles to static assets, needs no Node production server,
supports typed Svelte 5 runes and a mature accessibility toolchain, and produces
a small initial payload. The current app already exercises the parts that matter
— typed routing, forms, tables, charts, focus management, localization and
browser acceptance tests — rather than relying on SSR or server components that
this product cannot use.

There is no measured frontend problem that another framework would fix. The
dominant CI cost is browser/axe work, not Svelte's runtime or compiler. A
framework rewrite would replace a working, heavily tested UI with a large
migration whose main benefit would be model familiarity or a larger component
ecosystem, not a demonstrated user-facing improvement.

## What would and would not change

| Concern | A framework switch would change it | Current evidence |
|---|---|---|
| Static production shape | New build/adapter path; must still emit assets for the Go binary | SvelteKit `adapter-static` with `fallback: index.html`; no production Node server |
| Initial payload / runtime | Framework runtime and compiler output would change | 214 KiB raw / about 65 KiB gzip initial preloaded JS+CSS; 2.75 MiB raw / 0.80 MiB gzip whole build |
| Accessibility | Component primitives, focus behaviour and test tooling would be re-chosen | Bits UI primitives, native dialog wrapper, axe route/sub-view sweeps, focus/contrast journeys |
| Localization | Message compilation and locale strategy would be rebuilt | Paraglide compile-time messages; localStorage → browser language → English fallback |
| State and forms | Reactivity, context and form patterns would be rewritten | Svelte 5 runes in 149 files; `$state.raw`, `SvelteSet`/`SvelteMap`, `createContext`, attachments |
| Typed contracts | Same OpenAPI generator can feed any TypeScript framework | `openapi-typescript` + `openapi-fetch`; strict TypeScript 6.0.3 |
| Unit/component tests | Test harness and ecosystem would change | 79 Vitest files / 979 tests; no DOM component-test harness today |
| Browser/CI time | Mostly unchanged; browser and axe still dominate | E2E app step median 750.5 s, audits 728.0 s; `Evaluate` dominates |
| Build time / memory | New compiler/bundler trade-offs | 62.5 s build, about 1,731 MiB peak RSS; Vite/Svelte/Tailwind callbacks |
| AI-assisted coding | React has a larger model corpus; Svelte needs more explicit conventions | No production measurement; not a runtime or correctness property |

## Current stack and measured shape

At the current `main` line the frontend uses:

- Svelte 5.57.2 with runes-only compilation.
- SvelteKit 3.1.0 with `@sveltejs/adapter-static` 4.0.1.
- Vite 8.3.5 and TypeScript 6.0.3.
- Tailwind CSS 4.3.3 with semantic theme tokens.
- Bits UI primitives and shadcn-svelte scaffolding.
- TanStack Query for server state and TanStack Table for dense ledgers.
- Paraglide for compile-time localization; `openapi-fetch` for the typed API.
- Playwright with axe for browser accessibility and layout sweeps.

The build measured at `da2563b4` produced the figures below. Later `main`
commits only patch frontend dependencies — adapter-static 4.0.1,
`@inlang/paraglide-js` 2.29.3 and `tailwind-variants` 4.0.1 — with no framework
or runtime change.

| Measurement | Value |
|---|---:|
| Hand-written TypeScript/Svelte files | about 356 |
| Page routes / layouts | 35 / 3 |
| Svelte files using runes | 149 |
| Frontend unit tests | 79 files / 979 tests |
| Browser attempts in the latest run | 455 app + 14 audit |
| Build wall / peak RSS | 62.5 s / about 1,731 MiB |
| Frontend check wall / peak RSS | 96.7 s / about 1,718 MiB |
| Whole static output | 2.75 MiB raw / 0.80 MiB gzip |
| Initial preloaded JS+CSS | 214 KiB raw / about 65 KiB gzip |
| JavaScript chunks | 195, split by route/component |

These figures are not a cross-framework benchmark. They show that the current
stack already produces a small static payload and that its build/check cost is
Node/Vite work, not a user-facing runtime problem. The full method and CI
timings are in the
[CI and test performance profile](ci-test-performance-profile-2026-11-03.md).

## Criteria

Ratings describe this product, not theoretical benchmarks.

| Criterion | SvelteKit (current) | React | Vue | Solid | Angular | HTMX / vanilla | Flutter / native |
|---|---|---|---|---|---|---|---|
| Static SPA + embedded assets | Strong | Strong with Vite | Strong with Vite | Strong with SolidStart/Vite | Possible but heavy | Strong server-driven, weaker SPA | Poor fit; different app |
| Initial runtime / bundle | Strong (compiled) | Adequate | Adequate | Strong | Heavy | Strong | Large/native bundle |
| Reactivity / state model | Strong runes | Strong hooks/compiler | Strong reactivity | Strong signals | Strong RxJS/DI | Server state | Different model |
| TypeScript | Strong | Strong | Strong | Strong | Strongest opinionated | Adequate | Dart, not TS |
| Component ecosystem | Adequate for this app | Best | Strong | Smaller | Strong enterprise | Small | Different |
| Accessibility primitives | Strong (Bits UI) | Strong ecosystem | Strong | Smaller | Strong | Manual | Platform-specific |
| i18n / static locales | Strong (Paraglide) | Strong | Strong | Adequate | Strong | Manual | Platform-specific |
| AI/LLM familiarity | Good, below React | Best | Good | Lower | Good | Good | Good outside web |
| Build speed / memory | Adequate | Adequate | Adequate | Strong | Heavy | Strong | Heavy |
| Desktop/mobile reuse | Strong via Wails | Strong via Electron/Tauri | Strong | Possible | Possible | Strong | Separate UI |

A framework's from-scratch fit and the cost of moving this UI are different
questions. The ratings above describe the former. The table below isolates the
latter; a high transition cost does not make a framework a bad choice in
general, and a good from-scratch fit does not make a migration worthwhile now.

### Transition cost from the current SvelteKit UI

| Option | Transition from here | Main risks |
|---|---|---|
| SvelteKit | None; this is the current UI | None |
| React | Full UI rewrite | Rewrite every route/component/form, rebuild localization and test harness, re-prove accessibility |
| Vue | Full UI rewrite | Same scope plus Nuxt/SPA decisions and ecosystem translation |
| Solid | Full UI rewrite | Same scope plus smaller component/accessibility ecosystem |
| Angular | Full UI rewrite | Same scope plus heavier runtime/tooling and framework conventions |
| HTMX / vanilla | Full UI and server-rendering rewrite | Moves state into Go, changes the static-embedding shape, reworks every interaction |
| Flutter / native | Second UI product | Duplicates screens and loses the shared browser/accessibility test surface |

## SvelteKit as-is

**What it brings**

- A static build that matches the Go binary exactly. `adapter-static` emits
  `index.html` plus immutable assets; Go serves them and falls back to the shell
  for extensionless routes. There is no Node process in production.
- A compiler-first model. Components compile to small runtime code, and the
  measured initial preloaded JS+CSS is about 65 KiB gzip before route-specific
  chunks. The whole build is under 1 MiB gzip.
- Svelte 5 runes with strict TypeScript: explicit `$state`, `$derived`,
  `$effect` and `$props`, `$state.raw` for immutable server snapshots, and
  `SvelteSet`/`SvelteMap` for reactive collections. The conventions are already
  written down and tested.
- A single-file component model with scoped styles, snippets, attachments and
  native dialog integration. The UI uses semantic theme tokens rather than
  framework-specific styling.
- Accessibility as a first-class test surface: Bits UI primitives, a shared
  modal/dialog wrapper, keyboard journeys, axe route/sub-view sweeps, contrast
  checks and mobile overflow checks.
- A localization boundary that works without a server: Paraglide compiles the
  catalogs, and locale selection is entirely client-side, which preserves the
  static SPA fallback.
- A credible desktop path: the
  [desktop review](desktop-application-feasibility-2026-09-19.md) reuses the
  same static frontend through Wails rather than building a second UI.
- Adequate ecosystem coverage for this product: TanStack Query/Table, Bits UI,
  shadcn-svelte, Lucide, date-fns and Tailwind. No current screen is blocked on
  a React-only library.

**What it costs**

- The ecosystem and AI training corpus are smaller than React's. Some niche
  components or examples are React-first; the team must translate patterns
  rather than copy them.
- Svelte 5 runes are newer than React hooks. The codebase has already migrated
  and enforces runes-only compilation, but new contributors need the conventions.
- There is no DOM component-test harness today: Vitest runs plain TypeScript
  modules, while UI behaviour is covered by Playwright and axe. That is a
  testing choice, not a Svelte limitation, but it makes some component-level
  regressions slower to isolate.
- The Vite/Svelte/Tailwind build is memory-heavy on a small machine: about
  1.7 GiB peak and 62 s wall in this environment. That is build tooling, not the
  shipped runtime.
- SvelteKit injects an inline bootstrap script, which is why the CSP currently
  allows `script-src 'unsafe-inline'`. The repo already documents the hash-based
  fix and has a plugin for SvelteKit's own announcer; this is a known tooling
  cost, not a reason to change frameworks.
- A static SPA gives up SSR/SEO for the authenticated app. That is intentional:
  the public website is a separate static build, and the app does not need
  server-rendered marketing pages.

**Transition cost from here:** None; this is the current UI.

## React

**What it would bring**

- The largest component ecosystem, the largest AI/LLM training corpus, the
  broadest hiring pool and the most third-party examples.
- Mature forms, charts, tables, accessibility primitives and testing tools.
- React Server Components and server rendering are available if the product
  ever needs them.

**What it would take away or add**

- The current product cannot use the main advantage of Next.js/RSC: production
  has no Node server, and all financial reads/writes go through the Go API. A
  React rewrite would most likely be a Vite SPA, so it would replace Svelte's
  compiler with a larger runtime and a different state/effect model without
  gaining server rendering.
- Hooks, dependency arrays, memoization and context boundaries add ceremony and
  new failure modes. The current Svelte code already uses typed runes and
  explicit snapshots; a React migration would rewrite every screen and form.
- React's larger dependency graph and faster-moving ecosystem increase the
  review surface. The app already carries TanStack and Bits UI equivalents; the
  marginal ecosystem gain is small for these screens.

**Transition cost from here:** Very high. It would rewrite 35 routes, 138
library components, 149 rune-based files, the localization boundary and the
browser-test evidence: 979 unit tests and 469 browser attempts would need a new
harness or a route-by-route replacement.

**Verdict:** the strongest alternative on ecosystem and AI familiarity, but it
solves no measured problem and gives up the static/compiled fit. Revisit only if
a required capability has no Svelte path.

## Vue

**What it would bring**

- A gentle template/reactivity model, a strong ecosystem, good TypeScript
  support and excellent documentation.
- Vite-based SPA builds and a mature component library ecosystem.

**What it would take away or add**

- Nuxt's server features are not usable in the current production shape, so a
  Vue rewrite would also be a Vite SPA.
- The component/state ecosystem is strong but not clearly better than the
  current Svelte + TanStack + Bits UI combination for these screens.
- The AI/LLM corpus is smaller than React's.
- There is no measured runtime or build benefit that would justify the rewrite.

**Transition cost from here:** Very high. The route/component/form/localization
and browser-test scope is effectively the same as React, with an additional
Vue/Nuxt-versus-Vite decision.

**Verdict:** a credible alternative for a new project, but not an upgrade for
this one.

## Solid

**What it would bring**

- Fine-grained reactivity and a very small runtime.
- Excellent raw performance characteristics and a JSX-like authoring model.
- SolidStart can produce static output.

**What it would take away or add**

- A much smaller ecosystem, fewer accessible component primitives and fewer
  production examples.
- SolidStart's maturity and conventions are less proven than SvelteKit's for
  this kind of app.
- Svelte already provides compiled reactivity and a small measured payload; the
  performance headroom is not a current constraint.
- The long-term maintenance pool is smaller.

**Transition cost from here:** Very high. The full UI, localization and
accessibility/browser test suite would be rewritten onto a smaller ecosystem.

**Verdict:** technically interesting, but the performance gain is unmeasured and
the ecosystem risk is higher.

## Angular

**What it would bring**

- An opinionated, TypeScript-first framework with dependency injection, forms,
  routing and a strong enterprise testing story.
- Long-term support and structure for large teams.

**What it would take away or add**

- A much heavier runtime/tooling model and more framework ceremony for a
  single-maintainer, static SPA.
- Slower builds and a larger migration surface.
- The app already has explicit routing, typed API access, forms and tests; it
  does not need Angular's full platform.

**Transition cost from here:** Very high. It is a full UI rewrite onto a heavier
platform, with no measured benefit for this deployment shape.

**Verdict:** a poor fit for this product's size, deployment shape and maintenance
model.

## HTMX, vanilla JS and server-rendered HTML

**What they would bring**

- Very small client runtimes and a simple request/response model.
- Server-rendered HTML could reduce client state and JavaScript.

**What they would take away or add**

- The current app relies on rich client state: shared search, optimistic query
  caches, editable tables, focus-managed dialogs, charts, multi-step previews
  and rollback-aware forms. Rebuilding those as HTML fragments would move
  complexity into the Go server and lose the typed OpenAPI client boundary.
- Accessibility, keyboard focus, i18n and browser testing would still need
  custom work; a small runtime does not remove that.
- A server-rendered UI would conflict with the accepted static-embedding shape
  and require an ADR change.

**Transition cost from here:** Very high. It is not a component swap: it would
move state and rendering into Go, rewrite the routes as HTML fragments, replace
the typed client boundary and redo the browser/accessibility tests.

**Verdict:** good for content sites and simple CRUD, not for this interactive
ledger UI without a significant architecture change.

## Astro, Qwik and other meta-frameworks

- **Astro** is content-first; the authenticated app is a stateful SPA, and the
  public website already has its own static build.
- **Qwik** relies on resumability and a server-oriented delivery model that does
  not match the Go-served static shell. Its ecosystem is also smaller.
- **Elm** has strong correctness properties but a small ecosystem, a different
  language and a steep maintenance trade-off.
- **Flutter/native** would duplicate the UI and lose the browser/accessibility
  test surface; native desktop remains out of scope under the desktop review.

Each of these carries the same full-UI transition cost as React or Vue, plus its
own runtime or language risk. None removes the current browser/axe cost.

## Why SvelteKit remains the coherent choice

The frontend's job is not to be a general-purpose web platform. It is the
browser client for one Go API, embedded in one binary, with a strong emphasis on
financial correctness, accessibility and localization. SvelteKit's static
adapter, compiler-first components, typed runes, small payload and existing
browser-test investment fit that job directly. React's main advantages —
server rendering, the largest ecosystem and the largest AI corpus — are either
unusable in this runtime or not decisive for these screens.

The current stack is also already proven in the repo: 979 unit tests, 469
browser attempts, zero Svelte diagnostics and explicit accessibility sweeps.
A rewrite would invalidate that evidence and require a new test/tooling layer
before it could claim parity.

## Revisit triggers

Do not migrate for framework popularity or model familiarity. Reopen this
review only when one of these is true:

1. **A required capability has no viable Svelte path.** A named chart, table,
   editor, accessibility primitive or integration exists only in another
   ecosystem and cannot be adapted within a bounded spike.
2. **A measured user or CI problem is attributable to Svelte itself.** The
   current build/check cost is Node/Vite tooling. A future problem must survive
   profiling and be specific to Svelte's compiler/runtime, not axe, browser
   startup or test setup.
3. **The runtime shape changes.** If the product accepts a Node/SvelteKit server,
   SSR or server actions, the framework trade-off changes; that needs an ADR
   first.
4. **The maintenance model changes.** If future maintainers are primarily React
   developers and cannot sustain Svelte 5 runes, that is an organisational cost,
   not a performance claim.
5. **SvelteKit's direction or security posture becomes unacceptable.** A major
   end-of-life, breaking direction or unfixed security issue would justify a
   fresh comparison.

## If a migration is ever justified

Keep the Go API and OpenAPI contract. Move one route or one bounded interaction
at a time behind the same browser URLs, with Playwright journeys and axe checks
as the acceptance gate. Do not attempt a big-bang rewrite of all routes,
components, forms and localization at once. A second framework running beside
SvelteKit is acceptable only as a time-boxed spike; the production app should
have one UI stack.

## References

- [Backend language choice review — 2026-11-03](language-choice-review-2026-11-03.md).
- [CI and test performance profile — 2026-11-03](ci-test-performance-profile-2026-11-03.md)
  and its [evidence](fixtures/ci-test-profile-2026-11-03.json).
- [Early Architecture Decisions](../early-architecture-decisions.md#runtime-shape).
- [Desktop application feasibility review](desktop-application-feasibility-2026-09-19.md).
- [Frontend conventions](../conventions.md#frontend-conventions) — SvelteKit
  static output, runes, component stack, localization and accessibility rules.
