import type { components } from '#lib/api/schema.js';
import { APIClientError, apiClient, toAPIClientError, toNetworkError } from '#lib/api/client.ts';
import { reportsQueryKey } from '#lib/api/reports.ts';

export type OverviewAmount = components['schemas']['OverviewAmount'];
export type OverviewReportCard = components['schemas']['OverviewReportCard'];
export const planningOverviewQueryKey = ['api', 'planning', 'overview'] as const;

async function getOverview<T>(request: () => Promise<{ data?: T; error?: unknown; response: Response }>): Promise<T> {
  try {
    const { data, error, response } = await request();
    if (data !== undefined) return data;
    throw toAPIClientError(response, error);
  } catch (error) {
    if (error instanceof APIClientError) throw error;
    throw toNetworkError(error);
  }
}

export function planningOverviewQueryOptions(ownerToday: string) {
  return {
    queryKey: [...planningOverviewQueryKey, ownerToday] as const,
    queryFn: () => getOverview(() => apiClient.GET('/api/v1/planning/overview')),
    refetchOnMount: 'always' as const,
    refetchOnWindowFocus: true
  };
}

export function reportsOverviewQueryOptions(ownerToday: string) {
  return {
    queryKey: [...reportsQueryKey, 'overview', ownerToday] as const,
    queryFn: () => getOverview(() => apiClient.GET('/api/v1/reports/overview')),
    refetchOnMount: 'always' as const,
    refetchOnWindowFocus: true
  };
}
