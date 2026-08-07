# Trading 212 Online Import Plan

A detailed, implementation-ready plan for **online import from the Trading 212
public API**, built as the first concrete `OnlineSource` on top of the import
pipeline already shipped in Slice 1 (`docs/plans/import-plan.md`, roadmap R4).

This is the plan that realises roadmap **R7 (Online ingestion)** for one real
provider. It is written for the **Sonnet model to implement** directly: every
slice names the exact existing files/types to reuse, the new files to add, and
the acceptance test that proves it.

Governed by `docs/plans/import-plan.md` (the unified pipeline), `docs/roadmap.md` (R7),
ADR 0010 (durable background work), and `docs/conventions.md`. Aligns with the
FX-refresh precedent in `docs/archive/fx-refresh-implementation-plan.md`, which is the
template for the durable-fetch machinery here.

Status: **Slice 1 (Credential store + connection CRUD) shipped 2026-06-28. Slice 2
(Fetcher + adapter, parse offline) shipped 2026-06-30. Slice 3 (Durable fetch +
JSON `POST /imports` branch + refresh, plus a post-shipment review pass closing
T-14/T-15/T-16/T-17) shipped 2026-06-30/2026-07-01. Slice 4a (scheduled
auto-refresh, B-T212-SCHED) shipped 2026-07-01. Real-API verification pass
(2026-07-03, while scoping Slice 4b) found and fixed two live bugs in the
already-shipped cash-movement path: `historyPath` was `/history/transactions`
(404s against the real API; correct path is `/equity/history/transactions`),
and `cashMovementTypes` included several values (`WITHDRAWAL`, `DIVIDEND`,
`INTEREST`, `CARD_*`, `TRANSFER_IN/OUT`, `LENDING_INTEREST`) the real endpoint
never emits — its actual `type` enum is only `WITHDRAW`/`DEPOSIT`/`FEE`/`TRANSFER`.
Both were unverified guesses from before this plan's author had access to the
published OpenAPI spec. `Probe` now hits `/equity/account/summary` instead of
reusing the history endpoint. See `docs/backlog.md` T-21 (closed).
Slice 4b (investment lot import, B-T212-INVST) shipped 2026-07-03 — see its
"Delivery slices" entry below for what's actually built vs. deferred.**

**Also found and fixed while building Slice 4b (severity-1, pre-existing since
Slice 1, unrelated to Trading 212 specifically):** `buildTransactionSpec`
(`import_service.go`) set every journal entry's `EntryKind` to `"main"`, which
is not a member of the valid `entryKinds` set
(`transactions_validate.go`) — every `CommitImportBatch` call, for **any**
import source (QIF file upload or Trading 212), failed `"entry kind is
invalid"` the instant it reached `TransactionService.CreateTransaction`'s
real validation. This had gone completely undetected because no test in the
suite drove a staged row through the full commit path against a real
account/ledger — every prior test either checked `buildTransactionSpec`'s
output in isolation or never called `CommitImportBatch` at all (see the old
T-13 "narrowed, not fully closed" note, which was closer to the truth than
anyone realized). Fixed: `EntryKind: "ordinary"`. See `docs/backlog.md` T-22.

Slice 1 delivered: `internal/secretbox` (AES-256-GCM), `REKENRAAM_SECRET_KEY` config,
the pre-beta baseline migration, `ImportConnectionRepository`, `ImportConnectionService`
(probe-before-store, key masking), 4 REST endpoints (`/api/v1/import-connections`),
OpenAPI coverage, generated TS types, `$lib/api/connections.ts` client, connections UI
on the import page (masked list, add form, delete with confirm).

Slice 2 delivered: `internal/onlinesource/trading212` (`Fetcher`: cursor/`nextPagePath`
pagination, 401/403 → `ErrUnauthorized`, 429 + `Retry-After` in-call backoff, base URL
override); `Trading212Adapter` (`internal/app/import_trading212.go`) implementing
`SourceAdapter`, registered in `NewImportService`; connection-scoped provider-id
fingerprints (`trading212|<connection_id>|<providerID>`); cash movement types map to
plain staged rows, `BUY`/`SELL` and any unrecognized type are staged with a
`needs_attention` parse warning (B-T212-INVST unchanged). `ConnectionProber` extended
with a `configJSON` parameter so `config_json.base_url` can target the demo/sandbox
endpoint; `Trading212Prober` wired in `cmd/rekenraam/command.go`, closing T-11. The
flagged `stageParseResult` refactor is done: `StartImport` now creates the batch and
delegates fingerprint-hash → dedupe → insert to a shared method the Slice 3 fetch
worker will call directly. No queue wiring, no `POST /imports` JSON branch, and no UI
yet — those are Slice 3.

