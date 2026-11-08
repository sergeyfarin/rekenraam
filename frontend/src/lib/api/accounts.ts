import type { components } from '#lib/api/schema.js';
import { APIClientError, apiClient, toAPIClientError, toNetworkError } from '#lib/api/client.ts';

export type AccountResponse = components['schemas']['AccountResponse'];
export type AccountsResponse = components['schemas']['AccountsResponse'];
export type AccountListResponse = components['schemas']['AccountListResponse'];
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

// The account list is cursor-paged (#518). Pickers, filters and lookups
// need the whole catalog, so this follows next_cursor until the last page
// and hands consumers one complete list; a failed page fails the whole read
// instead of returning a silently partial catalog. An account renamed between
// pages can reappear after the cursor, so rows are kept once by id.
export async function getAccounts(includeArchived = false, includeSystem = false): Promise<AccountsResponse> {
  const accounts: AccountResponse[] = [];
  const seen = new Set<number>();
  let cursor: string | undefined;
  for (;;) {
    const page = await getAccountPage(includeArchived, includeSystem, cursor);
    for (const account of page.accounts) {
      if (!seen.has(account.id)) {
        seen.add(account.id);
        accounts.push(account);
      }
    }
    if (page.next_cursor === null || page.next_cursor === undefined || page.next_cursor === cursor) {
      return { accounts };
    }
    cursor = page.next_cursor;
  }
}

async function getAccountPage(includeArchived: boolean, includeSystem: boolean, cursor: string | undefined): Promise<AccountListResponse> {
  try {
    const { data, error, response } = await apiClient.GET('/api/v1/accounts', {
      params: {
        query: {
          include_archived: includeArchived || undefined,
          include_system: includeSystem || undefined,
          cursor
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

// One account by id, current version, whatever its status: a register
// reached directly needs its own account, not a search of the list (#518).
// The key sits under accountsQueryKey so every account invalidation refreshes it.
export function accountQueryOptions(accountID: number) {
  return {
    queryKey: [...accountsQueryKey, 'detail', accountID] as const,
    queryFn: () => getAccount(accountID),
    staleTime: 10_000
  };
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
