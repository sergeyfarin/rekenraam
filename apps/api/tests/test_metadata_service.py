from datetime import UTC, datetime

import pytest

from rekenraam_api.db.models.metadata import Category, Commodity, Country, Institution, Payee, Person, Project, Tag
from rekenraam_api.schemas.metadata import (
    CategoryUpdateInput,
    InstitutionCreateInput,
    InstitutionUpdateInput,
    PayeeUpdateInput,
    TagUpdateInput,
)
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
        return [
            (
                Institution(
                    id=1,
                    book_id=1,
                    name="Example Bank",
                    kind="bank",
                    routing="123456789",
                    website="https://example.test",
                    metadata_text="Primary",
                    country_id=None,
                    created_at=self._created_at,
                    updated_at=self._created_at,
                ),
                None,
            )
        ]

    async def create_institution(self, **kwargs: object) -> Institution:
        return Institution(
            id=2,
            book_id=int(kwargs["book_id"]),
            name=str(kwargs["name"]),
            kind=kwargs["kind"],
            routing=kwargs["routing"],
            website=kwargs["website"],
            metadata_text=kwargs["metadata"],
            country_id=kwargs["country_id"],
            created_at=self._created_at,
            updated_at=self._created_at,
        )

    async def update_institution(self, *, institution_id: int, **kwargs: object) -> Institution | None:
        if institution_id != 1:
            return None
        return Institution(
            id=institution_id,
            book_id=1,
            name=str(kwargs["name"]),
            kind=kwargs["kind"],
            routing=kwargs["routing"],
            website=kwargs["website"],
            metadata_text=kwargs["metadata"],
            country_id=kwargs["country_id"],
            created_at=self._created_at,
            updated_at=self._created_at,
        )

    async def delete_institution(self, institution_id: int) -> bool:
        return institution_id == 1

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

    async def update_category(self, *, category_id: int, parent_id: int | None, name: str, kind: str, color: str | None) -> Category | None:
        if category_id != 1:
            return None
        return Category(
            id=category_id,
            book_id=1,
            parent_id=parent_id,
            name=name,
            kind=kind,
            color=color,
            created_at=self._created_at,
            updated_at=self._created_at,
        )

    async def delete_category(self, category_id: int) -> bool:
        return category_id == 1

    async def update_payee(self, *, payee_id: int, name: str, kind: str, metadata: str | None) -> Payee | None:
        if payee_id != 1:
            return None
        return Payee(
            id=payee_id,
            book_id=1,
            name=name,
            kind=kind,
            metadata_text=metadata,
            created_at=self._created_at,
            updated_at=self._created_at,
        )

    async def delete_payee(self, payee_id: int) -> bool:
        return payee_id == 1

    async def update_tag(self, *, tag_id: int, name: str, color: str | None) -> Tag | None:
        if tag_id != 1:
            return None
        return Tag(
            id=tag_id,
            book_id=1,
            name=name,
            color=color,
            created_at=self._created_at,
            updated_at=self._created_at,
        )

    async def delete_tag(self, tag_id: int) -> bool:
        return tag_id == 1


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
    assert institutions[0].routing == "123456789"
    assert institutions[0].website == "https://example.test"
    assert categories[0].kind == "expense"
    assert payees[0].name == "Local Market"
    assert tags == []
    assert people[0].role == "household"
    assert projects[0].status == "active"


@pytest.mark.asyncio
async def test_metadata_service_updates_and_deletes_supported_reference_data() -> None:
    service = MetadataService(StubMetadataRepository())

    institution = await service.create_institution(
        InstitutionCreateInput(
            book_id=1,
            name="New Bank",
            kind="bank",
            routing="111222333",
            website="https://newbank.test",
            metadata="Note",
            country_id=None,
        )
    )
    updated_institution = await service.update_institution(
        1,
        InstitutionUpdateInput(
            book_id=1,
            name="Updated Bank",
            kind="brokerage",
            routing="333222111",
            website="https://updated.test",
            metadata="Updated",
            country_id=None,
        ),
    )

    category = await service.update_category(
        1,
        input=CategoryUpdateInput(book_id=1, parent_id=None, name="Food", kind="expense", color="#111111"),
    )
    payee = await service.update_payee(
        1,
        input=PayeeUpdateInput(book_id=1, name="Corner Shop", kind="business", metadata=None),
    )
    tag = await service.update_tag(1, input=TagUpdateInput(book_id=1, name="Shared", color="#222222"))

    assert institution.name == "New Bank"
    assert institution.routing == "111222333"
    assert updated_institution is not None
    assert updated_institution.kind == "brokerage"
    assert category is not None
    assert category.name == "Food"
    assert payee is not None
    assert payee.name == "Corner Shop"
    assert tag is not None
    assert tag.color == "#222222"
    assert await service.delete_category(1) is True
    assert await service.delete_payee(1) is True
    assert await service.delete_tag(1) is True
    assert await service.delete_institution(1) is True