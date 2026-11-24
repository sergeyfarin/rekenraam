import { QueryClient } from '@tanstack/svelte-query';
import { describe, expect, it } from 'vitest';
import { applySavedRead } from './saved-read.ts';

const key = ['api', 'auth', 'mfa'] as const;

describe('applySavedRead', () => {
  it('keeps the saved state when an older cached fetch answers after it (#661)', async () => {
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    client.setQueryData(key, { status: 'pending' });
    // A reconnect refetch sent before the write committed, answered late.
    let answerOld!: (value: { status: string }) => void;
    const older = client.fetchQuery({
      queryKey: key,
      queryFn: () => new Promise<{ status: string }>((resolve) => (answerOld = resolve)),
      staleTime: 0
    }).catch(() => undefined);

    await applySavedRead(client, key, async () => ({ status: 'active' }));
    answerOld({ status: 'pending' });
    await older;

    expect(client.getQueryData(key)).toEqual({ status: 'active' });
  });

  it('leaves the cache untouched when the read fails', async () => {
    const client = new QueryClient();
    client.setQueryData(key, { status: 'pending' });

    await expect(applySavedRead(client, key, async () => { throw new Error('offline'); })).rejects.toThrow('offline');

    expect(client.getQueryData(key)).toEqual({ status: 'pending' });
  });
});
