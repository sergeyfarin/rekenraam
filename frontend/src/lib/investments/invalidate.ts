import type { QueryClient } from '@tanstack/svelte-query';
import { forecastQueryKey } from '#lib/api/forecast.ts';
import {
  cashInLieuLotsQueryKey, datedHoldingsQueryKey, investmentCorrectionChainQueryKey, investmentGainsQueryKey, investmentInstrumentsQueryKey,
  investmentLotsQueryKey, investmentPositionsQueryKey
} from '#lib/api/investments.ts';
import { accountRegisterQueryKey, transactionsQueryKey } from '#lib/api/transactions.ts';

// Every read an investment write can change. A correction or reversal can
// restate earlier lots and realized gains (replay), and an import can create
// instruments, so one list serves every investment mutation (T-138).
export const investmentMutationQueryKeys = [
  transactionsQueryKey,
  accountRegisterQueryKey,
  forecastQueryKey,
  investmentPositionsQueryKey,
  investmentLotsQueryKey,
  cashInLieuLotsQueryKey,
  datedHoldingsQueryKey,
  investmentGainsQueryKey,
  investmentInstrumentsQueryKey,
  investmentCorrectionChainQueryKey
] as const;

export async function invalidateInvestmentReads(queryClient: QueryClient): Promise<void> {
  await Promise.all(investmentMutationQueryKeys.map((queryKey) => queryClient.invalidateQueries({ queryKey })));
}