Slice 3 delivered: `app/import_fetch_worker.go` — `kind="import.fetch.trading212"`
durable worker (same claim/lease/retry/backoff shape as `pricing_worker.go`), wired
via `ImportService.StartBackgroundWorker` next to `pricingService.StartBackgroundWorker`
in `cmd/rekenraam/command.go`. `POST /api/v1/imports` is now content-negotiated:
`application/json {source, connection_id}` enqueues a fetch and returns `202` with a
`previewing` batch (`source_meta.fetch_status="fetching"`); `multipart/form-data` is
unchanged (`201`). New `POST /api/v1/import-connections/{id}/refresh` opens a fresh
batch and fetches incrementally from the connection's saved cursor (`202`). Starting
a fetch (either entry point) is atomic — `ImportRepository.StartOnlineImportBatch`
does the in-flight guard check, batch insert, and work-item enqueue in one
transaction, closing a real race/stranded-batch gap found in review (T-16) — and
returns `ErrImportFetchInProgress` → `409 CONFLICT` when a fetch is already running
for the connection. Unauthorized (401/403) fetches fail terminally on the first
attempt (bad keys don't get better with retries); other errors retry with the
existing `retryDelay` backoff up to 8 attempts before the batch and connection are
marked `failed` — both in `import_batches.status` and in `source_meta_json`, the
field the frontend's polling loop actually reads (a review pass caught that only the
former was wired originally). `import_batches.connection_id` (already a column since
migration `0007`) is now actually written and read; T-12's provenance concern is
closed by also snapshotting `connection_display_name` into `source_meta_json` at
fetch time, so a batch's online provenance survives connection deletion (`ON DELETE
SET NULL` still clears the FK, but the human-readable name persists). A fetch beyond
`trading212.Fetcher`'s 50-page-per-call budget continues automatically via a
`reason="continuation"` work item chain rather than silently truncating (T-14); the
incremental cursor re-scans (rather than drops) movements sharing its exact
timestamp (T-17), and the fetcher refuses to follow an absolute `nextPagePath` so a
compromised/misconfigured provider response can't exfiltrate the API key (also
T-17). Frontend: a per-connection "Import"/"Refresh" button replaces the connections
table's bare list; clicking it polls `GET /imports/{id}` until `source_meta.fetch_status`
is `ready` (or shows a failure state), then hands off to the existing unchanged
preview/commit UI. Last updated 2026-07-01 (post-shipment review pass closed T-14
through T-17 — see `docs/backlog.md`).

---

## Why Trading 212 first

- It is the user's concrete second source after the MS Money (QIF) migration.
- Its public API is **token-based** (a personal API key minted in the app), not
  OAuth/PSD2 — so it proves the online model end-to-end **without** building an
  OAuth dance or open-banking connectivity first (both explicitly deferred in
  `docs/plans/import-plan.md` non-goals).
- It exercises every hard part of online import exactly once: credential storage,
  durable polling, pagination/cursoring against a provider, native-currency money,
  and provider-id-based dedupe — giving the next provider a paved road.

## Scope

**In scope (this plan):**
- A `trading212` `OnlineSource` adapter implementing the existing `SourceAdapter`
  contract, fetching the account **transaction/cash-movement history** and mapping
  it to canonical `StagedRow`s.
- An **online source registration + credential** model (store the Trading 212 API
  key encrypted at rest; this is a new capability — see "Credential storage").
- Durable, restart-safe fetching via the existing `background_work_items` queue
  (`kind="import.fetch.trading212"`), mirroring `pricing_worker.go`.
- Fetched payloads flow into the **same** batch → preview → commit pipeline. No
  new commit path. Imported rows land `posted` + `needs_review` exactly as file
  import does today.
- Incremental refresh: a saved cursor so a re-fetch pulls only new movements;
  provider-id dedupe (`import_commit_identities`) makes overlap safe.

**Out of scope (deferred, documented as backlog where load-bearing):**
- **Investment position / holdings sync** (open positions, instrument prices). This
  plan imports **cash-account movements** (deposits, withdrawals, dividends, fees,
  interest, card transactions, FX from order settlement *as cash legs*). Buys/sells
  of instruments are flagged `needs_attention` and **not** auto-booked as lots — the
  investments ledger UI is not built yet (same stance QIF takes on `!Type:Invst`).
  Tracked as **B-T212-INVST** below.
- Scheduled/automatic polling UI (cron-like). This plan ships **manual "refresh
  now"** + the durable queue; a scheduling toggle is a thin follow-up (B-T212-SCHED).
- Multiple Trading 212 accounts / multi-book. Single book (`BookID` constant) and
  one Trading 212 connection, consistent with the rest of the MVP (backlog T-02).

---

## What already exists (reuse, do not rebuild)

The implementer must treat this as a **thin adapter + fetch driver** over shipped
machinery. The temptation to build a parallel pipeline is the main risk.

| Capability | Where | How this plan uses it |
|---|---|---|
| `SourceAdapter` interface (`Kind`/`Detect`/`Parse`) | `backend/internal/app/import_adapter.go` | The Trading 212 adapter implements it. `Parse` reads the **fetched JSON** from `RawInput.Bytes` — no file is involved, but the shape is identical. |
| Adapter registry | `import_adapter.go` (`newAdapterRegistry`) | Register the new adapter alongside `&QIFAdapter{}` in `NewImportService`. |
| Whole pipeline: normalize → dedupe → stage → preview → commit | `app/import_service.go` | Unchanged. Online rows are just `StagedRow`s with a provider id in `ExternalRef`/`DedupeFingerprint`. |
| `import_commit_identities` idempotency (`UNIQUE(book_id, dedupe_fingerprint)`) | beta baseline, `db/imports.go` | Provider transaction id → fingerprint precedence (already described in `import-plan.md` "Dedupe identity"). Re-fetch overlap is a no-op. |
| Durable work queue (lease/retry/resume) | `db/background_work.go`, `app/pricing_worker.go` | Copy the worker shape for `import.fetch.trading212`. Restart-safe by construction. **Note:** the queue does **not** coalesce duplicate enqueues today (see T-09); the refresh handler must guard against double-fetch itself — see "Durable fetch worker". |
| `CreateTransaction(OriginType="import", NeedsReview=true, ...)` | `app/transactions_write.go` via `buildTransactionSpec` | Commit path is identical to QIF. No change. |
| Reconciliation skip/override at commit | `import_service.go` `CommitImportBatch` | Unchanged; backdated online rows behave like backdated QIF rows. |
| Currency resolution by code | `AccountRepository.CurrentCurrencyByCode` (`db/accounts.go:719`) | Trading 212 movements carry a real currency code (unlike QIF) → resolve `CommodityID` from it instead of forcing a batch-level pick. |
| Provider/config env pattern | `internal/config/config.go`, `marketdata.DefaultRegistry` | The Trading 212 **base URL** follows the same `os.Getenv` override pattern (for tests/sandbox). The **API key is per-connection data, not env** — see below. |

---

## Key design decisions

### 1. Online fetch produces `RawInput.Bytes`, then the existing adapter parses

`SourceAdapter.Parse` already takes `RawInput{ Bytes []byte }`. An online source
is therefore **two cooperating pieces**:

1. A **fetcher** (lives in a new `internal/marketdata`-sibling package,
   `internal/onlinesource/trading212`) that talks HTTP to Trading 212, handles
   auth, pagination, and rate limits, and returns the **raw provider JSON** (the
   concatenated/normalized page payloads) plus a fetch cursor.
2. The **`Trading212Adapter`** (in `internal/app`, next to `QIFAdapter`) whose
   `Parse` unmarshals that JSON into `[]StagedRow`. `Detect` returns
   `ConfidenceHigh` only for `source_kind="trading212"` synthetic input (online
   sources are never auto-detected from an uploaded file).

This split keeps the HTTP/provider quirks out of `internal/app` (matching how
`internal/marketdata` isolates FX providers from `PricingService`) while reusing
the canonical `Parse → StagedRow` contract verbatim.

```
Trading 212 API ──HTTP──▶ trading212.Fetcher ──raw JSON bytes──▶ RawInput
                          (auth, paging, rate-limit, cursor)         │
                                                                     ▼
                                          Trading212Adapter.Parse(RawInput) ─▶ []StagedRow
                                                                     │
                                                   (the existing pipeline from here on)
