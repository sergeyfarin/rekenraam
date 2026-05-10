from __future__ import annotations

from datetime import date, datetime

from pydantic import BaseModel, ConfigDict


class AccountSummary(BaseModel):
    model_config = ConfigDict(frozen=True)

    id: int
    book_id: int
    parent_id: int | None
    account_type: str
    name: str
    commodity_id: int
    institution_id: int | None
    institution_name: str | None
    country_id: int | None
    country_name: str | None
    number_last4: str | None
    is_closed: bool
    is_hidden: bool
    is_system: bool
    system_role: str | None
    created_at: datetime
    updated_at: datetime


class AccountBalanceSummary(BaseModel):
    model_config = ConfigDict(frozen=True)

    account_id: int
    balance_minor: int


class AccountBalancingSummary(BaseModel):
    model_config = ConfigDict(frozen=True)

    id: int
    account_id: int
    as_of_date: date
    balance_minor: int
    memo: str | None


class AccountDirectiveSummary(BaseModel):
    model_config = ConfigDict(frozen=True)

    id: int
    book_id: int
    account_id: int
    directive_type: str
    directive_date: date
    note: str | None
    metadata: str | None
    created_at: datetime


class AccountBookingPolicyUpdate(BaseModel):
    model_config = ConfigDict(frozen=True)

    booking_policy: str


class AccountCreateInput(BaseModel):
    model_config = ConfigDict(frozen=True)

    book_id: int
    parent_id: int | None
    account_type: str
    name: str
    commodity_id: int
    institution_id: int | None
    country_id: int | None
    number_last4: str | None
    is_closed: bool


class AccountUpdateInput(BaseModel):
    model_config = ConfigDict(frozen=True)

    book_id: int
    parent_id: int | None
    account_type: str
    name: str
    commodity_id: int
    institution_id: int | None
    country_id: int | None
    number_last4: str | None
    is_closed: bool


class AccountBalancingUnlockInput(BaseModel):
    model_config = ConfigDict(frozen=True)

    from_date: date
    reason: str | None
    confirm: bool


class AccountBalancingCreateInput(BaseModel):
    model_config = ConfigDict(frozen=True)

    book_id: int
    account_id: int
    as_of_date: date
    balance_minor: int
    memo: str | None


class AccountClosingValidationResult(BaseModel):
    model_config = ConfigDict(frozen=True)

    valid: bool
    issues: tuple[str, ...]


class AccountTreeNode(BaseModel):
    model_config = ConfigDict(frozen=True)

    id: int
    parent_id: int | None
    name: str
    account_type: str
    commodity_id: int
    commodity_name: str
    commodity_scale: int
    institution_name: str | None
    country_name: str | None
    balance_minor: int
    rollup_balance_minor: int
    children: tuple[AccountTreeNode, ...]


AccountTreeNode.model_rebuild()
