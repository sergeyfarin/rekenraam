from __future__ import annotations

from datetime import date, datetime

from pydantic import BaseModel, ConfigDict

from rekenraam_api.schemas.accounts import AccountBalancingSummary, AccountSummary
from rekenraam_api.schemas.transactions import TransactionSummary


class ReconciliationCommoditySummary(BaseModel):
    model_config = ConfigDict(frozen=True)

    id: int
    name: str
    symbol: str | None
    scale: int


class BalanceCheckSummary(BaseModel):
    model_config = ConfigDict(frozen=True)

    id: int
    book_id: int
    account_id: int
    as_of_date: date
    balance_minor: int
    memo: str | None
    status: str
    created_at: datetime


class ReconciliationBalancingSummary(BaseModel):
    model_config = ConfigDict(frozen=True)

    id: int
    book_id: int
    account_id: int
    as_of_date: date
    balance_minor: int
    memo: str | None
    created_at: datetime
    voided_at: datetime | None
    void_reason: str | None


class BalanceAdjustmentSummary(BaseModel):
    model_config = ConfigDict(frozen=True)

    id: int
    book_id: int
    balance_check_id: int | None
    account_id: int
    offset_account_id: int
    as_of_date: date
    target_balance_minor: int
    actual_balance_minor: int
    adjustment_minor: int
    tx_id: int
    mark: str
    memo: str | None
    created_at: datetime


class BalanceConstraintSummary(BaseModel):
    model_config = ConfigDict(frozen=True)

    id: int
    book_id: int
    account_id: int
    min_balance_minor: int | None
    max_balance_minor: int | None
    sign_rule: str
    created_at: datetime
    updated_at: datetime


class BalanceConstraintCreateInput(BaseModel):
    model_config = ConfigDict(frozen=True)

    book_id: int
    min_balance_minor: int | None = None
    max_balance_minor: int | None = None
    sign_rule: str = "any"


class BalanceConstraintValidation(BaseModel):
    model_config = ConfigDict(frozen=True)

    valid: bool
    balance_minor: int
    issues: tuple[str, ...]


class ReconciliationStartResponse(BaseModel):
    model_config = ConfigDict(frozen=True)

    account: AccountSummary
    commodity: ReconciliationCommoditySummary
    last_balancing: AccountBalancingSummary | None
    statement_start_date: date | None
    statement_end_date: date
    remembered_offset_account_id: int | None
    candidates: tuple[TransactionSummary, ...]


class ReconciliationFinishInput(BaseModel):
    model_config = ConfigDict(frozen=True)

    book_id: int
    as_of_date: date
    statement_start_date: date | None = None
    statement_balance_minor: int
    selected_transaction_ids: tuple[int, ...]
    offset_account_id: int | None = None
    remember_offset_account: bool = False
    memo: str | None = None
    confirm_unbalanced: bool = False
    downgrade_unselected_cleared: bool = True


class ReconciliationFinishResult(BaseModel):
    model_config = ConfigDict(frozen=True)

    balance_check: BalanceCheckSummary
    balancing: AccountBalancingSummary
    adjustment: BalanceAdjustmentSummary | None
    difference_minor: int
    reconciled_count: int
    uncleared_count: int


class ReconciliationHistoryResponse(BaseModel):
    model_config = ConfigDict(frozen=True)

    balancings: tuple[ReconciliationBalancingSummary, ...]
    balance_checks: tuple[BalanceCheckSummary, ...]
    balance_adjustments: tuple[BalanceAdjustmentSummary, ...]


class ReconciliationUnlockInput(BaseModel):
    model_config = ConfigDict(frozen=True)

    from_date: date
    reason: str | None
    confirm: bool
