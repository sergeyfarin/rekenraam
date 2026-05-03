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