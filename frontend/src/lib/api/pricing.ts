import type { components } from '$lib/api/schema';
import { APIClientError, apiClient, toAPIClientError, toNetworkError } from '$lib/api/client';

export type PriceObservationResponse = components['schemas']['PriceObservationResponse'];
export type PriceObservationsResponse = components['schemas']['PriceObservationsResponse'];
export type MarketDataSourceResponse = components['schemas']['MarketDataSourceResponse'];
export type MarketDataSourcesResponse = components['schemas']['MarketDataSourcesResponse'];
export type PricingPolicyResponse = components['schemas']['PricingPolicyResponse'];
export type PricingPolicyRequest = components['schemas']['PricingPolicyRequest'];
export type PricingSourceAssignmentResponse = components['schemas']['PricingSourceAssignmentResponse'];
export type PricingSourceAssignmentsResponse = components['schemas']['PricingSourceAssignmentsResponse'];
export type PricingSourceAssignmentRequest = components['schemas']['PricingSourceAssignmentRequest'];
export type PricingRefreshRunResponse = components['schemas']['PricingRefreshRunResponse'];
export type PricingRefreshRunsResponse = components['schemas']['PricingRefreshRunsResponse'];
export type PricingRefreshRunRequest = components['schemas']['PricingRefreshRunRequest'];
export type PricingSourceHealthListResponse = components['schemas']['PricingSourceHealthListResponse'];

export const pricingSourcesQueryKey = ['api', 'pricing', 'sources'] as const;
export const pricingPolicyQueryKey = ['api', 'pricing', 'policy'] as const;
export const pricingSourceAssignmentsQueryKey = ['api', 'pricing', 'source-assignments'] as const;
export const pricingRefreshRunsQueryKey = ['api', 'pricing', 'refresh-runs'] as const;
export const pricingSourceHealthQueryKey = ['api', 'pricing', 'source-health'] as const;

export function pricingSourcesQueryOptions() {
  return {
    queryKey: pricingSourcesQueryKey,
    queryFn: getPricingSources,
    staleTime: 60_000
  };
}

export function pricingPolicyQueryOptions() {
  return {
    queryKey: pricingPolicyQueryKey,
    queryFn: getPricingPolicy,
    staleTime: 10_000
  };
}

export function pricingSourceAssignmentsQueryOptions() {
  return {
    queryKey: pricingSourceAssignmentsQueryKey,
    queryFn: getPricingSourceAssignments,
    staleTime: 10_000
  };
}

export function pricingRefreshRunsQueryOptions() {
  return {
    queryKey: pricingRefreshRunsQueryKey,
    queryFn: getPricingRefreshRuns,
    staleTime: 10_000
  };
}

export function pricingSourceHealthQueryOptions() {
  return {
    queryKey: pricingSourceHealthQueryKey,
    queryFn: getPricingSourceHealth,
    staleTime: 10_000
  };
}

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

export async function getPricingSources(): Promise<MarketDataSourcesResponse> {
  try {
    const { data, error, response } = await apiClient.GET('/api/v1/pricing/sources');

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

export async function getPricingPolicy(): Promise<PricingPolicyResponse> {
  try {
    const { data, error, response } = await apiClient.GET('/api/v1/pricing/policy');

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

export async function savePricingPolicy(
  input: PricingPolicyRequest,
  csrfToken: string
): Promise<PricingPolicyResponse> {
  try {
    const { data, error, response } = await apiClient.PUT('/api/v1/pricing/policy', {
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

export async function getPricingSourceAssignments(): Promise<PricingSourceAssignmentsResponse> {
  try {
    const { data, error, response } = await apiClient.GET('/api/v1/pricing/source-assignments');

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

export async function createPricingSourceAssignment(
  input: PricingSourceAssignmentRequest,
  csrfToken: string
): Promise<PricingSourceAssignmentResponse> {
  try {
    const { data, error, response } = await apiClient.POST('/api/v1/pricing/source-assignments', {
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

export async function updatePricingSourceAssignment(
  assignmentID: number,
  input: PricingSourceAssignmentRequest,
  csrfToken: string
): Promise<PricingSourceAssignmentResponse> {
  try {
    const { data, error, response } = await apiClient.PATCH('/api/v1/pricing/source-assignments/{assignment_id}', {
      params: {
        path: {
          assignment_id: assignmentID
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

export async function runPricingRefresh(
  input: PricingRefreshRunRequest,
  csrfToken: string
): Promise<PricingRefreshRunResponse> {
  try {
    const { data, error, response } = await apiClient.POST('/api/v1/pricing/refresh/run', {
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

export async function getPricingRefreshRuns(): Promise<PricingRefreshRunsResponse> {
  try {
    const { data, error, response } = await apiClient.GET('/api/v1/pricing/refresh/runs');

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

export async function getPricingSourceHealth(): Promise<PricingSourceHealthListResponse> {
  try {
    const { data, error, response } = await apiClient.GET('/api/v1/pricing/source-health');

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
