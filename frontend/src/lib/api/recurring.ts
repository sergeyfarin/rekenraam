import type { components } from '$lib/api/schema';
import { APIClientError, apiClient, toAPIClientError, toNetworkError } from '$lib/api/client';

export type RecurringTemplate = components['schemas']['RecurringTemplateResponse'];
export type RecurringTemplates = components['schemas']['RecurringTemplatesResponse'];
export type CreateRecurringTemplateRequest = components['schemas']['CreateRecurringTemplateRequest'];
export type RecurringTemplatePatch = components['schemas']['RecurringTemplatePatch'];

export const recurringTemplatesQueryKey = ['api', 'recurring', 'templates'] as const;

export function recurringTemplatesQueryOptions(includeArchived = false) {
  return {
    queryKey: [...recurringTemplatesQueryKey, { includeArchived }] as const,
    queryFn: () => getRecurringTemplates(includeArchived),
    staleTime: 10_000
  };
}

export function recurringTemplateQueryOptions(templateID: number) {
  return {
    queryKey: [...recurringTemplatesQueryKey, templateID] as const,
    queryFn: () => getRecurringTemplate(templateID),
    staleTime: 10_000
  };
}

export async function getRecurringTemplates(includeArchived = false): Promise<RecurringTemplates> {
  try {
    const { data, error, response } = await apiClient.GET('/api/v1/recurring/templates', {
      params: { query: { include_archived: includeArchived || undefined } }
    });
    if (data !== undefined) return data;
    throw toAPIClientError(response, error);
  } catch (error) {
    if (error instanceof APIClientError) throw error;
    throw toNetworkError(error);
  }
}

export async function getRecurringTemplate(templateID: number): Promise<RecurringTemplate> {
  try {
    const { data, error, response } = await apiClient.GET('/api/v1/recurring/templates/{template_id}', {
      params: { path: { template_id: templateID } }
    });
    if (data !== undefined) return data;
    throw toAPIClientError(response, error);
  } catch (error) {
    if (error instanceof APIClientError) throw error;
    throw toNetworkError(error);
  }
}

export async function createRecurringTemplate(input: CreateRecurringTemplateRequest, csrfToken: string): Promise<RecurringTemplate> {
  try {
    const { data, error, response } = await apiClient.POST('/api/v1/recurring/templates', {
      params: { header: { 'X-CSRF-Token': csrfToken } }, body: input
    });
    if (data !== undefined) return data;
    throw toAPIClientError(response, error);
  } catch (error) {
    if (error instanceof APIClientError) throw error;
    throw toNetworkError(error);
  }
}

export async function updateRecurringTemplate(templateID: number, input: RecurringTemplatePatch, csrfToken: string): Promise<RecurringTemplate> {
  try {
    const { data, error, response } = await apiClient.PATCH('/api/v1/recurring/templates/{template_id}', {
      params: { path: { template_id: templateID }, header: { 'X-CSRF-Token': csrfToken } }, body: input
    });
    if (data !== undefined) return data;
    throw toAPIClientError(response, error);
  } catch (error) {
    if (error instanceof APIClientError) throw error;
    throw toNetworkError(error);
  }
}

export async function archiveRecurringTemplate(templateID: number, csrfToken: string): Promise<RecurringTemplate> {
  try {
    const { data, error, response } = await apiClient.POST('/api/v1/recurring/templates/{template_id}/archive', {
      params: { path: { template_id: templateID }, header: { 'X-CSRF-Token': csrfToken } }, body: {}
    });
    if (data !== undefined) return data;
    throw toAPIClientError(response, error);
  } catch (error) {
    if (error instanceof APIClientError) throw error;
    throw toNetworkError(error);
  }
}
