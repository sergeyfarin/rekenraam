# FastAPI dependency injection uses callable defaults throughout this codebase.

from datetime import date

from fastapi import APIRouter, Depends, HTTPException, Query, status

from rekenraam_api.api.dependencies import get_planning_service
from rekenraam_api.schemas.planning import (
    AmortizationRow,
    BudgetMutationInput,
    BudgetSummary,
    BudgetVarianceRow,
    LoanMutationInput,
    LoanPaymentDraft,
    LoanPaymentDraftInput,
    LoanSummary,
    PostedScheduleResult,
    ProjectedCashRow,
    ScheduleMutationInput,
    ScheduleOccurrenceSummary,
    ScheduleSummary,
)
from rekenraam_api.schemas.transactions import TransactionSummary
from rekenraam_api.services.planning import PlanningService

budgets_router = APIRouter(prefix="/budgets", tags=["budgets"])
schedules_router = APIRouter(prefix="/schedules", tags=["schedules"])
loans_router = APIRouter(prefix="/loans", tags=["loans"])


def _bad_request(error: ValueError) -> HTTPException:
    return HTTPException(status_code=status.HTTP_400_BAD_REQUEST, detail=str(error))


@budgets_router.get("", response_model=list[BudgetSummary])
async def list_budgets(
    book_id: int = Query(default=1),
    planning_service: PlanningService = Depends(get_planning_service),
) -> list[BudgetSummary]:
    return await planning_service.list_budgets(book_id)


@budgets_router.get("/{budget_id}", response_model=BudgetSummary)
async def get_budget(
    budget_id: int,
    planning_service: PlanningService = Depends(get_planning_service),
) -> BudgetSummary:
    budget = await planning_service.get_budget(budget_id)
    if budget is None:
        raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail="budget not found")
    return budget


@budgets_router.post("", response_model=BudgetSummary)
async def create_budget(
    input: BudgetMutationInput,
    planning_service: PlanningService = Depends(get_planning_service),
) -> BudgetSummary:
    try:
        return await planning_service.create_budget(input)
    except ValueError as error:
        raise _bad_request(error) from error


@budgets_router.put("/{budget_id}", response_model=BudgetSummary)
async def update_budget(
    budget_id: int,
    input: BudgetMutationInput,
    planning_service: PlanningService = Depends(get_planning_service),
) -> BudgetSummary:
    try:
        budget = await planning_service.update_budget(budget_id, input)
    except ValueError as error:
        raise _bad_request(error) from error
    if budget is None:
        raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail="budget not found")
    return budget


@budgets_router.delete("/{budget_id}", status_code=status.HTTP_204_NO_CONTENT)
async def delete_budget(
    budget_id: int,
    planning_service: PlanningService = Depends(get_planning_service),
) -> None:
    deleted = await planning_service.delete_budget(budget_id)
    if not deleted:
        raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail="budget not found")


@budgets_router.get("/{budget_id}/variance", response_model=list[BudgetVarianceRow])
async def budget_variance(
    budget_id: int,
    period_start: date = Query(...),
    planning_service: PlanningService = Depends(get_planning_service),
) -> list[BudgetVarianceRow]:
    try:
        return await planning_service.budget_variance(budget_id, period_start)
    except ValueError as error:
        detail = str(error)
        if detail == "budget not found":
            raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail=detail) from error
        raise _bad_request(error) from error


@schedules_router.get("", response_model=list[ScheduleSummary])
async def list_schedules(
    book_id: int = Query(default=1),
    planning_service: PlanningService = Depends(get_planning_service),
) -> list[ScheduleSummary]:
    return await planning_service.list_schedules(book_id)


@schedules_router.post("", response_model=ScheduleSummary)
async def create_schedule(
    input: ScheduleMutationInput,
    planning_service: PlanningService = Depends(get_planning_service),
) -> ScheduleSummary:
    try:
        return await planning_service.create_schedule(input)
    except ValueError as error:
        raise _bad_request(error) from error


@schedules_router.get("/instances", response_model=list[ScheduleOccurrenceSummary])
async def projected_instances(
    book_id: int = Query(default=1),
    start: date = Query(...),
    end: date = Query(...),
    planning_service: PlanningService = Depends(get_planning_service),
) -> list[ScheduleOccurrenceSummary]:
    return await planning_service.projected_instances(book_id, start, end)


@schedules_router.post("/{schedule_id}/skip", response_model=ScheduleOccurrenceSummary)
async def skip_schedule(
    schedule_id: int,
    occurrence_date: date = Query(...),
    planning_service: PlanningService = Depends(get_planning_service),
) -> ScheduleOccurrenceSummary:
    try:
        return await planning_service.skip_schedule(schedule_id, occurrence_date)
    except ValueError as error:
        raise _bad_request(error) from error


