from pathlib import Path

ROOT = Path(__file__).resolve().parents[3]
REMOVED_DB = "post" + "gres"


def _read(path: str) -> str:
    return (ROOT / path).read_text(encoding="utf-8")


def test_canonical_compose_is_single_sqlite_app_container() -> None:
    compose = _read("compose.yaml")

    assert "app:" in compose
    assert REMOVED_DB not in compose.lower()
    assert "frontend:" not in compose
    assert "${APP_PORT:-16888}:16888" in compose
    assert "SQLITE_PATH: /data/rekenraam.sqlite3" in compose
    assert "rekenraam_data:/data" in compose
    assert "./backups:/backups" in compose
    assert "http://localhost:16888/api/v1/health" in compose


def test_optional_public_overlay_keeps_app_private_and_uses_proxy() -> None:
    public = _read("compose.public.yaml")
    proxy = _read("compose.proxy.yaml")
    caddyfile = _read("docker/caddy/Caddyfile")

    assert "ports: !reset []" in public
    assert "SESSION_COOKIE_SECURE" in public
    assert "FIRST_ADMIN_PASSWORD_FILE" in public
    assert REMOVED_DB not in public.lower()
    assert "caddy:" in proxy
    assert "REKENRAAM_PUBLIC_HOST" in proxy
    assert "reverse_proxy app:16888" in caddyfile


def test_production_env_defaults_require_public_origin_and_secure_cookie() -> None:
    env = _read(".env.production.example")

    assert "APP_PORT=16888" in env
    assert "SQLITE_PATH=/data/rekenraam.sqlite3" in env
    assert "CORS_ALLOWED_ORIGINS=https://finance.example.com" in env
    assert "SESSION_COOKIE_SECURE=true" in env
    assert "SESSION_COOKIE_SAMESITE=lax" in env
    assert "MFA_SECRET_KEY=replace-with-a-long-random-string" in env
    assert "FIRST_ADMIN_PASSWORD_FILE=./secrets/first_admin_password.txt" in env
    assert REMOVED_DB not in env.lower()


def test_self_hosting_docs_cover_lan_and_public_vps_security_modes() -> None:
    docs = _read("docs/deployment/self-hosting.md")

    assert "Home/LAN Server" in docs
    assert "VPS With HTTPS" in docs
    assert "docker compose up -d --build" in docs
    assert "SESSION_COOKIE_SECURE=true" in docs
    assert "MFA_ENFORCED=true" in docs
    assert "Keep the `rekenraam_data` volume private" in docs
    assert "trusted LAN" in docs
    assert "one container" in docs
    assert "16888" in docs
