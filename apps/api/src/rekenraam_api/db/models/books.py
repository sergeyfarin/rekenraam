from datetime import datetime

from sqlalchemy import Boolean, DateTime, String, func
from sqlalchemy.orm import Mapped, mapped_column

from rekenraam_api.db.base import Base


class AppSchemaState(Base):
    __tablename__ = "app_schema_state"

    id: Mapped[bool] = mapped_column(Boolean, primary_key=True)
    schema_version: Mapped[str] = mapped_column(String(64), nullable=False)
    migrated_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), nullable=False, server_default=func.now())


class Book(Base):
    __tablename__ = "books"

    id: Mapped[int] = mapped_column(primary_key=True, autoincrement=True)
    slug: Mapped[str] = mapped_column(String(120), nullable=False, unique=True)
    name: Mapped[str] = mapped_column(String(200), nullable=False)
    base_currency_code: Mapped[str] = mapped_column(String(3), nullable=False)