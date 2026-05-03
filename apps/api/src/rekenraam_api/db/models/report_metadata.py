from datetime import datetime

from sqlalchemy import BigInteger, CheckConstraint, DateTime, ForeignKey, Index, Integer, String, Text, UniqueConstraint, func
from sqlalchemy.orm import Mapped, mapped_column

from rekenraam_api.db.base import Base


class ReportDefinition(Base):
    __tablename__ = "report_definitions"
    __table_args__ = (
        Index("ix_report_definitions_book", "book_id"),
        Index("ix_report_definitions_kind", "book_id", "kind"),
        Index("ix_report_definitions_previous", "previous_report_definition_id"),
        UniqueConstraint("previous_report_definition_id", name="uq_report_definitions_previous"),
        CheckConstraint("kind IN ('builtin', 'custom')", name="ck_report_definitions_kind"),
        CheckConstraint("query_type IN ('sql', 'template')", name="ck_report_definitions_query_type"),
    )

    id: Mapped[int] = mapped_column(BigInteger, primary_key=True, autoincrement=True)
    previous_report_definition_id: Mapped[int | None] = mapped_column(ForeignKey("report_definitions.id", ondelete="SET NULL"))
    session_id: Mapped[str | None] = mapped_column(String(255))
    book_id: Mapped[int] = mapped_column(ForeignKey("books.id", ondelete="CASCADE"), nullable=False)
    name: Mapped[str] = mapped_column(String(200), nullable=False)
    kind: Mapped[str] = mapped_column(String(20), nullable=False, server_default="custom")
    query_type: Mapped[str] = mapped_column(String(20), nullable=False, server_default="sql")
    query_text: Mapped[str] = mapped_column(Text, nullable=False)
    params_schema: Mapped[str | None] = mapped_column(Text)
    created_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), nullable=False, server_default=func.now())


class ReportRun(Base):
    __tablename__ = "report_runs"
    __table_args__ = (
        Index("ix_report_runs_book_def", "book_id", "definition_id"),
        Index("ix_report_runs_book_seq", "book_id", "as_of_seq"),
        UniqueConstraint("book_id", "definition_id", "params_hash", "as_of_seq", name="uq_report_runs_entry"),
        CheckConstraint("pricing_mode IN ('frozen', 'latest_corrected')", name="ck_report_runs_pricing_mode"),
    )

    id: Mapped[int] = mapped_column(BigInteger, primary_key=True, autoincrement=True)
    book_id: Mapped[int] = mapped_column(ForeignKey("books.id", ondelete="CASCADE"), nullable=False)
    definition_id: Mapped[int] = mapped_column(ForeignKey("report_definitions.id", ondelete="CASCADE"), nullable=False)
    params_hash: Mapped[str] = mapped_column(String(128), nullable=False)
    as_of_seq: Mapped[int] = mapped_column(Integer, nullable=False)
    pricing_mode: Mapped[str] = mapped_column(String(32), nullable=False, server_default="latest_corrected")
    pricing_policy_id: Mapped[int | None] = mapped_column(ForeignKey("pricing_policies.id", ondelete="SET NULL"))
    pricing_policy_version: Mapped[str | None] = mapped_column(String(255))
    valuation_snapshot_id: Mapped[int | None] = mapped_column(BigInteger)
    pricing_resolved_at: Mapped[datetime | None] = mapped_column(DateTime(timezone=True))
    result_json: Mapped[str] = mapped_column(Text, nullable=False)
    created_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), nullable=False, server_default=func.now())