import pytest
from fastapi import Response

from rekenraam_api.api.v1 import auth
from rekenraam_api.config.settings import Settings
from rekenraam_api.services.auth import (
    AuthenticationError,
    AuthService,
    _login_failures,
)


def test_session_cookie_uses_secure_httponly_and_configured_samesite(monkeypatch) -> None:
    monkeypatch.setattr(
        auth,
        "get_settings",
        lambda: Settings(session_cookie_secure=True, session_cookie_samesite="strict"),
    )
    response = Response()

    auth._set_session_cookie(response, "session-token")

    cookie = response.headers["set-cookie"]
    assert "rekenraam_session=session-token" in cookie
    assert "HttpOnly" in cookie
    assert "Secure" in cookie
    assert "SameSite=strict" in cookie


def test_session_cookie_falls_back_to_lax_for_invalid_samesite(monkeypatch) -> None:
    monkeypatch.setattr(
        auth,
        "get_settings",
        lambda: Settings(session_cookie_secure=False, session_cookie_samesite="surprise"),
    )
    response = Response()

    auth._set_session_cookie(response, "session-token")

    cookie = response.headers["set-cookie"]
    assert "Secure" not in cookie
    assert "SameSite=lax" in cookie


def test_trusted_proxy_cidr_matching_is_strict_and_tolerates_bad_input() -> None:
    assert auth._trusted_proxy("172.18.0.4", ["172.16.0.0/12"]) is True
    assert auth._trusted_proxy("203.0.113.10", ["172.16.0.0/12"]) is False
    assert auth._trusted_proxy("not-an-ip", ["172.16.0.0/12"]) is False
    assert auth._trusted_proxy("172.18.0.4", ["not-a-cidr"]) is False


class RateLimitRepository:
    audits: list[str]
    commits: int

    def __init__(self) -> None:
        self.audits = []
        self.commits = 0

    async def get_user_by_email(self, email: str):
        return None

    async def audit(
        self,
        *,
        user_id: int | None,
        session_id: int | None,
        device_id: int | None,
        event_type: str,
        target_type: str | None,
        target_id: int | None,
        summary: str,
    ) -> None:
        self.audits.append(event_type)

    async def commit(self) -> None:
        self.commits += 1


@pytest.fixture(autouse=True)
def clear_login_failures() -> None:
    _login_failures.clear()


@pytest.mark.asyncio
async def test_login_rate_limit_blocks_repeated_failures_by_email_and_ip() -> None:
    repository = RateLimitRepository()
    service = AuthService(
        repository,
        Settings(login_rate_limit_attempts=2, login_rate_limit_window_seconds=300),
    )

    for _ in range(2):
        with pytest.raises(AuthenticationError, match="invalid email or password"):
            await service.login(
                email="Admin@Example.Test",
                password="wrong",
                user_agent="pytest",
                ip_address="198.51.100.7",
            )

    with pytest.raises(AuthenticationError, match="too many failed login attempts"):
        await service.login(
            email="admin@example.test",
            password="wrong",
            user_agent="pytest",
            ip_address="198.51.100.7",
        )

    assert repository.audits == ["auth.login.failed", "auth.login.failed"]
    assert repository.commits == 2
