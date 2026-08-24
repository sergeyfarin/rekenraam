# Deployment Security

This guide is the minimum security posture for a LAN or externally reachable
Rekenraam deployment. The app is a single HTTP process; it does not terminate
TLS itself. Put it behind a reverse proxy for HTTPS.

## Public-deployment gate: turn MFA on

**Shipped 2026-08-07 (S-06).** Multi-factor authentication is TOTP (RFC 6238,
any authenticator app) plus ten single-use recovery codes. The product gate is
no longer "MFA does not exist"; it is now an operator step:

**Before exposing real financial data to the public internet, enrol the owner
account in two-factor authentication** at Settings → Security, and store the
recovery codes somewhere other than the machine running the app. Enrolment
requires `REKENRAAM_SECRET_KEY` (see below), because the shared secret is
sealed at rest rather than stored in the clear — without the key the server
refuses to enrol rather than degrade quietly.

What it does and does not do:

- A verified password alone no longer produces a session once MFA is active.
  It produces a five-minute, single-use challenge cookie that grants nothing.
- Wrong codes spend the same five-in-fifteen throttle budget as wrong
  passwords, so a six-digit code is not a free guessing target.
- A code cannot be replayed inside its own 30-second step.
- Enrolling, disabling, and regenerating recovery codes each require the
  password again, so a stolen session cannot change what protects the account.
- Recovery codes are the only remote way back in if the authenticator is lost.
  There is no server-side bypass, by design. With both gone, the last resort is
  the `recover-owner` command on the host, which resets the owner password and
  clears the enrolment in the same transaction — it already requires filesystem
  access to the database, so an enrolment must not outlast it.

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

## Login throttling and approved devices

Failed logins are throttled at five attempts per fifteen minutes. Because a
single-owner install publishes its owner's username by construction, a plain
username-scoped throttle would be a remote lockout switch: anyone could fail
five logins and keep the owner out of their own finances indefinitely.

A device that has completed a successful login (or first-run owner setup) is
therefore issued an **approved-device cookie**, and attempts from it are
throttled on that device's own budget instead of the shared username and
client-IP budgets. An attacker elsewhere on the internet can no longer lock the
owner out.

Read this carefully when reasoning about the security posture:

- The cookie is **not a credential**. It grants no access at all — a login
  still requires the password, and presenting the cookie alone authenticates
  nothing. Its only effect is which throttle budget an attempt spends.
- It is HttpOnly, SameSite=Strict, and only its SHA-256 hash is stored.
- The approved device keeps the same five-in-fifteen budget, so a stolen
  cookie buys no extra password guesses.
- Approval lapses after 180 days unused and slides forward on each successful
  login. Review and revoke devices at `GET`/`DELETE /api/v1/auth/trusted-devices`.

Approved devices are a throttle mechanism only, and are unrelated to the second
factor: an approved device still has to present a code.

## Secrets and data files

`REKENRAAM_SECRET_KEY` encrypts stored online-provider credentials **and the
MFA shared secret**. It is a
base64 value for exactly 32 random bytes:

```sh
openssl rand -base64 32
```

Keep it in the service environment or a secret manager, outside Git, and back
it up with the SQLite database. Losing it leaves ledger data intact but makes
stored provider credentials and any MFA enrolment unreadable — recover with a
recovery code, or with the `recover-owner` command from the host; see the root README for the
backup-first recovery procedure. Do not change it casually: no in-place key
rotation command exists yet.

The app enforces mode `0600` on the SQLite database and its `-wal` and `-shm`
sidecars after startup/migration; SQLite backups created by the app receive the
same mode, and a backup directory the app creates is made `0700`. Keep the
containing data directory accessible only to the service account — the app
cannot do this for you when the directory already exists, because it only sets
the mode on directories it creates itself. Pointing a backup path into an
existing world-readable directory still yields a `0600` file, but in a
directory that lists its name to every local user. Backups include financial
data and encrypted credential blobs, so protect them as strictly as the live
database. Never copy a live WAL database file as a normal backup; use the
documented SQLite-aware backup process.

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

## Backups

Scheduled backups are on by default and write to `BACKUP_DIR` (or
`<database directory>/backups`). Two deployment decisions the app cannot make
for itself:

1. **Put `BACKUP_DIR` on a different device from the database.** The default is
   convenient, not durable: it survives file corruption and human error, and not
   the disk. A mounted volume, a second disk, or an offsite sync target are all
   better than the default.
2. **Keep `REKENRAAM_SECRET_KEY` somewhere other than the backup directory.** It
   is deliberately absent from every backup — it seals two-factor enrolment and
   connection credentials, and storing a key beside its ciphertext defeats the
   sealing. A password manager or a secret store is the right home. Without it, a
   restored database is an intact ledger whose two-factor enrolment and stored
   connection credentials must be recreated.

The backup directory is created `0700` and each file `0600`. Retention deletes
only files the app recorded, named by its own pattern, that resolve inside the
configured directory; anything else placed there is left untouched.

Until the word "protected" can honestly cover both points above, the product
does not use it: `docs/plans/data-portability-plan.md` records why.

## Restore

`rekenraam verify-backup --from <path>` checks a backup without touching
anything: integrity, schema version against the running build, row counts, and
whether sealed rows decrypt under the configured `REKENRAAM_SECRET_KEY`. Run it
on a schedule of your own choosing — a backup nobody has ever opened is a
hypothesis.

`rekenraam restore --from <path>` installs one. It refuses while the server is
running, proved by an advisory lock the server process holds for its whole life
rather than inferred from an idle connection or a missing `-wal` file. It
preserves the previous database as a complete file set under
`<database>.before-restore-<timestamp>/`, checkpointing first so that copy is
self-contained, and installs the new file atomically with an fsync before
reporting success. The preserved copy is never deleted by the tool.

Restoring onto a host without the original `REKENRAAM_SECRET_KEY` succeeds and
leaves an intact ledger — and unreadable two-factor enrolment and unusable
connection credentials. `verify-backup` says so before you commit to it.
