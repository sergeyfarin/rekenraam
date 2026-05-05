from __future__ import annotations

from calendar import monthrange
from datetime import date, timedelta

from rekenraam_api.db.models.planning import (
    Budget,
    BudgetTarget,
    Loan,
    ScheduledTransaction,
    ScheduledTransactionSplit,
)
from rekenraam_api.repositories.planning import PlanningRepository
from rekenraam_api.schemas.planning import (
    AmortizationRow,
    BudgetMutationInput,
    BudgetSummary,
    BudgetTargetSummary,
    BudgetVarianceRow,
    LoanMutationInput,
    LoanPaymentDraft,
    LoanPaymentDraftInput,
    LoanSummary,
    PostedScheduleResult,
    ProjectedCashRow,
    ScheduleMutationInput,
    ScheduleOccurrenceSummary,
    ScheduleSplitSummary,
    ScheduleSummary,
)
from rekenraam_api.schemas.transactions import TransactionMutationInput, TransactionSplitInput
from rekenraam_api.services.access import AccessPolicy
from rekenraam_api.services.transactions import TransactionService

_CASH_ACCOUNT_TYPES = {"cash", "checking", "savings", "credit", "liability", "loan"}


def _add_months(value: date, months: int) -> date:
    month_index = value.month - 1 + months
    year = value.year + month_index // 12
    month = month_index % 12 + 1
    return date(year, month, min(value.day, monthrange(year, month)[1]))


def _period_end(start: date, period: str) -> date:
    if period == "annual":
        return date(start.year, 12, 31)
    return date(start.year, start.month, monthrange(start.year, start.month)[1])


