# Import Connection ↔ Account Linking Plan

Status: **planning only, nothing implemented**. Written 2026-07-01 while scoping
Trading 212 Slice 4 (`docs/trading212-import-plan.md`). This is a prerequisite
design for **B-T212-INVST** (investment lot import) and is written generically
because the same gap will hit every future online provider, not just Trading 212.

Governed by `docs/accounts-system-design.md` (institutions/accounts),
`docs/investments-plan.md` (holding accounts, lots), `docs/import-plan.md`
(the batch pipeline), and `docs/trading212-import-plan.md` (the concrete
consumer of this plan).

## Why this doc exists

Slice 4 of the Trading 212 plan was originally one line: "map order fills to
buys/sells... through the investment service." Implementing it surfaced a real
gap: **`import_connections` has no relationship to `accounts` or
`institutions` at all today.** Cash-movement rows sidestep this by asking the
user to pick a target account per row/batch at commit time (the existing
import-review UI). That mechanism is *insufficient* for investment lots
because:

- A **holding account** in this codebase is 1:1 with a single instrument
  (`CreateHoldingAccount` fixes `DefaultCommodityID` to one security —
  `backend/internal/app/investments.go:546`). A brokerage connection trades
  many instruments over time, so "the" holding account isn't a single value —
  it's a **growing set**, resolved per-instrument, that must stay stable
  across fetches (the *same* AAPL holding account must be reused every time
  AAPL appears again, so lots accumulate correctly).
- A **cash account** for trade settlement is realistically a **per-connection
  default** (a Trading 212 account has one base-currency cash balance), unlike
  the per-batch picker used for plain cash movements today.
- None of this has anywhere to live: `import_connections.config_json` is
  free-form and currently unused for account references, and there is no
  concept of "this account is fed by connection X."

This doc defines that model once, so B-T212-INVST (and every future brokerage
integration) builds on it instead of improvising per-provider.

## Scenarios this must cover

The user asked explicitly for these to be planned, not assumed:

1. **New connection, new accounts** — user adds a Trading 212 connection for
   the first time and has no pre-existing holding/cash accounts for it.
2. **New connection, existing accounts** — user already tracks their Trading
   212 cash balance and/or holdings manually (e.g. migrated from another
   provider or entered by hand) and wants to *attach* a connection to the
   accounts that already exist, so future fetches post into them instead of
   creating duplicates.
3. **Account lifecycle vs. connection lifecycle** — connections can be
   rotated (same connection row, new key), disabled (paused, not deleted),
   deleted (T-12: accounts and history survive, `connection_id` FK is
   `SET NULL`), or — the risky one — **deleted and replaced** with a new
   connection row pointing at the same real external account.

## Current state (verified against code)

- `import_connections` (migration `0007_online_import.sql`): `id`, `book_id`,
  `source_kind`, `display_name`, `secret_ciphertext`, `config_json`,
  `fetch_cursor`, `last_fetch_status`, `last_fetched_at`. No account or
  institution reference of any kind.
- `accounts`/`account_versions` already carry an optional `institution_id`
  (`backend/internal/db/accounts.go:42`) — purely descriptive grouping
  metadata (KMyMoney-style), never read by the import pipeline.
- `institutions` (`backend/internal/app/institutions.go`) is a standalone,
  versioned entity (name/kind/country/website) with no link to
  `import_connections` — a user could create an "Trading 212" institution
  today and manually tag accounts with it, but nothing wires that
  automatically.
- Holding accounts are created one-per-instrument
  (`InvestmentService.CreateHoldingAccount`, `investments.go:546`) via
  `AccountService.CreateAccount` with `AccountKind: "security_holding"` and a
  fixed `DefaultCommodityID`. There is no "find the holding account for
  commodity X under account-tree Y" lookup — callers must track the mapping
  themselves.
- **Dedupe fingerprint bakes in `connection_id`**
  (`buildTrading212Fingerprint`, `import_trading212.go:138`:
  `"trading212|<connection_id>|<providerID>"`). This means the *identity* of
  a connection row, not the real external account, is the dedupe key.

## The core design decision: two different kinds of account reference

Trading 212 (and brokerages generally) need exactly two account references,
and they have different lifetimes/cardinalities — treat them differently
rather than inventing one generic "linked accounts" bag:

### 1. Cash settlement account — one per connection, set at connect time

Add a real nullable FK column, not just a `config_json` blob (same rationale
`import_batches.connection_id` used: cheap, queryable, and it's load-bearing
for every trade/dividend the connection ever produces):

```sql
ALTER TABLE import_connections ADD COLUMN cash_account_id INTEGER
  REFERENCES accounts(id) ON DELETE SET NULL;
```

