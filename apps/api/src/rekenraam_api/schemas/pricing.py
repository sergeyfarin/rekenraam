from datetime import date, datetime

from pydantic import BaseModel, ConfigDict


class PriceSourceSummary(BaseModel):
    model_config = ConfigDict(frozen=True)

    id: int
    name: str
    kind: str
    country_code: str | None = None
    website_url: str | None = None
    notes: str | None = None
    created_at: datetime


class PricingPolicySummary(BaseModel):
    model_config = ConfigDict(frozen=True)

    book_id: int
    base_currency_id: int
    base_currency_symbol: str | None
    default_source_id: int | None
    default_source_name: str | None
    refresh_enabled: bool
    refresh_hour_utc: int
    refresh_minute_utc: int
    max_backfill_days: int
    weekend_policy: str
    created_at: datetime
    updated_at: datetime


class PricingPolicyUpdateInput(BaseModel):
    model_config = ConfigDict(frozen=True)

    book_id: int
    base_currency_id: int
    default_source_id: int | None = None
    refresh_enabled: bool
    refresh_hour_utc: int
    refresh_minute_utc: int
    max_backfill_days: int
    weekend_policy: str


class PricingSourceAssignmentSummary(BaseModel):
    model_config = ConfigDict(frozen=True)

    id: int
    book_id: int
    from_currency_id: int
    from_currency_symbol: str | None
    to_currency_id: int
    to_currency_symbol: str | None
    source_id: int
    source_name: str | None
    effective_from: date
    effective_to: date | None
    created_at: datetime
    updated_at: datetime


class PricingSourceAssignmentCreateInput(BaseModel):
    model_config = ConfigDict(frozen=True)

    book_id: int
    from_currency_id: int
    to_currency_id: int
    source_id: int
    effective_from: date
    effective_to: date | None = None


class PricingSourceAssignmentUpdateInput(BaseModel):
    model_config = ConfigDict(frozen=True)

    book_id: int
    from_currency_id: int
    to_currency_id: int
    source_id: int
    effective_from: date
    effective_to: date | None = None


class PricingRefreshStateSummary(BaseModel):
    model_config = ConfigDict(frozen=True)

    id: int
    book_id: int
    from_currency_id: int
    from_currency_symbol: str | None
    to_currency_id: int
    to_currency_symbol: str | None
    source_id: int
    source_name: str | None
    last_success_date: date | None
    last_attempt_at: datetime | None
    last_error: str | None
    created_at: datetime
    updated_at: datetime