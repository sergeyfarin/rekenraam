import type { components } from '$lib/api/schema';
import { APIClientError, apiClient, toAPIClientError, toNetworkError } from '$lib/api/client';

export type ForecastBalancesResponse = components['schemas']['ForecastBalancesResponse'];
export type ForecastEventsResponse = components['schemas']['ForecastEventsResponse'];
export type ForecastEvent = components['schemas']['ForecastEvent'];

export interface ForecastQuery {
  horizonDays?: number;
  accountIDs?: number[];
  includeDescendants?: boolean;
  reportingCurrencyID?: number;
  fxMethod?: 'constant_as_of';
}

export interface ForecastEventQuery extends ForecastQuery {
  date: string;
  basisToken: string;
  detailAccountID?: number;
  detailCommodityID?: number;
  limit?: number;
  cursor?: string;
}

export const forecastQueryKey = ['api', 'forecasts'] as const;
export const forecastBalancesQueryKey = [...forecastQueryKey, 'balances'] as const;
export const forecastEventsQueryKey = [...forecastQueryKey, 'balance-events'] as const;

export function normalizeForecastQuery(query: ForecastQuery = {}) {
  return {
    horizon_days: query.horizonDays,
    account_id: query.accountIDs ? [...new Set(query.accountIDs)].sort((a, b) => a - b) : undefined,
    include_descendants: query.includeDescendants,
    reporting_currency_id: query.reportingCurrencyID,
    fx_method: query.fxMethod
  };
}

export function forecastBalancesQueryOptions(query: ForecastQuery = {}) {
  const normalized = normalizeForecastQuery(query);
  return {
    queryKey: [...forecastBalancesQueryKey, normalized] as const,
    queryFn: () => getForecastBalances(query),
    staleTime: 5_000
  };
}

export function forecastEventsInfiniteQueryOptions(query: Omit<ForecastEventQuery, 'cursor'>) {
  const normalized = normalizeForecastQuery(query);
  const detail = {
    date: query.date,
    basis_token: query.basisToken,
    detail_account_id: query.detailAccountID,
    detail_commodity_id: query.detailCommodityID,
    limit: query.limit ?? 50
  };
  return {
    queryKey: [...forecastEventsQueryKey, normalized, detail] as const,
    initialPageParam: '',
    queryFn: ({ pageParam }: { pageParam: string }) => getForecastEvents({ ...query, cursor: pageParam || undefined }),
    getNextPageParam: (lastPage: ForecastEventsResponse) => lastPage.next_cursor || null,
    retry: shouldRetryForecastEvents,
    staleTime: 10_000
  };
}

export function shouldRetryForecastEvents(failureCount: number, error: unknown): boolean {
  if (error instanceof APIClientError && error.code === 'FORECAST_BASIS_CHANGED') return false;
  return failureCount < 1;
}

export async function getForecastBalances(query: ForecastQuery = {}): Promise<ForecastBalancesResponse> {
  try {
    const { data, error, response } = await apiClient.GET('/api/v1/forecasts/balances', {
      params: { query: normalizeForecastQuery(query) }
    });
    if (data !== undefined) return data;
    throw toAPIClientError(response, error);
  } catch (error) {
    if (error instanceof APIClientError) throw error;
    throw toNetworkError(error);
  }
}

export async function getForecastEvents(query: ForecastEventQuery): Promise<ForecastEventsResponse> {
  try {
    const { data, error, response } = await apiClient.GET('/api/v1/forecasts/balance-events', {
      params: {
        query: {
          ...normalizeForecastQuery(query),
          date: query.date,
          basis_token: query.basisToken,
          detail_account_id: query.detailAccountID,
          detail_commodity_id: query.detailCommodityID,
          limit: query.limit,
          cursor: query.cursor
        }
      }
    });
    if (data !== undefined) return data;
    throw toAPIClientError(response, error);
  } catch (error) {
    if (error instanceof APIClientError) throw error;
    throw toNetworkError(error);
  }
}
