# Trading 212 Online Import Plan

A detailed, implementation-ready plan for **online import from the Trading 212
public API**, built as the first concrete `OnlineSource` on top of the import
pipeline already shipped in Slice 1 (`docs/import-plan.md`, roadmap R4).

This is the plan that realises roadmap **R7 (Online ingestion)** for one real
provider. It is written for the **Sonnet model to implement** directly: every
slice names the exact existing files/types to reuse, the new files to add, and
the acceptance test that proves it.

Governed by `docs/import-plan.md` (the unified pipeline), `docs/roadmap.md` (R7),
ADR 0010 (durable background work), and `docs/conventions.md`. Aligns with the
FX-refresh precedent in `docs/fx-refresh-implementation-plan.md`, which is the
template for the durable-fetch machinery here.

Status: **Slice 1 (Credential store + connection CRUD) shipped 2026-06-28.** Slices 2–4 pending.

Slice 1 delivered: `internal/secretbox` (AES-256-GCM), `REKENRAAM_SECRET_KEY` config,
migration `0007_online_import.sql`, `ImportConnectionRepository`, `ImportConnectionService`
(probe-before-store, key masking), 4 REST endpoints (`/api/v1/import-connections`),
OpenAPI coverage, generated TS types, `$lib/api/connections.ts` client, connections UI
on the import page (masked list, add form, delete with confirm). Last updated 2026-06-28.

---

## Why Trading 212 first

- It is the user's concrete second source after the MS Money (QIF) migration.
- Its public API is **token-based** (a personal API key minted in the app), not
  OAuth/PSD2 — so it proves the online model end-to-end **without** building an
  OAuth dance or open-banking connectivity first (both explicitly deferred in
  `docs/import-plan.md` non-goals).
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
| `import_commit_identities` idempotency (`UNIQUE(book_id, dedupe_fingerprint)`) | `migrations/0004_import_core.sql`, `db/imports.go` | Provider transaction id → fingerprint precedence (already described in `import-plan.md` "Dedupe identity"). Re-fetch overlap is a no-op. |
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
- New table `import_connections` (migration `0005_online_import.sql`):
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

## Data model (new migration `0005_online_import.sql`)

Goose migration, additive only. Follows the column conventions in
`0004_import_core.sql` (denormalized `book_id`, RFC3339 text timestamps).

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
  Trading 212 (`/equity/account/info` or equivalent) and returns `201` with the
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
| type (`DEPOSIT`/`WITHDRAWAL`/`DIVIDEND`/`INTEREST`/`FEE`/`CARD`/…) | `Memo`/`Raw["type"]`, drives default category hint | Cash movements map cleanly. |
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
- `0005_online_import.sql`: `import_connections` (+ `connection_id` on
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

### Slice 3 — Durable fetch + the JSON `POST /imports` branch + refresh
- `app/import_fetch_worker.go` (durable, restart-safe; copy `pricing_worker.go`).
- `POST /imports` JSON branch → enqueue + `202` + `previewing` batch with
  `fetch_status`.
- `POST /import-connections/{id}/refresh` → incremental fetch via saved cursor.
- Wire the worker start in `cmd/rekenraam/command.go` next to
  `pricingService.StartBackgroundWorker`.
- Frontend: "Import from Trading 212" button → poll `GET /imports/{id}` until
  `ready` → existing preview/commit UI takes over unchanged.
- **Tests (service-level, the important ones):** kill-and-restart mid-fetch resumes
  (lease expiry → reclaim); incremental refresh after a commit pulls only new
  movements and re-fetched overlap is skipped via `import_commit_identities`;
  a provider 5xx retries with backoff and does not wedge the batch; commit of
  fetched rows creates balanced `posted`+`needs_review` transactions with the
  correct resolved currency.
- **Acceptance:** connect Trading 212 → import → rows land in the review queue as
  `posted`+`needs_review`; refresh a day later pulls only new movements; stopping
  the server mid-fetch and restarting completes the fetch with no duplicates.

### Slice 4 (follow-up, scoped not built here)
- Scheduled auto-refresh toggle per connection (B-T212-SCHED): a domain trigger
  enqueues `import.fetch.trading212` on a daily cadence, same machinery.
- Investment lot import (B-T212-INVST): map order fills to buys/sells and dividends
  to investment events through the investment service (its UI now ships — R12), so
  this is buildable as part of this work rather than blocked on a prerequisite.

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
- **Auditable:** every batch state change is an `import_batch_events` row;
  committed transactions carry `OriginType="import"` and Trading 212 metadata
  (movement id, type) in `MetadataJSON`.

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

## Prerequisites in the backlog (read before Slice 3)

Two pre-existing issues were found while validating this plan against the code.
Neither blocks Slices 1–2, but Slice 3 (the durable worker) depends on them:

- **T-10 — queue methods on `PricingRepository`.** Extract a standalone
  `BackgroundWorkRepository` first (pure move) so `import → pricing` is not a
  dependency. Prerequisite for the worker.
- **T-09 — queue does not coalesce enqueues.** Until fixed, the refresh handler
  guards against a duplicate in-flight fetch at the service layer (described in
  "Durable fetch worker"). Provider-id dedupe keeps the *committed* result correct
  regardless; the guard only prevents confusing duplicate preview batches.