- Set (optionally) at connection create time, editable via the existing
  `PATCH /import-connections/{id}`.
- **Scenario 1 (new accounts):** the create-connection UI gets a step to
  either pick an existing asset account or create one inline (reusing
  `AccountService.CreateAccount`, `account_kind="bank"` or similar cash kind —
  no new account-creation machinery needed, just wiring the existing form
  into the connection flow).
- **Scenario 2 (existing accounts):** the same picker lists existing accounts;
  the user attaches the connection to their pre-existing cash account. Nothing
  about the account changes — it is not "converted" or specially marked,
  it is just referenced.
- If unset, BUY/SELL/DIVIDEND rows fall back to `needs_attention` exactly as
  they do today (no regression — connections created before this ships keep
  working, they just don't get investment auto-mapping until the user sets
  a cash account).

### 2. Holding accounts — one per (connection, instrument), created lazily

This is **not** a fixed set configured up front — it grows as new instruments
appear in the fetched history. New table:

```sql
CREATE TABLE import_connection_holdings (
  id INTEGER PRIMARY KEY,
  connection_id INTEGER NOT NULL REFERENCES import_connections(id) ON DELETE CASCADE,
  commodity_id INTEGER NOT NULL REFERENCES commodities(id) ON DELETE RESTRICT,
  holding_account_id INTEGER NOT NULL REFERENCES accounts(id) ON DELETE RESTRICT,
  created_at TEXT NOT NULL,
  UNIQUE (connection_id, commodity_id)
);
```

