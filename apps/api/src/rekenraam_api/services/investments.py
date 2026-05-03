from rekenraam_api.repositories.investments import InvestmentRepository
from rekenraam_api.schemas.investments import (
    ConvertedPosition,
    ConvertedPositionsQuery,
    LotHoldingPeriod,
    LotsHoldingQuery,
    Position,
    PositionsQuery,
    RealizedGainEntry,
    RealizedGainsQuery,
    UnrealizedGainEntry,
    UnrealizedGainsQuery,
)


class InvestmentService:
    def __init__(self, repository: InvestmentRepository) -> None:
        self._repository = repository

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
