from __future__ import annotations

from datetime import UTC, datetime
from typing import cast

from sqlalchemy import Select, delete, func, or_, select, update
from sqlalchemy.engine import CursorResult
from sqlalchemy.ext.asyncio import AsyncSession

from rekenraam_api.db.models.access import AuthSession, BookMembership, User
from rekenraam_api.db.models.accounts import Account
from rekenraam_api.db.models.books import Book
from rekenraam_api.db.models.ergonomics import (
    AuditEvent,
    MarkdownNote,
    PayeeDefault,
    TransactionSavedView,
    TransactionTemplate,
    TransactionTemplateSplit,
    UserPreference,
)
from rekenraam_api.db.models.transactions import Transaction


class ErgonomicsRepository:
    def __init__(self, session: AsyncSession) -> None:
        self._session = session

    async def get_user(self, user_id: int) -> User | None:
        return await self._session.get(User, user_id)

    async def get_user_by_email(self, email: str) -> User | None:
        result = await self._session.execute(
            select(User).where(func.lower(User.email) == email.lower())
        )
        return result.scalar_one_or_none()

    async def list_users(self) -> list[User]:
        result = await self._session.execute(select(User).order_by(User.email))
        return list(result.scalars().all())

    async def create_user(
        self, *, email: str, password_hash: str, display_name: str, is_admin: bool
    ) -> User:
        user = User(
            email=email.lower(),
            password_hash=password_hash,
            display_name=display_name,
            is_admin=is_admin,
            is_active=True,
        )
        self._session.add(user)
        await self._session.flush()
        return user

    async def set_user_fields(
        self,
        user: User,
        *,
        display_name: str | None = None,
        is_admin: bool | None = None,
        is_active: bool | None = None,
        password_hash: str | None = None,
    ) -> User:
        if display_name is not None:
            user.display_name = display_name
        if is_admin is not None:
            user.is_admin = is_admin
        if is_active is not None:
            user.is_active = is_active
        if password_hash is not None:
            user.password_hash = password_hash
        user.updated_at = datetime.now(UTC)
        await self._session.flush()
        return user

    async def revoke_user_sessions(self, user_id: int) -> int:
        result = await self._session.execute(
            update(AuthSession)
            .where(AuthSession.user_id == user_id, AuthSession.revoked_at.is_(None))
            .values(revoked_at=datetime.now(UTC))
        )
        await self._session.flush()
        return int(cast(CursorResult[object], result).rowcount or 0)

    async def list_memberships(
        self, user_ids: list[int] | None = None
    ) -> list[tuple[BookMembership, Book]]:
        statement: Select[tuple[BookMembership, Book]] = select(BookMembership, Book).join(
            Book, Book.id == BookMembership.book_id
        )
        if user_ids is not None:
            if not user_ids:
                return []
            statement = statement.where(BookMembership.user_id.in_(user_ids))
        statement = statement.order_by(Book.name)
        result = await self._session.execute(statement)
        return list(result.tuples().all())

    async def set_membership(self, *, user_id: int, book_id: int, role: str) -> BookMembership:
        result = await self._session.execute(
            select(BookMembership).where(
                BookMembership.user_id == user_id, BookMembership.book_id == book_id
            )
        )
        membership = result.scalar_one_or_none()
        if membership is None:
            membership = BookMembership(user_id=user_id, book_id=book_id, role=role)
            self._session.add(membership)
        else:
            membership.role = role
        await self._session.flush()
        return membership

    async def delete_membership(self, *, user_id: int, book_id: int) -> bool:
        result = await self._session.execute(
            delete(BookMembership).where(
                BookMembership.user_id == user_id, BookMembership.book_id == book_id
            )
        )
        await self._session.flush()
        return bool(cast(CursorResult[object], result).rowcount)

    async def get_book(self, book_id: int) -> Book | None:
        return await self._session.get(Book, book_id)

    async def get_account(self, account_id: int) -> Account | None:
        return await self._session.get(Account, account_id)

    async def get_transaction(self, transaction_id: int) -> Transaction | None:
        return await self._session.get(Transaction, transaction_id)

    async def get_preferences(self, user_id: int) -> UserPreference | None:
        result = await self._session.execute(
            select(UserPreference).where(UserPreference.user_id == user_id)
        )
        return result.scalar_one_or_none()

    async def upsert_preferences(
        self,
        *,
        user_id: int,
        default_book_id: int | None,
        locale: str | None,
        date_format: str,
        number_format: str,
        theme: str,
    ) -> UserPreference:
        row = await self.get_preferences(user_id)
        if row is None:
            row = UserPreference(user_id=user_id)
            self._session.add(row)
        row.default_book_id = default_book_id
        row.locale = locale
        row.date_format = date_format
        row.number_format = number_format
        row.theme = theme
        row.updated_at = datetime.now(UTC)
        await self._session.flush()
        return row

    async def list_saved_views(self, *, user_id: int, book_id: int) -> list[TransactionSavedView]:
        result = await self._session.execute(
            select(TransactionSavedView)
            .where(
                TransactionSavedView.book_id == book_id,
                or_(
                    TransactionSavedView.user_id == user_id,
                    TransactionSavedView.is_shared.is_(True),
                ),
            )
            .order_by(TransactionSavedView.name)
        )
        return list(result.scalars().all())

    async def get_saved_view(self, view_id: int) -> TransactionSavedView | None:
        return await self._session.get(TransactionSavedView, view_id)

    async def create_saved_view(
        self, *, user_id: int, book_id: int, name: str, filters_json: str, is_shared: bool
    ) -> TransactionSavedView:
        row = TransactionSavedView(
            user_id=user_id,
            book_id=book_id,
            name=name,
            filters_json=filters_json,
            is_shared=is_shared,
        )
        self._session.add(row)
        await self._session.flush()
        return row

    async def update_saved_view(
        self, row: TransactionSavedView, *, name: str, filters_json: str, is_shared: bool
    ) -> TransactionSavedView:
        row.name = name
        row.filters_json = filters_json
        row.is_shared = is_shared
        row.updated_at = datetime.now(UTC)
        await self._session.flush()
        return row

    async def delete_saved_view(self, row: TransactionSavedView) -> None:
        await self._session.delete(row)
        await self._session.flush()

    async def list_templates(self, *, user_id: int, book_id: int) -> list[TransactionTemplate]:
        result = await self._session.execute(
            select(TransactionTemplate)
            .where(
                TransactionTemplate.book_id == book_id,
                or_(
                    TransactionTemplate.user_id == user_id, TransactionTemplate.is_shared.is_(True)
                ),
            )
            .order_by(TransactionTemplate.name)
        )
        return list(result.scalars().all())

    async def get_template(self, template_id: int) -> TransactionTemplate | None:
        return await self._session.get(TransactionTemplate, template_id)

    async def list_template_splits(self, template_ids: list[int]) -> list[TransactionTemplateSplit]:
        if not template_ids:
            return []
        result = await self._session.execute(
            select(TransactionTemplateSplit)
            .where(TransactionTemplateSplit.template_id.in_(template_ids))
            .order_by(TransactionTemplateSplit.template_id, TransactionTemplateSplit.line_order)
        )
        return list(result.scalars().all())

    async def create_template(
        self,
        *,
        user_id: int,
        book_id: int,
        name: str,
        payee_id: int | None,
        memo: str | None,
        status: str,
        reference: str | None,
        is_shared: bool,
    ) -> TransactionTemplate:
        row = TransactionTemplate(
            user_id=user_id,
            book_id=book_id,
            name=name,
            payee_id=payee_id,
            memo=memo,
            status=status,
            reference=reference,
            is_shared=is_shared,
        )
        self._session.add(row)
        await self._session.flush()
        return row

    async def replace_template_splits(
        self, template_id: int, splits: list[dict[str, int | str | None]]
    ) -> None:
        await self._session.execute(
            delete(TransactionTemplateSplit).where(
                TransactionTemplateSplit.template_id == template_id
            )
        )
        for index, split in enumerate(splits):
            self._session.add(
                TransactionTemplateSplit(template_id=template_id, line_order=index, **split)
            )
        await self._session.flush()

    async def update_template(
        self,
        row: TransactionTemplate,
        *,
        name: str,
        payee_id: int | None,
        memo: str | None,
        status: str,
        reference: str | None,
        is_shared: bool,
    ) -> TransactionTemplate:
        row.name = name
        row.payee_id = payee_id
        row.memo = memo
        row.status = status
        row.reference = reference
        row.is_shared = is_shared
        row.updated_at = datetime.now(UTC)
        await self._session.flush()
        return row

    async def delete_template(self, row: TransactionTemplate) -> None:
        await self._session.delete(row)
        await self._session.flush()

    async def get_explicit_payee_default(
        self, *, user_id: int, book_id: int, payee_id: int, account_id: int | None
    ) -> PayeeDefault | None:
        statement = select(PayeeDefault).where(
            PayeeDefault.user_id == user_id,
            PayeeDefault.book_id == book_id,
            PayeeDefault.payee_id == payee_id,
        )
        if account_id is None:
            statement = statement.where(PayeeDefault.account_id.is_(None))
        else:
            statement = statement.where(
                or_(PayeeDefault.account_id == account_id, PayeeDefault.account_id.is_(None))
            ).order_by(PayeeDefault.account_id.desc().nullslast())
        result = await self._session.execute(statement.limit(1))
        return result.scalar_one_or_none()

    async def upsert_payee_default(
        self,
        *,
        user_id: int,
        book_id: int,
        payee_id: int,
        account_id: int | None,
        category_id: int | None,
        memo: str | None,
    ) -> PayeeDefault:
        row = await self.get_explicit_payee_default(
            user_id=user_id, book_id=book_id, payee_id=payee_id, account_id=account_id
        )
        if row is None or row.account_id != account_id:
            row = PayeeDefault(
                user_id=user_id, book_id=book_id, payee_id=payee_id, account_id=account_id
            )
            self._session.add(row)
        row.category_id = category_id
        row.memo = memo
        row.updated_at = datetime.now(UTC)
        await self._session.flush()
        return row

    async def list_notes(self, *, target_type: str, target_id: int) -> list[MarkdownNote]:
        if target_type == "account":
            statement = select(MarkdownNote).where(MarkdownNote.account_id == target_id)
        else:
            statement = select(MarkdownNote).where(MarkdownNote.transaction_id == target_id)
        result = await self._session.execute(
            statement.order_by(MarkdownNote.pinned.desc(), MarkdownNote.updated_at.desc())
        )
        return list(result.scalars().all())

    async def get_note(self, note_id: int) -> MarkdownNote | None:
        return await self._session.get(MarkdownNote, note_id)

    async def create_note(
        self,
        *,
        book_id: int,
        target_type: str,
        target_id: int,
        title: str,
        body_markdown: str,
        pinned: bool,
        user_id: int | None,
    ) -> MarkdownNote:
        row = MarkdownNote(
            book_id=book_id,
            target_type=target_type,
            account_id=target_id if target_type == "account" else None,
            transaction_id=target_id if target_type == "transaction" else None,
            title=title,
            body_markdown=body_markdown,
            pinned=pinned,
            created_by_user_id=user_id,
            updated_by_user_id=user_id,
        )
        self._session.add(row)
        await self._session.flush()
        return row

    async def update_note(
        self,
        row: MarkdownNote,
        *,
        title: str,
        body_markdown: str,
        pinned: bool,
        user_id: int | None,
    ) -> MarkdownNote:
        row.title = title
        row.body_markdown = body_markdown
        row.pinned = pinned
        row.updated_by_user_id = user_id
        row.updated_at = datetime.now(UTC)
        await self._session.flush()
        return row

    async def delete_note(self, row: MarkdownNote) -> None:
        await self._session.delete(row)
        await self._session.flush()

    async def add_audit_event(
        self,
        *,
        book_id: int | None,
        actor_user_id: int | None,
        actor_session_id: int | None,
        actor_device_id: int | None,
        actor_request_id: str | None,
        event_type: str,
        target_type: str | None,
        target_id: int | None,
        summary: str,
        metadata_json: str | None,
    ) -> AuditEvent:
        row = AuditEvent(
            book_id=book_id,
            actor_user_id=actor_user_id,
            actor_session_id=actor_session_id,
            actor_device_id=actor_device_id,
            actor_request_id=actor_request_id,
            event_type=event_type,
            target_type=target_type,
            target_id=target_id,
            summary=summary,
            metadata_json=metadata_json,
        )
        self._session.add(row)
        await self._session.flush()
        return row

    async def list_audit_events(
        self, *, book_id: int | None = None, limit: int = 100
    ) -> list[AuditEvent]:
        statement = (
            select(AuditEvent)
            .order_by(AuditEvent.created_at.desc(), AuditEvent.id.desc())
            .limit(min(max(limit, 1), 500))
        )
        if book_id is not None:
            statement = statement.where(AuditEvent.book_id == book_id)
        result = await self._session.execute(statement)
        return list(result.scalars().all())

    async def commit(self) -> None:
        await self._session.commit()
