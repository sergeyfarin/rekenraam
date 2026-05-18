from fastapi import APIRouter, Depends, HTTPException, Query, status

from rekenraam_api.api.dependencies import get_pricing_service, get_pricing_worker
from rekenraam_api.schemas.pricing import (
    CommodityPriceSourceCreateInput,
    CommodityPriceSourceSummary,
    CommodityPriceSourceUpdateInput,
    FxRateDailyCreateInput,
    FxRateDailySummary,
    FxRateOfficialCreateInput,
    FxRateOfficialSummary,
    MarketPriceCreateInput,
    MarketPriceSummary,
    PriceSourceSummary,
    PricingExecutionStatusSummary,
    PricingPolicySummary,
    PricingPolicyUpdateInput,
    PricingRefreshRunInput,
    PricingRefreshRunSummary,
    PricingRefreshStateSummary,
    PricingSourceAssignmentCreateInput,
    PricingSourceAssignmentSummary,
    PricingSourceAssignmentUpdateInput,
    PricingSourceHealthSummary,
)
from rekenraam_api.services.pricing import PricingService
from rekenraam_api.workers.pricing import PricingRefreshWorker

router = APIRouter(prefix="/pricing", tags=["pricing"])


@router.get("/rates/daily", response_model=list[FxRateDailySummary])
async def list_fx_rates_daily(
    book_id: int = Query(default=1),
    limit: int = Query(default=100, ge=1, le=1000),
    pricing_service: PricingService = Depends(get_pricing_service),
) -> list[FxRateDailySummary]:
    return await pricing_service.list_fx_rates_daily(book_id, limit)


@router.post("/rates/daily", response_model=FxRateDailySummary)
async def create_fx_rate_daily(
    input: FxRateDailyCreateInput,
    pricing_service: PricingService = Depends(get_pricing_service),
) -> FxRateDailySummary:
    try:
        return await pricing_service.create_fx_rate_daily(input)
    except ValueError as error:
        raise HTTPException(status_code=status.HTTP_400_BAD_REQUEST, detail=str(error)) from error


@router.delete("/rates/daily/{observation_id}", status_code=status.HTTP_204_NO_CONTENT)
async def delete_fx_rate_daily(
    observation_id: int,
    pricing_service: PricingService = Depends(get_pricing_service),
) -> None:
    deleted = await pricing_service.delete_fx_rate_daily(observation_id)
    if not deleted:
        raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail="daily FX rate not found")


@router.get("/rates/official", response_model=list[FxRateOfficialSummary])
async def list_fx_rates_official(
    book_id: int = Query(default=1),
    limit: int = Query(default=100, ge=1, le=1000),
    pricing_service: PricingService = Depends(get_pricing_service),
) -> list[FxRateOfficialSummary]:
    return await pricing_service.list_fx_rates_official(book_id, limit)


@router.post("/rates/official", response_model=FxRateOfficialSummary)
async def create_fx_rate_official(
    input: FxRateOfficialCreateInput,
    pricing_service: PricingService = Depends(get_pricing_service),
) -> FxRateOfficialSummary:
    try:
        return await pricing_service.create_fx_rate_official(input)
    except ValueError as error:
        raise HTTPException(status_code=status.HTTP_400_BAD_REQUEST, detail=str(error)) from error


@router.delete("/rates/official/{observation_id}", status_code=status.HTTP_204_NO_CONTENT)
async def delete_fx_rate_official(
    observation_id: int,
    pricing_service: PricingService = Depends(get_pricing_service),
) -> None:
    deleted = await pricing_service.delete_fx_rate_official(observation_id)
    if not deleted:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND, detail="official FX rate not found"
        )


@router.get("/market-prices", response_model=list[MarketPriceSummary])
async def list_market_prices(
    book_id: int = Query(default=1),
    commodity_id: int | None = Query(default=None),
    quote_commodity_id: int | None = Query(default=None),
    limit: int = Query(default=100, ge=1, le=1000),
    pricing_service: PricingService = Depends(get_pricing_service),
) -> list[MarketPriceSummary]:
    return await pricing_service.list_market_prices(
        book_id=book_id,
        commodity_id=commodity_id,
        quote_commodity_id=quote_commodity_id,
        limit=limit,
    )


