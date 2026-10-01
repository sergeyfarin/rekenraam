# TODO — short-horizon working queue

This is the next-action view. The [roadmap](roadmap.md) owns sequence and the
[investment operation plan](plans/investment-operation-refactor-plan.md) owns
R16 acceptance criteria. [GitHub Issues](https://github.com/sergeyfarin/rekenraam/issues)
tracks actionable work; the [backlog](backlog.md) maps local IDs, and
[implemented](implemented.md) records shipped behavior.

Last reconciled: 2026-10-01.

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
  shipped sub-slices. Trading 212 BUY and SALE source replacement and their
  dedicated reconciliation previews are also shipped. Terminal native buy/sale
  reversal is shipped. Source cancellation, broader backdated admission, other
  operation corrections remain open.
- [x] Converge native BUY/SALE reversal transaction orchestration onto the
  existing investment writer (slice 4ag), with source guard before journal
  insertion and late checkpoint-failure rollback coverage. Compound
  replacements and source acceptance converge in slice 4ah below.
- [x] Converge compound BUY/SALE replacement orchestration and source acceptance
  onto the shared investment writer (slice 4ah), with guards before either
  journal and late checkpoint/acceptance rollback plus stale-command coverage.
  Verified source execution cancellation, wider source revisions and other
  operation correction/backdating gates remain in #99.
- [x] Prove dependent replay in manual/source-linked and Trading 212 buy
  replacement reconciliation previews (slice 4ai). Preview uses the actual
  writer in a rolled-back transaction, refuses named disposal/transfer conflicts,
  and preserves exact durable rows. The commit still rechecks and requires
  explicit checkpoint override.
- [x] Preserve Trading 212 fill taxonomy and hold unsupported or missing
  types before any trade/cash import or source correction (slice 4af,
  [T-112 #127](https://github.com/sergeyfarin/rekenraam/issues/127)). Source
  execution cancellation still needs verified provider evidence; cancelled
  order status and FOP_CORRECTION are insufficient to reverse an execution.
- [x] Complete [T-110 #125](https://github.com/sergeyfarin/rekenraam/issues/125)'s
  reopened slice 2a integrity gates: pinned component posting links,
  independent decision/clearing attribution, authoritative operation journal
  links, immutable lot identity/current-state separation, reconstruction,
  explicit nullable projected-basis knowledge, export and mutation/seeded tests.
  Current commands require known source basis; unknown immutable facts and
  resolution replay remain slice 5 gates. Sharing correction transaction
  orchestration is complete in T-75b slices 4ag–4ah; its remaining correction
  gates stay in #99.

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
