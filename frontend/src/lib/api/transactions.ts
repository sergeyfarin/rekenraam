import type { components } from '#lib/api/schema.js';
import { APIClientError, apiClient, toAPIClientError, toNetworkError } from '#lib/api/client.ts';
import type { InfiniteData } from '@tanstack/svelte-query';

export type TransactionResponse = components['schemas']['TransactionResponse'];
export type TransactionsResponse = components['schemas']['TransactionsResponse'];
export type AccountRegisterResponse = components['schemas']['AccountRegisterResponse'];
export type AccountRegisterEntryResponse = components['schemas']['AccountRegisterEntryResponse'];
export type TransactionRequest = components['schemas']['TransactionRequest'];
export type TransactionLifecycleRequest = components['schemas']['TransactionLifecycleWriteRequest'];
export type ApproveTransactionRequest = components['schemas']['TransactionLifecycleRequest'];
export type MoveRequest = components['schemas']['MoveRequest'];
export type ReconciliationImpactResponse = components['schemas']['ReconciliationImpactResponse'];
export type DeletedTransactionResponse = components['schemas']['DeletedTransactionResponse'];
export type DeletedTransactionsResponse = components['schemas']['DeletedTransactionsResponse'];

export type TransactionListOptions = {
  accountID?: number;
  categoryID?: number;
  /** Matches postings or dividend/withholding operation facts for this instrument, including corrections. */
  commodityID?: number;
  payeeID?: number;
  q?: string;
  status?: 'draft' | 'posted' | 'voided';
  kind?: 'ordinary' | 'transfer' | 'investment' | 'opening_balance' | 'adjustment';
  needsReview?: boolean;
  afterDate?: string;
  beforeDate?: string;
  /** Restricts to transactions touching an income or expense category. */
  categoryType?: 'income' | 'expense';
  /**
   * Which date `afterDate`/`beforeDate` apply to. `entry` is the basis the
   * reports sum on, and is what a spending drill-down must ask for.
   */
  dateBasis?: 'transaction' | 'entry';
  limit?: number;
  cursor?: string;
};

export type TransactionsInfiniteData = InfiniteData<TransactionsResponse, string>;
export type AccountRegisterInfiniteData = InfiniteData<AccountRegisterResponse, string>;

export const transactionsQueryKey = ['api', 'transactions'] as const;
export const accountRegisterQueryKey = ['api', 'account-register'] as const;
export const deletedTransactionsQueryKey = ['api', 'transactions-deleted'] as const;

export function transactionsQueryOptions(options: TransactionListOptions = {}) {
  return {
    queryKey: [...transactionsQueryKey, options] as const,
    queryFn: () => getTransactions(options),
    staleTime: 5_000
  };
}

/** One transaction as GET /transactions/{id} reads it, with the read-only
 *  extras only that read carries (an exchange's market comparison, #481). */
