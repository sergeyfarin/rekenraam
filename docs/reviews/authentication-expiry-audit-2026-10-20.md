# Authentication expiry admission audit — 2026-10-20

**Scope:** [#360](https://github.com/sergeyfarin/rekenraam/issues/360), under
[#260](https://github.com/sergeyfarin/rekenraam/issues/260). Baseline:
`31454a985d7836b59d9609f72de49561f18bec70`, clean `main` before the audit.
This follows [#267's identity audit](identity-concurrency-audit-2026-10-17.md)
and [#357's PAT audit](personal-access-token-audit-2026-10-20.md).

The audit reproduced four bounded defects, repaired together in [e7628dbd](https://github.com/sergeyfarin/rekenraam/commit/e7628dbdb0fdc61a1d1ee3f3ba8cfb9350959b93):
[#362](https://github.com/sergeyfarin/rekenraam/issues/362),
[#363](https://github.com/sergeyfarin/rekenraam/issues/363),
[#364](https://github.com/sergeyfarin/rekenraam/issues/364) and
[#365](https://github.com/sergeyfarin/rekenraam/issues/365).
**At completion of this audit, an additional confirmed reservation-lifecycle
gap remained open as [#366](https://github.com/sergeyfarin/rekenraam/issues/366).** Completing this
bounded audit does not certify that every authentication concurrency concern
is repaired. #366 owns the generation/settlement repair and its additional
consumer audit, now delivered in the follow-up below. Owner deployment remains #336.

**Follow-up, #366:** the [reservation repair and consumer audit](throttle-reservation-audit-2026-10-20.md)
supersedes the historical open status above and below. Migration 0020 persists
generation identity and all settlements are fenced. The original probe now
passes as a permanent regression; the retained failing output remains baseline
evidence, not current behavior.

## Independent expectations and admission inventory

Expectations come from the persisted absolute deadlines, TOTP's 30-second
period and one-step skew, the five-place/fifteen-minute budget, and the stated
session/revocation rules. They are not computed by the admission SQL under
test. The synthetic start is `2026-10-20T12:00:00Z`; deadline equality refuses
an expired credential and admits a scope whose block has ended.

| Deadline or transition | Admission boundary and in-flight rule |
|---|---|
| Browser session absolute expiry; default 30 days, configurable sign-in lifetime | Read the unrevoked session and owner, then compare the returned deadline with a fresh clock. No writer lock. A request admitted before expiry/logout may finish; the next request refuses. Settings inherit this admission rule; PAT management retains its stronger final-write guard from #359 |
| Trusted device absolute/sliding expiry; 180 days | Read device and owner binding, then check expiry before selecting private throttle scopes. A device admitted before expiry/revocation retains its selected scopes while the attempt finishes. Successful reuse renews the same identity; a revoked row is never renewed |
| MFA challenge expiry; five minutes | Preliminary fresh check after lookup, then final exclusive expiry check after SQLite write admission when consuming the challenge. Consumption is single-use. A challenge consumed before expiry may finish afterwards, subject to the revocation guard below |
| TOTP login and activation | Validate against a fresh clock after reading enrollment. Recheck that the matched step remains within the one-step skew window after acquiring SQLite write admission. For a code from 12:00:00, 12:00:59 is admitted and 12:01:00 refuses. Login replay-counter update also pins the verified secret |
| Pending TOTP enrollment | No independent expiry in the current contract; pending is not an active sign-in factor. Activation pins the exact pending secret, and the activation plus recovery set and security event are atomic |
| Recovery codes | No time deadline; single-use conditional update. Regeneration invalidates the old unspent set. A factor consumed before regeneration remains proved. Two concurrent distinct codes for one challenge may both be spent, but only one session is issued (retained #267 limit) |
| Username, IP, device and reauthentication throttle blocks | Reserve before every verifier. After acquiring pool and SQLite writer admission, read the row and sample the decision clock. `blocked_until == now` admits; failure starts its block from a fresh settlement clock |
| Unsettled throttle places | Five held places expire fifteen minutes after the last admission. Exact rollover admits a replacement window. Original audit: old settlements were **not generation-fenced**; subsequently repaired in #366 (follow-up above) |
| MFA disable / final session insertion | Session insertion atomically checks that the exact consumed challenge still exists with an active enrollment. Disable/recovery deleting it before insertion prevents a new session, device or success settlement, even after factor/challenge consumption (#365) |
| Owner password reset / recovery | Production `recover-owner` must own the server lock; it cannot overlap a running server. Recovery atomically changes the password, removes MFA/challenges and revokes sessions/PATs. Direct service overlap controls do not establish that the CLI lock can be bypassed |
| Cleanup and display timestamps | Daily session/device/challenge cleanup is retention, not admission. Device lists and event counts are informational snapshots. Logout's lookup is attribution; its unconditional revocation grants no access. Session/device/challenge creation and lifecycle-event timestamps remain issuance/recording timestamps, not a fresh admission guard |

The durable rules are recorded in `docs/conventions.md`. No endpoint, scope,
response shape, cookie policy, schema or financial rule changes in this slice.

## Method and evidence

All credentials, owners and databases are synthetic. Tests use real services,
repositories, native SQLite and HTTP consumers. No production scheduling or
authorization hook was added.

- A held `database.Conn` and a changed pool `WaitCount` establish that a
  consumer actually queued. The existing injected service clock advances to
  the exact deadline before releasing that connection.
- An independent `db.Open` on the same WAL file holds a real SQLite write
  transaction. A native authorizer observes compilation of the selected
  update/insert; it never changes SQL or its verdict. The clock advances
  before releasing writer admission. Deferred `BeginTx` does not suffice.
- The final-session overlap pauses the actual session insert before execution.
  A second real service over the independent pool commits MFA disable, then
  the insert continues. This avoids relying on a numbered clock-call seam.
- HTTP controls use actual session cookies, CSRF and routes. `PRAGMA
  query_only=ON` on the session connection proves reads remain read-only while
  an independent writer has an uncommitted expiry change. After commit,
  account/settings reads and a financial mutation refuse the cookie; account
  counts remain unchanged.
- Triggers force event-write failures. Session and challenge refusal remains
  refusal even when operational logging fails. Existing #288 security-event
  rollback tests and #267 failure/identity controls run in the shared gate.

The new app consumer regressions were replayed with the five production auth
source files restored to baseline `31454a98`, then restored to the fix. They
fail by assertions, not compilation. [Baseline output](evidence/auth-expiry-2026-10-20/baseline.txt) is retained with whitespace/local paths normalized.
No plaintext credential appears in the evidence or failure assertions.

## Scenario results

Names below use the prefix `TestAuthExpiryAudit`.

| Consumer / scenario | Independent expected result | Baseline actual | Fixed actual / evidence |
|---|---|---|---|
| `SessionPoolAdmission` | Session released exactly at expiry is unauthenticated | Authenticated, P1 #362 | Refused; same session immediately before deadline is authenticated |
| `TrustedDevicePoolAdmission` | Expired device falls back to the blocked shared username budget | New session despite shared block, P1 #362 | Rate limited; no extra session or renewed bypass |
| `MFAChallengeAdmission/pool` and `/writer` | Expired challenge cannot issue a session/device | Both issued sessions, P1 #363 | Invalid challenge; no session/device/success event |
| `TOTPWriterAdmission/login` and `/activate` | Exact skew-window end refuses the proved step | Both succeeded, P1 #363 | Invalid code; no session; activation stays pending |
| `MFAPositiveWriterControls/challenge` and `/totp` | Writer serialization still admits proofs within their deadlines | Pass | Pass, including TOTP one second before cutoff |
| `ThrottlePoolBoundary/login` and `/settings` | Block ending exactly at pool admission no longer refuses the owner | Rate limited, P2 #364 | Login/enrollment succeeds |
| `ThrottleWriterAdmission` | Reservation/failure follows actual writer admission time | Read-to-write upgrade fails `database is locked` | Invalid password as expected; throttle timestamp is fresh, #364 |
| `DisableBeforeSessionCommit` | Disable committing after challenge consumption cuts off the uncommitted session | New session/device, P2 #365 | Invalid challenge; no session/device |
| `ExpiredChallengeWithEventFailure` | Failed logging cannot turn expiry refusal into admission | Login succeeds even with event failure | Invalid challenge; no session/device |
| `ConcurrentChallengeIsSingleUse` | Two distinct recovery-code submissions issue at most one session | Pass | Exactly one session, device and success event |
| `TrustedDeviceAdmittedBeforeExpiry` | Device admitted before expiry may retain its private scope and finish | Pass | Pass; same device identity renewed |
| `SessionHTTPReadBoundary` | WAL writer does not block or require writes for session reads; committed expiry refuses account/settings/mutation consumers | Control added for final implementation | Pass; financial account count unchanged |
| `SessionHTTPInFlightCutoff/logout`, `/expiry`, `/recovery` | An admitted HTTP consumer finishes; next settings request refuses | Control added for final implementation | Pass, 200 for held consumer and 401 for next request |

Prior named controls retained in the shared backend gate include
`TestMFAEnrollmentCannotOverwriteAnActivation`,
`TestMFAActivationMustProveCurrentPendingSecret`,
`TestMFAChangesCutOffInFlightChallengesAndCodes`,
`TestFailedSessionInsertAfterMFASpendsTheChallenge`,
`TestFailedDeviceApprovalLeavesNoOrphanSession`,
`TestMFAManagementFailuresLeaveThePreviousStateIntact`,
`TestSecurityChangeIsRolledBackWhenItsEventCannotBeWritten`, the parallel
sign-in/reauthentication budgets, the recovery command lock tests, and all
`TestPersonalAccessTokenAudit*` controls from #357.

## Original finding: reservation window identity (#366)

The direct reservation-component probe independently reserves five old places,
advances exactly fifteen minutes, reserves all five replacement places, then
releases one old reservation. A sixth replacement reservation succeeds;
expected `ErrRateLimited`, actual nil. This reproduces after the #364 clock fix.
The refund identifies only the scope row, not the reserved window/place.

This is P2 and needs a reservation-lifecycle design and additive migration if
identity is persisted; it is larger than moving the audited clock checks.
#366 owns that repair plus real login/settings overlaps involving late
release, failure and success. This probe does **not** claim a demonstrated
HTTP password-guess burst: an attempt spanning a whole fifteen-minute window
is required, and it exercises the reservation component directly.

The [probe source](evidence/auth-expiry-2026-10-20/reservation-rollover_test.go.txt)
and [expected-failure output](evidence/auth-expiry-2026-10-20/reservation-rollover.txt)
are retained outside the passing suite so CI does not assert incorrect behavior
or silently skip a regression. The original reproduction at `e484955e` is now retained permanently as
`TestAuthExpiryAuditReservationRolloverProbe`. On the repaired tree run:

```sh
./scripts/go-backend.sh test ./internal/app -run '^TestAuthExpiryAuditReservationRolloverProbe$' -count=1
```

The command now passes. The archived source/output describe the earlier
expected failure; do not copy the archived test into the current suite, where
its name already exists. The #366 report adds failing-before real-consumer
and passing-after generation, missing-row and migration controls.

## Validation and limits

All checks below passed with the Go production/test inputs in the fix commit:

- `./scripts/go-backend.sh test ./internal/app ./internal/api -run '^TestAuthExpiryAudit' -count=1 -timeout=60s`: app 1.398s, API 0.796s. Preliminary focused DB activation/event controls and prior MFA/PAT regressions also passed.
- `./scripts/test-backend.sh`: selected-toolchain gofmt and vet, then the complete native SQLite race suite with two packages and five-minute package deadlines. App 218.696s, API 119.507s, DB 122.029s, command 75.142s; every package passes. [Output](evidence/auth-expiry-2026-10-20/backend.txt).
- `pnpm build`: frontend/static embed/integrated binary succeeds.
- `E2E_SKIP_BUILD=1 E2E_WORKERS=2 pnpm --dir e2e exec playwright test playwright/auth.spec.ts playwright/mfa.spec.ts`: all five rebuilt sign-in/MFA browser journeys pass (6.1s). [Output](evidence/auth-expiry-2026-10-20/browser.txt).
- `./scripts/check-post-merge.sh`: clean. `git diff --check`: clean.
- The retained #366 source artifact was copied into `internal/app` and rerun against final fixed production inputs: expected failure, actual sixth reservation admitted (0.422s); the temporary test was removed. [Final probe output](evidence/auth-expiry-2026-10-20/reservation-rollover.txt).

Push CI is pending when this report is published. Local shared backend validation
is complete; the issue closing comments record the pushed SHA and CI run.

The clock changes are deterministic logical time, not wall-clock sleeps or
production timeout bypasses. SQLite's bundled C engine is not instrumented by
Go's race detector. No live provider, owner deployment, proxy or TLS claim is
made. The earlier #357 production HTTPS/restore controls remain separate; this
slice changes neither PAT grants nor that boundary. Full browser smoke,
coverage mode and provider/owner operational scenarios were not rerun.

Maintainability: read admission and consuming-write admission are deliberately
different. Repository callbacks run under SQLite writer ownership and must
not perform database I/O; service code owns TOTP policy. The final MFA guard
must remain in the same statement that inserts its session. Reservation scope
identity was the concrete lifecycle hotspot, subsequently repaired/audited by #366; no unrelated
refactor is part of this audit.
