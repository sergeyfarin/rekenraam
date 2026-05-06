from rekenraam_api.repositories.investments import InvestmentRepository
from rekenraam_api.schemas.investments import (
    AccountValuationRow,
    BuyCommodityInput,
    ConvertedPosition,
    ConvertedPositionsQuery,
    CorporateActionCreateInput,
    CorporateActionSummary,
    CostBasisProfileCreateInput,
    CostBasisProfileSummary,
    CostBasisProfileUpdateInput,
    CurrencyExposureRow,
    DividendInput,
    DividendResult,
    InvestmentEventInput,
    InvestmentInstrumentCreateInput,
    InvestmentInstrumentSummary,
    InvestmentInstrumentUpdateInput,
    LotHoldingPeriod,
    LotsHoldingQuery,
    PortfolioPerformanceQuery,
    PortfolioPerformanceSummary,
    Position,
    PositionsQuery,
    RealizedGainEntry,
    RealizedGainsQuery,
    ReinvestedDividendInput,
    SellCommodityInput,
    TradeResult,
    UnrealizedGainEntry,
    UnrealizedGainsQuery,
)
from rekenraam_api.services.report_invalidation import bump_report_state


class InvestmentService:
    def __init__(self, repository: InvestmentRepository) -> None:
        self._repository = repository

    async def list_instruments(self, book_id: int) -> list[InvestmentInstrumentSummary]:
        rows = await self._repository.list_instruments(book_id)
        return [InvestmentInstrumentSummary.model_validate(row, from_attributes=True) for row in rows]

    async def create_instrument(self, input: InvestmentInstrumentCreateInput) -> InvestmentInstrumentSummary:
        self._validate_instrument_type(input.instrument_type)
        self._validate_scale(input.quantity_scale, "quantity_scale")
        self._validate_scale(input.price_scale, "price_scale")
        row = await self._repository.create_instrument(**input.model_dump())
        return InvestmentInstrumentSummary.model_validate(row, from_attributes=True)

    async def update_instrument(self, instrument_id: int, input: InvestmentInstrumentUpdateInput) -> InvestmentInstrumentSummary | None:
        self._validate_instrument_type(input.instrument_type)
        self._validate_scale(input.quantity_scale, "quantity_scale")
        self._validate_scale(input.price_scale, "price_scale")
        row = await self._repository.update_instrument(instrument_id, **input.model_dump())
        if row is None:
            return None
        return InvestmentInstrumentSummary.model_validate(row, from_attributes=True)

    async def list_cost_basis_profiles(self, book_id: int) -> list[CostBasisProfileSummary]:
        rows = await self._repository.list_cost_basis_profiles(book_id)
        return [CostBasisProfileSummary.model_validate(row, from_attributes=True) for row in rows]

    async def create_cost_basis_profile(self, input: CostBasisProfileCreateInput) -> CostBasisProfileSummary:
        self._validate_cost_basis_method(input.method)
        row = await self._repository.create_cost_basis_profile(**input.model_dump())
        return CostBasisProfileSummary.model_validate(row, from_attributes=True)

    async def update_cost_basis_profile(self, profile_id: int, input: CostBasisProfileUpdateInput) -> CostBasisProfileSummary | None:
        self._validate_cost_basis_method(input.method)
        row = await self._repository.update_cost_basis_profile(profile_id, **input.model_dump())
        if row is None:
            return None
        return CostBasisProfileSummary.model_validate(row, from_attributes=True)

    async def list_corporate_actions(self, book_id: int) -> list[CorporateActionSummary]:
        rows = await self._repository.list_corporate_actions(book_id)
        return [CorporateActionSummary.model_validate(row, from_attributes=True) for row in rows]

    async def create_corporate_action(self, input: CorporateActionCreateInput) -> CorporateActionSummary:
        self._validate_corporate_action(input)
        row = await self._repository.create_corporate_action(**input.model_dump())
        await bump_report_state(getattr(self._repository, "_session", None), input.book_id)
        return CorporateActionSummary.model_validate(row, from_attributes=True)

    async def create_buy(self, input: BuyCommodityInput) -> TradeResult:
        self._validate_positive(input.quantity_minor, "quantity")
        self._validate_positive(input.cash_amount_minor, "cash amount")
        row = await self._repository.create_buy(
            book_id=input.book_id,
            txn_date=input.txn_date,
            commodity_id=input.commodity_id,
            investment_account_id=input.investment_account_id,
            cash_account_id=input.cash_account_id,
            quantity_minor=input.quantity_minor,
            cash_amount_minor=input.cash_amount_minor,
            memo=input.memo,
            payee_id=input.payee_id,
            status=input.status or "uncleared",
        )
        await bump_report_state(getattr(self._repository, "_session", None), input.book_id)
        return TradeResult.model_validate(row)

    async def create_sell(self, input: SellCommodityInput) -> TradeResult:
        self._validate_positive(input.quantity_minor, "quantity")
        self._validate_positive(input.cash_amount_minor, "cash amount")
        row = await self._repository.create_sell(
            book_id=input.book_id,
            txn_date=input.txn_date,
            commodity_id=input.commodity_id,
            investment_account_id=input.investment_account_id,
            cash_account_id=input.cash_account_id,
            quantity_minor=input.quantity_minor,
            cash_amount_minor=input.cash_amount_minor,
            lot_strategy=input.lot_strategy,
            lot_allocations=None if input.lot_allocations is None else tuple(allocation.model_dump() for allocation in input.lot_allocations),
            allow_short=input.allow_short,
            memo=input.memo,
            payee_id=input.payee_id,
            status=input.status or "uncleared",
        )
        await bump_report_state(getattr(self._repository, "_session", None), input.book_id)
        return TradeResult.model_validate(row)

    async def create_dividend(self, input: DividendInput) -> DividendResult:
        self._validate_positive(input.amount_minor, "dividend amount")
        row = await self._repository.create_dividend(
            book_id=input.book_id,
            txn_date=input.txn_date,
            cash_account_id=input.cash_account_id,
            income_account_id=input.income_account_id,
            amount_minor=input.amount_minor,
            memo=input.memo,
            payee_id=input.payee_id,
            status=input.status or "uncleared",
        )
        await bump_report_state(getattr(self._repository, "_session", None), input.book_id)
        return DividendResult.model_validate(row)

    async def create_reinvested_dividend(self, input: ReinvestedDividendInput) -> TradeResult:
        self._validate_positive(input.quantity_minor, "quantity")
        self._validate_positive(input.amount_minor, "amount")
        row = await self._repository.create_reinvested_dividend(
            book_id=input.book_id,
            txn_date=input.txn_date,
            commodity_id=input.commodity_id,
            investment_account_id=input.investment_account_id,
            income_account_id=input.income_account_id,
            quantity_minor=input.quantity_minor,
            amount_minor=input.amount_minor,
            memo=input.memo,
            payee_id=input.payee_id,
            status=input.status or "uncleared",
        )
        await bump_report_state(getattr(self._repository, "_session", None), input.book_id)
        return TradeResult.model_validate(row)

    async def record_investment_event(self, input: InvestmentEventInput) -> CorporateActionSummary:
        action_type = "derivative_lifecycle" if input.action_type.startswith(("option_", "future_")) else "private_investment_event"
        row = await self._repository.create_corporate_action(
            book_id=input.book_id,
            action_type=action_type,
            old_instrument_id=input.instrument_id,
            new_instrument_id=None,
            effective_date=input.effective_date,
            ratio_numerator=input.quantity_minor,
            ratio_denominator=None,
            cash_in_lieu_minor=input.amount_minor,
            cash_commodity_id=input.cash_commodity_id,
            source_reference=input.action_type,
            memo=input.memo,
            generated_transaction_id=None,
            metadata_json=input.metadata_json,
        )
        await bump_report_state(getattr(self._repository, "_session", None), input.book_id)
        return CorporateActionSummary.model_validate(row, from_attributes=True)

    async def list_positions(self, input: PositionsQuery) -> list[Position]:
        rows = await self._repository.list_positions(book_id=input.book_id, as_of_date=input.as_of_date)
        return [Position.model_validate(row) for row in rows]

    async def convert_positions(self, input: ConvertedPositionsQuery) -> list[ConvertedPosition]:
        rows = await self._repository.convert_positions(
            book_id=input.book_id,
            base_commodity_id=input.base_commodity_id,
            as_of_date=input.as_of_date,
        )
        return [ConvertedPosition.model_validate(row) for row in rows]

    async def list_lots_with_holding_period(self, input: LotsHoldingQuery) -> list[LotHoldingPeriod]:
        rows = await self._repository.list_lots_with_holding_period(
            book_id=input.book_id,
            account_id=input.account_id,
            commodity_id=input.commodity_id,
            as_of_date=input.as_of_date,
        )
        return [LotHoldingPeriod.model_validate(row) for row in rows]

    async def report_realized_gains(self, input: RealizedGainsQuery) -> list[RealizedGainEntry]:
        rows = await self._repository.report_realized_gains(
            book_id=input.book_id,
            date_from=input.date_from,
            date_to=input.date_to,
            cost_basis_profile_id=input.cost_basis_profile_id,
        )
        return [RealizedGainEntry.model_validate(row) for row in rows]

    async def report_unrealized_gains(self, input: UnrealizedGainsQuery) -> list[UnrealizedGainEntry]:
        rows = await self._repository.report_unrealized_gains(
            book_id=input.book_id,
            base_commodity_id=input.base_commodity_id,
            as_of_date=input.as_of_date,
            cost_basis_profile_id=input.cost_basis_profile_id,
        )
        return [UnrealizedGainEntry.model_validate(row) for row in rows]

    async def portfolio_performance(self, input: PortfolioPerformanceQuery) -> PortfolioPerformanceSummary:
        row = await self._repository.portfolio_performance(
            book_id=input.book_id,
            base_commodity_id=input.base_commodity_id,
            as_of_date=input.as_of_date,
            cost_basis_profile_id=input.cost_basis_profile_id,
        )
        return PortfolioPerformanceSummary.model_validate(row)

    async def account_valuation(self, input: PortfolioPerformanceQuery) -> list[AccountValuationRow]:
        rows = await self._repository.account_valuation(
            book_id=input.book_id,
            base_commodity_id=input.base_commodity_id,
            as_of_date=input.as_of_date,
            cost_basis_profile_id=input.cost_basis_profile_id,
        )
        return [AccountValuationRow.model_validate(row) for row in rows]

    async def currency_exposure(self, input: PositionsQuery) -> list[CurrencyExposureRow]:
        rows = await self._repository.currency_exposure(book_id=input.book_id, as_of_date=input.as_of_date)
        return [CurrencyExposureRow.model_validate(row) for row in rows]

    @staticmethod
    def _validate_positive(value: int, label: str) -> None:
        if value <= 0:
            raise ValueError(f"{label} must be positive")

    @staticmethod
    def _validate_instrument_type(value: str) -> None:
        if value not in {"stock", "etf", "mutual_fund", "private_fund", "bond", "option", "future", "crypto", "private_investment", "generic"}:
            raise ValueError("unsupported instrument type")

    @staticmethod
    def _validate_cost_basis_method(value: str) -> None:
        if value not in {"fifo", "lifo", "average_cost", "specific_lot"}:
            raise ValueError("cost basis method must be fifo, lifo, average_cost, or specific_lot")

    @staticmethod
    def _validate_scale(value: int, label: str) -> None:
        if value < 0 or value > 12:
            raise ValueError(f"{label} must be between 0 and 12")

    @staticmethod
    def _validate_corporate_action(input: CorporateActionCreateInput) -> None:
        valid = {
            "split",
            "reverse_split",
            "spin_off",
            "merger",
            "acquisition",
            "conversion",
            "return_of_capital",
            "delisting",
            "write_off",
            "derivative_lifecycle",
            "private_investment_event",
        }
        if input.action_type not in valid:
            raise ValueError("unsupported corporate action type")
        if input.action_type in {"split", "reverse_split", "spin_off", "merger", "acquisition", "conversion"}:
            if input.old_instrument_id is None:
                raise ValueError("old_instrument_id is required")
        if input.ratio_numerator is not None and input.ratio_numerator <= 0:
            raise ValueError("ratio_numerator must be positive")
        if input.ratio_denominator is not None and input.ratio_denominator <= 0:
            raise ValueError("ratio_denominator must be positive")
