from __future__ import annotations

from datetime import UTC, datetime
from typing import cast

from sqlalchemy import Select, delete, func, select, update
from sqlalchemy.engine import CursorResult
from sqlalchemy.ext.asyncio import AsyncSession

from rekenraam_api.db.models.access import (
    AuthSession,
    BookMembership,
    MfaChallenge,
    MfaRecoveryCode,
    PasswordResetToken,
    User,
    UserDevice,
    UserInvite,
    UserMfaTotp,
)
from rekenraam_api.db.models.books import Book
from rekenraam_api.db.models.ergonomics import AuditEvent


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

        book_ids = list(
            (await self._session.execute(select(Book.id).order_by(Book.id))).scalars().all()
        )
        for book_id in book_ids:
            self._session.add(BookMembership(user_id=user.id, book_id=book_id, role="owner"))
        await self._session.flush()
        return user

    async def get_or_create_device(
        self, *, user_id: int, fingerprint: str, user_agent: str | None
    ) -> UserDevice:
        statement: Select[tuple[UserDevice]] = select(UserDevice).where(
            UserDevice.user_id == user_id,
            UserDevice.device_fingerprint == fingerprint,
        )
        result = await self._session.execute(statement)
        device = result.scalar_one_or_none()
        if device is None:
            device = UserDevice(
                user_id=user_id, device_fingerprint=fingerprint, user_agent=user_agent
            )
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

    async def get_confirmed_totp(self, user_id: int) -> UserMfaTotp | None:
        result = await self._session.execute(
            select(UserMfaTotp).where(
                UserMfaTotp.user_id == user_id, UserMfaTotp.confirmed_at.is_not(None)
            )
        )
        return result.scalar_one_or_none()

    async def get_totp(self, user_id: int) -> UserMfaTotp | None:
        result = await self._session.execute(
            select(UserMfaTotp).where(UserMfaTotp.user_id == user_id)
        )
        return result.scalar_one_or_none()

    async def upsert_totp_secret(
        self, *, user_id: int, secret_ciphertext: str, confirmed_at: datetime | None
    ) -> UserMfaTotp:
        row = await self.get_totp(user_id)
        if row is None:
            row = UserMfaTotp(
                user_id=user_id, secret_ciphertext=secret_ciphertext, confirmed_at=confirmed_at
            )
            self._session.add(row)
        else:
            row.secret_ciphertext = secret_ciphertext
            row.confirmed_at = confirmed_at
            row.updated_at = datetime.now(UTC)
        await self._session.flush()
        return row

    async def delete_mfa(self, user_id: int) -> None:
        await self._session.execute(
            delete(MfaRecoveryCode).where(MfaRecoveryCode.user_id == user_id)
        )
        await self._session.execute(delete(UserMfaTotp).where(UserMfaTotp.user_id == user_id))
        await self._session.flush()

    async def replace_recovery_codes(self, *, user_id: int, code_hashes: list[str]) -> None:
        await self._session.execute(
            delete(MfaRecoveryCode).where(MfaRecoveryCode.user_id == user_id)
        )
        for code_hash in code_hashes:
            self._session.add(MfaRecoveryCode(user_id=user_id, code_hash=code_hash))
        await self._session.flush()

    async def consume_recovery_code(self, *, user_id: int, code_hash: str) -> bool:
        result = await self._session.execute(
            select(MfaRecoveryCode).where(
                MfaRecoveryCode.user_id == user_id,
                MfaRecoveryCode.code_hash == code_hash,
                MfaRecoveryCode.used_at.is_(None),
            )
        )
        row = result.scalar_one_or_none()
        if row is None:
            return False
        row.used_at = datetime.now(UTC)
        await self._session.flush()
        return True

    async def count_unused_recovery_codes(self, user_id: int) -> int:
        result = await self._session.execute(
            select(func.count())
            .select_from(MfaRecoveryCode)
            .where(
                MfaRecoveryCode.user_id == user_id,
                MfaRecoveryCode.used_at.is_(None),
            )
        )
        return int(result.scalar_one())

    async def create_mfa_challenge(
        self,
        *,
        user_id: int,
        token_hash: str,
        user_agent: str | None,
        ip_address: str | None,
        expires_at: datetime,
    ) -> MfaChallenge:
        row = MfaChallenge(
            user_id=user_id,
            token_hash=token_hash,
            user_agent=user_agent,
            ip_address=ip_address,
            expires_at=expires_at,
        )
        self._session.add(row)
        await self._session.flush()
        return row

    async def get_active_mfa_challenge(self, token_hash: str, now: datetime) -> MfaChallenge | None:
        result = await self._session.execute(
            select(MfaChallenge).where(
                MfaChallenge.token_hash == token_hash,
                MfaChallenge.used_at.is_(None),
                MfaChallenge.expires_at > now,
            )
        )
        return result.scalar_one_or_none()

    async def mark_mfa_challenge_used(self, challenge: MfaChallenge) -> None:
        challenge.used_at = datetime.now(UTC)
        await self._session.flush()

    async def increment_mfa_challenge_attempts(self, challenge: MfaChallenge) -> None:
        challenge.attempts += 1
        await self._session.flush()

    async def create_password_reset_token(
        self,
        *,
        user_id: int,
        token_hash: str,
        user_agent: str | None,
        ip_address: str | None,
        expires_at: datetime,
    ) -> PasswordResetToken:
        row = PasswordResetToken(
            user_id=user_id,
            token_hash=token_hash,
            user_agent=user_agent,
            ip_address=ip_address,
            expires_at=expires_at,
        )
        self._session.add(row)
        await self._session.flush()
        return row

    async def get_active_password_reset_token(
        self, token_hash: str, now: datetime
    ) -> PasswordResetToken | None:
        result = await self._session.execute(
            select(PasswordResetToken).where(
                PasswordResetToken.token_hash == token_hash,
                PasswordResetToken.used_at.is_(None),
                PasswordResetToken.expires_at > now,
            )
        )
        return result.scalar_one_or_none()

    async def mark_password_reset_token_used(self, token: PasswordResetToken) -> None:
        token.used_at = datetime.now(UTC)
        await self._session.flush()

    async def update_user_password_hash(self, user: User, *, password_hash: str) -> None:
        user.password_hash = password_hash
        user.updated_at = datetime.now(UTC)
        await self._session.flush()

    async def create_user_invite(
        self,
        *,
        user_id: int,
        invited_by_user_id: int | None,
        token_hash: str,
        user_agent: str | None,
        ip_address: str | None,
        expires_at: datetime,
    ) -> UserInvite:
        row = UserInvite(
            user_id=user_id,
            invited_by_user_id=invited_by_user_id,
            token_hash=token_hash,
            user_agent=user_agent,
            ip_address=ip_address,
            expires_at=expires_at,
        )
        self._session.add(row)
        await self._session.flush()
        return row

    async def get_active_user_invite(self, token_hash: str, now: datetime) -> UserInvite | None:
        result = await self._session.execute(
            select(UserInvite).where(
                UserInvite.token_hash == token_hash,
                UserInvite.used_at.is_(None),
                UserInvite.expires_at > now,
            )
        )
        return result.scalar_one_or_none()

    async def invalidate_outstanding_user_invites(self, user_id: int) -> int:
        """Mark any unused, unexpired invites for the user as used.

        Used when re-inviting a still-pending user — the new token replaces
        any prior token issued for them. Returns the number invalidated for
        audit/logging.
        """

        result = await self._session.execute(
            update(UserInvite)
            .where(
                UserInvite.user_id == user_id,
                UserInvite.used_at.is_(None),
            )
            .values(used_at=datetime.now(UTC))
        )
        await self._session.flush()
        return int(cast(CursorResult[object], result).rowcount or 0)

    async def mark_user_invite_used(self, invite: UserInvite) -> None:
        invite.used_at = datetime.now(UTC)
        await self._session.flush()

    async def audit(
        self,
        *,
        user_id: int | None,
        session_id: int | None,
        device_id: int | None,
        event_type: str,
        summary: str,
        target_type: str | None = None,
        target_id: int | None = None,
    ) -> None:
        self._session.add(
            AuditEvent(
                book_id=None,
                actor_user_id=user_id,
                actor_session_id=session_id,
                actor_device_id=device_id,
                actor_request_id=None,
                event_type=event_type,
                target_type=target_type,
                target_id=target_id,
                summary=summary,
                metadata_json=None,
            )
        )
        await self._session.flush()

    async def get_active_session_by_token_hash(
        self, token_hash: str, now: datetime
    ) -> AuthSession | None:
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
            update(AuthSession)
            .where(AuthSession.id == session_id)
            .values(last_seen_at=datetime.now(UTC))
        )
        await self._session.flush()

    async def revoke_session(self, session_id: int) -> None:
        await self._session.execute(
            update(AuthSession)
            .where(AuthSession.id == session_id)
            .values(revoked_at=datetime.now(UTC))
        )
        await self._session.flush()

    async def revoke_all_user_sessions(self, user_id: int) -> int:
        result = await self._session.execute(
            update(AuthSession)
            .where(AuthSession.user_id == user_id, AuthSession.revoked_at.is_(None))
            .values(revoked_at=datetime.now(UTC))
        )
        await self._session.flush()
        return int(cast(CursorResult[object], result).rowcount or 0)

    async def list_user_book_ids(self, user_id: int) -> list[int]:
        result = await self._session.execute(
            select(BookMembership.book_id)
            .where(BookMembership.user_id == user_id)
            .order_by(BookMembership.book_id)
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
