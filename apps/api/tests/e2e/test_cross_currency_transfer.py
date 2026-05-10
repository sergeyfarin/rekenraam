from __future__ import annotations

import pytest
from httpx import AsyncClient
from sqlalchemy import select

from e2e.conftest import E2EApp
from e2e.factories import bootstrap_admin, create_account
from rekenraam_api.db.models.investments import PriceObservation

SEEDED_CASH_ACCOUNT_ID = 2
SEEDED_USD_COMMODITY_ID = 1


async def _create_eur_currency(client: AsyncClient) -> int:
    response = await client.post(
        "/api/v1/currencies",
        json={
            "book_id": 1,
            "symbol": "EUR",
            "name": "Euro",
            "scale": 2,
            "display_symbol": "EUR",
        },
    )
    response.raise_for_status()
    return int(response.json()["id"])


@pytest.mark.asyncio
async def test_cross_currency_transfer_posts_two_sides_and_stamps_fx_rate(
    e2e_app: E2EApp,
) -> None:
    await bootstrap_admin(e2e_app.client)
    eur_id = await _create_eur_currency(e2e_app.client)
    euro_account = await create_account(e2e_app.client, name="Euro Wallet", commodity_id=eur_id)

    response = await e2e_app.client.post(
        "/api/v1/transactions/transfer",
        json={
            "book_id": 1,
            "txn_date": "2026-05-10",
            "source_account_id": SEEDED_CASH_ACCOUNT_ID,
            "destination_account_id": euro_account["id"],
            "source_amount_minor": 11_200,
            "destination_amount_minor": 10_000,
            "fx_rate": 1.12,
            "memo": "USD to EUR",
            "source_memo": "sold USD",
            "destination_memo": "bought EUR",
        },
    )

    assert response.status_code == 200
    transaction = response.json()
    assert transaction["memo"] == "USD to EUR"
    assert [
        (split["account_id"], split["commodity_id"], split["amount_minor"])
        for split in transaction["splits"]
    ] == [
        (SEEDED_CASH_ACCOUNT_ID, SEEDED_USD_COMMODITY_ID, -11_200),
        (euro_account["id"], eur_id, 10_000),
    ]

    async with e2e_app.sessionmaker() as session:
        observation = await session.scalar(
            select(PriceObservation).where(
                PriceObservation.source == f"transfer:{transaction['id']}"
            )
        )
    assert observation is not None
    assert observation.commodity_id == eur_id
    assert observation.quote_commodity_id == SEEDED_USD_COMMODITY_ID
    assert observation.observation_kind == "fx_manual"
    assert observation.price_date.isoformat() == "2026-05-10"
    assert observation.price_minor == 112


@pytest.mark.asyncio
async def test_cross_currency_transfer_writes_realized_fx_gain_loss_split(
    e2e_app: E2EApp,
) -> None:
    await bootstrap_admin(e2e_app.client)
    eur_id = await _create_eur_currency(e2e_app.client)
    euro_account = await create_account(e2e_app.client, name="Euro Savings", commodity_id=eur_id)
    fx_loss_account = await create_account(
        e2e_app.client,
        name="Realized FX Gain Loss",
        account_type="expense",
        commodity_id=SEEDED_USD_COMMODITY_ID,
    )

    response = await e2e_app.client.post(
        "/api/v1/transactions/transfer",
        json={
            "book_id": 1,
            "txn_date": "2026-05-10",
            "source_account_id": SEEDED_CASH_ACCOUNT_ID,
            "destination_account_id": euro_account["id"],
            "source_amount_minor": 11_350,
            "destination_amount_minor": 10_000,
            "fx_rate": 1.12,
            "fx_gain_loss_account_id": fx_loss_account["id"],
            "memo": "USD to EUR with loss",
        },
    )

    assert response.status_code == 200
    splits = response.json()["splits"]
    assert [
        (split["account_id"], split["commodity_id"], split["amount_minor"]) for split in splits
    ] == [
        (SEEDED_CASH_ACCOUNT_ID, SEEDED_USD_COMMODITY_ID, -11_350),
        (euro_account["id"], eur_id, 10_000),
        (fx_loss_account["id"], SEEDED_USD_COMMODITY_ID, 150),
    ]


@pytest.mark.asyncio
async def test_cross_currency_transfer_rejects_missing_rate(e2e_client: AsyncClient) -> None:
    await bootstrap_admin(e2e_client)
    eur_id = await _create_eur_currency(e2e_client)
    euro_account = await create_account(e2e_client, name="Euro Wallet", commodity_id=eur_id)

    response = await e2e_client.post(
        "/api/v1/transactions/transfer",
        json={
            "book_id": 1,
            "txn_date": "2026-05-10",
            "source_account_id": SEEDED_CASH_ACCOUNT_ID,
            "destination_account_id": euro_account["id"],
            "source_amount_minor": 11_200,
            "destination_amount_minor": 10_000,
        },
    )

    assert response.status_code == 422
