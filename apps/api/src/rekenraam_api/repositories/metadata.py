from datetime import UTC, datetime

from sqlalchemy import Select, select
from sqlalchemy.exc import IntegrityError
from sqlalchemy.ext.asyncio import AsyncSession

from rekenraam_api.db.models.books import Book
from rekenraam_api.db.models.metadata import (
    Category,
    Commodity,
    Country,
    Institution,
    Payee,
    Person,
    Project,
    Tag,
)


class MetadataRepository:
    def __init__(self, session: AsyncSession) -> None:
        self._session = session

    async def list_commodities(self, book_id: int) -> list[Commodity]:
        statement: Select[tuple[Commodity]] = (
            select(Commodity)
            .where(Commodity.book_id == book_id)
            .order_by(Commodity.name.asc(), Commodity.id.asc())
        )
        result = await self._session.execute(statement)
        return list(result.scalars().all())

    async def list_book_commodities(self, book_id: int) -> list[Commodity]:
        statement: Select[tuple[Commodity]] = (
            select(Commodity)
            .where(Commodity.book_id == book_id)
            .order_by(Commodity.name.asc(), Commodity.id.asc())
        )
        result = await self._session.execute(statement)
        return list(result.scalars().all())

    async def get_commodity(self, commodity_id: int) -> Commodity | None:
        return await self._session.get(Commodity, commodity_id)

    async def update_commodity(
        self,
        *,
        commodity_id: int,
        symbol: str | None,
        name: str,
        metadata: str | None,
    ) -> Commodity | None:
        commodity = await self._session.get(Commodity, commodity_id)
        if commodity is None:
            return None

        commodity.symbol = symbol
        commodity.name = name
        commodity.metadata_text = metadata
        commodity.updated_at = datetime.now(UTC)
        await self._session.commit()
        await self._session.refresh(commodity)
        return commodity

    async def list_currencies(self, book_id: int) -> tuple[list[Commodity], str | None]:
        statement: Select[tuple[Commodity]] = (
            select(Commodity)
            .where(Commodity.book_id == book_id, Commodity.kind == "currency")
            .order_by(Commodity.symbol.asc(), Commodity.id.asc())
        )
        result = await self._session.execute(statement)
        base_currency_code = await self._session.scalar(
            select(Book.base_currency_code).where(Book.id == book_id)
        )
        return list(result.scalars().all()), base_currency_code

    async def create_currency(
        self, *, book_id: int, symbol: str, name: str, scale: int, metadata: str | None
    ) -> Commodity:
        duplicate = await self._session.scalar(
            select(Commodity.id).where(
                Commodity.book_id == book_id,
                Commodity.kind == "currency",
                Commodity.symbol == symbol,
            )
        )
        if duplicate is not None:
            raise ValueError("currency with this symbol already exists")

        commodity = Commodity(
            book_id=book_id,
            kind="currency",
            symbol=symbol,
            name=name,
            scale=scale,
            metadata_text=metadata,
        )
        self._session.add(commodity)
        await self._session.commit()
        await self._session.refresh(commodity)
        return commodity

    async def update_currency(
        self, *, currency_id: int, symbol: str, name: str, scale: int, metadata: str | None
    ) -> Commodity | None:
        commodity = await self._session.get(Commodity, currency_id)
        if commodity is None:
            return None
        if commodity.kind != "currency":
            raise ValueError("currency not found")

        duplicate = await self._session.scalar(
            select(Commodity.id).where(
                Commodity.book_id == commodity.book_id,
                Commodity.kind == "currency",
                Commodity.symbol == symbol,
                Commodity.id != currency_id,
            )
        )
        if duplicate is not None:
            raise ValueError("currency with this symbol already exists")

        previous_symbol = commodity.symbol
        commodity.symbol = symbol
        commodity.name = name
        commodity.scale = scale
        commodity.metadata_text = metadata
        commodity.updated_at = datetime.now(UTC)

        if previous_symbol and previous_symbol != symbol:
            book = await self._session.get(Book, commodity.book_id)
            if book is not None and book.base_currency_code == previous_symbol:
                book.base_currency_code = symbol
                book.updated_at = datetime.now(UTC)

        await self._session.commit()
        await self._session.refresh(commodity)
        return commodity

    async def set_default_currency(self, *, book_id: int, currency_id: int) -> Commodity | None:
        commodity = await self._session.get(Commodity, currency_id)
        if commodity is None or commodity.kind != "currency" or commodity.book_id != book_id:
            return None
        if commodity.symbol is None:
            raise ValueError("currency symbol is required")

        book = await self._session.get(Book, book_id)
        if book is None:
            return None

        book.base_currency_code = commodity.symbol
        book.updated_at = datetime.now(UTC)
        await self._session.commit()
        await self._session.refresh(commodity)
        return commodity

    async def set_currency_active(
        self, *, currency_id: int, metadata: str | None
    ) -> Commodity | None:
        commodity = await self._session.get(Commodity, currency_id)
        if commodity is None or commodity.kind != "currency":
            return None

        commodity.metadata_text = metadata
        commodity.updated_at = datetime.now(UTC)
        await self._session.commit()
        await self._session.refresh(commodity)
        return commodity

    async def list_countries(self) -> list[Country]:
        statement: Select[tuple[Country]] = select(Country).order_by(
            Country.name.asc(), Country.id.asc()
        )
        result = await self._session.execute(statement)
        return list(result.scalars().all())

    async def list_institutions(self, book_id: int) -> list[tuple[Institution, Country | None]]:
        statement = (
            select(Institution, Country)
            .outerjoin(Country, Country.id == Institution.country_id)
            .where(Institution.book_id == book_id)
            .order_by(Institution.name.asc(), Institution.id.asc())
        )
        result = await self._session.execute(statement)
        return list(result.tuples().all())

    async def get_institution(self, institution_id: int) -> Institution | None:
        return await self._session.get(Institution, institution_id)

    async def create_institution(
        self,
        *,
        book_id: int,
        name: str,
        kind: str | None,
        routing: str | None,
        website: str | None,
        metadata: str | None,
        country_id: int | None,
    ) -> Institution:
        institution = Institution(
            book_id=book_id,
            name=name,
            kind=kind,
            routing=routing,
            website=website,
            metadata_text=metadata,
            country_id=country_id,
        )
        self._session.add(institution)
        await self._session.commit()
        await self._session.refresh(institution)
        return institution

    async def update_institution(
        self,
        *,
        institution_id: int,
        name: str,
        kind: str | None,
        routing: str | None,
        website: str | None,
        metadata: str | None,
        country_id: int | None,
    ) -> Institution | None:
        institution = await self._session.get(Institution, institution_id)
        if institution is None:
            return None

        institution.name = name
        institution.kind = kind
        institution.routing = routing
        institution.website = website
        institution.metadata_text = metadata
        institution.country_id = country_id
        institution.updated_at = datetime.now(UTC)
        await self._session.commit()
        await self._session.refresh(institution)
        return institution

    async def delete_institution(self, institution_id: int) -> bool:
        institution = await self._session.get(Institution, institution_id)
        if institution is None:
            return False

        await self._session.delete(institution)
        try:
            await self._session.commit()
        except IntegrityError:
            await self._session.rollback()
            raise
        return True

    async def list_categories(self, book_id: int) -> list[Category]:
        statement: Select[tuple[Category]] = (
            select(Category)
            .where(Category.book_id == book_id)
            .order_by(Category.name.asc(), Category.id.asc())
        )
        result = await self._session.execute(statement)
        return list(result.scalars().all())

    async def get_category(self, category_id: int) -> Category | None:
        return await self._session.get(Category, category_id)

    async def create_category(
        self,
        *,
        book_id: int,
        parent_id: int | None,
        name: str,
        kind: str,
        color: str | None,
    ) -> Category:
        category = Category(book_id=book_id, parent_id=parent_id, name=name, kind=kind, color=color)
        self._session.add(category)
        await self._session.commit()
        await self._session.refresh(category)
        return category

    async def update_category(
        self,
        *,
        category_id: int,
        parent_id: int | None,
        name: str,
        kind: str,
        color: str | None,
    ) -> Category | None:
        category = await self._session.get(Category, category_id)
        if category is None:
            return None

        category.parent_id = parent_id
        category.name = name
        category.kind = kind
        category.color = color
        category.updated_at = datetime.now(UTC)
        await self._session.commit()
        await self._session.refresh(category)
        return category

    async def delete_category(self, category_id: int) -> bool:
        category = await self._session.get(Category, category_id)
        if category is None:
            return False

        await self._session.delete(category)
        try:
            await self._session.commit()
        except IntegrityError:
            await self._session.rollback()
            raise
        return True

    async def list_payees(self, book_id: int) -> list[Payee]:
        statement: Select[tuple[Payee]] = (
            select(Payee).where(Payee.book_id == book_id).order_by(Payee.name.asc(), Payee.id.asc())
        )
        result = await self._session.execute(statement)
        return list(result.scalars().all())

    async def get_payee(self, payee_id: int) -> Payee | None:
        return await self._session.get(Payee, payee_id)

    async def create_payee(
        self, *, book_id: int, name: str, kind: str, metadata: str | None
    ) -> Payee:
        payee = Payee(book_id=book_id, name=name, kind=kind, metadata_text=metadata)
        self._session.add(payee)
        await self._session.commit()
        await self._session.refresh(payee)
        return payee

    async def update_payee(
        self, *, payee_id: int, name: str, kind: str, metadata: str | None
    ) -> Payee | None:
        payee = await self._session.get(Payee, payee_id)
        if payee is None:
            return None

        payee.name = name
        payee.kind = kind
        payee.metadata_text = metadata
        payee.updated_at = datetime.now(UTC)
        await self._session.commit()
        await self._session.refresh(payee)
        return payee

    async def delete_payee(self, payee_id: int) -> bool:
        payee = await self._session.get(Payee, payee_id)
        if payee is None:
            return False

        await self._session.delete(payee)
        try:
            await self._session.commit()
        except IntegrityError:
            await self._session.rollback()
            raise
        return True

    async def list_tags(self, book_id: int) -> list[Tag]:
        statement: Select[tuple[Tag]] = (
            select(Tag).where(Tag.book_id == book_id).order_by(Tag.name.asc(), Tag.id.asc())
        )
        result = await self._session.execute(statement)
        return list(result.scalars().all())

    async def get_tag(self, tag_id: int) -> Tag | None:
        return await self._session.get(Tag, tag_id)

    async def create_tag(self, *, book_id: int, name: str, color: str | None) -> Tag:
        tag = Tag(book_id=book_id, name=name, color=color)
        self._session.add(tag)
        await self._session.commit()
        await self._session.refresh(tag)
        return tag

    async def update_tag(self, *, tag_id: int, name: str, color: str | None) -> Tag | None:
        tag = await self._session.get(Tag, tag_id)
        if tag is None:
            return None

        tag.name = name
        tag.color = color
        tag.updated_at = datetime.now(UTC)
        await self._session.commit()
        await self._session.refresh(tag)
        return tag

    async def delete_tag(self, tag_id: int) -> bool:
        tag = await self._session.get(Tag, tag_id)
        if tag is None:
            return False

        await self._session.delete(tag)
        try:
            await self._session.commit()
        except IntegrityError:
            await self._session.rollback()
            raise
        return True

    async def list_people(self, book_id: int) -> list[Person]:
        statement: Select[tuple[Person]] = (
            select(Person)
            .where(Person.book_id == book_id)
            .order_by(Person.name.asc(), Person.id.asc())
        )
        result = await self._session.execute(statement)
        return list(result.scalars().all())

    async def create_person(
        self, *, book_id: int, name: str, role: str, metadata: str | None
    ) -> Person:
        person = Person(book_id=book_id, name=name, role=role, metadata_text=metadata)
        self._session.add(person)
        await self._session.commit()
        await self._session.refresh(person)
        return person

    async def list_projects(self, book_id: int) -> list[Project]:
        statement: Select[tuple[Project]] = (
            select(Project)
            .where(Project.book_id == book_id)
            .order_by(Project.name.asc(), Project.id.asc())
        )
        result = await self._session.execute(statement)
        return list(result.scalars().all())

    async def create_project(
        self, *, book_id: int, name: str, status: str, metadata: str | None
    ) -> Project:
        project = Project(book_id=book_id, name=name, status=status, metadata_text=metadata)
        self._session.add(project)
        await self._session.commit()
        await self._session.refresh(project)
        return project
