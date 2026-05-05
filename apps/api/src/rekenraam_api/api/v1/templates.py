from fastapi import APIRouter, Depends, HTTPException, Query, status

from rekenraam_api.api.dependencies import get_ergonomics_service, get_transaction_service
from rekenraam_api.schemas.ergonomics import (
    TransactionTemplateInput,
    TransactionTemplatePostInput,
    TransactionTemplateSummary,
)
from rekenraam_api.schemas.transactions import TransactionSummary
from rekenraam_api.services.access import AuthorizationError
from rekenraam_api.services.ergonomics import ErgonomicsService
from rekenraam_api.services.transactions import TransactionService

router = APIRouter(prefix="/templates", tags=["templates"])


@router.get("/transactions", response_model=list[TransactionTemplateSummary])
async def list_transaction_templates(
    book_id: int = Query(...),
    ergonomics_service: ErgonomicsService = Depends(get_ergonomics_service),
) -> list[TransactionTemplateSummary]:
    try:
        return await ergonomics_service.list_templates(book_id)
    except AuthorizationError as error:
        raise HTTPException(status_code=status.HTTP_403_FORBIDDEN, detail=str(error)) from error


@router.post("/transactions", response_model=TransactionTemplateSummary)
async def create_transaction_template(
    input: TransactionTemplateInput,
    ergonomics_service: ErgonomicsService = Depends(get_ergonomics_service),
) -> TransactionTemplateSummary:
    try:
        return await ergonomics_service.create_template(input)
    except (AuthorizationError, ValueError) as error:
        status_code = (
            status.HTTP_403_FORBIDDEN
            if isinstance(error, AuthorizationError)
            else status.HTTP_400_BAD_REQUEST
        )
        raise HTTPException(status_code=status_code, detail=str(error)) from error


@router.put("/transactions/{template_id}", response_model=TransactionTemplateSummary)
async def update_transaction_template(
    template_id: int,
    input: TransactionTemplateInput,
    ergonomics_service: ErgonomicsService = Depends(get_ergonomics_service),
) -> TransactionTemplateSummary:
    try:
        template = await ergonomics_service.update_template(template_id, input)
    except (AuthorizationError, ValueError) as error:
        status_code = (
            status.HTTP_403_FORBIDDEN
            if isinstance(error, AuthorizationError)
            else status.HTTP_400_BAD_REQUEST
        )
        raise HTTPException(status_code=status_code, detail=str(error)) from error
    if template is None:
        raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail="template not found")
    return template


@router.delete("/transactions/{template_id}", status_code=status.HTTP_204_NO_CONTENT)
async def delete_transaction_template(
    template_id: int,
    ergonomics_service: ErgonomicsService = Depends(get_ergonomics_service),
) -> None:
    try:
        deleted = await ergonomics_service.delete_template(template_id)
    except AuthorizationError as error:
        raise HTTPException(status_code=status.HTTP_403_FORBIDDEN, detail=str(error)) from error
    if not deleted:
        raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail="template not found")


@router.post("/transactions/{template_id}/post", response_model=TransactionSummary)
async def post_transaction_template(
    template_id: int,
    input: TransactionTemplatePostInput,
    ergonomics_service: ErgonomicsService = Depends(get_ergonomics_service),
    transaction_service: TransactionService = Depends(get_transaction_service),
) -> TransactionSummary:
    try:
        mutation = await ergonomics_service.template_to_transaction_input(
            template_id, input.txn_date
        )
        if mutation is None:
            raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail="template not found")
        return await transaction_service.create_transaction(mutation)
    except AuthorizationError as error:
        raise HTTPException(status_code=status.HTTP_403_FORBIDDEN, detail=str(error)) from error
    except ValueError as error:
        raise HTTPException(status_code=status.HTTP_400_BAD_REQUEST, detail=str(error)) from error
