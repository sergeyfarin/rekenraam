import type { QueryClient } from '@tanstack/svelte-query';
import { reconciliationQueryKey } from '#lib/api/reconciliation.ts';
import { accountRegisterQueryKey, transactionsQueryKey } from '#lib/api/transactions.ts';

/**
 * The reads a finished or voided statement changes for one account: the
 * reconciliation reads, that account's register (its statement rows and
 * posting statuses, #595) and the transaction lists' reconciliation status.
 * Mounted reads refetch now; the rest are marked stale, so returning to the
 * register from Reconcile never shows a voided statement as reconciled.
 */
export function reconciliationWriteReadKeys(accountID: number) {
  return [reconciliationQueryKey, [...accountRegisterQueryKey, accountID], transactionsQueryKey] as const;
}

export async function invalidateReconciliationReads(queryClient: Pick<QueryClient, 'invalidateQueries'>, accountID: number): Promise<void> {
  await Promise.all(reconciliationWriteReadKeys(accountID).map((queryKey) => queryClient.invalidateQueries({ queryKey })));
}
