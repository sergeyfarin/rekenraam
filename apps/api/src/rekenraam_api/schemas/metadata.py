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


class CommodityUpdateInput(BaseModel):
    model_config = ConfigDict(frozen=True)

    book_id: int
    symbol: str | None
    name: str
    metadata: str | None


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
    routing: str | None
    website: str | None
    metadata: str | None
    country_id: int | None
    country_name: str | None
    created_at: datetime
    updated_at: datetime


class InstitutionCreateInput(BaseModel):
    model_config = ConfigDict(frozen=True)

    book_id: int
    name: str
    kind: str | None
    routing: str | None = None
    website: str | None = None
    metadata: str | None = None
    country_id: int | None = None


class InstitutionUpdateInput(BaseModel):
    model_config = ConfigDict(frozen=True)

    book_id: int
    name: str
    kind: str | None
    routing: str | None = None
    website: str | None = None
    metadata: str | None = None
    country_id: int | None = None


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


class CategoryCreateInput(BaseModel):
    model_config = ConfigDict(frozen=True)

    book_id: int
    parent_id: int | None
    name: str
    kind: str
    color: str | None


class CategoryUpdateInput(BaseModel):
    model_config = ConfigDict(frozen=True)

    book_id: int
    parent_id: int | None
    name: str
    kind: str
    color: str | None


class PayeeSummary(BaseModel):
    model_config = ConfigDict(frozen=True)

    id: int
    book_id: int
    name: str
    kind: str
    metadata: str | None
    created_at: datetime
    updated_at: datetime


class PayeeCreateInput(BaseModel):
    model_config = ConfigDict(frozen=True)

    book_id: int
    name: str
    kind: str
    metadata: str | None


class PayeeUpdateInput(BaseModel):
    model_config = ConfigDict(frozen=True)

    book_id: int
    name: str
    kind: str
    metadata: str | None


class TagSummary(BaseModel):
    model_config = ConfigDict(frozen=True)

    id: int
    book_id: int
    name: str
    color: str | None
    created_at: datetime
    updated_at: datetime


class TagCreateInput(BaseModel):
    model_config = ConfigDict(frozen=True)

    book_id: int
    name: str
    color: str | None


class TagUpdateInput(BaseModel):
    model_config = ConfigDict(frozen=True)

    book_id: int
    name: str
    color: str | None


class PersonSummary(BaseModel):
    model_config = ConfigDict(frozen=True)

    id: int
    book_id: int
    name: str
    role: str
    metadata: str | None
    created_at: datetime
    updated_at: datetime


class PersonCreateInput(BaseModel):
    model_config = ConfigDict(frozen=True)

    book_id: int
    name: str
    role: str
    metadata: str | None


class ProjectSummary(BaseModel):
    model_config = ConfigDict(frozen=True)

    id: int
    book_id: int
    name: str
    status: str
    metadata: str | None
    created_at: datetime
    updated_at: datetime


class ProjectCreateInput(BaseModel):
    model_config = ConfigDict(frozen=True)

    book_id: int
    name: str
    status: str
    metadata: str | None