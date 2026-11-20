# Connections Plan — banks, brokers, analytics providers

Status: **partly delivered** (reconciled 2026-10-17). The Twelve Data reviewed
dividend producer (#111), provider revision evidence (#254), short payment in
lieu (#255) and existing-payment linking (#258) ship. IBKR Flex, GoCardless
and other bank/broker adapters remain proposals; Twelve Data split mapping is
evidence-blocked in #256. The July provider availability/price assessment is
historical and must be rechecked at each adapter start. Feature plan for
expanding online connections beyond Trading 212 (R7, shipped). Governed by
`docs/product-requirements.md`; sequencing is decided in `docs/roadmap.md`
(see the pointer added under "Deliberately later"). Detailed designs below
for the two priority adapters — **IBKR Flex Query** and **EU/UK banks via
GoCardless Bank Account Data** — with an assessment matrix for everything
else that was evaluated.

## Provider-independent delivery (2026-10-15)

[ADR 0017](../adrs/0017-provider-independent-features-and-incomplete-data.md)
governs this plan. An online adapter is optional enrichment of usable manual/file
workflows. Missing fields may remain explicitly unresolved; users can supply
facts or link manual operations. Required financial effects and verified automatic
mapping remain guarded. The historical assessment below is not a promise that a
provider covers all domain features. Its adapter-start checks never block core
feature development, manual recovery or another adapter. The roadmap's R15 order
does not create a hard IBKR/GoCardless prerequisite for #111.

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
| Twelve Data | Quotes | ✅ | Plan/market/display entitlements apply; free EU coverage is not assumed | BYO API key | Delivered #117; [current provider evidence](quote-providers-crypto-plan.md#provider-review-primary-sources-checked-2026-10-14) |
| Financial Modeling Prep | Quotes + dividends + splits | ✅ | Free ~250 req/day (verify EU coverage) | BYO API key | Historical T-34 candidate, not chosen: #111 shipped on Twelve Data, which already had BYO-key identity plumbing (see *Provider dividend producer*) |
| Stooq | EOD quotes + indices (incl. EU) | quasi-official CSV endpoints | Free, keyless | none | Benchmark-series candidate for R13 |
| CoinGecko | Crypto prices | ✅ | Demo key; plan limits and attribution apply | BYO API key | Delivered #117; [current provider evidence](quote-providers-crypto-plan.md#provider-review-primary-sources-checked-2026-10-14) |
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
| `<Trade assetCategory="STK/FUND">` buy/sell | Resolve instrument ISIN → symbol → create and holding account via mapping table. R15 follows R16's exact-trade slice: preserve `tradeMoney`, `ibCommission`, other charges, and net settlement as distinct facts; route only through the shared investment operation writer. Until that slice ships, keep a row requiring unsupported fee semantics in review rather than flattening charges into basis/proceeds |
| `<Trade assetCategory="CASH">` (forex) | v1: stage as generic rows for user-mapped exchange entries (`transfer_clearing`); v2: auto-shape the 4-posting FX transfer |
| `<CashTransaction type="Dividends">` | `InvestmentService.Dividend`; pair same-symbol, same-date `type="Withholding Tax"` rows into the withholding legs |
| `type="Payment In Lieu Of Dividends"` | Stage as a distinct investment cash activity for review; do not book it as an ordinary dividend merely by changing the memo. A charge on a short position admits to the native `payment_in_lieu` operation (#255: expense, optional recovery, short entitlement checked at the close before the ex-date); a payment *received* in lieu by a lender of long shares stays in review until its own treatment is decided |
| `type="Broker Interest Received/Paid"`, `"Other Fees"` | Generic staged cash rows → user-resolved income/expense categories (import rules will help here) |
| `type="Deposits/Withdrawals"`, `<Transfer>` | Generic staged rows with transfer hints (QIF `[Account]` precedent) |
| `<CorporateAction>` (splits FS/RS, ticker changes TC, spin-offs SO, mergers TO/TC, delistings) | **Stage, never auto-post.** Write-offs landed 2026-08-06 (T-38), but each other action still needs its own posting, basis, date, and replay specification under the investment operation plan. Until its command ships, show `needs_attention` with the source description and manual guidance; do not silently drop or misbook it |
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

"Analytics connections" decomposes into three needs. R17 now implements the
market-data registry's `PriceProvider` alongside FX. `DividendProvider` and
`CorporateActionProvider` remain seams for a future event producer.

1. **Security/crypto quotes — delivered in R17 #117 (2026-10-14).**
   CoinGecko Demo, Twelve Data and explicitly unofficial/keyless Yahoo Finance
   share one registry. Explicit instrument identities and dated pair assignments
   opt into fetching; saved daily scheduling, durable work and refresh-run/health
   history are reused. The original keyless Yahoo decision is retained as an
   available adapter, without implying a supported API or automated-data rights.
   Current terms/entitlements require operator review; no symbol/currency guessing
   or guaranteed free EU coverage is implied. The
   [quote/crypto contract](quote-providers-crypto-plan.md) records current evidence,
   exact raw-quote normalization, staleness and recovery. Stooq remains a benchmark
   candidate for R13 rather than a shipped quote adapter.
2. **Dividend events — #111, delivered 2026-10-17** on Twelve Data; see
   *Provider dividend producer* below. Events land as
   `investment_provider_events` → suggestions and commit only through the
   native dividend command after owner completion, or link to an
   owner-recorded dividend. Other event families still need their own
   verified source mapping and reviewed admission to their named command
   (Trading 212 split fills #145, execution cancellation #136, provider
   short/cover rows #176). A dividend owed on a short position proposes the
   native payment-in-lieu operation (#255, 2026-10-17), completed by the
   owner. Incomplete or unsupported events remain informational. Lack of an automated source never
   disables those native commands or their corrections.
3. **Benchmarks for R13** (TWR/MWR comparison): an index series via
   Stooq/the chosen quote provider; no new architecture.

Morningstar and Wikifolio explicitly resolve to "no connection needed":
Morningstar's data is commercially licensed (rejected by rule 2) and its
user-visible value here (fund performance context) arrives with R13 over
our own data; Wikifolio certificates price like any ISIN security through a
quote provider that covers German certificate venues.

## Sequencing (adopted 2026-08-05 in roadmap.md, amended)

The 2026-08-05 decision moved quote-provider registration into R17 so crypto
and securities share it. That registry and its initial adapters now ship.

Current order belongs only to [the roadmap](../roadmap.md#deliberately-later).
IBKR Flex and GoCardless remain proposed adapters with the verification gates
below. CSV broker presets are profile/fixture work, not online adapters. The
#111 dividend producer shipped independently and consumes existing native
commands; it does not imply mappings for other corporate-action families.

## Provider dividend producer (#111, delivered 2026-10-17)

Bounded mapping: **Twelve Data `GET /dividends` → `dividend` family only.**

Primary-source contract, checked 2026-10-17 against Twelve Data's published
OpenAPI document (`https://api.twelvedata.com/doc/swagger/openapi.json`,
operation `GetDividends`):

- Parameters used: `symbol`, `mic_code`, `start_date`, `end_date`
  (`range` would take precedence, so it is never sent) and `adjust`, a boolean
  defaulting to `true`; the producer always sends `adjust=false` so a later
  split never rewrites the announced per-share amount.
- Response: `meta{symbol, name, currency, exchange, mic_code,
  exchange_timezone}` and `dividends[{ex_date, amount}]`; both row fields are
  required. `meta.currency` is documented as the currency the instrument is
  traded in, so it is kept as the listing currency and never as the payment
  currency. There is no payment date, record date, withholding, frequency or
  revision identity in the contract.
- Entitlement: `x-starting-plan: grow,venture` (paid plans) and
  `x-api-credits-cost: 20` per symbol. A key without the plan gets a provider
  error recorded as terminal work; nothing else changes.
- Representative fixture: `backend/internal/marketdata/testdata/twelve_data_dividends.json`
  (synthetic, shaped exactly to the schema above; it does not claim live
  coverage of any listing).

Mapping rules: opt-in only through an explicit `twelve_data_symbol` and MIC
on an active Twelve Data quote assignment; one fetch covers the last 365
owner-local days; an ex-date not yet reached, or one the ledger does not show
held (long or short) at the close of the previous day, is not retained. Each
retained row becomes one immutable event keyed by
`twelve_data:dividend:{symbol}:{mic}:{ex_date}` (two amounts for one ex-date
refuse the response). A later different amount for a staged ex-date is not
re-staged: the first observation and the owner's decision stand, and the
owner supplies the received amount anyway. Since there is no revision
identity, a revision is only a content difference (exact per-share amount or
listing currency, never the ledger-derived holdings) for the same identity;
each distinct content is appended once as immutable revision evidence with
its raw row and shown as "provider revised: old → new" on the suggestion
([#254](https://github.com/sergeyfarin/rekenraam/issues/254)). A return to the first announced amount is not
recorded again. Because the payment date, cash,
currency and withholding are never stated, every proposal is incomplete by
design; the owner's statement completes it. A book short (and long in no
holding) at the close proposes a payment in lieu instead (#255): the evidence
counts the short units and estimates the amount owed, the owner supplies the
payment date, cash account, amount owed and expense account, and acceptance
commits through the native payment-in-lieu command; a payment in lieu the
owner already recorded for that instrument, holding and ex-date is offered
for linking instead ([#258](https://github.com/sergeyfarin/rekenraam/issues/258)). Long in one holding and
short in another stays informational. There is no scheduler and no automation-rule auto-post; the
owner triggers *Check provider dividends*.

### Twelve Data splits — contract reviewed, not mapped ([#256](https://github.com/sergeyfarin/rekenraam/issues/256), 2026-10-17)

A second bounded mapping, Twelve Data `GET /splits` → `split` provider events
admitted through the native split command, was evaluated against the same
OpenAPI document (operation `GetSplits`) and **is not implemented**: the
contract does not state an unambiguous ratio or effective date.

- Parameters: `symbol` (or `figi`/`isin`/`cusip`), `exchange`, `mic_code`,
  `country`, `range` (`last` by default; `1m`…`full`), `start_date`,
  `end_date`. No `adjust` flag.
- Response: the same `meta` as `GetDividends` and
  `splits[{date, description, ratio, from_factor, to_factor}]`, all five
  required. `/splits_calendar` returns the same row shape plus `symbol`,
  `mic_code`, `exchange`.
- Entitlement: `x-starting-plan: grow,venture`, `x-api-credits-cost: 20` per
  symbol — identical to `/dividends`.
- **Ratio is ambiguous.** `ratio` is described as "the ratio by which the
  number of … outstanding shares … are increased", yet its example for a
  `4-for-1 split` is `0.25` (the inverse: a price factor). `from_factor` and
  `to_factor` are described only as "From/To factor of the split", with the
  example `from_factor: 4, to_factor: 1` for that same forward split — read
  literally, "from 4 to 1" is a reverse split. The contract shows no reverse
  split, so the direction of every field is inferred from one example, never
  stated, and `description` is free text.
- **Date is ambiguous.** `date` "stands for the split date". The contract does
  not say whether it is the announcement, record, ex/effective or payable
  date, and the native split command needs the date the position changes.

Consequence: a guessed direction would post a reversed split (a 4× unit error
on the lot replay), and a guessed date would land on the wrong side of trades
around the split. The mapping stays unbuilt until the provider documents the
field semantics or live responses for a known forward **and** reverse split
(with a known ex-date) pin them. Owners record splits manually through the
native split command in the meantime. This is a Twelve Data evidence gap only;
per ADR 0017 it neither blocks nor is blocked by Trading 212 split fills
([#145](https://github.com/sergeyfarin/rekenraam/issues/145)).

### Massive US split suggestions ([#615](https://github.com/sergeyfarin/rekenraam/issues/615))

Massive's Stocks split endpoint supplies typed forward and reverse events.
Live Stocks Basic responses matched the issuer or exchange notices for Netflix
(10-for-1, first split-adjusted date 2025-11-17) and SkinHealth Systems
(1-for-20, 2026-10-14). Stock dividends are a separate event type and are
excluded. The split's exact share ratio is `split_to / split_from`. The
execution date is the first split-adjusted trading session. Confirm the exact
assigned ticker, supported US exchange MIC and share-class FIGI from dated
ticker lookups on the execution date and range end. Use provider event IDs for
deduplication and preserve materially different observations as revisions.
[Massive split contract](https://massive.com/docs/rest/stocks/corporate-actions/splits)

A saved Massive ticker is explicit opt-in for a single user's own listing.
**Check Massive splits** queues per-instrument durable work with a 365-day
lookback. Nothing posts during a fetch. A single long holding at the close
before the date is eligible for acceptance through the native split writer.
Missing identity, inconsistent ratios, competing events, multiple holdings,
short positions, and other activity on the execution date require manual
review. The native writer enforces quantity precision, replay, reconciliation
and gain checks. An unrevised suggestion can link a matching posted split
with the same holding, instrument, date and ratio. Revisions cannot alter a
posted or linked split and prevent accepting a pending original. Manual
split entry remains available independently under ADR 0017.

## Slice-start preconditions (blocking)

Reclassified 2026-08-05: these were sitting in `todo.md` as "decisions
awaiting the owner", where they could never be actioned — they are
verification gates, not choices. Every "verify" marked inline above is
re-checked when its online adapter slice starts, and these two block that
adapter's integration work only. Manual/file input, shared domain services and
partial-data recovery can proceed:

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
