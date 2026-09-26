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
   and scales.
   Operation kinds are stable, non-empty codes, validated by their command;
   the database does not need a new migration for each new code. They are not
   reconstructed from transaction descriptions or negative quantities.
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
6. Cash corporate actions are typed. Return of capital changes lot basis;
   cash in lieu needs its own lot allocation or disposal relationship. Neither
   is accepted as ordinary dividend income solely because cash arrived. A
   split changes quantity but conserves total basis; a transfer moves lot
   identity and basis between holding accounts without realizing a gain. An
   external in-kind transfer with known carried basis also needs a balanced
   basis-currency bridge between `commodity_trading` and explicit book-boundary
   equity, so closing the transferred position does not leave all sale proceeds
   in clearing. Unknown carried basis remains an explicit unresolved state;
   clearing cannot be called gain until a sourced basis-resolution operation
   supplies the bridge and replays the position.
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
