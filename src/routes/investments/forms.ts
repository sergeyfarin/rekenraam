import type { AccountSummary } from "$lib/api/accounts";
import type {
  BuyCommodityInput,
  ConvertedInvestmentPositionSummary,
  DividendInput,
  InvestmentLotHoldingSummary,
  InvestmentPositionSummary,
  SellCommodityInput,
} from "$lib/api/investments";
import type { CommoditySummary } from "$lib/api/metadata";
import { formatMinorWithScale, parseAmountToMinor } from "$lib/money";

type SelectValue = number | string | null;

export type TradeType = "buy" | "sell";
export type InvestmentsTab = "holdings" | "lots";

export type TradeFormState = {
  txn_date: string;
  investment_account_id: SelectValue;
  cash_account_id: SelectValue;
  commodity_id: SelectValue;
  quantity: string;
  cash_amount: string;
  memo: string;
};

export type DividendFormState = {
  txn_date: string;
  cash_account_id: SelectValue;
  income_account_id: SelectValue;
  amount: string;
  memo: string;
};

export function createTodayDate(): string {
  return new Date().toISOString().split("T")[0];
}

export function createTradeFormState(txnDate = createTodayDate()): TradeFormState {
  return {
    txn_date: txnDate,
    investment_account_id: null,
    cash_account_id: null,
    commodity_id: null,
    quantity: "",
    cash_amount: "",
    memo: "",
  };
}

export function createDividendFormState(txnDate = createTodayDate()): DividendFormState {
  return {
    txn_date: txnDate,
    cash_account_id: null,
    income_account_id: null,
    amount: "",
    memo: "",
  };
}

function parseRequiredId(value: SelectValue, fieldName: string): number {
  const parsed = typeof value === "string" ? Number(value) : value;
  if (!parsed || Number.isNaN(parsed)) {
    throw new Error(`${fieldName} is required.`);
  }
  return parsed;
}

function parsePositiveMinor(value: string, scale: number, fieldName: string): number {
  const parsed = parseAmountToMinor(value, scale);
  if (parsed === null || parsed <= 0) {
    throw new Error(`${fieldName} must be greater than zero.`);
  }
  return parsed;
}

export function buildTradePayload(
  bookId: number,
  tradeType: "buy",
  form: TradeFormState,
  commodityScale: number
): BuyCommodityInput;
export function buildTradePayload(
  bookId: number,
  tradeType: "sell",
  form: TradeFormState,
  commodityScale: number
): SellCommodityInput;
export function buildTradePayload(
  bookId: number,
  tradeType: TradeType,
  form: TradeFormState,
  commodityScale: number
): BuyCommodityInput | SellCommodityInput {
  const investmentAccountId = parseRequiredId(form.investment_account_id, "Investment account");
  const cashAccountId = parseRequiredId(form.cash_account_id, "Cash account");
  const commodityId = parseRequiredId(form.commodity_id, "Security");
  const quantityMinor = parsePositiveMinor(form.quantity, commodityScale, "Quantity");
  const cashAmountMinor = parsePositiveMinor(form.cash_amount, 2, tradeType === "buy" ? "Total cost" : "Proceeds");
  const memo = form.memo.trim() || null;

  if (tradeType === "buy") {
    return {
      book_id: bookId,
      txn_date: form.txn_date,
      investment_account_id: investmentAccountId,
      cash_account_id: cashAccountId,
      commodity_id: commodityId,
      quantity_minor: quantityMinor,
      cash_amount_minor: cashAmountMinor,
      memo,
      payee_id: null,
      status: null,
    };
  }

  return {
    book_id: bookId,
    txn_date: form.txn_date,
    investment_account_id: investmentAccountId,
    cash_account_id: cashAccountId,
    commodity_id: commodityId,
    quantity_minor: quantityMinor,
    cash_amount_minor: cashAmountMinor,
    lot_strategy: "fifo",
    lot_allocations: null,
    allow_short: false,
    memo,
    payee_id: null,
    status: null,
  };
}

