import type { components } from '$lib/api/schema';
import { APIClientError, apiClient, toAPIClientError, toNetworkError } from '$lib/api/client';
import type { InfiniteData } from '@tanstack/svelte-query';

export type TransactionResponse = components['schemas']['TransactionResponse'];
export type TransactionsResponse = components['schemas']['TransactionsResponse'];
export type AccountRegisterResponse = components['schemas']['AccountRegisterResponse'];
export type TransactionRequest = components['schemas']['TransactionRequest'];
export type TransactionLifecycleRequest = components['schemas']['TransactionLifecycleRequest'];

export type TransactionListOptions = {
  accountID?: number;
  categoryID?: number;
  payeeID?: number;
  q?: string;
  status?: 'draft' | 'posted' | 'voided';
  kind?: 'ordinary' | 'transfer' | 'investment' | 'opening_balance' | 'adjustment';
  needsReview?: boolean;
  afterDate?: string;
  beforeDate?: string;
  limit?: number;
  cursor?: string;
};

export type TransactionsInfiniteData = InfiniteData<TransactionsResponse, string>;

export const transactionsQueryKey = ['api', 'transactions'] as const;

export function transactionsQueryOptions(options: TransactionListOptions = {}) {
  return {
    queryKey: [...transactionsQueryKey, options] as const,
    queryFn: () => getTransactions(options),
    staleTime: 5_000
  };
}

export function transactionsInfiniteQueryOptions(options: Omit<TransactionListOptions, 'cursor'> = {}) {
  return {
    queryKey: [...transactionsQueryKey, 'infinite', options] as const,
    queryFn: ({ pageParam }: { pageParam: string }) =>
      getTransactions({ ...options, cursor: pageParam || undefined }),
    initialPageParam: '',
    getNextPageParam: (lastPage: TransactionsResponse) => lastPage.next_cursor || null,
    staleTime: 5_000
  };
}

export async function getTransactions(options: TransactionListOptions = {}): Promise<TransactionsResponse> {
  try {
    const { data, error, response } = await apiClient.GET('/api/v1/transactions', {
      params: {
        query: {
          account_id: options.accountID,
          category_id: options.categoryID,
          payee_id: options.payeeID,
          q: options.q || undefined,
          status: options.status,
          kind: options.kind,
          needs_review: options.needsReview,
          after_date: options.afterDate,
          before_date: options.beforeDate,
          limit: options.limit,
          cursor: options.cursor
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

export async function getAccountRegister(accountID: number, options: Omit<TransactionListOptions, 'accountID' | 'payeeID' | 'q' | 'kind'> = {}): Promise<AccountRegisterResponse> {
  try {
    const { data, error, response } = await apiClient.GET('/api/v1/accounts/{account_id}/register', {
      params: {
        path: {
          account_id: accountID
        },
        query: {
          status: options.status,
          after_date: options.afterDate,
          before_date: options.beforeDate,
          limit: options.limit,
          cursor: options.cursor
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

export async function getTransaction(transactionID: number): Promise<TransactionResponse> {
  try {
    const { data, error, response } = await apiClient.GET('/api/v1/transactions/{transaction_id}', {
      params: {
        path: {
          transaction_id: transactionID
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

export async function createTransaction(input: TransactionRequest, csrfToken: string): Promise<TransactionResponse> {
  try {
    const { data, error, response } = await apiClient.POST('/api/v1/transactions', {
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

export async function updateTransaction(transactionID: number, input: TransactionRequest, csrfToken: string): Promise<TransactionResponse> {
  try {
    const { data, error, response } = await apiClient.PATCH('/api/v1/transactions/{transaction_id}', {
      params: {
        path: {
          transaction_id: transactionID
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

export async function postTransaction(transactionID: number, csrfToken: string, input: TransactionLifecycleRequest = {}): Promise<TransactionResponse> {
  return transactionLifecycleMutation('/api/v1/transactions/{transaction_id}/post', transactionID, csrfToken, input);
}

export async function voidTransaction(transactionID: number, csrfToken: string, input: TransactionLifecycleRequest): Promise<TransactionResponse> {
  return transactionLifecycleMutation('/api/v1/transactions/{transaction_id}/void', transactionID, csrfToken, input);
}

export async function unvoidTransaction(transactionID: number, csrfToken: string, input: TransactionLifecycleRequest): Promise<TransactionResponse> {
  return transactionLifecycleMutation('/api/v1/transactions/{transaction_id}/unvoid', transactionID, csrfToken, input);
}

export async function softDeleteTransaction(transactionID: number, csrfToken: string, input: TransactionLifecycleRequest): Promise<TransactionResponse> {
  return transactionLifecycleMutation('/api/v1/transactions/{transaction_id}/soft-delete', transactionID, csrfToken, input);
}

export async function restoreTransaction(transactionID: number, csrfToken: string, input: TransactionLifecycleRequest): Promise<TransactionResponse> {
  return transactionLifecycleMutation('/api/v1/transactions/{transaction_id}/restore', transactionID, csrfToken, input);
}

export async function approveTransaction(transactionID: number, csrfToken: string, input: TransactionLifecycleRequest = {}): Promise<TransactionResponse> {
  return transactionLifecycleMutation('/api/v1/transactions/{transaction_id}/approve', transactionID, csrfToken, input);
}

export async function correctTransaction(transactionID: number, input: TransactionRequest, csrfToken: string): Promise<TransactionResponse> {
  try {
    const { data, error, response } = await apiClient.POST('/api/v1/transactions/{transaction_id}/correct', {
      params: {
        path: {
          transaction_id: transactionID
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

export async function deleteDraftTransaction(transactionID: number, csrfToken: string): Promise<void> {
  try {
    const { error, response } = await apiClient.DELETE('/api/v1/transactions/{transaction_id}', {
      params: {
        path: {
          transaction_id: transactionID
        },
        header: {
          'X-CSRF-Token': csrfToken
        }
      }
    });

    if (response.ok) {
      return;
    }

    throw toAPIClientError(response, error);
  } catch (error) {
    if (error instanceof APIClientError) {
      throw error;
    }

    throw toNetworkError(error);
  }
}

async function transactionLifecycleMutation(
  path:
    | '/api/v1/transactions/{transaction_id}/post'
    | '/api/v1/transactions/{transaction_id}/void'
    | '/api/v1/transactions/{transaction_id}/unvoid'
    | '/api/v1/transactions/{transaction_id}/soft-delete'
    | '/api/v1/transactions/{transaction_id}/restore'
    | '/api/v1/transactions/{transaction_id}/approve',
  transactionID: number,
  csrfToken: string,
  input: TransactionLifecycleRequest
): Promise<TransactionResponse> {
  try {
    const { data, error, response } = await apiClient.POST(path, {
      params: {
        path: {
          transaction_id: transactionID
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
