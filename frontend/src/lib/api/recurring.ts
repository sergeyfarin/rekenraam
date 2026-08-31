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

export type RecurringDueResponse = components['schemas']['RecurringDueResponse'];
export type RecurringOccurrencesResponse = components['schemas']['RecurringOccurrencesResponse'];
export type RecurringGenerationResponse = components['schemas']['RecurringGenerationResponse'];
export const recurringDueQueryKey = ['api', 'recurring', 'due'] as const;

export function recurringDueInfiniteQueryOptions(limit = 50) {
  return {
    queryKey: [...recurringDueQueryKey, { limit }] as const,
    initialPageParam: '',
    queryFn: ({ pageParam }: { pageParam: string }) => getRecurringDue(pageParam || undefined, limit),
    getNextPageParam: (lastPage: RecurringDueResponse) => lastPage.next_cursor || null,
    staleTime: 5_000
  };
}

export async function getRecurringDue(cursor?: string, limit = 50): Promise<RecurringDueResponse> {
  try {
    const { data, error, response } = await apiClient.GET('/api/v1/recurring/due', {
      params: { query: { cursor, limit } }
    });
    if (data !== undefined) return data;
    throw toAPIClientError(response, error);
  } catch (error) {
    if (error instanceof APIClientError) throw error;
    throw toNetworkError(error);
  }
}

export async function getRecurringOccurrences(templateID: number, from: string, to: string): Promise<RecurringOccurrencesResponse> {
  try {
    const { data, error, response } = await apiClient.GET('/api/v1/recurring/templates/{template_id}/occurrences', {
      params: { path: { template_id: templateID }, query: { from, to } }
    });
    if (data !== undefined) return data;
    throw toAPIClientError(response, error);
  } catch (error) {
    if (error instanceof APIClientError) throw error;
    throw toNetworkError(error);
  }
}

export async function skipRecurringOccurrence(templateID: number, occurrenceDate: string, reason: string, csrfToken: string): Promise<void> {
  try {
    const { error, response } = await apiClient.POST('/api/v1/recurring/templates/{template_id}/skip', {
      params: { path: { template_id: templateID }, header: { 'X-CSRF-Token': csrfToken } },
      body: { occurrence_date: occurrenceDate, reason }
    });
    if (!response.ok) throw toAPIClientError(response, error);
  } catch (error) {
    if (error instanceof APIClientError) throw error;
    throw toNetworkError(error);
  }
}

export async function retryRecurringOccurrence(templateID: number, occurrenceDate: string, csrfToken: string): Promise<RecurringGenerationResponse> {
  try {
    const { data, error, response } = await apiClient.POST('/api/v1/recurring/templates/{template_id}/retry', {
      params: { path: { template_id: templateID }, header: { 'X-CSRF-Token': csrfToken } },
      body: { occurrence_date: occurrenceDate }
    });
    if (data !== undefined) return data;
    throw toAPIClientError(response, error);
  } catch (error) {
    if (error instanceof APIClientError) throw error;
    throw toNetworkError(error);
  }
}

export const recurringSummaryQueryKey = ['api', 'recurring', 'summary'] as const;
export function recurringSummaryQueryOptions() {
  return { queryKey: recurringSummaryQueryKey, queryFn: getRecurringSummary, staleTime: 5_000, refetchInterval: 30_000 };
}
export async function getRecurringSummary(): Promise<components['schemas']['RecurringSummaryResponse']> {
  try {
    const { data, error, response } = await apiClient.GET('/api/v1/recurring/summary');
    if (data !== undefined) return data;
    throw toAPIClientError(response, error);
  } catch (error) {
    if (error instanceof APIClientError) throw error;
    throw toNetworkError(error);
  }
}
export async function previewRecurringSchedule(schedule: RecurringTemplatePatch): Promise<components['schemas']['RecurringPreviewResponse']> {
  try {
    const { data, error, response } = await apiClient.POST('/api/v1/recurring/preview', { body: schedule });
    if (data !== undefined) return data;
    throw toAPIClientError(response, error);
  } catch (error) {
    if (error instanceof APIClientError) throw error;
    throw toNetworkError(error);
  }
}
export async function runRecurringNow(templateID: number, csrfToken: string): Promise<RecurringGenerationResponse> {
  try {
    const { data, error, response } = await apiClient.POST('/api/v1/recurring/templates/{template_id}/run-now', {
      params: { path: { template_id: templateID }, header: { 'X-CSRF-Token': csrfToken } }, body: {}
    });
    if (data !== undefined) return data;
    throw toAPIClientError(response, error);
  } catch (error) {
    if (error instanceof APIClientError) throw error;
    throw toNetworkError(error);
  }
}
