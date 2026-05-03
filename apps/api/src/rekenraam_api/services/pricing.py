from rekenraam_api.db.models.metadata import Commodity
from rekenraam_api.db.models.pricing import PriceSource, PricingPolicy, PricingRefreshState, PricingSourceAssignment
from rekenraam_api.repositories.pricing import PricingRepository
from rekenraam_api.schemas.pricing import (
    PriceSourceSummary,
    PricingPolicySummary,
    PricingPolicyUpdateInput,
    PricingRefreshStateSummary,
    PricingSourceAssignmentCreateInput,
    PricingSourceAssignmentSummary,
    PricingSourceAssignmentUpdateInput,
)


class PricingService:
    def __init__(self, repository: PricingRepository) -> None:
        self._repository = repository

    async def list_price_sources(self) -> list[PriceSourceSummary]:
        rows = await self._repository.list_price_sources()
        return [self._to_price_source_summary(row) for row in rows]

    async def get_pricing_policy(self, book_id: int) -> PricingPolicySummary | None:
        policy = await self._repository.get_pricing_policy(book_id)
        if policy is None:
            return None
        return await self._build_policy_summary(policy)

    async def update_pricing_policy(self, input: PricingPolicyUpdateInput) -> PricingPolicySummary | None:
        self._validate_policy_input(input)
        policy = await self._repository.update_pricing_policy(
            book_id=input.book_id,
            base_currency_id=input.base_currency_id,
            default_source_id=input.default_source_id,
            refresh_enabled=input.refresh_enabled,
            refresh_hour_utc=input.refresh_hour_utc,
            refresh_minute_utc=input.refresh_minute_utc,
            max_backfill_days=input.max_backfill_days,
            weekend_policy=input.weekend_policy,
        )
        if policy is None:
            return None
        return await self._build_policy_summary(policy)

    async def list_pricing_source_assignments(self, book_id: int) -> list[PricingSourceAssignmentSummary]:
        rows = await self._repository.list_pricing_source_assignments(book_id)
        return [self._to_assignment_summary(assignment, from_currency, to_currency, source) for assignment, from_currency, to_currency, source in rows]

    async def create_pricing_source_assignment(self, input: PricingSourceAssignmentCreateInput) -> PricingSourceAssignmentSummary:
        self._validate_assignment_dates(input.effective_from, input.effective_to)
        assignment = await self._repository.create_pricing_source_assignment(
            book_id=input.book_id,
            from_currency_id=input.from_currency_id,
            to_currency_id=input.to_currency_id,
            source_id=input.source_id,
            effective_from=input.effective_from,
            effective_to=input.effective_to,
        )
        return await self._build_assignment_summary(assignment)

    async def update_pricing_source_assignment(self, assignment_id: int, input: PricingSourceAssignmentUpdateInput) -> PricingSourceAssignmentSummary | None:
        self._validate_assignment_dates(input.effective_from, input.effective_to)
        assignment = await self._repository.update_pricing_source_assignment(
            assignment_id=assignment_id,
            book_id=input.book_id,
            from_currency_id=input.from_currency_id,
            to_currency_id=input.to_currency_id,
            source_id=input.source_id,
            effective_from=input.effective_from,
            effective_to=input.effective_to,
        )
        if assignment is None:
            return None
        return await self._build_assignment_summary(assignment)

    async def delete_pricing_source_assignment(self, assignment_id: int) -> bool:
        return await self._repository.delete_pricing_source_assignment(assignment_id)

    async def list_pricing_refresh_state(self, book_id: int) -> list[PricingRefreshStateSummary]:
        rows = await self._repository.list_pricing_refresh_state(book_id)
        return [self._to_refresh_state_summary(state, from_currency, to_currency, source) for state, from_currency, to_currency, source in rows]

    async def _build_policy_summary(self, policy: PricingPolicy) -> PricingPolicySummary:
        base_currency = await self._repository.get_commodity(policy.base_commodity_id)
        default_source = await self._repository.get_price_source(policy.default_source_id) if policy.default_source_id is not None else None
        return PricingPolicySummary(
            book_id=policy.book_id,
            base_currency_id=policy.base_commodity_id,
            base_currency_symbol=base_currency.symbol if base_currency is not None else None,
            default_source_id=policy.default_source_id,
            default_source_name=default_source.name if default_source is not None else None,
            refresh_enabled=policy.refresh_enabled,
            refresh_hour_utc=policy.refresh_hour_utc,
            refresh_minute_utc=policy.refresh_minute_utc,
            max_backfill_days=policy.max_backfill_days,
            weekend_policy=policy.weekend_policy,
            created_at=policy.created_at,
            updated_at=policy.updated_at,
        )

    async def _build_assignment_summary(self, assignment: PricingSourceAssignment) -> PricingSourceAssignmentSummary:
        from_currency = await self._repository.get_commodity(assignment.commodity_id)
        to_currency = await self._repository.get_commodity(assignment.quote_commodity_id)
        source = await self._repository.get_price_source(assignment.source_id)
        if from_currency is None or to_currency is None or source is None:
            raise ValueError("pricing source assignment references missing entities")
        return self._to_assignment_summary(assignment, from_currency, to_currency, source)

    @staticmethod
    def _to_price_source_summary(row: PriceSource) -> PriceSourceSummary:
        return PriceSourceSummary(
            id=row.id,
            name=row.name,
            kind=row.kind,
            country_code=None,
            website_url=row.base_url,
            notes=row.provider,
            created_at=row.created_at,
        )

    @staticmethod
    def _to_assignment_summary(
        assignment: PricingSourceAssignment,
        from_currency: Commodity,
        to_currency: Commodity,
        source: PriceSource,
    ) -> PricingSourceAssignmentSummary:
        return PricingSourceAssignmentSummary(
            id=assignment.id,
            book_id=assignment.book_id,
            from_currency_id=assignment.commodity_id,
            from_currency_symbol=from_currency.symbol,
            to_currency_id=assignment.quote_commodity_id,
            to_currency_symbol=to_currency.symbol,
            source_id=assignment.source_id,
            source_name=source.name,
            effective_from=assignment.effective_from,
            effective_to=assignment.effective_to,
            created_at=assignment.created_at,
            updated_at=assignment.updated_at,
        )

    @staticmethod
    def _to_refresh_state_summary(
        state: PricingRefreshState,
        from_currency: Commodity,
        to_currency: Commodity,
        source: PriceSource,
    ) -> PricingRefreshStateSummary:
        return PricingRefreshStateSummary(
            id=state.id,
            book_id=state.book_id,
            from_currency_id=state.commodity_id,
            from_currency_symbol=from_currency.symbol,
            to_currency_id=state.quote_commodity_id,
            to_currency_symbol=to_currency.symbol,
            source_id=state.source_id,
            source_name=source.name,
            last_success_date=state.last_success_date,
            last_attempt_at=state.last_attempt_at,
            last_error=state.last_error,
            created_at=state.created_at,
            updated_at=state.updated_at,
        )

    @staticmethod
    def _validate_policy_input(input: PricingPolicyUpdateInput) -> None:
        if input.refresh_hour_utc < 0 or input.refresh_hour_utc > 23:
            raise ValueError("refresh hour must be between 0 and 23")
        if input.refresh_minute_utc < 0 or input.refresh_minute_utc > 59:
            raise ValueError("refresh minute must be between 0 and 59")
        if input.max_backfill_days < 1:
            raise ValueError("max backfill days must be at least 1")
        if input.weekend_policy not in {"skip", "fill_previous", "download"}:
            raise ValueError("weekend policy must be skip, fill_previous, or download")

    @staticmethod
    def _validate_assignment_dates(effective_from: object, effective_to: object) -> None:
        if effective_to is not None and effective_to < effective_from:
            raise ValueError("effective_to must be on or after effective_from")