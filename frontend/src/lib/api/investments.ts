import type { components } from '$lib/api/schema';
import type { TransactionResponse } from '$lib/api/transactions';
import { APIClientError, apiClient, toAPIClientError, toNetworkError } from '$lib/api/client';

export type InvestmentInstrumentResponse = components['schemas']['InvestmentInstrumentResponse'];
export type InvestmentInstrumentsResponse = components['schemas']['InvestmentInstrumentsResponse'];
export type InvestmentPositionResponse = components['schemas']['InvestmentPositionResponse'];
export type InvestmentPositionsResponse = components['schemas']['InvestmentPositionsResponse'];
export type InvestmentLotResponse = components['schemas']['InvestmentLotResponse'];
export type InvestmentLotsResponse = components['schemas']['InvestmentLotsResponse'];
export type CostBasisProfileResponse = components['schemas']['CostBasisProfileResponse'];
export type CostBasisProfilesResponse = components['schemas']['CostBasisProfilesResponse'];
export type DividendDefaultResponse = components['schemas']['DividendDefaultResponse'];
export type DividendDefaultsResponse = components['schemas']['DividendDefaultsResponse'];
export type SellPreviewResponse = components['schemas']['SellPreviewResponse'];
export type InvestmentTradeResponse = components['schemas']['InvestmentTradeResponse'];
export type InvestmentTradeRequest = components['schemas']['InvestmentTradeRequest'];
export type DividendRequest = components['schemas']['DividendRequest'];
export type ReinvestedDividendRequest = components['schemas']['ReinvestedDividendRequest'];
export type CostBasisMethod = components['schemas']['CostBasisMethod'];
export type InvestmentGainsResponse = components['schemas']['InvestmentGainsResponse'];
export type RealizedGainEntry = components['schemas']['RealizedGainEntry'];
export type UnrealizedGainEntry = components['schemas']['UnrealizedGainEntry'];
export type RealizedGainTotal = components['schemas']['RealizedGainTotal'];
export type InvestmentEventSuggestionResponse = components['schemas']['InvestmentEventSuggestionResponse'];
export type InvestmentEventSuggestionsResponse = components['schemas']['InvestmentEventSuggestionsResponse'];
export type InvestmentAutomationRuleResponse = components['schemas']['InvestmentAutomationRuleResponse'];
export type InvestmentAutomationRulesResponse = components['schemas']['InvestmentAutomationRulesResponse'];
export type InvestmentAutomationRulesRequest = components['schemas']['InvestmentAutomationRulesRequest'];
export type InvestmentAutomationRuleRequest = components['schemas']['InvestmentAutomationRuleRequest'];

export const investmentPositionsQueryKey = ['api', 'investments', 'positions'] as const;
export const investmentLotsQueryKey = ['api', 'investments', 'lots'] as const;
export const investmentInstrumentsQueryKey = ['api', 'investments', 'instruments'] as const;
export const investmentGainsQueryKey = ['api', 'investments', 'gains'] as const;
export const investmentEventSuggestionsQueryKey = ['api', 'investments', 'event-suggestions'] as const;
export const investmentAutomationRulesQueryKey = ['api', 'investments', 'automation-rules'] as const;

export function investmentPositionsQueryOptions() {
  return {
    queryKey: investmentPositionsQueryKey,
    queryFn: () => getInvestmentPositions(),
    staleTime: 30_000
  };
}

export function investmentLotsQueryOptions(accountID?: number, commodityID?: number) {
  return {
    queryKey: [...investmentLotsQueryKey, { accountID, commodityID }] as const,
    queryFn: () => getInvestmentLots(accountID, commodityID),
    staleTime: 30_000
  };
}

export function investmentInstrumentsQueryOptions() {
  return {
    queryKey: investmentInstrumentsQueryKey,
    queryFn: () => getInvestmentInstruments(),
    staleTime: 60_000
  };
}

