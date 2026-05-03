from sqlalchemy.exc import IntegrityError

from rekenraam_api.repositories.metadata import MetadataRepository
from rekenraam_api.schemas.metadata import (
    CategoryCreateInput,
    CategorySummary,
    CategoryUpdateInput,
    CommoditySummary,
    CountrySummary,
    InstitutionCreateInput,
    InstitutionSummary,
    InstitutionUpdateInput,
    PayeeCreateInput,
    PayeeSummary,
    PayeeUpdateInput,
    PersonCreateInput,
    PersonSummary,
    ProjectCreateInput,
    ProjectSummary,
    TagCreateInput,
    TagSummary,
    TagUpdateInput,
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
                routing=institution.routing,
                website=institution.website,
                metadata=institution.metadata_text,
                country_id=institution.country_id,
                country_name=country.name if country is not None else None,
                created_at=institution.created_at,
                updated_at=institution.updated_at,
            )
            for institution, country in rows
        ]

    async def create_institution(self, input: InstitutionCreateInput) -> InstitutionSummary:
        name = input.name.strip()
        if not name:
            raise ValueError("name is required")
        kind = self._normalize_institution_kind(input.kind)
        row = await self._repository.create_institution(
            book_id=input.book_id,
            name=name,
            kind=kind,
            routing=self._clean_optional_text(input.routing),
            website=self._clean_optional_text(input.website),
            metadata=self._clean_optional_text(input.metadata),
            country_id=input.country_id,
        )
        return InstitutionSummary(
            id=row.id,
            book_id=row.book_id,
            name=row.name,
            kind=row.kind,
            routing=row.routing,
            website=row.website,
            metadata=row.metadata_text,
            country_id=row.country_id,
            country_name=None,
            created_at=row.created_at,
            updated_at=row.updated_at,
        )

    async def update_institution(self, institution_id: int, input: InstitutionUpdateInput) -> InstitutionSummary | None:
        name = input.name.strip()
        if not name:
            raise ValueError("name is required")
        kind = self._normalize_institution_kind(input.kind)
        row = await self._repository.update_institution(
            institution_id=institution_id,
            name=name,
            kind=kind,
            routing=self._clean_optional_text(input.routing),
            website=self._clean_optional_text(input.website),
            metadata=self._clean_optional_text(input.metadata),
            country_id=input.country_id,
        )
        if row is None:
            return None
        return InstitutionSummary(
            id=row.id,
            book_id=row.book_id,
            name=row.name,
            kind=row.kind,
            routing=row.routing,
            website=row.website,
            metadata=row.metadata_text,
            country_id=row.country_id,
            country_name=None,
            created_at=row.created_at,
            updated_at=row.updated_at,
        )

    async def delete_institution(self, institution_id: int) -> bool:
        try:
            return await self._repository.delete_institution(institution_id)
        except IntegrityError as error:
            raise ValueError("institution is still in use") from error

    async def list_categories(self) -> list[CategorySummary]:
        rows = await self._repository.list_categories()
        return [self._to_category_summary(row) for row in rows]

    async def create_category(self, input: CategoryCreateInput) -> CategorySummary:
        name = input.name.strip()
        kind = input.kind.strip().lower()
        if not name:
            raise ValueError("name is required")
        if kind not in {"expense", "income", "transfer"}:
            raise ValueError("category kind must be expense, income, or transfer")
        row = await self._repository.create_category(
            book_id=input.book_id,
            parent_id=input.parent_id,
            name=name,
            kind=kind,
            color=input.color,
        )
        return self._to_category_summary(row)

    async def update_category(self, category_id: int, input: CategoryUpdateInput) -> CategorySummary | None:
        name = input.name.strip()
        kind = input.kind.strip().lower()
        if not name:
            raise ValueError("name is required")
        if kind not in {"expense", "income", "transfer"}:
            raise ValueError("category kind must be expense, income, or transfer")

        row = await self._repository.update_category(
            category_id=category_id,
            parent_id=input.parent_id,
            name=name,
            kind=kind,
            color=input.color,
        )
        if row is None:
            return None
        return self._to_category_summary(row)

    async def delete_category(self, category_id: int) -> bool:
        try:
            return await self._repository.delete_category(category_id)
        except IntegrityError as error:
            raise ValueError("category is still in use") from error

    async def list_payees(self) -> list[PayeeSummary]:
        rows = await self._repository.list_payees()
        return [self._to_payee_summary(row) for row in rows]

    async def create_payee(self, input: PayeeCreateInput) -> PayeeSummary:
        name = input.name.strip()
        kind = input.kind.strip().lower()
        if not name:
            raise ValueError("name is required")
        if kind not in {"person", "business"}:
            raise ValueError("payee kind must be person or business")
        row = await self._repository.create_payee(
            book_id=input.book_id,
            name=name,
            kind=kind,
            metadata=input.metadata,
        )
        return self._to_payee_summary(row)

    async def update_payee(self, payee_id: int, input: PayeeUpdateInput) -> PayeeSummary | None:
        name = input.name.strip()
        kind = input.kind.strip().lower()
        if not name:
            raise ValueError("name is required")
        if kind not in {"person", "business"}:
            raise ValueError("payee kind must be person or business")

        row = await self._repository.update_payee(
            payee_id=payee_id,
            name=name,
            kind=kind,
            metadata=input.metadata,
        )
        if row is None:
            return None
        return self._to_payee_summary(row)

    async def delete_payee(self, payee_id: int) -> bool:
        try:
            return await self._repository.delete_payee(payee_id)
        except IntegrityError as error:
            raise ValueError("payee is still in use") from error

    async def list_tags(self) -> list[TagSummary]:
        rows = await self._repository.list_tags()
        return [self._to_tag_summary(row) for row in rows]

    async def create_tag(self, input: TagCreateInput) -> TagSummary:
        name = input.name.strip()
        if not name:
            raise ValueError("name is required")
        row = await self._repository.create_tag(book_id=input.book_id, name=name, color=input.color)
        return self._to_tag_summary(row)

    async def update_tag(self, tag_id: int, input: TagUpdateInput) -> TagSummary | None:
        name = input.name.strip()
        if not name:
            raise ValueError("name is required")

        row = await self._repository.update_tag(tag_id=tag_id, name=name, color=input.color)
        if row is None:
            return None
        return self._to_tag_summary(row)

    async def delete_tag(self, tag_id: int) -> bool:
        try:
            return await self._repository.delete_tag(tag_id)
        except IntegrityError as error:
            raise ValueError("tag is still in use") from error

    async def list_people(self) -> list[PersonSummary]:
        rows = await self._repository.list_people()
        return [self._to_person_summary(row) for row in rows]

    async def create_person(self, input: PersonCreateInput) -> PersonSummary:
        name = input.name.strip()
        role = input.role.strip().lower()
        if not name:
            raise ValueError("name is required")
        if role not in {"member", "household", "vendor", "contact"}:
            raise ValueError("person role must be member, household, vendor, or contact")
        row = await self._repository.create_person(
            book_id=input.book_id,
            name=name,
            role=role,
            metadata=input.metadata,
        )
        return self._to_person_summary(row)

    async def list_projects(self) -> list[ProjectSummary]:
        rows = await self._repository.list_projects()
        return [self._to_project_summary(row) for row in rows]

    async def create_project(self, input: ProjectCreateInput) -> ProjectSummary:
        name = input.name.strip()
        status = input.status.strip().lower()
        if not name:
            raise ValueError("name is required")
        if status not in {"active", "on_hold", "completed", "archived"}:
            raise ValueError("project status must be active, on_hold, completed, or archived")
        row = await self._repository.create_project(
            book_id=input.book_id,
            name=name,
            status=status,
            metadata=input.metadata,
        )
        return self._to_project_summary(row)

    @staticmethod
    def _to_category_summary(row: object) -> CategorySummary:
        return CategorySummary(
            id=row.id,
            book_id=row.book_id,
            parent_id=row.parent_id,
            name=row.name,
            kind=row.kind,
            color=row.color,
            created_at=row.created_at,
            updated_at=row.updated_at,
        )

    @staticmethod
    def _to_payee_summary(row: object) -> PayeeSummary:
        return PayeeSummary(
            id=row.id,
            book_id=row.book_id,
            name=row.name,
            kind=row.kind,
            metadata=row.metadata_text,
            created_at=row.created_at,
            updated_at=row.updated_at,
        )

    @staticmethod
    def _to_tag_summary(row: object) -> TagSummary:
        return TagSummary(
            id=row.id,
            book_id=row.book_id,
            name=row.name,
            color=row.color,
            created_at=row.created_at,
            updated_at=row.updated_at,
        )

    @staticmethod
    def _clean_optional_text(value: str | None) -> str | None:
        if value is None:
            return None
        cleaned = value.strip()
        return cleaned or None

    @staticmethod
    def _normalize_institution_kind(value: str | None) -> str | None:
        if value is None:
            return None
        cleaned = value.strip().lower()
        if not cleaned:
            return None
        if cleaned not in {"bank", "credit_union", "brokerage", "insurance", "employer", "other"}:
            raise ValueError("institution kind must be bank, credit_union, brokerage, insurance, employer, or other")
        return cleaned

    @staticmethod
    def _to_person_summary(row: object) -> PersonSummary:
        return PersonSummary(
            id=row.id,
            book_id=row.book_id,
            name=row.name,
            role=row.role,
            metadata=row.metadata_text,
            created_at=row.created_at,
            updated_at=row.updated_at,
        )

    @staticmethod
    def _to_project_summary(row: object) -> ProjectSummary:
        return ProjectSummary(
            id=row.id,
            book_id=row.book_id,
            name=row.name,
            status=row.status,
            metadata=row.metadata_text,
            created_at=row.created_at,
            updated_at=row.updated_at,
        )