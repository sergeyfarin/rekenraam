from datetime import date

from fastapi import APIRouter, Depends, HTTPException, Query, status

from rekenraam_api.api.dependencies import get_investment_service
from rekenraam_api.schemas.investments import (
    AccountValuationRow,
    BuyCommodityInput,
    ConvertedPosition,
    ConvertedPositionsQuery,
    CorporateActionCreateInput,
    CorporateActionSummary,
    CostBasisProfileCreateInput,
    CostBasisProfileSummary,
    CostBasisProfileUpdateInput,
    CurrencyExposureRow,
    DividendIncomeCategoryCreateInput,
    DividendIncomeCategorySummary,
    DividendIncomeCategoryUpdateInput,
    DividendInput,
    DividendResult,
    InvestmentEventInput,
    InvestmentInstrumentCreateInput,
    InvestmentInstrumentSummary,
    InvestmentInstrumentUpdateInput,
    LotHoldingPeriod,
    LotsHoldingQuery,
    PortfolioPerformanceQuery,
    PortfolioPerformanceSummary,
    Position,
    PositionsQuery,
    ReinvestedDividendInput,
    SellCommodityInput,
    TradeResult,
)
from rekenraam_api.services.investments import InvestmentService

router = APIRouter(prefix="/investments", tags=["investments"])


@router.get("/instruments", response_model=list[InvestmentInstrumentSummary])
async def list_instruments(
    book_id: int = Query(default=1),
    investment_service: InvestmentService = Depends(get_investment_service),
) -> list[InvestmentInstrumentSummary]:
    return await investment_service.list_instruments(book_id)


@router.post("/instruments", response_model=InvestmentInstrumentSummary)
async def create_instrument(
    input: InvestmentInstrumentCreateInput,
    investment_service: InvestmentService = Depends(get_investment_service),
) -> InvestmentInstrumentSummary:
    try:
        return await investment_service.create_instrument(input)
    except ValueError as error:
        raise HTTPException(status_code=status.HTTP_400_BAD_REQUEST, detail=str(error)) from error


@router.put("/instruments/{instrument_id}", response_model=InvestmentInstrumentSummary)
async def update_instrument(
    instrument_id: int,
    input: InvestmentInstrumentUpdateInput,
    investment_service: InvestmentService = Depends(get_investment_service),
) -> InvestmentInstrumentSummary:
    try:
        row = await investment_service.update_instrument(instrument_id, input)
    except ValueError as error:
        raise HTTPException(status_code=status.HTTP_400_BAD_REQUEST, detail=str(error)) from error
    if row is None:
        raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail="instrument not found")
    return row


@router.get("/cost-basis-profiles", response_model=list[CostBasisProfileSummary])
async def list_cost_basis_profiles(
    book_id: int = Query(default=1),
    investment_service: InvestmentService = Depends(get_investment_service),
) -> list[CostBasisProfileSummary]:
    return await investment_service.list_cost_basis_profiles(book_id)


@router.post("/cost-basis-profiles", response_model=CostBasisProfileSummary)
async def create_cost_basis_profile(
    input: CostBasisProfileCreateInput,
    investment_service: InvestmentService = Depends(get_investment_service),
) -> CostBasisProfileSummary:
    try:
        return await investment_service.create_cost_basis_profile(input)
    except ValueError as error:
        raise HTTPException(status_code=status.HTTP_400_BAD_REQUEST, detail=str(error)) from error


@router.put("/cost-basis-profiles/{profile_id}", response_model=CostBasisProfileSummary)
async def update_cost_basis_profile(
    profile_id: int,
    input: CostBasisProfileUpdateInput,
    investment_service: InvestmentService = Depends(get_investment_service),
) -> CostBasisProfileSummary:
    try:
        row = await investment_service.update_cost_basis_profile(profile_id, input)
    except ValueError as error:
        raise HTTPException(status_code=status.HTTP_400_BAD_REQUEST, detail=str(error)) from error
    if row is None:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND, detail="cost basis profile not found"
        )
    return row


@router.get("/dividend-income-categories", response_model=list[DividendIncomeCategorySummary])
async def list_dividend_income_categories(
    book_id: int = Query(default=1),
    commodity_id: int | None = Query(default=None),
    investment_service: InvestmentService = Depends(get_investment_service),
) -> list[DividendIncomeCategorySummary]:
    return await investment_service.list_dividend_income_categories(book_id, commodity_id)


@router.post("/dividend-income-categories", response_model=DividendIncomeCategorySummary)
async def create_dividend_income_category(
    input: DividendIncomeCategoryCreateInput,
    investment_service: InvestmentService = Depends(get_investment_service),
) -> DividendIncomeCategorySummary:
    try:
        return await investment_service.create_dividend_income_category(input)
    except ValueError as error:
        raise HTTPException(status_code=status.HTTP_400_BAD_REQUEST, detail=str(error)) from error


@router.put(
    "/dividend-income-categories/{dividend_income_category_id}",
    response_model=DividendIncomeCategorySummary,
)
async def update_dividend_income_category(
    dividend_income_category_id: int,
    input: DividendIncomeCategoryUpdateInput,
    investment_service: InvestmentService = Depends(get_investment_service),
) -> DividendIncomeCategorySummary:
    try:
        row = await investment_service.update_dividend_income_category(
            dividend_income_category_id, input
        )
    except ValueError as error:
        raise HTTPException(status_code=status.HTTP_400_BAD_REQUEST, detail=str(error)) from error
    if row is None:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail="dividend income category not found",
        )
    return row


