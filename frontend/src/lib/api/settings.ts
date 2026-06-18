import type { components } from '$lib/api/schema';
import { APIClientError, apiClient, toAPIClientError, toNetworkError } from '$lib/api/client';

export type UserPreferencesResponse = components['schemas']['UserPreferencesResponse'];
export type UserPreferencesRequest = components['schemas']['UserPreferencesRequest'];
export type CurrencySettingsPageResponse = components['schemas']['CurrencySettingsPageResponse'];

export const userPreferencesQueryKey = ['api', 'settings', 'preferences'] as const;
export const currencySettingsPageQueryKey = ['api', 'settings', 'currencies', 'page-data'] as const;

export function userPreferencesQueryOptions() {
  return {
    queryKey: userPreferencesQueryKey,
    queryFn: getUserPreferences,
    staleTime: 10_000
  };
}

export function currencySettingsPageQueryOptions() {
  return {
    queryKey: currencySettingsPageQueryKey,
    queryFn: getCurrencySettingsPage,
    staleTime: 10_000
  };
}

export async function getUserPreferences(): Promise<UserPreferencesResponse> {
  try {
    const { data, error, response } = await apiClient.GET('/api/v1/settings/preferences');

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

export async function getCurrencySettingsPage(): Promise<CurrencySettingsPageResponse> {
  try {
    const { data, error, response } = await apiClient.GET('/api/v1/settings/currencies/page-data');

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

export async function saveUserPreferences(
  input: UserPreferencesRequest,
  csrfToken: string
): Promise<UserPreferencesResponse> {
  try {
    const { data, error, response } = await apiClient.PUT('/api/v1/settings/preferences', {
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
