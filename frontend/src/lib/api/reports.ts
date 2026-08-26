import type { components } from '$lib/api/schema';
import { APIClientError, apiClient, toAPIClientError, toNetworkError } from '$lib/api/client';

export type ReportFilters = components['schemas']['ReportFilters'];
export type SpendingReportResponse = components['schemas']['SpendingReportResponse'];
export type SpendingReportGroup = components['schemas']['SpendingReportGroup'];
export type SpendingReportGroupTotal = components['schemas']['SpendingReportGroupTotal'];
export type SpendingReportDrillDown = components['schemas']['SpendingReportDrillDown'];
export type NetWorthSeriesResponse = components['schemas']['NetWorthSeriesResponse'];
export type CashflowReportResponse = components['schemas']['CashflowReportResponse'];
export type CashflowReportBucket = components['schemas']['CashflowReportBucket'];

export type SpendingGroupBy = 'category' | 'payee';
export type SpendingMode = 'spending' | 'income';

export type SpendingOptions = {
  startDate: string;
  endDate: string;
  groupBy: SpendingGroupBy;
  mode?: SpendingMode;
  categoryIDs?: number[];
  payeeIDs?: number[];
  accountIDs?: number[];
  includeDescendants?: boolean;
  commodityIDs?: number[];
  /**
   * Optional. Denominates the report in this currency *in addition to* the
   * exact per-commodity figures, which are unchanged either way.
   */
  reportingCurrencyID?: number | null;
};

export type CashflowOptions = {
  startDate: string;
  endDate: string;
  bucket: 'day' | 'week' | 'month' | 'quarter' | 'year';
  /** Selects the cash scope. Empty means the named liquid-cash default. */
  accountIDs?: number[];
  includeDescendants?: boolean;
  commodityIDs?: number[];
  /**
   * Optional. Denominates the report in this currency *in addition to* the
   * exact per-commodity figures, which are unchanged either way.
   */
  reportingCurrencyID?: number | null;
};

export type NetWorthSeriesOptions = {
  startDate: string;
  endDate: string;
  bucket: 'day' | 'week' | 'month' | 'quarter' | 'year';
  accountIDs?: number[];
  includeDescendants?: boolean;
  commodityIDs?: number[];
  /**
   * Optional. Denominates the report in this currency *in addition to* the
   * exact per-commodity figures, which are unchanged either way.
   */
  reportingCurrencyID?: number | null;
};

export const reportsQueryKey = ['api', 'reports'] as const;

export function netWorthSeriesQueryOptions(options: NetWorthSeriesOptions) {
  return {
    queryKey: [...reportsQueryKey, 'net-worth', options] as const,
    queryFn: () => getNetWorthSeries(options),
    staleTime: 5_000
  };
}

export function spendingQueryOptions(options: SpendingOptions) {
  return {
    queryKey: [...reportsQueryKey, 'spending', options] as const,
    queryFn: () => getSpending(options),
    staleTime: 5_000
  };
}

export function cashflowQueryOptions(options: CashflowOptions) {
  return {
    queryKey: [...reportsQueryKey, 'cashflow', options] as const,
    queryFn: () => getCashflow(options),
    staleTime: 5_000
  };
}

export async function getCashflow(options: CashflowOptions): Promise<CashflowReportResponse> {
  try {
    const { data, error, response } = await apiClient.GET('/api/v1/reports/cashflow', {
      params: {
        query: {
          start_date: options.startDate,
          end_date: options.endDate,
          bucket: options.bucket,
          account_id: options.accountIDs,
          include_descendants: options.includeDescendants,
          commodity_id: options.commodityIDs,
          reporting_currency_id: options.reportingCurrencyID ?? undefined
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

export async function getSpending(options: SpendingOptions): Promise<SpendingReportResponse> {
  try {
    const { data, error, response } = await apiClient.GET('/api/v1/reports/spending', {
      params: {
        query: {
          start_date: options.startDate,
          end_date: options.endDate,
          group_by: options.groupBy,
          mode: options.mode,
          category_id: options.categoryIDs,
          payee_id: options.payeeIDs,
          account_id: options.accountIDs,
          include_descendants: options.includeDescendants,
          commodity_id: options.commodityIDs,
          reporting_currency_id: options.reportingCurrencyID ?? undefined
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
          bucket: options.bucket,
          account_id: options.accountIDs,
          include_descendants: options.includeDescendants,
          commodity_id: options.commodityIDs,
          reporting_currency_id: options.reportingCurrencyID ?? undefined
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
