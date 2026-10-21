# Frontend routes, dialogs, accessibility, themes and localization audit — 2026-10-18

**Status:** [#278](https://github.com/sergeyfarin/rekenraam/issues/278), area 10
of [#260](https://github.com/sergeyfarin/rekenraam/issues/260). The audit has
run and every delivery bullet has a disposition. Seven findings were filed.
Five were fixed in this change, each with a regression:
[#324](https://github.com/sergeyfarin/rekenraam/issues/324) (P2),
[#325](https://github.com/sergeyfarin/rekenraam/issues/325),
[#328](https://github.com/sergeyfarin/rekenraam/issues/328),
[#329](https://github.com/sergeyfarin/rekenraam/issues/329) and
[#330](https://github.com/sergeyfarin/rekenraam/issues/330) (P3). Two remain
open, each with its impact stated:
[#327](https://github.com/sergeyfarin/rekenraam/issues/327) (P2, modal
dialogs do not take, trap or return keyboard focus) and
[#326](https://github.com/sergeyfarin/rekenraam/issues/326) (P3, some
filters and tabs are not kept in the URL). No finding affects financial
data. This is bounded evidence on the states each check reaches. It does not
certify every state of every screen, a native-language review (#102), or
release readiness.

## Follow-up — 2026-10-21

[#326](https://github.com/sergeyfarin/rekenraam/issues/326) now carries the
budget month/currency, recurring tab and all three scoped transaction-list
filters in the URL. [Delivery evidence](url-selection-delivery-2026-10-21.md)
records reload, Back and shared-link journeys at desktop and phone width.
Small adjacent #377 repairs impossible calendar-date links. #378 corrects this
report's instrument-route fixture: it previously used the instrument ID where
the route expects the commodity ID, and generic headings did not establish
that the intended instrument rendered. The corrected sweep requires the seeded
instrument heading and its activity search control. The original matrix below
is historical; the delivery report records the repeated bounded audit.

## Scope and baseline

- Baseline: `605563a823af1c425159dc1f394366a93f445802`, clean working tree.
- Governing sources: `AGENTS.md` frontend rules, the `frontend-screen` skill,
  ADR 0007, [the shared audit brief](app-state-and-audit-plan-2026-10-17.md),
  and the R3a rows in [implemented.md](../implemented.md).
- Book: the provider-free #266 reference book (`seedReferenceBook`), so
  lists, trees, positions, lots, reports and budgets render with real rows.
  No provider is called beyond the existing `MARKET_DATA_BASE_URL` stub.
- Independent expectations: WCAG 2.1 A/AA through axe 4.14 (rules, not app
  output); contrast ratios computed from the `app.css` token values with the
  WCAG luminance formula (`theme-contrast.test.ts`), never read from the
  rendered page; and the WAI-ARIA modal dialog pattern for focus behaviour.
- New consumer: `e2e/playwright/route-sweep.spec.ts`. It visits all 24 app
  routes. For each route it runs axe in the light and dark themes at
  1280 px, 390 px and 640 px (a 1280 px window at 200% zoom), and checks that
  the page never scrolls sideways at 390 px or 640 px. It then runs axe and
  the overflow check in es/fr/nl/de/ru at 390 px and asserts the declared
  page language. Finally, it answers each route's own `GET /api/v1/**`
  requests with 500 and requires an announced, translated, axe-clean error
  state. Each visit first asserts the requested pathname and the absence of
  the sign-in form. The first draft lacked that guard, and its passing run
  had audited the sign-in screen, not the routes. That run was discarded.

## Findings

| Finding | Severity | Expected versus actual | Disposition |
|---|---|---|---|
| [#324](https://github.com/sergeyfarin/rekenraam/issues/324): colour utilities that name no token | P2 | Every colour utility should resolve to an `@theme inline` token. Tailwind emits nothing for an unknown name. The built CSS has `.text-danger` but no `.text-destructive`. **40** uses named no token: 22 × `text-destructive` (investment errors and negative gains lost their danger colour), `text-danger-foreground`/`text-warning-foreground` on the danger and warning **confirmation buttons**, palette greens, `bg-black/50`, `text-white`, `bg-accent-soft`, `bg-subtle`, `bg-warning-subtle`, `border-warning-border`. Computed button contrast was about 3.1–3.2:1 in light and **1.6:1 in dark**, against 4.5:1. | **Confirmed, fixed.** Replaced with existing tokens (`text-background` on danger/warning fills, the reconcile-checkpoint pattern). `theme-tokens.test.ts` scans every `.svelte`/`.ts` source and lists all 40 on the baseline. New contrast pairs exposed dark danger text on surface-strong at 4.36:1, so dark danger lightness moved from 0.69 to 0.71. All 30 combinations now pass. |
| [#327](https://github.com/sergeyfarin/rekenraam/issues/327): modal focus | P2 | The WAI-ARIA dialog pattern: focus moves inside, Tab stays inside, Escape closes, and focus returns to the trigger. The real void dialog, opened from the keyboard: focus **stayed on Void…** behind the overlay; the first Tab left the dialog and 15 Tabs walked the top bar and every nav link; Escape did nothing; after Cancel, focus landed on `<body>`. About 28 hand-rolled overlays share the markup, and only `write-off-form` moves focus. | **Confirmed, open.** Cross-cutting: it needs one shared dialog primitive (native `<dialog>` + `showModal()` recommended). Impact: a keyboard user can operate hidden background controls while a confirmation is open. Each action still needs an explicit confirm, so no data is affected. |
| [#325](https://github.com/sergeyfarin/rekenraam/issues/325): page language and a raw code | P3 | `<html lang>` should follow the locale (WCAG 3.1.1). It was always `en`: `app.html` hard-codes it and nothing updates it. Trash badged a deleted voided transaction with the raw `voided` code in every locale. | **Confirmed, fixed.** The root layout sets `lang` from `getLocale()`. Trash uses `statusLabel()`. Regressions: `language.spec.ts` (en → ru → en), the sweep's lang check in five locales, and `trash-restore-reconciliation.spec.ts` (`Voided`, never `voided`). |
| [#328](https://github.com/sergeyfarin/rekenraam/issues/328): two axe findings | P3 | The light sweep, on its first run against real routes, found `scrollable-region-focusable` on the net-worth table scroller at 390 px (the cashflow and spending scrollers use the same markup), and `aria-prohibited-attr` on the five appearance swatches (a `div` with `aria-label` and no role). | **Confirmed, fixed.** The scrollers follow the gains-table region pattern (`role="region"`, label, `tabindex="0"`). The swatches are `role="img"`. The sweep now passes. |
| [#329](https://github.com/sergeyfarin/rekenraam/issues/329): load errors not announced | P3 | A failed load should be announced (WCAG 4.1.3). Twelve data routes had no `role="alert"`. Most showed translated plain text, which a screen reader does not announce: the shared transaction table (transactions, register, category, trash), investments and instrument, the three reports, currencies and Data settings. **Reconcile showed nothing**: an empty account picker that reads as an empty book. | **Confirmed, fixed.** `StatePanel alert` announces the title and copy, not the retry controls. The Data settings error lines are alerts, and reconcile shows `APIFormError`. Import's rules and connections load errors are alerts; a failed connections list read "Could not save the connection." and now has its own load message (six locales). The Settings overview keeps a failed status inline per value (it is scoped out in #329). |
| [#330](https://github.com/sergeyfarin/rekenraam/issues/330): unnamed import pickers | P3 | Every `<select>` should have an accessible name (WCAG 4.1.2). Import passed on a fresh worker book but failed `select-name` (critical) in all five locales on a worker that already held a Trading 212 connection and a staged CSV preview from earlier import specs. Five pickers were unnamed: the connection's cash account, and each preview row's account, currency, transfer account and category. | **Confirmed, fixed.** The connection picker uses its column header. Row pickers get row-specific names (`Category, row 2`, six locales), so they neither repeat nor collide with the bulk controls. Existing `getByLabel('Target account'/'Currency')` locators are now exact. `csv-import.spec.ts` runs axe on the staged preview. |
| [#326](https://github.com/sergeyfarin/rekenraam/issues/326): URL state | P3 | Transactions, reports, forecast, pricing, the investments tab and dividend repair keep their filters in the URL. The budget month and currency, the recurring Due/Templates tab, and the register, category and instrument activity filters are component state, so reload, Back and shared links reset them. | **Open, accepted gap.** No governing plan requires URL state outside reports, so this is consistency, not a contract breach. Impact: Back from a transaction resets the budget month. |

## Route × check matrix

**V** verified by the named consumer in this run. **R** verified by review
plus an existing named spec. **N** not verified. **—** not applicable. Every
cell in the axe, overflow and locale columns comes from `route-sweep.spec.ts`
at three widths (desktop, 390 px and 640 px for the theme columns; 390 px for
the locale column) after the fixes. "Success" means the seeded reference
book renders.

| Route family | Route(s) | Success | Error state | Empty | Loading | Light axe | Dark axe | 390 / 200% | 5 locales (lang, fit, axe) | URL state | Dialogs |
|---|---|---|---|---|---|---|---|---|---|---|---|
| Setup / root | `/` | R (`auth`, `accessibility`) | R (`auth`) | — | R | R (`accessibility` entry screen) | N | R (`auth` 390) | R (`catalog-localization` MFA) | — | — |
| App home | `/app` | V | R (`home_health_state_error`; reads only the shell's `/health`) | — | R | V | V | V | V | — | — |
| Accounts and register | `/app/accounts`, `/accounts/{id}/register` | V | V (#329 fix) | R (`accounts_empty_*`) | R | V | V | V | V | register filters: #326 | account editor, move-day: #327 |
| Categories and detail | `/app/categories`, `/categories/{id}` | V | V (#329 fix) | R | R | V | V | V | V | category filters: #326 | — |
| Transactions | `/app/transactions` | V | V (#329 fix) | R (`transactions.spec`) | R | V | V | V | V; `locale-amounts` 6 locales | V (URL filters) | void/delete/restore/override: #327 |
| Reconcile | `/app/reconcile` | V | V (#329 fix) | R (`reconcile_start_no_accounts`) | R | V | V | V | V | — | void checkpoint: #327 |
| Import and dividend repair | `/app/import`, `/import/dividends` | V | V | R (`import` specs) | R | V | V | V | V; `catalog-localization` CSV mapping 6 locales | dividends V | — |
| Investments, instrument, income | `/app/investments`, `/instruments/{id}`, `/investments/income` | V | V (#329 fix) | R (`investments_empty_*`; `income-coverage` states) | R | V | V | V | V | tab V; instrument filters #326 | entry sheet, corrections, reconciliation confirm: #327 |
| Pricing | `/app/pricing` | V | V | R | R | V | V | V | V | V | price void: #327 |
| Reports | `/app/reports` (net worth; `accessibility` runs all three views) | V | V (#329 fix) | R | R (`accessibility` fade sampler) | V (#328 fix) | V | V (#328 fix) | V; `language` ru | V (reports plan) | — |
| Budgets | `/app/budgets` | V | V | R | R | V | V | V | V | month: #326 | — |
| Recurring | `/app/recurring` | V | V | R | R | V | V | V | V | tab: #326 | — |
| Forecast | `/app/forecast` | V | V | R (`forecast` specs) | R | V | V | V | V | V | — |
| Settings overview | `/app/settings` | V | inline per value (#329 scope note) | — | R | V | V | V | V | — | — |
| Appearance | `/settings/appearance` | V | — (no fetch) | — | — | V (#328 fix) | V | V | V | — | — |
| Currencies | `/settings/currencies` | V | V (#329 fix) | R | R | V | V | V | V | — | rate dialog: #327 |
| Language | `/settings/language` | V | — (no fetch) | — | — | V | V | V | V | — | — |
| Security | `/settings/security` | V | V | R (`mfa`) | R | V | V | V | V; `catalog-localization` MFA | — | — |
| Data | `/settings/data` | V | V (#329 fix) | R (`data-settings`) | R | V | V | V | V | — | — |
| Trash | `/settings/trash` | V | V (#329 fix) | R | R | V | V | V | V | — | restore override: #327, #324 fix |

Cross-cutting checks:

| Check | Independent expectation | Consumer and result | Disposition |
|---|---|---|---|
| Semantic tokens only | Every colour utility names an `@theme inline` token | `theme-tokens.test.ts`: 40 offenders on the baseline, 0 after | **Verified** after #324. Grep for raw `#hex`, `rgb()` and `oklch()` in `.svelte`/`.ts` found only issue references in comments. |
| Contrast, every palette | 4.5:1 for each pair across 30 theme × base × accent combinations | `theme-contrast.test.ts` with new pairs: danger/warning button text, note text on warning-soft, positive/danger text on surface, background and surface-strong | **Verified** after the dark-danger adjustment. |
| Hard-coded copy | All user-visible text comes from Paraglide | A script that strips scripts, styles, tags and expressions from every `.svelte` file left only technical literals: the `recover-owner` command, `DD/MM/YYYY`-style layout names and a CSV header example. The sweep found one rendered code (#325). | **Verified** after #325. **Accepted limit:** fixed English `change_reason` strings (for example `'Updated monthly budget target'`, `'restored from trash'`) are stored as audit annotations, not chrome. |
| Catalog parity | Same keys and placeholders in six locales | `catalogs:check`, 66 tests | **Verified.** Parity is not terminology approval. |
| Numeric and date entry and display | Locale decimal and grouping, exact above 2^53 | `locale-amounts.spec.ts`, six locales at 390 px | **Verified (reused).** |
| Owner-local dates | Server/owner-local today drives defaults | `owner-local-dates.spec.ts`, `owner-local-date-contract.spec.ts` | **Verified (reused).** |
| Keyboard reachability | Named controls reached by Tab | `accessibility.spec.ts` | **Verified (reused).** Dialog focus is #327. |
| Shared-browser privacy | — | [#268 evidence](proxy-browser-privacy-audit-2026-10-17.md) | **Linked**, not re-run. |
| Native-language sign-off | A native reviewer per locale | [#102 packet](localization-native-review-2026-10-13.md) | **Not verified.** Tests cannot infer this. |
| Real zoom | Browser zoom at 200% | Emulated by a 640 px CSS viewport | **Accepted limit.** Text-only zoom and OS font scaling are not exercised. |
| Empty states | Each list's own empty copy | Review of `*_empty*` keys per screen and existing specs. The sweep's worker book is never empty | **Verified (review)** where marked R. No fresh-book browser pass was run. |
| Other `overflow-x-auto` scrollers | Keyboard-scrollable when they overflow | 16 more wrappers lack `tabindex`. axe flags one only when it overflows with nothing focusable inside, and none did on the seeded book | **Accepted limit.** It would surface in the sweep once a book makes them overflow. |

## Acceptance

- [x] A route × check matrix is published with dispositions (above).
- [x] Hard-coded colours and copy were found by grep and either fixed
  (#324, #325) or filed. The colour check is now a unit gate.
- [x] The axe suite runs on every reachable route and state: success in both
  themes at three widths, five more locales, and the forced error state of
  every data route (`route-sweep.spec.ts`), plus the R3a journeys.

#278 has no closure blocker. #327 (P2) and #326 (P3) stay open with their
impact stated, as the evidence contract allows for P2/P3.

## Validation

```sh
./scripts/test-frontend.sh
pnpm --dir frontend exec vitest run src/lib/theme-tokens.test.ts src/lib/theme-contrast.test.ts
pnpm --dir e2e exec playwright test --project app --workers 3 \
  playwright/route-sweep.spec.ts playwright/accessibility.spec.ts playwright/catalog-localization.spec.ts \
  playwright/language.spec.ts playwright/locale-amounts.spec.ts playwright/trash-restore-reconciliation.spec.ts \
  playwright/reports.spec.ts playwright/data-settings.spec.ts playwright/reconciliation-recovery.spec.ts \
  playwright/transactions-lifecycle-reconciliation.spec.ts playwright/currency-provider-accessibility.spec.ts \
  playwright/investments-gains-statement.spec.ts playwright/investments-short.spec.ts \
  playwright/forecast-learning.spec.ts playwright/reference-data-management.spec.ts \
  playwright/csv-import.spec.ts playwright/import-short-link.spec.ts playwright/import-dividend-review.spec.ts
```

Results on the final tree: frontend check 0 errors and 0 warnings; **690**
frontend unit tests pass (including the new token scan and 30 contrast
combinations); catalog parity 66/66. Browser results are recorded below.
`theme-tokens.test.ts` lists 40 offenders on the baseline. The #328, #329 and
#330 failures were observed in the sweep before their fixes. The #325
regressions were not run against the baseline: `app.html` hard-codes
`lang="en"`, and the trash badge rendered the raw field. The void-dialog
focus probe was a temporary spec and was not committed; its output is
reproduced in #327. Full backend, full browser and shared architecture gates
were not run; #280 owns them.

**86 browser journeys pass** (6.5 min, 3 workers) against a fresh
integrated build of the final tree. They comprise the four route-sweep checks
(light and dark × 24 routes × 3 widths; 5 locales × 24 routes; forced error
states on 20 data routes) and the 18 named specs above. `release-preflight.spec.ts`
received only the exact-locator change and was not run; it runs weekly and
on dispatch (#306).

Tooling note for #280: `e2e/package.json` pins `pnpm@11.20.0` while the root
pins `pnpm@12.11.2`. Under corepack, `pnpm exec` run from inside `e2e/`
refuses to start. The documented `pnpm --dir e2e …` form from the repository
root works.

## Maintainability notes

- **No shared dialog primitive.** About 28 overlays repeat
  `fixed inset-0 … role="dialog" aria-modal="true"` by hand, 15 of them in
  `lib/transactions/investment-correction-section.svelte`. Focus handling
  exists only in `write-off-form.svelte`. A dialog behaviour fix (#327) has
  to touch every copy.
- **Colour contract enforced only by convention until now.** Tailwind accepts
  any `text-*`/`bg-*` name silently. Three different "foreground on a fill"
  spellings (`text-background`, `text-danger-foreground`, `text-white`)
  coexisted for the same role (#324). The new scan pins the token set, but the
  pairing rule still lives in comments and in `theme-contrast.test.ts`.
- **Error-state announcement decided per screen.** Some screens nest
  `APIFormError` (which carries `role="alert"`) in `StatePanel`; others pass
  only title and copy. Before the opt-in `alert` prop, the same shared table
  error was silent on four routes (#329).
- **Two URL-state mechanisms.** `transaction-url-filters.ts` serves the
  transactions route only. Reports, forecast, pricing and investments each
  read and write `page.url` their own way. The register, category and
  instrument lists hold the same `TransactionFilters` in component state (#326).
- **State-dependent defects need a used book.** #330 surfaced only on a worker
  whose book an earlier spec had left with a connection and a staged import.
  The import screen has many conditional sub-views, and a fresh book renders
  few of them.
- **Locale read in several places.** `getLocale()` is called per component
  for `Intl` formatting, and the document language was never set (#325).

These notes feed #284; no refactoring or refactor issues were created.
