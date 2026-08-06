# Resolved Backlog Record — 2026-07

This is the historical record removed from the live `docs/backlog.md` during the
2026-07-12 documentation consolidation. The detailed original writeups are
preserved verbatim in `docs/reviews/backlog-pre-consolidation-2026-07-12.md`.
`docs/implemented.md`, tests, and the linked design records are the source for
current behaviour.

## Resolved

- **Security and operations:** S-01 setup ownership claim; S-02 slow-client
  protection; S-03 trusted forwarded-client parsing; S-05 external TLS posture;
  S-08 SQLite/WAL/SHM permissions; T-01 configurable session lifetime; T-18
  expired-session cleanup; T-19 secret-key loss/rotation guidance.
- **Import correctness and contracts:** T-05 pagination consumption; T-06
  atomic generic import commit; T-07 import OpenAPI coverage; T-08 encrypted
  connection secrets; T-11 real Trading 212 key probe; T-12 retained connection
  provenance; T-14 paginated-history continuation; T-15 retry re-staging signal;
  T-16 atomic start guard; T-17 same-timestamp cursor safety; T-21 verified
  provider endpoint/enums; T-22 import entry-kind validation; T-23 complete
  preview pagination; T-26 atomic investment-import commit; T-28 intraday fill
  ordering; T-29 unused routing-setup compensation; T-30/T-33 settlement-account
  eligibility; T-31 provider provenance; T-32 concurrent commit protection.
- **Product validation and UI coverage:** T-20 critical E2E expansion; T-24
  reconciliation guard for investment postings; T-25 pricing-history results
  signal; B-T212-SCHED scheduled refresh.

## Resolved later (recorded here to keep one resolution record)

- **T-35 QIF EU date misparse** and **T-36 decimal-comma amounts 100× off**,
  both fixed 2026-08-06. New `backend/internal/app/import_locale.go` owns the
  locale decisions for file imports: an import profile's `date_layout` and
  `decimal_separator` win when set, and otherwise the QIF adapter resolves the
  date field order across the whole file (one row with a day above 12 settles
  it) and the decimal separator per amount. Staged rows now carry ISO dates and
  canonical amounts; the raw fields keep the file's own spelling. A file whose
  dates are all ambiguous, or whose rows disagree, parses as `MM/DD` with a
  parse warning telling the user to set a profile layout. Covered by
  `import_locale_test.go` and the EU cases in `import_qif_test.go`.

