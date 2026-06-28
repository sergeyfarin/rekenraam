import type { components } from '$lib/api/schema';
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

export const investmentPositionsQueryKey = ['api', 'investments', 'positions'] as const;
export const investmentLotsQueryKey = ['api', 'investments', 'lots'] as const;
export const investmentInstrumentsQueryKey = ['api', 'investments', 'instruments'] as const;

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
