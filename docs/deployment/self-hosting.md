# Self-Hosting Rekenraam

Rekenraam supports a single-server Docker Compose deployment for both home/LAN
servers and small VPS installs. The app stack is PostgreSQL, FastAPI, and the
nginx-served Svelte frontend. TLS termination is owned by your reverse proxy or
hosting platform.

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
   `FIRST_ADMIN_EMAIL`, and set `CORS_ALLOWED_ORIGINS` to the browser origin
   users will visit.
4. Render-check the Compose files:

   ```bash
   make prod-config-check
   ```

5. Start the stack:

   ```bash
   docker compose -f compose.yaml -f compose.prod.example.yaml up -d --build postgres api frontend
   ```

6. Verify:

   ```bash
   curl --fail http://localhost:${FRONTEND_PORT:-3000}/api/v1/health
   ```

## Home/LAN Server

For trusted LAN use, `FRONTEND_PORT=3000` can be exposed directly and
`SESSION_COOKIE_SECURE=false` may be used only while the site is served over
plain HTTP. Do not expose that configuration to the public internet.

## VPS With HTTPS

For public access, put Caddy, Traefik, nginx, or your platform proxy in front of
the frontend container and serve the site over HTTPS. Use:

```env
SESSION_COOKIE_SECURE=true
CORS_ALLOWED_ORIGINS=https://finance.example.com
MFA_ENFORCED=true
```

Do not publish PostgreSQL on the host. The production override clears the base
Postgres port mapping so only services inside the Compose network can reach it.

## Backups

The supported logical backup is PostgreSQL custom format:

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
4. Start with `docker compose -f compose.yaml -f compose.prod.example.yaml up -d --build`.
5. Check `/api/v1/health`, Settings -> Runtime, and the integrity check.

## Public Security Checklist

- HTTPS is required for public access.
- `SESSION_COOKIE_SECURE=true`.
- `CORS_ALLOWED_ORIGINS` contains only the public origin.
- `MFA_ENFORCED=true` for public VPS installs.
- Use file-backed secrets for database and first-admin passwords.
- Keep PostgreSQL private to the Compose network.
- Schedule backups and perform restore drills.
