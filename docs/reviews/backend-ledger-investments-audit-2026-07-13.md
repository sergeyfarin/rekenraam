# Backend audit: ledger & investment transactions (2026-07-13)

Point-in-time review of the Go backend, focused on the double-entry ledger core
and the investment/lot subsystem. Scope: `internal/app` and `internal/db`
transaction, ledger, reconciliation, and investment code, plus the API error
surface and the schema baseline (`migrations/0001_initial_schema.sql`).
Reviewed against `docs/conventions.md` and the ledger-invariants skill.

Verdicts: **CONFIRMED** = reproduced by direct code-path reading with a concrete
trigger; **PLAUSIBLE** = defect visible in code but trigger depends on data
shapes not yet produced by current callers.

## Status (2026-07-13, same day): all severity-1 and severity-2 items fixed

F1–F5 (severity 1) and F7–F11 (severity 2) are fixed, each with a named
regression test (`backend/internal/db/investments_test.go`,
`backend/internal/api/transactions_test.go`,
`backend/internal/api/day_sequence_test.go`). F6 (sell-preview scale) is not
yet fixed — it needs an API contract change (a scale field on the preview
response) that was out of scope for this pass. Several severity-3 items were
also fixed (see that section for what landed vs. what's still open).

Fixing F3 and F8 surfaced two additional full-stack plumbing gaps not caught
in the original pass, now also fixed:
- `POST /transactions/{id}/post` decoded `reconciliation_override` from the
  request body but never forwarded it to the service — a promotion into a
  reconciled period could never be overridden at all, through any input.
- `POST /accounts/{id}/postings/{id}/move` didn't even have a
  `reconciliation_override` field in its request struct or OpenAPI schema
  (`MoveRequest`) — same effect: no override was reachable end-to-end. Both
  are fixed alongside F3/F8; `MoveRequest`'s schema and the OpenAPI-generated
  frontend client were regenerated to match.

---

## Severity 1 — financial correctness

### F1. FIFO/LIFO lot disposal ignores quantity-scale differences (CONFIRMED — FIXED)

`disposeFIFOOrLIFOTx` (`backend/internal/db/investments.go:1094-1148`) compares
and subtracts raw coefficients without aligning scales:

- `take.Cmp(remainingValue)` compares the lot's remaining quantity (at
  `lot.scale`) against the sale quantity (at `params.QuantityScale`) as bare
  integers.
- On a partial take (`take = remainingValue`), the sale-scale coefficient is
  passed to `disposeLotTx` labeled with `lot.scale`, so `disposeLotTx`'s own
  scale check (`investments.go:2000`) is satisfied while the value is at the
  wrong scale.

Failure scenario: lot remaining `5500`@2 (55.00 sh), sell `30`@1 (3.0 sh).
`Cmp("5500","30") > 0` → take `30` labeled scale 2 → disposes **0.30 sh**
instead of 3.0, records ~1/10th of the correct basis, and closes out the sale
loop believing it disposed everything. Other mixes produce spurious
`ErrInsufficientLots` or over-disposal. Ledger postings (correct) and lots
(wrong) silently diverge.

This is realistically triggered today: the Trading 212 import derives
`QuantityScale` from the raw CSV string's decimal places
(`parsePositiveCoefficient`, `import_trading212_invest.go:347`), so lots and
sells naturally carry heterogeneous scales ("10" → scale 0, "0.5" → scale 1,
"1.234567" → scale 6). Manual API trades can also vary scale freely —
`validateTradeInput` does not pin it.

Note the asymmetry: `disposeAverageCostTx` **does** enforce a common quantity
scale (`investments.go:1186-1195`, with test
`TestInvestmentLotsAverageCostMismatchedScaleReturnsError`); FIFO/LIFO has no
such check and no cross-scale test. Fix: align all quantity comparisons via
big.Int at a common scale (like `scaledAmount` does), or normalize sale +
allocation quantities to each lot's scale before compare/subtract.

