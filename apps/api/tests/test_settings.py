import pytest

from rekenraam_api.config.settings import Settings


def test_settings_build_database_url_from_fields() -> None:
    settings = Settings(
        database_kind="postgresql",
        postgres_user="finance",
        postgres_password="secret",
        postgres_host="db",
        postgres_port=6543,
        postgres_db="ledger",
    )

    assert settings.database_url == "postgresql+asyncpg://finance:secret@db:6543/ledger"


def test_settings_default_to_sqlite_data_volume(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.delenv("SQLITE_PATH", raising=False)
    settings = Settings()

    assert settings.database_url == "sqlite+aiosqlite:////data/rekenraam.sqlite3"


def test_database_url_override_takes_precedence() -> None:
    settings = Settings(DATABASE_URL="sqlite+aiosqlite:////data/rekenraam.sqlite3")

    assert settings.database_url == "sqlite+aiosqlite:////data/rekenraam.sqlite3"


def test_first_admin_seed_settings_are_optional() -> None:
    settings = Settings()

    assert settings.first_admin_email is None
    assert settings.first_admin_password is None
    assert settings.first_admin_display_name == "Admin"


def test_file_backed_postgres_password(tmp_path) -> None:
    password_file = tmp_path / "postgres_password.txt"
    password_file.write_text("from-file\n", encoding="utf-8")

    settings = Settings(database_kind="postgresql", postgres_password_file=str(password_file))

    assert settings.resolved_postgres_password == "from-file"
    assert "from-file" in settings.database_url


def test_file_backed_secret_conflict_fails(tmp_path) -> None:
    password_file = tmp_path / "postgres_password.txt"
    password_file.write_text("from-file\n", encoding="utf-8")
    settings = Settings(postgres_password="different", postgres_password_file=str(password_file))

    with pytest.raises(ValueError, match="POSTGRES_PASSWORD"):
        _ = settings.resolved_postgres_password


def test_file_backed_first_admin_password(tmp_path) -> None:
    password_file = tmp_path / "first_admin_password.txt"
    password_file.write_text("admin-from-file\n", encoding="utf-8")

    settings = Settings(
        first_admin_email="admin@example.test",
        first_admin_password_file=str(password_file),
    )

    assert settings.resolved_first_admin_password == "admin-from-file"


def test_cors_and_trusted_proxy_lists_trim_empty_values() -> None:
    settings = Settings(
        cors_allowed_origins=" https://finance.example.test, ,http://localhost:3000 ",
        trusted_proxy_cidrs=" 10.0.0.0/8,,172.16.0.0/12 ",
    )

    assert settings.cors_allowed_origins_list == [
        "https://finance.example.test",
        "http://localhost:3000",
    ]
    assert settings.trusted_proxy_cidrs_list == ["10.0.0.0/8", "172.16.0.0/12"]
