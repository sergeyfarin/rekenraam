# Self-Hosting

Rekenraam V1 is SQLite-only and defaults to one container. The app
listens on port `16888` inside the container and publishes
`http://localhost:16888` by default.

## Home/LAN Server

```bash
docker compose up -d --build
curl --fail http://localhost:16888/api/v1/health
```

Keep the `rekenraam_data` volume private to the host. This mode is appropriate
for a trusted LAN or local machine. Override the published host port with
`APP_PORT`, for example:

```bash
APP_PORT=18080 docker compose up -d --build
```

## VPS With HTTPS

1. Copy `.env.production.example` to `.env`.
2. Set `REKENRAAM_PUBLIC_HOST`, `CORS_ALLOWED_ORIGINS`, and a long
   `MFA_SECRET_KEY`.
3. Create `secrets/first_admin_password.txt`.
4. Start the app plus proxy:

```bash
docker compose -f compose.yaml -f compose.public.yaml -f compose.proxy.yaml up -d --build
```

Public deployments should keep:

- `SESSION_COOKIE_SECURE=true`
- `SESSION_COOKIE_SAMESITE=lax`
- `MFA_ENFORCED=true` once users have enrolled
- `TRUSTED_PROXY_CIDRS` limited to the private proxy network

Caddy checks:

```bash
docker compose -f compose.yaml -f compose.public.yaml -f compose.proxy.yaml logs -f caddy
docker compose -f compose.yaml -f compose.public.yaml -f compose.proxy.yaml exec caddy caddy validate --config /etc/caddy/Caddyfile
```

## Backups

The default Compose file mounts `./backups` at `/backups`. Use the online backup
command instead of copying the live database file:

```bash
make backup-now
make backup-smoke
```

Validate a backup before relying on it:

```bash
make restore-smoke BACKUP=backups/rekenraam-YYYYmmdd-HHMMSS.sqlite3
```

The smoke command opens the backup read-only, runs `PRAGMA integrity_check`, and
verifies both `alembic_version` and `books` are readable.

## Recovery

For a full restore, stop the app, copy the selected validated backup over the
database file in the `rekenraam_data` volume, then start the app and run the
admin integrity check. Keep the prior volume snapshot until the restored app has
been verified.

## Operational Smoke

```bash
make prod-config-check
docker compose up -d --build --wait
curl --fail http://localhost:16888/api/v1/health
make backup-smoke
docker compose down -v
```
