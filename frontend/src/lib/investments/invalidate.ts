import { ledgerQueryKey } from '#lib/api/ledger.ts';
import type { QueryClient } from '@tanstack/svelte-query';
import { priceManagementQueryKey } from '#lib/api/price-management.ts';
import { forecastQueryKey } from '#lib/api/forecast.ts';
import {
  cashInLieuLotsQueryKey, datedHoldingsQueryKey, incomeCoverageQueryKey, investmentCorrectionChainQueryKey, investmentGainsQueryKey, investmentInstrumentsQueryKey,
  investmentLotsQueryKey, investmentPositionsQueryKey, investmentReturnsQueryKey, investmentSplitsQueryKey
} from '#lib/api/investments.ts';
import { accountRegisterQueryKey, transactionsQueryKey } from '#lib/api/transactions.ts';

// Every read an investment write can change. A correction or reversal can
// restate earlier lots and realized gains (replay), and an import can create
// instruments, so one list serves every investment mutation (T-138).
export const investmentMutationQueryKeys = [
  ledgerQueryKey,
  transactionsQueryKey,
  accountRegisterQueryKey,
  forecastQueryKey,
  priceManagementQueryKey,
  investmentPositionsQueryKey,
  investmentSplitsQueryKey,
  investmentLotsQueryKey,
  cashInLieuLotsQueryKey,
  datedHoldingsQueryKey,
  investmentGainsQueryKey,
  investmentInstrumentsQueryKey,
  investmentCorrectionChainQueryKey,
  incomeCoverageQueryKey,
  investmentReturnsQueryKey
] as const;

export async function invalidateInvestmentReads(queryClient: QueryClient): Promise<void> {
  await Promise.all(investmentMutationQueryKeys.map((queryKey) => queryClient.invalidateQueries({ queryKey })));
}
