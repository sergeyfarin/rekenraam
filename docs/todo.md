# TODO — short-horizon working queue

This is the next-action view. The [roadmap](roadmap.md) owns sequence and the
[investment operation plan](plans/investment-operation-refactor-plan.md) owns
R16 acceptance criteria. [GitHub Issues](https://github.com/sergeyfarin/rekenraam/issues)
tracks actionable work; the [backlog](backlog.md) maps local IDs, and
[implemented](implemented.md) records shipped behavior.

Last reconciled: 2026-10-02.

## Current: R16 gain-impact safety

- [ ] Deliver replay gain restatement preview and explicit acknowledgement
  [T-114 #129](https://github.com/sergeyfarin/rekenraam/issues/129); cover manual and imported replay, stale preview and rollback.
- [ ] Deliver manual split/reverse split and verified Trading 212 review mapping
  [T-122 #137](https://github.com/sergeyfarin/rekenraam/issues/137) using the slice 5 exact-conservation contract.
- [ ] Support pooled average-cost internal transfers [T-123 #138](https://github.com/sergeyfarin/rekenraam/issues/138).
- [ ] Extend backdating to transfer-in and dependent disposals
  [T-117 #132](https://github.com/sergeyfarin/rekenraam/issues/132); preserve recorded methods and original acquisition dates.
- [ ] Decide cross-position replay and consolidate effective readers
  [T-124 #139](https://github.com/sergeyfarin/rekenraam/issues/139) before outbound transfers and compound actions.

## Then, within R16

- [ ] Deliver dividend/reinvestment correction [T-115 #130](https://github.com/sergeyfarin/rekenraam/issues/130), trade field
  correction [T-116 #131](https://github.com/sergeyfarin/rekenraam/issues/131), write-off correction [T-118 #133](https://github.com/sergeyfarin/rekenraam/issues/133), transfer
  correction [T-119 #134](https://github.com/sergeyfarin/rekenraam/issues/134), and correction grouping/net checkpoint impact
  [T-120 #135](https://github.com/sergeyfarin/rekenraam/issues/135), one bounded family at a time.
- [ ] Complete [#114 transfers and basis actions](https://github.com/sergeyfarin/rekenraam/issues/114):
  outbound/unknown transfers, sourced basis resolution, return of capital and
  linked cash in lieu. The [slice 5 contract](plans/investment-operation-slice-5-contract.md)
  governs postings, dates, reconciliation and basis knowledge.
- [ ] Add [named short sale/cover T-108 #103](https://github.com/sergeyfarin/rekenraam/issues/103),
  then [compound corporate actions #115](https://github.com/sergeyfarin/rekenraam/issues/115).

#99 long-buy/sale correction and #125 integrity are complete within their
bounded scopes. History belongs in [implemented](implemented.md) and dated
reviews. Provider execution cancellation [T-121 #136](https://github.com/sergeyfarin/rekenraam/issues/136) remains blocked on
verified evidence; cancelled order status is insufficient. The plain-buy
preview replay fix is shipped by this review; gain disclosure is still open.

## Parallel trust work

- [ ] Close [G-08 locale-aware amount input](https://github.com/sergeyfarin/rekenraam/issues/100)
  and [T-87 owner-local financial date](https://github.com/sergeyfarin/rekenraam/issues/101)
  defaults before a multilingual migration demo.
- [ ] Fill [T-80](https://github.com/sergeyfarin/rekenraam/issues/102)'s 65
  missing keys per non-English locale, add catalog parity validation, and
  arrange native review before calling the catalogs complete.

- [ ] Measure and shorten the backend race gate [T-125 #140](https://github.com/sergeyfarin/rekenraam/issues/140) without reducing
  meaningful coverage.

## Following R16

- [ ] Promote [R11 price/FX management UI](https://github.com/sergeyfarin/rekenraam/issues/116),
  then [R17 quotes and crypto](https://github.com/sergeyfarin/rekenraam/issues/117),
  [R18 reproducible gains](https://github.com/sergeyfarin/rekenraam/issues/118),
  and [R13 returns analytics](https://github.com/sergeyfarin/rekenraam/issues/119).
  The [roadmap](roadmap.md) governs their scope and order.
- [ ] Schedule remaining [GitHub Issues](https://github.com/sergeyfarin/rekenraam/issues)
  by priority and dependency.
  [T-34](https://github.com/sergeyfarin/rekenraam/issues/111) waits for R15;
  [T-48](https://github.com/sergeyfarin/rekenraam/issues/112) waits for
  upstream TypeScript 7 support. Decide whether displayed FX rates round or
  truncate when [G-09](https://github.com/sergeyfarin/rekenraam/issues/106)
  is selected.
