import { describe, expect, it } from 'vitest';
import { reconciliationQueryKey } from '#lib/api/reconciliation.ts';
import { accountRegisterInfiniteQueryOptions, accountRegisterQueryKey, transactionsQueryKey } from '#lib/api/transactions.ts';
import { invalidateReconciliationReads } from './invalidate.ts';

function prefixOf(prefix: readonly unknown[], key: readonly unknown[]): boolean {
  return prefix.every((part, index) => JSON.stringify(part) === JSON.stringify(key[index]));
}

describe('invalidateReconciliationReads', () => {
  it('refreshes the account register with the reconciliation reads (#595 review)', async () => {
    const keys: (readonly unknown[])[] = [];
    await invalidateReconciliationReads({ invalidateQueries: async (filters) => { keys.push(filters?.queryKey ?? []); } }, 12);

    expect(keys).toContainEqual(reconciliationQueryKey);
    expect(keys).toContainEqual(transactionsQueryKey);
    const register = accountRegisterInfiniteQueryOptions(12, { status: 'posted' }).queryKey;
    expect(keys.some((key) => prefixOf(key, register))).toBe(true);
    const otherRegister = accountRegisterInfiniteQueryOptions(13).queryKey;
    expect(keys.some((key) => prefixOf(key, otherRegister))).toBe(false);
    expect(keys).not.toContainEqual(accountRegisterQueryKey);
  });
});
