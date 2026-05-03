from sqlalchemy import Select, select
from sqlalchemy.ext.asyncio import AsyncSession

from rekenraam_api.db.models.metadata import Category, Commodity, Country, Institution, Payee, Person, Project, Tag


class MetadataRepository:
    def __init__(self, session: AsyncSession) -> None:
        self._session = session

    async def list_commodities(self) -> list[Commodity]:
        statement: Select[tuple[Commodity]] = select(Commodity).order_by(Commodity.name.asc(), Commodity.id.asc())
        result = await self._session.execute(statement)
        return list(result.scalars().all())

    async def list_countries(self) -> list[Country]:
        statement: Select[tuple[Country]] = select(Country).order_by(Country.name.asc(), Country.id.asc())
        result = await self._session.execute(statement)
        return list(result.scalars().all())

    async def list_institutions(self) -> list[tuple[Institution, Country | None]]:
        statement = (
            select(Institution, Country)
            .outerjoin(Country, Country.id == Institution.country_id)
            .order_by(Institution.name.asc(), Institution.id.asc())
        )
        result = await self._session.execute(statement)
        return list(result.all())

    async def list_categories(self) -> list[Category]:
        statement: Select[tuple[Category]] = select(Category).order_by(Category.name.asc(), Category.id.asc())
        result = await self._session.execute(statement)
        return list(result.scalars().all())

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

    async def list_payees(self) -> list[Payee]:
        statement: Select[tuple[Payee]] = select(Payee).order_by(Payee.name.asc(), Payee.id.asc())
        result = await self._session.execute(statement)
        return list(result.scalars().all())

    async def create_payee(self, *, book_id: int, name: str, kind: str, metadata: str | None) -> Payee:
        payee = Payee(book_id=book_id, name=name, kind=kind, metadata_text=metadata)
        self._session.add(payee)
        await self._session.commit()
        await self._session.refresh(payee)
        return payee

    async def list_tags(self) -> list[Tag]:
        statement: Select[tuple[Tag]] = select(Tag).order_by(Tag.name.asc(), Tag.id.asc())
        result = await self._session.execute(statement)
        return list(result.scalars().all())

    async def create_tag(self, *, book_id: int, name: str, color: str | None) -> Tag:
        tag = Tag(book_id=book_id, name=name, color=color)
        self._session.add(tag)
        await self._session.commit()
        await self._session.refresh(tag)
        return tag

    async def list_people(self) -> list[Person]:
        statement: Select[tuple[Person]] = select(Person).order_by(Person.name.asc(), Person.id.asc())
        result = await self._session.execute(statement)
        return list(result.scalars().all())

    async def create_person(self, *, book_id: int, name: str, role: str, metadata: str | None) -> Person:
        person = Person(book_id=book_id, name=name, role=role, metadata_text=metadata)
        self._session.add(person)
        await self._session.commit()
        await self._session.refresh(person)
        return person

    async def list_projects(self) -> list[Project]:
        statement: Select[tuple[Project]] = select(Project).order_by(Project.name.asc(), Project.id.asc())
        result = await self._session.execute(statement)
        return list(result.scalars().all())

    async def create_project(self, *, book_id: int, name: str, status: str, metadata: str | None) -> Project:
        project = Project(book_id=book_id, name=name, status=status, metadata_text=metadata)
        self._session.add(project)
        await self._session.commit()
        await self._session.refresh(project)
        return project