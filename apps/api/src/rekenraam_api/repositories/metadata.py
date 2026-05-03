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

    async def list_payees(self) -> list[Payee]:
        statement: Select[tuple[Payee]] = select(Payee).order_by(Payee.name.asc(), Payee.id.asc())
        result = await self._session.execute(statement)
        return list(result.scalars().all())

    async def list_tags(self) -> list[Tag]:
        statement: Select[tuple[Tag]] = select(Tag).order_by(Tag.name.asc(), Tag.id.asc())
        result = await self._session.execute(statement)
        return list(result.scalars().all())

    async def list_people(self) -> list[Person]:
        statement: Select[tuple[Person]] = select(Person).order_by(Person.name.asc(), Person.id.asc())
        result = await self._session.execute(statement)
        return list(result.scalars().all())

    async def list_projects(self) -> list[Project]:
        statement: Select[tuple[Project]] = select(Project).order_by(Project.name.asc(), Project.id.asc())
        result = await self._session.execute(statement)
        return list(result.scalars().all())