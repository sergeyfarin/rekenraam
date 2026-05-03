import { invokeTauri } from "$lib/api/client";

export async function getPositions<T>(baseCommodityId?: number | null): Promise<T> {
  return invokeTauri<T>("get_positions", { baseCommodityId });
}

export async function convertPositions<T>(positions: unknown[], baseCommodityId: number): Promise<T> {
  return invokeTauri<T>("convert_positions", { positions, baseCommodityId });
}

export async function listLotsWithHoldingPeriod<T>(accountId?: number | null): Promise<T> {
  return invokeTauri<T>("list_lots_with_holding_period", { accountId });
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