import type { components } from '$lib/api/schema';
import { APIClientError, apiClient, toAPIClientError, toNetworkError } from '$lib/api/client';

export type PriceObservationResponse = components['schemas']['PriceObservationResponse'];
export type PriceObservationsResponse = components['schemas']['PriceObservationsResponse'];

export async function getLatestPriceObservation(
  baseCommodityID: number,
  quoteCommodityID: number
): Promise<PriceObservationResponse | null> {
  try {
    const { data, error, response } = await apiClient.GET('/api/v1/pricing/prices', {
      params: {
        query: {
          base_commodity_id: baseCommodityID,
          quote_commodity_id: quoteCommodityID,
          limit: 1
        }
      }
    });

    if (data !== undefined) {
      return data.prices[0] ?? null;
    }

    throw toAPIClientError(response, error);
  } catch (error) {
    if (error instanceof APIClientError) {
      throw error;
    }

    throw toNetworkError(error);
  }
}
