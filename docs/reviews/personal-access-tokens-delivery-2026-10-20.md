# Personal-access-token delivery — 2026-10-20

**Status:** #334's bounded implementation acceptance is complete. Small
integration finding #358 is fixed. At delivery, independent audit #357 was
open. The [subsequent audit](personal-access-token-audit-2026-10-20.md) is now
complete with #359 fixed; adjacent expiry audit #360 remains separate. Neither
report establishes actual owner deployment; #336 retains that acceptance.

Starting baseline: `834e9c295a4641b70d47c1f6841d1657b558cfd0`.
Implementation: [`15498153eae8b73586aa4e64f8959582fd4e8d5c`](https://github.com/sergeyfarin/rekenraam/commit/15498153eae8b73586aa4e64f8959582fd4e8d5c).
The [contract](../plans/personal-access-tokens-plan.md) was written and posted on
#334 before implementation. [ADR 0018](../adrs/0018-personal-access-token-boundary.md)
records the durable authentication and security-bookkeeping boundary.

## Shipped boundary

Two independent **read-only** scopes, `ledger:read` and `reports:read`, admit 25
explicit registered GET patterns and their HEAD aliases. Every other route is
denied, including financial mutations, exports, backups, settings, session/MFA,
setup and token administration. A supplied Authorization header never falls
back to a valid browser cookie. This is a single-owner credential mechanism,
with no account-level restriction, write grant or multi-user expansion.

Settings → Security creates named credentials with password confirmation using
the existing shared session reauthentication budget. Defaults are 30 days,
maximum 365 days and at most 50 active credentials. Metadata listing consumes
ID cursors. The browser displays plaintext once, keeps it outside query caches
and browser storage, and scrubs component/DOM secrets on pagehide; a late
creation response cannot repopulate a scrubbed disclosure. Revocation remains
session/CSRF/Origin-only. Owner recovery revokes tokens atomically with sessions.

Credentials have 256 random bits; SQLite/native backups retain only
domain-separated SHA-256 digests and lifecycle metadata. Creation, revocation
and use/refusal events carry non-secret IDs and request/client attribution.
Event failure rolls back lifecycle changes or refuses authentication. Token
use is not cached. A request authenticated before revocation may finish; later
authentication is denied. Restoring an old backup restores its credential state,
so the operator guidance explicitly covers subsequent revocation.

## Consumer evidence

All inputs are synthetic and provider-free. Expected values below are fixed
contract assertions, independent of the code's returned metadata.

| Scenario / named regression | Expected result | Actual |
| --- | --- | --- |
| `TestPersonalAccessTokenGrantsMatchOpenAPI` | Runtime grants exactly match declared Bearer alternatives/scopes; every admitted GET documents 401/403 | Pass, all 25 patterns |
| `TestPersonalAccessTokenAPIReadScopesAndDefaultDeny` | Every granted HTTP consumer matches the owner-session result; the opposite scope is refused, both scopes form their union, HEAD works | Pass; no cookies issued |
| Same HTTP case, forbidden route matrix | Mutations, auth/setup/settings, exports/maintenance/investments, unknown routes and OPTIONS cannot inherit access even with a valid owner cookie/CSRF | Pass, 403; the real posted transaction retains one version and `posted` status |
| `TestPersonalAccessTokenAPIHeaderPrecedenceAndManagementProtection` | Missing browser protections refuse management; malformed/empty/multiple Authorization cannot fall back to cookies; URL tokens do not authenticate; expired/revoked tokens deny HTTP reads | Pass; 400 for null/fraction/string/out-of-range expiry; no secret/hash in listing |
| `TestPersonalAccessTokenLifecycleScopesExpiryAndHashOnlyBackup` | Creation at 2026-10-20 12:00Z defaults to 2026-11-19 12:00Z; exact expiry boundary denies; grants are independent; native backup has digest and no plaintext | Pass, including verified SQLite backup and non-secret attributed events |
| `TestPersonalAccessTokenCreationValidationAndSharedReauthenticationBudget` | Invalid names/scopes/lifetimes mint nothing; four wrong token confirmations plus one wrong MFA confirmation exhaust one shared budget; correct input remains blocked | Pass; no token rows created |
| `TestPersonalAccessTokenSecurityEventsFailClosedAndRecoveryRevokes` | Forced event failures roll back creation, revoke, last-use and recovery; successful recovery denies the previous token | Pass; failed creation returns no plaintext; recovery records `tokens_revoked: 1` |
| `TestPersonalAccessTokenListConsumesHistoryAndActiveLimit` | 51 historic rows are reachable as 50 + 1 with no overlap; revoked history does not spend the active limit | Pass; active limit refuses further creation |
| `TestPersonalAccessTokenManagementRechecksSessionAndPasswordAndConcurrentLimit` | Stale password or revoked session cannot mint; two requests competing for the final place produce exactly one success and one limit refusal; revoke between lookup/use denies authentication | Pass; no post-revocation use event |
| `TestPersonalAccessTokenMigrationPreservesHistoryAndRefusesLossyDowngrade` | Migration preserves prior event fields; unused schema can downgrade; token history prevents lossy downgrade | Pass |
| `TestMigrateSeededBaselineMatchesFreshSchema`, `TestMigrationsProduceTheExpectedSchema` | Migration 0019 adds only an empty credential table/index; every seeded durable financial figure remains exact; fresh/upgraded schema agrees | Pass; schema 19, 117 tables / 119 named indexes / 160 triggers / 15 views |

Migration 0019 preserves all prior authentication-event columns explicitly.
The existing baseline and pinned migrations are unchanged. No reset is needed;
ledger bundle schema remains 15 and excludes credential rows.

## Browser evidence and controls

`personal-access-tokens.spec.ts` runs eleven cases: two complete 390px light/dark
creation/use/revocation journeys, metadata loading/error recovery, immediate
pagehide scrubbing, delayed-response disclosure prevention, and permission
controls in all six locales. The complete journeys check no plaintext in local
or session storage, metadata, event responses or the real disposable app log;
they also exercise axe, modal initial/trapped focus and horizontal fit.

These ran with the two existing login cases and all three real MFA cases:
**16 passed, 23.0 seconds**, no retries or skips. MFA enrollment, authenticator
challenge and single-use recovery codes still work beside token management.

Two useful pre-delivery controls were retained in the execution record:

- An earlier local build without the disclosure-generation guard fails
  `a delayed creation response cannot restore a token after document hiding`:
  a delayed POST repopulates the secret and replaces the form after pagehide.
  The final build passes the same case.
- The first combined run found #358: substring `Your password` locators matched
  both MFA and token password fields (one failure, two dependent skips, thirteen
  passes). Both instances were made exact, including the proxy harness sibling;
  localized MFA cases already used exact matching. The final same-command run
  passes all sixteen. This fixes the integration harness, not an MFA verifier.

The grant/spec parity regression also caught a missing institution GET 403
declaration before delivery. The first broad migration gate named its two
stale schema/seed expectations; updating those explicitly admits the new empty
table while keeping all financial comparisons. These were corrected within
the new feature before publication.

## Validation commands

- `./scripts/test-backend.sh` — **pass**, including formatting, vet and all
  packages with Go race detection, two concurrent packages and five-minute
  per-package deadline. API 138.499s, app 253.684s, database 159.373s.
- `./scripts/test-frontend.sh` — **pass**: zero Svelte errors/warnings,
  66 catalog cases and 691 unit cases.
- `pnpm build` — **pass**: static frontend, embedded-asset test and Go binary.
- `E2E_SKIP_BUILD=1 pnpm --dir e2e exec playwright test personal-access-tokens.spec.ts mfa.spec.ts auth.spec.ts --workers=1`
  — **16 passed** against the rebuilt single binary.
- `./scripts/check-post-merge.sh HEAD` and staged `git diff --check` — **clean**.
  No merge was performed; the structural check also verifies client regeneration
  and numbered-file uniqueness.
- After the final migration EOF whitespace cleanup, the committed migration's
  focused upgrade/schema/credential-history tests and Go binary build were
  repeated. No financial/schema behavior changed in that cleanup.

An initial invocation of `test-e2e.sh` with filter arguments started the full
suite because that wrapper does not forward arguments. It was deliberately
interrupted; it is **not** a full-browser acceptance run. The documented direct
Playwright command above is the completed scoped evidence. Remote CI is
recorded on the delivery issues after pushing; local success is not a claim
that pending remote checks have passed.

## Disposition and retained limits

#334 is complete for the agreed read-only API credential contract. #358 is
fixed in the implementation commit. No unresolved confirmed financial or
application authentication defect was found in this bounded delivery.

**Additional audit #357**, logged on #260, independently challenges the new
route/header/concurrency/failure boundary through a real disposable HTTPS proxy
and external client, checks redacted logging and restored credential state, and
names any remaining owner-only deployment evidence for #336. The earlier
#267/#268 audits predate tokens. This report does not claim that audit, a fresh
full browser suite, a new Compose/proxy run or native-language approval (#102).
Go race detection does not instrument SQLite's C engine. Other release holds
and the existing #356 driver prerequisite remain unchanged.

Maintainability: token creation deliberately reuses the existing password
confirmation/throttle owner rather than a second verifier. Exact grants must
stay synchronized with the plan and OpenAPI; the permanent parity test guards
that coupling. No unrelated refactor is included.
