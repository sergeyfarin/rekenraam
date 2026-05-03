from datetime import date

from fastapi import APIRouter, Depends, HTTPException, Query, status

from rekenraam_api.api.dependencies import get_investment_service, get_report_service
from rekenraam_api.schemas.investments import RealizedGainEntry, RealizedGainsQuery, UnrealizedGainEntry, UnrealizedGainsQuery
from rekenraam_api.schemas.reports import (
    CashflowReportInput,
    CashflowRow,
    CategorySpendReportInput,
    CategorySpendRow,
    PayeeTotalsReportInput,
    PayeeTotalRow,
)
from rekenraam_api.services.investments import InvestmentService
from rekenraam_api.services.reports import ReportService


router = APIRouter(prefix="/reports", tags=["reports"])


@router.post("/cashflow", response_model=list[CashflowRow])
async def report_cashflow(
    input: CashflowReportInput,
    report_service: ReportService = Depends(get_report_service),
) -> list[CashflowRow]:
    try:
        return await report_service.report_cashflow(input)
    except ValueError as error:
        raise HTTPException(status_code=status.HTTP_400_BAD_REQUEST, detail=str(error)) from error


@router.post("/category-spend", response_model=list[CategorySpendRow])
async def report_category_spend(
    input: CategorySpendReportInput,
    report_service: ReportService = Depends(get_report_service),
) -> list[CategorySpendRow]:
    return await report_service.report_category_spend(input)


@router.post("/payee-totals", response_model=list[PayeeTotalRow])
async def report_payee_totals(
    input: PayeeTotalsReportInput,
    report_service: ReportService = Depends(get_report_service),
) -> list[PayeeTotalRow]:
    return await report_service.report_payee_totals(input)


@router.get("/realized-gains", response_model=list[RealizedGainEntry])
async def report_realized_gains(
    book_id: int = Query(default=1),
    date_from: date | None = Query(default=None),
    date_to: date | None = Query(default=None),
    investment_service: InvestmentService = Depends(get_investment_service),
) -> list[RealizedGainEntry]:
    return await investment_service.report_realized_gains(
        RealizedGainsQuery(book_id=book_id, date_from=date_from, date_to=date_to)
    )


@router.get("/unrealized-gains", response_model=list[UnrealizedGainEntry])
async def report_unrealized_gains(
    book_id: int = Query(default=1),
    base_commodity_id: int = Query(...),
    as_of_date: date | None = Query(default=None),
    investment_service: InvestmentService = Depends(get_investment_service),
) -> list[UnrealizedGainEntry]:
    return await investment_service.report_unrealized_gains(
        UnrealizedGainsQuery(book_id=book_id, base_commodity_id=base_commodity_id, as_of_date=as_of_date)
    )