from datetime import date, datetime

from pydantic import BaseModel, ConfigDict


class SplitEntry(BaseModel):
    model_config = ConfigDict(frozen=True)

    id: int
    tx_id: int
    account_id: int
    amount_minor: int
    memo: str | None
    created_at: datetime


class TransactionSummary(BaseModel):
    model_config = ConfigDict(frozen=True)

    id: int
    book_id: int
    occurred_date: date
    posted_date: date
    memo: str | None
    status: str
    created_at: datetime
    splits: tuple[SplitEntry, ...]