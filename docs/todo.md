# TODO — short-horizon working queue

This is the next-action view. The [roadmap](roadmap.md) owns sequence and the
[investment operation plan](plans/investment-operation-refactor-plan.md) owns
R16 acceptance criteria. [GitHub Issues](https://github.com/sergeyfarin/rekenraam/issues)
tracks actionable work; the [backlog](backlog.md) maps local IDs, and
[implemented](implemented.md) records shipped behavior.

Last reconciled: 2026-10-03.

Done: [T-114 #129](https://github.com/sergeyfarin/rekenraam/issues/129) shared gain-impact mechanism and
[T-126 #141](https://github.com/sergeyfarin/rekenraam/issues/141) rollout to every existing replay path (see
[implemented](implemented.md) for the coverage matrix).

## Done 2026-10-03: replay gaps

Manual splits [T-122 #137](https://github.com/sergeyfarin/rekenraam/issues/137), pooled internal transfers
[T-123 #138](https://github.com/sergeyfarin/rekenraam/issues/138), broader backdating [T-117 #132](https://github.com/sergeyfarin/rekenraam/issues/132), the
cross-position replay decision [T-124 #139](https://github.com/sergeyfarin/rekenraam/issues/139) and transfer-basis
propagation [T-132 #147](https://github.com/sergeyfarin/rekenraam/issues/147) ship. See [implemented](implemented.md).

## Now

1. [ ] Correct or reverse posted splits and adjust their journal delta
   [T-129 #144](https://github.com/sergeyfarin/rekenraam/issues/144) (P2). Today a mistaken split, or a quantity
   correction, backdated buy or reversal before a split, is refused with no
   recovery path. Integrate #129 gain acknowledgement and the reconciliation
   preview.
2. [ ] Alongside: add API and browser evidence for import gain review and Trading 212
   source revisions [T-128 #143](https://github.com/sergeyfarin/rekenraam/issues/143) (P2). The development-only
   provider base-URL override and e2e stub also give later Trading 212 work
   (#136, #145) a test harness.

Follow-ups, not gates: zero-delta splits [T-131 #146](https://github.com/sergeyfarin/rekenraam/issues/146) after #144
(reuses its adjustment rule); verified provider split mapping
[T-130 #145](https://github.com/sergeyfarin/rekenraam/issues/145) stays blocked on evidence.

## Then, within R16 (P2 correction families)

- [ ] Before transfer correction #134: decide pooled-lineage re-derivation
  [T-135 #150](https://github.com/sergeyfarin/rekenraam/issues/150) and add the full replay-equivalence self-check
  [T-134 #149](https://github.com/sergeyfarin/rekenraam/issues/149) (both P3, sequenced here deliberately).
- [ ] Deliver dividend/reinvestment correction [T-115 #130](https://github.com/sergeyfarin/rekenraam/issues/130), trade field
  correction [T-116 #131](https://github.com/sergeyfarin/rekenraam/issues/131), write-off correction [T-118 #133](https://github.com/sergeyfarin/rekenraam/issues/133), transfer
  correction [T-119 #134](https://github.com/sergeyfarin/rekenraam/issues/134), and correction grouping/net checkpoint impact
  [T-120 #135](https://github.com/sergeyfarin/rekenraam/issues/135), one bounded family at a time. #135 also tracks the verified
  same-day sequence over-invalidation; #142’s complete preview set does not fix it.
- [ ] Complete [#114 transfers and basis actions](https://github.com/sergeyfarin/rekenraam/issues/114):
  outbound/unknown transfers, sourced basis resolution, return of capital and
  linked cash in lieu. The [slice 5 contract](plans/investment-operation-slice-5-contract.md)
  governs postings, dates, reconciliation and basis knowledge.
- [ ] Add [named short sale/cover T-108 #103](https://github.com/sergeyfarin/rekenraam/issues/103),
  then [compound corporate actions #115](https://github.com/sergeyfarin/rekenraam/issues/115).

#99 long-buy/sale correction, #125 integrity and #142 shared checkpoint
preview selection are complete within their bounded scopes. History belongs
in [implemented](implemented.md) and dated reviews. Provider execution cancellation [T-121 #136](https://github.com/sergeyfarin/rekenraam/issues/136) remains blocked on
verified evidence; cancelled order status is insufficient. Buy and reinvestment
previews now replay and return the writer’s actual checkpoint set, and every existing replay path discloses revised gains.

## Parallel trust work

- [ ] Close [G-08 locale-aware amount input](https://github.com/sergeyfarin/rekenraam/issues/100)
  and [T-87 owner-local financial date](https://github.com/sergeyfarin/rekenraam/issues/101)
  defaults before a multilingual migration demo.
- [ ] Complete [T-80](https://github.com/sergeyfarin/rekenraam/issues/102) catalog parity, add validation, and arrange native
  terminology review before calling the multilingual surface complete.

- [ ] Before the v0.1.0 tag: make every lot's opening operation `NOT NULL`
  [T-133 #148](https://github.com/sergeyfarin/rekenraam/issues/148). This is a baseline change under the migration policy.
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
