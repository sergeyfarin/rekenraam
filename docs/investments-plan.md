# Investments Implementation Plan (UI + correctness gate)

A plan to take the **investments backend** (already substantial — ~4,300 lines
across `api/app/db`) to a **fully working feature including UI**, and to close the
**cost-basis correctness gaps** that must be fixed *before* any sell flow is
exposed. This is the prerequisite the Trading 212 online-import plan is now
deferred behind (`docs/trading212-import-plan.md`).

Realises roadmap **R12 (Investments UI + gains reporting)**. Written for the
Sonnet model to implement, grounded in the actual code as of 2026-06-27.

Status: Slice 1 (correctness gate) **complete**. Slice 2 (sell UI) next. Last updated 2026-06-27.

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
`average_cost`, `specific_lot` — each of which consumes lots and **determines the
realized gain** for the sale. There is exactly **one authoritative method per sale**:
the disposal physically reduces specific lot quantities, so a single sale cannot have
two contradictory cost bases. (Multi-method *reporting* is a separate, deferred
concern — see "Future" below.)

> **What "realized gain" means here today (verified).** The sell transaction posts
> **four legs** — security out, security→trading, cash in, cash←trading
> (`backend/internal/app/investments.go:790`) — and there is **no realized
> gain/loss account**; the account taxonomy has no gain/loss kind
> (`account_class IN (asset, liability, equity, income, expense)` only). Realized
> gain is therefore **computed/recorded from the disposal records** (cash proceeds −
> disposed cost basis), **not posted** to a dedicated gain account. The method drives
> the *disposed cost basis*, which drives the gain figure shown/stored — it does not
> add a ledger posting. **Posting realized gains to a gain/loss account is a separate
> future decision** (needs a new account kind/mapping + a sell transaction-shape
> change); tracked as **I-04**. This plan computes and surfaces gain; it does not
> change the transaction shape.

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

### `average_cost` representation — **DECIDED: per-lot, averaged at disposal (model 2)**

This is **decided up front**, not left inside the implementation slice, because the
math mutates persisted lot state and must not be improvised. Two models were
considered:

1. **Pooled (Section-104-style):** a single pooled cost + quantity per (account,
   instrument); sales dispose at the pool average and collapse individual lots.
   Loses per-lot identity that fifo/lifo/specific_lot need on the same tables.
2. **Per-lot with averaged disposal cost (chosen):** keep individual lots (as
   today); on an average-cost sale, compute one weighted-average unit cost across
   open lots and dispose the sold quantity across lots at *that pooled rate*.

Model 2 wins: it coexists with the other three methods on the existing
`investment_lots` tables, keeps the lot read model intact, and leaves any
pooled-reporting view derivable. **No further confirmation gates Slice 1** — the
representation is fixed; only the reporting *views* (Slice 4 / I-03) remain open.

### `average_cost` disposal algorithm + persistence invariants (specify, don't improvise)

The existing helper `proratedCostBasis` (`backend/internal/db/investments.go:1599`,
used by `disposeLotTx`) prorates basis **within a single lot** by truncating integer
division (`Quo`), and `nextRemainingCost = remaining − disposed` keeps that one lot
self-consistent. Average-cost is different: it disposes across **multiple** open lots
at a **pool** rate, so disposed basis must be distributed back over lots **and the
truncation residual assigned** so nothing leaks. The implementer MUST enforce these
invariants (and test each):

1. **Pool rate (no float):** `pooled_remaining_basis = Σ lot.remaining_cost_basis`;
   `pooled_remaining_qty = Σ lot.remaining_quantity` over open lots for
   (account, instrument). The total disposed basis for a sale of quantity `q` is
   `disposed_basis_total = pooled_remaining_basis × q ÷ pooled_remaining_qty`
   (big.Int, single division, **truncate — `Quo`, matching `proratedCostBasis`**;
   do not switch to half-up here or the residual-assignment rule would need to
   absorb a negative residual on some inputs — truncate is the safe consistent
   choice and must be pinned in a test).
