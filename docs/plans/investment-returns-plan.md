# R13 investment returns contract (`returns_v1`)

Status: **contract accepted by the owner 2026-10-24 (four policy answers
recorded below); slice 1 (backend read) delivered 2026-10-24 and corrected
the same day after two reviews (`e30413a5`: seven findings, recorded inline
as "finding N"; `ae084c4d`: two, recorded under the expected cases). Slice 2
(returns screen) slice 3 (cumulative TTWROR series and chart) and slice 4
(allocation at the closing day), contracts below, delivered the same day;
slice 5 (benchmarks) delivered the same day; slice 6 (saved runs) delivered
2026-10-25. R13 core is complete. Slice 7 (browser total-return price entry,
closing slice 5's API-only gap) delivered 2026-10-25. A per-instrument
contract (`instrument_returns_v1`) was accepted the same day; slice 8
(sleeve read) delivered 2026-10-25 and slice 9 (screen panel and saved-run
inclusion) 2026-10-26. #409 (unsettled trade cash across a period boundary) was fixed
the same day by trade-date recognition; see *Valuation `V(d)`*.**
Governs [#119](https://github.com/sergeyfarin/rekenraam/issues/119). The
[roadmap](../roadmap.md) owns order; ADR 0012 (reporting boundaries) and ADR
0017 (provider independence, honest incomplete data) govern this plan.

R13 answers one question: *how did a selected set of investment accounts
perform over a period, independently of when money was added or removed
(time-weighted), and for the money the owner actually put in
(money-weighted)?* The live calculation is read-only. It never writes postings,
operations, observations or audit events, and it never invents a transfer to
make a figure work. Explicitly saving a run persists its document and audit
event under the separate slice 6 contract.

## Accepted delivery and follow-up disposition (2026-10-26)

The owner accepted closing [#119](https://github.com/sergeyfarin/rekenraam/issues/119)
around the delivered `returns_v1` and `instrument_returns_v1` scope, including
all nine slices and their review corrections. Targets and advisory rebalancing
are retained as separate deferred features, not described as implemented.
This supersedes the earlier tracking choice to keep #119 open for them.

### Delivery evidence

| Delivered outcome | Commit evidence | Independent cases / regression owner |
|---|---|---|
| Live TTWROR/IRR, values, external flows and honest unavailable reasons | [e30413a5](https://github.com/sergeyfarin/rekenraam/commit/e30413a5), [ae084c4d](https://github.com/sergeyfarin/rekenraam/commit/ae084c4d), [790bd788](https://github.com/sergeyfarin/rekenraam/commit/790bd788) | `investment_returns_calc_test.go`, `investment_returns_test.go`; literal return cases and exact daily arithmetic |
| URL-held selection and repairable coverage on the returns screen | [f0b3b8ec](https://github.com/sergeyfarin/rekenraam/commit/f0b3b8ec), [e533a101](https://github.com/sergeyfarin/rekenraam/commit/e533a101), [181087fc](https://github.com/sergeyfarin/rekenraam/commit/181087fc) | `returns-model.test.ts`, `investment-returns.spec.ts`; stale FX leg, missing/stale notices and historical account choices |
| Cumulative series with a closing point identical to the headline | [df4a75b1](https://github.com/sergeyfarin/rekenraam/commit/df4a75b1), [8585b604](https://github.com/sergeyfarin/rekenraam/commit/8585b604) | C1–C8 and `TestReturnsSeriesClosingGapOutranksAnEarlierGap`; chart-only coverage stays separate |
| Closing allocation by account, asset type and currency | [62f9201d](https://github.com/sergeyfarin/rekenraam/commit/62f9201d), [52065a30](https://github.com/sergeyfarin/rekenraam/commit/52065a30) | A1–A8 in `investment_returns_allocation_test.go`; long/short, unknown values and explicit overflow |
| Declared price/total-return benchmarks, split-neutral quotes and FX gaps | [eeab1a2a](https://github.com/sergeyfarin/rekenraam/commit/eeab1a2a), [6cfd060d](https://github.com/sergeyfarin/rekenraam/commit/6cfd060d), [351039f4](https://github.com/sergeyfarin/rekenraam/commit/351039f4) | `investment_returns_benchmark_test.go`, `investment_returns_split_order_test.go`; independent basis/FX/split cases |
| Immutable saved runs, integrity on every read and exact saved-scale display | [f9195d67](https://github.com/sergeyfarin/rekenraam/commit/f9195d67), [506b1e46](https://github.com/sergeyfarin/rekenraam/commit/506b1e46), [a371481e](https://github.com/sergeyfarin/rekenraam/commit/a371481e) | `TestReturnsRunReproducesAfterBackdatedBuy`, `TestReturnsRunValidationPaginationAndReadOnlyRetrieval`; export, restore, tampering, page boundaries and omitted/null/zero validation |
| Browser entry/correction of benchmark-only total-return prices | [86c48efb](https://github.com/sergeyfarin/rekenraam/commit/86c48efb) | `TestManualTotalReturnPriceFeedsOnlyATotalReturnBenchmark`, `pricing-management.spec.ts` |
| Instrument sleeves and ex-date receivables with correction cancellation | [7d1996c3](https://github.com/sergeyfarin/rekenraam/commit/7d1996c3), [819b78ce](https://github.com/sergeyfarin/rekenraam/commit/819b78ce), [03ce3680](https://github.com/sergeyfarin/rekenraam/commit/03ce3680), [05aece6e](https://github.com/sergeyfarin/rekenraam/commit/05aece6e) | I1–I12b; `TestInstrumentReturnsReversedAmbiguousIncomeLeavesTheSleeve`, `TestInstrumentReturnsHoldingRepairCancelsTheAmbiguousPaymentInLieu`, `TestReturnsCorrectedPayerAcrossScopeCreatesNoGain`, `TestReturnsCancelledForeignDividendLeavesNoCoverageDependency` |
| Live/saved instrument panel and truthful fallback treatment | [6dedbb83](https://github.com/sergeyfarin/rekenraam/commit/6dedbb83), [b9c57ea3](https://github.com/sergeyfarin/rekenraam/commit/b9c57ea3) | `TestReturnsRunSavedBeforeInstrumentsReopensUnchanged`, `TestReturnsRunFreezesInstrumentReturns`; phone/tablet/desktop browser acceptance |

Application/API test files above live under `backend/internal/app/` and
`backend/internal/api/`; the model tests live under
`frontend/src/lib/investments/`, and browser journeys under `e2e/playwright/`.
The detailed contracts and review history below remain the formula and
policy authority; this table is delivery evidence, not another task checklist.

### Validation result and its limits

On 2026-10-26 the closing review independently checked hosted
[CI run 40285442638](https://github.com/sergeyfarin/rekenraam/actions/runs/40285442638)
at `b9c57ea37d1ee8d61441eae7c44c32c03f7bcfe6`: **all six jobs succeeded** —
Backend Tests (shared race gate), Backend Coverage (including its floor),
Frontend Check, Integrated Build, E2E Smoke (app) and E2E Smoke (audits).
The separate [code-scanning run](https://github.com/sergeyfarin/rekenraam/actions/runs/40285441590)
also succeeded. There is no pending CI in that implementation evidence.
The final delivery report additionally records 778 frontend unit tests and
all 10 returns browser journeys passing; these local counts are delivery
evidence, not a claim that the Frontend Check job runs the entire unit suite.
The instrument journey checks 390px, 768px and 1280px, including accessibility
and the focusable scrolling table at tablet width.

The closing change updates documentation/tracking only. It validates links,
named tests, commit ancestry and the diff; it does not rerun runtime suites or
claim a new release certification. Owner household/deployment and restore/export
acceptance remain [#336](https://github.com/sergeyfarin/rekenraam/issues/336),
separate from deterministic R13 acceptance. The roadmap retains all release gates.

### Retained work and accepted limits

The [target-allocation proposal](investment-target-allocation-plan.md) records
the concrete #411 contract and independent examples prepared on 2026-11-06.
The owner selected per-instrument targets and current-target history; the
additional market-exposure meaning and scope await clarification before runtime
implementation. This does not change delivered actual allocation or saved runs.

| Disposition | Owner / reason |
|---|---|
| **Deferred: target allocation and drift** | [#411](https://github.com/sergeyfarin/rekenraam/issues/411). Actual allocation ships; target grouping, exact weights, cash/short denominator, history and incomplete-value policy need their own accepted contract. |
| **Deferred: advisory rebalancing** | [#412](https://github.com/sergeyfarin/rekenraam/issues/412). Requires the target model from #411 and an accepted funding/rounding/coverage policy. No broker execution or automatic postings are implied. |
| **Retain: honest unavailable figures** | Ambiguous income, unclassified exchanges, missing required prices/FX, non-positive capital and unproven IRR roots stay unavailable under the named rules. Do not invent attribution or pick a numerical root to make a percentage appear. |
| **Retain: bounded short detection / no short-sleeve return** | D5 is accepted. Named short operations and end-of-day balances are detected; arbitrary plain intraday exposure is not reconstructed. Short sleeves retain supported values/gain but no percentage return. A different method requires a new accepted contract. |
| **Retain: benchmark source limits** | Manual total-return entry now ships. Price-return benchmarks exclude distributions and depend on recorded splits; an unrecorded split cannot be detected. Live benchmark adapters and broker comparisons remain optional future integrations with separate evidence, not a reason to reopen #119. |
| **Retain: saved-run presentation limits** | Figures, ids, dates and saved scales are frozen. Names resolve as they are today; older runs are not rebuilt. Freezing descriptive metadata would require a separately justified versioned extension. |
| **Deferred: performance evidence at scale** | [#413](https://github.com/sergeyfarin/rekenraam/issues/413). The large-book harness seeds no investments, so the returns read, `instruments=true` (one `compute` per sleeve) and saved runs have no wall-time or heap measurement. Measure and bound first; optimise only on evidence. |
| **Retain: not in `instrument_returns_v1`** | Native (quote-currency) per-instrument returns, per-instrument benchmark comparison and contribution-to-return attribution (Brinson and similar) are outside the accepted contract (*API, screen, saved runs*). Each needs its own contract; none is planned. |
| **Retain: valuation recognition, not full accrual accounting** | Recorded ex-date dividends/payments in lieu affect net worth and returns as receivables/payables; posting-date income, spending, cashflow, registers, reconciliation and forecast are unchanged. Separate receivable accounts and partial-payment workflows are not delivered. |

Only targets, advisory rebalancing and the scale measurement are retained
as delivery issues here.
The other rows are explicit accepted limits, not hidden outstanding R13 fixes.
Revisit one when a concrete user case or defect warrants a bounded issue and
an accepted change to the relevant contract. Existing #395 financial-field
corrections and #405 schema work keep their own issues; closing #119 neither
delivers nor changes their acceptance or the roadmap's order.

## Owner decisions (2026-10-24)

| Question | Decision |
|---|---|
| Income whose cash lands on the other side of the portfolio boundary | **Classify by the position the facts name, not by the cash account; fall back to the cash side only when no position can be named, and disclose the fallback.** See *Why not cash side alone* below. |
| Annualization | **Only for periods of at least 365 days.** Shorter periods show the period figure; the p.a. figure is unavailable with reason `period_shorter_than_year`. |
| Money-weighted roots | **Only a proven unique root** (Descartes' rule; see the IRR section). Otherwise unavailable with `root_not_unique`; no root is ever chosen silently. |
| First implementation | **Backend read + tests** (slice 1). Screen, chart, allocation, benchmark and saved runs are later slices. |

### Why not cash side alone (checked 2026-10-24)

The ledger alone would classify a dividend by where its cash lands: inside the
portfolio it is return, outside it is nothing. That is exact, but only
meaningful if cash and holdings are linked, and they are not:

- A dividend journal posts only the cash account and the income (and
  withholding) account (`app/investments.go` dividend writer). Its fact,
  `investment_dividend_facts.commodity_id`, names an **instrument**, nullable
  since #393; no holding account is recorded.
- Any non-system asset or liability can settle a trade, dividend, payment in
  lieu or capital return (`settlementRole` in `app/investments_roles.go`).
  Nothing requires a holding and its cash to share a brokerage container;
  `parent_account_id` on a holding is optional.
- A capital return posts cash against `commodity_trading` and touches no
  holding account; its fact (`investment_capital_return_facts.account_id`)
  names the holding. Payment-in-lieu facts name a nullable holding account
  and a nullable instrument.

So with cash side alone, an owner whose broker pays dividends into a
checking account would silently lose every dividend from the return, and
including that checking account would turn salary and groceries into
"performance". Classifying by the named position avoids both, without
writing anything: it is a read-time classification of facts that already
exist. Where the facts do not name one position (unassigned instrument, or
the instrument held both inside and outside the boundary), the cash side is
the only honest remaining rule, and the response lists every such entry.

**No transaction is created.** A dividend paid into checking stays exactly
one recorded journal (checking +20 against income −20), so reconciliation
against bank and broker statements is unchanged. Inside the return
calculation, income belongs to the entitled side and the cash payment is a
withdrawal. Without an ex date these coincide; with one, the receivable
bridges recognition and payment as *Valuation `V(d)`* specifies. Neither
read-time adjustment is stored. Net worth uses the same receivable timing;
cashflow, reconciliation and posted exports retain the original journal dates.

## Inputs

| Input | Rule |
|---|---|
| `from`, `to` | Inclusive calendar days, `from <= to <= owner-local today`. The **opening valuation** is at the end of `from − 1`; the **closing valuation** at the end of `to`. `days = to − (from − 1)`. |
| Reporting currency | Required, a `currency` commodity. Every value and flow is converted into it. There is no native multi-currency return: unlike commodities have no shared denominator. |
| Portfolio accounts | Optional account ids, each expanded to its subtree (as of `to`). Each must be a non-system asset or liability. Omitted → the **default investment scope**: every non-system asset/liability account whose kind's base kind is `investment_container`, `investment_cash`, `security_holding` or `digital_asset`, plus their descendants. The response echoes the resolved ids and which rule produced them. |
| `max_staleness_days` | Optional, 1–31, default 7: the on-or-before window for prices and FX (the R2 `observed_on_or_before` rule). |

Accounts are resolved once, as of `to`, for the whole period (the cashflow
report's reason applies: re-resolving mid-period would let an account join or
leave and break the flow identity).

## Valuation `V(d)`

`V(d)` is the portfolio's market value at the end of day `d`: every posted
ledger balance of every portfolio account through `d`, per commodity,
converted at `d` by the shared `RateTable` (`app/valuation.go`): a currency
directly, a security or crypto unit through its quote currency (#281 chain),
under `live_raw_v1` admission and the knowledge cutoff of the read. Negative
balances (named shorts, a margin liability) contribute negative values.
A stale quote dated before a recorded split is brought to `d`'s units by the
instrument's split factor in between, the shared rule of the
[reports plan](reports-plan.md) (#407); conflicting same-day factors in that
span make `V(d)` unavailable (`conflicting_splits` gap).

- It is **ledger based, not lot based**: return needs quantities and prices,
  never cost basis, so unknown basis does not affect any R13 figure.
- **Trade-date recognition (#409).** Every entry of an investment operation
  whose units post on a single day is read on that day. A trade posts its
  units on the trade date and its cash, clearing leg and charges on their own
  dates (`tradeJournalEntries`), so read entry by entry, the bought units and
  the cash not yet paid both sat in the portfolio between trade and
  settlement. In the reproducer, a flat price with no flows read an opening
  value of 1,200 instead of 1,100 and TTWROR −0.083333.
  - **Unsettled amounts.** Recognising the cash with the units makes the
    unsettled amount the portfolio's payable or receivable from the trade
    date. It is valued daily in its own currency, as any balance is.
  - **Flows.** A trade funded from outside the portfolio is a flow on the
    trade date, converted on that date.
  - **Which entries count.** An operation is in a period by its trade date.
    A settlement dated after `to` is read with its trade, and one inside the
    period whose trade preceded `from` belongs to the earlier period.
  - **Unchanged.** Operations whose units post on several days, or on none
    (a dividend, a capital return), keep their entry dates. Ledger balances,
    reconciliation and cashflow stay posting-based. Net worth adopted the
    same rule for [#410](https://github.com/sergeyfarin/rekenraam/issues/410),
    as an unsettled line (reports plan, *Net worth over time*).
  - **Cases** (`investment_returns_settlement_test.go`): R1 reproducer
    (`0.000000`); R2 checking-funded across `from` (no flow); R3 settling
    after `to` (closing 1,100, with allocation equal to it); R4
    checking-funded settling after `to` (contribution 100 on the trade date);
    R5 the R1 trade reversed (no figure moves).
- An **unclassified short** (a negative countable balance beyond named short
  lots, the net-worth rule) makes that valuation unavailable
  (`unclassified_short`).
- A commodity with no usable price/FX inside the window makes that valuation
  unavailable; the rate gap names the commodity and reason exactly as the
  existing `ValuationCoverage` does.

## External flows

A journal entry is examined only if it is dated within `[from, to]`. Its
postings are partitioned:

- **B**: postings on portfolio accounts.
- **P**: postings on income or expense accounts — performance, never a flow.
- **T**: postings on the `commodity_trading` system account — internal
  conversion legs, never a flow and never valued.
- **O**: every other posting (non-portfolio asset/liability/equity, and the
  remaining system roles such as `opening_balance`, `transfer_clearing`,
  `external_investment_transfer_equity`).

**Whether the entry belongs to the portfolio** is decided by its *position
anchor*:

1. Entries of an investment operation whose journals post to subledger-managed
   holding accounts: anchored to those holdings. All inside → in; all outside
   → out; both (an internal transfer or corporate action across the boundary)
   → *mixed*, handled by the plain rule below.
2. Otherwise, a capital-return or payment-in-lieu fact naming a holding
   account: anchored to it.
3. Otherwise, a dividend, withholding-only or payment-in-lieu fact naming an
   instrument: on the anchor date (the operation's `ex` date when recorded,
   else the entry date) look at the instrument's ledger balance in every
   subledger-managed holding. Held (non-zero) only inside → in; only
   outside → out; inside and outside → *ambiguous*; nowhere → *not held*.
4. No anchor (ordinary entries, unassigned instrument, ambiguous, not held):
   the **cash side** — the entry belongs iff it has a B posting.

The entry's external flow, in the reporting currency at the entry date:

| Entry | Flow `F` (positive = into the portfolio) |
|---|---|
| **Delivery**: belongs, and is an external transfer in/out (or its correction), or has an O posting on an equity-class account (never the T leg: `commodity_trading` is equity-class in production, but internal) | `F = Σ value(B) + Σ value(P)` |
| Belongs otherwise (anchored in, mixed, or cash side with a B posting) | `F = −Σ value(O)` |
| No, anchored out, but has B postings | `F = +Σ value(B)` (the cash entering is a contribution, not return) |
| No and no B posting | not examined |

**Why deliveries differ (review 2026-10-24, finding 1).** Equity-class
counterparts (`opening_balance`, `external_investment_transfer_equity`, owner
equity) record *book* value, not what crossed the boundary. An external
transfer-in posts units against `commodity_trading` and, only when basis is
known, the carried basis against the equity bridge; a transfer-out posts units
against `commodity_trading` alone. `−Σ value(O)` would therefore count a
security worth 100 with carried basis 1 as a contribution of 1 (and 99 of
gain), unknown basis as 0, and a transfer-out as a loss. A delivery is
instead recognised at the dated **market value** of what entered or left the
portfolio, `Σ value(B)`, less any performance posted in the same entry. R13
stays independent of cost basis. A security delivery that cannot be priced on
its date is `flow_unavailable`.

Consequences, each covered by an expected case:

- A deposit from checking into broker cash: `O = checking −1000` → `F = +1000`.
- A buy inside the portfolio: only B, P, T → `F = 0`. A buy paid from
  checking into a portfolio holding: `F = +cash paid` (the trade-versus-close
  difference is return, as it should be).
- An external transfer-in of one unit priced 100: `F = +100` whatever its
  carried basis, known or unknown. A transfer-out is a withdrawal at market.
- A dividend into broker cash: B and P only → `F = 0`; the income is return.
- A dividend for a portfolio holding paid into checking: anchored in,
  `O = checking +20` → `F = −20` — a same-day withdrawal, so the dividend
  is return and leaves.
- A dividend for an outside holding paid into broker cash: anchored out,
  `F = +20` — a contribution, not return.
- Fees and withholding are expense postings: returns are **net of fees and
  taxes**. A withholding booked to a receivable outside the portfolio is a
  withdrawal of that receivable.
- An unassigned dividend into broker cash counts as return under the cash
  side and is disclosed; one into checking is not part of the portfolio and
  is disclosed.

**Every** cash-side fallback for an investment operation is disclosed with its
transaction, date, reason (`unassigned_instrument`, `held_inside_and_outside`,
`not_held`) and whether it was counted — including `not_held` cash that landed
outside the portfolio (finding 7). A counted fallback marks the figures as
resting on a provisional attribution; the list is how a reader sees it.

**Correction cancellation before splitting (finding 3).** Flows are first
netted per *economic identity* and date: all entries of one investment
operation correction chain (the original, its reversals and replacements)
dated the same day form one net flow, and every other journal entry is its
own identity. Only then is each net flow split by sign into `In(d)` or
`Out(d)`. An erroneous payment and its same-day reversal therefore leave no
flow at all, and genuine, independent same-day flows keep the
inflow-at-start / outflow-at-end convention. A flow that cannot be converted
makes every figure depending on it unavailable.

## Time-weighted return (TTWROR)

Daily linking, Portfolio Performance's documented convention (inflows at the
start of the day, outflows at its end), adopted explicitly:

```
1 + r(d) = (V(d) + Out(d)) / (V(d−1) + In(d))
R_twr    = Π_{d=from..to} (1 + r(d)) − 1
```

- A factor whose denominator and numerator are both zero (no capital, nothing
  happening) is 1.
- A denominator `≤ 0` otherwise makes TTWROR unavailable:
  `non_positive_capital`, with the date.
- Arithmetic is exact (`big.Rat`) over **unrounded** values and flows
  (`RateTable.ConvertExact`: the same rate selection, coverage and gaps as
  `Convert`, without rounding to the reporting currency's scale). Only shown
  amounts and the final ratios are rounded, once.

**Evaluation days.** A run of days whose factors are all valid telescopes to
the ratio of its end values, so the calculator values only the days the
product and its validity checks actually need (finding 4: compressing over a
day whose denominator would be non-positive hides it). Prices are positive
(a trigger rejects `price_value <= 0`), so if every per-commodity portfolio
balance is non-negative and at least one is positive, `V > 0` with no price
needed. The evaluation days are:

1. the opening and closing days;
2. `d − 1` for every inflow day and `d` for every outflow day;
3. for every day `d` with a portfolio posting whose balances at the end of
   `d − 1` or `d` are not *safe* (some commodity negative, or all zero):
   `d − 1` and `d`;
4. every day of a run with constant but unsafe balances (a short, negative
   cash, a margin liability), because prices alone can turn the value's sign.

The proof is about exact values, which is why the chain must divide exact
values too (second review, finding 2): ten units at €0.00001 are worth
€0.0001, positive capital, though €0.00 at cent scale. Dividing rounded
values would make the result depend on whether that day happened to be an
evaluation day. All-zero balances value at exactly 0 without prices. Every other evaluation
day needs its own valuation; a gap is `valuation_unavailable` for TTWROR only.
This is exactly the daily product for every portfolio, and as cheap as the
original breakpoint set for long-only portfolios.

## Time-weighted series (slice 3)

The response's `time_weighted_series` is the cumulative TTWROR from the
opening boundary through the end of each point's day, read from **the same
exact chain** as the headline figure, never recomputed per point or from
rounded values.

- **Points.** The opening day (always `0` when the chain is defined), then
  every day for periods of at most 92 days (`day`), every seventh day counted
  from the opening day for at most 731 days (`week`; no locale week start),
  or every calendar month end otherwise (`month`), and always the closing
  day. The interval is echoed. A long period therefore needs at most a few
  hundred valuations.
- **Why extra points are safe.** A point that is not already an evaluation
  day lies in a run whose validity the evaluation days proved, so its value
  only splits a telescoping run: it changes no factor, cannot introduce
  non-positive capital, and the closing point equals `time_weighted` (or
  carries its reason) by construction. Every ledger test asserts this.
- **Coverage per point: one failure policy.** Each point applies the
  headline's policy as if it closed the period, in this order: an unknown
  flow (`flow_unavailable`), the opening valuation, the point's **own**
  valuation (dated at the point), the first other required valuation before
  it, then the chain's non-positive capital at or before it. The headline
  TTWROR *is* the closing point, so a missing closing valuation outranks an
  earlier gap or capital failure in both (review of `df4a75b1`, finding 1:
  the series had picked the first failure in date order and disagreed). A
  point-only price gap removes only that point. The series' `valuation`
  covers only point-only days, so a chart gap never reads as a gap behind the
  headline figures; its stale quotes are disclosed with the chart (finding 2:
  they had been dropped).
- **Consequence.** The policy is per point, not monotone: a point after a
  capital failure can report a later missing price of its own, and points
  after a required gap report that gap. Gain and IRR keep their own
  dependencies (the two boundary values and the flows).
- **No capital** (zero opening value and no flows) makes every point
  `no_capital`. Before the first inflow of a portfolio that starts empty,
  factors are 0/0 and the points read `0`, as the chain does.
- **Presentation.** The chart draws calendar time on x and breaks the line at
  every missing point; it never interpolates or fills a gap. A table of every
  point, with its reason in words, is the exact and accessible alternative.

## Allocation at the closing day (slice 4)

The response's `allocation` splits the closing value at `to`. It reads the
same image, scope and `RateTable` as `closing_value`, so with every balance
valued its denominator **is** the closing value (exact conversion is linear;
the tests assert equality). No separate request, date or scope exists.

- **Rows.** Each in-scope (account, commodity) ledger balance at the end of
  `to` is valued exactly at `to`. Its sign makes it **long** or **short**: a
  negative cash balance, a margin liability or a named short is short. A
  group sums its long balances and its short balances as *separate rows*;
  long and short are never netted inside one label. A single account's
  balance in one commodity is the ledger's own net and is not split.
- **Dimensions.**
  - *Account*: the outermost in-scope ancestor (the brokerage container a
    holding sits under), else the account itself.
  - *Asset type*: `cash` for a currency commodity; otherwise the instrument
    version's `instrument_type` effective on `to`; `unclassified` when no
    version is effective that day (never borrowed from a later version).
  - *Currency*: a currency is its own; otherwise the effective instrument
    version's quote currency, else none (`currency_id` null).
- **Weights.** `value / denominator`, exact, six places half away from zero;
  short weights are negative and each dimension's exact weights sum to 1
  (shown weights and values are each rounded once, so their sums may differ
  in the last place).
- **Unknown values.** A balance with no usable price or rate is listed in
  `unknown` by its units and is in no row or weight. `basis` is then
  `known_only` and the denominator is the sum of the known values; with
  everything valued it is `closing_value`.
- **Undefined shares.** A denominator `≤ 0` keeps every value and sets every
  weight null with `weights_reason: non_positive_value`.
- **Unclassified short** (the net-worth rule, as for `closing_value`) makes
  the whole allocation unavailable (`reason: unclassified_short`): its value
  is no better defined than the closing value's.
- **Coefficient range (review of `62f9201d`, finding 1).** Every reported
  amount, and every balance valued along the way, is written at the
  reporting (or posting) scale or, when that exceeds 38 digits, at a
  shallower scale that drops only trailing fractional zeros, so its value is
  unchanged. An amount that fits at no exact scale fails the whole read with
  `LEDGER_OVERFLOW` (422), as ledger balances do; it is never a null figure
  without a reason, and never relabelled a missing price. Offsetting 10^37
  balances therefore show as ±10^37 rows at scale 0.
- **Presentation.** One dimension at a time (account, asset type, currency),
  each row with value, share and a bar; a short row says "Short" in words.
  Shares carry no "+" sign, a short share a minus. Known-only basis,
  undefined shares and the unknown list are disclosed beside the rows. On a
  known-only basis a non-positive denominator says the *priced* holdings are
  worth zero or less and the total is unknown (finding 2): an unpriced
  portfolio's value is not zero.

## Benchmark (slice 5)

The response's `benchmark` (null unless requested) compares the portfolio
with **one owner-selected security or crypto commodity**
(`benchmark_commodity_id`), read as a hypothetical single unit held from the
opening boundary through `to`. Any commodity with observations qualifies,
held or not; manual observations are as valid as provider ones, and no live
provider is needed.

- **Value.** One unit is valued exactly in the reporting currency on each
  day by its own `RateTable` over the read's window and staleness rule, so a
  benchmark quoted in another currency includes the FX change, as the
  portfolio's figures do (#281 chain). Its gaps and stale quotes are reported
  in `benchmark.valuation` and never mark the portfolio's coverage incomplete.
- **Basis, declared, never inferred.** `benchmark_basis` is `price_return`
  (the default: raw observations under `live_raw_v1`, distributions
  excluded) or `total_return`: only observations recorded with
  `adjustment_basis = total_return` price the unit, under
  `live_total_return_v1` (`live_raw_v1` with that one change; currency legs
  stay raw). A raw price never stands in for a total-return one; a
  total-return request without such observations is unavailable, and the gap
  reads `incompatible_quote_basis`. `dividend_adjusted` and `split_adjusted`
  series are not accepted in v1.
- **Splits (review of `eeab1a2a`, finding 1).** Raw quotes are per current
  unit, so a price-return benchmark holds the units one unit becomes: from
  each effective recorded split of the instrument (any holding, inside or
  outside the portfolio; reversed and replaced splits excluded), its unit
  count is multiplied by the ratio on the split's effective date, exactly as
  a holding's ledger units change that day. A split is therefore neutral;
  reading raw quotes alone turned a 2:1 split into −50 %.
  - *Quote basis (review of `6cfd060d`, finding 1).* A quote prices a unit of
    its own day, and a stale quote can sit on either side of a split. With
    C(x) the product of split factors through x, the opening unit is worth
    `price(q) × C(q) / C(open)` for the quote date q actually used, so splits
    are read from the staleness window before the opening day. Multiplying a
    pre-split quote by a later split had read a 2:1 split as +100 % on the
    days that reused the old quote, and a stale opening quote before a split
    just before the period as −50 %.
  - *One factor per day (finding 2).* A holding's successive splits on one
    day compound (2:1 twice is 4:1, as its ledger units do); holdings that
    recorded the same event must agree on the day's factor. Deduplicating
    by ratio had dropped the second split. A day holdings disagree on is a
    conflict: a value whose span from q to the opening day crosses it is
    `conflicting_splits` (dated the day), never a guessed factor.
  - *Shared factors (#407).* The per-day factors and conflicts are the
    benchmark rate table's own, the ones every ledger valuation applies;
    the benchmark reads its quotes unadjusted (`ConvertExactObserved`) and
    applies `C(q) / C(open)` itself, because its unit is the opening day's.
  - *Same-day order (combined review of `cae8039d`, `21106562`, `129abf3f`).*
    On a split day, an event ordered before the split in replay order
    carried pre-split units (see the reports plan's *Same-day order*). A
    trade-implied quote from such a trade is brought to end-of-day units
    before any factor applies (it had valued 22 post-split units at the
    pre-split 100: +100 %), and a delivery's posted units are multiplied by
    the factor still pending after its operation before they are valued at
    the day's end-of-day rate (it had counted a 1-unit pre-split delivery as
    one post-split unit: a contribution of 50, not 100).
  - The factors used are listed in `benchmark.splits` as exact fractions in
    lowest terms (decimal strings). An instrument with an
  unrecorded split cannot be detected; the list shows what was applied. A
  total-return series is a continuous index level, never split-adjusted, and
  its list is empty. Other corporate actions (spin-offs, mergers, cash in
  lieu) are distributions and stay excluded from a price return.
- **Return over the same breakpoints.** A benchmark has no flows, so its
  daily chain telescopes exactly to `V(d) / V(open)` (prices are positive);
  requiring the portfolio's evaluation days would add failures that change no
  figure. `benchmark.points` are the portfolio series' dates; the headline
  `time_weighted` is the closing point and is annualized under the same
  365-day rule.
- **Failure policy.** A missing opening value makes every point
  `valuation_unavailable` dated the opening day; a missing point value removes
  that point alone, dated itself; then a split conflict at or before the point
  (the closing point is the headline).
- **No other figure changes.** Requesting a benchmark leaves every portfolio
  figure, series point, allocation and coverage byte-identical (tested).
  No excess-return figure is computed in v1: the reader compares the two
  returns side by side; an arithmetic or geometric difference is a separate
  choice.
- **Presentation.** The screen offers instruments as benchmarks, keeps the
  choice and basis in the URL (an id that is not an instrument is disclosed
  and never sent), shows the benchmark's period and annual figures with its
  basis in words and the splits it held through, and draws it on the chart's
  axes as a dashed line (a non-colour cue) with a legend and its own column
  in the table of points. A coverage gap is named by its missing leg (finding
  2): a price, a total-return price, or the quote currency's exchange rate
  when the gap carries `via_commodity_id`.
- **Recording total-return observations.** The general price API accepts any
  adjustment basis. The browser's manual price form offers unadjusted (the
  default) or total return for a security or crypto price (slice 7), and a
  correction keeps the replaced price's basis.

**Benchmark cases (slice 5).** S1 portfolio; benchmark IDX priced in EUR.

| Case | Observations | Benchmark |
|---|---|---|
| B1 price return | raw 200 (01-31), 220 (07-31), 210 (close); total_return 100 (01-31), 999 (02-14), 115 (close) | `0.050000`, p.a. `0.050000`; 02-07 `0` (7-day-old price); 02-14 `valuation_unavailable` on its own date (the total_return 999 is not read); 08-01 `0.100000`; portfolio `0.090909`, its coverage complete |
| B2 total return | B1 observations | `0.150000`; 02-14 `8.990000`; 08-01 unavailable (no raw price is borrowed); policy `live_total_return_v1` |
| B3 basis refused | raw only, 50 → 55 | total return: headline, p.a. and every point `valuation_unavailable` dated 01-31; gap `incompatible_quote_basis` |
| B4 FX | SPX 100 USD × 0.90 → 110 USD × 0.80 | 90 → 88 EUR: `−0.022222`; coverage names the USD leg |
| B5 closing gap | raw 200 (01-31), 210 (06-30) | full year: headline and p.a. unavailable dated the closing day; 02-07 `0` |
| B6 short period | B5, 02-01..06-30 | `0.050000`; p.a. `period_shorter_than_year` |
| B7 split | TEST itself; 2:1 split 07-01; raw 101 (06-27), 51 (07-01), 50 (close) | portfolio and benchmark `0.000000`; 06-27 `0.010000`; 07-04 `0.020000` = 2 × 51 / 100; splits `[2:1 on 07-01]` |
| B8 total return | B7 plus total_return 100 → 110 | `0.100000`, no split applied |
| B9 recorded twice | B7 split in two holdings | applied once, `0.000000` |
| B10 conflict | 2:1 and 3:1 on 07-01 in two holdings | 06-27 `0.010000`; 07-04, headline and p.a. `conflicting_splits` dated 07-01 |
| B11 reversed | B7 split reversed, period to 08-01, 102 on 08-01 | `0.020000`, no split applied |
| B12 stale quote after a split | 2:1 on 02-02; raw 100 (01-31), 50 (02-04); 02-01..02-04 | 02-02, 02-03 (01-31 quote) and close `0.000000`; factors `[02-02 2/1]` |
| B13 stale opening before a split | raw 100 (07-29); 2:1 on 07-30; 50 (08-03); 08-01..08-03 | opening 100 / 2 = 50 → 50: `0.000000`; factors `[07-30 2/1]` |
| B14 compounding | 2:1 twice on 02-02 in one holding; 25 on 02-02 | `0.000000`; factors `[02-02 4/1]` |
| O1 trade before split | buy 1 for 100 from broker cash 02-02, then 2:1 that day; 02-01..02-03 | closing 1,100 = 22 × 50; TTWROR `0.000000`; holdings value 1,100 |
| O2 trade after split | 2:1 on 02-02, then buy 1 for 50 | closing 1,100 = 21 × 50 + 50; `0.000000` |
| O3 delivery before split | external transfer-in of 1 on 02-02, then 2:1; stale (01-31) or fresh (50 on 02-02) quote | contribution 100, closing 1,200, `0.000000` |
| O4 quote from another holding | holding A sells its 10 at 100 on 02-02, then holding B (1 unit) records 2:1 | closing 1,100 = 1,000 + 2 × 50; portfolio and benchmark `0.000000` |
| O5 replacements | O1, then the buy and the split are replaced (split 4:1) | closing 1,100 = 44 × 25; `0.000000` |
| O6 between splits | 2:1, buy 1 for 50, 2:1, one holding, one day | closing 1,100 = 42 × 25 + 50; `0.000000` |
| O7 disputed basis | B records 2:1, then a buy in A, then A records 2:1 | closing value `valuation_unavailable`, gap `conflicting_splits`; holdings unvalued |

**Review of `eeab1a2a` (2026-10-25).** Both findings accepted: (1) a
price-return benchmark read raw quotes for one fixed unit, so a recorded 2:1
split showed −50 % against a neutral portfolio — now split-adjusted from the
recorded splits (B7–B11); (2) a missing quote-currency rate was disclosed as a
missing price — now named as the exchange rate (browser case in
`investment-returns.spec.ts`). Independent coverage, strict total-return
selection, FX inclusion, shared chart dates and axes, URL persistence and
API-only total-return entry were reviewed and kept.

**Review of `6cfd060d` (2026-10-25).** Both findings accepted (quote basis,
same-day compounding; see *Splits*), cases B12–B14, plus the reviewer's
replacement probe kept as a regression. Out of scope and tracked separately:
the shared `RateTable` values post-split ledger units with a stale pre-split
quote too, so the portfolio's own series (and every live valuation) reads a
split as a jump on days that reuse the old quote. Fixed in
[#407](https://github.com/sergeyfarin/rekenraam/issues/407) (2026-10-25): the
reproducer (2:1 on 02-02, quotes 100 on 01-31 and 50 on 02-04) read
`0.909091` on 02-02 and 02-03 and now reads `0.000000`
(`TestReturnsSeriesRescalesAStalePreSplitQuote`).

**Combined review of `cae8039d`, `21106562` and `129abf3f` (2026-10-25).**
Both findings accepted: a trade-implied quote from a trade ordered before a
same-day split, and a delivery posted before one, were read in post-split
units (cases O1–O3, `investment_returns_split_order_test.go`, including
holdings through the dated valuation read). The chained-FX benchmark probe
passed and is kept as a regression.

**Review of `942507ae` (2026-10-25).** Accepted: the quote rule consulted
only the trade's own holding, so a sale before another holding's same-day
split was read as a post-split price (O4: 2 units valued at 200). A quote's
basis is now instrument-wide, with a disputed basis a gap (O7); quantities
keep the holding rule. The reviewer's replacement and between-splits probes
passed and are kept (O5, O6).

## Money-weighted return (IRR)

Investor-perspective flows with the same intraday convention, measured in
days `t` from the opening boundary, Actual/365:

```
−V(open) at t = 0
−In(d)   at t = (d − open) − 1     (start of day)
+Out(d)  at t = (d − open)         (end of day)
+V(to)   at t = days
Σ c_i (1 + irr)^(−t_i / 365) = 0
```

- Uniqueness is accepted **only** under Descartes' rule of signs: exactly one
  sign change in the time-ordered, same-offset-merged flows proves exactly one
  root with `irr > −1`. Norström's criterion is **not** used (finding 2): it
  proves a unique *non-negative* rate, and `−1800, +2270, −309, +10` has
  roots at −95 %, −90 % and +11.1111 %. Anything else is `root_not_unique`.
  Interleaved contributions and withdrawals are therefore unavailable in v1;
  that is the conservative cost of never choosing among roots.
- All flows of one sign (nothing invested or nothing returned): `no_root`.
- Solver (finding 6): solve in log-growth space over the period,
  `u = ln(1 + period MWR)`, so terms are `c_i · exp(−u · t_i / days)`.
  Bracket from `[−1, 1]`, doubling outward until the NPV changes sign; under
  Descartes the sign change must exist, so failing to bracket before
  `|u| > 700` is `numerical_limit`, never `no_root`. Then 200 deterministic
  bisection steps in `float64` over flows scaled by their largest magnitude.
  These are ratios, not money: floating evaluation is allowed here, rounded
  to six places, and never stored as a monetary value.
- The period money-weighted return is `exp(u) − 1`; the annual IRR is
  `exp(u · 365 / days) − 1`, shown only for `days ≥ 365`, and
  `numerical_limit` if it overflows.

## Presentation, annualization and day count

- Every return is a decimal fraction string, rounded half away from zero to
  six places (`0.090909` = 9.0909 %). The screen formats percentages.
- Annualized figures only when `days ≥ 365`: TTWROR p.a.
  `= (1 + R_twr)^(365/days) − 1`; IRR is already annual. Shorter periods
  return `null` with `period_shorter_than_year`. Leap days count as days
  (Actual/365, as Portfolio Performance does).
- Also returned, when available, in the reporting currency at its scale:
  opening value, closing value, contributions `ΣIn`, withdrawals `ΣOut`, and
  absolute gain `V(to) − V(open) − ΣIn + ΣOut`.

## Failure states and coverage

Each figure is independent: a failure removes only the figures that need the
missing input (ADR 0017).

| Reason | Removes |
|---|---|
| `valuation_unavailable` / `unclassified_short` at the opening or closing day (with date) | that value, absolute gain, TTWROR, IRR |
| `valuation_unavailable` / `unclassified_short` at any other evaluation day | TTWROR only (finding 5: gain and IRR need only the boundary values and the flows) |
| `flow_unavailable` (a flow could not be converted) | contributions/withdrawals, gain, TTWROR, IRR |
| `non_positive_capital` (with date) | TTWROR |
| `no_root`, `root_not_unique`, `numerical_limit` | IRR |
| `period_shorter_than_year` | annualized TTWROR/IRR only |
| `non_positive_growth` (period return ≤ −100 %) | annualized TTWROR only |
| `no_capital` (opening value zero and no flows) | TTWROR and IRR |

The response carries the shared `ValuationCoverage` (rate uses and gaps) and
the attribution fallback list. Stale (in-window, earlier-dated) prices are
used and reported as stale, never refused.

## Reproducibility

Slice 1 is a **live read**: one disposable consistent image of the database
(the gains reader's `LiveGainsSnapshot`), today's effective history and the
UTC knowledge instant of the read, echoed as `knowledge_cutoff`. A later
correction restates an earlier period, as every live report does. Saved,
immutable return runs (input fingerprint, frozen output bytes) are slice 6
and follow the R18 gains-run pattern; a live read never saves one.

## Saved return runs (slice 6)

A saved run is an **immutable returns_v1 document**, saved explicitly by the
owner, so a later correction, price withdrawal or software change can never
restate a figure the owner kept. It reuses the R18 gains-run mechanism
(`docs/plans/investment-reporting-plan.md`, *Immutable operational runs
(#234)*) with its own table, because the request, descriptor and output of a
returns read are not a gains document; the rejected alternative
(knowledge-time reconstruction) is rejected here for the same reasons.

- `POST /api/v1/investments/returns/runs` takes the live read's inputs as a
  JSON body (`from`, `to`, `reporting_currency_id`, optional `account_ids`,
  `max_staleness_days`, `benchmark_commodity_id`, `benchmark_basis`),
  validated exactly as the live read. It **computes a fresh run**: it never
  freezes a response the browser displayed and accepts no client output.
- The read runs on one fingerprinted online-backup image (the gains-run
  `GainsSnapshot`), with the knowledge instant fixed at `captured_at`, the UTC
  copy-completion time, so `knowledge_cutoff` in the output equals the run's
  `captured_at`. No provider is called. Only the finished document and its
  `investment.returns_run.create` audit event commit to the live book, in one
  transaction; a failed read or write leaves neither.
- The **inputs** document is the resolved request (staleness and benchmark
  basis defaults filled in), `snapshot_sha256`, and a `descriptor` copied
  from the output: `policy`, `from`, `to`, `opening_date`, `days`,
  `account_scope`, `resolved_account_ids`, `knowledge_cutoff`. The
  **output** is the exact compact `InvestmentReturnsResponse` JSON. Both are
  hashed (`inputs_sha256`, `output_sha256`); `snapshot_sha256` identifies the
  full input image, not a portable ledger hash.
- Integrity (on every read, list and in the `investment_foundation`
  self-check): schema 1, both digests, `inputs.snapshot_sha256` equal to the
  row's, `descriptor.policy = returns_v1`, `descriptor.knowledge_cutoff =
  captured_at`, and the descriptor equal to the same fields of the output.
  It checks stored bytes and never compares a saved figure with today's
  book. A damaged run is refused by name, never rebuilt.
- `GET .../returns/runs` lists 20 summaries (inputs only) per page with a
  descending-id `next_cursor`; each run on the page is verified as a whole
  document first, so a page refuses (500) when a run on it is one a single
  read refuses (the lookahead row that only sets the cursor belongs to the
  next page and is not checked with this one); `GET .../returns/runs/{id}` returns the
  preserved document, `?download=true` as a JSON attachment of the server's
  exact bytes. UPDATE and DELETE are refused by triggers; there is no
  update, delete or pruning API. Downgrade refuses while any run exists.
- Backup and restore keep the rows. Bundle schema 18 adds
  `investment-returns-runs.csv` with every run (even in a scoped export).
- The returns screen saves the **applied** selection, lists saved runs and
  reopens one with the same figure, chart, allocation and benchmark views,
  labelled as frozen at its knowledge cutoff, with its fingerprints and a
  download. Figures, dates, ids and coverage come only from the document;
  account, currency and instrument names are today's reference data (ids are
  what the document fixes). Unlike a gains run, no currency metadata is
  captured: every amount is in the one reporting currency, already rounded
  to its standard scale at capture, so a reopened run shows each amount at
  its own stored scale and a later precision change cannot round it again.
- An explicit `max_staleness_days` of `0` is refused (400), as the live read
  refuses it, and so is `null` (the field is not nullable); only an omitted
  field takes the default of 7.

Named validation: `TestReturnsRunReproducesAfterBackdatedBuy` saves a run,
records a backdated buy inside the period, and checks the saved output
reproduces byte for byte while the live read changes, plus bundle, restore,
immutability triggers, integrity refusal of a consistently rehashed tampered
output (refused by both the single read and the list) and the self-check
finding.
`TestReturnsRunValidationPaginationAndReadOnlyRetrieval` covers validation
(no row or audit on refusal; explicit zero or null staleness refused, a
genuinely omitted one defaulted), per-page integrity (a damaged run refuses
only its own page), cursor pagination and that no GET writes. The
browser case in `investment-returns.spec.ts` saves, restates, reopens and
downloads at 390px with an accessibility check.

## Expected cases (independent, hand-derived)

Figures were computed outside the app with Python `fractions`/`decimal`
(scratch script, not code under test). All in EUR, one security ACME priced
in EUR, opening boundary 2026-01-31, period 2026-02-01..2027-01-31
(365 days). The portfolio is broker cash plus the ACME holding; *Checking*
is outside.

| Case | Ledger | Breakpoint values | TTWROR | IRR |
|---|---|---|---|---|
| S1 no flows | cash 100, 10 ACME; price 100 → 110 | 1100 → 1200 | `0.090909` | `0.090909` |
| S2 flows, dividend, fee inside | S1 + deposit 500 on 05-02 (t=90, start); dividend 20 into broker cash 07-31; fee 5 from broker cash 10-31; withdrawal 300 to Checking 11-01 (t=274, end); prices 105 (05-01), 120 (10-31), 125 (01-31) | 1100, 1150 (05-01), 1815 (10-31), 1515 (11-01), 1565 (close) | `0.187954` = 1150/1100 · 1815/1650 · 1565/1515 − 1 | `0.189483` |
| S3 dividend to Checking, attributed | S1 + dividend 20 for ACME paid into Checking 07-31 (t=181); price 105 on 07-31 | 1100 → 1150 (07-31) → 1200 | `0.109881` = 1170/1100 · 1200/1150 − 1 | `0.110074` |
| S4 unassigned dividend to Checking | S1 + unassigned dividend 20 into Checking | as S1 | `0.090909`, fallback disclosed (`unassigned_instrument`, not counted) | `0.090909` |
| S5 missing closing price | S1 without the closing price, beyond the window | — | unavailable `valuation_unavailable`, ACME gap | unavailable |
| S6 short period | S1 over 2026-02-01..2026-06-30 | — | period figure only, p.a. `period_shorter_than_year` | likewise |
| E1 solver | flows −1000, +2300, −1320 | — | — | `root_not_unique` |
| E2 solver | flows −1000 (t=0), +1100 (t=365) | — | — | `0.100000` |
| R1 delivery at market | S1 base, prices 100 on 07-30, 07-31, close; external transfer-in of 1 ACME on 07-31, carried basis 1.00 **or** unknown | 1100 (07-30), 1200 (close) | `0.000000` = 1100/1100 · 1200/1200 − 1; contribution 100, gain 0 | — |
| R2 solver, three roots | −1800, +2270 (t=365), −309 (t=730), +10 (t=1095) | — | — | `root_not_unique` (Norström would accept +11.1111 %) |
| R3 reversal neutrality | S1 base, prices 100 (07-30), 110 (07-31, close); dividend 20 for ACME into Checking on 07-31, reversed | as if neither existed | `0.090909`, no flows | `0.090909` |
| R4 hidden negative capital | broker cash only: 100 → −100 (fee 200 on 07-30) → 100 (income 200 on 07-31) | 100, −100 (07-30) | `non_positive_capital` on 2026-07-30 | — |
| R5 missing intermediate price | S1 base; deposit 500 on 05-02; no price within 7 days of 05-01; price 100 at close | open 1100, close 1600 | `valuation_unavailable` (2026-05-01) | gain 0, IRR `0.000000` |
| R6 one-day IRR | −100 (t=0), +110 (t=1) | — | — | period `0.100000`; annual not shown |
| R7 not-held disclosure | S1 base; all ACME sold 07-01; ACME dividend into Checking 07-31, no ex date | — | — | fallback `not_held`, not counted, disclosed |

**Series cases (slice 3).** S1 base (ACME 100 at the opening boundary):

| Case | Ledger | Points |
|---|---|---|
| C1 daily | period 02-01..02-05; prices 105 (02-02), 110 (02-04); deposit 500 into broker cash 02-03 | 01-31 `0`, 02-01 `0`, 02-02 `0.045455`, 02-03 `0.045455`, 02-04 `0.077135` = 1150/1100 · 1700/1650 − 1, 02-05 `0.077135` = headline |
| C2 weekly, local gap | full year; prices 105 (07-31), 110 (close) | 54 points; 02-07 `0` (7-day-old price); 02-14 and 07-25 `valuation_unavailable` on their own dates; 08-01 `0.045455`; close `0.090909`; headline coverage complete, series coverage names ACME |
| C4 chain stop | R4 ledger (broker cash only) | 07-25 `0`; 08-01 and close `non_positive_capital` dated 07-30 |
| C5 required gap | R5 ledger plus a price on 05-09 | 02-07 `0`; 05-02 own gap dated 05-02; 05-09 and close `valuation_unavailable` dated 05-01 |
| C6 closing gap first | R5 ledger, no closing price | headline and closing point `valuation_unavailable` dated the closing day, not 05-01 |
| C7 closing gap over capital | prices 100 on 07-29..07-31; fee 1,200 from broker cash 07-30, income 1,200 on 07-31; no closing price | 08-01 `non_positive_capital` dated 07-30; headline and closing point `valuation_unavailable` dated the closing day |
| C8 chart-only staleness | prices only at the boundaries (01-31, 02-05), period 02-01..02-05 | headline quotes all exact; series coverage marks 02-01..02-04 stale |

**Allocation cases (slice 4).** S1 base with TEST classified as a stock
quoted in EUR (unless stated), closing price 110:

| Case | Ledger | Allocation |
|---|---|---|
| A1 | holding under a brokerage container | denominator 1200 = closing value; account: container 1100 `0.916667`, broker cash 100 `0.083333`; asset type: stock `0.916667`, cash `0.083333`; currency: EUR long `1.000000` |
| A2 | fee 150 from broker cash 12-01 | denominator 1050; EUR long 1100 `1.047619` and EUR short −50 `−0.047619`, two rows; cash short `−0.047619` |
| A3 | no closing price | `known_only`, denominator 100, cash `1.000000`; unknown: holding, 10 TEST |
| A4 | fee 1,200 from broker cash | denominator 0; `non_positive_value`; rows 1100 long and −1100 short, no weights |
| A5 | TEST without an instrument version | asset type `unclassified` `0.916667`; currency none 1100 `0.916667` beside EUR 100 |
| A6 | −1 TEST posted to broker cash, no short lot | closing value and allocation `unclassified_short` |
| A7 | two extra broker-cash accounts at +10^37 and −10^37 EUR (scale 0) | denominator 1200; those rows exactly ±10^37, written at scale 0 |
| A8 | +10^37 TEST in broker cash, closing price 10.01 | value 1.001 × 10^38 fits no exact scale: the read fails `LEDGER_OVERFLOW` |

Slice 1 tests pin these literals; they never derive expected values from the
calculator. R1–R7 are the reproducers from the 2026-10-24 review of
`e30413a5`; they failed before the corrections recorded here.

**Second review of `ae084c4d` (2026-10-24).**

1. *Trading legs read as deliveries* — accepted. Delivery detection matched
   any equity-class O posting, and production seeds `commodity_trading` as
   equity, so an internal buy invented a contribution and a write-off became
   a withdrawal. Detection now excludes the T leg, and the shared test
   fixture seeds `commodity_trading` as equity like production (it used
   asset, which hid the defect). Cases: internal buy of 1 ACME for 100 at a
   110 close → contribution 0, gain 110, TTWROR `0.100000`; write-off of
   10 ACME → withdrawal 0, gain −1000, TTWROR `−0.909091`.
2. *Rounding to zero* — the defect accepted, the expected answer not. The
   review expected `non_positive_capital` for a holding valued
   1000 → €0.0001 → 1000. Every exact daily value there is positive, so the
   daily product is defined and is exactly 1 (`0.000000`); a zero
   denominator exists only after rounding, which is presentation, not
   capital. The real defect was the inconsistency: had that day been an
   evaluation day, rounded values would have divided by zero. The fix makes
   the chain exact rather than making the proof price every day of every
   portfolio. Cases: the telescoped sequence is `0.000000`; with a
   transfer-out of 1 unit on the €0.00001 day (forcing evaluation),
   (0.00009 + 0.00001) / 1000 · 900 / 0.00009 − 1 = `0.000000`, and a
   cent-rounded chain fails this case.

## Per-instrument returns (`instrument_returns_v1`)

Status: **accepted by the owner 2026-10-25 (D1–D7 as revised, including D5's
bounded short detection); slice 8 (sleeve read) delivered 2026-10-25; slice 9
(screen panel and saved runs) delivered 2026-10-26.** History: proposed and
revised (revision 2) the same day. Revision 2 resolves a same-day review of `4c3324e3` (five
gaps, all accepted; recorded below as "review 1" to "review 5"). The review
also exposed a shipped portfolio defect, [#409](https://github.com/sergeyfarin/rekenraam/issues/409):
unsettled trade cash was counted twice across a period boundary. It is now
fixed by the same trade-date recognition this draft uses for sleeves; see
*Valuation `V(d)`*. Decisions D1–D7
were answered with the recommended answer for each; the alternatives stay
recorded beside them.

A second review (of `e4d0eba3`) judged D1–D7 reasonable to accept, D7's
trade-date FX choice included. It asked for hand-derived foreign-currency
settlement and spin-off cases (now I11, I12, I12b). It also noted that
accepting D5 means accepting **bounded** short detection: named short
operations and end-of-day balances, not every plain intraday position. The
owner accepted that limitation explicitly.

The question: *how did each instrument the portfolio held perform over the
period, including its income, independently of when it was bought or sold?*
It answers that inside the existing returns read. It is read-only like the
rest of R13: income is attached to an instrument at read time, and no
posting, operation or fact is written. Reconciliation, balances and exports
are unchanged.

### Boundary: the instrument sleeve

For instrument commodity `i` (a security or crypto, never a currency), the
**sleeve** `S_i` is every posting of commodity `i` on an account in the
resolved portfolio scope (the same accounts, resolved once as of `to`). A
sleeve holds **units only, never cash**: cash that a sale produces, or a
dividend pays, has left the sleeve, wherever it lands. That is what makes
per-instrument attribution possible without linking holdings to cash
accounts, which the ledger does not do (see *Why not cash side alone*).

- `V_i(d)` = the balance of `i` across the scope at the end of `d`, valued at
  `d` with the read's price-selection rules (same admission, staleness window,
  split rescaling (#407) and `conflicting_splits` gap as `V(d)`). Balances
  that are all zero value at exactly 0 without a price.
- A sleeve is listed when it has a flow recognised in the period, a non-zero
  balance at the opening or closing boundary, or a posting dated in
  `[from, to]`. Rows are ordered by commodity code, then id.

**D1 — scope.** *Recommended:* the sleeve uses the portfolio's resolved
account scope, so a sleeve always describes part of the portfolio being
read. *Alternative:* every holding of `i` in the book, whatever the scope.
That makes a sleeve independent of the account selection, but then sleeves
no longer describe the selected portfolio.

### Sleeve flows: one per operation (review 1)

An investment operation can post on several dates. A trade posts its units
and their `commodity_trading` leg on the trade date, then the cash, its
clearing leg and any included charge on the settlement date
(`tradeJournalEntries`). A separately paid charge posts on its own date. Read
entry by entry, the trade-date entry has no cash and the settlement entry has
no units, so a purchase would gain value without being funded. Sleeve flows
are therefore built **per operation, across all of its entries**:

- **Partition.** Over all of the operation's entries: **P** =
  income/expense postings, **T** = `commodity_trading` postings, and
  **O_i** = every other posting, including other portfolio accounts and
  other commodities.
- **Recognition date `r`.** The date the operation's units post: the trade
  date of a trade, the effective date of a transfer or corporate action, and
  D2's date for income. The whole operation's flow is recognised on `r`. It
  counts in a period iff `r` is in `[from, to]`, whatever the dates of its
  cash entries, as long as they were recorded by the read's knowledge cutoff.
  A trade on `to` that settles later counts. A trade before `from` that
  settles inside the period does not: its flow belongs to the earlier period.
- **Identity.** A correction chain's operations net per (root operation,
  `r`) before the in/out split, as finding 3 does at portfolio level.
- **Entries outside operations** (plain journal entries) keep the
  entry-level rule, with `r` = the entry date.

| Operation | `F_i` (positive = into the sleeve) |
|---|---|
| **Delivery** (an external transfer in/out or its correction, or an equity-class O posting other than T; the portfolio rule) | `Σ value(S_i) + Σ value(P)`, at market on `r` |
| **Exchange** (`share_exchange`, `cash_and_stock_merger`, `spin_off` only; review 5) | by consideration (D6) |
| Any other operation with an `S_i` posting, or with income attributed to `i` | `F_i = −Σ value(O_i)`, each posting converted on `r` (D7) |
| A plain journal entry posting units of two or more non-currency commodities in scope | refused: the involved sleeves' flow-dependent figures are unavailable with `unclassified_exchange` |

The last row exists because a journal entry holding two instruments is not
necessarily a corporate action (review 5). The ledger does not say what was
exchanged for what, so the read never guesses.

**D7 — currency date of settled cash.** *Recommended:* every cash posting of
the operation is converted on `r`, so the flow and the units it buys are
valued on the same day. This is what the portfolio read does since #409.
FX movement between trade and settlement on a foreign-currency payable is a
cash effect: the portfolio values the payable daily, and the sleeve leaves
that movement to the residual. *Alternative:* convert each posting on its own
entry date, which values the funding of one purchase on two days.

Consequences, each an expected case:

- A buy of 1,000 with a fee of 5, settled two days later from broker cash:
  `O_i` = cash −1,005 across both entries, so `F_i = +1,005` on the trade
  date. The fee is expense (P) and is never a flow, so it lowers the
  instrument's return (D4).
- A sale for 995 net: `F_i = −995`. A realized-gain posting is P and changes
  nothing: returns stay ledger-based, never lot-based.
- A transfer between two in-scope holdings of `i` has only `S_i` and T
  postings, so no flow. A transfer to an out-of-scope holding puts the units
  in `O_i` at market, so it is an outflow at market value.
- A split of `i` posts only `S_i` and T: no flow, and neutral through the
  shared rescaling.
- A short sale (units −10, cash +1,000) is an outflow of 1,000. See D5 for
  which figures a short sleeve gets.

**D4 — trading costs.** *Recommended:* costs are net. Every charge linked to
the operation is part of its flow, including one paid separately on another
date (review 1), so the inflow is all the cash paid and the charge lowers
that instrument's return. This matches the portfolio, which is net of fees
and taxes. *Alternative:* gross, with flows at trade value excluding
charges. Charges then disappear from every sleeve and land in the residual.

### Income attribution and timing (review 2)

Income enters a sleeve only through facts that name its instrument; the
journal itself posts only cash, income and withholding.

- **Entitlement** is decided by holdings at the **end of the day before the
  ex date** when an ex date is recorded, else at the end of the entry date.
  This is the rule the portfolio read already applies (`anchorSide`), and it
  matches the dividend-event producer's entitlement date. An owner who sells
  on the ex date is entitled; one who buys that day is not. Attributed when
  `i` is held in scope at that point and nowhere outside it.
- **Dividends and withholding** (`investment_dividend_facts.commodity_id =
  i`). The flow is the net cash, `−Σ value(O_i)`: a gross 20 with 3 withheld
  leaves 17 in cash, so `F_i = −17`, and returns are net of withholding. A
  withholding-only payment of 3 gives `F_i = +3`, a contribution. A reclaim is
  an outflow.
- **Capital returns** name their holding; that account's commodity is `i`,
  so the cash received is an outflow.
- **Payments in lieu** name a holding or an instrument and are attributed
  like dividends. A payment made on a short is an inflow (the sleeve's cost).
- **Attributed income** is reported as one signed total (dividends,
  capital returns and reclaims positive; withholding-only payments and
  payments in lieu negative). Each flow also counts in contributions or
  withdrawals by its own sign.
- **Not attributed:** an unassigned instrument (`unassigned_instrument`),
  nothing held at the entitlement point (`not_held`), or `i` held both inside
  and outside the scope (D3). Each such entry is listed with its transaction,
  date and reason, wherever its cash landed: an entry paid into checking
  outside the portfolio is listed too, as the portfolio read lists its
  fallbacks. Only income for a position held entirely outside the scope is
  not listed. It counts only toward the portfolio's own figures and the
  residual below.
- **Income with a missing conversion.** Attributed income whose cash cannot be
  converted makes the sleeve's income `flow_unavailable`, not
  `income_unattributable`: the payer is known, and the gap is named in the
  sleeve's coverage.

**D2 — recognition date of income.** *Recommended:* attributed income is
recognised on the **ex date** when recorded, else the entry date, and
converted on that date. An entry dated after `to` whose ex date falls in the
period counts, if it was recorded by the read's knowledge cutoff.
Recognition and entitlement are different points: entitlement is the end of
the day before the ex date, recognition is the ex date itself.

*Why:* the price drops on the ex date, and an owner often sells between the
ex date and the pay date. With payment-date timing, such a dividend arrives
in a sleeve with zero capital: `(0 + 17) / (0 + 0)` makes TTWROR
`non_positive_capital` for an ordinary round trip (case I2). The choice also
moves the income's timing in the **money-weighted** return. The response
names the recognition rule (`income_recognition: ex_date`) so the earlier
timing is disclosed. *Cost:* a dividend that goes ex in the period but has
not been recorded yet is missing until it is. That is the usual live-read
restatement, and a saved run freezes it. *Alternative:* payment-date
timing, which matches the portfolio's cash flows but loses TTWROR in that
case.

**D3 — held inside and outside the scope (review 4).** *Recommended:* refuse,
with dependencies that follow the flow's direction. Boundary values stay.
The total that the unattributable flow would have entered (withdrawals for a
dividend, contributions for a withholding-only payment or a payment in
lieu) becomes unavailable with `income_unattributable`, naming the entry,
and so do gain, TTWROR and IRR. The other total stays. This arises only with
an explicit account selection, never under the default scope.
*Alternative:* split by units held inside versus outside at the entitlement
point, as an exact ratio, and disclose the split. It is deterministic, but it
is an allocation the facts do not state.

### Figures

Each sleeve gets the portfolio's figures, computed by the same code paths
from `V_i` and `F_i`:
- opening and closing value, contributions, withdrawals, and signed
  attributed income;
- absolute gain;
- TTWROR (the same evaluation-day chain) and money-weighted return (the
  Descartes-only rule), with annual forms for periods of 365+ days only;
- the sleeve's own coverage and gaps.

A missing price for `j` affects only `j`'s sleeve, plus any exchange flow
that needs `j`. All figures are in the reporting currency, so FX is part of
an instrument's return.

**D5 — shorts (review 3).** *Recommended:* a sleeve is **short in the
period** when either of these holds:
- a `short_sale` or `short_cover` operation chain on `i` in scope, recognised
  in `[from, to]`, leaves non-zero net units after its corrections. A short
  that was recorded and then reversed leaves none, and so does not suppress a
  later long position's return;
- the scope's balance of `i` is negative at the end of any day from `from − 1`
  to `to` (every posting day, not only evaluation days).

A short sleeve keeps its values, flows and absolute gain, but TTWROR and IRR
are unavailable with `short_position`. A sleeve has no cash to carry the
short's capital, so no positive denominator exists. Named operations are
what catch a short opened and covered on the same day: short 10 for 1,000
and cover for 900 leaves zero at both day ends. The plain chain would read
`(0 + 1,000) / (0 + 900) − 1 = 11.1111 %`, which is now refused.

*Limitation, disclosed:* the ledger orders postings within a day only per
account (`account_day_sequence`), not across accounts. A sleeve can span
several holdings, so a negative balance that exists only within one day and
comes from plain entries rather than named short operations is not used to
classify a short. An end-of-day
negative balance outside named shorts is still `unclassified_short`, as at
portfolio level. A same-day long round trip (buy for 900, sell for 1,000)
correctly reads `(0 + 1,000) / (0 + 900) − 1`. *Alternative:* none
proposed. Defining short capital (margin, collateral) is a separate
contract.

**D6 — exchanges (review 5).** *Recommended:* value an exchange by the
**consideration received at the exchange**:
- The source sleeve (exchanged instrument, or spin-off parent) has an outflow
  equal to the market value on `r` of the new units, plus the merger's own
  **contractual cash consideration** (the cash leg of a
  `cash_and_stock_merger`).
- The destination sleeve has an inflow equal to the market value of its own
  units only.

**Cash in lieu is not consideration.** The app records it as a later,
separate disposal of the destination's fractional units, linked by
`settled_operation_id` (#181). It is therefore an ordinary sale in the
destination sleeve. Adding it to the source too would count it twice (case
I4b).

*Why consideration:* a delisted source often has no price on the effective
date, and the deal premium belongs to the holder of the source.
*Alternative:* value each side at its own price, which leaves the source
`flow_unavailable` when it is delisted. In either case, a missing price for
the new instrument on `r` is `flow_unavailable` for both sleeves.

### Non-interference (required)

Requesting instrument returns must not change any portfolio figure, its
coverage, its rate uses or gaps, its series, allocation or benchmark. Sleeve
valuations share the read's snapshot and price-selection rules but use their
own rate tables and coverage, as the series and the benchmark already do. A
gap that only a sleeve needs must never appear in the portfolio's coverage.
Slice 8 asserts that the portfolio part of the response is byte-identical
with and without the flag across the existing case families.

**Residual, not an attribution.** When the portfolio gain and every sleeve
gain are available, the read also returns `not_attributed_gain = portfolio
gain − Σ sleeve gains`. Its label covers what goes there:
- cash interest, and FX on cash (including D7's trade-to-settlement FX);
- fees not linked to an operation, and unattributed income;
- the income timing difference of D2.

It is a difference, never a computed breakdown. Sleeve returns are not
weighted into the portfolio return, and the response says so. The portfolio
and the sleeves both recognise trades on their trade date (#409), so
settlement timing never lands in the residual.

### API, screen, saved runs

- `GET /api/v1/investments/returns?…&instruments=true` adds
  `instrument_returns: {policy: "instrument_returns_v1", income_recognition,
  rows, not_attributed, not_attributed_gain}` from the same snapshot. That is
  one request for the page, with no per-row fan-out. Without the flag the
  response is unchanged.
- **Saved runs.** The flag joins the request descriptor **only when true**,
  so every existing run's descriptor, fingerprints and bytes are unchanged
  and keep verifying. A new run with the flag stores the field. No migration
  is needed, and the bundle CSV gains no column. Slice 9 asserts an existing
  run reopens unchanged.
- **Screen.** A "By instrument" panel under the portfolio figures: a list of
  cards on a phone, a table on wider screens. Each figure carries its reason
  in words, and the panel shows the residual, the not-attributed list and the
  income recognition rule. Loading, empty ("no instruments in this period"),
  error and success states are required.
- **Not in v1:** native (quote-currency) per-instrument returns; the
  per-instrument benchmark comparison; contribution-to-return attribution
  (Brinson and similar).

### Expected cases (hand-derived; period lengths in days)

| Case | Facts | Expected |
|---|---|---|
| I1 settlement and fee | period 02-01..02-28 (28 days); buy 10 at 100 traded 02-01, settled 02-03 from broker cash with a fee of 5; price 110 on 02-28 | inflow 1,005 recognised 02-01; closing 1,100; gain 95; TTWROR `1100/1005 − 1 = 0.094527`; period MWR `0.094527`; annual figures `period_shorter_than_year` |
| I1b settles inside, traded before | 10 bought at 100 traded 01-31, settled 02-02; price 100 on 01-31, 110 on 02-28 | no sleeve flow in the period; opening 1,000; TTWROR `0.100000`; gain 100 |
| I1c traded on `to`, settles after | I1 with the trade on 02-28 and settlement on 03-02, price 100 on 02-28 | inflow 1,005 on 02-28; closing 1,000; gain −5; TTWROR `1000/1005 − 1 = −0.004975` |
| I1d separately paid charge | I1 with the fee of 5 paid separately from checking on 02-10 | identical to I1: one inflow of 1,005 on 02-01 |
| I2 ex-date income, exit before pay | holds 10 at end of 02-09, price 100 on 01-31; ex 02-10, price 98, gross 20, withholding 3, net 17 paid 02-20; sells all 10 for 1,050 on 02-15 | withdrawals 1,067; income 17; gain 67; TTWROR `(980+17)/1000 · 1050/980 − 1 = 2991/2800 − 1 = 0.068214`; period MWR `0.129425` (−1,000 at t 0, +17 at t 10, +1,050 at t 15); `income_recognition: ex_date`; with payment-date timing TTWROR would be `non_positive_capital` on 02-20 |
| I2b same-day trades on the ex date | (a) sells all 10 on 02-10; (b) holds nothing at end of 02-09 and buys 10 on 02-10; (c) moves all 10 to an out-of-scope holding on 02-10 | (a) attributed, income 17 on 02-10; (b) `not_held`, not in the sleeve; (c) attributed, since the units were held only inside at the entitlement point |
| I3 held inside and outside | I2 plus 5 units out of scope at end of 02-09; scope = the in-scope account | values and contributions present; withdrawals, gain, TTWROR, IRR `income_unattributable` naming the dividend |
| I3b ambiguous withholding-only | holds 10 inside and 5 outside; a withholding-only payment of 3 | withdrawals present; contributions, gain, TTWROR, IRR `income_unattributable` |
| I4 cash-and-stock merger | period 03-01..03-31; A 10 units, price 100 on 02-28; on 03-01, 10 A → 5 B plus 7 contractual cash; B price 210 on 03-01, 220 on 03-31 | A: outflow 1,057, TTWROR `0.057000`; B: inflow 1,050, closing 1,100, TTWROR `1100/1050 − 1 = 0.047619` |
| I4b fraction settled in cash | A 10 units worth 1,000 (price 100 on 02-28) → 10.5 B at 100 on 03-01 (share exchange); cash in lieu sells 0.5 B for 50 on 03-05; B 100 on 03-31 | A: outflow 1,050, gain 50, TTWROR `0.050000` (not 1,100 / 100); B: inflow 1,050, outflow 50, closing 1,000, gain 0, TTWROR `0.000000` |
| I5 transfers | 10 units moved between two in-scope holdings, then 4 to an out-of-scope holding at price 105 | no flow for the first move; outflow 420 for the second |
| I6 short | short 10 at 100 (proceeds 1,000) on 02-02; price 90 at the close | gain 100; TTWROR and IRR `short_position` |
| I6b intraday short | short 10 for 1,000 and cover 10 for 900, both on 02-10 | gain 100; TTWROR and IRR `short_position` (not `0.111111`) |
| I6c intraday long | buy 10 for 900 and sell 10 for 1,000, both on 02-10 | gain 100; TTWROR `1000/900 − 1 = 0.111111` |
| I7 unassigned dividend | a dividend with no instrument, paid into broker cash | in no sleeve; listed `unassigned_instrument`; in the portfolio figures and the residual |
| I8 residual | I1 inside a portfolio of broker cash funded by a 1,005 deposit | portfolio gain 95 = sleeve gain 95; `not_attributed_gain` 0 |
| I9 two-instrument journal | a plain journal entry: A −10, B +5 | both sleeves' flow-dependent figures `unclassified_exchange`; portfolio unaffected |
| I11 foreign-currency settlement (D7) | reporting EUR; USD broker cash 1,000 at the opening boundary; USD→EUR 0.90 on 01-31 and 02-01, 0.95 on 02-03, 0.92 on 02-28; buy 10 at 100 USD traded 02-01, settled 02-03 from that cash; price 110 USD on 02-28 | sleeve inflow `1,000 × 0.90 = 900` on 02-01; closing `1,100 × 0.92 = 1,012`; gain 112; TTWROR `1012/900 − 1 = 0.124444`; portfolio gain 112, `not_attributed_gain` 0. Under D7's alternative the inflow would be 950 (gain 62) and the 50 FX move would land in the residual |
| I12 spin-off (D6) | P 10 units, price 100 on 02-28; on 03-01, 1 C per 2 P (5 C) spun off; P 90 and C 20 on 03-01; P 95 and C 22 on 03-31 | P: outflow `5 × 20 = 100` on 03-01, closing 950, gain 50, TTWROR `(900+100)/1000 · 950/900 − 1 = 0.055556`; C: inflow 100, closing 110, gain 10, TTWROR `0.100000`; portfolio TTWROR `0.060000`, gain 60, `not_attributed_gain` 0 |
| I12b spin-off, child unpriced | I12 with no C price within the window of 03-01 | P and C: withdrawals (P) / contributions (C), gain, TTWROR and IRR `flow_unavailable` dated 03-01; values present; portfolio figures unchanged (no flow on 03-01, and 03-01 is not an evaluation day) |
| I10 non-interference | every existing portfolio case (S, R, C, O, B families) with `instruments=true` | portfolio part of the response byte-identical to the read without the flag |

Tests must own these fixtures and pin these literals, as the slice 1 cases
do. Each case also asserts that no posting, operation or audit row was
written.

### Slices

8. **Sleeve read**: `instrument_returns` on the returns read, OpenAPI, and
   cases I1–I12. *Delivered 2026-10-25*. Implementation notes:
   - The code is `app/investment_returns_instruments.go`, with cases in
     `investment_returns_instruments_test.go`.
   - Every case reads twice and requires the portfolio part to be
     byte-identical (I10), and asserts that nothing was written.
   - A sleeve is the portfolio reader restricted to one commodity's postings,
     run through the same `compute`. Each sleeve values through its own
     fresh-coverage copy of a rate table loaded once for all sleeves, so no
     sleeve gap reaches the portfolio's coverage.
   - Exchange facts come from `investment_transfer_facts`. A reversal mirrors
     every leg and cancels, which is pinned by
     `TestInstrumentReturnsReversedExchangeCancels`.
   - **Ex dates in practice:** a cash dividend, a withholding-only payment
     and a payment in lieu can each record an ex date (dividends since
     2026-10-25; contract in the
     [operation plan](investment-operation-refactor-plan.md), "Dividend ex
     dates"). A dividend recorded without one, including every imported
     dividend, falls back to its entry date, and an owner who sold before
     the pay date sees it listed `not_held` until a replacement adds the ex
     date. The I2 cases record their ex dates through the dividend command.
   - **Portfolio recognition (2026-10-25, dividends receivable).**
     The returns read (`recognise`) also recognises an operation with no
     units on its recorded ex date, the rule net worth applies (reports
     plan, "Dividends receivable"). Between the ex date and payment, the portfolio holds the
     dividend as a receivable; payment is settlement and creates no second
     flow. The receivable belongs to whoever is entitled until its cash
     moves (review 2026-10-25). When the cash lands on the entitled side,
     its posting simply moves to the ex date. When it crosses the boundary
     (paid into checking for a holding in scope), the entitled holding holds
     the receivable from the ex date, and the flow is counted on the payment
     date, when the cash crosses. Allocation shows it as cash under that
     holding. A receivable is currency, never position evidence: which side
     a journal belongs to is read from units only. Cases
     `TestReturnsKeepAnOutsidePaidDividendReceivableUntilPayment`,
     `TestReturnsWithdrawAnOutsidePaidDividendOnPayment`,
     `TestReturnsHoldADividendReceivableForAnAccountNotYetOpen`,
     `TestReturnsCorrectedPayerAcrossScopeCreatesNoGain`.
   - **Cancelled income (review 2026-10-25).** Before a sleeve refuses,
     converts or lists income, income cash is netted exactly in native
     amounts per subject operation, day and target instrument. The subject
     is the operation a journal carries the effect of: its own, or for a
     reversal or a replacement's inverse, the one it undoes. A pair that
     nets to zero is dropped, and a replacement in the same chain survives.
     The portfolio does the same per chain and day before converting, and
     converts only what survives: a commodity that nets to exactly zero
     (unless a same-day split scaled it) is never converted, so it needs no
     rate and adds no coverage gap or stale-rate notice. Reversed ambiguous income, an ex-date correction
     that resolves ambiguity, and reversed income with no rate therefore
     leave the sleeve available. Cases
     `TestInstrumentReturnsReversedAmbiguousIncomeLeavesTheSleeve`,
     `TestInstrumentReturnsExDateCorrectionClearsTheOldAmbiguity`,
     `TestInstrumentReturnsReversedIncomeNeedsNoConversion`,
     `TestInstrumentReturnsHoldingRepairCancelsTheAmbiguousPaymentInLieu`,
     `TestReturnsReversedForeignDividendPaidOutsideNeedsNoRate`,
     `TestReturnsCancelledForeignDividendLeavesNoCoverageDependency`.
   - **Review of `7d1996c3` and `8194782a` (2026-10-25).** All four findings
     were accepted and fixed. Each reviewer probe is a named regression case
     that failed before its fix:
     1. *Corrected payer:* a replacement links its inverse and its new
        journal to one operation, so sleeves group by transaction, not
        operation. The inverse keeps its predecessor's facts, so a €20
        dividend moved from TEST to OTHER leaves TEST with 0
        (`TestInstrumentReturnsCorrectedPayerMovesTheIncome`).
     2. *Reversed short:* short exposure is the net units of each short
        chain (`TestInstrumentReturnsReversedShortDoesNotBlockALaterLong`).
     3. *Not-held income paid outside:* listed wherever its cash landed
        (`TestInstrumentReturnsNotHeldIncomePaidOutsideIsDisclosed`).
     4. *Missing FX:* income keeps `flow_unavailable`
        (`TestInstrumentReturnsKnownIncomeWithMissingFXIsAFlowGap`).
9. **Screen panel and saved-run inclusion**, with a browser case on a phone
   and an existing run reopened unchanged. *Delivered 2026-10-26*:
   - **Selection.** A "Show returns by instrument" checkbox in the returns
     controls, applied with the rest and kept in the link as
     `instruments=true` (any other value reads as off). Off is the default, so
     every existing link opens as before.
   - **Panel** (`returns-instruments.svelte`, under the allocation). One card
     per instrument on a phone, one table from `md` up (rendered from the
     same rows; the hidden layout is out of the accessibility tree). Each
     figure that is unavailable shows its reason in words, including the
     slice 8 codes `short_position`, `income_unattributable`,
     `unclassified_exchange` and `instrument_gain_unavailable`. Each
     instrument names its own missing and stale prices from its own
     `valuation`, never the portfolio's. The panel also shows the
     not-attributed residual, the income no instrument carries (linked to
     its transaction), the ex-date recognition rule, and an empty state ("No
     instruments in this period."). Live and saved results render through
     the same component; a saved one keeps its stored scales.
   - **Saved runs.** `ReturnsRunRequest.instruments` is recorded in the
     resolved request only when true (`omitzero`), so a run without it keeps
     the slice 6 bytes. Integrity (single read, list, self-check) adds one
     rule: the request's `instruments` is true exactly when the output carries
     `instrument_returns`. The run list labels such a run "By instrument".
   - **Named validation.** `TestReturnsRunSavedBeforeInstrumentsReopensUnchanged`
     inserts a golden document saved by the slice 6 code
     (`backend/internal/api/testdata/returns-run-slice6.json`) and requires
     its single read, list entry and self-check to return it unchanged; a new
     run with `instruments: false` keeps that request shape.
     `TestReturnsRunFreezesInstrumentReturns` corrects one dividend's payer,
     reverses another and records an unassigned one. It requires the saved
     output to equal the live read byte for byte (apart from the knowledge
     instant), pins the sleeves (income 20 moved to the corrected payer,
     TTWROR `0.020000`; the other sleeve's income 0 and its missing closing
     price named in its own coverage; one `unassigned_instrument` entry), and
     checks that a later reversal restates only the live read. It also
     checks that a flag without the output, or the output without the flag,
     is refused even when consistently rehashed. Browser case `[acceptance]
     returns by instrument agree live and saved, through corrected and
     cancelled income, on a phone` (`investment-returns.spec.ts`) checks
     the same facts at 390px and 1280px, with accessibility checks at both.
     The slice 6 browser case now also asserts that a run saved without the
     flag has no panel.
   - **Review of `6dedbb83` (2026-10-26).** One P2 finding, accepted: the
     not-attributed help claimed every such payment "still counts in the
     portfolio figures". An unassigned dividend paid into checking outside
     the portfolio is not counted (case S4, `counted: false`). The panel now
     claims nothing general. It shows each entry's own treatment, read from
     the portfolio's `attribution_fallbacks` for the same transaction in the
     same document ("counted as portfolio income" or "not counted"), and
     says nothing for an entry without one. The browser case asserts "not
     counted" for that entry. The reviewer's probe also showed the table
     overflowing its box at 768px without being focusable, so the box is now
     a labelled, focusable region, with an accessibility check at that width.

Slices 8–9 complete the accepted instrument contract. Targets and rebalancing
stay deferred in #411 and #412; see the acceptance disposition above.

## Slices

1. **Backend live read** — `GET /api/v1/investments/returns`, OpenAPI,
   calculator and ledger/attribution reads, the cases above. *Delivered.*
2. **Returns screen** — period/account/currency selection, figures with
   reasons, fallback disclosure; loading/empty/error/success; mobile.
   *Delivered 2026-10-24* at `/app/investments/returns`: the selection lives
   in the URL; the reporting currency defaults to the book's default
   currency for new UI (shown and changeable, never a hidden base currency);
   presets are year to date and the last 12 months (the shortest period with
   annual figures); every unavailable figure shows its reason in words, and
   attribution fallbacks link to their transactions. A review of
   `f0b3b8ec` (four findings, all accepted) added: returns are invalidated by
   price corrections and are always refetched on a visit (`staleTime` 0,
   with an "updating" note), because the ledger writers that change them have
   no single invalidation hook; archived and closed accounts stay selectable,
   labelled with their status; a selected id that is not a selectable account
   is listed, removable and never sent, with a reset to every investment
   account; and missing and stale coverage are disclosed independently.
3. **Cumulative TTWROR series and chart** — points from the same chain,
   coverage per point (see *Time-weighted series*). *Delivered 2026-10-24*:
   `time_weighted_series` on the returns read, and a line chart with a table
   of points on the returns screen.
4. **Allocation at a date** — weights by account, instrument type and
   currency; denominator = closing portfolio value; unpriced holdings in an
   explicit unknown group and weights labelled known-only. Long and short
   are never netted inside a group label. *Delivered 2026-10-24* as
   `allocation` on the returns read at `to` (see *Allocation at the closing
   day*) and an allocation panel on the returns screen.
5. **Benchmarks** — an owner-selected price series (manual observations are
   valid); price-return unless the series is declared total-return;
   benchmark TTWROR over the same breakpoints; no live provider required.
   *Delivered 2026-10-24* as `benchmark` on the returns read (see
   *Benchmark*), a benchmark card and a dashed chart line on the screen.
6. **Saved return runs** — immutable output and input fingerprint, as R18.
   *Delivered 2026-10-25*; see *Saved return runs (slice 6)*.
7. **Total-return price entry** — closes slice 5's gap: `POST
   /api/v1/pricing/manual` accepts `adjustment_basis` `raw` (default) or
   `total_return`, refused for a currency base, and the pricing form offers it
   for securities and crypto with the benchmark-only consequence in words; the
   history marks such a price "Total return · Benchmark only". A correction
   keeps the replaced observation's basis and refuses another, because a
   relabel would move a price between the raw and total-return selections
   (before this slice a browser correction silently rewrote any basis to
   `raw`). *Delivered 2026-10-25*: API
   `TestManualTotalReturnPriceFeedsOnlyATotalReturnBenchmark` (100 → 110 reads
   `0.100000` on the total-return benchmark while the portfolio stays
   `0.000000` and the price-return benchmark stays unavailable; the corrected
   88 → 110 reads `0.250000`), browser case in `pricing-management.spec.ts`.

Targets and rebalancing are **deferred** to [#411](https://github.com/sergeyfarin/rekenraam/issues/411)
and [#412](https://github.com/sergeyfarin/rekenraam/issues/412); no trade
execution is implied. Security-level returns (`instrument_returns_v1`,
slices 8–9) are delivered, as recorded above.
