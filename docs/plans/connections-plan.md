# Connections Plan — banks, brokers, analytics providers

Status: **proposed** (2026-07-19). Feature plan for expanding online
connections beyond Trading 212 (R7, shipped). Governed by
`docs/product-requirements.md`; sequencing is decided in `docs/roadmap.md`
(see the pointer added under "Deliberately later"). Detailed designs below
for the two priority adapters — **IBKR Flex Query** and **EU/UK banks via
GoCardless Bank Account Data** — with an assessment matrix for everything
else that was evaluated.

## Ground rules (from product requirements, restated)

1. **Bring-your-own-key adapters, never coverage promises.** The user owns
   the provider relationship and credential; Rekenraam never proxies through
   project-operated infrastructure and never holds an intermediary/AISP
   license.
2. **Prefer free.** A provider that charges the *user* a modest fee (e.g.
   SimpleFIN ~$1.5/mo) is acceptable; a provider that would require the
   *project* to buy a commercial license (Morningstar) is rejected.
3. **No ToS-violating reverse engineering.** Unofficial APIs that require
   impersonating the official app or replaying a login/device-pairing flow
   (Trade Republic, DeGiro) are not adapter candidates; those brokers are
   served by CSV import profiles (R5). This is narrower than "undocumented":
   a public keyless endpoint that needs no credential and no impersonation
   (Yahoo Finance quotes) is allowed, labeled unofficial.
4. **Read-only.** No order placement (decided 2026-07-07, see
   `docs/reviews/competitive-analysis-2026-07.md` §4).
5. **Reuse the pipeline.** Every adapter lands in the existing shape:
   secretbox-encrypted credentials on `import_connections`, staged fetch via
   the durable background-work queue (lease/retry/continuation), rows into
   the staged preview → commit pipeline with fingerprint dedupe, investment
   rows through `InvestmentService` (real lots), account mappings via
   connection-scoped mapping tables. No second import path.

## Assessment matrix

Research date 2026-07; every "verify" must be re-checked at implementation
time (free tiers and signup policies shift).

