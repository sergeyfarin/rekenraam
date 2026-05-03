from datetime import date

from pydantic import BaseModel, ConfigDict


class CashflowReportInput(BaseModel):
    model_config = ConfigDict(frozen=True)

    book_id: int
    date_from: date | None = None
    date_to: date | None = None
    group_by: str | None = None


class CashflowRow(BaseModel):
    model_config = ConfigDict(frozen=True)

    period_start: date
    inflow_minor: int
    outflow_minor: int
    net_minor: int


class CategorySpendReportInput(BaseModel):
    model_config = ConfigDict(frozen=True)

    book_id: int
    date_from: date | None = None
    date_to: date | None = None
    category_ids: tuple[int, ...] | None = None


class CategorySpendRow(BaseModel):
    model_config = ConfigDict(frozen=True)

    category_id: int
    category_name: str
    total_minor: int


class PayeeTotalsReportInput(BaseModel):
    model_config = ConfigDict(frozen=True)

    book_id: int
    date_from: date | None = None
    date_to: date | None = None
    payee_ids: tuple[int, ...] | None = None


class PayeeTotalRow(BaseModel):
    model_config = ConfigDict(frozen=True)

    payee_id: int
    payee_name: str
    total_minor: int