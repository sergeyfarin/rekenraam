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
