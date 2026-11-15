# Bug classes — Auth, sessions and secrets

Part of the `validate-and-ship` review checklist. Sign-in, MFA, tokens,
throttles, secret handling. Every entry shipped as a real bug in this repo.
Numbers are stable IDs cited from code comments and docs; they are not an
order and are never reused.

## 3. TOCTOU guard races

Check + insert in separate statements/transactions. Expiry clocks sampled
before pool/SQLite writer waits also go stale (#359): acquire write admission
before sampling final token/session guards. Keep
`TestPersonalAccessTokenAuditExpiryWhileWaitingForDatabase`,
`TestPersonalAccessTokenAuditExpiryWhileWaitingForSQLiteWriter` and
`TestPersonalAccessTokenAuditManagementSessionExpiresWhileQueued` green.
Session/device reads check their returned deadline with a fresh clock, without
taking a writer lock (#362). MFA consumption and throttle decisions acquire
SQLite write admission before sampling time (#363/#364); the final MFA session
insert also pins the consumed challenge against disable (#365). Keep
`TestAuthExpiryAudit*` green. Reservations pin persisted generation identity
(#366): failure, refund and success reset must never affect a
replacement/missing generation, including same-timestamp recreation. Keep
`TestAuthExpiryAuditReservationRolloverProbe`, `TestThrottleRollover*`,
`TestThrottleReset*` and the migration-preservation control green. Count
verified guesses through real consumers, not just status codes; an older
generation may finish after rollover.

## 47. Security state checked before a later independent write

Enrollment used to overwrite an activation and activation could accept a code
for a replaced secret (#261). Pin the verified pending secret at the write and
refuse demoting an active enrollment. Exercise deterministic interleavings
through real services, not just a happy-path TOTP vector. Browser cookie
lifetimes must match persisted session/device policy too (#263); test both
password and completed-MFA HTTP consumers and existing-device renewal.

## 48. A budget checked before a verifier and recorded after it

The sign-in and reauthentication throttles read the counter, ran Argon2, then
wrote count+1: overlapping failures were lost and 16 parallel guesses all got
verified against a budget of 5 (#285). Every verifier entry point reserves a
place atomically first (`reserveAttempt`) and defers `release`. Count
verifications from the auth log in a parallel burst, not the response codes (a
`429` can follow a checked guess). Partial identity writes belong in one
transaction (#287), and a maintenance command that revokes sessions must hold
the server lock (#286). Named regressions:
`TestConcurrentPasswordGuessesNeverExceedTheBudget`,
`TestConcurrentReauthenticationGuessesNeverExceedTheBudget`,
`TestFailedDeviceApprovalLeavesNoOrphanSession`,
`TestRecoverOwnerCommandRefusesWhileTheServerHoldsTheDatabase`.

## 51. Transport errors copying operator secrets into history

Go HTTP errors render the full request URL, including Open Exchange Rates'
app_id (#316). Redact at the HTTP boundary before persisting/logging; preserve
classification through unwrapping rather than interpolating the cause.
Exercise the actual FX refresh history consumer
(`TestFXRefreshTransportFailureDoesNotPersistOperatorSecret`).

## 62. Previous-step TOTP fixtures crossing a real clock boundary

Preserving the current code for a later login means activation's previous-step
code can become two steps old while queued (#376). Refresh only after the
expected invalid-code refusal and an observed advancing step, with a bounded
retry. Never relax production skew or mask other failures. Keep the real
stale-clock-sample control
`TestMFAEnrollmentFixtureRefreshesExpiredPreviousStep` green; it uses no
sleeps and retains the production auth clock. Browser enrollment uses
`support/mfa-enrollment.ts` (#384): use the current step when later sign-in
uses a recovery code; reserve the current step only for a subsequent TOTP
login and record the step actually consumed. Keep
`mfa-enrollment-fixture.spec.ts` boundary, no-advance, unrelated-refusal and
attempt-limit controls green through the real enrollment consumer.
