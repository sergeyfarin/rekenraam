# R18 reporting policy

Status: **bounded R18 delivery complete**, reconciled 2026-10-17. All six
slices and #237/#238 are delivered; #246 adds saved schema-3 cash-attribution
disclosure. Returns/allocation remain R13; saved alternative/FX projections,
broker comparisons, tax policy and formal postings remain outside this contract.

ADR [0012](../adrs/0012-investment-ledger-subledger-and-reporting-boundaries.md)
governs this plan. The [roadmap](../roadmap.md#r18--reproducible-investment-basis-and-gains-projections)
owns sequencing; [#203](https://github.com/sergeyfarin/rekenraam/issues/203)
owns the bounded observation-selection delivery. The delivered slices below
add operational and alternative-method basis profiles and explicit immutable
operational report runs; tax calculations remain outside them.

## Maintainability handoff to R13 (#284)

The [third #284 slice](../reviews/maintainability-invariants-2026-10-21.md)
records existing reusable owners: current, dated and alternative gains already
share `db/investments.go:valuePositions`; live quote admission uses
`db/observation_selection.go`. R13 should start from those boundaries rather
than presume independent valuation implementations need consolidation. Signed
cover cash, released opening proceeds and disposed basis use different tuples;
current effective history, live observation knowledge and immutable saved
operational bytes also retain their documented contracts. This evidence neither
defines return equations nor accepts a new generic reporting engine. Final
investment/reporting dispositions remain in #284, the calculation contract in
[#119](https://github.com/sergeyfarin/rekenraam/issues/119), and order in the roadmap.

The [fourth #284 slice](../reviews/maintainability-layers-2026-10-21.md) also
records native realized-gain totals/unknown counts in the API response builder.
That is a bounded candidate for application ownership, not a requirement to
replace the existing valuation owner or merge FX/statement semantics. Any move
must preserve mixed-scale totals, unavailable/null disclosures, range failures
and immutable saved output. No incorrect total or accepted refactor is claimed.

The [fifth slice](../reviews/maintainability-reachability-2026-10-21.md) separates
standalone test-root wrappers from production consumers. Current reads resolve
through a dated boundary; a production saved-run path also reaches the existing
`valuePositions` owner. Unreachable convenience wrappers do not imply that
valuation is unused or that snapshot/transaction boundaries can be removed.
Their possible retirement belongs with the remaining fixture/oracle disposition.

## Live observation selection: `live_raw_v1`

One SQL owner, `backend/internal/db/observation_selection.go`, supplies daily
and latest candidates to holdings/gains, reporting conversions, currency
settings, pricing diagnostics and constant-rate forecasts. It preserves all
original series, observations, corrections, withdrawals and journal evidence.
It never calls a provider or writes financial records.

Admission happens **before** ranking:

- Only currently non-voided observations recorded at or before the read's UTC
  knowledge cutoff participate. Current withdrawals apply even to a backdated
  valuation. This is a live observation admission boundary, **not** a historical
  reconstruction of what was active before a withdrawal.
- The observation date cannot exceed the consumer's valuation ceiling. For
  current values that ceiling is the server's saved owner-zone calendar date.
- Currency bases accept `manual`, `valuation_override`, `official_fixing`,
  `spot_mid`, and `trade_implied`, with `raw` or `not_applicable` basis.
- Security/other countable bases accept `manual`, `valuation_override`,
  `close`, `nav`, `spot_mid`, and `trade_implied`, with **raw** basis.
  The existing unadjusted `trade_implied` fill marker `not_applicable` is also
  admitted; it does not denote split/dividend-adjusted units.
- `bid`, `ask`, `adjusted_close`, and split/dividend/total-return adjusted
  series cannot price unadjusted ledger units. A manual flag cannot make an
  incompatible quote admissible. Original incompatible evidence stays in
  history/export.

The latest eligible valuation date wins. Within that date: manual flag or
valuation override first, other non-approximate observations second,
approximate observations third; later recording instant then larger observation
ID break equal ranks. A newer approximate date therefore beats an older manual
date. Source assignment controls fetching, not retroactive preference between
stored observations. There is no hidden provider-specific rank. A provider's
publication/observation timestamp is provenance, not the recorded-at boundary.

## Consumer contracts and intentional differences

| Consumer | Valuation and knowledge boundary | Age/coverage rule |
| --- | --- | --- |
| Current positions and unrealized gains | Owner-local today and UTC read start. Holdings retain the current operational projection. Gains replay quantity/basis to the resolved valuation date (today by default), excluding later events, independently of disposal-date filters. | No age rejection: retain the latest compatible price, explicitly mark it stale after seven calendar days. Missing/future-only/incompatible/not-yet-known evidence leaves value/gain absent with a reason; quantity and cost currency/basis remain. |
| Currency settings latest rates | Owner-local today and UTC read start. | Retain old compatible rates with their date and the same seven-day stale warning. This is a management view, not a historical conversion guarantee. |
| Net worth series | Each bucket end, capped at owner-local today; observation knowledge limited to UTC read start. | Nearest on-or-before quote within requested age, default seven calendar days, inclusive. No eligible rate means no converted bucket total. Original commodity figures remain. |
| Spending and cashflow | Each posting entry date, with observation ceiling capped at owner-local today; UTC read start knowledge. | Same requested age window. Convert each posting with exact arithmetic then sum; incomplete converted groups/totals are absent. Original currency figures remain. |
| Constant-rate forecast | Owner-local forecast as-of date and UTC computation start. | Currency-only rates, fixed seven-day maximum age; one selected as-of rate stays constant across the future horizon. Missing/old rates omit conversion, retain exact source-currency series. This does not predict future FX. |
| Pricing workspace diagnostic | Explicitly displayed `as_of` date (owner-local today by default) and UTC read start. An explicitly future diagnostic date is allowed. | Configured `staleness_max_days`, independent of holdings' warning and reports' requested window. An old candidate remains inspectable with age/status; missing candidates disclose incompatible/future/knowledge reasons. History shows all original evidence. |

Age is a difference of calendar dates, not elapsed owner-zone hours. Seven days
is admissible; eight is too old for seven-day reports/forecasts. Holdings use
seven days as a **warning**, not a value gate. Pricing policy changes do not
silently reconfigure historical reporting or holdings.

Reporting responses identify `selection_policy`, `valuation_as_of`,
`knowledge_cutoff` and each selected observation/requested date, including
approximation. A `future_only` gap can also mean the report has a rate for a
later bucket but none at/before this posting. `no_observation_in_window` with a
nearest earlier date identifies old evidence; without one it identifies missing
evidence. Other reasons are `incompatible_quote_basis` and
`not_known_at_cutoff`. Holdings expose the same reasons plus `no_price` and
retain their existing `unrepresentable` arithmetic outcome. Screens disclose
selection and age policy and translate unavailable/stale reasons.

## Reproducible profile boundary

A reproducible projection must pin purpose, basis method/version, effective period,
valuation date, price knowledge boundary, quote/source policy, FX method,
staleness, reporting currency and rounding, and reconstruct retirements and
ledger/basis knowledge or persist an immutable run/snapshot. A timestamp on
this live selector alone cannot do that. No alternative profile may rewrite
operational elections, journal balances, tax liabilities or reconciliation.

## Basis and valuation profiles (#118)

A **profile** is a named, versioned, read-only rule set. Every projection
response names the profile it ran under and echoes each resolved field, so two
figures can be compared only when their descriptors match. Profiles never post,
never change lots, elections, prices or reconciliation, and never claim tax
compliance.

| Field | Meaning | Values defined so far |
| --- | --- | --- |
| `name` / `version` | Stable key; a rule change is a new version, never a silent reinterpretation. | `operational_v1`, `alternative_method_v1`, `person_pooled_average_v1` |
| `purpose` | What the figure is for. A label, not a compliance claim. | `operational`, `alternative_method` (later: `broker_comparison`, `tax_estimate`) |
| `basis_method` | How disposals consume basis. | `operational_elections`: each committed disposal's recorded method, tier and allocations (ADR 0012); comparisons explicitly select `fifo`, `lifo`, `average_cost` or `person_pooled_average` without re-election. |
| `valuation_as_of` | Calendar date the open positions are replayed to and priced at. | Any date up to owner-local today; default today. |
| `positions_as_of` | Which events build the holding. | Every effective event dated on or before `valuation_as_of`, replayed in a rolled-back read (the #166 dated-holdings replay, both sides). |
| `knowledge_cutoff` | UTC instant limiting admitted **observations**. | Read start. |
| `ledger_knowledge` | Which book history the replay sees. | `current_effective`: today's effective history — a later correction, reversal or withdrawal restates an earlier valuation date. Not reproducible as known then; see *Reproducibility* below. |
| `quote_policy` | Observation selection. | `live_raw_v1` (above). |
| `staleness` | Age rule for the selected price. | Holdings rule: retain the latest compatible price on or before `valuation_as_of`; mark it `stale` past seven calendar days; never a value gate. |
| `fx_method` / `reporting_currency_id` / `fx_max_age_days` | Conversion of results. | `none` in original figures; additive operational conversion under `valuation_date_all` or `historic_basis_valuation_close`, in the selected currency with an explicit inclusive age window (default seven days). |
| `rounding` | Arithmetic. | Exact; market value is one `exact.MulDivRound` half-up step at the deeper of quantity×price and basis scale. |

### Research recommendations

- **Which measure is authoritative where.** The portfolio dashboard shows the
  *position* view: market value, remaining operational basis and unrealized
  gain at a stated valuation date with the price date and stale marker beside
  it. Realized gain belongs to a *period* report (disposal-date range),
  alongside income and the change in unrealized gain between the period's
  start and end valuation dates. The dashboard never mixes a period's realized
  figure into a point-in-time total.
- **Jurisdiction or purpose without a tax engine.** Users choose a named
  profile, whose `purpose` and method are shown with every figure; a profile
  may carry a free-text jurisdiction note. Rekenraam computes the method the
  profile names and says which one it is — never "your taxable gain". Regimes
  whose pool spans accounts (UK s104, Canadian ACB, French PMP) are a separate
  profile method over the person's positions, not a re-election of the
  per-account operational lots.
- **Unrealized movement without flip-flop.** Unrealized gain is always
  presented *as of a valuation date with the price that produced it*, never as
  a bare "now" figure. Same-date selection is deterministic (`live_raw_v1`), so
  a refresh changes a figure only when a newer or higher-priority observation
  arrives, and the displayed price date says so. Period reports use the
  period-end date, so later refreshes cannot move a closed period's figure
  unless evidence for that date changes. Stale prices remain visible and
  marked rather than disappearing.
- **I-04 (formal postings).** Out of R18. ADR 0012 already forbids silent
  posting; an explicit linked reclassification workflow is not scheduled
  until a user need for formal gain statements is evidenced.

### Reproducibility

`operational_v1` is a *live* projection: rerunning it for the same
`valuation_as_of` returns the same figures until the book's effective history
or the admitted observations for that date change, and its descriptor says
which knowledge it used. Reproducing a figure *as it was known at an earlier
instant* is provided by an explicitly saved immutable run (#234, below). Live
reads keep using current effective history and never claim historical
reconstruction. An unsaved earlier read cannot be recovered retroactively.

The [completion audit](../reviews/r18-completion-audit-2026-10-15.md) found a
separate live-read consistency defect: shared observation cutoffs do not keep
the separate operational, comparison and FX reads on one input state when a
correction or withdrawal interleaves. [#237](https://github.com/sergeyfarin/rekenraam/issues/237)
owns that repair. Saved operational runs already use a consistent isolated
snapshot and retain their reproducibility guarantee.

### Period statement (#231)

A period `from..to` reports, per currency and under the same profile:
opening unrealized gain valued at the day before `from`; realized gain of
disposals dated in the period; investment income as the gross income-account
credit of effective cash and reinvested dividends dated in the period
(withholding is separate; a reversed or replaced dividend is not effective;
return of capital reduces basis and is not income); closing unrealized gain at
`to`; their change; and the total of realized, income and change. Each
valuation states its position, unpriced, unresolved and stale counts and the
range of price dates used. Any figure resting on a missing price or unresolved
basis is null with its reason, and so is every figure built from it; a
currency with nothing held at a date has a known zero.

### Return-of-capital period disclosure (#238)

**Decision (2026-10-15): preserve the existing limited-scope formula and expose
excluded capital-return excess beside it.** The period result remains realized
gain + gross dividend income + change in unrealized gain. It is not a complete
economic return, a tax calculation, or an instruction to post income. Basis
reduction already enters the dated position's basis and must not be added again.
No new combined economic-return or tax figure is inferred.

Each currency row includes `capital_return`: exact `receipt`, nullable
`reduction` and `excess`, `receipt_count`, `unresolved_count` (receipts with an
unknown effect) and `unknown_lots` (unknown lot effects). These describe effective
receipts **paid** within the inclusive `from..to` period, matching #171's payment
filter. Associated reductions are not a sum of basis movements dated in that
period: basis changes on `effective_on`, which can precede the payment period.
Top-level `capital_returns` retains each receipt's effective/payment dates,
operation/revision/audit identity, exact source evidence and complete latest
allocation set. Corrections replace retired operations; reversals remove them;
upstream replay supplies its latest whole set. All readers share #237's capture.
A currency with only a payment in the period still gets a row.

An unknown lot effect withholds the entire receipt's reduction/excess and the
currency's corresponding aggregate, never supplying known-only sums as totals.
Receipts remain exact and independently known lot effects stay inspectable in
the evidence. No receipts means known zero, distinct from unknown coverage.

Example: one share with EUR 7 basis and EUR 7 market value before June, EUR 10
return of capital effective/paid during June, unchanged closing price. The
statement shows EUR 7 limited-scope result, EUR 10 receipts, EUR 7 associated
basis reduction and EUR 3 **excluded excess**. Nothing calls that excess taxable
income or posts it. If effective May / paid June, the basis change enters May's
valuations; June discloses the receipt/reduction/excess cohort without moving that
change to June or claiming its result is comprehensive.

Reporting FX adds separate `capital_return_receipt`, `capital_return_reduction`
and `capital_return_excess` figures, retaining component operation IDs, source
amounts and ordinary rate provenance. `valuation_date_all` converts all three at
the statement's closing date. `historic_basis_valuation_close` uses payment date
for cash/excess and effective date for associated reduction; the reduction is
not translated at a lot's acquisition date. Same-currency amounts are exact
no-ops; foreign components use the existing inclusive age window, raw evidence
and component half-up rounding. Unknown source amounts and missing/stale FX
withhold the affected figure; they do not change original-currency amounts or
silently erase an independently available limited-scope result. Converted
receipt minus converted reduction need not equal converted excess when dates
or rounding differ. None enters the gain formula.

**Saved-document versioning:** new statements save as schema **2**, including
this disclosure and its source evidence, still in original currencies and under
`operational_v1`. Gains documents continue to use schema 1. Forward migration
0008 widens the document-version CHECK and copies all existing rows, exact JSON,
hashes, IDs and attribution unchanged; immutable guards remain. Downgrade refuses
while schema-2 documents exist. Schema-1 statements retain their original bytes
and meaning; the UI explicitly says their receipt/excess coverage is unavailable,
never zero. It does not fetch current evidence into an old report. Both versions
remain retrievable, exportable, backed up and checked for document integrity;
no bundle-format bump is needed because the schema-12 CSV already carries each
run's document version and exact JSON. Saved alternative/FX projections remain
out of scope. The six-locale 390 px screen places the scope note beside native
and converted totals, shows the three separate figures and unknown counts,
and exposes dated allocation evidence.

Named validation: `TestGainsStatementCapitalReturn*`,
`TestGainsFXStatementCapitalReturnDatesCoverageAndUnknown`,
`TestInvestmentGainsStatementDisclosesExcludedCapitalReturnAPI`,
`TestInvestmentGainsStatementAPIAbsentFiguresAreNull`,
`TestCapitalReturnStatementSchemaUpgradePreservesLegacyBytesAndNewFrozenDisclosure`
and `investments-capital-return-statement.spec.ts`.

### Unclassified imported dividend cash (#246)

Income stays the gross of effective named dividends only. Beside it, each
currency row discloses `unclassified_dividends`: the count and exact net cash
of committed imported dividend payments dated (by transaction/payment date) in
the period that still post as ordinary cash, identified only through #244's
import provenance (`ImportedDividendCashFallbacks`), never memo text. Their
gross and withholding are unknown, so the cash is never added to income, the
limited-scope total or any converted figure; a currency with only such cash
still gets a row. Dividend review (#242) links the payment to a named dividend,
which moves it from the disclosure into income at its gross. The UI links the
line to the income coverage page.

The reporting-currency projection does not convert this cash (it is not
income); the screen states beside it how many such payments are excluded and
points to the native rows. New saved statements use schema **3** with the
disclosure frozen; forward migration 0010 widens the document-version CHECK
and copies every row, exact JSON and hash unchanged, and downgrade refuses
while schema-3 documents exist. Schema-1/2 statements keep their bytes and the
UI says this disclosure is unavailable there, never zero.

Named validation: `TestGainsStatementDisclosesUnclassifiedImportedDividendCash`
(fallback, period boundary, repair, frozen saved run), `TestInvestmentGainsStatementAPI`,
`TestCapitalReturnStatementSchemaUpgradePreservesLegacyBytesAndNewFrozenDisclosure`
and the six-locale 390 px statement check in `income-coverage.spec.ts`.

### Consistent live input state (#237)

Each live gains or period-statement request captures one consistent disposable
SQLite input image before composition. Native operational results, capital-return
allocations, alternative/person-pooled comparisons, both statement boundaries,
disposals, income, historical basis, currency metadata and reporting rates all
use that image. Corrections, reversals and observation withdrawals committed
during composition affect subsequent requests, never a later stage of this one.
The application-service entry points are `ReadGains` and `ReadGainsStatement`;
standalone conversion/comparison stages must not compose a live response from
previously fetched inputs and live repositories. `GainsStatement` itself also
captures once when called outside an existing captured read.

This reuses #234's native backup capture and the existing replay, observation,
basis, comparison and FX owners. Replay's temporary writes require a writable
isolated image; a read-only WAL transaction cannot run those readers unchanged.
A live writer transaction spanning all stages would serialize unrelated writes
through replay. Instead, only capture occupies the live pool's connection;
composition runs against the private image and releases it on success, failure
or cancellation. The image uses a private 0700 directory and 0600 database and
is removed with its sidecars afterward. No GET saves a report run or audit event.
No provider call occurs. Saved-run fingerprinting is skipped for live reads.

The chosen implementation copies the full database per request: O(database
size) I/O and temporary disk, plus replay's temporary WAL and processing costs.
Capture/open/cleanup measured 66/112/210 ms for 1.6/18.4/68.7 MB synthetic images
on the validation host; these are not end-to-end report latencies or a production
capacity claim. The native copy has the existing 30-second deadline and honors
request cancellation; the source connection is released before replay. See the
[review and repeatable benchmark](../reviews/live-gains-consistency-2026-10-15.md).

`ledger_knowledge: current_effective` and the live/saved distinction remain:
the cutoff admits price evidence, but does not reconstruct earlier ledger or
retirement knowledge. The response is internally consistent, not retained.
Only an explicit immutable run preserves an output for later retrieval; no
unsaved past response can be recovered by supplying its cutoff.

### Alternative-method comparison (#232)

`GET /api/v1/investments/gains?alternative_method=fifo|lifo|average_cost`
adds an `alternative` object beside the unchanged operational response. It
contains its own `profile`, `realized`, `unrealized` and `realized_totals`.
`alternative_method_v1` has purpose `alternative_method`; `basis_method` is
the selected method. Every disposal, including an operational specific-lot
election, uses that method (`specific_lot_fallback: selected_method`). This
is a comparison, never an operational re-election or a tax computation.

The optional inclusive `from`/`to` disposal period is echoed in the alternative
descriptor; omitted bounds are unbounded. It is independent of
`valuation_as_of`. Replay starts at the first effective event, rather than at
`from`, so earlier purchases and disposals determine the basis used inside the
period. Both long and named-short positions use their existing exact disposal
algorithms. Merger cash keeps its recorded disposal date and zero quantity,
independently of its cash posting's payable date. Unknown basis remains
unresolved, never a gain against the known subset.

A single merged stream orders effective events by date, correction-root slot
and effect. Ordinary holding transfers retain each recorded link's quantity
and destination identity/acquisition-date evidence, but deplete using the selected method
(`transfer_policy: selected_method_fixed_link_quantities`). Hypothetical
carried basis reaches a destination before it opens, including onward and
return transfers; no operational transfer revision or bridge is written.
Source-lot and pooled-lot destinations are both supported. Structural actions
and explicit capital-return entitlements retain their sourced lot relationships
(`structural_action_policy: recorded_entitlements`). If a hypothetical method
leaves those entitlements impossible, the comparison refuses with HTTP 409
`CONFLICT`, naming the dependent operation, and returns no partial projection.
It never silently changes the sourced action to fit a method.

The transaction is always rolled back, on success or failure. Temporary lot
state, method locks and generated replay effects never survive the read;
operational lots, elections, transfer links, journals, reconciliation, prices
and audit rows stay unchanged. The profile retains `current_effective` history,
`live_raw_v1` observations (using the operational read's same price knowledge
cutoff and valuation date), original cost currencies, the seven-day stale
warning and exact half-up valuation. As with `operational_v1`, later corrected
history or observation evidence can restate the result; immutable operational runs ship separately in #234. Cross-account person-level pooling is defined below (#235). The period statement
endpoint continues to use `operational_v1`.

The Gains screen offers an optional method selector and compares realized
currency totals and each position's unrealized gain in adjacent columns. It
uses the same applied period and valuation controls as the operational figures,
shows the selected method and read-only policy, and retains loading, empty,
error and unavailable states in all six locales.

### Reporting-currency gains (#233)

Both gains endpoints accept `reporting_currency_id`, `fx_method` and optional
`fx_max_age_days` (1–365 calendar days, default seven). Currency and method
are required together; blank, duplicate, orphaned and invalid FX options are
rejected. Omitting all three preserves the original response shape. The
additive `reporting` object has its own operational profile, reporting currency,
FX method, window, valuation/period and rounding descriptor. Original-currency
figures and the alternative-method comparison remain available; this slice
converts the operational profile, not hypothetical alternative allocations.

- `valuation_date_all` converts each signed basis, proceeds, income and market
  component at the selected valuation date. The statement uses its opening
  date for opening unrealized and its closing date for closing unrealized,
  period proceeds/basis and income.
- `historic_basis_valuation_close` converts each operational lot's allocated
  disposed or remaining basis at its effective **original acquisition date**;
  proceeds (including the signed cost of a short cover) and gross income at
  their event dates; and signed market exposure at the valuation date. Long
  basis subtracts and short opening proceeds add. Operational pooling,
  subsequent basis adjustments and structural elections remain as recorded;
  this method converts the resulting per-lot amounts rather than creating a
  separate FX cost-basis pool. Transfers retain their effective carried original
  date (including the sourced latest-date convention of pooled transfers).
  An unknown original date leaves foreign basis and dependent gains unavailable;
  it never falls back to the transfer date. This is a named comparison rule,
  not a jurisdiction's tax or historical-FX accounting engine.

Rates use `live_raw_v1`, sharing the source projection's observation knowledge
cutoff. The nearest compatible observation on or before each requested date
must be within the inclusive window. Every signed foreign component is rounded
once, half-up on magnitude, to the reporting currency's standard scale, using
`exact.MulDivRound`; figures and totals sum those rounded components exactly.
Same-currency components are exact no-ops, retaining sub-cent precision without
requiring rates or acquisition dates. The descriptor names this rounding
`component_half_up_currency_scale`. Overflow of the supported exact coefficient
returns 422 `LEDGER_OVERFLOW` without changing original projections.

Each converted gain retains its component role, lot ID where applicable,
source currency and exact source/converted amount, requested date, observation
ID/date, exact rate and base quantity, approximation and derivation flags.
Missing, stale, incompatible, future-only or not-yet-known rates, unknown basis,
unknown acquisition dates and missing market values carry explicit reasons;
stale gaps also disclose the nearest prior date. A component gap makes its
figure null; a single incomplete figure makes the relevant aggregate null.
Known amounts in other currencies remain visible, but never form a partial
headline total. The statement combines converted opening, realized, income and
closing figures into exact unrealized change and period total, retaining
boundary/disposal/income evidence. It never converts an already summed
mixed-currency total or substitutes zero for missing coverage.

The six-locale Gains screen shares currency, method and maximum-age controls
with its period statement. Responsive reporting cards expose component/rate
provenance in keyboard-operable disclosures and show incomplete coverage beside
retained original figures. These remain live reads over current effective
history; immutable operational runs are defined below (#234); person-level pooling is defined below (#235).

### Immutable operational runs (#234)

**Decision (2026-10-15): persist immutable report documents**, saved explicitly
by the owner. Schema 1 covers the slice 1 operational gains output (including
realized totals, dated unrealized positions, capital-return evidence and currency
metadata) and the slice 2 period statement. They retain `operational_v1`, its
exact rules and `ledger_knowledge: current_effective` as evaluated at capture;
the run is frozen even though that same profile's live reads can later restate.
Alternative-method and reporting-currency projections are outside saved schema
1. The UI states that boundary even when such a comparison is selected.

**Rejected alternative:** knowledge-time reconstruction. `live_raw_v1` evaluates
current withdrawals; dated replay uses effective operations and latest replay
revisions across disposal, transfer and structural-action families. Retrospective
reconstruction would need a coordinated temporal contract for every reader,
retirement/revision and reference-data version. Adding an observation timestamp
predicate does not reconstruct these dependencies. Saving an immutable output
provides the bounded reproducibility guarantee without changing canonical
journal/subledger lifecycle or introducing a second replay engine. A future
historical reconstruction profile can coexist with these preserved documents.

`POST /api/v1/investments/gains/runs` accepts `report_type: gains|statement` and
applied dates. Gains accepts optional `from`, `to` and `valuation_as_of` (resolved
to owner-local today if omitted); statement requires `from..to` and uses its
closing date, with no separate valuation option. Periods and valuation dates
are validated as on the source projections. This **computes a fresh run**; it
does not freeze a previously displayed live response. No arbitrary past
`knowledge_cutoff`, client-supplied output or tax claim is accepted.

SQLite's existing native online-backup path captures one consistent database
image through the single writer connection, bounded to 30 seconds. Its private
0700 temporary directory/0600 file is removed on success or failure. Existing
Go projections run on the isolated copy, with all dated replay rolled back;
no provider is called. Concurrent source changes after capture cannot mix the
gains, opening/closing statement, income or currency metadata. Only the finished
report and its `investment.gains_run.create` audit commit to the live book in
one transaction. Failed computation or persistence leaves no run or audit.
The copied input database is disposable and is **not** retained as a backup.

Each row fixes its saved-document schema (1 for original gains/statements;
2 for new statements with #238 disclosure), report type, `captured_at`
(the UTC copy-completion boundary used as the profile's observation knowledge
cutoff), creation time, actor/audit IDs, normalized request and full descriptor,
exact compact output JSON and three SHA-256 fingerprints:

- `snapshot_sha256` identifies the complete SQLite input image before it is
  opened/replayed. It includes all database state, including unrelated metadata
  and earlier runs; it is not a portable logical ledger hash and cannot be
  recomputed from an evolved/restored book or a scoped export.
- `inputs_sha256` hashes the exact compact UTF-8 `inputs` document: resolved
  request, snapshot fingerprint and descriptor.
- `output_sha256` hashes the exact compact UTF-8 `output` document, preserving
  coefficient strings, null amounts, unavailable reasons, price dates and
  approximation/staleness evidence. It never hashes a displayed rounded value.

These are corruption checks, not signatures or proof against a database editor
who can replace both data and hashes. The run's ID identifies this document;
no comparison, later correction, withdrawal or software upgrade recalculates
it. Ordinary UPDATE/DELETE are refused by triggers; there is no update/delete
API or automatic pruning. Reads never manufacture saved rows.

`GET /api/v1/investments/gains/runs` lists 20 summaries per page with an exclusive
descending-ID `next_cursor`. `GET .../runs/{id}` verifies input/output integrity
and returns the preserved document without a live projection. `?download=true`
adds a JSON attachment filename and preserves the server's exact document
bytes; reserializing parsed JSON in a browser may change escaping and therefore
its fingerprint. The six-locale mobile UI saves applied original-currency gains
or statement dates, pages through runs, reopens frozen figures and downloads the
complete document. Saved gain rows use stable instrument/account IDs; currency
formatting uses the captured metadata. Statement views disable their live query.

Forward migration `0007_investment_gains_runs.sql` preserves existing financial
rows without a reset; downgrade refuses when any saved run exists. Backup and
restore retain the exact rows. Bundle schema **12** adds
`investment-gains-runs.csv` (all runs even for a scoped ledger export), with
schema/type, timestamps, actor/audit IDs, all fingerprints and the exact two JSON
documents. The run's own dates remain authoritative; CSV/QIF ledger rows remain
canonical journal facts. The `investment_foundation` self-check verifies saved
JSON hashes, input/output descriptor agreement and snapshot/type identity as
document integrity; it never compares old figures with today's book. A damaged
run is named explicitly and retrieval refuses it rather than silently rebuilding.

Named validation: `TestInvestmentGainsRunReproducesBeforeSaleReversal` values at
D, then reverses a sale before D: both saved output types reproduce byte for
byte while live gains and statements change. It also verifies scoped export,
backup/restore, immutability and corrupted-document diagnosis. Additional cases
cover withdrawn quotes, cursor pagination, invalid/authenticated requests,
failed writes and isolation from a source write before report persistence
(`TestGainsRunSnapshot*`). `investments-gains-runs.spec.ts` exercises save,
reversal, reopen, download, six locales and 390px accessibility.

### Report audit and export acceptance (#318)

The 2026-10-18 [export-acceptance disposition](../reviews/reports-r18-audit-2026-10-18.md#export-acceptance-disposition-318)
retains the existing delivery boundary. Original-currency operational gains
save as schema 1; current operational statements save as schema 3, with old
statement schemas 1/2 still readable. Alternative FIFO/LIFO/average and
person-pooled comparisons, and reporting-currency gains/statement projections,
remain live-only. None of the saved schemas adds those projections. The save
control computes a fresh original-currency operational report for the applied
dates even when a live comparison or gains-FX method is selected.

Audit acceptance follows the surfaces that ship:

- Every shipped report download must preserve its own exact API figures and
  independent expected figures, together with its applicable profile, dates,
  coverage and provenance. R2 CSVs include their shipped reporting-currency
  restatements; this is distinct from the live-only R18 gains-FX boundary.
- Live-only R18 projections must match independent expectations through their
  API and visible browser consumers, disclose unavailable figures and retain
  their named profile/date/FX rules. They carry no download-parity claim.
- Saving while a comparison is selected must still identify the operational
  output and satisfy that output's download checks. Saving computes a new run;
  equality with an earlier live response is conditional on unchanged inputs.

This explicitly replaces #275's overbroad all-projections export criterion;
its original unmet outcome remains in the dated audit and GitHub record.
The disposition changes audit acceptance, not calculations or export formats.
Any future comparison/FX download requires a separately agreed contract for
its input state, profile, coverage, rounding, provenance and preservation.

### Person-pooled average cost (#235)

`GET /api/v1/investments/gains?alternative_method=person_pooled_average`
adds `person_pooled_average_v1` in the existing `alternative` comparison.
Its purpose is `alternative_method`, basis method `person_pooled_average`,
`pool_scope: owner_security_cost_currency_position_side` and transfer policy
`internal_pool_neutral`. All owner accounts, including archived accounts, form
one pool for each security **and original cost currency and long/short side**.
There is no currency conversion or netting of long and short positions.

Every disposal uses the owner's pooled average, including specific-lot elections.
Only the selling account's quantities are depleted. Unrealized rows retain each
account's quantity and its proportional share of pooled remaining basis, so those
rows no longer claim that account's operational carried basis. The exact average
allocator also redistributes after acquisitions and structural effects. Basis is
split at the currency's dated maximum allocation scale, backing off for supported
integer range without narrowing recorded precision. Allocation truncates and the
last surviving lot receives the remainder; a final close consumes the full basis.
Sum of disposed and remaining basis equals acquired basis, adjusted for external
movements and sourced basis actions. Internal transfers create no realization and
leave total pool quantity and basis unchanged, for both source-lot and pooled-lot
lineage. Their destination receives hypothetical carried basis before it opens.

Structural actions retain recorded lot entitlements, with the same explicit 409
refusal as other alternatives if those entitlements cannot be satisfied. External
outbound transfers remove pooled basis without creating disposal gain. A single
unknown lot leaves the whole pool without a known rate; gains and all surviving
pool shares stay unresolved until sourced resolution or exhaustion. A subsequent
known acquisition after full exhaustion starts a known pool.

Optional `jurisdiction_note` is trimmed user-supplied context, at most 500 Unicode
characters, echoed in this profile and displayed as text. It is accepted only with
this method; supplied comparison options must be nonblank and unique. The note
selects no jurisdiction rules. **This is a method comparison, not a tax
computation**: no UK same-day/30-day matching, Canadian adjustments or French tax
rules are inferred. No account exemptions or residency rules are inferred either.

The comparison shares the operational valuation date, price knowledge cutoff,
`current_effective` history and `live_raw_v1` prices. Its optional disposal period
remains independent of its valuation date. All temporary lots, method locks,
replay effects and redistribution are rolled back on success or failure. No
migration or operational re-election occurs. Reporting FX, the period statement
and both saved-document versions remain operational; this comparison is a live read.

The six-locale Gains method selector exposes the method, optional applied note,
pooling scope and limitations, with account-labelled comparison rows, loading,
empty, error and unavailable states. Named `TestPersonPooledGains*` cases verify
buys in two accounts then a sale from one, both transfer lineages, exact residual
conservation, long/short and currency separation, unknown basis, corrected
history/splits/exchanges and unchanged durable snapshots including injected
failure. `TestInvestmentGainsPersonPooledAverageAPI` covers both descriptors,
amounts, authentication and invalid query options. The 390px
`investments-person-pooled-gains.spec.ts` journey checks pooled versus operational
figures, internal transfer, plain-text note and accessibility in six locales.

### Delivery slices

| Slice | Issue | Scope |
| --- | --- | --- |
| 1 | [#230](https://github.com/sergeyfarin/rekenraam/issues/230) | `operational_v1` descriptor and as-of valuation: positions replayed to `valuation_as_of`, priced under `live_raw_v1`, on the gains read and screen. |
| 2 | [#231](https://github.com/sergeyfarin/rekenraam/issues/231) | Delivered 2026-10-14. Period gains statement: opening unrealized (day before the period), realized, gross dividend income and closing unrealized per currency for a date range; absent figures are null with reason and counts. |
| 3 | [#232](https://github.com/sergeyfarin/rekenraam/issues/232) | Delivered 2026-10-15. Alternative-method projection (FIFO/LIFO/average) beside the operational result, computed by rolled-back replay. |
| 4 | [#233](https://github.com/sergeyfarin/rekenraam/issues/233) | Delivered 2026-10-15. Operational gains and period statement in a reporting currency under two named FX methods, with per-component provenance and complete-only totals. |
| 5 | [#234](https://github.com/sergeyfarin/rekenraam/issues/234) | Delivered 2026-10-15. Explicit immutable operational gains/statement runs with snapshot/input/output fingerprints, audit, save/reopen/download UI, bundle schema 12 and document self-check. |
| 6 | [#235](https://github.com/sergeyfarin/rekenraam/issues/235) | Delivered 2026-10-15. Person-pooled average-cost comparison across all owner accounts, with a user-supplied jurisdiction note and no tax matching rules. |

Broker-comparison reads (retained broker-reported basis beside the book's)
are not scheduled. A future slice may accept owner-entered statement values
with provenance; an online broker feed is optional input, not a prerequisite.
[ADR 0017](../adrs/0017-provider-independent-features-and-incomplete-data.md)
also keeps core reporting and R13 independent of complete provider coverage.
Known original-currency figures remain usable; existing missing-input reasons,
complete-only totals and immutable saved-document rules continue to apply.

## Validation evidence

- `TestGainsFX*` covers the two methods with independent expected amounts,
  signed short proceeds, dated period boundaries and income, mixed currencies,
  missing/stale/inclusively aged rates, exact same-currency sub-cent no-ops,
  unknown basis/acquisition dates, transfer origin evidence, validation and
  unchanged durable snapshots. `TestInvestmentGainsReportingCurrencyAPI`,
  `TestInvestmentGainsFXInvalidOptionsAPI` and
  `TestInvestmentGainsFXPrecisionOverflowIs422` verify the authenticated wire
  contract and original figures. `investments-gains-fx.spec.ts` exercises the
  shared mobile controls, coverage disclosure, method changes, original amounts,
  rate evidence, same-currency conversion and axe accessibility at 390px.

- `TestAlternativeGainsMethodsConserveQuantityAndBasisAcrossPartialSales`,
  `TestAlternativeGainsEqualOperationalWhenMethodsMatch`,
  `TestAlternativeGainsConserveBasisThroughTransfersAndDestinationSales` and
  `TestAlternativeGainsShortMethodsConserveOpeningProceeds` pin exact expected
  results and conservation under all three methods.
- `TestAlternativeGainsReadLeavesLotsElectionsJournalsAndAuditUnchanged` and
  `TestAlternativeGainsFailureRollsBackTemporaryProjection` snapshot durable
  state across repeated reads and injected late failures. The remaining
  `TestAlternativeGains*` cases cover independent period/valuation dates,
  corrected history/splits, unresolved basis, merger metadata and explicit
  refusal of impossible structural entitlements.
- `TestInvestmentGainsAlternativeMethodAPI` verifies both profiles, exact
  coefficients, totals and validation through authenticated HTTP.
  `TestAlternativeGainsOverflowIs422AndOperationalProjectionSurvives` verifies
  a hypothetical method exceeding supported precision returns 422 while the
  operational read remains usable. The 390px
  `investments-alternative-gains.spec.ts` journey covers the comparison
  control, expected values, unchanged operational basis and axe accessibility.

- `TestObservationSelectionThroughHoldingsAndLatestPrices`: future, adjusted,
  bid and raw admission through real holdings and settings price reads.
- `TestObservationSelectionPriorityKnowledgeAndAgeThroughConsumers`: named
  manual/provider, override, exact/approximate, date/time/ID ties, incompatible
  manual bases, future-only, knowledge cutoff and stale cases through real
  holdings, gains, latest prices and report conversion; original rows retained.
- `TestObservationSelectionAPIHoldingsAndReportingFX`: same policy through
  authenticated HTTP gains and spending FX, retaining original currency totals.
- `TestObservationSelectionOwnerLocalDateAndPreciseKnowledgeBoundary` pins
  opposite-midnight owner zones and refuses a quote recorded in the next second.
- `TestReportingCoverageRetainsEachSelectedObservationAndApproximation` pins
  distinct quote/date provenance instead of hiding later estimated uses.
- `TestForecastUsesSharedManualRawAndKnowledgePriority`,
  `TestForecastConstantFXStalenessAndTies` and related forecast suites retain
  the constant-as-of and complete-only conversion contract.
- Relevant 390px browser journeys cover displayed policy/reasons, gains,
  reporting conversion and #101's owner-local defaults. The backend race gate,
  frontend checks/tests and integrated build remain required.

An isolated mutation removing compatibility/date/knowledge admission and
same-day source priority makes the named service/API cases fail at runtime,
including manual/provider, exact/approximate, adjusted, future and knowledge
cases. These are observed assertion failures, not compilation failures.

Independent validation-tooling gap: [#207](https://github.com/sergeyfarin/rekenraam/issues/207)
tracks clean-checkout SvelteKit alias bootstrap before catalog checks.

Mobile validation also found [#208](https://github.com/sergeyfarin/rekenraam/issues/208):
the gains table scroll regions now have localized region names and keyboard
focus. The backend gate found [#209](https://github.com/sergeyfarin/rekenraam/issues/209):
the single test mutating the process-wide Trading 212 page limit is serial under
the existing reasoned opt-out convention; all fetch/continuation assertions and
race instrumentation remain.

The full report-screen axe check found [#210](https://github.com/sergeyfarin/rekenraam/issues/210):
the page-level Filters heading is now h2, keeping the localized filter label
and report-view headings at the correct page level.
