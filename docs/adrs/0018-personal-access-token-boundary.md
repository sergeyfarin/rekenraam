# ADR 0018: Scoped owner API credentials

## Status

Accepted for #334. Extends ADR 0002 only for explicit Bearer authentication
and its operational security bookkeeping; the browser session policy remains.

## Date

2026-10-20

## Decision

Provide opaque, revocable, hash-only personal-access tokens for the existing
single owner. The initial grants are independent read-only `ledger:read` and
`reports:read`, with the exact method/registered-route allowlist in the
[contract](../plans/personal-access-tokens-plan.md). All other operations default
to denial. A supplied Authorization header selects this authentication method
and never falls back to session cookies. Token management remains a browser
session operation; creation additionally confirms the password using the shared
session reauthentication budget. No financial write grant is introduced.

Use 256 random bits, domain-separated SHA-256 storage and absolute bounded
expiry. Return plaintext only at creation and never persist it in browser state
outside component memory. Native backups retain only hashes and lifecycle
metadata; operator recovery revokes tokens and sessions together.

Authentication on GET/HEAD records only operational security metadata in
`authentication_events` and `personal_access_tokens.last_used_at`. This is a
narrow exception to ADR 0002's ban on durable GET/HEAD mutations, needed to make
credential use and refusals observable and let owners identify unused tokens.
No financial facts, preferences, credentials, expiry, approvals or grants are
changed by reads. Token-use bookkeeping and lifecycle checks share a transaction;
event-write failure denies token access. Creation and revocation also commit
atomically with their events. Routine use/refusal events have 90-day retention;
security lifecycle events remain retained.

## Consequences

Scripts have least-privilege credentials without owning a browser session.
Exact route grants prevent new endpoints from silently inheriting access.
Read-only credentials deliberately cannot run financial commands, exports,
backups, settings or credential administration. Additional grants need a new
bounded contract and tests. Token reads take a short write transaction for
security events; a read with failed telemetry returns an error. An already
authenticated request may finish after revocation, matching the session cutoff.

This is a local application contract. Real deployment TLS, reverse-proxy header
redaction, owner release acceptance and independent security review need their
own evidence; this ADR makes no blanket public-deployment readiness claim.
