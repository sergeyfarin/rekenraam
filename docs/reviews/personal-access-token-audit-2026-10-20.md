# Personal-access-token boundary audit — 2026-10-20

**Status: complete.** #357's bounded engineering audit passed. Confirmed P2
findings #359 and #361 are fixed; additional expiry-clock audit #360 is open for adjacent
browser-session/MFA/trusted-device paths. Actual owner deployment and household
acceptance remain #336; this is not release sign-off.

Baseline: `a14901585cd353f514cf9dc54a85f8f1dd0ecc07`.
Fix: [`89951c9adfd1c18dc6b0a8ddf3ea5e6ba3551a9b`](https://github.com/sergeyfarin/rekenraam/commit/89951c9adfd1c18dc6b0a8ddf3ea5e6ba3551a9b).
Final tested source and fixture:
[`c709b913142067d1ee60ec0414470748bf592d7b`](https://github.com/sergeyfarin/rekenraam/commit/c709b913142067d1ee60ec0414470748bf592d7b),
clean checkout. The evidence commit adds documentation only.

## Independent oracle and scope

The [ADR 0018](../adrs/0018-personal-access-token-boundary.md) and
[contract](../plans/personal-access-tokens-plan.md) supply a literal expected
grant set: 19 ledger GET patterns, six report GET patterns and their HEAD
aliases. The external probe does **not** import the runtime allowlist. It
enumerates all 333 static `mux.HandleFunc` registrations, asserts complete
enumeration and challenges each method/path with ledger-only, reports-only and
combined credentials. Every ungranted pair is also retried with a valid owner
cookie plus valid CSRF/Origin. Non-HEAD denials must carry the token-specific
refusal, so a browser or business-rule 403 cannot mask fallback. Allowed calls
return 200 or a genuine missing-resource 404; required
report query parameters are supplied. Refusals must be 403, never a cookie
fallback. HEAD responses have no body over real HTTP.

The complete [route matrix](evidence/pat-audit-2026-10-20/routes.csv) records
2,462 calls (1,281 direct token probes and 1,181 cookie-fallback probes). Other
exact cases and redacted CLI results are in
[summary.json](evidence/pat-audit-2026-10-20/summary.json). Credentials are
synthetic; the financial sentinel is an independently specified posted
-2500/+2500 pair at scale 2, checked through the transaction API after denied
mutations, each native restore and owner recovery. No provider is involved.

Source review also checked the wrapper's position before ServeMux dispatch,
exact registered-pattern authorization, the absence of token principals in
management paths, password/CSRF/Origin guards, 256-bit credential generation,
domain-separated hash persistence, event/last-use atomicity, UI disclosure
lifecycle and native-versus-portable export contents. Existing #334 consumer
and OpenAPI parity tests remain supporting evidence, rather than the oracle.

## Expected and observed boundaries

| Scenario | Expected | Actual |
| --- | --- | --- |
| Every registered method/path, all three scope combinations, plus GET/HEAD | Exactly the two declared read grants; valid owner cookies cannot widen them | Pass, all 333 registrations; no forbidden handler admitted |
| Encoded IDs and names | Encoded granted segments still resolve correctly; encoded slash cannot create a new grant | `/api/v1/%61ccounts` and `/accounts/%31` 200; `/accounts%2f1` 403 |
| Dot/duplicate slashes, encoded management paths, extra segments, semicolons, NUL, case changes, unknown and frontend routes | Deny management/unknown paths; safe canonical redirects remain on the exact allowed read | Pass for GET/HEAD; two canonical reads 307 to `/api/v1/accounts`; denied paths 403, invalid numeric ID 400, NUL 400/403 |
| Empty, malformed, wrong-scheme and duplicate/mixed-case Authorization, with valid cookies | 401 through both proxy and direct app; no fallback | Pass; case-insensitive Bearer with tab separator remains valid (200) |
| PAT in a cookie/JSON or invalid URL credential | No implicit authentication | All 401; no usable credential was placed in a URL |
| Unknown mutation methods and all registered financial writes | 403 with no journal changes | Pass; sentinel stays posted with exact signed coefficients |
| Pool wait crosses exact expiry | Final admission refuses; no successful event | Original implementation failed; fixed test passes |
| SQLite writer wait crosses expiry after deferred BeginTx | Final guard must sample after acquiring writer admission | Pool-only candidate still failed; final fix passes |
| Token creation/revoke queued beyond management-session expiry | No new credential/plaintext or revocation | Both original siblings failed; final fix passes |
| Password/session changed after successful password verification | Final creation guard refuses | Pass, no token rows or plaintext |
| Real revoke commits between token lookup and use | The queued use must not pass its final guard | Pass; no successful admission event |
| Revoke or recovery after authentication commits, before consumer completes | Admitted consumer may finish; next authentication fails | Pass through actual HTTP middleware; one dispatched consumer, later 401 |
| Security-event insertion fails on invalid/scope-denied requests | Fail closed, even with owner cookies; last-used unchanged | 500, no dispatch; ordinary refusal returns after trigger removal |
| Lifecycle/use/recovery event failures | Entire associated state change rolls back | Existing creation/revoke/use/recovery regressions pass |
| Trusted proxy versus direct untrusted peer with forged forwarding headers | Record the actual observed peer; HSTS only on trusted HTTPS | Both recorded `172.31.68.1`, correlated by server UUID; forged `203.0.113.*` never controls attribution |
| curl same-origin / cross-origin redirect | Retain on same origin; strip on different origin | Both safe flows 200; controlled second origin receives no Authorization |
| Deliberate curl `--location-trusted` control | Controlled recipient detects unsafe forwarding | 418; no credential echoed or persisted |
| Native verified backups before and after revocation | Hash-only snapshots retain their respective credential state | CLI verification succeeds; three known digests, no plaintext in either snapshot |
| Restore snapshot before revocation | Old credential becomes valid again, as documented | 200 on restored instance |
| Restore snapshot after revocation | Revocation remains effective; independent unrevoked report token still works | 401 / 200; PAT does not depend on the missing MFA encryption key |
| Restore an expired unrevoked credential onto a fresh installation | Expiry still refuses, without synthesizing revocation | Named HTTP/native-restore test returns 401; original expiry and NULL revocation retained |
| CLI owner recovery after restore | Revoke all remaining PATs and carried session; permit password recovery and preserve journal | All three old PATs and old session 401; new login 200; one event records two newly revoked PATs; exact journal unchanged |
| Ledger CSV, bundle ZIP and QIF before/after restore | No credential rows, plaintext or known digests | All three formats and 56 payload/member chunks per phase pass |
| App/access logs | Actual Authorization redaction, no plaintext/digest | Redaction observed; in-memory leaky-copy negative control detected |
| Live SQLite/WAL/SHM and native snapshots | No generated plaintext; private file modes | Pass; all live database files 0600 |
| Browser local/session storage, Cache Storage URLs/bodies | No generated token | Pass on 390×844 phone viewport |
| Actual cross-document history restore with disclosure visible | Pagehide scrubs the token before bfcache retains it | `pageshow.persisted=true`; restored disclosure and DOM contain no token |
| Sign-out elsewhere and subsequent visibility refresh | Session UI/disclosure leaves; independent PAT still valid | Pass; anonymous `force-cache` request 401; bearer 200 with `no-store` |

## Finding #359 and disposition

The original service captured the clock before database waits and reused it in
both lookup and final transactional predicates. A held writer-pool connection
plus `WaitCount` and the existing atomic test clock reproduced authentication
at `expires_at`, when it must fail. The same control reproduced session-expiry
admission in both token creation and revocation. A separate SQLite writer and
test-only native authorizer showed that moving the clock merely after
`BeginTx` still failed: SQLite transactions begin deferred and can wait for a
writer after that sample.

The fix first obtains both the connection and SQLite write admission with an
empty UPDATE (no changed rows or row triggers), then samples the clock and
checks lifecycle/session predicates, changes state and records the event in
the same transaction. No grant, financial schema or migration changes.
Revocation still serializes at committed authentication; expiry is sampled at
the final admission guard, not continuously during admitted work. The named
regressions and the recurring-bug checklist retain this distinction.

The failure was P2, a bounded expired-credential admission defect. The audit
did not establish a scope bypass or financial write capability. #359 is fixed
in `89951c9a`. #360 owns the additional audit of adjacent session, MFA and
trusted-device deadline paths, identified by concrete pre-query timestamp
patterns. Those paths need their own consumer evidence; no unproven MFA bypass
is asserted and no broad auth refactor was folded into this fix.

## CI follow-through — finding #361

Reviewing the final push exposed a pre-existing main browser failure in
[CI 39389846174](https://github.com/sergeyfarin/rekenraam/actions/runs/39389846174):
`correct buy: reconciliation confirm` failed the dark-desktop and de-desktop
sweep with a retained axe contrast finding and `no text pixels`. #348's
replacement glyph-only helper injected an inline stylesheet, which the
unchanged production `style-src 'self'` policy blocks. Its isolated controls
had no such policy. The initial evidence push
[CI 39419119037](https://github.com/sergeyfarin/rekenraam/actions/runs/39419119037)
reproduced the same failure in light desktop; the other four CI jobs passed.

Two new real-Chromium controls reproduce the missing measurement before the
fix. With the same production policy, readable `#333` on white must measure 12.63:1
and low-contrast `#999` on white must measure 2.85:1 despite a black border/icon.
The fix uses permitted CSSOM property writes, retains each descendant's actual
fill and restores original property values/priorities in `finally`, including
on detached nodes. It keeps the 4.5:1 threshold, original axe diagnostics and
production CSP. This fixes test instrumentation; it does not weaken policy or
change the application's warning colours or financial behavior.

Fix: [`06c29263`](https://github.com/sergeyfarin/rekenraam/commit/06c29263beacf233e6c30029aeb4216a00df11d7). The fresh single-binary build, all five contrast controls,
all eight real subview configurations and the shared browser smoke suite
(363 cases) pass. The new controls also check style restoration.
[Retained control/matrix evidence](evidence/pat-audit-2026-10-20/csp-followthrough.json)
and the [smoke result](evidence/pat-audit-2026-10-20/browser-smoke.txt) record the
fix source and exact outcomes.
The smoke command is:

```sh
pnpm build
E2E_SKIP_BUILD=1 pnpm --dir e2e exec playwright test rendered-contrast.spec.ts subview-sweep.spec.ts --reporter=list
E2E_SKIP_BUILD=1 E2E_WORKERS=3 ./scripts/test-e2e-smoke.sh
```

#361 is fixed within this task and tracked on #260/#348. #348's original
maximum-pixel false-acceptance defect remains fixed. The independent PAT
fixture at `c709b913` does not use this helper, and all Go/frontend production
inputs are unchanged by the follow-through. Its 2,550 external cases and image
identities remain valid. The final main CI status is recorded on #357.

## Reproduction and validation

```sh
./scripts/go-backend.sh test -race -p 2 -timeout=5m \
  ./internal/app ./internal/db ./internal/api -run '^TestPersonalAccessToken' -count=1
./scripts/test-backend.sh
PAT_AUDIT_OUTPUT=/tmp/pat-audit-evidence deploy/docker/pat-audit/run.sh
./scripts/check-post-merge.sh
```

The final shared backend gate passes formatting, vet and all race cases at
`3a19d65d`; Go production/test inputs are identical at `c709b913` (the latter
strengthens only the external refusal oracle and its README).
`TestPersonalAccessTokenAudit*` cases cover pool/writer expiry, management-session
expiry, verified password/session changes, real revoke between lookup/use,
committed HTTP admission versus revoke/recovery, denial-event failure and
expired native restore. The [filtered negative controls](evidence/pat-audit-2026-10-20/negative-controls.txt)
record the baseline and intermediate pool-only failures; synthetic secrets were redacted from retained failure output.

The fresh Docker build serves the static frontend from one Go binary. The
inherited proxy drill passes eight deployment cases and one case in each of
three restart/key phases; phase-selection skips are deliberate. The separate
PAT browser case passes with an actual cache restore, no retries or skips.
The literal route/client/native-restore matrix passes 2,550 cases.
[Source/image metadata](evidence/pat-audit-2026-10-20/metadata.json),
[browser evidence](evidence/pat-audit-2026-10-20/browser.json), file modes and
final live-file/access-log scans are retained in the
[evidence directory](evidence/pat-audit-2026-10-20/README.md). Raw logs,
keys, cookie jars, databases and native backups were kept private and removed
with the disposable fixture. Final remote CI status is recorded on #357.

## Limits and follow-ups

- #360 is additional engineering work, not an owner prerequisite or a blocker
  to #357's bounded PAT acceptance. #260 tracks it explicitly.
- This uses the documented Caddy container/private-network topology and
  loopback Docker publishing; both observed client addresses are the bridge
  peer. Other proxies, external LAN/NAT paths, browsers and actual owner log
  retention/TLS settings remain #336.
- Headless tab switching needs the established explicit `visibilitychange`
  signal. The **bfcache restore itself is real**, with a retained listener
  proving `pageshow.persisted=true`; no dispatched pagehide/pageshow substitutes
  for that result. Session UI leaves at its existing visibility/60-second-poll
  boundary, not an invented instantaneous broadcast. Already rendered data in
  another hidden tab can remain until that check, as accepted in #268.
- The browser accepts the fixture's local CA; external Python and curl verify
  its certificate chain. Redirect safety depends on client configuration; the
  deliberate unsafe curl control demonstrates that distinction.
- Restoring an older backup intentionally restores older credential state.
  Treat native backups as sensitive, retain the separate encryption key, and
  revoke/reset credentials after an untrusted restore. Ledger exports exclude
  credential state and are not a full installation backup.
- Native SQLite's C engine is not instrumented by Go's race detector. Driver
  hooks are test-only barriers, and the live WAL/SHM byte copies are leakage
  samples, never substitutes for the verified native backup/restore drill.
- Signing, advisory/upstream-driver disposition, native-language review and
  human household use retain their existing issues; this audit does not
  change #336/#337/#356 or make a blanket release-readiness claim.


## Adjacent expiry-admission audit completed

The follow-up [#360 report](authentication-expiry-audit-2026-10-20.md) confirms
and fixes queued session/device admission, MFA challenge/TOTP deadlines,
throttle clocks and the final MFA-disable cutoff (#362–#365). Its open finding
[#366](https://github.com/sergeyfarin/rekenraam/issues/366) concerns reservation
settlement across throttle windows and has its own repair/audit scope. These
results supersede this report's earlier source-pattern concern; the PAT grant,
proxy and restore conclusions above keep their original bounded scope.
