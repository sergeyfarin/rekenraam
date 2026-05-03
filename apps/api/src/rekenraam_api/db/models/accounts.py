from datetime import date, datetime

from sqlalchemy import BigInteger, Boolean, Date, DateTime, ForeignKey, String, Text, func
from sqlalchemy.orm import Mapped, mapped_column

from rekenraam_api.db.base import Base


class Account(Base):
    __tablename__ = "accounts"

    id: Mapped[int] = mapped_column(BigInteger, primary_key=True, autoincrement=True)
    book_id: Mapped[int] = mapped_column(ForeignKey("books.id", ondelete="CASCADE"), nullable=False)
    parent_id: Mapped[int | None] = mapped_column(ForeignKey("accounts.id", ondelete="SET NULL"))
    previous_account_id: Mapped[int | None] = mapped_column(ForeignKey("accounts.id", ondelete="SET NULL"))
    account_type: Mapped[str] = mapped_column(String(32), nullable=False)
    name: Mapped[str] = mapped_column(String(200), nullable=False)
    commodity_id: Mapped[int] = mapped_column(BigInteger, nullable=False)
    booking_policy: Mapped[str] = mapped_column(String(16), nullable=False, server_default="fifo")
    number_last4: Mapped[str | None] = mapped_column(String(4))
    is_closed: Mapped[bool] = mapped_column(Boolean, nullable=False, server_default="false")
    is_hidden: Mapped[bool] = mapped_column(Boolean, nullable=False, server_default="false")
    is_system: Mapped[bool] = mapped_column(Boolean, nullable=False, server_default="false")
    system_role: Mapped[str | None] = mapped_column(String(64))
    effective_at: Mapped[date] = mapped_column(Date, nullable=False, server_default=func.current_date())
    lifecycle_event: Mapped[str] = mapped_column(String(16), nullable=False, server_default="open")
    lifecycle_note: Mapped[str | None] = mapped_column(Text)
    lifecycle_metadata: Mapped[str | None] = mapped_column(Text)
    created_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), nullable=False, server_default=func.now())
    updated_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), nullable=False, server_default=func.now())


class AccountBalancing(Base):
    __tablename__ = "account_balancings"

    id: Mapped[int] = mapped_column(BigInteger, primary_key=True, autoincrement=True)
    book_id: Mapped[int] = mapped_column(ForeignKey("books.id", ondelete="CASCADE"), nullable=False)
    account_id: Mapped[int] = mapped_column(ForeignKey("accounts.id", ondelete="CASCADE"), nullable=False)
    previous_account_balancing_id: Mapped[int | None] = mapped_column(
        ForeignKey("account_balancings.id", ondelete="SET NULL")
    )
    as_of_date: Mapped[date] = mapped_column(Date, nullable=False)
    balance_minor: Mapped[int] = mapped_column(BigInteger, nullable=False)
    memo: Mapped[str | None] = mapped_column(Text)
    created_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), nullable=False, server_default=func.now())
    voided_at: Mapped[datetime | None] = mapped_column(DateTime(timezone=True))
    void_reason: Mapped[str | None] = mapped_column(Text)