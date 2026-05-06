from datetime import UTC, date, datetime

import pytest

from rekenraam_api.db.models.metadata import Commodity
from rekenraam_api.db.models.pricing import (
    PriceSource,
    PricingPolicy,
    PricingRefreshState,
    PricingSourceAssignment,
)
from rekenraam_api.schemas.pricing import (
    PricingPolicyUpdateInput,
    PricingSourceAssignmentCreateInput,
)
from rekenraam_api.services.pricing import PricingService


class StubPricingRepository:
    _created_at = datetime(2026, 5, 3, tzinfo=UTC)

    async def list_price_sources(self) -> list[PriceSource]:
        return [
            PriceSource(id=1001, name="ECB", kind="provider", provider="ECB", base_url="https://ecb.test", created_at=self._created_at),
            PriceSource(id=1005, name="Federal Reserve", kind="provider", provider="Federal Reserve", base_url="https://fed.test", created_at=self._created_at),
        ]

    async def get_pricing_policy(self, book_id: int) -> PricingPolicy | None:
        if book_id != 1:
            return None
        return PricingPolicy(
            id=1,
            book_id=1,
            base_commodity_id=1,
            refresh_enabled=True,
            refresh_hour_utc=4,
            refresh_minute_utc=0,
            max_backfill_days=30,
            weekend_policy="skip",
            default_source_id=1001,
            created_at=self._created_at,
            updated_at=self._created_at,
        )

    async def update_pricing_policy(self, **kwargs: object) -> PricingPolicy | None:
        if int(kwargs["book_id"]) != 1:
            return None
        return PricingPolicy(
            id=1,
            book_id=1,
            base_commodity_id=int(kwargs["base_currency_id"]),
            refresh_enabled=bool(kwargs["refresh_enabled"]),
            refresh_hour_utc=int(kwargs["refresh_hour_utc"]),
            refresh_minute_utc=int(kwargs["refresh_minute_utc"]),
            max_backfill_days=int(kwargs["max_backfill_days"]),
            weekend_policy=str(kwargs["weekend_policy"]),
            default_source_id=kwargs["default_source_id"],
            created_at=self._created_at,
            updated_at=self._created_at,
        )

    async def get_commodity(self, commodity_id: int) -> Commodity | None:
        symbols = {1: "USD", 2: "EUR"}
        symbol = symbols.get(commodity_id)
        if symbol is None:
            return None
        return Commodity(
            id=commodity_id,
            book_id=1,
            kind="currency",
            symbol=symbol,
            name=symbol,
            scale=2,
            metadata_text=None,
            created_at=self._created_at,
            updated_at=self._created_at,
        )

    async def get_price_source(self, source_id: int | None) -> PriceSource | None:
        if source_id is None:
            return None
        return PriceSource(id=source_id, name="ECB", kind="provider", provider="ECB", base_url="https://ecb.test", created_at=self._created_at)

    async def list_pricing_source_assignments(self, book_id: int) -> list[tuple[PricingSourceAssignment, Commodity, Commodity, PriceSource]]:
        if book_id != 1:
            return []
        return [
            (
                PricingSourceAssignment(
                    id=10,
                    book_id=1,
                    commodity_id=2,
                    quote_commodity_id=1,
                    source_id=1001,
                    priority=100,
                    effective_from=date(2026, 5, 1),
                    effective_to=None,
                    created_at=self._created_at,
                    updated_at=self._created_at,
                ),
                Commodity(id=2, book_id=1, kind="currency", symbol="EUR", name="Euro", scale=2, metadata_text=None, created_at=self._created_at, updated_at=self._created_at),
                Commodity(id=1, book_id=1, kind="currency", symbol="USD", name="US Dollar", scale=2, metadata_text=None, created_at=self._created_at, updated_at=self._created_at),
                PriceSource(id=1001, name="ECB", kind="provider", provider="ECB", base_url="https://ecb.test", created_at=self._created_at),
            )
        ]

    async def create_pricing_source_assignment(self, **kwargs: object) -> PricingSourceAssignment:
        return PricingSourceAssignment(
            id=11,
            book_id=int(kwargs["book_id"]),
            commodity_id=int(kwargs["from_currency_id"]),
            quote_commodity_id=int(kwargs["to_currency_id"]),
            source_id=int(kwargs["source_id"]),
            priority=100,
            effective_from=kwargs["effective_from"],
            effective_to=kwargs["effective_to"],
            created_at=self._created_at,
            updated_at=self._created_at,
        )

    async def update_pricing_source_assignment(self, **kwargs: object) -> PricingSourceAssignment | None:
        if int(kwargs["assignment_id"]) != 10:
            return None
        return PricingSourceAssignment(
            id=10,
            book_id=int(kwargs["book_id"]),
            commodity_id=int(kwargs["from_currency_id"]),
            quote_commodity_id=int(kwargs["to_currency_id"]),
            source_id=int(kwargs["source_id"]),
            priority=100,
            effective_from=kwargs["effective_from"],
            effective_to=kwargs["effective_to"],
            created_at=self._created_at,
            updated_at=self._created_at,
        )

    async def delete_pricing_source_assignment(self, assignment_id: int) -> bool:
        return assignment_id == 10

    async def list_pricing_refresh_state(self, book_id: int) -> list[tuple[PricingRefreshState, Commodity, Commodity, PriceSource]]:
        if book_id != 1:
            return []
        return [
            (
                PricingRefreshState(
                    id=5,
                    book_id=1,
                    commodity_id=2,
                    quote_commodity_id=1,
                    source_id=1001,
                    last_success_date=date(2026, 5, 20),
                    last_attempt_at=self._created_at,
                    last_error=None,
                    created_at=self._created_at,
                    updated_at=self._created_at,
                ),
                Commodity(id=2, book_id=1, kind="currency", symbol="EUR", name="Euro", scale=2, metadata_text=None, created_at=self._created_at, updated_at=self._created_at),
                Commodity(id=1, book_id=1, kind="currency", symbol="USD", name="US Dollar", scale=2, metadata_text=None, created_at=self._created_at, updated_at=self._created_at),
                PriceSource(id=1001, name="ECB", kind="provider", provider="ECB", base_url="https://ecb.test", created_at=self._created_at),
            )
        ]


