import type { components } from '$lib/api/schema';
import { APIClientError, apiClient, toAPIClientError, toNetworkError } from '$lib/api/client';

export type AccountResponse = components['schemas']['AccountResponse'];
export type AccountsResponse = components['schemas']['AccountsResponse'];
export type AccountRequest = components['schemas']['AccountRequest'];
export type AccountLifecycleRequest = components['schemas']['AccountLifecycleRequest'];

export type BasicAccountRequest = Omit<AccountRequest, 'effective_from'> & {
  effective_from?: never;
};

export type AdvancedAccountRequest = AccountRequest;

export const accountsQueryKey = ['api', 'accounts'] as const;

export function accountsQueryOptions(includeArchived = false, includeSystem = false) {
  return {
    queryKey: [...accountsQueryKey, { includeArchived, includeSystem }] as const,
    queryFn: () => getAccounts(includeArchived, includeSystem),
    staleTime: 10_000
  };
}

export async function getAccounts(includeArchived = false, includeSystem = false): Promise<AccountsResponse> {
  try {
    const { data, error, response } = await apiClient.GET('/api/v1/accounts', {
      params: {
        query: {
          include_archived: includeArchived || undefined,
          include_system: includeSystem || undefined
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

export async function getAccount(accountID: number): Promise<AccountResponse> {
  try {
    const { data, error, response } = await apiClient.GET('/api/v1/accounts/{account_id}', {
      params: {
        path: {
          account_id: accountID
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

export async function getAccountVersions(accountID: number): Promise<AccountsResponse> {
  try {
    const { data, error, response } = await apiClient.GET('/api/v1/accounts/{account_id}/versions', {
      params: {
        path: {
          account_id: accountID
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

export async function createAccount(input: BasicAccountRequest, csrfToken: string): Promise<AccountResponse> {
  return createAccountAdvanced(input, csrfToken);
}

export async function createAccountAdvanced(
  input: AdvancedAccountRequest,
  csrfToken: string
): Promise<AccountResponse> {
  try {
    const { data, error, response } = await apiClient.POST('/api/v1/accounts', {
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

export async function updateAccount(
  accountID: number,
  input: AccountRequest,
  csrfToken: string
): Promise<AccountResponse> {
  try {
    const { data, error, response } = await apiClient.PATCH('/api/v1/accounts/{account_id}', {
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

export async function closeAccount(
  accountID: number,
  csrfToken: string,
  input?: AccountLifecycleRequest
): Promise<AccountResponse> {
  return accountLifecycleMutation('/api/v1/accounts/{account_id}/close', accountID, csrfToken, input);
}

export async function reopenAccount(
  accountID: number,
  csrfToken: string,
  input?: AccountLifecycleRequest
): Promise<AccountResponse> {
  return accountLifecycleMutation('/api/v1/accounts/{account_id}/reopen', accountID, csrfToken, input);
}

export async function archiveAccount(
  accountID: number,
  csrfToken: string,
  input?: AccountLifecycleRequest
): Promise<AccountResponse> {
  return accountLifecycleMutation('/api/v1/accounts/{account_id}/archive', accountID, csrfToken, input);
}

export async function restoreAccount(
  accountID: number,
  csrfToken: string,
  input?: AccountLifecycleRequest
): Promise<AccountResponse> {
  return accountLifecycleMutation('/api/v1/accounts/{account_id}/restore', accountID, csrfToken, input);
}

export async function deleteAccount(accountID: number, csrfToken: string): Promise<void> {
  try {
    const { error, response } = await apiClient.DELETE('/api/v1/accounts/{account_id}', {
      params: {
        path: {
          account_id: accountID
        },
        header: {
          'X-CSRF-Token': csrfToken
        }
      }
    });

    if (response?.ok) {
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

async function accountLifecycleMutation(
  path:
    | '/api/v1/accounts/{account_id}/close'
    | '/api/v1/accounts/{account_id}/reopen'
    | '/api/v1/accounts/{account_id}/archive'
    | '/api/v1/accounts/{account_id}/restore',
  accountID: number,
  csrfToken: string,
  input?: AccountLifecycleRequest
): Promise<AccountResponse> {
  try {
    const request = {
      params: {
        path: {
          account_id: accountID
        },
        header: {
          'X-CSRF-Token': csrfToken
        }
      },
      ...(input ? { body: input } : {})
    };

    const { data, error, response } = await apiClient.POST(path, request);

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
