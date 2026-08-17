# Technical Backlog

This is the **live** technical-debt list: items that are actionable now and are
not already scheduled as product work. It is intentionally short.

- **Feature sequence:** `docs/roadmap.md`.
- **Short-horizon queue:** `docs/todo.md`.
- **Shipped capability:** `docs/implemented.md`.
- **Resolved and deliberate decisions:** `docs/reviews/resolved-backlog-2026-07.md`.

Status legend: `[ ]` open, `[x]` closed (kept briefly with the fix and the
gate that prevents recurrence, then moved to `docs/reviews/`).

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

### T-44 Free-text payees never group in reports `[ ]`

**Files:** `backend/internal/app/transactions_validate.go` (payee resolution:
`payee_name` is stored as free text and only `payee_id` links a payee record);
`backend/internal/app/reports.go` (`Spending`, payee grouping).

A transaction may carry a `payee_name` with no `payee_id` — the create path
accepts the name as free text and only sets `payee_id` when the caller supplies
one. The spending report groups by `payee_id`, so every free-text payee falls
into the single "no payee recorded" group. The report still reconciles exactly
(the unattributed group is emitted, never dropped), but a user who typed payee
names without picking records sees one undifferentiated bucket instead of a
ranking. Grouping on the raw text is not the fix: it would duplicate rows across
spelling and casing variants. Needs a product decision on whether transaction
entry should resolve or create a payee record from a typed name, then the
report follows for free.

### T-42 TypeScript 7 upgrade blocked by `openapi-typescript` `[ ]`

**Files:** `frontend/package.json` (`typescript`, `openapi-typescript`);
`frontend/src/lib/api/schema.d.ts` generation step.

TypeScript 7.0.2 is released, but `openapi-typescript` 7.13.0 (latest) still
declares `peerDependencies.typescript: ^5.x` and emits `schema.d.ts` through
the TypeScript JS compiler API (`ts.factory`). TS 7 no longer exposes it, so
`pnpm run openapi:generate` fails immediately with
`TypeError: Cannot read properties of undefined (reading 'createKeywordTypeNode')`,
taking `dev`, `check`, and `build` down with it. The frontend is pinned to
TypeScript 6.0.3 until `openapi-typescript` ships TS 7 support; re-check on
each `openapi-typescript` release. The stale `typescript@7.0.2` and
`@typescript/typescript-*@7.0.2` entries were left in
`pnpm-workspace.yaml`'s `minimumReleaseAgeExclude` so the retry is a one-line
version bump.

## Closed

### T-43 `gofmt` drift in four backend files `[x]`

Fixed 2026-08-17. `gofmt -w` applied to
`backend/internal/app/transactions_service.go`,
`backend/internal/db/reconciliation.go`, `backend/internal/db/recovery.go`, and
`backend/internal/web/static_integration_test.go` (struct-field alignment, a
gofmt comment-block reindent, two missing trailing newlines — no behavior
change).

The drift was invisible because the validation contract was wider than the
script: `docs/developer-workflow.md` called for `gofmt -l .` and `go vet ./...`
alongside the tests, but `scripts/test-backend.sh` only ran
`go test -race ./...`, and CI runs the script. Both gates now live in the
script, so CI enforces them and local matches. Proven by
`./scripts/test-backend.sh` failing on unformatted input and passing on the
formatted tree.

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
