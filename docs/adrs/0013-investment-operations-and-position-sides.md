# ADR 0013: Named Investment Operations And Position Sides

## Status

Accepted

## Date

2026-09-26

## Context

The first investment slice records purchases as positive long lots and sales as
disposals of those lots. It cannot represent a short sale: an ordinary sale
requires existing long lots, while a generic negative share posting has no
evidence that the user intended to borrow and sell shares. The same gap affects
security transfers, basis adjustments, corporate actions, and corrections.
These operations cannot safely be inferred from the sign of a journal posting
or a provider event name.

The v0.1 release candidate has not been installed or used as a durable user
database. The owner explicitly authorized schema redesign before release. The
general journal and ADR 0012's four-layer boundary remain the foundation.

## Decision

1. Every investment-domain command has a **named operation kind**. A durable
   operation links its subledger events, source/provenance, audit attribution,
   and any posted journal transaction versions it creates. A basis-only
   adjustment can have no journal posting; a compound action can link more
   than one posted transaction version. A trade records gross consideration,
   charges, and net settlement as separate exact facts, with their currencies
   and scales. A long disposal decision also stores operational proceeds after
   clearing-included charges, independently of net cash. A charge component
   stores its kind, treatment, mapped account, policy version and source
   evidence; later policy edits do not change it.
   Operation kinds are stable, non-empty codes, validated by their command;
   the database does not need a new migration for each new code. They are not
   reconstructed from transaction descriptions or negative quantities.
   One imported source fingerprint admits one book-wide identity with ordered
   child effects. Each child names an investment operation, a journal
   transaction, or both; a staged row links to the same identity. All children,
   dedupe admission, and the staged result commit with the journal and
   subledger writes. The first transaction ID may be exposed as a compatibility
   summary, but the ordered children are the committed result.
2. Position lots have an explicit **long or short side** and a positive open and
   remaining quantity. A long lot preserves acquisition cost; a short lot
   preserves opening proceeds. A cover consumes short lots and measures its
   economic result from allocated opening proceeds less covering cost and
   charges. A normal sale only consumes long lots. Neither command silently
   crosses zero into the opposite side. A trade crossing zero is entered as
   two explicit operations.
3. The posted journal remains balanced by commodity. A short opening records
   cash received and a negative security holding; a cover records cash paid
   and reduces that negative holding. The `commodity_trading` account balances
   each commodity, as it does for long trades. Collateral movements, borrow
   fees, margin interest, and payments in lieu of dividends are separate named
   cash operations. Their accounting treatment is not folded into share
   quantity or invented by a valuation report.
4. Net worth, portfolio positions, gains, self-checks, exports, and import
   review use the named operation and side. Only the quantity supported by
   dated short events is classified as a short. Any unmatched negative
   quantity remains an unclassified warning, including during out-of-order
   imports. Mixed long and short exposure, if permitted for the same account
   and instrument, remains separately visible rather than netting away a side.
5. Lot events are immutable facts. Current lot balances and remaining
   consideration are rebuildable projections. Correction/reversal commands
   preserve the original and replay affected positions in event order, while
   checking dependent disposals, cost-basis elections, reconciliation, import
   identity, prices, and audit history in one SQLite transaction. Backdating
   behind a previous disposal or cover remains refused until replay exists.
   A correcting operation names exactly one earlier operation in the same
   book, an immutable reason, and either `replace` or `reverse`. Each operation
   has at most one direct successor, so a chain has one effective end. A pure
   reversal is terminal and contributes no replay intent. Replay excludes
   intents belonging to any operation with a correcting successor, preserving
   those original intents for audit while selecting only the effective end.
   A replacement inherits the root operation's same-day order slot during
   replay. Its new database ID does not move an old acquisition after a sale
   that originally followed it on the same date; effect sequence still orders
   events within the effective operation. A later specific-lot disposal's
   immutable election keeps its original lot ID for audit, while effective
   replay follows that acquisition's correction chain to the replacement
   lot. If the replacement cannot satisfy the elected quantity, the whole
   correction is refused.
