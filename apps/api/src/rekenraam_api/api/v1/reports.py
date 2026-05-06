from datetime import date

from fastapi import APIRouter, Depends, HTTPException, Query, status

from rekenraam_api.api.dependencies import get_investment_service, get_report_service
from rekenraam_api.schemas.investments import (
    AccountValuationRow,
    CorporateActionSummary,
    CurrencyExposureRow,
    PortfolioPerformanceQuery,
    PortfolioPerformanceSummary,
    PositionsQuery,
    RealizedGainEntry,
    RealizedGainsQuery,
    UnrealizedGainEntry,
    UnrealizedGainsQuery,
)
from rekenraam_api.schemas.reports import (
    CashflowReportInput,
    CashflowRow,
    AccountTrendReportInput,
    AccountTrendRow,
    CategorySpendReportInput,
    CategorySpendRow,
    NetWorthReportInput,
    NetWorthRow,
    PayeeTotalsReportInput,
    PayeeTotalRow,
    ReportDefinitionCreateInput,
    ReportDefinitionSummary,
    ReportDefinitionUpdateInput,
    ReportRunCreateInput,
    ReportRunSummary,
)
from rekenraam_api.services.investments import InvestmentService
from rekenraam_api.services.reports import ReportService


router = APIRouter(prefix="/reports", tags=["reports"])


@router.get("/definitions", response_model=list[ReportDefinitionSummary])
async def list_report_definitions(
    book_id: int = Query(default=1),
    report_service: ReportService = Depends(get_report_service),
) -> list[ReportDefinitionSummary]:
    return await report_service.list_report_definitions(book_id)


@router.get("/definitions/{definition_id}", response_model=ReportDefinitionSummary)
async def get_report_definition(
    definition_id: int,
    report_service: ReportService = Depends(get_report_service),
) -> ReportDefinitionSummary:
    item = await report_service.get_report_definition(definition_id)
    if item is None:
        raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail="report definition not found")
    return item


@router.post("/definitions", response_model=ReportDefinitionSummary)
async def create_report_definition(
    input: ReportDefinitionCreateInput,
    report_service: ReportService = Depends(get_report_service),
) -> ReportDefinitionSummary:
    try:
        return await report_service.create_report_definition(input)
    except ValueError as error:
        raise HTTPException(status_code=status.HTTP_400_BAD_REQUEST, detail=str(error)) from error


@router.put("/definitions/{definition_id}", response_model=ReportDefinitionSummary)
async def update_report_definition(
    definition_id: int,
    input: ReportDefinitionUpdateInput,
    report_service: ReportService = Depends(get_report_service),
) -> ReportDefinitionSummary:
    if definition_id != input.id:
        raise HTTPException(status_code=status.HTTP_400_BAD_REQUEST, detail="path id must match body id")
    try:
        item = await report_service.update_report_definition(input)
    except ValueError as error:
        raise HTTPException(status_code=status.HTTP_400_BAD_REQUEST, detail=str(error)) from error
    if item is None:
        raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail="report definition not found")
    return item


@router.delete("/definitions/{definition_id}", status_code=status.HTTP_204_NO_CONTENT)
async def delete_report_definition(
    definition_id: int,
    report_service: ReportService = Depends(get_report_service),
) -> None:
    deleted = await report_service.delete_report_definition(definition_id)
    if not deleted:
        raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail="report definition not found")


@router.get("/runs", response_model=list[ReportRunSummary])
async def list_report_runs(
    book_id: int = Query(default=1),
    definition_id: int | None = Query(default=None),
    report_service: ReportService = Depends(get_report_service),
) -> list[ReportRunSummary]:
    return await report_service.list_report_runs(book_id, definition_id)


@router.post("/runs", response_model=ReportRunSummary)
async def create_report_run(
    input: ReportRunCreateInput,
    report_service: ReportService = Depends(get_report_service),
) -> ReportRunSummary:
    try:
        return await report_service.create_report_run(input)
    except ValueError as error:
        detail = str(error)
        if detail == "report definition not found":
            raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail=detail) from error
        raise HTTPException(status_code=status.HTTP_400_BAD_REQUEST, detail=detail) from error


