# Investment operation and subledger refactor plan

Status: proposed implementation plan, 2026-09-26. No behavior described below
is shipped merely because it appears here. ADR 0012 and ADR 0013 govern; this
plan makes their next slices concrete. R16 owns the near-term work, T-75b owns
native correction, and T-108 owns short sales and covers.

## Outcome and boundaries

Make the canonical journal, investment operation, and immutable lot effects
three linked views of the **same** economic event. Preserve the balanced
multi-commodity journal, exact decimal quantities, `book_id`, one audit event
per command, reconciliation guards, and the existing operational cost-basis
methods. Do not infer an operation from a negative position, a transaction
description, or a broker code. A report never creates accounting postings.

Target stock/ETF/fund and ordinary broker cash activity first. Model bond and
derivative contracts separately before allowing their lifecycle events to
post; `investment_instrument_versions.instrument_type` alone does not provide
coupon, maturity, exercise, assignment, or expiry semantics. Keep unsupported
broker activity in review, with its raw evidence and reason, rather than
force-fitting it into a dividend or ordinary sale.

This design follows the distinctions present in
[IBKR activity codes](https://www.interactivebrokers.com/campus/ibkr-reporting/reporting-integration/),
[OFX investment statements](https://financialdataexchange.org/common/Uploaded%20files/OFX%20files/OFX%20Banking%20Specification%20v2.3.pdf),
and [KMyMoney investment activities](https://docs.kde.org/trunk_kf6/en/kmymoney/kmymoney/details.investments.ledger.html).
Those are input taxonomies, not the app's database enum or accounting policy.

## Why the current shape will not stretch safely

- `investment_operations` has a name, date, and mandatory unique transaction
  link, but no exact trade consideration, charges, source identity, or links
  directly to its lot effects. A basis-only change need not make a journal
  posting; a complex event can touch several securities and cash accounts.
- `investment_lot_events.event_kind` only admits acquisition, disposal, split
  adjustment, reinvestment, and manual adjustment. The optional transaction ID
  and free-form metadata do not distinguish transfer, basis-only change,
  short-cover, or cross-instrument conversion.
- `InvestmentTradeInput` has one cash amount. The buy path treats it as lot
  cost; realized gains find sale proceeds by summing cash postings. Once a
  trade has separate commission, transaction tax, withholding, rebates, or
  multiple cash legs, that inference can produce the wrong result.
- Buy, sell, dividend, and reinvestment use separate repository write paths.
  Adding an event type to each path risks drift in audit, reconciliation,
  import identity, and preview behavior.
- Lots contain both original evidence and mutable remaining-balance fields.
  The latter need a deterministic rebuild before an older operation can be
  corrected safely. T-95 currently rejects backdating behind disposals.
- Disposal decisions are unique by transaction, import identities require a
  committed transaction, and the position basis-state key has no side. Those
  keys cannot describe multi-disposal actions, basis-only imports, or separate
  long and short pools without redesign.

## Proposed data contract

Treat an **investment operation** as the durable parent of one user or
imported economic action. It has `book_id`, stable domain `operation_kind`,
effective position date, created/recorded time, audit event,
optional correction/reversal relationship, and source evidence reference.
Related trade, settlement, entitlement, record, ex, and payable dates live
in typed `investment_operation_dates` rows, with a sequence where a kind can
occur more than once; one nullable header settlement date would not describe
a compound action with several cash events.
Operation codes remain application-validated strings, not a growing SQLite
`CHECK` enum. The broker's raw activity code and row ID remain separate from
the normalized operation kind. A correction is a new operation referencing the
original, never an update to the original facts.

Change the current mandatory one-to-one `investment_operations.transaction_id`
into an **operation-to-journal link** (`investment_operation_transactions`):
`(book_id, operation_id, transaction_id, transaction_version_id, role)`,
unique on each transaction and transaction version. The version ID pins the exact posted entries and
posting versions checked when the operation committed; validate that it
belongs to the linked transaction and book. A later investment-native
correction writes new links and effects, rather than quietly rebinding this
historical link to a newer transaction version.
The normal trade links one posted transaction; a basis-only event may link
none; a compound action may link several if distinct settlement dates or
broker records require them. Every linked transaction must be posted and in
the same book. Absence of a journal link is permitted only for command kinds
whose effects cannot change cash or security ledger balances; enforce this in
the service and self-check. A split that changes security quantity **does**
need balanced security postings. Do not create a zero-value transaction to
represent a basis-only change.

Add immutable **operation components** for exact broker and economic facts:
typed role (`gross_consideration`, `commission`, `other_fee`,
`transaction_tax`, `withholding`, `net_settlement`, `income`, `cash_in_lieu`,
etc.), exact signed coefficient and scale, commodity ID, optional account and
linked `posting_version_id`, source line reference, and sequence. Record
whether source economics are complete or net-only. A signed cash convention
is from the owner's perspective: receipts positive, payments negative.
Validate each trade's gross + charges = net settlement *per currency*, using
exact arithmetic; do not silently convert fees in another currency. Allocate
sale proceeds across consumed lots with one explicit exact rounding/remainder
rule, and snapshot that allocation beside the disposal decision. When
the source reports only net, record that as net with `gross_unknown`, not a
fabricated zero commission. Separate the **source cash facts** from the
operational basis treatment: the acquisition or disposal lot effect records
the exact basis/opening-proceeds amount and the policy/election that produced
it. Neither the broker's displayed tax basis nor a later report silently
overwrites that election. The journal posts the actual cash, expense/income,
and security legs and balances per commodity. For a linked component,
self-check its commodity, amount, scale-aware value, account, and transaction
version against the posting using an explicit cash-flow-to-debit sign rule
for that account and component role.
Gross consideration may be source evidence with no individual posting; a
linked net settlement or fee must not disagree with its posting.

Link each immutable **lot effect** to `operation_id`, with side (`long` or
`short`), effect kind, quantity delta, basis/opening-proceeds delta, currency,
and source/destination lot references as appropriate. Retain the present
disposal-decision and allocation provenance, extending it to short covers.
Record operation-specific attributes in typed companion tables when they
are necessary to validate an event: rational split ratio; transfer source
and destination; corporate-action old/new instruments and basis allocation;
cash-in-lieu disposed fraction; broker trade and settlement IDs. Avoid an
opaque `metadata_json` contract for facts needed by gains, replay, or export.
Multiple lots may be affected by one operation, including lots in different
instruments. Key disposal decisions by `(book_id, operation_id,
decision_seq)`, one decision per disposed position/side/cost currency; keep
their allocations under the decision. A transaction/version link on a
decision is optional provenance, not its identity. Original decisions and
allocations remain immutable even if a later replay supersedes their
effective allocations.

Separate immutable `investment_lots` identity and opening facts from an
`investment_lot_state` projection keyed by lot ID. Move `status`,
`remaining_quantity_*`, `remaining_cost_basis_*`, and projection update
fields into that projection; original quantity and acquisition/opening
consideration stay in the lot/effect facts. Rebuild `investment_lot_state`
and `investment_position_basis_state` atomically from the effective event
history. Add `position_side` to the basis-state key and include it in every
selection/rebuild. The split costs repository and export work; it is worth
doing in the unused baseline before replay is added.
Keep projected remaining basis nullable with an explicit knowledge status;
an unknown opening never becomes a fabricated zero in either lot state or
an average-cost pool.

Store all investment monetary/basis coefficients as canonical signed decimal
TEXT with explicit scales, including lot openings, effects, decisions,
allocations, and components. Keep non-negative constraints on quantities and
remaining basis where their meanings require them. The current service may
retain its documented int64 admission limit until exact backend arithmetic
is widened; the schema must not require another rewrite then. New trade input
and API fields also use canonical coefficient strings, matching the existing
investment JSON boundary.

Keep operation provenance explicit: source type and stable external ID with
an idempotency key scoped to book/source/account, raw payload reference or
snapshot, import batch/row linkage where present, and the mapping version.
Generalize the existing `import_commit_identities` row identity, rather than
creating an independent investment dedupe table. It owns one unique source
row fingerprint scoped to the existing `(book_id, dedupe_fingerprint)` key;
child links are unique on `(identity_id, effect_seq)`.
Ordered child effects link that row to one or more
`operation_id`/`transaction_id` pairs. A bank row can still commit one
ordinary transaction; a basis-only broker row can commit one operation and
no transaction; one broker fill crossing zero can commit two operations.
Replace `import_staged_rows.committed_transaction_id` as the sole committed
result with the same child links. Dedupe, source correction, retry, and all
child writes commit in one SQLite transaction.
Do not loosen the unique key by adding `source_kind` or account ID: existing
QIF/CSV/Trading 212 fingerprint seeds already include source context, and
same-fingerprint conflicts must retain their present behavior. Cross-format
economic matching is a separate import-review problem; the current
source-specific fingerprints do not promise to detect one trade imported
through both CSV and OFX.
An importer maps `SHORT` to `short_sale` and `COVER` to `short_cover` only
when the row's meaning is unambiguous; a negative ordinary holding remains
an unclassified diagnostic. A source correction links its original source
record and normalized operation. Prevent one source row from posting twice.

### Operation families to support

| Family | Named operations and effects |
| --- | --- |
| Trades | Long buy/sell; short sale/cover; explicit gross, charges, net, trade/settlement dates; write-off. A trade crossing zero is two operations. |
| Cash and distributions | Cash dividend, reinvestment, fund capital-gains distribution, return of capital, interest, withholding, fees/rebates, margin or borrow interest, payment in lieu, cash FX conversion. Some need only journal postings; return of capital also changes basis. |
| Position movements | Security transfer in/out and between holding accounts, with carried lot basis and source/destination linkage; split/reverse split; stock dividend; cash in lieu. |
| Corporate actions | Merger, spin-off, conversion/class change, tender/partial redemption, rights issue/exercise/expiry, delisting. Each uses explicit multi-instrument effects, consideration, and basis allocation. Start with manual review and one supported action at a time. |
| Later instrument domains | Bond coupon/accrual, principal repayment, call/maturity, debt conversion; option/future open/close, exercise/assignment/expiry/cash settlement. Require instrument-specific contract terms and accounting decisions first. |

`ticker_change` may be an instrument identity/version change without a
position quantity or basis change. Do not manufacture a sale. A broker's
"add/remove shares" is a review category until its cause and carried basis
are known; otherwise it can conceal a transfer or corporate action.

### Dates, external transfers, and basis limits

An action can have distinct entitlement/ex/record, effective, trade,
settlement, and cash payable dates. Store typed dates supplied by the source;
do not overload the one operation date or infer lot eligibility from the
payment date. The per-kind builder selects the effective position date and
eligible lots from the actual corporate-action terms. Exchange and market
rules can differ, including [special distributions](https://www.investor.gov/introduction-investing/investing-basics/glossary/ex-dividend-dates-when-are-you-entitled-stock-and),
so no universal
"record-date equals lot date" rule belongs in the schema. If entitlement
and payment create separate receivable and cash journal events, link both
posted versions when one command creates both; do not post cash before it
moves. If payment arrives in a later command, create a related settlement
operation with its own audit event instead of changing the original operation.

An external in-kind transfer creates a destination lot with an account-entry
date and a separately retained original acquisition date, carried quantity,
basis currency/value, and source evidence. Keep `opened_on` as the date this
account acquired the position for disposal eligibility; add nullable
`original_acquired_on` and a basis knowledge status. When basis is unknown,
the basis coefficient is NULL, with a constraint tying it to that status;
zero is reserved for a known zero basis. An unknown original date or basis
is explicit `unknown`, never silently today or zero. Show that condition in
positions and block a definitive realized-gain figure until resolved. An
internal transfer links source and destination lot effects and conserves
quantity and basis with direct opposing security postings between holdings.
An external transfer needs both a security leg and a **basis-value bridge**
when carried basis is known. On transfer in, debit the holding and credit
`commodity_trading` in the security; debit `commodity_trading` and credit a
new book-boundary equity account (`external_investment_transfer_equity`) by
the carried basis in its currency. Transfer out reverses those four signs.
The value bridge is an equity contribution/withdrawal, not cash or a sale;
`opening_balance` remains available for a separately named initial-opening
workflow. A transfer wholly inside the book needs neither bridge nor
external equity. The ordinary long-position transfer slice does not imply
support for transferring a short obligation before T-108.
Seed the new equity account under a stable system role and translation key;
include it in export, restore, reports, and self-check without making its
English display label part of the accounting contract.

If basis is unknown, post only the balanced security legs and mark the
position and `commodity_trading` residual **basis unresolved**. Do not
interpret the residual as gain or permit an optional gain reclassification.
A later `transfer_basis_resolve` operation supplies sourced carried basis,
posts the missing value bridge dated to the transfer, and replays affected
lots and disposals through the reconciliation guard. It appends a fact;
it does not edit the original transfer or substitute zero basis.

Return of capital reduces each eligible lot's basis only to zero. Allocate
the cash/basis effect across eligible lots using sourced allocations or an
explicit per-share rule with a deterministic exact remainder. Record any
excess as a separate unresolved economic component, not negative basis or
ordinary dividend income. Post the actual cash; require an explicit
accounting/reporting classification before treating the excess as gain.
This is a domain safeguard, not a universal tax rule. For example,
[US guidance](https://www.irs.gov/publications/p550) treats a nondividend
distribution beyond recovered basis differently from the basis-reducing
portion; the app's operational record must preserve both portions without
choosing one jurisdiction's tax outcome for every book.

For a split or reverse split, post the quantity delta to the holding account
and its opposite to `commodity_trading` in the same security commodity; no
cash posting is invented. Conserve aggregate basis, carry a rational ratio,
and handle fractional cash in lieu as its own linked disposal/cash event.
For return of capital, debit the receiving cash account and credit
`commodity_trading` in that cash commodity. A cash-in-lieu disposal uses
the ordinary sale cash and security balancing legs, with its fractional
basis allocation recorded separately. Each kind's plan must specify all
postings, basis effects, date rules, and reconciliation impact before its
write command is exposed.

The existing QIF export intentionally excludes investment accounts and
non-currency holdings; the lossless investment export is the CSV bundle.
[Quicken's investment action list](https://info.quicken.com/win/tell-me-about-the-investment-transaction-list-s-ac)
is useful as a future mapping inventory (`ShrsIn`, `ShrsOut`, `StkSplit`,
`RtrnCap`, capital-gains distributions, reinvestment, short sale/cover), but
does not make investment QIF round-tripping a gate for this refactor. Keep
the existing explicit QIF unsupported-account verdict until a separate
investment-QIF contract and round-trip tests are accepted.

## One command pipeline

Use one application-level `InvestmentOperationPlan` for the kinds that are
implemented. It contains normalized facts, planned journal postings, typed
lot effects and elections, required source links, account-role dependencies,
and expected reconciliation impact. Kind-specific builders own their
business rules; they call shared validation for exact amounts, dates,
commodities, account roles, side, source identity, and book boundaries.
Preview, reconciliation-impact preview, and commit use the **same plan**.

A shared repository executor opens one SQLite transaction, checks all
dependencies and idempotency, writes one audit event, the operation, its
posted journal transaction(s) and components, lot effects/decisions,
checkpoint invalidations, and import/provider acceptance links, then commits.
Any failure rolls back the whole set. Preserve the existing generic mutation
fence. Keep command-specific result types; a shared executor should not turn
all event logic into a giant switch or allow callers to append arbitrary
holding-account postings without corresponding lot effects.

### Replay and correction

Replay reads **economic intents**, not previously selected disposal lots:
immutable acquisition and transfer-in openings, split/basis effects,
operation-level disposal or cover requests (quantity, side, date, selected
method and its policy provenance), and any explicit specific-lot election.
Resolve the correction chain before ordering them: a replacement supersedes
the original intent for current replay, and a pure reversal removes it from
the effective set. Original and reversing journal postings remain visible,
but replay must not count both the old and replacement acquisitions.
Per-lot disposal allocations and remaining-balance rows are replay outputs.
Original allocation snapshots remain evidence of what the first commit did;
feeding them back as FIFO/LIFO/average inputs would defeat re-selection.
Build the projection from these ordered inputs for each affected
`(book_id, holding_account_id, commodity_id, side, cost_commodity_id)`.
Use effective date plus durable operation/effect sequence for same-day order.
On a correction/reversal, append the new operation and corrective journal
transaction(s), then replay from the first affected intent within the same
SQLite transaction. Keep the committed decision's method, resolution tier,
profile/account version, and any explicit user election. Distinguish the
immutable **original allocation snapshot** from the **effective replay
allocation**: append a numbered revision/effect set linked to the correction and
superseding the earlier effective set; never mutate or delete the original
decision, allocation, or lot event. Current projections use the latest
effective set while exports retain the full chain.

Use **posted reversal plus posted replacement**, not void plus replacement.
For each journal-bearing original, a correction command writes inverse
postings at that original transaction's financial date and replacement
postings at their proper date, as new transactions under one audit event
and one correcting operation. A basis-only correction has no invented
journal rows; it replaces or removes the effective basis intent under the
same audit and replay rules. Each journal replacement uses the existing
`correction_of_transaction_id` to name its original transaction; the
correcting operation and its journal-link roles identify reversals. A pure
reversal omits the replacement. The original transaction
and pinned posted version remain posted, so a historical link's posted
invariant does not depend on which version a mutable transaction currently
selects. The register shows the original, reversal, and replacement as
separate auditable posted rows grouped by their correction chain, with an
expandable explanation; totals include their net effect once. Do not
expose the generic investment void/unvoid path. Reconciliation guards cover
both original-date reversal and replacement-date posting. Any trade-implied
price observation associated with the corrected trade must be superseded or
voided with provenance in the same SQLite command transaction, not left as
a current price for a reversed fill. The present post-commit best-effort
trade-price write must become an atomic, version-linked observation write
before this correction gate passes.

- `specific_lot`: retain the elected lot IDs and quantities. Reject replay
  with a named dependent-operation conflict if a chosen lot is not eligible
  or lacks quantity after the correction.
- `fifo`/`lifo`: retain the method and its original policy provenance, then
  select eligible lots again in corrected dated order. Do not label frozen
  allocations FIFO or LIFO when a new earlier lot changes that order.
- `average_cost`: retain the method and pool scope, then recalculate the pool
  and each disposed basis from the corrected prior events. A changed result
  is an effective revision, not an edit to the original snapshot.
- `short_cover`: apply the corresponding side-specific method and opening
  proceeds rule; never consume long lots or silently cross zero.

An unknown-basis lot stays quantitatively available but carries an unknown
basis through its effective state. Under FIFO/LIFO/specific-lot, a disposal
touching it has unknown disposed basis; under average cost, **one unknown
lot makes the entire affected pool's basis unknown**, so every disposal
from that pool has unknown basis until resolution. Gains views return an
explicit unavailable reason rather than zero. A later basis-resolution
operation records the sourced basis and triggers the same replay; it does
not mutate the opening fact. A resolution that makes a dependent specific-
lot election invalid fails atomically with the dependent operation named.

Realized gains, positions, self-checks, and exports read the current
effective allocation revision and rebuilt state. They must not silently
keep using an original allocation that replay has superseded; historical
views can select an earlier knowledge cutoff only under R18's explicit
projection contract.

If the corrected history makes a later disposal/cover impossible,
return a conflict naming that dependent operation, leaving all records and
checkpoints unchanged. The reconciliation impact preview must account for
every changed posted account balance and date. Accept older imports only when
the replay and dependency checks can complete; before then, keep T-95's
chronological restriction. Independent negative dated balances can be
accepted and flagged, but cannot be silently reclassified as shorts.

## Delivery slices and gates

Each slice keeps the app runnable, updates OpenAPI/types, export/restore and
self-check contracts if it changes them, and adds named tests with independent
expected amounts and conservation checks. Stop at a gate before widening the
next family.

1. **Baseline contract and fixtures.** Specify exact equations and posting
   matrices for current buy/sell/dividend/reinvestment/write-off, including
   mixed-scale and multi-currency charges. Write migration and export contract
   tests against fresh and seeded candidate databases. Settle the operational
   fee/basis and cross-currency rules here, before shaping the schema. This
   slice does not yet rewrite `0001`.
2. **Foundation in two independently validated sub-slices.** Both keep the
   app runnable; neither opens a new user-facing investment operation.

   - **2a — investment schema and writer.** Introduce the parent/link/date/
     component tables, operation-keyed disposal decisions, side-keyed basis
     state, fact/projection split, canonical TEXT coefficients, and direct
     effect links. Implement ADR 0013's link cardinality. Route existing
     commands through the new writer while preserving current import
     identity callbacks. Prove equivalent journal, lots, gains, audit count,
     checkpoint behavior, and import idempotency for old cases. Version the
     investment export contract without losing old facts.
   - **2b — generalized import identity.** Replace the one-transaction
     result with ordered operation/transaction child links. Keep the current
     unique `(book_id, dedupe_fingerprint)` admission rule. Test bank CSV,
     bank QIF, and Trading 212 retries, overlaps, duplicate fingerprints,
     partial failures, and staged-row results through the actual commit
     path. No basis-only import is accepted until this slice passes.

   Revise the unused `0001` baseline under ADR 0013 for 2a and again for
   2b if still pre-release: each rewrite declares `BREAKING DEV DATABASE`,
   updates checksum, fixture, upgrade/equivalence tests, and resets
   disposable developer databases. After release, 2b uses a forward
   migration. Do not combine 2a and 2b merely to save a migration number.
3. **Exact trade economics.** Add gross, fee/tax components, currencies,
   net settlement, and settlement date to buy/sell input/API and import
   mapping. Move gains from cash-posting inference to explicit per-disposal
   economics; preserve write-off as zero proceeds. Show gross, charges, and net in
   preview and UI. For fees in another currency, require explicit cash legs
   and rate/source facts; do not hide an implicit conversion.
4. **Replay and investment-native correction (T-75b).** Implement rebuild,
   correction/reversal, dependency conflicts, and reconciliation preview.
   Exercise a corrected old buy followed by several sells under each basis
   method, including the different allocation rules above, same-day ordering,
   import replacement, trade-price provenance, and failure rollback. Switch
   realized gains, positions, and self-checks to the latest effective
   allocation set and rebuilt state; test a correction where the original
   and effective FIFO allocations differ. Exact trade economics precede
   this slice so replay has one authoritative source for proceeds and charges;
   this refines ADR 0013's foundation-to-correction sequence.
5. **Transfer and basis actions.** Transfer lots in kind across accounts
   without a gain; return of capital with exact basis effects; split and
   reverse split with conserved basis; cash in lieu with allocated fraction.
   Before each command, approve a per-kind posting matrix, basis-allocation
   rule, dated eligibility rule, reconciliation preview, and unknown-basis
   behavior; the shared schema alone is not that specification. Add manual
   commands before provider auto-acceptance. These address the
   common broker migration and everyday holding cases in R16 first.
6. **Short sale and cover (T-108).** Add side-aware lot opening and covering,
   disposal allocation/realized-result logic, portfolio/gains/self-check,
   mobile UI, import classification, and cash-only borrow costs. A negative
   journal share balance with no short operation stays an explicit warning.
7. **Compound corporate actions.** Implement merger/spin-off/stock dividend,
   then tender/rights/conversion as actual broker examples justify them.
   Require per-kind posting and basis-allocation specifications before code.
8. **Instrument-specific expansion.** Design bonds and derivatives as
   separate contracts when demanded by product scope. Do not call a generic
   buy/sell of an `option` or `bond` complete support for that instrument.

### Required acceptance cases across slices

- A buy with gross `-100` and commission `-2` settles `-102`; a sale with
  gross `+100` and commission `-2` settles `+98`. Both produce the expected
  journal postings, basis/proceeds, and gain; rebate and separately paid fee
  cases reconcile exactly. Source-only net remains marked as such.
- Every operation's linked posted journal and lot effects agree after each
  command and after rebuilding projections; failure injected at each write
  stage leaves no partial operation, audit, checkpoint invalidation, or import
  identity.
- A two-lot partial sale, later backdated buy, correction, short opening and
  partial cover preserve lot quantity, basis/opening proceeds, and side
  independently; an impossible dependent disposal is rejected atomically.
- With a buy of 100 and a final net sale of 120, the cost-currency
  `commodity_trading` residual is -20 under the debit-positive journal
  convention, corresponding to an operational +20 gain. A known-basis
  external transfer in of 100 followed by the same sale gives the same -20
  residual after its equity bridge; transfer out cancels its carried basis
  from clearing. An unknown-basis transfer leaves gains and optional gain
  reclassification unavailable until a sourced resolution bridges and
  replays it.
- A posted correction retains the original transaction/version, adds a
  reversal and replacement (or only a reversal), and groups all rows in the
  register. Its pinned links pass self-check even after later corrections;
  its postings and dependent lot effects net to the corrected facts.
- One unknown-basis lot makes average-cost pool gains unavailable; resolving
  that lot via an audited operation rebuilds the pool and affected decisions.
- Transfer and split preserve total basis. Return of capital changes basis
  without dividend income, floors each lot at zero, and retains any excess
  for explicit classification; cash in lieu consumes exactly its fractional lot
  allocation. A cross-instrument action reconciles old/new quantities and
  allocated basis in one operation.
- An entitlement/ex date before a sale and cash payment after it selects the
  correct eligible lots without pre-dating the cash posting. An external
  transfer retains original acquisition date and known or unknown carried
  basis separately from the account-entry date.
- Every linked component agrees with its posting version under the declared
  sign rule. A restored/replayed database uses the same effective decision
  revision for gains, positions, and self-check, and keeps each superseded
  original visible in export.
- A source row that yields two operations has one dedupe identity and two
  ordered effects; an exact duplicate fingerprint across source kinds is
  still refused by the book-wide unique key. Separate CSV and OFX imports
  of the same economic event remain a review/matching concern unless their
  fingerprints actually match.
- Exports and restore retain the operation, components, links, lot effects,
  policies, source IDs, and correction chain. A newly restored database gives
  the same positions, gains, warnings, and reconciled checkpoints.

## Decisions to settle before schema implementation

1. Implement ADR 0013's clarified operation-to-journal link contract in
   slice 2. Do not leave a mandatory single-transaction shortcut in place.
2. Define the operational treatment of each fee/tax component for lot basis
   and proceeds, including source-only net and fees in another currency.
   Keep tax-jurisdiction projections outside this decision. Settle this in
   slice 1 and exercise it in slice 3.
3. Use canonical TEXT coefficients in the baseline for investment monetary
   facts; preserve the current documented int64 service admission limit until
   arithmetic is deliberately widened. Test upper-range and mixed-scale
   input, storage, and export values in slice 2.
4. Use a named investment operation for broker cash activity with an
   investment-specific meaning (distribution, standalone investment fee,
   borrow charge, payment in lieu, or an explicit broker FX trade); the
   instrument link is optional when the activity is account-wide. Ordinary
   cash deposits and withdrawals remain ledger transfers with the same
   generalized source-row identity. This avoids inventing an instrument
   for account-level interest or funding.

The unused baseline can be revised again before the first installed release,
with an explicit `BREAKING DEV DATABASE` note, checksum, fixture, and test
updates every time. Do not tag v0.1.0 halfway through a pending baseline
rewrite. After the first installed release, every later slice uses a numbered
forward migration and preserves installed data; no plan step assumes the
baseline remains editable indefinitely.
