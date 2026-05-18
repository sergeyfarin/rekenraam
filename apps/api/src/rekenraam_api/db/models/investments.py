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


class Lot(Base):
    __tablename__ = "lots"
    __audit_logged__ = True
    __table_args__ = (
        Index("ix_lots_book_id", "book_id"),
        Index("ix_lots_account_commodity_opened", "account_id", "commodity_id", "opened_date"),
    )

    id: Mapped[int] = mapped_column(BigInteger, primary_key=True, autoincrement=True)
    book_id: Mapped[int] = mapped_column(ForeignKey("books.id", ondelete="CASCADE"), nullable=False)
    account_id: Mapped[int] = mapped_column(
        ForeignKey("accounts.id", ondelete="CASCADE"), nullable=False
    )
    commodity_id: Mapped[int] = mapped_column(
        ForeignKey("commodities.id", ondelete="RESTRICT"), nullable=False
    )
    opened_date: Mapped[date | None] = mapped_column(Date)
    notes: Mapped[str | None] = mapped_column(Text)
    cost_basis_minor: Mapped[int] = mapped_column(BigInteger, nullable=False)
    created_at: Mapped[datetime] = mapped_column(
        DateTime(timezone=True), nullable=False, server_default=func.now()
    )


class InvestmentInstrument(Base):
    __tablename__ = "investment_instruments"
    __audit_logged__ = True
    __table_args__ = (
        Index("ix_investment_instruments_book_id", "book_id"),
        Index("ix_investment_instruments_symbol", "book_id", "symbol"),
        CheckConstraint(
            "instrument_type IN ('stock', 'etf', 'mutual_fund', 'private_fund', 'bond', 'option', 'future', 'crypto', 'private_investment', 'generic')",
            name="ck_investment_instruments_type",
        ),
        CheckConstraint(
            "quantity_scale >= 0 AND quantity_scale <= 12",
            name="ck_investment_instruments_quantity_scale_range",
        ),
        CheckConstraint(
            "price_scale >= 0 AND price_scale <= 12",
            name="ck_investment_instruments_price_scale_range",
        ),
        UniqueConstraint(
            "book_id", "commodity_id", name="uq_investment_instruments_book_commodity"
        ),
    )

    id: Mapped[int] = mapped_column(BigInteger, primary_key=True, autoincrement=True)
    book_id: Mapped[int] = mapped_column(ForeignKey("books.id", ondelete="CASCADE"), nullable=False)
    commodity_id: Mapped[int] = mapped_column(
        ForeignKey("commodities.id", ondelete="CASCADE"), nullable=False
    )
    instrument_type: Mapped[str] = mapped_column(String(32), nullable=False)
    display_name: Mapped[str] = mapped_column(String(200), nullable=False)
    symbol: Mapped[str | None] = mapped_column(String(64))
    isin: Mapped[str | None] = mapped_column(String(32))
    cusip: Mapped[str | None] = mapped_column(String(32))
    figi: Mapped[str | None] = mapped_column(String(32))
    exchange: Mapped[str | None] = mapped_column(String(64))
    venue: Mapped[str | None] = mapped_column(String(64))
    issuer: Mapped[str | None] = mapped_column(String(200))
    country_code: Mapped[str | None] = mapped_column(String(3))
    quote_commodity_id: Mapped[int | None] = mapped_column(
        ForeignKey("commodities.id", ondelete="SET NULL")
    )
    trading_commodity_id: Mapped[int | None] = mapped_column(
        ForeignKey("commodities.id", ondelete="SET NULL")
    )
    quantity_scale: Mapped[int] = mapped_column(Integer, nullable=False, server_default=text("4"))
    price_scale: Mapped[int] = mapped_column(Integer, nullable=False, server_default=text("4"))
    is_active: Mapped[bool] = mapped_column(Boolean, nullable=False, server_default=text("true"))
    metadata_json: Mapped[str | None] = mapped_column(Text)
    created_at: Mapped[datetime] = mapped_column(
        DateTime(timezone=True), nullable=False, server_default=func.now()
    )
    updated_at: Mapped[datetime] = mapped_column(
        DateTime(timezone=True), nullable=False, server_default=func.now()
    )


