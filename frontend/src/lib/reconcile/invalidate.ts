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

export async function invalidateReconciliationReads(queryClient: Pick<QueryClient, 'invalidateQueries'>, accountID: number): Promise<void> {
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

/** A voided statement: drop its cached row, then refetch what it changed. */
export async function afterStatementVoided(
  queryClient: Pick<QueryClient, 'setQueriesData' | 'invalidateQueries'>,
  accountID: number,
  checkpointID: number
): Promise<void> {
  dropStatementFromCachedRegister(queryClient, accountID, checkpointID);
  await invalidateReconciliationReads(queryClient, accountID);
}
