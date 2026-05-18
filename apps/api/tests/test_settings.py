import pytest

from rekenraam_api.config.settings import Settings


def test_settings_build_sqlite_url_from_path() -> None:
    settings = Settings(sqlite_path="/tmp/ledger.sqlite3")

    assert settings.database_url == "sqlite+aiosqlite:////tmp/ledger.sqlite3"


def test_settings_default_to_sqlite_data_volume(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.delenv("SQLITE_PATH", raising=False)
    settings = Settings()

    assert settings.database_url == "sqlite+aiosqlite:////data/rekenraam.sqlite3"


def test_database_url_override_takes_precedence() -> None:
    settings = Settings(DATABASE_URL="sqlite+aiosqlite:////data/rekenraam.sqlite3")

    assert settings.database_url == "sqlite+aiosqlite:////data/rekenraam.sqlite3"


def test_non_sqlite_database_kind_is_rejected() -> None:
    settings = Settings(database_kind="mysql")

    with pytest.raises(ValueError, match="DATABASE_KIND must be sqlite"):
        _ = settings.database_url


def test_non_sqlite_database_url_override_is_rejected() -> None:
    settings = Settings(DATABASE_URL="mysql://example")

    with pytest.raises(ValueError, match="DATABASE_URL must use sqlite"):
        _ = settings.database_url


def test_first_admin_seed_settings_are_optional() -> None:
    settings = Settings()

    assert settings.first_admin_email is None
    assert settings.first_admin_password is None
    assert settings.first_admin_display_name == "Admin"


def test_file_backed_secret_conflict_fails(tmp_path) -> None:
    password_file = tmp_path / "first_admin_password.txt"
    password_file.write_text("from-file\n", encoding="utf-8")
    settings = Settings(
        first_admin_password="different",
        first_admin_password_file=str(password_file),
    )

    with pytest.raises(ValueError, match="FIRST_ADMIN_PASSWORD"):
        _ = settings.resolved_first_admin_password


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