```

### 2. Start the batch from a fetch, not an upload

`POST /api/v1/imports` is multipart-only today. Add a **JSON branch**: when the
request is `application/json` with `{ "source": "trading212", "connection_id": N }`,
the handler enqueues a fetch work item and returns `202 Accepted` with the
`batch_id` in `previewing` state but **no rows yet** (rows arrive when the worker
completes the fetch + parse + stage). The existing `GET /imports/{id}` is polled
by the UI until `source_meta.fetch_status` flips to `ready`.

Rationale: fetching can be slow / rate-limited / need retries — it **must** run on
the durable queue (ADR 0010), not inline in the HTTP request. This mirrors FX
refresh, which never fetches synchronously in a request handler.

> **Decision recorded:** the batch row is created **before** the fetch (so the user
> has something to poll and the work item has a `batch_id` to attach rows to). A
> batch whose fetch ultimately fails lands in `failed` with an `import_batch_events`
> row, exactly like a parse failure.

### 3. Credential storage — the one genuinely new subsystem

Today the repo has **no encrypted-secret store**: passwords are argon2 *hashes*
(one-way), and the only third-party key (`OPEN_EXCHANGE_RATES_APP_ID`) is an
**env var**. A Trading 212 API key is different: it is a **reusable bearer secret
the user pastes at runtime** and we must **send it back to the provider**, so it
must be stored **reversibly but encrypted at rest**.

This plan introduces a minimal, dependency-light secret box:

- New env var `REKENRAAM_SECRET_KEY` (32-byte key, base64) loaded in
  `config.Config`. Required only when at least one online connection exists; the
  server boots without it but `POST /import-connections` returns a clear
  `CONFIG_REQUIRED` error if it is unset.
- New package `internal/secretbox` wrapping `crypto/aes` + `crypto/cipher`
  (AES-256-GCM, random 12-byte nonce prepended to ciphertext). Pure stdlib; no new
  dependency. `Seal(plaintext) -> base64(nonce||ct)` / `Open(...) -> plaintext`.
- New table `import_connections` (now in the beta baseline):
  `id`, `book_id`, `source_kind` (`"trading212"`), `display_name`,
  `secret_ciphertext` (the sealed API key), `config_json` (non-secret: base URL
  override, default target account id, instrument→account map later),
  `fetch_cursor` (last successful incremental cursor / timestamp),
  `last_fetch_status`, `last_fetched_at`, `created_at`. **Never** store the key in
  plaintext, in `config_json`, in logs, or in any API response.
- API never returns the key. `GET /import-connections` returns a masked hint
  (`••••last4`) and metadata only.

> **Security note for the implementer:** the GCM nonce must be fresh-random per
> seal; never reuse. Wrap key-load failure as a hard error. Add a unit test that a
> tampered ciphertext fails `Open`. Document the env var and the "lose the key →
> re-enter connections" recovery in `conventions.md`.

This subsystem is intentionally small and is the reusable substrate for **every**
future online provider, so it is built once, here.

### 4. Dedupe identity uses the provider transaction id (the strong key)

Per `import-plan.md` precedence, Trading 212 movements expose a stable id (the
`reference` / transaction id on the cash-history endpoint). The adapter sets
`StagedRow.ExternalRef = providerID` **and** builds the pre-hash fingerprint as
`trading212|<connection_id>|<providerID>` (connection-scoped so two connections
can't collide). The service hashes it via the existing `hashFingerprint`. This is
strictly stronger than QIF's content hash and makes incremental re-fetch
idempotent for free.

### 5. Money is exact, currency is real

Trading 212 amounts are decimal strings with a currency code. The adapter passes
the **raw decimal string** into `StagedRow.Amount` and the code into
`CommodityHint` — never through float. Normalization reuses `parseDecimalAmount`
(already in `import_service.go`). Because the currency is known, the commit
resolution can default `CommodityID` from `CurrentCurrencyByCode`, removing the
QIF "pick a currency for the whole batch" step for these rows.

---

## Data model (beta baseline)

Goose migration, additive only. Follows the column conventions in
the beta baseline (denormalized `book_id`, RFC3339 text timestamps).

- `import_connections` — as described in Decision 3.
  `UNIQUE(book_id, source_kind, display_name)`.
- (No new staging tables.) Online batches reuse `import_batches`,
  `import_staged_rows`, `import_batch_events`, `import_commit_identities`. The
  `import_batches.source_kind` column already holds arbitrary kinds; it stores
  `"trading212"`. Add a nullable `connection_id` column to `import_batches` (so a
  batch records which connection produced it) **and** a nullable `fetch_status`
  surfaced via `source_meta_json` rather than a schema change where possible —
  **decision:** add the `connection_id` column (cheap, queryable for history),
  keep `fetch_status` inside `source_meta_json` (transient, already JSON).

The background work payload (no table; stored in `background_work_items.payload_json`):

```go
type trading212FetchPayload struct {
    ConnectionID int64  `json:"connection_id"`
    BatchID      int64  `json:"batch_id"`
    Reason       string `json:"reason"`     // "manual" | "continuation"
    Cursor       string `json:"cursor"`     // incremental fetch cursor (empty = full)
}
```

---

## API surface (additions; OpenAPI-first per `conventions.md`)

Add these to `api/openapi/openapi.yaml` **and** generate types — and while
here, close **T-07** by adding the seven existing import paths too (see backlog).

Connections (`/api/v1/import-connections`):
- `POST` — create a connection: body `{ source, display_name, api_key, config }`.
  Seals `api_key`, stores it, validates by making one authenticated probe call to
  Trading 212 (`/equity/account/summary`, confirmed 2026-07-03 against the
  published API spec) and returns `201` with the
  masked connection. A failing probe returns `502 PROVIDER_ERROR` and stores
  nothing.
- `GET` — list connections (masked, never the key).
- `PATCH /{id}` — rename, rotate key (re-seal), update non-secret config.
- `DELETE /{id}` — remove a connection (does **not** touch already-committed
  transactions; clears `fetch_cursor`).

Imports (extend the existing handlers):
- `POST /api/v1/imports` — **content-negotiated**. `multipart/form-data` →
  existing file path. `application/json` `{ source, connection_id }` → enqueue a
  `import.fetch.trading212` work item, create a `previewing` batch with
  `source_meta.fetch_status="fetching"`, return `202` + `batch_id`.
- `GET /api/v1/imports/{id}` — unchanged shape; `source_meta.fetch_status`
  (`fetching` | `ready` | `failed`) lets the UI poll. Rows appear once `ready`.
- `POST /api/v1/imports/{id}/refresh` — **new**: enqueue an incremental fetch on
  the same connection that produced this batch, using the saved `fetch_cursor`.
  Returns `202`. (Equivalently a connection-level `POST
  /import-connections/{id}/refresh` that opens a new batch — **decision:** ship the
  connection-level form; it matches the "refresh now" mental model and a fresh
  batch keeps history clean.)
- All other import endpoints (`PATCH`, `preview-commit`, `commit`, `discard`,
  history `GET /imports`) are **unchanged** — online batches flow through them as-is.

Error mapping (extend `writeImportServiceError`): add `ErrImportConnectionNotFound`
→ 404, `ErrProviderUnauthorized` → 502 `PROVIDER_ERROR` (do not leak the key),
`ErrSecretKeyMissing` → 503 `CONFIG_REQUIRED`.

---

## The Trading 212 fetcher (`internal/onlinesource/trading212`)

> **Implementer note on API specifics:** Trading 212 exposes a REST API at
> `https://live.trading212.com/api/v0` (and `https://demo.trading212.com/api/v0`
> for the practice account). Auth is an `Authorization: <api-key>` header (the key
> is minted in the Trading 212 app under *Settings → API*). The endpoints used
> here are the **history** ones: cash transactions / order history / dividends. The
> exact JSON field names below are **assumptions to verify against the live API on
> first implementation** — isolate them in the fetcher's response structs and a
> golden-file test so a field rename is a one-line fix, never a pipeline change.
> Treat the base URL as configurable (`config_json.base_url`, default live) so the
> demo endpoint and a test server can be injected.

