from datetime import date, datetime

from pydantic import BaseModel, ConfigDict


class FxRateDailySummary(BaseModel):
    model_config = ConfigDict(frozen=True)

    id: int
    book_id: int
    from_currency_id: int
    from_currency_symbol: str | None
    to_currency_id: int
    to_currency_symbol: str | None
    rate_date: date
    rate: float
    source: str | None
    source_id: int | None = None
    is_derived: bool = False
    derived_via_currency_id: int | None = None
    created_at: datetime


class FxRateDailyCreateInput(BaseModel):
    model_config = ConfigDict(frozen=True)

    book_id: int
    from_currency_id: int
    to_currency_id: int
    rate_date: date
    rate: float
    source: str | None = None


class FxRateOfficialSummary(BaseModel):
    model_config = ConfigDict(frozen=True)

    id: int
    book_id: int
    from_currency_id: int
    from_currency_symbol: str | None
    to_currency_id: int
    to_currency_symbol: str | None
    period_type: str
    period_year: int
    period_month: int | None
    rate: float
    source_name: str
    source_url: str | None = None
    source_date: date | None = None
    notes: str | None = None
    created_at: datetime
    updated_at: datetime


class FxRateOfficialCreateInput(BaseModel):
    model_config = ConfigDict(frozen=True)

    book_id: int
    from_currency_id: int
    to_currency_id: int
    period_type: str
    period_year: int
    period_month: int | None = None
    rate: float
    source_name: str
    source_url: str | None = None
    source_date: date | None = None
    notes: str | None = None


class MarketPriceSummary(BaseModel):
    model_config = ConfigDict(frozen=True)

    id: int
    book_id: int
    commodity_id: int
    commodity_symbol: str | None
    commodity_name: str
    quote_commodity_id: int
    quote_commodity_symbol: str | None
    price_date: date
    price_minor: int
    source: str | None
    created_at: datetime


class MarketPriceCreateInput(BaseModel):
    model_config = ConfigDict(frozen=True)

    book_id: int = 1
    commodity_id: int
    quote_commodity_id: int
    price_date: date
    price_minor: int
    source: str | None = None


class PricingSourceHealthSummary(BaseModel):
    model_config = ConfigDict(frozen=True)

    book_id: int
    commodity_id: int
    commodity_symbol: str | None
    quote_commodity_id: int
    quote_commodity_symbol: str | None
    source_id: int
    source_name: str | None
    status: str
    last_success_date: date | None
    last_attempt_at: datetime | None
    last_error: str | None


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


class PricingRefreshRunSummary(BaseModel):
    model_config = ConfigDict(frozen=True)

    book_id: int
    trigger: str
    started_at: datetime
    finished_at: datetime
    pairs_total: int
    pairs_success: int
    pairs_failed: int
    rates_inserted: int
    derived_inserted: int
    last_error: str | None


class PricingExecutionStatusSummary(BaseModel):
    model_config = ConfigDict(frozen=True)

    book_id: int
    scheduler_enabled: bool
    scheduler_poll_seconds: int
    worker_started_at: datetime | None
    is_running: bool
    active_book_ids: list[int]
    next_scheduled_at: datetime | None
    last_run: PricingRefreshRunSummary | None


class PricingRefreshRunInput(BaseModel):
    model_config = ConfigDict(frozen=True)

    book_id: int = 1
