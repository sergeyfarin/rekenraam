# Investment operation and subledger refactor plan

Status: active contract and delivery gates, reviewed 2026-10-02.
ADR 0012 and ADR 0013 govern. R16 owns long-position lifecycle work; T-108
owns shorts. Planned behavior is not shipped behavior.

[Implemented features](../implemented.md) records shipped behavior.
[The 2026-10-01 progress snapshot](../reviews/investment-operation-progress-2026-10-01.md)
preserves the former slice-by-slice history. [The initial review](../reviews/investment-review-2026-10-02.md) and
[follow-up disposition](../reviews/investment-followup-review-2026-10-02.md)
record evidence, qualifications and issue boundaries.
The [roadmap](../roadmap.md) owns product order; GitHub owns live issue state
and priority. Always distinguish local `T-nn` IDs from GitHub `#nn` numbers.

| Contract area | Verified boundary | Next gate |
|---|---|---|
| Foundation / exact trade economics (1–3) | Shipped; #125 integrity complete; bundle schema 6; opening facts merged onto lots and effective reads in SQL views (T-124) | Require an opening operation on every lot ([T-133 #148](https://github.com/sergeyfarin/rekenraam/issues/148)) |
| Long buy/sale correction (4a–4ai, #99, T-117) | Reversal, replacement, recorded-method replay, backdated buys, transfer-in, sales and write-offs, Trading 212 quantity/net revisions, shared writer | Gain-impact disclosure and remaining families have separate bounded issues |
| Preview feasibility | Buy/source replacements, plain buys and reinvestment run rolled-back writer replay; openings return actual checkpoint sets | Disclosure shipped for every existing replay path (#129, #141); new commands opt in |
| Transfers (5b/5d, T-123, T-132) | Known-basis external inbound; explicit-lot and pooled average-cost internal, with exact remainder and snapshotted policy; changed carried basis propagates through destinations and chains as link revisions (T-132) | Pooled lineage changes ([T-135 #150](https://github.com/sergeyfarin/rekenraam/issues/150)); transfer correction (#134); unknown immutable facts/resolution |
| Split / basis actions (5) | Manual split/reverse split with replay, gain disclosure, export, self-check, mobile entry and Trading 212 split-row linking (T-122) | Split correction and guarded journal-delta adjustment; return of capital, cash in lieu |
| Shorts / compound actions (6–7) | Operation/side foundation only | Side-aware commands; compound date/effect cardinality and replay |

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

## Data contract

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

Use **operation-to-journal links** (`investment_operation_journal_links`):
`(book_id, operation_id, transaction_version_id, link_seq, role)`, unique on
transaction version. The version pins the exact posted entries; its transaction
and book are derived and validated. The retired operation transaction header
must not return. A correction appends new links and effects rather than
rebinding a historical link to another version.
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

For the operational basis/proceeds policy, a charge contributes to basis or
reduces proceeds only when its value reaches `commodity_trading` in the cost
currency, either in net settlement or through an explicit clearing leg. A
charge separately posted to expense is excluded from operational
basis/proceeds and reported as that expense. Do not count one charge both in
a lot result and in an expense account. A cross-currency charge needs
explicit conversion/value legs before it can affect the cost-currency
clearing identity. Slice 1 fixes the per-kind equations and tests both
treatments; this operational rule is not a universal tax rule.

Store each known charge component's treatment as an immutable election,
resolved from explicit transaction choice, account policy, global book policy, or
fallback, in that order. Snapshot the winning tier, version/effective date,
and expense account when used; later default changes never reinterpret a
committed trade. The fallback for ordinary same-currency trade commissions
is `clearing_included`, preserving today's net-cash treatment. A charge kind
without a specified fallback is rejected with a named error in manual entry;
an import row stays in review. Imports use the same policy resolution, and
changing an election requires an explicit correction.

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
decision is provenance, not its identity. Current journal-bearing decisions
require that pinned link; any future journal-free decision kind needs an
explicit admission, export and self-check contract. Original decisions and
allocations remain immutable even if a later replay supersedes their
effective allocations.

Keep immutable `investment_lots` identity and opening facts separate from
`investment_lot_state`, the shipped projection keyed by lot ID. Status,
remaining quantity/basis and update attribution belong in the projection;
original quantity/consideration stay in immutable evidence. Rebuild lot and
position basis state atomically from effective history. The basis-state key
includes `position_side`; every selection/rebuild must respect it. An
operation-opened lot row *is* that operation's opening fact
(`investment_lots.operation_id`, T-124); there is no duplicate facts table.
Effective readers select through the `effective_investment_operations`,
`latest_investment_disposal_revisions`, `latest_investment_split_revisions`
and `effective_investment_lot_events` views; audits read the base tables.
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
Use child links as the complete committed result;
`import_staged_rows.committed_transaction_id` is only a compatibility summary. Dedupe, source correction, retry, and all
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
Add the new equity role to `accounts.system_role`'s baseline `CHECK` list
and seed its account in slice 2a, before transfer commands ship. Give it a
stable translation key and include it in export, restore, reports, and
self-check without making its English display label part of the accounting
contract. After release, adding that role would require an `accounts` table
rebuild despite its many financial references.

For a transfer out, store sourced quantity, dates, and any broker-reported
basis as immutable evidence. The *operational* carried-basis bridge is a
method/election-derived amount, snapshotted with calculation provenance and
one value leg per cost currency. An alternative R18 report profile never
changes that posted bridge; a correction to the economic history may
require a new journal adjustment as described below.

If basis is unknown, post only the balanced security legs and mark the
position and `commodity_trading` residual **basis unresolved**. Do not
interpret the residual as gain or permit an optional gain reclassification.
A later `transfer_basis_resolve` operation supplies sourced carried basis,
posts the missing full value bridge dated to the transfer, and replays
affected lots and disposals through the reconciliation guard. This applies
to unknown-basis transfers **in and out**. It appends a fact; it does not
edit the original transfer or substitute zero basis. A correction that would
turn a known-basis transfer out into an unknown-basis one is refused with
that transfer named, leaving its original bridge and all other state intact.

Return of capital reduces each eligible lot's basis only to zero. Allocate
the cash/basis effect across eligible lots using sourced allocations or an
explicit per-share rule with a deterministic exact remainder. Record any
reported excess as source evidence; the effective excess is a replay-derived
allocation (cash received less eligible basis reduction), not a fixed source
component. It remains unresolved rather than becoming negative basis or
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
basis allocation recorded as a replay-derived revision, separate from
immutable fractional quantity and cash received. Each kind's plan must
specify all postings, basis effects, date rules, and reconciliation impact
before its write command is exposed.

The existing QIF export intentionally excludes investment accounts and
non-currency holdings; the lossless investment export is the CSV bundle.
[Quicken's investment action list](https://info.quicken.com/win/tell-me-about-the-investment-transaction-list-s-ac)
is useful as a future mapping inventory (`ShrsIn`, `ShrsOut`, `StkSplit`,
`RtrnCap`, capital-gains distributions, reinvestment, short sale/cover), but
does not make investment QIF round-tripping a gate for this refactor. Keep
the existing explicit QIF unsupported-account verdict until a separate
investment-QIF contract and round-trip tests are accepted.

## One command pipeline

Use a prepared command plan for each implemented kind, with shared writer
orchestration. It contains normalized facts, planned journal postings, typed
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
The operational transfer-out carried-basis amount, return-of-capital basis
reduction/excess split, and cash-in-lieu disposed basis are also replay
outputs. Preserve their first committed snapshots and append effective
revisions; never rewrite source components or original posted journal rows.
Build the projection from these ordered inputs for each affected
`(book_id, holding_account_id, commodity_id, side, cost_commodity_id)`.
Use effective date plus durable operation/effect sequence for same-day order.
On a correction, reversal, accepted backdated acquisition/import, or basis
resolution, append the new operation and its required journal postings,
then replay from the first affected intent within the same SQLite
transaction. Keep the committed decision's method, resolution tier,
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
a current price for a reversed fill. The shipped writer uses atomic, version-linked
price observation writes; every correction uses that same path.

If replay from any accepted operation changes a previously posted
transfer-out bridge, append the exact per-currency difference between old
and new effective bridge amounts as a
balanced `commodity_trading`/external-transfer-equity adjustment at the
transfer's financial date. Link it to the operation that triggered replay,
the original transfer, and both calculation revisions; include every
affected account, currency, and date in the reconciliation-impact preview.
Commit the adjustment, revised subledger projections, audit event, and checkpoint
effects together. If any dependent amount cannot be calculated exactly or
the reconciliation guard rejects a required adjustment, reject the triggering
command with the later transfer named and leave the database unchanged.
An unchanged, explicitly unknown-basis transfer remains unresolved under
the existing rule; it does not fabricate a bridge. A pure change of
reporting profile never triggers an adjustment.

A basis-resolution operation that first makes an unknown-basis transfer out
known posts the **full** bridge, rather than computing a delta from zero.

If a later formal posting classifies return-of-capital excess, its
replay-changed amount needs the same guarded adjustment or the triggering command
must fail with that classification named. Cash-in-lieu basis alone changes
its subledger gain result, not its fixed cash or security journal legs.

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

If the corrected history makes a later disposal/cover impossible, or leaves
a required known-basis bridge or classification allocation uncomputable,
return a conflict naming that dependent operation, leaving all records and
checkpoints unchanged. The reconciliation impact preview must account for
every changed posted account balance and date. Accept older imports only when
the replay and dependency checks can complete; before then, keep T-95's
chronological restriction. Independent negative dated balances can be
accepted and flagged, but cannot be silently reclassified as shorts.

## Delivery slices and gates

Each slice keeps the app runnable, updates API, export/restore and self-check
when affected, and has named exact-conservation and rollback tests. #129,
#141, #137, #138, #132, the #139 decision and #147 closure propagation have
shipped. #141 covered
reinvestment, imported acquisitions, native reversals/replacements and source
revisions; new commands opt into the same policy themselves.
Shared checkpoint preview under-reporting #142 is complete; same-day boundary
and combined-delta behavior remain #135. Current sequence:

1. **Shared gain-impact safety mechanism** — [T-114 #129](https://github.com/sergeyfarin/rekenraam/issues/129). **Shipped** for manual buys: a command sets
   `GainImpactPolicy` and the shared writer snapshots effective disposals before
   guards/journals, compares after domain effects in the same transaction
   (revised/replaced/removed, by correction root + decision sequence), and refuses a
   non-empty set without the preview's exact token. Removal/replacement are
   compared explicitly; `persistInvestmentReplayProjectionTx` is not the hook.
   Existing paths are rolled out under [T-126 #141](https://github.com/sergeyfarin/rekenraam/issues/141) (shipped; matrix in implemented.md).
   New split, transfer and correction commands must integrate #129 in their own acceptance.
   A permanent “gains acknowledged through” date is an unaccepted design option,
   not a new book rule. R18 still owns reproducible historical/tax reporting.
2. **Split/reverse split** — [T-122 #137](https://github.com/sergeyfarin/rekenraam/issues/137), under
   [#114](https://github.com/sergeyfarin/rekenraam/issues/114) and the
   [slice 5 contract](investment-operation-slice-5-contract.md). **Shipped**
   (2026-10-03): manual exact-ratio command with replay admission, gain
   disclosure, mobile entry, export/self-check, and Trading 212 split-row
   linking without double posting. `STOCK_SPLIT` alone supplies no ratio or
   entitlement, so automatic mapping stays blocked on verified payload evidence
   ([T-130 #145](https://github.com/sergeyfarin/rekenraam/issues/145)). Split correction and a guarded journal-delta adjustment
   ([T-129 #144](https://github.com/sergeyfarin/rekenraam/issues/144)) and zero-delta splits ([T-131 #146](https://github.com/sergeyfarin/rekenraam/issues/146)) remain; until then a
   history change that alters a split's quantity is refused with the split named.
3. **Pooled internal transfers** — [T-123 #138](https://github.com/sergeyfarin/rekenraam/issues/138). **Shipped**
   (2026-10-03): the source's method lock, else its resolved default, chooses
   selected-lot or pooled allocation. A pooled move reuses the sale's dated
   pool depletion (FIFO lot links for lineage, exact remainder on the last
   touched lot), snapshots method/tier/version on the transfer fact, locks the
   source family, and leaves the destination's method state to integrate the
   new lots. Replay re-runs the pool and refuses any changed link with the
   transfer named. This is operational average cost, not tax-policy
   completeness.
4. **General backdating** — [T-117 #132](https://github.com/sergeyfarin/rekenraam/issues/132). **Shipped**
   (2026-10-03). Known-basis transfer-in uses the opening replay admission
   (link written before replay so the original date orders FIFO/LIFO while
   the transfer date gates availability). A sale or write-off dated behind a
   later depletion joins the effective intents at its same-day slot after
   earlier entries; replay yields its allocations (written as historical
   evidence) and revises later decisions under their recorded policy and
   elections. Preview, reconciliation impact and commit run the same writer;
   both opt into gain acknowledgement. An impossible later decision is named
   (`INVESTMENT_SALE_DEPENDENCY`). Reinvestment admission is pinned unchanged.
5. **Replay scope / effective reader consolidation** — [T-124 #139](https://github.com/sergeyfarin/rekenraam/issues/139).
   **Decided and shipped** (2026-10-03, ADR 0013 *Cross-Position Replay Scope
   Refinement*): an affected-position dependency closure, seeded per changed
   position and date and following every recorded internal transfer to a fixed
   point, replayed as one merged dated stream in the command transaction. A
   whole-book rebuild was rejected (O(book) per command and preview, wider
   failure radius, same refusals) and kept only as a verifier
   ([T-134 #149](https://github.com/sergeyfarin/rekenraam/issues/149)). The
   closure (`InvestmentReplayClosure`) ships with chain, cycle, ordering and
   unrelated-position tests. **Propagation shipped**
   ([T-132 #147](https://github.com/sergeyfarin/rekenraam/issues/147),
   2026-10-03): a changed carried basis appends a transfer link revision and
   replays each destination to a fixed point, through chains, cycles,
   specific-lot elections, pooled sources and same-day sales, with gain
   disclosure and preview = commit. Removing a transferred acquisition and
   changed pooled lineage ([T-135 #150](https://github.com/sergeyfarin/rekenraam/issues/150))
   stay named refusals. Effective reads moved to
   SQL views, `investment_lot_facts` merged into `investment_lots`, and the
   baseline's ALTER/recreated view/trigger leftovers were folded into final
   DDL. Repeated typed-date sequence stays a #115 admission gate.
6. **Correction families** — dividends/reinvestment [T-115 #130](https://github.com/sergeyfarin/rekenraam/issues/130),
   date/account/instrument/currency [T-116 #131](https://github.com/sergeyfarin/rekenraam/issues/131), write-offs [T-118 #133](https://github.com/sergeyfarin/rekenraam/issues/133),
   transfers [T-119 #134](https://github.com/sergeyfarin/rekenraam/issues/134), register grouping/net checkpoint impact
   [T-120 #135](https://github.com/sergeyfarin/rekenraam/issues/135). Field and transfer changes depend on general backdating and
   the cross-position decision where applicable. No umbrella P0 depends on all
   future command families. Provider cancellation/wider revisions
   [T-121 #136](https://github.com/sergeyfarin/rekenraam/issues/136) are blocked pending verified execution evidence.
7. **Remaining slice 5 actions**, then **short sale/cover**
   [T-108 #103](https://github.com/sergeyfarin/rekenraam/issues/103), then
   **compound actions [#115](https://github.com/sergeyfarin/rekenraam/issues/115)**.
   Outbound transfers, unknown-basis resolution, return of capital and linked
   cash in lieu retain the slice 5 posting/replay contracts. Compound kinds
   require repeated typed-date sequences (`investment_operation_dates` is still
   keyed by `(operation_id, date_role)`), explicit clearing attribution and,
   for cross-position effects, the #147 propagation before admission. Bonds/derivatives need separate instrument contracts.

Independent validation performance work is [T-125 #140](https://github.com/sergeyfarin/rekenraam/issues/140); it must preserve
race and meaningful financial coverage. It does not block defining these slices.

### Required acceptance cases across slices

- A buy with gross `-100` and commission `-2` settles `-102`; a sale with
  gross `+100` and commission `-2` settles `+98`. Both produce the expected
  journal postings, basis/proceeds, and gain; rebate and separately paid fee
  cases reconcile exactly. Source-only net remains marked as such.
- For a 100 purchase plus a 2 same-currency commission, capitalized treatment
  leaves +102 in cost-currency clearing and a 102 opening basis; expensed
  treatment leaves +100 in clearing, a 100 basis, and +2 in expense. For a
  120 sale with a 2 commission, the corresponding proceeds are 118 or 120,
  respectively. No result counts the fee in both gain and expense.
- Fee treatment for each charge records its resolved tier and policy version;
  changing an account or book default does not alter an earlier trade, and
  imported charges use the same resolution as manually entered charges.
- In 2a a holding priced only by a net-derived trade observation retains its
  valuation and unrealized gain while the observation gains an approximate
  flag and transaction-version provenance. The trade, price, and any newly
  created series share exactly one audit event. In slice 3 a trusted price
  on the same date outranks that estimate; the estimate remains usable and
  visibly labeled when it is the best available price.
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
- Correcting an acquisition before a transfer out changes the effective
  carried basis and appends only the delta bridge at the transfer date under
  the reconciliation guard. Return-of-capital excess and cash-in-lieu basis
  gain revisions use the recalculated lot state while source cash stays fixed.
- A backdated acquisition that changes a later transfer-out basis makes the
  same guarded delta adjustment. Resolving an unknown-basis transfer out
  posts its full bridge; a correction that would make a known transferred
  basis unknown fails atomically with that transfer named.
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

## Schema and admission gates

Operation journal links, fee treatment and canonical TEXT coefficient contracts
are settled by ADR 0013 and the slice 1 contract; service int64 limits remain
explicit. Never infer investment meaning from an ordinary negative holding or
broker order status. Account-wide investment cash activity can have an optional
instrument link; ordinary deposits/withdrawals remain ledger transfers.

The unused baseline can be revised before the first installed release only
with `BREAKING DEV DATABASE`, checksum, seed, export/restore and fresh/seeded
validation updated together. After installation, use immutable numbered forward
migrations. Do not tag v0.1.0 during a pending baseline rewrite.
