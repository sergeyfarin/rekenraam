from __future__ import annotations

from datetime import date

from sqlalchemy import exists, func, literal, select
from sqlalchemy.ext.asyncio import AsyncSession

from rekenraam_api.db.models.accounts import Account
from rekenraam_api.db.models.planning import (
    Budget,
    BudgetTarget,
    Loan,
    LoanTerm,
    ScheduledTransaction,
    ScheduledTransactionOccurrence,
    ScheduledTransactionSplit,
)
from rekenraam_api.db.models.transactions import Split, Transaction


class PlanningRepository:
    def __init__(self, session: AsyncSession) -> None:
        self._session = session

    @staticmethod
    def _current_transaction_clause():
        newer = Transaction.__table__.alias("newer_transactions")
        return ~exists(select(literal(1)).where(newer.c.previous_tx_id == Transaction.id))

    async def list_budgets(self, book_id: int) -> list[Budget]:
        result = await self._session.execute(
            select(Budget).where(Budget.book_id == book_id).order_by(Budget.id.asc())
        )
        return list(result.scalars().all())

    async def get_budget(self, budget_id: int) -> Budget | None:
        return await self._session.get(Budget, budget_id)

    async def create_budget(self, budget: Budget, targets: list[BudgetTarget]) -> Budget:
        self._session.add(budget)
        await self._session.flush()
        for target in targets:
            target.budget_id = budget.id
            self._session.add(target)
        await self._session.commit()
        await self._session.refresh(budget)
        return budget

    async def update_budget(self, budget: Budget, targets: list[BudgetTarget]) -> Budget:
        existing = await self.list_budget_targets(budget.id)
        for target in existing:
            await self._session.delete(target)
        await self._session.flush()
        for target in targets:
            target.budget_id = budget.id
            self._session.add(target)
        await self._session.commit()
        await self._session.refresh(budget)
        return budget

    async def delete_budget(self, budget_id: int) -> bool:
        budget = await self.get_budget(budget_id)
        if budget is None:
            return False
        await self._session.delete(budget)
        await self._session.commit()
        return True

    async def list_budget_targets(self, budget_id: int) -> list[BudgetTarget]:
        result = await self._session.execute(
            select(BudgetTarget)
            .where(BudgetTarget.budget_id == budget_id)
            .order_by(BudgetTarget.category_id.asc())
        )
        return list(result.scalars().all())

    async def actuals_by_category(
        self, *, book_id: int, category_ids: set[int], start: date, end: date
    ) -> dict[int, int]:
        if not category_ids:
            return {}
        result = await self._session.execute(
            select(Split.category_id, func.coalesce(func.sum(Split.amount_minor), 0))
            .join(Transaction, Transaction.id == Split.tx_id)
            .where(Transaction.book_id == book_id)
            .where(Transaction.status != "void")
            .where(Transaction.occurred_date >= start)
            .where(Transaction.occurred_date <= end)
            .where(Transaction.status != "void")
            .where(Split.category_id.in_(category_ids))
            .where(self._current_transaction_clause())
            .group_by(Split.category_id)
        )
        return {
            int(category_id): int(amount)
            for category_id, amount in result.all()
            if category_id is not None
        }

    async def list_schedules(self, book_id: int) -> list[ScheduledTransaction]:
        result = await self._session.execute(
            select(ScheduledTransaction)
            .where(ScheduledTransaction.book_id == book_id)
            .order_by(ScheduledTransaction.id.asc())
        )
        return list(result.scalars().all())

    async def get_schedule(self, schedule_id: int) -> ScheduledTransaction | None:
        return await self._session.get(ScheduledTransaction, schedule_id)

    async def save_schedule(
        self, schedule: ScheduledTransaction, splits: list[ScheduledTransactionSplit]
    ) -> ScheduledTransaction:
        self._session.add(schedule)
        await self._session.flush()
        existing = await self.list_schedule_splits(schedule.id)
        for split in existing:
            await self._session.delete(split)
        await self._session.flush()
        for split in splits:
            split.scheduled_transaction_id = schedule.id
            self._session.add(split)
        await self._session.commit()
        await self._session.refresh(schedule)
        return schedule

    async def delete_schedule(self, schedule_id: int) -> bool:
        schedule = await self.get_schedule(schedule_id)
        if schedule is None:
            return False
        await self._session.delete(schedule)
        await self._session.commit()
        return True

    async def list_schedule_splits(self, schedule_id: int) -> list[ScheduledTransactionSplit]:
        result = await self._session.execute(
            select(ScheduledTransactionSplit)
            .where(ScheduledTransactionSplit.scheduled_transaction_id == schedule_id)
            .order_by(ScheduledTransactionSplit.id.asc())
        )
        return list(result.scalars().all())

    async def list_schedule_occurrences(
        self, schedule_ids: list[int], start: date, end: date
    ) -> list[ScheduledTransactionOccurrence]:
        if not schedule_ids:
            return []
        result = await self._session.execute(
            select(ScheduledTransactionOccurrence)
            .where(ScheduledTransactionOccurrence.scheduled_transaction_id.in_(schedule_ids))
            .where(ScheduledTransactionOccurrence.occurrence_date >= start)
            .where(ScheduledTransactionOccurrence.occurrence_date <= end)
        )
        return list(result.scalars().all())

    async def get_or_create_occurrence(
        self, schedule_id: int, occurrence_date: date, status: str
    ) -> ScheduledTransactionOccurrence:
        result = await self._session.execute(
            select(ScheduledTransactionOccurrence)
            .where(ScheduledTransactionOccurrence.scheduled_transaction_id == schedule_id)
            .where(ScheduledTransactionOccurrence.occurrence_date == occurrence_date)
        )
        occurrence = result.scalar_one_or_none()
        if occurrence is not None:
            return occurrence
        occurrence = ScheduledTransactionOccurrence(
            scheduled_transaction_id=schedule_id,
            occurrence_date=occurrence_date,
            status=status,
        )
        self._session.add(occurrence)
        await self._session.flush()
        return occurrence

    async def commit(self) -> None:
        await self._session.commit()

    async def account_balances(self, book_id: int, account_types: set[str]) -> dict[int, int]:
        result = await self._session.execute(
            select(Account.id, func.coalesce(func.sum(Split.amount_minor), 0))
            .outerjoin(Split, Split.account_id == Account.id)
            .outerjoin(Transaction, Transaction.id == Split.tx_id)
            .where(Account.book_id == book_id)
            .where(Account.account_type.in_(account_types))
            .where(
                (Transaction.id.is_(None))
                | ((Transaction.status != "void") & self._current_transaction_clause())
            )
            .group_by(Account.id)
        )
        return {int(account_id): int(balance) for account_id, balance in result.all()}

    async def list_accounts_by_ids(self, ids: set[int]) -> list[Account]:
        if not ids:
            return []
        result = await self._session.execute(select(Account).where(Account.id.in_(ids)))
        return list(result.scalars().all())

    async def list_loans(self, book_id: int) -> list[Loan]:
        result = await self._session.execute(
            select(Loan).where(Loan.book_id == book_id).order_by(Loan.id.asc())
        )
        return list(result.scalars().all())

    async def get_loan(self, loan_id: int) -> Loan | None:
        return await self._session.get(Loan, loan_id)

    async def save_loan(self, loan: Loan) -> Loan:
        self._session.add(loan)
        await self._session.flush()
        exists_term = await self._session.scalar(
            select(LoanTerm).where(LoanTerm.loan_id == loan.id).limit(1)
        )
        if exists_term is None:
            self._session.add(
                LoanTerm(
                    loan_id=loan.id,
                    effective_date=loan.start_date,
                    annual_rate_bps=loan.annual_rate_bps,
                )
            )
        await self._session.commit()
        await self._session.refresh(loan)
        return loan

    async def delete_loan(self, loan_id: int) -> bool:
        loan = await self.get_loan(loan_id)
        if loan is None:
            return False
        await self._session.delete(loan)
        await self._session.commit()
        return True
