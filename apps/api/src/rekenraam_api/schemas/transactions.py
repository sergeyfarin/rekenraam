from datetime import date, datetime

from pydantic import BaseModel, ConfigDict, model_validator


class TransactionListFilters(BaseModel):
    model_config = ConfigDict(frozen=True)

    book_id: int | None = None
    account_id: int | None = None
    payee_id: int | None = None
    status: str | None = None
    occurred_from: date | None = None
    occurred_to: date | None = None
    search: str | None = None
    amount_min: int | None = None
    amount_max: int | None = None
    sort_by: str | None = None
    sort_dir: str | None = None
    limit: int | None = None
    offset: int | None = None
    cursor: str | None = None

    @model_validator(mode="after")
    def validate_date_range(self) -> TransactionListFilters:
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
    commodity_id: int
    amount_minor: int
    category_id: int | None
    tag_id: int | None
    person_id: int | None
    project_id: int | None
    share_bps: int | None
    memo: str | None
    created_at: datetime


class TransactionSplitInput(BaseModel):
    model_config = ConfigDict(frozen=True)

    account_id: int
    commodity_id: int
    amount_minor: int
    category_id: int | None
    tag_id: int | None
    person_id: int | None
    project_id: int | None
    share_bps: int | None
    memo: str | None


class TransactionMutationInput(BaseModel):
    model_config = ConfigDict(frozen=True)

    book_id: int
    txn_date: date
    payee_id: int | None
    memo: str | None
    status: str
    reference: str | None
    import_id: str | None = None
    splits: tuple[TransactionSplitInput, ...]


class TransactionSummary(BaseModel):
    model_config = ConfigDict(frozen=True)

    id: int
    book_id: int
    previous_tx_id: int | None = None
    occurred_date: date
    posted_date: date
    payee_id: int | None
    memo: str | None
    status: str
    reference: str | None
    import_id: str | None = None
    created_at: datetime
    splits: tuple[SplitEntry, ...]


class TransactionPage(BaseModel):
    model_config = ConfigDict(frozen=True)

    items: tuple[TransactionSummary, ...]
    next_cursor: str | None


class PayeeDefaults(BaseModel):
    model_config = ConfigDict(frozen=True)

    category_id: int | None
    memo: str | None