- **T-41 scaled-integer arithmetic consolidated into `exact`**, done 2026-08-06.
  The two near-identical private helpers (`scaledAmount` in `app`,
  `scaledInteger` in `db`) and the duplicate `pow10`/`pow10DB` tables are gone,
  replaced by `exact.ScaledInt` and `exact.Pow10` in
  `backend/internal/exact/scaled.go`. `ScaledInt` carries
  add/sub/align/compare/truncate plus overflow-checked `Int64()` (returns
  `exact.ErrInt64Range`) and `Coefficient()`. Migrated call sites: ledger
  balances and register running balances, reconciliation totals, `PreviewSell`
  gain, and the `db` investment aggregations — `PositionsWithGains` and
  `ListRealizedGains`, whose hand-rolled basis→proceeds scale alignment is now
  `TruncatedTo`. The lot-selection loop in `disposeLotsTx` keeps its explicit
  `exact.Pow10` alignment because its exactness check (`QuoRem` remainder at the
  lot's own scale) has no `ScaledInt` equivalent. Behaviour is unchanged; tests
  moved to `internal/exact/scaled_test.go` and were extended to cover `Cmp`,
  `TruncatedTo`, `SubScaled`, operand immutability, and int64 overflow. The
  ledger-invariants skill now mandates the helper.

- **T-39 background work no longer retries forever**, fixed 2026-08-06. FX
  coverage work now gives up after `maxFXCoverageAttempts` (8) failed attempts
  and moves to the existing `failed` status instead of retrying against the 6h
  backoff cap indefinitely — matching what the Trading 212 fetch worker already
  did. Because a bounded cap is only safe if the work is recoverable, three
  things landed together: `BackgroundWorkRepository.ListBackgroundWorkByStatus`
  surfaces given-up items through `PricingService.FailedBackgroundWork`, which
  the source-health and currency-settings page responses now carry as
  `failed_background_work`; `RequeueBackgroundWork` moves a failed item back to
  pending with `attempts` reset, refusing the transition with
  `ErrBackgroundWorkAlreadyActive` when an equivalent item is already live (the
  active-unique index allows one); and
  `POST /api/v1/pricing/background-work/{work_id}/retry` plus a re-enqueue
  button on the currency settings page make that reachable from the UI. Covered
  by `TestProcessFXCoverageWork_GivesUpAfterMaxAttempts` and the two
  `TestRetryBackgroundWork_*` cases.

- **T-40 `triangulation_max_hops` is now honored**, fixed 2026-08-06. The
  policy value used to be a bare on/off check (`<= 0` disabled derivation) with
  the derived path hard-coded to exactly one intermediate currency, so a
  configured 2 silently behaved as 1. `fetchAndStoreDerivedFX` now resolves a
  chain of arbitrary length: `findFXPath` deepens one hop at a time and returns
  the first chain it can complete, so the **shortest** available route always
  wins — each extra leg compounds rounding and widens the vintage spread across
  source observations. Every leg fetch is memoized, so deepening re-walks
  shallower depths without extra provider calls, and the one-hop search order is
  unchanged from before. Storage generalized with it: `scaledFXProduct` takes N
  observations and stays a single exact `big.Int` quotient (no intermediate leg
  product is ever rounded), the derivation metadata records every leg and an
  N-factor formula, `via_currency_code` became `via_currency_codes`, and the
  chain is dated by its stalest leg. The derived observation's dedupe key keeps
  its single-hop spelling, so pre-existing derived rows still match. Covered by
  `TestRefreshFXTarget_DerivesMultiHopChainWhenPolicyAllowsMoreHops`,
  `TestRefreshFXTarget_PrefersShorterChainOverDeeperOneFoundFirst` (verified to
  fail against a plain depth-first search), and
  `TestRefreshFXTarget_ZeroHopsDisablesTriangulationEntirely`.

- **T-37 price observations can now be voided**, fixed 2026-08-06 (audit P3,
  R16 slice 1). `price_observations.voided_at` was written by nothing;
  `POST /api/v1/pricing/prices/{price_id}/void` now retires an observation
  with a **required** reason, so a poisoned rate stops feeding valuations
  while the row itself survives — prices are corrected by superseding or
  voiding, never edited or deleted. Three decisions are worth recording.
  (1) **The void cascades.** A triangulated rate is a separate observation
  carrying the same poisoned number under a new id, so
  `VoidPriceObservation` walks `derivation_json`'s `legs[].observation_id`
  (SQLite `json_each`, breadth-first, bounded by `maxVoidCascadeDepth`) and
  retires every rate derived from the voided one, tagging their reason with
  the originating id. Voiding only the leg would have left the bad number
  live. (2) **Not idempotent** — a second void would overwrite the first
  reason, so it returns `ErrPriceObservationAlreadyVoided` → 409. (3) **No
  reconciliation guard is involved**: observations carry no postings, so a
  void cannot change a reconciled balance; it changes reported market values
  only. Migration `0002` adds `voided_audit_event_id` so the one
  `audit_events` row per void is referenced from every row it retired, and a
  partial index over voided rows. `GET /pricing/prices` gained
  `include_voided=true` so what was retired stays inspectable. Covered by the
  five `TestVoidPrice_*` cases in `app/pricing_test.go` — including
  `..._CascadesToRatesTriangulatedFromTheVoidedLeg` and
  `..._StopsTheObservationBeingUsedForValuation` — plus `TestVoidPrice_HTTP`.
  The R11 pricing UI still owns the operator-facing surface.

## Deliberate non-work

- T-02 single runtime book ID, T-03 CSRF-token rotation, and T-04 CSP
  `unsafe-inline` are recorded design choices with their rationale in the prior
  backlog history and governing security/architecture documents.
- T-13 file-import-wrapper service coverage and B-T212-INVST linking an existing
  manually tracked holding account are deferred design follow-ups, not current
  defects. Revisit only when their respective product work is scheduled.

## Moved product decisions

I-03 (multi-method analytical gains) and I-04 (posting realized gains) are not
technical debt. They are deferred accounting/reporting product decisions and now
live in the open decisions section of `docs/roadmap.md`.
