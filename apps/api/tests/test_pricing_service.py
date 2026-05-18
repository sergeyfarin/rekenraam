from datetime import UTC, date, datetime

import pytest

from rekenraam_api.db.models.metadata import Commodity
from rekenraam_api.db.models.pricing import (
    CommodityPriceSource,
    PriceSource,
    PricingPolicy,
    PricingRefreshState,
    PricingSourceAssignment,
)
from rekenraam_api.schemas.pricing import (
    CommodityPriceSourceCreateInput,
    PricingPolicyUpdateInput,
    PricingSourceAssignmentCreateInput,
)
from rekenraam_api.services.pricing import PricingService


class StubPricingRepository:
    _created_at = datetime(2026, 5, 3, tzinfo=UTC)

    async def list_price_sources(self) -> list[PriceSource]:
        return [
            PriceSource(
                id=1001,
                name="ECB",
                kind="provider",
                provider="ECB",
                base_url="https://ecb.test",
                created_at=self._created_at,
            ),
            PriceSource(
                id=1005,
                name="Federal Reserve",
                kind="provider",
                provider="Federal Reserve",
                base_url="https://fed.test",
                created_at=self._created_at,
            ),
        ]

    async def list_commodity_price_sources(
        self,
        *,
        book_id: int,
        commodity_id: int | None = None,
        source_id: int | None = None,
    ) -> list[tuple[CommodityPriceSource, Commodity, PriceSource]]:
        if book_id != 1:
            return []
        row = CommodityPriceSource(
            id=20,
            book_id=1,
            commodity_id=2,
            source_id=1001,
            symbol="VWRL.AS",
            exchange_code="AS",
            mic="XAMS",
            is_primary=True,
            created_at=self._created_at,
        )
        commodity = Commodity(
            id=2,
            book_id=1,
            kind="security",
            symbol="VWRL",
            name="Vanguard FTSE All-World",
            scale=4,
            metadata_text=None,
            created_at=self._created_at,
            updated_at=self._created_at,
        )
        source = await self.get_price_source(1001)
        assert source is not None
        return [(row, commodity, source)]

    async def create_commodity_price_source(self, **kwargs: object) -> CommodityPriceSource:
        return CommodityPriceSource(
            id=21,
            book_id=int(kwargs["book_id"]),
            commodity_id=int(kwargs["commodity_id"]),
            source_id=int(kwargs["source_id"]),
            symbol=str(kwargs["symbol"]),
            provider_instrument_id=kwargs["provider_instrument_id"],
            exchange_code=kwargs["exchange_code"],
            mic=kwargs["mic"],
            name_override=kwargs["name_override"],
            is_primary=bool(kwargs["is_primary"]),
            metadata_json=kwargs["metadata_json"],
            effective_from=kwargs["effective_from"],
            effective_to=kwargs["effective_to"],
            created_at=self._created_at,
        )

    async def update_commodity_price_source(self, **kwargs: object) -> CommodityPriceSource | None:
        return await self.create_commodity_price_source(**kwargs)

    async def get_commodity_price_source(
        self, commodity_price_source_id: int
    ) -> CommodityPriceSource | None:
        if commodity_price_source_id != 20:
            return None
        return CommodityPriceSource(
            id=20,
            book_id=1,
            commodity_id=2,
            source_id=1001,
            symbol="VWRL.AS",
            is_primary=True,
            created_at=self._created_at,
        )

    async def delete_commodity_price_source(self, commodity_price_source_id: int) -> bool:
        return commodity_price_source_id == 20

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
        return PriceSource(
            id=source_id,
            name="ECB",
            kind="provider",
            provider="ECB",
            base_url="https://ecb.test",
            created_at=self._created_at,
        )

    async def list_pricing_source_assignments(
        self, book_id: int
    ) -> list[tuple[PricingSourceAssignment, Commodity, Commodity, PriceSource]]:
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
                Commodity(
                    id=2,
                    book_id=1,
                    kind="currency",
                    symbol="EUR",
                    name="Euro",
                    scale=2,
                    metadata_text=None,
                    created_at=self._created_at,
                    updated_at=self._created_at,
                ),
                Commodity(
                    id=1,
                    book_id=1,
                    kind="currency",
                    symbol="USD",
                    name="US Dollar",
                    scale=2,
                    metadata_text=None,
                    created_at=self._created_at,
                    updated_at=self._created_at,
                ),
                PriceSource(
                    id=1001,
                    name="ECB",
                    kind="provider",
                    provider="ECB",
                    base_url="https://ecb.test",
                    created_at=self._created_at,
                ),
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

    async def update_pricing_source_assignment(
        self, **kwargs: object
    ) -> PricingSourceAssignment | None:
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

    async def list_pricing_refresh_state(
        self, book_id: int
    ) -> list[tuple[PricingRefreshState, Commodity, Commodity, PriceSource]]:
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
                Commodity(
                    id=2,
                    book_id=1,
                    kind="currency",
                    symbol="EUR",
                    name="Euro",
                    scale=2,
                    metadata_text=None,
                    created_at=self._created_at,
                    updated_at=self._created_at,
                ),
                Commodity(
                    id=1,
                    book_id=1,
                    kind="currency",
                    symbol="USD",
                    name="US Dollar",
                    scale=2,
                    metadata_text=None,
                    created_at=self._created_at,
                    updated_at=self._created_at,
                ),
                PriceSource(
                    id=1001,
                    name="ECB",
                    kind="provider",
                    provider="ECB",
                    base_url="https://ecb.test",
                    created_at=self._created_at,
                ),
            )
        ]


