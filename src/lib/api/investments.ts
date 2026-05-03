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

export async function getPositions<T>(asOfDate?: string | null): Promise<T> {
  const path = `/investments/positions${buildQuery({ book_id: 1, as_of_date: asOfDate ?? null })}`;
  return apiGet<T>(path);
}

export async function convertPositions<T>(_positions: unknown[], baseCommodityId: number, asOfDate?: string | null): Promise<T> {
  const path = `/investments/positions/converted${buildQuery({ book_id: 1, base_commodity_id: baseCommodityId, as_of_date: asOfDate ?? null })}`;
  return apiGet<T>(path);
}

export async function listLotsWithHoldingPeriod<T>(accountId?: number | null, commodityId?: number | null, asOfDate?: string | null): Promise<T> {
  const path = `/investments/lots${buildQuery({ book_id: 1, account_id: accountId ?? null, commodity_id: commodityId ?? null, as_of_date: asOfDate ?? null })}`;
  return apiGet<T>(path);
}

export async function buyCommodity(input: Record<string, unknown>): Promise<void> {
  await apiPost<void, Record<string, unknown>>("/investments/buy", input);
}

export async function sellCommodity(input: Record<string, unknown>): Promise<void> {
  const payload = {
    ...input,
    cash_amount_minor: input.proceeds_minor,
    lot_strategy: input.lot_allocation_method,
  };
  await apiPost<void, Record<string, unknown>>("/investments/sell", payload);
}

export async function createDividendTransaction(input: Record<string, unknown>): Promise<void> {
  await apiPost<void, Record<string, unknown>>("/investments/dividend", input);
}