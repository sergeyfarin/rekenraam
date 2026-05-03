from __future__ import annotations

from datetime import datetime

from pydantic import BaseModel, ConfigDict


class AccountSummary(BaseModel):
    model_config = ConfigDict(frozen=True)

    id: int
    book_id: int
    parent_id: int | None
    account_type: str
    name: str
    is_closed: bool
    is_hidden: bool
    is_system: bool
    system_role: str | None
    created_at: datetime


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