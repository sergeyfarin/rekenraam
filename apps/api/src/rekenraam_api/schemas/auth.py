from datetime import datetime

from pydantic import BaseModel, Field


class BootstrapStatus(BaseModel):
    bootstrap_required: bool


class BootstrapAdminInput(BaseModel):
    email: str = Field(min_length=3, max_length=320)
    password: str = Field(min_length=12, max_length=1024)
    display_name: str = Field(min_length=1, max_length=200)


class LoginInput(BaseModel):
    email: str = Field(min_length=3, max_length=320)
    password: str = Field(min_length=1, max_length=1024)


class AuthUserSummary(BaseModel):
    id: int
    email: str
    display_name: str
    is_admin: bool
    is_active: bool


class AuthSessionSummary(BaseModel):
    id: int
    device_id: int | None
    expires_at: datetime


class AuthMe(BaseModel):
    user: AuthUserSummary
    session: AuthSessionSummary
