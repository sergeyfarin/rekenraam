from __future__ import annotations

from datetime import UTC, date, datetime

from sqlalchemy import Select, select
from sqlalchemy.ext.asyncio import AsyncSession
from sqlalchemy.orm import aliased

from rekenraam_api.db.models.books import Book
from rekenraam_api.db.models.metadata import Commodity
from rekenraam_api.db.models.pricing import PriceSource, PricingPolicy, PricingRefreshState, PricingSourceAssignment


class PricingRepository:
    def __init__(self, session: AsyncSession) -> None:
        self._session = session

    async def list_price_sources(self) -> list[PriceSource]:
        statement: Select[tuple[PriceSource]] = select(PriceSource).order_by(PriceSource.name.asc(), PriceSource.id.asc())
        result = await self._session.execute(statement)
        return list(result.scalars().all())

    async def get_pricing_policy(self, book_id: int) -> PricingPolicy | None:
        policy = await self._session.scalar(select(PricingPolicy).where(PricingPolicy.book_id == book_id))
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
        if base_currency is None or base_currency.book_id != book_id or base_currency.kind != "currency" or base_currency.symbol is None:
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

    async def list_pricing_source_assignments(self, book_id: int) -> list[tuple[PricingSourceAssignment, Commodity, Commodity, PriceSource]]:
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
        return list(result.all())

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
        await self._validate_assignment_refs(book_id=book_id, from_currency_id=from_currency_id, to_currency_id=to_currency_id, source_id=source_id)
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

        await self._validate_assignment_refs(book_id=book_id, from_currency_id=from_currency_id, to_currency_id=to_currency_id, source_id=source_id)
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
        assignment = await self._session.get(PricingSourceAssignment, assignment_id)
        if assignment is None:
            return False
        await self._session.delete(assignment)
        await self._session.commit()
        return True

    async def list_pricing_refresh_state(self, book_id: int) -> list[tuple[PricingRefreshState, Commodity, Commodity, PriceSource]]:
        from_currency = aliased(Commodity)
        to_currency = aliased(Commodity)
        statement = (
            select(PricingRefreshState, from_currency, to_currency, PriceSource)
            .join(from_currency, PricingRefreshState.commodity_id == from_currency.id)
            .join(to_currency, PricingRefreshState.quote_commodity_id == to_currency.id)
            .join(PriceSource, PricingRefreshState.source_id == PriceSource.id)
            .where(PricingRefreshState.book_id == book_id)
            .order_by(from_currency.symbol.asc(), to_currency.symbol.asc(), PriceSource.name.asc(), PricingRefreshState.id.asc())
        )
        result = await self._session.execute(statement)
        return list(result.all())

    async def _validate_assignment_refs(self, *, book_id: int, from_currency_id: int, to_currency_id: int, source_id: int) -> None:
        from_currency = await self._session.get(Commodity, from_currency_id)
        to_currency = await self._session.get(Commodity, to_currency_id)
        source = await self._session.get(PriceSource, source_id)
        if from_currency is None or from_currency.book_id != book_id or from_currency.kind != "currency":
            raise ValueError("from currency not found")
        if to_currency is None or to_currency.book_id != book_id or to_currency.kind != "currency":
            raise ValueError("to currency not found")
        if source is None:
            raise ValueError("source not found")