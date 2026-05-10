from __future__ import annotations

import base64
import json
from datetime import date
from decimal import ROUND_HALF_UP, Decimal
from typing import Any

from sqlalchemy import Select, asc, desc, exists, func, literal, or_, select
from sqlalchemy.ext.asyncio import AsyncSession

from rekenraam_api.db.models.accounts import Account, AccountBalancing
from rekenraam_api.db.models.investments import PriceObservation
from rekenraam_api.db.models.metadata import Category, Commodity, Payee, Person, Project, Tag
from rekenraam_api.db.models.transactions import Split, Transaction
from rekenraam_api.schemas.transactions import TransactionListFilters


def _encode_cursor(*, occurred_date: date, tx_id: int, split_id: int | None = None) -> str:
    payload: dict[str, Any] = {"occurred_date": occurred_date.isoformat(), "tx_id": tx_id}
    if split_id is not None:
        payload["split_id"] = split_id
    raw = json.dumps(payload, separators=(",", ":"), sort_keys=True).encode()
    return base64.urlsafe_b64encode(raw).decode().rstrip("=")


def _decode_cursor(cursor: str | None) -> dict[str, Any] | None:
    if not cursor:
        return None
    try:
        padded = cursor + ("=" * (-len(cursor) % 4))
        payload = json.loads(base64.urlsafe_b64decode(padded.encode()).decode())
        return {
            "occurred_date": date.fromisoformat(str(payload["occurred_date"])),
            "tx_id": int(payload["tx_id"]),
            "split_id": int(payload["split_id"]) if "split_id" in payload else None,
        }
    except (KeyError, TypeError, ValueError, json.JSONDecodeError) as error:
        raise ValueError("cursor is invalid") from error


def _required_int(value: int | str | None, field_name: str) -> int:
    if value is None:
        raise ValueError(f"{field_name} is required")
    return int(value)


def _optional_int(value: int | str | None) -> int | None:
    return None if value is None else int(value)