class CostBasisProfile(Base):
    __tablename__ = "cost_basis_profiles"
    __audit_logged__ = True
    __table_args__ = (
        Index("ix_cost_basis_profiles_book_id", "book_id"),
        CheckConstraint(
            "method IN ('fifo', 'lifo', 'average_cost', 'specific_lot')",
            name="ck_cost_basis_profiles_method",
        ),
        UniqueConstraint("book_id", "name", name="uq_cost_basis_profiles_book_name"),
    )

    id: Mapped[int] = mapped_column(BigInteger, primary_key=True, autoincrement=True)
    book_id: Mapped[int] = mapped_column(ForeignKey("books.id", ondelete="CASCADE"), nullable=False)
    name: Mapped[str] = mapped_column(String(120), nullable=False)
    method: Mapped[str] = mapped_column(String(32), nullable=False, server_default="fifo")
    description: Mapped[str | None] = mapped_column(Text)
    is_default: Mapped[bool] = mapped_column(Boolean, nullable=False, server_default=text("false"))
    metadata_json: Mapped[str | None] = mapped_column(Text)
    created_at: Mapped[datetime] = mapped_column(
        DateTime(timezone=True), nullable=False, server_default=func.now()
    )
    updated_at: Mapped[datetime] = mapped_column(
        DateTime(timezone=True), nullable=False, server_default=func.now()
    )


class DividendIncomeCategory(Base):
    __tablename__ = "dividend_income_categories"
    __audit_logged__ = True
    __table_args__ = (
        Index("ix_dividend_income_categories_book", "book_id"),
        Index("ix_dividend_income_categories_commodity", "commodity_id"),
        Index("ix_dividend_income_categories_previous", "previous_dividend_income_category_id"),
        CheckConstraint(
            "default_tax_withheld_minor IS NULL OR default_tax_withheld_minor >= 0",
            name="ck_dividend_income_categories_tax_withheld_nonnegative",
        ),
        CheckConstraint(
            "withholding_rate_bps IS NULL OR (withholding_rate_bps >= 0 AND withholding_rate_bps <= 10000)",
            name="ck_dividend_income_categories_withholding_rate",
        ),
        CheckConstraint(
            "effective_to IS NULL OR effective_from IS NULL OR effective_to >= effective_from",
            name="ck_dividend_income_categories_effective_range",
        ),
    )

    id: Mapped[int] = mapped_column(BigInteger, primary_key=True, autoincrement=True)
    previous_dividend_income_category_id: Mapped[int | None] = mapped_column(
        ForeignKey("dividend_income_categories.id", ondelete="SET NULL")
    )
    book_id: Mapped[int] = mapped_column(ForeignKey("books.id", ondelete="CASCADE"), nullable=False)
    commodity_id: Mapped[int | None] = mapped_column(
        ForeignKey("commodities.id", ondelete="SET NULL")
    )
    income_account_id: Mapped[int] = mapped_column(
        ForeignKey("accounts.id", ondelete="RESTRICT"), nullable=False
    )
    category_id: Mapped[int] = mapped_column(
        ForeignKey("categories.id", ondelete="RESTRICT"), nullable=False
    )
    tax_withheld_account_id: Mapped[int | None] = mapped_column(
        ForeignKey("accounts.id", ondelete="SET NULL")
    )
    tax_withheld_category_id: Mapped[int | None] = mapped_column(
        ForeignKey("categories.id", ondelete="SET NULL")
    )
    default_tax_withheld_minor: Mapped[int | None] = mapped_column(BigInteger)
    withholding_rate_bps: Mapped[int | None] = mapped_column(Integer)
    tax_country_code: Mapped[str | None] = mapped_column(String(3))
    tax_treatment: Mapped[str | None] = mapped_column(String(64))
    notes: Mapped[str | None] = mapped_column(Text)
    metadata_json: Mapped[str | None] = mapped_column(Text)
    effective_from: Mapped[date | None] = mapped_column(Date)
    effective_to: Mapped[date | None] = mapped_column(Date)
    voided_at: Mapped[datetime | None] = mapped_column(DateTime(timezone=True))
    created_at: Mapped[datetime] = mapped_column(
        DateTime(timezone=True), nullable=False, server_default=func.now()
    )
    updated_at: Mapped[datetime] = mapped_column(
        DateTime(timezone=True), nullable=False, server_default=func.now()
    )


