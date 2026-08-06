# Deployment Security

This guide is the minimum security posture for a LAN or externally reachable
Rekenraam deployment. The app is a single HTTP process; it does not terminate
TLS itself. Put it behind a reverse proxy for HTTPS.

## Non-negotiable public-deployment gate

**Do not deploy real financial data on the public internet until MFA has an
approved design and implementation.** This is a product requirement, not an
operator choice. The measures below make a future external deployment safer;
they do not lift the MFA gate.

Localhost development may use HTTP. For LAN use, HTTPS through a reverse proxy
is strongly preferred. A browser-warning-free private deployment needs a
certificate trusted by every client (for example, for a real domain or from a
local CA installed on those clients).

## First run

Production always protects owner creation with a setup token. Set a durable,
random `SETUP_TOKEN` of at least 32 characters before starting the app:

```sh
export SETUP_TOKEN="$(openssl rand -base64 32)"
```

Enter that value into the setup form's **Setup token** field. It is sent only
as the `X-Setup-Token` request header and is not stored by the browser. If the
variable is absent, production generates a random one-time token and writes it
to the server log; copy it from the local operator log before completing setup.
Do not expose those logs or put either token in Git. Once an owner exists, the
endpoint cannot create another owner.

## Reverse proxy and TLS

Bind the app to loopback and let the proxy own the public listener. For a
single binary on the same host, configure the app with an address such as
`HTTP_ADDR=127.0.0.1:16888`. The provided Compose example already publishes
only `127.0.0.1:16888`.

Use a reverse proxy that obtains and renews a real TLS certificate and sends
the original request host, scheme, and client-address chain. A Caddy-shaped
example is:

```caddyfile
finance.example.com {
    reverse_proxy 127.0.0.1:16888 {
        header_up X-Forwarded-For {remote_host}
        header_up X-Forwarded-Host {host}
        header_up X-Forwarded-Proto {scheme}
    }
}
```

Enable forwarded headers in Rekenraam only when the direct proxy source is
known, and allowlist only that source:

```sh
TRUST_PROXY_HEADERS=1 TRUSTED_PROXY_CIDRS=127.0.0.1/32,::1/128
```

For a containerized proxy, use the exact container-network address or narrow
CIDR observed by Rekenraam instead. Never use `0.0.0.0/0`, never allowlist an
entire LAN merely for convenience, and never enable proxy-header trust when a
client can connect to the app directly. The app parses XFF right-to-left and
stops at the first non-trusted hop; the proxy must overwrite or append the
actual client address instead of forwarding unverified client headers unchanged.

## Monitoring authentication

Every successful and failed authentication attempt is recorded durably, with
the proxy-aware client IP resolved by the rules above. Two ways to consume it:

- `GET /api/v1/auth/events` (owner session required) returns the recent log,
  newest first, plus `failed_last_24h` — the number to watch. A steady trickle
  of `login_failed` from one address is a brute-force run; `unknown_user`
  failures mean someone is guessing account names, not passwords.
- The same events are emitted as structured `slog` records ("authentication
  event"), failures at `WARN`, so a log shipper can alert on them without
  querying SQLite.

Events never contain password material or session tokens. They are pruned to a
90-day retention window by the daily session-cleanup pass, so this is an
incident-response aid, not a permanent sign-in archive.

Note that repeated failures against the known owner username currently rate
limit that username — see the open lockout concern in `docs/backlog.md` (S-04).

## Secrets and data files

`REKENRAAM_SECRET_KEY` encrypts stored online-provider credentials. It is a
base64 value for exactly 32 random bytes:

```sh
openssl rand -base64 32
```

Keep it in the service environment or a secret manager, outside Git, and back
it up with the SQLite database. Losing it leaves ledger data intact but makes
stored provider credentials unreadable; see the root README for the
backup-first recovery procedure. Do not change it casually: no in-place key
rotation command exists yet.

The app enforces mode `0600` on the SQLite database and its `-wal` and `-shm`
sidecars after startup/migration; SQLite backups created by the app receive the
same mode. Keep the containing data directory accessible only to the service
account. Backups include financial data and encrypted credential blobs, so
protect them as strictly as the live database. Never copy a live WAL database
file as a normal backup; use the documented SQLite-aware backup process.

## Docker Compose

Before starting the supplied Compose file, set the setup token in an
untracked environment file or secret manager:

```sh
export SETUP_TOKEN="$(openssl rand -base64 32)"
export REKENRAAM_SECRET_KEY="$(openssl rand -base64 32)" # needed before online connections are used
docker compose -f deploy/docker/compose.yaml up --build
```

The Compose example is deliberately loopback-only, runs as a non-root user,
uses a read-only root filesystem, drops Linux capabilities, and leaves only the
SQLite volume and a temporary directory writable. Add a TLS reverse proxy
instead of changing its port mapping to all interfaces.
