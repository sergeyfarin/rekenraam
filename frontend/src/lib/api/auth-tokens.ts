import type { components } from '#lib/api/schema.js';
import { APIClientError, apiClient, toAPIClientError, toNetworkError } from '#lib/api/client.ts';

export type PersonalAccessToken = components['schemas']['PersonalAccessToken'];
export type TokenScope = PersonalAccessToken['scopes'][number];
type CreateTokenRequest = components['schemas']['CreatePersonalAccessTokenRequest'];

export const personalAccessTokensQueryKey = ['api', 'auth', 'tokens'] as const;

export function personalAccessTokensQueryOptions() {
  return {
    queryKey: personalAccessTokensQueryKey,
    initialPageParam: undefined as string | undefined,
    queryFn: ({ pageParam }: { pageParam: string | undefined }) => listPersonalAccessTokens(pageParam),
    getNextPageParam: (lastPage: components['schemas']['PersonalAccessTokenList']) => lastPage.next_cursor ?? undefined
  };
}

async function listPersonalAccessTokens(cursor?: string) {
  try {
    const { data, error, response } = await apiClient.GET('/api/v1/auth/tokens', {
      params: { query: { cursor } }
    });
    if (data !== undefined) return data;
    throw toAPIClientError(response, error);
  } catch (error) {
    throw error instanceof APIClientError ? error : toNetworkError(error);
  }
}

// Call directly from the form: never put the one-time secret in a Query cache.
export async function createPersonalAccessToken(body: CreateTokenRequest, csrfToken: string) {
  try {
    const { data, error, response } = await apiClient.POST('/api/v1/auth/tokens', {
      body, params: { header: { 'X-CSRF-Token': csrfToken } }
    });
    if (data !== undefined) return data;
    throw toAPIClientError(response, error);
  } catch (error) {
    throw error instanceof APIClientError ? error : toNetworkError(error);
  }
}

export async function revokePersonalAccessToken(tokenId: number, csrfToken: string) {
  try {
    const { error, response } = await apiClient.DELETE('/api/v1/auth/tokens/{token_id}', {
      params: { path: { token_id: tokenId }, header: { 'X-CSRF-Token': csrfToken } }
    });
    if (response.ok) return;
    throw toAPIClientError(response, error);
  } catch (error) {
    throw error instanceof APIClientError ? error : toNetworkError(error);
  }
}
