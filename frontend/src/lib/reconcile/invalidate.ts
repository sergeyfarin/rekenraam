import type { QueryClient } from '@tanstack/svelte-query';
import { reconciliationQueryKey } from '#lib/api/reconciliation.ts';
import { accountRegisterQueryKey, transactionsQueryKey, type AccountRegisterInfiniteData } from '#lib/api/transactions.ts';

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

/**
 * Cancels the reads a statement change affects that are still in flight. A
 * response requested before the write would otherwise land after it, replace
 * the cache with the earlier state and clear the stale mark, so a voided
 * statement came back as reconciled (#595 review). Cancelling first keeps the
 * cache at what it held before the request; invalidation then fetches anew.
 */
export async function cancelReconciliationReads(queryClient: Pick<QueryClient, 'cancelQueries'>, accountID: number): Promise<void> {
  await Promise.all(reconciliationWriteReadKeys(accountID).map((queryKey) => queryClient.cancelQueries({ queryKey })));
}

export async function invalidateReconciliationReads(
  queryClient: Pick<QueryClient, 'cancelQueries' | 'invalidateQueries'>,
  accountID: number
): Promise<void> {
  await cancelReconciliationReads(queryClient, accountID);
  await Promise.all(reconciliationWriteReadKeys(accountID).map((queryKey) => queryClient.invalidateQueries({ queryKey })));
}

/**
 * Removes a voided statement's row from the account's cached register pages
 * at once (#595 review). The refetch that invalidation starts can still be
 * pending when the owner goes back to the register, and until it lands the
 * cached pages would show the voided statement as reconciled. Statement
 * indexes count entries only, so the other rows keep their places.
 */
export function dropStatementFromCachedRegister(
  queryClient: Pick<QueryClient, 'setQueriesData'>,
  accountID: number,
  checkpointID: number
): void {
  queryClient.setQueriesData<AccountRegisterInfiniteData>(
    { queryKey: [...accountRegisterQueryKey, accountID] },
    (data) => {
      if (!data?.pages) return data;
      let changed = false;
      const pages = data.pages.map((page) => {
        const statements = page.statements ?? [];
        const kept = statements.filter((statement) => statement.checkpoint_id !== checkpointID);
        if (kept.length === statements.length) return page;
        changed = true;
        return { ...page, statements: kept };
      });
      return changed ? { ...data, pages } : data;
    }
  );
}

/**
 * A voided statement: cancel the account's in-flight reads, drop its cached
 * row, then refetch what it changed. In that order, no response requested
 * before the void can put the row back.
 */
export async function afterStatementVoided(
  queryClient: Pick<QueryClient, 'cancelQueries' | 'setQueriesData' | 'invalidateQueries'>,
  accountID: number,
  checkpointID: number
): Promise<void> {
  await cancelReconciliationReads(queryClient, accountID);
  dropStatementFromCachedRegister(queryClient, accountID, checkpointID);
  await invalidateReconciliationReads(queryClient, accountID);
}
