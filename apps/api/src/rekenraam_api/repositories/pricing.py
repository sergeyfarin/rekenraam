from __future__ import annotations

import json
from datetime import UTC, date, datetime
from decimal import Decimal
from typing import cast

from sqlalchemy import Select, exists, func, or_, select
from sqlalchemy.ext.asyncio import AsyncSession
from sqlalchemy.orm import aliased
from sqlalchemy.sql.elements import ColumnElement

from rekenraam_api.db.models.books import Book
from rekenraam_api.db.models.investments import PriceObservation
from rekenraam_api.db.models.metadata import Commodity
from rekenraam_api.db.models.pricing import (
    CommodityPriceSource,
    PriceSource,
    PricingPolicy,
    PricingRefreshRun,
    PricingRefreshState,
    PricingSourceAssignment,
)
from rekenraam_api.db.models.transactions import Split, Transaction
from rekenraam_api.db.sql import dialect_insert


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
            staleness_max_days=3,
            triangulation_max_hops=1,
            rounding_mode="half_up",
            prefer_official_fx=True,
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
        staleness_max_days: int,
        triangulation_max_hops: int,
        rounding_mode: str,
        prefer_official_fx: bool,
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
        policy.staleness_max_days = staleness_max_days
        policy.triangulation_max_hops = triangulation_max_hops
        policy.rounding_mode = rounding_mode
        policy.prefer_official_fx = prefer_official_fx
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

    async def list_commodity_price_sources(
        self,
        *,
        book_id: int,
        commodity_id: int | None = None,
        source_id: int | None = None,
        as_of: datetime | None = None,
    ) -> list[tuple[CommodityPriceSource, Commodity, PriceSource]]:
        commodity = aliased(Commodity)
        newer = aliased(CommodityPriceSource)
        newer_conditions = [
            newer.previous_commodity_price_source_id == CommodityPriceSource.id,
        ]
        if as_of is not None:
            newer_conditions.append(newer.created_at <= as_of)
        current_filter = ~exists(select(newer.id).where(*newer_conditions))
        statement = (
            select(CommodityPriceSource, commodity, PriceSource)
            .join(commodity, CommodityPriceSource.commodity_id == commodity.id)
            .join(PriceSource, CommodityPriceSource.source_id == PriceSource.id)
            .where(
                CommodityPriceSource.book_id == book_id,
                CommodityPriceSource.voided_at.is_(None),
                current_filter,
            )
            .order_by(
                commodity.symbol.asc(),
                CommodityPriceSource.is_primary.desc(),
                CommodityPriceSource.symbol.asc(),
                CommodityPriceSource.id.asc(),
            )
        )
        if as_of is not None:
            statement = statement.where(CommodityPriceSource.created_at <= as_of)
        if commodity_id is not None:
            statement = statement.where(CommodityPriceSource.commodity_id == commodity_id)
        if source_id is not None:
            statement = statement.where(CommodityPriceSource.source_id == source_id)
        result = await self._session.execute(statement)
        return list(result.tuples().all())

    async def resolve_commodity_price_source(
        self,
        *,
        book_id: int,
        commodity_id: int,
        source_id: int,
        as_of: datetime | None = None,
    ) -> CommodityPriceSource | None:
        rows = await self.list_commodity_price_sources(
            book_id=book_id,
            commodity_id=commodity_id,
            source_id=source_id,
            as_of=as_of,
        )
        if not rows:
            return None
        primary_rows = [row for row, _, _ in rows if row.is_primary]
        return primary_rows[0] if primary_rows else rows[0][0]

    async def create_commodity_price_source(
        self,
        *,
        book_id: int,
        commodity_id: int,
        source_id: int,
        symbol: str,
        provider_instrument_id: str | None,
        exchange_code: str | None,
        mic: str | None,
        name_override: str | None,
        is_primary: bool,
        metadata_json: str | None,
        effective_from: date | None,
        effective_to: date | None,
    ) -> CommodityPriceSource:
        await self._validate_commodity_price_source_refs(
            book_id=book_id,
            commodity_id=commodity_id,
            source_id=source_id,
        )
        await self._validate_commodity_price_source_uniqueness(
            book_id=book_id,
            commodity_id=commodity_id,
            source_id=source_id,
            symbol=symbol,
            is_primary=is_primary,
            current_id=None,
        )
        row = CommodityPriceSource(
            book_id=book_id,
            commodity_id=commodity_id,
            source_id=source_id,
            symbol=symbol,
            provider_instrument_id=provider_instrument_id,
            exchange_code=exchange_code,
            mic=mic,
            name_override=name_override,
            is_primary=is_primary,
            metadata_json=metadata_json,
            effective_from=effective_from,
            effective_to=effective_to,
        )
        self._session.add(row)
        await self._session.commit()
        await self._session.refresh(row)
        return row

    async def get_commodity_price_source(
        self, commodity_price_source_id: int
    ) -> CommodityPriceSource | None:
        return await self._current_commodity_price_source(commodity_price_source_id)

    async def update_commodity_price_source(
        self,
        *,
        commodity_price_source_id: int,
        book_id: int,
        commodity_id: int,
        source_id: int,
        symbol: str,
        provider_instrument_id: str | None,
        exchange_code: str | None,
        mic: str | None,
        name_override: str | None,
        is_primary: bool,
        metadata_json: str | None,
        effective_from: date | None,
        effective_to: date | None,
    ) -> CommodityPriceSource | None:
        current = await self._current_commodity_price_source(commodity_price_source_id)
        if current is None or current.book_id != book_id:
            return None

        await self._validate_commodity_price_source_refs(
            book_id=book_id,
            commodity_id=commodity_id,
            source_id=source_id,
        )
        await self._validate_commodity_price_source_uniqueness(
            book_id=book_id,
            commodity_id=commodity_id,
            source_id=source_id,
            symbol=symbol,
            is_primary=is_primary,
            current_id=current.id,
        )
        row = CommodityPriceSource(
            previous_commodity_price_source_id=current.id,
            book_id=book_id,
            commodity_id=commodity_id,
            source_id=source_id,
            symbol=symbol,
            provider_instrument_id=provider_instrument_id,
            exchange_code=exchange_code,
            mic=mic,
            name_override=name_override,
            is_primary=is_primary,
            metadata_json=metadata_json,
            effective_from=effective_from,
            effective_to=effective_to,
        )
        self._session.add(row)
        await self._session.commit()
        await self._session.refresh(row)
        return row

    async def delete_commodity_price_source(self, commodity_price_source_id: int) -> bool:
        current = await self._current_commodity_price_source(commodity_price_source_id)
        if current is None:
            return False
        row = CommodityPriceSource(
            previous_commodity_price_source_id=current.id,
            book_id=current.book_id,
            commodity_id=current.commodity_id,
            source_id=current.source_id,
            symbol=current.symbol,
            provider_instrument_id=current.provider_instrument_id,
            exchange_code=current.exchange_code,
            mic=current.mic,
            name_override=current.name_override,
            is_primary=current.is_primary,
            metadata_json=current.metadata_json,
            effective_from=current.effective_from,
            effective_to=current.effective_to,
            voided_at=datetime.now(UTC),
        )
        self._session.add(row)
        await self._session.commit()
        return True

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
                PriceObservation.voided_at.is_(None),
                self._current_price_observation_filter(),
            )
            .order_by(PriceObservation.price_date.asc())
        )
        result = await self._session.execute(statement)
        return set(result.scalars().all())

    async def record_pricing_refresh_success(
        self,
        *,
        ingest_run_id: int | None,
        book_id: int,
        commodity_id: int,
        quote_commodity_id: int,
        source_id: int,
        last_success_date: date,
        attempted_at: datetime,
        observations: list[PriceObservation],
    ) -> None:
        if observations:
            for observation in observations:
                observation.source_id = source_id
                observation.is_manual = False
                observation.ingest_run_id = ingest_run_id
            self._session.add_all(observations)
        statement = dialect_insert(self._session, PricingRefreshState).values(
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
            index_elements=["book_id", "commodity_id", "quote_commodity_id", "source_id"],
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
        statement = dialect_insert(self._session, PricingRefreshState).values(
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
            index_elements=["book_id", "commodity_id", "quote_commodity_id", "source_id"],
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
        run_id: int | None = None,
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
        if run_id is not None:
            row = await self._session.get(PricingRefreshRun, run_id)
            if row is None:
                raise ValueError("pricing refresh run not found")
            row.book_id = book_id
            row.trigger = trigger
            row.started_at = started_at
            row.finished_at = finished_at
            row.pairs_total = pairs_total
            row.pairs_success = pairs_success
            row.pairs_failed = pairs_failed
            row.rates_inserted = rates_inserted
            row.derived_inserted = derived_inserted
            row.last_error = last_error
            await self._session.commit()
            await self._session.refresh(row)
            return row
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
            .where(PriceObservation.voided_at.is_(None))
            .where(self._current_price_observation_filter())
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
            .where(PriceObservation.voided_at.is_(None))
            .where(self._current_price_observation_filter())
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
        source_id: int | None = None,
        supersedes_observation_id: int | None = None,
    ) -> PriceObservation:
        commodity = await self._session.get(Commodity, commodity_id)
        quote = await self._session.get(Commodity, quote_commodity_id)
        if commodity is None or commodity.book_id != book_id:
            raise ValueError("commodity not found")
        if quote is None or quote.book_id != book_id:
            raise ValueError("quote commodity not found")
        if supersedes_observation_id is not None:
            await self._require_superseded_observation(
                supersedes_observation_id,
                book_id=book_id,
                commodity_id=commodity_id,
                quote_commodity_id=quote_commodity_id,
                observation_kinds={"commodity_market", "valuation_override"},
            )
        observation = PriceObservation(
            book_id=book_id,
            commodity_id=commodity_id,
            quote_commodity_id=quote_commodity_id,
            observation_kind="commodity_market",
            price_minor=price_minor,
            price_date=price_date,
            mode="market",
            source=source,
            source_id=source_id,
            is_manual=True,
            supersedes_observation_id=supersedes_observation_id,
            created_at=datetime.now(UTC),
        )
        self._session.add(observation)
        await self._session.commit()
        await self._session.refresh(observation)
        return observation

    async def delete_market_price_observation(self, observation_id: int) -> bool:
        observation = await self._current_price_observation(observation_id)
        if observation is None or observation.observation_kind not in {
            "commodity_market",
            "valuation_override",
        }:
            return False
        return await self._supersede_price_observation_with_void(observation, "delete")

    async def create_implicit_market_price_from_transaction(
        self, transaction_id: int
    ) -> PriceObservation | None:
        transaction = await self._session.get(Transaction, transaction_id)
        if transaction is None:
            raise ValueError("transaction not found")

        split_totals_result = await self._session.execute(
            select(Split.commodity_id, func.sum(Split.amount_minor))
            .where(Split.tx_id == transaction_id)
            .group_by(Split.commodity_id)
        )
        split_totals = [
            (commodity_id, int(amount_minor))
            for commodity_id, amount_minor in split_totals_result.tuples().all()
            if int(amount_minor) != 0
        ]
        if len(split_totals) != 2:
            return None

        commodities_result = await self._session.execute(
            select(Commodity).where(
                Commodity.id.in_([commodity_id for commodity_id, _ in split_totals]),
                Commodity.book_id == transaction.book_id,
            )
        )
        commodities = {commodity.id: commodity for commodity in commodities_result.scalars().all()}
        if len(commodities) != 2:
            return None

        book = await self._session.get(Book, transaction.book_id)
        base_currency_id = next(
            (
                commodity.id
                for commodity in commodities.values()
                if book is not None
                and commodity.kind == "currency"
                and commodity.symbol == book.base_currency_code
            ),
            None,
        )
        quote_commodity_id = base_currency_id
        if quote_commodity_id is None:
            currency_ids = [
                commodity.id for commodity in commodities.values() if commodity.kind == "currency"
            ]
            if len(currency_ids) != 1:
                return None
            quote_commodity_id = currency_ids[0]

        commodity_id = next(
            commodity_id for commodity_id, _ in split_totals if commodity_id != quote_commodity_id
        )
        commodity_amount = abs(
            next(amount for current_id, amount in split_totals if current_id == commodity_id)
        )
        quote_amount = abs(
            next(amount for current_id, amount in split_totals if current_id == quote_commodity_id)
        )
        if commodity_amount == 0 or quote_amount == 0:
            return None

        commodity = commodities[commodity_id]
        price_minor = (quote_amount * (10**commodity.scale)) // commodity_amount
        if price_minor <= 0:
            return None

        existing = await self._session.scalar(
            select(PriceObservation)
            .where(
                PriceObservation.book_id == transaction.book_id,
                PriceObservation.commodity_id == commodity_id,
                PriceObservation.quote_commodity_id == quote_commodity_id,
                PriceObservation.observation_kind == "commodity_market",
                PriceObservation.price_date == transaction.occurred_date,
                PriceObservation.source == "implicit",
                PriceObservation.voided_at.is_(None),
                self._current_price_observation_filter(),
            )
            .order_by(PriceObservation.id.desc())
            .limit(1)
        )
        if existing is not None:
            return existing

        observation = PriceObservation(
            book_id=transaction.book_id,
            commodity_id=commodity_id,
            quote_commodity_id=quote_commodity_id,
            observation_kind="commodity_market",
            price_minor=price_minor,
            price_date=transaction.occurred_date,
            mode="transaction_implied",
            source="implicit",
            is_manual=False,
            is_derived=True,
            triangulation_path_json=json.dumps([quote_commodity_id, commodity_id]),
            period_type="daily",
            period_year=transaction.occurred_date.year,
            period_month=transaction.occurred_date.month,
            created_at=datetime.now(UTC),
        )
        self._session.add(observation)
        await self._session.commit()
        await self._session.refresh(observation)
        return observation

    async def create_fx_rate_daily_observation(
        self,
        *,
        book_id: int,
        from_currency_id: int,
        to_currency_id: int,
        rate_date: date,
        rate: Decimal,
        source: str | None,
        source_id: int | None,
        rounding: str,
        supersedes_observation_id: int | None = None,
    ) -> PriceObservation:
        from_currency = await self._require_currency(book_id, from_currency_id)
        await self._require_currency(book_id, to_currency_id)
        if supersedes_observation_id is not None:
            await self._require_superseded_observation(
                supersedes_observation_id,
                book_id=book_id,
                commodity_id=from_currency_id,
                quote_commodity_id=to_currency_id,
                observation_kinds={"fx_daily"},
            )

        scale_factor = Decimal(10) ** from_currency.scale
        price_minor = int((rate * scale_factor).to_integral_value(rounding=rounding))
        observation = PriceObservation(
            book_id=book_id,
            commodity_id=from_currency_id,
            quote_commodity_id=to_currency_id,
            observation_kind="fx_daily",
            price_minor=price_minor,
            price_date=rate_date,
            mode="daily",
            source=source,
            source_id=source_id,
            is_manual=True,
            supersedes_observation_id=supersedes_observation_id,
            period_type="daily",
            period_year=rate_date.year,
            period_month=rate_date.month,
            created_at=datetime.now(UTC),
        )
        self._session.add(observation)
        await self._session.commit()
        await self._session.refresh(observation)
        return observation

    async def delete_fx_rate_daily_observation(self, observation_id: int) -> bool:
        observation = await self._current_price_observation(observation_id)
        if observation is None or observation.observation_kind != "fx_daily":
            return False
        return await self._supersede_price_observation_with_void(observation, "delete")

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
            .where(PriceObservation.voided_at.is_(None))
            .where(self._current_price_observation_filter())
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
        source_id: int | None,
        rounding: str,
        period_type: str,
        period_year: int,
        period_month: int | None,
        supersedes_observation_id: int | None = None,
    ) -> PriceObservation:
        from_currency = await self._require_currency(book_id, from_currency_id)
        await self._require_currency(book_id, to_currency_id)
        if supersedes_observation_id is not None:
            await self._require_superseded_observation(
                supersedes_observation_id,
                book_id=book_id,
                commodity_id=from_currency_id,
                quote_commodity_id=to_currency_id,
                observation_kinds={"fx_manual"},
            )

        scale_factor = Decimal(10) ** from_currency.scale
        price_minor = int((rate * scale_factor).to_integral_value(rounding=rounding))
        observation = PriceObservation(
            book_id=book_id,
            commodity_id=from_currency_id,
            quote_commodity_id=to_currency_id,
            observation_kind="fx_manual",
            price_minor=price_minor,
            price_date=price_date,
            mode="official",
            source=source_name,
            source_id=source_id,
            is_manual=True,
            supersedes_observation_id=supersedes_observation_id,
            period_type=period_type,
            period_year=period_year,
            period_month=period_month,
            created_at=datetime.now(UTC),
        )
        self._session.add(observation)
        await self._session.commit()
        await self._session.refresh(observation)
        return observation

    async def delete_fx_rate_official_observation(self, observation_id: int) -> bool:
        observation = await self._current_price_observation(observation_id)
        if observation is None or observation.observation_kind != "fx_manual":
            return False
        return await self._supersede_price_observation_with_void(observation, "delete")

    async def rollback(self) -> None:
        await self._session.rollback()

    async def get_price_observation(self, observation_id: int) -> PriceObservation | None:
        return await self._current_price_observation(observation_id)

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

    async def _current_commodity_price_source(
        self, commodity_price_source_id: int
    ) -> CommodityPriceSource | None:
        current = await self._session.get(CommodityPriceSource, commodity_price_source_id)
        while current is not None:
            child = await self._session.scalar(
                select(CommodityPriceSource)
                .where(
                    CommodityPriceSource.previous_commodity_price_source_id == current.id,
                )
                .order_by(CommodityPriceSource.id.desc())
                .limit(1)
            )
            if child is None:
                break
            current = child
        if current is None or current.voided_at is not None:
            return None
        return current

    async def _current_price_observation(self, observation_id: int) -> PriceObservation | None:
        current = await self._session.get(PriceObservation, observation_id)
        while current is not None:
            child = await self._session.scalar(
                select(PriceObservation)
                .where(PriceObservation.supersedes_observation_id == current.id)
                .order_by(PriceObservation.id.desc())
                .limit(1)
            )
            if child is None:
                break
            current = child
        if current is None or current.voided_at is not None:
            return None
        return current

    @staticmethod
    def _current_price_observation_filter() -> ColumnElement[bool]:
        newer = aliased(PriceObservation)
        return ~exists(
            select(newer.id).where(newer.supersedes_observation_id == PriceObservation.id)
        )

    async def _require_superseded_observation(
        self,
        observation_id: int,
        *,
        book_id: int,
        commodity_id: int,
        quote_commodity_id: int,
        observation_kinds: set[str],
    ) -> PriceObservation:
        observation = await self._current_price_observation(observation_id)
        if observation is None:
            raise ValueError("superseded observation not found")
        if (
            observation.book_id != book_id
            or observation.commodity_id != commodity_id
            or observation.quote_commodity_id != quote_commodity_id
            or observation.observation_kind not in observation_kinds
        ):
            raise ValueError("superseded observation does not match price kind")
        return observation

    async def _supersede_price_observation_with_void(
        self, observation: PriceObservation, reason: str
    ) -> bool:
        tombstone = PriceObservation(
            book_id=observation.book_id,
            commodity_id=observation.commodity_id,
            quote_commodity_id=observation.quote_commodity_id,
            observation_kind=observation.observation_kind,
            price_minor=observation.price_minor,
            price_date=observation.price_date,
            mode=observation.mode,
            source=observation.source,
            source_id=observation.source_id,
            is_manual=observation.is_manual,
            is_derived=observation.is_derived,
            derived_via_commodity_id=observation.derived_via_commodity_id,
            triangulation_path_json=observation.triangulation_path_json,
            supersedes_observation_id=observation.id,
            ingest_run_id=observation.ingest_run_id,
            period_type=observation.period_type,
            period_year=observation.period_year,
            period_month=observation.period_month,
            voided_at=datetime.now(UTC),
            void_reason=reason,
        )
        self._session.add(tombstone)
        await self._session.commit()
        return True

    async def _validate_commodity_price_source_refs(
        self, *, book_id: int, commodity_id: int, source_id: int
    ) -> None:
        commodity = await self._session.get(Commodity, commodity_id)
        source = await self._session.get(PriceSource, source_id)
        if commodity is None or commodity.book_id != book_id:
            raise ValueError("commodity not found")
        if source is None:
            raise ValueError("source not found")

    async def _validate_commodity_price_source_uniqueness(
        self,
        *,
        book_id: int,
        commodity_id: int,
        source_id: int,
        symbol: str,
        is_primary: bool,
        current_id: int | None,
    ) -> None:
        rows = await self.list_commodity_price_sources(
            book_id=book_id,
            commodity_id=commodity_id,
            source_id=source_id,
        )
        normalized_symbol = symbol.casefold()
        for row, _, _ in rows:
            if current_id is not None and row.id == current_id:
                continue
            if row.symbol.casefold() == normalized_symbol:
                raise ValueError("commodity price source mapping already exists")
            if is_primary and row.is_primary:
                raise ValueError("primary commodity price source mapping already exists")

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