@schedules_router.post("/{schedule_id}/post", response_model=PostedScheduleResult)
async def post_schedule(
    schedule_id: int,
    occurrence_date: date = Query(...),
    planning_service: PlanningService = Depends(get_planning_service),
) -> PostedScheduleResult:
    try:
        return await planning_service.post_schedule(schedule_id, occurrence_date)
    except ValueError as error:
        raise _bad_request(error) from error


@schedules_router.get("/projected-cash", response_model=list[ProjectedCashRow])
async def projected_cash(
    book_id: int = Query(default=1),
    start: date = Query(...),
    end: date = Query(...),
    planning_service: PlanningService = Depends(get_planning_service),
) -> list[ProjectedCashRow]:
    return await planning_service.projected_cash(book_id, start, end)


@schedules_router.get("/{schedule_id}", response_model=ScheduleSummary)
async def get_schedule(
    schedule_id: int,
    planning_service: PlanningService = Depends(get_planning_service),
) -> ScheduleSummary:
    schedule = await planning_service.get_schedule(schedule_id)
    if schedule is None:
        raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail="schedule not found")
    return schedule


@schedules_router.put("/{schedule_id}", response_model=ScheduleSummary)
async def update_schedule(
    schedule_id: int,
    input: ScheduleMutationInput,
    planning_service: PlanningService = Depends(get_planning_service),
) -> ScheduleSummary:
    try:
        schedule = await planning_service.update_schedule(schedule_id, input)
    except ValueError as error:
        raise _bad_request(error) from error
    if schedule is None:
        raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail="schedule not found")
    return schedule


@schedules_router.delete("/{schedule_id}", status_code=status.HTTP_204_NO_CONTENT)
async def delete_schedule(
    schedule_id: int,
    planning_service: PlanningService = Depends(get_planning_service),
) -> None:
    deleted = await planning_service.delete_schedule(schedule_id)
    if not deleted:
        raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail="schedule not found")


@loans_router.get("", response_model=list[LoanSummary])
async def list_loans(
    book_id: int = Query(default=1),
    planning_service: PlanningService = Depends(get_planning_service),
) -> list[LoanSummary]:
    return await planning_service.list_loans(book_id)


@loans_router.post("", response_model=LoanSummary)
async def create_loan(
    input: LoanMutationInput,
    planning_service: PlanningService = Depends(get_planning_service),
) -> LoanSummary:
    try:
        return await planning_service.create_loan(input)
    except ValueError as error:
        raise _bad_request(error) from error


@loans_router.get("/{loan_id}", response_model=LoanSummary)
async def get_loan(
    loan_id: int,
    planning_service: PlanningService = Depends(get_planning_service),
) -> LoanSummary:
    loan = await planning_service.get_loan(loan_id)
    if loan is None:
        raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail="loan not found")
    return loan


@loans_router.put("/{loan_id}", response_model=LoanSummary)
async def update_loan(
    loan_id: int,
    input: LoanMutationInput,
    planning_service: PlanningService = Depends(get_planning_service),
) -> LoanSummary:
    try:
        loan = await planning_service.update_loan(loan_id, input)
    except ValueError as error:
        raise _bad_request(error) from error
    if loan is None:
        raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail="loan not found")
    return loan


@loans_router.delete("/{loan_id}", status_code=status.HTTP_204_NO_CONTENT)
async def delete_loan(
    loan_id: int,
    planning_service: PlanningService = Depends(get_planning_service),
) -> None:
    deleted = await planning_service.delete_loan(loan_id)
    if not deleted:
        raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail="loan not found")


@loans_router.get("/{loan_id}/amortization", response_model=list[AmortizationRow])
async def loan_amortization(
    loan_id: int,
    planning_service: PlanningService = Depends(get_planning_service),
) -> list[AmortizationRow]:
    try:
        return await planning_service.amortization(loan_id)
    except ValueError as error:
        raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail=str(error)) from error


@loans_router.post("/{loan_id}/payment-draft", response_model=LoanPaymentDraft)
async def loan_payment_draft(
    loan_id: int,
    input: LoanPaymentDraftInput,
    planning_service: PlanningService = Depends(get_planning_service),
) -> LoanPaymentDraft:
    try:
        return await planning_service.loan_payment_draft(loan_id, input)
    except ValueError as error:
        raise _bad_request(error) from error


@loans_router.post("/{loan_id}/post-payment", response_model=TransactionSummary)
async def post_loan_payment(
    loan_id: int,
    input: LoanPaymentDraftInput,
    planning_service: PlanningService = Depends(get_planning_service),
) -> TransactionSummary:
    try:
        return await planning_service.post_loan_payment(loan_id, input)
    except ValueError as error:
        raise _bad_request(error) from error