@router.post("/cashflow", response_model=list[CashflowRow])
async def report_cashflow(
    input: CashflowReportInput,
    report_service: ReportService = Depends(get_report_service),
) -> list[CashflowRow]:
    try:
        return await report_service.report_cashflow(input)
    except ValueError as error:
        raise HTTPException(status_code=status.HTTP_400_BAD_REQUEST, detail=str(error)) from error


@router.post("/net-worth", response_model=list[NetWorthRow])
async def report_net_worth(
    input: NetWorthReportInput,
    report_service: ReportService = Depends(get_report_service),
) -> list[NetWorthRow]:
    try:
        return await report_service.report_net_worth(input)
    except ValueError as error:
        raise HTTPException(status_code=status.HTTP_400_BAD_REQUEST, detail=str(error)) from error


@router.post("/account-trends", response_model=list[AccountTrendRow])
async def report_account_trends(
    input: AccountTrendReportInput,
    report_service: ReportService = Depends(get_report_service),
) -> list[AccountTrendRow]:
    try:
        return await report_service.report_account_trends(input)
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
    cost_basis_profile_id: int | None = Query(default=None),
    investment_service: InvestmentService = Depends(get_investment_service),
) -> list[RealizedGainEntry]:
    return await investment_service.report_realized_gains(
        RealizedGainsQuery(
            book_id=book_id,
            date_from=date_from,
            date_to=date_to,
            cost_basis_profile_id=cost_basis_profile_id,
        )
    )


@router.get("/unrealized-gains", response_model=list[UnrealizedGainEntry])
async def report_unrealized_gains(
    book_id: int = Query(default=1),
    base_commodity_id: int = Query(...),
    as_of_date: date | None = Query(default=None),
    cost_basis_profile_id: int | None = Query(default=None),
    investment_service: InvestmentService = Depends(get_investment_service),
) -> list[UnrealizedGainEntry]:
    return await investment_service.report_unrealized_gains(
        UnrealizedGainsQuery(
            book_id=book_id,
            base_commodity_id=base_commodity_id,
            as_of_date=as_of_date,
            cost_basis_profile_id=cost_basis_profile_id,
        )
    )


@router.get("/investment-performance", response_model=PortfolioPerformanceSummary)
async def report_investment_performance(
    book_id: int = Query(default=1),
    base_commodity_id: int = Query(...),
    as_of_date: date | None = Query(default=None),
    cost_basis_profile_id: int | None = Query(default=None),
    investment_service: InvestmentService = Depends(get_investment_service),
) -> PortfolioPerformanceSummary:
    return await investment_service.portfolio_performance(
        PortfolioPerformanceQuery(
            book_id=book_id,
            base_commodity_id=base_commodity_id,
            as_of_date=as_of_date,
            cost_basis_profile_id=cost_basis_profile_id,
        )
    )


@router.get("/account-valuation", response_model=list[AccountValuationRow])
async def report_account_valuation(
    book_id: int = Query(default=1),
    base_commodity_id: int = Query(...),
    as_of_date: date | None = Query(default=None),
    cost_basis_profile_id: int | None = Query(default=None),
    investment_service: InvestmentService = Depends(get_investment_service),
) -> list[AccountValuationRow]:
    return await investment_service.account_valuation(
        PortfolioPerformanceQuery(
            book_id=book_id,
            base_commodity_id=base_commodity_id,
            as_of_date=as_of_date,
            cost_basis_profile_id=cost_basis_profile_id,
        )
    )


@router.get("/currency-exposure", response_model=list[CurrencyExposureRow])
async def report_currency_exposure(
    book_id: int = Query(default=1),
    as_of_date: date | None = Query(default=None),
    investment_service: InvestmentService = Depends(get_investment_service),
) -> list[CurrencyExposureRow]:
    return await investment_service.currency_exposure(PositionsQuery(book_id=book_id, as_of_date=as_of_date))


@router.get("/corporate-actions", response_model=list[CorporateActionSummary])
async def report_corporate_actions(
    book_id: int = Query(default=1),
    investment_service: InvestmentService = Depends(get_investment_service),
) -> list[CorporateActionSummary]:
    return await investment_service.list_corporate_actions(book_id)
