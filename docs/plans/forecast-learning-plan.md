# R10 extension — lightweight learned spending

Status: **in progress — M1 complete; M2 next**, updated 2026-09-07. Added at the owner's request
for basic ML/AI within modest hardware limits. Execute M1–M4 below **after the
eight core slices** in `docs/plans/projected-balances-plan.md`, before R10's
final closure/R8 planning. The core forecast remains independently usable and
must not depend on a successful model fit. This extension supersedes only the
core plan's exclusion of historical-average/learned variable-spending estimates
and seasonal expense profiles. The owner's cadence clarification is part of this
scope: daily/weekly fluctuations, monthly costs and annual peaks such as
July–August leave must not all become a flat weekly average.

## 1. What to build

Add an opt-in **“Include estimated spending”** view. Learn ordinary variable
expenses from this owner's posted history, for example groceries, transport,
dining and seasonal travel. Display the additional estimated outflows separately
from posted facts and recurring entries. Preserve the existing recorded-only and
with-recurring curves byte-for-byte when learning is switched off.

This is lightweight statistical learning: recent-average benchmarks and a small
exponential-smoothing or calendar-seasonal model checked against chronological
tests. It is not an LLM, a neural network, or an automatic financial
decision-maker. Its value is learning normal spending the owner has not put into
recurring rules.

Required properties:

- CPU-only, local Go code, no GPU, Python service, model download, API key,
  cloud requests or training on another owner's data.
- No transactions, recurring templates, occurrences, audit events, provider
  requests or investment effects created by fitting or viewing a model.
- Results carry source=`estimated_spending`, model/version, training window,
  sample counts and retrospective error. They never masquerade as saved bills.
- No learned income, transfer, loan-payment, investment-price or FX predictions
  in this first extension. No automatic discovery/creation of recurring rules.
- No probability bands or “95% confidence” badges in M1–M4. They need separate
  calibration; a historical error statistic is not a prediction interval.
- Lack of history, ambiguous accounting, overlap or resource exhaustion yields
  an explicit unavailable/fallback state while the core forecast still works.

### Cadence and calendar contract

There are two separate sources; a learner must not replace an explicit schedule.
The existing `backend/internal/recur/schedule.go` supports daily, weekly,
monthly and yearly schedules, including month-end clamping. Core R10 reuses that
enumerator for known fixed bills, subscriptions and annual renewals.

For expenses without an explicit schedule, let the owner choose **one pattern
per expense category**, applied independently to each funding-account/currency
group. Default to `weekly`; never infer annual holidays from one large purchase.
Selections are forecast URL options, not saved category/account changes.

| Pattern | Amount model | Timing assumption | Examples |
|---|---|---|---|
| `daily` | Weekly spending level | Equal share per remaining day | Small everyday purchases with variable daily amounts |
| `weekly` | Weekly spending level | Learned weekday shares | Groceries heavier at weekends; weekly transport |
| `monthly` | Calendar-month spending level | Learned day-of-month shares, with short-month clamping | Variable utility purchases not already represented by recurring bills |
| `annual_seasonal` | Separate expected amount for each calendar month | Month-of-year profile; equal daily shares within each month | Annual leave/travel concentrated around July–August |

Daily and weekly reuse the same small amount model; neither forecasts a precise
shopping day. Monthly is not “four weeks.” Annual seasonal is not an annual
total divided by 52. It can have several peak months, different yearly amounts
and zero months. Allow all four patterns across different categories in one
forecast, but **never add multiple pattern models for the same group**. A mixed
category cannot safely distinguish annual travel from everyday purchases in this
version; explain that limitation and recommend separate categories or known
recurring rules. No automatic category splitting or ledger edits.

Use **posted entry/payment dates**, not the activity's name: a July holiday paid
in March affects March (for card purchases, the card balance changes then;
repayment is still a separate transfer). Do not move historical booking deposits
to July or infer school holidays, geography, public holidays or travel dates. A
new one-off trip with no history needs an explicitly recorded future
transaction, or a suitable recurring template if genuinely repeating; unsaved
what-if editing is still out of scope. A yearly template represents its chosen
payment date, not a multi-month holiday spending envelope.

Fluctuations are modeled as changing levels and historical calendar shares, not
random daily noise. Display observed variation and forecast error with units;
never claim exact payment timing or convert observed ranges into probability
bands. No automatic inflation, trend, holiday calendar or economic adjustment.

