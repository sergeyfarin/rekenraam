from datetime import date, datetime

from sqlalchemy import (
    BigInteger,
    Boolean,
    CheckConstraint,
    Date,
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


class PriceSource(Base):
    __tablename__ = "price_sources"
    __table_args__ = (
        Index("ix_price_sources_kind", "kind"),
        UniqueConstraint("name", name="uq_price_sources_name"),
    )

    id: Mapped[int] = mapped_column(BigInteger, primary_key=True, autoincrement=True)
    name: Mapped[str] = mapped_column(String(100), nullable=False)
    kind: Mapped[str] = mapped_column(String(32), nullable=False, server_default="provider")
    provider: Mapped[str | None] = mapped_column(String(100))
    base_url: Mapped[str | None] = mapped_column(String(255))
    created_at: Mapped[datetime] = mapped_column(
        DateTime(timezone=True), nullable=False, server_default=func.now()
    )


class PricingPolicy(Base):
    __tablename__ = "pricing_policies"
    __table_args__ = (
        Index("ix_pricing_policies_book_id", "book_id"),
        UniqueConstraint("book_id", name="uq_pricing_policies_book_id"),
        CheckConstraint(
            "refresh_hour_utc BETWEEN 0 AND 23", name="ck_pricing_policies_refresh_hour"
        ),
        CheckConstraint(
            "refresh_minute_utc BETWEEN 0 AND 59", name="ck_pricing_policies_refresh_minute"
        ),
        CheckConstraint("max_backfill_days >= 1", name="ck_pricing_policies_backfill_days"),
        CheckConstraint(
            "weekend_policy IN ('skip', 'fill_previous', 'download')",
            name="ck_pricing_policies_weekend_policy",
        ),
    )

    id: Mapped[int] = mapped_column(BigInteger, primary_key=True, autoincrement=True)
    book_id: Mapped[int] = mapped_column(ForeignKey("books.id", ondelete="CASCADE"), nullable=False)
    base_commodity_id: Mapped[int] = mapped_column(
        ForeignKey("commodities.id", ondelete="RESTRICT"), nullable=False
    )
    refresh_enabled: Mapped[bool] = mapped_column(
        Boolean, nullable=False, server_default=text("false")
    )
    refresh_hour_utc: Mapped[int] = mapped_column(Integer, nullable=False, server_default=text("4"))
    refresh_minute_utc: Mapped[int] = mapped_column(
        Integer, nullable=False, server_default=text("0")
    )
    max_backfill_days: Mapped[int] = mapped_column(
        Integer, nullable=False, server_default=text("370")
    )
    weekend_policy: Mapped[str] = mapped_column(String(32), nullable=False, server_default="skip")
    default_source_id: Mapped[int | None] = mapped_column(
        ForeignKey("price_sources.id", ondelete="SET NULL")
    )
    created_at: Mapped[datetime] = mapped_column(
        DateTime(timezone=True), nullable=False, server_default=func.now()
    )
    updated_at: Mapped[datetime] = mapped_column(
        DateTime(timezone=True), nullable=False, server_default=func.now()
    )


class PricingSourceAssignment(Base):
    __tablename__ = "pricing_source_assignments"
    __table_args__ = (
        Index("ix_pricing_source_assignments_book_id", "book_id"),
        CheckConstraint("priority >= 0", name="ck_pricing_source_assignments_priority"),
    )

    id: Mapped[int] = mapped_column(BigInteger, primary_key=True, autoincrement=True)
    book_id: Mapped[int] = mapped_column(ForeignKey("books.id", ondelete="CASCADE"), nullable=False)
    commodity_id: Mapped[int] = mapped_column(
        ForeignKey("commodities.id", ondelete="CASCADE"), nullable=False
    )
    quote_commodity_id: Mapped[int] = mapped_column(
        ForeignKey("commodities.id", ondelete="CASCADE"), nullable=False
    )
    source_id: Mapped[int] = mapped_column(
        ForeignKey("price_sources.id", ondelete="CASCADE"), nullable=False
    )
    priority: Mapped[int] = mapped_column(Integer, nullable=False, server_default=text("100"))
    effective_from: Mapped[date] = mapped_column(Date, nullable=False)
    effective_to: Mapped[date | None] = mapped_column(Date)
    created_at: Mapped[datetime] = mapped_column(
        DateTime(timezone=True), nullable=False, server_default=func.now()
    )
    updated_at: Mapped[datetime] = mapped_column(
        DateTime(timezone=True), nullable=False, server_default=func.now()
    )


class PricingRefreshState(Base):
    __tablename__ = "pricing_refresh_state"
    __table_args__ = (
        Index("ix_pricing_refresh_state_book_id", "book_id"),
        UniqueConstraint(
            "book_id",
            "commodity_id",
            "quote_commodity_id",
            "source_id",
            name="uq_pricing_refresh_state_pair_source",
        ),
    )

    id: Mapped[int] = mapped_column(BigInteger, primary_key=True, autoincrement=True)
    book_id: Mapped[int] = mapped_column(ForeignKey("books.id", ondelete="CASCADE"), nullable=False)
    commodity_id: Mapped[int] = mapped_column(
        ForeignKey("commodities.id", ondelete="CASCADE"), nullable=False
    )
    quote_commodity_id: Mapped[int] = mapped_column(
        ForeignKey("commodities.id", ondelete="CASCADE"), nullable=False
    )
    source_id: Mapped[int] = mapped_column(
        ForeignKey("price_sources.id", ondelete="CASCADE"), nullable=False
    )
    last_success_date: Mapped[date | None] = mapped_column(Date)
    last_attempt_at: Mapped[datetime | None] = mapped_column(DateTime(timezone=True))
    last_error: Mapped[str | None] = mapped_column(Text)
    created_at: Mapped[datetime] = mapped_column(
        DateTime(timezone=True), nullable=False, server_default=func.now()
    )
    updated_at: Mapped[datetime] = mapped_column(
        DateTime(timezone=True), nullable=False, server_default=func.now()
    )


class PricingRefreshRun(Base):
    __tablename__ = "pricing_refresh_runs"
    __table_args__ = (
        Index("ix_pricing_refresh_runs_book_id", "book_id"),
        Index("ix_pricing_refresh_runs_finished_at", "finished_at"),
    )

    id: Mapped[int] = mapped_column(BigInteger, primary_key=True, autoincrement=True)
    book_id: Mapped[int] = mapped_column(ForeignKey("books.id", ondelete="CASCADE"), nullable=False)
    trigger: Mapped[str] = mapped_column(String(32), nullable=False)
    started_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), nullable=False)
    finished_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), nullable=False)
    pairs_total: Mapped[int] = mapped_column(Integer, nullable=False, server_default=text("0"))
    pairs_success: Mapped[int] = mapped_column(Integer, nullable=False, server_default=text("0"))
    pairs_failed: Mapped[int] = mapped_column(Integer, nullable=False, server_default=text("0"))
    rates_inserted: Mapped[int] = mapped_column(Integer, nullable=False, server_default=text("0"))
    derived_inserted: Mapped[int] = mapped_column(Integer, nullable=False, server_default=text("0"))
    last_error: Mapped[str | None] = mapped_column(Text)
    created_at: Mapped[datetime] = mapped_column(
        DateTime(timezone=True), nullable=False, server_default=func.now()
    )
