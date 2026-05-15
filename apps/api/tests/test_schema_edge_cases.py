from datetime import date

import pytest
from pydantic import ValidationError

from rekenraam_api.schemas.imports import (
    ImportPreviewRequest,
    ImportRuleCreate,
)
from rekenraam_api.schemas.transactions import (
    CrossCurrencyTransferInput,
    TransactionListFilters,
)


def test_transaction_filters_reject_inverted_date_ranges() -> None:
    with pytest.raises(ValidationError, match="occurred_from must be on or before occurred_to"):
        TransactionListFilters(
            occurred_from=date(2026, 5, 31),
            occurred_to=date(2026, 5, 1),
        )


@pytest.mark.parametrize(
    ("payload", "message"),
    [
        ({"source_account_id": 1, "destination_account_id": 1}, "accounts must differ"),
        ({"source_amount_minor": 0}, "source_amount_minor must be greater than zero"),
        ({"destination_amount_minor": 0}, "destination_amount_minor must be greater than zero"),
        ({"fx_rate": 0}, "fx_rate must be greater than zero"),
    ],
)
def test_cross_currency_transfer_rejects_invalid_shape(
    payload: dict[str, int],
    message: str,
) -> None:
    values = {
        "book_id": 1,
        "txn_date": date(2026, 5, 3),
        "source_account_id": 2,
        "destination_account_id": 4,
        "source_amount_minor": 1000,
        "destination_amount_minor": 900,
        "fx_rate": 1.1,
    }
    values.update(payload)

    with pytest.raises(ValidationError, match=message):
        CrossCurrencyTransferInput(**values)


def test_import_preview_requires_inline_or_base64_content() -> None:
    with pytest.raises(ValidationError, match="content or content_base64 is required"):
        ImportPreviewRequest(book_id=1, account_id=2, format="csv")


@pytest.mark.parametrize(
    ("payload", "message"),
    [
        (
            {"amount_min_minor": 100, "amount_max_minor": 99},
            "amount_min_minor must be <= amount_max_minor",
        ),
        (
            {"date_from": date(2026, 5, 2), "date_to": date(2026, 5, 1)},
            "date_from must be <= date_to",
        ),
    ],
)
def test_import_rule_rejects_inverted_ranges(payload: dict[str, object], message: str) -> None:
    values = {
        "book_id": 1,
        "rule_kind": "payee",
        "match_type": "contains",
        "match_text": "Coffee",
        "priority": 100,
    }
    values.update(payload)

    with pytest.raises(ValidationError, match=message):
        ImportRuleCreate(**values)
