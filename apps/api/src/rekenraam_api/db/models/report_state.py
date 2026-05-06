from datetime import datetime

from sqlalchemy import (
    BigInteger,
    DateTime,
    ForeignKey,
    Index,
    Integer,
    String,
    Text,
    UniqueConstraint,
    func,
)
from sqlalchemy.orm import Mapped, mapped_column

from rekenraam_api.db.base import Base


class BookState(Base):
    __tablename__ = "book_state"

    book_id: Mapped[int] = mapped_column(ForeignKey("books.id", ondelete="CASCADE"), primary_key=True)
    change_seq: Mapped[int] = mapped_column(Integer, nullable=False, server_default="0")
    updated_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), nullable=False, server_default=func.now())


class ReportCache(Base):
    __tablename__ = "report_cache"
    __table_args__ = (
        Index("ix_report_cache_book_type", "book_id", "report_type"),
        Index("ix_report_cache_book_seq", "book_id", "as_of_seq"),
        UniqueConstraint("book_id", "report_type", "params_hash", "as_of_seq", name="uq_report_cache_entry"),
    )

    id: Mapped[int] = mapped_column(BigInteger, primary_key=True, autoincrement=True)
    book_id: Mapped[int] = mapped_column(ForeignKey("books.id", ondelete="CASCADE"), nullable=False)
    report_type: Mapped[str] = mapped_column(String(100), nullable=False)
    params_hash: Mapped[str] = mapped_column(String(128), nullable=False)
    params_json: Mapped[str | None] = mapped_column(Text)
    as_of_seq: Mapped[int] = mapped_column(Integer, nullable=False)
    payload_json: Mapped[str] = mapped_column(Text, nullable=False)
    created_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), nullable=False, server_default=func.now())