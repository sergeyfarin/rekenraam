from datetime import UTC, date, datetime

import pytest
from sqlalchemy.ext.asyncio import AsyncSession

from rekenraam_api.db.models.books import Book
from rekenraam_api.db.models.investments import PriceObservation
from rekenraam_api.db.models.metadata import Commodity
from rekenraam_api.db.models.pricing import PricingRefreshState
from rekenraam_api.repositories.pricing import PricingRepository


@pytest.mark.asyncio
async def test_pricing_repository_lists_sources_updates_policy_and_manages_assignments(repository_session: AsyncSession) -> None:
    repository = PricingRepository(repository_session)
    eur = Commodity(book_id=1, kind="currency", symbol="EUR", name="Euro", scale=2)
    await repository_session.merge(eur)
    await repository_session.commit()
    await repository_session.refresh(eur)

    sources = await repository.list_price_sources()
    assert sources[0].name == "Bank of Canada"
    assert any(source.name == "ECB" for source in sources)

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
        weekend_policy="fill_previous",
    )
    assert updated_policy is not None
    assert updated_policy.base_commodity_id == eur.id
    assert updated_policy.default_source_id == 1001

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
                source="ECB",
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

    assert await repository.delete_pricing_source_assignment(assignment.id) is True
    assert await repository.list_pricing_source_assignments(1) == []