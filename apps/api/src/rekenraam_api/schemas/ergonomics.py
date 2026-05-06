from datetime import date, datetime

from pydantic import BaseModel, ConfigDict, Field

from rekenraam_api.schemas.transactions import (
    TransactionListFilters,
    TransactionMutationInput,
    TransactionSplitInput,
)


class BookMembershipSummary(BaseModel):
    model_config = ConfigDict(frozen=True)

    book_id: int
    book_slug: str
    book_name: str
    role: str


class AdminUserSummary(BaseModel):
    model_config = ConfigDict(frozen=True)

    id: int
    email: str
    display_name: str
    is_admin: bool
    is_active: bool
    mfa_required: bool
    created_at: datetime
    updated_at: datetime
    memberships: tuple[BookMembershipSummary, ...]


class AdminUserCreateInput(BaseModel):
    model_config = ConfigDict(frozen=True)

    email: str = Field(min_length=3, max_length=320)
    display_name: str = Field(min_length=1, max_length=200)
    password: str = Field(min_length=12, max_length=1024)
    is_admin: bool = False
    memberships: tuple[tuple[int, str], ...] = ()


class AdminUserUpdateInput(BaseModel):
    model_config = ConfigDict(frozen=True)

    display_name: str | None = Field(default=None, min_length=1, max_length=200)
    is_admin: bool | None = None
    is_active: bool | None = None
    mfa_required: bool | None = None


class AdminPasswordResetInput(BaseModel):
    model_config = ConfigDict(frozen=True)

    password: str = Field(min_length=12, max_length=1024)


class BookMembershipInput(BaseModel):
    model_config = ConfigDict(frozen=True)

    book_id: int
    role: str


class UserPreferenceSummary(BaseModel):
    model_config = ConfigDict(frozen=True)

    default_book_id: int | None
    locale: str | None
    date_format: str
    number_format: str
    theme: str


class UserPreferenceUpdateInput(BaseModel):
    model_config = ConfigDict(frozen=True)

    default_book_id: int | None = None
    locale: str | None = Field(default=None, max_length=64)
    date_format: str | None = Field(default=None, max_length=32)
    number_format: str | None = Field(default=None, max_length=32)
    theme: str | None = Field(default=None, max_length=64)


class ProfileUpdateInput(BaseModel):
    model_config = ConfigDict(frozen=True)

    display_name: str = Field(min_length=1, max_length=200)


class PasswordChangeInput(BaseModel):
    model_config = ConfigDict(frozen=True)

    current_password: str = Field(min_length=1, max_length=1024)
    new_password: str = Field(min_length=12, max_length=1024)


class TransactionSavedViewInput(BaseModel):
    model_config = ConfigDict(frozen=True)

    book_id: int
    name: str = Field(min_length=1, max_length=120)
    filters: TransactionListFilters
    is_shared: bool = False


class TransactionSavedViewSummary(BaseModel):
    model_config = ConfigDict(frozen=True)

    id: int
    user_id: int
    book_id: int
    name: str
    filters: TransactionListFilters
    is_shared: bool
    created_at: datetime
    updated_at: datetime


class TransactionTemplateSplitSummary(TransactionSplitInput):
    model_config = ConfigDict(frozen=True)

    id: int
    line_order: int


class TransactionTemplateInput(BaseModel):
    model_config = ConfigDict(frozen=True)

    book_id: int
    name: str = Field(min_length=1, max_length=120)
    payee_id: int | None
    memo: str | None
    status: str = "uncleared"
    reference: str | None
    is_shared: bool = False
    splits: tuple[TransactionSplitInput, ...]


class TransactionTemplateSummary(BaseModel):
    model_config = ConfigDict(frozen=True)

    id: int
    user_id: int
    book_id: int
    name: str
    payee_id: int | None
    memo: str | None
    status: str
    reference: str | None
    is_shared: bool
    created_at: datetime
    updated_at: datetime
    splits: tuple[TransactionTemplateSplitSummary, ...]


class TransactionTemplatePostInput(BaseModel):
    model_config = ConfigDict(frozen=True)

    txn_date: date


class PayeeDefaultInput(BaseModel):
    model_config = ConfigDict(frozen=True)

    book_id: int
    payee_id: int
    account_id: int | None = None
    category_id: int | None = None
    memo: str | None = None


class PayeeDefaultSummary(BaseModel):
    model_config = ConfigDict(frozen=True)

    id: int
    user_id: int
    book_id: int
    payee_id: int
    account_id: int | None
    category_id: int | None
    memo: str | None
    created_at: datetime
    updated_at: datetime


class MarkdownNoteInput(BaseModel):
    model_config = ConfigDict(frozen=True)

    title: str = Field(min_length=1, max_length=200)
    body_markdown: str = Field(min_length=0, max_length=100000)
    pinned: bool = False


class MarkdownNoteSummary(BaseModel):
    model_config = ConfigDict(frozen=True)

    id: int
    book_id: int
    target_type: str
    account_id: int | None
    transaction_id: int | None
    title: str
    body_markdown: str
    pinned: bool
    created_at: datetime
    updated_at: datetime
    created_by_user_id: int | None
    updated_by_user_id: int | None


class AuditEventSummary(BaseModel):
    model_config = ConfigDict(frozen=True)

    id: int
    book_id: int | None
    actor_user_id: int | None
    event_type: str
    target_type: str | None
    target_id: int | None
    summary: str
    metadata: dict[str, object] | None
    created_at: datetime


class TemplateTransactionMutation(TransactionMutationInput):
    model_config = ConfigDict(frozen=True)
