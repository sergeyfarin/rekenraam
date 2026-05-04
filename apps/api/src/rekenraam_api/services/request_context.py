from __future__ import annotations

from contextvars import ContextVar
from dataclasses import dataclass
from datetime import datetime


@dataclass(frozen=True)
class RequestContext:
    user_id: int
    session_id: int
    device_id: int | None
    request_id: str
    request_timestamp: datetime


_request_context: ContextVar[RequestContext | None] = ContextVar("request_context", default=None)


def set_request_context(context: RequestContext | None) -> None:
    _request_context.set(context)


def get_request_context() -> RequestContext | None:
    return _request_context.get()
