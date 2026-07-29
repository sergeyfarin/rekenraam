# Security Audit — 2026-07-07

Point-in-time security review of Rekenraam for its intended deployment shapes:
primarily local network, potentially exposed externally (VPS or reverse proxy).
Scope: auth stack, CSRF/session handling, HTTP middleware, config, outbound
fetchers, secret storage, static serving, Docker deployment, and the existing
backlog. Backlog items already tracking security work (T-01 session lifetime,
T-03 CSRF rotation, T-04 CSP `unsafe-inline`, T-18 session cleanup) are
excluded from the findings below.

## Overall verdict

The security baseline is strong for a self-hosted app:

- argon2id with rehash-on-login and a precomputed dummy hash defeating the
  "no such user" timing oracle (`backend/internal/app/auth.go`).
- Opaque session tokens (32 random bytes) stored only as SHA-256 hashes.
- `__Host-` prefixed cookies over HTTPS, `HttpOnly`, `SameSite=Strict`.
- Origin validation plus an `X-CSRF-Token` header required on every mutation
  (`requireAuthenticatedMutation` in `backend/internal/api/auth.go`).
- Dual-scope login throttling (username + client IP) with `Retry-After`.
- Locked-down CSP and a full security-header set; HSTS only over HTTPS
  (`backend/internal/api/middleware.go`).
- Proxy-header trust gated by explicit `TRUSTED_PROXY_CIDRS`, refusing
  0-bit prefixes (`backend/internal/config/config.go`).
- AES-256-GCM sealing of provider API keys (`backend/internal/secretbox`);
  keys never echoed back in API responses (only `key_hint`).
- SSRF hardening in the Trading 212 fetcher (absolute `nextPagePath` refused,
  page cap); fixed provider base URLs, not user-controlled.
- Embedded-FS static serving — no filesystem path traversal possible
  (`backend/internal/web/static.go`).
- 1 MB request body caps, strict JSON decoding (unknown fields rejected,
  content-type enforced).
- Non-root container user; govulncheck + Dependabot in CI.
- No `{@html}` usage in the frontend (Svelte auto-escaping intact).

## Findings not in the backlog

### S-01 First-run setup race (highest priority for VPS exposure)

`POST /api/v1/setup/owner` (`backend/internal/api/health.go`) is
unauthenticated — same-origin check only. If the app is reachable (VPS, opened
port, misconfigured proxy) before the owner completes setup, anyone can claim
ownership. The same window reopens conceptually around `recover-owner` resets.

**Fix options:** print a one-time setup token to the server log at boot and
require it in the create-owner request; or refuse non-loopback setup unless an
env flag opts in.

### S-02 No `http.Server` timeouts

`backend/cmd/rekenraam/command.go` sets only `Addr` and `Handler`. No
`ReadHeaderTimeout`, `ReadTimeout`, `WriteTimeout`, or `IdleTimeout` — a
trivial slowloris connection-exhaustion target when internet-exposed.
Five-line fix.

### S-03 `X-Forwarded-For` parsing takes the leftmost value

`loginClientIP` (`backend/internal/api/auth.go`) cuts the *first* XFF entry,
but the leftmost value is client-supplied even behind a trusted proxy (the
proxy appends, it does not replace). With `TRUST_PROXY_HEADERS` enabled an
attacker can spoof arbitrary client IPs, defeating the `client_ip` throttle
scope and poisoning any future IP-based logging.

**Fix:** walk the XFF chain from the right and take the first hop that is not
inside `TRUSTED_PROXY_CIDRS`.

### S-04 Username throttle doubles as an owner-lockout DoS

Five bad attempts on the (single, guessable) username blocks the *username*
scope for 15 minutes (`backend/internal/app/auth.go`), so an internet attacker
can keep the legitimate owner locked out indefinitely with ~20 requests/hour.

**Fix direction:** OWASP "device cookie" pattern — a browser that previously
logged in successfully carries a signed cookie exempting it from the
username-scope block, so throttling only bites unknown devices.

### S-05 In-app TLS never landed; deployment posture undocumented

ADR 0002's follow-up ("add config for TLS certificate and key paths") was
never implemented and is not in the backlog. Reverse-proxy-only is a valid
posture, but then deploy docs must say so explicitly — currently
`deploy/docker/compose.yaml` publishes `16888:16888` on all interfaces with no
proxy, no HTTPS, and no mention of `REKENRAAM_SECRET_KEY`,
`TRUST_PROXY_HEADERS`, or `TRUSTED_PROXY_CIDRS`.

### S-06 No second factor

For external exposure, TOTP (or WebAuthn/passkeys) is the single
highest-value auth upgrade available. Absent from the app and from the
roadmap/backlog.

### S-07 No auth-event visibility

Request logs record method/path/status but not client IP, and there is no
durable record of login successes/failures. Adding the (proxy-aware) client IP
to the request log line — or a dedicated auth-event log — enables fail2ban
integration and makes brute-force attempts observable.

### S-08 SQLite file permissions

`backend/internal/db/sqlite.go` creates the database with the process umask
(typically 0644): on a shared host, other local users can read the entire
financial history plus the sealed API-key blobs. Chmod the DB (and
`-wal`/`-shm` siblings) to 0600 after open, or document a umask requirement.

## Smaller hardening items

- **Container** (`deploy/docker/`): add `cap_drop: [ALL]`,
  `security_opt: [no-new-privileges:true]`, `read_only: true` (data volume
  writable), a healthcheck; bind `127.0.0.1:16888:16888` in the compose
  example when a reverse proxy runs on the same host.
- **Headers** (`backend/internal/api/middleware.go`): add
  `Cross-Origin-Opener-Policy: same-origin` and
  `Cross-Origin-Resource-Policy: same-origin`.
- **`X-Request-ID` echo** (`backend/internal/api/middleware.go`): the
  client-supplied header is reflected verbatim into the response and logs; cap
  its length and restrict its charset.
- **Docs**: a "deployment security" page — reverse proxy + TLS example,
  trusted-proxy CIDRs, secret key generation, backup guidance (backups contain
  everything) — would close ADR 0002's other open follow-up.

## Suggested priority

For "LAN today, maybe VPS tomorrow":

1. **Immediately** (quick wins): S-02 server timeouts, S-03 XFF fix.
2. **Before any external exposure**: S-01 setup token, S-05/S-08
   deployment-security docs + DB permissions.
3. **Feature investments that make VPS exposure defensible**: S-06 2FA,
   S-04 device-cookie lockout protection, S-07 auth-event logging.
