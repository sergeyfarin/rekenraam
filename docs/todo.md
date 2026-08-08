# TODO — short-horizon working queue

The distilled "what next" view. Items here are pointers — detail lives in the
roadmap (initiatives), backlog (defect registry), or the linked review docs.
Delete items when done; promote items when they grow. This file is allowed to
be edited freely and is never the source of truth for a decision.

Last updated: 2026-08-08.

## Decisions — one pending

All seven `reviews/roadmap-review-2026-07-19.md` §3/§4 proposals were decided
2026-08-05 — **all accepted**, each with a scope fence. Recorded in
`roadmap.md` ("Decisions adopted 2026-08-05"); do not re-litigate here.

The three `plans/connections-plan.md` and `plans/receipts-plan.md` questions
were also decided 2026-08-05: sequencing adopted with the quote-provider
slice moved into R17; R14a stays after R5 with an attachments hook in R3;
the provider "verify" items reclassified as blocking slice-start
preconditions. Also in `roadmap.md`.

**One decision is now pending (opened 2026-08-08).** `scaledRatioToDecimalText`
in `routes/app/settings/currencies/+page.svelte` renders an FX rate by
**truncating** the division at 8 decimals. `ledger-invariants` says division and
implied prices round **half-up via the shared helpers**, and the backend's
`scaledDivision` does. Should a *displayed* rate follow that rule, or is
truncation correct because it never overstates a rate? The difference is visible
at the 8th decimal. Deliberately not decided while closing G-09 — see
`backlog.md` G-09.

## Current initiative — R2 reports (see roadmap.md) — **complete 2026-08-08**

- [x] Spending view — done 2026-08-08 (`GET /api/v1/reports/spending`,
      `group_by=category|payee`, `direction=expense|income`; its own read model
      rather than an overload of `ledger/category-totals`).
- [x] Cashflow read model + view — done 2026-08-08
      (`GET /api/v1/reports/cashflow`; classification derived from the
      per-journal-entry balancing identity, so `net_movement` reconciles to the
      cash balance change by construction and a many-counterpart split needs no
      allocation rule).
- [~] Filters — **date, bucket, group_by, and direction shipped; the repeated-ID
      filters (`account_id`/`category_id`/`payee_id`/`commodity_id`) and
      drill-down did not.** Recorded as the one delivery gap in the R2
      acceptance review and scheduled as the next reports work. The URL filter
      parser already carries more state than the reports read, so this is
      additive.
- [x] CSV + print-friendly tables; summary charts alongside (not replacing) the
      accessible data table — done 2026-08-08. CSV uses `formatLedgerAmount`,
      not the display formatter, because a spreadsheet cannot parse locale group
      separators. Charts are `aria-hidden`, single-commodity only.
- [x] R2 acceptance review — done 2026-08-08. Every preserved follow-up is
      answered yes or no with reasoning at the end of
      `docs/plans/reports-plan.md`. Next: ID filters + drill-down, then the
      named reporting-currency valuation method. Saved definitions, snapshots,
      tax/jurisdiction dimensions, and a report builder are all deferred with
      stated reasons.

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
- [x] T-42 commodity enable date no longer blocks earlier history — done
      2026-08-06 (`db.CommodityGenesisDate`; retired the Trading 212
      instrument-backdating workaround). Follow-ups T-43 and T-44 both
      done 2026-08-07.
- [x] T-43 user-created categories open at the genesis date — done
      2026-08-07 (`app.categoryGenesisDate`; `opened_on` removed from the
      category create/update API and the editor).
- [x] T-44 a later import carrying an earlier trade commits — done
      2026-08-07 (import-created holding accounts open at the genesis date;
      `docs/design/holding-account-opened-date.md`).
- [x] S-04 lockout-safe login throttle — done 2026-08-06 (approved-device
      cookie moves a known device onto its own throttle scope; the cookie is
      not a credential). Public-deployment gate closed.
- [x] S-07 authentication-event visibility — done 2026-08-06
      (`authentication_events` + `GET /auth/events` + structured logs;
      90-day retention). Public-deployment gate closed.
- [x] T-38 zero-proceeds write-off — done 2026-08-06
      (`POST /investments/write-off`; dedicated endpoint, not a zero-amount
      sell; loss stays a computed gains value, see I-04).
- [x] T-41 scaled-integer arithmetic consolidated — done 2026-08-06
      (`internal/exact/scaled.go`: `exact.ScaledInt` + `exact.Pow10` replace
      `scaledAmount`/`scaledInteger`/`pow10DB`).
- [x] S-06 multi-factor authentication — done 2026-08-07 (TOTP + recovery
      codes, `internal/totp/` + migration 0005 + Settings → Security). The
      last public-deployment gate; what remains is enrolling the owner
      account before an internet deployment.

- [x] G-07 CI coverage signal — done 2026-08-07 (`COVERAGE=1
      scripts/test-backend.sh` + non-gating `backend-coverage` job + soft floor
      `scripts/check-coverage-floor.sh`; merged total 75.2%, floor 73.0%).
      Closes the last open item of `plans/backend-test-coverage-plan.md`
      besides Workstream 6, whose harnesses wait for their first consumer.

## Open, unscheduled (from the doc sweep of 2026-08-07)

- [x] G-02 frontend money logic untested — **done 2026-08-08**. The editor and
      reconcile form landed first (turning up T-45 and T-46); the three
      investment forms followed, turning up **T-47 — the same decimal-comma
      100× error, still live in every one of them**. They held seven copies of
      three helpers, not the two the survey counted. Validation is now in
      `lib/investments/form-amounts.ts` behind 35 named tests, because with no
      component-test harness a per-form behaviour change cannot be pinned in
      place. `toSafeInt` became `toInt64Coefficient` in `lib/api/investments.ts`
      — it adapts an API contract quirk and is not money arithmetic, so it
      stayed out of `$lib/money`.
- [~] G-09 `.svelte` files carrying private money math — opened and mostly
      closed 2026-08-08. `category-transactions.svelte` retired onto a new
      `sumByCommodity` in `$lib/money/amount.ts` (it held a third copy of the
      ledger's normal-sign rule). `gains-report.svelte` turned out **not** to be
      a defect on closer reading — its lossy conversion feeds only a sign test
      that picks a CSS colour. Remaining: `settings/currencies/+page.svelte`,
      which is larger than first recorded — it does ad-hoc *truncating* division
      on an implied FX rate where `ledger-invariants` requires half-up via a
      shared helper, and then drops the exact result through `Number()` before
      formatting. **Needs a decision** (truncate or round a displayed rate?) so
      it was not folded in silently. See `backlog.md` G-09.
- [~] G-08 amount input is not locale-aware — **display half settled
      2026-08-08**: `Intl.NumberFormat` wins over Dinero.js, the unused
      `dinero.js` dependency is gone, and `formatQuantity` now lives in
      `frontend/src/lib/money/format.ts` as the read-only display half of
      `$lib/money`. The input half (resolving the separator from the active
      locale when parsing and when refilling a form field) stays open and is
      still gated on the first non-`en` locale. See `backlog.md` G-08.
- [ ] T-34 investment provider-event producer — `[blocked]` on R15's third
      slice and an unmade provider choice. See `backlog.md`.

## Hygiene

- [ ] Keep `docs/README.md` accurate when adding or moving documentation.
