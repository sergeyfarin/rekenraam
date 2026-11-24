import { describe, expect, it } from 'vitest';
import { QueryClient } from '@tanstack/svelte-query';
import { applyCategoryWrite, categoryLeavingPickers } from './category-cache.ts';

const activeCategories = ['api', 'categories', {}] as const;
const allCategories = ['api', 'categories', { includeArchived: true }] as const;
const activeAccounts = ['api', 'accounts', { includeArchived: false, includeSystem: false }] as const;
const dividendAccounts = ['api', 'accounts', 'investment-entry', '2026-11-24'] as const;
const accountDetail = ['api', 'accounts', 'detail', 7] as const;

function seeded(): QueryClient {
  const client = new QueryClient({ defaultOptions: { queries: { staleTime: Infinity, retry: false } } });
  const rows = [{ id: 7, status: 'active' }, { id: 8, status: 'active' }];
  client.setQueryData(activeCategories, { categories: rows });
  client.setQueryData(allCategories, { categories: rows });
  client.setQueryData(activeAccounts, { accounts: rows });
  client.setQueryData(dividendAccounts, { accounts: rows });
  client.setQueryData(accountDetail, { id: 7, status: 'active' });
  return client;
}

const ids = (client: QueryClient, key: readonly unknown[], field: string) =>
  (client.getQueryData<Record<string, Array<{ id: number; status: string }>>>(key)?.[field] ?? []).map((row) => `${row.id}:${row.status}`);

describe('a category write applied to cached pickers (#664 review)', () => {
  it('names the category a disable or delete takes out of pickers', () => {
    expect(categoryLeavingPickers('POST', '/api/v1/categories/7/disable')).toBe(7);
    expect(categoryLeavingPickers('DELETE', '/api/v1/categories/7')).toBe(7);
    expect(categoryLeavingPickers('POST', '/api/v1/categories/7/restore')).toBeUndefined();
    expect(categoryLeavingPickers('PATCH', '/api/v1/categories/7')).toBeUndefined();
    expect(categoryLeavingPickers('POST', '/api/v1/categories')).toBeUndefined();
  });

  it('drops a disabled category from active lists and pickers and marks it archived where archived rows show', async () => {
    const client = seeded();
    await applyCategoryWrite(client, 'POST', '/api/v1/categories/7/disable');
    expect(ids(client, activeCategories, 'categories')).toEqual(['8:active']);
    expect(ids(client, activeAccounts, 'accounts')).toEqual(['8:active']);
    expect(ids(client, dividendAccounts, 'accounts')).toEqual(['8:active']);
    expect(ids(client, allCategories, 'categories')).toEqual(['7:archived', '8:active']);
    expect(client.getQueryData(accountDetail)).toEqual({ id: 7, status: 'active' });
    expect(client.getQueryState(activeCategories)?.isInvalidated).toBe(true);
    expect(client.getQueryState(dividendAccounts)?.isInvalidated).toBe(true);
  });

  it('drops a deleted category from every list', async () => {
    const client = seeded();
    await applyCategoryWrite(client, 'DELETE', '/api/v1/categories/7');
    expect(ids(client, allCategories, 'categories')).toEqual(['8:active']);
    expect(ids(client, activeAccounts, 'accounts')).toEqual(['8:active']);
  });

  it('only refetches after a restore', async () => {
    const client = seeded();
    await applyCategoryWrite(client, 'POST', '/api/v1/categories/7/restore');
    expect(ids(client, activeCategories, 'categories')).toEqual(['7:active', '8:active']);
    expect(client.getQueryState(activeCategories)?.isInvalidated).toBe(true);
  });

  it('a read begun before the disable cannot put the category back', async () => {
    const client = seeded();
    let release: (value: unknown) => void = () => {};
    const stale = client.fetchQuery({
      queryKey: activeCategories,
      queryFn: () => new Promise((resolve) => { release = resolve; }),
      staleTime: 0
    }).catch(() => undefined);
    await applyCategoryWrite(client, 'POST', '/api/v1/categories/7/disable');
    release({ categories: [{ id: 7, status: 'active' }, { id: 8, status: 'active' }] });
    await stale;
    expect(ids(client, activeCategories, 'categories')).toEqual(['8:active']);
  });
});
