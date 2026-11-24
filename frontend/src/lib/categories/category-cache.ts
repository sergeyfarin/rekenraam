import type { QueryClient, QueryKey } from '@tanstack/svelte-query';
import { accountsQueryKey } from '#lib/api/accounts.ts';
import { categoriesQueryKey, changesCategories } from '#lib/api/categories.ts';
import { searchCatalogQueryKey } from '#lib/api/search.ts';

type CacheClient = Pick<QueryClient, 'cancelQueries' | 'getQueryCache' | 'setQueryData' | 'invalidateQueries'>;

const categoryPath = /^\/api\/v1\/categories\/(\d+)(\/disable)?$/;

/**
 * The category a successful write takes out of everyday choices: a disable or
 * a delete. A restore or an edit only needs the refetch.
 */
export function categoryLeavingPickers(method: string, path: string): number | undefined {
  const match = categoryPath.exec(path);
  if (!match) return undefined;
  const disable = match[2] !== undefined;
  if ((disable && method === 'POST') || (!disable && method === 'DELETE')) return Number(match[1]);
  return undefined;
}

/**
 * Applies a successful category write to every cached category and account
 * list before the caller sees the response (#664 review). The ledger accepts a
 * posting dated a category's disable day, so a picker must not offer it even
 * while its refetch is pending: a disabled category leaves active-only lists
 * and pickers at once and is marked archived where archived rows are listed; a
 * deleted one leaves every list. Reads already in flight are cancelled first,
 * keeping current data, so a response begun before the write cannot restore
 * the row. Then every list is refetched.
 */
export async function applyCategoryWrite(queryClient: CacheClient, method: string, path: string): Promise<void> {
  if (!changesCategories(path)) return;
  const leaving = categoryLeavingPickers(method, path);
  if (leaving !== undefined) {
    await Promise.all([categoriesQueryKey, accountsQueryKey].map((queryKey) => queryClient.cancelQueries({ queryKey }, { revert: false })));
    const deleted = method === 'DELETE';
    for (const query of queryClient.getQueryCache().findAll({ predicate: (q) => startsWith(q.queryKey, categoriesQueryKey) || startsWith(q.queryKey, accountsQueryKey) })) {
      const field = startsWith(query.queryKey, categoriesQueryKey) ? 'categories' : 'accounts';
      const keep = !deleted && listsArchived(query.queryKey);
      queryClient.setQueryData(query.queryKey, (data: unknown) => withoutCategory(data, field, leaving, keep));
    }
  }
  for (const queryKey of [categoriesQueryKey, accountsQueryKey, searchCatalogQueryKey]) {
    void queryClient.invalidateQueries({ queryKey });
  }
}

function startsWith(key: QueryKey, prefix: readonly unknown[]): boolean {
  return prefix.every((part, index) => key[index] === part);
}

// A list option object such as { includeArchived: true } keeps archived rows.
function listsArchived(key: QueryKey): boolean {
  const options = key[key.length - 1];
  return typeof options === 'object' && options !== null && (options as { includeArchived?: unknown }).includeArchived === true;
}

function withoutCategory(data: unknown, field: string, id: number, keepAsArchived: boolean): unknown {
  if (typeof data !== 'object' || data === null) return data;
  const rows = (data as Record<string, unknown>)[field];
  if (!Array.isArray(rows) || !rows.some((row) => isRow(row, id))) return data;
  const next = keepAsArchived
    ? rows.map((row) => (isRow(row, id) ? { ...row, status: 'archived' } : row))
    : rows.filter((row) => !isRow(row, id));
  return { ...data, [field]: next };
}

function isRow(row: unknown, id: number): boolean {
  return typeof row === 'object' && row !== null && (row as { id?: unknown }).id === id;
}
