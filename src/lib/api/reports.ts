import { apiGet, apiPost } from "$lib/api/client";

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

export async function reportCashflow<T>(input: Record<string, unknown>): Promise<T> {
  return apiPost<T, Record<string, unknown>>("/reports/cashflow", input);
}

export async function reportCategorySpend<T>(input: Record<string, unknown>): Promise<T> {
  return apiPost<T, Record<string, unknown>>("/reports/category-spend", input);
}

export async function reportPayeeTotals<T>(input: Record<string, unknown>): Promise<T> {
  return apiPost<T, Record<string, unknown>>("/reports/payee-totals", input);
}

export async function realizedGainsReport<T>(dateFrom: string | null, dateTo: string | null): Promise<T> {
  const path = `/reports/realized-gains${buildQuery({ book_id: 1, date_from: dateFrom, date_to: dateTo })}`;
  return apiGet<T>(path);
}

export async function unrealizedGainsReport<T>(baseCommodityId: number, asOfDate: string | null): Promise<T> {
  const path = `/reports/unrealized-gains${buildQuery({ book_id: 1, base_commodity_id: baseCommodityId, as_of_date: asOfDate })}`;
  return apiGet<T>(path);
}