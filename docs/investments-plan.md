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

**Backend exists and is wired** (`RegisterRoutesWithAuth`, 24 investment routes):

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

**OpenAPI: unverified.** Confirm investment paths are in
`backend/api/openapi/openapi.yaml` with generated TS types before building the UI
(same convention the import endpoints violated — T-07). If absent, add them; do
**not** hand-roll a second typed client.

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
- **I-02:** either **implement** `lifo` (`ORDER BY opened_on DESC, id DESC`) and
  `average_cost` (single synthetic weighted-average cost) in
  `disposeLotsWithAuditTx`, **or** narrow the accepted method set to
  `{fifo, specific_lot}` and make `lifo`/`average_cost` return a clear
  `NOT_IMPLEMENTED` validation error at profile-save time. **Decision:** ship
  `fifo` + `specific_lot` working, gate `lifo`/`average_cost` behind a clear error,
  and file the remaining methods as a follow-up (I-03). Do not let a stored method
  silently disagree with disposal behaviour.
- **Tests:** oversell rejected (`ErrInsufficientLots`); FIFO order asserted;
  specific-lot disposal validates ownership/quantity; a `lifo` profile-save (or
  sell) returns the chosen error rather than silently doing FIFO; gains amounts are
  server-computed and balance.
- **Acceptance:** no API call can dispose lots in a way that contradicts the
  account's declared cost-basis method; unsupported methods fail loudly.

### Slice 2 — Read-only investments UI (positions, lots, events)

The "see my portfolio" surface — safe because it is read-only.

- `routes/app/investments`: positions table (instrument, quantity, avg cost, market
  value if a price exists, unrealized gain), lot drill-down, transaction/event
  history. Investment nav link in the app shell.
- Typed API client from the OpenAPI spec (add paths first if missing).
- Empty/loading/error/success states per screen; i18n via Paraglide; a11y + mobile
  per the app's existing boundaries.
- **Tests:** e2e render of positions/lots from a seeded book; empty-state; a
  position with no current price shows cost only, no fabricated market value.
- **Acceptance:** a user with seeded buys/dividends sees correct holdings, lots,
  and history with no way to mutate state.

### Slice 3 — Buy / sell / dividend entry UI

The mutating surface — depends on Slice 1's gate.

- Buy form (instrument autocomplete → quantity, price, fees, cash account).
- Sell form: quantity + cost-basis method shown; **server computes the
  allocation/gain** and the UI previews it (`specific_lot` shows a lot picker;
  fifo/lifo show the derived allocation read-only). Realized gain displayed from
  the server, never entered.
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

1. **Confirm OpenAPI coverage** of the 24 investment routes before Slice 2; add if
   missing.
2. **Average-cost semantics** (I-03) if implemented later: single weighted-average
   lot vs. preserving individual lots with an average disposal cost — decide
   against the reporting requirements, not ad hoc.
3. **Market price availability** for unrealized gains depends on the pricing
   backend having instrument prices (distinct from FX). Read-only UI must degrade
   gracefully (cost-only) when no price exists rather than fabricate one.