## 2. Hardware envelope and exact arithmetic

The owner's actual deployment hardware is not specified. Use **one CPU thread,
no GPU, and a machine with roughly 1 GiB RAM** as the provisional benchmark
environment, not a measured minimum requirement or performance promise.

Bound the incremental learning work:

| Item | Limit / target |
|---|---|
| Historical window | Daily/weekly: 52 completed Monday–Sunday weeks; monthly/annual: 60 completed calendar months |
| Eligible model groups | At most 100 account/category/currency combinations |
| Additional posting rows (history + private future window) | 100,000, detected with limit+1 or streaming counter |
| Requested expense categories | At most 20 IDs |
| Learned horizon | Same 1–366 days as core; weekly long-range output is explicitly an unvalidated continuation |
| Algorithms per group | Daily/weekly/monthly: two baselines + two smoothing candidates; annual: one pooled candidate + seasonal and flat references |
| Parallel fitting | One operation at a time per process; no request fan-out |
| Additional peak live memory | Target ≤64 MiB over the core forecast, measured with heap profiling |
| Incremental latency | Target ≤2 seconds on the declared benchmark machine, including extra reads |

These are engineering acceptance targets. Record CPU model, Go version, dataset,
cold/warm timings and measured heap; do not claim them from asymptotic
complexity. The core plan's budgets still apply. Do not quietly fit the first
100 of 101 groups. Return the core result plus
`learned_spending.status="unavailable"` and `reason="resource_limit"`, with no
partial learned curve. A busy fitting slot returns `reason="busy"`; do not
accumulate an unbounded queue. Context cancellation stops work. A learning-only
computation deadline returns an explicit unavailable overlay; an actual
database/infrastructure failure uses the normal API error, not a fabricated core
response.

Use streaming/batched reads into compact period totals and calendar-share bins.
No persistent model store, worker or cache is required initially; fitting a few
fixed recurrences is cheap enough to benchmark first. Recompute on an explicit
enabled forecast request; never on every keystroke. Add caching only after
measured need, with bounded memory and a key covering all training
inputs/policy.

All posted quantities remain exact strings. Use `exact.ScaledInt` for bins and
`math/big.Rat` for means, smoothing and errors. With rational alpha and at most
60 months or 52 weeks there is no need to introduce floating-point money or an
ML dependency. Round a learned period amount at its output boundary, half away
from zero, to the funding account's currency standard scale (or a lower account
precision override). Check the existing 38-digit coefficient ceiling; a
projection overflow remains `LEDGER_OVERFLOW`, never a model “outlier” to
discard.

## 3. Training basis and eligibility

Read history under the **same read-only snapshot** as the core forecast. Do not
join a trained result from one snapshot to current recurring/posted data from
another. Fetch complete journal entries for classification; the existing
per-category report result alone does not identify which account funded a split.

The target is gross ordinary purchase spending, grouped by `(funding_account_id,
expense_category_account_id, commodity_id)`.

An eligible entry has exactly one non-system posting-enabled asset/liability
funding account, one currency, ordinary/transfer transaction kind, positive
expense-category postings and negative funding postings that balance exactly.
Aggregate repeated postings to the same account first. Split expense categories
are allowed because there is only one funding account: each category's debit is
its exact funded spend. Negative expense/refund legs, income/equity legs,
multiple funding accounts, transfers, FX exchanges, investment entries and
non-currency legs make the **entire entry ineligible**, not proportionally
allocated. Count exclusions and explain that this is purchase-spending coverage,
not a reconstruction of all cashflow. Refunds still affect the core ledger
projection; the learned model does not predict them or treat them as new income.
Credit-card purchases may train that liability's expense model; card repayments
are transfers and must not become a second purchase or a learned cash payment.

Use only current posted, non-voided, non-deleted versions with entry dates in
the history interval. Never train on drafts, forecast rows or future-dated
postings. Exclude transactions linked to any recurring occurrence from training.
A stored opening balance or old transaction timestamp is not evidence of a
complete expense history. Do not silently remove unusually large valid expenses
as “outliers”; the owner can exclude an unsuitable category from this view.