Responsibilities:
- **Auth:** set the `Authorization` header from the sealed key (opened in-memory
  for the duration of the fetch only).
- **Pagination:** Trading 212 history endpoints are cursor/`nextPagePath`-paged.
  Loop pages until exhausted or until a movement older than `cursor` is reached
  (incremental). Concatenate normalized movements.
- **Rate limits:** Trading 212 enforces per-endpoint limits (HTTP 429). Respect
  `Retry-After`; on 429 without it, back off. The **durable queue's** retry/backoff
  (`retryDelay` in `pricing_worker.go`) handles cross-restart retries; in-fetch
  paging backoff handles within-call throttling.
- **Output:** marshal a stable internal shape (`[]movement`) to JSON bytes for
  `RawInput.Bytes`, plus the new cursor (max movement timestamp/id seen).
- **No ledger access. No DB access.** Pure HTTP + transform, like a `marketdata`
  provider.

Movement → `StagedRow` mapping (in `Trading212Adapter.Parse`):

| Trading 212 movement | `StagedRow` | Notes |
|---|---|---|
| transaction id / reference | `ExternalRef`, fingerprint seed | Strong dedupe key. |
| timestamp | `Date` (normalized to `YYYY-MM-DD`) | Provider gives ISO; pass through, `parseQIFDate` accepts ISO. |
| amount + currency | `Amount` (raw decimal), `CommodityHint` | Exact; no float. Sign per movement direction. |
| type (real enum, confirmed 2026-07-03: `WITHDRAW`/`DEPOSIT`/`FEE`/`TRANSFER`) | `Memo`/`Raw["type"]`, drives default category hint | Cash movements map cleanly. Dividends/interest/card movements are **not** emitted by this endpoint at all — see the dedicated dividends endpoint in Slice 4b. |
| order/instrument fill (`BUY`/`SELL`) | row flagged `needs_attention` via `ParseWarning` | Not auto-booked as a lot (B-T212-INVST). The **cash leg** of a settled order may still import as cash if the API exposes it separately; instrument lots do not. |
| counterparty / description | `PayeeHint` | |

---

## Durable fetch worker (`app/import_fetch_worker.go`)

A near-copy of `pricing_worker.go`, parameterized for import fetch. Key points:

- **Prerequisite refactor (do first):** the queue methods (`EnqueueBackgroundWork`,
  `ClaimBackgroundWork`, `Complete/Retry/Fail/BackgroundWorkByID`) live on
  `*db.PricingRepository` today (`db/background_work.go`), even though the queue is
  generic (`kind`-based). The import worker must not import the pricing repository.
  **Extract them onto a standalone `*db.BackgroundWorkRepository`** (same `*sql.DB`)
  and have `PricingRepository` embed or delegate to it, so both `PricingService`
  and `ImportService` share one queue type. This is a pure move (tracked as T-10);
  do it in Slice 3 before writing the worker. Without it the dependency graph is
  wrong (import → pricing).
