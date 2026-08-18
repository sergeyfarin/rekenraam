# TODO — short-horizon working queue

The distilled "what next" view. Items here are pointers — detail lives in the
roadmap (initiatives), backlog (defect registry), or the linked review docs.
Delete items when done; promote items when they grow. This file is allowed to
be edited freely and is never the source of truth for a decision.

Last updated: 2026-08-17.

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

- [x] Shared report filter contract (`account_id`, `include_descendants`,
      `commodity_id`) on the backend, with a `query.filters` echo carrying the
      resolved account expansion — done 2026-08-17.
- [x] Spending read model: `GET /api/v1/reports/spending`, category/payee
      grouping, exact per-commodity totals, within-commodity shares, and
      drill-down queries — done 2026-08-17 (backend only).
- [x] Spending view (frontend): view switch, dense table, category/payee and
      spending/income switches, single-commodity bar chart, all screen states —
      done 2026-08-17. Filter *controls* and drill-down links still open below.
- [x] Filter controls (account / commodity / category / payee pickers) for both
      views — done 2026-08-18. Also fixed the net-worth OpenAPI path, which
      never declared the shared filter parameters its handler already parsed,
      so the generated client could not send them.
- [ ] Drill-down links from a spending row, once the transactions route can
      honour the same date/category/payee semantics.
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
- [x] T-41 scaled-integer arithmetic consolidated — done 2026-08-06
      (`internal/exact/scaled.go`: `exact.ScaledInt` + `exact.Pow10` replace
      `scaledAmount`/`scaledInteger`/`pow10DB`).
- [x] T-43 gofmt drift cleared — done 2026-08-17; `gofmt -l` and `go vet` moved
      into `scripts/test-backend.sh` so CI enforces them.
- [ ] T-42 TypeScript 7 — blocked upstream on `openapi-typescript` TS 7
      support. Re-check on each of its releases; nothing to do here until then.
- [x] T-45 net-worth series re-reads the ledger per bucket — done 2026-08-18;
      one ledger read folded forward across buckets, with account versions
      replayed in one pass instead of a snapshot query per bucket. `bucket=day`
      over a year drops from ~1.4 s to ~9 ms, and response time is now flat
      across bucket granularity.
- [x] T-46 CSP-blocked inline style cleared — done 2026-08-18; it was a
      `style` *attribute* (`style-src-attr`), not a stylesheet: SvelteKit's
      generated `#svelte-announcer` hardcodes its visually-hidden rules inline.
      A Vite plugin strips the attribute and `app.css` styles the announcer, so
      `style-src` stays `'self'` with no `'unsafe-inline'` or `'unsafe-hashes'`.

Everything else open in `backlog.md` is either roadmap-scheduled (T-37/T-38 are
R16 slice 1; T-34's producer is R15) or awaiting an owner decision
(S-04 throttle design, S-06 MFA mechanism, S-07 log-vs-table).

## Hygiene

- [ ] Keep `docs/README.md` accurate when adding or moving documentation.
