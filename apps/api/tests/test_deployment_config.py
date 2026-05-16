from pathlib import Path

ROOT = Path(__file__).resolve().parents[3]


def _read(path: str) -> str:
    return (ROOT / path).read_text(encoding="utf-8")


def test_production_compose_keeps_database_private_and_uses_proxy_overlay() -> None:
    compose = _read("compose.prod.example.yaml")
    proxy = _read("compose.proxy.yaml")

    assert "postgres:" in compose
    assert "ports: !reset []" in compose
    assert "DATABASE_KIND: postgresql" in compose
    assert "POSTGRES_PASSWORD_FILE: /run/secrets/postgres_password" in compose
    assert "FIRST_ADMIN_PASSWORD_FILE: /run/secrets/first_admin_password" in compose
    assert "caddy:" in proxy
    assert "REKENRAAM_PUBLIC_HOST" in proxy


def test_test_compose_keeps_postgres_off_host_port_5432() -> None:
    compose = _read("compose.test.yaml")

    assert "postgres:" in compose
    assert "ports: !reset []" in compose


def test_postgres_compose_is_explicit_three_container_stack() -> None:
    compose = _read("compose.postgres.yaml")

    assert "postgres:" in compose
    assert "api:" in compose
    assert "frontend:" in compose
    assert "DATABASE_KIND: postgresql" in compose
    assert "postgres_data:/var/lib/postgresql/data" in compose


def test_sqlite_compose_uses_private_data_volume_and_no_postgres_dependency() -> None:
    compose = _read("compose.sqlite.yaml")
    public = _read("compose.sqlite.public.yaml")

    assert "postgres:" not in compose
    assert "frontend:" not in compose
    assert "DATABASE_URL" not in compose
    assert "rekenraam_data:/data" in compose
    assert "${APP_PORT:-8080}:8080" in compose
    assert "SESSION_COOKIE_SECURE" in public
    assert "FIRST_ADMIN_PASSWORD_FILE" in public


def test_production_env_defaults_require_public_origin_and_secure_cookie() -> None:
    env = _read(".env.production.example")

    assert "CORS_ALLOWED_ORIGINS=https://finance.example.com" in env
    assert "SESSION_COOKIE_SECURE=true" in env
    assert "SESSION_COOKIE_SAMESITE=lax" in env
    assert "MFA_SECRET_KEY=replace-with-a-long-random-string" in env
    assert "POSTGRES_PASSWORD_FILE=./secrets/postgres_password.txt" in env
    assert "/data/rekenraam.sqlite3" in env
    assert "FIRST_ADMIN_PASSWORD_FILE=./secrets/first_admin_password.txt" in env


def test_self_hosting_docs_cover_lan_and_public_vps_security_modes() -> None:
    docs = _read("docs/deployment/self-hosting.md")

    assert "Home/LAN Server" in docs
    assert "VPS With HTTPS" in docs
    assert "SESSION_COOKIE_SECURE=true" in docs
    assert "MFA_ENFORCED=true" in docs
    assert "Do not publish PostgreSQL" in docs
    assert "trusted LAN" in docs
    assert "one container" in docs
    assert "tooling is intentionally deferred until after V1" in docs
    assert "restore-smoke" in docs
