import { describe, expect, it } from 'vitest';
import { parseExchangeRatio, parseSplitRatio } from './split-ratio';

describe('parseSplitRatio', () => {
  it('accepts forward and reverse ratios as typed', () => {
    expect(parseSplitRatio('3', '2')).toEqual({ ok: true, numerator: 3, denominator: 2 });
    expect(parseSplitRatio(' 1 ', '10')).toEqual({ ok: true, numerator: 1, denominator: 10 });
  });

  it('refuses 1-for-1, zero, signs, decimals, separators and out-of-range sides', () => {
    for (const [a, b] of [['2', '2'], ['0', '1'], ['-2', '1'], ['1.5', '1'], ['1,000', '1'], ['', '1'], ['1000000001', '1']]) {
      expect(parseSplitRatio(a, b)).toEqual({ ok: false });
    }
  });
});

describe('parseExchangeRatio', () => {
  it('accepts a one-for-one conversion and uneven merger terms', () => {
    expect(parseExchangeRatio('1', '1')).toEqual({ ok: true, numerator: 1, denominator: 1 });
    expect(parseExchangeRatio('3', '2')).toEqual({ ok: true, numerator: 3, denominator: 2 });
  });

  it('refuses zero, signs, decimals, separators and out-of-range sides', () => {
    for (const [a, b] of [['0', '1'], ['-2', '1'], ['1.5', '1'], ['1,000', '1'], ['', '1'], ['1', '1000000001']]) {
      expect(parseExchangeRatio(a, b)).toEqual({ ok: false });
    }
  });
});