- `const trading212FetchWorkKind = "import.fetch.trading212"`.
- `StartBackgroundWorker` ticks every minute, claims due work with a lease, and on
  claim runs: open connection secret → `trading212.Fetch(cursor)` → write
  `RawInput.Bytes` → `Trading212Adapter.Parse` → run the **existing**
  stage/dedupe logic from `StartImport` (refactor that staging logic into a private
  `s.stageRows(ctx, batchID, parseResult)` so both the file path and the fetch path
  call it — **do not duplicate it**) → mark batch `source_meta.fetch_status=ready`
  → save `import_connections.fetch_cursor` + `last_fetched_at`.
- On error: `RetryBackgroundWork` with `retryDelay(attempts, id)` (reuse the
  existing helper) — restart-safe. After N terminal failures, mark the batch
  `failed` with an `import_batch_events` row and `last_fetch_status="failed"`.
- **Double-fetch guard (the queue does NOT coalesce — verified):**
  `EnqueueBackgroundWork` is a plain `INSERT`; there is no uniqueness or
  `WHERE NOT EXISTS` on enqueue (the FX path tolerates duplicates because coverage
  work is idempotent). A naive "refresh" button could therefore enqueue two
  concurrent fetches for the same connection, producing two `previewing` batches.
  Guard at the **service** layer: the refresh handler refuses to enqueue if the
  connection already has a `previewing` batch with `fetch_status="fetching"` (or an
  un-completed `import.fetch.trading212` work item for that `connection_id`).
  Provider-id dedupe makes the *committed* result safe regardless, but the guard
  avoids confusing duplicate preview batches. The broader fix (a coalescing enqueue)
  is tracked as T-09 and is out of scope here.

> **Refactor flagged (do in Slice 1):** lift the row-staging block of
> `StartImport` (`import_service.go` lines ~64–167) into a shared
> `stageParseResult` method. Today it is inline; the fetch worker needs the same
> logic. This is a clean extraction, not a rewrite, and it also makes
> `StartImport` shorter. Note it touches the crash-consistency area of **T-06** —
> do not regress that; the staging insert and batch creation stay as they are.

---

## Delivery slices

