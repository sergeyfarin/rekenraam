from datetime import date, datetime

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


class ReportDefinitionCreateInput(BaseModel):
    model_config = ConfigDict(frozen=True)

    book_id: int
    name: str
    kind: str = "custom"
    query_type: str = "sql"
    query_text: str
    params_schema: str | None = None


class ReportDefinitionUpdateInput(BaseModel):
    model_config = ConfigDict(frozen=True)

    id: int
    book_id: int
    name: str
    kind: str = "custom"
    query_type: str = "sql"
    query_text: str
    params_schema: str | None = None


class ReportDefinitionSummary(BaseModel):
    model_config = ConfigDict(frozen=True)

    id: int
    book_id: int
    name: str
    kind: str
    query_type: str
    query_text: str
    params_schema: str | None
    created_at: datetime


class ReportRunCreateInput(BaseModel):
    model_config = ConfigDict(frozen=True)

    book_id: int
    definition_id: int
    params_hash: str
    as_of_seq: int
    pricing_mode: str = "latest_corrected"
    pricing_policy_id: int | None = None
    pricing_policy_version: str | None = None
    valuation_snapshot_id: int | None = None
    pricing_resolved_at: datetime | None = None
    result_json: str


class ReportRunSummary(BaseModel):
    model_config = ConfigDict(frozen=True)

    id: int
    book_id: int
    definition_id: int
    params_hash: str
    as_of_seq: int
    pricing_mode: str
    pricing_policy_id: int | None
    pricing_policy_version: str | None
    valuation_snapshot_id: int | None
    pricing_resolved_at: datetime | None
    result_json: str
    created_at: datetime