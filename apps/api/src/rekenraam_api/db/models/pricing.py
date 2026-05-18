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
    __audit_logged__ = True
    __table_args__ = (
        Index("ix_price_sources_kind", "kind"),
        UniqueConstraint("name", name="uq_price_sources_name"),
    )

    id: Mapped[int] = mapped_column(BigInteger, primary_key=True, autoincrement=True)
    name: Mapped[str] = mapped_column(String(100), nullable=False)
    kind: Mapped[str] = mapped_column(String(32), nullable=False, server_default="provider")
    provider: Mapped[str | None] = mapped_column(String(100))
    provider_key: Mapped[str | None] = mapped_column(String(100))
    plugin_id: Mapped[str | None] = mapped_column(String(100))
    base_url: Mapped[str | None] = mapped_column(String(255))
    created_at: Mapped[datetime] = mapped_column(
        DateTime(timezone=True), nullable=False, server_default=func.now()
    )


class CommodityPriceSource(Base):
    __tablename__ = "commodity_price_sources"
    __audit_logged__ = True
    __table_args__ = (
        Index("ix_commodity_price_sources_book", "book_id"),
        Index("ix_commodity_price_sources_commodity_source", "commodity_id", "source_id"),
        Index("ix_commodity_price_sources_previous", "previous_commodity_price_source_id"),
        CheckConstraint(
            "effective_to IS NULL OR effective_from IS NULL OR effective_to >= effective_from",
            name="ck_commodity_price_sources_effective_range",
        ),
    )

    id: Mapped[int] = mapped_column(BigInteger, primary_key=True, autoincrement=True)
    previous_commodity_price_source_id: Mapped[int | None] = mapped_column(
        ForeignKey("commodity_price_sources.id", ondelete="SET NULL")
    )
    book_id: Mapped[int] = mapped_column(ForeignKey("books.id", ondelete="CASCADE"), nullable=False)
    commodity_id: Mapped[int] = mapped_column(
        ForeignKey("commodities.id", ondelete="CASCADE"), nullable=False
    )
    source_id: Mapped[int] = mapped_column(
        ForeignKey("price_sources.id", ondelete="CASCADE"), nullable=False
    )
    symbol: Mapped[str] = mapped_column(String(100), nullable=False)
    provider_instrument_id: Mapped[str | None] = mapped_column(String(200))
    exchange_code: Mapped[str | None] = mapped_column(String(32))
    mic: Mapped[str | None] = mapped_column(String(16))
    name_override: Mapped[str | None] = mapped_column(String(200))
    is_primary: Mapped[bool] = mapped_column(Boolean, nullable=False, server_default=text("false"))
    metadata_json: Mapped[str | None] = mapped_column(Text)
    effective_from: Mapped[date | None] = mapped_column(Date)
    effective_to: Mapped[date | None] = mapped_column(Date)
    voided_at: Mapped[datetime | None] = mapped_column(DateTime(timezone=True))
    created_at: Mapped[datetime] = mapped_column(
        DateTime(timezone=True), nullable=False, server_default=func.now()
    )


class PricingPolicy(Base):
    __tablename__ = "pricing_policies"
    __audit_logged__ = True
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
        CheckConstraint("staleness_max_days >= 1", name="ck_pricing_policies_staleness_days"),
        CheckConstraint(
            "triangulation_max_hops >= 0 AND triangulation_max_hops <= 4",
            name="ck_pricing_policies_triangulation_hops",
        ),
        CheckConstraint(
            "weekend_policy IN ('skip', 'fill_previous', 'download')",
            name="ck_pricing_policies_weekend_policy",
        ),
        CheckConstraint(
            "rounding_mode IN ('half_up', 'half_even', 'down', 'up')",
            name="ck_pricing_policies_rounding_mode",
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
    staleness_max_days: Mapped[int] = mapped_column(
        Integer, nullable=False, server_default=text("3")
    )
    triangulation_max_hops: Mapped[int] = mapped_column(
        Integer, nullable=False, server_default=text("1")
    )
    rounding_mode: Mapped[str] = mapped_column(String(32), nullable=False, server_default="half_up")
    prefer_official_fx: Mapped[bool] = mapped_column(
        Boolean, nullable=False, server_default=text("true")
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
    __audit_logged__ = True
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