Each slice leaves the app runnable, tested, documented (`implemented.md`,
`roadmap.md` R7, this file's status line). Update docs **in the same commit** as
the code — a slice is not done until its docs reflect it.

### Slice 1 — Credential store + connection CRUD (no fetch yet)
- `internal/secretbox` (AES-256-GCM seal/open) + `REKENRAAM_SECRET_KEY` in config.
- Beta baseline: `import_connections` (+ `connection_id` on
  `import_batches`).
- `ImportConnectionService` + repository: create (seal key + provider probe),
  list (masked), patch (rotate), delete.
- `/api/v1/import-connections` CRUD; OpenAPI paths + generated types.
- Frontend: a "Connections" section on the import page — add Trading 212 key,
  see masked list, delete. All copy via Paraglide.
- **Tests:** secretbox round-trip + tamper-fail; connection create seals and never
  returns the key; probe-failure stores nothing; key absent → `CONFIG_REQUIRED`.
- **Acceptance:** a user pastes a Trading 212 API key, it is validated by a live
  probe, stored encrypted, and listed masked; the plaintext key never appears in
  any response, log line, or DB column.

### Slice 2 — Fetcher + adapter (parse offline, no queue yet)
- `internal/onlinesource/trading212`: `Fetcher` (HTTP, paging, rate-limit, cursor),
  response structs, base-URL override. Driven by a fake HTTP server in tests.
- `Trading212Adapter` in `internal/app` implementing `SourceAdapter`; register it
  in `NewImportService`.
- Extract `stageParseResult` from `StartImport` (the flagged refactor).
- **Tests:** golden-file parse of a captured Trading 212 history payload →
  expected `StagedRow`s (deposit, dividend, fee, withdrawal, a `needs_attention`
  order row); pagination across two pages; 429 + `Retry-After` honored; provider-id
  fingerprint stable across re-parse.
- **Acceptance:** given a saved JSON sample, the adapter produces correct canonical
  rows with provider-id fingerprints, and re-parsing the same sample yields
  identical fingerprints (idempotency precondition).

### Slice 3 — Durable fetch + the JSON `POST /imports` branch + refresh — ✅ shipped 2026-06-30
- `app/import_fetch_worker.go` (durable, restart-safe; copy `pricing_worker.go`). ✅
- `POST /imports` JSON branch → enqueue + `202` + `previewing` batch with
  `fetch_status`. ✅
- `POST /import-connections/{id}/refresh` → incremental fetch via saved cursor. ✅
- Wire the worker start in `cmd/rekenraam/command.go` next to
  `pricingService.StartBackgroundWorker`. ✅
- Frontend: "Import from Trading 212" button → poll `GET /imports/{id}` until
  `ready` → existing preview/commit UI takes over unchanged. ✅
- **Tests (service-level, the important ones):** kill-and-restart mid-fetch resumes
  (lease expiry → reclaim); incremental refresh after a commit pulls only new
  movements and re-fetched overlap is skipped via `import_commit_identities`;
  a provider 5xx retries with backoff and does not wedge the batch; commit of
  fetched rows creates balanced `posted`+`needs_review` transactions with the
  correct resolved currency. ✅ `internal/app/import_fetch_worker_test.go` (7 tests,
  real SQLite DB + fake HTTP T212 server); the currency/needs-review assertion is a
  direct `buildTransactionSpec` unit test rather than a full ledger-commit
  integration test (no account/commodity fixture harness exists yet — see T-13).
- **Acceptance:** connect Trading 212 → import → rows land in the review queue as
  `posted`+`needs_review`; refresh a day later pulls only new movements; stopping
  the server mid-fetch and restarting completes the fetch with no duplicates. ✅
  Verified both by the automated tests above and a manual HTTP smoke test against a
  running server + fake Trading 212 endpoint (full flow: connect → import → poll →
  ready → refresh → 409 on concurrent refresh → delete connection → batch retains
  `connection_display_name` in `source_meta`).

### Slice 4 — decomposed 2026-07-01, subsequently completed

Scoping this slice for real implementation (rather than the one-line stub
above) surfaced that its two backlog items are not the same size. **4a is
small and self-contained.** **4b turned out to depend on a design gap that
didn't exist in any prior slice** — see `docs/plans/import-connection-accounts-plan.md`
(new doc) for the full writeup. Splitting them so 4a can ship independently.

#### Slice 4a — Scheduled auto-refresh (B-T212-SCHED) — ✅ shipped 2026-07-01

Delivered almost exactly as planned, with two implementation-time refinements
kept for the record:
- The scheduler and its cadence check (`runDueTrading212AutoRefreshes`) live
  on `ImportService` (`app/import_scheduler.go`), not `ImportConnectionService`
  — `RefreshImportConnection` (the method it calls) is already on
  `ImportService`, and `ImportConnectionService.ListDueAutoRefreshConnectionIDs`
  is the thin query it calls back into. Splitting the "when" (`ImportService`)
  from the "which connections" (`ImportConnectionService`) matched the
  existing service boundary rather than moving fetch orchestration onto the
  connection service.
- `ImportRepository.CurrentBookOwnerID` was added (mirroring
  `PricingRepository.CurrentBookOwnerID`) so a scheduled refresh can attribute
  itself to the real book owner, exactly like `pricing_scheduler.go` does for
  scheduled FX refreshes — a system-triggered fetch still needs a valid
  `OwnerUserID` for the batch's audit trail.

Verified by `internal/app/import_scheduler_test.go` (due-boundary at exactly
24h, disabled connections never trigger, a never-fetched connection triggers
immediately, the in-flight guard silently skips rather than double-enqueuing)
and a manual end-to-end pass: built and ran the real server, bootstrapped an
owner, created a Trading 212 connection against a local fake provider server,
and drove the actual rendered toggle in a real (Playwright-controlled)
browser — confirmed the switch renders, the click sends
`PATCH /import-connections/{id}` with `auto_refresh_enabled`, the UI reflects
the new state, and the state survives a page reload (not just optimistic
local state).

Mirrors the existing `PricingService.StartScheduler` /
`runScheduledRefreshIfDue` pattern (`backend/internal/app/pricing_scheduler.go`)
almost exactly — a once-a-minute ticker that checks "is it due" and, if so,
calls the *existing* `RefreshImportConnection` service method (Slice 3), not
new fetch logic.

- **Migration:** add `auto_refresh_enabled INTEGER NOT NULL DEFAULT 0` to
  `import_connections`. Deliberately **no** per-connection hour/minute/timezone
  policy like pricing has — simpler cadence: "due if
  `auto_refresh_enabled` and `now - last_fetched_at >= 24h` (or
  `last_fetched_at` is null)". This avoids needing the book-owner-timezone
  plumbing pricing's scheduler needs, and a fixed 24h-since-last-success
  interval is a better fit for a rate-limited third-party API than a fixed
  wall-clock time anyway (no thundering-herd at midnight, self-correcting if
  the server was down at the usual time).
- **Service:** `ImportConnectionService.runDueAutoRefreshes(ctx, logger)` —
  list connections with `auto_refresh_enabled=1` and
  `source_kind="trading212"`, filter to those whose `last_fetched_at` is
  null or `>= 24h` old, and call the same `RefreshImportConnection` path the
  manual "Refresh" button already uses (Slice 3) — including its existing
  in-flight guard (`ErrImportFetchInProgress`), which this scheduler must
  treat as a normal skip-this-tick outcome, not an error to log loudly.
- **Wiring:** `ImportConnectionService.StartScheduler(ctx, logger)` started in
  `cmd/rekenraam/command.go` next to `pricingService.StartScheduler(...)`.
- **API:** `PATCH /import-connections/{id}` gains `auto_refresh_enabled` in
  its existing config-update body (no new endpoint). Add to OpenAPI + regen
  TS types.
- **Frontend:** a toggle on each connection row in the existing connections
  table (`routes/app/import/+page.svelte`), reusing the existing PATCH call.
- **Tests:** due-detection boundary (23h59m vs 24h01m since last fetch);
  disabled connections never trigger; in-flight guard causes a skip, not a
  failure log; a connection with `last_fetched_at IS NULL` triggers
  immediately (first-time enable shouldn't wait 24h).
- **Acceptance:** enabling the toggle causes a fetch within the next minute if
  more than 24h have passed (or immediately if never fetched); disabling it
  stops future automatic fetches; the manual refresh button and the
  scheduler share one in-flight guard so they can never race each other.

#### Slice 4b — Investment lot import (B-T212-INVST) — ✅ shipped 2026-07-03

Delivered on top of `docs/plans/import-connection-accounts-plan.md`'s
`cash_account_id` column and `import_connection_holdings` table (both
shipped in the same pass — that doc was "planning only" before this). What
actually shipped, and where it differs from the original plan above:

1. **Fetcher:** `internal/onlinesource/trading212` gained `FetchOrders`
   (`/equity/history/orders`) and `FetchDividends`
   (`/equity/history/dividends`), verified against the **real, published**
   Trading 212 OpenAPI spec rather than guessed — resolving this plan's
   biggest listed risk outright. A second endpoint was required (order
   fills carry ticker/ISIN/quantity/price in a nested `{order, fill}` shape;
   dividends are a fully separate endpoint neither cash-history rows nor
   order rows ever include). Rather than duplicating `Fetch`'s
   pagination/incremental-cursor/continuation loop three times, it was
   factored into one shared generic `fetchPaginated[T]` engine
   (`fetcher.go`) parameterized per endpoint — exactly the reuse this plan
   asked for. **Correctness catch:** an order fill's actual cash effect
   (`fill.walletImpact.netValue`) is denominated in `walletImpact.currency`,
   which is **not** always the same as the order's own trading currency
   (`order.currency`) — a multi-currency account can trade a USD-priced
   instrument while its wallet nets out in EUR. `OrderFill` carries both
   (`Currency` for price, `NetValueCurrency` for the cash amount) so the
   cash leg is never posted under the wrong currency.
2. **Cursor storage:** three independently-paginated endpoints need three
   independent incremental cursors. `import_connections.fetch_cursor` was
   renamed to `transactions_cursor` and `orders_cursor`/`dividends_cursor`
   added (migration `0010`) — a straight rename/split, not a
   backward-compat shim, since no production data exists yet. The durable
   worker (`import_fetch_worker.go`) now walks three **stages**
   (transactions → orders → dividends) per logical fetch, each with its own
   maxPages/continuation chain identical to the original Slice 3 machinery;
   a stage transitions to the next only once its own pagination naturally
   exhausts, and the connection's three cursors are all persisted together
   only once the final (dividends) stage finishes.
3. **Instrument resolution:** `InvestmentService.ResolveOrCreateInstrumentForImport`
   — ISIN match (`InstrumentByISIN`, a `json_extract` scan over
   `identifiers_json` since ISIN isn't an indexed column) first, ticker/symbol
   match (`InstrumentBySymbol`, indexed) second, create third. **Deferred-
   creation invariant kept:** all of this runs at **commit** time only, not
   fetch time — the plan's discard-orphan concern is fully avoided because
   nothing durable is created until the user actually clicks commit. The
   tradeoff (also a deliberate scope cut, see below): the preview UI does
   **not** yet show a resolved/proposed instrument name before commit — a
   documented, deferred UX enhancement, not a correctness gap.
   **P2 follow-up (2026-07-12, T-29):** creation remains commit-time, but is
   now compensating and reference-safe: a row that falls back to generic cash
   or fails after creating setup reverses only its new holding map, unused
   account, and unused instrument/security commodity. Migration `0011` keeps
   instrument and commodity versions append-only after any durable reference,
   while allowing this never-used setup to be removed.
4. **Holding + cash account resolution:** `import_connection_holdings`
   lookup/create via `ImportConnectionService.HoldingAccountForCommodity`/
   `LinkHolding`, same deferred-to-commit-time rule. **Scope cut:**
   accounts-plan "scenario 2" (link to an *existing* pre-connection holding
   account, with explicit human confirmation before the first auto-link) is
   **not implemented** — resolution always either reuses a
   connection-linked account or creates a fresh one. Tracked as a follow-up
   in `docs/backlog.md` (B-T212-INVST, now the open remainder of that item).
   **New bug found and fixed while building this:** a holding account (and
   instrument) created for a back-dated fill defaulted its
   `opened_on`/`effective_from` to "today" (`CreateAccount`'s default when
   left blank) — for any fill dated before today, every posting to that
   brand-new account then failed `"posting account is invalid"`
   (`PostingAccountRule` finds no account version effective as of the
   entry date). Fixed by passing the trade's own date as
   `OpenedOn`/`EffectiveFrom` for both the instrument and the holding account.
   **Both halves of that fix were later replaced:** the instrument's by T-42
   (`db.CommodityGenesisDate`) and the holding account's by T-44 — the
   trade's own date only ever fixed the fill that arrived first, so an
   import-created holding account now opens at the genesis date
   (`docs/design/holding-account-opened-date.md`).
   The settlement-account selector and service both require a postable,
   active, non-system **asset** account (T-30/T-33); a category, closed
   account, or internal system account can no longer receive a brokerage cash
   leg through a crafted API request.
5. **Commit-path branch:** `CommitImportBatch` (`import_service.go`) now
   tries `commitTrading212InvestmentRow` (`import_trading212_invest.go`)
   before the generic path; `handled=false` (not a trading212 investment
   row, or resolution/posting failed for any reason — no cash account
   configured, insufficient lots, no dividend default, ...) falls through
   unchanged to `buildTransactionSpec`, a strict superset of pre-4b
   behavior. Both the wiring gap (`ImportService` now takes an
   `*InvestmentService`) and the attribution gap flagged during scoping were
   closed: `InvestmentTradeInput`/`DividendInput` gained optional
   `OriginType`/`Operation` override fields (empty = unchanged default
   `"browser_api"`/`"investment.buy"` etc., so every existing browser-API
   caller is unaffected) and the import commit path passes `"import"`.
   **P1 follow-up (2026-07-11, T-26):** the original implementation only
   checked `FindCommitIdentity` before posting, then wrote the identity in a
   second transaction after `Buy`/`Sell`/`Dividend` committed. That left a
   crash window which could duplicate a retried investment import. The
   investment and ordinary transaction repositories now expose a narrow
   post-write callback that runs inside their existing SQLite transaction;
   the import identity and staged-row marker are written there after the
   ledger/lot rows and before commit. Buy, sell, and dividend rollback tests
   inject an identity-write failure and prove no ledger transaction, lot, or
   disposal escapes.
   `CreateHoldingAccount` still hardcodes `OriginType: "browser_api"` (not
   fixed) — deliberately out of scope: it's a one-time structural action
   per instrument, not a recurring money-moving posting, so misattributing
   it is much lower-impact than the trade/dividend postings this slice did
   fix. Tracked in `docs/backlog.md` if it ever needs closing.
6. **Dividends:** route through `InvestmentService.Dividend`, which already
   auto-resolves the income account via `ResolveDividendDefault` keyed off
   the resolved instrument's `CommodityID` (unchanged, exactly as planned).
   Investment transactions now retain compact Trading 212 provenance in their
   metadata (`source_kind`, connection id, provider kind/id) and use the
   provider id as the external reference; buy lots and sell disposal events
   receive the same metadata (T-31).
7. **Severity-1 bug found and fixed, unrelated to Trading 212 itself:**
   `buildTransactionSpec`'s `EntryKind: "main"` was not a valid entry kind —
   see this doc's status line above and `docs/backlog.md` T-22. Discovered
   only because this slice's tests were the first in the whole suite to
   drive a staged row through the complete `CommitImportBatch` →
   `TransactionService.CreateTransaction` path against a real account.
- **Tests:** `internal/onlinesource/trading212/fetcher_test.go` — golden-file
  parses for both new endpoints against the real schema, path-correctness
  guards, incremental-cursor-on-`filledAt`/`paidOn` tests, a same-vs-different
  wallet/order currency guard. `internal/app/import_trading212_invest_test.go`
  — buy creates instrument+holding+lot, a second fill reuses the same holding
  account (not a duplicate), no-cash-account falls back to the generic cash
  path (and creates no instrument), dividend posts as investment income
  without creating a lot, and the severity-1 `EntryKind` regression test
  (source-agnostic, no Trading 212 involved).
  `TestCommitImportBatch_SameDaySellBeforeBuyStillCommitsBothAsInvestmentTrades`
  proves an intraday
  buy is ordered before a later same-day sell using Trading 212's full
  `filledAt` timestamp (T-28). Three identity-failure tests cover atomic
  buy, sell, and dividend rollback (T-26).
- **Acceptance:** met for the core flow — a buy/sell with a configured
  `cash_account_id` and either an ISIN/ticker match or a creatable new
  instrument results in a real lot via the same holding account across
  fetches; a dividend with a configured default posts as investment income;
  anything that can't resolve falls back to a plain cash row exactly like
  before this slice, never blocking the batch. The investment identity is
  atomic with the ledger write, not merely reasoned about by the outer
  preflight lookup. Sell-specific insufficient-lots fallback still lacks a
  dedicated named test.

---

## Cross-cutting rules (inherited, restated for the implementer)

- **Nothing implicit:** the fetch stages rows into preview; **only `commit`**
  writes to the ledger, through `TransactionService`. A fetch never posts.
- **Idempotent:** provider-id fingerprints in `import_commit_identities` make
  re-fetch and re-import safe; commit re-runs skip committed fingerprints.
- **Restart-safe:** all fetching is durable-queue work (ADR 0010); no fetch in a
  request handler.
- **Secrets:** API keys sealed at rest (AES-256-GCM), never logged, never returned,
  opened only in-memory for the duration of a fetch.
- **No floating point:** provider decimals → `exact.Coefficient` + scale via the
  existing `parseDecimalAmount`.
- **i18n / a11y / mobile:** connection + import UI follow the same boundaries as the
  rest of the app; loading/empty/error/success states defined per screen.
- **Auditable:** every batch state change is an `import_batch_events` row and
  committed transactions carry `OriginType="import"`. Investment rows also
  carry their Trading 212 source kind, connection id, provider kind/id in
  transaction metadata and preserve the provider id as `ExternalRefHint`;
  buy lots and sell disposal events carry the same compact source metadata.

## Risks & open questions (resolve during Slice 2 against the live API)

1. **Exact endpoint set & field names** for cash history vs order history vs
   dividends — verify and lock in golden files. Isolated in the fetcher; no
   pipeline impact.
2. **Whether settled-order cash legs appear in cash history** or must be derived
   from order history. If the former, cash import is complete; if the latter,
   keep order rows `needs_attention` (B-T212-INVST) and revisit.
3. **Rate-limit headers** — confirm `Retry-After` presence; otherwise rely on
   queue backoff.
4. **T-06 interaction:** the fetch path reuses the same commit; it inherits T-06's
   crash-consistency gap but does not worsen it. Do not attempt to fix T-06 inside
   this plan; it is a transaction-service refactor tracked separately.
5. **Pagination beyond `maxPages` — closed (T-14).** `Fetcher.Fetch` reports
   `HasMore`/`NextPageToken`; the worker chains `reason="continuation"` work items
   until a fetch naturally exhausts, rather than truncating at 50 pages. See
   `docs/backlog.md` T-14 for why the resume mechanism had to be a page token
   separate from the incremental cursor (an early draft of the fix conflated the
   two and didn't actually work — pagination always restarts at page 1).
6. **Re-staging on partial-success retry — closed (T-15), as a side effect of T-14.**
   `stageParseResult` now seeds its within-batch dedupe set from rows already
   staged by an earlier call on the same batch, so a retry that re-stages already-
   fetched movements flags them `needs_attention` instead of silently duplicating
   them as `new`. Ledger safety was never actually at risk here (commit-time
   `FindCommitIdentity` already prevented double-posting) — see `docs/backlog.md` T-15.
7. **Same-timestamp incremental cursor + absolute nextPagePath — closed (T-17).**
   The incremental boundary now uses strict `<` so a movement sharing the cursor's
   exact timestamp is re-scanned, not dropped; `fetchPage` refuses to follow an
   absolute `nextPagePath` so it can't be tricked into sending the API key to an
   untrusted host. Deeper ordering assumptions (strictly chronological pages,
   no backdated corrections) remain unverified against the live API — see
   `docs/backlog.md` T-17 for what's still open there and why.

## Prerequisites in the backlog — resolved before Slice 3 started

Two pre-existing issues were found while validating this plan against the code.
Neither blocked Slices 1–2, and both were closed (see `docs/backlog.md`) before
Slice 3 began:

- **T-10 — queue methods on `PricingRepository`.** Closed: `db.BackgroundWorkRepository`
  is now standalone (`PricingRepository` embeds it); `ImportService` takes its own
  instance, so `import → pricing` is not a dependency.
- **T-09 — queue does not coalesce enqueues.** Closed for *identical* payloads via a
  partial unique index (`(book_id, kind, payload_json) WHERE status IN
  ('pending','running')`). That alone does not stop two *different* in-flight fetches
  for the same connection (different payloads can't collide on the unique index), so
  Slice 3 still added the dedicated service-layer guard described in "Durable fetch
  worker" (`ImportRepository.HasInFlightFetch`, `ErrImportFetchInProgress` → `409`).
