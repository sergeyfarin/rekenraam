from datetime import date, datetime

from sqlalchemy import BigInteger, CheckConstraint, Date, DateTime, ForeignKey, Index, String, Text, func
from sqlalchemy.orm import Mapped, mapped_column

from rekenraam_api.db.base import Base


class Transaction(Base):
    __tablename__ = "transactions"
    __table_args__ = (
        Index("ix_transactions_book_occurred_date", "book_id", "occurred_date"),
        CheckConstraint(
            "status IN ('uncleared', 'cleared', 'reconciled', 'void')",
            name="ck_transactions_status",
        ),
    )

    id: Mapped[int] = mapped_column(BigInteger, primary_key=True, autoincrement=True)
    book_id: Mapped[int] = mapped_column(ForeignKey("books.id", ondelete="CASCADE"), nullable=False)
    occurred_date: Mapped[date] = mapped_column(Date, nullable=False)
    posted_date: Mapped[date] = mapped_column(Date, nullable=False)
    payee_id: Mapped[int | None] = mapped_column(ForeignKey("payees.id", ondelete="SET NULL"))
    memo: Mapped[str | None] = mapped_column(Text)
    status: Mapped[str] = mapped_column(String(20), nullable=False, server_default="uncleared")
    reference: Mapped[str | None] = mapped_column(Text)
    created_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), nullable=False, server_default=func.now())
    created_by_user_id: Mapped[int | None] = mapped_column(ForeignKey("users.id", ondelete="SET NULL"))
    created_session_id: Mapped[int | None] = mapped_column(ForeignKey("auth_sessions.id", ondelete="SET NULL"))
    created_device_id: Mapped[int | None] = mapped_column(ForeignKey("user_devices.id", ondelete="SET NULL"))
    created_request_id: Mapped[str | None] = mapped_column(String(64))


class Split(Base):
    __tablename__ = "splits"
    __table_args__ = (
        Index("ix_splits_tx_id", "tx_id"),
        Index("ix_splits_account_id", "account_id"),
    )

    id: Mapped[int] = mapped_column(BigInteger, primary_key=True, autoincrement=True)
    tx_id: Mapped[int] = mapped_column(ForeignKey("transactions.id", ondelete="CASCADE"), nullable=False)
    account_id: Mapped[int] = mapped_column(ForeignKey("accounts.id", ondelete="RESTRICT"), nullable=False)
    commodity_id: Mapped[int] = mapped_column(ForeignKey("commodities.id", ondelete="RESTRICT"), nullable=False)
    amount_minor: Mapped[int] = mapped_column(BigInteger, nullable=False)
    category_id: Mapped[int | None] = mapped_column(ForeignKey("categories.id", ondelete="SET NULL"))
    tag_id: Mapped[int | None] = mapped_column(ForeignKey("tags.id", ondelete="SET NULL"))
    person_id: Mapped[int | None] = mapped_column(ForeignKey("people.id", ondelete="SET NULL"))
    project_id: Mapped[int | None] = mapped_column(ForeignKey("projects.id", ondelete="SET NULL"))
    share_bps: Mapped[int | None] = mapped_column(BigInteger)
    memo: Mapped[str | None] = mapped_column(Text)
    created_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), nullable=False, server_default=func.now())
    created_by_user_id: Mapped[int | None] = mapped_column(ForeignKey("users.id", ondelete="SET NULL"))
    created_session_id: Mapped[int | None] = mapped_column(ForeignKey("auth_sessions.id", ondelete="SET NULL"))
    created_device_id: Mapped[int | None] = mapped_column(ForeignKey("user_devices.id", ondelete="SET NULL"))
    created_request_id: Mapped[str | None] = mapped_column(String(64))