@pytest.mark.asyncio
async def test_pricing_service_maps_policy_sources_assignments_and_refresh_state() -> None:
    service = PricingService(StubPricingRepository())

    sources = await service.list_price_sources()
    policy = await service.get_pricing_policy(1)
    assignments = await service.list_pricing_source_assignments(1)
    refresh_state = await service.list_pricing_refresh_state(1)

    assert sources[0].name == "ECB"
    assert policy is not None
    assert policy.base_currency_symbol == "USD"
    assert assignments[0].from_currency_symbol == "EUR"
    assert refresh_state[0].source_name == "ECB"


@pytest.mark.asyncio
async def test_pricing_service_updates_policy_and_validates_assignment_dates() -> None:
    service = PricingService(StubPricingRepository())

    policy = await service.update_pricing_policy(
        PricingPolicyUpdateInput(
            book_id=1,
            base_currency_id=2,
            default_source_id=1001,
            refresh_enabled=True,
            refresh_hour_utc=6,
            refresh_minute_utc=15,
            max_backfill_days=60,
            weekend_policy="fill_previous",
        )
    )
    assignment = await service.create_pricing_source_assignment(
        PricingSourceAssignmentCreateInput(
            book_id=1,
            from_currency_id=2,
            to_currency_id=1,
            source_id=1001,
            effective_from=date(2026, 5, 1),
            effective_to=None,
        )
    )

    assert policy is not None
    assert policy.base_currency_symbol == "EUR"
    assert assignment.source_name == "ECB"

    with pytest.raises(ValueError, match="effective_to must be on or after effective_from"):
        await service.create_pricing_source_assignment(
            PricingSourceAssignmentCreateInput(
                book_id=1,
                from_currency_id=2,
                to_currency_id=1,
                source_id=1001,
                effective_from=date(2026, 5, 2),
                effective_to=date(2026, 5, 1),
            )
        )