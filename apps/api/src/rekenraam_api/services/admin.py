from __future__ import annotations

from dataclasses import dataclass
from datetime import UTC, date, datetime

from sqlalchemy import Select, func, select, text
from sqlalchemy.ext.asyncio import AsyncSession

from rekenraam_api.config.settings import Settings
from rekenraam_api.db.models.accounts import Account
from rekenraam_api.db.models.books import Book
from rekenraam_api.db.models.metadata import Commodity
from rekenraam_api.db.models.transactions import Split, Transaction
from rekenraam_api.schemas.admin import (
    AdminRuntimeStatusSummary,
    FiscalYearCloseInput,
    FiscalYearCloseResult,
)
from rekenraam_api.services.report_invalidation import bump_report_state

LATEST_MIGRATION_VERSION = "0001_initial_schema"


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
        database_name = await self._session.scalar(select(func.current_database()))
        size_bytes = await self._session.scalar(select(func.pg_database_size(func.current_database())))

        current_version: str | None = None
        try:
            current_version = await self._session.scalar(text("SELECT version_num FROM alembic_version LIMIT 1"))
        except Exception:
            current_version = None

        pending_versions: tuple[str, ...]
        if current_version in {None, LATEST_MIGRATION_VERSION}:
            pending_versions = ()
        else:
            pending_versions = (LATEST_MIGRATION_VERSION,)

        display_path = f"{self._settings.postgres_host}:{self._settings.postgres_port}/{database_name or self._settings.postgres_db}"
        return AdminRuntimeStatusSummary(
            database_kind="postgresql",
            database_name=database_name or self._settings.postgres_db,
            database_host=self._settings.postgres_host,
            display_path=display_path,
            size_bytes=size_bytes,
            writable=True,
            foreign_keys=True,
            current_version=current_version,
            latest_version=LATEST_MIGRATION_VERSION,
            pending_versions=pending_versions,
            note=(
                "The web runtime uses a server-managed PostgreSQL database. Desktop file pickers,"
                " path switching, and local backup folder selection do not apply in this deployment."
            ),
        )

    async def run_integrity_check(self) -> str:
        await self._session.execute(text("SELECT 1"))
        return "ok"

    async def close_fiscal_year(self, input: FiscalYearCloseInput) -> FiscalYearCloseResult:
        today = date.today()
        if input.close_date > today:
            raise ValueError("close_date cannot be in the future")

        book_id = 1
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
            splits.append((system_accounts.retained_earnings_account_id, retained_earnings_delta_minor))

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
            select(Split.account_id.label("account_id"), func.sum(Split.amount_minor).label("balance_minor"))
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
        return [(account_id, account_type, commodity_id, balance_minor) for account_id, account_type, commodity_id, balance_minor in result.all()]