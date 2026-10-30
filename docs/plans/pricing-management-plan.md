# Price and FX management (#116)

Status: **shipped** through #200–#202 (2026-10-13). Retained as the bounded
operator contract; current consumer selection policy lives in
[investment reporting](investment-reporting-plan.md).

ADR 0008 governs immutable source observations; ADR 0010 governs existing FX
fetching. Delivery acceptance lives in #116 and #200–#202. Execution order
remains in [the roadmap](../roadmap.md).

## Read contract

The operator workspace composes current commodity/source choices and price-pair
coverage in one response. Pairs include configured instruments, current open
lot cost currencies, active account default currencies relative to the configured
FX base, nonzero posted currency balances on asset/liability accounts, and
existing series. A deliberate additional pair can be inspected and
entered even without a configured series. History pages sort by descending
observation ID (recording order), with an opaque keyset cursor bound to filters;
valuation and recorded dates remain separate, visible fields. Voided rows are
available as evidence. Stored price coefficients remain positive int64 at scales
0–18; browser DTOs and new manual requests use canonical decimal strings.
This is a deliberate pre-release wire change for `PriceObservationResponse`
(including currency-settings composition); legacy create requests remain
numeric, while new manual/correction requests are lossless strings. Existing
storage and exports are unchanged; no migration or database reset is needed.

Coverage uses an explicit valuation date, defaulting to owner-local today,
current recorded knowledge, and the configured pricing-policy age threshold.
Candidates rank by valuation date, manual/override first at equal dates, then
non-approximate, then recorded time and ID. Future and voided observations are
excluded. A stale candidate stays inspectable but is not called usable by this
diagnostic. Same-commodity pairs require no observation and are rejected as
manual entry. This diagnostic is not a new report engine: current holdings use
latest-price selection without this age gate; reporting FX uses its named
window (default seven days). #203 now supplies their shared compatible-quote
selection and explicit date/knowledge ceilings, with seven-day stale warnings
on retained holdings/settings values. The [R18 policy](investment-reporting-plan.md)
records the consumer differences and remaining reproducible-profile boundary.

### Posted currency discovery (#464)

The same workspace supplies attention's current missing/stale price and FX
counts. In addition to configured pairs, it discovers each currency with a
nonzero exact posted balance on an asset or liability account, paired with the
configured FX base (or book default). This includes currencies on accounts
without a default commodity. It folds current, undeleted, posted transaction
versions through the selected journal-entry date, with account classification
effective on that date. Omitted dates use owner-local today. Drafts, voids,
deleted transactions, superseded posting versions and future entries do not
create current exposure. Explicit future workspace dates remain diagnostics.

Closure does not erase a nonzero balance: closed accounts remain covered, as
in net worth. A zero balance at the account/currency level creates no new pair;
equal and opposite balances on separate accounts still require one shared
pair, since either account can be valued separately. Income, expense, equity
and commodity-trading residuals do not independently create current stock
exposure. Existing series, configured instruments/lots and active account
defaults remain discoverable even without a nonzero balance. All pair sources
are deduplicated. Exact scale-aware accumulation happens in the application;
the repository streams amounts inside the same read snapshot as candidates,
without materializing posting history, SQLite monetary sums or per-row queries.

This extends discovery, not observation selection or conversion policy:
`live_raw_v1`, recording/valuation ceilings and the configured diagnostic age
threshold still govern; a valid manual rate repairs the pair for both workspace
and attention. These counts do not certify every historical, filtered,
alternative-reporting-currency or trade-date/unsettled valuation. Reports,
including Home's future reporting-currency consumer (#437), must retain their
own dated conversion coverage and complete-only totals. Historical provider
refresh planning remains a separate demand owner under ADR 0010 below; a valid
manual rate can repair current coverage before automated history arrives.

### Posted-only historical demand (#465, #466)

