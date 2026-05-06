from fastapi import APIRouter, Depends, HTTPException, Query, status

from rekenraam_api.api.dependencies import (
    get_admin_service,
    get_auth_service,
    get_ergonomics_service,
    require_admin_user,
    require_request_context,
)
from rekenraam_api.schemas.admin import (
    AdminRuntimeStatusSummary,
    FiscalYearCloseInput,
    FiscalYearCloseResult,
    IntegrityCheckSummary,
)
from rekenraam_api.schemas.ergonomics import (
    AdminPasswordResetInput,
    AdminUserCreateInput,
    AdminUserSummary,
    AdminUserUpdateInput,
    AuditEventSummary,
    BookMembershipInput,
)
from rekenraam_api.services.admin import AdminService
from rekenraam_api.services.auth import AuthService
from rekenraam_api.services.ergonomics import ErgonomicsService
from rekenraam_api.services.request_context import RequestContext

router = APIRouter(prefix="/admin", tags=["admin"], dependencies=[Depends(require_admin_user)])


@router.get("/runtime", response_model=AdminRuntimeStatusSummary)
async def get_runtime_status(
    admin_service: AdminService = Depends(get_admin_service),
) -> AdminRuntimeStatusSummary:
    return await admin_service.get_runtime_status()


@router.post("/integrity-check", response_model=IntegrityCheckSummary)
async def run_integrity_check(
    admin_service: AdminService = Depends(get_admin_service),
) -> IntegrityCheckSummary:
    return await admin_service.run_integrity_check()


@router.post("/fiscal-year-close", response_model=FiscalYearCloseResult)
async def close_fiscal_year(
    input: FiscalYearCloseInput,
    admin_service: AdminService = Depends(get_admin_service),
) -> FiscalYearCloseResult:
    try:
        return await admin_service.close_fiscal_year(input)
    except ValueError as error:
        detail = str(error)
        status_code = status.HTTP_400_BAD_REQUEST
        if detail == "fiscal year already closed for close_date":
            status_code = status.HTTP_409_CONFLICT
        raise HTTPException(status_code=status_code, detail=detail) from error


@router.get("/users", response_model=list[AdminUserSummary])
async def list_users(
    ergonomics_service: ErgonomicsService = Depends(get_ergonomics_service),
) -> list[AdminUserSummary]:
    return await ergonomics_service.list_admin_users()


@router.post("/users", response_model=AdminUserSummary)
async def create_user(
    input: AdminUserCreateInput,
    ergonomics_service: ErgonomicsService = Depends(get_ergonomics_service),
) -> AdminUserSummary:
    try:
        return await ergonomics_service.create_admin_user(input)
    except ValueError as error:
        raise HTTPException(status_code=status.HTTP_400_BAD_REQUEST, detail=str(error)) from error


@router.put("/users/{user_id}", response_model=AdminUserSummary)
async def update_user(
    user_id: int,
    input: AdminUserUpdateInput,
    ergonomics_service: ErgonomicsService = Depends(get_ergonomics_service),
) -> AdminUserSummary:
    try:
        return await ergonomics_service.update_admin_user(user_id, input)
    except ValueError as error:
        status_code = (
            status.HTTP_404_NOT_FOUND
            if str(error) == "user not found"
            else status.HTTP_400_BAD_REQUEST
        )
        raise HTTPException(status_code=status_code, detail=str(error)) from error


@router.post("/users/{user_id}/password", response_model=AdminUserSummary)
async def reset_user_password(
    user_id: int,
    input: AdminPasswordResetInput,
    ergonomics_service: ErgonomicsService = Depends(get_ergonomics_service),
) -> AdminUserSummary:
    try:
        return await ergonomics_service.reset_user_password(user_id, input.password)
    except ValueError as error:
        status_code = (
            status.HTTP_404_NOT_FOUND
            if str(error) == "user not found"
            else status.HTTP_400_BAD_REQUEST
        )
        raise HTTPException(status_code=status_code, detail=str(error)) from error


@router.post("/users/{user_id}/sessions/revoke", response_model=int)
async def revoke_user_sessions(
    user_id: int,
    ergonomics_service: ErgonomicsService = Depends(get_ergonomics_service),
) -> int:
    try:
        return await ergonomics_service.revoke_user_sessions(user_id)
    except ValueError as error:
        raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail=str(error)) from error


@router.delete("/users/{user_id}/mfa", status_code=status.HTTP_204_NO_CONTENT)
async def reset_user_mfa(
    user_id: int,
    context: RequestContext = Depends(require_request_context),
    auth_service: AuthService = Depends(get_auth_service),
) -> None:
    await auth_service.admin_reset_mfa(admin_context=context, user_id=user_id)


@router.put("/users/{user_id}/memberships", response_model=AdminUserSummary)
async def set_user_membership(
    user_id: int,
    input: BookMembershipInput,
    ergonomics_service: ErgonomicsService = Depends(get_ergonomics_service),
) -> AdminUserSummary:
    try:
        return await ergonomics_service.set_user_membership(user_id, input)
    except ValueError as error:
        status_code = (
            status.HTTP_404_NOT_FOUND
            if str(error) in {"user not found", "book not found"}
            else status.HTTP_400_BAD_REQUEST
        )
        raise HTTPException(status_code=status_code, detail=str(error)) from error


@router.delete("/users/{user_id}/memberships/{book_id}", response_model=AdminUserSummary)
async def delete_user_membership(
    user_id: int,
    book_id: int,
    ergonomics_service: ErgonomicsService = Depends(get_ergonomics_service),
) -> AdminUserSummary:
    try:
        return await ergonomics_service.delete_user_membership(user_id, book_id)
    except ValueError as error:
        raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail=str(error)) from error


@router.get("/audit-events", response_model=list[AuditEventSummary])
async def list_audit_events(
    book_id: int | None = None,
    limit: int = Query(default=100, ge=1, le=500),
    ergonomics_service: ErgonomicsService = Depends(get_ergonomics_service),
) -> list[AuditEventSummary]:
    return await ergonomics_service.list_audit_events(book_id=book_id, limit=limit)
