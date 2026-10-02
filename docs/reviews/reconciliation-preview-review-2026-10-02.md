# Shared reconciliation preview review — 2026-10-02

Reviewed the remaining finding against `ef926d60`. The prior opening-specific
fix was sound, but left the common reconciliation resolver unchanged. This
follow-up fixes the shared root cause, tracked as T-127 #142.

## Confirmed defect and fix

`periodScopedCheckpointInvalidationRefs` admitted candidates using the latest
active checkpoint’s date/account sequence, then returned only that checkpoint.
`invalidateReconciliationCheckpoints` expanded an admitted candidate to every
active checkpoint dated at or after its entry date. Thus a 15 March sale
previewed only the 30 April checkpoint while commit invalidated both 31 March
and 30 April. Multiple dated generic postings could also repeat April in the
warning while omitting March.

The resolver now returns the complete unique set, with each checkpoint’s own
statement metadata and the earliest triggering candidate date. Both resolution
and invalidation use `activeReconciliationCheckpointRefsFromDate` for the same
book/account/commodity/active-status/date selection. The latest-boundary
eligibility rule and transactional write revalidation remain intact.

This applies to every shared resolver caller: ordinary create/update/lifecycle
and draft promotion, sells, write-offs, dividends, external/internal transfers,
reversals and replacement/source-correction impact calculations. Buy and
reinvestment still return the actual rolled-back writer’s checkpoint set.
This change establishes checkpoint-selection parity; it does not claim every
journal-planning preview now proves dependent replay feasibility.

Named regressions first failed on the old selector, then passed:

- `TestCreatePreviewReportsEveryCheckpointCommitInvalidates`: two active cash
  checkpoints, two proposed entry dates, one warning per checkpoint with earliest
  triggering date and enriched labels. Repeated previews preserve durable rows.
- `TestSalePreviewReportsEveryCheckpointCommitInvalidates`: the reported March
  sale case. Repeated previews preserve durable rows.

Both pair preview with commit: no override refuses and preserves all durable
rows; explicit override invalidates exactly the two named checkpoints.

## Remaining sequence issue and acquisition wording

The date-only cascade is not a per-boundary sequence test. A new 31 March posting
at sequence 2 falls after a March checkpoint at sequence 1, but adding an April
checkpoint makes latest-boundary admission succeed and the cascade invalidates
both. Without April, March stays active. Documented this case and the required
March-preserved/April-invalidated regression in T-120 #135; its exact combined
balance-delta and sequence rule must apply to generic transactions too. This
fix reports the current write set rather than silently changing that rule.

Keep the historical `INVESTMENT_BUY_DEPENDENCY` code for compatibility. OpenAPI
now explicitly describes it as an acquisition replay conflict also used for
reinvestment, including the reinvestment commit response. No new opening kinds
or error enums are introduced.

## Validation

The two focused regressions pass. `./scripts/test-backend.sh` passes formatting,
vet and the complete race suite (API 207.117s, application 557.219s, database
512.297s). `./scripts/test-frontend.sh` passes with zero Svelte errors/warnings
and all 422 tests in 28 files; OpenAPI generation succeeds. `git diff --check`
and changed-document local link checks pass.
