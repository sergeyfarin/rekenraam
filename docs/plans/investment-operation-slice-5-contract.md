# Investment operation slice 5: transfers and basis actions

Status: accepted implementation contract, 2026-09-28. ADR 0012 and ADR 0013
govern. This document fixes the journal, lot, date, and reconciliation rules
that each slice 5 command must satisfy. It does not claim a command has shipped.

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
For a partial lot, carry its proportional remaining basis at the position's
allocation scale; truncate non-final allocations toward zero and assign the
exact remainder to the final selected portion. A full-lot movement carries
the entire remaining basis. Source and destination effects link one-to-one
and their quantity and basis sums must agree exactly. The destination lot's
account-entry date is the transfer date, while its original acquisition
date and source lineage are retained independently. An internal transfer has
no cash, gain, clearing, or equity posting.

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

For the 3-for-2 example above, suppose 0.5 share receives 8.00 EUR. The
linked cash-in-lieu journal posts security `H −0.5`, `T +0.5` and EUR
`A +8.00`, `T −8.00`. The fraction's disposed basis is allocated from the
post-split lots at the position's basis scale, and a later correction revises
that allocation rather than rewriting the 8.00 EUR receipt.

## Delivery gates

1. Add typed transfer source/destination, original-date knowledge, nullable
   basis knowledge, split ratio/eligibility, and basis-action links to the
   schema. Expand replay intents and self-check before allowing writes that
   use them. Export the immutable source and every effective revision.
2. Ship manual known-basis external inbound transfer, then explicit-lot
   internal transfer, outbound transfer and unknown-basis resolution. Prove
   bridge change/refusal and reconciliation behavior with named tests.
3. Ship return of capital, split/reverse split, then linked cash in lieu.
   Prove exact conservation and dependent replay with named tests. Provider
   suggestions remain in review until the matching manual command and import
   mapping are complete.
4. Add mobile entry and read-side labels for unresolved basis and action
   provenance before calling the family complete. Update the feature ledger
   for each shipped command, not merely for the schema.