class TransactionRepository:
    def __init__(self, session: AsyncSession) -> None:
        self._session = session

    @staticmethod
    def _is_current_clause() -> Any:
        newer = Transaction.__table__.alias("newer_transactions")
        return ~exists(select(literal(1)).where(newer.c.previous_tx_id == Transaction.id))

    async def list_transactions(
        self,
        filters: TransactionListFilters | None = None,
        allowed_book_ids: list[int] | None = None,
    ) -> tuple[list[Transaction], str | None]:
        active_filters = filters or TransactionListFilters()
        limit = min(max(active_filters.limit or 100, 1), 500)

        statement: Select[tuple[Transaction]] = (
            select(Transaction)
            .outerjoin(Payee, Payee.id == Transaction.payee_id)
            .where(self._is_current_clause())
        )

        if active_filters.book_id is not None:
            statement = statement.where(Transaction.book_id == active_filters.book_id)
        elif allowed_book_ids is not None:
            if not allowed_book_ids:
                return [], None
            statement = statement.where(Transaction.book_id.in_(allowed_book_ids))

        status_filter = active_filters.status
        if status_filter is None or status_filter == "active":
            statement = statement.where(Transaction.status != "void")
        elif status_filter == "void":
            statement = statement.where(Transaction.status == "void")
        elif status_filter != "all":
            statement = statement.where(Transaction.status == status_filter)

        if active_filters.account_id is not None:
            account_ids = await self.list_account_chain_ids(active_filters.account_id)
            if not account_ids:
                return [], None
            account_tx_ids = select(Split.tx_id).where(Split.account_id.in_(account_ids))
            statement = statement.where(Transaction.id.in_(account_tx_ids))

        if active_filters.payee_id is not None:
            statement = statement.where(Transaction.payee_id == active_filters.payee_id)

        if active_filters.occurred_from is not None:
            statement = statement.where(Transaction.occurred_date >= active_filters.occurred_from)

        if active_filters.occurred_to is not None:
            statement = statement.where(Transaction.occurred_date <= active_filters.occurred_to)

        if active_filters.search:
            like_pattern = f"%{active_filters.search}%"
            statement = statement.where(
                or_(
                    Transaction.memo.ilike(like_pattern),
                    Transaction.reference.ilike(like_pattern),
                    Payee.name.ilike(like_pattern),
                )
            )

        if active_filters.amount_min is not None:
            matching_tx_ids = select(Split.tx_id).where(func.abs(Split.amount_minor) >= active_filters.amount_min)
            statement = statement.where(Transaction.id.in_(matching_tx_ids))

        if active_filters.amount_max is not None:
            matching_tx_ids = select(Split.tx_id).where(func.abs(Split.amount_minor) <= active_filters.amount_max)
            statement = statement.where(Transaction.id.in_(matching_tx_ids))

        sort_by = active_filters.sort_by or "date"
        sort_dir = active_filters.sort_dir or "desc"
        if sort_by != "date":
            sort_by = "date"
        cursor = _decode_cursor(active_filters.cursor)
        if cursor is not None:
            cursor_date = cursor["occurred_date"]
            cursor_tx_id = cursor["tx_id"]
            if sort_dir == "asc":
                statement = statement.where(
                    (Transaction.occurred_date > cursor_date)
                    | ((Transaction.occurred_date == cursor_date) & (Transaction.id > cursor_tx_id))
                )
            else:
                statement = statement.where(
                    (Transaction.occurred_date < cursor_date)
                    | ((Transaction.occurred_date == cursor_date) & (Transaction.id < cursor_tx_id))
                )

        primary_direction = asc if sort_dir == "asc" else desc
        statement = statement.order_by(primary_direction(Transaction.occurred_date), primary_direction(Transaction.id)).limit(limit + 1)
        if active_filters.offset is not None and cursor is None:
            statement = statement.offset(max(active_filters.offset, 0))

        result = await self._session.execute(statement)
        rows = list(result.scalars().all())
        next_cursor = None
        if len(rows) > limit:
            last = rows[limit - 1]
            next_cursor = _encode_cursor(occurred_date=last.occurred_date, tx_id=last.id)
            rows = rows[:limit]
        return rows, next_cursor

    async def get_transaction_by_id(self, transaction_id: int) -> Transaction | None:
        transaction = await self._session.get(Transaction, transaction_id)
        if transaction is None:
            return None

        while True:
            statement: Select[tuple[Transaction]] = select(Transaction).where(Transaction.previous_tx_id == transaction.id).limit(1)
            newer = await self._session.scalar(statement)
            if newer is None:
                return transaction
            transaction = newer

    async def create_transaction(
        self,
        *,
        book_id: int,
        txn_date: date,
        payee_id: int | None,
        memo: str | None,
        status: str,
        reference: str | None,
        import_id: str | None = None,
        import_session_id: int | None = None,
        previous_tx_id: int | None = None,
        created_by_user_id: int | None = None,
        created_session_id: int | None = None,
        created_device_id: int | None = None,
        created_request_id: str | None = None,
    ) -> Transaction:
        transaction = Transaction(
            book_id=book_id,
            previous_tx_id=previous_tx_id,
            occurred_date=txn_date,
            posted_date=txn_date,
            payee_id=payee_id,
            memo=memo,
            status=status,
            reference=reference,
            import_id=import_id,
            import_session_id=import_session_id,
            created_by_user_id=created_by_user_id,
            created_session_id=created_session_id,
            created_device_id=created_device_id,
            created_request_id=created_request_id,
        )
        self._session.add(transaction)
        await self._session.flush()
        return transaction

    async def update_transaction(
        self,
        *,
        transaction_id: int,
        txn_date: date,
        payee_id: int | None,
        memo: str | None,
        status: str,
        reference: str | None,
        created_by_user_id: int | None = None,
        created_session_id: int | None = None,
        created_device_id: int | None = None,
        created_request_id: str | None = None,
    ) -> Transaction | None:
        current = await self.get_transaction_by_id(transaction_id)
        if current is None or current.status == "void":
            return None
        return await self.create_transaction(
            book_id=current.book_id,
            previous_tx_id=current.id,
            txn_date=txn_date,
            payee_id=payee_id,
            memo=memo,
            status=status,
            reference=reference,
            import_id=current.import_id,
            import_session_id=current.import_session_id,
            created_by_user_id=created_by_user_id,
            created_session_id=created_session_id,
            created_device_id=created_device_id,
            created_request_id=created_request_id,
        )

    async def delete_transaction(
        self,
        transaction_id: int,
        *,
        created_by_user_id: int | None = None,
        created_session_id: int | None = None,
        created_device_id: int | None = None,
        created_request_id: str | None = None,
    ) -> bool:
        current = await self.get_transaction_by_id(transaction_id)
        if current is None or current.status == "void":
            return False
        replacement = await self.create_transaction(
            book_id=current.book_id,
            previous_tx_id=current.id,
            txn_date=current.occurred_date,
            payee_id=current.payee_id,
            memo=current.memo,
            status="void",
            reference=current.reference,
            import_id=current.import_id,
            import_session_id=current.import_session_id,
            created_by_user_id=created_by_user_id,
            created_session_id=created_session_id,
            created_device_id=created_device_id,
            created_request_id=created_request_id,
        )
        await self.copy_transaction_splits(
            source_transaction_id=current.id,
            target_transaction_id=replacement.id,
            created_by_user_id=created_by_user_id,
            created_session_id=created_session_id,
            created_device_id=created_device_id,
            created_request_id=created_request_id,
        )
        await self._session.commit()
        return True

    async def list_splits_for_transaction_ids(self, transaction_ids: list[int]) -> list[Split]:
        if not transaction_ids:
            return []

        statement: Select[tuple[Split]] = select(Split).where(Split.tx_id.in_(transaction_ids)).order_by(Split.id)
        result = await self._session.execute(statement)
        return list(result.scalars().all())

    async def replace_transaction_splits(
        self,
        transaction_id: int,
        splits: list[dict[str, int | str | None]],
        created_by_user_id: int | None = None,
        created_session_id: int | None = None,
        created_device_id: int | None = None,
        created_request_id: str | None = None,
    ) -> list[Split]:
        existing = await self.list_splits_for_transaction_ids([transaction_id])
        for split in existing:
            await self._session.delete(split)
        await self._session.flush()

        created: list[Split] = []
        for split_input in splits:
            split = Split(
                tx_id=transaction_id,
                account_id=_required_int(split_input["account_id"], "account_id"),
                commodity_id=_required_int(split_input["commodity_id"], "commodity_id"),
                amount_minor=_required_int(split_input["amount_minor"], "amount_minor"),
                category_id=_optional_int(split_input["category_id"]),
                tag_id=_optional_int(split_input["tag_id"]),
                person_id=_optional_int(split_input["person_id"]),
                project_id=_optional_int(split_input["project_id"]),
                share_bps=_optional_int(split_input["share_bps"]),
                memo=str(split_input["memo"]) if split_input["memo"] is not None else None,
                created_by_user_id=created_by_user_id,
                created_session_id=created_session_id,
                created_device_id=created_device_id,
                created_request_id=created_request_id,
            )
            self._session.add(split)
            created.append(split)

        await self._session.flush()
        return created

    async def copy_transaction_splits(
        self,
        *,
        source_transaction_id: int,
        target_transaction_id: int,
        created_by_user_id: int | None = None,
        created_session_id: int | None = None,
        created_device_id: int | None = None,
        created_request_id: str | None = None,
    ) -> list[Split]:
        source_splits = await self.list_splits_for_transaction_ids([source_transaction_id])
        return await self.replace_transaction_splits(
            target_transaction_id,
            [
                {
                    "account_id": split.account_id,
                    "commodity_id": split.commodity_id,
                    "amount_minor": split.amount_minor,
                    "category_id": split.category_id,
                    "tag_id": split.tag_id,
                    "person_id": split.person_id,
                    "project_id": split.project_id,
                    "share_bps": split.share_bps,
                    "memo": split.memo,
                }
                for split in source_splits
            ],
            created_by_user_id=created_by_user_id,
            created_session_id=created_session_id,
            created_device_id=created_device_id,
            created_request_id=created_request_id,
        )

    async def list_account_chain_ids(self, account_id: int) -> list[int]:
        result = await self._session.execute(select(Account).order_by(Account.id.asc()))
        accounts = list(result.scalars().all())
        chain_ids = {account_id}
        changed = True
        while changed:
            changed = False
            for account in accounts:
                if account.id in chain_ids and account.previous_account_id is not None and account.previous_account_id not in chain_ids:
                    chain_ids.add(account.previous_account_id)
                    changed = True
                if account.previous_account_id in chain_ids and account.id not in chain_ids:
                    chain_ids.add(account.id)
                    changed = True
        return sorted(chain_ids)

    async def list_account_register_splits(
        self,
        account_id: int,
        *,
        limit: int = 100,
        cursor: str | None = None,
    ) -> tuple[list[tuple[Transaction, Split, int]], str | None]:
        limit = min(max(limit, 1), 500)
        account_ids = await self.list_account_chain_ids(account_id)
        decoded = _decode_cursor(cursor)

        statement = (
            select(Transaction, Split)
            .join(Split, Split.tx_id == Transaction.id)
            .where(Split.account_id.in_(account_ids))
            .where(Transaction.status != "void")
            .where(self._is_current_clause())
        )
        opening_statement = (
            select(func.coalesce(func.sum(Split.amount_minor), 0))
            .join(Transaction, Transaction.id == Split.tx_id)
            .where(Split.account_id.in_(account_ids))
            .where(Transaction.status != "void")
            .where(self._is_current_clause())
        )
        if decoded is not None:
            cursor_date = decoded["occurred_date"]
            cursor_tx_id = decoded["tx_id"]
            cursor_split_id = decoded["split_id"] or 0
            before_clause = (
                (Transaction.occurred_date < cursor_date)
                | ((Transaction.occurred_date == cursor_date) & (Transaction.id < cursor_tx_id))
                | (
                    (Transaction.occurred_date == cursor_date)
                    & (Transaction.id == cursor_tx_id)
                    & (Split.id <= cursor_split_id)
                )
            )
            after_clause = (
                (Transaction.occurred_date > cursor_date)
                | ((Transaction.occurred_date == cursor_date) & (Transaction.id > cursor_tx_id))
                | (
                    (Transaction.occurred_date == cursor_date)
                    & (Transaction.id == cursor_tx_id)
                    & (Split.id > cursor_split_id)
                )
            )
            opening_statement = opening_statement.where(before_clause)
            statement = statement.where(after_clause)

        opening_balance = (
            0 if decoded is None else int(await self._session.scalar(opening_statement) or 0)
        )
        statement = statement.order_by(Transaction.occurred_date.asc(), Transaction.id.asc(), Split.id.asc()).limit(limit + 1)
        result = await self._session.execute(statement)
        raw_rows = list(result.all())
        next_cursor = None
        if len(raw_rows) > limit:
            last_transaction, last_split = raw_rows[limit - 1]
            next_cursor = _encode_cursor(
                occurred_date=last_transaction.occurred_date,
                tx_id=last_transaction.id,
                split_id=last_split.id,
            )
            raw_rows = raw_rows[:limit]

        running_balance = opening_balance
        rows: list[tuple[Transaction, Split, int]] = []
        for transaction, split in raw_rows:
            running_balance += split.amount_minor
            rows.append((transaction, split, running_balance))
        return rows, next_cursor

    async def get_payee_defaults(self, payee_id: int, account_id: int | None = None) -> tuple[int | None, str | None]:
        statement = (
            select(Split.category_id, Transaction.memo)
            .join(Transaction, Transaction.id == Split.tx_id)
            .where(Transaction.payee_id == payee_id)
            .where(Split.category_id.is_not(None))
            .where(Transaction.status != "void")
            .where(self._is_current_clause())
        )

        if account_id is not None:
            account_ids = await self.list_account_chain_ids(account_id)
            statement = statement.where(Split.account_id.in_(account_ids))

        statement = statement.order_by(Transaction.occurred_date.desc(), Transaction.id.desc()).limit(1)
        result = await self._session.execute(statement)
        row = result.first()
        if row is None:
            return None, None
        return row[0], row[1]

    async def duplicate_transaction(
        self,
        transaction_id: int,
        today: date,
        *,
        created_by_user_id: int | None = None,
        created_session_id: int | None = None,
        created_device_id: int | None = None,
        created_request_id: str | None = None,
    ) -> Transaction | None:
        source = await self.get_transaction_by_id(transaction_id)
        if source is None or source.status == "void":
            return None

        duplicate = await self.create_transaction(
            book_id=source.book_id,
            txn_date=today,
            payee_id=source.payee_id,
            memo=source.memo,
            status="uncleared",
            reference=None,
            created_by_user_id=created_by_user_id,
            created_session_id=created_session_id,
            created_device_id=created_device_id,
            created_request_id=created_request_id,
        )
        await self.copy_transaction_splits(
            source_transaction_id=source.id,
            target_transaction_id=duplicate.id,
            created_by_user_id=created_by_user_id,
            created_session_id=created_session_id,
            created_device_id=created_device_id,
            created_request_id=created_request_id,
        )

        await self._session.commit()
        await self._session.refresh(duplicate)
        return duplicate

    async def bulk_void_transactions(
        self,
        transaction_ids: list[int],
        *,
        created_by_user_id: int | None = None,
        created_session_id: int | None = None,
        created_device_id: int | None = None,
        created_request_id: str | None = None,
    ) -> int:
        if not transaction_ids:
            return 0

        voided = 0
        for transaction_id in transaction_ids:
            if await self.delete_transaction(
                transaction_id,
                created_by_user_id=created_by_user_id,
                created_session_id=created_session_id,
                created_device_id=created_device_id,
                created_request_id=created_request_id,
            ):
                voided += 1
        return voided

    async def bulk_delete_transactions(
        self,
        transaction_ids: list[int],
        *,
        created_by_user_id: int | None = None,
        created_session_id: int | None = None,
        created_device_id: int | None = None,
        created_request_id: str | None = None,
    ) -> int:
        return await self.bulk_void_transactions(
            transaction_ids,
            created_by_user_id=created_by_user_id,
            created_session_id=created_session_id,
            created_device_id=created_device_id,
            created_request_id=created_request_id,
        )

    async def list_accounts_by_ids(self, account_ids: set[int]) -> list[Account]:
        if not account_ids:
            return []
        result = await self._session.execute(select(Account).where(Account.id.in_(account_ids)))
        return list(result.scalars().all())

    async def get_commodity(self, commodity_id: int) -> Commodity | None:
        return await self._session.get(Commodity, commodity_id)

    async def create_manual_fx_observation(
        self,
        *,
        book_id: int,
        from_currency_id: int,
        to_currency_id: int,
        rate_date: date,
        rate: Decimal,
        source: str,
    ) -> PriceObservation:
        from_currency = await self._session.get(Commodity, from_currency_id)
        to_currency = await self._session.get(Commodity, to_currency_id)
        if (
            from_currency is None
            or to_currency is None
            or from_currency.book_id != book_id
            or to_currency.book_id != book_id
            or from_currency.kind != "currency"
            or to_currency.kind != "currency"
        ):
            raise ValueError("currency not found")

        scale_factor = Decimal(10) ** from_currency.scale
        observation = PriceObservation(
            book_id=book_id,
            commodity_id=from_currency_id,
            quote_commodity_id=to_currency_id,
            observation_kind="fx_manual",
            price_minor=int((rate * scale_factor).to_integral_value(rounding=ROUND_HALF_UP)),
            price_date=rate_date,
            source=source,
        )
        self._session.add(observation)
        await self._session.flush()
        return observation

    async def get_locked_account_ids(self, account_ids: set[int], occurred_date: date) -> list[tuple[int, date]]:
        if not account_ids:
            return []
        newer_balancing = AccountBalancing.__table__.alias("newer_account_balancings")
        statement = (
            select(AccountBalancing.account_id, AccountBalancing.as_of_date)
            .where(AccountBalancing.account_id.in_(account_ids))
            .where(AccountBalancing.voided_at.is_(None))
            .where(AccountBalancing.as_of_date >= occurred_date)
            .where(~exists(select(literal(1)).where(newer_balancing.c.previous_account_balancing_id == AccountBalancing.id)))
            .order_by(AccountBalancing.as_of_date.asc())
        )
        result = await self._session.execute(statement)
        return [(account_id, as_of_date) for account_id, as_of_date in result.all()]

    async def metadata_refs_belong_to_book(
        self,
        *,
        book_id: int,
        payee_id: int | None,
        category_ids: set[int],
        tag_ids: set[int],
        person_ids: set[int],
        project_ids: set[int],
    ) -> bool:
        checks: list[tuple[type[Any], set[int] | int | None]] = [
            (Payee, payee_id),
            (Category, category_ids),
            (Tag, tag_ids),
            (Person, person_ids),
            (Project, project_ids),
        ]
        for model, ids in checks:
            if ids is None or ids == set():
                continue
            id_set = {ids} if isinstance(ids, int) else ids
            count = await self._session.scalar(select(func.count()).select_from(model).where(model.id.in_(id_set)).where(model.book_id == book_id))
            if count != len(id_set):
                return False
        return True
