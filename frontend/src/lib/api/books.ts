import type { components } from '$lib/api/schema';
import { APIClientError, apiClient, toAPIClientError, toNetworkError } from '$lib/api/client';

export type BookResponse = components['schemas']['BookResponse'];

export const currentBookQueryKey = ['api', 'books', 'current'] as const;

export function currentBookQueryOptions() {
  return {
    queryKey: currentBookQueryKey,
    queryFn: getCurrentBook,
    staleTime: 10_000
  };
}

export async function getCurrentBook(): Promise<BookResponse> {
  try {
    const { data, error, response } = await apiClient.GET('/api/v1/books/current');

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
