from datetime import date

import pytest
from sqlalchemy import select
from sqlalchemy.ext.asyncio import AsyncSession

from rekenraam_api.db.models.accounts import Account, AccountBalancing
from rekenraam_api.db.models.imports import ImportSession, ImportSessionTransaction
from rekenraam_api.db.models.transactions import Transaction
from rekenraam_api.repositories.imports import ImportRepository
from rekenraam_api.repositories.investments import InvestmentRepository
from rekenraam_api.repositories.reports import ReportRepository
from rekenraam_api.schemas.imports import ImportCommitRequest, ImportPreviewRequest
from rekenraam_api.services.exports import ExportService
from rekenraam_api.services.imports import ImportService
from rekenraam_api.services.investments import InvestmentService
from rekenraam_api.services.reports import ReportService


class _FakeCommodity:
    id = 1
    symbol = "USD"


class _FakeImportRepository:
    async def get_account_commodity(self, account_id: int) -> _FakeCommodity:
        return _FakeCommodity()

    async def find_matching_transaction(self, *args, **kwargs) -> None:
        return None


@pytest.mark.asyncio
async def test_import_preview_parses_csv_without_database() -> None:
    service = ImportService(_FakeImportRepository())  # type: ignore[arg-type]
    preview = await service.preview(
        ImportPreviewRequest(
            account_id=2,
            format="csv",
            content="date,amount,payee,memo,import_id\n2026-05-01,12.34,Cafe,Lunch,fit-1\n",
            file_name="bank.csv",
        )
    )

    assert preview.file_error is None
    assert preview.rows[0].draft is not None
    assert preview.rows[0].draft.amount_minor == 1234
    assert preview.rows[0].draft.txn_date == date(2026, 5, 1)


@pytest.mark.asyncio
async def test_import_preview_parses_csv_and_marks_duplicates(
    repository_session: AsyncSession,
) -> None:
    service = ImportService(ImportRepository(repository_session))
    csv_content = "date,amount,payee,memo,import_id\n2026-05-01,12.34,Cafe,Lunch,fit-1\n"

    preview = await service.preview(
        ImportPreviewRequest(account_id=2, format="csv", content=csv_content, file_name="bank.csv")
    )

    assert preview.file_error is None
    assert preview.format == "csv"
    assert len(preview.rows) == 1
    assert preview.rows[0].draft is not None
    assert preview.rows[0].draft.txn_date == date(2026, 5, 1)
    assert preview.rows[0].draft.amount_minor == 1234
    assert preview.rows[0].is_duplicate is False


@pytest.mark.asyncio
async def test_import_commit_creates_session_transaction_and_export(
    repository_session: AsyncSession,
) -> None:
    import_service = ImportService(ImportRepository(repository_session))
    export_service = ExportService(
        repository_session,
        ReportService(ReportRepository(repository_session)),
        InvestmentService(InvestmentRepository(repository_session)),
    )
    csv_content = "date,amount,payee,memo,import_id\n2026-05-02,-5.25,Grocer,Milk,fit-2\n"
    preview = await import_service.preview(
        ImportPreviewRequest(account_id=2, format="csv", content=csv_content, file_name="bank.csv")
    )

    result = await import_service.commit(
        ImportCommitRequest(
            account_id=2,
            drafts=tuple(row.draft for row in preview.rows if row.draft is not None),
            file_name="bank.csv",
        )
    )

    assert result.session.status == "committed"
    assert len(result.batch.created_tx_ids) == 1
    tx_id = result.batch.created_tx_ids[0]
    tx = await repository_session.get(Transaction, tx_id)
    assert tx is not None
    assert tx.import_id == "fit-2"
    audit_rows = (
        (
            await repository_session.execute(
                select(ImportSessionTransaction).where(
                    ImportSessionTransaction.session_id == result.session.id
                )
            )
        )
        .scalars()
        .all()
    )
    assert [(row.tx_id, row.action) for row in audit_rows] == [(tx_id, "created")]

    transactions_csv = await export_service.transactions_csv(1)
    assert "fit-2" in transactions_csv
    assert "Grocer" in transactions_csv


@pytest.mark.asyncio
async def test_import_commit_marks_session_abandoned_on_locked_account(
    repository_session: AsyncSession,
) -> None:
    service = ImportService(ImportRepository(repository_session))
    account = Account(
        book_id=1,
        account_type="asset",
        name="Locked Import Test",
        commodity_id=1,
    )
    repository_session.add(account)
    await repository_session.flush()
    repository_session.add(
        AccountBalancing(
            book_id=1,
            account_id=account.id,
            as_of_date=date(2026, 5, 3),
            balance_minor=0,
        )
    )
    await repository_session.commit()

    with pytest.raises(ValueError, match="cannot post transaction dated"):
        await service.commit(
            ImportCommitRequest(
                account_id=account.id,
                drafts=(
                    {
                        "txn_date": "2026-05-02",
                        "amount_minor": 100,
                        "payee_name": "Late",
                    },
                ),
                file_name="locked.csv",
            )
        )

    session = (
        (
            await repository_session.execute(
                select(ImportSession)
                .where(ImportSession.file_name == "locked.csv")
                .order_by(ImportSession.id.desc())
            )
        )
        .scalars()
        .first()
    )
    assert session is not None
    assert session.status == "abandoned"
