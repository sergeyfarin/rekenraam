from rekenraam_api.repositories.investments import InvestmentRepository
from rekenraam_api.services.report_invalidation import bump_report_state
from rekenraam_api.schemas.investments import (
    BuyCommodityInput,
    ConvertedPosition,
    ConvertedPositionsQuery,
    DividendInput,
    DividendResult,
    LotHoldingPeriod,
    LotsHoldingQuery,
    Position,
    PositionsQuery,
    RealizedGainEntry,
    RealizedGainsQuery,
    SellCommodityInput,
    TradeResult,
    UnrealizedGainEntry,
    UnrealizedGainsQuery,
)


class InvestmentService:
    def __init__(self, repository: InvestmentRepository) -> None:
        self._repository = repository

    async def create_buy(self, input: BuyCommodityInput) -> TradeResult:
        self._validate_positive(input.quantity_minor, "quantity")
        self._validate_positive(input.cash_amount_minor, "cash amount")
        row = await self._repository.create_buy(
            book_id=input.book_id,
            txn_date=input.txn_date,
            commodity_id=input.commodity_id,
            investment_account_id=input.investment_account_id,
            cash_account_id=input.cash_account_id,
            quantity_minor=input.quantity_minor,
            cash_amount_minor=input.cash_amount_minor,
            memo=input.memo,
            payee_id=input.payee_id,
            status=input.status or "uncleared",
        )
        await bump_report_state(getattr(self._repository, "_session", None), input.book_id)
        return TradeResult.model_validate(row)

    async def create_sell(self, input: SellCommodityInput) -> TradeResult:
        self._validate_positive(input.quantity_minor, "quantity")
        self._validate_positive(input.cash_amount_minor, "cash amount")
        row = await self._repository.create_sell(
            book_id=input.book_id,
            txn_date=input.txn_date,
            commodity_id=input.commodity_id,
            investment_account_id=input.investment_account_id,
            cash_account_id=input.cash_account_id,
            quantity_minor=input.quantity_minor,
            cash_amount_minor=input.cash_amount_minor,
            lot_strategy=input.lot_strategy,
            lot_allocations=None if input.lot_allocations is None else tuple(allocation.model_dump() for allocation in input.lot_allocations),
            allow_short=input.allow_short,
            memo=input.memo,
            payee_id=input.payee_id,
            status=input.status or "uncleared",
        )
        await bump_report_state(getattr(self._repository, "_session", None), input.book_id)
        return TradeResult.model_validate(row)

    async def create_dividend(self, input: DividendInput) -> DividendResult:
        self._validate_positive(input.amount_minor, "dividend amount")
        row = await self._repository.create_dividend(
            book_id=input.book_id,
            txn_date=input.txn_date,
            cash_account_id=input.cash_account_id,
            income_account_id=input.income_account_id,
            amount_minor=input.amount_minor,
            memo=input.memo,
            payee_id=input.payee_id,
            status=input.status or "uncleared",
        )
        await bump_report_state(getattr(self._repository, "_session", None), input.book_id)
        return DividendResult.model_validate(row)

    async def list_positions(self, input: PositionsQuery) -> list[Position]:
        rows = await self._repository.list_positions(book_id=input.book_id, as_of_date=input.as_of_date)
        return [Position.model_validate(row) for row in rows]

    async def convert_positions(self, input: ConvertedPositionsQuery) -> list[ConvertedPosition]:
        rows = await self._repository.convert_positions(
            book_id=input.book_id,
            base_commodity_id=input.base_commodity_id,
            as_of_date=input.as_of_date,
        )
        return [ConvertedPosition.model_validate(row) for row in rows]

    async def list_lots_with_holding_period(self, input: LotsHoldingQuery) -> list[LotHoldingPeriod]:
        rows = await self._repository.list_lots_with_holding_period(
            book_id=input.book_id,
            account_id=input.account_id,
            commodity_id=input.commodity_id,
            as_of_date=input.as_of_date,
        )
        return [LotHoldingPeriod.model_validate(row) for row in rows]

    async def report_realized_gains(self, input: RealizedGainsQuery) -> list[RealizedGainEntry]:
        rows = await self._repository.report_realized_gains(
            book_id=input.book_id,
            date_from=input.date_from,
            date_to=input.date_to,
        )
        return [RealizedGainEntry.model_validate(row) for row in rows]

    async def report_unrealized_gains(self, input: UnrealizedGainsQuery) -> list[UnrealizedGainEntry]:
        rows = await self._repository.report_unrealized_gains(
            book_id=input.book_id,
            base_commodity_id=input.base_commodity_id,
            as_of_date=input.as_of_date,
        )
        return [UnrealizedGainEntry.model_validate(row) for row in rows]

    @staticmethod
    def _validate_positive(value: int, label: str) -> None:
        if value <= 0:
            raise ValueError(f"{label} must be positive")
