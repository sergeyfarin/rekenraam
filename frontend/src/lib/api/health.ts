import type { components } from '$lib/api/schema';
import { APIClientError, apiClient, toAPIClientError } from '$lib/api/client';

export type HealthResponse = components['schemas']['HealthResponse'];

export const healthQueryKey = ['api', 'health'] as const;

export function healthQueryOptions() {
  return {
    queryKey: healthQueryKey,
    queryFn: getHealth,
    staleTime: 30_000
  };
}

async function getHealth(): Promise<HealthResponse> {
  try {
    const { data, error, response } = await apiClient.GET('/api/v1/health');

    if (data !== undefined) {
      return data;
    }

    throw toAPIClientError(response, error, 'Unable to read backend health.');
  } catch (error) {
    if (error instanceof APIClientError) {
      throw error;
    }

    throw new Error('Unable to read backend health.');
  }
}