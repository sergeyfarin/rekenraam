# Investments Implementation Plan (UI + correctness gate)

A plan to take the **investments backend** (already substantial — ~4,300 lines
across `api/app/db`) to a **fully working feature including UI**, and to close the
**cost-basis correctness gaps** that must be fixed *before* any sell flow is
exposed. This is the prerequisite the Trading 212 online-import plan is now
deferred behind (`docs/trading212-import-plan.md`).

Realises roadmap **R12 (Investments UI + gains reporting)**. Written for the
Sonnet model to implement, grounded in the actual code as of 2026-06-27.

Status: planning. Not yet started. Last updated 2026-06-27.

---

## Why this comes before Trading 212

Trading 212 is a brokerage. Importing it meaningfully requires booking instrument
buys/sells as **lots** and dividends as **investment events** — exactly the
machinery the investments feature owns. The Trading 212 plan deliberately defers
instrument lots (`B-T212-INVST`) precisely because the investments UI and the
cost-basis correctness work below are not done. So: **finish investments, then
return to Trading 212** and lift the `needs_attention` restriction on order rows.

---

## Current state (verified against code, not docs)

**Backend exists and is wired** (`RegisterRoutesWithAuth`, 23 investment routes):

- Instruments CRUD + search/autocomplete (`/investments/instruments`,
  `/investments/search`).
- Holding accounts (`/investments/holding-accounts`).
- Buy / sell / dividend / reinvested-dividend
  (`/investments/{buy,sell,dividend,reinvested-dividend}`), each going through
  `TransactionService` with commodity-trading balancing and half-up implied-price
  rounding.
- Lots & positions read models (`/investments/lots`, `/investments/positions`).
- Cost-basis profiles (`/investments/cost-basis-profiles`) and dividend defaults
  (`/investments/dividend-defaults`).
- Provider events + reviewable suggestions + automation rules
  (`/investments/events`, `/event-suggestions/*`, `/automation-rules`).

**Frontend: none.** No `routes/app/investments`, no investment API client. This is
the bulk of the work.

**OpenAPI: missing as of 2026-06-27 (verified).** The spec lives at
`api/openapi/openapi.yaml` (repo root, **not** `backend/api/...`) and contains
**zero** investment paths, while all 23 routes are wired in Go
(`backend/internal/api/health.go:149`). **Required pre-Slice-2 task:** add path
items + schemas for all 23 (24 after the sell-preview endpoint below) investment
routes and generate TS types — same convention the import endpoints violated
(T-07). Do **not** hand-roll a second typed client.

### Correctness gaps that gate the sell UI (found while reviewing)

Both the roadmap and `implemented.md` say "verify FIFO lot-matching is enforced
server-side before exposing sell flows." That verification is done — and it found
**two real holes** (filed as I-01, I-02 below):

1. **Explicit lot allocations bypass the cost-basis method.**
   `disposeLotsWithAuditTx` (`db/investments.go:899`) enforces FIFO only on the
   *implicit* path (`ORDER BY opened_on, id`, with `ErrInsufficientLots` guarding
   oversell). When the sell request carries `LotAllocations`
   (`app/investments.go:802`), the backend disposes **exactly those lots with no
   check** that the selection is permitted for the account's cost-basis method.
   A client could sell newest-first under a "FIFO" account. **A sell UI that lets
   the user pick lots makes this reachable.**

2. **`lifo` / `average_cost` / `specific_lot` are accepted but not implemented.**
   The cost-basis method is validated against
   `{fifo, lifo, average_cost, specific_lot}` (`app/investments.go:1267`) and
   stored on a profile, but **disposal only ever does FIFO**. A user can save a
   "LIFO" profile and a sell will silently dispose FIFO — a correctness/trust bug.

These must be resolved (implement or constrain) **before** Slice 3 (sell UI).
The cheapest honest fix is in Slice 1; see there.

---

## Delivery slices

