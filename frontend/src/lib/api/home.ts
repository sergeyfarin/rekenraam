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
    refetchOnWindowFocus: true,
    // A read the book changed under is shown as such and read again (#478).
    refetchInterval: (query: { state: { data?: Home } }) => (query.state.data?.consistent === false ? 2_000 : false)
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

export type HomeLayout = components['schemas']['HomeLayout'];
export type HomeLayoutShortcut = components['schemas']['HomeLayoutShortcut'];
export type HomeLayoutAccount = components['schemas']['HomeLayoutAccount'];
export type HomeLayoutState = components['schemas']['HomeLayoutState'];
export type HomeWidgetID = components['schemas']['HomeWidgetID'];
export type HomeWidgetOptions = components['schemas']['HomeWidgetOptions'];

export const homeLayoutQueryOptions = {
  queryKey: ['api', 'home-layout'] as const,
  staleTime: 0,
  queryFn: async (): Promise<HomeLayoutState> => {
    try {
      const { data, error, response } = await apiClient.GET('/api/v1/home/layout');
      if (data !== undefined) return data;
      throw toAPIClientError(response, error);
    } catch (error) {
      if (error instanceof APIClientError) throw error;
      throw toNetworkError(error);
    }
  }
};

export async function putHomeLayout(expectedRevision: number, layout: HomeLayout | null, csrfToken: string): Promise<HomeLayoutState> {
  try {
    const { data, error, response } = await apiClient.PUT('/api/v1/home/layout', {
      params: { header: { 'X-CSRF-Token': csrfToken } },
      body: { expected_revision: expectedRevision, layout }
    });
    if (data !== undefined) return data;
    throw toAPIClientError(response, error);
  } catch (error) {
    if (error instanceof APIClientError) throw error;
    throw toNetworkError(error);
  }
}
