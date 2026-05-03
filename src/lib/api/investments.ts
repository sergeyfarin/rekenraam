import { apiGetWithTauriFallback, invokeTauri } from "$lib/api/client";

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
  return apiGetWithTauriFallback<T>(path, "get_positions", { bookId: 1, asOfDate: asOfDate ?? null });
}

export async function convertPositions<T>(positions: unknown[], baseCommodityId: number, asOfDate?: string | null): Promise<T> {
  const path = `/investments/positions/converted${buildQuery({ book_id: 1, base_commodity_id: baseCommodityId, as_of_date: asOfDate ?? null })}`;
  return apiGetWithTauriFallback<T>(path, "convert_positions", { positions, baseCommodityId, asOfDate: asOfDate ?? null });
}

export async function listLotsWithHoldingPeriod<T>(accountId?: number | null, commodityId?: number | null, asOfDate?: string | null): Promise<T> {
  const path = `/investments/lots${buildQuery({ book_id: 1, account_id: accountId ?? null, commodity_id: commodityId ?? null, as_of_date: asOfDate ?? null })}`;
  return apiGetWithTauriFallback<T>(path, "list_lots_with_holding_period", {
    accountId,
    commodityId,
    asOfDate: asOfDate ?? null,
  });
}

export async function buyCommodity(input: Record<string, unknown>): Promise<void> {
  await invokeTauri("buy_commodity", { input });
}

export async function sellCommodity(input: Record<string, unknown>): Promise<void> {
  await invokeTauri("sell_commodity", { input });
}

export async function createDividendTransaction(input: Record<string, unknown>): Promise<void> {
  await invokeTauri("create_dividend_transaction", { input });
}