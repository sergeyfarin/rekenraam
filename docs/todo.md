# TODO — short-horizon working queue

This is the next-action view. The [roadmap](roadmap.md) owns sequence and the
[investment operation plan](plans/investment-operation-refactor-plan.md) owns
R16 acceptance criteria. [GitHub Issues](https://github.com/sergeyfarin/rekenraam/issues)
tracks actionable work; the [backlog](backlog.md) maps local IDs, and
[implemented](implemented.md) records shipped behavior.

Last reconciled: 2026-09-27.

## Current: R16 correction

- [ ] Finish slice 4: add the investment-native
  correction/reversal command
  ([T-75b #99](https://github.com/sergeyfarin/rekenraam/issues/99)). A posted
  reversal and replacement must update journal, effective lots and gains,
  prices, audit links, and reconciliation impact together. Keep backdated
  writes fenced until the dependency and rollback tests pass.
- [ ] Validate a corrected old buy followed by dependent sells under each cost
  basis method, including failure rollback and original versus effective
  allocations. The immutable intent reader, reversible replay simulation,
  revision storage, gains and self-check readers, revision-chain export, and
  correction-aware current reads are already complete (slices 4a–4g).
  Manual long-sale pure reversal is complete in slice 4h, including its API,
  replay, price retirement and reconciliation preview. Slice 4i adds the
  correction-chain read API; slice 4j presents the history and manual reversal
  in transaction detail. Next are replacement and buy correction; imported
  fills still require source-aware identity handling.

## Then, within R16

- [ ] Specify and deliver
  [in-kind transfers and basis actions](https://github.com/sergeyfarin/rekenraam/issues/114)
  (return of capital, manual splits, cash in lieu), one validated operation at a time.
  Keep basis-affecting provider suggestions in review until their operation
  exists.
- [ ] Add [named short sale and cover T-108](https://github.com/sergeyfarin/rekenraam/issues/103)
  with side-aware gains, dated positions, self-check, export, API, and mobile
  entry. An ordinary negative
  holding stays flagged as unclassified until corrected or explicitly entered
  as a short.
- [ ] Specify [compound corporate actions](https://github.com/sergeyfarin/rekenraam/issues/115)
  after the common operations work.

## Parallel trust work

- [ ] Close [G-08 locale-aware amount input](https://github.com/sergeyfarin/rekenraam/issues/100)
  and [T-87 owner-local financial date](https://github.com/sergeyfarin/rekenraam/issues/101)
  defaults before a multilingual migration demo.
- [ ] Fill [T-80](https://github.com/sergeyfarin/rekenraam/issues/102)'s 65
  missing keys per non-English locale, add catalog parity validation, and
  arrange native review before calling the catalogs complete.

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
