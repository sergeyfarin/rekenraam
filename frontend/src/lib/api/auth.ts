import type { components } from '$lib/api/schema';
import { APIClientError, apiClient, toAPIClientError, toNetworkError } from '$lib/api/client';

export type AuthSessionResponse = components['schemas']['AuthSessionResponse'];
export type LoginRequest = components['schemas']['LoginRequest'];
export type LoginResponse = components['schemas']['LoginResponse'];
export type AuthenticationEventResponse = components['schemas']['AuthenticationEventResponse'];
export type AuthenticationEventsResponse = components['schemas']['AuthenticationEventsResponse'];

export const authSessionQueryKey = ['api', 'auth', 'session'] as const;
export const authenticationEventsQueryKey = ['api', 'auth', 'events'] as const;

export function authSessionQueryOptions() {
  return {
    queryKey: authSessionQueryKey,
    queryFn: getAuthSession,
    staleTime: 10_000
  };
}

export function authenticationEventsQueryOptions(limit?: number) {
  return {
    queryKey: [...authenticationEventsQueryKey, { limit }] as const,
    queryFn: () => getAuthenticationEvents(limit),
    staleTime: 10_000
  };
}

// The operator-facing authentication log: successful and failed attempts with
// the proxy-aware client IP, newest first.
export async function getAuthenticationEvents(
  limit?: number
): Promise<AuthenticationEventsResponse> {
  try {
    const { data, error, response } = await apiClient.GET('/api/v1/auth/events', {
      params: { query: { limit } }
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

export async function getAuthSession(): Promise<AuthSessionResponse> {
  try {
    const { data, error, response } = await apiClient.GET('/api/v1/auth/session');

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

export async function login(input: LoginRequest): Promise<LoginResponse> {
  try {
    const { data, error, response } = await apiClient.POST('/api/v1/auth/login', {
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

export async function logout(csrfToken?: string): Promise<void> {
  try {
    let currentCSRFToken = csrfToken;

    if (!currentCSRFToken) {
      const session = await getAuthSession();
      currentCSRFToken = session.csrf_token;
    }

    const { error, response } = await apiClient.POST('/api/v1/auth/logout', {
      params: {
        header: {
          'X-CSRF-Token': currentCSRFToken ?? ''
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
