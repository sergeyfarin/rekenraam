import type { QueryClient } from '@tanstack/svelte-query';
import { describe, expect, it } from 'vitest';
import { forecastQueryKey } from '#lib/api/forecast.ts';
import {
  cashInLieuLotsQueryKey, investmentCorrectionChainQueryKey, investmentGainsQueryKey, investmentInstrumentsQueryKey,
  investmentLotsQueryKey, investmentPositionsQueryKey
} from '#lib/api/investments.ts';
import { accountRegisterQueryKey, transactionsQueryKey } from '#lib/api/transactions.ts';
import { invalidateInvestmentReads } from './invalidate';

describe('invalidateInvestmentReads', () => {
  it('invalidates every read an investment write can restate, including gains', async () => {
    const invalidated: unknown[] = [];
    const queryClient = {
      invalidateQueries: async (filters: { queryKey: unknown }) => { invalidated.push(filters.queryKey); }
    } as unknown as QueryClient;

    await invalidateInvestmentReads(queryClient);

    expect(invalidated).toEqual(expect.arrayContaining([
      cashInLieuLotsQueryKey, investmentGainsQueryKey, investmentPositionsQueryKey, investmentLotsQueryKey,
      investmentInstrumentsQueryKey, investmentCorrectionChainQueryKey,
      forecastQueryKey, transactionsQueryKey, accountRegisterQueryKey
    ]));
  });
});
