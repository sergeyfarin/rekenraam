"""End-to-end tests for the self-service password-reset flow.

Covers:
- happy path: token issued -> confirmed -> new password works, old does not
- token is single-use: a confirmed token cannot be reused
- expired tokens are rejected
- unknown emails return 204 without issuing a token (no enumeration)
- inactive users are treated like unknown emails
- confirming reset revokes all active sessions for the user
- request before bootstrap is rejected with 409
- new-password length validation surfaces as 400

Tests reach into the database directly only to retrieve the issued token (for
which there is no public API since SMTP delivery is deferred) and to age tokens
out for the expiry test. Every other assertion goes through HTTP.
"""

from __future__ import annotations

from datetime import UTC, datetime, timedelta

import pytest
from sqlalchemy import text
from sqlalchemy.ext.asyncio import AsyncSession

from e2e.conftest import E2EApp
from e2e.factories import (
    DEFAULT_ADMIN_EMAIL,
    DEFAULT_ADMIN_PASSWORD,
    bootstrap_admin,
    login,
)

NEW_PASSWORD = "fresh-correct-horse-battery"


async def _fetch_latest_token_hash(session: AsyncSession, *, user_email: str) -> str | None:
    result = await session.execute(
        text(
            """
            SELECT prt.token_hash
            FROM password_reset_tokens prt
            JOIN users u ON u.id = prt.user_id
            WHERE LOWER(u.email) = LOWER(:email)
            ORDER BY prt.id DESC
            LIMIT 1
            """
        ),
        {"email": user_email},
    )
    row = result.first()
    return row[0] if row else None


async def _expire_token(session: AsyncSession, *, token_hash: str) -> None:
    await session.execute(
        text(
            """
            UPDATE password_reset_tokens
            SET expires_at = :expired_at
            WHERE token_hash = :token_hash
            """
        ),
        {
            "token_hash": token_hash,
            "expired_at": datetime.now(UTC) - timedelta(hours=1),
        },
    )
    await session.commit()


async def issue_reset_token_via_service(e2e_app: E2EApp, *, user_email: str) -> str:
    """Call the service layer directly to obtain a cleartext reset token.

    Mirrors what the public `/password-reset/request` endpoint does, but
    returns the cleartext token instead of discarding it. Used by tests that
    need to drive the confirm step. The public-endpoint contract is exercised
    separately in `test_request_endpoint_returns_204_without_leaking_email`.
    """

    from rekenraam_api.config.settings import get_settings
    from rekenraam_api.repositories.access import AccessRepository
    from rekenraam_api.services.auth import AuthService

    async with e2e_app.sessionmaker() as session:
        service = AuthService(AccessRepository(session), get_settings())
        token = await service.request_password_reset(
            email=user_email, user_agent=None, ip_address=None
        )
        assert token is not None, "expected a token to be issued for the test user"
        return token


@pytest.mark.asyncio
async def test_request_endpoint_returns_204_for_known_and_unknown_emails(
    e2e_app: E2EApp,
) -> None:
    """The public request endpoint must not let callers tell whether an email
    is registered. Both known and unknown emails get 204 and the response
    body is empty."""

    await bootstrap_admin(e2e_app.client)

    known = await e2e_app.client.post(
        "/api/v1/auth/password-reset/request",
        json={"email": DEFAULT_ADMIN_EMAIL},
    )
    assert known.status_code == 204
    assert known.text == ""

    unknown = await e2e_app.client.post(
        "/api/v1/auth/password-reset/request",
        json={"email": "nobody@example.com"},
    )
    assert unknown.status_code == 204
    assert unknown.text == ""

    # But the database must show: token row for the known email, none for the
    # unknown one. (Audit rows are written for both; we don't assert on those
    # here — that's the existing audit-trail tests' territory.)
    async with e2e_app.sessionmaker() as session:
        known_hash = await _fetch_latest_token_hash(session, user_email=DEFAULT_ADMIN_EMAIL)
        unknown_hash = await _fetch_latest_token_hash(session, user_email="nobody@example.com")
    assert known_hash is not None
    assert unknown_hash is None


@pytest.mark.asyncio
async def test_request_before_bootstrap_returns_409(e2e_app: E2EApp) -> None:
    response = await e2e_app.client.post(
        "/api/v1/auth/password-reset/request",
        json={"email": "anyone@example.com"},
    )
    assert response.status_code == 409
    # Database confirms no token row exists.
    async with e2e_app.sessionmaker() as session:
        result = await session.execute(text("SELECT COUNT(*) FROM password_reset_tokens"))
        assert result.scalar_one() == 0


@pytest.mark.asyncio
async def test_full_password_reset_round_trip(e2e_app: E2EApp) -> None:
    """The canonical happy path. Bootstrap, request a reset, confirm with the
    cleartext token, then prove the new password works and the old does not."""

    await bootstrap_admin(e2e_app.client)
    # bootstrap_admin leaves the client logged in; clear that session so we
    # genuinely test logging in fresh after the reset.
    e2e_app.client.cookies.clear()

    token = await issue_reset_token_via_service(e2e_app, user_email=DEFAULT_ADMIN_EMAIL)
    assert len(token) > 32  # secrets.token_urlsafe(48) is well over 32 chars

    confirm = await e2e_app.client.post(
        "/api/v1/auth/password-reset/confirm",
        json={"token": token, "new_password": NEW_PASSWORD},
    )
    assert confirm.status_code == 204

    # New password works.
    fresh = await login(e2e_app.client, password=NEW_PASSWORD)
    assert fresh["user"]["email"] == DEFAULT_ADMIN_EMAIL

    # Old password no longer works.
    e2e_app.client.cookies.clear()
    failed = await e2e_app.client.post(
        "/api/v1/auth/login",
        json={"email": DEFAULT_ADMIN_EMAIL, "password": DEFAULT_ADMIN_PASSWORD},
    )
    assert failed.status_code == 401


