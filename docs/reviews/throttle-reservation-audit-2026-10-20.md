# Throttle reservation repair and consumer audit — 2026-10-20

Scope: [#366](https://github.com/sergeyfarin/rekenraam/issues/366), following
[#360](https://github.com/sergeyfarin/rekenraam/issues/360), under
[#260](https://github.com/sergeyfarin/rekenraam/issues/260). Baseline:
`e484955e752c74ced428cf0706156111a8d21376`. The implementation, named tests,
migration and evidence are delivered together; the issue closing comments
record the exact pushed commit and remote CI state.

## Contract and repair

Each admitted scope pins a persisted random generation. An expired block or
exhausted in-flight window starts a new generation. Each refund, failure and
successful reset identifies the admitted generation. A late verdict cannot
refund, debit, block, reset or recreate a replacement/missing generation.
Identity is independent of timestamps and survives restart. It is never
reused when a scope row is deleted and recreated at the same logical time.

Admitted attempts may finish after rollover. Their original admission, not
the time verification completes, owns the guess. A superseded failure still
returns the normal credential/code rejection and records its failure event;
it does not consume the replacement budget or inherit that generation's block.
An already blocked generation retains its original deadline, even if a late
failure finishes after the block expires. New admission starts the next
window. Unsettled places still count against their generation and age out.
This bounds admissions per generation, not every rolling interval of verdict
timestamps across generations. A successful reset intentionally admits a fresh
budget, as before.

A success clears only its admitted username, approved-device or settings-session
generation. The IP scope only refunds that attempt's place, so owner success
cannot clear another attempt’s shared-address block. A success from an old
generation can complete its verified workflow while leaving all replacement
budgets intact. Scope selection, reauthentication isolation, MFA deadline and
revocation cutoffs, session/device atomicity and event policy remain in force.

Migration `0020_throttle_generations.sql` adds the identity column and assigns
fresh identities to existing rows without changing their counters or deadlines.
No installed migration is rewritten. Fresh startup, baseline-to-head schema
parity, populated 19→20 upgrade, restart/idempotent migration and local down/up
are covered. Production rollback continues to require the verified pre-upgrade
backup and previous binary; down migration is a development control. The
ledger bundle format is unchanged: throttle rows are operational state in the
native database, not ledger CSV/QIF facts.

## Independent oracle and deterministic overlap

The independent budget is five places. The synthetic start is
`2026-10-20T12:00:00Z`; rollover occurs exactly fifteen minutes later. The test
holds one real old consumer plus four additional reservations, advances the
existing service clock, and holds five real replacement password consumers
before verification. Each consumer uses a normal native SQLite one-connection
pool on the same WAL database. A native authorizer pauses credential-read
preparation after reservation, allowing other pools to commit admissions. It
does not alter SQL, verification results or production code. Approved-device
controls skip the earlier identity lookup and pause only after admission.

After the old verdict, every replacement row must remain byte-for-byte equal
(counter, block, timestamp and generation). A sixth replacement request must
be refused before verification. Resuming the five admitted consumers must
record exactly five replacement invalid-password events. Counts exclude the
separately admitted old failure and include verified guesses returning a
rate-limited status. Thus status counts cannot conceal an extra verifier call.

Baseline replay restores only the two production files
`internal/app/auth_throttle.go` and `internal/db/auth.go` from `e484955e`, keeps
the additive schema/test harness compatible, runs the command below, and
restores the fix immediately afterward. The regressions fail by assertions,
not compilation. Evidence logs normalize whitespace, terminal color and local
paths only. The [retained output](evidence/throttle-reservation-2026-10-20/baseline.txt)
shows six verified replacement guesses after late success/release, unexpected
replacement blocks after old failures, and resurrection of missing rows.
The earlier [original component probe](evidence/auth-expiry-2026-10-20/reservation-rollover.txt)
independently reproduces the sixth admitted place against the unchanged baseline.

```sh
./scripts/go-backend.sh test ./internal/app \
  -run 'Test(AuthExpiryAuditReservationRolloverProbe|Throttle(Rollover|Reset))' \
  -count=1 -timeout=2m
```

This now passes on the repaired tree. The original probe is a permanent named
test; do not copy its historical source into the current suite.

## Scenario evidence

| Named test / consumer | Expected result | Baseline | Repaired |
|---|---|---|---|
| `TestAuthExpiryAuditReservationRolloverProbe` | Old release cannot admit sixth replacement place | Admitted sixth | Refused |
| `TestThrottleRolloverRealConsumersPreserveReplacementBudget/login` | Late password failure/success leaves username and IP replacement unchanged; five replacement verifications | Old failure blocks replacement; old success admits sixth verified guess | Unchanged replacement; five verified guesses |
| Same test, `/device` | Same rule for the private approved-device scope | Old failure blocks replacement; old success admits sixth | Unchanged replacement; five verified guesses |
| Same test, `/settings` | Late enrollment-password failure/success cannot change replacement session budget | Blocks/clears replacement; sixth verified guess after success | Unchanged replacement; five verified guesses |
| Same test, `/activation` | Late activation-code failure/success cannot change replacement budget shared with settings passwords | Blocks/clears replacement; sixth verified password after activation | Invalid code/activation success as appropriate; replacement unchanged, five guesses |
| Same test, `/mfa_password_release` | Password correct, MFA still owed: release cannot refund replacement username/IP | Six replacement password verifications | Challenge only, replacement unchanged, five guesses |
| Same test, `/internal_error_release` | Malformed stored password hash yields internal error without refunding replacement | Six replacement password verifications | Error retained, replacement unchanged, five guesses |
| Same test, `/mfa_code` | Late wrong factor or valid factor whose challenge expired cannot mutate replacement | Wrong factor blocks replacement; expired challenge release refunds replacement | Correct rejection, no session, replacement unchanged, five guesses |
| `TestThrottleResetRealConsumersDoNotResurrectMissingRows` | Real login/settings failures and successes leave missing rows absent; failures remain observable | Failures recreate rows | Missing stays missing; wrong-password event retained |
| `TestThrottleRolloverMissingRowsAndSuccessReset` | All four scopes fence failure/success/release after deletion and same-time recreation; repeated release is harmless | Missing failures resurrect; replacement affected | All controls pass; current username/device/session success resets preserved |
| `TestThrottleRolloverExpiredBlockAndLateFailure` | Old failure cannot resurrect or extend expired block; next generation gets exactly five places | Rewrites expired blocked row | Deadline preserved, five admissions then refusal |
| `TestThrottleRolloverPartialAdmissionRefundIsFenced` | Later-scope refusal refunds earlier admitted place only | Existing behavior control | Username refunded; full IP budget preserved |
| `TestThrottleGenerationMigrationPreservesBudgetsAndRestarts` | Preserve all four populated counters/deadlines and assign distinct persisted identities | New migration control | Upgrade/restart/down-up pass; identities persist on restart and change on down/up |
| `TestMigrateSeededBaselineMatchesFreshSchema` | Existing seeded financial figures remain exact and head schema matches fresh install | Existing shared gate | Pass |

Existing controls run in the shared backend gate include concurrent password
and settings guessing, username/IP/approved-device/session isolation, MFA
single-use and final-disable cutoffs, failed device/session insertion, failed
operational event logging and atomic management-event rollback, PAT creation,
and native backup/restore. Failure settlement still takes SQLite write
admission before sampling its clock; conditional refunds/resets and login
session/device settlement remain atomic SQL writes/transactions.

## Validation and disposition

All local gates passed with the implementation/test inputs delivered here:

- `./scripts/go-backend.sh test -race -p 2 ./internal/app ./internal/db -run 'Test(AuthExpiryAuditReservationRolloverProbe|Throttle(Rollover|Reset|Generation))' -count=1`: app 8.334s, DB 2.352s. [Focused output](evidence/throttle-reservation-2026-10-20/focused.txt).
- `./scripts/test-backend.sh`: selected-toolchain formatting, vet and complete native SQLite race suite, two packages/five-minute package deadlines. App **226.416s**, API **119.478s**, DB **154.897s**, command **75.033s**; every package passes. Includes all named consumer/migration controls and the seeded-baseline preservation/fresh-schema comparison. [Full output](evidence/throttle-reservation-2026-10-20/backend.txt).
- `pnpm build`: production frontend, static embed check and single binary pass. [Output](evidence/throttle-reservation-2026-10-20/build.txt).
- `E2E_SKIP_BUILD=1 E2E_WORKERS=2 pnpm --dir e2e exec playwright test playwright/auth.spec.ts playwright/mfa.spec.ts playwright/personal-access-tokens.spec.ts`: **16 passed (21.6s)** against the rebuilt binary, including phone light/dark token management. [Output](evidence/throttle-reservation-2026-10-20/browser.txt).
- `./scripts/check-post-merge.sh`, `git diff --check` and report-link checks pass. [Structural output](evidence/throttle-reservation-2026-10-20/structural.txt).

The focused output precedes the final baseline-evidence assertion adjustment
(nonfatal snapshot/sixth-admission assertions allow the baseline to reach its
verified-guess count). The complete backend gate ran the final test sources.
Remote push CI is pending at publication; closing comments record its run/state.

The confirmed P2 #366 mechanism is repaired. The additional focused consumer
audit requested by #366 is complete. No separate application defect or further
independent audit was identified in this scope. #260 and #360 receive the final
commit/evidence links. Actual owner deployment #336 and existing release holds
retain their own prerequisites.

Limits: this is deterministic service/native-SQLite evidence using synthetic
credentials, logical time and stalled consumers. It is stronger than the
original component-only reproduction, but does not claim an observed deployed
HTTP guessing burst or a universal rolling bound on completion timestamps.
The Go race detector does not instrument SQLite's C engine. No live provider,
operator deployment, full browser smoke or fresh whole-app audit is claimed.
