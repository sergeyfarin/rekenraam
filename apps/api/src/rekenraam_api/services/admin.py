from __future__ import annotations

import asyncio
from dataclasses import dataclass
from datetime import UTC, date, datetime
from pathlib import Path

from alembic.config import Config
from alembic.script import ScriptDirectory
from sqlalchemy import Select, func, select, text
from sqlalchemy.engine import Connection, Engine
from sqlalchemy.ext.asyncio import AsyncSession

from rekenraam_api.config.settings import Settings
from rekenraam_api.db.dialect import sqlite_path_from_url
from rekenraam_api.db.models.accounts import Account
from rekenraam_api.db.models.books import Book
from rekenraam_api.db.models.metadata import Commodity
from rekenraam_api.db.models.transactions import Split, Transaction
from rekenraam_api.schemas.admin import (
    AdminRuntimeStatusSummary,
    FiscalYearCloseInput,
    FiscalYearCloseResult,
    IntegrityCheckSummary,
    RuntimeCheckSummary,
)
from rekenraam_api.services.access import SUPPORTED_V1_BOOK_ID
from rekenraam_api.services.report_invalidation import bump_report_state

SOURCE_API_ROOT = Path(__file__).resolve().parents[3]


def _api_root() -> Path:
    for candidate in (Path.cwd(), SOURCE_API_ROOT):
        if (candidate / "alembic.ini").exists() and (candidate / "alembic").exists():
            return candidate
    return SOURCE_API_ROOT


def _database_url_from_bind(bind: Engine | Connection) -> str:
    if isinstance(bind, Engine):
        return str(bind.url)
    return str(bind.engine.url)


def _file_size(path: str | None) -> int | None:
    if not path:
        return None
    file_path = Path(path)
    return file_path.stat().st_size if file_path.exists() else None


@dataclass(frozen=True)
class _ProfitLossSystemAccounts:
    income_summary_account_id: int
    expense_summary_account_id: int
    retained_earnings_account_id: int
    commodity_id: int


