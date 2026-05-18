from datetime import date, datetime

from pydantic import BaseModel, ConfigDict


class PricingObservationAuditMixin(BaseModel):
    model_config = ConfigDict(frozen=True)

    mode: str
    is_derived: bool = False
    derived_via_commodity_id: int | None = None
    triangulation_path_commodity_ids: list[int] | None = None
    supersedes_observation_id: int | None = None
    ingest_run_id: int | None = None


class FxRateDailySummary(PricingObservationAuditMixin):
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
    is_manual: bool = False
    created_at: datetime


class FxRateDailyCreateInput(BaseModel):
    model_config = ConfigDict(frozen=True)

    book_id: int
    from_currency_id: int
    to_currency_id: int
    rate_date: date
    rate: float
    source: str | None = None
    source_id: int | None = None
    supersedes_observation_id: int | None = None


class FxRateOfficialSummary(PricingObservationAuditMixin):
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
    source_id: int | None = None
    is_manual: bool = False
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
    source_id: int | None = None
    source_url: str | None = None
    source_date: date | None = None
    notes: str | None = None
    supersedes_observation_id: int | None = None


class MarketPriceSummary(PricingObservationAuditMixin):
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
    source_id: int | None = None
    is_manual: bool = False
    created_at: datetime


class MarketPriceCreateInput(BaseModel):
    model_config = ConfigDict(frozen=True)

    book_id: int = 1
    commodity_id: int
    quote_commodity_id: int
    price_date: date
    price_minor: int
    source: str | None = None
    source_id: int | None = None
    supersedes_observation_id: int | None = None


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
    provider_key: str | None = None
    plugin_id: str | None = None
    country_code: str | None = None
    website_url: str | None = None
    notes: str | None = None
    created_at: datetime


class CommodityPriceSourceSummary(BaseModel):
    model_config = ConfigDict(frozen=True)

    id: int
    previous_commodity_price_source_id: int | None
    book_id: int
    commodity_id: int
    commodity_symbol: str | None
    commodity_name: str
    source_id: int
    source_name: str | None
    symbol: str
    provider_instrument_id: str | None
    exchange_code: str | None
    mic: str | None
    name_override: str | None
    is_primary: bool
    metadata_json: str | None
    effective_from: date | None
    effective_to: date | None
    created_at: datetime


class CommodityPriceSourceCreateInput(BaseModel):
    model_config = ConfigDict(frozen=True)

    book_id: int = 1
    commodity_id: int
    source_id: int
    symbol: str
    provider_instrument_id: str | None = None
    exchange_code: str | None = None
    mic: str | None = None
    name_override: str | None = None
    is_primary: bool = False
    metadata_json: str | None = None
    effective_from: date | None = None
    effective_to: date | None = None


class CommodityPriceSourceUpdateInput(BaseModel):
    model_config = ConfigDict(frozen=True)

    book_id: int = 1
    commodity_id: int
    source_id: int
    symbol: str
    provider_instrument_id: str | None = None
    exchange_code: str | None = None
    mic: str | None = None
    name_override: str | None = None
    is_primary: bool = False
    metadata_json: str | None = None
    effective_from: date | None = None
    effective_to: date | None = None


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
    staleness_max_days: int
    triangulation_max_hops: int
    rounding_mode: str
    prefer_official_fx: bool
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
    staleness_max_days: int
    triangulation_max_hops: int
    rounding_mode: str
    prefer_official_fx: bool
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

    id: int | None = None
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
