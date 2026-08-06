# Technical Backlog

This is the **live** technical-debt list: items that are actionable now and are
not already scheduled as product work. It is intentionally short.

- **Feature sequence:** `docs/roadmap.md`.
- **Short-horizon queue:** `docs/todo.md`.
- **Shipped capability:** `docs/implemented.md`.
- **Resolved and deliberate decisions:** `docs/reviews/resolved-backlog-2026-07.md`.

Status legend: `[ ]` open.

## General

### T-34 No producer of investment provider events/suggestions `[ ]`

**Files:** `backend/internal/app/investments.go` (`DividendProvider`,
`CorporateActionProvider` interfaces, declared but never implemented or
referenced); `investment_provider_events` / `investment_event_suggestions`
tables (`backend/migrations/0001_initial_schema.sql`).

Verified 2026-07-15 (Workstream 3 investment test coverage): nothing in the
codebase writes to `investment_provider_events` or
`investment_event_suggestions` — no fetcher, worker, or import path creates
them. The review UI and the accept/ignore/automation-rules endpoints all work
correctly (fixed the same day — see below), but have no data to act on until
a producer exists. `docs/product-requirements.md` lists "provider events and
reviewable suggestions" as a real requirement. Needs: a chosen data source
(no candidate picked yet), a fetch/detection design, and — separately — a
lot-mutation design for structural corporate actions (split, merger,
spin_off, ticker_change, delisting, `corporate_action`), which
`AcceptSuggestion` currently rejects outright since only the
`dividend_income` proposed-transaction kind (dividend, distribution,
cash_in_lieu, return_of_capital) is implemented.

### T-37 Price observations can never be voided `[ ]`

**Files:** `backend/internal/db/pricing.go`, `backend/internal/app/pricing.go`,
`backend/internal/api/pricing.go`.

`price_observations.voided_at` exists and every read filters on it, but no
repository/service/endpoint ever sets it — the "correct a price by
superseding **or voiding**" invariant is half-implemented, and a poisoned
observation cannot be retired from historical listings or derivations.
Audit P3. Natural home: R11 pricing UI, but the endpoint can land earlier.

### T-38 Zero-proceeds disposal (write-off) is impossible `[ ]`

**Files:** `backend/internal/app/investments.go` (`validateTradeInput`
requires `CashAmountValue > 0`; repository `DisposeLots` is unexposed).

A fund closure, worthless delisting, or any total-loss position cannot be
recorded: shares stay in open lots forever and the loss never reaches
realized gains. Audit P4. Needs a small product decision (which account the
loss books against) plus either `CashAmountValue == 0` support on Sell or a
dedicated write-off endpoint over the existing disposal engine.

### T-39 Background work retries forever `[ ]`

**Files:** `backend/internal/db/background_work.go`,
`backend/internal/app/pricing_worker.go` (`retryDelay`).

Exponential backoff caps at 6h but nothing ever gives up; a permanently
failing item retries indefinitely. Add a max-attempts → `failed` transition
(status exists), surface in pricing source health, and provide a manual
re-enqueue. Audit P7.

### T-40 `TriangulationMaxHops` policy knob is not honored `[ ]`

**File:** `backend/internal/app/pricing_refresh.go` (`refreshFXTarget`).

The stored policy value is only checked for `<= 0`; derivation is hard-coded
to one hop, so a configured `2` silently behaves as `1`. Implement multi-hop
or clamp/remove the knob. Audit P6.

### T-41 Consolidate scaled-integer arithmetic into `exact` `[ ]`

**Files:** `backend/internal/app/ledger.go` (`scaledAmount`),
`backend/internal/db/investments.go` (`scaledInteger`, inline alignment in
`ListRealizedGains`/`PositionsWithGains`).

Every historical severity-1 money bug (2026-07-13 audit F1–F6 family) shared
one root cause: int64+scale math done outside a shared helper. Promote one
`exact.ScaledInt` (add/sub/align/cmp, overflow-checked `Int64()`), migrate
both helpers and the inline call sites, then update the ledger-invariants
skill to mandate it. Refactor recommendation §5.1 of the 2026-07-19 audit.

## Public-deployment security gates

These do not block private/local development. They must be complete before
allowing real financial data on an internet-exposed deployment.

### S-04 Username login throttle can cause owner lockout `[ ]`

**File:** `backend/internal/app/auth.go` (username-scope throttle).

An internet attacker can keep the known single-owner username rate-limited.
Design and implement an approved-device cookie or equivalent lockout-safe
throttle before public beta.

### S-06 Multi-factor authentication `[ ]`

**File:** `docs/product-requirements.md` (public-deployment requirement).

Public deployment with real financial data remains prohibited until MFA has an
approved design and implementation. Select and implement TOTP, WebAuthn, or an
approved equivalent before lifting this product gate.

### S-07 Authentication-event visibility `[ ]`

**File:** `backend/internal/api/auth.go`, `backend/internal/app/auth.go`.

Add privacy-conscious, durable or operator-consumable visibility for successful
and failed authentication events, including the proxy-aware client IP, so
operators can detect brute-force attempts and support incident response.
