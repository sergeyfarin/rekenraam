import hashlib
import json
from collections.abc import Sequence
from datetime import date
from typing import Protocol, cast

from rekenraam_api.db.models.report_metadata import ReportDefinition, ReportRun
from rekenraam_api.repositories.reports import ReportRepository
from rekenraam_api.schemas.reports import (
    AccountTrendReportInput,
    AccountTrendRow,
    CashflowReportInput,
    CashflowRow,
    CategorySpendReportInput,
    CategorySpendRow,
    NetWorthReportInput,
    NetWorthRow,
    PayeeTotalRow,
    PayeeTotalsReportInput,
    ReportDefinitionCreateInput,
    ReportDefinitionSummary,
    ReportDefinitionUpdateInput,
    ReportRunCreateInput,
    ReportRunSummary,
)
from rekenraam_api.services.access import AccessPolicy


class CacheableReportRow(Protocol):
    def model_dump(self, *, mode: str) -> dict[str, object]: ...


type ReportCacheInput = (
    CashflowReportInput
    | NetWorthReportInput
    | AccountTrendReportInput
    | CategorySpendReportInput
    | PayeeTotalsReportInput
)


class ReportService:
    def __init__(
        self, repository: ReportRepository, access_policy: AccessPolicy | None = None
    ) -> None:
        self._repository = repository
        self._access_policy = access_policy

    async def list_report_definitions(self, book_id: int) -> list[ReportDefinitionSummary]:
        if self._access_policy is not None:
            await self._access_policy.require_book_read(book_id)
        rows = await self._repository.list_report_definitions(book_id)
        return [self._to_report_definition_summary(row) for row in rows]

    async def get_report_definition(self, definition_id: int) -> ReportDefinitionSummary | None:
        row = await self._repository.get_report_definition(definition_id)
        if row is None:
            return None
        if self._access_policy is not None:
            await self._access_policy.require_book_read(row.book_id)
        return self._to_report_definition_summary(row)

    async def create_report_definition(
        self, input: ReportDefinitionCreateInput
    ) -> ReportDefinitionSummary:
        self._validate_report_definition(input.kind, input.query_type)
        if self._access_policy is not None:
            await self._access_policy.require_book_write(input.book_id)
        row = await self._repository.create_report_definition(
            book_id=input.book_id,
            name=input.name,
            kind=input.kind,
            query_type=input.query_type,
            query_text=input.query_text,
            params_schema=input.params_schema,
        )
        return self._to_report_definition_summary(row)

    async def update_report_definition(
        self, input: ReportDefinitionUpdateInput
    ) -> ReportDefinitionSummary | None:
        self._validate_report_definition(input.kind, input.query_type)
        current = await self._repository.get_report_definition(input.id)
        if current is not None and self._access_policy is not None:
            await self._access_policy.require_book_write(current.book_id)
            if input.book_id != current.book_id:
                raise ValueError("report definition book cannot be changed")
        row = await self._repository.update_report_definition(
            definition_id=input.id,
            book_id=input.book_id,
            name=input.name,
            kind=input.kind,
            query_type=input.query_type,
            query_text=input.query_text,
            params_schema=input.params_schema,
        )
        if row is None:
            return None
        return self._to_report_definition_summary(row)

    async def delete_report_definition(self, definition_id: int) -> bool:
        current = await self._repository.get_report_definition(definition_id)
        if current is not None and self._access_policy is not None:
            await self._access_policy.require_book_write(current.book_id)
        return await self._repository.delete_report_definition(definition_id)

    async def list_report_runs(
        self, book_id: int, definition_id: int | None = None
    ) -> list[ReportRunSummary]:
        if self._access_policy is not None:
            await self._access_policy.require_book_read(book_id)
        rows = await self._repository.list_report_runs(book_id, definition_id)
        return [self._to_report_run_summary(row) for row in rows]

    async def create_report_run(self, input: ReportRunCreateInput) -> ReportRunSummary:
        if input.pricing_mode not in {"frozen", "latest_corrected"}:
            raise ValueError("pricing_mode must be frozen or latest_corrected")
        if self._access_policy is not None:
            await self._access_policy.require_book_write(input.book_id)
        definition = await self._repository.get_report_definition(input.definition_id)
        if definition is None:
            raise ValueError("report definition not found")
        row = await self._repository.create_report_run(
            book_id=input.book_id,
            definition_id=input.definition_id,
            params_hash=input.params_hash,
            as_of_seq=input.as_of_seq,
            pricing_mode=input.pricing_mode,
            pricing_policy_id=input.pricing_policy_id,
            pricing_policy_version=input.pricing_policy_version,
            valuation_snapshot_id=input.valuation_snapshot_id,
            pricing_resolved_at=input.pricing_resolved_at,
            result_json=input.result_json,
        )
        return self._to_report_run_summary(row)

    async def report_cashflow(self, input: CashflowReportInput) -> list[CashflowRow]:
        if self._access_policy is not None:
            await self._access_policy.require_book_read(input.book_id)
        group_by = (input.group_by or "month").strip().lower()
        if group_by not in {"day", "month", "quarter", "year"}:
            raise ValueError("group_by must be day, month, quarter, or year")

        cached_rows = await self._get_cached_rows("cashflow", input)
        if cached_rows is not None:
            return [CashflowRow.model_validate(row) for row in cached_rows]

        rows = await self._repository.report_cashflow(
            book_id=input.book_id,
            date_from=input.date_from,
            date_to=input.date_to,
            group_by=group_by,
        )
        mapped_rows = [
            CashflowRow(
                period_start=date.fromisoformat(period_start),
                inflow_minor=inflow_minor,
                outflow_minor=outflow_minor,
                net_minor=net_minor,
            )
            for period_start, inflow_minor, outflow_minor, net_minor in rows
        ]
        await self._save_cached_rows("cashflow", input, mapped_rows)
        return mapped_rows

    async def report_net_worth(self, input: NetWorthReportInput) -> list[NetWorthRow]:
        if self._access_policy is not None:
            await self._access_policy.require_book_read(input.book_id)
        group_by = self._validate_group_by(input.group_by)
        cached_rows = await self._get_cached_rows("net_worth", input)
        if cached_rows is not None:
            return [NetWorthRow.model_validate(row) for row in cached_rows]
        rows = await self._repository.report_net_worth(
            book_id=input.book_id,
            date_from=input.date_from,
            date_to=input.date_to,
            group_by=group_by,
        )
        mapped_rows = [
            NetWorthRow(
                period_start=date.fromisoformat(period_start),
                assets_minor=assets_minor,
                liabilities_minor=liabilities_minor,
                net_worth_minor=net_worth_minor,
            )
            for period_start, assets_minor, liabilities_minor, net_worth_minor in rows
        ]
        await self._save_cached_rows("net_worth", input, mapped_rows)
        return mapped_rows

    async def report_account_trends(self, input: AccountTrendReportInput) -> list[AccountTrendRow]:
        if self._access_policy is not None:
            await self._access_policy.require_book_read(input.book_id)
        group_by = self._validate_group_by(input.group_by)
        cached_rows = await self._get_cached_rows("account_trends", input)
        if cached_rows is not None:
            return [AccountTrendRow.model_validate(row) for row in cached_rows]
        rows = await self._repository.report_account_trends(
            book_id=input.book_id,
            account_ids=input.account_ids,
            date_from=input.date_from,
            date_to=input.date_to,
            group_by=group_by,
        )
        mapped_rows = [
            AccountTrendRow(
                period_start=date.fromisoformat(period_start),
                account_id=account_id,
                account_name=account_name,
                balance_minor=balance_minor,
            )
            for period_start, account_id, account_name, balance_minor in rows
        ]
        await self._save_cached_rows("account_trends", input, mapped_rows)
        return mapped_rows

    async def report_category_spend(
        self, input: CategorySpendReportInput
    ) -> list[CategorySpendRow]:
        if self._access_policy is not None:
            await self._access_policy.require_book_read(input.book_id)
        cached_rows = await self._get_cached_rows("category_spend", input)
        if cached_rows is not None:
            return [CategorySpendRow.model_validate(row) for row in cached_rows]

        rows = await self._repository.report_category_spend(
            book_id=input.book_id,
            date_from=input.date_from,
            date_to=input.date_to,
            category_ids=input.category_ids,
        )
        mapped_rows = [
            CategorySpendRow(
                category_id=category_id, category_name=category_name, total_minor=total_minor
            )
            for category_id, category_name, total_minor in rows
        ]
        await self._save_cached_rows("category_spend", input, mapped_rows)
        return mapped_rows

    async def report_payee_totals(self, input: PayeeTotalsReportInput) -> list[PayeeTotalRow]:
        if self._access_policy is not None:
            await self._access_policy.require_book_read(input.book_id)
        cached_rows = await self._get_cached_rows("payee_totals", input)
        if cached_rows is not None:
            return [PayeeTotalRow.model_validate(row) for row in cached_rows]

        rows = await self._repository.report_payee_totals(
            book_id=input.book_id,
            date_from=input.date_from,
            date_to=input.date_to,
            payee_ids=input.payee_ids,
        )
        mapped_rows = [
            PayeeTotalRow(payee_id=payee_id, payee_name=payee_name, total_minor=total_minor)
            for payee_id, payee_name, total_minor in rows
        ]
        await self._save_cached_rows("payee_totals", input, mapped_rows)
        return mapped_rows

    async def bump_report_state(self, book_id: int) -> int:
        if self._access_policy is not None:
            await self._access_policy.require_book_write(book_id)
        return await self._repository.bump_book_change_seq(book_id)

    async def _get_cached_rows(
        self, report_type: str, input: ReportCacheInput
    ) -> list[dict[str, object]] | None:
        params_json = self._params_json(input)
        params_hash = self._params_hash(params_json)
        as_of_seq = await self._repository.get_book_change_seq(input.book_id)
        cache_row = await self._repository.get_cached_report(
            book_id=input.book_id,
            report_type=report_type,
            params_hash=params_hash,
            as_of_seq=as_of_seq,
        )
        if cache_row is None:
            return None
        return cast(list[dict[str, object]], json.loads(cache_row.payload_json))

    async def _save_cached_rows(
        self, report_type: str, input: ReportCacheInput, rows: Sequence[CacheableReportRow]
    ) -> None:
        params_json = self._params_json(input)
        params_hash = self._params_hash(params_json)
        as_of_seq = await self._repository.get_book_change_seq(input.book_id)
        await self._repository.save_cached_report(
            book_id=input.book_id,
            report_type=report_type,
            params_hash=params_hash,
            params_json=params_json,
            as_of_seq=as_of_seq,
            payload_json=json.dumps([row.model_dump(mode="json") for row in rows], sort_keys=True),
        )

    @staticmethod
    def _params_json(input: ReportCacheInput) -> str:
        return json.dumps(input.model_dump(mode="json"), sort_keys=True)

    @staticmethod
    def _params_hash(params_json: str) -> str:
        return hashlib.sha256(params_json.encode("utf-8")).hexdigest()

    @staticmethod
    def _validate_report_definition(kind: str, query_type: str) -> None:
        if kind not in {"builtin", "custom"}:
            raise ValueError("kind must be builtin or custom")
        if query_type not in {"sql", "template"}:
            raise ValueError("query_type must be sql or template")

    @staticmethod
    def _validate_group_by(value: str | None) -> str:
        group_by = (value or "month").strip().lower()
        if group_by not in {"day", "month", "quarter", "year"}:
            raise ValueError("group_by must be day, month, quarter, or year")
        return group_by

    @staticmethod
    def _to_report_definition_summary(row: ReportDefinition) -> ReportDefinitionSummary:
        return ReportDefinitionSummary(
            id=row.id,
            book_id=row.book_id,
            name=row.name,
            kind=row.kind,
            query_type=row.query_type,
            query_text=row.query_text,
            params_schema=row.params_schema,
            created_at=row.created_at,
        )

    @staticmethod
    def _to_report_run_summary(row: ReportRun) -> ReportRunSummary:
        return ReportRunSummary(
            id=row.id,
            book_id=row.book_id,
            definition_id=row.definition_id,
            params_hash=row.params_hash,
            as_of_seq=row.as_of_seq,
            pricing_mode=row.pricing_mode,
            pricing_policy_id=row.pricing_policy_id,
            pricing_policy_version=row.pricing_policy_version,
            valuation_snapshot_id=row.valuation_snapshot_id,
            pricing_resolved_at=row.pricing_resolved_at,
            result_json=row.result_json,
            created_at=row.created_at,
        )
