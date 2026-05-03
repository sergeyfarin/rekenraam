from rekenraam_api.repositories.metadata import MetadataRepository
from rekenraam_api.schemas.metadata import (
    CategorySummary,
    CommoditySummary,
    CountrySummary,
    InstitutionSummary,
    PayeeSummary,
    PersonSummary,
    ProjectSummary,
    TagSummary,
)


class MetadataService:
    def __init__(self, repository: MetadataRepository) -> None:
        self._repository = repository

    async def list_commodities(self) -> list[CommoditySummary]:
        rows = await self._repository.list_commodities()
        return [
            CommoditySummary(
                id=row.id,
                book_id=row.book_id,
                kind=row.kind,
                symbol=row.symbol,
                name=row.name,
                scale=row.scale,
                metadata=row.metadata_text,
                created_at=row.created_at,
                updated_at=row.updated_at,
            )
            for row in rows
        ]

    async def list_countries(self) -> list[CountrySummary]:
        rows = await self._repository.list_countries()
        return [
            CountrySummary(
                id=row.id,
                book_id=row.book_id,
                code=row.code,
                name=row.name,
                created_at=row.created_at,
                updated_at=row.updated_at,
            )
            for row in rows
        ]

    async def list_institutions(self) -> list[InstitutionSummary]:
        rows = await self._repository.list_institutions()
        return [
            InstitutionSummary(
                id=institution.id,
                book_id=institution.book_id,
                name=institution.name,
                kind=institution.kind,
                country_id=institution.country_id,
                country_name=country.name if country is not None else None,
                created_at=institution.created_at,
                updated_at=institution.updated_at,
            )
            for institution, country in rows
        ]

    async def list_categories(self) -> list[CategorySummary]:
        rows = await self._repository.list_categories()
        return [
            CategorySummary(
                id=row.id,
                book_id=row.book_id,
                parent_id=row.parent_id,
                name=row.name,
                kind=row.kind,
                color=row.color,
                created_at=row.created_at,
                updated_at=row.updated_at,
            )
            for row in rows
        ]

    async def list_payees(self) -> list[PayeeSummary]:
        rows = await self._repository.list_payees()
        return [
            PayeeSummary(
                id=row.id,
                book_id=row.book_id,
                name=row.name,
                kind=row.kind,
                metadata=row.metadata_text,
                created_at=row.created_at,
                updated_at=row.updated_at,
            )
            for row in rows
        ]

    async def list_tags(self) -> list[TagSummary]:
        rows = await self._repository.list_tags()
        return [
            TagSummary(
                id=row.id,
                book_id=row.book_id,
                name=row.name,
                color=row.color,
                created_at=row.created_at,
                updated_at=row.updated_at,
            )
            for row in rows
        ]

    async def list_people(self) -> list[PersonSummary]:
        rows = await self._repository.list_people()
        return [
            PersonSummary(
                id=row.id,
                book_id=row.book_id,
                name=row.name,
                role=row.role,
                metadata=row.metadata_text,
                created_at=row.created_at,
                updated_at=row.updated_at,
            )
            for row in rows
        ]

    async def list_projects(self) -> list[ProjectSummary]:
        rows = await self._repository.list_projects()
        return [
            ProjectSummary(
                id=row.id,
                book_id=row.book_id,
                name=row.name,
                status=row.status,
                metadata=row.metadata_text,
                created_at=row.created_at,
                updated_at=row.updated_at,
            )
            for row in rows
        ]