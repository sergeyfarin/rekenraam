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
async def test_metadata_repository_creates_updates_and_deletes_institution(repository_session: AsyncSession) -> None:
    repository = MetadataRepository(repository_session)

    created = await repository.create_institution(
        book_id=1,
        name="Example Bank",
        kind="bank",
        routing="123456789",
        website="https://example.test",
        metadata="Primary checking",
        country_id=None,
    )

    assert created.name == "Example Bank"
    assert created.routing == "123456789"
    assert created.website == "https://example.test"
    assert created.metadata_text == "Primary checking"

    updated = await repository.update_institution(
        institution_id=created.id,
        name="Example Credit Union",
        kind="credit_union",
        routing="987654321",
        website="https://credit.test",
        metadata="Updated",
        country_id=None,
    )

    assert updated is not None
    assert updated.name == "Example Credit Union"
    assert updated.routing == "987654321"
    assert updated.website == "https://credit.test"
    assert updated.metadata_text == "Updated"

    institutions = await repository.list_institutions()
    assert len(institutions) == 1
    assert institutions[0][0].name == "Example Credit Union"
    assert await repository.delete_institution(created.id) is True
    assert await repository.list_institutions() == []


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


@pytest.mark.asyncio
async def test_metadata_repository_updates_and_deletes_category_payee_and_tag(repository_session: AsyncSession) -> None:
    repository = MetadataRepository(repository_session)
    category = Category(book_id=1, parent_id=None, name="Groceries", kind="expense", color="#00aa00")
    payee = Payee(book_id=1, name="Local Market", kind="business", metadata_text=None)
    tag = Tag(book_id=1, name="Shared", color="#123456")
    repository_session.add_all([category, payee, tag])
    await repository_session.commit()

    updated_category = await repository.update_category(
        category_id=category.id,
        parent_id=None,
        name="Food",
        kind="expense",
        color="#112233",
    )
    updated_payee = await repository.update_payee(
        payee_id=payee.id,
        name="Corner Shop",
        kind="business",
        metadata="{\"source\":\"manual\"}",
    )
    updated_tag = await repository.update_tag(tag_id=tag.id, name="Family", color="#654321")

    assert updated_category is not None
    assert updated_category.name == "Food"
    assert updated_payee is not None
    assert updated_payee.name == "Corner Shop"
    assert updated_payee.metadata_text == '{"source":"manual"}'
    assert updated_tag is not None
    assert updated_tag.name == "Family"

    assert await repository.delete_category(category.id) is True
    assert await repository.delete_payee(payee.id) is True
    assert await repository.delete_tag(tag.id) is True
    assert await repository.list_categories() == []
    assert await repository.list_payees() == []
    assert await repository.list_tags() == []