| Provider | Type | Official API? | Cost to user | Auth shape | Verdict |
|---|---|---|---|---|---|
| **IBKR Flex Query** | Broker (global, incl. UK/EU) | ✅ official | Free | Token + query ID (user-generated, up to 1-yr expiry) | **Adapter — priority 1** (detailed below) |
| **GoCardless Bank Account Data** (ex-Nordigen) | Bank aggregation, EU + UK PSD2/OB | ✅ official | Free tier (verify current personal-use terms and per-account rate caps) | secret_id + secret_key → JWT; per-bank user consent (SCA), 90–180-day renewals | **Adapter — priority 2** (detailed below) |
| SimpleFIN Bridge | Bank aggregation, US | ✅ official | ~$1.5/mo paid by user | Setup token → access URL | Adapter — later (US persona; pattern identical to GoCardless but simpler) |
| Enable Banking | Bank aggregation EU | ✅ official | Commercial (free sandbox only) | eIDAS-backed app registration | Fallback only if GoCardless free tier closes; re-verify terms then |
| Tink / TrueLayer / Yapily / Salt Edge | Bank aggregation | ✅ official | Commercial | — | Rejected (project-side contracts) |
| Direct PSD2 bank APIs | Banks | ✅ | Free | Requires TPP/AISP license + eIDAS certs | Rejected permanently (license rule) |
| **Raisin / WeltSparen** | Savings marketplace | ❌ none public | — | — | **CSV/statement import profile (R5)** + manual accounts. Deposits sit at partner banks and are generally not reachable via PSD2 aggregators either. Revisit if Raisin ships an API |
| **Trade Republic** | Broker (DE/EU) | ❌ (community `pytr` is unofficial device-pairing reverse engineering) | — | — | **CSV/PDF import profile (R5)**; watch for an official API |
| **DeGiro** | Broker (EU) | ❌ (unofficial connectors are ToS-gray) | — | — | **CSV import profile (R5)** — DeGiro's Portfolio + Account exports are well-formed and popular |
| UK brokers: Hargreaves Lansdown, AJ Bell, interactive investor, Freetrade, Vanguard UK | Brokers | ❌ no retail APIs | — | — | **CSV import profiles (R5)**; Trading 212 (shipped) and IBKR cover the API-accessible UK brokers |
| **Morningstar** | Analytics/fund data | ❌ retail (commercial licensing only) | Commercial | — | Rejected as a connection. The underlying user needs (quotes, ratings-adjacent metadata, dividends) are served by the free quote/event providers below |
| **Wikifolio** | Social investing | Trading API exists but automation is ToS-restricted | — | — | No adapter. Wikifolio certificates have ISINs (DE000LS9…) and trade on Lang & Schwarz — they price like any other security once a quote provider covers L&S/German certificates |
| Alpha Vantage | Quotes + fundamentals | ✅ | Free 25 req/day (verify; adjusted series moved partly to premium) | BYO API key | Quote-provider candidate |
| Twelve Data | Quotes | ✅ | Free ~800 req/day, 8/min (verify EU-exchange coverage on free tier) | BYO API key | Quote-provider candidate — likely first pick |
| Financial Modeling Prep | Quotes + dividends + splits | ✅ | Free ~250 req/day (verify EU coverage) | BYO API key | **T-34 producer candidate** (dividend/split event feed) |
| Stooq | EOD quotes + indices (incl. EU) | quasi-official CSV endpoints | Free, keyless | none | Benchmark-series candidate for R13 |
| CoinGecko | Crypto prices | ✅ | Free demo-key tier | BYO API key | Crypto quote candidate (pairs with the crypto-instrument proposal) |
| **Yahoo Finance** | Quotes (broadest EU coverage) | ❌ undocumented (public, keyless — no app impersonation) | Free | none | **Adapter — ship** (decided 2026-08-05). Keyless `PriceProvider`, opt-in, labeled "unofficial" in the provider picker; breakage is expected and non-blocking because a BYO-key provider is always available as the fallback |

## Architecture: what all adapters share

The Trading 212 implementation established the shape; new adapters add
provider packages, not pipeline changes:

- `internal/onlinesource/<provider>/` — HTTP client: pagination, rate-limit
  backoff (`Retry-After`), page caps, typed responses, no DB access.
- A `ConnectionProber` (validate credentials before storing) and a fetch
  adapter registered with `ImportService`.
- Credentials in `import_connections.secret_ciphertext` (AES-256-GCM
  secretbox). Multi-field credentials are a JSON object inside the sealed
  payload (T212 precedent: single key; IBKR: `{token, query_id}`;
  GoCardless: `{secret_id, secret_key, …cached refresh token}`).
- Fetch runs on the durable background-work queue: staged cursors in
  `import_connections` columns or payload, lease/retry/continuation,
  terminal-vs-retryable error classification written to
  `import_batches.source_meta_json`.
- Rows stage into the preview/commit pipeline; fingerprints from provider
  stable IDs where they exist, content hash otherwise; commit identities
  guarantee idempotency.
- Account resolution: connection-scoped mapping tables
  (`import_connection_holdings` precedent). GoCardless needs the
  generalization to *cash* accounts described below.

**Shared prerequisite worth building once (small):** a
`import_connection_accounts` table mapping `(connection_id,
provider_account_ref)` → ledger `account_id`, replacing the single
`cash_account_id` column pattern for providers that expose multiple
accounts (IBKR multi-account structures, every bank connection).

---

## Detailed plan 1 — IBKR Flex Query

**Why first:** already the roadmap's chosen second provider; free; official;
token-based (fits BYO-key exactly); serves the investor persona; and its
statement is the richest source available (trades with explicit commissions,
dividends with withholding, corporate actions, multi-currency cash).