class AdminService:
    def __init__(self, session: AsyncSession, settings: Settings) -> None:
        self._session = session
        self._settings = settings

    async def get_runtime_status(self) -> AdminRuntimeStatusSummary:
        bind_url = _database_url_from_bind(self._session.get_bind())
        return await self._get_sqlite_runtime_status(bind_url)

    async def _get_sqlite_runtime_status(self, database_url: str) -> AdminRuntimeStatusSummary:
        path = sqlite_path_from_url(database_url)
        display_path = path or database_url
        database_name = Path(path).name if path else "sqlite"
        sqlite_version = await self._session.scalar(text("SELECT sqlite_version()"))
        foreign_keys_enabled = bool(await self._session.scalar(text("PRAGMA foreign_keys")))
        size_bytes = await asyncio.to_thread(_file_size, path)

        current_version = await self._current_migration_version()
        latest_version = self._latest_migration_version()
        pending_versions: tuple[str, ...] = (
            () if current_version == latest_version else (latest_version,)
        )
        writable = await self._probe_writable()

        return AdminRuntimeStatusSummary(
            database_kind="sqlite",
            database_name=database_name,
            database_host=None,
            database_user=None,
            database_version=sqlite_version,
            display_path=display_path,
            size_bytes=size_bytes,
            writable=writable,
            foreign_keys=foreign_keys_enabled,
            current_version=current_version,
            latest_version=latest_version,
            pending_versions=pending_versions,
            pending_migration_count=len(pending_versions),
            health_status=(
                "ok" if writable and foreign_keys_enabled and not pending_versions else "warning"
            ),
            backup_guidance=(
                "Use the documented SQLite online backup command before copying database files."
            ),
            note=(
                "The web runtime uses a server-local SQLite database file. Keep the /data volume"
                " private and rely on the online backup command for consistent snapshots."
            ),
        )

    async def run_integrity_check(self) -> IntegrityCheckSummary:
        checks: list[RuntimeCheckSummary] = []
        await self._session.execute(text("SELECT 1"))
        checks.append(
            RuntimeCheckSummary(
                name="database_connectivity", status="ok", detail="Database responded."
            )
        )

        current_version = await self._current_migration_version()
        latest_version = self._latest_migration_version()
        checks.append(
            RuntimeCheckSummary(
                name="migrations",
                status="ok" if current_version == latest_version else "warning",
                detail=f"Current {current_version or 'none'}; latest {latest_version}.",
            )
        )

        writable = await self._probe_writable()
        checks.append(
            RuntimeCheckSummary(
                name="writable",
                status="ok" if writable else "failed",
                detail="Writable probe completed." if writable else "Writable probe failed.",
            )
        )
        await self._add_count_check(
            checks,
            "double_entry_balance",
            "SELECT count(*) FROM (SELECT tx_id FROM splits GROUP BY tx_id HAVING sum(amount_minor) <> 0) bad",
            "All transactions balance to zero.",
            "{count} transaction(s) have unbalanced splits.",
        )
        await self._add_count_check(
            checks,
            "orphan_splits",
            """
            SELECT count(*)
            FROM splits s
            LEFT JOIN transactions t ON t.id = s.tx_id
            LEFT JOIN accounts a ON a.id = s.account_id
            WHERE t.id IS NULL OR a.id IS NULL
            """,
            "No orphan split references found.",
            "{count} orphan split reference(s) found.",
        )
        await self._add_count_check(
            checks,
            "auth_sessions",
            """
            SELECT count(*)
            FROM auth_sessions s
            LEFT JOIN users u ON u.id = s.user_id
            WHERE u.id IS NULL
            """,
            "Auth sessions reference valid users.",
            "{count} auth session(s) reference missing users.",
        )
        failed = any(check.status == "failed" for check in checks)
        warning = any(check.status == "warning" for check in checks)
        return IntegrityCheckSummary(
            status="failed" if failed else "warning" if warning else "ok",
            checked_at=datetime.now(UTC).isoformat(),
            checks=tuple(checks),
        )

    async def _add_count_check(
        self,
        checks: list[RuntimeCheckSummary],
        name: str,
        sql: str,
        ok_detail: str,
        fail_detail: str,
    ) -> None:
        count = int((await self._session.scalar(text(sql))) or 0)
        checks.append(
            RuntimeCheckSummary(
                name=name,
                status="ok" if count == 0 else "failed",
                detail=ok_detail if count == 0 else fail_detail.format(count=count),
            )
        )

    async def _current_migration_version(self) -> str | None:
        try:
            return await self._session.scalar(
                text("SELECT version_num FROM alembic_version LIMIT 1")
            )
        except Exception:
            return None

    @staticmethod
    def _latest_migration_version() -> str:
        api_root = _api_root()
        config = Config(str(api_root / "alembic.ini"))
        config.set_main_option("script_location", str(api_root / "alembic"))
        return str(ScriptDirectory.from_config(config).get_current_head())

    async def _probe_writable(self) -> bool:
        try:
            await self._session.execute(
                text("CREATE TEMP TABLE IF NOT EXISTS rekenraam_writable_probe (id integer)")
            )
            await self._session.execute(
                text("INSERT INTO rekenraam_writable_probe (id) VALUES (1)")
            )
            await self._session.execute(text("DELETE FROM rekenraam_writable_probe"))
            return True
        except Exception:
            await self._session.rollback()
            return False

    async def close_fiscal_year(self, input: FiscalYearCloseInput) -> FiscalYearCloseResult:
        today = date.today()
        if input.close_date > today:
            raise ValueError("close_date cannot be in the future")

        book_id = SUPPORTED_V1_BOOK_ID
        reference = f"year-close:{input.close_date.isoformat()}"
        existing_close_id = await self._session.scalar(
            select(Transaction.id)
            .where(Transaction.book_id == book_id)
            .where(Transaction.reference == reference)
            .where(Transaction.status != "void")
            .limit(1)
        )
        if existing_close_id is not None:
            raise ValueError("fiscal year already closed for close_date")

        system_accounts = await self._ensure_profit_loss_system_accounts(book_id)
        balances = await self._load_profit_loss_balances(book_id, input.close_date)

        account_adjustments: list[tuple[int, int, str]] = []
        retained_earnings_delta_minor = 0
        for account_id, account_type, commodity_id, balance_minor in balances:
            if commodity_id != system_accounts.commodity_id:
                raise ValueError(
                    "fiscal close requires income and expense accounts to use the book base commodity"
                )
            account_adjustments.append((account_id, -balance_minor, account_type))
            retained_earnings_delta_minor += balance_minor

        if not account_adjustments:
            return FiscalYearCloseResult(
                tx_id=None,
                closed_accounts_count=0,
                retained_earnings_delta_minor=0,
                close_date=input.close_date,
            )

        splits: list[tuple[int, int]] = []
        income_summary_balance = 0
        expense_summary_balance = 0
        for account_id, adjustment_minor, account_type in account_adjustments:
            splits.append((account_id, adjustment_minor))
            summary_amount = -adjustment_minor
            if account_type == "income":
                splits.append((system_accounts.income_summary_account_id, summary_amount))
                income_summary_balance += summary_amount
            else:
                splits.append((system_accounts.expense_summary_account_id, summary_amount))
                expense_summary_balance += summary_amount

        if income_summary_balance != 0:
            splits.append((system_accounts.income_summary_account_id, -income_summary_balance))
        if expense_summary_balance != 0:
            splits.append((system_accounts.expense_summary_account_id, -expense_summary_balance))
        if retained_earnings_delta_minor != 0:
            splits.append(
                (system_accounts.retained_earnings_account_id, retained_earnings_delta_minor)
            )

        if sum(amount_minor for _, amount_minor in splits) != 0:
            raise ValueError("fiscal close failed: generated splits are unbalanced")

        now = datetime.now(UTC)
        transaction = Transaction(
            book_id=book_id,
            occurred_date=input.close_date,
            posted_date=input.close_date,
            payee_id=None,
            memo=input.memo or f"Fiscal year close {input.close_date.isoformat()}",
            status="cleared",
            reference=reference,
            created_at=now,
        )
        self._session.add(transaction)
        await self._session.flush()

        for account_id, amount_minor in splits:
            if amount_minor == 0:
                continue
            self._session.add(
                Split(
                    tx_id=transaction.id,
                    account_id=account_id,
                    commodity_id=system_accounts.commodity_id,
                    amount_minor=amount_minor,
                    category_id=None,
                    tag_id=None,
                    person_id=None,
                    project_id=None,
                    share_bps=None,
                    memo=None,
                    created_at=now,
                )
            )

        await self._session.commit()
        await bump_report_state(self._session, book_id)
        return FiscalYearCloseResult(
            tx_id=transaction.id,
            closed_accounts_count=len(account_adjustments),
            retained_earnings_delta_minor=retained_earnings_delta_minor,
            close_date=input.close_date,
        )

    async def _ensure_profit_loss_system_accounts(self, book_id: int) -> _ProfitLossSystemAccounts:
        commodity_id = await self._resolve_base_commodity_id(book_id)
        income_account = await self._ensure_system_account(
            book_id=book_id,
            account_type="income",
            name="System Income Summary",
            commodity_id=commodity_id,
            system_role="income_summary",
        )
        expense_account = await self._ensure_system_account(
            book_id=book_id,
            account_type="expense",
            name="System Expense Summary",
            commodity_id=commodity_id,
            system_role="expense_summary",
        )
        retained_earnings = await self._ensure_system_account(
            book_id=book_id,
            account_type="equity",
            name="Retained Earnings",
            commodity_id=commodity_id,
            system_role="retained_earnings",
        )
        return _ProfitLossSystemAccounts(
            income_summary_account_id=income_account.id,
            expense_summary_account_id=expense_account.id,
            retained_earnings_account_id=retained_earnings.id,
            commodity_id=commodity_id,
        )

    async def _resolve_base_commodity_id(self, book_id: int) -> int:
        base_currency_code = await self._session.scalar(
            select(Book.base_currency_code).where(Book.id == book_id)
        )
        if base_currency_code is None:
            raise ValueError("book not found")

        commodity_id = await self._session.scalar(
            select(Commodity.id)
            .where(Commodity.book_id == book_id)
            .where(Commodity.symbol == base_currency_code)
            .limit(1)
        )
        if commodity_id is None:
            raise ValueError("base commodity not found")
        return commodity_id

    async def _ensure_system_account(
        self,
        *,
        book_id: int,
        account_type: str,
        name: str,
        commodity_id: int,
        system_role: str,
    ) -> Account:
        account = await self._session.scalar(
            select(Account)
            .where(Account.book_id == book_id)
            .where(Account.system_role == system_role)
            .limit(1)
        )
        if account is not None:
            return account

        account = Account(
            book_id=book_id,
            parent_id=None,
            previous_account_id=None,
            account_type=account_type,
            name=name,
            commodity_id=commodity_id,
            booking_policy="fifo",
            number_last4=None,
            is_closed=False,
            is_hidden=True,
            is_system=True,
            system_role=system_role,
            effective_at=date.today(),
            lifecycle_event="open",
            lifecycle_note=None,
            lifecycle_metadata=None,
            created_at=datetime.now(UTC),
            updated_at=datetime.now(UTC),
        )
        self._session.add(account)
        await self._session.flush()
        return account

    async def _load_profit_loss_balances(
        self,
        book_id: int,
        close_date: date,
    ) -> list[tuple[int, str, int, int]]:
        balance_subquery = (
            select(
                Split.account_id.label("account_id"),
                func.sum(Split.amount_minor).label("balance_minor"),
            )
            .join(Transaction, Transaction.id == Split.tx_id)
            .where(Transaction.book_id == book_id)
            .where(Transaction.occurred_date <= close_date)
            .where(Transaction.status != "void")
            .group_by(Split.account_id)
            .subquery()
        )

        statement: Select[tuple[int, str, int, int]] = (
            select(
                Account.id,
                Account.account_type,
                Account.commodity_id,
                func.coalesce(balance_subquery.c.balance_minor, 0),
            )
            .outerjoin(balance_subquery, balance_subquery.c.account_id == Account.id)
            .where(Account.book_id == book_id)
            .where(Account.account_type.in_(["income", "expense"]))
            .where(Account.is_system.is_(False))
            .where(func.coalesce(balance_subquery.c.balance_minor, 0) != 0)
        )
        result = await self._session.execute(statement)
        return [
            (account_id, account_type, commodity_id, balance_minor)
            for account_id, account_type, commodity_id, balance_minor in result.all()
        ]
