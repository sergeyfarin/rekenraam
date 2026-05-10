from __future__ import annotations

import json
from datetime import UTC, date, datetime
from decimal import ROUND_HALF_UP, Decimal
from typing import cast

from sqlalchemy import Select, or_, select
from sqlalchemy.dialects.postgresql import insert as pg_insert
from sqlalchemy.ext.asyncio import AsyncSession
from sqlalchemy.orm import aliased

from rekenraam_api.db.models.books import Book
from rekenraam_api.db.models.investments import PriceObservation
from rekenraam_api.db.models.metadata import Commodity
from rekenraam_api.db.models.pricing import (
    PriceSource,
    PricingPolicy,
    PricingRefreshRun,
    PricingRefreshState,
    PricingSourceAssignment,
)


class PricingRepository:
    def __init__(self, session: AsyncSession) -> None:
        self._session = session

    async def list_price_sources(self) -> list[PriceSource]:
        statement: Select[tuple[PriceSource]] = select(PriceSource).order_by(
            PriceSource.name.asc(), PriceSource.id.asc()
        )
        result = await self._session.execute(statement)
        return list(result.scalars().all())

    async def list_pricing_refresh_run_history(
        self, book_id: int, limit: int = 10
    ) -> list[PricingRefreshRun]:
        statement: Select[tuple[PricingRefreshRun]] = (
            select(PricingRefreshRun)
            .where(PricingRefreshRun.book_id == book_id)
            .order_by(PricingRefreshRun.finished_at.desc(), PricingRefreshRun.id.desc())
            .limit(limit)
        )
        result = await self._session.execute(statement)
        return list(result.scalars().all())

    async def get_latest_pricing_refresh_run(self, book_id: int) -> PricingRefreshRun | None:
        statement: Select[tuple[PricingRefreshRun]] = (
            select(PricingRefreshRun)
            .where(PricingRefreshRun.book_id == book_id)
            .order_by(PricingRefreshRun.finished_at.desc(), PricingRefreshRun.id.desc())
            .limit(1)
        )
        return await self._session.scalar(statement)

    async def list_enabled_pricing_policies(self) -> list[PricingPolicy]:
        statement: Select[tuple[PricingPolicy]] = (
            select(PricingPolicy)
            .where(PricingPolicy.refresh_enabled.is_(True))
            .order_by(PricingPolicy.book_id.asc(), PricingPolicy.id.asc())
        )
        result = await self._session.execute(statement)
        return list(result.scalars().all())

    async def get_pricing_policy(self, book_id: int) -> PricingPolicy | None:
        policy = await self._session.scalar(
            select(PricingPolicy).where(PricingPolicy.book_id == book_id)
        )
        if policy is not None:
            return policy

        book = await self._session.get(Book, book_id)
        if book is None:
            return None

        base_commodity = await self._session.scalar(
            select(Commodity).where(
                Commodity.book_id == book_id,
                Commodity.kind == "currency",
                Commodity.symbol == book.base_currency_code,
            )
        )
        if base_commodity is None:
            return None

        policy = PricingPolicy(
            book_id=book_id,
            base_commodity_id=base_commodity.id,
            refresh_enabled=False,
            refresh_hour_utc=4,
            refresh_minute_utc=0,
            max_backfill_days=370,
            weekend_policy="skip",
            default_source_id=None,
        )
        self._session.add(policy)
        await self._session.commit()
        await self._session.refresh(policy)
        return policy

    async def update_pricing_policy(
        self,
        *,
        book_id: int,
        base_currency_id: int,
        default_source_id: int | None,
        refresh_enabled: bool,
        refresh_hour_utc: int,
        refresh_minute_utc: int,
        max_backfill_days: int,
        weekend_policy: str,
    ) -> PricingPolicy | None:
        policy = await self.get_pricing_policy(book_id)
        if policy is None:
            return None

        base_currency = await self._session.get(Commodity, base_currency_id)
        if (
            base_currency is None
            or base_currency.book_id != book_id
            or base_currency.kind != "currency"
            or base_currency.symbol is None
        ):
            raise ValueError("base currency not found")

        if default_source_id is not None:
            source = await self._session.get(PriceSource, default_source_id)
            if source is None:
                raise ValueError("default source not found")

        policy.base_commodity_id = base_currency_id
        policy.default_source_id = default_source_id
        policy.refresh_enabled = refresh_enabled
        policy.refresh_hour_utc = refresh_hour_utc
        policy.refresh_minute_utc = refresh_minute_utc
        policy.max_backfill_days = max_backfill_days
        policy.weekend_policy = weekend_policy
        policy.updated_at = datetime.now(UTC)

        book = await self._session.get(Book, book_id)
        if book is not None:
            book.base_currency_code = base_currency.symbol

        await self._session.commit()
        await self._session.refresh(policy)
        return policy

    async def get_commodity(self, commodity_id: int) -> Commodity | None:
        return await self._session.get(Commodity, commodity_id)

    async def get_price_source(self, source_id: int) -> PriceSource | None:
        return await self._session.get(PriceSource, source_id)

    async def list_book_currencies(self, book_id: int) -> tuple[list[Commodity], str | None]:
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

    async def list_pricing_source_assignments(
        self, book_id: int
    ) -> list[tuple[PricingSourceAssignment, Commodity, Commodity, PriceSource]]:
        from_currency = aliased(Commodity)
        to_currency = aliased(Commodity)
        statement = (
            select(PricingSourceAssignment, from_currency, to_currency, PriceSource)
            .join(from_currency, PricingSourceAssignment.commodity_id == from_currency.id)
            .join(to_currency, PricingSourceAssignment.quote_commodity_id == to_currency.id)
            .join(PriceSource, PricingSourceAssignment.source_id == PriceSource.id)
            .where(PricingSourceAssignment.book_id == book_id)
            .order_by(
                from_currency.symbol.asc(),
                to_currency.symbol.asc(),
                PricingSourceAssignment.effective_from.desc(),
                PricingSourceAssignment.id.asc(),
            )
        )
        result = await self._session.execute(statement)
        return list(result.tuples().all())

    async def list_effective_pricing_source_assignments(
        self, book_id: int, effective_on: date
    ) -> list[tuple[PricingSourceAssignment, Commodity, Commodity, PriceSource]]:
        from_currency = aliased(Commodity)
        to_currency = aliased(Commodity)
        statement = (
            select(PricingSourceAssignment, from_currency, to_currency, PriceSource)
            .join(from_currency, PricingSourceAssignment.commodity_id == from_currency.id)
            .join(to_currency, PricingSourceAssignment.quote_commodity_id == to_currency.id)
            .join(PriceSource, PricingSourceAssignment.source_id == PriceSource.id)
            .where(
                PricingSourceAssignment.book_id == book_id,
                PricingSourceAssignment.effective_from <= effective_on,
                or_(
                    PricingSourceAssignment.effective_to.is_(None),
                    PricingSourceAssignment.effective_to >= effective_on,
                ),
            )
            .order_by(
                PricingSourceAssignment.commodity_id.asc(),
                PricingSourceAssignment.quote_commodity_id.asc(),
                PricingSourceAssignment.effective_from.desc(),
                PricingSourceAssignment.id.asc(),
            )
        )
        result = await self._session.execute(statement)
        return list(result.tuples().all())

    async def create_pricing_source_assignment(
        self,
        *,
        book_id: int,
        from_currency_id: int,
        to_currency_id: int,
        source_id: int,
        effective_from: date,
        effective_to: date | None,
    ) -> PricingSourceAssignment:
        await self._validate_assignment_refs(
            book_id=book_id,
            from_currency_id=from_currency_id,
            to_currency_id=to_currency_id,
            source_id=source_id,
        )
        assignment = PricingSourceAssignment(
            book_id=book_id,
            commodity_id=from_currency_id,
            quote_commodity_id=to_currency_id,
            source_id=source_id,
            effective_from=effective_from,
            effective_to=effective_to,
            priority=100,
        )
        self._session.add(assignment)
        await self._session.commit()
        await self._session.refresh(assignment)
        return assignment

    async def get_pricing_source_assignment(
        self, assignment_id: int
    ) -> PricingSourceAssignment | None:
        return await self._session.get(PricingSourceAssignment, assignment_id)

    async def update_pricing_source_assignment(
        self,
        *,
        assignment_id: int,
        book_id: int,
        from_currency_id: int,
        to_currency_id: int,
        source_id: int,
        effective_from: date,
        effective_to: date | None,
    ) -> PricingSourceAssignment | None:
        assignment = await self._session.get(PricingSourceAssignment, assignment_id)
        if assignment is None or assignment.book_id != book_id:
            return None

        await self._validate_assignment_refs(
            book_id=book_id,
            from_currency_id=from_currency_id,
            to_currency_id=to_currency_id,
            source_id=source_id,
        )
        assignment.commodity_id = from_currency_id
        assignment.quote_commodity_id = to_currency_id
        assignment.source_id = source_id
        assignment.effective_from = effective_from
        assignment.effective_to = effective_to
        assignment.updated_at = datetime.now(UTC)
        await self._session.commit()
        await self._session.refresh(assignment)
        return assignment

    async def delete_pricing_source_assignment(self, assignment_id: int) -> bool:
        assignment = await self.get_pricing_source_assignment(assignment_id)
        if assignment is None:
            return False
        await self._session.delete(assignment)
        await self._session.commit()
        return True

    async def list_pricing_refresh_state(
        self, book_id: int
    ) -> list[tuple[PricingRefreshState, Commodity, Commodity, PriceSource]]:
        from_currency = aliased(Commodity)
        to_currency = aliased(Commodity)
        statement = (
            select(PricingRefreshState, from_currency, to_currency, PriceSource)
            .join(from_currency, PricingRefreshState.commodity_id == from_currency.id)
            .join(to_currency, PricingRefreshState.quote_commodity_id == to_currency.id)
            .join(PriceSource, PricingRefreshState.source_id == PriceSource.id)
            .where(PricingRefreshState.book_id == book_id)
            .order_by(
                from_currency.symbol.asc(),
                to_currency.symbol.asc(),
                PriceSource.name.asc(),
                PricingRefreshState.id.asc(),
            )
        )
        result = await self._session.execute(statement)
        return list(result.tuples().all())

    async def list_pricing_refresh_state_rows(self, book_id: int) -> list[PricingRefreshState]:
        statement: Select[tuple[PricingRefreshState]] = (
            select(PricingRefreshState)
            .where(PricingRefreshState.book_id == book_id)
            .order_by(
                PricingRefreshState.commodity_id.asc(),
                PricingRefreshState.quote_commodity_id.asc(),
                PricingRefreshState.source_id.asc(),
                PricingRefreshState.id.asc(),
            )
        )
        result = await self._session.execute(statement)
        return list(result.scalars().all())

    async def list_existing_price_observation_dates(
        self,
        *,
        book_id: int,
        commodity_id: int,
        quote_commodity_id: int,
        observation_kind: str,
        source: str,
        start_date: date,
        end_date: date,
    ) -> set[date]:
        statement = (
            select(PriceObservation.price_date)
            .where(
                PriceObservation.book_id == book_id,
                PriceObservation.commodity_id == commodity_id,
                PriceObservation.quote_commodity_id == quote_commodity_id,
                PriceObservation.observation_kind == observation_kind,
                PriceObservation.source == source,
                PriceObservation.price_date >= start_date,
                PriceObservation.price_date <= end_date,
            )
            .order_by(PriceObservation.price_date.asc())
        )
        result = await self._session.execute(statement)
        return set(result.scalars().all())

    async def record_pricing_refresh_success(
        self,
        *,
        book_id: int,
        commodity_id: int,
        quote_commodity_id: int,
        source_id: int,
        last_success_date: date,
        attempted_at: datetime,
        observations: list[PriceObservation],
    ) -> None:
        if observations:
            self._session.add_all(observations)
        statement = pg_insert(PricingRefreshState).values(
            book_id=book_id,
            commodity_id=commodity_id,
            quote_commodity_id=quote_commodity_id,
            source_id=source_id,
            last_success_date=last_success_date,
            last_attempt_at=attempted_at,
            last_error=None,
            created_at=attempted_at,
            updated_at=attempted_at,
        )
        statement = statement.on_conflict_do_update(
            constraint="uq_pricing_refresh_state_pair_source",
            set_={
                "last_success_date": statement.excluded.last_success_date,
                "last_attempt_at": statement.excluded.last_attempt_at,
                "last_error": None,
                "updated_at": statement.excluded.updated_at,
            },
        )
        await self._session.execute(statement)
        await self._session.commit()

    async def record_pricing_refresh_error(
        self,
        *,
        book_id: int,
        commodity_id: int,
        quote_commodity_id: int,
        source_id: int,
        attempted_at: datetime,
        error: str,
    ) -> None:
        statement = pg_insert(PricingRefreshState).values(
            book_id=book_id,
            commodity_id=commodity_id,
            quote_commodity_id=quote_commodity_id,
            source_id=source_id,
            last_success_date=None,
            last_attempt_at=attempted_at,
            last_error=error,
            created_at=attempted_at,
            updated_at=attempted_at,
        )
        statement = statement.on_conflict_do_update(
            constraint="uq_pricing_refresh_state_pair_source",
            set_={
                "last_attempt_at": statement.excluded.last_attempt_at,
                "last_error": statement.excluded.last_error,
                "updated_at": statement.excluded.updated_at,
            },
        )
        await self._session.execute(statement)
        await self._session.commit()

    async def record_pricing_refresh_run(
        self,
        *,
        book_id: int,
        trigger: str,
        started_at: datetime,
        finished_at: datetime,
        pairs_total: int,
        pairs_success: int,
        pairs_failed: int,
        rates_inserted: int,
        derived_inserted: int,
        last_error: str | None,
    ) -> PricingRefreshRun:
        row = PricingRefreshRun(
            book_id=book_id,
            trigger=trigger,
            started_at=started_at,
            finished_at=finished_at,
            pairs_total=pairs_total,
            pairs_success=pairs_success,
            pairs_failed=pairs_failed,
            rates_inserted=rates_inserted,
            derived_inserted=derived_inserted,
            last_error=last_error,
        )
        self._session.add(row)
        await self._session.commit()
        await self._session.refresh(row)
        return row

    async def list_fx_rate_daily_observations(
        self,
        *,
        book_id: int,
        limit: int,
    ) -> list[tuple[PriceObservation, Commodity, Commodity]]:
        from_currency = aliased(Commodity)
        to_currency = aliased(Commodity)
        statement = (
            select(PriceObservation, from_currency, to_currency)
            .join(from_currency, PriceObservation.commodity_id == from_currency.id)
            .join(to_currency, PriceObservation.quote_commodity_id == to_currency.id)
            .where(PriceObservation.book_id == book_id)
            .where(PriceObservation.observation_kind == "fx_daily")
            .order_by(
                PriceObservation.price_date.desc(),
                PriceObservation.created_at.desc(),
                PriceObservation.id.desc(),
            )
            .limit(limit)
        )
        result = await self._session.execute(statement)
        return list(result.tuples().all())

    async def list_market_price_observations(
        self,
        *,
        book_id: int,
        commodity_id: int | None,
        quote_commodity_id: int | None,
        limit: int,
    ) -> list[tuple[PriceObservation, Commodity, Commodity]]:
        commodity = aliased(Commodity)
        quote = aliased(Commodity)
        statement = (
            select(PriceObservation, commodity, quote)
            .join(commodity, PriceObservation.commodity_id == commodity.id)
            .join(quote, PriceObservation.quote_commodity_id == quote.id)
            .where(PriceObservation.book_id == book_id)
            .where(
                PriceObservation.observation_kind.in_(["commodity_market", "valuation_override"])
            )
            .order_by(
                PriceObservation.price_date.desc(),
                PriceObservation.created_at.desc(),
                PriceObservation.id.desc(),
            )
            .limit(limit)
        )
        if commodity_id is not None:
            statement = statement.where(PriceObservation.commodity_id == commodity_id)
        if quote_commodity_id is not None:
            statement = statement.where(PriceObservation.quote_commodity_id == quote_commodity_id)
        result = await self._session.execute(statement)
        return list(result.tuples().all())

    async def create_market_price_observation(
        self,
        *,
        book_id: int,
        commodity_id: int,
        quote_commodity_id: int,
        price_date: date,
        price_minor: int,
        source: str | None,
    ) -> PriceObservation:
        commodity = await self._session.get(Commodity, commodity_id)
        quote = await self._session.get(Commodity, quote_commodity_id)
        if commodity is None or commodity.book_id != book_id:
            raise ValueError("commodity not found")
        if quote is None or quote.book_id != book_id:
            raise ValueError("quote commodity not found")
        observation = PriceObservation(
            book_id=book_id,
            commodity_id=commodity_id,
            quote_commodity_id=quote_commodity_id,
            observation_kind="commodity_market",
            price_minor=price_minor,
            price_date=price_date,
            source=source,
            created_at=datetime.now(UTC),
        )
        self._session.add(observation)
        await self._session.commit()
        await self._session.refresh(observation)
        return observation

    async def delete_market_price_observation(self, observation_id: int) -> bool:
        observation = await self._session.get(PriceObservation, observation_id)
        if observation is None or observation.observation_kind not in {
            "commodity_market",
            "valuation_override",
        }:
            return False
        await self._session.delete(observation)
        await self._session.commit()
        return True

    async def create_fx_rate_daily_observation(
        self,
        *,
        book_id: int,
        from_currency_id: int,
        to_currency_id: int,
        rate_date: date,
        rate: Decimal,
        source: str | None,
    ) -> PriceObservation:
        from_currency = await self._require_currency(book_id, from_currency_id)
        await self._require_currency(book_id, to_currency_id)

        scale_factor = Decimal(10) ** from_currency.scale
        price_minor = int((rate * scale_factor).to_integral_value(rounding=ROUND_HALF_UP))
        observation = PriceObservation(
            book_id=book_id,
            commodity_id=from_currency_id,
            quote_commodity_id=to_currency_id,
            observation_kind="fx_daily",
            price_minor=price_minor,
            price_date=rate_date,
            source=source,
            created_at=datetime.now(UTC),
        )
        self._session.add(observation)
        await self._session.commit()
        await self._session.refresh(observation)
        return observation

    async def delete_fx_rate_daily_observation(self, observation_id: int) -> bool:
        observation = await self._session.get(PriceObservation, observation_id)
        if observation is None or observation.observation_kind != "fx_daily":
            return False
        await self._session.delete(observation)
        await self._session.commit()
        return True

    async def list_fx_rate_official_observations(
        self,
        *,
        book_id: int,
        limit: int,
    ) -> list[tuple[PriceObservation, Commodity, Commodity]]:
        from_currency = aliased(Commodity)
        to_currency = aliased(Commodity)
        statement = (
            select(PriceObservation, from_currency, to_currency)
            .join(from_currency, PriceObservation.commodity_id == from_currency.id)
            .join(to_currency, PriceObservation.quote_commodity_id == to_currency.id)
            .where(PriceObservation.book_id == book_id)
            .where(PriceObservation.observation_kind == "fx_manual")
            .order_by(
                PriceObservation.price_date.desc(),
                PriceObservation.created_at.desc(),
                PriceObservation.id.desc(),
            )
            .limit(limit)
        )
        result = await self._session.execute(statement)
        return list(result.tuples().all())

    async def create_fx_rate_official_observation(
        self,
        *,
        book_id: int,
        from_currency_id: int,
        to_currency_id: int,
        price_date: date,
        rate: Decimal,
        source_name: str,
    ) -> PriceObservation:
        from_currency = await self._require_currency(book_id, from_currency_id)
        await self._require_currency(book_id, to_currency_id)

        scale_factor = Decimal(10) ** from_currency.scale
        price_minor = int((rate * scale_factor).to_integral_value(rounding=ROUND_HALF_UP))
        observation = PriceObservation(
            book_id=book_id,
            commodity_id=from_currency_id,
            quote_commodity_id=to_currency_id,
            observation_kind="fx_manual",
            price_minor=price_minor,
            price_date=price_date,
            source=source_name,
            created_at=datetime.now(UTC),
        )
        self._session.add(observation)
        await self._session.commit()
        await self._session.refresh(observation)
        return observation

    async def delete_fx_rate_official_observation(self, observation_id: int) -> bool:
        observation = await self._session.get(PriceObservation, observation_id)
        if observation is None or observation.observation_kind != "fx_manual":
            return False
        await self._session.delete(observation)
        await self._session.commit()
        return True

    async def rollback(self) -> None:
        await self._session.rollback()

    async def get_price_observation(self, observation_id: int) -> PriceObservation | None:
        return await self._session.get(PriceObservation, observation_id)

    async def _require_currency(self, book_id: int, commodity_id: int) -> Commodity:
        commodity = await self._session.get(Commodity, commodity_id)
        if commodity is None or commodity.book_id != book_id or commodity.kind != "currency":
            raise ValueError("currency not found")
        return commodity

    async def _validate_assignment_refs(
        self, *, book_id: int, from_currency_id: int, to_currency_id: int, source_id: int
    ) -> None:
        from_currency = await self._session.get(Commodity, from_currency_id)
        to_currency = await self._session.get(Commodity, to_currency_id)
        source = await self._session.get(PriceSource, source_id)
        if (
            from_currency is None
            or from_currency.book_id != book_id
            or from_currency.kind != "currency"
        ):
            raise ValueError("from currency not found")
        if to_currency is None or to_currency.book_id != book_id or to_currency.kind != "currency":
            raise ValueError("to currency not found")
        if source is None:
            raise ValueError("source not found")

    @staticmethod
    def currency_is_active(row: Commodity, base_currency_code: str | None) -> bool:
        if (
            row.symbol is not None
            and base_currency_code is not None
            and row.symbol == base_currency_code
        ):
            return True
        if row.metadata_text is None:
            return True
        try:
            payload = json.loads(row.metadata_text)
        except json.JSONDecodeError:
            return True
        if not isinstance(payload, dict):
            return True
        metadata = cast(dict[str, object], payload)
        is_active = metadata.get("is_active")
        return is_active if isinstance(is_active, bool) else True
