import type { components } from '$lib/api/schema';
import { APIClientError, apiClient, toAPIClientError, toNetworkError } from '$lib/api/client';
import type { TransactionLifecycleRequest, TransactionsResponse } from '$lib/api/transactions';

export type ReconciliationSessionResponse = components['schemas']['ReconciliationSessionResponse'];
export type ReconciliationCheckpointResponse = components['schemas']['ReconciliationCheckpointResponse'];
export type ReconciliationCheckpointsResponse = components['schemas']['ReconciliationCheckpointsResponse'];
export type StartReconciliationRequest = components['schemas']['StartReconciliationRequest'];
export type ReconciliationSelectionRequest = components['schemas']['ReconciliationSelectionRequest'];
export type PostingReconciliationStatusRequest = components['schemas']['PostingReconciliationStatusRequest'];

export const reconciliationQueryKey = ['api', 'reconciliation'] as const;

export function reconciliationSessionQueryOptions(reconciliationID: number) {
  return {
    queryKey: [...reconciliationQueryKey, 'session', reconciliationID] as const,
    queryFn: () => getReconciliationSession(reconciliationID),
    staleTime: 5_000
  };
}

export function reconciliationCheckpointsQueryOptions(accountID: number, commodityID?: number) {
  return {
    queryKey: [...reconciliationQueryKey, 'checkpoints', accountID, commodityID] as const,
    queryFn: () => getReconciliationCheckpoints(accountID, commodityID),
    staleTime: 5_000
  };
}

export async function startReconciliation(accountID: number, input: StartReconciliationRequest, csrfToken: string): Promise<ReconciliationSessionResponse> {
  try {
    const { data, error, response } = await apiClient.POST('/api/v1/accounts/{account_id}/reconciliations/start', {
      params: {
        path: {
          account_id: accountID
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

export async function getReconciliationSession(reconciliationID: number): Promise<ReconciliationSessionResponse> {
  try {
    const { data, error, response } = await apiClient.GET('/api/v1/reconciliations/{reconciliation_id}', {
      params: {
        path: {
          reconciliation_id: reconciliationID
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

export async function updateReconciliationSelection(reconciliationID: number, input: ReconciliationSelectionRequest, csrfToken: string): Promise<ReconciliationSessionResponse> {
  try {
    const { data, error, response } = await apiClient.PATCH('/api/v1/reconciliations/{reconciliation_id}', {
      params: {
        path: {
          reconciliation_id: reconciliationID
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

export async function previewReconciliation(reconciliationID: number, input: ReconciliationSelectionRequest, csrfToken: string): Promise<ReconciliationSessionResponse> {
  return reconciliationLifecycleMutation('/api/v1/reconciliations/{reconciliation_id}/preview', reconciliationID, csrfToken, input);
}

export async function finishReconciliation(reconciliationID: number, input: TransactionLifecycleRequest, csrfToken: string): Promise<ReconciliationSessionResponse> {
  return reconciliationLifecycleMutation('/api/v1/reconciliations/{reconciliation_id}/finish', reconciliationID, csrfToken, input);
}

export async function voidReconciliation(reconciliationID: number, input: TransactionLifecycleRequest, csrfToken: string): Promise<ReconciliationSessionResponse> {
  return reconciliationLifecycleMutation('/api/v1/reconciliations/{reconciliation_id}/void', reconciliationID, csrfToken, input);
}

export async function getReconciliationCheckpoints(accountID: number, commodityID?: number): Promise<ReconciliationCheckpointsResponse> {
  try {
    const { data, error, response } = await apiClient.GET('/api/v1/accounts/{account_id}/reconciliation-checkpoints', {
      params: {
        path: {
          account_id: accountID
        },
        query: {
          commodity_id: commodityID
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

export async function voidReconciliationCheckpoint(checkpointID: number, input: TransactionLifecycleRequest, csrfToken: string): Promise<ReconciliationCheckpointResponse> {
  try {
    const { data, error, response } = await apiClient.POST('/api/v1/reconciliation-checkpoints/{checkpoint_id}/void', {
      params: {
        path: {
          checkpoint_id: checkpointID
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

export async function changePostingReconciliationStatus(accountID: number, input: PostingReconciliationStatusRequest, csrfToken: string): Promise<TransactionsResponse> {
  try {
    const { data, error, response } = await apiClient.POST('/api/v1/accounts/{account_id}/postings/reconciliation-status', {
      params: {
        path: {
          account_id: accountID
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

async function reconciliationLifecycleMutation(
  path:
    | '/api/v1/reconciliations/{reconciliation_id}/preview'
    | '/api/v1/reconciliations/{reconciliation_id}/finish'
    | '/api/v1/reconciliations/{reconciliation_id}/void',
  reconciliationID: number,
  csrfToken: string,
  input: ReconciliationSelectionRequest | TransactionLifecycleRequest
): Promise<ReconciliationSessionResponse> {
  try {
    const { data, error, response } = await apiClient.POST(path, {
      params: {
        path: {
          reconciliation_id: reconciliationID
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
