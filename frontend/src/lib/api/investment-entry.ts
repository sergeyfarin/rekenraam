import type { components } from '#lib/api/schema.js';
import { APIClientError, apiClient, toAPIClientError, toNetworkError } from './client.ts';

export type InvestmentEntryAccount = components['schemas']['InvestmentEntryAccountResponse'];

export function investmentEntryAccountsQueryOptions(asOf: string) {
  return {
    queryKey: ['api', 'accounts', 'investment-entry', asOf] as const,
    queryFn: async () => {
      try {
        const { data, error, response } = await apiClient.GET('/api/v1/investments/entry-accounts', {
          params: { query: { as_of: asOf } }
        });
        if (data !== undefined) return data;
        throw toAPIClientError(response, error);
      } catch (error) {
        if (error instanceof APIClientError) throw error;
        throw toNetworkError(error);
      }
    },
    // Holding and cash pickers may observe the same query. Notify both of
    // status changes even when a conditional loading expression has only
    // read one observer's flags; a failed read must reach the retry state.
    notifyOnChangeProps: 'all' as const,
    staleTime: 10_000
  };
}
