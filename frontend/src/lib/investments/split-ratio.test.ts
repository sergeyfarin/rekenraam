import { describe, expect, it } from 'vitest';
import { basisFractionPercent, parseBasisPercent, parseExchangeRatio, parseSplitRatio } from './split-ratio';

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

describe('parseBasisPercent', () => {
  it.each([
    ['14.1', '141', 3],
    ['14,1', '141', 3],
    ['50', '5', 1],
    ['20', '2', 1],
    ['0.5', '5', 3],
    ['007', '7', 2],
    ['99.9999999999', '999999999999', 12]
  ])('reads %s %% as %s at scale %d', (input, value, scale) => {
    expect(parseBasisPercent(input)).toEqual({ ok: true, value, scale });
  });

  it.each(['', '0', '0,000', '100', '100.0', '150', '1,234.5', '14.1.2', '-5', '1e1', '99.99999999999', ' , '])(
    'refuses %j', (input) => {
      expect(parseBasisPercent(input)).toEqual({ ok: false });
    });
});

describe('basisFractionPercent', () => {
  it.each([
    ['2', 1, '20', 0],
    ['25', 2, '25', 0],
    ['141', 3, '141', 1],
    ['5', 3, '5', 1]
  ])('shows %s at scale %d as %s at scale %d', (value, scale, percentValue, percentScale) => {
    expect(basisFractionPercent(value, scale)).toEqual({ value: percentValue, scale: percentScale });
  });
});
