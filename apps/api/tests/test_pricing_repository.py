from datetime import UTC, date, datetime

import pytest
from sqlalchemy import select
from sqlalchemy.ext.asyncio import AsyncSession

from rekenraam_api.db.models.accounts import Account
from rekenraam_api.db.models.books import Book
from rekenraam_api.db.models.investments import PriceObservation
from rekenraam_api.db.models.metadata import Commodity
from rekenraam_api.db.models.pricing import CommodityPriceSource, PricingRefreshState
from rekenraam_api.db.models.transactions import Split, Transaction
from rekenraam_api.repositories.pricing import PricingRepository


@pytest.mark.asyncio
async def test_pricing_repository_lists_sources_updates_policy_and_manages_assignments(
    repository_session: AsyncSession,
) -> None:
    repository = PricingRepository(repository_session)
    # session.merge() returns the merged, persistent instance — the original
    # `Commodity()` argument stays detached, so subsequent .refresh() on it
    # raises InvalidRequestError. Bind the merged instance to `eur` instead.
    eur = await repository_session.merge(
        Commodity(book_id=1, kind="currency", symbol="EUR", name="Euro", scale=2)
    )
    await repository_session.commit()
    await repository_session.refresh(eur)

    sources = await repository.list_price_sources()
    assert sources[0].name == "Bank of Canada"
    assert any(source.name == "ECB" for source in sources)
    assert any(source.name == "ExchangeRate.host" for source in sources)
    assert any(source.name == "Yahoo Finance" for source in sources)

    policy = await repository.get_pricing_policy(1)
    assert policy is not None
    assert policy.base_commodity_id == 1

    updated_policy = await repository.update_pricing_policy(
        book_id=1,
        base_currency_id=eur.id,
        default_source_id=1001,
        refresh_enabled=True,
        refresh_hour_utc=6,
        refresh_minute_utc=30,
        max_backfill_days=45,
        staleness_max_days=2,
        triangulation_max_hops=1,
        rounding_mode="half_even",
        prefer_official_fx=True,
        weekend_policy="fill_previous",
    )
    assert updated_policy is not None
    assert updated_policy.base_commodity_id == eur.id
    assert updated_policy.default_source_id == 1001
    assert updated_policy.rounding_mode == "half_even"

    book = await repository_session.get(Book, 1)
    assert book is not None
    assert book.base_currency_code == "EUR"

    assignment = await repository.create_pricing_source_assignment(
        book_id=1,
        from_currency_id=eur.id,
        to_currency_id=1,
        source_id=1001,
        effective_from=date(2026, 5, 1),
        effective_to=None,
    )
    assignments = await repository.list_pricing_source_assignments(1)
    assert len(assignments) == 1
    assert assignments[0][0].id == assignment.id
    assert assignments[0][1].symbol == "EUR"
    assert assignments[0][3].name == "ECB"

    updated_assignment = await repository.update_pricing_source_assignment(
        assignment_id=assignment.id,
        book_id=1,
        from_currency_id=eur.id,
        to_currency_id=1,
        source_id=1005,
        effective_from=date(2026, 5, 15),
        effective_to=date(2026, 6, 30),
    )
    assert updated_assignment is not None
    assert updated_assignment.source_id == 1005

    repository_session.add(
        PricingRefreshState(
            book_id=1,
            commodity_id=eur.id,
            quote_commodity_id=1,
            source_id=1005,
            last_success_date=date(2026, 5, 20),
            last_attempt_at=datetime(2026, 5, 20, 6, 31, tzinfo=UTC),
            last_error=None,
        )
    )
    await repository_session.commit()

    refresh_state = await repository.list_pricing_refresh_state(1)
    assert len(refresh_state) == 1
    assert refresh_state[0][1].symbol == "EUR"
    assert refresh_state[0][3].name == "Federal Reserve"

    enabled_policies = await repository.list_enabled_pricing_policies()
    assert [policy.book_id for policy in enabled_policies] == [1]

    currencies, base_currency_code = await repository.list_book_currencies(1)
    assert base_currency_code == "EUR"
    assert {currency.symbol for currency in currencies} >= {"USD", "EUR"}

    await repository.record_pricing_refresh_success(
        ingest_run_id=999,
        book_id=1,
        commodity_id=eur.id,
        quote_commodity_id=1,
        source_id=1001,
        last_success_date=date(2026, 5, 21),
        attempted_at=datetime(2026, 5, 21, 6, 30, tzinfo=UTC),
        observations=[
            PriceObservation(
                book_id=1,
                commodity_id=eur.id,
                quote_commodity_id=1,
                observation_kind="fx_daily",
                price_minor=12_500,
                price_date=date(2026, 5, 21),
                mode="daily",
                source="ECB",
                period_type="daily",
                period_year=2026,
                period_month=5,
            )
        ],
    )

    existing_dates = await repository.list_existing_price_observation_dates(
        book_id=1,
        commodity_id=eur.id,
        quote_commodity_id=1,
        observation_kind="fx_daily",
        source="ECB",
        start_date=date(2026, 5, 21),
        end_date=date(2026, 5, 21),
    )
    assert existing_dates == {date(2026, 5, 21)}
    persisted = await repository_session.scalar(
        select(PriceObservation).where(PriceObservation.ingest_run_id == 999)
    )
    assert persisted is not None
    assert persisted.mode == "daily"

    recorded_run = await repository.record_pricing_refresh_run(
        book_id=1,
        trigger="manual",
        started_at=datetime(2026, 5, 21, 6, 30, tzinfo=UTC),
        finished_at=datetime(2026, 5, 21, 6, 31, tzinfo=UTC),
        pairs_total=1,
        pairs_success=1,
        pairs_failed=0,
        rates_inserted=1,
        derived_inserted=0,
        last_error=None,
    )
    history = await repository.list_pricing_refresh_run_history(1, limit=10)
    latest_run = await repository.get_latest_pricing_refresh_run(1)

    assert history[0].id == recorded_run.id
    assert latest_run is not None
    assert latest_run.id == recorded_run.id

    assert await repository.delete_pricing_source_assignment(assignment.id) is True
    assert await repository.list_pricing_source_assignments(1) == []


