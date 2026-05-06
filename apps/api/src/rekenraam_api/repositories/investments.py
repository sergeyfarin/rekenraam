from __future__ import annotations

from collections import defaultdict
from datetime import UTC, date, datetime
from typing import Any

from sqlalchemy import Select, case, func, or_, select
from sqlalchemy.ext.asyncio import AsyncSession

from rekenraam_api.db.models.accounts import Account
from rekenraam_api.db.models.investments import CorporateAction, CostBasisProfile, InvestmentInstrument, Lot, PriceObservation, SplitLotAllocation
from rekenraam_api.db.models.metadata import Commodity
from rekenraam_api.db.models.transactions import Split, Transaction


class InvestmentRepository:
    def __init__(self, session: AsyncSession) -> None:
        self._session = session

    async def list_instruments(self, book_id: int) -> list[InvestmentInstrument]:
        result = await self._session.execute(
            select(InvestmentInstrument)
            .where(InvestmentInstrument.book_id == book_id)
            .order_by(InvestmentInstrument.display_name.asc(), InvestmentInstrument.id.asc())
        )
        return list(result.scalars().all())

    async def create_instrument(self, **values: Any) -> InvestmentInstrument:
        await self._validate_instrument_refs(values)
        row = InvestmentInstrument(**values)
        self._session.add(row)
        await self._session.commit()
        await self._session.refresh(row)
        return row

    async def update_instrument(self, instrument_id: int, **values: Any) -> InvestmentInstrument | None:
        row = await self._session.get(InvestmentInstrument, instrument_id)
        if row is None or row.book_id != values["book_id"]:
            return None
        await self._validate_instrument_refs(values)
        for key, value in values.items():
            setattr(row, key, value)
        row.updated_at = datetime.now(UTC)
        await self._session.commit()
        await self._session.refresh(row)
        return row

    async def list_cost_basis_profiles(self, book_id: int) -> list[CostBasisProfile]:
        await self.ensure_default_cost_basis_profile(book_id)
        result = await self._session.execute(
            select(CostBasisProfile)
            .where(CostBasisProfile.book_id == book_id)
            .order_by(CostBasisProfile.is_default.desc(), CostBasisProfile.name.asc(), CostBasisProfile.id.asc())
        )
        return list(result.scalars().all())

    async def ensure_default_cost_basis_profile(self, book_id: int) -> CostBasisProfile:
        row = await self._session.scalar(
            select(CostBasisProfile)
            .where(CostBasisProfile.book_id == book_id)
            .where(CostBasisProfile.is_default.is_(True))
            .limit(1)
        )
        if row is not None:
            return row
        row = CostBasisProfile(
            book_id=book_id,
            name="Default FIFO",
            method="fifo",
            description="Default personal accounting cost-basis profile",
            is_default=True,
            metadata_json=None,
        )
        self._session.add(row)
        await self._session.commit()
        await self._session.refresh(row)
        return row

    async def create_cost_basis_profile(self, **values: Any) -> CostBasisProfile:
        if values.get("is_default"):
            await self._clear_default_cost_basis_profiles(values["book_id"])
        row = CostBasisProfile(**values)
        self._session.add(row)
        await self._session.commit()
        await self._session.refresh(row)
        return row

    async def update_cost_basis_profile(self, profile_id: int, **values: Any) -> CostBasisProfile | None:
        row = await self._session.get(CostBasisProfile, profile_id)
        if row is None or row.book_id != values["book_id"]:
            return None
        if values.get("is_default"):
            await self._clear_default_cost_basis_profiles(values["book_id"])
        for key, value in values.items():
            setattr(row, key, value)
        row.updated_at = datetime.now(UTC)
        await self._session.commit()
        await self._session.refresh(row)
        return row

    async def get_cost_basis_profile(self, profile_id: int | None, book_id: int) -> CostBasisProfile:
        if profile_id is None:
            return await self.ensure_default_cost_basis_profile(book_id)
        row = await self._session.get(CostBasisProfile, profile_id)
        if row is None or row.book_id != book_id:
            raise ValueError("cost basis profile not found")
        return row

    async def list_corporate_actions(self, book_id: int) -> list[CorporateAction]:
        result = await self._session.execute(
            select(CorporateAction)
            .where(CorporateAction.book_id == book_id)
            .order_by(CorporateAction.effective_date.desc(), CorporateAction.id.desc())
        )
        return list(result.scalars().all())

    async def create_corporate_action(self, **values: Any) -> CorporateAction:
        await self._validate_corporate_action_refs(values)
        row = CorporateAction(**values)
        self._session.add(row)
        await self._session.commit()
        await self._session.refresh(row)
        return row

    async def create_buy(
        self,
        *,
        book_id: int,
        txn_date: date,
        commodity_id: int,
        investment_account_id: int,
        cash_account_id: int,
        quantity_minor: int,
        cash_amount_minor: int,
        memo: str | None,
        payee_id: int | None,
        status: str,
    ) -> dict[str, Any]:
        investment_account = await self._get_account(investment_account_id)
        cash_account = await self._get_account(cash_account_id)

        if investment_account is None or cash_account is None:
            raise ValueError("account not found")

        transaction = Transaction(
            book_id=book_id,
            occurred_date=txn_date,
            posted_date=txn_date,
            payee_id=payee_id,
            memo=memo,
            status=status,
            reference=None,
        )
        self._session.add(transaction)
        await self._session.flush()

        security_split = Split(
            tx_id=transaction.id,
            account_id=investment_account_id,
            commodity_id=commodity_id,
            amount_minor=quantity_minor,
            category_id=None,
            tag_id=None,
            person_id=None,
            project_id=None,
            share_bps=None,
            memo=None,
        )
        cash_split = Split(
            tx_id=transaction.id,
            account_id=cash_account_id,
            commodity_id=cash_account.commodity_id,
            amount_minor=-cash_amount_minor,
            category_id=None,
            tag_id=None,
            person_id=None,
            project_id=None,
            share_bps=None,
            memo=None,
        )
        self._session.add_all([security_split, cash_split])
        await self._session.flush()

        lot = Lot(
            book_id=book_id,
            account_id=investment_account_id,
            commodity_id=commodity_id,
            opened_date=txn_date,
            notes=None,
            cost_basis_minor=cash_amount_minor,
        )
        self._session.add(lot)
        await self._session.flush()

        self._session.add(
            SplitLotAllocation(
                split_id=security_split.id,
                lot_id=lot.id,
                quantity_minor=quantity_minor,
            )
        )
        await self._session.commit()

        return {
            "transaction_id": transaction.id,
            "allocations": [{"lot_id": lot.id, "quantity_minor": quantity_minor}],
            "lot_id": lot.id,
        }

    async def create_sell(
        self,
        *,
        book_id: int,
        txn_date: date,
        commodity_id: int,
        investment_account_id: int,
        cash_account_id: int,
        quantity_minor: int,
        cash_amount_minor: int,
        lot_strategy: str,
        lot_allocations: tuple[dict[str, int], ...] | None,
        allow_short: bool,
        memo: str | None,
        payee_id: int | None,
        status: str,
    ) -> dict[str, Any]:
        investment_account = await self._get_account(investment_account_id)
        cash_account = await self._get_account(cash_account_id)

        if investment_account is None or cash_account is None:
            raise ValueError("account not found")

        strategy = lot_strategy.strip().lower()
        if not strategy or strategy == "default":
            strategy = investment_account.booking_policy
        if strategy not in {"fifo", "lifo", "average", "custom", "strict"}:
            raise ValueError("lot strategy must be fifo, lifo, average, or custom")
        if strategy == "strict":
            strategy = "custom"
        if investment_account.booking_policy == "strict" and strategy != "custom":
            raise ValueError("strict booking requires custom lot allocations")
        if investment_account.booking_policy == "strict" and allow_short:
            raise ValueError("strict booking does not allow short allocations")

        transaction = Transaction(
            book_id=book_id,
            occurred_date=txn_date,
            posted_date=txn_date,
            payee_id=payee_id,
            memo=memo,
            status=status,
            reference=None,
        )
        self._session.add(transaction)
        await self._session.flush()

        security_split = Split(
            tx_id=transaction.id,
            account_id=investment_account_id,
            commodity_id=commodity_id,
            amount_minor=-quantity_minor,
            category_id=None,
            tag_id=None,
            person_id=None,
            project_id=None,
            share_bps=None,
            memo=None,
        )
        cash_split = Split(
            tx_id=transaction.id,
            account_id=cash_account_id,
            commodity_id=cash_account.commodity_id,
            amount_minor=cash_amount_minor,
            category_id=None,
            tag_id=None,
            person_id=None,
            project_id=None,
            share_bps=None,
            memo=None,
        )
        self._session.add_all([security_split, cash_split])
        await self._session.flush()

        allocations = await self._plan_sell_allocations(
            investment_account_id=investment_account_id,
            commodity_id=commodity_id,
            quantity_minor=quantity_minor,
            strategy=strategy,
            custom_allocations=lot_allocations,
            allow_short=allow_short,
            opened_date=txn_date,
            book_id=book_id,
        )

        for allocation in allocations:
            self._session.add(
                SplitLotAllocation(
                    split_id=security_split.id,
                    lot_id=allocation["lot_id"],
                    quantity_minor=-allocation["quantity_minor"],
                )
            )

        await self._session.commit()
        return {
            "transaction_id": transaction.id,
            "allocations": allocations,
            "lot_id": None,
        }

    async def create_dividend(
        self,
        *,
        book_id: int,
        txn_date: date,
        cash_account_id: int,
        income_account_id: int,
        amount_minor: int,
        memo: str | None,
        payee_id: int | None,
        status: str,
    ) -> dict[str, Any]:
        cash_account = await self._get_account(cash_account_id)
        income_account = await self._get_account(income_account_id)
        if cash_account is None or income_account is None:
            raise ValueError("account not found")

        transaction = Transaction(
            book_id=book_id,
            occurred_date=txn_date,
            posted_date=txn_date,
            payee_id=payee_id,
            memo=memo,
            status=status,
            reference=None,
        )
        self._session.add(transaction)
        await self._session.flush()

        self._session.add_all(
            [
                Split(
                    tx_id=transaction.id,
                    account_id=cash_account_id,
                    commodity_id=cash_account.commodity_id,
                    amount_minor=amount_minor,
                    category_id=None,
                    tag_id=None,
                    person_id=None,
                    project_id=None,
                    share_bps=None,
                    memo=None,
                ),
                Split(
                    tx_id=transaction.id,
                    account_id=income_account_id,
                    commodity_id=income_account.commodity_id,
                    amount_minor=-amount_minor,
                    category_id=None,
                    tag_id=None,
                    person_id=None,
                    project_id=None,
                    share_bps=None,
                    memo=None,
                ),
            ]
        )

        await self._session.commit()
        return {"transaction_id": transaction.id}

    async def create_reinvested_dividend(
        self,
        *,
        book_id: int,
        txn_date: date,
        commodity_id: int,
        investment_account_id: int,
        income_account_id: int,
        quantity_minor: int,
        amount_minor: int,
        memo: str | None,
        payee_id: int | None,
        status: str,
    ) -> dict[str, Any]:
        investment_account = await self._get_account(investment_account_id)
        income_account = await self._get_account(income_account_id)
        if investment_account is None or income_account is None:
            raise ValueError("account not found")
        if investment_account.book_id != book_id or income_account.book_id != book_id:
            raise ValueError("accounts must belong to the book")

        transaction = Transaction(
            book_id=book_id,
            occurred_date=txn_date,
            posted_date=txn_date,
            payee_id=payee_id,
            memo=memo,
            status=status,
            reference=None,
        )
        self._session.add(transaction)
        await self._session.flush()

        security_split = Split(
            tx_id=transaction.id,
            account_id=investment_account_id,
            commodity_id=commodity_id,
            amount_minor=quantity_minor,
            category_id=None,
            tag_id=None,
            person_id=None,
            project_id=None,
            share_bps=None,
            memo="Reinvested dividend quantity",
        )
        income_split = Split(
            tx_id=transaction.id,
            account_id=income_account_id,
            commodity_id=income_account.commodity_id,
            amount_minor=-amount_minor,
            category_id=None,
            tag_id=None,
            person_id=None,
            project_id=None,
            share_bps=None,
            memo="Reinvested dividend income",
        )
        self._session.add_all([security_split, income_split])
        await self._session.flush()

        lot = Lot(
            book_id=book_id,
            account_id=investment_account_id,
            commodity_id=commodity_id,
            opened_date=txn_date,
            notes="Reinvested dividend",
            cost_basis_minor=amount_minor,
        )
        self._session.add(lot)
        await self._session.flush()
        self._session.add(SplitLotAllocation(split_id=security_split.id, lot_id=lot.id, quantity_minor=quantity_minor))
        await self._session.commit()
        return {
            "transaction_id": transaction.id,
            "allocations": [{"lot_id": lot.id, "quantity_minor": quantity_minor}],
            "lot_id": lot.id,
        }

    async def list_positions(self, *, book_id: int, as_of_date: date | None) -> list[dict[str, Any]]:
        balance_expr = func.coalesce(func.sum(Split.amount_minor), 0)
        position_statement: Select[tuple[int, str, str, int, str, int, int]] = (
            select(
                Account.id,
                Account.name,
                Account.account_type,
                Commodity.id,
                Commodity.name,
                Commodity.scale,
                balance_expr.label("balance_minor"),
            )
            .join(Split, Split.account_id == Account.id)
            .join(Transaction, Transaction.id == Split.tx_id)
            .join(Commodity, Commodity.id == Split.commodity_id)
            .where(Account.book_id == book_id)
            .group_by(
                Account.id,
                Account.name,
                Account.account_type,
                Commodity.id,
                Commodity.name,
                Commodity.scale,
            )
            .having(balance_expr != 0)
            .order_by(Account.name.asc(), Commodity.name.asc())
        )
        if as_of_date is not None:
            position_statement = position_statement.where(Transaction.occurred_date <= as_of_date)

        lot_balance_expr = func.coalesce(func.sum(SplitLotAllocation.quantity_minor), 0)
        total_positive_expr = func.coalesce(
            func.sum(case((SplitLotAllocation.quantity_minor > 0, SplitLotAllocation.quantity_minor), else_=0)),
            0,
        )
        lot_statement: Select[tuple[int, int, int, date | None, int, int, int]] = (
            select(
                Lot.id,
                Lot.account_id,
                Lot.commodity_id,
                Lot.opened_date,
                Lot.cost_basis_minor,
                lot_balance_expr.label("balance_minor"),
                total_positive_expr.label("total_positive"),
            )
            .outerjoin(SplitLotAllocation, SplitLotAllocation.lot_id == Lot.id)
            .outerjoin(Split, Split.id == SplitLotAllocation.split_id)
            .outerjoin(Transaction, Transaction.id == Split.tx_id)
            .where(Lot.book_id == book_id)
            .group_by(
                Lot.id,
                Lot.account_id,
                Lot.commodity_id,
                Lot.opened_date,
                Lot.cost_basis_minor,
            )
            .having(lot_balance_expr != 0)
            .order_by(Lot.opened_date.asc(), Lot.id.asc())
        )
        if as_of_date is not None:
            lot_statement = lot_statement.where(or_(Transaction.occurred_date <= as_of_date, Transaction.id.is_(None)))

        position_rows = (await self._session.execute(position_statement)).all()
        lot_rows = (await self._session.execute(lot_statement)).all()

        lot_map: dict[tuple[int, int], list[dict[str, Any]]] = defaultdict(list)
        for lot_id, account_id, commodity_id, opened_date, cost_basis_minor, balance_minor, total_positive in lot_rows:
            remaining_cost_basis_minor = 0
            if total_positive > 0:
                remaining_cost_basis_minor = (cost_basis_minor * balance_minor) // total_positive

            lot_map[(account_id, commodity_id)].append(
                {
                    "lot_id": lot_id,
                    "opened_date": opened_date,
                    "quantity_minor": balance_minor,
                    "cost_basis_minor": cost_basis_minor,
                    "remaining_cost_basis_minor": remaining_cost_basis_minor,
                    "converted_value_minor": None,
                    "converted_cost_basis_minor": None,
                    "price_missing": None,
                }
            )

        return [
            {
                "account_id": account_id,
                "account_name": account_name,
                "account_type": account_type,
                "commodity_id": commodity_id,
                "commodity_name": commodity_name,
                "commodity_scale": commodity_scale,
                "balance_minor": balance_minor,
                "lots": lot_map.get((account_id, commodity_id), []),
            }
            for account_id, account_name, account_type, commodity_id, commodity_name, commodity_scale, balance_minor in position_rows
        ]

    async def convert_positions(
        self,
        *,
        book_id: int,
        base_commodity_id: int,
        as_of_date: date | None,
    ) -> list[dict[str, Any]]:
        positions = await self.list_positions(book_id=book_id, as_of_date=as_of_date)
        prices = await self._latest_prices(book_id=book_id, base_commodity_id=base_commodity_id, as_of_date=as_of_date)

        converted_positions: list[dict[str, Any]] = []
        for position in positions:
            commodity_id = int(position["commodity_id"])
            commodity_scale = int(position["commodity_scale"])
            balance_minor = int(position["balance_minor"])
            lots = [dict(lot) for lot in position["lots"]]

            if commodity_id == base_commodity_id:
                for lot in lots:
                    lot["converted_value_minor"] = int(lot["quantity_minor"])
                    lot["converted_cost_basis_minor"] = int(lot["remaining_cost_basis_minor"])
                    lot["price_missing"] = False
                converted_positions.append({**position, "value_minor": balance_minor, "price_missing": False, "lots": lots})
                continue

            price_minor = prices.get(commodity_id)
            if price_minor is None:
                for lot in lots:
                    lot["converted_value_minor"] = None
                    lot["converted_cost_basis_minor"] = None
                    lot["price_missing"] = True
                converted_positions.append({**position, "value_minor": 0, "price_missing": True, "lots": lots})
                continue

            scale_factor = 10 ** commodity_scale
            value_minor = (balance_minor * price_minor) // scale_factor
            for lot in lots:
                lot["converted_value_minor"] = (int(lot["quantity_minor"]) * price_minor) // scale_factor
                lot["converted_cost_basis_minor"] = int(lot["remaining_cost_basis_minor"])
                lot["price_missing"] = False

            converted_positions.append({**position, "value_minor": value_minor, "price_missing": False, "lots": lots})

        return converted_positions

    async def list_lots_with_holding_period(
        self,
        *,
        book_id: int,
        account_id: int | None,
        commodity_id: int | None,
        as_of_date: date | None,
    ) -> list[dict[str, Any]]:
        balance_expr = func.coalesce(func.sum(SplitLotAllocation.quantity_minor), 0)
        statement: Select[tuple[int, int, str, int, str, date | None, int, int, int]] = (
            select(
                Lot.id,
                Lot.account_id,
                Account.name,
                Lot.commodity_id,
                Commodity.name,
                Lot.opened_date,
                balance_expr.label("quantity_minor"),
                Lot.cost_basis_minor,
                Commodity.scale,
            )
            .join(Account, Account.id == Lot.account_id)
            .join(Commodity, Commodity.id == Lot.commodity_id)
            .outerjoin(SplitLotAllocation, SplitLotAllocation.lot_id == Lot.id)
            .where(Lot.book_id == book_id)
            .group_by(
                Lot.id,
                Lot.account_id,
                Account.name,
                Lot.commodity_id,
                Commodity.name,
                Lot.opened_date,
                Lot.cost_basis_minor,
                Commodity.scale,
            )
            .having(balance_expr != 0)
            .order_by(Lot.opened_date.asc(), Lot.id.asc())
        )
        if account_id is not None:
            statement = statement.where(Lot.account_id == account_id)
        if commodity_id is not None:
            statement = statement.where(Lot.commodity_id == commodity_id)

        rows = (await self._session.execute(statement)).all()
        reference_date = as_of_date or date.today()

        results: list[dict[str, Any]] = []
        for lot_id, row_account_id, account_name, row_commodity_id, commodity_name, opened_date, quantity_minor, cost_basis_minor, commodity_scale in rows:
            holding_days = None if opened_date is None else (reference_date - opened_date).days
            results.append(
                {
                    "lot_id": lot_id,
                    "account_id": row_account_id,
                    "account_name": account_name,
                    "commodity_id": row_commodity_id,
                    "commodity_name": commodity_name,
                    "opened_date": opened_date,
                    "quantity_minor": quantity_minor,
                    "cost_basis_minor": cost_basis_minor,
                    "commodity_scale": commodity_scale,
                    "holding_days": holding_days,
                    "is_long_term": holding_days is not None and holding_days >= 365,
                }
            )

        return results

    async def report_realized_gains(
        self,
        *,
        book_id: int,
        date_from: date | None,
        date_to: date | None,
        cost_basis_profile_id: int | None = None,
    ) -> list[dict[str, Any]]:
        await self.get_cost_basis_profile(cost_basis_profile_id, book_id)
        statement: Select[tuple[int, date, int, int, int]] = (
            select(
                Transaction.id,
                Transaction.occurred_date,
                Split.commodity_id,
                SplitLotAllocation.lot_id,
                func.abs(SplitLotAllocation.quantity_minor).label("quantity_minor"),
            )
            .join(Split, Split.id == SplitLotAllocation.split_id)
            .join(Transaction, Transaction.id == Split.tx_id)
            .where(Transaction.book_id == book_id)
            .where(SplitLotAllocation.quantity_minor < 0)
            .order_by(Transaction.occurred_date.asc(), Transaction.id.asc())
        )
        if date_from is not None:
            statement = statement.where(Transaction.occurred_date >= date_from)
        if date_to is not None:
            statement = statement.where(Transaction.occurred_date <= date_to)

        allocations = (await self._session.execute(statement)).all()
        if not allocations:
            return []

        lot_ids = sorted({lot_id for _, _, _, lot_id, _ in allocations})
        tx_ids = sorted({tx_id for tx_id, _, _, _, _ in allocations})

        total_positive_expr = func.coalesce(
            func.sum(case((SplitLotAllocation.quantity_minor > 0, SplitLotAllocation.quantity_minor), else_=0)),
            0,
        )
        lot_rows = (
            await self._session.execute(
                select(Lot.id, Lot.cost_basis_minor, total_positive_expr.label("total_positive"))
                .outerjoin(SplitLotAllocation, SplitLotAllocation.lot_id == Lot.id)
                .where(Lot.id.in_(lot_ids))
                .group_by(Lot.id, Lot.cost_basis_minor)
            )
        ).all()
        lot_map = {lot_id: {"cost_basis_minor": cost_basis_minor, "total_positive": total_positive} for lot_id, cost_basis_minor, total_positive in lot_rows}

        cash_rows = (
            await self._session.execute(
                select(Split.tx_id, Split.commodity_id, func.sum(Split.amount_minor).label("amount_minor"))
                .where(Split.tx_id.in_(tx_ids))
                .where(Split.amount_minor > 0)
                .group_by(Split.tx_id, Split.commodity_id)
            )
        ).all()
        cash_map: dict[int, list[tuple[int, int]]] = defaultdict(list)
        for tx_id, commodity_id, amount_minor in cash_rows:
            cash_map[tx_id].append((commodity_id, amount_minor))

        results: list[dict[str, Any]] = []
        for tx_id, txn_date, commodity_id, lot_id, quantity_minor in allocations:
            lot_info = lot_map.get(lot_id, {"cost_basis_minor": 0, "total_positive": 0})
            total_positive = int(lot_info["total_positive"])
            cost_basis_minor = 0
            if total_positive > 0:
                cost_basis_minor = (int(lot_info["cost_basis_minor"]) * quantity_minor) // total_positive

            proceeds_options = [entry for entry in cash_map.get(tx_id, []) if entry[0] != commodity_id]
            if len(proceeds_options) == 1:
                quote_commodity_id, proceeds_minor = proceeds_options[0]
                proceeds_missing = False
            else:
                quote_commodity_id = None
                proceeds_minor = 0
                proceeds_missing = True

            results.append(
                {
                    "tx_id": tx_id,
                    "txn_date": txn_date,
                    "commodity_id": commodity_id,
                    "quantity_minor": quantity_minor,
                    "proceeds_minor": proceeds_minor,
                    "quote_commodity_id": quote_commodity_id,
                    "cost_basis_minor": cost_basis_minor,
                    "gain_loss_minor": proceeds_minor - cost_basis_minor,
                    "proceeds_missing": proceeds_missing,
                }
            )

        return results

    async def report_unrealized_gains(
        self,
        *,
        book_id: int,
        base_commodity_id: int,
        as_of_date: date | None,
        cost_basis_profile_id: int | None = None,
    ) -> list[dict[str, Any]]:
        await self.get_cost_basis_profile(cost_basis_profile_id, book_id)
        converted_positions = await self.convert_positions(
            book_id=book_id,
            base_commodity_id=base_commodity_id,
            as_of_date=as_of_date,
        )

        results: list[dict[str, Any]] = []
        for position in converted_positions:
            cost_basis_minor = sum(
                int(lot["converted_cost_basis_minor"])
                for lot in position["lots"]
                if lot["converted_cost_basis_minor"] is not None
            )
            results.append(
                {
                    "account_id": position["account_id"],
                    "account_name": position["account_name"],
                    "account_type": position["account_type"],
                    "commodity_id": position["commodity_id"],
                    "commodity_name": position["commodity_name"],
                    "value_minor": position["value_minor"],
                    "cost_basis_minor": cost_basis_minor,
                    "unrealized_gain_minor": int(position["value_minor"]) - cost_basis_minor,
                    "price_missing": position["price_missing"],
                }
            )

        return results

    async def portfolio_performance(
        self,
        *,
        book_id: int,
        base_commodity_id: int,
        as_of_date: date | None,
        cost_basis_profile_id: int | None,
    ) -> dict[str, Any]:
        unrealized = await self.report_unrealized_gains(
            book_id=book_id,
            base_commodity_id=base_commodity_id,
            as_of_date=as_of_date,
            cost_basis_profile_id=cost_basis_profile_id,
        )
        realized = await self.report_realized_gains(
            book_id=book_id,
            date_from=None,
            date_to=as_of_date,
            cost_basis_profile_id=cost_basis_profile_id,
        )
        income_statement = (
            select(func.coalesce(func.sum(-Split.amount_minor), 0))
            .join(Transaction, Transaction.id == Split.tx_id)
            .join(Account, Account.id == Split.account_id)
            .where(Transaction.book_id == book_id)
            .where(Account.account_type == "income")
        )
        if as_of_date is not None:
            income_statement = income_statement.where(Transaction.occurred_date <= as_of_date)
        income_rows = (await self._session.execute(income_statement)).scalar_one()
        market_value_minor = sum(int(row["value_minor"]) for row in unrealized)
        cost_basis_minor = sum(int(row["cost_basis_minor"]) for row in unrealized)
        unrealized_gain_minor = sum(int(row["unrealized_gain_minor"]) for row in unrealized)
        realized_gain_minor = sum(int(row["gain_loss_minor"]) for row in realized)
        price_missing_count = sum(1 for row in unrealized if row["price_missing"])
        return {
            "book_id": book_id,
            "base_commodity_id": base_commodity_id,
            "as_of_date": as_of_date,
            "market_value_minor": market_value_minor,
            "cost_basis_minor": cost_basis_minor,
            "unrealized_gain_minor": unrealized_gain_minor,
            "realized_gain_minor": realized_gain_minor,
            "income_minor": int(income_rows or 0),
            "price_missing_count": price_missing_count,
        }

    async def account_valuation(
        self,
        *,
        book_id: int,
        base_commodity_id: int,
        as_of_date: date | None,
        cost_basis_profile_id: int | None,
    ) -> list[dict[str, Any]]:
        await self.get_cost_basis_profile(cost_basis_profile_id, book_id)
        converted_positions = await self.convert_positions(
            book_id=book_id,
            base_commodity_id=base_commodity_id,
            as_of_date=as_of_date,
        )
        account_ids = [int(position["account_id"]) for position in converted_positions]
        account_currency: dict[int, int] = {}
        if account_ids:
            account_rows = (await self._session.execute(select(Account.id, Account.commodity_id).where(Account.id.in_(account_ids)))).all()
            account_currency = {account_id: commodity_id for account_id, commodity_id in account_rows}

        totals: dict[int, dict[str, Any]] = {}
        for position in converted_positions:
            account_id = int(position["account_id"])
            row = totals.setdefault(
                account_id,
                {
                    "account_id": account_id,
                    "account_name": position["account_name"],
                    "account_commodity_id": account_currency.get(account_id, base_commodity_id),
                    "market_value_minor": 0,
                    "cost_basis_minor": 0,
                    "unrealized_gain_minor": 0,
                    "price_missing": False,
                    "fx_missing": False,
                },
            )
            cost_basis_minor = sum(int(lot["converted_cost_basis_minor"]) for lot in position["lots"] if lot["converted_cost_basis_minor"] is not None)
            row["market_value_minor"] += int(position["value_minor"])
            row["cost_basis_minor"] += cost_basis_minor
            row["unrealized_gain_minor"] += int(position["value_minor"]) - cost_basis_minor
            row["price_missing"] = bool(row["price_missing"] or position["price_missing"])
        return list(totals.values())

    async def currency_exposure(
        self,
        *,
        book_id: int,
        as_of_date: date | None,
    ) -> list[dict[str, Any]]:
        positions = await self.list_positions(book_id=book_id, as_of_date=as_of_date)
        exposure: dict[int, dict[str, Any]] = {}
        for position in positions:
            commodity_id = int(position["commodity_id"])
            row = exposure.setdefault(
                commodity_id,
                {
                    "commodity_id": commodity_id,
                    "commodity_name": position["commodity_name"],
                    "quantity_minor": 0,
                    "native_value_minor": 0,
                    "price_missing": False,
                },
            )
            row["quantity_minor"] += int(position["balance_minor"])
            row["native_value_minor"] += int(position["balance_minor"])
        return list(exposure.values())

    async def _latest_prices(
        self,
        *,
        book_id: int,
        base_commodity_id: int,
        as_of_date: date | None,
    ) -> dict[int, int]:
        ranking = func.row_number().over(
            partition_by=PriceObservation.commodity_id,
            order_by=(PriceObservation.price_date.desc(), PriceObservation.id.desc()),
        ).label("rank_index")
        statement = select(
            PriceObservation.commodity_id,
            PriceObservation.price_minor,
            ranking,
        ).where(PriceObservation.book_id == book_id)
        statement = statement.where(PriceObservation.quote_commodity_id == base_commodity_id)
        statement = statement.where(PriceObservation.observation_kind == "commodity_market")
        if as_of_date is not None:
            statement = statement.where(PriceObservation.price_date <= as_of_date)

        ranked_prices = statement.subquery()
        rows = (
            await self._session.execute(
                select(ranked_prices.c.commodity_id, ranked_prices.c.price_minor).where(ranked_prices.c.rank_index == 1)
            )
        ).all()
        return {commodity_id: price_minor for commodity_id, price_minor in rows}

    async def _clear_default_cost_basis_profiles(self, book_id: int) -> None:
        rows = await self._session.execute(select(CostBasisProfile).where(CostBasisProfile.book_id == book_id))
        for row in rows.scalars().all():
            row.is_default = False
            row.updated_at = datetime.now(UTC)

    async def _validate_instrument_refs(self, values: dict[str, Any]) -> None:
        book_id = int(values["book_id"])
        commodity = await self._session.get(Commodity, values["commodity_id"])
        if commodity is None or commodity.book_id != book_id:
            raise ValueError("commodity not found")
        for key in ("quote_commodity_id", "trading_commodity_id"):
            commodity_id = values.get(key)
            if commodity_id is None:
                continue
            row = await self._session.get(Commodity, commodity_id)
            if row is None or row.book_id != book_id:
                raise ValueError(f"{key} not found")

    async def _validate_corporate_action_refs(self, values: dict[str, Any]) -> None:
        book_id = int(values["book_id"])
        for key in ("old_instrument_id", "new_instrument_id"):
            instrument_id = values.get(key)
            if instrument_id is None:
                continue
            row = await self._session.get(InvestmentInstrument, instrument_id)
            if row is None or row.book_id != book_id:
                raise ValueError(f"{key} not found")
        cash_commodity_id = values.get("cash_commodity_id")
        if cash_commodity_id is not None:
            row = await self._session.get(Commodity, cash_commodity_id)
            if row is None or row.book_id != book_id:
                raise ValueError("cash commodity not found")

    async def _get_account(self, account_id: int) -> Account | None:
        result = await self._session.execute(select(Account).where(Account.id == account_id))
        return result.scalar_one_or_none()

    async def _list_lot_balances(self, *, account_id: int, commodity_id: int) -> list[dict[str, Any]]:
        balance_expr = func.coalesce(func.sum(SplitLotAllocation.quantity_minor), 0)
        statement = (
            select(Lot.id, Lot.opened_date, balance_expr.label("balance_minor"))
            .outerjoin(SplitLotAllocation, SplitLotAllocation.lot_id == Lot.id)
            .where(Lot.account_id == account_id)
            .where(Lot.commodity_id == commodity_id)
            .group_by(Lot.id, Lot.opened_date)
            .order_by(Lot.opened_date.asc(), Lot.id.asc())
        )
        rows = (await self._session.execute(statement)).all()
        return [
            {"lot_id": lot_id, "opened_date": opened_date, "balance_minor": balance_minor}
            for lot_id, opened_date, balance_minor in rows
        ]

    async def _create_short_lot(self, *, book_id: int, account_id: int, commodity_id: int, opened_date: date) -> int:
        lot = Lot(
            book_id=book_id,
            account_id=account_id,
            commodity_id=commodity_id,
            opened_date=opened_date,
            notes="Short sale",
            cost_basis_minor=0,
        )
        self._session.add(lot)
        await self._session.flush()
        return lot.id

    async def _plan_sell_allocations(
        self,
        *,
        investment_account_id: int,
        commodity_id: int,
        quantity_minor: int,
        strategy: str,
        custom_allocations: tuple[dict[str, int], ...] | None,
        allow_short: bool,
        opened_date: date,
        book_id: int,
    ) -> list[dict[str, int]]:
        allocations: list[dict[str, int]] = []

        if strategy == "custom":
            if not custom_allocations:
                raise ValueError("custom allocations required")
            total = sum(int(allocation["quantity_minor"]) for allocation in custom_allocations)
            if total != quantity_minor:
                raise ValueError("custom allocations must sum to quantity")

            lot_balances = {
                lot["lot_id"]: int(lot["balance_minor"])
                for lot in await self._list_lot_balances(account_id=investment_account_id, commodity_id=commodity_id)
            }
            for allocation in custom_allocations:
                lot_id = int(allocation["lot_id"])
                alloc_qty = int(allocation["quantity_minor"])
                if alloc_qty <= 0:
                    raise ValueError("allocation quantities must be positive")
                if alloc_qty > lot_balances.get(lot_id, 0) and not allow_short:
                    raise ValueError("insufficient lot quantity for custom allocation")
                allocations.append({"lot_id": lot_id, "quantity_minor": alloc_qty})
            return allocations

        lots = await self._list_lot_balances(account_id=investment_account_id, commodity_id=commodity_id)
        positive_lots = [lot for lot in lots if int(lot["balance_minor"]) > 0]

        if strategy == "average":
            total_balance = sum(int(lot["balance_minor"]) for lot in positive_lots)
            allocated = 0
            for index, lot in enumerate(positive_lots):
                balance = int(lot["balance_minor"])
                if total_balance <= 0 or balance <= 0:
                    continue
                if index == len(positive_lots) - 1:
                    alloc_qty = min(quantity_minor - allocated, balance)
                else:
                    alloc_qty = min((quantity_minor * balance) // total_balance, balance)
                if alloc_qty > 0:
                    allocations.append({"lot_id": int(lot["lot_id"]), "quantity_minor": alloc_qty})
                    allocated += alloc_qty
            remaining = quantity_minor - allocated
        else:
            ordered_lots = list(positive_lots)
            if strategy == "lifo":
                ordered_lots.sort(key=lambda lot: (lot["opened_date"] or date.min, int(lot["lot_id"])), reverse=True)
            remaining = quantity_minor
            for lot in ordered_lots:
                if remaining <= 0:
                    break
                take = min(remaining, int(lot["balance_minor"]))
                if take > 0:
                    allocations.append({"lot_id": int(lot["lot_id"]), "quantity_minor": take})
                    remaining -= take

        if remaining > 0:
            if not allow_short:
                raise ValueError("insufficient lots available for sale")
            short_lot_id = await self._create_short_lot(
                book_id=book_id,
                account_id=investment_account_id,
                commodity_id=commodity_id,
                opened_date=opened_date,
            )
            allocations.append({"lot_id": short_lot_id, "quantity_minor": remaining})

        return allocations
