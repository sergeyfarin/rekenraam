import type { components } from '$lib/api/schema';
import { APIClientError, apiClient, toAPIClientError, toNetworkError } from '$lib/api/client';

export type SetupStatusResponse = components['schemas']['SetupStatusResponse'];
export type CreateOwnerRequest = components['schemas']['CreateOwnerRequest'];
export type CreateOwnerResponse = components['schemas']['CreateOwnerResponse'];

export const setupStatusQueryKey = ['api', 'setup', 'status'] as const;

export function setupStatusQueryOptions() {
  return {
    queryKey: setupStatusQueryKey,
    queryFn: getSetupStatus,
    staleTime: 10_000
  };
}

export async function getSetupStatus(): Promise<SetupStatusResponse> {
  try {
    const { data, error, response } = await apiClient.GET('/api/v1/setup/status');

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

export async function createOwner(input: CreateOwnerRequest): Promise<CreateOwnerResponse> {
  try {
    const { data, error, response } = await apiClient.POST('/api/v1/setup/owner', {
      body: input
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