class PlanningService:
    def __init__(
        self,
        repository: PlanningRepository,
        transaction_service: TransactionService,
        access_policy: AccessPolicy | None = None,
    ) -> None:
        self._repository = repository
        self._transaction_service = transaction_service
        self._access_policy = access_policy

    async def list_budgets(self, book_id: int) -> list[BudgetSummary]:
        await self._require_read(book_id)
        budgets = await self._repository.list_budgets(book_id)
        return [await self._map_budget(budget) for budget in budgets]

    async def get_budget(self, budget_id: int) -> BudgetSummary | None:
        budget = await self._repository.get_budget(budget_id)
        if budget is None:
            return None
        await self._require_read(budget.book_id)
        return await self._map_budget(budget)

    async def create_budget(self, input: BudgetMutationInput) -> BudgetSummary:
        await self._require_write(input.book_id)
        self._validate_budget_input(input)
        budget = Budget(
            book_id=input.book_id,
            name=input.name,
            period=input.period,
            starts_on=input.starts_on,
            ends_on=input.ends_on,
            commodity_id=input.commodity_id,
            is_active=input.is_active,
        )
        targets = [
            BudgetTarget(
                budget_id=0,
                book_id=input.book_id,
                category_id=target.category_id,
                amount_minor=target.amount_minor,
                rollover_enabled=target.rollover_enabled,
            )
            for target in input.targets
        ]
        return await self._map_budget(await self._repository.create_budget(budget, targets))

    async def update_budget(
        self, budget_id: int, input: BudgetMutationInput
    ) -> BudgetSummary | None:
        budget = await self._repository.get_budget(budget_id)
        if budget is None:
            return None
        await self._require_write(budget.book_id)
        if budget.book_id != input.book_id:
            raise ValueError("budget book cannot be changed")
        self._validate_budget_input(input)
        budget.name = input.name
        budget.period = input.period
        budget.starts_on = input.starts_on
        budget.ends_on = input.ends_on
        budget.commodity_id = input.commodity_id
        budget.is_active = input.is_active
        targets = [
            BudgetTarget(
                budget_id=budget.id,
                book_id=input.book_id,
                category_id=target.category_id,
                amount_minor=target.amount_minor,
                rollover_enabled=target.rollover_enabled,
            )
            for target in input.targets
        ]
        return await self._map_budget(await self._repository.update_budget(budget, targets))

    async def delete_budget(self, budget_id: int) -> bool:
        budget = await self._repository.get_budget(budget_id)
        if budget is None:
            return False
        await self._require_write(budget.book_id)
        return await self._repository.delete_budget(budget_id)

    async def budget_variance(self, budget_id: int, period_start: date) -> list[BudgetVarianceRow]:
        budget = await self._repository.get_budget(budget_id)
        if budget is None:
            raise ValueError("budget not found")
        await self._require_read(budget.book_id)
        period_start = (
            date(period_start.year, 1, 1)
            if budget.period == "annual"
            else date(period_start.year, period_start.month, 1)
        )
        period_end = _period_end(period_start, budget.period)
        targets = await self._repository.list_budget_targets(budget.id)
        category_ids = {target.category_id for target in targets}
        actuals = await self._repository.actuals_by_category(
            book_id=budget.book_id, category_ids=category_ids, start=period_start, end=period_end
        )
        rows: list[BudgetVarianceRow] = []
        for target in targets:
            rollover = 0
            if target.rollover_enabled and period_start > budget.starts_on:
                prior_start = budget.starts_on
                prior_end = period_start - timedelta(days=1)
                prior_actuals = await self._repository.actuals_by_category(
                    book_id=budget.book_id,
                    category_ids={target.category_id},
                    start=prior_start,
                    end=prior_end,
                )
                periods = max(1, self._period_count(budget.period, prior_start, prior_end))
                rollover = target.amount_minor * periods - prior_actuals.get(target.category_id, 0)
            actual = actuals.get(target.category_id, 0)
            rows.append(
                BudgetVarianceRow(
                    category_id=target.category_id,
                    target_minor=target.amount_minor,
                    actual_minor=actual,
                    rollover_minor=rollover,
                    remaining_minor=target.amount_minor + rollover - actual,
                )
            )
        return rows

    async def list_schedules(self, book_id: int) -> list[ScheduleSummary]:
        await self._require_read(book_id)
        schedules = await self._repository.list_schedules(book_id)
        return [await self._map_schedule(schedule) for schedule in schedules]

    async def get_schedule(self, schedule_id: int) -> ScheduleSummary | None:
        schedule = await self._repository.get_schedule(schedule_id)
        if schedule is None:
            return None
        await self._require_read(schedule.book_id)
        return await self._map_schedule(schedule)

    async def create_schedule(self, input: ScheduleMutationInput) -> ScheduleSummary:
        await self._require_write(input.book_id)
        self._validate_schedule_input(input)
        schedule = ScheduledTransaction(**input.model_dump(exclude={"splits"}))
        splits = [
            ScheduledTransactionSplit(scheduled_transaction_id=0, **split.model_dump())
            for split in input.splits
        ]
        return await self._map_schedule(await self._repository.save_schedule(schedule, splits))

    async def update_schedule(
        self, schedule_id: int, input: ScheduleMutationInput
    ) -> ScheduleSummary | None:
        schedule = await self._repository.get_schedule(schedule_id)
        if schedule is None:
            return None
        await self._require_write(schedule.book_id)
        if schedule.book_id != input.book_id:
            raise ValueError("schedule book cannot be changed")
        self._validate_schedule_input(input)
        for field, value in input.model_dump(exclude={"splits"}).items():
            setattr(schedule, field, value)
        splits = [
            ScheduledTransactionSplit(scheduled_transaction_id=schedule.id, **split.model_dump())
            for split in input.splits
        ]
        return await self._map_schedule(await self._repository.save_schedule(schedule, splits))

    async def delete_schedule(self, schedule_id: int) -> bool:
        schedule = await self._repository.get_schedule(schedule_id)
        if schedule is None:
            return False
        await self._require_write(schedule.book_id)
        return await self._repository.delete_schedule(schedule_id)

    async def projected_instances(
        self, book_id: int, start: date, end: date
    ) -> list[ScheduleOccurrenceSummary]:
        await self._require_read(book_id)
        schedules = await self._repository.list_schedules(book_id)
        occurrences = await self._repository.list_schedule_occurrences(
            [schedule.id for schedule in schedules], start, end
        )
        overrides = {
            (occurrence.scheduled_transaction_id, occurrence.occurrence_date): occurrence
            for occurrence in occurrences
        }
        rows: list[ScheduleOccurrenceSummary] = []
        for schedule in schedules:
            if not schedule.enabled:
                continue
            for occurrence_date in self._generate_dates(schedule, start, end):
                override = overrides.get((schedule.id, occurrence_date))
                rows.append(
                    ScheduleOccurrenceSummary(
                        schedule_id=schedule.id,
                        occurrence_date=occurrence_date,
                        status=override.status if override is not None else "pending",
                        posted_transaction_id=override.posted_transaction_id
                        if override is not None
                        else None,
                    )
                )
        return sorted(rows, key=lambda row: (row.occurrence_date, row.schedule_id))

    async def skip_schedule(
        self, schedule_id: int, occurrence_date: date
    ) -> ScheduleOccurrenceSummary:
        schedule = await self._repository.get_schedule(schedule_id)
        if schedule is None:
            raise ValueError("schedule not found")
        await self._require_write(schedule.book_id)
        occurrence = await self._repository.get_or_create_occurrence(
            schedule_id, occurrence_date, "skipped"
        )
        if occurrence.status == "posted":
            raise ValueError("posted occurrences cannot be skipped")
        occurrence.status = "skipped"
        await self._repository.commit()
        return ScheduleOccurrenceSummary(
            schedule_id=schedule_id, occurrence_date=occurrence_date, status="skipped"
        )

    async def post_schedule(self, schedule_id: int, occurrence_date: date) -> PostedScheduleResult:
        schedule = await self._repository.get_schedule(schedule_id)
        if schedule is None:
            raise ValueError("schedule not found")
        await self._require_write(schedule.book_id)
        occurrence = await self._repository.get_or_create_occurrence(
            schedule_id, occurrence_date, "pending"
        )
        if occurrence.status == "posted":
            raise ValueError("occurrence already posted")
        if occurrence.status == "skipped":
            raise ValueError("skipped occurrence cannot be posted")
        splits = await self._repository.list_schedule_splits(schedule.id)
        transaction = await self._transaction_service.create_transaction(
            TransactionMutationInput(
                book_id=schedule.book_id,
                txn_date=occurrence_date,
                payee_id=schedule.payee_id,
                memo=schedule.memo,
                status=schedule.status,
                reference=schedule.reference,
                import_id=None,
                splits=tuple(
                    TransactionSplitInput(
                        account_id=split.account_id,
                        commodity_id=split.commodity_id,
                        amount_minor=split.amount_minor,
                        category_id=split.category_id,
                        tag_id=None,
                        person_id=None,
                        project_id=None,
                        share_bps=None,
                        memo=split.memo,
                    )
                    for split in splits
                ),
            )
        )
        occurrence.status = "posted"
        occurrence.posted_transaction_id = transaction.id
        await self._repository.commit()
        return PostedScheduleResult(
            occurrence=ScheduleOccurrenceSummary(
                schedule_id=schedule.id,
                occurrence_date=occurrence_date,
                status="posted",
                posted_transaction_id=transaction.id,
            ),
            transaction=transaction,
        )

    async def projected_cash(self, book_id: int, start: date, end: date) -> list[ProjectedCashRow]:
        await self._require_read(book_id)
        balances = await self._repository.account_balances(book_id, _CASH_ACCOUNT_TYPES)
        schedules = await self._repository.list_schedules(book_id)
        schedule_splits: dict[int, list[ScheduledTransactionSplit]] = {}
        for schedule in schedules:
            schedule_splits[schedule.id] = await self._repository.list_schedule_splits(schedule.id)
        occurrences = await self.projected_instances(book_id, start, end)
        rows: list[ProjectedCashRow] = []
        running = dict(balances)
        for occurrence in occurrences:
            if occurrence.status != "pending":
                continue
            deltas: dict[int, int] = {}
            for split in schedule_splits.get(occurrence.schedule_id, []):
                if split.account_id not in running:
                    continue
                deltas[split.account_id] = deltas.get(split.account_id, 0) + split.amount_minor
            for account_id, delta in deltas.items():
                running[account_id] += delta
                rows.append(
                    ProjectedCashRow(
                        account_id=account_id,
                        projection_date=occurrence.occurrence_date,
                        scheduled_delta_minor=delta,
                        projected_balance_minor=running[account_id],
                    )
                )
        return rows

    async def list_loans(self, book_id: int) -> list[LoanSummary]:
        await self._require_read(book_id)
        return [self._map_loan(loan) for loan in await self._repository.list_loans(book_id)]

    async def get_loan(self, loan_id: int) -> LoanSummary | None:
        loan = await self._repository.get_loan(loan_id)
        if loan is None:
            return None
        await self._require_read(loan.book_id)
        return self._map_loan(loan)

    async def create_loan(self, input: LoanMutationInput) -> LoanSummary:
        await self._require_write(input.book_id)
        self._validate_loan_input(input)
        loan = Loan(**input.model_dump())
        return self._map_loan(await self._repository.save_loan(loan))

    async def update_loan(self, loan_id: int, input: LoanMutationInput) -> LoanSummary | None:
        loan = await self._repository.get_loan(loan_id)
        if loan is None:
            return None
        await self._require_write(loan.book_id)
        if loan.book_id != input.book_id:
            raise ValueError("loan book cannot be changed")
        self._validate_loan_input(input)
        for field, value in input.model_dump().items():
            setattr(loan, field, value)
        return self._map_loan(await self._repository.save_loan(loan))

    async def delete_loan(self, loan_id: int) -> bool:
        loan = await self._repository.get_loan(loan_id)
        if loan is None:
            return False
        await self._require_write(loan.book_id)
        return await self._repository.delete_loan(loan_id)

    async def amortization(self, loan_id: int) -> list[AmortizationRow]:
        loan = await self._repository.get_loan(loan_id)
        if loan is None:
            raise ValueError("loan not found")
        await self._require_read(loan.book_id)
        return self._amortization_rows(loan)

    async def loan_payment_draft(
        self, loan_id: int, input: LoanPaymentDraftInput
    ) -> LoanPaymentDraft:
        loan = await self._repository.get_loan(loan_id)
        if loan is None:
            raise ValueError("loan not found")
        await self._require_read(loan.book_id)
        payment_account_id = input.payment_account_id or loan.payment_account_id
        if payment_account_id is None:
            raise ValueError("payment account is required")
        row = next(
            (
                item
                for item in self._amortization_rows(loan)
                if item.payment_date >= input.payment_date
            ),
            None,
        )
        if row is None:
            raise ValueError("loan is already paid off")
        return LoanPaymentDraft(
            transaction=TransactionMutationInput(
                book_id=loan.book_id,
                txn_date=input.payment_date,
                payee_id=None,
                memo=input.memo or f"{loan.name} payment",
                status="uncleared",
                reference=None,
                import_id=None,
                splits=(
                    TransactionSplitInput(
                        account_id=payment_account_id,
                        commodity_id=1,
                        amount_minor=-(row.principal_minor + row.interest_minor),
                        category_id=None,
                        tag_id=None,
                        person_id=None,
                        project_id=None,
                        share_bps=None,
                        memo="Payment",
                    ),
                    TransactionSplitInput(
                        account_id=loan.account_id,
                        commodity_id=1,
                        amount_minor=row.principal_minor,
                        category_id=None,
                        tag_id=None,
                        person_id=None,
                        project_id=None,
                        share_bps=None,
                        memo="Principal",
                    ),
                    TransactionSplitInput(
                        account_id=payment_account_id,
                        commodity_id=1,
                        amount_minor=row.interest_minor,
                        category_id=input.interest_category_id or loan.interest_category_id,
                        tag_id=None,
                        person_id=None,
                        project_id=None,
                        share_bps=None,
                        memo="Interest",
                    ),
                ),
            )
        )

    async def post_loan_payment(self, loan_id: int, input: LoanPaymentDraftInput):
        await self._require_write((await self._loan_required(loan_id)).book_id)
        draft = await self.loan_payment_draft(loan_id, input)
        return await self._transaction_service.create_transaction(draft.transaction)

    async def _loan_required(self, loan_id: int) -> Loan:
        loan = await self._repository.get_loan(loan_id)
        if loan is None:
            raise ValueError("loan not found")
        return loan

    async def _map_budget(self, budget: Budget) -> BudgetSummary:
        targets = await self._repository.list_budget_targets(budget.id)
        return BudgetSummary(
            id=budget.id,
            book_id=budget.book_id,
            name=budget.name,
            period=budget.period,
            starts_on=budget.starts_on,
            ends_on=budget.ends_on,
            commodity_id=budget.commodity_id,
            is_active=budget.is_active,
            created_at=budget.created_at,
            targets=tuple(
                BudgetTargetSummary(
                    id=target.id,
                    budget_id=target.budget_id,
                    category_id=target.category_id,
                    amount_minor=target.amount_minor,
                    rollover_enabled=target.rollover_enabled,
                )
                for target in targets
            ),
        )

    async def _map_schedule(self, schedule: ScheduledTransaction) -> ScheduleSummary:
        splits = await self._repository.list_schedule_splits(schedule.id)
        return ScheduleSummary(
            id=schedule.id,
            book_id=schedule.book_id,
            name=schedule.name,
            payee_id=schedule.payee_id,
            memo=schedule.memo,
            status=schedule.status,
            reference=schedule.reference,
            frequency=schedule.frequency,
            interval=schedule.interval,
            start_date=schedule.start_date,
            end_date=schedule.end_date,
            reminder_days=schedule.reminder_days,
            enabled=schedule.enabled,
            splits=tuple(
                ScheduleSplitSummary(
                    id=split.id,
                    account_id=split.account_id,
                    commodity_id=split.commodity_id,
                    amount_minor=split.amount_minor,
                    category_id=split.category_id,
                    memo=split.memo,
                )
                for split in splits
            ),
        )

    def _map_loan(self, loan: Loan) -> LoanSummary:
        return LoanSummary(
            id=loan.id,
            book_id=loan.book_id,
            account_id=loan.account_id,
            name=loan.name,
            principal_minor=loan.principal_minor,
            annual_rate_bps=loan.annual_rate_bps,
            term_months=loan.term_months,
            payment_frequency=loan.payment_frequency,
            start_date=loan.start_date,
            payment_account_id=loan.payment_account_id,
            interest_category_id=loan.interest_category_id,
            payment_minor=self._payment_minor(loan),
        )

    def _generate_dates(
        self, schedule: ScheduledTransaction, window_start: date, window_end: date
    ) -> list[date]:
        current = schedule.start_date
        rows: list[date] = []
        while current <= window_end:
            if current >= window_start and (
                schedule.end_date is None or current <= schedule.end_date
            ):
                rows.append(current)
            if schedule.frequency == "daily":
                current += timedelta(days=schedule.interval)
            elif schedule.frequency == "weekly":
                current += timedelta(weeks=schedule.interval)
            elif schedule.frequency == "monthly":
                current = _add_months(current, schedule.interval)
            else:
                current = _add_months(current, schedule.interval * 12)
        return rows

    def _amortization_rows(self, loan: Loan) -> list[AmortizationRow]:
        payment = self._payment_minor(loan)
        balance = loan.principal_minor
        rows: list[AmortizationRow] = []
        monthly_rate = loan.annual_rate_bps / 10000 / 12
        for period in range(1, loan.term_months + 1):
            interest = round(balance * monthly_rate)
            principal = min(balance, max(payment - interest, 0))
            if period == loan.term_months:
                principal = balance
                payment = principal + interest
            balance -= principal
            rows.append(
                AmortizationRow(
                    period=period,
                    payment_date=_add_months(loan.start_date, period),
                    payment_minor=payment,
                    principal_minor=principal,
                    interest_minor=interest,
                    remaining_principal_minor=balance,
                )
            )
            if balance <= 0:
                break
        return rows

    def _payment_minor(self, loan: Loan) -> int:
        if loan.term_months <= 0:
            return 0
        monthly_rate = loan.annual_rate_bps / 10000 / 12
        if monthly_rate == 0:
            return loan.principal_minor // loan.term_months
        payment = (
            loan.principal_minor * monthly_rate / (1 - (1 + monthly_rate) ** -loan.term_months)
        )
        return round(payment)

    def _period_count(self, period: str, start: date, end: date) -> int:
        if period == "annual":
            return end.year - start.year + 1
        return (end.year - start.year) * 12 + end.month - start.month + 1

    def _validate_budget_input(self, input: BudgetMutationInput) -> None:
        if input.period not in {"monthly", "annual"}:
            raise ValueError("period must be monthly or annual")
        if input.ends_on is not None and input.ends_on < input.starts_on:
            raise ValueError("ends_on must be on or after starts_on")

    def _validate_schedule_input(self, input: ScheduleMutationInput) -> None:
        if input.frequency not in {"daily", "weekly", "monthly", "yearly"}:
            raise ValueError("frequency must be daily, weekly, monthly, or yearly")
        if sum(split.amount_minor for split in input.splits) != 0:
            raise ValueError("scheduled transaction splits must balance")

    def _validate_loan_input(self, input: LoanMutationInput) -> None:
        if input.principal_minor <= 0:
            raise ValueError("principal_minor must be positive")
        if input.term_months <= 0:
            raise ValueError("term_months must be positive")
        if input.annual_rate_bps < 0:
            raise ValueError("annual_rate_bps cannot be negative")
        if input.payment_frequency != "monthly":
            raise ValueError("only monthly loan payments are supported")

    async def _require_read(self, book_id: int) -> None:
        if self._access_policy is not None:
            await self._access_policy.require_book_read(book_id)

    async def _require_write(self, book_id: int) -> None:
        if self._access_policy is not None:
            await self._access_policy.require_book_write(book_id)