**Explicitly rejected:** the Client Portal Web API / TWS gateway (local Java
process, interactive 2FA session keep-alive — wrong shape for an unattended
self-hosted server, and it is the surface IBKR polices for automation).
Flex Query is the sanctioned reporting channel.

### User setup (documented in-app, step by step)

1. IBKR Client Portal → Performance & Reports → Flex Queries → create an
   **Activity Flex Query** with sections: Trades (executions), Cash
   Transactions, Corporate Actions, Transfers, Open Positions, Cash Report;
   period "Last 365 Calendar Days"; date format `yyyy-MM-dd`.
2. Settings → Account Settings → Flex Web Service → enable, generate token
   (user chooses expiry, up to ~1 year).
3. Paste **token + query ID** into Rekenraam's add-connection form. Probe
   validates both before storing.

### API contract (verify exact URLs/error codes at implementation)

Two-step, XML over HTTPS:

1. `SendRequest` (`…/AccountManagement/FlexWebService/SendRequest?t=<token>&q=<queryId>&v=3`)
   → `<Status>Success</Status>` + `ReferenceCode` + service URL, or an error
   code. Known classes: invalid/expired token (terminal → connection status
   `failed`, user must regenerate), throttled (retryable with backoff),
   statement not ready.
2. `GetStatement?t=<token>&q=<referenceCode>&v=3` → the statement XML, or
   "generation in progress" (retryable — poll with backoff inside the
   worker's lease, continuation item if the lease would expire).

Constraints to design around: data is T+1 (previous business day) — a 24h
scheduler cadence like T212's is correct; requests are rate-limited (roughly
one generation per minute; the two-step + retry pattern absorbs this); the
report window is fixed in the query definition, so **incremental fetch =
re-fetch the rolling window and dedupe**, not server-side cursoring.

### Cursor and dedupe strategy

Every `<Trade>` has a stable `transactionID` (and `tradeID`); every
`<CashTransaction>` has `transactionID`. Fingerprint = provider ID where
present. Cursor = max `dateTime` seen per section, stored per connection
(same three-stage cursor pattern as T212's
transactions/orders/dividends), with strict-inequality re-scan of
equal-timestamp rows (T212 precedent). Rows older than the rolling window
that were never imported are reachable by the user widening the query
period — document this.

### Data mapping

| Flex element | Route into Rekenraam |
|---|---|
| `<Trade assetCategory="STK/FUND">` buy/sell | `InvestmentService.Buy`/`Sell` via the T212-established commit path: instrument resolution ISIN → symbol → create; holding account via mapping table; cash amount = `abs(netCash)` (commission-inclusive, matching the current folded-fee model). When first-class trade fees land (2026-07-19 audit §7.4), switch to `tradeMoney` + `ibCommission` split — note the dependency, don't block on it |
| `<Trade assetCategory="CASH">` (forex) | v1: stage as generic rows for user-mapped exchange entries (`transfer_clearing`); v2: auto-shape the 4-posting FX transfer |
| `<CashTransaction type="Dividends">` | `InvestmentService.Dividend`; pair same-symbol, same-date `type="Withholding Tax"` rows into the withholding legs |
| `type="Payment In Lieu Of Dividends"` | Dividend (flagged in memo) |
| `type="Broker Interest Received/Paid"`, `"Other Fees"` | Generic staged cash rows → user-resolved income/expense categories (import rules will help here) |
| `type="Deposits/Withdrawals"`, `<Transfer>` | Generic staged rows with transfer hints (QIF `[Account]` precedent) |
| `<CorporateAction>` (splits FS/RS, ticker changes TC, spin-offs SO, mergers TO/TC, delistings) | **Stage, never auto-post.** Write-offs landed 2026-08-06 (T-38), so delistings have a real destination; until the lot-mutation design exists (backlog T-34), the rest of these rows land as `needs_attention` with the action description and a link to manual guidance. This is the honest version of "supported": visible, never silently dropped, never wrongly booked |
| `<OpenPosition>` | Not imported — used for a post-import **position reconciliation check**: compare IBKR-reported open quantity per instrument against Rekenraam lots, surface mismatches in the batch result. This is the trust feature that catches missed history |

