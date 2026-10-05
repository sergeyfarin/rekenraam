# Investment operation slice 5: transfers and basis actions

Status: accepted implementation contract, 2026-09-28. ADR 0012 and ADR 0013
govern. This document fixes the journal, lot, date, and reconciliation rules
that each slice 5 command must satisfy. The known-basis external inbound API
command shipped in slice 5b, its entry screen in 5c, the explicit-lot
internal transfer API in 5d, and its entry screen in 5e. The manual split /
reverse split command shipped under T-122 #137 (see below). The known-basis
external outbound API shipped 2026-10-05 (*Outbound transfer, first command*
below). The remaining commands are unimplemented.

## Common rules

All amounts are exact coefficient and scale pairs. Positive journal postings
are debits. `H` is a security holding account, `T` is `commodity_trading`, `E`
is `external_investment_transfer_equity`, and `A` is the cash account. `q` is
a positive security quantity, `b` is nonnegative carried basis in one named
cost currency, and `r` is a nonnegative cash receipt. A blank posting means
there is no journal line. Every journal balances separately in each commodity.

The effective date determines lot eligibility and the position replay slot;
the account-entry date of an inbound transfer is its `opened_on`. Preserve an
original acquisition date separately when the source supplies it. If it is
missing, record an explicit unknown status; neither the current date nor the
entry date is a substitute. A position can mix lots with different basis
currencies, but each basis calculation and bridge is separately denominated.
No implicit FX conversion is permitted. A long transfer is not a short sale,
and none of these commands accepts or creates a short position.

The command, all journal versions, lot facts/effects, replay revisions,
bridge revisions, price changes if any, checkpoint invalidations, and one
audit event commit together. A preview runs the same proposed journal and
replay before writing. It names affected account, commodity, date, and active
checkpoint; the write rechecks the guard inside its transaction. A required
override invalidates that checkpoint and all later active checkpoints. A
basis-only replay can change reported gains without changing a reconciled
balance; it still refuses an impossible dependent disposal. A dated bridge
or cash/security posting is always subject to reconciliation review.

Amounts calculated from effective lot state are replay outputs, with a
version, causing operation, source evidence, and audit link. They are never
silently edited into immutable broker facts. The supported first command of
each family must either replay dependent later operations or refuse with the
named dependency before posting; it must never leave the journal and lot
projection at different historical states. Until unknown-basis replay exists,
manual entry requiring unknown basis fails with a named unresolved-basis
error and an imported row remains in review.

## In-kind transfers

| Operation | Security postings | Basis-currency postings | Lot effect |
| --- | --- | --- | --- |
| Internal source → destination | `H_source −q`, `H_dest +q` | None | Deplete selected source lots by `q` and `b`; open linked destination lots with the same `q` and `b` |
| External in, known basis | `H +q`, `T −q` | `T +b`, `E −b` | Open destination lot with known carried basis `b` |
| External out, known basis | `H −q`, `T +q` | `T −b`, `E +b` | Deplete selected source lots by `q` and replay-calculated `b` |
| External in/out, unknown basis | Same security legs as the known case | None | Mark basis unresolved; do not fabricate zero or report a definitive gain |
| Resolve unknown inbound/outbound basis | None | Full omitted bridge: inbound `T +b`, `E −b`; outbound `T −b`, `E +b`, dated to the transfer | Append sourced basis fact and replay affected lots and later disposals |

The source and destination of an internal transfer must be different active
holding accounts in the same book, with the same security and cost currency.
Select exact source-lot quantities. The first command uses explicit lot
selection so it does not silently choose a cost-basis policy for a non-sale.
The first internal command snapshots known carried basis per source lot. Replay
of later sales includes its source depletion. A correction that would change
that linked basis or make its source lot unavailable refuses with the named
transfer until cross-account transfer revisions can update the destination
and its dependent disposals atomically.
For a partial lot, carry its proportional remaining basis at the position's
allocation scale; truncate non-final allocations toward zero and assign the
exact remainder to the final selected portion. A full-lot movement carries
the entire remaining basis. Source and destination effects link one-to-one
and their quantity and basis sums must agree exactly. The destination lot's
account-entry date is the transfer date, while its original acquisition
date and source lineage are retained independently. An internal transfer has
no cash, gain, clearing, or equity posting.
For FIFO and LIFO, the original acquisition date controls priority when known;
otherwise the account-entry `opened_on` date does. `opened_on` alone controls
dated eligibility, so a lot cannot be consumed before it entered the book.
The same ordering applies in replay.
The selected-lot internal command resolves the source account's current
cost-basis method through the same account, book and fallback policy tiers as
a sale. It refuses average cost even before a first sale has created a method
family lock: taking the selected lot's individual basis would violate the pool
rate. It also refuses an existing average-cost lock. A completed lot-specific
move records `individual_lot` as the source position's method-family lock, so
a later sale cannot switch the still-open source position to average cost.
Closing the source position releases the lock. Preview and write return the
same named conflict until pool-aware transfer allocation and replay exist.

