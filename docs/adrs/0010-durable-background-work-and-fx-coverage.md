# ADR 0010: Durable Background Work And FX Coverage

## Status

Accepted

## Date

2026-06-20; amended 2026-08-31 at R9 acceptance.

## Context

FX refresh currently runs synchronously for either a user request or the daily
scheduler. It requests only the provider's latest value. A newly active currency
therefore waits for the next refresh, and adding an older transaction does not
fill the newly required historical interval.

A process stop or loss of network connectivity can also leave an ingest run in
`running`. The daily scheduler considers any scheduled run for that local day to
satisfy the schedule, including failed and abandoned runs, so interrupted work is
not reliably retried.

Imports, exports, backups, and future provider ingestion will need the same
restart-safe execution behavior. A pricing-only in-memory retry loop would not be
a durable foundation.

## Decision

Rekenraam will use a generic SQLite-backed background-work queue and an
at-least-once worker model.

- Domain operations record follow-up work in the same SQLite transaction as the
  change that created the need. Network access never occurs inside that domain
  transaction. This is the transactional outbox pattern, with the queue stored in
  the same database.
- Work items have a stable kind, versioned JSON payload, status, attempt count,
  next-attempt time, lease owner and expiry, timestamps, and last-error summary.
  Payloads must contain identifiers and dates, not translated labels or financial
  record content intended for logs.
- A worker leases due items for a bounded time. A process restart or expired lease
  makes unfinished work eligible again. Handlers must therefore be idempotent.
- Retryable failures use capped exponential backoff with jitter. Connectivity,
  timeout, provider availability, and server interruption are retryable. Invalid
  payloads and permanently unsupported currency pairs are terminal and visible to
  the user. Retries are bounded: each work kind has a maximum attempt count, and
  an item that exhausts it becomes terminal rather than retrying at the backoff
  cap indefinitely. A terminal item can be explicitly retried after configuration
  changes.
- Equivalent pending FX work may be coalesced by keeping the earliest requested
  start date. Coalescing must never move the start date forward or lose work that
  arrives while an item is leased.
- Manual and scheduled refreshes enqueue the same work kind as domain triggers.
  The API reports accepted work separately from completed ingest runs; it must not
  imply that provider download completed synchronously.

FX coverage is demand-driven:

- A currency becomes active when an active account uses it. Creating, updating,
  or reopening an account in a currency enqueues FX coverage work immediately.
- Creating or updating a `posted` transaction enqueues coverage from its earliest
  journal-entry date for every currency involved. Manual entry saves directly as
  `posted`. Producer-created drafts do not enqueue or extend required coverage;
  explicit promotion to `posted` does. R9 review shows exact per-currency amounts
  and needs no converted draft totals.
- Import preview rows belong in dedicated import staging, not in the transaction
  ledger as drafts. Previewing an import does not enqueue FX work; committing
  selected rows creates transactions and does enqueue it.
- For each active currency relative to the configured FX base, required coverage
  begins at the earliest of the active account's opening date and any durable
  posted journal-entry date using that currency, and extends through
  today in the book owner's time zone.
- Refresh planning compares required coverage with stored non-voided observations
  and schedules only missing dates. Existing observations make repeated work
  idempotent.
- “Daily” means provider publication days. Weekend and holiday behavior follows
  the pricing policy; the default `skip` policy does not fabricate observations.
- `max_backfill_days` bounds one execution chunk, not the total historical horizon.
  If more history is required, the worker checkpoints progress and enqueues or
  retains continuation work until the interval is complete. Historical demand is
  never silently truncated by this setting.
- Each bounded provider batch commits before the next batch. A retry resumes from
  persisted observations/checkpoints instead of restarting the entire interval.

## Consequences

- A newly introduced account currency starts downloading without waiting for the
  next daily schedule.
- Backdated posted transactions can extend FX history by years while each worker
  execution remains bounded.
- Going offline or stopping the server delays work but does not lose it. Work is
  retried after connectivity returns or the app restarts.
- At-least-once delivery can repeat provider requests, so provider identifiers and
  observation uniqueness remain required.
- Queue state and FX coverage progress need a small settings/status read model so
  users can distinguish pending, running, retrying, completed, and terminal work.
- The generic queue may serve later background workflows, but each new work kind
  still requires an explicit idempotency and retry policy. It is not permission to
  move arbitrary business logic out of application services.

## Amendment — R9 acceptance, 2026-08-31

The original decision anticipated downloading FX for future producer drafts.
R9 now supplies that producer and reviews exact amounts separately per currency;
it does not need FX preparation before posting. This amendment deliberately
replaces the earlier draft-coverage policy with posted-only transaction demand,
matching the posted-only outbox trigger. The refresh planner now applies the
same rule (T-83), so a manual or scheduled refresh cannot indirectly introduce
draft demand. Active-account demand is unchanged. Any future converted draft
review must explicitly revisit this decision. See
`docs/reviews/r9-acceptance-review-2026-08-31.md`.
