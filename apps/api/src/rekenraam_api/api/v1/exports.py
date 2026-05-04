from typing import Annotated

from fastapi import APIRouter, Depends, HTTPException, Query, Response, status

from rekenraam_api.api.dependencies import get_export_service
from rekenraam_api.schemas.exports import ReportCsvExportRequest
from rekenraam_api.services.exports import ExportService

router = APIRouter(prefix="/exports", tags=["exports"])
ExportServiceDep = Annotated[ExportService, Depends(get_export_service)]


def _csv_response(content: str, filename: str, media_type: str = "text/csv") -> Response:
    return Response(
        content=content,
        media_type=f"{media_type}; charset=utf-8",
        headers={"Content-Disposition": f'attachment; filename="{filename}"'},
    )


@router.get("/accounts.csv")
async def export_accounts_csv(
    export_service: ExportServiceDep, book_id: int = Query(default=1)
) -> Response:
    return _csv_response(await export_service.accounts_csv(book_id), "accounts.csv")


@router.get("/transactions.csv")
async def export_transactions_csv(
    export_service: ExportServiceDep, book_id: int = Query(default=1)
) -> Response:
    return _csv_response(await export_service.transactions_csv(book_id), "transactions.csv")


@router.get("/registers/{account_id}.qif")
async def export_register_qif(account_id: int, export_service: ExportServiceDep) -> Response:
    try:
        return _csv_response(
            await export_service.register_qif(account_id),
            f"register-{account_id}.qif",
            media_type="application/x-qif",
        )
    except ValueError as error:
        raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail=str(error)) from error


@router.post("/reports/{report_kind}.csv")
async def export_report_csv(
    report_kind: str,
    input: ReportCsvExportRequest,
    export_service: ExportServiceDep,
) -> Response:
    try:
        return _csv_response(
            await export_service.report_csv(report_kind, input), f"{report_kind}.csv"
        )
    except ValueError as error:
        raise HTTPException(status_code=status.HTTP_400_BAD_REQUEST, detail=str(error)) from error