@router.delete(
    "/dividend-income-categories/{dividend_income_category_id}",
    status_code=status.HTTP_204_NO_CONTENT,
)
async def delete_dividend_income_category(
    dividend_income_category_id: int,
    investment_service: InvestmentService = Depends(get_investment_service),
) -> None:
    deleted = await investment_service.delete_dividend_income_category(dividend_income_category_id)
    if not deleted:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail="dividend income category not found",
        )


@router.get("/corporate-actions", response_model=list[CorporateActionSummary])
async def list_corporate_actions(
    book_id: int = Query(default=1),
    investment_service: InvestmentService = Depends(get_investment_service),
) -> list[CorporateActionSummary]:
    return await investment_service.list_corporate_actions(book_id)


@router.post("/corporate-actions", response_model=CorporateActionSummary)
async def create_corporate_action(
    input: CorporateActionCreateInput,
    investment_service: InvestmentService = Depends(get_investment_service),
) -> CorporateActionSummary:
    try:
        return await investment_service.create_corporate_action(input)
    except ValueError as error:
        raise HTTPException(status_code=status.HTTP_400_BAD_REQUEST, detail=str(error)) from error


@router.get("/positions", response_model=list[Position])
async def list_positions(
    book_id: int = Query(default=1),
    as_of_date: date | None = Query(default=None),
    investment_service: InvestmentService = Depends(get_investment_service),
) -> list[Position]:
    return await investment_service.list_positions(
        PositionsQuery(book_id=book_id, as_of_date=as_of_date)
    )


@router.get("/positions/converted", response_model=list[ConvertedPosition])
async def convert_positions(
    book_id: int = Query(default=1),
    base_commodity_id: int = Query(...),
    as_of_date: date | None = Query(default=None),
    investment_service: InvestmentService = Depends(get_investment_service),
) -> list[ConvertedPosition]:
    return await investment_service.convert_positions(
        ConvertedPositionsQuery(
            book_id=book_id, base_commodity_id=base_commodity_id, as_of_date=as_of_date
        )
    )


@router.get("/performance", response_model=PortfolioPerformanceSummary)
async def portfolio_performance(
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
async def account_valuation(
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
async def currency_exposure(
    book_id: int = Query(default=1),
    as_of_date: date | None = Query(default=None),
    investment_service: InvestmentService = Depends(get_investment_service),
) -> list[CurrencyExposureRow]:
    return await investment_service.currency_exposure(
        PositionsQuery(book_id=book_id, as_of_date=as_of_date)
    )


@router.get("/lots", response_model=list[LotHoldingPeriod])
async def list_lots_with_holding_period(
    book_id: int = Query(default=1),
    account_id: int | None = Query(default=None),
    commodity_id: int | None = Query(default=None),
    as_of_date: date | None = Query(default=None),
    investment_service: InvestmentService = Depends(get_investment_service),
) -> list[LotHoldingPeriod]:
    return await investment_service.list_lots_with_holding_period(
        LotsHoldingQuery(
            book_id=book_id,
            account_id=account_id,
            commodity_id=commodity_id,
            as_of_date=as_of_date,
        )
    )


@router.post("/buy", response_model=TradeResult)
async def create_buy(
    input: BuyCommodityInput,
    investment_service: InvestmentService = Depends(get_investment_service),
) -> TradeResult:
    try:
        return await investment_service.create_buy(input)
    except ValueError as error:
        raise HTTPException(status_code=status.HTTP_400_BAD_REQUEST, detail=str(error)) from error


@router.post("/sell", response_model=TradeResult)
async def create_sell(
    input: SellCommodityInput,
    investment_service: InvestmentService = Depends(get_investment_service),
) -> TradeResult:
    try:
        return await investment_service.create_sell(input)
    except ValueError as error:
        raise HTTPException(status_code=status.HTTP_400_BAD_REQUEST, detail=str(error)) from error


@router.post("/dividend", response_model=DividendResult)
async def create_dividend(
    input: DividendInput,
    investment_service: InvestmentService = Depends(get_investment_service),
) -> DividendResult:
    try:
        return await investment_service.create_dividend(input)
    except ValueError as error:
        raise HTTPException(status_code=status.HTTP_400_BAD_REQUEST, detail=str(error)) from error


@router.post("/reinvested-dividend", response_model=TradeResult)
async def create_reinvested_dividend(
    input: ReinvestedDividendInput,
    investment_service: InvestmentService = Depends(get_investment_service),
) -> TradeResult:
    try:
        return await investment_service.create_reinvested_dividend(input)
    except ValueError as error:
        raise HTTPException(status_code=status.HTTP_400_BAD_REQUEST, detail=str(error)) from error


@router.post("/short-sell", response_model=TradeResult)
async def create_short_sell(
    input: SellCommodityInput,
    investment_service: InvestmentService = Depends(get_investment_service),
) -> TradeResult:
    try:
        short_input = input.model_copy(update={"allow_short": True})
        return await investment_service.create_sell(short_input)
    except ValueError as error:
        raise HTTPException(status_code=status.HTTP_400_BAD_REQUEST, detail=str(error)) from error


@router.post("/short-cover", response_model=TradeResult)
async def create_short_cover(
    input: BuyCommodityInput,
    investment_service: InvestmentService = Depends(get_investment_service),
) -> TradeResult:
    try:
        return await investment_service.create_buy(input)
    except ValueError as error:
        raise HTTPException(status_code=status.HTTP_400_BAD_REQUEST, detail=str(error)) from error


@router.post("/events", response_model=CorporateActionSummary)
async def record_investment_event(
    input: InvestmentEventInput,
    investment_service: InvestmentService = Depends(get_investment_service),
) -> CorporateActionSummary:
    try:
        return await investment_service.record_investment_event(input)
    except ValueError as error:
        raise HTTPException(status_code=status.HTTP_400_BAD_REQUEST, detail=str(error)) from error
