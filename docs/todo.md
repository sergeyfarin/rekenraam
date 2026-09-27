# TODO — short-horizon working queue

This is the next-action view. The [roadmap](roadmap.md) owns sequence and the
[investment operation plan](plans/investment-operation-refactor-plan.md) owns
R16 acceptance criteria. The [backlog](backlog.md) holds independent defects
and debt; [implemented](implemented.md) records shipped behavior.

Last reconciled: 2026-09-27.

## Current: runtime maintenance and R16 correction

- [ ] Extract the application runtime in the next suitable backend maintenance
  window, before the correction command. Scope and acceptance are in the
  [roadmap](roadmap.md#current-plan).
- [ ] Finish slice 4: resolve correction chains and add the investment-native
  correction/reversal command (T-75b). A posted reversal and replacement must
  update journal, effective lots and gains, prices, audit links, and
  reconciliation impact together. Keep backdated writes fenced until the
  dependency and rollback tests pass.
- [ ] Validate a corrected old buy followed by dependent sells under each cost
  basis method, including failure rollback and original versus effective
  allocations. The immutable intent reader, reversible replay simulation,
  revision storage, gains and self-check readers, and revision-chain export are
  already complete (slices 4a–4e).

## Then, within R16

- [ ] Specify and deliver in-kind transfers and basis actions (return of
  capital, manual splits, cash in lieu), one validated operation at a time.
  Keep basis-affecting provider suggestions in review until their operation
  exists.
- [ ] Add named short sale and cover (T-108) with side-aware gains, dated
  positions, self-check, export, API, and mobile entry. An ordinary negative
  holding stays flagged as unclassified until corrected or explicitly entered
  as a short.
- [ ] Specify compound corporate actions after the common operations work.

## Parallel trust work

- [ ] Close G-08 locale-aware amount input and T-87 owner-local financial date
  defaults before a multilingual migration demo.
- [ ] Fill T-80's 65 missing keys per non-English locale, add catalog parity
  validation, and arrange native review before calling the catalogs complete.

## Following R16

- [ ] Promote R11 price/FX management UI, then R17 quote-provider registry and
  crypto entry, R18 reproducible gains, and R13 returns analytics. The
  [roadmap](roadmap.md) governs their scope and order.
- [ ] Schedule remaining [backlog](backlog.md) items by priority and dependency.
  T-34 waits for R15; T-48 waits for upstream TypeScript 7 support. Decide
  whether displayed FX rates round or truncate when G-09 is selected.
