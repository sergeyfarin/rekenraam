# Investment follow-up review — 2026-10-02

Reviewed the five remaining points against `73994e92`, the actual investment
writer, effective-revision/correction readers, regression tests and live GitHub
issue bodies. This follows the [initial review](investment-review-2026-10-02.md).

| Point | Verdict and action |
|---|---|
| 1. Reinvestment previews skip replay | **Confirmed and fixed.** Commit already admits earlier reinvestments through `createTransactionAndLot`; the preview only planned journals. Share the exact reinvestment preparation between preview and commit and run the same writer in a rolled-back transaction. No gain-disclosure work is required to fix feasibility. |
| 2. #129 arithmetic/test assertion | **Confirmed and corrected.** FIFO buy 100.00 / sell 100.00 / earlier buy 20.00 gives basis 50.00 → 10.00 and gain 50.00 → 90.00. The actual fixture uses buy 200.00 / sell 150.00 / earlier buy 20.00, giving basis 100.00 → 10.00 and gain 50.00 → 140.00. #129 now names the latter inputs; the test now asserts both gains. |
| 3. #129 bottleneck | **Confirmed with a qualification.** Seven issue bodies document dependencies on #129: #130, #131, #132, #133, #134, #137 and #138. Split it into shared mechanism/pilot T-114 #129 and existing-path rollout T-126 #141. New commands integrate #129 themselves and need not wait for all legacy wiring. Safety rollout remains open P1 work. |
| 4. Priority labels | **Partly accepted.** There were twelve P1 issues and no P0. Labels describe urgency, not a complete dependency order; independent trust defects can remain P1. Move the “Then” correction families #130/#131 to P2, matching #133–#135, and keep the roadmap/TODO/#120 as the ordered queue. No P0 is required merely to create an ordering. |
| 5. Duplicate checkpoint calculation | **Confirmed, with an actual mismatch.** The old preview could return only the latest active checkpoint while the writer invalidated that checkpoint and an earlier affected one. Return references hydrated from the actual writer invalidation IDs inside its rolled-back transaction. This fixes an underreported warning, rather than only reducing hypothetical drift. |

## Shared mechanism boundary

`persistInvestmentReplayProjectionTx` is a useful shared capture point for revised
allocations, but comparing revisions there alone is insufficient. A reversal can
remove a disposal from the effective intent set; a replacement can supersede its
identity before replay projection persistence runs. Snapshot the effective
before-state before corrective mutations, then compare the resulting effective
state, including removals/replacements and known/unknown transitions.

#129 now requires a usable exact impact/acknowledgement API/UI contract, a
representative manual-buy pilot, stable identity and stale acknowledgement
revalidation. #141 owns the explicit existing-path coverage matrix. Splits #137
and pooled transfers #138 depend on the shared mechanism only, but must integrate
its safeguards themselves. Neither ticket claims gain disclosure already ships.
A permanent filed-through date remains an unaccepted option.

Updated the active plan, roadmap, TODO, backlog, feature ledger and GitHub
#114/#120/#129/#130/#131/#137/#138; created #141. Historical review evidence stays
intact with a follow-up link. Updated both opening-preview OpenAPI descriptions
and their existing dependency-conflict response, then regenerated client types.

## Regression evidence

Before the fix, a backdated reinvestment preview returned success for a changed
internal-transfer carried basis that commit refused. The named regression now
checks the same existing dependent operation in preview and commit, with no
temporary decision ID or durable changes. Reinvestment reuses the existing
acquisition dependency conflict mapping, so the API can report that refusal.

A successful reconciled holding case previews twice and compares every durable
row, then proves commit refuses without its own override and succeeds with it,
invalidating the exact previewed checkpoint and producing gain 140.00. The buy
fixture includes two active cash checkpoints; preview and commit now name both,
and asserts gain 50.00 before and 140.00 after replay.

Named regressions:

- `TestReinvestmentPreviewRejectsChangedTransferBasisWithoutWriting`
- `TestReinvestmentPreviewReplaysAndRollsBackReconciledHolding`
- `TestBuyPreviewRejectsChangedInternalTransferBasisWithoutWriting`
- `TestBuyPreviewReplaysAndRollsBackReconciledHistory`

Validation: the focused buy/reinvestment tests pass. OpenAPI generation and
`./scripts/test-frontend.sh` pass: zero Svelte errors/warnings, 28 test files and
422 tests. `./scripts/test-backend.sh` passes formatting, vet and the complete
race suite (API 202.163s, application 556.015s, database 514.599s).
`git diff --check` and changed-document local link checks pass.