@pytest.mark.asyncio
async def test_pricing_repository_manages_append_only_commodity_price_sources(
    repository_session: AsyncSession,
) -> None:
    repository = PricingRepository(repository_session)
    security = await repository_session.merge(
        Commodity(
            book_id=1,
            kind="security",
            symbol="VWRL",
            name="Vanguard FTSE All-World",
            scale=4,
        )
    )
    await repository_session.commit()
    await repository_session.refresh(security)

    created = await repository.create_commodity_price_source(
        book_id=1,
        commodity_id=security.id,
        source_id=1010,
        symbol="VWRL.AS",
        provider_instrument_id=None,
        exchange_code="AS",
        mic="XAMS",
        name_override="VWRL Amsterdam",
        is_primary=True,
        metadata_json='{"assetClass":"equity"}',
        effective_from=None,
        effective_to=None,
    )
    listed = await repository.list_commodity_price_sources(book_id=1, commodity_id=security.id)
    resolved = await repository.resolve_commodity_price_source(
        book_id=1, commodity_id=security.id, source_id=1010
    )

    assert listed[0][0].id == created.id
    assert listed[0][0].symbol == "VWRL.AS"
    assert listed[0][1].name == "Vanguard FTSE All-World"
    assert listed[0][2].name == "Yahoo Finance"
    assert resolved is not None
    assert resolved.symbol == "VWRL.AS"

    updated = await repository.update_commodity_price_source(
        commodity_price_source_id=created.id,
        book_id=1,
        commodity_id=security.id,
        source_id=1010,
        symbol="VWCE.DE",
        provider_instrument_id=None,
        exchange_code="DE",
        mic="XETR",
        name_override=None,
        is_primary=True,
        metadata_json=None,
        effective_from=None,
        effective_to=None,
    )
    assert updated is not None
    assert updated.previous_commodity_price_source_id == created.id
    assert updated.symbol == "VWCE.DE"

    listed_after_update = await repository.list_commodity_price_sources(
        book_id=1, commodity_id=security.id
    )
    assert [row[0].id for row in listed_after_update] == [updated.id]

    assert await repository.delete_commodity_price_source(created.id) is True
    assert await repository.list_commodity_price_sources(book_id=1, commodity_id=security.id) == []

    all_rows = (
        (
            await repository_session.execute(
                select(CommodityPriceSource).where(CommodityPriceSource.commodity_id == security.id)
            )
        )
        .scalars()
        .all()
    )
    assert len(all_rows) == 3


