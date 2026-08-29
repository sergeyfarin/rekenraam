import type { components } from '$lib/api/schema';
import { APIClientError, apiClient, toAPIClientError, toNetworkError } from '$lib/api/client';

export type ImportRule = components['schemas']['ImportRuleResponse'];
export type CreateImportRuleRequest = components['schemas']['CreateImportRuleRequest'];
export type UpdateImportRuleRequest = components['schemas']['UpdateImportRuleRequest'];

export const importRulesQueryKey = ['api', 'import-rules'] as const;

export function importRulesQueryOptions() {
  return { queryKey: importRulesQueryKey, queryFn: listImportRules, staleTime: 10_000 };
}

export async function listImportRules(): Promise<components['schemas']['ListImportRulesResponse']> {
  return request(() => apiClient.GET('/api/v1/import-rules'));
}

export async function createImportRule(input: CreateImportRuleRequest, csrfToken: string): Promise<ImportRule> {
  return request(() => apiClient.POST('/api/v1/import-rules', {
    params: { header: { 'X-CSRF-Token': csrfToken } }, body: input
  }));
}

export async function updateImportRule(ruleID: number, input: UpdateImportRuleRequest, csrfToken: string): Promise<ImportRule> {
  return request(() => apiClient.PATCH('/api/v1/import-rules/{rule_id}', {
    params: { path: { rule_id: ruleID }, header: { 'X-CSRF-Token': csrfToken } }, body: input
  }));
}

export async function deleteImportRule(ruleID: number, csrfToken: string): Promise<void> {
  try {
    const { error, response } = await apiClient.DELETE('/api/v1/import-rules/{rule_id}', {
      params: { path: { rule_id: ruleID }, header: { 'X-CSRF-Token': csrfToken } }
    });
    if (response.ok) return;
    throw toAPIClientError(response, error);
  } catch (error) {
    if (error instanceof APIClientError) throw error;
    throw toNetworkError(error);
  }
}

async function request<T>(call: () => Promise<{ data?: T; error?: unknown; response: Response }>): Promise<T> {
  try {
    const { data, error, response } = await call();
    if (data !== undefined) return data;
    throw toAPIClientError(response, error);
  } catch (error) {
    if (error instanceof APIClientError) throw error;
    throw toNetworkError(error);
  }
}
