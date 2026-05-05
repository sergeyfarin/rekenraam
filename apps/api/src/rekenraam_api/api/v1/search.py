from fastapi import APIRouter, Depends, HTTPException, Query, status

from rekenraam_api.api.dependencies import get_ergonomics_service
from rekenraam_api.schemas.ergonomics import TransactionSavedViewInput, TransactionSavedViewSummary
from rekenraam_api.services.access import AuthorizationError
from rekenraam_api.services.ergonomics import ErgonomicsService

router = APIRouter(prefix="/search", tags=["search"])


@router.get("/transaction-views", response_model=list[TransactionSavedViewSummary])
async def list_transaction_views(
    book_id: int = Query(...),
    ergonomics_service: ErgonomicsService = Depends(get_ergonomics_service),
) -> list[TransactionSavedViewSummary]:
    try:
        return await ergonomics_service.list_saved_views(book_id)
    except AuthorizationError as error:
        raise HTTPException(status_code=status.HTTP_403_FORBIDDEN, detail=str(error)) from error


@router.post("/transaction-views", response_model=TransactionSavedViewSummary)
async def create_transaction_view(
    input: TransactionSavedViewInput,
    ergonomics_service: ErgonomicsService = Depends(get_ergonomics_service),
) -> TransactionSavedViewSummary:
    try:
        return await ergonomics_service.create_saved_view(input)
    except AuthorizationError as error:
        raise HTTPException(status_code=status.HTTP_403_FORBIDDEN, detail=str(error)) from error


@router.put("/transaction-views/{view_id}", response_model=TransactionSavedViewSummary)
async def update_transaction_view(
    view_id: int,
    input: TransactionSavedViewInput,
    ergonomics_service: ErgonomicsService = Depends(get_ergonomics_service),
) -> TransactionSavedViewSummary:
    try:
        view = await ergonomics_service.update_saved_view(view_id, input)
    except AuthorizationError as error:
        raise HTTPException(status_code=status.HTTP_403_FORBIDDEN, detail=str(error)) from error
    if view is None:
        raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail="saved view not found")
    return view


@router.delete("/transaction-views/{view_id}", status_code=status.HTTP_204_NO_CONTENT)
async def delete_transaction_view(
    view_id: int,
    ergonomics_service: ErgonomicsService = Depends(get_ergonomics_service),
) -> None:
    try:
        deleted = await ergonomics_service.delete_saved_view(view_id)
    except AuthorizationError as error:
        raise HTTPException(status_code=status.HTTP_403_FORBIDDEN, detail=str(error)) from error
    if not deleted:
        raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail="saved view not found")
