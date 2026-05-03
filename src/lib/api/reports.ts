import { invokeTauri } from "$lib/api/client";

export async function reportCashflow<T>(input: Record<string, unknown>): Promise<T> {
  return invokeTauri<T>("report_cashflow", { input });
}

export async function reportCategorySpend<T>(input: Record<string, unknown>): Promise<T> {
  return invokeTauri<T>("report_category_spend", { input });
}

export async function reportPayeeTotals<T>(input: Record<string, unknown>): Promise<T> {
  return invokeTauri<T>("report_payee_totals", { input });
}

export async function realizedGainsReport<T>(dateFrom: string | null, dateTo: string | null): Promise<T> {
  return invokeTauri<T>("realized_gains_report", { dateFrom, dateTo });
}

export async function unrealizedGainsReport<T>(baseCommodityId: number, asOfDate: string | null): Promise<T> {
  return invokeTauri<T>("unrealized_gains_report", { baseCommodityId, asOfDate });
}