import type { components } from '$lib/api/schema';
import { APIClientError, apiClient, toAPIClientError, toNetworkError } from '$lib/api/client';

export type PayeeResponse = components['schemas']['PayeeResponse'];
export type PayeesResponse = components['schemas']['PayeesResponse'];
export type PayeeRequest = components['schemas']['PayeeRequest'];
export type PayeeLifecycleRequest = components['schemas']['PayeeLifecycleRequest'];

export const payeesQueryKey = ['api', 'payees'] as const;

export function payeesQueryOptions(options: { includeArchived?: boolean; q?: string } = {}) {
  return {
    queryKey: [...payeesQueryKey, options] as const,
    queryFn: () => getPayees(options),
    staleTime: 10_000
  };
}

export async function getPayees(options: { includeArchived?: boolean; q?: string } = {}): Promise<PayeesResponse> {
  try {
    const { data, error, response } = await apiClient.GET('/api/v1/payees', {
      params: {
        query: {
          include_archived: options.includeArchived || undefined,
          q: options.q || undefined
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

export async function getPayee(payeeID: number): Promise<PayeeResponse> {
  try {
    const { data, error, response } = await apiClient.GET('/api/v1/payees/{payee_id}', {
      params: {
        path: {
          payee_id: payeeID
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

export async function createPayee(input: PayeeRequest, csrfToken: string): Promise<PayeeResponse> {
  try {
    const { data, error, response } = await apiClient.POST('/api/v1/payees', {
      params: {
        header: {
          'X-CSRF-Token': csrfToken
        }
      },
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

export async function updatePayee(payeeID: number, input: PayeeRequest, csrfToken: string): Promise<PayeeResponse> {
  try {
    const { data, error, response } = await apiClient.PATCH('/api/v1/payees/{payee_id}', {
      params: {
        path: {
          payee_id: payeeID
        },
        header: {
          'X-CSRF-Token': csrfToken
        }
      },
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

export async function archivePayee(payeeID: number, csrfToken: string, input: PayeeLifecycleRequest = {}): Promise<PayeeResponse> {
  return payeeLifecycleMutation('/api/v1/payees/{payee_id}/archive', payeeID, csrfToken, input);
}

export async function restorePayee(payeeID: number, csrfToken: string, input: PayeeLifecycleRequest = {}): Promise<PayeeResponse> {
  return payeeLifecycleMutation('/api/v1/payees/{payee_id}/restore', payeeID, csrfToken, input);
}

async function payeeLifecycleMutation(
  path: '/api/v1/payees/{payee_id}/archive' | '/api/v1/payees/{payee_id}/restore',
  payeeID: number,
  csrfToken: string,
  input: PayeeLifecycleRequest
): Promise<PayeeResponse> {
  try {
    const { data, error, response } = await apiClient.POST(path, {
      params: {
        path: {
          payee_id: payeeID
        },
        header: {
          'X-CSRF-Token': csrfToken
        }
      },
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
