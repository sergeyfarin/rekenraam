import { afterEach, describe, expect, it, vi } from 'vitest';
import { apiClient, APIClientError } from './client';
import { getRecurringTemplates, createRecurringTemplate, updateRecurringTemplate, archiveRecurringTemplate } from './recurring';

afterEach(() => vi.restoreAllMocks());

describe('recurring template client', () => {
  it('preserves false, zero, null and an empty replacement set in PATCH', async () => {
    const patch = { enabled: false, lead_days: 0, ends_on: null, day_of_month: null, tag_ids: [] };
    const request = vi.spyOn(apiClient, 'PATCH').mockResolvedValue({ data: undefined, error: undefined, response: new Response(null, { status: 500 }) });
    await expect(updateRecurringTemplate(7, patch, 'test-csrf')).rejects.toBeInstanceOf(APIClientError);
    expect(request).toHaveBeenCalledWith('/api/v1/recurring/templates/{template_id}', {
      params: { path: { template_id: 7 }, header: { 'X-CSRF-Token': 'test-csrf' } }, body: patch
    });
    expect(patch).not.toHaveProperty('name');
  });

  it('keeps large quantity coefficients as strings on create', async () => {
    const request = vi.spyOn(apiClient, 'POST').mockResolvedValue({ data: undefined, error: undefined, response: new Response(null, { status: 500 }) });
    const input = { name: 'Example', frequency: 'monthly' as const, day_of_month: 1, starts_on: '2026-09-01', postings: [
      { account_id: 1, commodity_id: 1, quantity_value: '9007199254740993', quantity_scale: 2 },
      { account_id: 2, commodity_id: 1, quantity_value: '-9007199254740993', quantity_scale: 2 }
    ] };
    await expect(createRecurringTemplate(input, 'test-csrf')).rejects.toBeInstanceOf(APIClientError);
    expect(request).toHaveBeenCalledWith('/api/v1/recurring/templates', {
      params: { header: { 'X-CSRF-Token': 'test-csrf' } }, body: input
    });
  });

  it('archives through the authenticated JSON mutation route', async () => {
    const request = vi.spyOn(apiClient, 'POST').mockResolvedValue({ data: undefined, error: undefined, response: new Response(null, { status: 500 }) });
    await expect(archiveRecurringTemplate(7, 'test-csrf')).rejects.toBeInstanceOf(APIClientError);
    expect(request).toHaveBeenCalledWith('/api/v1/recurring/templates/{template_id}/archive', {
      params: { path: { template_id: 7 }, header: { 'X-CSRF-Token': 'test-csrf' } }, body: {}
    });
  });

  it('returns the complete list and preserves stable error codes', async () => {
    vi.spyOn(apiClient, 'GET').mockResolvedValueOnce({ data: { templates: [] }, response: new Response() });
    expect(await getRecurringTemplates()).toEqual({ templates: [] });
    vi.spyOn(apiClient, 'PATCH').mockResolvedValue({ error: { error: { code: 'RECURRING_TEMPLATE_ARCHIVED', message: 'archived' } }, response: new Response(null, { status: 409 }) });
    await expect(updateRecurringTemplate(7, { name: 'Example' }, 'test-csrf')).rejects.toMatchObject({ status: 409, code: 'RECURRING_TEMPLATE_ARCHIVED' });
  });
});

describe('recurring review client', () => {
  it('continues the inbox cursor until the backend reports the last page', async () => {
    const { recurringDueInfiniteQueryOptions } = await import('./recurring');
    const get = vi.spyOn(apiClient, 'GET')
      .mockResolvedValueOnce({ data: { items: [], next_cursor: 'next-page' }, response: new Response() })
      .mockResolvedValueOnce({ data: { items: [], next_cursor: null }, response: new Response() });
    const options = recurringDueInfiniteQueryOptions(20);
    const first = await options.queryFn({ pageParam: options.initialPageParam });
    const next = options.getNextPageParam(first);
    expect(next).toBe('next-page');
    const last = await options.queryFn({ pageParam: next! });
    expect(options.getNextPageParam(last)).toBeNull();
    expect(get).toHaveBeenLastCalledWith('/api/v1/recurring/due', { params: { query: { cursor: 'next-page', limit: 20 } } });
  });

  it('accepts an empty skip response and distinguishes a still-blocked retry', async () => {
    const { skipRecurringOccurrence, retryRecurringOccurrence } = await import('./recurring');
    const post = vi.spyOn(apiClient, 'POST')
      .mockResolvedValueOnce({ response: new Response(null, { status: 204 }) })
      .mockResolvedValueOnce({ data: { generated: 0, blocked: 1 }, response: new Response() });
    await expect(skipRecurringOccurrence(7, '2026-09-01', 'Not this month', 'csrf')).resolves.toBeUndefined();
    expect(post).toHaveBeenNthCalledWith(1, '/api/v1/recurring/templates/{template_id}/skip', {
      params: { path: { template_id: 7 }, header: { 'X-CSRF-Token': 'csrf' } }, body: { occurrence_date: '2026-09-01', reason: 'Not this month' }
    });
    await expect(retryRecurringOccurrence(7, '2026-10-01', 'csrf')).resolves.toEqual({ generated: 0, blocked: 1 });
  });

  it('uses saved-draft posting preview without submitting an edited transaction', async () => {
    const { getPostReconciliationImpact } = await import('./transactions');
    const get = vi.spyOn(apiClient, 'GET').mockResolvedValue({ data: { affected_checkpoints: [] }, response: new Response() });
    expect(await getPostReconciliationImpact(11)).toEqual({ affected_checkpoints: [] });
    expect(get).toHaveBeenCalledWith('/api/v1/transactions/{transaction_id}/post/reconciliation-impact', {
      params: { path: { transaction_id: 11 } }
    });
  });
});
