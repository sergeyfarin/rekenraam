import { describe, expect, it } from 'vitest';
import { parseBudgetFilters } from './budget-url-filters.ts';

describe('budget URL filters', () => {
  it('reads a selected month and currency independently of unrelated parameters', () => {
    expect(parseBudgetFilters(new URLSearchParams('month=2026-02&currency_id=7&highlight=12')))
      .toEqual({ month: '2026-02', currencyID: 7 });
  });

  it.each(['', '2026-00', '2026-13', '0000-01', '2026-2', '2026-02-01', 'invalid'])(
    'uses the owner-local default for an invalid month %s', (month) => {
      expect(parseBudgetFilters(new URLSearchParams({ month })).month).toBe('');
    }
  );

  it.each(['', '0', '-1', '1.5', 'Infinity', '9007199254740992', 'invalid'])(
    'uses the currency default for an invalid ID %s', (currency_id) => {
      expect(parseBudgetFilters(new URLSearchParams({ currency_id })).currencyID).toBe(0);
    }
  );
});