Conservative overlap rule: disable a group for the **whole learned horizon** if
a current unarchived template (enabled or paused) has that funding account,
category and currency, or any saved recurring draft/blocked/computed core event
would affect that group. Use full posting sets, not fuzzy payee matching. For
ambiguous recurring funding, disable every implicated funding/category/currency
group; do not infer an allocation. This may omit useful estimates, but it avoids
learning rent from history and adding another rent beside a known recurrence.
Archived templates alone do not disable a group; their retained drafts do.
Report `recurring_overlap` as an exclusion with the source reference.

The UI asks the owner to confirm a complete recording start date for the
selected accounts: `history_complete_from=YYYY-MM-DD`, on/before A. An opening
balance or oldest transaction does not establish coverage. Exclude the first
partial period and the current incomplete period from fitting. Zero-spend
complete periods inside confirmed coverage count as zero; unknown earlier
coverage does not.

| Pattern | Minimum confirmed complete history | Activity requirement | Maximum fitted window |
|---|---|---|---|
| Daily / weekly | 16 weeks | At least 8 positive-spend weeks | Latest 52 complete weeks |
| Monthly | 16 calendar months | At least 8 positive-spend months | Latest 60 complete months |
| Annual seasonal | 36 consecutive calendar months | Positive spend in at least two distinct 12-month blocks among the three most recent blocks | Latest 60 complete months |

For annual activity counts, anchor three consecutive 12-month blocks backward
from the latest complete month; these need not start in January.

Annual eligibility deliberately does not require frequent spending: one holiday
per year is a valid candidate. Thirty-six months allow two prior observations of
each month and a full held-out year; that is still limited evidence. With only
one or two years, return `insufficient_seasonal_history`, not a flat weekly
substitute or a silently accepted annual pattern. Do not include partial months
to reach the threshold. Later extra history is bounded by the stated lookback;
this is a model policy, not truncation of core ledger totals.

Read the union of required windows once for all groups, with complete sibling
postings and book scope. Weekly-only requests must not read five years because
another pattern could theoretically need it. Enforce the posting-row limit
across the entire additional read, not once per pattern. Count a sibling posting
once in that budget even if it serves multiple groups. Display actual window,
complete-period count, positive-period count and exclusions per group.

## 4. Models, evaluation and fallback

### Daily, weekly and monthly levels

Fit one level per week for daily/weekly and per calendar month for monthly. No
trend extrapolation. Baselines are `mean_8` (preceding eight complete periods,
including zeros) and `last_period`. Learned candidates are `ses_025` (alpha=1/4)
and `ses_050` (alpha=1/2):

```text
level[0] = first training period's spending
level[t] = alpha * spending[t] + (1-alpha) * level[t-1]
forecast future period spending = final level
```

Reserve the latest eight periods: four chronological tuning periods, then four
held-out test periods. At least 16 periods leave eight for initial training. At
each tuning origin fit only its prefix, predict the next period and compare
exact mean absolute error (MAE). Choose the better SES candidate (tie:
alpha=1/4) and the better baseline (tie: mean_8). Freeze those algorithm choices
before testing; refitting their states on an expanding prefix is permitted.

Evaluate four one-period predictions on the held-out periods, and one frozen
four-period-total prediction at the beginning of that holdout. SES wins only
with MAE at least 10% lower than the baseline and no worse four-period-total
absolute error. Otherwise use the baseline and label it as such. A zero-error
baseline always wins. Never randomly split dates, tune on final errors or fit a
target's features using its own observations.

### Annual calendar seasonality

Work with exact calendar-month totals, including known zero months. Fix the
candidate and references in advance; there is no fitted alpha or search over
month groupings in this version:

```text
same_month_2y(target) = mean of the latest two observed complete months
                      with the target's calendar month number
seasonal_last_year(target) = latest observed complete month with that number
flat_mean_12(target) = mean of the latest 12 observed complete months
```

Only use observations before the forecast origin. For a future target more than
12 months beyond the latest complete month (possible at the 366-day boundary),
reuse the same observed month-number values; never treat a previous forecast as
training history. July and August remain separate bins, so a trip shifting
between those months contributes to a two-month summer profile over the years.
This cannot infer the exact date or guarantee the trip will repeat.

Reserve the latest 12 complete months for evaluation, leaving at least 24 months
of initial history. Evaluate all three fixed methods at 12 expanding one-month
origins, and also freeze a complete 12-month path at the first holdout origin.
Record monthly MAE, frozen-path monthly MAE, frozen annual-total absolute error
and maximum frozen monthly absolute error. Include a fixture checking the
July–August combined total, without hard-coding summer as the only important
season for every category. Evaluate every calendar month; a short winter holdout
cannot validate summer travel. No separate hyperparameter tuning.

