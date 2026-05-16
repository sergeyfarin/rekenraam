from datetime import datetime

from sqlalchemy import (
    JSON,
    BigInteger,
    Boolean,
    CheckConstraint,
    DateTime,
    ForeignKey,
    Index,
    Integer,
    String,
    Text,
    UniqueConstraint,
    func,
    text,
)
from sqlalchemy.orm import Mapped, mapped_column

from rekenraam_api.db.base import Base


class UserPreference(Base):
    __tablename__ = "user_preferences"
    __table_args__ = (UniqueConstraint("user_id", name="uq_user_preferences_user_id"),)

    id: Mapped[int] = mapped_column(BigInteger, primary_key=True, autoincrement=True)
    user_id: Mapped[int] = mapped_column(ForeignKey("users.id", ondelete="CASCADE"), nullable=False)
    default_book_id: Mapped[int | None] = mapped_column(ForeignKey("books.id", ondelete="SET NULL"))
    locale: Mapped[str | None] = mapped_column(String(64))
    date_format: Mapped[str] = mapped_column(String(32), nullable=False, server_default="iso")
    number_format: Mapped[str] = mapped_column(String(32), nullable=False, server_default="system")
    theme: Mapped[str] = mapped_column(String(64), nullable=False, server_default="system")
    created_at: Mapped[datetime] = mapped_column(
        DateTime(timezone=True), nullable=False, server_default=func.now()
    )
    updated_at: Mapped[datetime] = mapped_column(
        DateTime(timezone=True), nullable=False, server_default=func.now()
    )


class AuditEvent(Base):
    __tablename__ = "audit_events"
    __table_args__ = (
        Index("ix_audit_events_book_created", "book_id", "created_at"),
        Index("ix_audit_events_actor_created", "actor_user_id", "created_at"),
        Index("ix_audit_events_event_type", "event_type"),
    )

    id: Mapped[int] = mapped_column(BigInteger, primary_key=True, autoincrement=True)
    book_id: Mapped[int | None] = mapped_column(ForeignKey("books.id", ondelete="SET NULL"))
    actor_user_id: Mapped[int | None] = mapped_column(ForeignKey("users.id", ondelete="SET NULL"))
    actor_session_id: Mapped[int | None] = mapped_column(
        ForeignKey("auth_sessions.id", ondelete="SET NULL")
    )
    actor_device_id: Mapped[int | None] = mapped_column(
        ForeignKey("user_devices.id", ondelete="SET NULL")
    )
    actor_request_id: Mapped[str | None] = mapped_column(String(64))
    event_type: Mapped[str] = mapped_column(String(100), nullable=False)
    target_type: Mapped[str | None] = mapped_column(String(64))
    target_id: Mapped[int | None] = mapped_column(BigInteger)
    summary: Mapped[str] = mapped_column(Text, nullable=False)
    metadata_json: Mapped[str | None] = mapped_column(Text)
    created_at: Mapped[datetime] = mapped_column(
        DateTime(timezone=True), nullable=False, server_default=func.now()
    )


class AuditLogEntry(Base):
    """Generic mutation snapshot for every audit-logged business table.

    Sister to :class:`AuditEvent`. ``AuditEvent`` is a curated, service-emitted
    event log ("transaction.created" with a human-readable summary).
    ``AuditLogEntry`` is the raw per-mutation row capture: one row per insert,
    update, or delete observed by the SQLAlchemy ``before_flush`` /
    ``after_flush`` listener. Together they answer "who did what when, and
    what changed exactly."
    """

    __tablename__ = "audit_log"
    __table_args__ = (
        Index("ix_audit_log_table_row", "table_name", "row_pk"),
        Index("ix_audit_log_actor_created", "actor_user_id", "created_at"),
        Index("ix_audit_log_created", "created_at"),
        CheckConstraint("op IN ('insert', 'update', 'delete')", name="ck_audit_log_op"),
    )

    id: Mapped[int] = mapped_column(BigInteger, primary_key=True, autoincrement=True)
    table_name: Mapped[str] = mapped_column(String(64), nullable=False)
    row_pk: Mapped[str] = mapped_column(String(64), nullable=False)
    op: Mapped[str] = mapped_column(String(16), nullable=False)
    before_state: Mapped[dict[str, object] | None] = mapped_column(JSON)
    after_state: Mapped[dict[str, object] | None] = mapped_column(JSON)
    actor_user_id: Mapped[int | None] = mapped_column(ForeignKey("users.id", ondelete="SET NULL"))
    actor_session_id: Mapped[int | None] = mapped_column(
        ForeignKey("auth_sessions.id", ondelete="SET NULL")
    )
    actor_device_id: Mapped[int | None] = mapped_column(
        ForeignKey("user_devices.id", ondelete="SET NULL")
    )
    actor_request_id: Mapped[str | None] = mapped_column(String(64))
    created_at: Mapped[datetime] = mapped_column(
        DateTime(timezone=True), nullable=False, server_default=func.now()
    )


