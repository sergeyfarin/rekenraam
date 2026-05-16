from pathlib import Path

ROOT = Path(__file__).resolve().parents[3]


def _read(path: str) -> str:
    return (ROOT / path).read_text(encoding="utf-8")


def test_production_compose_keeps_database_and_api_private() -> None:
    compose = _read("compose.prod.example.yaml")

    assert "postgres:" in compose
    assert "ports: !reset []" in compose
    assert "POSTGRES_PASSWORD_FILE: /run/secrets/postgres_password" in compose
    assert "FIRST_ADMIN_PASSWORD_FILE: /run/secrets/first_admin_password" in compose
    assert "FRONTEND_BIND:-127.0.0.1" in compose


def test_test_compose_keeps_postgres_off_host_port_5432() -> None:
    compose = _read("compose.test.yaml")

    assert "postgres:" in compose
    assert "ports: !reset []" in compose


def test_sqlite_compose_uses_private_data_volume_and_no_postgres_dependency() -> None:
    compose = _read("compose.sqlite.yaml")

    assert "DATABASE_URL: sqlite+aiosqlite:////data/rekenraam.sqlite3" in compose
    assert "depends_on: !reset []" in compose
    assert "sqlite_data:/data" in compose
    assert 'profiles: ["postgres"]' in compose


def test_production_env_defaults_require_public_origin_and_secure_cookie() -> None:
    env = _read(".env.production.example")

    assert "CORS_ALLOWED_ORIGINS=https://finance.example.com" in env
    assert "SESSION_COOKIE_SECURE=true" in env
    assert "SESSION_COOKIE_SAMESITE=lax" in env
    assert "MFA_SECRET_KEY=replace-with-a-long-random-string" in env
    assert "POSTGRES_PASSWORD_FILE=./secrets/postgres_password.txt" in env
    assert "DATABASE_URL=sqlite+aiosqlite:////data/rekenraam.sqlite3" in env
    assert "FIRST_ADMIN_PASSWORD_FILE=./secrets/first_admin_password.txt" in env


def test_self_hosting_docs_cover_lan_and_public_vps_security_modes() -> None:
    docs = _read("docs/deployment/self-hosting.md")

    assert "Home/LAN Server" in docs
    assert "VPS With HTTPS" in docs
    assert "SESSION_COOKIE_SECURE=true" in docs
    assert "MFA_ENFORCED=true" in docs
    assert "Do not publish PostgreSQL" in docs
    assert "SQLite low-memory stack" in docs
    assert "tooling is intentionally deferred until after V1" in docs
    assert "restore-smoke" in docs
