from datetime import date

from fastapi import APIRouter, Depends, Query

from rekenraam_api.api.dependencies import get_investment_service
from rekenraam_api.schemas.investments import (
    ConvertedPosition,
    ConvertedPositionsQuery,
    LotHoldingPeriod,
    LotsHoldingQuery,
    Position,
    PositionsQuery,
)
from rekenraam_api.services.investments import InvestmentService


router = APIRouter(prefix="/investments", tags=["investments"])


@router.get("/positions", response_model=list[Position])
async def list_positions(
    book_id: int = Query(default=1),
    as_of_date: date | None = Query(default=None),
    investment_service: InvestmentService = Depends(get_investment_service),
) -> list[Position]:
    return await investment_service.list_positions(PositionsQuery(book_id=book_id, as_of_date=as_of_date))


@router.get("/positions/converted", response_model=list[ConvertedPosition])
async def convert_positions(
    book_id: int = Query(default=1),
    base_commodity_id: int = Query(...),
    as_of_date: date | None = Query(default=None),
    investment_service: InvestmentService = Depends(get_investment_service),
) -> list[ConvertedPosition]:
    return await investment_service.convert_positions(
        ConvertedPositionsQuery(book_id=book_id, base_commodity_id=base_commodity_id, as_of_date=as_of_date)
    )


@router.get("/lots", response_model=list[LotHoldingPeriod])
async def list_lots_with_holding_period(
    book_id: int = Query(default=1),
    account_id: int | None = Query(default=None),
    commodity_id: int | None = Query(default=None),
    as_of_date: date | None = Query(default=None),
    investment_service: InvestmentService = Depends(get_investment_service),
) -> list[LotHoldingPeriod]:
    return await investment_service.list_lots_with_holding_period(
        LotsHoldingQuery(
            book_id=book_id,
            account_id=account_id,
            commodity_id=commodity_id,
            as_of_date=as_of_date,
        )
    )
