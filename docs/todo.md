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

## Independent work and decisions

- [ ] Resolve the remaining active defects and debt by priority in the
  [backlog](backlog.md), including G-08/G-09, T-63, T-72/T-73, T-80, and T-87.
  T-34 remains dependent on R15; T-48 remains dependent on upstream support.
- [ ] Arrange native-language review for the drafted Spanish, French, Dutch,
  German, and Russian catalogs before presenting them as reviewed translations.
- [ ] Decide whether displayed FX rates round or truncate (G-09).
