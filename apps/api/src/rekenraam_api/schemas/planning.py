from datetime import date, datetime

from pydantic import BaseModel, ConfigDict, model_validator

from rekenraam_api.schemas.transactions import (
    TransactionMutationInput,
    TransactionSummary,
)


class BudgetTargetInput(BaseModel):
    model_config = ConfigDict(frozen=True)

    category_id: int
    amount_minor: int
    rollover_enabled: bool = False


class BudgetMutationInput(BaseModel):
    model_config = ConfigDict(frozen=True)

    book_id: int
    name: str
    period: str
    starts_on: date
    ends_on: date | None = None
    commodity_id: int
    is_active: bool = True
    targets: tuple[BudgetTargetInput, ...] = ()


class BudgetTargetSummary(BaseModel):
    model_config = ConfigDict(frozen=True)

    id: int
    budget_id: int
    category_id: int
    amount_minor: int
    rollover_enabled: bool


class BudgetSummary(BaseModel):
    model_config = ConfigDict(frozen=True)

    id: int
    book_id: int
    name: str
    period: str
    starts_on: date
    ends_on: date | None
    commodity_id: int
    is_active: bool
    created_at: datetime
    targets: tuple[BudgetTargetSummary, ...] = ()


class BudgetVarianceRow(BaseModel):
    model_config = ConfigDict(frozen=True)

    category_id: int
    target_minor: int
    actual_minor: int
    rollover_minor: int
    remaining_minor: int


class ScheduleSplitInput(BaseModel):
    model_config = ConfigDict(frozen=True)

    account_id: int
    commodity_id: int
    amount_minor: int
    category_id: int | None = None
    memo: str | None = None


class ScheduleMutationInput(BaseModel):
    model_config = ConfigDict(frozen=True)

    book_id: int
    name: str
    payee_id: int | None = None
    memo: str | None = None
    status: str = "uncleared"
    reference: str | None = None
    frequency: str
    interval: int = 1
    start_date: date
    end_date: date | None = None
    reminder_days: int = 0
    enabled: bool = True
    splits: tuple[ScheduleSplitInput, ...]

    @model_validator(mode="after")
    def validate_schedule(self) -> ScheduleMutationInput:
        if self.interval < 1:
            raise ValueError("interval must be at least 1")
        if self.end_date is not None and self.end_date < self.start_date:
            raise ValueError("end_date must be on or after start_date")
        return self


class ScheduleSplitSummary(BaseModel):
    model_config = ConfigDict(frozen=True)

    id: int
    account_id: int
    commodity_id: int
    amount_minor: int
    category_id: int | None
    memo: str | None


class ScheduleSummary(BaseModel):
    model_config = ConfigDict(frozen=True)

    id: int
    book_id: int
    name: str
    payee_id: int | None
    memo: str | None
    status: str
    reference: str | None
    frequency: str
    interval: int
    start_date: date
    end_date: date | None
    reminder_days: int
    enabled: bool
    splits: tuple[ScheduleSplitSummary, ...]


class ScheduleOccurrenceSummary(BaseModel):
    model_config = ConfigDict(frozen=True)

    schedule_id: int
    occurrence_date: date
    status: str
    posted_transaction_id: int | None = None


class ProjectedCashRow(BaseModel):
    model_config = ConfigDict(frozen=True)

    account_id: int
    projection_date: date
    scheduled_delta_minor: int
    projected_balance_minor: int


class LoanMutationInput(BaseModel):
    model_config = ConfigDict(frozen=True)

    book_id: int
    account_id: int
    name: str
    principal_minor: int
    annual_rate_bps: int
    term_months: int
    payment_frequency: str = "monthly"
    start_date: date
    payment_account_id: int | None = None
    interest_category_id: int | None = None


class LoanSummary(BaseModel):
    model_config = ConfigDict(frozen=True)

    id: int
    book_id: int
    account_id: int
    name: str
    principal_minor: int
    annual_rate_bps: int
    term_months: int
    payment_frequency: str
    start_date: date
    payment_account_id: int | None
    interest_category_id: int | None
    payment_minor: int


class AmortizationRow(BaseModel):
    model_config = ConfigDict(frozen=True)

    period: int
    payment_date: date
    payment_minor: int
    principal_minor: int
    interest_minor: int
    remaining_principal_minor: int


class LoanPaymentDraftInput(BaseModel):
    model_config = ConfigDict(frozen=True)

    payment_date: date
    payment_account_id: int | None = None
    interest_category_id: int | None = None
    memo: str | None = None


class LoanPaymentDraft(BaseModel):
    model_config = ConfigDict(frozen=True)

    transaction: TransactionMutationInput


class PostedScheduleResult(BaseModel):
    model_config = ConfigDict(frozen=True)

    occurrence: ScheduleOccurrenceSummary
    transaction: TransactionSummary