Durable FX planning derives its currency set from active account defaults plus
every currency in the current posted ledger. One `FXDemand` repository read
returns each demanded currency, its code and earliest required date; coverage
traversal, target planning and triangulation candidates all consume that single
read, so a sweep does not scan the posted ledger twice. The posting outbox,
manual refresh, daily schedule and durable worker therefore share one
base-relative pair set. Posting a multi-currency receivable or payable without
a default commodity now attempts the required pairs instead of completing an
empty sweep.

An empty demand result is a loaded set, so failed triangulation attempts do
not query it again. Immediate latest refreshes, like durable sweeps, pin
reference-data selection to the owner-local day captured at run start, even
if midnight passes during the run.

An account default contributes only while its current version is active and
postable and its commodity version is active; it then supplies both a start
date and a target. A non-postable or archived default supplies neither, so a
traversal never sees a required date with no owning pair.

Historical demand is deliberately wider than current exposure. A balance that
later returns to zero, a closed account, an income/expense/equity posting, and
a currency archived after posting all retain their posted rate obligation.
Drafts, voided and deleted transactions, superseded versions, unsaved working
copies and import previews create none; only a durable `posted` current
transaction version does. A future-dated posted entry remains in the demand
set, and durable scans bound requests through owner-local today. Future-only
currencies (#471) create no immediate latest requests, including manual and
scheduled refreshes. The worker atomically completes the sweep and publishes
one canonical currency/date wakeup at owner-local midnight, converting the
IANA-zone boundary to UTC with its date's DST offset. This works with the daily
schedule disabled and survives database reopen or process restart. If a DST
transition skips midnight, the wakeup uses the first valid instant after the
gap, preventing an early wakeup from repeatedly rescheduling itself.

Waiting has zero provider attempts and no failure state. Repeated sweeps share
the same active wakeup; another currency exhausting the chunk budget cannot
discard it. Its nonempty start date prevents the posting outbox from absorbing
new earlier/current demand into a later wait. At wake, current demand replaces
the saved date hint: cancellation/void/delete or currency replacement removes
the obligation, while a later posting date publishes a new wait. Independent
current demand for the same currency remains immediately eligible. Due work
retains the existing weekend/publication policy and eight-attempt pair/date
retry identity; waiting does not consume that budget. Named regressions:
`TestFXFutureDemand*`, `TestFXFutureWakeDoesNotCoalesceAwayNewCurrentDemand`,
and `TestFXFuturePostingDoesNotDeferIndependentCurrentDemand`.

Provider selection, exact observation storage, the persisted pair/date cursor,
continuation and bounded retry are unchanged. An unsupported pair remains
explicit failed work with the pair/date in its error instead of a successful
empty sweep that implies coverage. A refresh run is not reported successful
when its per-pair item or source-health write fails: the first persistence
failure stops the sweep, marks the run failed and is returned to the caller.
Observations already stored remain, while the missing item or health row is
never fabricated (#469).

Every observation exposes source, quote type and adjustment basis, valuation,
observation, publication and recording times, manual/derived/approximate flags,
trade-version linkage, derivation/metadata, and supersession/void evidence.
Original derivation JSON is also returned as text, so inspection never rounds
its nested numeric leg coefficients through a browser JSON Number. A
manual observation is an owner assertion, not provider verification. Correcting
a provider/trade quote produces explicitly manual evidence.

## Commands and review

FX refresh source assignments are resolved at the explicitly requested
valuation date, or the owner's day at refresh start when no valuation date is
supplied (#343). The latter uses `SettingsService.OwnerTodayAt`, matching
assignment-entry defaults, independently of the pricing schedule's firing
time. Missing preferences retain the UTC default. Provider observations keep
their actual valuation dates; this lookup default does not manufacture a
historical request or redate returned observations.

The [2026-10-18 audit](../reviews/pricing-durable-work-audit-2026-10-18.md)
verified these management/selection workflows. Its publication-gap finding
[#315](https://github.com/sergeyfarin/rekenraam/issues/315) is corrected by the
[FX progress delivery](../reviews/fx-coverage-progress-fix-2026-10-18.md).
Latest-pair diagnostic coverage still does not certify a fully fetched
historical horizon; no absent rate is manufactured or treated as zero.

### Historical FX progress (#315)

Each durable sweep stores its next pair/date and an owner-local end date in
the work payload. Pair ordering is stable. Every attempted missing date consumes
one `max_backfill_days` budget slot, including a failed download or a valid
response carrying an earlier observation. An earlier quote keeps its actual
valuation date, and ingest evidence records requested/observed dates with a
skipped outcome. This is traversal progress, not proof of a holiday or complete
rate coverage. Future or invalid returned valuation dates are refused.

Failed dates become independent pair/date retry items. Completing the scan
chunk, queuing those retries and queuing the remaining scan happen in one
lease-fenced transaction; late failure leaves the parent recoverable. The
initial failed download counts toward the existing eight-attempt cap. Exhausted
items appear in the existing failed-work/retry surface with pair/date context;
ingest history retains failed/partial outcomes. A retry never becomes a skipped
publication merely because it failed. Other dates and pairs keep progressing.

A cursor applies only to its sweep. Fresh manual/scheduled refreshes and new
financial demand start fresh missing-date scans, so late publications and
earlier required dates remain discoverable. The posting outbox's range guard
coalesces only against an unattempted full pending scan, never a running scan,
suffix continuation or targeted retry; exact active-payload deduplication still
applies. Migration 0018 updates that trigger without
rewriting financial data or existing queued items. No new tables or public API
are required. Network I/O stays outside write transactions; observations and
their deduplication identity retain the existing immutable model.

### Manual commands

Manual entry requires an exact positive price and base quantity, a valuation
date, and a reason. It always uses the seeded manual source and manual quote
type, with an optional plain-text reference/observation timestamp. Its
adjustment basis is `raw` unless the owner chooses `total_return` for a
security or crypto price (R13 slice 7, #119): a total-return price values only
a returns benchmark declared total-return and never a ledger balance, so the
form explains that and the history labels it benchmark-only. No other basis is
recordable by hand, and a currency price is never total-return.
The coefficient range is checked before converting to the existing int64 model;
overflow refuses without writes. The legacy create route may not carry a
supersedes ID: correction has its own reviewed command.

An impact preview reads the exact bounded dependency closure used by voiding,
under one database transaction, and never writes audit, prices or work. Its
acknowledgement fingerprints the root and every active derived observation.
The command rechecks that closure and fingerprint inside its writer transaction;
a changed/retired root or newly derived observation requires a fresh review.
Legacy callers may void without an acknowledgement; the new UI always supplies
one. The existing depth-16 bound remains atomic: deeper closure refuses wholly.

Correction retires the root and active derived closure, then appends a manual
replacement for the root's same commodity pair, valuation date and adjustment
basis. Other dates, pairs or bases require explicit void plus separate manual
entry: relabelling a basis would move the price between selections. It preserves all
original values/source/trade identities, records immutable supersession linkage,
and uses one audit event for retirement and replacement. It does not regenerate
derived rates, reinterpret provider IDs or edit trade-implied ledger facts.
Voiding/correction changes market valuations only; journal postings, operational
basis and reconciliation checkpoints are untouched. Late failure rolls back
retirements, replacement, series and audit together.

## Browser behavior

A six-locale mobile/keyboard workspace provides loading, empty, error, success,
filter, paginated history, provenance detail and deliberate mutation review.
The review shows every affected observation and valuation-only consequences.
Cancel writes nothing; stale refusal preserves the form and clears its review.
Success invalidates pricing/currency/investment/report/forecast reads. Provider
configuration and refresh remain in existing currency settings; no new adapter,
network request on page reads, report snapshots or attachments are introduced.