export async function getInvestmentPositions(): Promise<InvestmentPositionsResponse> {
  try {
    const { data, error, response } = await apiClient.GET('/api/v1/investments/positions');

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

export async function getInvestmentLots(
  accountID?: number,
  commodityID?: number
): Promise<InvestmentLotsResponse> {
  try {
    const { data, error, response } = await apiClient.GET('/api/v1/investments/lots', {
      params: {
        query: {
          account_id: accountID,
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

export async function getInvestmentInstruments(): Promise<InvestmentInstrumentsResponse> {
  try {
    const { data, error, response } = await apiClient.GET('/api/v1/investments/instruments');

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

export async function searchInvestmentInstruments(q: string): Promise<InvestmentInstrumentsResponse> {
  try {
    const { data, error, response } = await apiClient.GET('/api/v1/investments/search', {
      params: { query: { q } }
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

export async function getDividendDefaults(): Promise<DividendDefaultsResponse> {
  try {
    const { data, error, response } = await apiClient.GET('/api/v1/investments/dividend-defaults');

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

export async function recordBuy(
  input: InvestmentTradeRequest,
  csrfToken: string
): Promise<InvestmentTradeResponse> {
  try {
    const { data, error, response } = await apiClient.POST('/api/v1/investments/buy', {
      params: { header: { 'X-CSRF-Token': csrfToken } },
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

export async function previewSell(
  input: InvestmentTradeRequest
): Promise<SellPreviewResponse> {
  try {
    const { data, error, response } = await apiClient.POST('/api/v1/investments/sell/preview', {
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

export async function recordSell(
  input: InvestmentTradeRequest,
  csrfToken: string
): Promise<InvestmentTradeResponse> {
  try {
    const { data, error, response } = await apiClient.POST('/api/v1/investments/sell', {
      params: { header: { 'X-CSRF-Token': csrfToken } },
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

export async function recordDividend(
  input: DividendRequest,
  csrfToken: string
): Promise<TransactionResponse> {
  try {
    const { data, error, response } = await apiClient.POST('/api/v1/investments/dividend', {
      params: { header: { 'X-CSRF-Token': csrfToken } },
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

export async function recordReinvestedDividend(
  input: ReinvestedDividendRequest,
  csrfToken: string
): Promise<InvestmentTradeResponse> {
  try {
    const { data, error, response } = await apiClient.POST('/api/v1/investments/reinvested-dividend', {
      params: { header: { 'X-CSRF-Token': csrfToken } },
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

export function investmentGainsQueryOptions(from?: string, to?: string) {
  return {
    queryKey: [...investmentGainsQueryKey, { from, to }] as const,
    queryFn: () => getInvestmentGains(from, to),
    staleTime: 30_000
  };
}

export function investmentEventSuggestionsQueryOptions() {
  return {
    queryKey: investmentEventSuggestionsQueryKey,
    queryFn: () => getInvestmentEventSuggestions(),
    staleTime: 15_000
  };
}

export function investmentAutomationRulesQueryOptions() {
  return {
    queryKey: investmentAutomationRulesQueryKey,
    queryFn: () => getInvestmentAutomationRules(),
    staleTime: 60_000
  };
}

export async function getInvestmentGains(from?: string, to?: string): Promise<InvestmentGainsResponse> {
  try {
    const { data, error, response } = await apiClient.GET('/api/v1/investments/gains', {
      params: { query: { from, to } }
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

export async function getInvestmentEventSuggestions(): Promise<InvestmentEventSuggestionsResponse> {
  try {
    const { data, error, response } = await apiClient.GET('/api/v1/investments/event-suggestions');

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

export async function getInvestmentAutomationRules(): Promise<InvestmentAutomationRulesResponse> {
  try {
    const result = await apiClient.GET('/api/v1/investments/automation-rules');

    if (result.data !== undefined) {
      return result.data;
    }

    throw toAPIClientError(result.response, result.error);
  } catch (error) {
    if (error instanceof APIClientError) {
      throw error;
    }

    throw toNetworkError(error);
  }
}

export async function acceptEventSuggestion(
  suggestionId: number,
  csrfToken: string
): Promise<InvestmentEventSuggestionResponse> {
  try {
    const { data, error, response } = await apiClient.POST(
      '/api/v1/investments/event-suggestions/{suggestion_id}/accept',
      {
        params: { path: { suggestion_id: suggestionId }, header: { 'X-CSRF-Token': csrfToken } }
      }
    );

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

export async function ignoreEventSuggestion(
  suggestionId: number,
  csrfToken: string
): Promise<InvestmentEventSuggestionResponse> {
  try {
    const { data, error, response } = await apiClient.POST(
      '/api/v1/investments/event-suggestions/{suggestion_id}/ignore',
      {
        params: { path: { suggestion_id: suggestionId }, header: { 'X-CSRF-Token': csrfToken } }
      }
    );

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

export async function saveAutomationRules(
  input: InvestmentAutomationRulesRequest,
  csrfToken: string
): Promise<InvestmentAutomationRulesResponse> {
  try {
    const { data, error, response } = await apiClient.PUT('/api/v1/investments/automation-rules', {
      params: { header: { 'X-CSRF-Token': csrfToken } },
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
