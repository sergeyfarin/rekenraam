from ipaddress import ip_address, ip_network
from typing import Literal, cast

from fastapi import APIRouter, Depends, HTTPException, Request, Response, status

from rekenraam_api.api.dependencies import (
    get_auth_service,
    get_ergonomics_service,
    require_request_context,
)
from rekenraam_api.config.settings import get_settings
from rekenraam_api.schemas.auth import (
    AuthMe,
    BootstrapAdminInput,
    BootstrapStatus,
    LoginInput,
    LoginResult,
    MfaConfirmInput,
    MfaConfirmResult,
    MfaDisableInput,
    MfaLoginInput,
    MfaSetupInput,
    MfaSetupResult,
    MfaStatus,
)
from rekenraam_api.schemas.ergonomics import PasswordChangeInput, ProfileUpdateInput
from rekenraam_api.services.auth import (
    SESSION_COOKIE_NAME,
    SESSION_DAYS,
    AuthenticationError,
    AuthService,
    BootstrapUnavailableError,
    MfaRequiredError,
    to_auth_me,
)
from rekenraam_api.services.ergonomics import ErgonomicsService
from rekenraam_api.services.request_context import RequestContext

router = APIRouter(prefix="/auth", tags=["auth"])


def _client_ip(request: Request) -> str | None:
    settings = get_settings()
    forwarded_for = request.headers.get("x-forwarded-for")
    client_host = request.client.host if request.client is not None else None
    if forwarded_for and client_host and _trusted_proxy(client_host, settings.trusted_proxy_cidrs_list):
        return forwarded_for.split(",", maxsplit=1)[0].strip()
    return client_host


def _trusted_proxy(client_host: str, cidrs: list[str]) -> bool:
    try:
        address = ip_address(client_host)
        return any(address in ip_network(cidr, strict=False) for cidr in cidrs)
    except ValueError:
        return False


def _set_session_cookie(response: Response, token: str) -> None:
    settings = get_settings()
    samesite = cast(
        Literal["lax", "strict", "none"],
        settings.session_cookie_samesite if settings.session_cookie_samesite in {"lax", "strict", "none"} else "lax",
    )
    response.set_cookie(
        SESSION_COOKIE_NAME,
        token,
        max_age=SESSION_DAYS * 24 * 60 * 60,
        httponly=True,
        secure=settings.session_cookie_secure,
        samesite=samesite,
        path="/",
    )


def _clear_session_cookie(response: Response) -> None:
    response.delete_cookie(SESSION_COOKIE_NAME, path="/", samesite="lax")


@router.get("/bootstrap/status", response_model=BootstrapStatus)
async def get_bootstrap_status(
    auth_service: AuthService = Depends(get_auth_service),
) -> BootstrapStatus:
    return BootstrapStatus(bootstrap_required=await auth_service.bootstrap_required())


@router.post("/bootstrap/admin", response_model=AuthMe)
async def create_first_admin(
    input: BootstrapAdminInput,
    request: Request,
    response: Response,
    auth_service: AuthService = Depends(get_auth_service),
) -> AuthMe:
    try:
        created = await auth_service.create_first_admin(
            email=input.email,
            password=input.password,
            display_name=input.display_name,
            user_agent=request.headers.get("user-agent"),
            ip_address=_client_ip(request),
        )
    except BootstrapUnavailableError as error:
        raise HTTPException(status_code=status.HTTP_409_CONFLICT, detail=str(error)) from error
    _set_session_cookie(response, created.token)
    return to_auth_me(created.user, created.session)


@router.post("/login", response_model=LoginResult)
async def login(
    input: LoginInput,
    request: Request,
    response: Response,
    auth_service: AuthService = Depends(get_auth_service),
) -> LoginResult:
    try:
        created = await auth_service.login(
            email=input.email,
            password=input.password,
            user_agent=request.headers.get("user-agent"),
            ip_address=_client_ip(request),
        )
    except MfaRequiredError as error:
        return LoginResult(mfa_required=True, challenge_token=error.challenge_token)
    except AuthenticationError as error:
        raise HTTPException(status_code=status.HTTP_401_UNAUTHORIZED, detail=str(error)) from error
    _set_session_cookie(response, created.token)
    auth_me = to_auth_me(created.user, created.session)
    return LoginResult(user=auth_me.user, session=auth_me.session)