class CorporateAction(Base):
    __tablename__ = "corporate_actions"
    __audit_logged__ = True
    __table_args__ = (
        Index("ix_corporate_actions_book_effective", "book_id", "effective_date"),
        Index("ix_corporate_actions_instrument", "old_instrument_id", "new_instrument_id"),
        CheckConstraint(
            "action_type IN ('split', 'reverse_split', 'spin_off', 'merger', 'acquisition', 'conversion', 'return_of_capital', 'delisting', 'write_off', 'derivative_lifecycle', 'private_investment_event')",
            name="ck_corporate_actions_type",
        ),
    )

    id: Mapped[int] = mapped_column(BigInteger, primary_key=True, autoincrement=True)
    book_id: Mapped[int] = mapped_column(ForeignKey("books.id", ondelete="CASCADE"), nullable=False)
    action_type: Mapped[str] = mapped_column(String(32), nullable=False)
    old_instrument_id: Mapped[int | None] = mapped_column(
        ForeignKey("investment_instruments.id", ondelete="SET NULL")
    )
    new_instrument_id: Mapped[int | None] = mapped_column(
        ForeignKey("investment_instruments.id", ondelete="SET NULL")
    )
    effective_date: Mapped[date] = mapped_column(Date, nullable=False)
    ratio_numerator: Mapped[int | None] = mapped_column(BigInteger)
    ratio_denominator: Mapped[int | None] = mapped_column(BigInteger)
    cash_in_lieu_minor: Mapped[int | None] = mapped_column(BigInteger)
    cash_commodity_id: Mapped[int | None] = mapped_column(
        ForeignKey("commodities.id", ondelete="SET NULL")
    )
    source_reference: Mapped[str | None] = mapped_column(Text)
    memo: Mapped[str | None] = mapped_column(Text)
    generated_transaction_id: Mapped[int | None] = mapped_column(
        ForeignKey("transactions.id", ondelete="SET NULL")
    )
    metadata_json: Mapped[str | None] = mapped_column(Text)
    created_at: Mapped[datetime] = mapped_column(
        DateTime(timezone=True), nullable=False, server_default=func.now()
    )


class SplitLotAllocation(Base):
    __tablename__ = "split_lot_allocations"
    __audit_logged__ = True
    __table_args__ = (
        Index("ix_split_lot_allocations_split_id", "split_id"),
        Index("ix_split_lot_allocations_lot_id", "lot_id"),
        UniqueConstraint("split_id", "lot_id", name="uq_split_lot_allocations_split_lot"),
    )

    id: Mapped[int] = mapped_column(BigInteger, primary_key=True, autoincrement=True)
    split_id: Mapped[int] = mapped_column(
        ForeignKey("splits.id", ondelete="CASCADE"), nullable=False
    )
    lot_id: Mapped[int] = mapped_column(ForeignKey("lots.id", ondelete="CASCADE"), nullable=False)
    quantity_minor: Mapped[int] = mapped_column(BigInteger, nullable=False)
    created_at: Mapped[datetime] = mapped_column(
        DateTime(timezone=True), nullable=False, server_default=func.now()
    )


class PriceObservation(Base):
    __tablename__ = "price_observations"
    __audit_logged__ = True
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
            "observation_kind IN ('commodity_market', 'fx_daily', 'fx_manual', 'valuation_override')",
            name="ck_price_observations_observation_kind",
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
    observation_kind: Mapped[str] = mapped_column(
        String(32), nullable=False, server_default="commodity_market"
    )
    price_minor: Mapped[int] = mapped_column(BigInteger, nullable=False)
    price_date: Mapped[date] = mapped_column(Date, nullable=False)
    source: Mapped[str | None] = mapped_column(String(128))
    created_at: Mapped[datetime] = mapped_column(
        DateTime(timezone=True), nullable=False, server_default=func.now()
    )
