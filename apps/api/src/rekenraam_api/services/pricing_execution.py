from __future__ import annotations

from collections.abc import Callable
from dataclasses import dataclass
from datetime import UTC, date, datetime, time, timedelta
from decimal import Decimal, ROUND_HALF_UP

import httpx

from rekenraam_api.db.models.investments import PriceObservation
from rekenraam_api.db.models.metadata import Commodity
from rekenraam_api.db.models.pricing import PriceSource, PricingPolicy
from rekenraam_api.repositories.pricing import PricingRepository
from rekenraam_api.schemas.pricing import PricingExecutionStatusSummary, PricingRefreshRunSummary


@dataclass(frozen=True)
class FxRatePoint:
    date: date
    base: str
    quote: str
    rate: Decimal
    source_name: str
    is_derived: bool = False
    derived_via: str | None = None


@dataclass(frozen=True)
class ProviderRequest:
    base: str
    symbols: tuple[str, ...]
    start_date: date
    end_date: date


@dataclass(frozen=True)
class PricingRefreshTask:
    from_currency: Commodity
    to_currency: Commodity
    source: PriceSource
    start_date: date
    end_date: date
    last_attempt_at: datetime | None


class PricingRateProvider:
    def __init__(self, name: str) -> None:
        self.name = name

    async def fetch_daily_rates(self, request: ProviderRequest) -> list[FxRatePoint]:
        raise NotImplementedError


class FrankfurterProvider(PricingRateProvider):
    def __init__(self) -> None:
        super().__init__("ECB")

    async def fetch_daily_rates(self, request: ProviderRequest) -> list[FxRatePoint]:
        url = (
            f"https://api.frankfurter.app/{request.start_date.isoformat()}..{request.end_date.isoformat()}"
            f"?from={request.base}&to={','.join(request.symbols)}"
        )
        async with httpx.AsyncClient(timeout=30.0) as client:
            response = await client.get(url)
            response.raise_for_status()
            payload = response.json()
        base = str(payload.get("base", request.base))
        points: list[FxRatePoint] = []
        for date_text, quotes in payload.get("rates", {}).items():
            if not isinstance(quotes, dict):
                continue
            point_date = date.fromisoformat(date_text)
            for quote, rate in quotes.items():
                decimal_rate = Decimal(str(rate))
                if decimal_rate <= 0:
                    continue
                points.append(FxRatePoint(date=point_date, base=base, quote=str(quote), rate=decimal_rate, source_name=self.name))
        return points


class FederalReserveProvider(PricingRateProvider):
    _series_map = {
        "EUR": "DEXUSEU",
        "GBP": "DEXUSUK",
        "JPY": "DEXJPUS",
        "CHF": "DEXSZUS",
        "CAD": "DEXCAUS",
        "AUD": "DEXUSAL",
        "CNY": "DEXCHUS",
        "SEK": "DEXSDUS",
        "NOK": "DEXNOUS",
        "DKK": "DEXDNUS",
    }

    def __init__(self) -> None:
        super().__init__("Federal Reserve")

    async def fetch_daily_rates(self, request: ProviderRequest) -> list[FxRatePoint]:
        if request.base != "USD":
            raise ValueError("Federal Reserve provider supports USD base only")
        points: list[FxRatePoint] = []
        async with httpx.AsyncClient(timeout=30.0) as client:
            for quote in request.symbols:
                series_id = self._series_map.get(quote)
                if series_id is None:
                    raise ValueError(f"No Federal Reserve series mapping for {quote}")
                url = (
                    "https://api.stlouisfed.org/fred/series/observations"
                    f"?series_id={series_id}"
                    f"&observation_start={request.start_date.isoformat()}"
                    f"&observation_end={request.end_date.isoformat()}"
                    "&file_type=json"
                )
                response = await client.get(url)
                response.raise_for_status()
                payload = response.json()
                for row in payload.get("observations", []):
                    value = row.get("value")
                    if value in {None, "."}:
                        continue
                    rate = Decimal(str(value))
                    if rate <= 0:
                        continue
                    if series_id in {"DEXUSEU", "DEXUSUK"}:
                        rate = Decimal("1") / rate
                    points.append(
                        FxRatePoint(
                            date=date.fromisoformat(str(row["date"])),
                            base=request.base,
                            quote=quote,
                            rate=rate,
                            source_name=self.name,
                        )
                    )
        return points