- **Resolution at fetch-worker/commit time:** given a BUY/SELL row's resolved
  `CommodityID` (instrument), look up `(connection_id, commodity_id)` in this
  table.
  - **Found:** reuse `holding_account_id` — same lot pool every time,
    required for cost-basis correctness.
  - **Not found (scenario 1, brand new instrument):** auto-create a holding
    account via `InvestmentService.CreateHoldingAccount` (name it from the
    instrument's display name, e.g. "Trading 212 — AAPL"), parented under
    an account named after the connection/institution if one exists (cosmetic
    grouping only), and insert the mapping row.
  - **Not found but the user already has a matching holding account
    (scenario 2):** before auto-creating, check for an existing
    `security_holding` account with `default_commodity_id = commodity_id` and
    no existing `import_connection_holdings` row pointing elsewhere. If
    exactly one match exists, surface it in the import preview as "link to
    existing account AAPL (#123)?" with an explicit user confirmation before
    the first commit creates the mapping row (auto-linking silently would
    risk merging two portfolios' lots if the user happens to have an
    unrelated same-ticker account) — **this one step is the only place a
    human decision is required**; every subsequent fetch for that instrument
    is fully automatic.
  - **Ambiguous (multiple candidate accounts):** row stays `needs_attention`
    with a warning naming the candidates; user resolves manually once via the
    same import preview UI already used for account/category selection.

This makes scenario 1 and scenario 2 the same code path with one branch
("did we find a reusable candidate") rather than two separate flows.

### 3. Institution — optional, cosmetic only

An `institutions` row is **not required** for any of the above to work. If
the user wants "Trading 212" to show up as a grouping institution in the
account tree, they create one via the existing institutions UI and set it on
the cash/holding accounts through the existing account edit form — orthogonal
to this plan, no new coupling needed. (Do not add
`import_connections.institution_id`; it would be a second, redundant way to
express something the account tree already expresses via
`accounts.institution_id`.)

## Connection lifecycle behaviors

| Action | Effect on `cash_account_id` / holdings map | Effect on accounts/lots |
|---|---|---|
| **Rotate key** (`PATCH`, same row) | Unchanged | Unchanged — this is the *safe* replace path: same `connection_id`, so dedupe fingerprints and the holdings map keep working. **This is the answer to "replace a connection": rotate the key on the existing row, don't delete-and-recreate.** |
| **Disable auto-refresh** (new in Slice 4a, see trading212-import-plan.md) | Unchanged | Unchanged — purely pauses the scheduler, not a data-affecting action |
| **Delete connection** (existing, T-12) | `import_connections` row removed; `import_connection_holdings` rows cascade-delete (mapping only, not the accounts themselves — `accounts` is untouched by the cascade since the FK is from the mapping table to `import_connections`, not the reverse); `import_batches.connection_id` `SET NULL` (existing behavior) | Cash/holding accounts and all posted transactions/lots are **untouched** — they simply stop being fed automatically. This matches T-12's existing "delete doesn't erase provenance" stance. |
| **Delete + create a brand-new connection row for the same real external account** | New row gets a new `connection_id`; fingerprints change (`trading212\|<new_id>\|<providerID>`); the old holdings map is gone (cascaded) | **Known limitation, not fixed here:** a full re-fetch will restage every historical movement as "new" against the fresh fingerprint namespace, and BUY/SELL rows will re-trigger the holding-account link-or-create step from scratch. This is why "rotate" (not delete+recreate) is the documented path for replacing credentials on the *same* real account. Surfacing a warning in the delete-confirmation dialog is the one product-facing change worth making alongside Slice 4a/4b: *"Deleting removes this connection's fetch history and holding-account links. If you plan to reconnect the same brokerage account, rotate the key instead (Edit → Rotate key) to keep history intact."* |

## What Slice 4b (Trading 212 investment lots) builds on top of this

With the above in place, `docs/trading212-import-plan.md` Slice 4b's job
narrows to:
1. Fetch order-fill/dividend detail (ticker/ISIN, quantity, price, fees) —
   still an open API-shape question, see that doc.
2. Resolve/create the `CommodityID` (instrument) from ticker/ISIN — this doc
   does not cover instrument matching itself (that's existing
   `InvestmentService.Search`/`CreateInstrument` territory); Slice 4b adds a
   thin "find by ISIN, else by ticker, else create" helper.
3. Resolve `HoldingAccountID` via `import_connection_holdings` (this doc).
4. Resolve `CashAccountID` via `import_connections.cash_account_id` (this doc).
5. Branch `CommitImportBatch` for these specific staged rows to call
   `InvestmentService.Buy`/`Sell`/`Dividend` instead of the generic
   `buildTransactionSpec` path.

## Migration plan

One migration, additive only, no data backfill needed (existing connections
simply have `cash_account_id = NULL` and an empty holdings map until the user
sets one):

```sql
-- 0008_import_connection_accounts.sql
ALTER TABLE import_connections ADD COLUMN cash_account_id INTEGER
  REFERENCES accounts(id) ON DELETE SET NULL;

CREATE TABLE import_connection_holdings (
  id INTEGER PRIMARY KEY,
  connection_id INTEGER NOT NULL REFERENCES import_connections(id) ON DELETE CASCADE,
  commodity_id INTEGER NOT NULL REFERENCES commodities(id) ON DELETE RESTRICT,
  holding_account_id INTEGER NOT NULL REFERENCES accounts(id) ON DELETE RESTRICT,
  created_at TEXT NOT NULL,
  UNIQUE (connection_id, commodity_id)
);
CREATE INDEX import_connection_holdings_connection_idx
  ON import_connection_holdings (connection_id);
```

## API surface additions

- `CreateImportConnectionInput` / `UpdateImportConnectionInput` gain an
  optional `CashAccountID *int64`. Validate it references an existing,
  non-archived account (any postable asset kind — do not restrict to
  `security_holding`, since the cash leg is ordinary cash).
- `ImportConnection` (the read DTO) surfaces `cash_account_id` so the
  frontend can show/edit it.
- No new endpoints needed for the holdings map — it is an internal
  implementation detail of the fetch/commit path, not user-editable directly.
  (If a mismatch ever needs manual correction, that's a follow-up, not part
  of this plan — flag as a backlog item if Slice 4b implementation finds a
  real need.)

## Risks / open questions

1. **Multi-currency cash settlement.** Trading 212 lets a single account
   trade instruments priced in EUR/USD/GBP while holding one base-currency
   cash balance (FX happens inside the provider). This plan assumes **one**
   `cash_account_id` per connection, in the account's own currency; a trade
   in a different currency needs its cash leg converted the same way the
   existing cash-movement rows already handle currency (via
   `AccountRepository.CurrentCurrencyByCode` / the batch's normal currency
   resolution) — not a new mechanism, but confirm this against real Trading
   212 payloads in Slice 4b before assuming it's actually single-currency.
2. **Ambiguous existing-holding-account matching** (scenario 2) is a
   heuristic (match by `default_commodity_id`), not a guarantee. If a user
   has two unrelated `security_holding` accounts for the same instrument
   (e.g. tracking AAPL in two real brokerages manually before ever
   connecting Trading 212), the plan requires an explicit one-time user
   choice rather than guessing — by design, not an oversight.
3. **Delete+recreate dedupe break** (above) is documented, not solved. A real
   fix (carrying a fingerprint namespace forward across a connection
   replacement) would need a "merge/relink connection" operation that
   rewrites `import_commit_identities` and `import_connection_holdings`
   under a new connection id — out of scope until someone actually hits it;
   tracked as a backlog item once Slice 4a/4b ship (see
   `docs/backlog.md`).
