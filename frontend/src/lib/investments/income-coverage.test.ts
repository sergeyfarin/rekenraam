import { describe, expect, it } from 'vitest';
import { coverageMoney, coverageRepairHref } from './income-coverage';

describe('coverageMoney', () => {
  it('keeps every known digit and the currency label', () => {
    expect(coverageMoney({ value: '8500', scale: 2 }, 1, 'EUR', [{ id: 1, standard_scale: 2 }], 'en')).toBe('EUR 85.00');
    expect(coverageMoney({ value: '85004', scale: 3 }, 1, 'EUR', [{ id: 1, standard_scale: 2 }], 'en')).toBe('EUR 85.004');
  });

  it('formats a known zero as zero, not as missing', () => {
    expect(coverageMoney({ value: '0', scale: 0 }, 1, 'EUR', [{ id: 1, standard_scale: 2 }], 'en')).toBe('EUR 0.00');
  });
});

describe('coverageRepairHref', () => {
  it('sends imported cash to its batch review and named dividends to the payment', () => {
    expect(coverageRepairHref({ kind: 'imported_cash', transaction_id: 4, source: { batch_id: 9, row_id: 2, instrument: '', date: '2026-01-01', cash_amount: '1', currency: 'EUR' } }))
      .toBe('/app/ledger/import/dividends?batch_id=9');
    expect(coverageRepairHref({ kind: 'named_unassigned', transaction_id: 4, source: null })).toBe('/app/ledger/transactions?transaction_id=4');
  });
});
