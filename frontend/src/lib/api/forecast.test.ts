import { afterEach, describe, expect, it, vi } from 'vitest';
import { APIClientError, apiClient } from './client';
import { forecastBalancesQueryOptions, forecastEventsInfiniteQueryOptions, forecastQueryKey, getForecastBalances, shouldRetryForecastEvents } from './forecast';

afterEach(() => vi.restoreAllMocks());

describe('forecast API client', () => {
  it('normalizes repeated account IDs without losing explicit false', async () => {
    const get = vi.spyOn(apiClient, 'GET').mockResolvedValue({ data: { series: [], totals: [] }, response: new Response() });
    await getForecastBalances({ horizonDays: 30, accountIDs: [9, 2, 9], includeDescendants: false });
    expect(get).toHaveBeenCalledWith('/api/v1/forecasts/balances', {
      params: { query: { horizon_days: 30, account_id: [2, 9], include_descendants: false, reporting_currency_id: undefined, fx_method: undefined } }
    });
    expect(forecastBalancesQueryOptions({ accountIDs: [9, 2, 9] }).queryKey).toEqual([
      'api', 'forecasts', 'balances', { horizon_days: undefined, account_id: [2, 9], include_descendants: undefined, reporting_currency_id: undefined, fx_method: undefined }
    ]);
    expect(forecastQueryKey).toEqual(['api', 'forecasts']);
  });

  it('serializes the explicit constant-as-of FX recipe', async () => {
    const get = vi.spyOn(apiClient, 'GET').mockResolvedValue({ data: { series: [], totals: [], valuation: null, converted: null }, response: new Response() });
    await getForecastBalances({ reportingCurrencyID: 7, fxMethod: 'constant_as_of' });
    expect(get).toHaveBeenCalledWith('/api/v1/forecasts/balances', {
      params: { query: { horizon_days: undefined, account_id: undefined, include_descendants: undefined, reporting_currency_id: 7, fx_method: 'constant_as_of' } }
    });
  });

  it('continues event pages with the exact basis and detail recipe', async () => {
    const get = vi.spyOn(apiClient, 'GET')
      .mockResolvedValueOnce({ data: { items: [], next_cursor: 'next', basis_token: 'a'.repeat(64), date: '2026-09-08', total_count: 1 }, response: new Response() })
      .mockResolvedValueOnce({ data: { items: [], next_cursor: null, basis_token: 'a'.repeat(64), date: '2026-09-08', total_count: 1 }, response: new Response() });
    const options = forecastEventsInfiniteQueryOptions({ date: '2026-09-08', basisToken: 'a'.repeat(64), accountIDs: [3], detailCommodityID: 7, limit: 20 });
    const first = await options.queryFn({ pageParam: options.initialPageParam });
    const cursor = options.getNextPageParam(first);
    expect(cursor).toBe('next');
    const last = await options.queryFn({ pageParam: cursor! });
    expect(options.getNextPageParam(last)).toBeNull();
    expect(get).toHaveBeenLastCalledWith('/api/v1/forecasts/balance-events', {
      params: { query: { horizon_days: undefined, account_id: [3], include_descendants: undefined, reporting_currency_id: undefined, fx_method: undefined, date: '2026-09-08', basis_token: 'a'.repeat(64), detail_account_id: undefined, detail_commodity_id: 7, limit: 20, cursor: 'next' } }
    });
  });

  it('preserves forecast-specific stable errors', async () => {
    vi.spyOn(apiClient, 'GET').mockResolvedValue({ error: { error: { code: 'FORECAST_TOO_LARGE', message: 'large' } }, response: new Response(null, { status: 422 }) });
    await expect(getForecastBalances()).rejects.toMatchObject({ status: 422, code: 'FORECAST_TOO_LARGE' } satisfies Partial<APIClientError>);
  });

  it('does not retry a stale event basis but retries one transient event failure', () => {
    expect(shouldRetryForecastEvents(0, new APIClientError({ status: 409, code: 'FORECAST_BASIS_CHANGED' }))).toBe(false);
    expect(shouldRetryForecastEvents(0, new APIClientError({ status: 503, code: 'RESOURCE_BUSY' }))).toBe(true);
    expect(shouldRetryForecastEvents(1, new APIClientError({ status: 503, code: 'RESOURCE_BUSY' }))).toBe(false);
  });
});