### Slices and acceptance

1. **IBKR-1 client + prober.** `internal/onlinesource/ibkrflex/`; SendRequest/
   GetStatement with retry classification; golden-XML fixtures + fake-server
   tests (T212 test pattern); connection CRUD with `{token, query_id}`
   sealed payload; probe = SendRequest round-trip. *Accepts:* invalid token
   → `PROVIDER_ERROR` at add time; valid credentials stored masked.
2. **IBKR-2 staged import of Trades + Cash Transactions.** Mapping table
   generalization (`import_connection_accounts`); investment commit path
   reuse; dividend/withholding pairing; fingerprints from `transactionID`.
   *Accepts:* re-running a fetch commits zero duplicates; buy/sell create
   real lots; a dividend with withholding books both legs.
3. **IBKR-3 worker + scheduler.** Three-stage-style cursors; 24h auto-refresh
   toggle; terminal-vs-retryable statuses into `source_meta_json`; statement
   polling within lease. *Accepts:* expired token surfaces as a failed
   connection with a reconnect prompt, not a silent stall.
4. **IBKR-4 corporate-action staging + position check.** CA rows to
   `needs_attention`; OpenPosition comparison in the batch report.
   *Accepts:* a split in the statement is visible and blocked from silent
   commit; a lot/position mismatch is reported with quantities.

---

## Detailed plan 2 — EU + UK banks via GoCardless Bank Account Data

**Why this route:** direct PSD2/Open Banking access requires an AISP license
and eIDAS certificates (rejected permanently); among aggregators, GoCardless
BAD (the former Nordigen) is the one with a genuinely free tier, ~2,500+
EU/UK institutions, and token-based server-side API that the Firefly/Actual
communities already use in exactly this BYO-key fashion. One adapter covers
the EU **and UK** persona (institutions are filtered per country, `GB`
included).

**Risk box (verify before building):** the free tier's terms have tightened
over time (signup questions, per-account daily rate caps of a handful of
calls per endpoint). Re-verify: personal-use eligibility, rate limits,
consent duration options (90 vs 180 days), and `max_historical_days` per
target bank. Fallback if the free tier closes: Enable Banking (commercial
terms — would demote this to "user brings a paid aggregator account"), or
per-user SimpleFIN-style paid bridges as they appear in the EU.

### Credential and consent model (differs from T212/IBKR — design driver)

Two layers, and the UX must show both:

1. **API credentials** (`secret_id` + `secret_key` from the user's GoCardless
   portal account) — long-lived, sealed in secretbox. Exchanged at fetch
   time for a 24h access JWT (refresh token cached inside the sealed
   payload).
2. **Per-bank consent** — the user is redirected to their bank for SCA
   approval; the resulting *requisition* grants account access for a limited
   window (typically 90 days, some banks 180). **Consent expires by design
   and requires the user to re-approve.** This is the fundamental UX
   difference from key-based brokers: the connection page must show consent
   status/expiry per bank link, the scheduler must stop fetching expired
   requisitions and flag them, and "reconnect" must be one click into a
   fresh consent flow.

### API flow (verify endpoint details at implementation)

1. `POST /api/v2/token/new/` `{secret_id, secret_key}` → access/refresh JWTs.
2. `GET /api/v2/institutions/?country=NL|DE|GB|…` → institution picker
   (searchable in-app).
