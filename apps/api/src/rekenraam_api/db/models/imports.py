from datetime import date, datetime

from sqlalchemy import (
    BigInteger,
    CheckConstraint,
    Date,
    DateTime,
    ForeignKey,
    Index,
    String,
    Text,
    UniqueConstraint,
    func,
    text,
)
from sqlalchemy.orm import Mapped, mapped_column

from rekenraam_api.db.base import Base


class ImportRule(Base):
    __tablename__ = "import_rules"
    __audit_logged__ = True
    __table_args__ = (
        Index("ix_import_rules_book_kind", "book_id", "rule_kind"),
        Index("ix_import_rules_priority", "priority", "id"),
        Index("ix_import_rules_previous", "previous_import_rule_id"),
        UniqueConstraint("previous_import_rule_id", name="uq_import_rules_previous"),
        CheckConstraint(
            "rule_kind IN ('payee', 'memo', 'amount', 'date', 'account')",
            name="ck_import_rules_kind",
        ),
        CheckConstraint("match_type IN ('contains', 'equals')", name="ck_import_rules_match_type"),
    )

    id: Mapped[int] = mapped_column(BigInteger, primary_key=True, autoincrement=True)
    previous_import_rule_id: Mapped[int | None] = mapped_column(
        ForeignKey("import_rules.id", ondelete="SET NULL")
    )
    book_id: Mapped[int] = mapped_column(ForeignKey("books.id", ondelete="CASCADE"), nullable=False)
    rule_kind: Mapped[str] = mapped_column(String(20), nullable=False)
    match_type: Mapped[str] = mapped_column(String(20), nullable=False, server_default="contains")
    match_text: Mapped[str] = mapped_column(Text, nullable=False)
    priority: Mapped[int] = mapped_column(BigInteger, nullable=False, server_default=text("100"))
    amount_min_minor: Mapped[int | None] = mapped_column(BigInteger)
    amount_max_minor: Mapped[int | None] = mapped_column(BigInteger)
    date_from: Mapped[date | None] = mapped_column(Date)
    date_to: Mapped[date | None] = mapped_column(Date)
    match_account_id: Mapped[int | None] = mapped_column(
        ForeignKey("accounts.id", ondelete="SET NULL")
    )
    target_account_id: Mapped[int | None] = mapped_column(
        ForeignKey("accounts.id", ondelete="SET NULL")
    )
    target_category_id: Mapped[int | None] = mapped_column(
        ForeignKey("categories.id", ondelete="SET NULL")
    )
    target_payee_id: Mapped[int | None] = mapped_column(
        ForeignKey("payees.id", ondelete="SET NULL")
    )
    deleted_at: Mapped[datetime | None] = mapped_column(DateTime(timezone=True))
    created_at: Mapped[datetime] = mapped_column(
        DateTime(timezone=True), nullable=False, server_default=func.now()
    )
    created_by_user_id: Mapped[int | None] = mapped_column(
        ForeignKey("users.id", ondelete="SET NULL")
    )
    created_session_id: Mapped[int | None] = mapped_column(
        ForeignKey("auth_sessions.id", ondelete="SET NULL")
    )
    created_device_id: Mapped[int | None] = mapped_column(
        ForeignKey("user_devices.id", ondelete="SET NULL")
    )
    created_request_id: Mapped[str | None] = mapped_column(String(64))


class ImportSession(Base):
    __tablename__ = "import_sessions"
    __table_args__ = (
        Index("ix_import_sessions_book_status", "book_id", "status"),
        Index("ix_import_sessions_file_hash", "file_hash"),
        CheckConstraint(
            "status IN ('started', 'committed', 'abandoned')", name="ck_import_sessions_status"
        ),
    )

    id: Mapped[int] = mapped_column(BigInteger, primary_key=True, autoincrement=True)
    book_id: Mapped[int] = mapped_column(ForeignKey("books.id", ondelete="CASCADE"), nullable=False)
    source: Mapped[str | None] = mapped_column(String(120))
    file_name: Mapped[str | None] = mapped_column(Text)
    file_hash: Mapped[str | None] = mapped_column(String(128))
    file_size: Mapped[int | None] = mapped_column(BigInteger)
    status: Mapped[str] = mapped_column(String(20), nullable=False, server_default="started")
    started_at: Mapped[datetime] = mapped_column(
        DateTime(timezone=True), nullable=False, server_default=func.now()
    )
    committed_at: Mapped[datetime | None] = mapped_column(DateTime(timezone=True))
    notes: Mapped[str | None] = mapped_column(Text)
    created_by_user_id: Mapped[int | None] = mapped_column(
        ForeignKey("users.id", ondelete="SET NULL")
    )
    created_session_id: Mapped[int | None] = mapped_column(
        ForeignKey("auth_sessions.id", ondelete="SET NULL")
    )
    created_device_id: Mapped[int | None] = mapped_column(
        ForeignKey("user_devices.id", ondelete="SET NULL")
    )
    created_request_id: Mapped[str | None] = mapped_column(String(64))


class ImportSessionTransaction(Base):
    __tablename__ = "import_session_transactions"
    __table_args__ = (
        Index("ix_import_session_transactions_session", "session_id"),
        Index("ix_import_session_transactions_tx", "tx_id"),
        UniqueConstraint(
            "session_id", "tx_id", "action", name="uq_import_session_transaction_action"
        ),
        CheckConstraint(
            "action IN ('created', 'updated', 'validated')",
            name="ck_import_session_transactions_action",
        ),
    )

    id: Mapped[int] = mapped_column(BigInteger, primary_key=True, autoincrement=True)
    session_id: Mapped[int] = mapped_column(
        ForeignKey("import_sessions.id", ondelete="CASCADE"), nullable=False
    )
    tx_id: Mapped[int] = mapped_column(
        ForeignKey("transactions.id", ondelete="CASCADE"), nullable=False
    )
    action: Mapped[str] = mapped_column(String(20), nullable=False)
    created_at: Mapped[datetime] = mapped_column(
        DateTime(timezone=True), nullable=False, server_default=func.now()
    )


class ImportTransactionKey(Base):
    __tablename__ = "import_transaction_keys"
    __table_args__ = (
        Index("ix_import_transaction_keys_book", "book_id"),
        Index("ix_import_transaction_keys_tx", "tx_id"),
        UniqueConstraint("account_id", "import_id", name="uq_import_transaction_keys_account_id"),
        CheckConstraint("length(trim(import_id)) > 0", name="ck_import_transaction_keys_import_id"),
    )

    id: Mapped[int] = mapped_column(BigInteger, primary_key=True, autoincrement=True)
    book_id: Mapped[int] = mapped_column(ForeignKey("books.id", ondelete="CASCADE"), nullable=False)
    account_id: Mapped[int] = mapped_column(
        ForeignKey("accounts.id", ondelete="CASCADE"), nullable=False
    )
    tx_id: Mapped[int] = mapped_column(
        ForeignKey("transactions.id", ondelete="CASCADE"), nullable=False
    )
    import_id: Mapped[str] = mapped_column(String(255), nullable=False)
    created_at: Mapped[datetime] = mapped_column(
        DateTime(timezone=True), nullable=False, server_default=func.now()
    )