@router.post("/mfa/login", response_model=AuthMe)
async def complete_mfa_login(
    input: MfaLoginInput,
    request: Request,
    response: Response,
    auth_service: AuthService = Depends(get_auth_service),
) -> AuthMe:
    try:
        created = await auth_service.complete_mfa_login(
            challenge_token=input.challenge_token,
            code=input.code,
            user_agent=request.headers.get("user-agent"),
            ip_address=_client_ip(request),
        )
    except AuthenticationError as error:
        raise HTTPException(status_code=status.HTTP_401_UNAUTHORIZED, detail=str(error)) from error
    _set_session_cookie(response, created.token)
    return to_auth_me(created.user, created.session)


@router.post("/logout", status_code=status.HTTP_204_NO_CONTENT)
async def logout(
    response: Response,
    context: RequestContext = Depends(require_request_context),
    auth_service: AuthService = Depends(get_auth_service),
) -> None:
    await auth_service.logout(context.session_id)
    _clear_session_cookie(response)


@router.get("/me", response_model=AuthMe)
async def get_me(
    context: RequestContext = Depends(require_request_context),
    auth_service: AuthService = Depends(get_auth_service),
) -> AuthMe:
    return await auth_service.me(context)


@router.get("/mfa/status", response_model=MfaStatus)
async def get_mfa_status(
    context: RequestContext = Depends(require_request_context),
    auth_service: AuthService = Depends(get_auth_service),
) -> MfaStatus:
    enabled, remaining = await auth_service.mfa_status(context)
    return MfaStatus(
        enabled=enabled,
        enforced=get_settings().mfa_enforced,
        recovery_codes_remaining=remaining,
    )


@router.post("/mfa/setup", response_model=MfaSetupResult)
async def begin_mfa_setup(
    input: MfaSetupInput,
    context: RequestContext = Depends(require_request_context),
    auth_service: AuthService = Depends(get_auth_service),
) -> MfaSetupResult:
    try:
        setup_key, otpauth_uri = await auth_service.begin_mfa_setup(context, input.current_password)
    except AuthenticationError as error:
        raise HTTPException(status_code=status.HTTP_400_BAD_REQUEST, detail=str(error)) from error
    return MfaSetupResult(setup_key=setup_key, otpauth_uri=otpauth_uri)


@router.post("/mfa/confirm", response_model=MfaConfirmResult)
async def confirm_mfa_setup(
    input: MfaConfirmInput,
    context: RequestContext = Depends(require_request_context),
    auth_service: AuthService = Depends(get_auth_service),
) -> MfaConfirmResult:
    try:
        return MfaConfirmResult(recovery_codes=await auth_service.confirm_mfa_setup(context, input.code))
    except AuthenticationError as error:
        raise HTTPException(status_code=status.HTTP_400_BAD_REQUEST, detail=str(error)) from error


@router.delete("/mfa", status_code=status.HTTP_204_NO_CONTENT)
async def disable_mfa(
    input: MfaDisableInput,
    context: RequestContext = Depends(require_request_context),
    auth_service: AuthService = Depends(get_auth_service),
) -> None:
    try:
        await auth_service.disable_mfa(context, input.current_password, input.code)
    except AuthenticationError as error:
        raise HTTPException(status_code=status.HTTP_400_BAD_REQUEST, detail=str(error)) from error


@router.post("/mfa/disable", status_code=status.HTTP_204_NO_CONTENT)
async def disable_mfa_post(
    input: MfaDisableInput,
    context: RequestContext = Depends(require_request_context),
    auth_service: AuthService = Depends(get_auth_service),
) -> None:
    try:
        await auth_service.disable_mfa(context, input.current_password, input.code)
    except AuthenticationError as error:
        raise HTTPException(status_code=status.HTTP_400_BAD_REQUEST, detail=str(error)) from error


@router.put("/me/profile", response_model=AuthMe)
async def update_profile(
    input: ProfileUpdateInput,
    context: RequestContext = Depends(require_request_context),
    auth_service: AuthService = Depends(get_auth_service),
    ergonomics_service: ErgonomicsService = Depends(get_ergonomics_service),
) -> AuthMe:
    try:
        await ergonomics_service.update_profile(input)
        return await auth_service.me(context)
    except ValueError as error:
        raise HTTPException(status_code=status.HTTP_400_BAD_REQUEST, detail=str(error)) from error


@router.post("/me/password", status_code=status.HTTP_204_NO_CONTENT)
async def change_password(
    input: PasswordChangeInput,
    ergonomics_service: ErgonomicsService = Depends(get_ergonomics_service),
) -> None:
    try:
        await ergonomics_service.change_password(input)
    except ValueError as error:
        raise HTTPException(status_code=status.HTTP_400_BAD_REQUEST, detail=str(error)) from error
