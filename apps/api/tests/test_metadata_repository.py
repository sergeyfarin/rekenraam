import pytest
from sqlalchemy.ext.asyncio import AsyncSession

from rekenraam_api.db.models.metadata import Category, Payee, Person, Project, Tag
from rekenraam_api.repositories.metadata import MetadataRepository


@pytest.mark.asyncio
async def test_metadata_repository_lists_seeded_commodities(
    repository_session: AsyncSession,
) -> None:
    repository = MetadataRepository(repository_session)

    commodities = await repository.list_commodities(1)

    assert [commodity.name for commodity in commodities] == ["US Dollar"]
    assert commodities[0].symbol == "USD"


@pytest.mark.asyncio
async def test_metadata_repository_updates_commodity(repository_session: AsyncSession) -> None:
    repository = MetadataRepository(repository_session)

    commodities = await repository.list_commodities(1)
    updated = await repository.update_commodity(
        commodity_id=commodities[0].id,
        symbol="USDX",
        name="US Dollar Updated",
        metadata="Primary currency",
    )

    assert updated is not None
    assert updated.symbol == "USDX"
    assert updated.name == "US Dollar Updated"
    assert updated.metadata_text == "Primary currency"


@pytest.mark.asyncio
async def test_metadata_repository_lists_creates_updates_and_sets_default_currency(
    repository_session: AsyncSession,
) -> None:
    repository = MetadataRepository(repository_session)

    currencies, base_currency_code = await repository.list_currencies(1)

    assert [currency.symbol for currency in currencies] == ["USD"]
    assert base_currency_code == "USD"

    created = await repository.create_currency(
        book_id=1, symbol="EUR", name="Euro", scale=2, metadata='{"display_symbol":"EUR"}'
    )
    assert created.symbol == "EUR"

    updated = await repository.update_currency(
        currency_id=created.id,
        symbol="EUR",
        name="Euro Updated",
        scale=2,
        metadata='{"display_symbol":"EUR"}',
    )
    assert updated is not None
    assert updated.name == "Euro Updated"
    assert updated.metadata_text == '{"display_symbol":"EUR"}'

    default_currency = await repository.set_default_currency(book_id=1, currency_id=created.id)
    assert default_currency is not None

    _, updated_base_currency_code = await repository.list_currencies(1)
    assert updated_base_currency_code == "EUR"

    deactivated = await repository.set_currency_active(
        currency_id=created.id, metadata='{"display_symbol":"EUR","is_active":false}'
    )
    assert deactivated is not None
    assert deactivated.metadata_text == '{"display_symbol":"EUR","is_active":false}'


@pytest.mark.asyncio
async def test_metadata_repository_lists_empty_countries_and_institutions(
    repository_session: AsyncSession,
) -> None:
    repository = MetadataRepository(repository_session)

    countries = await repository.list_countries()
    institutions = await repository.list_institutions(1)

    assert countries == []
    assert institutions == []


@pytest.mark.asyncio
async def test_metadata_repository_creates_updates_and_deletes_institution(
    repository_session: AsyncSession,
) -> None:
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

    institutions = await repository.list_institutions(1)
    assert len(institutions) == 1
    assert institutions[0][0].name == "Example Credit Union"
    assert await repository.delete_institution(created.id) is True
    assert await repository.list_institutions(1) == []


@pytest.mark.asyncio
async def test_metadata_repository_lists_reference_data_by_name(
    repository_session: AsyncSession,
) -> None:
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

    categories = await repository.list_categories(1)
    payees = await repository.list_payees(1)
    tags = await repository.list_tags(1)
    people = await repository.list_people(1)
    projects = await repository.list_projects(1)

    assert [category.name for category in categories] == ["Groceries"]
    assert [payee.name for payee in payees] == ["Local Market"]
    assert [tag.name for tag in tags] == ["Shared"]
    assert [person.name for person in people] == ["Alex"]
    assert [project.name for project in projects] == ["Kitchen Remodel"]


@pytest.mark.asyncio
async def test_metadata_repository_updates_and_deletes_category_payee_and_tag(
    repository_session: AsyncSession,
) -> None:
    repository = MetadataRepository(repository_session)
    category = Category(
        book_id=1, parent_id=None, name="Groceries", kind="expense", color="#00aa00"
    )
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
        metadata='{"source":"manual"}',
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
    assert await repository.list_categories(1) == []
    assert await repository.list_payees(1) == []
    assert await repository.list_tags(1) == []
