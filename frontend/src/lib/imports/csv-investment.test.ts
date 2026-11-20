import { describe, expect, it } from 'vitest';
import { locales } from '#lib/paraglide/runtime.js';
import { canonicalCSVInvestmentAmount, editableCSVInvestmentAmount } from './csv-investment';

describe('CSV investment review amounts', () => {
  it.each(locales)('round-trips exact source precision in %s', (locale) => {
    const source = '90071992547409.93';
    expect(canonicalCSVInvestmentAmount(editableCSVInvestmentAmount(source, locale), locale)).toBe(source);
    expect(canonicalCSVInvestmentAmount(editableCSVInvestmentAmount('0.00000001', locale), locale)).toBe('0.00000001');
  });
  it('rejects invalid grouping instead of changing the amount', () => {
    expect(canonicalCSVInvestmentAmount('1,50', 'en')).toBeNull();
    expect(canonicalCSVInvestmentAmount('1.50', 'nl')).toBeNull();
    expect(canonicalCSVInvestmentAmount('', 'en')).toBeNull();
  });
});
