# Maintainability: layers, frontend arithmetic and request composition — 2026-10-21

**Status: fourth slice of [#284](https://github.com/sergeyfarin/rekenraam/issues/284),
bounded layer review delivered; overall audit partial.** This slice adds fresh
Go and TS/Svelte boundary scans, semantic triage of their financial/date signals
and the repeated-component request paths. It finds one confirmed frontend
admission defect, tracked separately as
[#380](https://github.com/sergeyfarin/rekenraam/issues/380). No runtime, API or
schema changes or accepted refactors are included.

Source remains `13d15eabd68ed2d3b6177512c9c8fe56c528bbf0`; working HEAD was
`6cb8b044c19a2860397ce802a93b00ea0d58eb10`. The intervening three slices changed
audit artifacts/docs only; runtime and test sources still match that baseline.
This report extends the [invariant ownership matrix](maintainability-invariants-2026-10-21.md),
not the financial contracts. The roadmap owns work order.

## Method and retained evidence

| Artifact | Scope / result | Limit |
|---|---|---|
| [Go index](fixtures/maintainability-go-layers-2026-10-21.csv), [scanner](fixtures/maintainability-layers.go) | All 311 runtime Go files; 2,354 selected import/call/operator signals | AST syntax and import aliases, not receiver/data-flow resolution or a proof of all financial policy |
| [Web index](fixtures/maintainability-web-layers-2026-10-21.csv), [scanner](fixtures/maintainability-web-layers.mjs) | All 98 runtime TS and 153 Svelte files, including scripts and template expressions; 2,771 rows | Imports, call/operator syntax and static component edges; includes unevaluated syntax such as negative type literals. Counts are not executed rules/requests |
| [Provenance and component paths](fixtures/maintainability-layer-provenance-2026-10-21.json) | Versions, index hashes, kind counts and all five repeated-component paths reaching a query | CSS, tests, generated code and archives excluded; static named imports do not resolve every dynamic component or callback |
| [Controls](fixtures/maintainability-layer-controls.mjs), [results](fixtures/maintainability-layer-controls-2026-10-21.json), [Go control](fixtures/maintainability-layer-go-control.go.txt) | Aliased imports, TS/Svelte/template calls, loop contexts, arithmetic, actual transport and comment/string negatives | Sensitivity to selected patterns, not a general architecture linter or new CI gate |
| [Focused results](fixtures/maintainability-layer-tests-2026-10-21.json), [HTTP probe](fixtures/maintainability-forecast-calendar-probe.go.txt) | 281 frontend tests; seven existing backend top-level tests under race mode; one additional disposable HTTP probe with six date cases | No full-suite, browser request-count, deployment or release certification |

Go signals: **484 imports, 951 SQL-shaped calls, 581 exact-package calls,
304 selected numeric-method names, 32 API arithmetic expressions and two float
casts**. Method-name matches include nonfinancial calls; `database/sql` imports
can be transaction types rather than SQL execution. Web signals: **1,024 calls
through API imports, 275 transport-shaped calls, 208 query factories, 243
numeric/date calls, 241 arithmetic expressions, 41 date-helper calls and 739
component uses**. Calls through API imports include pure parsers, query-option
builders and error constructors. They are not 1,024 HTTP requests.

Manual review also swept date-shaped regular expressions and string coefficient
adapters, so `BigInt` detection did not silently stand in for all exact arithmetic.
The rows and the grouped dispositions below are navigation/review evidence;
they do not claim exhaustive interprocedural validation of every financial path.

## Backend layer dispositions

Paths in this table are under `backend/internal/`.

| Boundary / inspected evidence | Disposition and consequence |
|---|---|
| Handler SQL/float parsing and reverse layer imports | All 951 selected SQL-shaped calls occur in `db` (101 files); none occur in `api` or `app`. No API `database/sql` import or selected float parse/cast occurs, and `db` imports neither `app` nor `api`. App's 31 `database/sql` imports include transactional callback/types; they are not 31 SQL execution leaks. Retain the SQL boundary; do not add interfaces merely to hide those types |
| API → `db` dependencies | Nine API files import `db`: backups, basis resolution, cash in lieu, gain impact, gains runs, split reads, investments, middleware and recurring. Inspected uses are domain errors, constants and record/impact types; service methods execute the work. This is type/error coupling, not evidence of direct SQL or a reason to wrap every DTO in another layer |
| API arithmetic signals | The 32 operator expressions are strings/URLs/error copy, query indexes, upload limit + 1 and rate-pair slice capacity. Exact constructors/conversions in corporate-action and gain-impact responses represent already calculated amounts. Those are distinct from the actual aggregation below |
| **Financial aggregation in a response builder** | `api/investments.go:gainsProjectionResponse` groups realized gains by cost currency, aligns scales through `exact.ScaledInt`, counts unknown basis, omits incomplete totals and enforces int64 output range. It serves operational/alternative responses and saved-output formatting. This is financial policy in an API helper, even though the HTTP handler itself delegates to a service. The literal service-only policy target is therefore **not met** |
| Scope of that aggregation candidate | `app/investment_gains_statement.go` independently groups realized results by currency and tracks unresolved basis; FX aggregation has its own conversion/coverage contract. #284 criterion **3** applies to repeated exact/complete-only financial aggregation. A **later, S/M candidate** is an application-owned native realized-total read model, preserving ordering, mixed scales, unknown counts/nulls, range errors and existing saved bytes. This is not an accepted refactor, a new returns formula, or permission to merge native/FX/statement contracts. No incorrect total was reproduced |
| Transaction-coupled repository policy | `db/investments.go` allocation/proration, `investment_transfer_internal.go` allocation/method admission and the shared investment writer remain the owners identified in slice 3. Moving them outside the admitted write solely to satisfy a directory rule is **rejected**. Any eventual extraction must preserve the same transaction/facts and characterize rounding/knowledge first |
| Repository read policy | `valuePositions`, effective movements and observation selection already have shared bounded owners. Current/dated/alternative inputs and immutable saved evidence retain their different contracts. No additional universal projection layer is justified by this scan |
| Float-shaped signals and documentation | The two selected casts are backup-space calculation and pricing retry duration, not coefficient conversion. No `shopspring/decimal` import exists in the baseline. `docs/conventions.md` still named that library; it now names the shipped `internal/exact` arbitrary-precision helpers, consistent with ADR 0009 and July's delivered arithmetic consolidation. This corrects stale documentation; it introduces no new arithmetic policy |

The passing HTTP mixed-scale/property and above-JavaScript-safe-integer tests
constrain the aggregation candidate. A syntactic absence of handler SQL is not
certification that every API helper is policy-free.

## Frontend arithmetic and date dispositions

Paths below are under `frontend/src/lib/` unless stated otherwise.

| Signal / semantic trace | Disposition |
|---|---|
| `money/amount.ts`, `money/format.ts`; transaction/category consumers | Shared parsing, exact comparison/summing, editable formatting and read-only formatting already exist. Category amounts select the category's own legs and use `sumByCommodity`; backend report figures remain authoritative. Do not introduce a frontend report engine or new decimal dependency |
| 131 `Number` calls; 33 `BigInt` calls | Reviewed `Number` inputs are IDs, bounded ratio integers, calendar fields or already bounded chart quotients. No direct monetary coefficient-to-Number conversion was identified. This is not a proof of arbitrary ID precision or aliased dynamic flows. Sixteen `BigInt` calls remain outside `money/` in the five modules below |
| `investments/trade-economics.ts:parseCorrectionLotChoices` (eight BigInt calls) | Repeats scaled comparison against available lot quantity and exact selected-total matching. `internal-transfer-amounts.ts` already uses `compareScaledAmounts`, so the per-lot comparison can reuse an existing owner. **Later S candidate, criterion 3**: consolidate identical comparison/range leaves, retain distinct total/election rules and current error precedence. No numerical failure was found |
| `pricing/form.ts:parsePriceAmount` (two BigInt calls) | Repeats positive/int64 admission using a literal maximum; `coefficientSign`/`fitsInt64Coefficient` already own those leaves. Preserve pricing's scale-18 model and its return/error contract. Include this in the same later leaf-reuse disposition, not a new parser or an accepted refactor issue |
| `forecast/forecast-model.ts`, `reports/report-series.ts`, `reports/spending.ts` (two BigInt calls each) | Rescale coefficients exactly, then convert bounded fixed-point quotients to chart coordinates. Chart geometry has different zero baselines, signed/magnitude treatment and precision denominators. Displayed/exported money still uses exact source coefficients. **Retain for now**; similar powers of ten do not justify making canonical calculations in the browser or merging chart policies |
| Other arithmetic expressions (201 outside `money/`) | Counters, ID sorting, keys/strings, selection indexes, calendar arithmetic, chart geometry, basis-point display and the explicitly named coefficient adapters. Count is syntax, including nested operators/type literals, not 201 separate financial formulas. No additional balance/gain calculation engine was found in this sweep |
| `investments/split-ratio.ts` | Whole-number ratio sides are bounded to 1,000,000,000. Two exact percentage parsers share decimal-mark/normalization syntax but intentionally differ on zero/100% endpoints. `basisFractionPercent` changes representation without floats. Retain the explicit fraction contract; replacing it with a localized amount parser would change accepted input. Any small shared leaf must preserve both endpoints and the existing decimal-mark behavior |
| `investments/provider-dividend.ts:grossPrefill`; `imports/short-link-panel.svelte:decimal` | First pads a same-currency exact provider estimate only when representable; second decomposes a canonical API dot-decimal before locale display. These are residual exact string adapters outside `money`, including a local Svelte helper. Record that the literal shared-helper convention is not fully met; no precision defect was reproduced. Do not substitute an editable locale parser for trusted machine-decimal evidence without characterization |
| **Forecast `history_complete_from`** | `forecast-model.ts:131` and `forecast-screen.svelte:112` use regex + `Date.parse`. The real URL parser accepts `2026-02-29`, `2026-02-31` and `2026-04-31`; shared `isCalendarDate` rejects them. A real HTTP probe rejects all three with 400/`VALIDATION_FAILED`; ordinary `2026-02-28` and leap `2024-02-29` succeed. **Confirmed admission/UX defect, #380, P3**. Backend strict dates remain correct; no wrong financial result or durable corruption established |
| Remaining date-shaped regexes | Budget month routing bounds month/year; API budget/datetime query `enabled` checks and basis-provenance shape checks are early readiness checks, not authoritative date admission. Native date fields and backend validation still matter. Do not call a shape test a calendar validator or expand #380 to speculative failures without a real consumer |
| Other `Date` calls and 41 date-helper calls | Ten constructors cover timestamps, month-name fixtures and UTC-pinned calendar display; one `Date.parse` formats a PAT timestamp, and two `Date.now` calls bound polling. The three investment day displays pin UTC, so browser zone does not shift the calendar date. Owner defaults continue through server `useOwnerToday`; no browser-clock financial default was found. #380 owns the two actual date-admission calls |

No general “frontend arithmetic is centralized” claim is made: the explicit
exceptions above remain inputs to #284's final disposition. Existing exactness
tests pass; a passing suite does not erase a reproduced input mismatch.

## Per-item requests and component expansion

There are **208 query factory sites in 79 files**, not 208 simultaneous requests.
All **275 transport-shaped sites** are in `lib/api/`. No query factory or direct
transport call is syntactically inside a scanned iteration. Calls through API
imports do occur under loops, so those were triaged rather than declared fan-out:

| Loop group | Sites | Disposition |
|---|---:|---|
| `imports/csv-investment-review.svelte`, `imports/payee-resolution.ts` | 1 + 1 | `parseNormalized` is local source-data parsing, no request |
| `routes/app/import/+page.svelte` | 6 | `parseResolution`/`parseNormalized` in maps, loops and markup; local parsing, no request |
| `investments/event-suggestions.svelte` | 3 | Link/accept/ignore commands run in owner action callbacks, not while rendering the list |
| `recurring/recurring-screen.svelte` | 10 | Bulk-post review/submit sequentially obtains each selected draft's impact and posts it; other sites are local refusal/review helpers or explicit inspect/retry/update/run actions. This is action-size-dependent work, not page-data loading for every due row. It is not reclassified as a single bulk transaction |

The 739 static component uses include **91 under iteration**. Following their
named-import edges to query-owning descendants yields five paths, all retained
in the provenance JSON:

| Repeated component path | Mount/admission condition | Disposition |
|---|---|---|
| Forecast points → `ForecastEvents` | `openDetailKey === detailKey`, one selected point/series | One selected shareable detail; queries its paginated composed events |
| Suggestions → `ProviderPaymentInLieuCompletion` | `completing === suggestion.id`, matching shape | One selected completion form; reference accounts/currencies loaded for it |
| Suggestions → `ProviderDividendCompletion` | Same selected suggestion ID, other shape | Same; not one form per rendered suggestion |
| Import rows → `SplitLinkPanel` | `splitLinkRowID === row.id` | Selected source-row candidate read |
| Import rows → `ShortLinkPanel` | `shortLinkRowID === row.id` | Selected source-row candidate read; independently selectable from split panel |

**No unbounded per-rendered-item page-data request path was identified in this
static sweep.** Query caching, selected-component gates and explicit command
loops differ. This does not establish a browser request-count upper bound for
every interaction, resolve arbitrary dynamic components, or prove absence of
backend N+1 SQL. No new composed endpoint or generic query wrapper is justified
by these results.

## Reproduction and validation

Prepare fixed inputs with the [slice-2 tool](fixtures/maintainability-clones.py)
and installed, frozen frontend dependencies. It extracts original Go/TS/Svelte
files from Git; generated projections are not substituted for template parsing.

```sh
python3 docs/reviews/fixtures/maintainability-clones.py \
  --ref 13d15eabd68ed2d3b6177512c9c8fe56c528bbf0 \
  --output /tmp/rekenraam-284-layers-reproduction
GOCACHE=/tmp/rekenraam-go-build-cache go run \
  docs/reviews/fixtures/maintainability-layers.go \
  -root /tmp/rekenraam-284-layers-reproduction/source \
  > /tmp/rekenraam-284-layers-reproduction/go.csv
node docs/reviews/fixtures/maintainability-web-layers.mjs \
  /tmp/rekenraam-284-layers-reproduction \
  /tmp/rekenraam-284-layers-reproduction/web.csv
node docs/reviews/fixtures/maintainability-layer-controls.mjs \
  /tmp/rekenraam-284-layers-reproduction \
  docs/reviews/fixtures/maintainability-web-layers.mjs \
  /tmp/rekenraam-284-layers-reproduction/controls.json
```

For the Go sensitivity control, copy the retained `.go.txt` into a disposable
`backend/internal/api/control.go` and run the same scanner on its root. Expect
two imports and one each of exact-package, SQL-shaped, float-call, float-cast
and API-arithmetic signals; string/comment decoys add no call sites. It is a
parser fixture, deliberately not a type-correct application package.

To reproduce the HTTP probe without adding a production test or changing the
baseline, extract `git archive 13d15eab backend scripts fixtures` to a fresh
temporary directory, copy the retained HTTP `.go.txt` to
`backend/internal/api/maintainability_forecast_calendar_probe_test.go` there,
then from that root run:

```sh
GOCACHE=/tmp/rekenraam-go-build-cache ./scripts/go-backend.sh test \
  -race -p 2 -timeout=5m ./internal/api \
  -run '^TestMaintainabilityForecastCalendarAdmissionProbe$'
```

**Passed:** 281 unit cases across 13 frontend files, seven selected existing
backend top-level tests under race mode, and all six HTTP date cases in the
additional disposable probe. The retained results name files/tests. Coverage
includes exact money/display, mixed-scale gains, safe-string wire values,
lot choices, provider evidence/percent boundaries, charts, calendar parsing,
forecast validation/read-only behavior and exact learned allocation. Go race
mode does not instrument SQLite C. #380's current frontend mismatch is retained
as an observed audit control, not masked by these passing tests.

**Artifact checks passed:** all 562 original input hashes; byte-identical second
indexes and frontend controls; index counts/source coordinates; the five
repeated-component paths; both scanners' positive/negative controls; all 115
relative links/anchors across the seven touched Markdown documents; helper
formatting/syntax; `./scripts/check-post-merge.sh HEAD` and `git diff --check`. These tools are
deliberate audit artifacts; no dependency or CI-gate change is introduced.

## Remaining acceptance and follow-ups

#284 remains open for dead-code/single-caller analysis, broader fixture/oracle
and test-duplication work, any enforcement sites those reviews uncover, and
final ranked investment/reporting dispositions with owner review. This slice
adds the native realized-total and frontend comparison/range candidates to that
review; it does not create accepted refactor issues. #380 separately owns the
confirmed forecast defect. The previously filed P3 #379 payment-density audit
is unchanged. Neither follow-up is a blanket R13 prerequisite. R13 specification
may proceed; implementation still needs its own calculation contract and #284's
investment/reporting dispositions. Owner/release gates remain independent.

Subsequent evidence: the [fifth slice](maintainability-reachability-2026-10-21.md)
delivers the bounded reachability and named-interface/options review. Its
separate test roots and frontend export-visibility view constrain future
cleanup; this report's historical remaining-work list does not undo that delivery.
