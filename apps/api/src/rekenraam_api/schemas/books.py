from pydantic import BaseModel, ConfigDict


class BookSummary(BaseModel):
    model_config = ConfigDict(frozen=True)

    id: int
    slug: str
    name: str
    base_currency_code: str