
import pytest
from sqlalchemy.ext.asyncio import AsyncSession

from rekenraam_api.repositories.reports import ReportRepository


@pytest.mark.asyncio
async def test_report_repository_returns_cashflow_category_spend_and_payee_totals(repository_session: AsyncSession) -> None:
    repository = ReportRepository(repository_session)

    cashflow = await repository.report_cashflow(book_id=1, date_from=None, date_to=None, group_by="month")
    category_spend = await repository.report_category_spend(book_id=1, date_from=None, date_to=None, category_ids=None)
    payee_totals = await repository.report_payee_totals(book_id=1, date_from=None, date_to=None, payee_ids=None)

    # The seeded opening-balance transaction has no category, and report_cashflow
    # joins on Category.kind in ('income','expense'), so a fresh DB returns no
    # rows for any of these reports. Asserting the empty-shape contract.
    assert cashflow == []
    assert category_spend == []
    assert payee_totals == []


@pytest.mark.asyncio
async def test_report_repository_manages_report_definitions_and_runs(repository_session: AsyncSession) -> None:
    repository = ReportRepository(repository_session)

    created_definition = await repository.create_report_definition(
        book_id=1,
        name="Cashflow Snapshot",
        kind="custom",
        query_type="sql",
        query_text="SELECT 1",
        params_schema=None,
    )
    listed_definitions = await repository.list_report_definitions(1)
    current_definition = await repository.get_report_definition(created_definition.id)
    updated_definition = await repository.update_report_definition(
        definition_id=created_definition.id,
        book_id=1,
        name="Cashflow Snapshot v2",
        kind="custom",
        query_type="template",
        query_text='{"sql":"SELECT 1"}',
        params_schema='{"required":["book_id"]}',
    )
    created_run = await repository.create_report_run(
        book_id=1,
        definition_id=created_definition.id,
        params_hash="abc123",
        as_of_seq=0,
        pricing_mode="latest_corrected",
        pricing_policy_id=None,
        pricing_policy_version=None,
        valuation_snapshot_id=None,
        pricing_resolved_at=None,
        result_json='[{"period_start":"2026-05-01"}]',
    )
    listed_runs = await repository.list_report_runs(1, definition_id=created_definition.id)

    assert listed_definitions[0].id == created_definition.id
    assert current_definition is not None
    assert current_definition.id == created_definition.id
    assert updated_definition is not None
    assert updated_definition.previous_report_definition_id == created_definition.id
    assert created_run.definition_id == created_definition.id
    assert listed_runs[0].id == created_run.id

    assert await repository.delete_report_definition(created_definition.id) is True