from fastapi import APIRouter, Depends, HTTPException, Request, Response, status

from rekenraam_api.api.dependencies import get_auth_service, require_request_context
from rekenraam_api.schemas.auth import AuthMe, BootstrapAdminInput, BootstrapStatus, LoginInput
from rekenraam_api.services.auth import (
    SESSION_COOKIE_NAME,
    SESSION_DAYS,
    AuthService,
    AuthenticationError,
    BootstrapUnavailableError,
    to_auth_me,
)
from rekenraam_api.services.request_context import RequestContext


router = APIRouter(prefix="/auth", tags=["auth"])


def _client_ip(request: Request) -> str | None:
    forwarded_for = request.headers.get("x-forwarded-for")
    if forwarded_for:
        return forwarded_for.split(",", maxsplit=1)[0].strip()
    if request.client is None:
        return None
    return request.client.host


def _set_session_cookie(response: Response, token: str) -> None:
    response.set_cookie(
        SESSION_COOKIE_NAME,
        token,
        max_age=SESSION_DAYS * 24 * 60 * 60,
        httponly=True,
        secure=False,
        samesite="lax",
        path="/",
    )


def _clear_session_cookie(response: Response) -> None:
    response.delete_cookie(SESSION_COOKIE_NAME, path="/", samesite="lax")


@router.get("/bootstrap/status", response_model=BootstrapStatus)
async def get_bootstrap_status(auth_service: AuthService = Depends(get_auth_service)) -> BootstrapStatus:
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


@router.post("/login", response_model=AuthMe)
async def login(
    input: LoginInput,
    request: Request,
    response: Response,
    auth_service: AuthService = Depends(get_auth_service),
) -> AuthMe:
    try:
        created = await auth_service.login(
            email=input.email,
            password=input.password,
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
    try:
        return await auth_service.me(context)
    except AuthenticationError as error:
        raise HTTPException(status_code=status.HTTP_401_UNAUTHORIZED, detail=str(error)) from error