6. Cash corporate actions are typed. Return of capital changes lot basis;
   cash in lieu needs its own lot allocation or disposal relationship. Neither
   is accepted as ordinary dividend income solely because cash arrived. A
   split changes quantity but conserves total basis; a transfer moves lot
   identity and basis between holding accounts without realizing a gain. An
   external in-kind transfer with known carried basis also needs a balanced
   basis-currency bridge between `commodity_trading` and explicit book-boundary
   equity (`external_investment_transfer_equity`), so closing the transferred position does not leave all sale proceeds
   in clearing. Unknown carried basis remains an explicit unresolved state;
   clearing cannot be called gain until a sourced basis-resolution operation
   supplies the bridge and replays the position. A transfer-out bridge uses
   the committed operational carried-basis calculation. A later correction
   that changes that calculation appends a balanced dated bridge adjustment
   with reconciliation review; it does not rewrite the original journal.
   Backdated acquisitions or basis resolutions that trigger the same replay
   use the same guarded adjustment. Unknown-basis transfers out post only
   security legs until a sourced resolution posts the full bridge; a
   correction that would make a known outbound basis unknown is refused.
   For the first internal-transfer command, source-lot quantities and carried
   basis are immutable links to destination lots. Replay includes transfer
   depletion. If a correction would change a linked carried basis or remove
   its source lot, it refuses with a named dependency until cross-account
   transfer revisions can replay destination lots and dependent disposals
   atomically.
7. The implementation proceeds in bounded slices: operation/side schema and
   exact trade economics; replay and native correction; transfers, basis
   adjustments, and splits; short opening and covering with diagnostics and
   UI; then complex corporate actions. Exact trade economics precede replay
   so corrected allocations use explicit proceeds and charges. Transfers and
   splits precede short trading because they serve existing long-position and
   broker-migration workflows. Each slice remains runnable and has explicit
   API, export, and self-check behavior. Provider events stay suggestions
   until the matching domain operation can be posted safely.

This decision refines ADR 0012. It does not select a jurisdiction's tax rules
or change the operational long-position cost-basis methods. R18 still owns
named reporting profiles and alternative read-side projections.

## Migration Policy For This Pre-Release Redesign

Because no supported v0.1 installation exists, the baseline schema may be
revised if that produces a simpler durable model. Such a rewrite is declared
`BREAKING DEV DATABASE`, updates the schema checksum and release fixtures, and
requires disposable development databases to be reset. A forward migration is
equally valid when it gives the same model with less disruption. Once a release
with actual installations ships, its migrations are immutable and upgrades
must preserve data.

## Disposal-Clearing Attribution Refinement (2026-09-30)

Operational disposal proceeds are attributed explicitly to pinned
`commodity_trading` posting versions in the decision's cost currency and
non-reversal operation journal. Immutable signed portions support several
decisions sharing a posting and one decision spanning settlement and fee legs.
Conservation is checked independently per decision and per posting, in exact
scaled arithmetic. A group total alone cannot prove individual attribution.
Fee payment dates may differ from the disposal date. Separately expensed fees
do not contribute; a zero-proceeds write-off may have no cost-currency leg.
Basis-only replay preserves the attribution; a replacement sale records new
portions while originals remain audit evidence. Current single-disposal
commands allocate whole legs atomically; compound commands must supply their
own portions before posting. This clarifies decision 1 without admitting a
compound sale workflow or changing fee treatment.

## Cross-Position Replay Scope Refinement (2026-10-03, T-124)

Decision 6 refuses a correction that changes a linked internal-transfer
carried basis "until cross-account transfer revisions can replay destination
lots and dependent disposals atomically". This refinement selects how that
replay is scoped, before outbound transfers and compound actions build on it.

**Chosen: affected-position dependency closure, replayed as one merged dated
stream inside the command's SQLite transaction.**

- *Closure.* Seed it with the positions `(account, instrument, side, cost
  currency)` whose effective intents the command changes, each from the
  earliest date it can differ. Follow every recorded internal-transfer lot link
  from source to destination when the transfer is dated on or after the
  source's affected date; the destination is affected from the transfer date.
  Repeat to a fixed point. Corrected and reversed transfers are followed too,
  so the closure covers the edges before and after the command. Positions with
  no path to a seed have identical inputs and are not touched. A transfer out
  of the book ends the walk; its change is a guarded dated bridge adjustment,
  not destination replay (decision 6).