Each slice leaves the app runnable, tested, documented (`implemented.md`,
`roadmap.md` R12, this file's status line) **in the same commit** as the code.

### Slice 1 — Correctness gate (backend only; no UI yet) — *do first*

Close the gaps above so the read models the UI will show are trustworthy.

- **I-01:** in the sell service path, when `LotAllocations` are supplied, validate
  them against the account's cost-basis method:
  - `fifo` / `lifo`: reject explicit allocations that don't match the method's
    ordering (or ignore client allocations and always derive them server-side from
    the method — **preferred**: the client sends quantity + method-derived
    preview; the server computes the authoritative allocation). Gains must be
    computed, never free-form.
  - `specific_lot`: explicit allocations allowed (that's the point), but still
    validated for ownership, open status, and sufficient remaining quantity.
- **New read-only sell-preview endpoint (required by the Slice 3 UI).** Today the
  only sell route is the **mutating** `POST /api/v1/investments/sell`
  (`api/health.go:163`), and the service computes disposals *while creating the
  transaction* (`app/investments.go:806`). The plan's "server computes the
  allocation/gain, the UI previews it" is unsatisfiable without posting first or
  duplicating cost-basis logic in the frontend. **Add `POST
  /api/v1/investments/sell/preview`** (and a `PreviewSell` service method) that runs
  the *same* allocation + gain computation against the same lots **without writing**
  — extract the allocation/gain calc out of the create path into a shared function
  both call, so preview and commit can never diverge. Returns the derived
  allocation, realized gain, and any `ErrInsufficientLots`/method error the real
  sell would raise.
- **I-02:** either **implement** `lifo` (`ORDER BY opened_on DESC, id DESC`) and
  `average_cost` (single synthetic weighted-average cost) in
  `disposeLotsWithAuditTx`, **or** narrow the *supported* method set to
  `{fifo, specific_lot}` and make `lifo`/`average_cost` fail loudly. **Decision:**
  ship `fifo` + `specific_lot` working; reject `lifo`/`average_cost` with a clear
  `NOT_IMPLEMENTED` error. **Reject at BOTH points, not just profile-save:** the
  schema CHECK already permits all four values
  (`0001_initial_schema.sql:1017`) and the validator accepts them
  (`app/investments.go:1267`), so existing rows and direct DB inserts can already
  hold `lifo`/`average_cost`. The **sell/disposal path must therefore also reject**
  an unsupported method — otherwise a pre-existing "LIFO" profile silently disposes
  FIFO. Profile-save rejection is UX nicety; disposal-path rejection is the
  correctness guarantee. File the remaining methods as a follow-up (I-03).
- **Tests:** oversell rejected (`ErrInsufficientLots`); FIFO order asserted;
  specific-lot disposal validates ownership/quantity; preview and commit produce
  identical allocations/gains for the same input; **a sell against an account whose
  stored method is `lifo`/`average_cost` is rejected at the disposal path** (not
  silently FIFO), even when the profile predates the validator change; gains amounts
  are server-computed and balance.
- **Acceptance:** no API call can dispose lots in a way that contradicts the
  account's declared cost-basis method; unsupported methods fail loudly **at
  disposal**, regardless of how the profile row was created; the sell preview
  matches the committed result exactly.

### Slice 2 — Read-only investments UI (positions, lots, events)

The "see my portfolio" surface — safe because it is read-only.

- `routes/app/investments`: positions table (instrument, quantity, avg cost, market
  value if a price exists, unrealized gain), lot drill-down, transaction/event
  history. Investment nav link in the app shell.
- Typed API client generated from the OpenAPI spec. **Adding the 23/24 investment
  paths to `api/openapi/openapi.yaml` is a required pre-Slice-2 task** (they are
  absent today — see "Current state"); the UI must not start until the client is
  generated.
- Empty/loading/error/success states per screen; i18n via Paraglide; a11y + mobile
  per the app's existing boundaries.
- **Tests:** e2e render of positions/lots from a seeded book; empty-state; a
  position with no current price shows cost only, no fabricated market value.
- **Acceptance:** a user with seeded buys/dividends sees correct holdings, lots,
  and history with no way to mutate state.

### Slice 3 — Buy / sell / dividend entry UI

The mutating surface — depends on Slice 1's gate.

- Buy form (instrument autocomplete → quantity, price, fees, cash account).
- Sell form: quantity + cost-basis method shown; the UI calls **`POST
  /investments/sell/preview`** (added in Slice 1) as the user edits and renders the
  server-computed allocation/gain (`specific_lot` shows a lot picker; fifo/lifo show
  the derived allocation read-only), then commits via `POST /investments/sell`.
  Realized gain comes from the server, never entered. Preview and commit share the
  same backend calc, so what the user sees is what posts.
- Dividend + reinvested-dividend forms using existing dividend-defaults
  (income/withholding).
- **Tests:** e2e buy→sell→gain shows server-computed realized gain; oversell
  blocked with a clear message; specific-lot picker disallows over-allocation;
  dividend applies defaults.
- **Acceptance:** a full buy → dividend → sell lifecycle works end-to-end through
  the UI with correct, server-authoritative gains and no method bypass.

### Slice 4 — Gains reporting + provider-event review UI

- Realized/unrealized gains report (over the existing read models); multi-currency
  presentation where positions span currencies; report snapshots where
  reproducibility matters (per product requirements).
- Provider-event **suggestion review** UI (`/event-suggestions/*`): corporate
  actions surface as reviewable suggestions; explicit automation rules required
  before any auto-posting (the backend already enforces this — expose it).
- **Acceptance:** realized/unrealized gains reconcile with the ledger; a corporate
  action arrives as a suggestion the user accepts/ignores, never auto-posted
  without a rule.

### Slice 5 (then) — Return to Trading 212

With investments working, reopen `docs/trading212-import-plan.md`:
- Lift the `B-T212-INVST` restriction: map Trading 212 order fills to buys/sells
  (lots) and dividends to investment events, routed through the now-UI-backed
  investment service.
- Everything else in that plan (credential store, durable fetch, dedupe) is
  unchanged.

---

## Cross-cutting rules (inherited)

- **Server-authoritative gains:** realized/unrealized gains are *computed* by the
  backend from lots, never accepted as free-form input. The UI displays; it does
  not decide.
- **No floating point:** quantities/prices/cost-basis are `exact.Coefficient` +
  scale throughout (already true in the backend; the UI must not round through
  float either).
- **Auditable:** trades/dividends go through `TransactionService`
  (`OriginType` set appropriately) and the hybrid audit model; lot disposals carry
  their own audit event (already implemented).
- **i18n / a11y / mobile:** all investment UI follows the same boundaries as the
  rest of the app.
- **OpenAPI-first:** typed client generated from the spec; no second hand-rolled
  client (the import path's deviation, T-07, is the anti-pattern to avoid).

## Risks / open questions

1. **OpenAPI coverage is absent** (verified 2026-06-27): add all 23 existing
   investment paths (+ the new sell-preview = 24) to `api/openapi/openapi.yaml`
   before Slice 2. Not optional.
2. **Average-cost semantics** (I-03) if implemented later: single weighted-average
   lot vs. preserving individual lots with an average disposal cost — decide
   against the reporting requirements, not ad hoc.
3. **Market price availability** for unrealized gains depends on the pricing
   backend having instrument prices (distinct from FX). Read-only UI must degrade
   gracefully (cost-only) when no price exists rather than fabricate one.
