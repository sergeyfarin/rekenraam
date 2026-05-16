from __future__ import annotations

# pyright: reportUnusedFunction=false
from sqlalchemy import BigInteger
from sqlalchemy.ext.compiler import compiles


@compiles(BigInteger, "sqlite")
def _compile_big_integer_sqlite(_type_: BigInteger, _compiler: object, **_kw: object) -> str:
    return "INTEGER"
