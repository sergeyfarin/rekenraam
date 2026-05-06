from datetime import date, datetime

from pydantic import BaseModel, ConfigDict


class PositionsQuery(BaseModel):
    model_config = ConfigDict(frozen=True)

    book_id: int
    as_of_date: date | None = None


class ConvertedPositionsQuery(BaseModel):
    model_config = ConfigDict(frozen=True)

    book_id: int
    base_commodity_id: int
    as_of_date: date | None = None


class LotsHoldingQuery(BaseModel):
    model_config = ConfigDict(frozen=True)

    book_id: int
    account_id: int | None = None
    commodity_id: int | None = None
    as_of_date: date | None = None


class RealizedGainsQuery(BaseModel):
    model_config = ConfigDict(frozen=True)

    book_id: int
    date_from: date | None = None
    date_to: date | None = None
    cost_basis_profile_id: int | None = None


class UnrealizedGainsQuery(BaseModel):
    model_config = ConfigDict(frozen=True)

    book_id: int
    base_commodity_id: int
    as_of_date: date | None = None
    cost_basis_profile_id: int | None = None


class InvestmentInstrumentCreateInput(BaseModel):
    model_config = ConfigDict(frozen=True)

    book_id: int = 1
    commodity_id: int
    instrument_type: str
    display_name: str
    symbol: str | None = None
    isin: str | None = None
    cusip: str | None = None
    figi: str | None = None
    exchange: str | None = None
    venue: str | None = None
    issuer: str | None = None
    country_code: str | None = None
    quote_commodity_id: int | None = None
    trading_commodity_id: int | None = None
    quantity_scale: int = 4
    price_scale: int = 4
    is_active: bool = True
    metadata_json: str | None = None


class InvestmentInstrumentUpdateInput(InvestmentInstrumentCreateInput):
    model_config = ConfigDict(frozen=True)


class InvestmentInstrumentSummary(InvestmentInstrumentCreateInput):
    model_config = ConfigDict(frozen=True)

    id: int
    created_at: datetime
    updated_at: datetime


class CostBasisProfileCreateInput(BaseModel):
    model_config = ConfigDict(frozen=True)

    book_id: int = 1
    name: str
    method: str = "fifo"
    description: str | None = None
    is_default: bool = False
    metadata_json: str | None = None


class CostBasisProfileUpdateInput(CostBasisProfileCreateInput):
    model_config = ConfigDict(frozen=True)


class CostBasisProfileSummary(CostBasisProfileCreateInput):
    model_config = ConfigDict(frozen=True)

    id: int
    created_at: datetime
    updated_at: datetime


class CorporateActionCreateInput(BaseModel):
    model_config = ConfigDict(frozen=True)

    book_id: int = 1
    action_type: str
    old_instrument_id: int | None = None
    new_instrument_id: int | None = None
    effective_date: date
    ratio_numerator: int | None = None
    ratio_denominator: int | None = None
    cash_in_lieu_minor: int | None = None
    cash_commodity_id: int | None = None
    source_reference: str | None = None
    memo: str | None = None
    metadata_json: str | None = None


class CorporateActionSummary(CorporateActionCreateInput):
    model_config = ConfigDict(frozen=True)

    id: int
    generated_transaction_id: int | None = None
    created_at: datetime


class BuyCommodityInput(BaseModel):
    model_config = ConfigDict(frozen=True)

    book_id: int = 1
    txn_date: date
    commodity_id: int
    investment_account_id: int
    cash_account_id: int
    quantity_minor: int
    cash_amount_minor: int
    memo: str | None = None
    payee_id: int | None = None
    status: str | None = None


class SellLotAllocationInput(BaseModel):
    model_config = ConfigDict(frozen=True)

    lot_id: int
    quantity_minor: int


class SellCommodityInput(BaseModel):
    model_config = ConfigDict(frozen=True, populate_by_name=True)

    book_id: int = 1
    txn_date: date
    commodity_id: int
    investment_account_id: int
    cash_account_id: int
    quantity_minor: int
    cash_amount_minor: int
    lot_strategy: str = "default"
    lot_allocations: tuple[SellLotAllocationInput, ...] | None = None
    allow_short: bool = False
    memo: str | None = None
    payee_id: int | None = None
    status: str | None = None


