import { apiDelete, apiGet, apiPost, apiPut } from "$lib/api/client";

export type GroupBy = "day" | "month" | "quarter" | "year";

export type CashflowReportInput = {
  book_id: number;
  date_from: string | null;
  date_to: string | null;
  group_by: GroupBy | null;
};

export type CashflowRow = {
  period_start: string;
  inflow_minor: number;
  outflow_minor: number;
  net_minor: number;
};

export type NetWorthReportInput = {
  book_id: number;
  date_from: string | null;
  date_to: string | null;
  group_by: GroupBy | null;
};

export type NetWorthRow = {
  period_start: string;
  assets_minor: number;
  liabilities_minor: number;
  net_worth_minor: number;
};

export type AccountTrendReportInput = {
  book_id: number;
  account_ids: number[] | null;
  date_from: string | null;
  date_to: string | null;
  group_by: GroupBy | null;
};

export type AccountTrendRow = {
  period_start: string;
  account_id: number;
  account_name: string;
  balance_minor: number;
};

export type CategorySpendReportInput = {
  book_id: number;
  date_from: string | null;
  date_to: string | null;
  category_ids: number[] | null;
};

export type CategorySpendRow = {
  category_id: number;
  category_name: string;
  total_minor: number;
};

export type PayeeTotalsReportInput = {
  book_id: number;
  date_from: string | null;
  date_to: string | null;
  payee_ids: number[] | null;
};

export type PayeeTotalRow = {
  payee_id: number;
  payee_name: string;
  total_minor: number;
};

export type RealizedGainEntry = {
  tx_id: number;
  txn_date: string;
  commodity_id: number;
  quantity_minor: number;
  proceeds_minor: number;
  quote_commodity_id: number | null;
  cost_basis_minor: number;
  gain_loss_minor: number;
  proceeds_missing: boolean;
};