@router.post("/market-prices", response_model=MarketPriceSummary)
async def create_market_price(
    input: MarketPriceCreateInput,
    pricing_service: PricingService = Depends(get_pricing_service),
) -> MarketPriceSummary:
    try:
        return await pricing_service.create_market_price(input)
    except ValueError as error:
        raise HTTPException(status_code=status.HTTP_400_BAD_REQUEST, detail=str(error)) from error


@router.delete("/market-prices/{observation_id}", status_code=status.HTTP_204_NO_CONTENT)
async def delete_market_price(
    observation_id: int,
    pricing_service: PricingService = Depends(get_pricing_service),
) -> None:
    deleted = await pricing_service.delete_market_price(observation_id)
    if not deleted:
        raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail="market price not found")


@router.get("/sources", response_model=list[PriceSourceSummary])
async def list_price_sources(
    pricing_service: PricingService = Depends(get_pricing_service),
) -> list[PriceSourceSummary]:
    return await pricing_service.list_price_sources()


@router.get("/commodity-price-sources", response_model=list[CommodityPriceSourceSummary])
async def list_commodity_price_sources(
    book_id: int = Query(default=1),
    commodity_id: int | None = Query(default=None),
    source_id: int | None = Query(default=None),
    pricing_service: PricingService = Depends(get_pricing_service),
) -> list[CommodityPriceSourceSummary]:
    return await pricing_service.list_commodity_price_sources(
        book_id=book_id,
        commodity_id=commodity_id,
        source_id=source_id,
    )


@router.post("/commodity-price-sources", response_model=CommodityPriceSourceSummary)
async def create_commodity_price_source(
    input: CommodityPriceSourceCreateInput,
    pricing_service: PricingService = Depends(get_pricing_service),
) -> CommodityPriceSourceSummary:
    try:
        return await pricing_service.create_commodity_price_source(input)
    except ValueError as error:
        raise HTTPException(status_code=status.HTTP_400_BAD_REQUEST, detail=str(error)) from error


@router.put(
    "/commodity-price-sources/{commodity_price_source_id}",
    response_model=CommodityPriceSourceSummary,
)
async def update_commodity_price_source(
    commodity_price_source_id: int,
    input: CommodityPriceSourceUpdateInput,
    pricing_service: PricingService = Depends(get_pricing_service),
) -> CommodityPriceSourceSummary:
    try:
        row = await pricing_service.update_commodity_price_source(commodity_price_source_id, input)
    except ValueError as error:
        raise HTTPException(status_code=status.HTTP_400_BAD_REQUEST, detail=str(error)) from error
    if row is None:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND, detail="commodity price source not found"
        )
    return row


@router.delete(
    "/commodity-price-sources/{commodity_price_source_id}",
    status_code=status.HTTP_204_NO_CONTENT,
)
async def delete_commodity_price_source(
    commodity_price_source_id: int,
    pricing_service: PricingService = Depends(get_pricing_service),
) -> None:
    deleted = await pricing_service.delete_commodity_price_source(commodity_price_source_id)
    if not deleted:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND, detail="commodity price source not found"
        )


@router.get("/policy", response_model=PricingPolicySummary)
async def get_pricing_policy(
    book_id: int = Query(default=1),
    pricing_service: PricingService = Depends(get_pricing_service),
) -> PricingPolicySummary:
    policy = await pricing_service.get_pricing_policy(book_id)
    if policy is None:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND, detail="pricing policy not found"
        )
    return policy


@router.put("/policy", response_model=PricingPolicySummary)
async def update_pricing_policy(
    input: PricingPolicyUpdateInput,
    pricing_service: PricingService = Depends(get_pricing_service),
) -> PricingPolicySummary:
    try:
        policy = await pricing_service.update_pricing_policy(input)
    except ValueError as error:
        raise HTTPException(status_code=status.HTTP_400_BAD_REQUEST, detail=str(error)) from error
    if policy is None:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND, detail="pricing policy not found"
        )
    return policy


@router.get("/source-assignments", response_model=list[PricingSourceAssignmentSummary])
async def list_pricing_source_assignments(
    book_id: int = Query(default=1),
    pricing_service: PricingService = Depends(get_pricing_service),
) -> list[PricingSourceAssignmentSummary]:
    return await pricing_service.list_pricing_source_assignments(book_id)


