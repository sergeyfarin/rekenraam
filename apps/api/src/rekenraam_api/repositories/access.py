from __future__ import annotations

from datetime import UTC, datetime

from sqlalchemy import Select, func, select, update
from sqlalchemy.ext.asyncio import AsyncSession

from rekenraam_api.db.models.access import AuthSession, BookMembership, User, UserDevice
from rekenraam_api.db.models.books import Book


class AccessRepository:
    def __init__(self, session: AsyncSession) -> None:
        self._session = session

    async def count_users(self) -> int:
        result = await self._session.execute(select(func.count()).select_from(User))
        return int(result.scalar_one())

    async def get_user_by_email(self, email: str) -> User | None:
        statement: Select[tuple[User]] = select(User).where(func.lower(User.email) == email.lower())
        result = await self._session.execute(statement)
        return result.scalar_one_or_none()

    async def get_user_by_id(self, user_id: int) -> User | None:
        return await self._session.get(User, user_id)

    async def create_admin_user(self, *, email: str, password_hash: str, display_name: str) -> User:
        user = User(
            email=email.lower(),
            password_hash=password_hash,
            display_name=display_name,
            is_admin=True,
        )
        self._session.add(user)
        await self._session.flush()

        book_ids = list((await self._session.execute(select(Book.id).order_by(Book.id))).scalars().all())
        for book_id in book_ids:
            self._session.add(BookMembership(user_id=user.id, book_id=book_id, role="owner"))
        await self._session.flush()
        return user

    async def get_or_create_device(self, *, user_id: int, fingerprint: str, user_agent: str | None) -> UserDevice:
        statement: Select[tuple[UserDevice]] = select(UserDevice).where(
            UserDevice.user_id == user_id,
            UserDevice.device_fingerprint == fingerprint,
        )
        result = await self._session.execute(statement)
        device = result.scalar_one_or_none()
        if device is None:
            device = UserDevice(user_id=user_id, device_fingerprint=fingerprint, user_agent=user_agent)
            self._session.add(device)
        device.user_agent = user_agent
        device.last_seen_at = datetime.now(UTC)
        await self._session.flush()
        return device

    async def create_session(
        self,
        *,
        user_id: int,
        device_id: int | None,
        token_hash: str,
        user_agent: str | None,
        ip_address: str | None,
        expires_at: datetime,
    ) -> AuthSession:
        session = AuthSession(
            user_id=user_id,
            device_id=device_id,
            token_hash=token_hash,
            user_agent=user_agent,
            ip_address=ip_address,
            expires_at=expires_at,
        )
        self._session.add(session)
        await self._session.flush()
        return session

    async def get_active_session_by_token_hash(self, token_hash: str, now: datetime) -> AuthSession | None:
        statement: Select[tuple[AuthSession]] = select(AuthSession).where(
            AuthSession.token_hash == token_hash,
            AuthSession.revoked_at.is_(None),
            AuthSession.expires_at > now,
        )
        result = await self._session.execute(statement)
        return result.scalar_one_or_none()

    async def get_session_by_id(self, session_id: int) -> AuthSession | None:
        return await self._session.get(AuthSession, session_id)

    async def touch_session(self, session_id: int) -> None:
        await self._session.execute(
            update(AuthSession).where(AuthSession.id == session_id).values(last_seen_at=datetime.now(UTC))
        )
        await self._session.flush()

    async def revoke_session(self, session_id: int) -> None:
        await self._session.execute(
            update(AuthSession).where(AuthSession.id == session_id).values(revoked_at=datetime.now(UTC))
        )
        await self._session.flush()

    async def list_user_book_ids(self, user_id: int) -> list[int]:
        result = await self._session.execute(
            select(BookMembership.book_id).where(BookMembership.user_id == user_id).order_by(BookMembership.book_id)
        )
        return list(result.scalars().all())

    async def get_book_role(self, user_id: int, book_id: int) -> str | None:
        result = await self._session.execute(
            select(BookMembership.role).where(
                BookMembership.user_id == user_id,
                BookMembership.book_id == book_id,
            )
        )
        return result.scalar_one_or_none()

    async def commit(self) -> None:
        await self._session.commit()
