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