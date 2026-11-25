import { QueryClient } from '@tanstack/svelte-query';
import { describe, expect, it } from 'vitest';
import { forecastQueryKey } from '#lib/api/forecast.ts';
import {
  cashInLieuLotsQueryKey, incomeCoverageQueryKey, investmentCorrectionChainQueryKey, investmentGainsQueryKey, investmentInstrumentsQueryKey,
  investmentLotsQueryKey, investmentPositionsQueryKey, investmentReturnsQueryKey, investmentSplitsQueryKey
} from '#lib/api/investments.ts';
import { netWorthSeriesQueryOptions } from '#lib/api/reports.ts';
import { accountRegisterQueryKey, transactionsQueryKey } from '#lib/api/transactions.ts';
import { invalidateInvestmentReads } from './invalidate';

describe('invalidateInvestmentReads', () => {
  it('marks the canonical net-worth report stale after investment entry', async () => {
    const client = new QueryClient();
    const netWorth = netWorthSeriesQueryOptions({ startDate: '2026-10-01', endDate: '2026-10-31', bucket: 'month' }).queryKey;
    client.setQueryData(netWorth, {});

    await invalidateInvestmentReads(client);

    expect(client.getQueryState(netWorth)?.isInvalidated).toBe(true);
  });

  it('invalidates every read an investment write can restate, including gains', async () => {
    const invalidated: unknown[] = [];
    const queryClient = {
      invalidateQueries: async (filters: { queryKey: unknown }) => { invalidated.push(filters.queryKey); }
    } as unknown as QueryClient;

    await invalidateInvestmentReads(queryClient);

    expect(invalidated).toEqual(expect.arrayContaining([
      cashInLieuLotsQueryKey, investmentGainsQueryKey, investmentPositionsQueryKey, investmentLotsQueryKey,
      investmentInstrumentsQueryKey, investmentCorrectionChainQueryKey, investmentSplitsQueryKey,
      forecastQueryKey, transactionsQueryKey, accountRegisterQueryKey, incomeCoverageQueryKey, investmentReturnsQueryKey
    ]));
  });
});
