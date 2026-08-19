# TODO — short-horizon working queue

The distilled "what next" view. Items here are pointers — detail lives in the
roadmap (initiatives), backlog (defect registry), or the linked review docs.
Delete items when done; promote items when they grow. This file is allowed to
be edited freely and is never the source of truth for a decision.

Last updated: 2026-08-19.

## Decisions — owner answers of 2026-08-19

Recorded here as pointers; the source of truth is `roadmap.md` and
`product-requirements.md`.

- **Reporting-currency selector: build it** (one reporting currency, named
  valuation method), sequenced after R3. Per-commodity exact totals stay in
  every response — conversion is additive, never replacing.
- **Free-text payees (T-44): resolve on entry, but never silently.** Typing a
  new payee name prompts for confirmation, offering existing payees through a
  fuzzy search before creating a record.
- **Zero-proceeds write-off (T-38): a disposal at zero proceeds**, booking
  through the existing realized gain/loss treatment rather than a dedicated
  expense category.
- **Cashflow keeps its reconciliation guarantee.** Category and payee filters
  are not added to it; the filtered question is answered by the spending report,
  reached by drill-down from a cashflow row. Approach still to be confirmed.
- **Public-deployment gates (S-04, S-06, S-07): parked.** Self-hosted locally
  for now; unpark when an internet-exposed deployment is planned, R3 at the
  earliest.
- **Gains reporting (I-03 / I-04): research task, not a decision.** See
  `roadmap.md` — realized vs unrealized, per-country tax treatment, and
  presentation stability all need study first. Does not block R16 slice 1.
- **First non-English languages: Spanish, French, Dutch, German, Russian.**

## Decisions — none otherwise pending

All seven `reviews/roadmap-review-2026-07-19.md` §3/§4 proposals were decided
2026-08-05 — **all accepted**, each with a scope fence. Recorded in
`roadmap.md` ("Decisions adopted 2026-08-05"); do not re-litigate here.

The three `plans/connections-plan.md` and `plans/receipts-plan.md` questions
were also decided 2026-08-05: sequencing adopted with the quote-provider
slice moved into R17; R14a stays after R5 with an attachments hook in R3;
the provider "verify" items reclassified as blocking slice-start
preconditions. Also in `roadmap.md`.

**Awaiting an owner decision:** only the cashflow drill-down approach (above)
and who produces the translations for the five target languages.

## Current initiative — R2 reports (see roadmap.md)

- [x] Shared report filter contract (`account_id`, `include_descendants`,
      `commodity_id`) on the backend, with a `query.filters` echo carrying the
      resolved account expansion — done 2026-08-17.
- [x] Spending read model: `GET /api/v1/reports/spending`, category/payee
      grouping, exact per-commodity totals, within-commodity shares, and
      drill-down queries — done 2026-08-17 (backend only).
- [x] Spending view (frontend): view switch, dense table, category/payee and
      spending/income switches, single-commodity bar chart, all screen states —
      done 2026-08-17. Filter *controls* and drill-down links followed on
      2026-08-18, below.
- [x] Filter controls (account / commodity / category / payee pickers) for both
      views — done 2026-08-18. Also fixed the net-worth OpenAPI path, which
      never declared the shared filter parameters its handler already parsed,
      so the generated client could not send them.
- [x] Drill-down links from a spending row — done 2026-08-18. The blocker was
      real but narrower than stated: the transactions list already took
      account/category/payee and inclusive dates, but filtered on the
      transaction's own date while the reports sum on entry dates. Added
      `date_basis=entry` and `category_type` to `GET /transactions`, and the
      list now reads its filters from the URL. Rows the route cannot express
      (the unattributed group, a report narrowed to several accounts) stay
      unlinked rather than leading somewhere that disagrees with the number.