Choose `same_month_2y` only if it improves one-month MAE by at least 10% over
`seasonal_last_year`, has no worse frozen-path MAE or annual-total error, and
has no worse maximum frozen monthly absolute error. Otherwise use
`seasonal_last_year` as an explicitly labeled seasonal baseline. Zero-error
baseline wins. Report the flat reference errors too. If the selected method does
not beat the flat reference's one-month MAE, attach
`seasonality_not_better_than_flat`; the owner selected this pattern, so retain
it as a visibly weak seasonal assumption rather than changing its meaning to
weekly spending. These are conservative policy gates that protect peak months
regardless of the category or hemisphere. The general month-by-month test
remains required for other seasonal peaks.

After evaluation, refit/recompute the selected permitted method using all
available complete periods in the bounded window. Do not select one method per
future month by inspecting holdout outcomes.

### Honest variation and quality reporting

The 10% margin is engineering policy, not a significance test. Report period
unit, counts, test ranges, exact errors, selected method and fallback reason.
Four weeks/months or one held-out year are limited samples. For weekly/monthly
patterns show min/max observed complete-period spending in their fitted window;
for annual show min/max observed amounts for each month number and its count.
Label these as historical variation, **not** a plausible future range or a
confidence band. An annual profile can have very few observations per month.
Never add random jitter to make a chart look realistic.

Daily/weekly evaluation does not establish 90/366-day accuracy, nor monthly
one-step tests an accurate annual total. The UI must show the tested horizon and
label longer projections as continuing the chosen assumptions. Annual tests
include a frozen year, but one held-out year does not guarantee next year's
accuracy. If a requested pattern lacks history, exclude that group explicitly;
do not quietly switch cadence. A baseline must still pass all data/overlap
rules.

Retrospective evaluation uses today's current ledger versions. It is not a
knowledge-time replay of what the owner had entered at each historical date;
late imports/corrections can change the evaluation. In each fold, feature
construction, calendar shares and model fitting may use only its date prefix.
Use the current conservative recurring exclusions consistently throughout and
disclose that they are current configuration, not historically reconstructed
rules. Do not describe this as a fully historical production simulation.

## 5. Add only the spending not already represented

Never add an unconditional period estimate on top of known expenses. For each
eligible group and owner-local calendar period p:

```text
B[p] = nonnegative learned/fallback spending estimate for p
K[p] = positive eligible purchase spending already recorded through A in p
     + positive eligible future purchase spending in the core projection in p
R[p] = max(0, B[p] - K[p])
```

K uses posted, included saved drafts and included computed core events exactly
once under the core plan's occurrence precedence. Recurring overlap normally
disables the group before this step; retain the same source deduplication. Use
core **projected** dates for carried-forward future items. Recorded through-A
spending uses original entry dates. Excluded/invalid core assumptions do not
count as reliable K: if they implicate this group/period, suppress that period's
learned addition and flag `ambiguous_known_spending`, rather than filling the
gap with an apparently complete estimate. Apply the same conservative rule to
ambiguous future multi-funding or mixed-sign entries touching the group.

B is the period estimate rounded to output scale under section 2. Subtract K
using its full exact posted precision, then round the nonnegative residual once
to output scale, half away from zero. Return any difference between the exact
residual and its allocated amount as `residual_rounding_delta`; the allocated
integer units must sum to that rounded residual. Do not round individual known
postings before subtraction. This handles older postings with higher precision
than a current account output override without changing ledger amounts.

The subtraction period is a Monday–Sunday week for daily/weekly, or a calendar
month for monthly/annual seasonal. A group uses exactly one period system. An
annual expense paid in July reduces July's estimate, not January's or every
month's equally. Do not subtract one purchase from both weekly and annual
models.

Read known future spending through the end of the last forecast period: up to
six extra days for weekly groups or 30 for monthly/annual groups. Read the union
once, under the same snapshot, with core occurrence precedence and validation.
This private window does not extend public E, core balances/minima or create
occurrences. Extra inputs belong in the enabled basis token and resource budget.
If calendar bounds cannot support it, return an unavailable overlay; never use
five-digit years or silently shorten coverage.

