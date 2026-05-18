from functools import lru_cache
from pathlib import Path

from pydantic import Field
from pydantic_settings import BaseSettings, SettingsConfigDict


class Settings(BaseSettings):
    model_config = SettingsConfigDict(env_file=".env", frozen=True, extra="ignore")

    app_host: str = "0.0.0.0"
    app_port: int = 16888
    database_kind: str | None = None
    database_url_override: str | None = Field(default=None, validation_alias="DATABASE_URL")
    sqlite_path: str = "/data/rekenraam.sqlite3"
    frontend_static_dir: str = "/app/static"
    cors_allowed_origins: str = (
        "http://localhost:16888,http://127.0.0.1:16888,"
        "http://localhost:3000,http://127.0.0.1:3000,"
        "http://localhost:4173,http://127.0.0.1:4173,"
        "http://localhost:5173,http://127.0.0.1:5173"
    )
    pricing_scheduler_enabled: bool = True
    pricing_scheduler_poll_seconds: int = 60
    first_admin_email: str | None = None
    first_admin_password: str | None = None
    first_admin_password_file: str | None = None
    first_admin_display_name: str = "Admin"
    session_cookie_secure: bool = False
    session_cookie_samesite: str = "lax"
    trusted_proxy_cidrs: str = ""
    login_rate_limit_attempts: int = 8
    login_rate_limit_window_seconds: int = 300
    mfa_enforced: bool = False
    mfa_secret_key: str = "dev-only-change-me"

    @property
    def resolved_first_admin_password(self) -> str | None:
        if self.first_admin_password is None and self.first_admin_password_file is None:
            return None
        return self._resolve_secret(
            "FIRST_ADMIN_PASSWORD", self.first_admin_password or "", self.first_admin_password_file
        )

    @staticmethod
    def _resolve_secret(name: str, direct_value: str, file_path: str | None) -> str:
        if not file_path:
            return direct_value
        file_value = Path(file_path).read_text(encoding="utf-8").strip()
        if (
            direct_value
            and direct_value not in {"change-me", "rekenraam"}
            and direct_value != file_value
        ):
            raise ValueError(f"{name} and {name}_FILE are both set with different values")
        return file_value

    @property
    def database_url(self) -> str:
        if self.database_url_override:
            if not self.database_url_override.startswith("sqlite+aiosqlite:///"):
                raise ValueError("DATABASE_URL must use sqlite+aiosqlite:/// for SQLite-only v1")
            return self.database_url_override
        database_kind = (self.database_kind or "sqlite").lower()
        if database_kind in {"sqlite", "sqlite3"}:
            return f"sqlite+aiosqlite:///{Path(self.sqlite_path)}"
        raise ValueError("DATABASE_KIND must be sqlite")

    @property
    def cors_allowed_origins_list(self) -> list[str]:
        return [origin.strip() for origin in self.cors_allowed_origins.split(",") if origin.strip()]

    @property
    def trusted_proxy_cidrs_list(self) -> list[str]:
        return [cidr.strip() for cidr in self.trusted_proxy_cidrs.split(",") if cidr.strip()]


@lru_cache(maxsize=1)
def get_settings() -> Settings:
    return Settings()
