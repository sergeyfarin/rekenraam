# Bug classes — Browser journeys and test fixtures

Part of the `validate-and-ship` review checklist. Playwright specs, sweeps,
provider stubs, worker books. Every entry shipped as a real bug in this repo.
Numbers are stable IDs cited from code comments and docs; they are not an
order and are never reused.

## 42. Browser journeys that depend on live providers or late handlers

The e2e app runs its background workers, so a real network call can add
evidence to a worker's book (#226: a live ECB fixing outranked a voided
fixture in evening CI runs). Every outbound provider goes through a
development-only stub override (`TRADING212_BASE_URL`, `MARKET_DATA_BASE_URL`)
refused outside `APP_ENV=development`. A `route.fetch()` handler still reading
a refetch when the page closes fails as "Response has been disposed" (#236):
unroute with `ignoreErrors` after each test. The once-a-minute recurring
scheduler likewise sweeps every enabled template, so a spec must not assume
nothing materializes between saving a template and its next API call (#282):
assert the occurrence the app recorded, not a `run-now` counter. Named
regressions: `TestDefaultRegistryAtSendsEveryProviderToTheOverrideHost`,
`TestLoadRefusesMarketDataBaseURLOutsideDevelopment`,
`TestRuntimeRefusesMarketDataBaseURLOutsideDevelopment`. Synthetic provider
dates must also be explicit scenario facts (#386): capture one calendar
reference and register its ex-date with the stub, rather than letting the stub
subtract elapsed UTC days independently. Keep midnight/DST stub controls and
exact provider-dividend browser dates; do not align deliberately opposite
browser/owner timezone controls.

## 49. Partial form labels matching data inside another control

The mobile account journey's `getByLabel('Name')` also matched the Parent
select when its wrapping label contained an account option with "name" in it.
Worker history made this intermittent. Match the Name field exactly on create
and edit, and seed a colliding account in the journey itself rather than
relying on another spec to expose it (`reference-data-management.spec.ts`).
Likewise, scope payment cards by a fixture's unique source identity rather
than amount/kind alone (`income-coverage.spec.ts`); repeated specs may retain
earlier equal-valued payments in the same worker's book.

## 53. Sweep that audits the sign-in screen

Every Playwright test has its own browser context, so a spec that seeds in one
test and visits routes in another is signed out and silently runs axe on the
entry screen. Sign in per test and assert the visited pathname before checking
(`route-sweep.spec.ts` `settle`).

## 57. CSP-blocked browser measurement

A screenshot helper's inline stylesheet passed isolated controls but
production `style-src 'self'` blocked it, leaving no measured glyphs (#361).
Run readable and deliberate low-contrast controls under the real policy. The
rendered-contrast helper uses CSSOM property writes and restores the original
styles; keep its 4.5:1 threshold and text-only border/icon controls. Named
cases: `rendered-contrast.spec.ts` and the real `subview-sweep.spec.ts`.

## 60. Test harness exception disclosure

A localhost provider stub returned parser input excerpts and implementation
diagnostics through `String(err)` (#373, CodeQL alert #84). Keep caught
exceptions out of HTTP responses, including development helpers. `pnpm --dir
e2e run test:stubs` exercises malformed JSON, account encoding and provider
transformation failures; both shared E2E wrappers run it before browser
journeys.

## 64. A generic heading certifying the wrong route fixture

The frontend sweep passed an instrument ID to a commodity-addressed route
(#378). Use the route's actual identity and assert the seeded instrument
heading and activity controls; an unchanged pathname and any visible heading
can still be a not-found screen. Keep these assertions locale-independent.

## 65. Cold compilation inside a browser startup deadline

The #385 fixed-clock fixture ran `go test` inside its readiness budget. A warm
local cache hid the compilation cost and both cases timed out on GitHub.
Compile test servers once in global setup, then launch their executables per
case. Retain startup limits and cache inheritance; verify with an empty
`GOCACHE`. Keep `global-setup.test.mjs` and `owner-current-reference.spec.ts`
green.

## 67. A spec finding its own entry by scanning a shared list page

The transfer-out journey looked for its bridge anywhere on `/app/transactions`
(#406). A worker's book is shared by every spec it runs, and the list's first
page holds 100 rows. Once other specs add later-dated entries, the bridge
moves to page 2 and the check fails. Locate the spec's own record through an
owned link: the POST response, then an operation or metadata link. Follow
every cursor, then assert on that record's detail view. Use `--repeat-each` to
verify, since it fills the same worker book.

## 68. A book-wide self-check read in a shared worker book

The cash-in-lieu and payment-in-lieu journeys required their own transaction
in a sample capped at 20, and the unknown-basis status journey branched on
whether the run came back `incomplete` or `failed` (#398). Other files'
findings decide both. Assert run status, counts and samples in a book the test
owns (`support/owned-book.ts`); assert a transaction's attribution through its
own diagnostic. `investments-payment-in-lieu.spec.ts` saturates an owned book
with 21 unrelated records to keep that evidence.
