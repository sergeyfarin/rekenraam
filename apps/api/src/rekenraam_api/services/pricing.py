from datetime import date
from decimal import Decimal

from rekenraam_api.db.models.metadata import Commodity
from rekenraam_api.db.models.investments import PriceObservation
from rekenraam_api.db.models.pricing import PriceSource, PricingPolicy, PricingRefreshState, PricingSourceAssignment
from rekenraam_api.repositories.pricing import PricingRepository
from rekenraam_api.services.report_invalidation import bump_report_state
from rekenraam_api.schemas.pricing import (
    FxRateDailyCreateInput,
    FxRateDailySummary,
    FxRateOfficialCreateInput,
    FxRateOfficialSummary,
    MarketPriceCreateInput,
    MarketPriceSummary,
    PriceSourceSummary,
    PricingSourceHealthSummary,
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
        await bump_report_state(getattr(self._repository, "_session", None), input.book_id)
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
        await bump_report_state(getattr(self._repository, "_session", None), input.book_id)
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
        await bump_report_state(getattr(self._repository, "_session", None), input.book_id)
        return await self._build_assignment_summary(assignment)

    async def delete_pricing_source_assignment(self, assignment_id: int) -> bool:
        assignment_row = await self._repository.get_pricing_source_assignment(assignment_id)
        deleted = await self._repository.delete_pricing_source_assignment(assignment_id)
        if deleted and assignment_row is not None:
            await bump_report_state(getattr(self._repository, "_session", None), assignment_row.book_id)
        return deleted

    async def list_pricing_refresh_state(self, book_id: int) -> list[PricingRefreshStateSummary]:
        rows = await self._repository.list_pricing_refresh_state(book_id)
        return [self._to_refresh_state_summary(state, from_currency, to_currency, source) for state, from_currency, to_currency, source in rows]

    async def list_source_health(self, book_id: int) -> list[PricingSourceHealthSummary]:
        rows = await self._repository.list_pricing_refresh_state(book_id)
        summaries: list[PricingSourceHealthSummary] = []
        for state, commodity, quote, source in rows:
            if state.last_error:
                status = "failed"
            elif state.last_success_date is None:
                status = "missing"
            else:
                status = "ok"
            summaries.append(
                PricingSourceHealthSummary(
                    book_id=state.book_id,
                    commodity_id=state.commodity_id,
                    commodity_symbol=commodity.symbol,
                    quote_commodity_id=state.quote_commodity_id,
                    quote_commodity_symbol=quote.symbol,
                    source_id=state.source_id,
                    source_name=source.name,
                    status=status,
                    last_success_date=state.last_success_date,
                    last_attempt_at=state.last_attempt_at,
                    last_error=state.last_error,
                )
            )
        return summaries

    async def list_market_prices(
        self,
        *,
        book_id: int,
        commodity_id: int | None,
        quote_commodity_id: int | None,
        limit: int,
    ) -> list[MarketPriceSummary]:
        rows = await self._repository.list_market_price_observations(
            book_id=book_id,
            commodity_id=commodity_id,
            quote_commodity_id=quote_commodity_id,
            limit=limit,
        )
        return [self._to_market_price_summary(observation, commodity, quote) for observation, commodity, quote in rows]

    async def create_market_price(self, input: MarketPriceCreateInput) -> MarketPriceSummary:
        if input.price_minor <= 0:
            raise ValueError("price_minor must be positive")
        observation = await self._repository.create_market_price_observation(
            book_id=input.book_id,
            commodity_id=input.commodity_id,
            quote_commodity_id=input.quote_commodity_id,
            price_date=input.price_date,
            price_minor=input.price_minor,
            source=self._clean_optional_text(input.source),
        )
        await bump_report_state(getattr(self._repository, "_session", None), input.book_id)
        commodity = await self._repository.get_commodity(observation.commodity_id)
        quote = await self._repository.get_commodity(observation.quote_commodity_id)
        if commodity is None or quote is None:
            raise ValueError("price observation references missing commodity")
        return self._to_market_price_summary(observation, commodity, quote)

    async def delete_market_price(self, observation_id: int) -> bool:
        observation = await self._repository._session.get(PriceObservation, observation_id)
        deleted = await self._repository.delete_market_price_observation(observation_id)
        if deleted and observation is not None:
            await bump_report_state(getattr(self._repository, "_session", None), observation.book_id)
        return deleted

    async def list_fx_rates_daily(self, book_id: int, limit: int = 100) -> list[FxRateDailySummary]:
        rows = await self._repository.list_fx_rate_daily_observations(book_id=book_id, limit=limit)
        return [self._to_fx_rate_daily_summary(observation, from_currency, to_currency) for observation, from_currency, to_currency in rows]

    async def create_fx_rate_daily(self, input: FxRateDailyCreateInput) -> FxRateDailySummary:
        if input.rate <= 0:
            raise ValueError("rate must be greater than zero")
        observation = await self._repository.create_fx_rate_daily_observation(
            book_id=input.book_id,
            from_currency_id=input.from_currency_id,
            to_currency_id=input.to_currency_id,
            rate_date=input.rate_date,
            rate=Decimal(str(input.rate)),
            source=self._clean_optional_text(input.source),
        )
        await bump_report_state(getattr(self._repository, "_session", None), input.book_id)
        from_currency = await self._repository.get_commodity(observation.commodity_id)
        to_currency = await self._repository.get_commodity(observation.quote_commodity_id)
        if from_currency is None or to_currency is None:
            raise ValueError("currency not found")
        return self._to_fx_rate_daily_summary(observation, from_currency, to_currency)

    async def delete_fx_rate_daily(self, observation_id: int) -> bool:
        observation = await self._repository._session.get(PriceObservation, observation_id)
        deleted = await self._repository.delete_fx_rate_daily_observation(observation_id)
        if deleted and observation is not None:
            await bump_report_state(getattr(self._repository, "_session", None), observation.book_id)
        return deleted

    async def list_fx_rates_official(self, book_id: int, limit: int = 100) -> list[FxRateOfficialSummary]:
        rows = await self._repository.list_fx_rate_official_observations(book_id=book_id, limit=limit)
        return [self._to_fx_rate_official_summary(observation, from_currency, to_currency) for observation, from_currency, to_currency in rows]

    async def create_fx_rate_official(self, input: FxRateOfficialCreateInput) -> FxRateOfficialSummary:
        if input.period_type not in {"yearly", "monthly"}:
            raise ValueError("period_type must be yearly or monthly")
        if input.period_type == "monthly" and (input.period_month is None or input.period_month < 1 or input.period_month > 12):
            raise ValueError("period_month must be between 1 and 12")
        if input.period_type == "yearly":
            period_month = None
            price_date = date(input.period_year, 1, 1)
        else:
            if input.period_month is None:
                raise ValueError("period_month must be between 1 and 12")
            period_month = input.period_month
            price_date = date(input.period_year, period_month, 1)
        if input.rate <= 0:
            raise ValueError("rate must be greater than zero")
        source_name = input.source_name.strip()
        if not source_name:
            raise ValueError("source_name is required")

        observation = await self._repository.create_fx_rate_official_observation(
            book_id=input.book_id,
            from_currency_id=input.from_currency_id,
            to_currency_id=input.to_currency_id,
            price_date=price_date,
            rate=Decimal(str(input.rate)),
            source_name=source_name,
        )
        await bump_report_state(getattr(self._repository, "_session", None), input.book_id)
        from_currency = await self._repository.get_commodity(observation.commodity_id)
        to_currency = await self._repository.get_commodity(observation.quote_commodity_id)
        if from_currency is None or to_currency is None:
            raise ValueError("currency not found")
        return self._to_fx_rate_official_summary(observation, from_currency, to_currency)

    async def delete_fx_rate_official(self, observation_id: int) -> bool:
        observation = await self._repository._session.get(PriceObservation, observation_id)
        deleted = await self._repository.delete_fx_rate_official_observation(observation_id)
        if deleted and observation is not None:
            await bump_report_state(getattr(self._repository, "_session", None), observation.book_id)
        return deleted

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
    def _to_fx_rate_daily_summary(
        observation: PriceObservation,
        from_currency: Commodity,
        to_currency: Commodity,
    ) -> FxRateDailySummary:
        scale_factor = 10 ** from_currency.scale
        return FxRateDailySummary(
            id=observation.id,
            book_id=observation.book_id,
            from_currency_id=observation.commodity_id,
            from_currency_symbol=from_currency.symbol,
            to_currency_id=observation.quote_commodity_id,
            to_currency_symbol=to_currency.symbol,
            rate_date=observation.price_date,
            rate=observation.price_minor / scale_factor,
            source=observation.source,
            created_at=observation.created_at,
        )

    @staticmethod
    def _to_market_price_summary(
        observation: PriceObservation,
        commodity: Commodity,
        quote: Commodity,
    ) -> MarketPriceSummary:
        return MarketPriceSummary(
            id=observation.id,
            book_id=observation.book_id,
            commodity_id=observation.commodity_id,
            commodity_symbol=commodity.symbol,
            commodity_name=commodity.name,
            quote_commodity_id=observation.quote_commodity_id,
            quote_commodity_symbol=quote.symbol,
            price_date=observation.price_date,
            price_minor=observation.price_minor,
            source=observation.source,
            created_at=observation.created_at,
        )

    @staticmethod
    def _to_fx_rate_official_summary(
        observation: PriceObservation,
        from_currency: Commodity,
        to_currency: Commodity,
    ) -> FxRateOfficialSummary:
        scale_factor = 10 ** from_currency.scale
        period_month = observation.price_date.month if observation.price_date.month != 1 else None
        period_type = "monthly" if period_month is not None else "yearly"
        return FxRateOfficialSummary(
            id=observation.id,
            book_id=observation.book_id,
            from_currency_id=observation.commodity_id,
            from_currency_symbol=from_currency.symbol,
            to_currency_id=observation.quote_commodity_id,
            to_currency_symbol=to_currency.symbol,
            period_type=period_type,
            period_year=observation.price_date.year,
            period_month=period_month,
            rate=observation.price_minor / scale_factor,
            source_name=observation.source or "Manual",
            source_url=None,
            source_date=observation.price_date,
            notes=None,
            created_at=observation.created_at,
            updated_at=observation.created_at,
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
    def _validate_assignment_dates(effective_from: date, effective_to: date | None) -> None:
        if effective_to is not None and effective_to < effective_from:
            raise ValueError("effective_to must be on or after effective_from")

    @staticmethod
    def _clean_optional_text(value: str | None) -> str | None:
        if value is None:
            return None
        cleaned = value.strip()
        return cleaned or None
