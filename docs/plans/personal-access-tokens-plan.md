# Personal-access tokens (#334)

Contract established 2026-10-20 before implementation. This is the bounded
single-owner API credential contract for the public-announcement gate.

## Authorization

The first version supports two independent, read-only scopes. Neither implies
the other. No wildcard, administrator, write, export or credential-management
scope exists. Expanding these grants requires an explicit contract change and
route-level regression coverage.

| Scope | Admitted `/api/v1` GET routes (HEAD has the same grant) |
| --- | --- |
| `ledger:read` | `/books/current`; `/currencies`, `/currencies/catalog`; `/institutions`, `/institutions/{institution_id}`, `/institutions/{institution_id}/versions`; `/accounts`, `/accounts/{account_id}`, `/accounts/{account_id}/versions`, `/accounts/{account_id}/register`; `/categories`, `/categories/{category_id}`; `/tags`, `/tags/{tag_id}`; `/payees`, `/payees/{payee_id}`; `/transactions`, `/transactions/deleted`, `/transactions/{transaction_id}` |
| `reports:read` | `/ledger/account-balances`, `/ledger/category-totals`, `/ledger/net-worth`; `/reports/net-worth`, `/reports/spending`, `/reports/cashflow` |

An exact method + registered route allowlist defaults to denial for every other
route, including new routes, financial mutations, investment commands, imports,
exports, maintenance, settings, setup, login/session/MFA and token management.
These grants expose sensitive financial data across the owner's one book;
there is no account-level restriction or multi-user expansion.

Clients send only `Authorization: Bearer <token>` over HTTPS (loopback HTTP is
available for local development). Tokens are never accepted in URLs, JSON or
cookies. Any Authorization header selects token authentication: malformed,
duplicate, unknown, expired or revoked credentials return 401 without session
fallback, even when a valid cookie is present. A valid token lacking the grant
returns 403. There is no session creation, cookie issuance, sliding expiry,
browser credential persistence or CSRF exemption for session authentication.
Bearer reads need no Origin or CSRF header because the credential is explicit,
not ambient. Browser sessions keep their existing CSRF and origin rules.

## Lifecycle and secrets

`GET/POST /api/v1/auth/tokens` and `DELETE /api/v1/auth/tokens/{token_id}`
are owner-session-only. Creation also verifies the current password using the
existing session reauthentication budget (shared with MFA settings), then
rechecks the session and password hash in its database transaction. The active
session may already have completed MFA; tokens do not constitute an MFA bypass
for creating credentials. Revocation needs the owner session, Origin and CSRF.

Creation accepts a nonblank name of at most 80 bytes, a nonempty unique subset
of the two scopes and optional integer `expires_in_days` (1–365, default 30).
At most 50 unexpired, unrevoked tokens may exist. Names are metadata, never
places to paste secrets. Listing uses descending immutable IDs and an explicit
`next_cursor` (50 rows per page), including expired and revoked history.

Generate 32 cryptographically random bytes, encoded as base64url after
`rr_pat_`. Store only a domain-separated SHA-256 digest, never encryption or a
recoverable token. Only the successful creation response contains plaintext;
the UI holds it in component memory until dismissal/navigation. It never enters
query caches or browser storage. Document `pagehide` immediately scrubs component
and DOM secrets, including when the browser retains a history document; a
creation response arriving after that event cannot restore the disclosure.
Responses are `Cache-Control: no-store`.
SQLite, WAL, backups and exports contain no generated plaintext token. Native
backups retain hashes and lifecycle state; restoring an old backup can restore
then-valid credentials, so revoke them after restoring an untrusted snapshot.
Ledger exports contain no credential rows.

Revocation retains history and denies subsequent authentications, with no token
cache. A request whose authentication transaction committed before revocation
may finish, matching session revocation's existing in-flight cutoff. Expiry is
absolute and checked on every request (`expires_at <= now` denies). Management
session and token expiry checks sample the clock after acquiring both the database
connection and SQLite write admission; pool or writer-lock waits cannot extend
a credential's lifetime (#359). The committed authentication remains the
in-flight cutoff if expiry passes while its admitted consumer runs. Owner
password recovery revokes all tokens atomically with sessions and records the
count; browser logout ends only that session.

## Events and failure behavior

Creation and revocation append attributed `authentication_events` in the same
transaction; failure to record the event rolls back the change. Token
authentication and scope refusals record token ID, required scope, request ID,
owner and proxy-aware client IP, never headers, token hash, token, password,
names, financial values or query strings. Unknown/expired/revoked credentials
record a generic failed token authentication without disclosing their identity
to the caller. Token authentication events are operational security bookkeeping
on GET/HEAD, an explicit exception to the rule against durable GET mutations;
they never change financial state. Event failure refuses token access (500),
not a partial authentication. Routine token-use/refusal events have the existing
90-day operational retention; creation/revocation events are retained.

The existing authentication-event API presents these events; its request ID
correlates a use with request logging. High-entropy token checks use a bounded
format check and indexed digest lookup, not Argon2 or password throttle budgets.
Operators must prevent proxy access logs from capturing Authorization headers.
Independent external penetration testing and owner deployment acceptance remain
separate from the named service/API/browser regression evidence.
