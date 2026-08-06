---
name: background-work
description: Durable background work queue patterns for Rekenraam (ADR 0010) - workers, schedulers, leases, idempotency, cursors, online provider fetches. Use when adding or changing scheduled/async work, FX or price refresh, online import fetching, or any provider polling.
---

# Durable Background Work

Restart-sensitive operations (provider downloads, FX backfill, online import
fetches) run through the SQLite-backed **at-least-once** work queue
(`background_work_items`, ADR 0010). Reference implementations:
`app/pricing_worker.go` (original), `app/import_fetch_worker.go` (most
hardened — copy this shape for new workers).

## Core rules (from ADR 0010 + conventions)

- **Enqueue atomically with the originating domain change** — same DB
  transaction. **Never perform network access inside that transaction.**
- Handlers are **idempotent** (at-least-once delivery means re-runs happen).
- Work is **leased** for a bounded period; expired leases are reclaimed.
- Classify failures: **terminal** (e.g. 401/403 bad credentials → fail fast,
  mark the domain object failed) vs **retryable**. Retries are always
  **bounded** — every worker needs an attempt cap that flips the item to
  `failed` (both workers use 8; T-39 was the FX worker not having one, so it
  retried at the 6h backoff cap forever). Write failure state where the
  frontend actually polls (the import worker writes both
  `import_batches.status` and `source_meta_json`).
- A cap is only safe with a way back: surface failed items in a read model the
  UI already loads and expose a manual re-enqueue
  (`RequeueBackgroundWork` resets `attempts` and refuses when an equivalent
  item is already active — one live copy per payload).
- `EnqueueBackgroundWork` (`db/background_work.go`) coalesces exact active
  duplicates via a partial unique index on `(book_id, kind, payload_json)
  WHERE status IN ('pending','running')` and returns the existing item;
  equivalent work can be enqueued again after completion.

## Scheduler shape

Pattern: a once-a-minute ticker service started from `command.go`
(`PricingService.StartScheduler`, `ImportService.StartScheduler`).
- Prefer "due when last-run is null or ≥ N hours old" over fixed wall-clock
  times (self-corrects after downtime, avoids timezone plumbing) — this is why
  `import_scheduler.go` differs from `pricing_scheduler.go`.
- A scheduled run needs a real owner for audit attribution
  (`CurrentBookOwnerID` pattern).
- Treat "already in flight" sentinels (`ErrImportFetchInProgress`) as a normal
  per-tick skip, never an error.
- If a schedule is user-facing wall-clock, store local time + IANA zone and
  record actual runs as UTC (DST rule in conventions § Data).

## Hard-won lessons — each of these was a real bug (backlog T-14..T-17)

1. **Guard-then-create is a TOCTOU race.** In-flight check, domain insert, and
   work-item enqueue must be ONE repo-layer transaction
   (`ImportRepository.StartOnlineImportBatch`). Under `SetMaxOpenConns(1)` the
   transaction is a full lock. Also: if enqueue fails after the insert
   committed separately, the domain object is stranded "in progress" forever.
2. **Page budgets must not silently truncate.** If a fetch has a page cap,
   report `HasMore`/`NextPageToken` and chain a `reason="continuation"` work
   item; only persist the incremental cursor once the whole chain naturally
   exhausts. Keep the resume token separate from the incremental boundary —
   they are different concepts.
3. **Incremental cursors: stop with strict `<`, not `<=`.** Distinct items can
   share a timestamp across a page boundary; re-scanning a known item is
   idempotent (dedupe keys on provider ID), dropping one loses money movements.
4. **Retries re-stage:** staging inserts must seed their within-batch dedupe
   set from rows already staged for that batch, and continue `row_index` from
   the existing count — a retry or continuation chunk must not create
   duplicate "new" rows.
5. **Never follow absolute URLs from a provider response** with credentials
   attached. Only paths relative to the configured base URL get the
   `Authorization` header (`trading212/fetcher.go` refuses absolute
   `nextPagePath`). A compromised provider response must not be able to
   exfiltrate the user's API key.
6. Respect `Retry-After` on 429s (see `trading212.Fetcher`).

## Provider adapters

- Providers return **normalized facts only** — they never mutate ledger,
  pricing, lot, dividend, or corporate-action tables. Commit/normalization
  belongs to services.
- Provider order of preference: built-in Go adapters → declarative HTTP
  adapters → external plugins (conventions § market-data providers).
- Provider secrets: operator config (`REKENRAAM_SECRET_KEY`-sealed
  `internal/secretbox` for stored credentials, env vars for global keys) —
  never plaintext in business tables.
- External market data is untrusted input; events become suggestions unless an
  automation rule explicitly allows auto-posting (see `ledger-invariants`).

## Testing workers

Drive the real worker against a temp DB (`db.Open` + `t.TempDir()`) and a fake
HTTP server. Must-have cases (all exist in `import_fetch_worker_test.go` /
`fetcher_test.go` — mirror them): concurrency race (N goroutines, exactly one
wins), failure-leaves-no-stranded-state, continuation across chunks without
duplication, cursor boundary at exact timestamp, terminal-vs-retryable
classification. Use test seams like `SetMaxPagesForTest` rather than giant
fixtures.