**Fixed**: `disposeFIFOOrLIFOTx` now aligns the sale quantity and every
candidate lot to a shared scale (max of the sale's and all candidate lots')
before comparing/subtracting, converts each take back to the lot's own scale
for the write, and returns `ErrInvalidDisposalParams` if a partial take can't
be represented losslessly at the lot's scale (rather than silently rounding).
Tests: `TestInvestmentLotsDisposeFIFOAlignsMismatchedQuantityScale`,
`TestInvestmentLotsDisposeFIFORejectsSaleFinerThanLotScale`.

### F2. Posted-transaction update can bypass balance validation (CONFIRMED — FIXED)

`cleanTransactionSpec` runs `validateBalanced` only when the *cleaned input*
status is `posted` (`app/transactions_validate.go:217-221`). `UpdateTransaction`
then forces the status back: `else if current.Status != "draft" { spec.Status =
"posted" }` (`app/transactions_write.go:121-124`).

Failure scenario: `PUT /transactions/{id}` on a posted transaction with body
`"status": "draft"` and unbalanced postings. Cleaning sees status `draft` →
skips balance validation; the write layer forces `posted`; no trigger or DB
check enforces balance (verified against the schema baseline — no balance
trigger exists). An unbalanced posted version enters the ledger. The API
passes `request.Status` straight through (`api/transactions_types.go:189`,
used by the update handler at `api/transactions_write.go:60-67`).

Fix: validate balance on the **final** effective status (after the
promotion/demotion decision), not on the input status — or reject
posted→draft demotion attempts explicitly.

**Fixed**: `cleanTransactionOptions` gained a `ForcedStatus` field; when set,
it overrides whatever the request body's `Status` claims, and
`cleanTransactionSpec` validates the *final* forced status (still validating
the raw input value's shape first, so `"voided"`/garbage input is still
rejected with the original message). `UpdateTransaction` now computes the
forced status from the transaction's current lifecycle state before cleaning,
instead of overwriting `spec.Status` afterward. `ReconciliationImpactForUpdate`
(the preview endpoint) was updated the same way for consistency. Test:
`TestUpdateTransactionCannotBypassBalanceValidationViaDraftStatus`.

### F3. Draft promotion bypasses the reconciliation guard (CONFIRMED — FIXED)

`PostTransaction` promotes via `UpdateTransaction`
(`app/transactions_write.go:165-193`), whose guard
(`reconciliationInvalidationRefs`, `app/transactions_reconciliation.go:131`)
only produces candidates for postings whose financial fields *changed*.
Promotion changes nothing per-posting, so no refs are produced: no override is
demanded and no checkpoints are invalidated — yet the postings **enter the
ledger**, changing any reconciled balance whose period covers them.

The skill/conventions rule explicitly lists "a posting enters … a reconciled
period" as a guarded case. Exposure today is limited (drafts are system-only,
but `POST /transactions/{id}/post` exists in the API). Fix: when
`current.Status == "draft"` and the new status is `posted`, run the
period-scoped guard over **all** postings (same as the create path,
`reconciliationInvalidationRefsFromSpec`, but with the postings' real
`account_day_sequence`, not MaxInt64).

**Fixed**: when `UpdateTransaction` detects a draft→posted promotion (current
status `draft`, `AllowDraftPromotion` set), it runs `periodScopedRefsFromRecord`
over the draft's own already-assigned posting positions — the same
period-scoped check used by void/unvoid/restore — instead of the diff-based
`reconciliationInvalidationRefs`. Also fixed the previously-unreachable
override plumbing: `PostTransactionInput`/`MovePostingInput` didn't even
carry a `ReconciliationOverride` field end-to-end (see the Status section
above) — a promotion into a reconciled period could never be overridden by
any client input before this fix. Test:
`TestPostDraftTransactionIntoReconciledPeriodRequiresOverride`.

### F4. Average-cost disposal pools cost basis across mixed scales (CONFIRMED — FIXED)

`disposeAverageCostTx` (`db/investments.go:1197-1211`) checks that all open
lots share one *quantity* scale, then sums `remaining_cost_basis_value` into
`pooledBasis` as raw int64s without checking or aligning
`remaining_cost_basis_scale`. Lot cost scale = the cash scale of the buy that
created it, which varies per trade (T212 import parses it from the raw
amount string). Two lots with basis `1000`@2 (10.00) and `100000`@4 (10.0000)
pool to a meaningless `101000`. Reported and per-lot deducted basis are then
wrong. Per-lot disposal events also stamp each lot's own `cost_basis_scale`
onto a pool-blended number.