External inbound basis is a sourced fact: accept a known nonnegative value
including known zero, or record unknown with a NULL coefficient. An outbound
broker-reported basis is source evidence only; its operational bridge uses
the elected cost-basis method and effective remaining lots at the transfer
slot. For each cost currency, the selected source quantities must be fully
available on that date. An outbound transfer with unresolved source basis
posts only its security legs. A later resolution posts the full omitted
bridge, rather than a delta from invented zero. If replay changes a posted
outbound `b_old` to `b_new`, append an adjustment dated to that transfer:
`T −(b_new − b_old)`, `E +(b_new − b_old)`. Refuse a correction that would
turn a known outbound bridge into unknown basis. A bridge adjustment is
linked to the operation whose replay caused it; it does not rewrite the
original journal.

**Outbound transfer, first command (2026-10-05).**
`POST /api/v1/investments/transfers/external/out` (plus `/preview` and
`/reconciliation-impact`) allocates like an internal transfer: selected lots for
an individual-lot source, a dated pool quantity for an average-cost source,
under the same method-lock and policy snapshot (`basis_allocation`,
`cost_basis_method` and tier on the `external_out` fact). The primary journal
carries only `H −q`, `T +q`. Because `b` is an output of the depletion, which
writes lot events against the posted journal, the writer posts the basis
legs `T −b`, `E +b` as a separate bridge journal in the same audit event,
linked to the operation as `transfer_bridge` (system label
`transfer_bridge`, localized) and netted by the command's checkpoint guard.
A known zero basis posts no bridge. Each depleted source lot gets one link
with no destination. Replay includes the depletion. A history change that
changes `b` (T-143 #158, 2026-10-05) appends a link revision (the corrected
successor lot and new basis) and posts one dated `transfer_bridge`
adjustment per transfer and cost currency, `T −(b_new − b_old)`,
`E +(b_new − b_old)`, under the causing command's audit event and checkpoint
guard; the first bridge and link stay immutable evidence. A change in which
source lots a pooled outbound takes is still refused with the transfer named:
its links are fixed per source lot. An outbound transfer dated behind a later
depletion is admitted through replay at its own slot (as a backdated sale
is), revising later decisions under the gain acknowledgement, or refusing
with the impossible decision named (`INVESTMENT_SALE_DEPENDENCY`).
Self-check verifies the links against the depletions, that the bridge plus
its adjustments post exactly the effective links' carried basis per cost
currency, and replays revised outbound depletions. Mobile entry shipped as
T-142 #157. Correction (T-144 #159): `reverse-transfer` posts one inverse
journal that cancels the security legs and the net bridge (first bridge plus
adjustments, pinned in the correction record so an adjustment landing after
planning refuses the write) and replays the source; `replace-transfer-out`
posts that inverse and a corrected outbound transfer whose depletion replays
at the replaced transfer's slot, replaying every source either depleted.
Still open: unknown basis (T-145 #160).

Example: transfer 2 shares carrying 80.00 EUR into the book. Post security
`H +2`, `T −2` and EUR `T +80.00`, `E −80.00`. A later full sale for 100.00
EUR posts EUR `T −100.00`; the closed clearing balance is −20.00, reflecting
a 20.00 operational gain. Transfer the same 2 shares back out instead:
security `H −2`, `T +2` and EUR `T −80.00`, `E +80.00`; neither movement is a
sale or dividend.

## Return of capital

Post `A +r`, `T −r` in the actual receipt currency on the cash payment date.
Record an effective-date basis action for the entitled lots; do not post a
fictitious expense, dividend, or gain. Entitlement is an explicit set of
lots/quantities or a documented per-share rule from the corporate action.
It is not universally inferred from the cash payment or record date. For an
allocation `r_i`, reduce each known remaining basis by `min(r_i, basis_i)`.
The excess `r_i − reduction_i` is a replay-derived unresolved amount, never
negative lot basis or automatic dividend income. Allocation uses exact
per-share ratios, a stated allocation scale, stable lot order, and a final
remainder, so allocated cash sums to `r`. A different receipt and effective
date requires separately linked journal and basis dates; the payment must
never be backdated merely to simplify replay. An unknown-basis eligible lot
cannot yield a definitive reduction/excess split and remains unresolved.

**First command (2026-10-05, T-146 #161).** `POST
/api/v1/investments/return-of-capital` (plus `/preview`) applies the
documented per-share rule (`entitlement_rule` `open_lots_per_share`): every
long lot of the holding open on the effective date, in stable lot order, with
truncated allocations and the exact remainder on the last lot. The receipt
currency must be the position's cost currency. Facts and per-lot effects
(entitled quantity, allocated, reduction, excess) are immutable, each with a
`basis_reduction` lot event (quantity 0, basis −reduction) linked to the
operation. Replay recomputes the effects at the effective-date slot; a
history change that alters them appends a revision of the whole effect set
(T-148 #163: `investment_capital_return_revisions` and `-revision_effects`,
the original basis_reduction events then leave the effective lot events), and
a slot with no entitled lot is refused with the operation named. A return
dated behind a later depletion is admitted through replay at its own slot. An
unknown-basis lot is refused.
Self-check verifies facts, events and conservation (allocations sum to the
receipt, reduction + excess = allocation, event = −reduction); bundle files
`investment-capital-return-facts.csv` and `-effects.csv`. Still open (T-148
#163): explicit lot entitlement, correction/reversal and entry UI.

Example: 10.00 EUR return of capital on one lot with 7.00 EUR remaining
basis reduces basis to zero and records 3.00 EUR unresolved excess. The cash
journal still posts the full 10.00 EUR. Later source correction of an earlier
purchase replays the 7.00/3.00 split without changing the source receipt.

## Split and reverse split

The sourced ratio is a positive integer numerator and denominator in lowest
terms, neither zero. For each eligible lot at the effective date, derive the
new quantity as `old_quantity × numerator / denominator` with exact integer
arithmetic. Require representability within the security's permitted scale;
never round or discard a fraction. The split journal posts the aggregate
signed quantity delta `d = new_total − old_total` as `H +d`, `T −d` in the
security commodity. Every lot retains its entire basis; its per-share basis
changes, its aggregate basis does not. A ratio `> 1` is a forward split;
`< 1` is a reverse split. Zero-delta events still preserve the sourced ratio
and dated lot effects but create no zero-quantity journal posting.

The effective date and eligible lots are explicit source facts. Same-day
ordering is fixed to the operation's replay slot. A correction or backdated
acquisition that changes entitlement or quantity replays the split and every
dependent disposal; it refuses an impossible later disposal atomically. A
broker that settles only whole shares requires a separate, linked cash-in-lieu
disposal for the fraction. The split itself does not invent sale cash.

Example: a 3-for-2 split of 5 shares changes the holding to 7.5. Post
`H +2.5`, `T −2.5`; a 100.00 EUR aggregate basis remains 100.00 EUR. If the
broker pays cash for 0.5 share, post the separate cash-in-lieu event below.

**First command (T-122 #137).** `POST /api/v1/investments/splits` (and
`/preview`) takes one holding account, security, effective date and ratio; a
ratio is reduced to lowest terms and 1:1 is refused. Eligibility is derived,
not listed: every long lot open at the operation's slot with `opened_on` on or
before the effective date, across all cost currencies. Each lot's new quantity
widens scale only as needed up to the security's `max_quantity_scale` (capped
by commodity kind); anything finer is refused with
`INVESTMENT_SPLIT_FRACTION_UNREPRESENTABLE`, never rounded. Original per-lot
effects are `split_adjustment` lot events with signed quantity deltas and zero
basis, linked to the operation; `investment_split_facts` holds the sourced
terms. The service plans the delta before the write; the writer recomputes it
inside its transaction and refuses a mismatch (`INVESTMENT_SPLIT_CHANGED`).
A split dated before a later depletion is admitted through position replay
under the T-114 gain-impact policy; an impossible later disposal is refused
with it named (`INVESTMENT_SPLIT_DEPENDENCY`). Replay of a later correction
re-derives each split's per-lot effects and appends a per-cost-currency
revision only when they change. Basis-only corrections replay through it.

**Journal-delta adjustment (T-129 #144).** History that changes the quantity
a split multiplied — a quantity correction, backdated acquisition, or a buy or
sale reversal dated before it — no longer refuses. When a replayed split
revision's effects move a different aggregate quantity than the current
effective effects (per cost currency), the difference `e` posts as an
adjustment journal dated to the split, `H +e`, `T −e` in the security, under
the triggering command's audit event. It is linked to the split operation
(journal-link role `split_adjustment`) and to the revision
(`investment_split_revisions.adjustment_transaction_version_id`); the revision
records the triggering operation and supersedes the previous revision. A
split whose remaining eligible lots disappear keeps its facts and is fully
offset by its adjustment. The investment writer applies the reconciliation
guard, override and preview reporting to adjustment journals after the domain
effects, at the split's date. Gain disclosure follows the triggering command's
T-114 policy. Self-check requires each split's `primary` plus
`split_adjustment` journals to equal its effective effects, and every
adjustment link to belong to a revision dated to the split. A fraction the
security cannot represent still refuses with the split named.

**Reversal and replacement (T-129 #144).** A split is corrected like a trade:
reversal posts one inverse journal dated to the split that negates its primary
plus adjustment journals on the holding, as a `reversal` operation correcting
the split; replacement posts that inverse (linked to the successor as
`reversal`) and a new split operation (`correction_mode = 'replace'`) with
corrected date, ratio or evidence for the same holding and security, planned
without the replaced split and ordered at its correction-root same-day slot.
Both replay every cost currency of the holding in the command transaction,
refuse an impossible later disposal with it named, and use the shared
correction fences, gain acknowledgement and checkpoint guard. A corrected
split's facts, effects, revisions and journals stay immutable; self-check
includes its inverse journal and expects no effects from a split that is no
longer effective. Deferred: zero-delta splits (no eligible holdings, refused
with `INVESTMENT_SPLIT_NO_HOLDINGS` because a journal-free operation path does
not exist; [T-131 #146](https://github.com/sergeyfarin/rekenraam/issues/146)), verified Trading 212 mapping ([T-130 #145](https://github.com/sergeyfarin/rekenraam/issues/145)), and linked
cash in lieu.

## Cash in lieu

Record a linked long disposal for the exact fractional quantity `f` on its
economic disposal date. Post security `H −f`, `T +f`; post the actual
cash settlement `A +p`, `T −p` in its currency on the payment date. If those
dates differ, use separate dated journal entries under the operation. The
fractional quantity and sourced cash amount are immutable; the disposed
basis and realized result come from replay at the disposal slot. Use the
ordinary long-disposal election and allocation rules, with explicit lot
selection when the corporate action identifies the affected fraction.
Unknown basis leaves gain unresolved. Do not treat the cash as a dividend.

**First command (2026-10-05, T-147 #162).** `POST
/api/v1/investments/cash-in-lieu` (plus `/preview`) runs the ordinary long
disposal writer with operation kind `cash_in_lieu`: the fraction (below one
share) is disposed under the holding's election with the cash as proceeds,
the security legs on `disposal_on` and the cash on `payment_on` as separate
dated entries of one transaction. `investment_cash_in_lieu_facts` links it to
the split it settles, written inside the same transaction where a trigger
rechecks that the split is still effective and matches the holding; a split
correction is then refused with the cash in lieu named. Replay revises its
disposed basis like any disposal decision. Correction of a cash in lieu
itself and entry UI follow (T-150 #165).

For the 3-for-2 example above, suppose 0.5 share receives 8.00 EUR. The
linked cash-in-lieu journal posts security `H −0.5`, `T +0.5` and EUR
`A +8.00`, `T −8.00`. The fraction's disposed basis is allocated from the
post-split lots at the position's basis scale, and a later correction revises
that allocation rather than rewriting the 8.00 EUR receipt.

## Delivery gates

1. Add each command's typed source and lot links before exposing that command.
   Slice 5b added transfer endpoints, original-date knowledge, nullable
   **source** basis knowledge, a known-basis inbound lot, replay opening,
   self-check and export. The projection now accepts explicit unknown knowledge
   with NULL remaining basis/scale, and read APIs, gains, UI and bundle schema 5
   preserve it. Known zero stays numeric. Current writers refuse unresolved
   positions; replay reconstructs known state from the currently known immutable
   opening evidence and rolls back knowledge/NULLs atomically. Unknown-basis
   transfers remain refused until immutable opening/event/disposal knowledge,
   unknown-basis replay and sourced resolution propagate that state end to end.
   Split ratio/eligibility and basis-action links
   are likewise prerequisites for their respective commands.
2. Known-basis external inbound and explicit-lot internal transfers are shipped.
   Manual split/reverse split with Trading 212 split-row linking shipped
   ([T-122 #137](https://github.com/sergeyfarin/rekenraam/issues/137)), as did pooled average-cost internal transfer allocation
   ([T-123 #138](https://github.com/sergeyfarin/rekenraam/issues/138)), and broader transfer-in/disposal backdating ([T-117 #132](https://github.com/sergeyfarin/rekenraam/issues/132)).
3. Decide cross-position replay ([T-124 #139](https://github.com/sergeyfarin/rekenraam/issues/139)) before outbound transfers,
   then deliver unknown-basis resolution, return of capital and linked cash in
   lieu. Prove bridge adjustment/refusal, exact conservation, dependent replay
   and reconciliation with named tests. Provider suggestions stay in review
   until their command and mapping are supported.
4. Add mobile entry and read-side labels for unresolved basis and action
   provenance before calling the family complete. Update the feature ledger
   for each shipped command, not merely for the schema.
