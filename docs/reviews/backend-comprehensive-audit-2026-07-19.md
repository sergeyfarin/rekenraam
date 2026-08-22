# Backend comprehensive audit (2026-07-19)

> **Status: all §1 findings closed as of 2026-08-07.** P1/P2 (EU dates,
> decimal comma) fixed 2026-08-06 as T-35/T-36; P3 (price voiding) as T-37;
> P4 (zero-proceeds write-off) as T-38; P7 (unbounded background retries) as
> T-39; P6 (`triangulation_max_hops` theater) as T-40. P5 was a
> **documentation** defect, not a code one — the code's posted-only FX
> coverage is correct, and the taxonomy wording in `docs/conventions.md` and
> the `ledger-invariants` skill was corrected to match (2026-08-07). The §5
> refactor list is partly done (#1 `exact.ScaledInt` = T-41, 2026-08-06); the
> rest of §5–§7 are unscheduled suggestions, not defects — see
> `docs/backlog.md` for what is actually queued. This document stays as the
> dated audit record.

Second-pass, wider-scope audit following the 2026-07-13 ledger/investments
audit (all eleven findings from that pass are fixed and tested; see
`backend-ledger-investments-audit-2026-07-13.md`). This pass covers the areas
the first audit did not: the database layer itself (SQLite setup, schema,
triggers, background work, backup/recovery), the pricing/FX subsystem and
market-data providers, the import pipeline end to end, investment automation
and corporate-action coverage, feature completeness across every transaction
kind (ordinary, transfer, FX exchange, opening balance, adjustment, buy/sell
across all four cost-basis methods, cash and reinvested dividends, crypto,
closed funds, shorts), API-integration readiness in both directions, and
refactoring/feature opportunities.

Verdicts: **CONFIRMED** = reproduced by direct code-path reading with a
concrete trigger; **LOW** = real but low-impact or operational.

---

## 1. Correctness and integrity findings

No new severity-1 defects were found in the ledger or lot engine this pass —
the money-path core (balance validation, scale alignment, reconciliation
guards, audit coupling, lot disposal) held up under re-examination after the
2026-07-13 fixes. The findings below are in the surrounding subsystems.

### P1. QIF import misparses EU dates; the profile override is a stub (CONFIRMED)

`parseQIFDate` (`app/import_service.go:1003`) tries `MM/DD/YY` **before**
`DD/MM/YY`, so `03/04/2026` in a European QIF export always parses as
March 4. The profile-based date-layout override the comment promises is
unimplemented — `QIFAdapter.Parse` (`app/import_qif.go:34-37`) sets a
`dateLayout` variable and then discards it (`// Future: parse
profile.ConfigJSON for date_layout`). For a product whose declared niche is
expat/multi-currency users, EU-format QIF files silently land on wrong dates
whenever the day is ≤ 12 — the worst kind of wrong, because unambiguous rows
(day > 12) parse correctly and mask the pattern. The staged preview gives the
user a chance to notice, but a 40-row statement with 30 plausible-looking
dates will sail through.

**Fix shape:** honor a `date_layout` in the import profile's `config_json`
(the schema and plumbing already exist), and when no profile is set, detect
the layout batch-wide (a single file is internally consistent; one row with
day > 12 disambiguates the whole file) instead of per-row first-match.

### P2. Decimal-comma amounts parse 100× off (CONFIRMED)

`parseDecimalAmount` (`app/import_service.go:1049`) strips **all** commas as
thousands separators before parsing. A European-locale amount `"1,50"`
(one euro fifty) becomes coefficient `150` at scale 0 — one hundred and fifty
euros. Same failure class as P1: EU-format bank exports are exactly this
product's target data. Trading 212's CSV uses period decimals so the current
online path is safe; the exposure is QIF/file imports (and the upcoming R5
CSV import if it reuses this helper).

**Fix shape:** make the decimal separator locale-aware via the import
profile, or apply the standard heuristic (a single comma followed by 1–2
digits and no period is a decimal separator). Fold into the same profile
work as P1 — R5's "saved mapping profiles" is the natural home.

### P3. Price observations can be superseded but never voided (CONFIRMED)

The conventions say "correct a price by superseding or voiding, never
overwriting." Half of that exists: `price_observations.voided_at` is in the
schema, every read filters `voided_at IS NULL`, and observations can be
created with `supersedes_observation_id`. But **nothing anywhere sets
`voided_at`** — no repository method, no service method, no endpoint
(`db/pricing.go`, `app/pricing.go`, `api/pricing.go` all lack it). A wrong
manual price can only be shadowed by a later-recorded superseding
observation, which works for "latest price" reads (recency ordering) but
leaves the bad observation in every historical listing and derivation, and
gives the supersede mechanism no way to actually retire a poisoned
observation (e.g. a provider glitch rate that triangulation legs now
reference). Ties into roadmap item R11 (pricing/FX management UI), which
will need this endpoint anyway.

### P4. A worthless security cannot be written off (CONFIRMED)

`validateTradeInput` requires `CashAmountValue > 0`, and Sell is the **only**
API path that disposes lots (`DisposeLots` exists at the repository layer but
is unexposed). Consequence: a fund closure, a delisting at zero, or any
total-loss position can never be recorded — the shares are stuck in the
holding account and open lots forever, and the loss never reaches realized
gains. There is also no instrument lifecycle state for "delisted"/"closed"
(instruments only have active/archived). This blocks a real, if infrequent,
event that every long-lived portfolio eventually hits.

**Fix shape:** either allow `CashAmountValue == 0` on Sell with an explicit
`write_off` marker (keeps the balanced 4-posting shape degenerating to a
2-posting holding→trading disposal, with realized loss = full basis), or add
a dedicated write-off endpoint over the existing disposal engine. Needs a
product decision on the expense/loss account it books against.

### P5. Drafts do not trigger FX coverage, contradicting the documented taxonomy (LOW)

The lifecycle taxonomy (conventions + ledger-invariants skill) says durable
drafts "may trigger background FX coverage because they are durable." The
trigger `fx_work_after_posting_version_insert`
(`migrations/0001_initial_schema.sql`, rebuilt version) fires only `WHEN
tv.status = 'posted'`. Behavior is self-healing — promotion inserts new
posting rows with status posted, enqueueing coverage then — so there is no
financial harm, just a period where a draft's foreign-currency dates have no
rates yet (draft review UIs showing converted amounts would miss rates).
Either implement the documented behavior or fix the doc; today the doc is
wrong.

### P6. `TriangulationMaxHops` is config theater (LOW)

The pricing policy stores and validates `triangulation_max_hops`, but
`refreshFXTarget` (`app/pricing_refresh.go:226`) only checks `<= 0` to
disable triangulation; the derivation path is hard-coded to exactly one hop.
A user setting 2+ gets 1. Either implement multi-hop (rarely needed with a
sensible base currency) or clamp/remove the knob so config matches reality.

### P7. Background work retries forever (LOW)

`background_work_items` has an `attempts` counter and `retryDelay` backs off
exponentially to a 6-hour cap with per-item jitter — but nothing ever gives
up. A permanently failing item (e.g. FX coverage for a currency no free
provider serves) retries every 6 hours indefinitely, cluttering refresh-run
history and provider quota. Add a max-attempts → `failed` transition (the
status already exists) with surfacing in the pricing health endpoint, plus a
manual re-enqueue path.

### Carried over, still open by deliberate choice (from 2026-07-13)

Unchanged and re-confirmed this pass: zero-quantity postings accepted;
trade-implied-price errors swallowed (`_ =`) with `OriginType` hardcoded
`browser_api` even for imports; truncating (not half-up) division in
`proratedCostBasis` and `PositionsWithGains` market value. All are documented
product-judgment items, not regressions.

---

## 2. Integrity verification — what was checked and held

- **SQLite layer** (`db/sqlite.go`): PRAGMAs both applied per-connection and
  *verified* after open (foreign_keys, WAL, busy_timeout, synchronous,
  autocheckpoint); 0600 permissions enforced on DB + WAL/SHM; single-writer
  pool (`SetMaxOpenConns(1)`) making guard+insert patterns true critical
  sections; backup via `VACUUM INTO` with post-backup `integrity_check` +
  `foreign_key_check` verification. Only gap: backups are CLI-recovery-only
  (see §6).
- **Schema**: append-only version tables enforced by `no_update`/`no_delete`
  triggers (including the rebuilt post-ALTER versions); same-book triggers on
  every cross-table reference; lot value validity triggers (positive
  quantities, non-negative remaining basis); referenced-record delete
  protection on commodity/instrument versions with a carefully scoped
  exception for the import-compensation path; coefficient length CHECKs
  consistent with `exact.MaxCoefficientDigits` (the 38-vs-39 asymmetry
  between lots and postings is correct — lot columns are positive-only, so
  no sign character).
- **Audit model**: `insertAuditEvent` validates origin-type enum, dotted
  operation pattern, strict UTC RFC3339, and JSON-object metadata at the
  door; every mutation path creates its event in the same transaction.
- **Background queue**: claim/complete/retry/fail are lease-owner-guarded
  single transactions; the partial unique index on
  `(book_id, kind, payload_json) WHERE status IN ('pending','running')`
  dedupes trigger-enqueued and Go-enqueued items, and the Go payload struct's
  field order matches the trigger's `json_object` key order, so the dedupe
  actually fires across both producers.
- **Import pipeline**: fingerprints hashed and checked against durable
  `import_commit_identities`; identity + ledger transaction + staged-row
  marker written in one DB transaction on every path (including the T212
  investment path via postWrite callbacks); concurrent-committer races
  resolved by adopting the winner; expected data gaps (insufficient lots, no
  dividend default) fall back to the generic cash path instead of failing
  batches, while unexpected errors fail loudly per-row.
- **T212 fetcher**: staged per-endpoint cursors with resume paths, page caps
  against adversarial pagination, strict-inequality cursor semantics for
  equal-timestamp items, API keys encrypted at rest (AES-256-GCM secretbox).
- **API surface**: session cookie + derived CSRF token (SHA-256 of session
  token) + origin check on mutations; login rate limiting with named tests;
  security headers, request-ID propagation into audit events, panic
  recovery; FTS queries phrase-quoted (injection-safe); pagination cursors
  opaque and validated on decode.

---

## 3. Feature completeness by transaction kind

| Capability | Status | Notes |
|---|---|---|
| Ordinary income/expense | ✅ Complete | Balanced per commodity; categories as income/expense accounts |
| Same-currency transfer | ✅ Complete | Ordinary balanced transaction, `transfer_leg` entry kind |
| Cross-currency transfer | ✅ Structural | `exchange` entry kind + `transfer_clearing` system account exist; correctness enforced by per-commodity balancing. No dedicated backend flow — the client must construct the 4-posting shape |
| Opening balance | ✅ Structural | `opening_balance` kind + equity system account; no guided flow |
| Adjustment/correction | ✅ Complete | `correction_of_transaction_id` lineage + corrective workflow |
| Buy (fifo/lifo/average_cost/specific_lot) | ✅ Complete, tested | Scale-safe since 2026-07-13; 3-tier method resolution |
| Sell + realized gains | ✅ Complete, tested | Preview + post; per-lot disposal events |
| Cash dividend (+ withholding) | ✅ Complete | Dividend defaults resolution; withholding legs |
| Reinvested dividend | ✅ Complete | Creates lot with `reinvested_dividend` event kind |
| Dividend via T212 import | ✅ Complete | Cash dividends only — an imported *reinvestment* arrives as separate dividend + buy rows, which happens to compose correctly |
| Crypto | ⚠️ Partial | Ledger-level tracking works (kind `crypto`, scale ≤ 24, cash scale ≤ 12). The lot engine is commodity-kind-agnostic, so Buy/Sell on a crypto commodity works **via API** — but instruments can only be created as `security` commodities, so there is no instrument/UI entry point, no crypto instrument type, and no crypto price source |
| Stock splits / reverse splits | ❌ Missing | `split_adjustment` lot-event kind exists in schema only; no code references it. Backlog T-34 notes the lot-mutation design gap |
| Mergers, spin-offs, ticker changes | ❌ Missing | Event families defined for the (dormant) suggestion pipeline; no structural handling |
| Delisting / fund closure / write-off | ❌ Blocked | See P4 — zero-proceeds disposal is impossible |
| Return of capital | ⚠️ Cash-only | Postable as cash income via the suggestion contract's `dividend_income` shape; **basis reduction** (the economically correct treatment) is not implemented |
| Short selling | ❌ By design | Oversell → `ErrInsufficientLots`; no short-lot model. Reasonable for the niche; document it |
| Fees/commissions on trades | ⚠️ Folded in | No first-class fee field — buys book fees into cost basis, sells net them from proceeds. Defensible accounting, but gains won't reconcile line-for-line against broker statements and fee spend is invisible as an expense category |
| Recurring/scheduled transactions | ❌ Planned | Roadmap R9; the reserved draft-producer workflow is the right hook and drafts are ready for it |
| Provider events → suggestions → auto-post | ⚠️ Dormant | Accept/ignore/automation-rules/auto-post surface all works (tested), but nothing produces events — backlog T-34. Only `dividend_income`-shaped proposals can post; structural kinds are rejected loudly (correct) |

## 4. API-integration readiness

**As a server (letting other software call Rekenraam):**
- Strong foundation: spec-first OpenAPI (source of truth, generated TS
  client), `/api/v1` versioning, uniform error envelope with stable codes,
  cursor pagination, request IDs threaded into the audit trail.
- **Gap: no programmatic auth.** The only credential is the session cookie +
  CSRF header — a script or third-party tool must emulate a browser login.
  Personal-access tokens (scoped, revocable, hashed at rest like session
  tokens) are the single biggest enabler for integrations, and cheap given
  the existing auth-session infrastructure.
- Gap: no idempotency-key support on general mutations (`POST
  /transactions` twice = two transactions). The import path solves this with
  fingerprints; an `Idempotency-Key` header honored on financial mutations
  would extend the guarantee to API clients.
- Gap (R3-adjacent): no export endpoints yet (CSV/QIF export is the next
  roadmap item and is also the integration story for downstream tools).

**As a client of provider APIs:**
- The adapter pattern is proven twice (FX providers behind a registry;
  T212 behind staged fetch/commit with BYO-key). Adding IBKR Flex Query or
  GoCardless/SimpleFIN (already roadmap candidates) fits without
  architectural change.
- **Gap: no security-price provider.** The registry is FX-only
  (ECB/Frankfurter/ExchangeRate-API/OXR); instrument valuations depend
  entirely on trade-implied and manual prices, so unrealized gains go stale
  the moment you stop trading an instrument. The `PriceProvider` interface
  exists, unimplemented — same pattern as T-34's dividend/CA providers.
- Gap: `InstrumentSearchProvider`, `DividendProvider`,
  `CorporateActionProvider` are declared, never implemented (T-34 tracks
  this; a data source hasn't been chosen).

## 5. Refactoring suggestions (highest leverage first)

1. **Promote aligned scaled-integer arithmetic into `exact`.** The
  scale-mixing bug family (F1/F4/F5/F6, all fixed) had one root cause:
  int64+scale math done ad hoc outside a helper. Today there are two
  near-identical helpers — `scaledAmount` (app) and `scaledInteger` (db) —
  plus residual inline pow10 alignment in `ListRealizedGains` and
  `PositionsWithGains`. One `exact.ScaledInt` with add/sub/align/cmp and an
  overflow-checked `Int64()` would make the bug class structurally hard to
  reintroduce. Add the rule to the ledger-invariants skill afterward.
2. **Split `db/investments.go` (~2,600 lines).** Natural seams: lot engine
  (create/dispose/simulate), instruments, gains reporting, provider
  events/suggestions/rules. Same for `app/investments.go` (~2,100).
3. **Flatten the baseline migration while schema freedom lasts.** The 0001
  file preserves the pre-beta chain verbatim: DROP/re-CREATE trigger pairs,
  ALTER+backfill UPDATEs that run over empty tables, and runtime legacy
  fallbacks in Go (`tableHasColumn`, `listLegacyPriceObservations`) for a
  pre-baseline shape no database has. A declarative rewrite plus deleting the
  legacy read path removes real confusion (this audit initially flagged the
  duplicate triggers as a defect). Do it before first release freezes the
  schema.
4. **Consolidate error-writer duplication.** `writeTransactionServiceError`,
  `writeLedgerServiceError`, `writeInvestmentServiceError`,
  `writePricingServiceError` each re-map overlapping error sets (the missing
  `LedgerOverflowError` arm found last audit was this duplication biting).
  One table-driven mapper with per-domain extensions would keep new sentinel
  errors from being forgotten in one of four places.
5. **Merge `periodScopedRefsFromTransaction`/`FromRecord`** (identical logic
  over two types) and similar record/domain double-walks.
6. **Report read-model layer before R2 grows.** Every balance/report request
  loads all postings into Go (`current_transaction_versions` is a correlated
  subquery view), and `NetWorthSeries` repeats the full scan per bucket —
  O(buckets × postings). Correct-by-construction and fine at personal scale,
  but R2's charts multiply query counts. An incremental running-balance table
  or memoized as-of snapshots is the eventual answer; at minimum, compute a
  date-bucketed series in one pass instead of N.

## 6. Improvement suggestions (operational)

- **Scheduled backups.** `BackupSQLiteDatabase` (VACUUM INTO + verification)
  exists but is reachable only from CLI recovery. A daily scheduled backup
  with retention and a surfaced last-backup-status is table stakes for
  self-hosted financial data, and the background-work queue is already there
  to run it.
- **Trial-balance self-check.** A periodic (or on-demand, `/api/v1/health`-
  adjacent) integrity job asserting: per-commodity posting sums equal zero
  across every posted transaction; lot remaining quantities reconcile with
  holding-account balances; `PRAGMA integrity_check`/`foreign_key_check`
  clean. Cheap to run at this scale, and turns any future silent-corruption
  bug into a loud alarm instead of an audit finding.
- **Dead-letter visibility for background work** (pairs with P7): max
  attempts, a failed-items view in pricing health, manual retry.
- **Worker observability**: FX refresh runs are recorded in DB but there are
  no counters/log summaries for the import fetch worker's stage progress;
  a `/health`-style worker status would shorten "why is my import stuck"
  debugging.
- **Price provenance surfacing**: derivation JSON and mixed-vintage warnings
  are recorded but not exposed in the pricing API responses — R11 UI will
  want them; expose now, cheaply.

## 7. Features worth adding (not already on the roadmap)

Roadmap/backlog already cover: reports (R2), export (R3), CSV import + saved
mapping profiles (R5 — fold P1/P2 into it), budgets (R8), recurring (R9),
projections (R10), pricing UI (R11), return analytics (R13), second broker
provider, import rules, and the T-34 provider-event producer. Beyond those:

1. **Corporate-action engine** (split/merger/spin-off/ticker-change lot
   mutations + delisting write-off). The schema stubs exist; T-34 covers the
   *feed*, but the lot-mutation design is the harder half and is what blocks
   splits even as *manual* entries today. A manual "record a split" flow
   (no provider needed) would close the most common gap first.
2. **Zero-proceeds write-off** (P4) — small, unblocks closed funds.
3. **API tokens / personal-access tokens** — unblocks all programmatic use.
4. **First-class trade fees** — a fee field on Buy/Sell/dividend flowing to a
   designated expense account, keeping basis/proceeds reconcilable against
   broker statements.
5. **Return of capital as basis reduction** — small once the lot engine has
   a basis-adjustment event (shares `manual_adjustment` machinery).
6. **Security-quote provider** — one BYO-key quote source keeps unrealized
   gains honest between trades; the `PriceProvider` seam is already there.
7. **Crypto as a first-class instrument type** — the lot engine already
   handles it; what's missing is the instrument type, an entry point, and a
   price source.
8. **Price void endpoint** (P3) — prerequisite to trustworthy R11.

---

## Bottom line

The core ledger is in genuinely good shape: the invariants that matter
(exact arithmetic, per-commodity balancing, append-only versions with paired
audit events, reconciliation guards on every mutation path, atomic
import idempotency) are implemented, enforced at multiple layers, and now
regression-tested. This pass found no new money-corruption paths. The
material findings are at the edges: two import-parsing localization bugs that
directly threaten the product's own target audience (P1, P2 — fix with or
before R5), two completeness holes with correctness flavor (P3 price voiding,
P4 write-offs), and a set of dormant-but-sound investment-automation
scaffolding waiting on T-34. The highest-leverage engineering investments are
the `exact.ScaledInt` consolidation (structurally retires the worst historic
bug class) and personal-access tokens (unlocks the integration story).

Suggested order: P1+P2 (data quality for the target user) → P4+P3 (small,
closes real gaps) → `exact.ScaledInt` refactor → scheduled backups +
trial-balance check → manual split entry → API tokens.
