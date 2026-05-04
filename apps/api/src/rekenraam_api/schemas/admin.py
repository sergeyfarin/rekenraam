from datetime import date

from pydantic import BaseModel, ConfigDict


class AdminRuntimeStatusSummary(BaseModel):
    model_config = ConfigDict(frozen=True)

    database_kind: str
    database_name: str
    database_host: str | None
    display_path: str
    size_bytes: int | None
    writable: bool
    foreign_keys: bool
    current_version: str | None
    latest_version: str
    pending_versions: tuple[str, ...]
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