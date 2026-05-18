import { apiGet, apiPost, apiPut } from "$lib/api/client";

function buildQuery(params: Record<string, string | number | null | undefined>): string {
  const searchParams = new URLSearchParams();

  for (const [key, value] of Object.entries(params)) {
    if (value !== null && value !== undefined) {
      searchParams.set(key, String(value));
    }
  }

  const query = searchParams.toString();
  return query ? `?${query}` : "";
}

export type InvestmentInstrumentCreateInput = {
  book_id: number;
  commodity_id: number;
  instrument_type: string;
  display_name: string;
  symbol: string | null;
  isin: string | null;
  cusip: string | null;
  figi: string | null;
  exchange: string | null;
  venue: string | null;
  issuer: string | null;
  country_code: string | null;
  quote_commodity_id: number | null;
  trading_commodity_id: number | null;
  quantity_scale: number;
  price_scale: number;
  is_active: boolean;
  metadata_json: string | null;
};

export type InvestmentInstrumentUpdateInput = InvestmentInstrumentCreateInput & { id: number };

export type InvestmentInstrumentSummary = InvestmentInstrumentCreateInput & {
  id: number;
  created_at: string;
  updated_at: string;
};

export type CostBasisProfileCreateInput = {
  book_id: number;
  name: string;
  method: string;
  description: string | null;
  is_default: boolean;
  metadata_json: string | null;
};

export type CostBasisProfileUpdateInput = CostBasisProfileCreateInput & { id: number };

export type CostBasisProfileSummary = CostBasisProfileCreateInput & {
  id: number;
  created_at: string;
  updated_at: string;
};

export type CorporateActionCreateInput = {
  book_id: number;
  action_type: string;
  old_instrument_id: number | null;
  new_instrument_id: number | null;
  effective_date: string;
  ratio_numerator: number | null;
  ratio_denominator: number | null;
  cash_in_lieu_minor: number | null;
  cash_commodity_id: number | null;
  source_reference: string | null;
  memo: string | null;
  metadata_json: string | null;
};

export type CorporateActionSummary = CorporateActionCreateInput & {
  id: number;
  generated_transaction_id: number | null;
  created_at: string;
};

export type BuyCommodityInput = {
  book_id: number;
  txn_date: string;
  commodity_id: number;
  investment_account_id: number;
  cash_account_id: number;
  quantity_minor: number;
  cash_amount_minor: number;
  memo: string | null;
  payee_id: number | null;
  status: string | null;
};

export type SellLotAllocationInput = {
  lot_id: number;
  quantity_minor: number;
};

export type SellCommodityInput = {
  book_id: number;
  txn_date: string;
  commodity_id: number;
  investment_account_id: number;
  cash_account_id: number;
  quantity_minor: number;
  cash_amount_minor: number;
  lot_strategy: string;
  lot_allocations: SellLotAllocationInput[] | null;
  allow_short: boolean;
  memo: string | null;
  payee_id: number | null;
  status: string | null;
};

export type DividendInput = {
  book_id: number;
  txn_date: string;
  cash_account_id: number;
  income_account_id: number;
  amount_minor: number;
  memo: string | null;
  payee_id: number | null;
  status: string | null;
};

export type ReinvestedDividendInput = {
  book_id: number;
  txn_date: string;
  commodity_id: number;
  investment_account_id: number;
  income_account_id: number;
  quantity_minor: number;
  amount_minor: number;
  memo: string | null;
  payee_id: number | null;
  status: string | null;
};

export type TradeAllocation = {
  lot_id: number;
  quantity_minor: number;
};

export type TradeResult = {
  transaction_id: number;
  allocations: TradeAllocation[];
  lot_id: number | null;
};

export type DividendResult = {
  transaction_id: number;
};

export type InvestmentPositionLot = {
  lot_id: number;
  opened_date: string | null;
  quantity_minor: number;
  cost_basis_minor: number;
  remaining_cost_basis_minor: number;
  converted_value_minor: number | null;
  converted_cost_basis_minor: number | null;
  price_missing: boolean | null;
};

export type InvestmentPositionSummary = {
  account_id: number;
  account_name: string;
  account_type: string;
  commodity_id: number;
  commodity_name: string;
  commodity_scale: number;
  balance_minor: number;
  lots: InvestmentPositionLot[];
};

export type ConvertedInvestmentPositionSummary = InvestmentPositionSummary & {
  value_minor: number;
  price_missing: boolean;
};

export type InvestmentLotHoldingSummary = {
  lot_id: number;
  account_id: number;
  account_name: string;
  commodity_id: number;
  commodity_name: string;
  opened_date: string | null;
  quantity_minor: number;
  cost_basis_minor: number;
  commodity_scale: number;
  holding_days: number | null;
  is_long_term: boolean;
};

export type PortfolioPerformanceSummary = {
  book_id: number;
  base_commodity_id: number;
  as_of_date: string | null;
  market_value_minor: number;
  cost_basis_minor: number;
  unrealized_gain_minor: number;
  realized_gain_minor: number;
  income_minor: number;
  price_missing_count: number;
};

export type AccountValuationRow = {
  account_id: number;
  account_name: string;
  account_commodity_id: number;
  market_value_minor: number;
  cost_basis_minor: number;
  unrealized_gain_minor: number;
  price_missing: boolean;
  fx_missing: boolean;
};

