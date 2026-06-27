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
   **Resolution (per product decision):** implement all four authoritative methods
   in Slice 1 rather than gating — see "Cost-basis methods" — plus a 3-tier
   (global → account → per-transaction) method selection.

These must be resolved (implement or constrain) **before** Slice 3 (sell UI).
The honest fix is in Slice 1; see there.

---

## Cost-basis methods (model + 3-tier selection)

The feature supports **four authoritative disposal methods** — `fifo`, `lifo`,
`average_cost`, `specific_lot` — each of which actually consumes lots and posts the
realized gain. There is exactly **one authoritative method per sale**: the disposal
physically reduces specific lot quantities and the ledger is double-entry, so a
single sale cannot post two contradictory cost bases. (Multi-method *reporting* is a
separate, deferred concern — see "Future" below.)

### 3-tier method selection

The method effective for a given sale resolves by precedence, most specific wins:

```
per-transaction override   (this sale only; optional)
        ↓ falls back to
account default            (per holding account; optional)
        ↓ falls back to
global default             (book-wide; the existing default cost_basis_profile)
```

What exists vs. what Slice 1 adds:

- **Global default — exists.** `DefaultCostBasisProfile` /
  `EnsureDefaultCostBasisProfile` (`backend/internal/db/investments.go:544`) already
  seed a default FIFO profile per book. Keep as the base tier.
- **Account default — new.** `CreateHoldingAccount`
  (`backend/internal/app/investments.go`) takes **no** method today. Add an optional
  cost-basis method (or `cost_basis_profile_id`) on the holding account so a "T212
  ISA" account can default to, say, average-cost while the rest of the book is FIFO.
  Store it on the account (migration), nullable → "inherit global".
- **Per-transaction override — new field.** The sell input carries `LotAllocations`
  but no explicit *method*. Add an optional `cost_basis_method` to the sell (and
  sell-preview) input; when set it overrides both defaults for that sale only.
  `specific_lot` is effectively always a per-transaction choice (you pick lots),
  but an explicit method field makes fifo/lifo/average overridable per sale too.
- **Resolver** — one small server-side function `resolveCostBasisMethod(accountID,
  txnOverride)` used by **both** sell-preview and sell-commit so the previewed and
  posted method are always identical.

### `average_cost` representation (decide consciously)

Two valid models; pick one and document it, because it changes the lot read model
the UI renders:

1. **Pooled (Section-104-style):** maintain a single pooled cost + pooled quantity
   per (account, instrument); a sale disposes at the pool's average and reduces the
   pool. Individual lots collapse into the pool. Simplest for average-cost
   jurisdictions; but it loses per-lot identity, which fifo/specific_lot need — so a
   book mixing methods across accounts must keep per-lot data and derive the pool.
2. **Per-lot with averaged disposal cost:** keep individual lots (as today), but on
   an average-cost sale compute one weighted-average unit cost across open lots and
   reduce each lot's remaining quantity pro rata at that cost.

**Recommendation:** model (2) — keep per-lot data always, compute the average at
disposal time. It coexists with fifo/lifo/specific_lot on the same lot tables, keeps
the existing lot read model intact, and leaves pooled-reporting as a view. Confirm
against the reporting requirements before implementing (Slice 1 test asserts the
chosen model against a hand-computed example).

### Future — multi-method *reporting* (deferred, design-only here)

The user noted that tax vs. performance reporting can legitimately need *different*
methods. This is real but does **not** mean multiple authoritative postings. The
seam, recorded now so the data model does not preclude it later:

- The **authoritative** method (above) drives lot disposal + the gain that posts to
  the ledger. Unchanged.
- **Analytical reports** re-derive realized/unrealized gains under *alternative*
  methods directly from the immutable **lot + disposal history**, at report time,
  **without posting** anything. A "gains under average-cost vs. FIFO" comparison is
  a read-side computation, not a second ledger entry.
- Requirement on the data model **now**: keep the full per-lot acquisition and
  disposal history (already true) so any method can be recomputed after the fact.
  Do **not** collapse lots irreversibly (another reason to prefer average-cost
  model (2) above). This is the only thing this plan must protect; the reporting
  itself is designed when Slice 4's gains reporting is specified.

