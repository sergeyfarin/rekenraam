# TODO — short-horizon working queue

The distilled "what next" view. Items here are pointers — detail lives in the
roadmap (initiatives), backlog (defect registry), or the linked review docs.
Delete items when done; promote items when they grow. This file is allowed to
be edited freely and is never the source of truth for a decision.

Last updated: 2026-08-05.

## Decisions — none pending

All seven `reviews/roadmap-review-2026-07-19.md` §3/§4 proposals were decided
2026-08-05 — **all accepted**, each with a scope fence. Recorded in
`roadmap.md` ("Decisions adopted 2026-08-05"); do not re-litigate here.

The three `plans/connections-plan.md` and `plans/receipts-plan.md` questions
were also decided 2026-08-05: sequencing adopted with the quote-provider
slice moved into R17; R14a stays after R5 with an attachments hook in R3;
the provider "verify" items reclassified as blocking slice-start
preconditions. Also in `roadmap.md`.

**Nothing is currently awaiting an owner decision.**

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

- [x] T-35 QIF EU date misparse — fixed 2026-08-06 (`app/import_locale.go`:
      profile `date_layout` honored, otherwise the layout is detected across
      the whole file).
- [x] T-36 decimal-comma amounts 100× off — fixed 2026-08-06, same file
      (`canonicalDecimal`, profile `decimal_separator`).
- [x] T-39 background work no longer retries forever — fixed 2026-08-06
      (`app/pricing_worker.go` `maxFXCoverageAttempts`; failed items listed in
      pricing source health with a manual re-enqueue endpoint).
- [x] T-40 `triangulation_max_hops` now honored — done 2026-08-06
      (`app/pricing_refresh.go`: multi-hop chain search, shortest route wins).
- [x] T-37 price observations can be voided — done 2026-08-06
      (`POST /pricing/prices/{id}/void`; cascades to rates triangulated from
      the voided leg; R11 still owns the UI).
- [x] S-07 authentication-event visibility — done 2026-08-06
      (`authentication_events` + `GET /auth/events` + structured logs;
      90-day retention). Public-deployment gate closed.
- [x] T-38 zero-proceeds write-off — done 2026-08-06
      (`POST /investments/write-off`; dedicated endpoint, not a zero-amount
      sell; loss stays a computed gains value, see I-04).
- [x] T-41 scaled-integer arithmetic consolidated — done 2026-08-06
      (`internal/exact/scaled.go`: `exact.ScaledInt` + `exact.Pow10` replace
      `scaledAmount`/`scaledInteger`/`pow10DB`).

## Hygiene

- [ ] Keep `docs/README.md` accurate when adding or moving documentation.
