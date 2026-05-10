from __future__ import annotations

import json
from collections import defaultdict
from collections.abc import Sequence
from datetime import date
from typing import TYPE_CHECKING

from argon2 import PasswordHasher
from argon2.exceptions import VerifyMismatchError

from rekenraam_api.db.models.access import BookMembership, User
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
from rekenraam_api.repositories.ergonomics import ErgonomicsRepository
from rekenraam_api.schemas.ergonomics import (
    AdminInviteCreated,
    AdminInviteCreateInput,
    AdminUserCreateInput,
    AdminUserSummary,
    AdminUserUpdateInput,
    AuditEventSummary,
    BookMembershipInput,
    BookMembershipSummary,
    MarkdownNoteInput,
    MarkdownNoteSummary,
    PasswordChangeInput,
    PayeeDefaultInput,
    PayeeDefaultSummary,
    ProfileUpdateInput,
    TransactionSavedViewInput,
    TransactionSavedViewSummary,
    TransactionTemplateInput,
    TransactionTemplateSplitSummary,
    TransactionTemplateSummary,
    UserPreferenceSummary,
    UserPreferenceUpdateInput,
)
from rekenraam_api.schemas.transactions import (
    TransactionListFilters,
    TransactionMutationInput,
    TransactionSplitInput,
)
from rekenraam_api.services.access import AccessPolicy
from rekenraam_api.services.request_context import RequestContext

if TYPE_CHECKING:
    from rekenraam_api.services.auth import AuthService

_password_hasher = PasswordHasher()
VALID_ROLES = {"owner", "editor", "viewer"}
VALID_TX_STATUSES = {"uncleared", "cleared"}


class AdminRequiredError(ValueError):
    pass