@pytest.mark.asyncio
async def test_token_is_single_use(e2e_app: E2EApp) -> None:
    """A token that's been used to confirm a reset cannot be used again."""

    await bootstrap_admin(e2e_app.client)
    token = await issue_reset_token_via_service(e2e_app, user_email=DEFAULT_ADMIN_EMAIL)

    first = await e2e_app.client.post(
        "/api/v1/auth/password-reset/confirm",
        json={"token": token, "new_password": NEW_PASSWORD},
    )
    assert first.status_code == 204

    second = await e2e_app.client.post(
        "/api/v1/auth/password-reset/confirm",
        json={"token": token, "new_password": "another-different-password"},
    )
    assert second.status_code == 400


@pytest.mark.asyncio
async def test_expired_token_is_rejected(e2e_app: E2EApp) -> None:
    """A token whose expires_at is in the past must be rejected as if it
    never existed. Validates the SQL filter, not just service-level dates."""

    await bootstrap_admin(e2e_app.client)
    token = await issue_reset_token_via_service(e2e_app, user_email=DEFAULT_ADMIN_EMAIL)

    from rekenraam_api.services.auth import hash_session_token

    async with e2e_app.sessionmaker() as session:
        await _expire_token(session, token_hash=hash_session_token(token))

    response = await e2e_app.client.post(
        "/api/v1/auth/password-reset/confirm",
        json={"token": token, "new_password": NEW_PASSWORD},
    )
    assert response.status_code == 400


@pytest.mark.asyncio
async def test_short_new_password_is_rejected(e2e_app: E2EApp) -> None:
    await bootstrap_admin(e2e_app.client)
    token = await issue_reset_token_via_service(e2e_app, user_email=DEFAULT_ADMIN_EMAIL)

    response = await e2e_app.client.post(
        "/api/v1/auth/password-reset/confirm",
        json={"token": token, "new_password": "tooshort"},
    )
    # Pydantic enforces min_length=12 at the schema layer, so this surfaces as
    # 422 from FastAPI input validation rather than 400 from service logic.
    assert response.status_code in (400, 422)

    # And the token must still be usable: a rejected confirm must not consume.
    retry = await e2e_app.client.post(
        "/api/v1/auth/password-reset/confirm",
        json={"token": token, "new_password": NEW_PASSWORD},
    )
    assert retry.status_code == 204


@pytest.mark.asyncio
async def test_unknown_token_is_rejected(e2e_app: E2EApp) -> None:
    await bootstrap_admin(e2e_app.client)

    response = await e2e_app.client.post(
        "/api/v1/auth/password-reset/confirm",
        json={
            "token": "totally-bogus-token-string-that-was-never-issued",
            "new_password": NEW_PASSWORD,
        },
    )
    assert response.status_code == 400


@pytest.mark.asyncio
async def test_confirm_revokes_existing_sessions(e2e_app: E2EApp) -> None:
    """If an attacker has already established a session, a password reset
    must kick them out. After confirm, calls authenticated by any pre-existing
    session must 401."""

    await bootstrap_admin(e2e_app.client)
    # The bootstrap left the client holding an active session cookie.
    me_before = await e2e_app.client.get("/api/v1/auth/me")
    assert me_before.status_code == 200

    token = await issue_reset_token_via_service(e2e_app, user_email=DEFAULT_ADMIN_EMAIL)
    confirm = await e2e_app.client.post(
        "/api/v1/auth/password-reset/confirm",
        json={"token": token, "new_password": NEW_PASSWORD},
    )
    assert confirm.status_code == 204

    # The cookie is still on the client — but the server has revoked the
    # session, so /me must now 401.
    me_after = await e2e_app.client.get("/api/v1/auth/me")
    assert me_after.status_code == 401


@pytest.mark.asyncio
async def test_inactive_user_treated_like_unknown_email(e2e_app: E2EApp) -> None:
    """An admin can deactivate a user (set is_active=false). A reset request
    for that user must behave like an unknown email — 204 with no token."""

    await bootstrap_admin(e2e_app.client)
    async with e2e_app.sessionmaker() as session:
        await session.execute(
            text("UPDATE users SET is_active = false WHERE LOWER(email) = LOWER(:email)"),
            {"email": DEFAULT_ADMIN_EMAIL},
        )
        await session.commit()

    response = await e2e_app.client.post(
        "/api/v1/auth/password-reset/request",
        json={"email": DEFAULT_ADMIN_EMAIL},
    )
    assert response.status_code == 204

    async with e2e_app.sessionmaker() as session:
        # No token row should be created for the inactive user.
        token_hash = await _fetch_latest_token_hash(session, user_email=DEFAULT_ADMIN_EMAIL)
    assert token_hash is None
