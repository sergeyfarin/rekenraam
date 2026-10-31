import type { QueryClient } from '@tanstack/svelte-query';
import { budgetQueryKey } from '#lib/api/budgets.ts';
import { forecastQueryKey } from '#lib/api/forecast.ts';
import { datedHoldingsQueryKey, investmentPositionsQueryKey, investmentReturnsQueryKey } from '#lib/api/investments.ts';
import { ledgerQueryKey } from '#lib/api/ledger.ts';
import { priceManagementQueryKey } from '#lib/api/price-management.ts';
import { reconciliationQueryKey } from '#lib/api/reconciliation.ts';
import { reportsQueryKey } from '#lib/api/reports.ts';
import { accountRegisterQueryKey, transactionsQueryKey } from '#lib/api/transactions.ts';

/**
 * Every read an ordinary transaction write can change. The shared entry
 * dialog (#477) saves over any page, so the page beneath it (a report, a
 * budget month, balances, a reconciliation, a brokerage's cash, or Prices,
 * whose coverage lists the rates posted exposure needs) must refetch,
 * not only the transaction lists. Mounted reads refetch now; the rest are
 * marked stale for their next visit. Home and the attention count are
 * refreshed by the shell's write middleware.
 */
export const transactionWriteReadKeys = [
  transactionsQueryKey,
  accountRegisterQueryKey,
  ledgerQueryKey,
  reportsQueryKey,
  budgetQueryKey,
  forecastQueryKey,
  reconciliationQueryKey,
  priceManagementQueryKey,
  investmentPositionsQueryKey,
  datedHoldingsQueryKey,
  investmentReturnsQueryKey
] as const;

export async function invalidateTransactionReads(queryClient: QueryClient): Promise<void> {
  await Promise.all(transactionWriteReadKeys.map(queryKey => queryClient.invalidateQueries({ queryKey })));
}
