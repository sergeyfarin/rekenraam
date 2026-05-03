import pytest
from sqlalchemy.ext.asyncio import AsyncSession

from rekenraam_api.db.models.metadata import Category, Payee, Person, Project, Tag
from rekenraam_api.repositories.metadata import MetadataRepository


@pytest.mark.asyncio
async def test_metadata_repository_lists_seeded_commodities(repository_session: AsyncSession) -> None:
    repository = MetadataRepository(repository_session)

    commodities = await repository.list_commodities()

    assert [commodity.name for commodity in commodities] == ["US Dollar"]
    assert commodities[0].symbol == "USD"


@pytest.mark.asyncio
async def test_metadata_repository_lists_empty_countries_and_institutions(repository_session: AsyncSession) -> None:
    repository = MetadataRepository(repository_session)

    countries = await repository.list_countries()
    institutions = await repository.list_institutions()

    assert countries == []
    assert institutions == []


@pytest.mark.asyncio
async def test_metadata_repository_lists_reference_data_by_name(repository_session: AsyncSession) -> None:
    repository = MetadataRepository(repository_session)
    repository_session.add_all(
        [
            Category(book_id=1, parent_id=None, name="Groceries", kind="expense", color="#00aa00"),
            Payee(book_id=1, name="Local Market", kind="business", metadata_text=None),
            Tag(book_id=1, name="Shared", color="#123456"),
            Person(book_id=1, name="Alex", role="household", metadata_text=None),
            Project(book_id=1, name="Kitchen Remodel", status="active", metadata_text=None),
        ]
    )
    await repository_session.commit()

    categories = await repository.list_categories()
    payees = await repository.list_payees()
    tags = await repository.list_tags()
    people = await repository.list_people()
    projects = await repository.list_projects()

    assert [category.name for category in categories] == ["Groceries"]
    assert [payee.name for payee in payees] == ["Local Market"]
    assert [tag.name for tag in tags] == ["Shared"]
    assert [person.name for person in people] == ["Alex"]
    assert [project.name for project in projects] == ["Kitchen Remodel"]