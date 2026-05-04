from typing import Annotated

from fastapi import APIRouter, Depends, HTTPException, Query, status

from rekenraam_api.api.dependencies import get_import_service
from rekenraam_api.schemas.imports import (
    ImportCommitRequest,
    ImportCommitResult,
    ImportDraft,
    ImportPreviewRequest,
    ImportPreviewResult,
    ImportRuleCreate,
    ImportRulesApplyRequest,
    ImportRuleSummary,
    ImportSessionSummary,
    ImportSessionTransactionSummary,
)
from rekenraam_api.services.imports import ImportService

router = APIRouter(prefix="/imports", tags=["imports"])
ImportServiceDep = Annotated[ImportService, Depends(get_import_service)]


@router.post("/preview", response_model=ImportPreviewResult)
async def preview_import(
    input: ImportPreviewRequest, import_service: ImportServiceDep
) -> ImportPreviewResult:
    try:
        return await import_service.preview(input)
    except ValueError as error:
        raise HTTPException(status_code=status.HTTP_400_BAD_REQUEST, detail=str(error)) from error


@router.post("/rules/apply", response_model=tuple[ImportDraft, ...])
async def apply_import_rules(
    input: ImportRulesApplyRequest, import_service: ImportServiceDep
) -> tuple[ImportDraft, ...]:
    return tuple(await import_service.apply_rules(input))


@router.get("/rules", response_model=list[ImportRuleSummary])
async def list_import_rules(
    import_service: ImportServiceDep, book_id: int = Query(default=1)
) -> list[ImportRuleSummary]:
    return await import_service.list_rules(book_id)


@router.post("/rules", response_model=ImportRuleSummary)
async def create_import_rule(
    input: ImportRuleCreate, import_service: ImportServiceDep
) -> ImportRuleSummary:
    try:
        return await import_service.create_rule(input)
    except ValueError as error:
        raise HTTPException(status_code=status.HTTP_400_BAD_REQUEST, detail=str(error)) from error


@router.delete("/rules/{rule_id}", status_code=status.HTTP_204_NO_CONTENT)
async def delete_import_rule(rule_id: int, import_service: ImportServiceDep) -> None:
    deleted = await import_service.delete_rule(rule_id)
    if not deleted:
        raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail="import rule not found")


@router.post("/commit", response_model=ImportCommitResult)
async def commit_import(
    input: ImportCommitRequest, import_service: ImportServiceDep
) -> ImportCommitResult:
    try:
        return await import_service.commit(input)
    except ValueError as error:
        message = str(error)
        status_code = (
            status.HTTP_409_CONFLICT
            if message.startswith("cannot post transaction dated")
            else status.HTTP_400_BAD_REQUEST
        )
        raise HTTPException(status_code=status_code, detail=message) from error


@router.get("/sessions", response_model=list[ImportSessionSummary])
async def list_import_sessions(
    import_service: ImportServiceDep, book_id: int = Query(default=1)
) -> list[ImportSessionSummary]:
    return await import_service.list_sessions(book_id)


@router.get("/sessions/{session_id}", response_model=ImportSessionSummary)
async def get_import_session(
    session_id: int, import_service: ImportServiceDep
) -> ImportSessionSummary:
    session = await import_service.get_session(session_id)
    if session is None:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND, detail="import session not found"
        )
    return session


@router.get(
    "/sessions/{session_id}/transactions", response_model=list[ImportSessionTransactionSummary]
)
async def list_import_session_transactions(
    session_id: int,
    import_service: ImportServiceDep,
) -> list[ImportSessionTransactionSummary]:
    rows = await import_service.list_session_transactions(session_id)
    if rows is None:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND, detail="import session not found"
        )
    return rows
