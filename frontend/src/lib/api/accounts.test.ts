import { afterEach, describe, expect, it, vi } from 'vitest';
import { APIClientError, apiClient } from './client';
import { accountQueryOptions, accountsQueryKey, getAccounts, type AccountResponse } from './accounts';

afterEach(() => vi.restoreAllMocks());

const account = (id: number) => ({ id, name: `Account ${id}` }) as AccountResponse;

describe('accounts API client', () => {
  it('follows every cursor page into one complete catalog (#518)', async () => {
    const get = vi.spyOn(apiClient, 'GET')
      .mockResolvedValueOnce({ data: { accounts: [account(1), account(2)], next_cursor: 'page-2' }, response: new Response() })
      .mockResolvedValueOnce({ data: { accounts: [account(2), account(3)], next_cursor: 'page-3' }, response: new Response() })
      .mockResolvedValueOnce({ data: { accounts: [account(4)], next_cursor: null }, response: new Response() });

    const result = await getAccounts(true, false);

    expect(result.accounts.map((row) => row.id)).toEqual([1, 2, 3, 4]);
    expect(get).toHaveBeenCalledTimes(3);
    expect(get).toHaveBeenNthCalledWith(1, '/api/v1/accounts', { params: { query: { include_archived: true, include_system: undefined, cursor: undefined } } });
    expect(get).toHaveBeenNthCalledWith(3, '/api/v1/accounts', { params: { query: { include_archived: true, include_system: undefined, cursor: 'page-3' } } });
  });

  it('fails the whole read when a later page fails instead of returning a partial catalog', async () => {
    vi.spyOn(apiClient, 'GET')
      .mockResolvedValueOnce({ data: { accounts: [account(1)], next_cursor: 'page-2' }, response: new Response() })
      .mockResolvedValueOnce({ error: { error: { code: 'VALIDATION_FAILED', message: 'bad cursor' } }, response: new Response(null, { status: 400 }) });

    await expect(getAccounts()).rejects.toMatchObject({ status: 400, code: 'VALIDATION_FAILED' } satisfies Partial<APIClientError>);
  });

  it('stops on a cursor that does not advance', async () => {
    const get = vi.spyOn(apiClient, 'GET')
      .mockResolvedValueOnce({ data: { accounts: [account(1)], next_cursor: 'same' }, response: new Response() })
      .mockResolvedValueOnce({ data: { accounts: [account(1)], next_cursor: 'same' }, response: new Response() });

    expect((await getAccounts()).accounts).toHaveLength(1);
    expect(get).toHaveBeenCalledTimes(2);
  });

  it('keys a single account under the accounts key so account invalidation refreshes it', () => {
    expect(accountQueryOptions(7).queryKey).toEqual([...accountsQueryKey, 'detail', 7]);
  });
});
