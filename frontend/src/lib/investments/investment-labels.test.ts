import { afterEach, describe, expect, it, vi } from 'vitest';
import * as runtime from '#lib/paraglide/runtime.js';
import { COST_BASIS_METHODS, costBasisMethodLabel } from './investment-labels.ts';

afterEach(() => vi.restoreAllMocks());

describe('costBasisMethodLabel (#461)', () => {
  it.each(runtime.locales)('labels every method code through the %s catalog', (locale) => {
    vi.spyOn(runtime, 'getLocale').mockReturnValue(locale);
    const labels = COST_BASIS_METHODS.map(costBasisMethodLabel);
    for (const [index, label] of labels.entries()) {
      expect(label, COST_BASIS_METHODS[index]).not.toBe(COST_BASIS_METHODS[index]);
      expect(label.trim()).not.toBe('');
    }
    expect(new Set(labels).size).toBe(COST_BASIS_METHODS.length);
  });

  it('does not fall back to English labels in other locales', () => {
    vi.spyOn(runtime, 'getLocale').mockReturnValue('en');
    const english = [costBasisMethodLabel('average_cost'), costBasisMethodLabel('specific_lot')];
    for (const locale of runtime.locales.filter((code) => code !== 'en')) {
      vi.spyOn(runtime, 'getLocale').mockReturnValue(locale);
      expect(costBasisMethodLabel('average_cost'), locale).not.toBe(english[0]);
      expect(costBasisMethodLabel('specific_lot'), locale).not.toBe(english[1]);
    }
  });

  it('keeps the durable method codes', () => {
    expect(COST_BASIS_METHODS).toEqual(['fifo', 'lifo', 'average_cost', 'specific_lot']);
  });
});
