from datetime import date
from typing import Literal

from pydantic import BaseModel, ConfigDict

ReportExportKind = Literal[
    "cashflow", "category-spend", "payee-totals", "realized-gains", "unrealized-gains"
]


class ReportCsvExportRequest(BaseModel):
    model_config = ConfigDict(frozen=True)

    book_id: int = 1
    date_from: date | None = None
    date_to: date | None = None
    account_id: int | None = None
    base_commodity_id: int | None = None
    as_of_date: date | None = None