2. **Distribution across lots (FIFO order for determinism):** consume `q` across open
   lots oldest-first (only to decide *which lots' quantity* decrements — the *rate*
   is the pool rate, not each lot's own). Each lot's disposed basis is its share of
   `disposed_basis_total`.
3. **Residual assignment (exactness):** because integer division truncates, the sum
   of per-lot disposed basis may be 1..n minor units short of `disposed_basis_total`.
   Assign the residual deterministically (e.g. to the last lot touched) so that:
   **`Σ disposed_basis = disposed_basis_total`** exactly, and for every lot
   **`remaining_cost_basis_new = remaining_cost_basis_old − its_disposed_basis ≥ 0`**.
4. **Pool conservation (the key invariant):** after the sale,
   `Σ remaining_cost_basis + disposed_basis_total == pooled_remaining_basis_before`
   exactly (no minor unit created or destroyed). A lot whose remaining quantity hits
   zero is `closed`; its remaining basis must be zero at that point.
5. **Disposal records + events:** each touched lot still writes an
   `investment_lot_events` `disposal` row (as `disposeLotTx` does) with its share of
   quantity and basis, so the history remains per-lot and any method can be
   recomputed later (see "Future").

**Scale constraint for average-cost:** `disposeLotTx` already guards
`quantityScale != lot.RemainingQuantityScale` and returns an error. The
average-cost path must enforce that all open lots for (account, instrument) share
the same `quantity_scale` before computing the pool; if scales diverge (unlikely
given `HoldingAccount.QuantityScaleOverride`, but possible via direct lot edits),
fail loudly rather than silently mis-compute. A test with mismatched-scale lots
must confirm the error path.

**Tests (Slice 1):** a 3-lot average-cost sale matches a hand-computed pool average;
residual lands deterministically and `Σ disposed + Σ remaining == original pool`
to the minor unit; a sale that exactly closes lots leaves zero remaining basis; a
sale larger than the pool returns `ErrInsufficientLots`; average-cost and FIFO over
the same lots produce *different* disposed-basis totals (proving the method actually
changed behaviour); mismatched quantity scales across open lots produce a hard error.

### Future — multi-method *reporting* (deferred, design-only here)

The user noted that tax vs. performance reporting can legitimately need *different*
methods. This is real but does **not** mean multiple authoritative postings. The
seam, recorded now so the data model does not preclude it later:

- The **authoritative** method (above) drives lot disposal and the realized-gain
  figure computed from it. Unchanged.
- **Analytical reports** re-derive realized/unrealized gains under *alternative*
  methods directly from the immutable **lot + disposal history**, at report time,
  **without mutating lots**. A "gains under average-cost vs. FIFO" comparison is a
  read-side computation. (Neither the authoritative nor the analytical path posts a
  gain to a ledger account today — gain is computed, not posted; see the realized-
  gain note above and I-04.)
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
    lots (pooled cost ÷ pooled quantity). **Representation is decided** (per-lot,
    model 2) and the **persistence invariants + residual rules are specified** in
    "Cost-basis methods" above — implement to those exactly; do not improvise the
    basis distribution.
  - `specific_lot` — explicit allocations (already planned in I-01).
  This supersedes the earlier "gate `lifo`/`average_cost`" decision: the user wants
  all four available. The disposal path still **rejects any value outside the
  implemented set** loudly (defence for future schema additions), but with all four
  built nothing is gated in normal use.
- **Required struct changes (name them explicitly so nothing is missed):**
  - `db.DisposeLotsParams` — add `CostBasisMethod string` (passed through from the
    resolver; `disposeLotsWithAuditTx` reads it to pick the branch).
  - `app.InvestmentTradeInput` — add `CostBasisMethod string` (the per-transaction
    override field from the sell/sell-preview request body).
  - `app.HoldingAccountInput` — add `CostBasisMethod string` (nullable/optional;
    empty = inherit global). Stored on the account (migration: add
    `cost_basis_method TEXT` nullable to `accounts` or a separate
    `holding_account_settings` table — prefer the accounts table nullable column to
    avoid a join in the resolver).
  - The `resolveCostBasisMethod(ctx, accountID, txnOverride string)` function lives
    in the app layer; it reads the holding account's method (new column) and falls
    back to `DefaultCostBasisProfile`. Both `Sell` and the new `PreviewSell` call it
    before constructing `DisposeLotsParams`.
- **3-tier method resolution (new — see "Cost-basis methods" below for the model).**
  The method for a given sale resolves: **per-transaction override → account default
  → global default**. Today only the *global* default exists
  (`DefaultCostBasisProfile`); **holding-account creation takes no method**
  (`CreateHoldingAccount`, `app/investments.go`) and the sell input has no explicit
  method field (only `LotAllocations`). Slice 1 adds the account-tier link and the
  per-sale override field, plus the resolver.
- **Tests (note on existing coverage):** `TestInvestmentLotsProrateRoundingIntoFinalDisposal`
  (`db/investments_test.go:258`) uses explicit `LotAllocations` and tests the *single-lot*
  truncation residual path; it does **not** test the multi-lot average-cost pool or
  LIFO ordering — those are new. Do not mistake existing coverage for the new
  method tests. The rounding test remains valid and must keep passing.
- **Tests:** oversell rejected (`ErrInsufficientLots`); FIFO **and** LIFO ordering
  asserted; average-cost disposal matches a hand-computed weighted average;
  specific-lot validates ownership/quantity; **method resolution** picks
  transaction-override over account-default over global-default; preview and commit
  produce identical allocations/gains; a method outside the implemented set is
  rejected at the disposal path (not silently FIFO); gains are server-computed and
  balance; mismatched quantity scales under average-cost fail loudly.
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
  server-computed allocation/gain. All four methods ship (Slice 1): **`fifo`** and
  **`lifo`** show the derived allocation read-only; **`average_cost`** shows the
  pooled-average disposed basis + resulting gain read-only; **`specific_lot`** shows
  a lot picker. The effective method follows the 3-tier resolution, overridable on
  the form per sale. Commit goes via `POST /investments/sell`. Realized gain comes
  from the server, never entered; preview and commit share the same backend calc, so
  what the user sees is what posts. (Gain is *computed/displayed* from the disposal,
  not posted to a gain account — see "Cost-basis methods".)
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
2. **Realized-gain posting (I-04):** gain is currently *computed* from disposals,
   not posted to a gain/loss account (none exists in the taxonomy). If product later
   wants gain as a ledger movement, that is a separate transaction-shape + account-
   mapping change — scope it with Slice 4 gains reporting, not inside this work.
3. **Market price availability** for unrealized gains depends on the pricing
   backend having instrument prices (distinct from FX). Read-only UI must degrade
   gracefully (cost-only) when no price exists rather than fabricate one.
4. **Migration placement for account-level `cost_basis_method`:** the nullable column
   belongs on **`account_versions`** (not `accounts`), following the existing pattern
   where all mutable account properties are versioned there. `accounts` is a thin
   identity table; `account_versions` is append-only and carries the fields. Add
   `cost_basis_method TEXT` nullable to `account_versions` (migration), add it to
   `db.AccountSpec` (used by both `CreateAccountParams` and `UpdateAccountParams`)
   and rebuild the `current_account_versions` view to include the new column.
   The resolver reads it from `current_account_versions` via the existing
   `AccountRecord` struct (add the field there too). Changing the account's
   method creates a new account version (normal versioning behaviour).
5. **`resolveCostBasisMethod` needs access to `DefaultCostBasisProfile`** (the global
   tier). `DefaultCostBasisProfile` is a DB method on `InvestmentRepository`, so the
   resolver lives in the app layer and takes the repository. Ensure `PreviewSell`
   and `Sell` are both routed through the same resolver function and not
   copy-pasted — the spec says "shared function so preview and commit never
   diverge."
