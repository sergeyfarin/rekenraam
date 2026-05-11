from datetime import UTC, date, datetime

from sqlalchemy import Select, exists, literal, select
from sqlalchemy.dialects.postgresql import insert as pg_insert
from sqlalchemy.ext.asyncio import AsyncSession

from rekenraam_api.db.models.accounts import Account, AccountBalancing
from rekenraam_api.db.models.imports import (
    ImportRule,
    ImportSession,
    ImportSessionTransaction,
    ImportTransactionKey,
)
from rekenraam_api.db.models.metadata import Commodity, Payee
from rekenraam_api.db.models.transactions import Split, Transaction


def _escape_like(value: str) -> str:
    return value.replace("\\", "\\\\").replace("%", "\\%").replace("_", "\\_")


class ImportRepository:
    def __init__(self, session: AsyncSession) -> None:
        self._session = session

    @staticmethod
    def _is_current_transaction_clause():
        newer = Transaction.__table__.alias("newer_transactions_import")
        return ~exists(select(literal(1)).where(newer.c.previous_tx_id == Transaction.id))

    async def get_account(self, account_id: int) -> Account | None:
        return await self._session.get(Account, account_id)

    async def get_account_commodity(self, account_id: int) -> Commodity | None:
        statement = (
            select(Commodity)
            .join(Account, Account.commodity_id == Commodity.id)
            .where(Account.id == account_id)
        )
        return await self._session.scalar(statement)

    async def find_matching_transaction(
        self,
        account_id: int,
        *,
        txn_date: date | None,
        amount_minor: int | None,
        import_id: str | None,
        payee_name: str | None,
        memo: str | None,
    ) -> int | None:
        account_ids = [account_id]
        if import_id:
            statement = (
                select(Transaction.id)
                .join(Split, Split.tx_id == Transaction.id)
                .where(Split.account_id.in_(account_ids))
                .where(Transaction.import_id == import_id)
                .where(Transaction.status != "void")
                .where(self._is_current_transaction_clause())
                .order_by(Transaction.id.desc())
                .limit(1)
            )
            match_id = await self._session.scalar(statement)
            if match_id is not None:
                return int(match_id)
            key_match_id = await self.find_import_key_transaction(account_id, import_id)
            if key_match_id is not None:
                return key_match_id

        if txn_date is None or amount_minor is None:
            return None

        statement = (
            select(Transaction.id)
            .join(Split, Split.tx_id == Transaction.id)
            .outerjoin(Payee, Payee.id == Transaction.payee_id)
            .where(Split.account_id.in_(account_ids))
            .where(Transaction.occurred_date == txn_date)
            .where(Split.amount_minor == amount_minor)
            .where(Transaction.status != "void")
            .where(self._is_current_transaction_clause())
        )
        if payee_name:
            statement = statement.where(
                Payee.name.ilike(f"%{_escape_like(payee_name)}%", escape="\\")
            )
        elif memo:
            statement = statement.where(
                Transaction.memo.ilike(f"%{_escape_like(memo)}%", escape="\\")
            )
        statement = statement.order_by(Transaction.id.desc()).limit(1)
        match_id = await self._session.scalar(statement)
        return int(match_id) if match_id is not None else None

    async def find_import_key_transaction(self, account_id: int, import_id: str) -> int | None:
        statement = (
            select(ImportTransactionKey.tx_id)
            .where(
                ImportTransactionKey.account_id == account_id,
                ImportTransactionKey.import_id == import_id,
            )
            .limit(1)
        )
        match_id = await self._session.scalar(statement)
        return int(match_id) if match_id is not None else None

    async def list_rules(self, book_id: int) -> list[ImportRule]:
        statement: Select[tuple[ImportRule]] = (
            select(ImportRule)
            .where(ImportRule.book_id == book_id, ImportRule.deleted_at.is_(None))
            .order_by(ImportRule.priority.asc(), ImportRule.id.asc())
        )
        result = await self._session.execute(statement)
        return list(result.scalars().all())

    async def create_rule(self, **values: object) -> ImportRule:
        rule = ImportRule(**values)
        self._session.add(rule)
        await self._session.commit()
        await self._session.refresh(rule)
        return rule

    async def get_rule(self, rule_id: int) -> ImportRule | None:
        return await self._session.get(ImportRule, rule_id)

    async def delete_rule(self, rule_id: int) -> bool:
        rule = await self._session.get(ImportRule, rule_id)
        if rule is None or rule.deleted_at is not None:
            return False
        rule.deleted_at = datetime.now(UTC)
        await self._session.commit()
        return True

    async def create_session(self, **values: object) -> ImportSession:
        item = ImportSession(**values)
        self._session.add(item)
        await self._session.flush()
        return item

    async def mark_session_committed(self, session_id: int) -> ImportSession | None:
        item = await self._session.get(ImportSession, session_id)
        if item is None:
            return None
        item.status = "committed"
        item.committed_at = datetime.now(UTC)
        await self._session.flush()
        return item

    async def mark_session_abandoned(self, session_id: int, reason: str) -> ImportSession | None:
        item = await self._session.get(ImportSession, session_id)
        if item is None:
            return None
        item.status = "abandoned"
        item.notes = f"{item.notes} | abandoned: {reason}" if item.notes else f"abandoned: {reason}"
        await self._session.flush()
        return item

    async def list_sessions(self, book_id: int) -> list[ImportSession]:
        statement = (
            select(ImportSession)
            .where(ImportSession.book_id == book_id)
            .order_by(ImportSession.started_at.desc(), ImportSession.id.desc())
        )
        result = await self._session.execute(statement)
        return list(result.scalars().all())

    async def get_session(self, session_id: int) -> ImportSession | None:
        return await self._session.get(ImportSession, session_id)

    async def record_session_transaction(self, session_id: int, tx_id: int, action: str) -> None:
        statement = (
            pg_insert(ImportSessionTransaction)
            .values(session_id=session_id, tx_id=tx_id, action=action)
            .on_conflict_do_nothing(index_elements=["session_id", "tx_id", "action"])
        )
        await self._session.execute(statement)

    async def record_import_key(
        self, *, book_id: int, account_id: int, tx_id: int, import_id: str | None
    ) -> bool:
        if import_id is None or not import_id.strip():
            return False
        statement = (
            pg_insert(ImportTransactionKey)
            .values(
                book_id=book_id,
                account_id=account_id,
                tx_id=tx_id,
                import_id=import_id.strip(),
            )
            .on_conflict_do_nothing(index_elements=["account_id", "import_id"])
            .returning(ImportTransactionKey.id)
        )
        result = await self._session.execute(statement)
        return result.scalar_one_or_none() is not None

    async def list_session_transactions(self, session_id: int) -> list[ImportSessionTransaction]:
        statement = (
            select(ImportSessionTransaction)
            .where(ImportSessionTransaction.session_id == session_id)
            .order_by(ImportSessionTransaction.id.asc())
        )
        result = await self._session.execute(statement)
        return list(result.scalars().all())

    async def find_payee_by_name(self, book_id: int, name: str) -> Payee | None:
        statement = (
            select(Payee)
            .where(Payee.book_id == book_id, Payee.name == name)
            .order_by(Payee.id.desc())
            .limit(1)
        )
        return await self._session.scalar(statement)

    async def create_payee(self, book_id: int, name: str) -> Payee:
        payee = Payee(book_id=book_id, name=name, kind="payee")
        self._session.add(payee)
        await self._session.flush()
        return payee

    async def get_or_create_imbalance_account(self, book_id: int, commodity_id: int) -> Account:
        statement = (
            select(Account)
            .where(
                Account.book_id == book_id,
                Account.commodity_id == commodity_id,
                Account.account_type == "equity",
                Account.name == "Imbalance-Import",
            )
            .order_by(Account.id.asc())
            .limit(1)
        )
        account = await self._session.scalar(statement)
        if account is not None:
            return account
        account = Account(
            book_id=book_id,
            parent_id=None,
            account_type="equity",
            name="Imbalance-Import",
            commodity_id=commodity_id,
            is_system=True,
            system_role="imbalance_import",
        )
        self._session.add(account)
        await self._session.flush()
        return account

    async def locked_account_dates(
        self, account_ids: set[int], txn_date: date
    ) -> list[tuple[int, date]]:
        if not account_ids:
            return []
        statement = (
            select(AccountBalancing.account_id, AccountBalancing.as_of_date)
            .where(AccountBalancing.account_id.in_(account_ids))
            .where(AccountBalancing.voided_at.is_(None))
            .where(AccountBalancing.as_of_date >= txn_date)
            .order_by(AccountBalancing.as_of_date.asc())
        )
        result = await self._session.execute(statement)
        seen: set[int] = set()
        locked: list[tuple[int, date]] = []
        for account_id, as_of_date in result.all():
            if account_id not in seen:
                locked.append((account_id, as_of_date))
                seen.add(account_id)
        return locked

    async def create_import_transaction(
        self,
        *,
        book_id: int,
        account_id: int,
        offset_account_id: int,
        commodity_id: int,
        txn_date: date,
        amount_minor: int,
        payee_id: int | None,
        memo: str | None,
        reference: str | None,
        import_id: str | None,
        import_session_id: int,
        category_id: int | None,
        audit: dict[str, int | str | None],
    ) -> Transaction:
        tx = Transaction(
            book_id=book_id,
            occurred_date=txn_date,
            posted_date=txn_date,
            payee_id=payee_id,
            memo=memo,
            status="uncleared",
            reference=reference,
            import_id=import_id,
            import_session_id=import_session_id,
            **audit,
        )
        self._session.add(tx)
        await self._session.flush()
        self._session.add_all(
            [
                Split(
                    tx_id=tx.id,
                    account_id=account_id,
                    commodity_id=commodity_id,
                    amount_minor=amount_minor,
                    category_id=category_id,
                    memo=memo,
                    **audit,
                ),
                Split(
                    tx_id=tx.id,
                    account_id=offset_account_id,
                    commodity_id=commodity_id,
                    amount_minor=-amount_minor,
                    **audit,
                ),
            ]
        )
        await self._session.flush()
        return tx

    async def enrich_transaction(
        self,
        tx_id: int,
        *,
        payee_id: int | None,
        memo: str | None,
        reference: str | None,
        import_id: str | None,
        import_session_id: int,
        audit: dict[str, int | str | None],
    ) -> Transaction | None:
        current = await self._session.get(Transaction, tx_id)
        if current is None or current.status == "void":
            return None
        resolved_payee_id = current.payee_id or payee_id
        resolved_memo = current.memo or memo
        resolved_reference = current.reference or reference
        resolved_import_id = current.import_id or import_id
        resolved_session_id = current.import_session_id or import_session_id
        if (
            resolved_payee_id == current.payee_id
            and resolved_memo == current.memo
            and resolved_reference == current.reference
            and resolved_import_id == current.import_id
            and resolved_session_id == current.import_session_id
        ):
            return None
        replacement = Transaction(
            book_id=current.book_id,
            previous_tx_id=current.id,
            occurred_date=current.occurred_date,
            posted_date=current.posted_date,
            payee_id=resolved_payee_id,
            memo=resolved_memo,
            status=current.status,
            reference=resolved_reference,
            import_id=resolved_import_id,
            import_session_id=resolved_session_id,
            **audit,
        )
        self._session.add(replacement)
        await self._session.flush()
        splits = await self._session.execute(
            select(Split).where(Split.tx_id == current.id).order_by(Split.id.asc())
        )
        for split in splits.scalars().all():
            self._session.add(
                Split(
                    tx_id=replacement.id,
                    account_id=split.account_id,
                    commodity_id=split.commodity_id,
                    amount_minor=split.amount_minor,
                    category_id=split.category_id,
                    tag_id=split.tag_id,
                    person_id=split.person_id,
                    project_id=split.project_id,
                    share_bps=split.share_bps,
                    memo=split.memo,
                    **audit,
                )
            )
        await self._session.flush()
        return replacement

    async def list_transaction_account_ids(self, tx_id: int) -> set[int]:
        result = await self._session.execute(select(Split.account_id).where(Split.tx_id == tx_id))
        return {int(row[0]) for row in result.all()}
