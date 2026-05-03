from datetime import date, datetime

from sqlalchemy import BigInteger, Date, DateTime, ForeignKey, String, Text, func
from sqlalchemy.orm import Mapped, mapped_column

from rekenraam_api.db.base import Base


class Transaction(Base):
    __tablename__ = "transactions"

    id: Mapped[int] = mapped_column(BigInteger, primary_key=True, autoincrement=True)
    book_id: Mapped[int] = mapped_column(ForeignKey("books.id", ondelete="CASCADE"), nullable=False)
    occurred_date: Mapped[date] = mapped_column(Date, nullable=False)
    posted_date: Mapped[date] = mapped_column(Date, nullable=False)
    memo: Mapped[str | None] = mapped_column(Text)
    status: Mapped[str] = mapped_column(String(20), nullable=False, server_default="uncleared")
    created_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), nullable=False, server_default=func.now())


class Split(Base):
    __tablename__ = "splits"

    id: Mapped[int] = mapped_column(BigInteger, primary_key=True, autoincrement=True)
    tx_id: Mapped[int] = mapped_column(ForeignKey("transactions.id", ondelete="CASCADE"), nullable=False)
    account_id: Mapped[int] = mapped_column(ForeignKey("accounts.id", ondelete="RESTRICT"), nullable=False)
    amount_minor: Mapped[int] = mapped_column(BigInteger, nullable=False)
    memo: Mapped[str | None] = mapped_column(Text)
    created_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), nullable=False, server_default=func.now())