class TransactionSavedView(Base):
    __tablename__ = "transaction_saved_views"
    __table_args__ = (
        Index("ix_transaction_saved_views_user_book", "user_id", "book_id"),
        UniqueConstraint(
            "user_id", "book_id", "name", name="uq_transaction_saved_views_user_book_name"
        ),
    )

    id: Mapped[int] = mapped_column(BigInteger, primary_key=True, autoincrement=True)
    user_id: Mapped[int] = mapped_column(ForeignKey("users.id", ondelete="CASCADE"), nullable=False)
    book_id: Mapped[int] = mapped_column(ForeignKey("books.id", ondelete="CASCADE"), nullable=False)
    name: Mapped[str] = mapped_column(String(120), nullable=False)
    filters_json: Mapped[str] = mapped_column(Text, nullable=False)
    is_shared: Mapped[bool] = mapped_column(Boolean, nullable=False, server_default=text("false"))
    created_at: Mapped[datetime] = mapped_column(
        DateTime(timezone=True), nullable=False, server_default=func.now()
    )
    updated_at: Mapped[datetime] = mapped_column(
        DateTime(timezone=True), nullable=False, server_default=func.now()
    )


class TransactionTemplate(Base):
    __tablename__ = "transaction_templates"
    __table_args__ = (
        Index("ix_transaction_templates_user_book", "user_id", "book_id"),
        UniqueConstraint(
            "user_id", "book_id", "name", name="uq_transaction_templates_user_book_name"
        ),
    )

    id: Mapped[int] = mapped_column(BigInteger, primary_key=True, autoincrement=True)
    user_id: Mapped[int] = mapped_column(ForeignKey("users.id", ondelete="CASCADE"), nullable=False)
    book_id: Mapped[int] = mapped_column(ForeignKey("books.id", ondelete="CASCADE"), nullable=False)
    name: Mapped[str] = mapped_column(String(120), nullable=False)
    payee_id: Mapped[int | None] = mapped_column(ForeignKey("payees.id", ondelete="SET NULL"))
    memo: Mapped[str | None] = mapped_column(Text)
    status: Mapped[str] = mapped_column(String(20), nullable=False, server_default="uncleared")
    reference: Mapped[str | None] = mapped_column(Text)
    is_shared: Mapped[bool] = mapped_column(Boolean, nullable=False, server_default=text("false"))
    created_at: Mapped[datetime] = mapped_column(
        DateTime(timezone=True), nullable=False, server_default=func.now()
    )
    updated_at: Mapped[datetime] = mapped_column(
        DateTime(timezone=True), nullable=False, server_default=func.now()
    )