- *Ordering.* Positions are not replayed one after another: a chain may
  cycle (A→B, later B→A) and same-day order crosses positions. All closure
  intents are merged and sorted by date, correction-root slot and effect
  sequence. A transfer's destination opening is produced at the transfer's slot
  from the replayed depletion; the original acquisition date still orders
  FIFO/LIFO and the transfer date gates availability.
- *Atomicity.* One savepoint simulates the closure; the shared writer runs
  gain-impact disclosure (#129) and reconciliation impact over all of it, and
  preview runs the same writer. Any refusal leaves the database unchanged with
  the dependent operation named.

**Rejected: whole-book rebuild per command.** For positions inside the
closure it computes the same result, and outside it every input is unchanged,
so the extra work is a no-op. It costs O(book) on every command and every
preview, and widens the failure radius: an unrelated position with an
unmodeled lot, unresolved basis or a not-yet-supported action would block
unrelated commands. Rebuilding also does not remove legitimate refusals. An
invalid specific-lot election, missing or unknown source basis, a split whose
posted quantity would change, or an uncomputable bridge is a fact about the
history, not about replay scope, and stays a named refusal under either choice.
A full rebuild remains useful as an offline self-check verifier
([T-134 #149](https://github.com/sergeyfarin/rekenraam/issues/149)). It shipped 2026-10-04 as the
`investment_replay_equivalence` check: every long position is replayed from
its effective intents in a rolled-back savepoint, one short write transaction
per position, and compared exactly with its stored lots, disposal allocations,
split effects, method lock and transfer links. Because each position is
checked against the stored effective links, a stale link or a destination
replayed from one is found, which verifies the whole fixed point without a
merged whole-book replay.

**Status.** The closure is implemented and tested
(`InvestmentReplayClosure`); propagation shipped in
[T-132 #147](https://github.com/sergeyfarin/rekenraam/issues/147), below.

**Propagation (2026-10-03, T-132).** This supersedes decision 6's refusal of a
changed internal carried basis. A transfer's units, destination lot and
original acquisition date are fixed facts, so the only input one position
gives another is the basis a link carries and which source lot it is taken
from. The merged stream is therefore computed exactly by replaying a position,
recording each link whose replayed depletion differs from its effective one,
appending an `investment_transfer_link_revisions` row, and replaying each
destination from that effective link, repeating until nothing changes. A
source's depletion at a transfer's slot depends only on its own earlier
history, so a cycle settles; a position in a cycle may be replayed (and its
disposals revised) twice in one command, and the last revision is current.
Positions reached are the subset of the closure whose links actually moved.

*Single merged pass (2026-10-05, [T-140 #155](https://github.com/sergeyfarin/rekenraam/issues/155)).*
The first implementation reached that result by rounds: each round replayed
every destination whose incoming link had changed, up to a fixed 256 rounds.
One round crosses one transfer, so a valid chain deeper than the bound (one
share moved back and forth 257 times between two accounts) was refused as
non-convergent, and a chain of n transfers replayed whole histories n times
(128 transfers: 25 s). Propagation now runs the merged dated stream itself:
the closure downstream of the changed links is simulated once in a savepoint,
all positions' intents in one causal order (date, correction-root slot,
effect sequence). Each transfer appends its revised link inside the pass, so
the destination lot opens at the new basis and its FIFO/LIFO order reads the
new original date; a destination reached out of causal order is an internal
refusal, not a silent result. Every changed link gets one revision and every
affected position is persisted once from its final inputs. There is no round
bound; depth costs one pass (300 transfers: about 2 s).

- A transfer from a replaced acquisition depletes the replacement lot of the
  same correction root, as a specific-lot election does; the revision records
  that effective source lot. A successor opened on another date is not
  followed, because the link's original date orders the destination.
- The first committed link and destination lot opening stay immutable
  evidence. Effective reads and self-check use the latest link revision for
  both ends; an in-book transfer posts no basis, so no journal changes.
- Still refused with the transfer named: removing the transferred acquisition
  (reversal), and a `source_lots` transfer from an average-cost pool that
  would now deplete different source lots or quantities. The default
  `pooled_lot` lineage admits that change (*Pooled Transfer Lineage
  Refinement*, below; [T-135 #150](https://github.com/sergeyfarin/rekenraam/issues/150)).
  A future transfer correction (T-119 #134) must also seed propagation with
  the destinations of edges it removes or replaces.

**Effective reads and opening facts.** Effective selection lives in SQL views:
`effective_investment_operations` (the end of each correction chain; a pure
reversal is not effective), `latest_investment_disposal_revisions`,
`latest_investment_split_revisions` and `effective_investment_lot_events`.
Replay intents, realized gains, gain impact, split-link import and the
projection checks of self-check read through them. Self-check still audits
every original and superseded allocation set from the base tables, and
correction admission still asks directly whether an operation already has a
successor. The duplicate `investment_lot_facts` table is merged into
`investment_lots.operation_id`, which keeps its canonical-fact constraints and
immutability; `source_transaction_id` stays as journal provenance and disposal
decisions keep their transaction/version links. Repeated typed-date sequence
remains a gate for the first compound kind (#115) rather than unused schema.

## Split Journal-Delta Adjustment Refinement (2026-10-03, T-129)

This supersedes the example above that lists "a split whose posted quantity
would change" as a standing refusal. A journal-bearing operation whose replayed
quantity differs from what its journals posted is reconciled by an
**adjustment journal**, not by refusal and not by rewriting the original: the
difference posts under the triggering command's audit event, dated to the
operation, linked to it with a non-primary journal-link role, and recorded on
the revision that introduced the difference. The operation's primary plus
adjustment journals must equal its effective effects, and self-check verifies
that per operation. Splits are the first such operation (role
`split_adjustment`). The investment writer applies the reconciliation guard to
adjustment journals discovered during domain effects. Native correction
admission and the operation-for-transaction lookup continue to read only
`primary` links, so an adjustment journal cannot be corrected on its own.

## Pooled Transfer Lineage Refinement (2026-10-04, T-135)

The research and the options weighed are in
`docs/reviews/internal-transfer-lineage-research-2026-10-04.md`.

An average-cost source's per-lot split of a transfer is a FIFO lineage
convention, not a choice the user made. Storing that split as fixed fact made
any backdated buy into the source a refusal. The destination lineage is now an
explicit part of the transfer, `investment_transfer_facts.destination_lineage`:

- **Individual-lot source: `source_lots`, always.** The user chose the lots;
  they stay fixed facts, as before.
- **Average-cost source: `pooled_lot` by default.** The transfer opens **one**
  destination lot for the whole quantity, carrying the pool's exact basis. Its
  single link has no source lot (`source_lot_id` is null). Its original
  acquisition date is the **latest** original date among the units the pool's
  FIFO lineage moved, which never overstates how long the units were held; one
  unknown date makes it unknown. The source depletions stay the operation's
  `transfer_out` lot effects.
- **Average-cost source: `source_lots` on request.** One destination lot per
  depleted source lot with its own date, for holding history such as US
  average basis. A replay that changes which lots or quantities the pool
  depletes stays a named refusal; the remedy is to re-record the transfer as a
  pooled lot (transfer correction, T-119).

Under replay a `pooled_lot` transfer is one intent for its fixed quantity. The
destination lot's ID and quantity never change, so destination specific-lot
elections stay valid. When the replayed carried basis, original date or source
depletion set differs from the effective one, a link revision is appended with
a null source lot, the revised date (`original_date_knowledge`,
`original_acquired_on`) and its complete depletion set
(`investment_transfer_link_revision_depletions`). The destination then replays
from the effective link, and its FIFO/LIFO ordering reads the revised date.
That adds the date to the cross-position inputs of the T-132 fixed point; it
still flows only forward from source to destination, so cycles still settle.
Readers of a transferred lot's basis or original date use the view
`effective_investment_transfer_links`.

Self-check audits every committed and revised depletion set of a pooled lot
against the link's quantity and basis, and lot reconciliation uses the latest
revision's depletions. Bundle schema 8 exports the lineage, the revised date
and `investment-transfer-link-revision-depletions.csv` (ADR 0011).

Rejected options:

- **Re-deriving per-lot destination lots under replay.** Replay would have to
  retire and open destination lots, and a destination specific-lot election
  naming a retired lot would still be refused.
- **Pooling an individual-lot source.** An average-cost destination merges
  arriving lots anyway, and a lot-based destination would receive an invented
  lot.
- **Transfer-date or earliest-unit dating.** Transfer dating discards holding
  history. Earliest-unit dating overstates the holding period of the rest.

Moves into tax wrappers (ISA, TFSA/RRSP, pension) are usually a sale and a new
acquisition, not an internal transfer; the transfer form says so. Tax-exact
per-person pools (UK s104, Canada ACB, France PMP) remain R18 projections.

## Transfer Correction Refinement (2026-10-04, T-119)

A transfer is corrected by its own commands, never by the generic transaction
editor and never by sale or buy correction. Each direction has its own
contract; they share the correction fences of every other family (one
correction per operation, an imported lineage must still name its committed
source, the journal must not have moved since the plan, reconciled balances
need the override, changed gains need the exact acknowledgement) and one rule:
**the transfer's first journal, facts, links, link revisions and lot events
stay immutable evidence**. A correction appends an inverse journal and, for a
replacement, a new operation at the correction root's same-day slot; it never
rewrites them.

**Reversal (all directions).** The inverse journal negates every leg the
transfer posted, including the external equity bridge, on the transfer date.
The transfer leaves `effective_investment_operations`, so replay no longer
sees its depletions or the lots it opened. Every position it touched replays
in the command transaction, source first: the source (internal, outbound)
gets its units and carried basis back, and each destination lot (internal,
external-in) is retired by the replay (status closed, zero remaining; the lot
row stays). Replaying the destination explicitly is the seeding with *removed
edges* that the T-132 propagation needs; anything downstream propagates from
there. A destination disposal or a `source_lots` onward transfer that needed
the removed units is a named dependency (`INVESTMENT_TRANSFER_DEPENDENCY`)
and nothing is written; the remedy is to correct the dependent operation
first, so a chain unwinds from its end. A `pooled_lot` onward transfer is
revised from the remaining pool instead. Revisions of a reversed transfer stay
as evidence; self-check's effective lot events and lot reconciliation read
only revisions of effective transfers.

**Internal replacement.** Inverse plus a new internal transfer under one audit
event, replaying source and destination of both the old and the new transfer
together with propagation. Date, quantity or lot allocations, destination
account and destination lineage may change; re-recording a `source_lots`
transfer from an average-cost source as one `pooled_lot` is the remedy the
*Pooled Transfer Lineage Refinement* names. A new destination lot is a new lot
row: a destination specific-lot election or a `source_lots` onward transfer
follows it through the correction root only when it is the transfer's single
destination lot on the same date (the rule acquisition replacement already
uses); otherwise it is a named dependency. The replacement's own source
depletion is computed the way a backdated split computes its effects: the
source replays its effective history with the new transfer as the *subject*
intent at the replaced transfer's correction-root slot, and replay reports
the depletions instead of checking them. Those become the replacement's
`transfer_out` lot events, its destination lots open by replay admission, and
then every position either transfer touched replays from the committed facts.
Depleting today's lots instead would take units a later disposal already
consumed, or miss ones it has not.

**External-in replacement.** Inverse plus a new external transfer in.
Quantity, carried basis, original acquisition date, effective date and
holding account may change; the security and basis currency may not. The new
lot needs no subject intent: as an opening it already takes the correction
root's same-day slot, so it opens by replay admission and the old and new
holdings replay. The bridge difference is therefore the inverse bridge plus the new
bridge, appended under the correcting audit event; the original bridge journal
and source evidence are never overwritten. An unknown original date stays
unknown unless the replacement supplies one; carried basis must stay known
until unknown-basis resolution (#114) exists.

**Outbound.** No outbound writer exists yet. When #114 adds one, its reversal
restores the source lots the way an internal reversal does (no destination),
and its replacement follows the internal contract without a destination;
gains never arise from a transfer, so only later source disposals restate.

**Status.** Shipped 2026-10-04: reversal of internal and external-in
transfers (`POST /api/v1/investments/transactions/{id}/reverse-transfer` and
its preview), internal replacement (`.../replace-transfer` and its
`/preview`) and external-in replacement (`.../replace-transfer-in` and its
`/reconciliation-impact`). Outbound correction waits for the outbound writer
(#114).

## Return Of Capital Completion Refinement (2026-10-07, T-148)

A native replacement posts the exact inverse receipt and corrected receipt
under one audit event, and derives the replacement's basis effects by subject
replay at the correction root's same-day slot. Amount, effective/payment dates,
cash account, evidence and entitlement may change; holding, security and cost
currency stay fixed. Reconciliation and gain disclosure guard the combined
command, including every dependent replay. Originals remain audit evidence.

`explicit_quantities` fixes a positive quantity per named lot, no greater than
the units held at the action's slot. Receipt allocations are proportional to
those entitled units; each lot's reduction is capped to the entitled units'
proportional share of its remaining basis, truncated at the position allocation
scale. Unentitled basis cannot absorb unresolved excess. Replay follows a named
lot's corrected acquisition successor while retaining the fixed quantity.
Existing `explicit_lots` continues to mean the whole remaining quantity at the
slot; its historical quantity snapshot is not reinterpreted as a fixed election.

Allocations use the cost currency's maximum allocation scale, backed off only
for the int64 basis projection range, never below recorded basis precision or
the normalized receipt scale. Equivalent receipt spellings allocate identically.
Self-check audits each original and each revision effect set independently,
including superseded and missing sets, while projection checks use effective
history. Unknown-basis handling remains T-145; excess is unresolved, not income.

**BREAKING DEV DATABASE:** the pre-release baseline adds `explicit_quantities`
to the entitlement rule and its insert guard. Its checksum is updated. Reset
only disposable development databases as documented in developer-workflow;
no supported installed database exists. Export columns are unchanged.


## Cash In Lieu Completion Refinement (2026-10-07, T-150)

Cash-in-lieu reversal and replacement reuse native long-disposal corrections.
Replacement retains the original split link and its correction-root same-day
slot; it may change fraction, proceeds, dates and election. The split's holding
and security follow that fixed link. Explicit attempts to change the split are
refused. The split correction fence counts effective cash in lieu only, so a
native reversal releases it while retaining original facts as evidence.

Both entry and replacement previews include the typed split-link fact guard in
the simulated domain write. They never accept provider source evidence. Exact
per-lot allocation, disposed basis and signed realized result come from the same
writer used by commit, with checkpoint invalidation and gain disclosure simulated
under the same rollback boundary. A composed dated quantity read supplies
specific-lot choices at a new entry's slot or at the replacement's original root
slot with its predecessor excluded; today's closed state does not hide historical
lots. The read is never write authorization, and commits recheck all dependencies.

When a correction changes cost currency, gain review identifies each before/after
amount in its own currency instead of suggesting a common-currency comparison.

This completes manual known-basis entry/correction in six locales. It adds no
schema change and does not admit unknown basis or unverified provider mapping;
those retain their own open acceptance in T-145 and T-130.

## Immutable opening knowledge refinement (T-145, 2026-10-07)

Original lot-opening evidence and lot events now distinguish known basis from
unknown basis with an explicit code and paired nullable coefficient/scale.
Known zero remains known. The current projection's knowledge is separate:
rebuilding an unresolved opening must keep NULL amounts, while replay of known
original evidence can repair a damaged unknown projection.

Opening intents select a transfer link's amount, scale and knowledge as one
effective tuple; they do not independently coalesce NULLs back to the original
lot's numeric amount. Replay activation, its output and persistence preserve
knowledge. Range admission for an opening replay verifies the known subtotal;
existing known-basis command guards still refuse unresolved inputs.

The API names immutable knowledge `opening_basis_knowledge`, independent of
remaining `basis_knowledge`; original amounts are NULL when unknown. Bundle
schema 9 exports original/event knowledge and blank unknown amounts. Self-check
keeps quantity verification, verifies replay knowledge and names unresolved
basis rather than comparing it as zero. Public unknown transfer admission,
unknown disposal/pool snapshots and sourced resolution remain open in #160.
