from __future__ import annotations

from collections import defaultdict
from datetime import date
from typing import Any

from sqlalchemy import Select, case, func, or_, select
from sqlalchemy.ext.asyncio import AsyncSession

from rekenraam_api.db.models.accounts import Account
from rekenraam_api.db.models.investments import Lot, PriceObservation, SplitLotAllocation
from rekenraam_api.db.models.metadata import Commodity
from rekenraam_api.db.models.transactions import Split, Transaction


class InvestmentRepository:
    def __init__(self, session: AsyncSession) -> None:
        self._session = session

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
                lot["converted_cost_basis_minor"] = (int(lot["remaining_cost_basis_minor"]) * price_minor) // scale_factor
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
    ) -> list[dict[str, Any]]:
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
    ) -> list[dict[str, Any]]:
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