class DividendInput(BaseModel):
    model_config = ConfigDict(frozen=True)

    book_id: int = 1
    txn_date: date
    cash_account_id: int
    income_account_id: int
    amount_minor: int
    memo: str | None = None
    payee_id: int | None = None
    status: str | None = None


class ReinvestedDividendInput(BaseModel):
    model_config = ConfigDict(frozen=True)

    book_id: int = 1
    txn_date: date
    commodity_id: int
    investment_account_id: int
    income_account_id: int
    quantity_minor: int
    amount_minor: int
    memo: str | None = None
    payee_id: int | None = None
    status: str | None = None


class InvestmentEventInput(BaseModel):
    model_config = ConfigDict(frozen=True)

    book_id: int = 1
    action_type: str
    instrument_id: int | None = None
    effective_date: date
    amount_minor: int | None = None
    quantity_minor: int | None = None
    cash_commodity_id: int | None = None
    memo: str | None = None
    metadata_json: str | None = None


class TradeAllocation(BaseModel):
    model_config = ConfigDict(frozen=True)

    lot_id: int
    quantity_minor: int


class TradeResult(BaseModel):
    model_config = ConfigDict(frozen=True)

    transaction_id: int
    allocations: tuple[TradeAllocation, ...]
    lot_id: int | None


class DividendResult(BaseModel):
    model_config = ConfigDict(frozen=True)

    transaction_id: int


class PortfolioPerformanceQuery(BaseModel):
    model_config = ConfigDict(frozen=True)

    book_id: int
    base_commodity_id: int
    as_of_date: date | None = None
    cost_basis_profile_id: int | None = None


class PortfolioPerformanceSummary(BaseModel):
    model_config = ConfigDict(frozen=True)

    book_id: int
    base_commodity_id: int
    as_of_date: date | None
    market_value_minor: int
    cost_basis_minor: int
    unrealized_gain_minor: int
    realized_gain_minor: int
    income_minor: int
    price_missing_count: int


class AccountValuationRow(BaseModel):
    model_config = ConfigDict(frozen=True)

    account_id: int
    account_name: str
    account_commodity_id: int
    market_value_minor: int
    cost_basis_minor: int
    unrealized_gain_minor: int
    price_missing: bool
    fx_missing: bool


class CurrencyExposureRow(BaseModel):
    model_config = ConfigDict(frozen=True)

    commodity_id: int
    commodity_name: str
    quantity_minor: int
    native_value_minor: int
    price_missing: bool


class PositionLot(BaseModel):
    model_config = ConfigDict(frozen=True)

    lot_id: int
    opened_date: date | None
    quantity_minor: int
    cost_basis_minor: int
    remaining_cost_basis_minor: int
    converted_value_minor: int | None = None
    converted_cost_basis_minor: int | None = None
    price_missing: bool | None = None


class Position(BaseModel):
    model_config = ConfigDict(frozen=True)

    account_id: int
    account_name: str
    account_type: str
    commodity_id: int
    commodity_name: str
    commodity_scale: int
    balance_minor: int
    lots: tuple[PositionLot, ...]


class ConvertedPosition(BaseModel):
    model_config = ConfigDict(frozen=True)

    account_id: int
    account_name: str
    account_type: str
    commodity_id: int
    commodity_name: str
    commodity_scale: int
    balance_minor: int
    value_minor: int
    price_missing: bool
    lots: tuple[PositionLot, ...]


class LotHoldingPeriod(BaseModel):
    model_config = ConfigDict(frozen=True)

    lot_id: int
    account_id: int
    account_name: str
    commodity_id: int
    commodity_name: str
    opened_date: date | None
    quantity_minor: int
    cost_basis_minor: int
    commodity_scale: int
    holding_days: int | None
    is_long_term: bool


class RealizedGainEntry(BaseModel):
    model_config = ConfigDict(frozen=True)

    tx_id: int
    txn_date: date
    commodity_id: int
    quantity_minor: int
    proceeds_minor: int
    quote_commodity_id: int | None
    cost_basis_minor: int
    gain_loss_minor: int
    proceeds_missing: bool


class UnrealizedGainEntry(BaseModel):
    model_config = ConfigDict(frozen=True)

    account_id: int
    account_name: str
    account_type: str
    commodity_id: int
    commodity_name: str
    value_minor: int
    cost_basis_minor: int
    unrealized_gain_minor: int
    price_missing: bool
