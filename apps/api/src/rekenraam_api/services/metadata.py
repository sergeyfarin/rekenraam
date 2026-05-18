import json
from typing import cast

from sqlalchemy.exc import IntegrityError

from rekenraam_api.db.models.metadata import Category, Commodity, Institution, Payee, Person, Project, Tag
from rekenraam_api.repositories.metadata import MetadataRepository
from rekenraam_api.schemas.metadata import (
    CategoryCreateInput,
    CategorySummary,
    CategoryUpdateInput,
    CommodityAutocompleteOption,
    CommoditySummary,
    CommodityUpdateInput,
    CountrySummary,
    CurrencyActivationInput,
    CurrencyCreateInput,
    CurrencySummary,
    CurrencyUpdateInput,
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
from rekenraam_api.services.access import AccessPolicy
from rekenraam_api.services.report_invalidation import bump_report_state


class MetadataService:
    def __init__(self, repository: MetadataRepository, access_policy: AccessPolicy | None = None) -> None:
        self._repository = repository
        self._access_policy = access_policy

    async def _require_book_read(self, book_id: int) -> None:
        if self._access_policy is not None:
            await self._access_policy.require_book_read(book_id)

    async def _require_book_write(self, book_id: int) -> None:
        if self._access_policy is not None:
            await self._access_policy.require_book_write(book_id)

    async def autocomplete_commodities(
        self,
        *,
        book_id: int,
        query: str,
        limit: int,
        active_only: bool,
    ) -> list[CommodityAutocompleteOption]:
        normalized = query.strip().lower()
        rows = await self._repository.list_book_commodities(book_id)
        _, base_currency_code = await self._repository.list_currencies(book_id)

        options: list[CommodityAutocompleteOption] = []
        for row in rows:
            symbol = row.symbol or ""
            symbol_lower = symbol.lower()
            name_lower = row.name.lower()
            is_active = row.kind != "currency" or self._currency_is_active(row, base_currency_code)
            is_default = (
                row.kind == "currency"
                and base_currency_code is not None
                and row.symbol == base_currency_code
            )
            if active_only and not is_active:
                continue
            if normalized and normalized not in symbol_lower and normalized not in name_lower:
                continue

            score = 10
            if symbol_lower == normalized:
                score = 120
            elif name_lower == normalized:
                score = 110
            elif symbol_lower.startswith(normalized):
                score = 100
            elif name_lower.startswith(normalized):
                score = 90
            elif normalized and normalized in symbol_lower:
                score = 70
            elif normalized and normalized in name_lower:
                score = 60

            options.append(
                CommodityAutocompleteOption(
                    id=row.id,
                    book_id=row.book_id,
                    kind=row.kind,
                    symbol=row.symbol,
                    name=row.name,
                    is_active=is_active,
                    is_default=is_default,
                    score=score,
                )
            )

        options.sort(
            key=lambda option: (
                -option.score,
                -int(option.is_default),
                -int(option.is_active),
                len(option.symbol or option.name),
                option.name.lower(),
            )
        )
        return options[: max(1, min(limit, 50))]

    async def list_commodities(self, book_id: int) -> list[CommoditySummary]:
        await self._require_book_read(book_id)
        rows = await self._repository.list_commodities(book_id)
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

    async def update_commodity(
        self, commodity_id: int, input: CommodityUpdateInput
    ) -> CommoditySummary | None:
        await self._require_book_write(input.book_id)
        name = input.name.strip()
        if not name:
            raise ValueError("name is required")

        row = await self._repository.update_commodity(
            commodity_id=commodity_id,
            symbol=self._clean_optional_text(input.symbol),
            name=name,
            metadata=self._clean_optional_text(input.metadata),
        )
        if row is None:
            return None
        await bump_report_state(getattr(self._repository, "_session", None), row.book_id)
        return CommoditySummary(
            id=row.id,
            book_id=row.book_id,
            kind=row.kind,
            symbol=row.symbol or "",
            name=row.name,
            scale=row.scale,
            metadata=row.metadata_text,
            created_at=row.created_at,
            updated_at=row.updated_at,
        )

    async def list_currencies(self, book_id: int) -> list[CurrencySummary]:
        await self._require_book_read(book_id)
        rows, base_currency_code = await self._repository.list_currencies(book_id)
        return [
            self._to_currency_summary(row, base_currency_code)
            for row in rows
            if row.symbol is not None
        ]

    async def create_currency(self, input: CurrencyCreateInput) -> CurrencySummary:
        await self._require_book_write(input.book_id)
        symbol = input.symbol.strip().upper()
        name = input.name.strip()
        if not symbol:
            raise ValueError("symbol is required")
        if not name:
            raise ValueError("name is required")
        if input.scale < 0 or input.scale > 12:
            raise ValueError("scale must be between 0 and 12")

        row = await self._repository.create_currency(
            book_id=input.book_id,
            symbol=symbol,
            name=name,
            scale=input.scale,
            metadata=self._currency_metadata_text(
                display_symbol=input.display_symbol, is_active=True
            ),
        )
        await bump_report_state(getattr(self._repository, "_session", None), input.book_id)
        return self._to_currency_summary(row, None)

    async def update_currency(
        self, currency_id: int, input: CurrencyUpdateInput
    ) -> CurrencySummary | None:
        await self._require_book_write(input.book_id)
        symbol = input.symbol.strip().upper()
        name = input.name.strip()
        if not symbol:
            raise ValueError("symbol is required")
        if not name:
            raise ValueError("name is required")
        if input.scale < 0 or input.scale > 12:
            raise ValueError("scale must be between 0 and 12")

        existing_rows, existing_base_currency_code = await self._repository.list_currencies(
            input.book_id
        )
        existing_row = next(
            (existing for existing in existing_rows if existing.id == currency_id), None
        )
        if existing_row is None:
            return None

        row = await self._repository.update_currency(
            currency_id=currency_id,
            symbol=symbol,
            name=name,
            scale=input.scale,
            metadata=self._currency_metadata_text(
                existing_metadata_text=existing_row.metadata_text,
                display_symbol=input.display_symbol,
                is_active=self._currency_is_active(existing_row, existing_base_currency_code),
            ),
        )
        if row is None:
            return None
        await bump_report_state(getattr(self._repository, "_session", None), input.book_id)
        rows, base_currency_code = await self._repository.list_currencies(input.book_id)
        effective_base_currency = base_currency_code
        if row.symbol is not None and any(existing.id == row.id for existing in rows):
            return self._to_currency_summary(row, effective_base_currency)
        return self._to_currency_summary(row, effective_base_currency)

    async def set_default_currency(
        self, *, book_id: int, currency_id: int
    ) -> CurrencySummary | None:
        await self._require_book_write(book_id)
        row = await self._repository.set_default_currency(book_id=book_id, currency_id=currency_id)
        if row is None:
            return None
        row = (
            await self._repository.set_currency_active(
                currency_id=currency_id,
                metadata=self._currency_metadata_text(
                    existing_metadata_text=row.metadata_text,
                    display_symbol=self._extract_display_symbol(row.metadata_text),
                    is_active=True,
                ),
            )
            or row
        )
        await bump_report_state(getattr(self._repository, "_session", None), book_id)
        return self._to_currency_summary(row, row.symbol, None)

    async def set_currency_active(
        self, *, currency_id: int, input: CurrencyActivationInput
    ) -> CurrencySummary | None:
        await self._require_book_write(input.book_id)
        rows, base_currency_code = await self._repository.list_currencies(input.book_id)
        existing_row = next((existing for existing in rows if existing.id == currency_id), None)
        if existing_row is None or existing_row.symbol is None:
            return None
        if (
            not input.is_active
            and base_currency_code is not None
            and existing_row.symbol == base_currency_code
        ):
            raise ValueError("default currency must remain active")

        row = await self._repository.set_currency_active(
            currency_id=currency_id,
            metadata=self._currency_metadata_text(
                existing_metadata_text=existing_row.metadata_text,
                display_symbol=self._extract_display_symbol(existing_row.metadata_text),
                is_active=input.is_active,
            ),
        )
        if row is None:
            return None
        await bump_report_state(getattr(self._repository, "_session", None), input.book_id)
        return self._to_currency_summary(row, base_currency_code, None)

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

    async def list_institutions(self, book_id: int) -> list[InstitutionSummary]:
        await self._require_book_read(book_id)
        rows = await self._repository.list_institutions(book_id)
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
        await self._require_book_write(input.book_id)
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
        await bump_report_state(getattr(self._repository, "_session", None), input.book_id)
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

    async def update_institution(
        self, institution_id: int, input: InstitutionUpdateInput
    ) -> InstitutionSummary | None:
        await self._require_book_write(input.book_id)
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
        await bump_report_state(getattr(self._repository, "_session", None), row.book_id)
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
        existing = await self._repository._session.get(Institution, institution_id)
        if existing is not None:
            await self._require_book_write(existing.book_id)
        institutions = await self._repository.list_institutions(existing.book_id) if existing is not None else []
        existing = next(
            (institution for institution, _ in institutions if institution.id == institution_id),
            None,
        )
        try:
            deleted = await self._repository.delete_institution(institution_id)
        except IntegrityError as error:
            raise ValueError("institution is still in use") from error
        if deleted and existing is not None:
            await bump_report_state(getattr(self._repository, "_session", None), existing.book_id)
        return deleted

    async def list_categories(self, book_id: int) -> list[CategorySummary]:
        await self._require_book_read(book_id)
        rows = await self._repository.list_categories(book_id)
        return [self._to_category_summary(row) for row in rows]

    async def create_category(self, input: CategoryCreateInput) -> CategorySummary:
        await self._require_book_write(input.book_id)
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
        await bump_report_state(getattr(self._repository, "_session", None), input.book_id)
        return self._to_category_summary(row)

    async def update_category(
        self, category_id: int, input: CategoryUpdateInput
    ) -> CategorySummary | None:
        await self._require_book_write(input.book_id)
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
        await bump_report_state(getattr(self._repository, "_session", None), row.book_id)
        return self._to_category_summary(row)

    async def delete_category(self, category_id: int) -> bool:
        existing = await self._repository._session.get(Category, category_id)
        if existing is not None:
            await self._require_book_write(existing.book_id)
        rows = await self._repository.list_categories(existing.book_id) if existing is not None else []
        existing = next((row for row in rows if row.id == category_id), None)
        try:
            deleted = await self._repository.delete_category(category_id)
        except IntegrityError as error:
            raise ValueError("category is still in use") from error
        if deleted and existing is not None:
            await bump_report_state(getattr(self._repository, "_session", None), existing.book_id)
        return deleted

    async def list_payees(self, book_id: int) -> list[PayeeSummary]:
        await self._require_book_read(book_id)
        rows = await self._repository.list_payees(book_id)
        return [self._to_payee_summary(row) for row in rows]

    async def create_payee(self, input: PayeeCreateInput) -> PayeeSummary:
        await self._require_book_write(input.book_id)
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
        await bump_report_state(getattr(self._repository, "_session", None), input.book_id)
        return self._to_payee_summary(row)

    async def update_payee(self, payee_id: int, input: PayeeUpdateInput) -> PayeeSummary | None:
        await self._require_book_write(input.book_id)
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
        await bump_report_state(getattr(self._repository, "_session", None), row.book_id)
        return self._to_payee_summary(row)

    async def delete_payee(self, payee_id: int) -> bool:
        existing = await self._repository._session.get(Payee, payee_id)
        if existing is not None:
            await self._require_book_write(existing.book_id)
        rows = await self._repository.list_payees(existing.book_id) if existing is not None else []
        existing = next((row for row in rows if row.id == payee_id), None)
        try:
            deleted = await self._repository.delete_payee(payee_id)
        except IntegrityError as error:
            raise ValueError("payee is still in use") from error
        if deleted and existing is not None:
            await bump_report_state(getattr(self._repository, "_session", None), existing.book_id)
        return deleted

    async def list_tags(self, book_id: int) -> list[TagSummary]:
        await self._require_book_read(book_id)
        rows = await self._repository.list_tags(book_id)
        return [self._to_tag_summary(row) for row in rows]

    async def create_tag(self, input: TagCreateInput) -> TagSummary:
        await self._require_book_write(input.book_id)
        name = input.name.strip()
        if not name:
            raise ValueError("name is required")
        row = await self._repository.create_tag(book_id=input.book_id, name=name, color=input.color)
        await bump_report_state(getattr(self._repository, "_session", None), input.book_id)
        return self._to_tag_summary(row)

    async def update_tag(self, tag_id: int, input: TagUpdateInput) -> TagSummary | None:
        await self._require_book_write(input.book_id)
        name = input.name.strip()
        if not name:
            raise ValueError("name is required")

        row = await self._repository.update_tag(tag_id=tag_id, name=name, color=input.color)
        if row is None:
            return None
        await bump_report_state(getattr(self._repository, "_session", None), row.book_id)
        return self._to_tag_summary(row)

    async def delete_tag(self, tag_id: int) -> bool:
        existing = await self._repository._session.get(Tag, tag_id)
        if existing is not None:
            await self._require_book_write(existing.book_id)
        rows = await self._repository.list_tags(existing.book_id) if existing is not None else []
        existing = next((row for row in rows if row.id == tag_id), None)
        try:
            deleted = await self._repository.delete_tag(tag_id)
        except IntegrityError as error:
            raise ValueError("tag is still in use") from error
        if deleted and existing is not None:
            await bump_report_state(getattr(self._repository, "_session", None), existing.book_id)
        return deleted

    async def list_people(self, book_id: int) -> list[PersonSummary]:
        await self._require_book_read(book_id)
        rows = await self._repository.list_people(book_id)
        return [self._to_person_summary(row) for row in rows]

    async def create_person(self, input: PersonCreateInput) -> PersonSummary:
        await self._require_book_write(input.book_id)
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
        await bump_report_state(getattr(self._repository, "_session", None), input.book_id)
        return self._to_person_summary(row)

    async def list_projects(self, book_id: int) -> list[ProjectSummary]:
        await self._require_book_read(book_id)
        rows = await self._repository.list_projects(book_id)
        return [self._to_project_summary(row) for row in rows]

    async def create_project(self, input: ProjectCreateInput) -> ProjectSummary:
        await self._require_book_write(input.book_id)
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
        await bump_report_state(getattr(self._repository, "_session", None), input.book_id)
        return self._to_project_summary(row)

    @staticmethod
    def _to_category_summary(row: Category) -> CategorySummary:
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
    def _to_payee_summary(row: Payee) -> PayeeSummary:
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
    def _to_tag_summary(row: Tag) -> TagSummary:
        return TagSummary(
            id=row.id,
            book_id=row.book_id,
            name=row.name,
            color=row.color,
            created_at=row.created_at,
            updated_at=row.updated_at,
        )

    @classmethod
    def _to_currency_summary(
        cls,
        row: Commodity,
        base_currency_code: str | None,
        display_symbol_override: str | None = None,
    ) -> CurrencySummary:
        display_symbol = (
            display_symbol_override
            if display_symbol_override not in {None, ""}
            else cls._extract_display_symbol(row.metadata_text)
        )
        if display_symbol in {None, ""}:
            display_symbol = row.symbol
        return CurrencySummary(
            id=row.id,
            book_id=row.book_id,
            symbol=row.symbol or "",
            display_symbol=display_symbol,
            name=row.name,
            scale=row.scale,
            is_active=cls._currency_is_active(row, base_currency_code),
            is_default=base_currency_code is not None and row.symbol == base_currency_code,
            created_at=row.created_at,
            updated_at=row.updated_at,
        )

    @classmethod
    def _currency_is_active(cls, row: Commodity, base_currency_code: str | None) -> bool:
        if (
            row.symbol is not None
            and base_currency_code is not None
            and row.symbol == base_currency_code
        ):
            return True
        is_active = cls._parse_currency_metadata(row.metadata_text).get("is_active")
        if isinstance(is_active, bool):
            return is_active
        return True

    @classmethod
    def _extract_display_symbol(cls, metadata_text: str | None) -> str | None:
        payload = cls._parse_currency_metadata(metadata_text)
        display_symbol = payload.get("display_symbol")
        if isinstance(display_symbol, str) and display_symbol.strip():
            return display_symbol
        return None

    @staticmethod
    def _parse_currency_metadata(metadata_text: str | None) -> dict[str, object]:
        if metadata_text is None:
            return {}
        try:
            payload = json.loads(metadata_text)
        except json.JSONDecodeError:
            return {}
        if not isinstance(payload, dict):
            return {}
        return {str(key): value for key, value in cast(dict[object, object], payload).items()}

    @classmethod
    def _currency_metadata_text(
        cls,
        *,
        existing_metadata_text: str | None = None,
        display_symbol: str | None,
        is_active: bool,
    ) -> str:
        payload = cls._parse_currency_metadata(existing_metadata_text)
        cleaned_display_symbol = display_symbol.strip() if display_symbol is not None else ""
        if cleaned_display_symbol:
            payload["display_symbol"] = cleaned_display_symbol
        else:
            payload.pop("display_symbol", None)
        payload["is_active"] = is_active
        return json.dumps(payload)

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
            raise ValueError(
                "institution kind must be bank, credit_union, brokerage, insurance, employer, or other"
            )
        return cleaned

    @staticmethod
    def _to_person_summary(row: Person) -> PersonSummary:
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
    def _to_project_summary(row: Project) -> ProjectSummary:
        return ProjectSummary(
            id=row.id,
            book_id=row.book_id,
            name=row.name,
            status=row.status,
            metadata=row.metadata_text,
            created_at=row.created_at,
            updated_at=row.updated_at,
        )
