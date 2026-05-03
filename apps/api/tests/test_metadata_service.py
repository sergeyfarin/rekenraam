from datetime import UTC, datetime

import pytest

from rekenraam_api.db.models.metadata import Category, Commodity, Country, Institution, Payee, Person, Project, Tag
from rekenraam_api.services.metadata import MetadataService


class StubMetadataRepository:
    _created_at = datetime(2026, 5, 3, tzinfo=UTC)

    async def list_commodities(self) -> list[Commodity]:
        return [
            Commodity(
                id=1,
                book_id=1,
                kind="currency",
                symbol="USD",
                name="US Dollar",
                scale=2,
                metadata_text=None,
                created_at=self._created_at,
                updated_at=self._created_at,
            )
        ]

    async def list_countries(self) -> list[Country]:
        return []

    async def list_institutions(self) -> list[tuple[Institution, Country | None]]:
        return []

    async def list_categories(self) -> list[Category]:
        return [
            Category(
                id=1,
                book_id=1,
                parent_id=None,
                name="Groceries",
                kind="expense",
                color="#00aa00",
                created_at=self._created_at,
                updated_at=self._created_at,
            )
        ]

    async def list_payees(self) -> list[Payee]:
        return [
            Payee(
                id=1,
                book_id=1,
                name="Local Market",
                kind="business",
                metadata_text=None,
                created_at=self._created_at,
                updated_at=self._created_at,
            )
        ]

    async def list_tags(self) -> list[Tag]:
        return []

    async def list_people(self) -> list[Person]:
        return [
            Person(
                id=1,
                book_id=1,
                name="Alex",
                role="household",
                metadata_text=None,
                created_at=self._created_at,
                updated_at=self._created_at,
            )
        ]

    async def list_projects(self) -> list[Project]:
        return [
            Project(
                id=1,
                book_id=1,
                name="Kitchen Remodel",
                status="active",
                metadata_text=None,
                created_at=self._created_at,
                updated_at=self._created_at,
            )
        ]


@pytest.mark.asyncio
async def test_metadata_service_maps_reference_data() -> None:
    service = MetadataService(StubMetadataRepository())

    commodities = await service.list_commodities()
    countries = await service.list_countries()
    institutions = await service.list_institutions()
    categories = await service.list_categories()
    payees = await service.list_payees()
    tags = await service.list_tags()
    people = await service.list_people()
    projects = await service.list_projects()

    assert commodities[0].name == "US Dollar"
    assert commodities[0].symbol == "USD"
    assert countries == []
    assert institutions == []
    assert categories[0].kind == "expense"
    assert payees[0].name == "Local Market"
    assert tags == []
    assert people[0].role == "household"
    assert projects[0].status == "active"