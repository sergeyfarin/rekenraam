from datetime import datetime

from pydantic import BaseModel, ConfigDict


class CommoditySummary(BaseModel):
    model_config = ConfigDict(frozen=True)

    id: int
    book_id: int
    kind: str
    symbol: str | None
    name: str
    scale: int
    metadata: str | None
    created_at: datetime
    updated_at: datetime


class CountrySummary(BaseModel):
    model_config = ConfigDict(frozen=True)

    id: int
    book_id: int
    code: str
    name: str
    created_at: datetime
    updated_at: datetime


class InstitutionSummary(BaseModel):
    model_config = ConfigDict(frozen=True)

    id: int
    book_id: int
    name: str
    kind: str | None
    country_id: int | None
    country_name: str | None
    created_at: datetime
    updated_at: datetime


class CategorySummary(BaseModel):
    model_config = ConfigDict(frozen=True)

    id: int
    book_id: int
    parent_id: int | None
    name: str
    kind: str
    color: str | None
    created_at: datetime
    updated_at: datetime


class PayeeSummary(BaseModel):
    model_config = ConfigDict(frozen=True)

    id: int
    book_id: int
    name: str
    kind: str
    metadata: str | None
    created_at: datetime
    updated_at: datetime


class TagSummary(BaseModel):
    model_config = ConfigDict(frozen=True)

    id: int
    book_id: int
    name: str
    color: str | None
    created_at: datetime
    updated_at: datetime


class PersonSummary(BaseModel):
    model_config = ConfigDict(frozen=True)

    id: int
    book_id: int
    name: str
    role: str
    metadata: str | None
    created_at: datetime
    updated_at: datetime


class ProjectSummary(BaseModel):
    model_config = ConfigDict(frozen=True)

    id: int
    book_id: int
    name: str
    status: str
    metadata: str | None
    created_at: datetime
    updated_at: datetime