# TODO — short-horizon working queue

The distilled "what next" view. Items here are pointers — detail lives in the
roadmap (initiatives), backlog (defect registry), or the linked review docs.
Delete items when done; promote items when they grow. This file is allowed to
be edited freely and is never the source of truth for a decision.

Last updated: 2026-07-19.

## Decisions awaiting the owner

From `reviews/roadmap-review-2026-07-19.md` (§3) — proposals only until
accepted into `roadmap.md` or rejected here:

- [ ] Add EU import correctness (T-35, T-36) to the pre-announcement gates?
- [ ] Extend R3 scope with scheduled backups + trial-balance self-check?
- [ ] Pull a minimal import-rules v1 into R5, or record the deferral?
- [ ] Resolve the R10-forecasting promotion question (analysis recommends
      promoting; roadmap currently sequences it third — decide and record).
- [ ] Name an "investment lifecycle completeness" slice (manual splits,
      write-off T-38, price void T-37, return-of-capital basis reduction)?
- [ ] Personal-access tokens before the public announcement?
- [ ] Widen the persona to crypto-holding expats (instrument type + one
      BYO-key price adapter), per review §4?

From `plans/connections-plan.md` and `plans/receipts-plan.md` (2026-07-19):

- [ ] Adopt (or amend) the connections sequencing: IBKR Flex → quote
      provider → GoCardless EU/UK banks → T-34 event producer; API-less
      brokers (Trade Republic, DeGiro, Raisin, UK brokers) as R5 CSV
      mapping-profile presets.
- [ ] Decide the Yahoo Finance question (best EU quote coverage, unofficial
      API — ship as clearly-labeled community adapter, or skip).
- [ ] Pull receipts slice R14a (attachment storage + backup-story extension)
      forward next to R3, or keep all of R14 later?
- [ ] Re-verify at adapter start: GoCardless free-tier terms/rate caps and
      IBKR Flex endpoint/error specifics (marked "verify" in the plan).

## Current initiative — R2 reports (see roadmap.md)

- [ ] Spending view over the existing category-totals read model.
- [ ] Cashflow read model + view (inflow / outflow / transfers / net).
- [ ] Date/account/category/payee/commodity filters.
- [ ] CSV + print-friendly tables; summary charts alongside (not replacing)
      the accessible data table.
- [ ] R2 acceptance review: explicitly decide which `plans/reports-plan.md`
      items (saved definitions, cross-currency valuation, investment
      dimensions, snapshots) are justified before starting R3.

## Bug-fix queue (from backlog — schedule independent of R2)

- [ ] T-35 QIF EU date misparse — small, launch-critical; profile plumbing
      already exists.
- [ ] T-36 decimal-comma amounts 100× off — same fix area as T-35.

## Hygiene

- [ ] Keep `docs/README.md` accurate when adding or moving documentation.
- [ ] After the next ledger-touching refactor lands (`exact.ScaledInt`,
      T-41), update the ledger-invariants skill to mandate it.
