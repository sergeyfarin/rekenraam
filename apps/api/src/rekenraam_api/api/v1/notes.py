from fastapi import APIRouter, Depends, HTTPException, status

from rekenraam_api.api.dependencies import get_ergonomics_service
from rekenraam_api.schemas.ergonomics import MarkdownNoteInput, MarkdownNoteSummary
from rekenraam_api.services.access import AuthorizationError
from rekenraam_api.services.ergonomics import ErgonomicsService

router = APIRouter(prefix="/notes", tags=["notes"])


@router.get("/accounts/{account_id}", response_model=list[MarkdownNoteSummary])
async def list_account_notes(
    account_id: int,
    ergonomics_service: ErgonomicsService = Depends(get_ergonomics_service),
) -> list[MarkdownNoteSummary]:
    try:
        return await ergonomics_service.list_account_notes(account_id)
    except AuthorizationError as error:
        raise HTTPException(status_code=status.HTTP_403_FORBIDDEN, detail=str(error)) from error
    except ValueError as error:
        raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail=str(error)) from error


@router.post("/accounts/{account_id}", response_model=MarkdownNoteSummary)
async def create_account_note(
    account_id: int,
    input: MarkdownNoteInput,
    ergonomics_service: ErgonomicsService = Depends(get_ergonomics_service),
) -> MarkdownNoteSummary:
    try:
        return await ergonomics_service.create_account_note(account_id, input)
    except AuthorizationError as error:
        raise HTTPException(status_code=status.HTTP_403_FORBIDDEN, detail=str(error)) from error
    except ValueError as error:
        raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail=str(error)) from error


@router.get("/transactions/{transaction_id}", response_model=list[MarkdownNoteSummary])
async def list_transaction_notes(
    transaction_id: int,
    ergonomics_service: ErgonomicsService = Depends(get_ergonomics_service),
) -> list[MarkdownNoteSummary]:
    try:
        return await ergonomics_service.list_transaction_notes(transaction_id)
    except AuthorizationError as error:
        raise HTTPException(status_code=status.HTTP_403_FORBIDDEN, detail=str(error)) from error
    except ValueError as error:
        raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail=str(error)) from error


@router.post("/transactions/{transaction_id}", response_model=MarkdownNoteSummary)
async def create_transaction_note(
    transaction_id: int,
    input: MarkdownNoteInput,
    ergonomics_service: ErgonomicsService = Depends(get_ergonomics_service),
) -> MarkdownNoteSummary:
    try:
        return await ergonomics_service.create_transaction_note(transaction_id, input)
    except AuthorizationError as error:
        raise HTTPException(status_code=status.HTTP_403_FORBIDDEN, detail=str(error)) from error
    except ValueError as error:
        raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail=str(error)) from error


@router.put("/{note_id}", response_model=MarkdownNoteSummary)
async def update_note(
    note_id: int,
    input: MarkdownNoteInput,
    ergonomics_service: ErgonomicsService = Depends(get_ergonomics_service),
) -> MarkdownNoteSummary:
    try:
        note = await ergonomics_service.update_note(note_id, input)
    except AuthorizationError as error:
        raise HTTPException(status_code=status.HTTP_403_FORBIDDEN, detail=str(error)) from error
    if note is None:
        raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail="note not found")
    return note


@router.delete("/{note_id}", status_code=status.HTTP_204_NO_CONTENT)
async def delete_note(
    note_id: int,
    ergonomics_service: ErgonomicsService = Depends(get_ergonomics_service),
) -> None:
    try:
        deleted = await ergonomics_service.delete_note(note_id)
    except AuthorizationError as error:
        raise HTTPException(status_code=status.HTTP_403_FORBIDDEN, detail=str(error)) from error
    if not deleted:
        raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail="note not found")