Tracked as **I-03** (was "implement remaining methods"; now repurposed to
multi-method analytical reporting, since the four authoritative methods ship in
Slice 1).

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
  (`backend/internal/api/health.go:163`), and the service computes disposals *while
  creating the transaction* (`backend/internal/app/investments.go:806`). The plan's "server computes the
  allocation/gain, the UI previews it" is unsatisfiable without posting first or
  duplicating cost-basis logic in the frontend. **Add `POST
  /api/v1/investments/sell/preview`** (and a `PreviewSell` service method) that runs
  the *same* allocation + gain computation against the same lots **without writing**
  — extract the allocation/gain calc out of the create path into a shared function
  both call, so preview and commit can never diverge. Returns the derived
  allocation, realized gain, and any `ErrInsufficientLots`/method error the real
  sell would raise.
- **I-02 — implement all four methods (decision: build, don't gate).** All four
  schema-permitted methods become real, authoritative disposal logic in
  `disposeLotsWithAuditTx`:
  - `fifo` — existing (`ORDER BY opened_on, id`).
  - `lifo` — `ORDER BY opened_on DESC, id DESC` (reverse FIFO).
  - `average_cost` — dispose against a single weighted-average cost across all open
    lots (pooled cost ÷ pooled quantity); reduce open-lot remaining quantities pro
    rata. Decide the **representation** consciously (see "Cost-basis methods"
    below) — pooled vs. individual-lots-with-averaged-cost — because it affects the
    lot read model the UI shows.
  - `specific_lot` — explicit allocations (already planned in I-01).
  This supersedes the earlier "gate `lifo`/`average_cost`" decision: the user wants
  all four available. The disposal path still **rejects any value outside the
  implemented set** loudly (defence for future schema additions), but with all four
  built nothing is gated in normal use.
- **3-tier method resolution (new — see "Cost-basis methods" below for the model).**
  The method for a given sale resolves: **per-transaction override → account default
  → global default**. Today only the *global* default exists
  (`DefaultCostBasisProfile`); **holding-account creation takes no method**
  (`CreateHoldingAccount`, `app/investments.go`) and the sell input has no explicit
  method field (only `LotAllocations`). Slice 1 adds the account-tier link and the
  per-sale override field, plus the resolver.
- **Tests:** oversell rejected (`ErrInsufficientLots`); FIFO **and** LIFO ordering
  asserted; average-cost disposal matches a hand-computed weighted average;
  specific-lot validates ownership/quantity; **method resolution** picks
  transaction-override over account-default over global-default; preview and commit
  produce identical allocations/gains; a method outside the implemented set is
  rejected at the disposal path (not silently FIFO); gains are server-computed and
  balance.
- **Acceptance:** all four methods post correct, server-computed gains; the
  effective method follows the 3-tier resolution; an unimplemented method value
  fails loudly at disposal; the sell preview matches the committed result exactly.

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
  server-computed allocation/gain. Per Slice 1's decision (`fifo` + `specific_lot`
  ship; `lifo`/`average_cost` are gated): **`fifo`** shows the derived allocation
  read-only; **`specific_lot`** shows a lot picker; an account whose stored method
  is **`lifo`/`average_cost`** surfaces the planned `NOT_IMPLEMENTED` error inline
  (the preview endpoint returns it) rather than rendering an allocation — so the UI
  never implies a method the backend won't honour. Commit goes via `POST
  /investments/sell`. Realized gain comes from the server, never entered; preview
  and commit share the same backend calc, so what the user sees is what posts.
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
2. **Average-cost representation:** pooled vs. per-lot-with-averaged-cost — decided
   in "Cost-basis methods" (recommend per-lot, model 2) but **confirm against the
   reporting requirements** before implementing in Slice 1; a Slice 1 test pins the
   chosen model to a hand-computed example.
3. **Market price availability** for unrealized gains depends on the pricing
   backend having instrument prices (distinct from FX). Read-only UI must degrade
   gracefully (cost-only) when no price exists rather than fabricate one.