export type CurrencyExposureRow = {
  commodity_id: number;
  commodity_name: string;
  quantity_minor: number;
  native_value_minor: number;
  price_missing: boolean;
};

export async function getPositions(bookId: number, asOfDate?: string | null): Promise<InvestmentPositionSummary[]> {
  const path = `/investments/positions${buildQuery({ book_id: bookId, as_of_date: asOfDate ?? null })}`;
  return apiGet<InvestmentPositionSummary[]>(path);
}

export async function convertPositions(
  bookId: number,
  baseCommodityId: number,
  asOfDate?: string | null
): Promise<ConvertedInvestmentPositionSummary[]> {
  const path = `/investments/positions/converted${buildQuery({
    book_id: bookId,
    base_commodity_id: baseCommodityId,
    as_of_date: asOfDate ?? null,
  })}`;
  return apiGet<ConvertedInvestmentPositionSummary[]>(path);
}

export async function listLotsWithHoldingPeriod(
  bookId: number,
  accountId?: number | null,
  commodityId?: number | null,
  asOfDate?: string | null
): Promise<InvestmentLotHoldingSummary[]> {
  const path = `/investments/lots${buildQuery({
    book_id: bookId,
    account_id: accountId ?? null,
    commodity_id: commodityId ?? null,
    as_of_date: asOfDate ?? null,
  })}`;
  return apiGet<InvestmentLotHoldingSummary[]>(path);
}

export async function listInvestmentInstruments(bookId: number): Promise<InvestmentInstrumentSummary[]> {
  return apiGet<InvestmentInstrumentSummary[]>(`/investments/instruments?book_id=${bookId}`);
}

export async function saveInvestmentInstrument(
  input: InvestmentInstrumentCreateInput | InvestmentInstrumentUpdateInput
): Promise<InvestmentInstrumentSummary> {
  if ("id" in input) {
    return apiPut<InvestmentInstrumentSummary, InvestmentInstrumentCreateInput>(`/investments/instruments/${input.id}`, input);
  }
  return apiPost<InvestmentInstrumentSummary, InvestmentInstrumentCreateInput>("/investments/instruments", input);
}

export async function listCostBasisProfiles(bookId: number): Promise<CostBasisProfileSummary[]> {
  return apiGet<CostBasisProfileSummary[]>(`/investments/cost-basis-profiles?book_id=${bookId}`);
}

export async function saveCostBasisProfile(
  input: CostBasisProfileCreateInput | CostBasisProfileUpdateInput
): Promise<CostBasisProfileSummary> {
  if ("id" in input) {
    return apiPut<CostBasisProfileSummary, CostBasisProfileCreateInput>(`/investments/cost-basis-profiles/${input.id}`, input);
  }
  return apiPost<CostBasisProfileSummary, CostBasisProfileCreateInput>("/investments/cost-basis-profiles", input);
}

export async function listCorporateActions(bookId: number): Promise<CorporateActionSummary[]> {
  return apiGet<CorporateActionSummary[]>(`/investments/corporate-actions?book_id=${bookId}`);
}

export async function createCorporateAction(input: CorporateActionCreateInput): Promise<CorporateActionSummary> {
  return apiPost<CorporateActionSummary, CorporateActionCreateInput>("/investments/corporate-actions", input);
}

export async function buyCommodity(input: BuyCommodityInput): Promise<TradeResult> {
  return apiPost<TradeResult, BuyCommodityInput>("/investments/buy", input);
}

export async function sellCommodity(input: SellCommodityInput): Promise<TradeResult> {
  return apiPost<TradeResult, SellCommodityInput>("/investments/sell", input);
}

export async function createDividendTransaction(input: DividendInput): Promise<DividendResult> {
  return apiPost<DividendResult, DividendInput>("/investments/dividend", input);
}

export async function createReinvestedDividend(input: ReinvestedDividendInput): Promise<TradeResult> {
  return apiPost<TradeResult, ReinvestedDividendInput>("/investments/reinvested-dividend", input);
}

export async function getPortfolioPerformance(
  bookId: number,
  baseCommodityId: number,
  asOfDate?: string | null,
  costBasisProfileId?: number | null
): Promise<PortfolioPerformanceSummary> {
  const path = `/investments/performance${buildQuery({
    book_id: bookId,
    base_commodity_id: baseCommodityId,
    as_of_date: asOfDate ?? null,
    cost_basis_profile_id: costBasisProfileId ?? null,
  })}`;
  return apiGet<PortfolioPerformanceSummary>(path);
}

export async function getAccountValuation(
  bookId: number,
  baseCommodityId: number,
  asOfDate?: string | null,
  costBasisProfileId?: number | null
): Promise<AccountValuationRow[]> {
  const path = `/investments/account-valuation${buildQuery({
    book_id: bookId,
    base_commodity_id: baseCommodityId,
    as_of_date: asOfDate ?? null,
    cost_basis_profile_id: costBasisProfileId ?? null,
  })}`;
  return apiGet<AccountValuationRow[]>(path);
}

export async function getCurrencyExposure(bookId: number, asOfDate?: string | null): Promise<CurrencyExposureRow[]> {
  const path = `/investments/currency-exposure${buildQuery({ book_id: bookId, as_of_date: asOfDate ?? null })}`;
  return apiGet<CurrencyExposureRow[]>(path);
}