from __future__ import annotations

from dataclasses import dataclass

from rekenraam_api.repositories.access import AccessRepository
from rekenraam_api.services.request_context import RequestContext


SUPPORTED_V1_BOOK_ID = 1


class AuthorizationError(Exception):
    """Raised when an authenticated request lacks the role to perform an action.

    Intentionally NOT a subclass of ValueError: routes throughout the API catch
    ValueError to convert business-rule failures into 400 Bad Request, which
    would silently swallow authz errors and return 400 instead of 403. The
    global handler in rekenraam_api/app.py maps this to 403.
    """

    pass


class SingleBookNotImplementedError(Exception):
    """Raised when a request targets a non-primary book in the temporary v1 build."""

    def __init__(
        self,
        message: str = f"Multi-book support is not implemented in this v1 API yet. Use book_id={SUPPORTED_V1_BOOK_ID}.",
    ) -> None:
        super().__init__(message)


@dataclass(frozen=True)
class AuditStamp:
    created_by_user_id: int | None
    created_session_id: int | None
    created_device_id: int | None
    created_request_id: str | None


class AccessPolicy:
    def __init__(self, repository: AccessRepository, context: RequestContext | None) -> None:
        self._repository = repository
        self._context = context

    @property
    def context(self) -> RequestContext | None:
        return self._context

    def audit_stamp(self) -> AuditStamp:
        if self._context is None:
            return AuditStamp(None, None, None, None)
        return AuditStamp(
            created_by_user_id=self._context.user_id,
            created_session_id=self._context.session_id,
            created_device_id=self._context.device_id,
            created_request_id=self._context.request_id,
        )

    async def list_readable_book_ids(self) -> list[int]:
        if self._context is None:
            return []
        return await self._repository.list_user_book_ids(self._context.user_id)

    @staticmethod
    def require_supported_book(book_id: int) -> None:
        if book_id != SUPPORTED_V1_BOOK_ID:
            raise SingleBookNotImplementedError()

    async def require_book_role(self, book_id: int, allowed_roles: set[str]) -> str:
        self.require_supported_book(book_id)
        if self._context is None:
            raise AuthorizationError("authentication required")
        role = await self._repository.get_book_role(self._context.user_id, book_id)
        if role not in allowed_roles:
            raise AuthorizationError("book access denied")
        return role

    async def require_book_read(self, book_id: int) -> None:
        await self.require_book_role(book_id, {"owner", "editor", "viewer"})

    async def require_book_write(self, book_id: int) -> None:
        await self.require_book_role(book_id, {"owner", "editor"})
