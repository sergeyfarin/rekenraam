from datetime import date, datetime

from sqlalchemy import BigInteger, CheckConstraint, Date, DateTime, ForeignKey, Index, String, Text, func
from sqlalchemy.orm import Mapped, mapped_column

from rekenraam_api.db.base import Base


class Lot(Base):
    __tablename__ = "lots"
    __table_args__ = (
        Index("ix_lots_book_id", "book_id"),
        Index("ix_lots_account_commodity_opened", "account_id", "commodity_id", "opened_date"),
    )

    id: Mapped[int] = mapped_column(BigInteger, primary_key=True, autoincrement=True)
    book_id: Mapped[int] = mapped_column(ForeignKey("books.id", ondelete="CASCADE"), nullable=False)
    account_id: Mapped[int] = mapped_column(ForeignKey("accounts.id", ondelete="CASCADE"), nullable=False)
    commodity_id: Mapped[int] = mapped_column(ForeignKey("commodities.id", ondelete="RESTRICT"), nullable=False)
    opened_date: Mapped[date | None] = mapped_column(Date)
    notes: Mapped[str | None] = mapped_column(Text)
    cost_basis_minor: Mapped[int] = mapped_column(BigInteger, nullable=False)
    created_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), nullable=False, server_default=func.now())


class SplitLotAllocation(Base):
    __tablename__ = "split_lot_allocations"
    __table_args__ = (
        Index("ix_split_lot_allocations_split_id", "split_id"),
        Index("ix_split_lot_allocations_lot_id", "lot_id"),
    )

    id: Mapped[int] = mapped_column(BigInteger, primary_key=True, autoincrement=True)
    split_id: Mapped[int] = mapped_column(ForeignKey("splits.id", ondelete="CASCADE"), nullable=False)
    lot_id: Mapped[int] = mapped_column(ForeignKey("lots.id", ondelete="CASCADE"), nullable=False)
    quantity_minor: Mapped[int] = mapped_column(BigInteger, nullable=False)
    created_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), nullable=False, server_default=func.now())


class PriceObservation(Base):
    __tablename__ = "price_observations"
    __table_args__ = (
        Index("ix_price_observations_book_id", "book_id"),
        Index(
            "ix_price_observations_lookup",
            "commodity_id",
            "quote_commodity_id",
            "observation_kind",
            "price_date",
        ),
        CheckConstraint(
            "observation_kind IN ('commodity_market', 'fx_manual', 'valuation_override')",
            name="ck_price_observations_observation_kind",
        ),
    )

    id: Mapped[int] = mapped_column(BigInteger, primary_key=True, autoincrement=True)
    book_id: Mapped[int] = mapped_column(ForeignKey("books.id", ondelete="CASCADE"), nullable=False)
    commodity_id: Mapped[int] = mapped_column(ForeignKey("commodities.id", ondelete="CASCADE"), nullable=False)
    quote_commodity_id: Mapped[int] = mapped_column(ForeignKey("commodities.id", ondelete="CASCADE"), nullable=False)
    observation_kind: Mapped[str] = mapped_column(String(32), nullable=False, server_default="commodity_market")
    price_minor: Mapped[int] = mapped_column(BigInteger, nullable=False)
    price_date: Mapped[date] = mapped_column(Date, nullable=False)
    source: Mapped[str | None] = mapped_column(String(128))
    created_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), nullable=False, server_default=func.now())