export type UnrealizedGainEntry = {
  account_id: number;
  account_name: string;
  account_type: string;
  commodity_id: number;
  commodity_name: string;
  value_minor: number;
  cost_basis_minor: number;
  unrealized_gain_minor: number;
  price_missing: boolean;
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

export type ReportDefinitionCreateInput = {
  book_id: number;
  name: string;
  kind: string;
  query_type: string;
  query_text: string;
  params_schema: string | null;
};

export type ReportDefinitionUpdateInput = ReportDefinitionCreateInput & {
  id: number;
};

export type ReportDefinitionSummary = {
  id: number;
  book_id: number;
  name: string;
  kind: string;
  query_type: string;
  query_text: string;
  params_schema: string | null;
  created_at: string;
};

export type ReportRunCreateInput = {
  book_id: number;
  definition_id: number;
  params_hash: string;
  as_of_seq: number;
  pricing_mode: "frozen" | "latest_corrected";
  pricing_policy_id: number | null;
  pricing_policy_version: string | null;
  valuation_snapshot_id: number | null;
  pricing_resolved_at: string | null;
  result_json: string;
};

export type ReportRunSummary = {
  id: number;
  book_id: number;
  definition_id: number;
  params_hash: string;
  as_of_seq: number;
  pricing_mode: "frozen" | "latest_corrected";
  pricing_policy_id: number | null;
  pricing_policy_version: string | null;
  valuation_snapshot_id: number | null;
  pricing_resolved_at: string | null;
  result_json: string;
  created_at: string;
};

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

export async function reportCashflow(input: CashflowReportInput): Promise<CashflowRow[]> {
  return apiPost<CashflowRow[], CashflowReportInput>("/reports/cashflow", input);
}

export async function reportNetWorth(input: NetWorthReportInput): Promise<NetWorthRow[]> {
  return apiPost<NetWorthRow[], NetWorthReportInput>("/reports/net-worth", input);
}

export async function reportAccountTrends(input: AccountTrendReportInput): Promise<AccountTrendRow[]> {
  return apiPost<AccountTrendRow[], AccountTrendReportInput>("/reports/account-trends", input);
}

export async function reportCategorySpend(input: CategorySpendReportInput): Promise<CategorySpendRow[]> {
  return apiPost<CategorySpendRow[], CategorySpendReportInput>("/reports/category-spend", input);
}

export async function reportPayeeTotals(input: PayeeTotalsReportInput): Promise<PayeeTotalRow[]> {
  return apiPost<PayeeTotalRow[], PayeeTotalsReportInput>("/reports/payee-totals", input);
}

export async function realizedGainsReport(
  bookId: number,
  dateFrom: string | null,
  dateTo: string | null,
  costBasisProfileId?: number | null
): Promise<RealizedGainEntry[]> {
  const path = `/reports/realized-gains${buildQuery({
    book_id: bookId,
    date_from: dateFrom,
    date_to: dateTo,
    cost_basis_profile_id: costBasisProfileId ?? null,
  })}`;
  return apiGet<RealizedGainEntry[]>(path);
}

export async function unrealizedGainsReport(
  bookId: number,
  baseCommodityId: number,
  asOfDate: string | null,
  costBasisProfileId?: number | null
): Promise<UnrealizedGainEntry[]> {
  const path = `/reports/unrealized-gains${buildQuery({
    book_id: bookId,
    base_commodity_id: baseCommodityId,
    as_of_date: asOfDate,
    cost_basis_profile_id: costBasisProfileId ?? null,
  })}`;
  return apiGet<UnrealizedGainEntry[]>(path);
}

export async function investmentPerformanceReport(
  bookId: number,
  baseCommodityId: number,
  asOfDate: string | null,
  costBasisProfileId?: number | null
): Promise<PortfolioPerformanceSummary> {
  const path = `/reports/investment-performance${buildQuery({
    book_id: bookId,
    base_commodity_id: baseCommodityId,
    as_of_date: asOfDate,
    cost_basis_profile_id: costBasisProfileId ?? null,
  })}`;
  return apiGet<PortfolioPerformanceSummary>(path);
}

export async function accountValuationReport(
  bookId: number,
  baseCommodityId: number,
  asOfDate: string | null,
  costBasisProfileId?: number | null
): Promise<AccountValuationRow[]> {
  const path = `/reports/account-valuation${buildQuery({
    book_id: bookId,
    base_commodity_id: baseCommodityId,
    as_of_date: asOfDate,
    cost_basis_profile_id: costBasisProfileId ?? null,
  })}`;
  return apiGet<AccountValuationRow[]>(path);
}

export async function currencyExposureReport(bookId: number, asOfDate: string | null): Promise<CurrencyExposureRow[]> {
  const path = `/reports/currency-exposure${buildQuery({ book_id: bookId, as_of_date: asOfDate })}`;
  return apiGet<CurrencyExposureRow[]>(path);
}

export async function listReportDefinitions(bookId: number): Promise<ReportDefinitionSummary[]> {
  return apiGet<ReportDefinitionSummary[]>(`/reports/definitions${buildQuery({ book_id: bookId })}`);
}

export async function getReportDefinition(definitionId: number): Promise<ReportDefinitionSummary> {
  return apiGet<ReportDefinitionSummary>(`/reports/definitions/${definitionId}`);
}

export async function saveReportDefinition(
  input: ReportDefinitionCreateInput | ReportDefinitionUpdateInput
): Promise<ReportDefinitionSummary> {
  if ("id" in input) {
    return apiPut<ReportDefinitionSummary, ReportDefinitionUpdateInput>(`/reports/definitions/${input.id}`, input);
  }
  return apiPost<ReportDefinitionSummary, ReportDefinitionCreateInput>("/reports/definitions", input);
}

export async function deleteReportDefinition(definitionId: number): Promise<void> {
  return apiDelete<void>(`/reports/definitions/${definitionId}`);
}

export async function listReportRuns(bookId: number, definitionId?: number | null): Promise<ReportRunSummary[]> {
  return apiGet<ReportRunSummary[]>(
    `/reports/runs${buildQuery({ book_id: bookId, definition_id: definitionId ?? null })}`
  );
}

export async function createReportRun(input: ReportRunCreateInput): Promise<ReportRunSummary> {
  return apiPost<ReportRunSummary, ReportRunCreateInput>("/reports/runs", input);
}
