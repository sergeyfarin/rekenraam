# TODO — short-horizon working queue

This is the next-action view. The [roadmap](roadmap.md) owns sequence and the
[investment operation plan](plans/investment-operation-refactor-plan.md) owns
R16 acceptance criteria. [GitHub Issues](https://github.com/sergeyfarin/rekenraam/issues)
tracks actionable work; the [backlog](backlog.md) maps local IDs, and
[implemented](implemented.md) records shipped behavior.

Last reconciled: 2026-09-29.

## Current: R16 correction

- [ ] Finish slice 4: add the investment-native
  correction/reversal command
  ([T-75b #99](https://github.com/sergeyfarin/rekenraam/issues/99)). A posted
  reversal and replacement must update journal, effective lots and gains,
  prices, audit links, and reconciliation impact together. Keep backdated
  writes fenced until the dependency and rollback tests pass.
- [x] Validate old manual buy and sale corrections through dependent disposals
  under all four methods, with rollback and effective allocation revisions.
  See the [feature ledger](implemented.md)
  and [slice 4 plan](plans/investment-operation-refactor-plan.md) for the
  shipped sub-slices. Imported source corrections, backdated admission, buy
  reversal, and other operation corrections remain open.
- [ ] Close the reopened slice 2a integrity gates: link component cash facts
  to posting versions, reconcile decision proceeds with clearing in self-check,
  migrate operation reads to journal links, separate lot projection state,
  and share correction transaction orchestration. The disposal-decision key
  now supports `(operation_id, decision_seq)`.

## Then, within R16

- [ ] Specify and deliver
  [in-kind transfers and basis actions](https://github.com/sergeyfarin/rekenraam/issues/114)
  (return of capital, manual splits, cash in lieu), one validated operation at a time.
  The [slice 5 contract](plans/investment-operation-slice-5-contract.md)
  fixes posting, allocation, date, reconciliation and unknown-basis behavior.
  Typed transfer facts and the first known-basis external transfer-in API
  command and mobile entry screen are complete. The internal transfer API
  now moves explicitly selected long lots between holding accounts with
  carried basis, reconciliation review and a mobile entry screen. Next:
  pooled-basis allocation for internal transfers from open average-cost
  positions, outbound transfers and cross-account carried-basis replay after
  the correction and integrity gates. Keep basis-affecting
  provider suggestions in review until their operation exists.
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
