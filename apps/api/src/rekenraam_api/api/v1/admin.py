from fastapi import APIRouter, Depends, HTTPException, status

from rekenraam_api.api.dependencies import get_admin_service
from rekenraam_api.schemas.admin import (
    AdminRuntimeStatusSummary,
    FiscalYearCloseInput,
    FiscalYearCloseResult,
    IntegrityCheckSummary,
)
from rekenraam_api.services.admin import AdminService


router = APIRouter(prefix="/admin", tags=["admin"])


@router.get("/runtime", response_model=AdminRuntimeStatusSummary)
async def get_runtime_status(
    admin_service: AdminService = Depends(get_admin_service),
) -> AdminRuntimeStatusSummary:
    return await admin_service.get_runtime_status()


@router.post("/integrity-check", response_model=IntegrityCheckSummary)
async def run_integrity_check(
    admin_service: AdminService = Depends(get_admin_service),
) -> IntegrityCheckSummary:
    status_text = await admin_service.run_integrity_check()
    return IntegrityCheckSummary(status=status_text)


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