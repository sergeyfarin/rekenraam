import { describe, expect, it } from 'vitest';
import { QueryClient } from '@tanstack/svelte-query';
import { importMutationQueryKeys, invalidateImportReads } from './invalidate.ts';

describe('invalidateImportReads', () => {
  it('marks attention, Home, history, the Dividends queue and Ledger reads stale', async () => {
    const client = new QueryClient();
    for (const key of importMutationQueryKeys) client.setQueryData([...key, 'child'], 1);
    client.setQueryData(['api', 'unrelated'], 1);
    await invalidateImportReads(client);
    for (const key of importMutationQueryKeys) expect(client.getQueryState([...key, 'child'])?.isInvalidated).toBe(true);
    expect(client.getQueryState(['api', 'unrelated'])?.isInvalidated).toBe(false);
    expect(importMutationQueryKeys.map(key => key.join('/'))).toEqual(
      expect.arrayContaining(['api/attention', 'api/home', 'api/import-batches', 'api/import-dividends', 'api/ledger'])
    );
  });
});
