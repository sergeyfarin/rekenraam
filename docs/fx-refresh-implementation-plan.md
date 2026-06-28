# FX Refresh And Durable Work Implementation Plan

Status: **backend shipped** (the durable work queue + demand-driven FX coverage are
implemented — see `implemented.md`, "FX & Pricing"; ADR 0010). No management UI yet
(roadmap R11). Kept for the design rationale; current state lives in `implemented.md`
and `roadmap.md`.

This plan implements ADR 0010 in independently shippable slices. Each slice keeps
the app runnable and uses the existing generic pricing observation model.

## Slice 1: Durable Work Queue

- Add a migration for generic background work, attempts, leases, and indexes for
  claiming due work.
- Add repository operations to enqueue/coalesce, lease, complete, retry, fail,
  reclaim expired leases, and inspect work.
- Start a bounded worker with the Go server and stop leasing new work during
  graceful shutdown.
- Add tests for atomic enqueue, concurrent claim exclusion, expired-lease recovery,
  work arriving during a lease, backoff, and idempotent completion.
- Route scheduled and manual FX refresh requests through the queue. Reconcile old
  `running` ingest runs as interrupted when their associated work lease expires.

Acceptance: stopping the process during an FX job and starting it again causes the
job to resume automatically; a failed scheduled attempt does not suppress retries
for the rest of the local day.

## Slice 2: Historical FX Coverage

- Add repository queries for the earliest required date per active currency and
  for missing observation dates per currency pair.
- Extend provider refresh calls with an explicit valuation date and process dates
  in bounded chunks. Prefer provider range endpoints when available, while keeping
  the normalized adapter result identical.
- Treat provider weekends/holidays according to `weekend_policy`; do not create a
  rate for a date the source did not publish under the default policy.
- Persist progress after every batch and continue beyond `max_backfill_days` until
  today is covered.
- Add tests for a two-year backfill, gaps in an otherwise populated interval,
  weekends/holidays, inverse and triangulated pairs, retries midway through a
  range, and provider idempotency.

Acceptance: adding a posted transaction two years before the first existing FX
observation eventually stores all available provider publication dates from that
transaction date through today without duplicate observations.

## Slice 3: Domain Triggers And Status UI

- Enqueue FX coverage in the same transaction when an account creation, currency
  change, or reopen makes a currency active.
- Enqueue or extend coverage whenever a durable transaction is created or updated:
  future producer-created `draft` rows, `posted` transactions, and corrections
  to an earlier date or another currency all count. Do **not** enqueue for unsaved
  entries (in-progress UI working copies with no database row) — there is nothing
  persisted to back the work. Keep import preview data in import staging and
  enqueue only when rows are committed into transactions.
- Expose queue state and historical coverage in the currency settings page using
  one backend-composed request.
- Add localized pending, running, retrying, failed, and completed states, plus an
  explicit retry action for terminal work.
- Add service and API tests proving domain persistence succeeds independently of
  provider availability and that the matching work item is never omitted.

Acceptance: a new account currency and a backdated, saved manual `posted`
transaction both create durable work immediately, while an
unsaved in-progress entry and merely previewing an import do not; going
offline shows retrying work and reconnecting lets it complete without another user
action.
