import { describe, it, expect } from 'vitest';
import { parseStatementBalance } from './statement-balance';

describe('parseStatementBalance', () => {
  it.each([
    ['500', { value: '-500', scale: 0 }],
    ['-42.10', { value: '4210', scale: 2 }],
    ['-0.00', { value: '0', scale: 2 }],
    ['90071992547409.93', { value: '-9007199254740993', scale: 2 }],
    ['abc', null]
  ])('converts a liability statement balance of %s to debit-positive', (input, expected) => {
    expect(parseStatementBalance(input, 'liability')).toEqual(expected);
  });

  it('parses a plain integer (scale 0)', () => {
    expect(parseStatementBalance('1234', 'asset')).toEqual({ value: '1234', scale: 0 });
  });

  it('parses two decimal places', () => {
    expect(parseStatementBalance('1234.56', 'asset')).toEqual({ value: '123456', scale: 2 });
  });

  it('keeps the scale the user typed (trailing zeros)', () => {
    expect(parseStatementBalance('100.00', 'asset')).toEqual({ value: '10000', scale: 2 });
  });

  it('parses an amount smaller than one unit', () => {
    expect(parseStatementBalance('0.05', 'asset')).toEqual({ value: '5', scale: 2 });
  });

  it('keeps a negative balance signed', () => {
    expect(parseStatementBalance('-42.10', 'asset')).toEqual({ value: '-4210', scale: 2 });
  });

  it('strips thousands separators', () => {
    expect(parseStatementBalance('1,234.56', 'asset')).toEqual({ value: '123456', scale: 2 });
  });

  it('strips leading zeros on the integer part', () => {
    expect(parseStatementBalance('007.50', 'asset')).toEqual({ value: '750', scale: 2 });
  });

  it('normalises negative zero to unsigned zero', () => {
    expect(parseStatementBalance('-0.00', 'asset')).toEqual({ value: '0', scale: 2 });
  });

  it('preserves precision beyond Number.MAX_SAFE_INTEGER', () => {
    expect(parseStatementBalance('90071992547409.93', 'asset')).toEqual({
      value: '9007199254740993',
      scale: 2
    });
  });

  it('trims surrounding whitespace', () => {
    expect(parseStatementBalance('  12.00  ', 'asset')).toEqual({ value: '1200', scale: 2 });
  });

  it('returns null for an empty string', () => {
    expect(parseStatementBalance('', 'asset')).toBeNull();
    expect(parseStatementBalance('   ', 'asset')).toBeNull();
  });

  it('returns null for non-numeric input', () => {
    expect(parseStatementBalance('abc', 'asset')).toBeNull();
    expect(parseStatementBalance('1.2.3', 'asset')).toBeNull();
    expect(parseStatementBalance('$5', 'asset')).toBeNull();
  });
});
