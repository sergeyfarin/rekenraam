from __future__ import annotations

from typing import Any

from sqlalchemy import Integer, case, cast, func, literal
from sqlalchemy.dialects.postgresql import insert as postgres_insert
from sqlalchemy.dialects.sqlite import insert as sqlite_insert
from sqlalchemy.ext.asyncio import AsyncSession

from rekenraam_api.db.dialect import is_sqlite_session


def dialect_insert(session: AsyncSession, table: Any):
    if is_sqlite_session(session):
        return sqlite_insert(table)
    return postgres_insert(table)


def period_start_expr(session: AsyncSession, date_column: object, group_by: str):
    if not is_sqlite_session(session):
        if group_by == "year":
            return func.to_char(func.date_trunc("year", date_column), "YYYY-MM-DD")
        if group_by == "quarter":
            return func.to_char(func.date_trunc("quarter", date_column), "YYYY-MM-DD")
        if group_by == "day":
            return func.to_char(date_column, "YYYY-MM-DD")
        return func.to_char(func.date_trunc("month", date_column), "YYYY-MM-DD")

    if group_by == "year":
        return func.strftime("%Y-01-01", date_column)
    if group_by == "month":
        return func.strftime("%Y-%m-01", date_column)
    if group_by == "day":
        return func.strftime("%Y-%m-%d", date_column)

    month = cast(func.strftime("%m", date_column), Integer)
    year = func.strftime("%Y", date_column)
    return case(
        (month <= 3, year + literal("-01-01")),
        (month <= 6, year + literal("-04-01")),
        (month <= 9, year + literal("-07-01")),
        else_=year + literal("-10-01"),
    )