@router.post("/source-assignments", response_model=PricingSourceAssignmentSummary)
async def create_pricing_source_assignment(
    input: PricingSourceAssignmentCreateInput,
    pricing_service: PricingService = Depends(get_pricing_service),
) -> PricingSourceAssignmentSummary:
    try:
        return await pricing_service.create_pricing_source_assignment(input)
    except ValueError as error:
        raise HTTPException(status_code=status.HTTP_400_BAD_REQUEST, detail=str(error)) from error


@router.put("/source-assignments/{assignment_id}", response_model=PricingSourceAssignmentSummary)
async def update_pricing_source_assignment(
    assignment_id: int,
    input: PricingSourceAssignmentUpdateInput,
    pricing_service: PricingService = Depends(get_pricing_service),
) -> PricingSourceAssignmentSummary:
    try:
        assignment = await pricing_service.update_pricing_source_assignment(assignment_id, input)
    except ValueError as error:
        raise HTTPException(status_code=status.HTTP_400_BAD_REQUEST, detail=str(error)) from error
    if assignment is None:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND, detail="pricing source assignment not found"
        )
    return assignment


@router.delete("/source-assignments/{assignment_id}", status_code=status.HTTP_204_NO_CONTENT)
async def delete_pricing_source_assignment(
    assignment_id: int,
    pricing_service: PricingService = Depends(get_pricing_service),
) -> None:
    deleted = await pricing_service.delete_pricing_source_assignment(assignment_id)
    if not deleted:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND, detail="pricing source assignment not found"
        )


@router.get("/refresh-state", response_model=list[PricingRefreshStateSummary])
async def list_pricing_refresh_state(
    book_id: int = Query(default=1),
    pricing_service: PricingService = Depends(get_pricing_service),
) -> list[PricingRefreshStateSummary]:
    return await pricing_service.list_pricing_refresh_state(book_id)


@router.get("/source-health", response_model=list[PricingSourceHealthSummary])
async def list_source_health(
    book_id: int = Query(default=1),
    pricing_service: PricingService = Depends(get_pricing_service),
) -> list[PricingSourceHealthSummary]:
    return await pricing_service.list_source_health(book_id)


@router.post("/refresh/run", response_model=PricingRefreshRunSummary)
async def run_pricing_refresh(
    input: PricingRefreshRunInput,
    pricing_worker: PricingRefreshWorker = Depends(get_pricing_worker),
) -> PricingRefreshRunSummary:
    try:
        summary = await pricing_worker.run_book(input.book_id, trigger="manual", force=True)
    except ValueError as error:
        detail = str(error)
        if detail == "pricing policy not found":
            raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail=detail) from error
        raise HTTPException(status_code=status.HTTP_400_BAD_REQUEST, detail=detail) from error
    if summary is None:
        raise HTTPException(
            status_code=status.HTTP_409_CONFLICT, detail="pricing refresh already running"
        )
    return summary


@router.get("/refresh/execution-status", response_model=PricingExecutionStatusSummary)
async def get_pricing_execution_status(
    book_id: int = Query(default=1),
    pricing_worker: PricingRefreshWorker = Depends(get_pricing_worker),
) -> PricingExecutionStatusSummary:
    try:
        return await pricing_worker.get_status(book_id)
    except ValueError as error:
        detail = str(error)
        if detail == "pricing policy not found":
            raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail=detail) from error
        raise HTTPException(status_code=status.HTTP_400_BAD_REQUEST, detail=detail) from error


@router.get("/refresh/history", response_model=list[PricingRefreshRunSummary])
async def list_pricing_refresh_history(
    book_id: int = Query(default=1),
    limit: int = Query(default=10, ge=1, le=100),
    pricing_worker: PricingRefreshWorker = Depends(get_pricing_worker),
) -> list[PricingRefreshRunSummary]:
    try:
        return await pricing_worker.get_run_history(book_id, limit)
    except ValueError as error:
        detail = str(error)
        if detail == "pricing policy not found":
            raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail=detail) from error
        raise HTTPException(status_code=status.HTTP_400_BAD_REQUEST, detail=detail) from error