class ErgonomicsService:
    def __init__(
        self,
        repository: ErgonomicsRepository,
        access_policy: AccessPolicy,
        auth_service: AuthService | None = None,
    ) -> None:
        self._repository = repository
        self._access_policy = access_policy
        self._auth_service = auth_service

    async def require_admin(self) -> User:
        context = self._require_context()
        user = await self._repository.get_user(context.user_id)
        if user is None or not user.is_active or not user.is_admin:
            raise AdminRequiredError("admin access required")
        return user

    async def list_admin_users(self) -> list[AdminUserSummary]:
        await self.require_admin()
        users = await self._repository.list_users()
        memberships = await self._repository.list_memberships([user.id for user in users])
        by_user: dict[int, list[tuple[BookMembership, Book]]] = defaultdict(list)
        for membership, book in memberships:
            by_user[membership.user_id].append((membership, book))
        return [self._user_summary(user, by_user.get(user.id, [])) for user in users]

    async def create_admin_user(self, input: AdminUserCreateInput) -> AdminUserSummary:
        await self.require_admin()
        email = input.email.strip().lower()
        if await self._repository.get_user_by_email(email) is not None:
            raise ValueError("user email already exists")
        user = await self._repository.create_user(
            email=email,
            password_hash=_password_hasher.hash(input.password),
            display_name=input.display_name.strip(),
            is_admin=input.is_admin,
        )
        for book_id, role in input.memberships:
            await self._set_membership_checked(user.id, int(book_id), str(role))
        await self._audit("admin.user.created", "user", user.id, f"Created user {user.email}", None)
        await self._repository.commit()
        return await self._load_user_summary(user.id)

    async def create_invite(
        self,
        input: AdminInviteCreateInput,
        *,
        user_agent: str | None,
        ip_address: str | None,
    ) -> AdminInviteCreated:
        """Issue a single-use invite token for a new or pending user.

        Idempotent for re-invites: if a user with the email already exists
        and is still pending (`password_hash IS NULL`), reuse the row and
        invalidate any earlier outstanding invites. If the user already
        accepted (has a password set), reject with `user email already exists`
        — the admin reset endpoint is the right tool for those cases.
        """

        admin_user = await self.require_admin()
        if self._auth_service is None:
            # Defensive: callers wired through `get_ergonomics_service` always
            # supply an AuthService. This branch only fires in tests that build
            # the service without the dependency, in which case calling the
            # invite path is a programming error.
            raise RuntimeError("ErgonomicsService.create_invite requires AuthService injection")

        email = input.email.strip().lower()
        existing = await self._repository.get_user_by_email(email)
        if existing is not None and existing.password_hash is not None:
            raise ValueError("user email already exists")

        if existing is None:
            user = await self._repository.create_user(
                email=email,
                password_hash=None,
                display_name=input.display_name.strip(),
                is_admin=input.is_admin,
                is_active=False,
            )
        else:
            # Pending user already exists (a prior invite that hasn't been
            # accepted). Refresh the metadata in case the admin changed
            # display name or admin flag, but keep is_active=False until
            # acceptance.
            await self._repository.set_user_fields(
                existing,
                display_name=input.display_name.strip(),
                is_admin=input.is_admin,
                is_active=False,
            )
            user = existing

        for book_id, role in input.memberships:
            await self._set_membership_checked(user.id, int(book_id), str(role))

        issued = await self._auth_service.issue_invite_token(
            user_id=user.id,
            invited_by_user_id=admin_user.id,
            user_agent=user_agent,
            ip_address=ip_address,
        )
        await self._audit(
            "admin.user.invited",
            "user",
            user.id,
            f"Invited {user.email}",
            {"expires_at": issued.expires_at.isoformat()},
        )
        await self._repository.commit()
        summary = await self._load_user_summary(user.id)
        return AdminInviteCreated(user=summary, token=issued.token, expires_at=issued.expires_at)

    async def update_admin_user(
        self, user_id: int, input: AdminUserUpdateInput
    ) -> AdminUserSummary:
        await self.require_admin()
        user = await self._require_user(user_id)
        next_is_admin = user.is_admin if input.is_admin is None else input.is_admin
        next_is_active = user.is_active if input.is_active is None else input.is_active
        await self._ensure_admin_survives(
            user, next_is_admin=next_is_admin, next_is_active=next_is_active
        )
        updated = await self._repository.set_user_fields(
            user,
            display_name=input.display_name.strip() if input.display_name is not None else None,
            is_admin=input.is_admin,
            is_active=input.is_active,
            mfa_required=input.mfa_required,
        )
        if input.is_active is False:
            await self._repository.revoke_user_sessions(user_id)
        await self._audit(
            "admin.user.updated", "user", user_id, f"Updated user {updated.email}", None
        )
        await self._repository.commit()
        return await self._load_user_summary(user_id)

    async def reset_user_password(self, user_id: int, password: str) -> AdminUserSummary:
        await self.require_admin()
        user = await self._require_user(user_id)
        await self._repository.set_user_fields(user, password_hash=_password_hasher.hash(password))
        await self._repository.revoke_user_sessions(user_id)
        await self._audit(
            "admin.user.password_reset", "user", user_id, f"Reset password for {user.email}", None
        )
        await self._repository.commit()
        return await self._load_user_summary(user_id)

    async def revoke_user_sessions(self, user_id: int) -> int:
        await self.require_admin()
        user = await self._require_user(user_id)
        count = await self._repository.revoke_user_sessions(user_id)
        await self._audit(
            "admin.user.sessions_revoked",
            "user",
            user_id,
            f"Revoked sessions for {user.email}",
            {"count": count},
        )
        await self._repository.commit()
        return count

    async def set_user_membership(
        self, user_id: int, input: BookMembershipInput
    ) -> AdminUserSummary:
        await self.require_admin()
        await self._require_user(user_id)
        await self._set_membership_checked(user_id, input.book_id, input.role)
        await self._audit(
            "admin.book_role.updated",
            "user",
            user_id,
            f"Set book {input.book_id} role to {input.role}",
            {"book_id": input.book_id, "role": input.role},
        )
        await self._repository.commit()
        return await self._load_user_summary(user_id)

    async def delete_user_membership(self, user_id: int, book_id: int) -> AdminUserSummary:
        await self.require_admin()
        await self._require_user(user_id)
        await self._repository.delete_membership(user_id=user_id, book_id=book_id)
        await self._audit(
            "admin.book_role.deleted",
            "user",
            user_id,
            f"Removed book {book_id} role",
            {"book_id": book_id},
        )
        await self._repository.commit()
        return await self._load_user_summary(user_id)

    async def get_preferences(self) -> UserPreferenceSummary:
        context = self._require_context()
        row = await self._repository.get_preferences(context.user_id)
        if row is None:
            readable = await self._access_policy.list_readable_book_ids()
            return UserPreferenceSummary(
                default_book_id=readable[0] if readable else None,
                locale=None,
                date_format="iso",
                number_format="system",
                theme="system",
            )
        return self._preferences_summary(row)

    async def update_preferences(self, input: UserPreferenceUpdateInput) -> UserPreferenceSummary:
        context = self._require_context()
        current = await self.get_preferences()
        default_book_id = (
            input.default_book_id if input.default_book_id is not None else current.default_book_id
        )
        if default_book_id is not None:
            await self._access_policy.require_book_read(default_book_id)
        row = await self._repository.upsert_preferences(
            user_id=context.user_id,
            default_book_id=default_book_id,
            locale=input.locale if input.locale is not None else current.locale,
            date_format=input.date_format or current.date_format,
            number_format=input.number_format or current.number_format,
            theme=input.theme or current.theme,
        )
        await self._repository.commit()
        return self._preferences_summary(row)

    async def update_profile(self, input: ProfileUpdateInput) -> User:
        context = self._require_context()
        user = await self._require_user(context.user_id)
        updated = await self._repository.set_user_fields(
            user, display_name=input.display_name.strip()
        )
        await self._repository.commit()
        return updated

    async def change_password(self, input: PasswordChangeInput) -> None:
        context = self._require_context()
        user = await self._require_user(context.user_id)
        if user.password_hash is None:
            raise ValueError("password is not set")
        try:
            valid = _password_hasher.verify(user.password_hash, input.current_password)
        except VerifyMismatchError as error:
            raise ValueError("current password is incorrect") from error
        if not valid:
            raise ValueError("current password is incorrect")
        await self._repository.set_user_fields(
            user, password_hash=_password_hasher.hash(input.new_password)
        )
        await self._audit("auth.password_changed", "user", user.id, "Changed own password", None)
        await self._repository.commit()

    async def list_saved_views(self, book_id: int) -> list[TransactionSavedViewSummary]:
        context = self._require_context()
        await self._access_policy.require_book_read(book_id)
        rows = await self._repository.list_saved_views(user_id=context.user_id, book_id=book_id)
        return [self._saved_view_summary(row) for row in rows]

    async def create_saved_view(
        self, input: TransactionSavedViewInput
    ) -> TransactionSavedViewSummary:
        context = self._require_context()
        await self._access_policy.require_book_read(input.book_id)
        row = await self._repository.create_saved_view(
            user_id=context.user_id,
            book_id=input.book_id,
            name=input.name.strip(),
            filters_json=input.filters.model_dump_json(),
            is_shared=input.is_shared,
        )
        await self._repository.commit()
        return self._saved_view_summary(row)

    async def update_saved_view(
        self, view_id: int, input: TransactionSavedViewInput
    ) -> TransactionSavedViewSummary | None:
        row = await self._repository.get_saved_view(view_id)
        if row is None:
            return None
        await self._ensure_owned_or_admin(row.user_id, row.book_id)
        row = await self._repository.update_saved_view(
            row,
            name=input.name.strip(),
            filters_json=input.filters.model_dump_json(),
            is_shared=input.is_shared,
        )
        await self._repository.commit()
        return self._saved_view_summary(row)

    async def delete_saved_view(self, view_id: int) -> bool:
        row = await self._repository.get_saved_view(view_id)
        if row is None:
            return False
        await self._ensure_owned_or_admin(row.user_id, row.book_id)
        await self._repository.delete_saved_view(row)
        await self._repository.commit()
        return True

    async def list_templates(self, book_id: int) -> list[TransactionTemplateSummary]:
        context = self._require_context()
        await self._access_policy.require_book_read(book_id)
        rows = await self._repository.list_templates(user_id=context.user_id, book_id=book_id)
        return await self._template_summaries(rows)

    async def create_template(self, input: TransactionTemplateInput) -> TransactionTemplateSummary:
        context = self._require_context()
        await self._access_policy.require_book_write(input.book_id)
        self._validate_template_input(input)
        row = await self._repository.create_template(
            user_id=context.user_id,
            book_id=input.book_id,
            name=input.name.strip(),
            payee_id=input.payee_id,
            memo=input.memo,
            status=input.status,
            reference=input.reference,
            is_shared=input.is_shared,
        )
        await self._repository.replace_template_splits(
            row.id, [split.model_dump() for split in input.splits]
        )
        await self._repository.commit()
        return (await self._template_summaries([row]))[0]

    async def update_template(
        self, template_id: int, input: TransactionTemplateInput
    ) -> TransactionTemplateSummary | None:
        row = await self._repository.get_template(template_id)
        if row is None:
            return None
        await self._ensure_owned_or_admin(row.user_id, row.book_id)
        await self._access_policy.require_book_write(input.book_id)
        self._validate_template_input(input)
        row = await self._repository.update_template(
            row,
            name=input.name.strip(),
            payee_id=input.payee_id,
            memo=input.memo,
            status=input.status,
            reference=input.reference,
            is_shared=input.is_shared,
        )
        await self._repository.replace_template_splits(
            row.id, [split.model_dump() for split in input.splits]
        )
        await self._repository.commit()
        return (await self._template_summaries([row]))[0]

    async def delete_template(self, template_id: int) -> bool:
        row = await self._repository.get_template(template_id)
        if row is None:
            return False
        await self._ensure_owned_or_admin(row.user_id, row.book_id)
        await self._repository.delete_template(row)
        await self._repository.commit()
        return True

    async def template_to_transaction_input(
        self, template_id: int, txn_date: date
    ) -> TransactionMutationInput | None:
        row = await self._repository.get_template(template_id)
        if row is None:
            return None
        await self._access_policy.require_book_write(row.book_id)
        splits = await self._repository.list_template_splits([row.id])
        return TransactionMutationInput(
            book_id=row.book_id,
            txn_date=txn_date,
            payee_id=row.payee_id,
            memo=row.memo,
            status=row.status,
            reference=row.reference,
            import_id=None,
            splits=tuple(self._split_input(split) for split in splits),
        )

    async def get_explicit_payee_default(
        self, *, book_id: int, payee_id: int, account_id: int | None
    ) -> PayeeDefaultSummary | None:
        context = self._require_context()
        await self._access_policy.require_book_read(book_id)
        row = await self._repository.get_explicit_payee_default(
            user_id=context.user_id,
            book_id=book_id,
            payee_id=payee_id,
            account_id=account_id,
        )
        return self._payee_default_summary(row) if row is not None else None

    async def upsert_payee_default(self, input: PayeeDefaultInput) -> PayeeDefaultSummary:
        context = self._require_context()
        await self._access_policy.require_book_write(input.book_id)
        row = await self._repository.upsert_payee_default(
            user_id=context.user_id,
            book_id=input.book_id,
            payee_id=input.payee_id,
            account_id=input.account_id,
            category_id=input.category_id,
            memo=input.memo,
        )
        await self._repository.commit()
        return self._payee_default_summary(row)

    async def list_account_notes(self, account_id: int) -> list[MarkdownNoteSummary]:
        account = await self._require_account(account_id)
        await self._access_policy.require_book_read(account.book_id)
        return [
            self._note_summary(row)
            for row in await self._repository.list_notes(
                target_type="account", target_id=account_id
            )
        ]

    async def list_transaction_notes(self, transaction_id: int) -> list[MarkdownNoteSummary]:
        transaction = await self._require_transaction(transaction_id)
        await self._access_policy.require_book_read(transaction.book_id)
        return [
            self._note_summary(row)
            for row in await self._repository.list_notes(
                target_type="transaction", target_id=transaction_id
            )
        ]

    async def create_account_note(
        self, account_id: int, input: MarkdownNoteInput
    ) -> MarkdownNoteSummary:
        account = await self._require_account(account_id)
        await self._access_policy.require_book_write(account.book_id)
        context = self._require_context()
        row = await self._repository.create_note(
            book_id=account.book_id,
            target_type="account",
            target_id=account_id,
            title=input.title.strip(),
            body_markdown=input.body_markdown,
            pinned=input.pinned,
            user_id=context.user_id,
        )
        await self._repository.commit()
        return self._note_summary(row)

    async def create_transaction_note(
        self, transaction_id: int, input: MarkdownNoteInput
    ) -> MarkdownNoteSummary:
        transaction = await self._require_transaction(transaction_id)
        await self._access_policy.require_book_write(transaction.book_id)
        context = self._require_context()
        row = await self._repository.create_note(
            book_id=transaction.book_id,
            target_type="transaction",
            target_id=transaction_id,
            title=input.title.strip(),
            body_markdown=input.body_markdown,
            pinned=input.pinned,
            user_id=context.user_id,
        )
        await self._repository.commit()
        return self._note_summary(row)

    async def update_note(
        self, note_id: int, input: MarkdownNoteInput
    ) -> MarkdownNoteSummary | None:
        row = await self._repository.get_note(note_id)
        if row is None:
            return None
        await self._access_policy.require_book_write(row.book_id)
        context = self._require_context()
        row = await self._repository.update_note(
            row,
            title=input.title.strip(),
            body_markdown=input.body_markdown,
            pinned=input.pinned,
            user_id=context.user_id,
        )
        await self._repository.commit()
        return self._note_summary(row)

    async def delete_note(self, note_id: int) -> bool:
        row = await self._repository.get_note(note_id)
        if row is None:
            return False
        await self._access_policy.require_book_write(row.book_id)
        await self._repository.delete_note(row)
        await self._repository.commit()
        return True

    async def list_audit_events(
        self, *, book_id: int | None, limit: int
    ) -> list[AuditEventSummary]:
        await self.require_admin()
        rows = await self._repository.list_audit_events(book_id=book_id, limit=limit)
        return [self._audit_summary(row) for row in rows]

    async def _load_user_summary(self, user_id: int) -> AdminUserSummary:
        user = await self._require_user(user_id)
        memberships = await self._repository.list_memberships([user_id])
        return self._user_summary(user, memberships)

    async def _require_user(self, user_id: int) -> User:
        user = await self._repository.get_user(user_id)
        if user is None:
            raise ValueError("user not found")
        return user

    async def _require_account(self, account_id: int):
        account = await self._repository.get_account(account_id)
        if account is None:
            raise ValueError("account not found")
        return account

    async def _require_transaction(self, transaction_id: int):
        transaction = await self._repository.get_transaction(transaction_id)
        if transaction is None:
            raise ValueError("transaction not found")
        return transaction

    def _require_context(self) -> RequestContext:
        context = self._access_policy.context
        if context is None:
            raise ValueError("authentication required")
        return context

    async def _set_membership_checked(self, user_id: int, book_id: int, role: str) -> None:
        if role not in VALID_ROLES:
            raise ValueError("role must be owner, editor, or viewer")
        if await self._repository.get_book(book_id) is None:
            raise ValueError("book not found")
        await self._repository.set_membership(user_id=user_id, book_id=book_id, role=role)

    async def _ensure_admin_survives(
        self, user: User, *, next_is_admin: bool, next_is_active: bool
    ) -> None:
        if user.is_admin and user.is_active and (not next_is_admin or not next_is_active):
            users = await self._repository.list_users()
            other_active_admins = [
                candidate
                for candidate in users
                if candidate.id != user.id and candidate.is_admin and candidate.is_active
            ]
            if not other_active_admins:
                raise ValueError("at least one active admin is required")

    async def _ensure_owned_or_admin(self, owner_user_id: int, book_id: int) -> None:
        context = self._require_context()
        if owner_user_id == context.user_id:
            await self._access_policy.require_book_read(book_id)
            return
        await self.require_admin()

    def _validate_template_input(self, input: TransactionTemplateInput) -> None:
        if input.status not in VALID_TX_STATUSES:
            raise ValueError("template status must be uncleared or cleared")
        if len(input.splits) < 2:
            raise ValueError("template requires at least two splits")

    async def _audit(
        self,
        event_type: str,
        target_type: str | None,
        target_id: int | None,
        summary: str,
        metadata: dict[str, object] | None,
        book_id: int | None = None,
    ) -> None:
        context = self._access_policy.context
        await self._repository.add_audit_event(
            book_id=book_id,
            actor_user_id=context.user_id if context is not None else None,
            actor_session_id=context.session_id if context is not None else None,
            actor_device_id=context.device_id if context is not None else None,
            actor_request_id=context.request_id if context is not None else None,
            event_type=event_type,
            target_type=target_type,
            target_id=target_id,
            summary=summary,
            metadata_json=json.dumps(metadata, sort_keys=True) if metadata is not None else None,
        )

    @staticmethod
    def _user_summary(
        user: User, memberships: Sequence[tuple[BookMembership, Book]]
    ) -> AdminUserSummary:
        return AdminUserSummary(
            id=user.id,
            email=user.email,
            display_name=user.display_name,
            is_admin=user.is_admin,
            is_active=user.is_active,
            mfa_required=user.mfa_required,
            created_at=user.created_at,
            updated_at=user.updated_at,
            memberships=tuple(
                BookMembershipSummary(
                    book_id=membership.book_id,
                    book_slug=book.slug,
                    book_name=book.name,
                    role=membership.role,
                )
                for membership, book in memberships
            ),
        )

    @staticmethod
    def _preferences_summary(row: UserPreference) -> UserPreferenceSummary:
        return UserPreferenceSummary(
            default_book_id=row.default_book_id,
            locale=row.locale,
            date_format=row.date_format,
            number_format=row.number_format,
            theme=row.theme,
        )

    @staticmethod
    def _saved_view_summary(row: TransactionSavedView) -> TransactionSavedViewSummary:
        return TransactionSavedViewSummary(
            id=row.id,
            user_id=row.user_id,
            book_id=row.book_id,
            name=row.name,
            filters=TransactionListFilters.model_validate_json(row.filters_json),
            is_shared=row.is_shared,
            created_at=row.created_at,
            updated_at=row.updated_at,
        )

    async def _template_summaries(
        self, rows: list[TransactionTemplate]
    ) -> list[TransactionTemplateSummary]:
        split_rows = await self._repository.list_template_splits([row.id for row in rows])
        by_template: dict[int, list[TransactionTemplateSplit]] = defaultdict(list)
        for split in split_rows:
            by_template[split.template_id].append(split)
        return [
            TransactionTemplateSummary(
                id=row.id,
                user_id=row.user_id,
                book_id=row.book_id,
                name=row.name,
                payee_id=row.payee_id,
                memo=row.memo,
                status=row.status,
                reference=row.reference,
                is_shared=row.is_shared,
                created_at=row.created_at,
                updated_at=row.updated_at,
                splits=tuple(
                    self._template_split_summary(split) for split in by_template.get(row.id, [])
                ),
            )
            for row in rows
        ]

    @staticmethod
    def _template_split_summary(row: TransactionTemplateSplit) -> TransactionTemplateSplitSummary:
        return TransactionTemplateSplitSummary(
            id=row.id,
            line_order=row.line_order,
            account_id=row.account_id,
            commodity_id=row.commodity_id,
            amount_minor=row.amount_minor,
            category_id=row.category_id,
            tag_id=row.tag_id,
            person_id=row.person_id,
            project_id=row.project_id,
            share_bps=row.share_bps,
            memo=row.memo,
        )

    @staticmethod
    def _split_input(row: TransactionTemplateSplit) -> TransactionSplitInput:
        return TransactionSplitInput(
            account_id=row.account_id,
            commodity_id=row.commodity_id,
            amount_minor=row.amount_minor,
            category_id=row.category_id,
            tag_id=row.tag_id,
            person_id=row.person_id,
            project_id=row.project_id,
            share_bps=row.share_bps,
            memo=row.memo,
        )

    @staticmethod
    def _payee_default_summary(row: PayeeDefault) -> PayeeDefaultSummary:
        return PayeeDefaultSummary(
            id=row.id,
            user_id=row.user_id,
            book_id=row.book_id,
            payee_id=row.payee_id,
            account_id=row.account_id,
            category_id=row.category_id,
            memo=row.memo,
            created_at=row.created_at,
            updated_at=row.updated_at,
        )

    @staticmethod
    def _note_summary(row: MarkdownNote) -> MarkdownNoteSummary:
        return MarkdownNoteSummary(
            id=row.id,
            book_id=row.book_id,
            target_type=row.target_type,
            account_id=row.account_id,
            transaction_id=row.transaction_id,
            title=row.title,
            body_markdown=row.body_markdown,
            pinned=row.pinned,
            created_at=row.created_at,
            updated_at=row.updated_at,
            created_by_user_id=row.created_by_user_id,
            updated_by_user_id=row.updated_by_user_id,
        )

    @staticmethod
    def _audit_summary(row: AuditEvent) -> AuditEventSummary:
        return AuditEventSummary(
            id=row.id,
            book_id=row.book_id,
            actor_user_id=row.actor_user_id,
            event_type=row.event_type,
            target_type=row.target_type,
            target_id=row.target_id,
            summary=row.summary,
            metadata=json.loads(row.metadata_json) if row.metadata_json else None,
            created_at=row.created_at,
        )
