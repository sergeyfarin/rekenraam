from fastapi import APIRouter, Depends, HTTPException, Query, status

from rekenraam_api.api.dependencies import get_pricing_service, get_pricing_worker
from rekenraam_api.schemas.pricing import (
    PricingExecutionStatusSummary,
    PriceSourceSummary,
    PricingPolicySummary,
    PricingPolicyUpdateInput,
    PricingRefreshRunInput,
    PricingRefreshRunSummary,
    PricingRefreshStateSummary,
    PricingSourceAssignmentCreateInput,
    PricingSourceAssignmentSummary,
    PricingSourceAssignmentUpdateInput,
)
from rekenraam_api.services.pricing import PricingService
from rekenraam_api.workers.pricing import PricingRefreshWorker


router = APIRouter(prefix="/pricing", tags=["pricing"])


@router.get("/sources", response_model=list[PriceSourceSummary])
async def list_price_sources(
    pricing_service: PricingService = Depends(get_pricing_service),
) -> list[PriceSourceSummary]:
    return await pricing_service.list_price_sources()


@router.get("/policy", response_model=PricingPolicySummary)
async def get_pricing_policy(
    book_id: int = Query(default=1),
    pricing_service: PricingService = Depends(get_pricing_service),
) -> PricingPolicySummary:
    policy = await pricing_service.get_pricing_policy(book_id)
    if policy is None:
        raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail="pricing policy not found")
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
        raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail="pricing policy not found")
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
        raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail="pricing source assignment not found")
    return assignment


@router.delete("/source-assignments/{assignment_id}", status_code=status.HTTP_204_NO_CONTENT)
async def delete_pricing_source_assignment(
    assignment_id: int,
    pricing_service: PricingService = Depends(get_pricing_service),
) -> None:
    deleted = await pricing_service.delete_pricing_source_assignment(assignment_id)
    if not deleted:
        raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail="pricing source assignment not found")


@router.get("/refresh-state", response_model=list[PricingRefreshStateSummary])
async def list_pricing_refresh_state(
    book_id: int = Query(default=1),
    pricing_service: PricingService = Depends(get_pricing_service),
) -> list[PricingRefreshStateSummary]:
    return await pricing_service.list_pricing_refresh_state(book_id)


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
        raise HTTPException(status_code=status.HTTP_409_CONFLICT, detail="pricing refresh already running")
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