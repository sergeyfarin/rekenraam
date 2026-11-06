import type { components } from '#lib/api/schema.js';
import { APIClientError, apiClient, toAPIClientError, toNetworkError } from './client.ts';

export type AllocationResponse = components['schemas']['AllocationResponse'];
export type AllocationPoint = components['schemas']['AllocationPoint'];
export type AllocationSaveRequest = components['schemas']['AllocationSaveRequest'];
export type AllocationRevision = components['schemas']['AllocationRevision'];
export const allocationQueryKey = ['api', 'investments', 'allocation'] as const;
export type AllocationQuery = { from?: string; to?: string; reporting_currency_id?: number };

export function allocationQueryOptions(query: AllocationQuery) {
  return { queryKey: [...allocationQueryKey, query], queryFn: () => getAllocation(query), staleTime: 0, refetchOnWindowFocus: true };
}
export async function getAllocation(query: AllocationQuery): Promise<AllocationResponse> {
  try {
    const { data, error, response } = await apiClient.GET('/api/v1/investments/allocation', { params: { query } });
    if (data) return data;
    throw toAPIClientError(response, error);
  } catch (error) {
    if (error instanceof APIClientError) throw error;
    throw toNetworkError(error);
  }
}
export async function saveAllocation(body: AllocationSaveRequest, csrfToken: string): Promise<AllocationRevision> {
  try {
    const { data, error, response } = await apiClient.PUT('/api/v1/investments/allocation', { body, headers: { 'X-CSRF-Token': csrfToken } });
    if (data) return data;
    throw toAPIClientError(response, error);
  } catch (error) {
    if (error instanceof APIClientError) throw error;
    throw toNetworkError(error);
  }
}