class BankOfCanadaProvider(PricingRateProvider):
    def __init__(self) -> None:
        super().__init__("Bank of Canada")

    async def fetch_daily_rates(self, request: ProviderRequest) -> list[FxRatePoint]:
        points: list[FxRatePoint] = []
        async with httpx.AsyncClient(timeout=30.0) as client:
            for quote in request.symbols:
                if request.base != "CAD" and quote != "CAD":
                    raise ValueError("Bank of Canada supports only pairs with CAD")
                series = f"FXCAD{quote}" if request.base == "CAD" else f"FX{request.base}CAD"
                url = (
                    f"https://www.bankofcanada.ca/valet/observations/{series}"
                    f"?start_date={request.start_date.isoformat()}&end_date={request.end_date.isoformat()}"
                )
                response = await client.get(url)
                response.raise_for_status()
                payload = response.json()
                for row in payload.get("observations", []):
                    point_date = row.get("d")
                    value = row.get(series, {}).get("v")
                    if point_date in {None, ""} or value in {None, ""}:
                        continue
                    rate = Decimal(str(value))
                    if rate <= 0:
                        continue
                    points.append(
                        FxRatePoint(
                            date=date.fromisoformat(str(point_date)),
                            base="CAD" if request.base == "CAD" else request.base,
                            quote=quote if request.base == "CAD" else "CAD",
                            rate=rate,
                            source_name=self.name,
                        )
                    )
        return points


class PricingProviderRegistry:
    def __init__(self, providers: dict[str, PricingRateProvider] | None = None) -> None:
        self._providers = providers or {
            "ecb": FrankfurterProvider(),
            "federal reserve": FederalReserveProvider(),
            "bank of canada": BankOfCanadaProvider(),
        }

    def resolve(self, source: PriceSource) -> PricingRateProvider | None:
        for candidate in (source.provider, source.name):
            if candidate is None:
                continue
            provider = self._providers.get(candidate.strip().lower())
            if provider is not None:
                return provider
        return None