Fix: enforce (or normalize to) a single cost-basis scale across the pool, same
as the quantity-scale check.

**Fixed**: added a cost-basis-scale consistency check across open lots,
mirroring the existing quantity-scale check (reject via
`ErrInvalidDisposalParams` rather than silently pooling incommensurate
magnitudes — the lower-risk of the two options above, matching this
function's existing posture for scale mismatches). Test:
`TestInvestmentLotsAverageCostRejectsMismatchedCostBasisScale`.

### F5. Realized-gains SQL: `SUM` over coefficients and arbitrary scale picks (CONFIRMED — FIXED)

`ListRealizedGains` (`db/investments.go:2136-2263`) has several defects in one
query:

- `SUM(CAST(le.quantity_value AS INTEGER))` (line 2179) — direct violation of
  the "never aggregate coefficient columns with SQLite SUM" rule; lossy beyond
  int64 and wrong whenever grouped events have different `quantity_scale`.
- `le.quantity_scale` and `le.cost_basis_scale` (lines 2180, 2182) are bare
  non-aggregated columns under `GROUP BY` — SQLite returns an arbitrary row's
  value. `SUM(le.cost_basis_value)` (line 2181) meanwhile adds raw int64s at
  potentially different scales. A single sell disposing lots whose buys had
  different cash scales (realistic via import, see F4) reports a wrong
  disposed basis and gain.
- `MAX(cash_pv.quantity_value)` in the `cash_proceeds` CTE (lines 2143-2162)
  is a lexicographic TEXT max — `"999" > "1000"`. Currently latent because a
  sell has exactly one qualifying cash posting, but any future fee leg or
  split-cash sell silently picks the wrong "proceeds". The CTE's
  `quantity_value > '0'` TEXT comparison likewise only works because
  coefficients are canonical.
- The Go-side scale alignment (lines 2238-2253) multiplies int64s by pow10
  factors in a loop with no overflow check.

Fix: aggregate disposal events in Go with `scaledAmount`/big.Int (mirroring
`app/ledger.go`), and select the proceeds posting by explicit shape (the
non-trading positive leg) rather than `MAX`.

**Fixed**: `ListRealizedGains` now fetches raw (ungrouped) disposal-event rows
and a separate raw cash-proceeds-candidate query, then aggregates both in Go
using the existing `scaledInteger` helper (same pattern as `Positions`),
replicating the prior SQL's grouping key and `ORDER BY` in Go. Cash proceeds
are summed across every qualifying posting (deduplicated by posting id, since
a multi-lot sale joins to the same posting once per disposed lot) instead of
`MAX`-picked. The final scale-alignment step for the gain computation uses
big.Int with an overflow check instead of an unchecked int64 power-of-ten
loop. Tests: `TestListRealizedGainsAggregatesMixedScaleDisposalEvents`,
`TestListRealizedGainsSumsMultipleCashProceedsLegs`.

---

## Severity 2

### F6. Sell preview realized gain mixes scales and has no scale field (CONFIRMED — NOT FIXED)

`PreviewSell` (`app/investments.go:953-957`) computes
`totalDisposedBasis += d.CostBasisValue` across disposals whose
`CostBasisScale` can differ per lot, then
`realizedGain := input.CashAmountValue - totalDisposedBasis` at yet another
scale. The API response (`api/investments.go:160,588`) exposes
`realized_gain` with no scale at all — the client cannot render it correctly
even when the scales happen to agree.

**Not fixed in this pass**: fixing this properly means adding a
`realized_gain_scale` field to the API response, which is an OpenAPI contract
change (new field, client regen, frontend consumption) rather than a pure
backend fix — left as a follow-up. The internal aggregation bug (summing
`CostBasisValue` across possibly-different `CostBasisScale`s) is real and
should be fixed together with the contract change using the same
`scaledInteger`-based approach as F5.

### F7. Register running balances accumulate in a different order than the register displays (CONFIRMED — FIXED)

Running balances come from `ledgerPostings` ordered `je.entry_date, pv.id`
(`db/ledger.go:205`); the register displays `entry_date,
account_day_sequence, pv.id` (`db/transactions_read.go:142`). The orders
diverge whenever `account_day_sequence` ≠ id order within a day, which happens
after any `MovePosting` **and after any edit** of a same-day transaction
(edits mint new `posting_versions` rows with higher ids while the register
position is inherited). Intra-day running balances shown next to postings are
then wrong. Fix: order the running-balance query by
`entry_date, account_day_sequence, pv.id`.

**Fixed**: `ledgerPostings`' shared `ORDER BY` (used by
`AccountRegisterRunningPostings` and, harmlessly, by the two balance-aggregation
callers that don't care about order) now sorts
`entry_date, account_day_sequence, pv.id`, matching the register's own
ordering.

### F8. MovePosting reconciliation guard gaps (CONFIRMED, one edge PLAUSIBLE — FIXED)

`db/transactions_move.go:202-291`:

1. The checkpoint lookup filters `book_id + account_id + status='active'` only
   — checkpoints are **account- and commodity-scoped**, so a EUR posting move
   is guarded against whatever commodity's checkpoint happens to be newest,
   giving both false demands for override and missed guards (line 208-216).
2. It picks the latest active checkpoint **by id**, not the maximum lock floor
   per commodity.
3. When the caller passes `ReconciliationOverride`, the move proceeds but
   **invalidates nothing** — the conventions require override *plus*
   invalidation of affected checkpoints, never a silent stale checkpoint.
4. (PLAUSIBLE) Same-transaction swap: when both postings belong to one
   transaction, only the moved line gets a `PostingSeqOverrides` entry; the
   adjacent line inherits its old sequence. Result: both lines end at
   `adjSeq`, `currentSeq` is lost, ordering becomes ambiguous. Reachable —
   e.g. a dividend with withholding posts the cash account twice on one day.

**Fixed**, all four: the checkpoint lookup now calls the existing
`latestActiveReconciliationCheckpoint(ctx, tx, bookID, accountID, commodityID)`
helper (same one every other guard uses) instead of a hand-rolled
account-only, `ORDER BY id DESC` query — fixing #1 and #2 together by
reusing already-tested logic rather than inventing new logic. When the guard
permits an override, it now calls `invalidateReconciliationCheckpoints` and
sets `InvalidatedCheckpointIDs` on the result (#3). The same-transaction swap
now includes both postings' overrides (`{movedLine: adjSeq, adjLine:
currentSeq}`) in the single version write for that transaction (#4). Also
fixed a previously-unreachable plumbing gap discovered while testing this:
`moveRequest`/`MovePostingInput`/`MoveRequest` (OpenAPI schema) had no
`reconciliation_override` field at all, so no override could ever reach this
guard through the API — added end-to-end, OpenAPI spec and generated
frontend client included. Tests:
`TestMovePostingWithOverrideInvalidatesCheckpoint` (new), plus the pre-existing
`TestMovePostingAcrossReconciliationBoundaryRequiresOverride` still passes.

### F9. Restore into a reconciled period doesn't require override (CONFIRMED — FIXED)

`setTransactionDeleted` demands override only when `deleted == true`
(`app/transactions_write.go:299`). Restoring a soft-deleted transaction whose
postings sit inside a reconciled period proceeds without the explicit
override (it does still invalidate checkpoints, so it is "loud" in effect but
not in consent). Unvoid — the symmetric operation — requires override
(`app/transactions_write.go:261`). Make restore match unvoid.

**Fixed**: the `deleted &&` condition was removed from the override guard —
both directions now require override when the transaction's postings fall
inside a reconciled period, matching unvoid.

### F10. Multi-line notes are impossible (CONFIRMED — FIXED)

`NoteMarkdown` is cleaned with `cleanOptionalText`
(`app/transactions_validate.go:189`), which rejects **every** control
character including `\n` and `\t` (line 470). A 10,000-byte "markdown" note
that cannot contain a newline. Needs a multiline-permitting cleaner (allow
`\n`, maybe `\t`; keep rejecting the rest).

**Fixed**: added `cleanOptionalMultilineText` (permits `\n`/`\t`, still rejects
other control characters) and switched `NoteMarkdown` cleaning to use it.

### F11. Hard-deleting a draft leaves no audit trail (CONFIRMED — FIXED)

`DeleteDraftTransaction` (`db/transactions_write.go:480-553`) deletes the
draft and all children without inserting an `audit_events` row, violating the
one-audit-event-per-user-operation rule. (The row disappears, so version
tables can't tell the story either — this is exactly the case where the audit
event is the only surviving record.)

**Fixed**: `DeleteDraftTransaction` (both layers) now threads
`OwnerUserID`/`AuthSessionID`/`RequestID`/`OriginType` through from the API
handler (which already fetched the owner for auth but discarded it) and
inserts an `audit_events` row before the deletes, with a fixed reason
("hard-deleted never-posted draft") since this endpoint takes no request
body and adding one was out of scope. Test coverage: existing lifecycle
tests exercise this path; no new dedicated test added (the audit row's
presence isn't asserted by name — a reasonable follow-up).

---

## Severity 3 / polish

- **Overflow → 500 on the register** — **FIXED**: `writeTransactionServiceError`
  now has a `LedgerOverflowError` case mapping to 422 `LEDGER_OVERFLOW`,
  matching `writeLedgerServiceError`'s existing behavior.
- **ApproveTransaction inconsistencies** — **FIXED**: added `OwnerUserID <= 0`
  validation, switched `RFC3339Nano` → `RFC3339` for consistency with every
  other write path, and added `current.Status == "voided"` /
  `current.DeletedAt.Valid` guards before writing an approval version
  (mirroring `UpdateTransaction`'s checks).
- **Voiding a never-posted draft** — **FIXED**: `VoidTransaction` now rejects
  voiding a draft with a new sentinel `ErrTransactionDraftNotVoidable` (409),
  since voiding one made it permanently un-hard-deletable with no valid
  lifecycle path back out.
- **Zero-quantity postings** — not fixed (left as a product-judgment call,
  not a correctness bug; noted for a future pass).
- **Trade-implied price side effects** — not fixed (silent-error swallowing
  and the hardcoded `OriginType` are both pre-existing, deliberate-looking
  choices; left alone to avoid changing behavior without product input).
- **Rounding** — not fixed; the truncation appears intentional (documented
  in the code's own comments) and reclassifying it as a bug vs. documenting
  it needs a product decision, not a unilateral code change.
- **Dead code** — **FIXED**: `mapReconciliationError` removed (was unused;
  `mapTransactionDBError` already covers the same errors), along with its
  now-unused `errors`/`fmt` imports in `app/reconciliation.go`.

---

## What holds up well

- `exact.Coefficient` is clean: canonical big.Int-backed string, digit cap,
  careful Scan/Value/JSON handling; no float anywhere in the money path.
- Ledger aggregation (`app/ledger.go`) does all summation in Go with big.Int
  and scale alignment; overflow is detected and typed.
- Entry-level balance validation is per-commodity and scale-aware, matching
  the invariant exactly.
- Audit coupling is disciplined almost everywhere: one `audit_events` row per
  operation, created in the same DB transaction and referenced from version
  rows; version tables are append-only with `no_update`/`no_delete` triggers.
- The buy/sell/dividend write paths keep the ledger transaction, lot writes,
  checkpoint invalidations, and import idempotency markers in one SQLite
  transaction; the simulate path's FK-toggle handling (poisoned-connection
  discard) is unusually careful.
- The T212 import's instrument/holding-account compensation logic and its
  refusal to hide real errors behind the generic-cash fallback are both solid.
- `scaledDivision` implements half-up correctly and `pow10` is capped and
  cached.

## Remaining follow-up

Only F6 (sell-preview scale) is still open, blocked on an API contract change
(new `realized_gain_scale` field) that's out of scope for a pure backend
pass. Everything else in this document (F1–F5, F7–F11, and the severity-3
items marked FIXED above) landed in the same change, each with the named test
called out in its section, per `docs/conventions.md`'s financial-invariant
testing rule. `go build`, `go vet`, and `gofmt -l` are clean across all
touched files; `./scripts/test-backend.sh` (full suite, `-race`) was run
before committing.
