import { describe, expect, it } from 'vitest';
import { QueryClient } from '@tanstack/svelte-query';
import { authSessionQueryKey } from '#lib/api/auth.ts';
import { categoriesQueryKey } from '#lib/api/categories.ts';
import { invalidateTransactionReads } from './invalidate.ts';

describe('invalidateTransactionReads', () => {
  it('marks the financial reads beneath an entry stale, and leaves reference data and the session alone', async () => {
    const client = new QueryClient();
    const affected = [
      ['api', 'reports', 'spending', { start: '2026-10-01' }],
      ['api', 'budgets', 'month', '2026-10-01'],
      ['api', 'ledger', 'account-balances', {}],
      ['api', 'transactions', { q: '' }],
      ['api', 'account-register', 7],
      ['api', 'forecasts', 'balances', {}],
      ['api', 'reconciliation', 'checkpoints', 7, 1],
      // Prices lists the rates posted exposure needs, created or removed by a write.
      ['api', 'pricing', 'management', 'workspace', '2026-10-31'],
      ['api', 'investments', 'returns', {}]
    ];
    const untouched = [authSessionQueryKey, categoriesQueryKey];
    for (const key of [...affected, ...untouched]) client.setQueryData(key, {});

    await invalidateTransactionReads(client);

    for (const key of affected) expect(client.getQueryState(key)?.isInvalidated, JSON.stringify(key)).toBe(true);
    for (const key of untouched) expect(client.getQueryState(key)?.isInvalidated, JSON.stringify(key)).toBe(false);
  });
});
