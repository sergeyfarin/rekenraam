import type { components } from '#lib/api/schema.js';
import { APIClientError, apiClient, toAPIClientError, toNetworkError } from '#lib/api/client.ts';

export type BalanceQuantity = components['schemas']['BalanceQuantity'];
export type AccountBalancesResponse = components['schemas']['AccountBalancesResponse'];
export type CategoryTotalsResponse = components['schemas']['CategoryTotalsResponse'];
export type NetWorthResponse = components['schemas']['NetWorthResponse'];

export type LedgerStatus = components['schemas']['LedgerStatus'];

export type AccountBalancesOptions = {
  asOf?: string;
  status?: LedgerStatus;
  includeSystem?: boolean;
};

export type CategoryTotalsOptions = {
  afterDate?: string;
  beforeDate?: string;
  status?: LedgerStatus;
  categoryType?: 'income' | 'expense';
};

export type NetWorthOptions = {
  asOf?: string;
  status?: LedgerStatus;
};

export const ledgerQueryKey = ['api', 'ledger'] as const;

export function accountBalancesQueryOptions(options: AccountBalancesOptions = {}) {
  return {
    queryKey: [...ledgerQueryKey, 'account-balances', options] as const,
    queryFn: () => getAccountBalances(options),
    staleTime: 5_000
  };
}

export function categoryTotalsQueryOptions(options: CategoryTotalsOptions = {}) {
  return {
    queryKey: [...ledgerQueryKey, 'category-totals', options] as const,
    queryFn: () => getCategoryTotals(options),
    staleTime: 5_000
  };
}

export function netWorthQueryOptions(options: NetWorthOptions = {}) {
  return {
    queryKey: [...ledgerQueryKey, 'net-worth', options] as const,
    queryFn: () => getNetWorth(options),
    staleTime: 5_000
  };
}

export async function getAccountBalances(options: AccountBalancesOptions = {}): Promise<AccountBalancesResponse> {
  try {
    const { data, error, response } = await apiClient.GET('/api/v1/ledger/account-balances', {
      params: {
        query: {
          as_of: options.asOf,
          status: options.status,
          include_system: options.includeSystem
        }
      }
    });

    if (data !== undefined) {
      return data;
    }

    throw toAPIClientError(response, error);
  } catch (error) {
    if (error instanceof APIClientError) {
      throw error;
    }

    throw toNetworkError(error);
  }
}

export async function getCategoryTotals(options: CategoryTotalsOptions = {}): Promise<CategoryTotalsResponse> {
  try {
    const { data, error, response } = await apiClient.GET('/api/v1/ledger/category-totals', {
      params: {
        query: {
          after_date: options.afterDate,
          before_date: options.beforeDate,
          status: options.status,
          category_type: options.categoryType
        }
      }
    });

    if (data !== undefined) {
      return data;
    }

    throw toAPIClientError(response, error);
  } catch (error) {
    if (error instanceof APIClientError) {
      throw error;
    }

    throw toNetworkError(error);
  }
}

export async function getNetWorth(options: NetWorthOptions = {}): Promise<NetWorthResponse> {
  try {
    const { data, error, response } = await apiClient.GET('/api/v1/ledger/net-worth', {
      params: {
        query: {
          as_of: options.asOf,
          status: options.status
        }
      }
    });

    if (data !== undefined) {
      return data;
    }

    throw toAPIClientError(response, error);
  } catch (error) {
    if (error instanceof APIClientError) {
      throw error;
    }

    throw toNetworkError(error);
  }
}

export type LedgerOverview = components['schemas']['LedgerOverviewResponse'];
export type LedgerAccount = components['schemas']['LedgerAccount'];
async function readLedger(accountID?: number, asOf?: string, cursor?: string): Promise<LedgerOverview> {
  try {
    const result = accountID
      ? await apiClient.GET('/api/v1/ledger/accounts/{account_id}', { params: { path: { account_id: accountID }, query: { as_of: asOf } } })
      : await apiClient.GET('/api/v1/ledger/overview', { params: { query: { as_of: asOf, cursor } } });
    if (result.data !== undefined) return result.data;
    throw toAPIClientError(result.response, result.error);
  } catch (error) { if (error instanceof APIClientError) throw error; throw toNetworkError(error); }
}
export function ledgerAccountQueryOptions(accountID: number, asOf?: string) {
  return { queryKey: [...ledgerQueryKey, 'account', accountID, asOf], queryFn: () => readLedger(accountID, asOf), staleTime: 10_000 };
}
export function ledgerOverviewQueryOptions(asOf?: string) {
  return { queryKey: [...ledgerQueryKey, 'overview', asOf], queryFn: async () => {
    const first = await readLedger(undefined, asOf);
    const result = { ...first, accounts: [...first.accounts] };
    let cursor = first.next_cursor;
    const seen = new Set(first.accounts.map(row => row.account.id));
    while (cursor) {
      const next = await readLedger(undefined, first.as_of, cursor);
      for (const row of next.accounts) if (!seen.has(row.account.id)) { seen.add(row.account.id); result.accounts.push(row); }
      if (next.next_cursor === cursor) throw new APIClientError({ status: 500, code: 'INTERNAL_ERROR' });
      cursor = next.next_cursor;
    }
    return { ...result, next_cursor: null };
  }, staleTime: 10_000 };
}
