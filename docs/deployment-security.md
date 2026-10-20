# Deployment Security

This guide is the minimum security posture for a LAN or externally reachable
Rekenraam deployment. The app is a single HTTP process; it does not terminate
TLS itself. Put it behind a reverse proxy for HTTPS.

Run exactly one Rekenraam process or container against a SQLite database file.
Multiple replicas sharing one database volume are not a supported v0.1
deployment. Cross-process queue/restore guards are verified in #105/#227;
they do not authorize multiple server replicas sharing one database.

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
- Wrong **sign-in** codes spend the same five-in-fifteen throttle budget as
  wrong sign-in passwords.
- Wrong **settings** confirmations — passwords for enrolling, disabling, or
  regenerating recovery codes, and pending activation codes — share one
  five-in-fifteen budget per session
  ([#262](https://github.com/sergeyfarin/rekenraam/issues/262)). Once spent,
  that session gets `429 RATE_LIMITED` with `Retry-After` for every settings
  confirmation, even a correct one, and nothing changes. The budget belongs to
  the session, not the owner: a stolen session cannot lock the owner out, and
  the owner's way back is simply to sign in again. It neither spends nor
  clears the sign-in budget.
- A code cannot be replayed inside its own 30-second step. Enrollment writes
  cannot overwrite an already active secret; activation pins the exact pending
  secret whose code was verified (#261).
- Enrolling, disabling, and regenerating recovery codes each require the
  password again, so a stolen session cannot change what protects the account.
- Recovery codes are the only remote way back in if the authenticator is lost.
  There is no server-side bypass, by design. With both gone, the last resort is
  the `recover-owner` command on the host, with the server stopped (it refuses
  while the server holds the database), which resets the owner password and
  clears the enrolment in the same transaction — it already requires filesystem
  access to the database, so an enrolment must not outlast it.

Localhost development may use HTTP. Browser authentication uses Secure cookies;
on a non-loopback LAN hostname or address it needs HTTPS through a reverse
proxy for the browser to retain those cookies. A browser-warning-free private deployment needs a
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
    reverse_proxy 127.0.0.1:16888
}
```

Caddy's defaults already do what the app needs. They pass `Host` through and
replace any client-supplied `X-Forwarded-For`, `X-Forwarded-Host` and
`X-Forwarded-Proto` with the values Caddy observed, unless Caddy's own
`trusted_proxies` says otherwise. Explicit `header_up` lines for those headers
are redundant, and Caddy warns about them. Caddy passes `Forwarded` and
`X-Real-IP` through unchanged; Rekenraam ignores both. This behavior was
observed through a real Caddy deployment in the
[#268 audit](reviews/proxy-browser-privacy-audit-2026-10-17.md). Its
disposable recipe is in
[`deploy/docker/proxy-audit/`](../deploy/docker/proxy-audit/README.md). For
another proxy, confirm the same overwrite behavior before trusting it.

Enable forwarded headers in Rekenraam only when the direct proxy source is
known, and allowlist only that source:

```sh
TRUST_PROXY_HEADERS=1 TRUSTED_PROXY_CIDRS=127.0.0.1/32,::1/128
```

For a containerized proxy, give it a fixed address on its own network and
allowlist exactly that address (`/32`), as the audit recipe does. Never use `0.0.0.0/0`, never allowlist an
entire LAN merely for convenience, and never enable proxy-header trust when a
client can connect to the app directly. The app parses XFF right-to-left and
stops at the first non-trusted hop; the proxy must overwrite or append the
actual client address instead of forwarding unverified client headers unchanged.

After deploying, check the `client_ip` values in `GET /api/v1/auth/events`.
They are the addresses the proxy saw. When clients reach a containerized proxy
through a loopback-published port, Docker's userland proxy hides them, and
every event shows the bridge gateway. That result is correct, but the per-IP
throttle budget is then one budget shared by every client. Approved devices,
not the IP budget, are what keep a remote attacker from locking the owner out.

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

Failed logins are throttled at five attempts per fifteen minutes. Attempts
still being verified count against that budget, so sending guesses in parallel
does not get more of them checked (#285). Because a
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
- Successful password or completed-MFA login renews both the approved-device
  database expiry and its browser cookie, retaining one device identity (#263).
- The approved device keeps the same five-in-fifteen budget, so a stolen
  cookie buys no extra password guesses.
- Approval lapses after 180 days unused and slides forward on each successful
  login. Review and revoke devices at `GET`/`DELETE /api/v1/auth/trusted-devices`.

Approved devices are a throttle mechanism only, and are unrelated to the second
factor: an approved device still has to present a code.

## Secrets and data files

Personal-access tokens (#334) are managed at **Settings → Security**. Create a
named token, choose its read permissions and lifetime, confirm your password,
then save the plaintext when it is shown once. Default expiry is 30 days;
the maximum is 365 days. Use `Authorization: Bearer <token>` over HTTPS with
the [explicitly granted read endpoints](plans/personal-access-tokens-plan.md).
No financial-write, backup, export, settings or token-management grant exists.
Tokens store only SHA-256 hashes and do not need `REKENRAAM_SECRET_KEY`.
Never put tokens in URLs or configure proxy logs to capture Authorization.
Revoke unwanted tokens here; `recover-owner` revokes all tokens with sessions.
Restoring an older backup restores its credential state, so revoke tokens after
restoring an untrusted snapshot. [ADR 0018](adrs/0018-personal-access-token-boundary.md)
records the exact authentication and event-bookkeeping boundary.

`REKENRAAM_SECRET_KEY` encrypts stored online-provider credentials **and the
MFA shared secret**. It is a
base64 value for exactly 32 random bytes:

```sh
openssl rand -base64 32
```

Keep it in the service environment or a secret manager, outside Git, and back
it up with the SQLite database. Losing it leaves ledger data intact but makes
stored provider credentials and any MFA enrolment unreadable — recover with a
recovery code, or with the `recover-owner` command from the host; see the
[developer workflow](developer-workflow.md#local-owner-recovery) for the
backup-first recovery procedure.

With a changed or missing key, the authenticator app's codes stop working.
The sign-in screen says so and asks for a recovery code instead
(`MFA_AUTHENTICATOR_UNAVAILABLE`). These attempts do not spend the sign-in
budget, because no authenticator code could have been checked. They appear
in the security log as `mfa_authenticator_unavailable`. A wrong recovery code
still counts as a failed attempt. Enter a recovery code. Then, at Settings → Security, turn two-factor
off with your password and set it up again. Setup needs a configured key:
without one it refuses with `CONFIG_REQUIRED`. The status still reads "On"
until you do this. Do not change it casually: no in-place key
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

The Docker build context admits only the source inputs used by the Dockerfile.
`.dockerignore` excludes local runtime databases/backups, environment files and
generated dependencies/output, so these do not reach a remote builder. The
runtime stage applies available Debian package updates before installing its
utilities. Rebuild regularly; this does not mean the image has no advisories.
The [2026-10-20 image review](reviews/runtime-image-advisory-audit-2026-10-20.md)
classifies the current residual inventory against the actual app linkage and
fixed loopback health check. No application exploit was reproduced; this is
not a zero-advisory or blanket safety claim. **Release is on hold and #337
is blocked on the owner's explicit disposition of the recorded inventory and
limits.** The separate [bundled SQLite review
(#354)](reviews/native-sqlite-advisory-audit-2026-10-20.md) is complete: malformed
search refusal #355 is fixed; two reproduced upstream correctness defects are
retained in #356, blocked on a released driver carrying their fixes. No affected
application financial consumer or native memory exploit was reproduced.
Debian-package and Go-module scans alone do not establish native-engine status.
The [2026-10-18 deployment audit](reviews/deployment-audit-2026-10-18.md)
retains the original package-update evidence.

## Backups

**A backup is the only complete copy; an export is not a backup.** Restoring a
backup brings back the whole installation: ledger, investments, budgets,
recurring transactions, reconciliations, settings and audit history. Use one to
recover, or to move to another server. The CSV/QIF/archive exports on the same
screen are portable files for other tools. They cannot be imported back into
Rekenraam and leave out budgets, recurring transactions, saved reconciliations
and settings (decided 2026-10-18, `product-requirements.md`).

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

A complete backup is **the database and the attachments directory**. Attachments
are not implemented yet, so today the second half is empty and the scheduled
backup covers everything there is. It is named here, in the export manifest, in
the restore output, and in the ledger self-check from the start, so that adding
attachment storage extends an existing promise instead of quietly breaking one
that was already made.

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

Restore holds the same `<database>.lock` throughout verification and replacement,
so another restore or server startup refuses until it finishes. The lock file
remains after shutdown or restore; a leftover file or PID does not mean a
process is running. The OS lock establishes ownership. Leave that file in
place: removing it can let processes lock different files at the same path.

**Interrupted restore.** Ctrl-C or a stop signal before the previous database
is moved aside ends the restore with nothing replaced. Once it has been moved,
the install runs to completion. A restore killed outright between those two
points (power loss, `SIGKILL`) cannot say so. It leaves nothing at the database
path, the previous files whole in `<database>.before-restore-<timestamp>/`, and
possibly a partial `<database>.restoring`. Do not start the server in that
state: it would open a new, empty database. Run the same `restore` again. It
installs the backup, overwrites the partial file and leaves the earlier
preserved copy untouched. To go back instead, move the preserved files beside
the database path under their original names.

Restoring onto a host without the original `REKENRAAM_SECRET_KEY` succeeds and
leaves an intact ledger — and unreadable two-factor enrolment and unusable
connection credentials. `verify-backup` says so before you commit to it.
