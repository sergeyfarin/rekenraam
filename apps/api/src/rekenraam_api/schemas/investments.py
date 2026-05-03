from datetime import date

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


class UnrealizedGainsQuery(BaseModel):
    model_config = ConfigDict(frozen=True)

    book_id: int
    base_commodity_id: int
    as_of_date: date | None = None


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
