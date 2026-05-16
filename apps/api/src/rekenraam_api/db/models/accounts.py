from datetime import date, datetime

from sqlalchemy import (
    BigInteger,
    Boolean,
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


class Account(Base):
    __tablename__ = "accounts"
    __audit_logged__ = True
    __table_args__ = (
        Index("ix_accounts_book_id", "book_id"),
        Index("ix_accounts_parent_id", "parent_id"),
        Index("ix_accounts_previous_account_id", "previous_account_id"),
        UniqueConstraint("previous_account_id", name="uq_accounts_previous_account_id"),
        CheckConstraint(
            "account_type IN ('cash', 'checking', 'savings', 'credit', 'loan', "
            "'investment', 'asset', 'liability', 'income', 'expense', 'equity')",
            name="ck_accounts_account_type",
        ),
        CheckConstraint(
            "booking_policy IN ('fifo', 'lifo', 'strict', 'average')",
            name="ck_accounts_booking_policy",
        ),
        CheckConstraint(
            "lifecycle_event IN ('open', 'close', 'reopen', 'update')",
            name="ck_accounts_lifecycle_event",
        ),
        CheckConstraint(
            "(system_role IS NULL) OR is_system",
            name="ck_accounts_system_role_requires_system",
        ),
        CheckConstraint(
            "system_role IS NULL OR system_role IN "
            "('opening_balance', 'imbalance_import', 'income_summary', "
            "'expense_summary', 'retained_earnings')",
            name="ck_accounts_system_role_allowed",
        ),
        Index(
            "uq_accounts_system_role_per_book",
            "book_id",
            "system_role",
            unique=True,
            postgresql_where=text("system_role IS NOT NULL"),
            sqlite_where=text("system_role IS NOT NULL"),
        ),
    )

    id: Mapped[int] = mapped_column(BigInteger, primary_key=True, autoincrement=True)
    book_id: Mapped[int] = mapped_column(ForeignKey("books.id", ondelete="CASCADE"), nullable=False)
    parent_id: Mapped[int | None] = mapped_column(ForeignKey("accounts.id", ondelete="SET NULL"))
    previous_account_id: Mapped[int | None] = mapped_column(
        ForeignKey("accounts.id", ondelete="SET NULL")
    )
    account_type: Mapped[str] = mapped_column(String(32), nullable=False)
    name: Mapped[str] = mapped_column(String(200), nullable=False)
    commodity_id: Mapped[int] = mapped_column(
        ForeignKey("commodities.id", ondelete="RESTRICT"), nullable=False
    )
    booking_policy: Mapped[str] = mapped_column(String(16), nullable=False, server_default="fifo")
    number_last4: Mapped[str | None] = mapped_column(String(4))
    is_closed: Mapped[bool] = mapped_column(Boolean, nullable=False, server_default=text("false"))
    is_hidden: Mapped[bool] = mapped_column(Boolean, nullable=False, server_default=text("false"))
    is_system: Mapped[bool] = mapped_column(Boolean, nullable=False, server_default=text("false"))
    system_role: Mapped[str | None] = mapped_column(String(64))
    effective_at: Mapped[date] = mapped_column(
        Date, nullable=False, server_default=func.current_date()
    )
    lifecycle_event: Mapped[str] = mapped_column(String(16), nullable=False, server_default="open")
    lifecycle_note: Mapped[str | None] = mapped_column(Text)
    lifecycle_metadata: Mapped[str | None] = mapped_column(Text)
    created_at: Mapped[datetime] = mapped_column(
        DateTime(timezone=True), nullable=False, server_default=func.now()
    )
    updated_at: Mapped[datetime] = mapped_column(
        DateTime(timezone=True), nullable=False, server_default=func.now()
    )


class AccountBalancing(Base):
    __tablename__ = "account_balancings"
    __audit_logged__ = True
    __table_args__ = (
        Index("ix_account_balancings_book_id", "book_id"),
        Index("ix_account_balancings_account_date", "account_id", "as_of_date"),
    )

    id: Mapped[int] = mapped_column(BigInteger, primary_key=True, autoincrement=True)
    book_id: Mapped[int] = mapped_column(ForeignKey("books.id", ondelete="CASCADE"), nullable=False)
    account_id: Mapped[int] = mapped_column(
        ForeignKey("accounts.id", ondelete="CASCADE"), nullable=False
    )
    previous_account_balancing_id: Mapped[int | None] = mapped_column(
        ForeignKey("account_balancings.id", ondelete="SET NULL")
    )
    as_of_date: Mapped[date] = mapped_column(Date, nullable=False)
    balance_minor: Mapped[int] = mapped_column(BigInteger, nullable=False)
    memo: Mapped[str | None] = mapped_column(Text)
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
    voided_at: Mapped[datetime | None] = mapped_column(DateTime(timezone=True))
    void_reason: Mapped[str | None] = mapped_column(Text)


class BalanceCheck(Base):
    __tablename__ = "balance_checks"
    __audit_logged__ = True
    __table_args__ = (
        Index("ix_balance_checks_account_date", "account_id", "as_of_date"),
        CheckConstraint(
            "status IN ('recorded', 'matched', 'failed', 'unbalanced', 'balanced')",
            name="ck_balance_checks_status",
        ),
    )

    id: Mapped[int] = mapped_column(BigInteger, primary_key=True, autoincrement=True)
    book_id: Mapped[int] = mapped_column(ForeignKey("books.id", ondelete="CASCADE"), nullable=False)
    account_id: Mapped[int] = mapped_column(
        ForeignKey("accounts.id", ondelete="CASCADE"), nullable=False
    )
    as_of_date: Mapped[date] = mapped_column(Date, nullable=False)
    balance_minor: Mapped[int] = mapped_column(BigInteger, nullable=False)
    memo: Mapped[str | None] = mapped_column(Text)
    status: Mapped[str] = mapped_column(String(20), nullable=False, server_default="recorded")
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


class BalanceAdjustment(Base):
    __tablename__ = "balance_adjustments"
    __audit_logged__ = True
    __table_args__ = (
        Index("ix_balance_adjustments_account_date", "account_id", "as_of_date"),
        Index("ix_balance_adjustments_check", "balance_check_id"),
    )

    id: Mapped[int] = mapped_column(BigInteger, primary_key=True, autoincrement=True)
    book_id: Mapped[int] = mapped_column(ForeignKey("books.id", ondelete="CASCADE"), nullable=False)
    balance_check_id: Mapped[int | None] = mapped_column(
        ForeignKey("balance_checks.id", ondelete="SET NULL")
    )
    account_id: Mapped[int] = mapped_column(
        ForeignKey("accounts.id", ondelete="CASCADE"), nullable=False
    )
    offset_account_id: Mapped[int] = mapped_column(
        ForeignKey("accounts.id", ondelete="CASCADE"), nullable=False
    )
    as_of_date: Mapped[date] = mapped_column(Date, nullable=False)
    target_balance_minor: Mapped[int] = mapped_column(BigInteger, nullable=False)
    actual_balance_minor: Mapped[int] = mapped_column(BigInteger, nullable=False)
    adjustment_minor: Mapped[int] = mapped_column(BigInteger, nullable=False)
    tx_id: Mapped[int] = mapped_column(
        ForeignKey("transactions.id", ondelete="CASCADE"), nullable=False
    )
    mark: Mapped[str] = mapped_column(
        String(64), nullable=False, server_default="balance_adjustment"
    )
    memo: Mapped[str | None] = mapped_column(Text)
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


class BalanceConstraint(Base):
    __tablename__ = "balance_constraints"
    __audit_logged__ = True
    __table_args__ = (
        Index("ix_balance_constraints_account", "account_id"),
        CheckConstraint(
            "sign_rule IN ('any', 'nonnegative', 'nonpositive')",
            name="ck_balance_constraints_sign_rule",
        ),
    )

    id: Mapped[int] = mapped_column(BigInteger, primary_key=True, autoincrement=True)
    book_id: Mapped[int] = mapped_column(ForeignKey("books.id", ondelete="CASCADE"), nullable=False)
    account_id: Mapped[int] = mapped_column(
        ForeignKey("accounts.id", ondelete="CASCADE"), nullable=False
    )
    min_balance_minor: Mapped[int | None] = mapped_column(BigInteger)
    max_balance_minor: Mapped[int | None] = mapped_column(BigInteger)
    sign_rule: Mapped[str] = mapped_column(String(20), nullable=False, server_default="any")
    created_at: Mapped[datetime] = mapped_column(
        DateTime(timezone=True), nullable=False, server_default=func.now()
    )
    updated_at: Mapped[datetime] = mapped_column(
        DateTime(timezone=True), nullable=False, server_default=func.now()
    )


class ReconciliationPreference(Base):
    __tablename__ = "reconciliation_preferences"
    __table_args__ = (
        UniqueConstraint(
            "user_id", "account_id", name="uq_reconciliation_preferences_user_account"
        ),
        Index("ix_reconciliation_preferences_account", "account_id"),
    )

    id: Mapped[int] = mapped_column(BigInteger, primary_key=True, autoincrement=True)
    user_id: Mapped[int] = mapped_column(ForeignKey("users.id", ondelete="CASCADE"), nullable=False)
    book_id: Mapped[int] = mapped_column(ForeignKey("books.id", ondelete="CASCADE"), nullable=False)
    account_id: Mapped[int] = mapped_column(
        ForeignKey("accounts.id", ondelete="CASCADE"), nullable=False
    )
    offset_account_id: Mapped[int | None] = mapped_column(
        ForeignKey("accounts.id", ondelete="SET NULL")
    )
    created_at: Mapped[datetime] = mapped_column(
        DateTime(timezone=True), nullable=False, server_default=func.now()
    )
    updated_at: Mapped[datetime] = mapped_column(
        DateTime(timezone=True), nullable=False, server_default=func.now()
    )
