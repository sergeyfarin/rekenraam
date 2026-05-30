# ADR 0002: HTTP Security Policy

## Status

Accepted

## Date

2026-05-30

## Context

Rekenraam is a browser-based personal finance app that uses one Go binary to serve both the static SvelteKit frontend and the Go API from the same origin. The app will store sensitive financial data, but the first deployable shape is still self-hosted and single-user.

The project needs a clear policy for HTTPS, browser cookies, and CSRF before local authentication and real data entry are implemented.

## Decision

The app will use a same-origin browser security model:

1. The production frontend and API are served from the same origin.
2. Production deployments do not enable cross-origin browser API access by default.
3. Frontend development uses the Vite `/api` proxy so browser requests still behave as same-origin requests.

HTTPS policy:

1. Public deployment requires HTTPS.
2. Localhost development may use HTTP.
3. LAN and other private-network deployments should use HTTPS.
4. LAN/private HTTPS is supported by either a reverse proxy or app-provided certificate and key configuration.
5. Browser-warning-free LAN/private HTTPS requires either a certificate for a real trusted domain or installing a trusted local certificate authority on every client device that should trust the app.

Session policy:

1. Browser sessions use opaque random tokens stored in `HttpOnly` cookies.
2. Only a hash of each session token is stored in SQLite.
3. Session cookies use `SameSite=Strict`, `Path=/`, and no `Domain`.
4. Session cookies use `Secure` whenever the app is served over HTTPS.
5. HTTPS deployments should use a `__Host-` prefixed session cookie name once cookie implementation lands.

CSRF policy:

1. `GET`, `HEAD`, and `OPTIONS` endpoints must not mutate durable state.
2. Mutating API requests use JSON request bodies unless a feature explicitly needs another content type.
3. Mutating API requests must verify the request `Origin` matches the app origin.
4. Mutating API requests must include a CSRF token in a custom header such as `X-CSRF-Token`.
5. The CSRF token may be implemented as a session-bound synchronizer token or a signed double-submit token, but it must be validated server-side before privileged mutations run.

## Consequences

### Positive

- The browser security model stays simple because the app is same-origin by default.
- `SameSite=Strict` reduces ambient cross-site cookie exposure.
- Origin checks and explicit CSRF tokens protect mutating endpoints even if browser defaults change or a future flow relaxes SameSite behavior.
- Local development remains low-friction.
- Operators have clear paths for warning-free HTTPS without pretending private IP certificates can be automatically trusted by browsers.

### Negative

- LAN/private HTTPS without browser warnings requires operator setup: a trusted domain, reverse proxy, or local CA installation.
- Public deployment needs more documentation before it is recommended for real financial data.
- CSRF support adds a small API/client contract that every mutating frontend request must follow.

### Follow-Up

- Add app configuration for TLS certificate and key paths when implementing deployment configuration.
- Document Caddy/reverse-proxy and local-CA examples before recommending LAN/private deployments for real financial data.
- Implement auth middleware, session storage, cookie issuance, CSRF token issuance, and CSRF validation before real financial data entry ships.
