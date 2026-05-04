from __future__ import annotations

import hashlib
import secrets
from dataclasses import dataclass
from datetime import UTC, datetime, timedelta

from argon2 import PasswordHasher
from argon2.exceptions import VerifyMismatchError

from rekenraam_api.db.models.access import AuthSession, User
from rekenraam_api.repositories.access import AccessRepository
from rekenraam_api.schemas.auth import AuthMe, AuthSessionSummary, AuthUserSummary
from rekenraam_api.services.request_context import RequestContext


SESSION_COOKIE_NAME = "rekenraam_session"
SESSION_DAYS = 14

_password_hasher = PasswordHasher()


class AuthenticationError(ValueError):
    pass


class BootstrapUnavailableError(ValueError):
    pass


@dataclass(frozen=True)
class CreatedSession:
    token: str
    user: User
    session: AuthSession


def hash_session_token(token: str) -> str:
    return hashlib.sha256(token.encode("utf-8")).hexdigest()


def _device_fingerprint(user_agent: str | None, ip_address: str | None) -> str:
    raw = f"{user_agent or ''}|{ip_address or ''}"
    return hashlib.sha256(raw.encode("utf-8")).hexdigest()


class AuthService:
    def __init__(self, repository: AccessRepository) -> None:
        self._repository = repository

    async def bootstrap_required(self) -> bool:
        return await self._repository.count_users() == 0

    async def create_first_admin(
        self,
        *,
        email: str,
        password: str,
        display_name: str,
        user_agent: str | None,
        ip_address: str | None,
    ) -> CreatedSession:
        if not await self.bootstrap_required():
            raise BootstrapUnavailableError("bootstrap is already complete")
        user = await self._repository.create_admin_user(
            email=email,
            password_hash=_password_hasher.hash(password),
            display_name=display_name.strip(),
        )
        created = await self._create_session(user, user_agent=user_agent, ip_address=ip_address)
        await self._repository.commit()
        return created

    async def seed_first_admin(self, *, email: str, password: str, display_name: str) -> User | None:
        if not await self.bootstrap_required():
            return None
        user = await self._repository.create_admin_user(
            email=email,
            password_hash=_password_hasher.hash(password),
            display_name=display_name.strip() or "Admin",
        )
        await self._repository.commit()
        return user

    async def login(
        self,
        *,
        email: str,
        password: str,
        user_agent: str | None,
        ip_address: str | None,
    ) -> CreatedSession:
        user = await self._repository.get_user_by_email(email)
        if user is None or user.password_hash is None:
            raise AuthenticationError("invalid email or password")
        try:
            valid = _password_hasher.verify(user.password_hash, password)
        except VerifyMismatchError as error:
            raise AuthenticationError("invalid email or password") from error
        if not valid:
            raise AuthenticationError("invalid email or password")
        if _password_hasher.check_needs_rehash(user.password_hash):
            user.password_hash = _password_hasher.hash(password)
        created = await self._create_session(user, user_agent=user_agent, ip_address=ip_address)
        await self._repository.commit()
        return created

    async def authenticate_token(self, token: str) -> tuple[User, AuthSession] | None:
        now = datetime.now(UTC)
        session = await self._repository.get_active_session_by_token_hash(hash_session_token(token), now)
        if session is None:
            return None
        user = await self._repository.get_user_by_id(session.user_id)
        if user is None:
            return None
        await self._repository.touch_session(session.id)
        await self._repository.commit()
        return user, session

    async def logout(self, session_id: int) -> None:
        await self._repository.revoke_session(session_id)
        await self._repository.commit()

    async def me(self, context: RequestContext) -> AuthMe:
        user = await self._repository.get_user_by_id(context.user_id)
        session = await self._repository.get_session_by_id(context.session_id)
        if user is None or session is None:
            raise AuthenticationError("authentication required")
        return to_auth_me(user, session)

    async def _create_session(self, user: User, *, user_agent: str | None, ip_address: str | None) -> CreatedSession:
        token = secrets.token_urlsafe(48)
        device = await self._repository.get_or_create_device(
            user_id=user.id,
            fingerprint=_device_fingerprint(user_agent, ip_address),
            user_agent=user_agent,
        )
        session = await self._repository.create_session(
            user_id=user.id,
            device_id=device.id,
            token_hash=hash_session_token(token),
            user_agent=user_agent,
            ip_address=ip_address,
            expires_at=datetime.now(UTC) + timedelta(days=SESSION_DAYS),
        )
        return CreatedSession(token=token, user=user, session=session)


def to_auth_me(user: User, session: AuthSession) -> AuthMe:
    return AuthMe(
        user=AuthUserSummary(
            id=user.id,
            email=user.email,
            display_name=user.display_name,
            is_admin=user.is_admin,
        ),
        session=AuthSessionSummary(
            id=session.id,
            device_id=session.device_id,
            expires_at=session.expires_at,
        ),
    )
