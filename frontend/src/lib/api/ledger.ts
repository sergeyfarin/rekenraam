import type { components } from '$lib/api/schema';
import { APIClientError, apiClient, toAPIClientError, toNetworkError } from '$lib/api/client';

export type BalanceQuantity = components['schemas']['BalanceQuantity'];
export type AccountBalancesResponse = components['schemas']['AccountBalancesResponse'];
export type CategoryTotalsResponse = components['schemas']['CategoryTotalsResponse'];
export type NetWorthResponse = components['schemas']['NetWorthResponse'];
export type NetWorthSeriesResponse = components['schemas']['NetWorthSeriesResponse'];

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

export type NetWorthSeriesOptions = {
  startDate: string;
  endDate: string;
  bucket: 'day' | 'week' | 'month' | 'quarter' | 'year';
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

export function netWorthSeriesQueryOptions(options: NetWorthSeriesOptions) {
  return {
    queryKey: [...ledgerQueryKey, 'reports', 'net-worth', options] as const,
    queryFn: () => getNetWorthSeries(options),
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

export async function getNetWorthSeries(options: NetWorthSeriesOptions): Promise<NetWorthSeriesResponse> {
  try {
    const { data, error, response } = await apiClient.GET('/api/v1/reports/net-worth', {
      params: {
        query: {
          start_date: options.startDate,
          end_date: options.endDate,
          bucket: options.bucket
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
