import type { components } from '#lib/api/schema.js';
import { APIClientError, apiClient, toAPIClientError, toNetworkError } from '#lib/api/client.ts';

export type Home = components['schemas']['HomeResponse'];
export type HomeHeadline = components['schemas']['HomeHeadline'];
export type HomeValuationInput = components['schemas']['HomeValuationInput'];
export type HomeCommodity = components['schemas']['HomeReportingCurrency'];
export type HomeWidgets = components['schemas']['HomeWidgets'];

/** Every Home summary shares this prefix, so one invalidation refreshes them all. */
export const homeQueryKey = ['api', 'home'] as const;

/** Keyed by owner-local today: the date rolling over refetches the month. */
export function homeQueryOptions(ownerToday: string) {
  return {
    queryKey: [...homeQueryKey, ownerToday] as const,
    queryFn: getHome,
    staleTime: 30_000,
    refetchOnWindowFocus: true
  };
}

export async function getHome(): Promise<Home> {
  try {
    const { data, error, response } = await apiClient.GET('/api/v1/home');
    if (data !== undefined) return data;
    throw toAPIClientError(response, error);
  } catch (error) {
    if (error instanceof APIClientError) throw error;
    throw toNetworkError(error);
  }
}
