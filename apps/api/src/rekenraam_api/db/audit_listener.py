"""SQLAlchemy session listener that records every mutation of an audit-logged
business table into the ``audit_log`` table.

Each watched ORM class opts in by setting ``__audit_logged__ = True`` on the
class body. Inserts, updates, and deletes that flow through the SQLAlchemy
session generate one ``AuditLogEntry`` row each, with full before/after JSON
snapshots and attribution stamped from the request-context ``ContextVar``.

The listener is the v1 stand-in for the v2 trigger-based approach described in
``docs/architecture/accounting-foundations.md``. It catches everything that
goes through SQLAlchemy but not direct-SQL writes; that gap is intentional for
v1 and is what triggers will close in v2.
"""

from __future__ import annotations

from dataclasses import dataclass
from datetime import date, datetime
from decimal import Decimal
from typing import Any, cast

from sqlalchemy import event, inspect
from sqlalchemy.orm import Session

from rekenraam_api.db.models.ergonomics import AuditLogEntry
from rekenraam_api.services.request_context import get_request_context


@dataclass
class _PendingEntry:
    """A queued audit row waiting for the post-flush PK assignment."""

    instance: object
    op: str
    before_state: dict[str, Any] | None
    after_state: dict[str, Any] | None


_PENDING_KEY = "_rekenraam_audit_pending"
_registered = False


def _is_audit_logged(instance: object) -> bool:
    """Return True if the instance belongs to an audit-logged class."""
    cls = type(instance)
    return bool(getattr(cls, "__audit_logged__", False))


def _coerce_json(value: object) -> object:
    """Convert a column value to something JSON-encodable."""
    if value is None or isinstance(value, (bool, int, float, str)):
        return value
    if isinstance(value, Decimal):
        return str(value)
    if isinstance(value, (datetime, date)):
        return value.isoformat()
    if isinstance(value, bytes):
        return value.hex()
    if isinstance(value, (list, tuple)):
        items: list[object] = list(cast(list[Any], value))
        return [_coerce_json(item) for item in items]
    if isinstance(value, dict):
        mapping = cast(dict[object, object], value)
        return {str(k): _coerce_json(v) for k, v in mapping.items()}
    return str(value)


def _mapper_for(instance: object) -> Any:
    """Return the SQLAlchemy Mapper for an instance, or None if unmapped."""
    return inspect(type(instance), raiseerr=False)


def _column_attribute_keys(instance: object) -> list[str]:
    """Return Python attribute names for the instance's mapped columns.

    Using ``mapper.column_attrs`` (rather than ``mapper.columns``) guarantees
    we iterate by Python attribute name. The two differ for columns where
    ``mapped_column("metadata", Text)`` renames the DB column independently
    of the attribute (e.g. ``Payee.metadata_text``).
    """
    mapper = _mapper_for(instance)
    if mapper is None:
        return []
    return [cast(str, attr.key) for attr in mapper.column_attrs]


def _snapshot_state(instance: object) -> dict[str, Any]:
    """Build a JSON-safe dict of all mapped column values on an instance."""
    state: dict[str, Any] = {}
    for key in _column_attribute_keys(instance):
        try:
            raw = getattr(instance, key)
        except AttributeError:
            continue
        state[key] = _coerce_json(raw)
    return state


def _snapshot_committed(instance: object) -> dict[str, Any]:
    """Build a JSON-safe dict from the instance's last-loaded (pre-edit) state."""
    inst_state = inspect(instance, raiseerr=False)
    if inst_state is None:
        return {}
    committed = cast(dict[str, Any], inst_state.committed_state)
    result: dict[str, Any] = {}
    for key in _column_attribute_keys(instance):
        if key in committed:
            result[key] = _coerce_json(committed[key])
        else:
            try:
                result[key] = _coerce_json(getattr(instance, key))
            except AttributeError:
                continue
    return result


def _primary_key_text(instance: object) -> str:
    """Render the instance's primary key as a stable string."""
    state = inspect(instance, raiseerr=False)
    if state is not None:
        identity = state.identity
        if identity is not None:
            return ",".join(str(v) for v in identity)
    mapper = _mapper_for(instance)
    if mapper is None:
        return ""
    values: list[object] = []
    for col in mapper.primary_key:
        key = cast(str, col.key)
        values.append(getattr(instance, key, None))
    if all(v is None for v in values):
        return ""
    return ",".join("" if v is None else str(v) for v in values)


def _table_name(instance: object) -> str:
    """Best-effort lookup of __tablename__ on the instance class."""
    cls = type(instance)
    raw = getattr(cls, "__tablename__", None)
    return str(raw) if raw is not None else cls.__name__


def _build_audit_entry(pending: _PendingEntry) -> AuditLogEntry:
    """Materialize an AuditLogEntry from a queued pending record."""
    context = get_request_context()
    return AuditLogEntry(
        table_name=_table_name(pending.instance),
        row_pk=_primary_key_text(pending.instance),
        op=pending.op,
        before_state=pending.before_state,
        after_state=pending.after_state,
        actor_user_id=context.user_id if context is not None else None,
        actor_session_id=context.session_id if context is not None else None,
        actor_device_id=context.device_id if context is not None else None,
        actor_request_id=context.request_id if context is not None else None,
    )


def _on_before_flush(session: Session, _flush_context: Any, _instances: Any) -> None:
    """Queue audit rows for every new/dirty/deleted audited instance."""
    pending: list[_PendingEntry] = []

    for instance in session.new:
        if not _is_audit_logged(instance):
            continue
        pending.append(
            _PendingEntry(
                instance=instance,
                op="insert",
                before_state=None,
                after_state=_snapshot_state(instance),
            )
        )

    for instance in session.dirty:
        if not _is_audit_logged(instance):
            continue
        if not session.is_modified(instance, include_collections=False):
            continue
        pending.append(
            _PendingEntry(
                instance=instance,
                op="update",
                before_state=_snapshot_committed(instance),
                after_state=_snapshot_state(instance),
            )
        )

    for instance in session.deleted:
        if not _is_audit_logged(instance):
            continue
        pending.append(
            _PendingEntry(
                instance=instance,
                op="delete",
                before_state=_snapshot_state(instance),
                after_state=None,
            )
        )

    if pending:
        session.info[_PENDING_KEY] = pending


def _on_after_flush(session: Session, _flush_context: Any) -> None:
    """Insert the queued audit rows now that PKs are assigned."""
    pending_obj = session.info.pop(_PENDING_KEY, None)
    if not pending_obj:
        return
    pending = cast(list[_PendingEntry], pending_obj)
    for record in pending:
        if record.op == "insert":
            # PK is now assigned; rebuild after_state to capture the assigned id.
            record.after_state = _snapshot_state(record.instance)
        entry = _build_audit_entry(record)
        session.add(entry)


def install_audit_listener() -> None:
    """Register the audit listener on the SQLAlchemy ``Session`` class.

    Idempotent: safe to call from import-time as well as from tests that
    rebuild engines. Listeners are attached to the base ``Session`` class so
    every ``async_sessionmaker``-created session inherits them.
    """
    global _registered
    if _registered:
        return
    event.listen(Session, "before_flush", _on_before_flush)
    event.listen(Session, "after_flush", _on_after_flush)
    _registered = True