export function buildDividendPayload(bookId: number, form: DividendFormState): DividendInput {
  return {
    book_id: bookId,
    txn_date: form.txn_date,
    cash_account_id: parseRequiredId(form.cash_account_id, "Cash account"),
    income_account_id: parseRequiredId(form.income_account_id, "Income account"),
    amount_minor: parsePositiveMinor(form.amount, 2, "Dividend amount"),
    memo: form.memo.trim() || null,
    payee_id: null,
    status: null,
  };
}

export function calculatePositionTotals(positions: ConvertedInvestmentPositionSummary[]) {
  const totalValue = positions.reduce((sum, position) => sum + position.value_minor, 0);
  const totalCostBasis = positions.reduce((sum, position) => {
    return sum + position.lots.reduce((lotSum, lot) => lotSum + (lot.converted_cost_basis_minor || lot.remaining_cost_basis_minor), 0);
  }, 0);
  return {
    totalValue,
    totalCostBasis,
    totalGainLoss: totalValue - totalCostBasis,
  };
}

export function formatCurrency(minor: number, scale = 2): string {
  return (minor / 10 ** scale).toLocaleString(undefined, {
    minimumFractionDigits: scale,
    maximumFractionDigits: scale,
  });
}

export function formatDate(dateStr: string | null): string {
  if (!dateStr) return "—";
  return new Date(dateStr).toLocaleDateString();
}

export function formatQuantity(minor: number, scale: number): string {
  return formatMinorWithScale(minor, scale);
}

export function getInvestmentAccounts(accounts: AccountSummary[]): AccountSummary[] {
  return accounts.filter((account) => ["investment", "brokerage", "stock"].includes(account.account_type));
}

export function getCashAccounts(accounts: AccountSummary[]): AccountSummary[] {
  return accounts.filter((account) => ["checking", "savings", "cash", "asset"].includes(account.account_type));
}

export function getIncomeAccounts(accounts: AccountSummary[]): AccountSummary[] {
  return accounts.filter((account) => account.account_type === "income");
}

export function getSecurities(commodities: CommoditySummary[]): CommoditySummary[] {
  return commodities.filter((commodity) => ["security", "stock", "mutual_fund"].includes(commodity.kind));
}

export function getCurrencies(commodities: CommoditySummary[]): CommoditySummary[] {
  return commodities.filter((commodity) => commodity.kind === "currency");
}

export type HoldingsTablePosition = InvestmentPositionSummary | ConvertedInvestmentPositionSummary;

export function calculateHoldingRow(position: HoldingsTablePosition) {
  const costBasis = position.lots.reduce(
    (sum, lot) => sum + (lot.converted_cost_basis_minor || lot.remaining_cost_basis_minor),
    0
  );
  const value = "value_minor" in position ? position.value_minor : 0;
  return {
    costBasis,
    value,
    gainLoss: value - costBasis,
    priceMissing: "price_missing" in position ? position.price_missing : false,
  };
}

export function getSelectedSecurity(
  securities: CommoditySummary[],
  selectedCommodityId: SelectValue
): CommoditySummary | null {
  const numericId = typeof selectedCommodityId === "string" ? Number(selectedCommodityId) : selectedCommodityId;
  return securities.find((commodity) => commodity.id === numericId) ?? null;
}

export function getCommodityScale(commodities: CommoditySummary[], selectedCommodityId: SelectValue): number {
  const numericId = typeof selectedCommodityId === "string" ? Number(selectedCommodityId) : selectedCommodityId;
  return commodities.find((commodity) => commodity.id === numericId)?.scale ?? 0;
}

export function shouldLoadLots(activeTab: InvestmentsTab, lots: InvestmentLotHoldingSummary[]): boolean {
  return activeTab === "lots" && lots.length === 0;
}