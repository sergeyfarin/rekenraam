from __future__ import annotations

from datetime import UTC, date, datetime
from decimal import Decimal

import pytest

from rekenraam_api.db.models.metadata import Commodity
from rekenraam_api.db.models.pricing import (
    PriceSource,
    PricingPolicy,
    PricingRefreshRun,
    PricingRefreshState,
)
from rekenraam_api.services.pricing_execution import (
    FxRatePoint,
    PricingExecutionService,
    PricingProviderRegistry,
    PricingRateProvider,
    ProviderRequest,
)


class StubProvider(PricingRateProvider):
    def __init__(self) -> None:
        super().__init__("ECB")

    async def fetch_daily_rates(self, request: ProviderRequest) -> list[FxRatePoint]:
        return [
            FxRatePoint(
                date=request.start_date,
                base=request.base,
                quote=request.symbols[0],
                rate=Decimal("1.2500"),
                source_name=self.name,
            )
        ]


class StubPricingRepository:
    _created_at = datetime(2026, 5, 3, tzinfo=UTC)

    def __init__(self) -> None:
        self.success_calls: list[dict[str, object]] = []
        self.error_calls: list[dict[str, object]] = []
        self.run_calls: list[dict[str, object]] = []

    async def list_enabled_pricing_policies(self) -> list[PricingPolicy]:
        policy = await self.get_pricing_policy(1)
        assert policy is not None
        return [policy]

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
            staleness_max_days=3,
            triangulation_max_hops=1,
            rounding_mode="half_up",
            prefer_official_fx=True,
            weekend_policy="fill_previous",
            default_source_id=1001,
            created_at=self._created_at,
            updated_at=self._created_at,
        )

    async def get_commodity(self, commodity_id: int) -> Commodity | None:
        data = {
            1: Commodity(
                id=1,
                book_id=1,
                kind="currency",
                symbol="USD",
                name="US Dollar",
                scale=4,
                metadata_text=None,
                created_at=self._created_at,
                updated_at=self._created_at,
            ),
            2: Commodity(
                id=2,
                book_id=1,
                kind="currency",
                symbol="EUR",
                name="Euro",
                scale=4,
                metadata_text='{"is_active": true}',
                created_at=self._created_at,
                updated_at=self._created_at,
            ),
        }
        return data.get(commodity_id)

    async def list_book_currencies(self, book_id: int) -> tuple[list[Commodity], str | None]:
        usd = await self.get_commodity(1)
        eur = await self.get_commodity(2)
        assert usd is not None and eur is not None
        return [usd, eur], "USD"

    async def list_effective_pricing_source_assignments(
        self, book_id: int, effective_on: date
    ) -> list[tuple[object, Commodity, Commodity, PriceSource]]:
        return []

    async def get_price_source(self, source_id: int | None) -> PriceSource | None:
        if source_id is None:
            return None
        return PriceSource(
            id=1001,
            name="ECB",
            kind="provider",
            provider="ECB",
            base_url="https://ecb.test",
            created_at=self._created_at,
        )

    async def list_pricing_refresh_state_rows(self, book_id: int) -> list[PricingRefreshState]:
        return [
            PricingRefreshState(
                id=1,
                book_id=1,
                commodity_id=2,
                quote_commodity_id=1,
                source_id=1001,
                last_success_date=date(2026, 4, 30),
                last_attempt_at=datetime(2026, 4, 30, 4, 0, tzinfo=UTC),
                last_error=None,
                created_at=self._created_at,
                updated_at=self._created_at,
            )
        ]

    async def resolve_commodity_price_source(self, **kwargs: object) -> None:
        return None

    async def list_existing_price_observation_dates(self, **kwargs: object) -> set[date]:
        return set()

    async def record_pricing_refresh_success(self, **kwargs: object) -> None:
        self.success_calls.append(kwargs)

    async def record_pricing_refresh_error(self, **kwargs: object) -> None:
        self.error_calls.append(kwargs)

    async def record_pricing_refresh_run(self, **kwargs: object) -> PricingRefreshRun:
        self.run_calls.append(kwargs)
        row_kwargs = {key: value for key, value in kwargs.items() if key != "run_id"}
        return PricingRefreshRun(id=1, created_at=self._created_at, **row_kwargs)

    async def list_pricing_refresh_run_history(
        self, book_id: int, limit: int = 10
    ) -> list[PricingRefreshRun]:
        return [
            PricingRefreshRun(
                id=1,
                book_id=1,
                trigger="manual",
                started_at=self._created_at,
                finished_at=self._created_at,
                pairs_total=1,
                pairs_success=1,
                pairs_failed=0,
                rates_inserted=1,
                derived_inserted=0,
                last_error=None,
                created_at=self._created_at,
            )
        ]

    async def get_latest_pricing_refresh_run(self, book_id: int) -> PricingRefreshRun | None:
        return PricingRefreshRun(
            id=1,
            book_id=1,
            trigger="manual",
            started_at=self._created_at,
            finished_at=self._created_at,
            pairs_total=1,
            pairs_success=1,
            pairs_failed=0,
            rates_inserted=1,
            derived_inserted=0,
            last_error=None,
            created_at=self._created_at,
        )

    async def rollback(self) -> None:
        return None

    @staticmethod
    def currency_is_active(row: Commodity, base_currency_code: str | None) -> bool:
        return row.symbol == base_currency_code or row.symbol == "EUR"


@pytest.mark.asyncio
async def test_pricing_execution_service_runs_manual_refresh() -> None:
    repository = StubPricingRepository()
    service = PricingExecutionService(
        repository,
        provider_registry=PricingProviderRegistry({"ecb": StubProvider()}),
        now_provider=lambda: datetime(2026, 5, 3, 5, 0, tzinfo=UTC),
    )

    summary = await service.run_book_refresh(1, trigger="manual", force=True)

    assert summary is not None
    assert summary.pairs_total == 1
    assert summary.pairs_success == 1
    assert summary.rates_inserted == 1
    assert repository.success_calls
    assert repository.run_calls
    observations = repository.success_calls[0]["observations"]
    assert len(observations) == 1
    assert observations[0].observation_kind == "fx_daily"
    assert observations[0].mode == "daily"
    assert observations[0].period_type == "daily"
    assert repository.success_calls[0]["ingest_run_id"] == 1


@pytest.mark.asyncio
async def test_pricing_execution_service_skips_scheduled_run_before_due_time() -> None:
    repository = StubPricingRepository()
    service = PricingExecutionService(
        repository,
        provider_registry=PricingProviderRegistry({"ecb": StubProvider()}),
        now_provider=lambda: datetime(2026, 5, 3, 3, 0, tzinfo=UTC),
    )

    summary = await service.run_book_refresh(1, trigger="scheduled", force=False)

    assert summary is None
    assert repository.success_calls == []


def test_pricing_provider_registry_resolves_aliases_beyond_initial_three() -> None:
    registry = PricingProviderRegistry()

    hmrc = registry.resolve(
        PriceSource(
            id=1003,
            name="HMRC",
            kind="provider",
            provider="HMRC",
            base_url=None,
            created_at=datetime(2026, 5, 3, tzinfo=UTC),
        )
    )
    yahoo = registry.resolve(
        PriceSource(
            id=1010,
            name="Yahoo Finance",
            kind="provider",
            provider="Yahoo Finance",
            base_url=None,
            created_at=datetime(2026, 5, 3, tzinfo=UTC),
        )
    )

    assert hmrc is not None
    assert hmrc.name == "ExchangeRate.host"
    assert yahoo is not None
    assert yahoo.name == "Yahoo Finance"
