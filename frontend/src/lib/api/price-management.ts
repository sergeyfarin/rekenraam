import type { components } from '#lib/api/schema.js';
import { APIClientError, apiClient, toAPIClientError, toNetworkError } from '#lib/api/client.ts';
import type { QueryClient } from '@tanstack/svelte-query';
import { currencySettingsPageQueryKey } from '#lib/api/settings.ts';
import { reportsQueryKey } from '#lib/api/reports.ts';
import { forecastQueryKey } from '#lib/api/forecast.ts';
import { investmentPositionsQueryKey, investmentGainsQueryKey, investmentReturnsQueryKey } from '#lib/api/investments.ts';

export type PricingWorkspace = components['schemas']['PricingWorkspaceResponse'];
export type PricingCommodity = components['schemas']['PricingCommodityResponse'];
export type PriceImpact = components['schemas']['PriceImpactResponse'];
export type PriceHistory = components['schemas']['PriceHistoryResponse'];
export type ManualPriceRequest = components['schemas']['ManualPriceRequest'];
export type CorrectPriceRequest = components['schemas']['CorrectPriceRequest'];
export type PriceObservation = components['schemas']['PriceObservationResponse'];
export const priceManagementQueryKey = ['api', 'pricing', 'management'] as const;

async function pricingRequest<T>(request: Promise<{ data?: T; error?: unknown; response: Response }>): Promise<T> {
  try {
    const { data, error, response } = await request;
    if (data !== undefined) return data;
    throw toAPIClientError(response, error);
  } catch (error) {
    if (error instanceof APIClientError) throw error;
    throw toNetworkError(error);
  }
}
export function pricingWorkspaceQueryOptions(asOf: string) {
  return { queryKey: [...priceManagementQueryKey, 'workspace', asOf], queryFn: () => getPricingWorkspace(asOf), staleTime: 0 };
}
export function getPricingWorkspace(asOf: string): Promise<PricingWorkspace> {
  return pricingRequest(apiClient.GET('/api/v1/pricing/workspace', { params: { query: { as_of: asOf || undefined } } }));
}
export function priceHistoryQueryOptions(base: number, quote: number, includeVoided: boolean) {
  return {
    queryKey: [...priceManagementQueryKey, 'history', base, quote, includeVoided],
    initialPageParam: '',
    queryFn: ({ pageParam }: { pageParam: string }) => getPriceHistory(base, quote, includeVoided, pageParam),
    getNextPageParam: (page: PriceHistory) => page.next_cursor || undefined,
    enabled: base > 0 && quote > 0 && base !== quote,
    staleTime: 0
  };
}
export function getPriceHistory(base: number, quote: number, includeVoided: boolean, cursor = ''): Promise<PriceHistory> {
  return pricingRequest(apiClient.GET('/api/v1/pricing/history', {
    params: { query: { base_commodity_id: base, quote_commodity_id: quote, include_voided: includeVoided, cursor: cursor || undefined, limit: 50 } }
  }));
}
export function getPriceImpact(id: number): Promise<PriceImpact> {
  return pricingRequest(apiClient.GET('/api/v1/pricing/prices/{price_id}/impact', { params: { path: { price_id: id } } }));
}
export function createManualPrice(input: ManualPriceRequest, csrfToken: string, requestKey: string): Promise<PriceObservation> {
  return pricingRequest(apiClient.POST('/api/v1/pricing/manual', { params: { header: { 'X-CSRF-Token': csrfToken, 'Idempotency-Key': requestKey } }, body: input }));
}
export function correctPrice(id: number, input: CorrectPriceRequest, csrfToken: string) {
  return pricingRequest(apiClient.POST('/api/v1/pricing/prices/{price_id}/correct', { params: { path: { price_id: id }, header: { 'X-CSRF-Token': csrfToken } }, body: input }));
}
export function withdrawPrice(id: number, reason: string, expectedImpact: string, csrfToken: string) {
  return pricingRequest(apiClient.POST('/api/v1/pricing/prices/{price_id}/void', {
    params: { path: { price_id: id }, header: { 'X-CSRF-Token': csrfToken } },
    body: { void_reason: reason, change_reason: reason, expected_impact: expectedImpact }
  }));
}
export async function invalidatePriceReads(client: QueryClient) {
  await Promise.all([priceManagementQueryKey, currencySettingsPageQueryKey, reportsQueryKey, forecastQueryKey,
    investmentPositionsQueryKey, investmentGainsQueryKey, investmentReturnsQueryKey].map(queryKey => client.invalidateQueries({ queryKey })));
}