Allocate residual R across dates after A in its whole period, before clipping at
E. Daily uses equal weights; weekly learns seven weekday weights from the last
eight complete training weeks. Monthly learns nominal day-of-month weights 1–31
from its last eight complete training months; for a short future month combine
weights of days beyond month-end onto its last day. Annual seasonal uses equal
day weights within each month, without pretending to know a trip date. Weights
use eligible positive purchases only. Normalize the weights of remaining dates;
if their sum is zero, fall back to equal weights and report
`uniform_timing_fallback`. Compute shares from each fold's prefix for evaluation
features; never from held-out dates.

Allocate integer output units by floor then largest remainder; earliest date
breaks ties. The whole remaining period sums exactly to R. Return only dates ≤E,
without moving outside-E allocations into the visible horizon. Learned monthly
day 31 clamping is a timing assumption, not a new recurring rule. Test February,
leap years, month/year boundaries, local midnight/DST and incomplete first/final
periods. Existing core recurring calendar rules remain unchanged.

Synthetic daily events contain group, date and negative funding-account amount;
they live only in the read model. They have no transaction or occurrence ID. The
third curve is:

```text
with_estimated_spending[d] = core.projected_balance[d]
                          + cumulative estimated_spending_delta through d
```

Core posted/draft/template deltas and balances remain untouched. Additional
estimated expense is never positive income. For optional combined currency
output, include model currencies in FX coverage and convert estimated deltas
using the existing constant-as-of/component-rounding policy. If those require
missing FX, omit the **learned combined curve**, retaining complete core
conversion if it is still covered, and explain the separate coverage result.
Never keep a core-only combined total labeled “including estimated spending.”

Example: a future full week's learned groceries level is EUR 140, and EUR 50 is
already posted for that week. Add only EUR 90 of estimated groceries. A known
EUR 170 purchase yields zero extra, never a EUR 30 refund. If weights are equal
over seven future days, EUR 90 allocates five EUR 12.86 days then two EUR 12.85
days. A recurring rent group gets no learned overlay at all.

Seasonal fixture (all amounts EUR, eligible non-recurring travel): at the start
of 2026, 2024 July/August were 1800/600 and 2025 July/August were 2200/1000. For
a unit test where evaluation has permitted `same_month_2y`, 2026 July/August
amounts are 2000/800. If EUR 500 is already known for July, add only 1500 in
July and 800 in August. Do not turn 2800 into 53.85 every week. Test selection
itself with a separate full 36+ month fixture; these four amounts alone cannot
establish model eligibility or quality. If all other same-month observations are
zero, those future months stay zero. A March booking appears in March's profile.

Monthly fixture: EUR 120 period estimate minus EUR 30 known = EUR 90 residual. A
learned nominal-day-31 weight maps to February 28/29, then March 31; it must not
drift to March 28 or become a four-week cadence. A matching recurring bill
disables that group before this calculation, preserving the no-double-count
rule.

## 6. API, UI and failure contract

Extend the core endpoint only in M3, together with OpenAPI/types/tests. Before
M3 it must continue rejecting these unknown parameters as the core plan states.
New optional recipe parameters:

- `spending_model=off|adaptive_v1`, default off;
- `history_complete_from`, required only with adaptive_v1;
- repeated `expense_category_id`, optional selected expense categories, max 20;
  absence means all eligible categories within existing account scope;
- repeated `expense_pattern=<category-id>:<pattern>`, at most 20, pattern one of
  `daily|weekly|monthly|annual_seasonal`; absent override defaults to weekly.
  Split at the final colon, validate the category ID normally, reject duplicate
  overrides, unsupported patterns or categories outside explicit category scope.
  This version applies a category override across its selected funding accounts;
  it does not support different patterns for the same category per account.

Reject invalid/non-expense IDs and orphan model-specific parameters. Resolve
identities from the same snapshot. Use the same 1–366-day horizon as core;
return an explicit unavailable overlay on budget exhaustion, never a silently
shorter model curve. Turning the feature off requires no historical data reads.

Add a nullable `learned_spending` response object; do not add amounts into the
existing core point fields. It contains:

- status (`ready|partial|unavailable`), policy version `adaptive_spending_v1`,
  requested/eligible/excluded group counts and coverage reasons;
