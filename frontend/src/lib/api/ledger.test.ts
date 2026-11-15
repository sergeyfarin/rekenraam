import { afterEach, describe, expect, it, vi } from 'vitest';
import { apiClient } from './client.ts';
import { ledgerOverviewQueryOptions } from './ledger.ts';
afterEach(() => vi.restoreAllMocks());
describe('composed Ledger read (#448)', () => {
  it('keeps the first page date across cursors and never truncates or duplicates account rows', async () => {
    const get = vi.spyOn(apiClient, 'GET')
      .mockResolvedValueOnce({ data: { as_of: '2026-11-13', accounts: [{ account: { id: 1 } }], institutions: [], commodities: [], next_cursor: '1' }, response: new Response() })
      .mockResolvedValueOnce({ data: { as_of: '2026-11-13', accounts: [{ account: { id: 1 } }, { account: { id: 2 } }], institutions: [], commodities: [], next_cursor: null }, response: new Response() });
    const result = await ledgerOverviewQueryOptions().queryFn();
    expect(result.accounts.map(row => row.account.id)).toEqual([1, 2]);
    expect(get).toHaveBeenNthCalledWith(2, '/api/v1/ledger/overview', { params: { query: { as_of: '2026-11-13', cursor: '1' } } });
  });
  it('fails a later page instead of displaying a partial account index', async () => {
    vi.spyOn(apiClient, 'GET')
      .mockResolvedValueOnce({ data: { as_of: '2026-11-13', accounts: [], institutions: [], commodities: [], next_cursor: '1' }, response: new Response() })
      .mockResolvedValueOnce({ error: { error: { code: 'INTERNAL_ERROR' } }, response: new Response(null, { status: 500 }) });
    await expect(ledgerOverviewQueryOptions().queryFn()).rejects.toMatchObject({ status: 500 });
  });
});
