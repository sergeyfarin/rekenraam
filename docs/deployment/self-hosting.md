# Self-Hosting Rekenraam

Rekenraam supports a single-server Docker Compose deployment for both home/LAN
servers and small VPS installs. The simplest SQLite path is one FastAPI
container that serves both the API and the built frontend. PostgreSQL deployments
use three containers: API, frontend, and PostgreSQL. Public HTTPS is a separate
Caddy proxy overlay.

## Fresh Install

1. Install Docker Engine and the Docker Compose plugin.
2. For the simplest trusted LAN install, start SQLite:

   ```bash
   docker compose -f compose.sqlite.yaml up -d --build
   ```

   Open `http://localhost:8080`, create the first owner account, and keep this
   basic setup on a trusted local network only. Do not expose it directly to the
   public internet before adding HTTPS and stronger setup controls.

3. For public or PostgreSQL installs, copy the production environment template:

   ```bash
   cp .env.production.example .env
   mkdir -p secrets backups
   openssl rand -base64 36 > secrets/postgres_password.txt
   openssl rand -base64 36 > secrets/first_admin_password.txt
   openssl rand -hex 32
   ```

4. Put the final command output in `MFA_SECRET_KEY`, set
   `FIRST_ADMIN_EMAIL`, set `REKENRAAM_PUBLIC_HOST` to the public hostname, and
   set `CORS_ALLOWED_ORIGINS` to `https://` plus that same hostname.
5. Render-check the Compose files:

   ```bash
   make prod-config-check
   make prod-sqlite-config-check
   ```

6. Start the public PostgreSQL stack:

   ```bash
   docker compose -f compose.postgres.yaml -f compose.prod.example.yaml -f compose.proxy.yaml up -d --build
   ```

   Or start the public SQLite stack:

   ```bash
   docker compose -f compose.sqlite.yaml -f compose.sqlite.public.yaml -f compose.proxy.yaml up -d --build
   ```

7. Verify:

   ```bash
   curl --fail http://127.0.0.1:8080/api/v1/health
   curl --fail https://${REKENRAAM_PUBLIC_HOST}/api/v1/health
   ```

## Home/LAN Server

For trusted LAN use, prefer SQLite:

```bash
docker compose -f compose.sqlite.yaml up -d --build
```

This runs one container, stores the database in the `rekenraam_data` volume, and
serves the app at `http://localhost:8080`. `SESSION_COOKIE_SECURE=false` is
acceptable only while the site is served over plain HTTP on a trusted LAN. Do
not expose this basic setup to the public internet.

The first-run owner setup screen is designed for this trusted local-network
case. For public deployments, seed the first admin from environment/secrets or
use a one-time setup-token flow before exposing the site.

## VPS With HTTPS

For public access, point an `A` or `AAAA` record for `REKENRAAM_PUBLIC_HOST` at
the server, and allow inbound TCP `80`, TCP `443`, and optionally UDP `443`.
Caddy uses the HTTP-01 ACME challenge on port `80`, stores certificates in the
`caddy_data` volume, redirects HTTP to HTTPS, and proxies HTTPS traffic to
`app:8080`. In the SQLite stack, `app` is the FastAPI container. In the
PostgreSQL stack, the frontend container has the `app` network alias.

Use:

```env
REKENRAAM_PUBLIC_HOST=finance.example.com
SESSION_COOKIE_SECURE=true
CORS_ALLOWED_ORIGINS=https://finance.example.com
MFA_ENFORCED=true
TRUSTED_PROXY_CIDRS=172.16.0.0/12
```

Do not publish PostgreSQL on the host. The production override clears the
Postgres port mapping so only services inside the Compose network can reach it.
When using SQLite, keep the `rekenraam_data` volume private to the host.

To inspect the TLS proxy:

```bash
docker compose -f compose.sqlite.yaml -f compose.proxy.yaml logs -f caddy
docker compose -f compose.sqlite.yaml -f compose.proxy.yaml exec caddy caddy validate --config /etc/caddy/Caddyfile
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
15 2 * * * cd /srv/rekenraam && docker compose -f compose.postgres.yaml -f compose.prod.example.yaml --profile backup run --rm backup
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
   `docker compose -f compose.sqlite.yaml up -d --build` for trusted LAN
   SQLite, `docker compose -f compose.sqlite.yaml -f compose.sqlite.public.yaml -f compose.proxy.yaml up -d --build`
   for public SQLite, or
   `docker compose -f compose.postgres.yaml -f compose.prod.example.yaml -f compose.proxy.yaml up -d --build`
   for public PostgreSQL.
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
- Do not expose the basic first-run SQLite setup directly to the internet.
- Schedule backups and perform restore drills.