- eligible category options with stable built-in localization keys;
- history confirmation date, actual per-group training start/end, period unit and counts;
- per-group account/category/currency IDs, selected model, tuning/test ranges,
  errors and fallback reason, chosen pattern, nonzero periods, historical variation,
  timing assumptions and classification exclusions;
- separate exact estimated daily deltas and with-estimated-spending account/
  currency curves, plus separately covered constant-FX conversion if requested.

`ready` means all in-scope groups can be estimated; it is not a claim of future
accuracy. `partial` means some groups or implicated periods are explicitly
excluded for data/overlap, with their counts and reasons visible. If no group
has any estimable period, use `unavailable` and return null learned curves, not
a reassuring zero-spending forecast. Zero estimated spend from an eligible zero
seasonal month is a valid result, distinct from unavailable coverage. Resource
limits never produce a prefix of groups. Reason codes include
`insufficient_history`, `insufficient_seasonal_history`, `sparse_history`,
`recurring_overlap`, `ambiguous_known_spending`, `resource_limit`, `busy`,
`calendar_limit` and `computation_timeout`. Non-exclusion warnings include
`uniform_timing_fallback` and `seasonality_not_better_than_flat`. Fallback
reasons distinguish `no_validated_improvement` and `perfect_baseline`.
Overflow/infrastructure errors keep their normal error contract. Never log raw
training rows or amounts.

An enabled basis token includes model policy/options, history, pattern
selections and extra period known-spend input, model selection and result.
Extend event-detail source with `estimated_spending`, null saved-record IDs,
category/group/model fields and a stable key
`estimate:<account>:<category>:<currency>:<date>`. Preserve the core
date/account/currency ordering; rank estimates after core sources when earlier
sort keys tie. Enforce the same stale-basis rejection. Event totals
reconcile separately to learned deltas; do not imply they sum to a core delta.

UI shows three distinguishable choices: Recorded only / With recurring / With
estimated spending. Same currency axes and exact tables remain. The last view is
opt-in and clearly labeled as an estimate. Show the confirmed history start,
category pattern controls (daily/weekly/monthly/annual seasonal), categories
included/excluded, fallback/model name, calendar profile, variation, test
horizon/error and limited-history caveat. The annual view shows month names and
peaks; do not hide July–August inside a smoothed yearly average. Explain why 36
months are required when a selected seasonal group is unavailable. Let the owner
remove categories and turn estimates off without changing any ledger/template.
Use the normal six-locale glossary, translation, accessible chart/table and
mobile requirements. No AI chat panel or generated narrative is necessary;
deterministic explanations are sufficient.

## 7. Execution slices and acceptance

Apply the same source documents and ledger/backend/API/frontend/validation
skills as the core plan. Each step has a scoped commit and an evidence record.
No application code or new dependency is part of this planning change.

### M1 — Training data and transparent baseline

Add `backend/internal/db/forecast_learning.go` for snapshot-aware complete-entry
history reads, and `backend/internal/app/forecast_learning.go` plus tests for
classification, pattern eligibility, weekly/monthly bins, mean/last-period and
seasonal baselines, overlap, known-period-spend subtraction and exact
daily/weekday/month-day allocation. Do not add a public option yet. Reconcile
full journal entries before assigning a funding account. Benchmark the read and
allocation path before adding model selection.

Gate: current core tests unchanged, coherent snapshot/no-write proof and all
classification/allocation tests below. Commit the prototype as internal only.

Completed 2026-09-07. The internal prototype adds a bounded, limit+1
`ForecastRepository.LoadLearningSnapshot` that reads current posted history as
complete journal entries with real recurring-occurrence identity, account and
commodity versions, and recurring overlap inputs inside one deferred read
transaction. No API parameter, public response field, UI, cache, persistent
model, or write path was added. Pure application primitives now cover exact
whole-entry purchase classification, conservative template/draft overlap,
confirmed complete weekly/monthly windows, all four cadence eligibility rules,
`mean_8`/`last_period` and fixed seasonal reference baselines, exact known-spend
subtraction, weekday/month-day timing bins, short-month clamping, and floor plus
largest-remainder allocation with deterministic date ties. The constructor
bounds an old confirmation date before allocating period storage.

