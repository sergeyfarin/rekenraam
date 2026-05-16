# Self-Hosting Rekenraam

Rekenraam supports a single-server Docker Compose deployment for both home/LAN
servers and small VPS installs. The app stack is FastAPI, the nginx-served
Svelte frontend, and either PostgreSQL or SQLite. PostgreSQL remains the default
server database; SQLite is available as an explicit low-memory overlay for
small machines such as Raspberry Pi-class hosts. The production Compose example
includes Caddy as the public reverse proxy and TLS terminator.

## Fresh Install

1. Install Docker Engine and the Docker Compose plugin.
2. Copy the production environment template:

   ```bash
   cp .env.production.example .env
   mkdir -p secrets backups
   openssl rand -base64 36 > secrets/postgres_password.txt
   openssl rand -base64 36 > secrets/first_admin_password.txt
   openssl rand -hex 32
   ```

3. Put the final command output in `MFA_SECRET_KEY`, set
   `FIRST_ADMIN_EMAIL`, set `REKENRAAM_PUBLIC_HOST` to the public hostname, and
   set `CORS_ALLOWED_ORIGINS` to `https://` plus that same hostname.
4. Render-check the Compose files:

   ```bash
   make prod-config-check
   make prod-sqlite-config-check # only if using compose.sqlite.yaml
   ```

5. Start the PostgreSQL stack:

   ```bash
   docker compose -f compose.yaml -f compose.prod.example.yaml up -d --build
   ```

   Or start the SQLite low-memory stack:

   ```bash
   docker compose -f compose.yaml -f compose.prod.example.yaml -f compose.sqlite.yaml up -d --build
   ```

6. Verify:

   ```bash
   curl --fail http://127.0.0.1:${FRONTEND_PORT:-3000}/api/v1/health
   curl --fail https://${REKENRAAM_PUBLIC_HOST}/api/v1/health
   ```

## Home/LAN Server

For trusted LAN use, `FRONTEND_PORT=3000` can be exposed directly and
`SESSION_COOKIE_SECURE=false` may be used only while the site is served over
plain HTTP. The production template binds that direct frontend port to
`127.0.0.1` by default; set `FRONTEND_BIND=0.0.0.0` only for a trusted LAN.
Do not expose that configuration to the public internet.

For low-memory LAN installs, prefer the SQLite overlay:

```bash
docker compose -f compose.yaml -f compose.sqlite.yaml up -d --build
```

This keeps the API and frontend deployment shape the same, but stores the
database in the `sqlite_data` volume instead of starting PostgreSQL.

## VPS With HTTPS

For public access, point an `A` or `AAAA` record for `REKENRAAM_PUBLIC_HOST` at
the server, and allow inbound TCP `80`, TCP `443`, and optionally UDP `443`.
Caddy uses the HTTP-01 ACME challenge on port `80`, stores certificates in the
`caddy_data` volume, redirects HTTP to HTTPS, and proxies HTTPS traffic to the
frontend container.

Use:

```env
REKENRAAM_PUBLIC_HOST=finance.example.com
SESSION_COOKIE_SECURE=true
CORS_ALLOWED_ORIGINS=https://finance.example.com
MFA_ENFORCED=true
TRUSTED_PROXY_CIDRS=172.16.0.0/12
```

Do not publish PostgreSQL on the host. The production override clears the base
Postgres port mapping so only services inside the Compose network can reach it.
When using SQLite, keep the `sqlite_data` volume private to the host. The direct
frontend check port is bound to `127.0.0.1` unless you explicitly change
`FRONTEND_BIND`.

To inspect the TLS proxy:

```bash
docker compose -f compose.yaml -f compose.prod.example.yaml logs -f caddy
docker compose -f compose.yaml -f compose.prod.example.yaml exec caddy caddy validate --config /etc/caddy/Caddyfile
```

## Backups

For PostgreSQL, the supported logical backup is PostgreSQL custom format:

```bash
make backup-now
```

This runs `pg_dump -Fc` through the `backup` Compose profile and writes
`backups/rekenraam-YYYYmmdd-HHMMSS.dump`. Schedule it from the host rather than
from the app:

```cron
15 2 * * * cd /srv/rekenraam && docker compose -f compose.yaml -f compose.prod.example.yaml --profile backup run --rm backup
```

A systemd timer can run the same command from the repository directory. Keep at
least one copy off the server. Volume snapshots are useful as a supplement only
when the storage layer guarantees a consistent snapshot or PostgreSQL is
stopped before the snapshot.

For SQLite, stop the API before copying the database files, or use SQLite's
online backup API from an operator script. Keep the main database file and any
`-wal` and `-shm` files together. Bidirectional PostgreSQL/SQLite migration
tooling is intentionally deferred until after V1; do not switch database engines
for an existing deployment without a tested export/import procedure.

## Restore Drill

Always restore into a clean database first:

```bash
make restore-smoke BACKUP=backups/rekenraam-YYYYmmdd-HHMMSS.dump
```

The smoke check creates a temporary database, restores the dump with
`pg_restore`, verifies Alembic metadata and book readability, then drops the
temporary database. Restoring an untrusted dump can execute SQL from the source;
inspect untrusted dumps before use.

For a real disaster restore, stop the app, create a fresh database or volume,
restore the selected dump with `pg_restore`, start the stack, then run the admin
runtime and integrity checks from Settings.

## Upgrade Flow

1. Run `make backup-now`.
2. Pull or deploy the new code.
3. Run `make prod-config-check`.
4. Start with the same database engine already in use:
   `docker compose -f compose.yaml -f compose.prod.example.yaml up -d --build`
   for PostgreSQL, or add `-f compose.sqlite.yaml` for SQLite.
5. Check `/api/v1/health`, Settings -> Runtime, and the integrity check.

## Public Security Checklist

- HTTPS is required for public access.
- `SESSION_COOKIE_SECURE=true`.
- `CORS_ALLOWED_ORIGINS` contains only the public origin.
- `REKENRAAM_PUBLIC_HOST` resolves to the server before starting Caddy.
- `MFA_ENFORCED=true` for public VPS installs.
- Use file-backed secrets for database and first-admin passwords when running
  PostgreSQL.
- Keep PostgreSQL private to the Compose network, or keep the SQLite volume
  private to the host.
- Keep the direct frontend port bound to `127.0.0.1`, or block it at the host
  firewall.
- Schedule backups and perform restore drills.
