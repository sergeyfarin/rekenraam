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
    mfa_enabled: bool = False


class AuthSessionSummary(BaseModel):
    id: int
    device_id: int | None
    expires_at: datetime


class AuthMe(BaseModel):
    user: AuthUserSummary
    session: AuthSessionSummary


class LoginResult(BaseModel):
    mfa_required: bool = False
    challenge_token: str | None = None
    user: AuthUserSummary | None = None
    session: AuthSessionSummary | None = None


class MfaStatus(BaseModel):
    enabled: bool
    enforced: bool
    recovery_codes_remaining: int


class MfaSetupInput(BaseModel):
    current_password: str = Field(min_length=1, max_length=1024)


class MfaSetupResult(BaseModel):
    setup_key: str
    otpauth_uri: str


class MfaConfirmInput(BaseModel):
    code: str = Field(min_length=6, max_length=64)


class MfaConfirmResult(BaseModel):
    recovery_codes: list[str]


class MfaDisableInput(BaseModel):
    current_password: str = Field(min_length=1, max_length=1024)
    code: str = Field(min_length=6, max_length=64)


class MfaLoginInput(BaseModel):
    challenge_token: str = Field(min_length=16, max_length=256)
    code: str = Field(min_length=6, max_length=64)


class PasswordResetRequestInput(BaseModel):
    email: str = Field(min_length=3, max_length=320)


class PasswordResetConfirmInput(BaseModel):
    token: str = Field(min_length=16, max_length=256)
    new_password: str = Field(min_length=12, max_length=1024)