Focused named tests prove current-version/posted-only history, void/delete/
draft/future exclusion, full sibling reads, real recurring linkage, no-write
behavior, resource-limit refusal without prefixes, classification exclusions,
cold/sparse boundaries, overlap, exact residuals, and deterministic allocation.
On the available AMD EPYC 3251 / Go 1.27.0 host, the synthetic prototype
benchmarks measured approximately 0.35 ms and 97,784 B/op for a 366-date exact
allocation and 1.35 ms and 10,413 B/op for a one-entry snapshot read. These are
microbenchmarks, not the declared 100,000-row workload or peak-live-memory
acceptance; M2 still owns representative workload, heap, single-slot/deadline,
chronological selection, and quality measurements.

### M2 — Chronological model selection and hardware evaluation

Add `backend/internal/app/forecast_learning_model.go`/tests and benchmarks.
Implement rational SES with period-aware tuning/holdout, the two-year same-month
candidate, the 12-month seasonal holdout/frozen path, all quality gates and
honest baseline fallback. Implement observed-variation metadata without bands.
Verify chronology with a fixture whose late regime shift would look perfect only
with future leakage. Measure the declared workload on available hardware; record
the machine and state which targets remain unverified if it differs from the
reference envelope.

Gate: model never wins merely on in-sample fit; exact/error boundary tests pass;
resource policies proven at maximum mixed-pattern history and 366-day output. A
dataset where baseline wins is a successful test, not a reason to weaken the
gate. Do not claim real-user quality from synthetic fixtures; real-owner
evaluation needs explicitly supplied/authorized data.

### M3 — Opt-in API and separate learned-spending UI

Extend forecast OpenAPI/client/DTOs and the existing screen, add the options,
coverage/status/error metadata, synthetic event details and third curve. Wire
the existing snapshot into extra bounded reads; do not refetch history per
category/card or train in the browser. Add forecast-learning terminology in six
locales. Prototype internal type names become explicit versioned schemas in this
slice, without changing the core's existing response meanings.

Gate: browser opt-in/off, mixed eligibility, missing history, fallback, partial
periods, annual cold start, mixed cadences, stale token, exact totals, missing
learned FX, mobile/keyboard and resource unavailable states. Switching off
returns the same core data as before M1.

### M4 — Final learning acceptance and R10 closure

Write a dated review covering classification/overlap,
daily/weekly/monthly/annual behavior, evaluation limits, benchmark results and
user-facing disclosures. Core acceptance at slice 8 remains its own milestone;
R10 including this extension closes here. Update roadmap/feature ledger/working
queue and move next work to R8 planning. If a hardware/quality gate cannot be
met, record a specific blocker; do not silently ship the learned curve or
substitute fabricated model quality claims.

Required new tests (table-driven subcases encouraged):

| Test | Must prove |
|---|---|
| `TestForecastLearningClassifiesCompleteEntries` | Split categories with one funder; reject two funders, transfers, refunds/mixed signs, income and investments; card repayment not double-counted. |
| `TestForecastLearningUsesPostedHistoryOnly` | Current versions, void/delete/draft/future exclusions, confirmed coverage, complete weeks/months, real recurring linkage. |
| `TestForecastLearningDoesNotOverlapRecurringGroups` | Paused templates, archived retained drafts, blocked/ambiguous funding; no fuzzy payee shortcut. |
| `TestForecastLearningColdStartAndSparseHistory` | 15/16-week and month boundaries; 35/36-month seasonal boundary; annual activity in two years, partial periods, zero versus unknown coverage. |
| `TestForecastLearningSubtractsKnownPeriodSpend` | EUR 140/50→90; 140/170→0; present-period prefix, monthly 120/30→90, seasonal 2000/500→1500, beyond-E same-period facts, future edited dates. |
| `TestForecastLearningAllocationIsExact` | Largest remainder, deterministic ties, partial first/final periods, zero remaining weights, no invented positive income. |
| `TestForecastLearningNoFutureLeakage` | Per-origin fitting and calendar features see only earlier dates; alpha/baseline choices fixed before holdout. |
| `TestForecastLearningFallsBackUnlessValidatedBetter` | 10% threshold, zero-error baseline, ties, period MAE and four-period-total gate; annual monthly/frozen-path/maximum-month/annual-total gates. |
| `TestForecastLearningPreservesExactMoney` | >2^53 coefficients, mixed scales, rational fit, output/residual rounding, exact allocation and 39-digit overflow. |
| `TestForecastLearningSharesCoreSnapshot` | Concurrent import/template/post changes cannot mix history and known future spending. |
| `TestForecastLearningLimitsNeverReturnGroupPrefixes` | Limit+1, busy, deadline and cancellation; unchanged core if only model budget unavailable. |
| `TestForecastLearningLeavesCoreAndLedgerUntouched` | Option off identical; option on core components unchanged; domain/audit/work/report/export/checkpoint/lot state unchanged. |
| `TestForecastLearningEnabledBasisAndEvents` | History/model/category changes stale cursors; estimated events sum to learned deltas with null saved IDs. |
| `TestForecastLearningPatternsAreExclusive` | All four patterns coexist across categories; one model per group, duplicate/invalid/orphan override errors, no weekly+annual double count. |
| `TestForecastLearningSeasonalPeaks` | Full 36+ month fixture, separate July/August peaks, shifting summer spend, other seasonal peaks, zero off-season months, March booking cash dates. |
| `TestForecastLearningSeasonalValidation` | Full held-out year; no future-date leakage within expanding folds; fixed references; zero-error/tie fallback; weak-seasonality warning; 366-day repeated month uses observations only. |
| `TestForecastLearningCalendarAllocation` | Monthly day 31 clamps and reanchors, leap February, DST/local midnight, partial month, 30-day private extension and unchanged core E. |
| `TestForecastLearningVariationIsNotConfidence` | Exact historical ranges/counts, no random jitter or probability labels, period-specific tested horizon and sparse annual caveat. |
| `TestForecastLearningSeparateFXCoverage` | A model-only missing currency hides learned combined curve but never falsely relabels core conversion. |

