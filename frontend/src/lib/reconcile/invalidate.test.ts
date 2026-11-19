import { describe, expect, it } from 'vitest';
import { QueryClient } from '@tanstack/svelte-query';
import { reconciliationQueryKey } from '#lib/api/reconciliation.ts';
import {
  accountRegisterInfiniteQueryOptions,
  accountRegisterQueryKey,
  transactionsQueryKey,
  type AccountRegisterInfiniteData
} from '#lib/api/transactions.ts';
import type { AccountRegisterStatementResponse } from '#lib/transactions/register-statements.ts';
import { afterStatementVoided, invalidateReconciliationReads } from './invalidate.ts';

function prefixOf(prefix: readonly unknown[], key: readonly unknown[]): boolean {
  return prefix.every((part, index) => JSON.stringify(part) === JSON.stringify(key[index]));
}

describe('invalidateReconciliationReads', () => {
  it('refreshes the account register with the reconciliation reads (#595 review)', async () => {
    const keys: (readonly unknown[])[] = [];
    const cancelled: (readonly unknown[])[] = [];
    await invalidateReconciliationReads({
      cancelQueries: async (filters) => { cancelled.push(filters?.queryKey ?? []); },
      invalidateQueries: async (filters) => { keys.push(filters?.queryKey ?? []); }
    }, 12);
    expect(cancelled).toEqual(keys);

    expect(keys).toContainEqual(reconciliationQueryKey);
    expect(keys).toContainEqual(transactionsQueryKey);
    const register = accountRegisterInfiniteQueryOptions(12, { status: 'posted' }).queryKey;
    expect(keys.some((key) => prefixOf(key, register))).toBe(true);
    const otherRegister = accountRegisterInfiniteQueryOptions(13).queryKey;
    expect(keys.some((key) => prefixOf(key, otherRegister))).toBe(false);
    expect(keys).not.toContainEqual(accountRegisterQueryKey);
  });
});

describe('afterStatementVoided', () => {
  function statement(checkpointID: number, beforeEntryIndex: number): AccountRegisterStatementResponse {
    return {
      checkpoint_id: checkpointID, commodity_id: 1, commodity_code: 'EUR', commodity_symbol: '€',
      statement_date: '2026-03-31', statement_account_sequence: 0,
      statement_balance: { commodity_id: 1, quantity_value: '100', quantity_scale: 0, normal_quantity_value: '100' },
      before_entry_index: beforeEntryIndex
    };
  }

  it('drops the voided row from cached register pages before the refetch lands (#595 review)', async () => {
    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    const key = accountRegisterInfiniteQueryOptions(12, { status: 'posted' }).queryKey;
    const otherKey = accountRegisterInfiniteQueryOptions(13, { status: 'posted' }).queryKey;
    const pages = {
      pageParams: ['', 'next'],
      pages: [
        { entries: [], statements: [statement(9, 0), statement(8, 0)], next_cursor: 'next' },
        { entries: [], statements: [statement(7, 0)], next_cursor: null }
      ]
    } as unknown as AccountRegisterInfiniteData;
    queryClient.setQueryData(key, pages);
    queryClient.setQueryData(otherKey, pages);
    // No observer is mounted (the owner is on Reconcile), so nothing refetches
    // here: what the register shows on return is the cache until it does.

    await afterStatementVoided(queryClient, 12, 8);

    const cached = queryClient.getQueryData<AccountRegisterInfiniteData>(key)!;
    expect(cached.pages.map((page) => page.statements.map((s) => s.checkpoint_id))).toEqual([[9], [7]]);
    expect(cached.pages[0].statements[0].before_entry_index).toBe(0);
    expect(queryClient.getQueryState(key)?.isInvalidated).toBe(true);
    // Another account's register is untouched.
    const other = queryClient.getQueryData<AccountRegisterInfiniteData>(otherKey)!;
    expect(other.pages[0].statements.map((s) => s.checkpoint_id)).toEqual([9, 8]);
  });

  it('keeps a register response requested before the void from restoring the row (#595 review)', async () => {
    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    const options = accountRegisterInfiniteQueryOptions(12, { status: 'posted' });
    const before = {
      pageParams: [''],
      pages: [{ entries: [], statements: [statement(9, 0), statement(8, 0)], next_cursor: null }]
    } as unknown as AccountRegisterInfiniteData;
    queryClient.setQueryData(options.queryKey, before);

    // A refetch requested before the void, answered only after it succeeded.
    let answer: (page: AccountRegisterInfiniteData['pages'][number]) => void = () => {};
    const held = new Promise<AccountRegisterInfiniteData['pages'][number]>((resolve) => { answer = resolve; });
    let requested = 0;
    const inflight = queryClient
      .fetchInfiniteQuery({ ...options, staleTime: 0, queryFn: () => { requested++; return held; } })
      .catch(() => undefined);
    await Promise.resolve();
    expect(requested).toBe(1);
    expect(queryClient.getQueryState(options.queryKey)?.fetchStatus).toBe('fetching');

    await afterStatementVoided(queryClient, 12, 8);
    answer(before.pages[0]);
    await inflight;

    const cached = queryClient.getQueryData<AccountRegisterInfiniteData>(options.queryKey)!;
    expect(cached.pages[0].statements.map((s) => s.checkpoint_id)).toEqual([9]);
    expect(queryClient.getQueryState(options.queryKey)?.isInvalidated).toBe(true);
  });
});
