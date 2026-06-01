import type { components } from '$lib/api/schema';
import { APIClientError, apiClient, toAPIClientError, toNetworkError } from '$lib/api/client';

export type CurrencyCatalogResponse = components['schemas']['CurrencyCatalogResponse'];
export type CurrencyCatalogEntry = components['schemas']['CurrencyCatalogEntry'];
export type CurrencyResponse = components['schemas']['CurrencyResponse'];
export type CurrenciesResponse = components['schemas']['CurrenciesResponse'];
export type CreateCurrencyRequest = components['schemas']['CreateCurrencyRequest'];
export type CompleteCurrencySetupRequest = components['schemas']['CompleteCurrencySetupRequest'];
export type CompleteCurrencySetupResponse = components['schemas']['CompleteCurrencySetupResponse'];
export type SetDefaultCurrencyResponse = components['schemas']['SetDefaultCurrencyResponse'];

export const currencyCatalogQueryKey = ['api', 'currencies', 'catalog'] as const;
export const currenciesQueryKey = ['api', 'currencies'] as const;

export function currencyCatalogQueryOptions() {
  return {
    queryKey: currencyCatalogQueryKey,
    queryFn: getCurrencyCatalog,
    staleTime: 60_000
  };
}

export function currenciesQueryOptions() {
  return {
    queryKey: currenciesQueryKey,
    queryFn: getCurrencies,
    staleTime: 10_000
  };
}

export async function getCurrencyCatalog(): Promise<CurrencyCatalogResponse> {
  try {
    const { data, error, response } = await apiClient.GET('/api/v1/currencies/catalog');

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

export async function getCurrencies(): Promise<CurrenciesResponse> {
  try {
    const { data, error, response } = await apiClient.GET('/api/v1/currencies');

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

export async function createCurrency(input: CreateCurrencyRequest, csrfToken: string): Promise<CurrencyResponse> {
  try {
    const { data, error, response } = await apiClient.POST('/api/v1/currencies', {
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

export async function completeCurrencySetup(
  input: CompleteCurrencySetupRequest,
  csrfToken: string
): Promise<CompleteCurrencySetupResponse> {
  try {
    const { data, error, response } = await apiClient.POST('/api/v1/setup/currencies', {
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

export async function setDefaultCurrency(commodityID: number, csrfToken: string): Promise<SetDefaultCurrencyResponse> {
  try {
    const { data, error, response } = await apiClient.POST('/api/v1/currencies/{commodity_id}/default', {
      params: {
        path: {
          commodity_id: commodityID
        },
        header: {
          'X-CSRF-Token': csrfToken
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