@pytest.mark.asyncio
async def test_pricing_service_maps_policy_sources_assignments_and_refresh_state() -> None:
    service = PricingService(StubPricingRepository())

    sources = await service.list_price_sources()
    policy = await service.get_pricing_policy(1)
    assignments = await service.list_pricing_source_assignments(1)
    commodity_sources = await service.list_commodity_price_sources(book_id=1)
    refresh_state = await service.list_pricing_refresh_state(1)

    assert sources[0].name == "ECB"
    assert policy is not None
    assert policy.base_currency_symbol == "USD"
    assert assignments[0].from_currency_symbol == "EUR"
    assert commodity_sources[0].symbol == "VWRL.AS"
    assert commodity_sources[0].is_primary is True
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


@pytest.mark.asyncio
async def test_pricing_service_manages_commodity_price_source_inputs() -> None:
    service = PricingService(StubPricingRepository())

    created = await service.create_commodity_price_source(
        CommodityPriceSourceCreateInput(
            book_id=1,
            commodity_id=2,
            source_id=1001,
            symbol=" VWRL.AS ",
            exchange_code=" AS ",
            mic="XAMS",
            is_primary=True,
            metadata_json='{"assetClass":"equity"}',
        )
    )
    deleted = await service.delete_commodity_price_source(20)

    assert created.symbol == "VWRL.AS"
    assert created.exchange_code == "AS"
    assert deleted is True

    with pytest.raises(ValueError, match="metadata_json must be valid JSON"):
        await service.create_commodity_price_source(
            CommodityPriceSourceCreateInput(
                book_id=1,
                commodity_id=2,
                source_id=1001,
                symbol="VWRL.AS",
                metadata_json="{not-json}",
            )
        )


@pytest.mark.asyncio
@pytest.mark.parametrize(
    ("field", "value", "message"),
    [
        ("refresh_hour_utc", 24, "refresh hour must be between 0 and 23"),
        ("refresh_minute_utc", 60, "refresh minute must be between 0 and 59"),
        ("max_backfill_days", 0, "max backfill days must be at least 1"),
        ("weekend_policy", "invent", "weekend policy must be skip, fill_previous, or download"),
    ],
)
async def test_pricing_service_rejects_policy_boundary_values(
    field: str,
    value: object,
    message: str,
) -> None:
    service = PricingService(StubPricingRepository())
    values = {
        "book_id": 1,
        "base_currency_id": 2,
        "default_source_id": 1001,
        "refresh_enabled": True,
        "refresh_hour_utc": 6,
        "refresh_minute_utc": 15,
        "max_backfill_days": 60,
        "weekend_policy": "fill_previous",
    }
    values[field] = value

    with pytest.raises(ValueError, match=message):
        await service.update_pricing_policy(PricingPolicyUpdateInput(**values))