3. `POST /api/v2/agreements/enduser/` — set `max_historical_days` (request
   the bank's maximum; up to 24 months at some banks), `access_valid_for_days`,
   scopes `balances, details, transactions`.
4. `POST /api/v2/requisitions/` `{redirect, institution_id, agreement,
   reference}` → consent `link`; the app opens it; the bank redirects back
   to a Rekenraam route with the requisition reference.
5. `GET /api/v2/requisitions/{id}` → status (`LN` linked, `EX` expired…) and
   `accounts[]` — **one consent may return several bank accounts.**
6. Per account: `GET /accounts/{id}/transactions/` (`booked[]` +
   `pending[]`), `/balances/`, `/details/` (IBAN, currency, name).

### Data mapping

- Booked transactions only in v1 (pending is display-only noise for a
  ledger; revisit later). Fields: `transactionId` /
  `internalTransactionId` (stable ID → fingerprint; **some banks omit it**
  → fall back to content-hash fingerprint, which the pipeline already
  supports), `bookingDate` → entry date, `valueDate` → metadata,
  `transactionAmount.amount` (decimal **string** — parses exactly via the
  existing no-float `parseDecimalAmount`; period decimal separator per
  spec), `creditorName`/`debtorName` → payee hint,
  `remittanceInformationUnstructured` → description,
  `bankTransactionCode` → metadata for future rules.
- Rows stage as generic cash rows against the mapped ledger account —
  the same preview/commit/dedupe/categorize flow as QIF/T212-cash. This is
  also where a minimal import-rules engine (roadmap-review proposal) pays
  off most; note the synergy, not a hard dependency.
- Account mapping: `import_connection_accounts` (provider account ref =
  GoCardless account UUID; store IBAN/display name for the picker). A bank
  account with no mapping stages nothing and prompts for mapping —
  never guess.
- `/balances/` → **reconciliation assist**: after each fetch, compare the
  bank-reported booked balance with the ledger account balance and surface
  the difference on the connection page ("bank says X, ledger says Y —
  reconcile?"). Cheap, and it turns the feed into a trust feature; the
  reconciliation engine and `online_balance` source kind already exist.

### Rate-limit and scheduling posture

Free-tier caps are per-account-per-day (single digits — verify). Design for
scarcity: one scheduled fetch per account per 24h (existing cadence);
`Retry-After`-aware backoff; never burn calls on balances if the
transactions call failed; per-connection manual refresh guarded by the
existing in-flight lock.

### Slices and acceptance

1. **GC-1 client + credential lifecycle.** Token exchange/refresh inside the
   sealed payload; institutions listing; prober = token exchange.
   *Accepts:* bad secrets rejected at add time; access token refresh is
   transparent.
2. **GC-2 consent flow.** Agreement + requisition creation; redirect
   round-trip route; account discovery + `import_connection_accounts`
   mapping UI; consent status/expiry on the connection page. *Accepts:* a
   two-account bank consent yields two mappable accounts; expired consent
   shows a working one-click reconnect.
3. **GC-3 staged fetch.** Booked transactions → staged rows with stable-ID
   or content-hash fingerprints; cursor = max `bookingDate` with re-scan;
   commit through the existing pipeline. *Accepts:* refetch commits zero
   duplicates, including at banks without stable transaction IDs.
4. **GC-4 scheduler + balance assist.** 24h auto-refresh honoring rate caps;
   expired-consent flagging (stop + surface, never silent); balance
   comparison on the connection page. *Accepts:* an expired requisition
   never marks the batch "failed-retryable" forever — it lands in a
   distinct "reconsent needed" state.

---

## Analytics providers — the actual plan

"Analytics connections" decomposes into three real needs, all served by the
existing seams (`PriceProvider`, `DividendProvider`,
`CorporateActionProvider` interfaces are declared and unimplemented; the FX
registry shows the adapter pattern):

1. **Security quotes — now delivered in R17, not R15** (decided 2026-08-05;
   closes the unrealized-gains staleness gap, 2026-07-19 audit §4):
   implement the `PriceProvider` registry mirroring
   the FX one; first adapter Twelve Data or Alpha Vantage (BYO free key —
   decide on measured EU-exchange coverage, which is the persona's actual
   requirement); Stooq keyless as index/benchmark source; CoinGecko if/when
   the crypto instrument type ships. **Yahoo Finance ships too** (decided
   2026-08-05): keyless, broadest EU coverage, so it is the zero-setup
   default in the registry, labeled "unofficial" in the picker with a
   one-line note that the endpoint is undocumented and may break. Because
   the registry is multi-provider, a user who hits breakage switches to a
   BYO-key provider without losing data — that fallback is what makes the
   ToS/stability risk acceptable rather than a coverage promise. Scheduled refresh reuses the pricing
   worker and refresh-run bookkeeping wholesale.
2. **Dividend/corporate-action events — the T-34 producer**: Financial
   Modeling Prep's dividend + split endpoints are the leading free
   candidate (verify EU coverage). Events land as
   `investment_provider_events` → suggestions, which the accept/ignore UI
   and automation rules already handle end to end. Dividend suggestions
   work today; structural actions stay blocked on the lot-mutation design
   (T-34's second half) and should stage as informational until then.
3. **Benchmarks for R13** (TWR/MWR comparison): an index series via
   Stooq/the chosen quote provider; no new architecture.

Morningstar and Wikifolio explicitly resolve to "no connection needed":
Morningstar's data is commercially licensed (rejected by rule 2) and its
user-visible value here (fund performance context) arrives with R13 over
our own data; Wikifolio certificates price like any ISIN security through a
quote provider that covers German certificate venues.

## Sequencing (adopted 2026-08-05 in roadmap.md, amended)

The proposed order was adopted; its **contents were amended** because the
same-day R17 decision (crypto instrument type) also needs prices. The quote
provider is no longer an R15 slice:

0. **`PriceProvider` registry + quote adapters — moved to R17**, ahead of
   all of R15. Crypto needs prices regardless, so the registry is built once
   there (Yahoo keyless + CoinGecko + one BYO-key equity provider) rather
   than claimed by two slices. This also closes the unrealized-gains
   staleness gap earlier than planned.

R15 itself is then:

1. **IBKR Flex** (investor persona, roadmap already points here, no consent
   treadmill — the gentlest second adapter). Lands after R16, so its
   corporate-action rows have somewhere real to go: write-off (T-38) and the
   lot-mutation design exist by then, and IBKR-4 no longer stages into a
   permanent holding pen.
2. **GoCardless BAD** (EU+UK banks — biggest manual-entry win, hardest UX
   due to consent renewals; benefits from landing after R5's rules/profile
   work).
3. **T-34 producer via FMP** (shares the BYO-key plumbing the R17 registry
   establishes).
4. Raisin / Trade Republic / DeGiro / UK brokers ship as **R5 CSV mapping
   profile presets** — a documentation-plus-fixtures task per broker, not
   adapters.

## Slice-start preconditions (blocking)

Reclassified 2026-08-05: these were sitting in `todo.md` as "decisions
awaiting the owner", where they could never be actioned — they are
verification gates, not choices. Every "verify" marked inline above is
re-checked when its slice starts, and these two block their slice from
beginning:

- **IBKR-1 cannot start** until the Flex Web Service `SendRequest` /
  `GetStatement` URLs, version parameter, and error-code classes are
  re-confirmed against current IBKR documentation. The retry logic is built
  directly on the terminal-vs-retryable split, so guessing here produces a
  connection that stalls silently instead of prompting for a new token.
- **GC-1 cannot start** until the GoCardless Bank Account Data free tier is
  re-confirmed: current personal-use terms, per-account rate caps, and
  whether free access still exists at all. This one is strategic, not
  cosmetic — if the free tier has closed, the only official fallback
  (Enable Banking) is commercial, which ground rule 2 rejects, and EU bank
  aggregation leaves the roadmap entirely rather than shipping degraded.
  Escalate that outcome to a roadmap decision; do not quietly substitute a
  paid provider.

Research in this document dates from 2026-07. Free tiers and signup policies
shift, so treat every figure here as provisional until its slice starts.