class PricingExecutionService:
    def __init__(
        self,
        repository: PricingRepository,
        provider_registry: PricingProviderRegistry | None = None,
        now_provider: Callable[[], datetime] | None = None,
    ) -> None:
        self._repository = repository
        self._provider_registry = provider_registry or PricingProviderRegistry()
        self._now_provider = now_provider or (lambda: datetime.now(UTC))

    async def list_enabled_book_ids(self) -> list[int]:
        policies = await self._repository.list_enabled_pricing_policies()
        return [policy.book_id for policy in policies]

    async def run_book_refresh(self, book_id: int, *, trigger: str, force: bool) -> PricingRefreshRunSummary | None:
        started_at = self._now()
        policy = await self._repository.get_pricing_policy(book_id)
        if policy is None:
            raise ValueError("pricing policy not found")

        base_currency = await self._repository.get_commodity(policy.base_commodity_id)
        if base_currency is None or base_currency.symbol is None:
            raise ValueError("base currency not found")

        if not force and not policy.refresh_enabled:
            return None

        refresh_date = self._adjust_for_weekend(policy.weekend_policy, started_at.date())
        scheduled_at = self._scheduled_at(policy, started_at.date())
        if not force and started_at < scheduled_at:
            return None

        tasks = await self._build_tasks(policy, base_currency, refresh_date)
        if not force and not any(task.last_attempt_at is None or task.last_attempt_at.date() < refresh_date for task in tasks):
            return None

        pairs_success = 0
        pairs_failed = 0
        rates_inserted = 0
        derived_inserted = 0
        last_error: str | None = None

        for task in tasks:
            attempted_at = self._now()
            try:
                provider = self._provider_registry.resolve(task.source)
                if provider is None:
                    raise ValueError(f"No provider implementation for source '{task.source.name}'")
                direct_points, derived_points = await self._fetch_task_points(provider, task)
                direct_inserted, derived_inserted_for_task, observations = await self._build_observations(
                    book_id=book_id,
                    direct_points=direct_points,
                    derived_points=derived_points,
                    base_currency=base_currency,
                )
                await self._repository.record_pricing_refresh_success(
                    book_id=book_id,
                    commodity_id=task.from_currency.id,
                    quote_commodity_id=task.to_currency.id,
                    source_id=task.source.id,
                    last_success_date=task.end_date,
                    attempted_at=attempted_at,
                    observations=observations,
                )
                pairs_success += 1
                rates_inserted += direct_inserted
                derived_inserted += derived_inserted_for_task
            except Exception as error:
                await self._repository.rollback()
                last_error = str(error)
                pairs_failed += 1
                await self._repository.record_pricing_refresh_error(
                    book_id=book_id,
                    commodity_id=task.from_currency.id,
                    quote_commodity_id=task.to_currency.id,
                    source_id=task.source.id,
                    attempted_at=attempted_at,
                    error=last_error,
                )

        return PricingRefreshRunSummary(
            book_id=book_id,
            trigger=trigger,
            started_at=started_at,
            finished_at=self._now(),
            pairs_total=len(tasks),
            pairs_success=pairs_success,
            pairs_failed=pairs_failed,
            rates_inserted=rates_inserted,
            derived_inserted=derived_inserted,
            last_error=last_error,
        )

    async def get_execution_status(
        self,
        *,
        book_id: int,
        scheduler_enabled: bool,
        scheduler_poll_seconds: int,
        worker_started_at: datetime | None,
        active_book_ids: list[int],
        last_run: PricingRefreshRunSummary | None,
    ) -> PricingExecutionStatusSummary:
        policy = await self._repository.get_pricing_policy(book_id)
        if policy is None:
            raise ValueError("pricing policy not found")
        next_scheduled_at = self._scheduled_at(policy, self._now().date()) if policy.refresh_enabled else None
        if next_scheduled_at is not None and next_scheduled_at <= self._now():
            next_scheduled_at = next_scheduled_at + timedelta(days=1)
        return PricingExecutionStatusSummary(
            book_id=book_id,
            scheduler_enabled=scheduler_enabled,
            scheduler_poll_seconds=scheduler_poll_seconds,
            worker_started_at=worker_started_at,
            is_running=book_id in active_book_ids,
            active_book_ids=active_book_ids,
            next_scheduled_at=next_scheduled_at,
            last_run=last_run,
        )

    async def _build_tasks(self, policy: PricingPolicy, base_currency: Commodity, refresh_date: date) -> list[PricingRefreshTask]:
        currencies, base_currency_code = await self._repository.list_book_currencies(policy.book_id)
        active_currencies = [currency for currency in currencies if self._repository.currency_is_active(currency, base_currency_code)]
        assignments = await self._repository.list_effective_pricing_source_assignments(policy.book_id, refresh_date)
        assignment_map: dict[tuple[int, int], tuple[object, Commodity, Commodity, PriceSource]] = {}
        for row in assignments:
            assignment = row[0]
            assignment_map.setdefault((assignment.commodity_id, assignment.quote_commodity_id), row)
        default_source = await self._repository.get_price_source(policy.default_source_id) if policy.default_source_id is not None else None
        refresh_states = await self._repository.list_pricing_refresh_state_rows(policy.book_id)
        refresh_state_map = {(row.commodity_id, row.quote_commodity_id, row.source_id): row for row in refresh_states}

        tasks: list[PricingRefreshTask] = []
        for currency in active_currencies:
            if currency.id == base_currency.id:
                continue
            assignment_row = assignment_map.get((currency.id, base_currency.id))
            source = assignment_row[3] if assignment_row is not None else default_source
            if source is None:
                raise ValueError("no default pricing source configured for pricing refresh")
            state = refresh_state_map.get((currency.id, base_currency.id, source.id))
            last_success_date = state.last_success_date if state is not None else None
            start_date = last_success_date + timedelta(days=1) if last_success_date is not None else refresh_date - timedelta(days=max(policy.max_backfill_days, 1))
            if start_date > refresh_date:
                continue
            tasks.append(
                PricingRefreshTask(
                    from_currency=currency,
                    to_currency=base_currency,
                    source=source,
                    start_date=start_date,
                    end_date=refresh_date,
                    last_attempt_at=state.last_attempt_at if state is not None else None,
                )
            )
        return tasks

    async def _build_observations(
        self,
        *,
        book_id: int,
        direct_points: list[FxRatePoint],
        derived_points: list[FxRatePoint],
        base_currency: Commodity,
    ) -> tuple[int, int, list[PriceObservation]]:
        currencies, _ = await self._repository.list_book_currencies(book_id)
        symbol_map = {currency.symbol: currency for currency in currencies if currency.symbol is not None}
        symbol_map[base_currency.symbol] = base_currency

        observations: list[PriceObservation] = []
        direct_inserted = 0
        derived_inserted = 0
        for point in direct_points:
            direct_inserted += await self._append_observation(book_id, point, symbol_map, observations)
        for point in derived_points:
            derived_inserted += await self._append_observation(book_id, point, symbol_map, observations)
        return direct_inserted, derived_inserted, observations

    async def _append_observation(
        self,
        book_id: int,
        point: FxRatePoint,
        symbol_map: dict[str | None, Commodity],
        observations: list[PriceObservation],
    ) -> int:
        from_currency = symbol_map.get(point.base)
        to_currency = symbol_map.get(point.quote)
        if from_currency is None or to_currency is None:
            return 0
        existing_dates = await self._repository.list_existing_price_observation_dates(
            book_id=book_id,
            commodity_id=from_currency.id,
            quote_commodity_id=to_currency.id,
            observation_kind="fx_daily",
            source=point.source_name,
            start_date=point.date,
            end_date=point.date,
        )
        if point.date in existing_dates:
            return 0
        scale_factor = Decimal(10) ** from_currency.scale
        price_minor = int((point.rate * scale_factor).to_integral_value(rounding=ROUND_HALF_UP))
        observations.append(
            PriceObservation(
                book_id=book_id,
                commodity_id=from_currency.id,
                quote_commodity_id=to_currency.id,
                observation_kind="fx_daily",
                price_minor=price_minor,
                price_date=point.date,
                source=point.source_name,
            )
        )
        return 1

    async def _fetch_task_points(self, provider: PricingRateProvider, task: PricingRefreshTask) -> tuple[list[FxRatePoint], list[FxRatePoint]]:
        if task.from_currency.symbol is None or task.to_currency.symbol is None:
            raise ValueError("currency symbol is required for pricing refresh")
        base_symbol = task.to_currency.symbol
        quote_symbol = task.from_currency.symbol
        request_base = base_symbol
        request_symbols: tuple[str, ...] = (quote_symbol,)
        derived_via: str | None = None
        if provider.name == "Federal Reserve" and base_symbol != "USD":
            request_base = "USD"
            request_symbols = (base_symbol, quote_symbol)
            derived_via = "USD"
        elif provider.name == "Bank of Canada" and base_symbol != "CAD":
            request_base = "CAD"
            request_symbols = (base_symbol, quote_symbol)
            derived_via = "CAD"

        direct_points = await provider.fetch_daily_rates(
            ProviderRequest(
                base=request_base,
                symbols=request_symbols,
                start_date=task.start_date,
                end_date=task.end_date,
            )
        )
        normalized_direct = [
            FxRatePoint(
                date=point.date,
                base=point.base,
                quote=point.quote,
                rate=point.rate,
                source_name=task.source.name,
            )
            for point in direct_points
        ]
        derived_points = self._derive_from_via(normalized_direct, derived_via, base_symbol, quote_symbol) if derived_via is not None else []
        return normalized_direct, derived_points

    @staticmethod
    def _derive_from_via(points: list[FxRatePoint], via: str, base: str, quote: str) -> list[FxRatePoint]:
        via_to_base: dict[date, Decimal] = {}
        via_to_quote: dict[date, Decimal] = {}
        for point in points:
            if point.base == via and point.quote == base:
                via_to_base[point.date] = point.rate
            if point.base == via and point.quote == quote:
                via_to_quote[point.date] = point.rate
        derived: list[FxRatePoint] = []
        for point_date, quote_rate in via_to_quote.items():
            base_rate = via_to_base.get(point_date)
            if base_rate is None or base_rate == 0:
                continue
            derived.append(
                FxRatePoint(
                    date=point_date,
                    base=base,
                    quote=quote,
                    rate=quote_rate / base_rate,
                    source_name=f"derived:{via}",
                    is_derived=True,
                    derived_via=via,
                )
            )
        return derived

    @staticmethod
    def _scheduled_at(policy: PricingPolicy, target_date: date) -> datetime:
        return datetime.combine(target_date, time(policy.refresh_hour_utc, policy.refresh_minute_utc, tzinfo=UTC))

    @staticmethod
    def _adjust_for_weekend(weekend_policy: str, target_date: date) -> date:
        if weekend_policy != "fill_previous":
            return target_date
        current = target_date
        while current.weekday() >= 5:
            current -= timedelta(days=1)
        return current

    def _now(self) -> datetime:
        current = self._now_provider()
        return current if current.tzinfo is not None else current.replace(tzinfo=UTC)