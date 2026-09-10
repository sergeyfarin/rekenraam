import type { components } from '$lib/api/schema';
import { APIClientError, apiClient, toAPIClientError, toNetworkError } from '$lib/api/client';

export type BudgetMonth = components['schemas']['BudgetMonthResponse'];
export type BudgetTreatment = components['schemas']['BudgetAccount']['treatment'];
export const budgetQueryKey = ['api', 'budgets', 'month'] as const;

export function budgetMonthQueryOptions(periodStart?: string) {
  return {
    queryKey: [...budgetQueryKey, periodStart] as const,
    queryFn: () => getBudgetMonth(periodStart),
    enabled: periodStart === undefined || /^\d{4}-\d{2}-01$/.test(periodStart)
  };
}

export async function getBudgetMonth(periodStart?: string): Promise<BudgetMonth> {
  try {
    const { data, error, response } = await apiClient.GET('/api/v1/budgets/month', {
      params: { query: { period_start: periodStart } }
    });
    if (data) return data;
    throw toAPIClientError(response, error);
  } catch (error) {
    if (error instanceof APIClientError) throw error;
    throw toNetworkError(error);
  }
}

export async function setBudgetTarget(input: {
  categoryID: number;
  commodityID: number;
  periodStart: string;
  quantityValue: string;
  quantityScale: number;
  csrfToken: string;
}): Promise<BudgetMonth> {
  try {
    const { data, error, response } = await apiClient.PUT('/api/v1/budgets/targets/{category_id}/{commodity_id}', {
      params: { path: { category_id: input.categoryID, commodity_id: input.commodityID } },
      headers: { 'X-CSRF-Token': input.csrfToken },
      body: { period_start: input.periodStart, quantity_value: input.quantityValue, quantity_scale: input.quantityScale, change_reason: 'Updated monthly budget target' }
    });
    if (data) return data;
    throw toAPIClientError(response, error);
  } catch (error) {
    if (error instanceof APIClientError) throw error;
    throw toNetworkError(error);
  }
}

export async function setBudgetTreatment(input: { accountID: number; treatment: BudgetTreatment; effectiveFrom: string; csrfToken: string }): Promise<BudgetMonth> {
  try {
    const { data, error, response } = await apiClient.PUT('/api/v1/budgets/accounts/{account_id}', {
      params: { path: { account_id: input.accountID } },
      headers: { 'X-CSRF-Token': input.csrfToken },
      body: { effective_from: input.effectiveFrom, treatment: input.treatment, change_reason: 'Updated budget account treatment' }
    });
    if (data) return data;
    throw toAPIClientError(response, error);
  } catch (error) {
    if (error instanceof APIClientError) throw error;
    throw toNetworkError(error);
  }
}
