# Identity concurrency and failure-atomicity audit — 2026-10-17

**Status:** targeted identity sub-audit
[#267](https://github.com/sergeyfarin/rekenraam/issues/267) of
[#260](https://github.com/sergeyfarin/rekenraam/issues/260), completed with
findings. Three confirmed defects are fixed in this change:
[#285](https://github.com/sergeyfarin/rekenraam/issues/285) (P1),
[#286](https://github.com/sergeyfarin/rekenraam/issues/286) (P2) and
[#287](https://github.com/sergeyfarin/rekenraam/issues/287) (P3). The
attribution gap is filed as
[#288](https://github.com/sergeyfarin/rekenraam/issues/288) (P3) and is still
open. This report follows the
[setup/auth/privacy audit](setup-auth-privacy-audit-2026-10-17.md), which fixed
two specific MFA interleavings (#261). It does not cover the #262 attempt-budget
policy or a real proxy deployment (#268).

## Scope and method

The audit started from `37b1c2ba` with a clean working tree. It reviewed the
identity mutation paths: `Login`, `CompleteLoginMFA` and `completeLogin`; the
throttle helpers; MFA enrollment, activation, disable, regeneration and
reauthentication (`backend/internal/app/auth.go` and `auth_mfa.go`); the
repositories (`backend/internal/db/auth.go`, `auth_mfa.go` and `recovery.go`);
and the `recover-owner` command (`backend/cmd/rekenraam/command.go`).

Evidence came from the real service, repository and command, run on isolated
temporary SQLite files with a synthetic owner. No production scheduling hooks
were added. The audit used three techniques:

- **Interleaving:** the service's existing injected clock runs a second real
  service call at the first clock read inside the first call, the same seam
  #261 used.
- **Parallel bursts:** 16 goroutines start together. The tests do not count
  response codes. They count the `login_failed/invalid_credentials` rows that
  record one password verification each. A `429` can still follow a guess
  that was already checked.
- **Failure injection:** a test-only `TEMP TRIGGER` on the connection raises
  `RAISE(ABORT)` before a chosen insert or delete. A row-level trigger only
  fires when a row exists, so the tests set up the row first.

Expected results were derived independently from the stated contract:
5 failures per 15 minutes per scope (`docs/conventions.md`), "a rejected
request leaves no session, spendable factor or success record" (#267), and
"recovery revokes every session" (`docs/deployment-security.md`). They were not
computed by the code under test. Every regression was run at `37b1c2ba` first
and failed there by assertion, not by compilation. The reservation unit test
is the exception: it exercises the new API and cannot run on the old code.

## Findings and disposition

| Finding | Severity and evidence | Disposition |
|---|---|---|
| Throttles admit concurrent guesses beyond the budget: a lost update on the counter, and a check-then-record split around Argon2 | P1, #285. Two overlapping failures on the username scope counted as one, and the correct password then signed in after five failures. 16 parallel wrong-password sign-ins verified **16/16** guesses (budget 5). 16 parallel wrong `DisableMFA` confirmations from one session also verified **16/16**. Go's race detector cannot see either: they are logical SQL races | Fixed. Each attempt atomically reserves a place before any verifier runs, and places still in flight count against the budget. A failure keeps its place. A success or an attempt with no verdict gives its place back. A place a crashed process never settled expires with the window. This applies to username, IP, approved-device and `reauth_session` scopes |
| `recover-owner` runs beside a live server | P2, #286. With the server's `<database>.lock` held, the command exited 0 and replaced the password. A request that verified the old password before the reset transaction can insert its session after it, so that session survives recovery. A rehash write can also put the old password's new hash over the recovery password, and migrations run under the live server | Fixed. The command holds the lock for the whole recovery and refuses with the `restore` wording. The docs and the recovery panel say to stop the server first |
| A sign-in that fails after its session insert leaves an orphan session | P3, #287. With device approval forced to fail, `Login` returned an error but left one live `auth_sessions` row. The token was never returned, so the row could not be used | Fixed. The session, the throttle settlement and the device approval are written in one transaction. The event row stays outside it, by design |
| MFA management, recovery-code sign-in and owner recovery leave no attributed record | P3 enhancement, #288. `authentication_events` covers sign-in attempts and logout only. Disabling MFA from a session that has the password is invisible. A recovery-code sign-in looks the same as a TOTP sign-in | Open, filed as a bounded issue. This audit does not choose the schema (`authentication_events` against `audit_events`) |

## Overlap and failure-point matrix

| Scenario | Result | Evidence |
|---|---|---|
| Concurrent password failures against username/IP counters (lost update) | Confirmed defect #285, fixed | `TestOverlappingPasswordFailuresAreAllCounted` |
| Burst admission, sign-in | Confirmed defect #285, fixed | `TestConcurrentPasswordGuessesNeverExceedTheBudget` |
| Burst admission, MFA-settings reauthentication (#262 budget) | Confirmed defect #285, fixed | `TestConcurrentReauthenticationGuessesNeverExceedTheBudget` |
| In-flight places hold the budget; released place reusable; unsettled place ages out | Verified (new mechanism) | `TestReservedAttemptsHoldTheBudgetUntilSettled` |
| Wrong MFA codes at sign-in share the same admission | Verified by code path: `CompleteLoginMFA` reserves before `verifySecondFactor`; existing `TestCompleteLoginMFAThrottlesWrongCodes` passes | No separate parallel test |
| Enrollment overwriting activation; activation of a replaced secret | Verified (fixed earlier in #261) | `TestMFAEnrollmentCannotOverwriteAnActivation`, `TestMFAActivationMustProveCurrentPendingSecret` |
| Disable against an in-flight sign-in that has proved its password | Verified: disable deletes outstanding challenges in its transaction, so a later completion is `ErrMFAChallengeInvalid` | `TestMFAChangesCutOffInFlightChallengesAndCodes` |
| Regeneration against an in-flight recovery-code sign-in | Verified: the old set is dead once regeneration commits. If a code was consumed before the commit, that sign-in was valid when it was verified | same test; `ConsumeMFARecoveryCode` is one conditional `UPDATE` |
| Activation against an in-flight password-only sign-in | Accepted limit: a sign-in that read the inactive state just before activation may finish. Activation does not revoke existing sessions anyway (recorded in the first audit) | Code inspection: `Login` reads `mfaActive` once |
| Password rehash during sign-in | Verified after #286: the only other in-process writer of `password_hash` is recovery, which can no longer run beside the server. Two concurrent rehashes both store a valid hash of the same password | `grep` of `password_hash` writers; recovery lock test |
| `recover-owner` against a running server and in-flight sign-ins | Confirmed defect #286, fixed | `TestRecoverOwnerCommandRefusesWhileTheServerHoldsTheDatabase` |
| Failure at device approval, after the session insert | Confirmed defect #287, fixed | `TestFailedDeviceApprovalLeavesNoOrphanSession` |
| Failure at session insert, after factor and challenge consumption | Verified: no session or success record. Neither the challenge nor the spent TOTP step can be replayed | `TestFailedSessionInsertAfterMFASpendsTheChallenge` |
| Failure at auth-event recording | Verified documented choice: the sign-in still succeeds and the error is logged, so a logging fault never becomes a lockout | `TestFailedEventWriteDoesNotTurnIntoALockout` |
| Failure part-way through disable, regeneration or recovery | Verified: each is one transaction, and the previous enrollment, codes, password, sessions and challenges stay intact | `TestMFAManagementFailuresLeaveThePreviousStateIntact` |
| Failure at activation code insertion | Verified (existing) | the first audit's atomic activation rollback test |
| Failure between factor consumption and challenge consumption | Not injected. By inspection, the factor is spent and the challenge stays live, so a retry needs a new code within the same budget. Nothing spendable is left | `verifySecondFactor` then `ConsumeMFAChallenge` |
| Two submissions of one challenge with two different recovery codes | Accepted limit: both codes are consumed and only one submission gets the challenge, so the owner loses one recovery code to a race they caused. It fails closed and no session is duplicated | Code inspection; not worth the cost of a combined factor and challenge transaction |
| Device revocation against an in-flight sign-in using that device | Verified: the device extension is `WHERE revoked_at IS NULL` and does not bring a revoked device back. The attempt keeps the device budget it was admitted with | `CompleteLogin` SQL |

## Revocation-cutoff rule

The rule is recorded in `docs/conventions.md` (Authentication And Session
Security): a revoking change cuts off every attempt that has not yet committed
its session.

- **Owner recovery.** It runs only while the server is stopped, so no attempt
  is in flight (#286).
- **MFA disable and recovery-code regeneration.** They take effect inside their
  own transactions. An in-flight sign-in that has already proved its password
  still fails on the second factor.
- **Logout and device revocation.** A request already authenticated by the
  session may finish, and the next request is refused.
- **MFA activation.** It does not revoke sessions. A password-only sign-in
  that read the inactive state just before activation may finish.

This changes durable behavior in one place: `recover-owner` now refuses while
the server runs. The deployment and developer docs and the recovery panel copy
say so.

## Behavior notes for the fix

- While all five places in a scope are held by attempts still being verified, a
  sixth attempt gets `429` even with the correct password. `Retry-After` gives
  the latest time the held places can last. In practice they settle within
  seconds, and normal use never has five sign-ins in flight at once.
- When MFA is still owed, a correct password now gives its place back. Before,
  the attempt left the counter untouched. The two have the same effect.
- `Retry-After` is now taken from the service clock instead of `time.Until`.
  Production uses the same wall clock.

## Maintainability notes

These notes record what the audit observed. Nothing here was refactored, and
#284 ranks them.

- **Every verifier entry point must reserve a throttle place and defer its
  release.** There are four: `Login`, `CompleteLoginMFA`,
  `verifyOwnerPassword` and `ActivateTOTP` (`backend/internal/app/auth.go`
  and `auth_mfa.go`). A new verifier that skips this step gets back the burst
  bypass from #285. The old check-then-record pair also existed in two
  diverging copies, one for sign-in and one for reauthentication, and both had
  the defect.
- **There are two session writers.** One is `completeLogin` through
  `db.AuthRepository.CompleteLogin`. The other is owner setup in
  `backend/internal/db/setup.go`, which inserts `auth_sessions` itself. A
  session-wide rule, such as the #287 atomicity or a future revocation, has to
  be enforced in both places.
- **Auth-event parameters are assembled by hand at 11 call sites** across
  `auth.go` and `auth_mfa.go`. The username, user, IP and request fields are
  repeated at each one. #288 will add more call sites.

## Validation

- Every regression named above failed at `37b1c2ba` and passes after the
  fixes. On the baseline: 16/16 guesses verified for both sign-in and
  reauthentication, a lost update allowed a sixth sign-in, an orphan session
  was left, and recovery succeeded under a held lock.
- Focused suites with `-race` passed through `scripts/go-backend.sh`:
  `./internal/app/` (identity, MFA, reauthentication, throttle, setup,
  recovery and session tests), and the full `./internal/db/`,
  `./internal/api/` and `./cmd/rekenraam/` packages.
- The browser specs were not rerun. HTTP status codes and shapes are
  unchanged, and the only frontend change is catalog copy. The frontend
  catalog gate was run for that copy change.
- Not verified: a real proxy or multi-process deployment (#268), and
  native-language review of the new recovery-panel sentence.


## Follow-up expiry and final-session audit — 2026-10-20

[#360](https://github.com/sergeyfarin/rekenraam/issues/360) completed the
[adjacent expiry-admission audit](authentication-expiry-audit-2026-10-20.md).
Actual queued service consumers confirmed session/device expiry (#362), MFA
challenge/TOTP writer deadlines (#363), stale throttle admission clocks (#364),
and MFA disable after challenge consumption but before session insertion (#365).
These bounded defects are fixed. The prior audit's earlier-stage controls did
not prove those boundaries. Pure session reads remain read-only. A separate
reservation-window settlement defect is confirmed and remains open as
[#366](https://github.com/sergeyfarin/rekenraam/issues/366), including its
additional login/settings concurrency audit. The dated report separates that
component-level probe from demonstrated HTTP exposure.