- [x] Cashflow read model + view (inflow / outflow / transfers / net) — done
      2026-08-18. `GET /api/v1/reports/cashflow` is exact per commodity per
      bucket, with `net_movement = operating_net + transfer_net` holding as an
      identity because journal entries balance per commodity. The view names its
      cash scope and states the transfer policy on screen. Category and payee
      filters on cashflow are deferred with a recorded reason — they break
      `net_movement`'s reconciliation guarantee.
- [x] Date/account/category/payee/commodity filters — done 2026-08-18 across all
      three views (category and payee only where a report can express them).
- [x] CSV + print-friendly tables; summary charts alongside (not replacing)
      the accessible data table — done 2026-08-18. CSV carries exact
      unformatted decimals with the commodity in its own column, because a
      locale-formatted figure destroys a CSV. Printing drops navigation,
      filters, the view switch, and the export button, keeping the headings,
      the stated scope, and the table. Net worth and cashflow gained signed
      column charts; all three views keep the table as the source of truth.
- [ ] R2 acceptance review — the only thing left in R2, and an owner decision
      rather than implementation. Cross-currency valuation is already decided
      (build it, after R3), so the review covers the remaining
      `plans/reports-plan.md` follow-ups: named saved report definitions,
      immutable report snapshots, investment/tax/benchmark dimensions, and the
      user-configurable report builder. Investment dimensions depend on R16/R17
      lot lifecycle work that does not exist yet.

## Ready to start — unblocked by the 2026-08-19 decisions

Ordered by value. None of these needs a further decision.

1. **T-44 payee resolution on entry.** Highest value: it repairs the payee
   ranking in the spending report that shipped this week. Typing a new payee
   name prompts for confirmation, offering existing payees through fuzzy search
   before creating a record. Three parts the decision implies:
   - manual entry gets the confirm-and-search flow;
   - **imports cannot show a dialog per row**, so the import path needs its own
     rule (the resolution step already carries a `PayeeID`); auto-link exact
     matches, leave the rest unlinked, surface them in import review;
   - **existing rows keep `payee_name` with no `payee_id`**, so history stays
     degraded without a one-off "link unlinked payees" tool.
   Fuzzy search needs `minisearch` added — not currently a dependency, though
   `conventions.md` already names it as the sanctioned choice for dropdowns.
2. **T-37 price observation voiding**, R16 slice 1 and a prerequisite for the
   reporting-currency work.
3. **T-38 zero-proceeds write-off**, R16 slice 1. Zero proceeds must be explicit
   intent rather than an empty field defaulting to zero, or a mistyped sell
   silently becomes a write-off; and it is a disposal, so it goes through the
   reconciliation guard.
4. **Cashflow → spending drill-down** *(approach not yet confirmed)*: link a
   cashflow row's in/out cell to the spending report scoped to that bucket's
   dates, the same cash accounts, and the matching direction. Wants spending's
   account filter tightened from transaction level to journal-entry level so the
   two agree exactly for multi-entry transactions.

Localization of the five target languages is unblocked on *which* languages but
still open on **who writes the translations** — roughly 250 messages of
financial vocabulary where a plausible-but-wrong term is worse than English.

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

Everything else open in `backlog.md` is roadmap-scheduled or parked:

- **T-37 and T-38 are R16 slice 1, and both now have their decisions** (T-38
  books a write-off as a disposal at zero proceeds through the existing
  realized gain/loss treatment). T-37 is also a prerequisite for the
  reporting-currency work — once rates drive headline numbers, an unvoidable
  poisoned observation stops being cosmetic.
- **T-44 has its decision** (resolve a typed payee name on entry, with
  confirmation and fuzzy search over existing payees; never auto-create
  silently) and is ready to schedule.
- **T-34's producer is R15**, still with no chosen data source.
- **S-04, S-06, and S-07 are parked**, not awaiting a decision: the app is
  self-hosted locally, and the trigger to unpark is the first planned
  internet-exposed deployment.

## Hygiene

- [ ] Keep `docs/README.md` accurate when adding or moving documentation.
