# Internal Transfer Lineage Research (2026-10-04)

Point-in-time research behind the pooled-lineage decision for
[T-135 #150](https://github.com/sergeyfarin/rekenraam/issues/150). The decision
itself is recorded in ADR 0013 (*Pooled Transfer Lineage Refinement*); this file
keeps the evidence and the options that were weighed.

## Problem

An average-cost source moves units out of its dated pool. Until T-135 each
transfer opened one destination lot **per source lot** the pool's FIFO lineage
depleted, and stored that per-lot split as immutable fact. A backdated buy into
the source changes which lots FIFO depletes, so replay could no longer
reproduce the split and refused the backdated buy with the transfer named
(`TestBackdatedBuyRefusesChangedPooledTransferLineageWithoutWriting`).

The refusal is not about pooling. It is about deriving per-lot detail from a
pool by a FIFO convention and then treating the convention as permanent fact.
An individual-lot source has no such problem: its lots are the user's choice
and stay stable under replay.

## When internal transfers happen

| Situation | Tax treatment | Consequence for the book |
|---|---|---|
| Broker switch (US ACATS, DE Depotübertrag, UK in-specie) | Not a disposal. Lot regimes require per-lot acquisition data to travel: US §1.6045A-1 transfer statements (adjusted basis, original acquisition date); DE §43a(2) EStG (acquisition data, holding periods, FIFO order). | Per-lot detail is real evidence when the source keeps lots. |
| Restructuring at one broker (sub-accounts, currency sub-accounts) | Same as a broker switch. | Same. |
| Average-cost regimes (UK s104, CA ACB, FR PMP) | The pool belongs to the **person across all accounts**. A move between one's own accounts changes nothing. | Per-account pools are an operational approximation; the tax-exact pool is an R18 projection (ADR 0012). Dates inside the pool carry no tax meaning. |
| US average basis (mutual funds) | Average basis; holding period is FIFO-deemed. Transfer statements still carry dates; acquisitions older than five years may be reported as `VARIOUS`. | The one case where an average-cost source's per-lot dates carry legal meaning. |
| Into a tax wrapper (ISA, TFSA/RRSP, pension) | Usually a disposal at market value plus a new acquisition. | Not a basis-carrying internal transfer: record a sale and a buy. |
| Gift, inheritance, change of owner | Different owner. | Out of the book (external transfer). |

## How other products handle it

| Product | Behaviour |
|---|---|
| Quicken | "Shares transferred between accounts": one remove, one add **per lot**; partial moves pick first-in shares or specific lots. Users report acquisition dates are not always kept. |
| Wealthfolio | Paired `TRANSFER_OUT`/`TRANSFER_IN` keep each lot's original date and cost; a standalone `TRANSFER_IN` creates one lot at a supplied (often average) basis. |
| IBKR (incoming transfer basis) | Per-lot entry: quantity, acquisition date, total cost per lot. |
| Portfolio Performance | Weak: users report changed FIFO cost and lost dates after an *Umbuchung*. |
| Sharesight | Holdings are per portfolio, not per broker, so a broker switch needs no entry; moving between portfolios is "sell at cost base, buy at cost base". |
| Beancount | Per-lot by default (`{}` lot specs); `AVERAGE` booking merges lots, and users write transfers at a manually computed average. |
| ERP inventory (SAP moving average) | A stock transport moves value at the sending plant's moving average price on that day; the receiving plant re-averages. No acquisition dates. |

## Options considered for an average-cost source

| Option | Detail kept | Stable under backdating | Cost |
|---|---|---|---|
| A. Per-source-lot links (pre-T-135 behaviour) | One date per FIFO lineage lot | No. Admitting it needs replay to retire and reopen destination lots; a destination specific-lot election naming a retired lot must be refused. | High |
| B1. One pooled destination lot, **latest unit's** original date | Pool-average basis plus a date that never overstates the holding period | Yes, with the date revisable by replay | Medium |
| B2. One pooled destination lot, earliest unit's date | Pool-average basis plus the oldest date (overstates holding of the rest) | Yes, with the date revisable | Medium |
| B3. One pooled destination lot, transfer date | Pool-average basis on the day, no history (ERP / Sharesight model) | Fully: only carried basis changes | Lowest |

The carried basis is identical in every pooled option: the pool rate on the
transfer date, conserved exactly by the shared pool depletion.

Per-lot pooling of an **individual-lot** source was rejected: an average-cost
destination merges arriving lots into its pool anyway, and an individual-lot
destination would receive an invented lot.

## Decision (accepted by the owner 2026-10-04)

1. An individual-lot source keeps per-lot links (unchanged; already stable).
2. An average-cost source chooses its destination lineage per transfer:
   - **`pooled_lot` (default):** one destination lot carrying the pooled basis
     and the latest original acquisition date among the units the pool's FIFO
     lineage moved (B1). Replay may revise the carried basis, that date and the
     source depletion set; the destination lot's identity and quantity never
     change, so destination elections stay valid.
   - **`source_lots` (opt-in):** the pre-T-135 per-lot links, for holding
     history such as US average basis. A replay that changes which lots or
     quantities the pool depletes stays a named refusal; the remedy is to
     re-record the transfer as pooled (transfer correction, T-119).
3. The transfer form tells the user that moving into a tax wrapper is usually a
   sale and a buy, not an internal transfer.
4. Tax-exact per-person pools stay in R18.

## Sources

- [26 CFR § 1.6045A-1 — transfer statements](https://www.law.cornell.edu/cfr/text/26/1.6045A-1)
- [IRS Publication 551 — Basis of Assets](https://www.irs.gov/publications/p551)
- [§ 43a EStG](https://www.juraforum.de/gesetze/estg/43a-bemessung-der-kapitalertragsteuer)
- [Clearstream — Depotübertrag FAQ](https://www.clearstream.com/caas/v1/media/1314920/data/1e7b4d7198a75fc1359fac2b1da2e39d/ti03-frage-antwort-katalog-de.pdf)
- [CGT across multiple UK brokers](https://taxbull.co.uk/blog/capital-gains-tax-multiple-brokers-uk/)
- [finiki — Adjusted cost base](https://www.finiki.org/wiki/Adjusted_cost_base)
- [TaxTips.ca — ACB](https://www.taxtips.ca/glossary/adjusted-cost-base.htm)
- [Quicken — How do I transfer shares?](https://help.quicken.com/pages/viewpage.action?pageId=3216602)
- [Wealthfolio — Cost basis & lots](https://wealthfolio.app/docs/concepts/cost-basis-and-lots/)
- [IBKR — Position transfer basis](https://www.ibkrguides.com/brokerportal/performanceandstatements/positiontransfer.htm)
- [Portfolio Performance forum — FIFO after Umbuchung](https://forum.portfolio-performance.info/t/veranderter-einstandspreis-fifo-bei-umbuchung/23163)
- [Sharesight — broker transfers](https://help.sharesight.com/how-to-record-share-buybacks-share-transfers-and-broker-transfers-in/)
- [Sharesight — transfer between portfolios](https://help.sharesight.com/nz/how-to-record-share-transfer-between-portfolios/)
- [Beancount — transferring lots](https://groups.google.com/g/beancount/c/C8dpE9M61Qo)
- [Beancount — AVERAGE booking PR](https://github.com/beancount/beancount/pull/591)
- [SAP — stock transport order](https://help.sap.com/docs/SAP_ERP/b704a8db767040a08100adc846218964/5213b953495bb44ce10000000a174cb4.html)
