# Target allocation and drift (#411)

Status: **owner selected per-instrument targets and current-target history
on 2026-11-06; geography is a separate enhancement; allocation-over-time
chart and saved-revision inspection added by the owner; shared liquidity/cash
scope remains to be settled before runtime implementation**. Governs [#411](https://github.com/sergeyfarin/rekenraam/issues/411).
The owner explicitly requested this independent feature in an isolated worktree
and a pull request. The [roadmap](../roadmap.md) remains the source of overall
work order; this authorization does not reorder the other agent's work.

This extends the [accepted actual-allocation contract](investment-returns-plan.md#allocation-at-the-closing-day-slice-4).
ADRs [0012](../adrs/0012-investment-ledger-subledger-and-reporting-boundaries.md)
and [0017](../adrs/0017-provider-independent-features-and-incomplete-data.md)
continue to govern valuation, incomplete data and the read-only ledger boundary.

## Contract and owner decisions

### Portfolio and grouping

- Owner choice A (2026-11-06): one current target set per book and portfolio
  selection, with targets per directly owned instrument plus one cash bucket.
  The same instrument held in several selected accounts is aggregated. ETFs
  and funds each retain their own target; constituents do not enter this view.
- Instrument identity is its existing stable commodity ID, not its ticker,
  display name, brokerage or issuer. Holdings of that ID aggregate across
  brokers. Distinct commodity records are not automatically joined by ticker,
  and different funds/share classes are not silently merged.
- Portfolio selection follows returns: either the default investment scope or
  explicit selected account subtrees. Sort and deduplicate explicit roots and
  remove a root already covered by another root. Parent and child selections
  cannot double-count an exposure. Default and explicit scopes remain distinct.
- Identity uses the selection mode and canonical root account IDs, not the
  reporting currency, valuation date, display name or current descendant list.
  A new eligible account joins the default scope; a new descendant joins an
  explicitly selected subtree. This does not silently create a new target set.
- Existing actual-only account, instrument-type and quote-currency views keep
  their accepted meanings. They do not acquire independent target sets in
  this slice. Per-instrument comparison values come from the same exact
  account/commodity balances before grouping and rounding.
- Owner additionally requested a market-exposure view, usable without targets,
  then explicitly chose a separate geographic-exposure enhancement (#514).
  #411 keeps its instrument targets and the existing quote-currency view; see
  *Market and look-through follow-ups*. No listing/currency-based geographic
  inference is authorized by this request.

### Shared liquidity and portfolio cash (open decision)

Owner request (2026-11-06): "Cash includes - need to have one common definition
of liquid accounts to exclude some also important for net worth calculation".
The unconditional combined-cash rule below is therefore a proposal pending this
clarification, not acceptance of every selected currency balance as available
investment cash.

Current app evidence:

- `app/cashflow.go` owns `defaultCashAccountKinds`/`defaultCashAccountIDs`:
  active, non-system cash/checking/savings/brokerage-cash accounts. Forecasting
  reuses this cash-scope owner. There is no persisted per-account liquidity
  classification or override.
- The [cashflow contract](reports-plan.md#cashflow) names those kinds and allows
  explicit account scope. [Account kinds](../design/account-hierarchy.md#kinds)
  distinguish savings from term deposits, but do not say whether a particular
  savings/brokerage account is restricted or reserved.
- [Net worth](reports-plan.md#net-worth-over-time) includes assets and
  liabilities under its explicit scope. Liquidity is not a current net-worth
  admission rule; [ADR 0021](../adrs/0021-entered-asset-values.md) explicitly
  includes entered values for illiquid property/vehicles/pensions.

Recommended distinction, awaiting owner confirmation:

- Share one explicit cash/liquidity classification and resolver across consumers
  that actually ask that question; do not create an allocation-only hard-coded
  account-kind list. Define account overrides, defaults, historical application
  and audit/backup/export behavior before adding storage.
- Portfolio membership is independent. An accessible emergency reserve may be
  liquid but outside the investment portfolio; a restricted pension can still
  be an owned investment in the portfolio. Explicit account inclusion/exclusion
  must be visible and apply consistently to the portfolio values, allocation
  table, series and denominator. Never omit cash from just one side of the
  comparison or silently make a partial portfolio add to 100%.
- Preserve total net worth as all included assets less liabilities. A separate
  liquid-cash/liquid-assets subtotal or filter needs its own precise label and
  definition. Cash available without a sale and marketable securities are also
  different concepts; a single unexplained boolean must not equate them.

Clarify which real account examples the owner wants excluded, whether the shared
rule concerns available cash or broader liquid assets, how it should affect
existing cashflow/forecast defaults, and whether net worth needs a labelled
liquid subtotal/filter or a change to total inclusion. No choice is inferred.

### Exact targets, cash and shorts

- Targets are long-only, from 0% through 100% each, with at most four decimal
  places in percentage units. Store integer units: 1 unit = 0.0001 percentage
  points, and every saved set sums to exactly 1,000,000 units (100%). Reject
  negatives, excess precision, duplicate groups, mixed dimensions and totals
  other than 100%; never round input into acceptance. Zero targets are allowed.
- Forms parse and prefill percentages in the active locale with the shared
  exact decimal helpers. API values are integers within this small exact
  range; no monetary coefficient crosses JSON as a number.
- Cash across the selected accounts/currencies is one explicit target bucket,
  valued in the requested reporting currency and included in the same
  denominator as securities. The actual quote-currency view retains its own
  currency splits. No implicit cash exclusion, residual target, or automatic
  normalization is performed.
- Keep actual long and short rows separate, including negative cash. Targets
  apply only to the long row; short rows have an explicit zero target. Actual
  long weights may exceed 100% and actual short weights are negative. This
  version cannot express a desired leveraged or short target portfolio.
- A classified actual group omitted from the target set has an explicit zero
  target. A targeted group with no actual balance is a known zero only when
  the complete portfolio is valued. Such rows remain visible.
- Missing instrument-type or geographic metadata does not remove a known
  commodity identity from per-instrument actuals or turn its value into zero.
  Display its available name/code or stable ID. An unregistered countable
  commodity remains visible but is not silently matched to a different target.
  Missing quote currency follows the shared valuation policy: a usable direct
  reporting-currency price is sufficient; a required missing rate is not.

### Comparison and unavailable figures

- The portfolio/date/reporting-currency/staleness selection is the actual
  allocation's own. Reuse its same consistent database image, posted balances,
  trade-date recognition, income receivables, rate table and classification.
  There is no second pricing engine or separate denominator policy.
- For complete prices and a positive closing portfolio value, actual share is
  the exact group value divided by that exact closing value. Signed drift is
  actual share minus target share, displayed in percentage points; positive
  means over target and negative means under target.
- Calculate from exact rational values, then round once to six decimal places
  in ratio units, half away from zero, matching the existing weight precision.
  Do not subtract separately rounded actual and target displays. Rounded row
  displays need not add to exactly 100% or zero drift.
- If any required price or FX leg is missing, show the existing known-only
  actual breakdown and unknown units, but all target-comparison actual shares
  and drifts are unavailable. Target percentages remain visible. Never compare
  a 100% target portfolio against a subtotal of only its priced holdings.
- Zero or negative closing totals preserve values and targets but have no
  comparison shares or drift. An unclassified short makes the comparison
  unavailable under the existing allocation reason. Missing classification
  never becomes a guessed type, currency or known-zero holding.
- Valuation age notices follow the existing actual read. The comparison
  echoes its date, currency, selected/resolved scope, coverage and target
  revision so the UI cannot mix snapshots.

### Persistence, lifecycle and history

- Persist settings beside the ledger with book identity, stable group keys,
  creator, UTC time and immutable revisions. Create/change is an explicit
  audited command; use an expected revision to refuse stale concurrent edits
  atomically. Refetch never silently replaces unsaved form fields or their
  revision token. An empty working copy does not erase saved targets.
- Owner choice A (2026-11-06): editing creates a new complete revision. This
  first version has one current
  revision and retains previous revisions for audit; it has no effective-date
  schedule or historical target selection. A historical actual date is explicitly
  compared against today's current targets, never called the targets in force
  on that historical date. No automatic historical target backfill occurs.
- Renaming an account or instrument does not change target identity. Archived
  or closed accounts and inactive instruments retain their references; they
  are not silently removed or redistributed. A previously saved group that
  no longer resolves stays visible with its ID and an explanation. New targets
  validate against eligible current group identities, including retained
  historical accounts/currencies supported by the existing returns selection.
- A changed portfolio selection opens its own target set. Warn before
  discarding an unsaved form. Switching an actual-only breakdown never converts
  instrument targets into account, market, type or currency targets.
- Native SQLite backup/restore preserves all target revisions and audit rows.
  Under the existing portable-ledger export policy (ADR 0011), target settings
  are omitted from ledger CSV/QIF/bundle; add that omission explicitly to the
  bundle manifest/documentation and verify it. Backup/restore remains the full
  installation round trip. No bundle importer is introduced.

### UI and immutable runs

- Add manual target create/edit/reopen and actual/target/signed-drift rows to
  the live returns allocation workflow, with the grouping and scope explicit.
  Show exact total and remaining/excess percentage while editing, validate on
  both client and server, and preserve input after validation/network failures.
- Use one backend-composed read for the shareable comparison component, with
  no request per row. The comparison obtains targets and actuals from one
  input image; it does not join separately fetched revisions and values in
  browser code. Reads never write financial or application settings.
- Use translated text, semantic theme tokens, labelled controls, visible signs
  and words, keyboard focus management, mobile layout, and loading, no-target,
  error, unavailable and success states. Support the maintained en/nl/ru
  catalogs and explicit fallback for partial es/fr/de catalogs.
- Existing returns calculations, request fingerprints, saved output bytes,
  immutable-run integrity checks and reopening behavior remain unchanged.
  The live comparison is a separate read model and is not included in saved
  returns runs. A saved-run view never overlays today's targets on its
  immutable allocation. The live UI explains that saving a returns run does
  not save targets or drift; target revision history remains separate.
- No rebalancing advice, trade recommendations, orders, ledger postings,
  cost-basis changes or tax optimization. Advisory rebalancing stays in #412.

## Allocation over time and saved target changes (owner request)

The owner explicitly added the following to #411 on 2026-11-06. This extends
actual allocation presentation; it does not introduce effective-dated targets or
ETF look-through.

### Actual series and target column

- Show a 100% stacked area chart of actual per-instrument/cash weights over the
  selected period. Use the allocation table's exact portfolio membership,
  reporting currency, trade-date/income recognition, price/FX policy and input
  image. Price changes, FX, purchases, sales and cash flows can change weights;
  cash weight can change while its nominal balance remains constant.
- Put a stacked **current target** column beside the chart's right edge, with
  each target percentage labelled and a consistent component order. Individual
  target ticks are unsuitable: area boundaries are cumulative, while targets
  describe component shares. A missing target set shows the actual series with
  a no-target state, never fabricated target zeroes totalling 100%.
- Selecting an instrument and sampled date exposes exact server-rounded actual
  share, current target and signed drift for that date. Provide labelled date
  and instrument controls or an equivalent keyboard/touch path; hovering is
  optional, never the only means to inspect a point.
- Use stable identities/order/colours across actual stacks and targets, including
  instruments appearing/disappearing during the period and targets with no
  current holding. Legend/readout/table retain full names and percentages;
  colour alone is insufficient to identify a component.
- Compose targets, revisions, dated actuals and coverage in one backend read,
  without per-date or per-instrument browser fan-out. Reuse exact balances/rate
  selection before grouping and rounding. Do not recompute the entire period's
  returns separately for each chart date.
- Proposed sampling: reuse the existing returns daily/weekly/month-end grid,
  echo the interval, and retain both period boundaries. Make sampling visible;
  do not claim an unsampled date is measured. Sampling details/resource bounds
  are implementation choices to document and validate, not a new daily-price
  guarantee.

### Honest gaps and signed allocations

- Missing required prices/FX at a sampled date make that date's full actual
  stack and dependent drift unavailable. Preserve known values/unknown units
  in the table. Split chart segments at gaps; never fill unknown shares with
  zero, bridge a missing sample, or normalize only the priced subset to 100%.
- A conventional 100% area stack is only shown for fully valued, nonnegative
  components and a strictly positive total. Named shorts, negative cash and
  zero/negative totals retain the signed allocation table and an explanation
  instead of being clamped, netted into positive bands or hidden. A sampled
  date with such a condition breaks the conventional area segment.
- Classification as a chart gap is per sampled date. An earlier chart-only
  missing price must not make a later fully valued allocation unavailable;
  cumulative TTWROR's dependency/reason policy is not reused for this purpose.
- ETF value appears once as the directly owned instrument. Underlying-company
  exposure belongs in #513 and never enters this stack alongside its ETF.

### Saved-change markers and accessible history

- Offer optional vertical markers for target revisions **saved** during the
  displayed period, scoped to this exact portfolio target set. The timestamp is
  a UTC system fact; any chart-day placement must be derived from the explicit
  owner/report timezone and remain labelled as save time, not an effective date.
- Selecting a marker shows the previous and new target percentages, saved
  timestamp and affected scope. The first revision says no previous target
  set, rather than inventing a previous 0% target portfolio.
- Provide an accessible revision list with the same detail, usable by keyboard,
  screen reader and touch without chart hover. Revisions outside the plotted
  period remain inspectable in history but do not acquire an in-period marker.
- At every sampled date, drift still compares actuals against the **current**
  target revision captured for the read. Selecting a saved-change marker
  inspects history; it does not switch the comparison's target or backdate a
  new target. Label this explicitly beside chart and detail readout.
- Existing immutable returns runs retain their bytes and view. This new live
  series/history panel is not silently appended to old saved output.

## Independent worked cases

All values below are exact EUR values at the chosen closing date. Percentage
point drift is actual percentage minus target percentage.

| Case | Facts and targets | Expected comparison |
|---|---|---|
| T1 complete/cash | Equity ETF A 600, bond ETF B 300, cash 100; instrument targets 50%, 40%, 10% | Total 1,000; actual 60%, 30%, 10%; drift +10, -10, 0 pp |
| T2 missing instrument | ETF A 900, cash 100, no ETF B held; targets A 60%, B 30%, cash 10% | B appears as actual 0%, drift -30 pp; A +30 pp; cash 0 pp |
| T3 missing price | Known cash 100, 10 unpriced stock units; targets stock 90%, cash 10% | Existing allocation says cash 100% of known-only 100; comparison actual shares/drifts all unavailable, targets 90%/10% stay visible |
| T4 short cash | Stocks +1,100, cash -100; targets stock-long 90%, cash-long 10% | Total 1,000; stock-long actual 110%, drift +20 pp; cash-long actual 0%, drift -10 pp; cash-short actual -10%, target 0%, drift -10 pp |
| T5 non-positive | Stocks +100, cash -100 (then cash -150) | Total 0 (then -50); values and targets stay, comparison shares/drifts unavailable |
| T6 missing metadata | Instrument A 600 and priced instrument B 400, B has no type/region effective on the date; target A 100% | Total 1,000; A actual 60%, drift -40 pp; B actual 40%, implicit target 0%, drift +40 pp; stable identity is sufficient, geographic/type classification remains unknown |
| T7 exact input | 33.3333%, 33.3333%, 33.3334% | Units 333333 + 333333 + 333334 = 1000000 accepted; three 33.3333% values rejected (99.9999%); 33.33333% rejected for precision |
| T8 rounding | Group A 1, B 1,999,999; targets 0.0001% and 99.9999% | Total 2,000,000; A exact actual ratio 0.0000005 rounds to 0.000001; exact drift 0.0000005 - 0.000001 = -0.0000005 rounds to -0.000001 (-0.0001 pp), not zero from subtracting rounded shares |
| T9 portfolio identity | Selected parent P and descendant H, then P alone | One canonical root P and one target set; all actual balances counted once |
| T10 currency | Instrument A worth USD 100 at EUR 0.9/USD, EUR cash 10; targets A 80%, cash 20% | Total EUR 100; A actual 90%, drift +10 pp; cash actual 10%, drift -10 pp; missing required USD/EUR makes drift unavailable |
| T11 lifecycle | Instrument renamed, then retired with nonzero historical exposure; its holding account is closed | Same commodity target ID/percentage retained; name/status updates; historical dated actual rules unchanged; no redistribution |
| T12 current vs historical | Revision 1 stock 60%/cash 40%, then revision 2 70%/30%; historical actual 65%/35% | Comparison labels current revision 2, drift -5/+5 pp; revision 1 retained; existing saved returns bytes unchanged |
| T13 changing cash weight | Sample 1: A 900/cash 100; sample 2: A 1,900/cash 100; current targets A 80%/cash 20% | Actual A/cash 90%/10% then 95%/5%; cash drift -10 then -15 pp despite unchanged nominal cash; target column stays 80%/20% |
| T14 actual gap and recovery | T13 with missing A price at an intermediate sample and usable prices at both endpoints | Two separate available chart segments; missing sample is a gap with unknown A units; recovered closing actual stays 95%/5%, never cash-only 100% |
| T15 saved changes, current comparison | Historical actual A/cash 65%/35%; saved revision 1 60%/40%, later revision 2 70%/30% | All selected historical-date comparisons use current revision 2, drift -5/+5 pp; marker/list shows previous 60%/40% and new 70%/30% with save time and scope, without changing the comparison |
| T16 signed chart fallback | A 1,100/cash -100 at one sample, positive-only holdings at adjacent samples | The signed date has no conventional 100% area stack and breaks the segment; table retains +110%/-10% and their signed drifts; no clipping to 100%/0% |

## Market and look-through follow-ups

The owner requested two independent exposure views during contract review.
Both are separate enhancements and do not block #411:

- **Market/geography and quote currency**, separately tracked in
  [#514](https://github.com/sergeyfarin/rekenraam/issues/514), available even
  with no target set. Proposed interpretation: geographic investment allocation and quote currency
  are separate axes, optionally crossed in a table. Listing exchange, fund
  domicile, country classification, investment mandate and underlying-company
  geography are different facts. Neither a EUR quote nor an Irish fund domicile
  establishes European underlying exposure. EU, euro area and Europe also need
  explicit region definitions. Existing MIC/country fields alone cannot prove
  an ETF's geographic allocation. Missing or mixed allocations need visible
  unknown/multi-region treatment or sourced weighted breakdowns. The owner
  accepted separate enhancement scope; #514 retains the detailed geography
  meaning, source and classification contract for its own acceptance.
- **Direct plus ETF look-through security exposure** is separately tracked in
  [#513](https://github.com/sergeyfarin/rekenraam/issues/513), created at the
  owner's request. It does not change per-instrument targets or the selected
  current-target history policy. Dated holdings, stable security identities,
  partial coverage, direct/indirect breakdown and double-count prevention need
  their own accepted contract. No dependency on #411 delivery is asserted.

Concrete issuer evidence: [iShares IUSA](https://www.ishares.com/uk/professionals/en/products/251900/iusa?siteEntryPassthrough=true&switchLocale=y)
publishes holdings CSV and lists the same US-company fund in EUR/GBP/USD;
[Vanguard VUKG](https://www.vanguard.co.uk/professional/product/etf/equity/9580/vanguard-ftse-100-ucits-etf-gbp-accumulating)
publishes market/holdings data with an explicit incomplete-total warning.
These are feasibility examples, not verified app adapter contracts.

## Planned validation

Named service/API tests own the literal cases above, validation of duplicate
keys and stale writes, no read-side writes, and targets/actuals captured from
one input image. Existing allocation and saved-run compatibility/integrity
regressions must remain green. Exercise target backup/restore and the explicit
export omission. A phone browser journey creates, changes and reopens targets,
checks exact locale input and signed drift, partial-price and empty states,
stale edit preservation, keyboard/accessibility and both themes. Add literal
series cases T13-T16, no-target actuals, stable stacking order, gap recovery,
zero/negative totals, saved-marker detail and accessible revision inspection.
Verify date/instrument selection on phone and keyboard, target-column labels,
no chart overflow and no saved-run serialization change. Shared liquidity/cash
scope needs cross-consumer tests after its contract is settled. Run the relevant
shared backend/frontend gates and integrated build before the PR.

## Acceptance record

Owner answers on 2026-11-06:

1. Grouping: "A - most conventional and straightforward" (per instrument plus
   cash), additionally requesting market/currency exposure even without targets.
2. History: "A" (compare historical actuals against current targets, retaining
   prior revisions for audit).
3. Create a separate enhancement for direct MSFT exposure across brokers plus
   underlying MSFT held through multiple ETFs. Created #513; the owner explicitly
   said this does not affect choices 1 or 2.

4. Market scope: "Finish #411 targets + existing currency view; create a separate
   geographic-exposure enhancement (recommended)". Created #514.
5. Remaining contract: "Change one or more rules before coding". The specific
   requested changes are not yet supplied. A focused clarification lists cash,
   target precision/total, shorts, missing-price drift, revision history,
   backup/export and saved-run inclusion so the owner can name affected rules.

The owner invited challenges and further guidance. The numerical, coverage and
storage rules above remain proposals, not accepted decisions. Await the specific
requested changes and discuss their trade-offs before updating the contract or
starting runtime code, as #411 requires. The grouping/history and separate
exposure-enhancement choices remain accepted.

### Owner amendments after the remaining-rules question (2026-11-06)

- The owner specified rule 1 as the change: a common definition of liquid
  accounts is needed to exclude some, also relevant to net worth. Account
  examples and the distinction between liquidity, portfolio membership and
  total/liquid net worth require clarification; see the open decision above.
- The owner added allocation over time, a labelled stacked target column,
  date/instrument inspection, optional saved-revision markers with before/after
  details and accessible history. The supplied chart honesty rules are recorded
  above, including explicit gaps and signed-allocation fallback.
- The owner explicitly retained current-target historical comparisons and
  saved-change timestamps rather than an effective-date target schedule.
- No further amendments to rules 2-7 were specified. Their proposed behavior
  remains as recorded; no new rejection or acceptance beyond the supplied
  answers is inferred. Runtime implementation awaits the settled cash scope.
