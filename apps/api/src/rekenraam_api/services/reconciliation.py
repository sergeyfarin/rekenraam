from __future__ import annotations

from datetime import timedelta

from rekenraam_api.db.models.accounts import (
    Account,
    AccountBalancing,
    BalanceAdjustment,
    BalanceCheck,
    BalanceConstraint,
)
from rekenraam_api.db.models.metadata import Commodity
from rekenraam_api.db.models.transactions import Split, Transaction
from rekenraam_api.repositories.reconciliation import ReconciliationRepository
from rekenraam_api.schemas.accounts import AccountBalancingSummary, AccountSummary
from rekenraam_api.schemas.reconciliation import (
    BalanceAdjustmentSummary,
    BalanceCheckSummary,
    BalanceConstraintCreateInput,
    BalanceConstraintSummary,
    BalanceConstraintValidation,
    ReconciliationBalancingSummary,
    ReconciliationCommoditySummary,
    ReconciliationFinishInput,
    ReconciliationFinishResult,
    ReconciliationHistoryResponse,
    ReconciliationStartResponse,
)
from rekenraam_api.schemas.transactions import SplitEntry, TransactionSummary
from rekenraam_api.services.access import AccessPolicy
from rekenraam_api.services.report_invalidation import bump_report_state


class ReconciliationService:
    def __init__(
        self, repository: ReconciliationRepository, access_policy: AccessPolicy | None = None
    ) -> None:
        self._repository = repository
        self._access_policy = access_policy

    async def start(
        self, account_id: int, as_of_date, statement_start_date=None
    ) -> ReconciliationStartResponse | None:
        account = await self._repository.get_account(account_id)
        if account is None:
            return None
        if self._access_policy is not None:
            await self._access_policy.require_book_read(account.book_id)
        commodity = await self._repository.get_commodity(account.commodity_id)
        if commodity is None:
            raise ValueError("account commodity not found")

        last_balancing = await self._repository.get_last_active_balancing(account_id)
        effective_start_date = statement_start_date
        if effective_start_date is None and last_balancing is not None:
            effective_start_date = last_balancing.as_of_date + timedelta(days=1)
        if (
            last_balancing is not None
            and effective_start_date is not None
            and effective_start_date <= last_balancing.as_of_date
        ):
            raise ValueError(
                "statement_start_date is locked; unlock the affected reconciliation period first"
            )

        candidates = await self._repository.list_candidates(
            account_id, as_of_date, effective_start_date
        )
        splits = await self._repository.list_splits_for_transaction_ids(
            [transaction.id for transaction in candidates]
        )
        remembered_offset_account_id = None
        if self._access_policy is not None:
            remembered_offset_account_id = await self._repository.get_remembered_offset_account_id(
                self._access_policy.audit_stamp().created_by_user_id or 0,
                account_id,
            )
        return ReconciliationStartResponse(
            account=self._account_summary(account),
            commodity=self._commodity_summary(commodity),
            last_balancing=self._account_balancing_summary(last_balancing),
            statement_start_date=effective_start_date,
            statement_end_date=as_of_date,
            remembered_offset_account_id=remembered_offset_account_id,
            candidates=tuple(self._map_transactions(candidates, splits)),
        )

    async def finish(
        self, account_id: int, input: ReconciliationFinishInput
    ) -> ReconciliationFinishResult | None:
        account = await self._repository.get_account(account_id)
        if account is None:
            return None
        if account.book_id != input.book_id:
            raise ValueError("account does not belong to book")
        if self._access_policy is not None:
            await self._access_policy.require_book_write(account.book_id)
        audit = self._access_policy.audit_stamp() if self._access_policy is not None else None

        last_balancing = await self._repository.get_last_active_balancing(account_id)
        if last_balancing is not None and input.as_of_date <= last_balancing.as_of_date:
            raise ValueError("balancing date must be after last active balancing")
        effective_start_date = input.statement_start_date
        if effective_start_date is None and last_balancing is not None:
            effective_start_date = last_balancing.as_of_date + timedelta(days=1)
        if (
            last_balancing is not None
            and effective_start_date is not None
            and effective_start_date <= last_balancing.as_of_date
        ):
            raise ValueError(
                "statement_start_date is locked; unlock the affected reconciliation period first"
            )
        if effective_start_date is not None and effective_start_date > input.as_of_date:
            raise ValueError("statement_start_date must be on or before as_of_date")

        candidates = await self._repository.list_candidates(
            account_id, input.as_of_date, effective_start_date
        )
        candidate_by_id = {transaction.id: transaction for transaction in candidates}
        selected_ids = set(input.selected_transaction_ids)
        if unknown_ids := selected_ids - set(candidate_by_id):
            raise ValueError(f"selected transactions are not eligible: {sorted(unknown_ids)}")

        splits = await self._repository.list_splits_for_transaction_ids(
            [transaction.id for transaction in candidates]
        )
        account_chain_ids = set(await self._repository.list_account_chain_ids(account_id))
        selected_amount_minor = sum(
            split.amount_minor
            for split in splits
            if split.tx_id in selected_ids and split.account_id in account_chain_ids
        )
        opening_balance_minor = last_balancing.balance_minor if last_balancing is not None else 0
        cleared_balance_minor = opening_balance_minor + selected_amount_minor
        difference_minor = cleared_balance_minor - input.statement_balance_minor

        offset_account: Account | None = None
        if difference_minor != 0:
            if not input.confirm_unbalanced:
                raise ValueError("unbalanced reconciliation must be confirmed")
            if input.offset_account_id is None:
                raise ValueError("offset_account_id is required when reconciliation is unbalanced")
            offset_account = await self._repository.get_account(input.offset_account_id)
            if offset_account is None:
                raise ValueError("offset account not found")
            if offset_account.book_id != account.book_id:
                raise ValueError("offset account does not belong to book")
            if offset_account.commodity_id != account.commodity_id:
                raise ValueError("account and offset account must share the same commodity")
            if offset_account.is_closed:
                raise ValueError("offset account is closed")

        reconciled_count = 0
        uncleared_count = 0
        for transaction in candidates:
            if transaction.id in selected_ids and transaction.status != "reconciled":
                await self._repository.append_transaction_status(
                    transaction=transaction,
                    status="reconciled",
                    created_by_user_id=audit.created_by_user_id if audit is not None else None,
                    created_session_id=audit.created_session_id if audit is not None else None,
                    created_device_id=audit.created_device_id if audit is not None else None,
                    created_request_id=audit.created_request_id if audit is not None else None,
                )
                reconciled_count += 1
            elif input.downgrade_unselected_cleared and transaction.status == "cleared":
                await self._repository.append_transaction_status(
                    transaction=transaction,
                    status="uncleared",
                    created_by_user_id=audit.created_by_user_id if audit is not None else None,
                    created_session_id=audit.created_session_id if audit is not None else None,
                    created_device_id=audit.created_device_id if audit is not None else None,
                    created_request_id=audit.created_request_id if audit is not None else None,
                )
                uncleared_count += 1

        check = await self._repository.create_balance_check(
            book_id=input.book_id,
            account_id=account_id,
            as_of_date=input.as_of_date,
            balance_minor=input.statement_balance_minor,
            memo=input.memo,
            status="matched" if difference_minor == 0 else "unbalanced",
            created_by_user_id=audit.created_by_user_id if audit is not None else None,
            created_session_id=audit.created_session_id if audit is not None else None,
            created_device_id=audit.created_device_id if audit is not None else None,
            created_request_id=audit.created_request_id if audit is not None else None,
        )

        adjustment = None
        if difference_minor != 0 and offset_account is not None:
            adjustment_minor = -difference_minor
            adjustment_tx = await self._repository.create_adjustment_transaction(
                book_id=input.book_id,
                account_id=account_id,
                account_commodity_id=account.commodity_id,
                offset_account_id=offset_account.id,
                as_of_date=input.as_of_date,
                adjustment_minor=adjustment_minor,
                memo=input.memo,
                created_by_user_id=audit.created_by_user_id if audit is not None else None,
                created_session_id=audit.created_session_id if audit is not None else None,
                created_device_id=audit.created_device_id if audit is not None else None,
                created_request_id=audit.created_request_id if audit is not None else None,
            )
            adjustment = await self._repository.create_balance_adjustment(
                book_id=input.book_id,
                balance_check_id=check.id,
                account_id=account_id,
                offset_account_id=offset_account.id,
                as_of_date=input.as_of_date,
                target_balance_minor=input.statement_balance_minor,
                actual_balance_minor=cleared_balance_minor,
                adjustment_minor=adjustment_minor,
                tx_id=adjustment_tx.id,
                memo=input.memo,
                created_by_user_id=audit.created_by_user_id if audit is not None else None,
                created_session_id=audit.created_session_id if audit is not None else None,
                created_device_id=audit.created_device_id if audit is not None else None,
                created_request_id=audit.created_request_id if audit is not None else None,
            )
            await self._repository.update_balance_check_status(check, "balanced")
            if input.remember_offset_account and audit is not None:
                await self._repository.remember_offset_account(
                    user_id=audit.created_by_user_id or 0,
                    book_id=input.book_id,
                    account_id=account_id,
                    offset_account_id=offset_account.id,
                )

        balancing = await self._repository.create_account_balancing(
            book_id=input.book_id,
            account_id=account_id,
            as_of_date=input.as_of_date,
            balance_minor=input.statement_balance_minor,
            memo=input.memo,
            created_by_user_id=audit.created_by_user_id if audit is not None else None,
            created_session_id=audit.created_session_id if audit is not None else None,
            created_device_id=audit.created_device_id if audit is not None else None,
            created_request_id=audit.created_request_id if audit is not None else None,
        )

        session = getattr(self._repository, "_session", None)
        if session is not None:
            await session.commit()
        await bump_report_state(getattr(self._repository, "_session", None), input.book_id)

        balancing_summary = self._account_balancing_summary(balancing)
        if balancing_summary is None:
            raise ValueError("balancing not found after create")

        return ReconciliationFinishResult(
            balance_check=self._balance_check_summary(check),
            balancing=balancing_summary,
            adjustment=self._balance_adjustment_summary(adjustment)
            if adjustment is not None
            else None,
            difference_minor=difference_minor,
            reconciled_count=reconciled_count,
            uncleared_count=uncleared_count,
        )

    async def history(self, account_id: int) -> ReconciliationHistoryResponse | None:
        account = await self._repository.get_account(account_id)
        if account is None:
            return None
        if self._access_policy is not None:
            await self._access_policy.require_book_read(account.book_id)
        return ReconciliationHistoryResponse(
            balancings=tuple(
                self._reconciliation_balancing_summary(row)
                for row in await self._repository.list_balancing_history(account_id)
            ),
            balance_checks=tuple(
                self._balance_check_summary(row)
                for row in await self._repository.list_balance_checks(account_id)
            ),
            balance_adjustments=tuple(
                self._balance_adjustment_summary(row)
                for row in await self._repository.list_balance_adjustments(account_id)
            ),
        )

    async def unlock(
        self, account_id: int, from_date, reason: str | None, confirm: bool
    ) -> int | None:
        if not confirm:
            raise ValueError("unlock not confirmed")
        account = await self._repository.get_account(account_id)
        if account is None:
            return None
        if self._access_policy is not None:
            await self._access_policy.require_book_write(account.book_id)
        count = await self._repository.unlock_balancings(account_id, from_date, reason)
        session = getattr(self._repository, "_session", None)
        if session is not None:
            await session.commit()
        if count:
            await bump_report_state(getattr(self._repository, "_session", None), account.book_id)
        return count

    async def list_constraints(
        self, account_id: int
    ) -> tuple[BalanceConstraintSummary, ...] | None:
        account = await self._repository.get_account(account_id)
        if account is None:
            return None
        if self._access_policy is not None:
            await self._access_policy.require_book_read(account.book_id)
        return tuple(
            self._constraint_summary(row)
            for row in await self._repository.list_constraints(account_id)
        )

    async def create_constraint(
        self, account_id: int, input: BalanceConstraintCreateInput
    ) -> BalanceConstraintSummary | None:
        account = await self._repository.get_account(account_id)
        if account is None:
            return None
        if input.book_id != account.book_id:
            raise ValueError("account does not belong to book")
        if self._access_policy is not None:
            await self._access_policy.require_book_write(account.book_id)
        sign_rule = input.sign_rule.strip().lower()
        if sign_rule not in {"any", "nonnegative", "nonpositive"}:
            raise ValueError("sign_rule must be any, nonnegative, or nonpositive")
        if (
            input.min_balance_minor is not None
            and input.max_balance_minor is not None
            and input.min_balance_minor > input.max_balance_minor
        ):
            raise ValueError("min_balance_minor must be <= max_balance_minor")
        constraint = await self._repository.create_constraint(
            book_id=input.book_id,
            account_id=account_id,
            min_balance_minor=input.min_balance_minor,
            max_balance_minor=input.max_balance_minor,
            sign_rule=sign_rule,
        )
        session = getattr(self._repository, "_session", None)
        if session is not None:
            await session.commit()
        await bump_report_state(getattr(self._repository, "_session", None), account.book_id)
        return self._constraint_summary(constraint)

    async def delete_constraint(self, account_id: int, constraint_id: int) -> bool | None:
        account = await self._repository.get_account(account_id)
        if account is None:
            return None
        if self._access_policy is not None:
            await self._access_policy.require_book_write(account.book_id)
        deleted = await self._repository.delete_constraint(constraint_id, account_id)
        session = getattr(self._repository, "_session", None)
        if session is not None:
            await session.commit()
        if deleted:
            await bump_report_state(getattr(self._repository, "_session", None), account.book_id)
        return deleted

    async def validate_constraints(self, account_id: int) -> BalanceConstraintValidation | None:
        account = await self._repository.get_account(account_id)
        if account is None:
            return None
        if self._access_policy is not None:
            await self._access_policy.require_book_read(account.book_id)
        balance = await self._repository.current_balance_minor(account_id)
        issues: list[str] = []
        for constraint in await self._repository.list_constraints(account_id):
            if constraint.min_balance_minor is not None and balance < constraint.min_balance_minor:
                issues.append("balance constraint violated: below minimum")
            if constraint.max_balance_minor is not None and balance > constraint.max_balance_minor:
                issues.append("balance constraint violated: above maximum")
            if constraint.sign_rule == "nonnegative" and balance < 0:
                issues.append("balance constraint violated: must be nonnegative")
            if constraint.sign_rule == "nonpositive" and balance > 0:
                issues.append("balance constraint violated: must be nonpositive")
        return BalanceConstraintValidation(
            valid=not issues, balance_minor=balance, issues=tuple(issues)
        )

    @staticmethod
    def _account_summary(account: Account) -> AccountSummary:
        return AccountSummary(
            id=account.id,
            book_id=account.book_id,
            parent_id=account.parent_id,
            account_type=account.account_type,
            name=account.name,
            commodity_id=account.commodity_id,
            institution_id=None,
            institution_name=None,
            country_id=None,
            country_name=None,
            number_last4=account.number_last4,
            is_closed=account.is_closed,
            is_hidden=account.is_hidden,
            is_system=account.is_system,
            system_role=account.system_role,
            created_at=account.created_at,
            updated_at=account.updated_at,
        )

    @staticmethod
    def _commodity_summary(commodity: Commodity) -> ReconciliationCommoditySummary:
        return ReconciliationCommoditySummary(
            id=commodity.id, name=commodity.name, symbol=commodity.symbol, scale=commodity.scale
        )

    @staticmethod
    def _account_balancing_summary(
        balancing: AccountBalancing | None,
    ) -> AccountBalancingSummary | None:
        if balancing is None:
            return None
        return AccountBalancingSummary(
            id=balancing.id,
            account_id=balancing.account_id,
            as_of_date=balancing.as_of_date,
            balance_minor=balancing.balance_minor,
            memo=balancing.memo,
        )

    @staticmethod
    def _reconciliation_balancing_summary(
        balancing: AccountBalancing,
    ) -> ReconciliationBalancingSummary:
        return ReconciliationBalancingSummary(
            id=balancing.id,
            book_id=balancing.book_id,
            account_id=balancing.account_id,
            as_of_date=balancing.as_of_date,
            balance_minor=balancing.balance_minor,
            memo=balancing.memo,
            created_at=balancing.created_at,
            voided_at=balancing.voided_at,
            void_reason=balancing.void_reason,
        )

    @staticmethod
    def _balance_check_summary(check: BalanceCheck) -> BalanceCheckSummary:
        return BalanceCheckSummary(
            id=check.id,
            book_id=check.book_id,
            account_id=check.account_id,
            as_of_date=check.as_of_date,
            balance_minor=check.balance_minor,
            memo=check.memo,
            status=check.status,
            created_at=check.created_at,
        )

    @staticmethod
    def _balance_adjustment_summary(adjustment: BalanceAdjustment) -> BalanceAdjustmentSummary:
        return BalanceAdjustmentSummary(
            id=adjustment.id,
            book_id=adjustment.book_id,
            balance_check_id=adjustment.balance_check_id,
            account_id=adjustment.account_id,
            offset_account_id=adjustment.offset_account_id,
            as_of_date=adjustment.as_of_date,
            target_balance_minor=adjustment.target_balance_minor,
            actual_balance_minor=adjustment.actual_balance_minor,
            adjustment_minor=adjustment.adjustment_minor,
            tx_id=adjustment.tx_id,
            mark=adjustment.mark,
            memo=adjustment.memo,
            created_at=adjustment.created_at,
        )

    @staticmethod
    def _constraint_summary(constraint: BalanceConstraint) -> BalanceConstraintSummary:
        return BalanceConstraintSummary(
            id=constraint.id,
            book_id=constraint.book_id,
            account_id=constraint.account_id,
            min_balance_minor=constraint.min_balance_minor,
            max_balance_minor=constraint.max_balance_minor,
            sign_rule=constraint.sign_rule,
            created_at=constraint.created_at,
            updated_at=constraint.updated_at,
        )

    @staticmethod
    def _map_transactions(
        transactions: list[Transaction], splits: list[Split]
    ) -> list[TransactionSummary]:
        splits_by_tx: dict[int, list[SplitEntry]] = {}
        for split in splits:
            splits_by_tx.setdefault(split.tx_id, []).append(
                SplitEntry(
                    id=split.id,
                    tx_id=split.tx_id,
                    account_id=split.account_id,
                    commodity_id=split.commodity_id,
                    amount_minor=split.amount_minor,
                    category_id=split.category_id,
                    tag_id=split.tag_id,
                    person_id=split.person_id,
                    project_id=split.project_id,
                    share_bps=split.share_bps,
                    memo=split.memo,
                    created_at=split.created_at,
                )
            )
        return [
            TransactionSummary(
                id=transaction.id,
                book_id=transaction.book_id,
                previous_tx_id=transaction.previous_tx_id,
                occurred_date=transaction.occurred_date,
                posted_date=transaction.posted_date,
                payee_id=transaction.payee_id,
                memo=transaction.memo,
                status=transaction.status,
                reference=transaction.reference,
                created_at=transaction.created_at,
                splits=tuple(splits_by_tx.get(transaction.id, [])),
            )
            for transaction in transactions
        ]