class TransactionTemplateSplit(Base):
    __tablename__ = "transaction_template_splits"
    __table_args__ = (
        Index("ix_transaction_template_splits_template", "template_id", "line_order"),
    )

    id: Mapped[int] = mapped_column(BigInteger, primary_key=True, autoincrement=True)
    template_id: Mapped[int] = mapped_column(
        ForeignKey("transaction_templates.id", ondelete="CASCADE"), nullable=False
    )
    line_order: Mapped[int] = mapped_column(Integer, nullable=False)
    account_id: Mapped[int] = mapped_column(
        ForeignKey("accounts.id", ondelete="RESTRICT"), nullable=False
    )
    commodity_id: Mapped[int] = mapped_column(
        ForeignKey("commodities.id", ondelete="RESTRICT"), nullable=False
    )
    amount_minor: Mapped[int] = mapped_column(BigInteger, nullable=False)
    category_id: Mapped[int | None] = mapped_column(
        ForeignKey("categories.id", ondelete="SET NULL")
    )
    tag_id: Mapped[int | None] = mapped_column(ForeignKey("tags.id", ondelete="SET NULL"))
    person_id: Mapped[int | None] = mapped_column(ForeignKey("people.id", ondelete="SET NULL"))
    project_id: Mapped[int | None] = mapped_column(ForeignKey("projects.id", ondelete="SET NULL"))
    share_bps: Mapped[int | None] = mapped_column(BigInteger)
    memo: Mapped[str | None] = mapped_column(Text)


class PayeeDefault(Base):
    __tablename__ = "payee_defaults"
    __table_args__ = (
        Index("ix_payee_defaults_user_book", "user_id", "book_id"),
        UniqueConstraint(
            "user_id", "book_id", "payee_id", "account_id", name="uq_payee_defaults_scope"
        ),
    )

    id: Mapped[int] = mapped_column(BigInteger, primary_key=True, autoincrement=True)
    user_id: Mapped[int] = mapped_column(ForeignKey("users.id", ondelete="CASCADE"), nullable=False)
    book_id: Mapped[int] = mapped_column(ForeignKey("books.id", ondelete="CASCADE"), nullable=False)
    payee_id: Mapped[int] = mapped_column(
        ForeignKey("payees.id", ondelete="CASCADE"), nullable=False
    )
    account_id: Mapped[int | None] = mapped_column(ForeignKey("accounts.id", ondelete="CASCADE"))
    category_id: Mapped[int | None] = mapped_column(
        ForeignKey("categories.id", ondelete="SET NULL")
    )
    memo: Mapped[str | None] = mapped_column(Text)
    created_at: Mapped[datetime] = mapped_column(
        DateTime(timezone=True), nullable=False, server_default=func.now()
    )
    updated_at: Mapped[datetime] = mapped_column(
        DateTime(timezone=True), nullable=False, server_default=func.now()
    )


class MarkdownNote(Base):
    __tablename__ = "markdown_notes"
    __table_args__ = (
        Index("ix_markdown_notes_account", "account_id", "pinned", "updated_at"),
        Index("ix_markdown_notes_transaction", "transaction_id", "pinned", "updated_at"),
        CheckConstraint(
            "target_type IN ('account', 'transaction')", name="ck_markdown_notes_target_type"
        ),
        CheckConstraint(
            "(target_type = 'account' AND account_id IS NOT NULL AND transaction_id IS NULL) OR "
            "(target_type = 'transaction' AND transaction_id IS NOT NULL AND account_id IS NULL)",
            name="ck_markdown_notes_single_target",
        ),
    )

    id: Mapped[int] = mapped_column(BigInteger, primary_key=True, autoincrement=True)
    book_id: Mapped[int] = mapped_column(ForeignKey("books.id", ondelete="CASCADE"), nullable=False)
    target_type: Mapped[str] = mapped_column(String(20), nullable=False)
    account_id: Mapped[int | None] = mapped_column(ForeignKey("accounts.id", ondelete="CASCADE"))
    transaction_id: Mapped[int | None] = mapped_column(
        ForeignKey("transactions.id", ondelete="CASCADE")
    )
    title: Mapped[str] = mapped_column(String(200), nullable=False)
    body_markdown: Mapped[str] = mapped_column(Text, nullable=False)
    pinned: Mapped[bool] = mapped_column(Boolean, nullable=False, server_default=text("false"))
    created_at: Mapped[datetime] = mapped_column(
        DateTime(timezone=True), nullable=False, server_default=func.now()
    )
    updated_at: Mapped[datetime] = mapped_column(
        DateTime(timezone=True), nullable=False, server_default=func.now()
    )
    created_by_user_id: Mapped[int | None] = mapped_column(
        ForeignKey("users.id", ondelete="SET NULL")
    )
    updated_by_user_id: Mapped[int | None] = mapped_column(
        ForeignKey("users.id", ondelete="SET NULL")
    )