@pytest.mark.asyncio
async def test_pricing_repository_supersedes_price_observations_append_only(
    repository_session: AsyncSession,
) -> None:
    repository = PricingRepository(repository_session)
    security = await repository_session.merge(
        Commodity(book_id=1, kind="security", symbol="ACME", name="Acme Corp", scale=4)
    )
    await repository_session.commit()
    await repository_session.refresh(security)

    created = await repository.create_market_price_observation(
        book_id=1,
        commodity_id=security.id,
        quote_commodity_id=1,
        price_date=date(2026, 5, 1),
        price_minor=12_345,
        source="manual",
    )
    corrected = await repository.create_market_price_observation(
        book_id=1,
        commodity_id=security.id,
        quote_commodity_id=1,
        price_date=date(2026, 5, 1),
        price_minor=12_500,
        source="manual",
        supersedes_observation_id=created.id,
    )

    listed = await repository.list_market_price_observations(
        book_id=1, commodity_id=security.id, quote_commodity_id=1, limit=10
    )
    current_from_original = await repository.get_price_observation(created.id)

    assert [row[0].id for row in listed] == [corrected.id]
    assert current_from_original is not None
    assert current_from_original.id == corrected.id
    assert corrected.supersedes_observation_id == created.id
    assert corrected.is_manual is True
    assert corrected.mode == "market"

    assert await repository.delete_market_price_observation(created.id) is True
    assert (
        await repository.list_market_price_observations(
            book_id=1, commodity_id=security.id, quote_commodity_id=1, limit=10
        )
        == []
    )

    all_rows = (
        (
            await repository_session.execute(
                select(PriceObservation).where(PriceObservation.commodity_id == security.id)
            )
        )
        .scalars()
        .all()
    )
    assert len(all_rows) == 3
    tombstone = next(row for row in all_rows if row.voided_at is not None)
    assert tombstone.void_reason == "delete"


@pytest.mark.asyncio
async def test_pricing_repository_extracts_implicit_market_price_from_transaction(
    repository_session: AsyncSession,
) -> None:
    repository = PricingRepository(repository_session)
    security = Commodity(
        id=210,
        book_id=1,
        kind="security",
        symbol="VWRL",
        name="Vanguard ETF",
        scale=4,
    )
    repository_session.add(security)
    repository_session.add_all(
        [
            Account(
                id=200,
                book_id=1,
                account_type="cash",
                name="Cash",
                commodity_id=1,
                booking_policy="fifo",
            ),
            Account(
                id=201,
                book_id=1,
                account_type="investment",
                name="Brokerage",
                commodity_id=1,
                booking_policy="fifo",
            ),
        ]
    )
    await repository_session.commit()
    await repository_session.refresh(security)

    repository_session.add(
        Transaction(
            id=300,
            book_id=1,
            occurred_date=date(2026, 5, 4),
            posted_date=date(2026, 5, 4),
            status="cleared",
        )
    )
    await repository_session.commit()

    repository_session.add_all(
        [
            Split(
                id=301,
                tx_id=300,
                account_id=201,
                commodity_id=security.id,
                amount_minor=10_000,
            ),
            Split(
                id=302,
                tx_id=300,
                account_id=200,
                commodity_id=1,
                amount_minor=-25_000,
            ),
        ]
    )
    await repository_session.commit()

    observation = await repository.create_implicit_market_price_from_transaction(300)
    same_observation = await repository.create_implicit_market_price_from_transaction(300)

    assert observation is not None
    assert same_observation is not None
    assert same_observation.id == observation.id
    assert observation.book_id == 1
    assert observation.commodity_id == security.id
    assert observation.quote_commodity_id == 1
    assert observation.price_date == date(2026, 5, 4)
    assert observation.price_minor == 25_000
    assert observation.source == "implicit"
    assert observation.is_manual is False
    assert observation.is_derived is True

    all_implicit_rows = (
        (
            await repository_session.execute(
                select(PriceObservation).where(PriceObservation.source == "implicit")
            )
        )
        .scalars()
        .all()
    )
    assert len(all_implicit_rows) == 1