Run the core plan's validation commands, adding the new
`e2e/playwright/forecast-learning.spec.ts` for browser coverage. From `backend/`,
run `go test ./internal/app -run '^$' -bench ForecastLearning -benchmem` for benchmarks. Record incremental heap profiling
separately; allocation totals are not peak live memory. Do not run competing
frontend generation/build commands concurrently.

| Extension slice | Status | Commit/evidence |
|---|---|---|
| M1 Training basis and baseline | [x] Complete | 2026-09-07; internal repository/application prototype, named tests and `BenchmarkForecastLearningRead`/`BenchmarkForecastLearningAllocation` evidence above |
| M2 Model selection and hardware measurements | [ ] Not started | — |
| M3 API/UI integration | [ ] Not started | — |
| M4 Final acceptance | [ ] Not started | — |

## 8. Research basis and choices still outside this extension

Simple exponential smoothing gives recent observations more weight and models a
level rather than a trend/seasonal pattern. It is therefore used only for
daily/weekly/monthly levels; annual peaks use the separate calendar model. The
alpha grid, hardware limits and acceptance thresholds above are our design
choices, not claims from the source. [Hyndman and Athanasopoulos, simple
exponential smoothing](https://otexts.com/fpp3/ses.html).

Chronological evaluation must train only on earlier observations; testing beyond
one forecast horizon is distinct from one-step fitting. Our tuning/holdout
schedule, four-period and frozen-year checks apply that principle with small bounded
work. They do not establish guaranteed future accuracy. [Hyndman and
Athanasopoulos, time-series
cross-validation](https://otexts.com/fpp3/tscv.html).

Seasonal naive forecasts reuse the last observation from the corresponding
season. Our two-year pooling candidate, 36-month minimum, pattern controls and
quality gates are bounded product choices, not a guarantee of seasonal accuracy.
[Hyndman and Athanasopoulos, simple forecasting
methods](https://otexts.com/fpp3/simple-methods.html).

Later candidates, not M1–M4 commitments: suggesting possible missing recurring
bills for owner review; calibrated uncertainty bands; automatic cadence
discovery or geographic holiday calendars; a small regularized regression if
benchmarks demonstrate value. An LLM could explain already-computed results in a
separate opt-in feature, but must never supply authoritative amounts or receive
private ledger data without separate authorization. None of these is needed to
deliver this CPU-only learned-spending extension.

Planning validation (2026-08-31): checked existing recurrence frequencies and
transaction-kind names against code; reconciled this extension with the core
plan, requirements and tracking documents. The 19 named test contracts and
worked weekly/monthly/seasonal amounts were checked for consistency. Runtime
behavior, predictive quality and hardware budgets remain unimplemented and
unmeasured; these checks are not implementation acceptance.
