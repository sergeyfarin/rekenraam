import pytest

from rekenraam_api.config.settings import Settings


def test_settings_build_database_url_from_fields() -> None:
    settings = Settings(
        postgres_user="finance",
        postgres_password="secret",
        postgres_host="db",
        postgres_port=6543,
        postgres_db="ledger",
    )

    assert settings.database_url == "postgresql+asyncpg://finance:secret@db:6543/ledger"


def test_first_admin_seed_settings_are_optional() -> None:
    settings = Settings()

    assert settings.first_admin_email is None
    assert settings.first_admin_password is None
    assert settings.first_admin_display_name == "Admin"


def test_file_backed_postgres_password(tmp_path) -> None:
    password_file = tmp_path / "postgres_password.txt"
    password_file.write_text("from-file\n", encoding="utf-8")

    settings = Settings(postgres_password_file=str(password_file))

    assert settings.resolved_postgres_password == "from-file"
    assert "from-file" in settings.database_url


def test_file_backed_secret_conflict_fails(tmp_path) -> None:
    password_file = tmp_path / "postgres_password.txt"
    password_file.write_text("from-file\n", encoding="utf-8")
    settings = Settings(postgres_password="different", postgres_password_file=str(password_file))

    with pytest.raises(ValueError, match="POSTGRES_PASSWORD"):
        _ = settings.resolved_postgres_password