export function transactionQueryOptions(transactionID: number) {
  return {
    queryKey: [...transactionsQueryKey, 'one', transactionID] as const,
    queryFn: () => getTransaction(transactionID)
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

export type RegisterFilterOptions = Pick<TransactionListOptions, 'status' | 'afterDate' | 'beforeDate' | 'limit'>;
export type DeletedTransactionsInfiniteData = InfiniteData<DeletedTransactionsResponse, string>;

export function accountRegisterInfiniteQueryOptions(accountID: number, options: RegisterFilterOptions = {}) {
  return {
    queryKey: [...accountRegisterQueryKey, accountID, 'infinite', options] as const,
    queryFn: ({ pageParam }: { pageParam: string }) =>
      getAccountRegister(accountID, { ...options, cursor: pageParam || undefined }),
    initialPageParam: '',
    getNextPageParam: (lastPage: AccountRegisterResponse) => lastPage.next_cursor || null,
    staleTime: 5_000
  };
}

export function deletedTransactionsInfiniteQueryOptions(limit?: number) {
  return {
    queryKey: [...deletedTransactionsQueryKey, 'infinite', { limit }] as const,
    queryFn: ({ pageParam }: { pageParam: string }) =>
      getDeletedTransactions({ limit, cursor: pageParam || undefined }),
    initialPageParam: '',
    getNextPageParam: (lastPage: DeletedTransactionsResponse) => lastPage.next_cursor || null,
    staleTime: 5_000
  };
}

export async function getDeletedTransactions(options: { limit?: number; cursor?: string } = {}): Promise<DeletedTransactionsResponse> {
  try {
    const { data, error, response } = await apiClient.GET('/api/v1/transactions/deleted', {
      params: {
        query: {
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

export async function getTransactions(options: TransactionListOptions = {}): Promise<TransactionsResponse> {
  try {
    const { data, error, response } = await apiClient.GET('/api/v1/transactions', {
      params: {
        query: {
          account_id: options.accountID,
          category_id: options.categoryID,
          commodity_id: options.commodityID,
          category_type: options.categoryType,
          date_basis: options.dateBasis,
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

export async function approveTransaction(transactionID: number, csrfToken: string, input: ApproveTransactionRequest = {}): Promise<TransactionResponse> {
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

export async function moveTransaction(transactionID: number, direction: MoveRequest['direction'], csrfToken: string): Promise<TransactionResponse> {
  try {
    const { data, error, response } = await apiClient.POST('/api/v1/transactions/{transaction_id}/move', {
      params: {
        path: { transaction_id: transactionID },
        header: { 'X-CSRF-Token': csrfToken }
      },
      body: { direction }
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

/**
 * A reorder's override names the checkpoints its refusal reported (a reorder
 * has no preview), so a set that grew meanwhile is refused, not invalidated
 * (#296). Undefined means no override.
 */
function reorderOverride(reviewedCheckpointIDs: number[] | undefined) {
  return reviewedCheckpointIDs
    ? { reconciliation_override: true, reconciliation_override_checkpoint_ids: reviewedCheckpointIDs }
    : {};
}

/**
 * Place a transaction before or after another on their shared day (#185): in
 * the journal order and in every account register holding both that day.
 */
export async function placeTransaction(transactionID: number, anchorTransactionID: number, position: 'before' | 'after',
  csrfToken: string, reviewedCheckpointIDs?: number[]): Promise<TransactionResponse> {
  try {
    const { data, error, response } = await apiClient.POST('/api/v1/transactions/{transaction_id}/place', {
      params: {
        path: { transaction_id: transactionID },
        header: { 'X-CSRF-Token': csrfToken }
      },
      body: { anchor_transaction_id: anchorTransactionID, position, ...reorderOverride(reviewedCheckpointIDs) }
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

export async function movePosting(accountID: number, postingLineID: number, direction: MoveRequest['direction'], csrfToken: string,
  reviewedCheckpointIDs?: number[]): Promise<TransactionResponse> {
  try {
    const { data, error, response } = await apiClient.POST('/api/v1/accounts/{account_id}/postings/{posting_line_id}/move', {
      params: {
        path: { account_id: accountID, posting_line_id: postingLineID },
        header: { 'X-CSRF-Token': csrfToken }
      },
      body: { direction, ...reorderOverride(reviewedCheckpointIDs) }
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

/** Moves one account posting to the previous or next calendar day (#187). */
export async function movePostingToDay(accountID: number, postingLineID: number, direction: MoveRequest['direction'], csrfToken: string,
  reviewedCheckpointIDs?: number[]): Promise<TransactionResponse> {
  try {
    const { data, error, response } = await apiClient.POST('/api/v1/accounts/{account_id}/postings/{posting_line_id}/move-day', {
      params: {
        path: { account_id: accountID, posting_line_id: postingLineID },
        header: { 'X-CSRF-Token': csrfToken }
      },
      body: { direction, ...reorderOverride(reviewedCheckpointIDs) }
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

export async function reconciliationImpactForCreate(input: TransactionRequest): Promise<ReconciliationImpactResponse> {
  try {
    const { data, error, response } = await apiClient.POST('/api/v1/transactions/reconciliation-impact', {
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

export async function reconciliationImpactForUpdate(transactionID: number, input: TransactionRequest): Promise<ReconciliationImpactResponse> {
  try {
    const { data, error, response } = await apiClient.POST('/api/v1/transactions/{transaction_id}/reconciliation-impact', {
      params: {
        path: { transaction_id: transactionID }
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

export type TransactionLifecycleAction = 'void' | 'unvoid' | 'soft-delete' | 'restore';

/**
 * True when a write was refused only because it would invalidate a finished
 * reconciliation; the caller previews the impact and may retry with override.
 */
export function isReconciliationOverrideRequired(error: unknown): boolean {
  return (
    error instanceof APIClientError &&
    error.code === 'CONFLICT' &&
    (error.detail?.includes('reconciliation override') ?? false)
  );
}

/** True when an override was refused because the checkpoint set it acknowledged is no longer the set the write would invalidate (#296). */
export function isReconciliationImpactStale(error: unknown): boolean {
  return error instanceof APIClientError && error.code === 'RECONCILIATION_IMPACT_STALE';
}

/** True when the write was composed against a version another write has replaced (#296). */
export function isTransactionVersionStale(error: unknown): boolean {
  return error instanceof APIClientError && error.code === 'TRANSACTION_VERSION_STALE';
}

/** The override fields that acknowledge exactly the previewed checkpoints (#296). */
export function reviewedOverride(impact: ReconciliationImpactResponse) {
  return {
    reconciliation_override: true,
    reconciliation_override_checkpoint_ids: impact.affected_checkpoints.map((checkpoint) => checkpoint.checkpoint_id)
  };
}

const lifecycleImpactPaths = {
  void: '/api/v1/transactions/{transaction_id}/void/reconciliation-impact',
  unvoid: '/api/v1/transactions/{transaction_id}/unvoid/reconciliation-impact',
  'soft-delete': '/api/v1/transactions/{transaction_id}/soft-delete/reconciliation-impact',
  restore: '/api/v1/transactions/{transaction_id}/restore/reconciliation-impact'
} as const;

/** Checkpoints a guarded lifecycle change would invalidate on override (#220). */
export async function getLifecycleReconciliationImpact(
  transactionID: number,
  action: TransactionLifecycleAction
): Promise<ReconciliationImpactResponse> {
  try {
    const { data, error, response } = await apiClient.GET(lifecycleImpactPaths[action], {
      params: { path: { transaction_id: transactionID } }
    });
    if (data !== undefined) return data;
    throw toAPIClientError(response, error);
  } catch (error) {
    if (error instanceof APIClientError) throw error;
    throw toNetworkError(error);
  }
}

export async function getPostReconciliationImpact(transactionID: number): Promise<ReconciliationImpactResponse> {
  try {
    const { data, error, response } = await apiClient.GET('/api/v1/transactions/{transaction_id}/post/reconciliation-impact', {
      params: { path: { transaction_id: transactionID } }
    });
    if (data !== undefined) return data;
    throw toAPIClientError(response, error);
  } catch (error) {
    if (error instanceof APIClientError) throw error;
    throw toNetworkError(error);
  }
}
