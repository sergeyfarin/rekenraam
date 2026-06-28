import { describe, it, expect } from 'vitest';
import { parseStatementBalance } from './statement-balance';

describe('parseStatementBalance', () => {
  it('parses a plain integer (scale 0)', () => {
    expect(parseStatementBalance('1234')).toEqual({ value: '1234', scale: 0 });
  });

  it('parses two decimal places', () => {
    expect(parseStatementBalance('1234.56')).toEqual({ value: '123456', scale: 2 });
  });

  it('keeps the scale the user typed (trailing zeros)', () => {
    expect(parseStatementBalance('100.00')).toEqual({ value: '10000', scale: 2 });
  });

  it('parses an amount smaller than one unit', () => {
    expect(parseStatementBalance('0.05')).toEqual({ value: '5', scale: 2 });
  });

  it('keeps a negative balance signed', () => {
    expect(parseStatementBalance('-42.10')).toEqual({ value: '-4210', scale: 2 });
  });

  it('strips thousands separators', () => {
    expect(parseStatementBalance('1,234.56')).toEqual({ value: '123456', scale: 2 });
  });

  it('strips leading zeros on the integer part', () => {
    expect(parseStatementBalance('007.50')).toEqual({ value: '750', scale: 2 });
  });

  it('normalises negative zero to unsigned zero', () => {
    expect(parseStatementBalance('-0.00')).toEqual({ value: '0', scale: 2 });
  });

  it('preserves precision beyond Number.MAX_SAFE_INTEGER', () => {
    expect(parseStatementBalance('90071992547409.93')).toEqual({
      value: '9007199254740993',
      scale: 2
    });
  });

  it('trims surrounding whitespace', () => {
    expect(parseStatementBalance('  12.00  ')).toEqual({ value: '1200', scale: 2 });
  });

  it('returns null for an empty string', () => {
    expect(parseStatementBalance('')).toBeNull();
    expect(parseStatementBalance('   ')).toBeNull();
  });

  it('returns null for non-numeric input', () => {
    expect(parseStatementBalance('abc')).toBeNull();
    expect(parseStatementBalance('1.2.3')).toBeNull();
    expect(parseStatementBalance('$5')).toBeNull();
  });
});
