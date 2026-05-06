from datetime import date

from pydantic import BaseModel, ConfigDict


class RuntimeCheckSummary(BaseModel):
    model_config = ConfigDict(frozen=True)

    name: str
    status: str
    detail: str


class AdminRuntimeStatusSummary(BaseModel):
    model_config = ConfigDict(frozen=True)

    database_kind: str
    database_name: str
    database_host: str | None
    database_user: str | None = None
    postgres_version: str | None = None
    display_path: str
    size_bytes: int | None
    writable: bool
    foreign_keys: bool
    current_version: str | None
    latest_version: str
    pending_versions: tuple[str, ...]
    pending_migration_count: int = 0
    health_status: str = "ok"
    backup_status: str = "operator-managed"
    backup_guidance: str = "Use pg_dump -Fc through the documented Compose backup profile."
    last_integrity_status: str | None = None
    last_integrity_checked_at: str | None = None
    note: str


class FiscalYearCloseInput(BaseModel):
    model_config = ConfigDict(frozen=True)

    close_date: date
    memo: str | None = None


class FiscalYearCloseResult(BaseModel):
    model_config = ConfigDict(frozen=True)

    tx_id: int | None
    closed_accounts_count: int
    retained_earnings_delta_minor: int
    close_date: date


class IntegrityCheckSummary(BaseModel):
    model_config = ConfigDict(frozen=True)

    status: str
    checked_at: str | None = None
    checks: tuple[RuntimeCheckSummary, ...] = ()
