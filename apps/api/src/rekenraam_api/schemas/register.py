from datetime import date, datetime

from pydantic import BaseModel, ConfigDict


class RegisterEntry(BaseModel):
    model_config = ConfigDict(frozen=True)

    tx_id: int
    split_id: int
    account_id: int
    occurred_date: date
    posted_date: date
    payee_id: int | None
    memo: str | None
    status: str
    reference: str | None
    commodity_id: int
    category_id: int | None
    amount_minor: int
    running_balance_minor: int
    created_at: datetime