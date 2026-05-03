from datetime import date, datetime

from pydantic import BaseModel, ConfigDict, model_validator


class TransactionListFilters(BaseModel):
    model_config = ConfigDict(frozen=True)

    book_id: int | None = None
    account_id: int | None = None
    status: str | None = None
    occurred_from: date | None = None
    occurred_to: date | None = None

    @model_validator(mode="after")
    def validate_date_range(self) -> "TransactionListFilters":
        if (
            self.occurred_from is not None
            and self.occurred_to is not None
            and self.occurred_from > self.occurred_to
        ):
            raise ValueError("occurred_from must be on or before occurred_to")
        return self


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