# URL selection delivery and focused route-audit correction — 2026-10-21

Delivery for [#326](https://github.com/sergeyfarin/rekenraam/issues/326), with
small adjacent findings [#377](https://github.com/sergeyfarin/rekenraam/issues/377)
and [#378](https://github.com/sergeyfarin/rekenraam/issues/378), originating in
[#278](https://github.com/sergeyfarin/rekenraam/issues/278) / #260.

Source baseline: `fe80145ab3b0f8bfcbac41d2193ceb084dc6d728`.
Validated implementation: [06f83ef254761302ff2257ecf4317157866f60a0](https://github.com/sergeyfarin/rekenraam/commit/06f83ef254761302ff2257ecf4317157866f60a0).
Production shape: the static frontend embedded in the Go binary; synthetic,
provider-free books and the #266 reference fixture with isolated worker databases.

## Delivered behavior

| Screen | URL selection | Independent browser expectation |
|---|---|---|
| Budgets | `month=YYYY-MM`, `currency_id` | January 2026 / USD survives reload, in-app Back, Back from Transactions and a shared link. Default owner-local month remains server-selected; target saves refresh both default and explicitly selected month caches. |
| Recurring | `tab=templates` (Due is the default) | Templates survives the same navigation paths; choosing Due removes the tab parameter. Existing generation, posting, archive and repair journeys keep working. |
| Account register | `status`, `after_date`, `before_date` | Voided / January 2026 returns exactly the scoped reference duplicate; date clearing survives reload. The register still offers one lifecycle at a time and defaults to Posted. |
| Category detail | `status`, `q`, `after_date`, `before_date` | Voided / January / Duplicate groceries returns exactly that category's duplicate; clearing controls removes their URL parameters. |
| Instrument activity | `status`, `q`, `after_date`, `before_date` | Posted / February / a unique synthetic dividend returns exactly the route instrument's matching transaction; clearing controls removes their URL parameters. |

The five selection journeys run at 1280 px and 390 px. They check that changes
do not add history entries, unrelated `highlight` survives, copied URLs work in
a fresh signed-in page, and actual API requests retain the fixed route identity
even when unrelated query parameters name other account/category/commodity IDs.
SvelteKit 3 shallow changes are read through `page.shallow?.url ?? page.url`;
`replaceState` preserves the page state. Transaction surfaces share the existing
parser/writer, without additional API query semantics or frontend fan-out.

## Adjacent findings and dispositions

- **#377, P3, repaired.** The shared date parser accepted date-shaped but
  impossible dates. Four named parser cases failed before repair. Reusing
  `isCalendarDate` drops February 31, month 13/0 and April 31 while keeping a
  valid leap day and the existing inverted-range behavior. A real-browser
  malformed-link control covers global Transactions and all three scoped
  lists, plus invalid budget/recurring selections, with no failing API response.
- **#378, P3, repaired; focused audit repeated.** The route sweep used the
  instrument primary key where the route expects its commodity ID. A generic
  visible heading could accept a not-found or unrelated landing state. The
  fixture now uses `REF` (the commodity ID), and settling requires the seeded
  Reference Corp heading and its activity search control, including in other
  locales. Historical #278 instrument evidence did not prove that identity;
  the corrected repeated matrix below supplies the bounded evidence.

## Validation

| Command / control | Result |
|---|---|
| `./scripts/test-frontend.sh` | Zero errors/warnings; 66 catalog cases and all 713 unit cases pass. |
| Production build through Playwright global setup (`pnpm build`) | Static build, embedded web test and Go binary build pass. |
| `pnpm --dir frontend run test src/lib/transactions/transaction-url-filters.test.ts src/lib/budgets/budget-url-filters.test.ts` | All 29 parser cases pass; four impossible-date cases were observed failing before #377's repair. |
| `E2E_SKIP_BUILD=1 pnpm --dir e2e test url-selection.spec.ts route-sweep.spec.ts recurring.spec.ts transactions-workspace.spec.ts transaction-row-actions.spec.ts register-correction-chain.spec.ts --workers=2` | 47 passed; one old #223 test expected reload to reset the chosen status. That expectation conflicts with #326 and was corrected. All four route checks, seven recurring journeys, five correction-register checks and two register/category entry journeys passed. |
| `E2E_SKIP_BUILD=1 pnpm --dir e2e test url-selection.spec.ts transaction-row-actions.spec.ts --workers=2` | Final run: **31 passed**, including all 12 URL/default-budget/malformed-link cases, all 19 row-action cases and the corrected #223 lifecycle assertion. |

Together these establish **49 distinct passing browser tests**. The corrected
route matrix visits all 24 route families in light/dark at 1280, 390 and 640 px,
checks es/fr/nl/de/ru at phone and desktop widths, and forces data-route errors.
The final URL journeys also exercise in-app navigation and Back directly from
shallow selection changes, before reloading; default and explicit budget-month
saves update displayed remaining amounts immediately and survive reload.

The #223 correction verifies that Voided remains selected on reload after an
unvoid, that the unvoided row leaves that list, and that explicitly selecting
Posted shows it again. No production unvoid/lifecycle behavior changed.
Push CI is reported on the linked delivery issues; this report records local
focused validation, rather than claiming a fresh full backend/full-app gate.

## Limits

No backend service, schema, ledger calculation or API contract changed.
The repeated route checks establish only their visited landing/error states;
this is not another complete interactive-subview audit, native terminology
sign-off or release-readiness claim. Existing provider/owner/release holds
remain separate. #326 has no retained blocker. No further independent defect
or additional audit was identified in this bounded work.
