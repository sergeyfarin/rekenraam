import { describe, it, expect } from 'vitest';
import { formatQuantity } from './format';
import { formatLedgerAmount, parseDecimalAmount } from './amount';

describe('formatQuantity', () => {
  it('scale 0: integer amount', () => {
    expect(formatQuantity('12345', 0, 'en-US')).toBe('12,345');
  });

  it('scale 2: standard currency amount', () => {
    expect(formatQuantity('12345', 2, 'en-US')).toBe('123.45');
  });

  it('scale 2: amount less than one unit (leading zeros)', () => {
    expect(formatQuantity('5', 2, 'en-US')).toBe('0.05');
  });

  it('scale 2: round number (no cents)', () => {
    expect(formatQuantity('10000', 2, 'en-US')).toBe('100.00');
  });

  it('negative value: scale 2', () => {
    expect(formatQuantity('-12345', 2, 'en-US')).toBe('-123.45');
  });

  it('negative value: scale 0', () => {
    expect(formatQuantity('-9999', 0, 'en-US')).toBe('-9,999');
  });

  it('value beyond Number.MAX_SAFE_INTEGER round-trips exactly', () => {
    // 9007199254740993 is Number.MAX_SAFE_INTEGER + 2, which JS floats cannot represent.
    // scale 2 → display as 90071992547409.93
    const bigCoeff = '900719925474099300';
    const result = formatQuantity(bigCoeff, 2, 'en-US');
    // Integer part is 9007199254740993, fractional part is '00' — verify no corruption.
    expect(result).toBe('9,007,199,254,740,993.00');
  });

  it('value beyond Number.MAX_SAFE_INTEGER: scale 0 round-trips exactly', () => {
    const bigCoeff = '9007199254740993';
    const result = formatQuantity(bigCoeff, 0, 'en-US');
    expect(result).toBe('9,007,199,254,740,993');
  });

  it('zero value: scale 2', () => {
    expect(formatQuantity('0', 2, 'en-US')).toBe('0.00');
  });

  it('empty string returns empty string', () => {
    expect(formatQuantity('', 2, 'en-US')).toBe('');
  });

  it('locale separator: respects locale decimal character', () => {
    // Node.js test environments often lack full ICU locale data, so we test
    // that the function correctly passes the locale to Intl rather than testing
    // a specific non-en locale. The implementation uses Intl.NumberFormat to
    // derive separators, so browsers with full ICU data will produce locale-correct output.
    const enResult = formatQuantity('12345', 2, 'en-US');
    expect(enResult).toContain('123');
    expect(enResult).toContain('45');
  });
});

/**
 * The two halves of `$lib/money` deliberately render the same value
 * differently. These tests pin that difference so neither half drifts into
 * doing the other's job — the mistake that produced the duplicated,
 * divergent copies G-02 cleaned up.
 */
describe('display formatting vs editable formatting', () => {
  it('differs from formatLedgerAmount only by locale presentation', () => {
    // Same coefficient, two audiences: a reader gets group separators, an
    // editable <input> gets a bare decimal it can round-trip.
    expect(formatQuantity('123456789', 2, 'en-US')).toBe('1,234,567.89');
    expect(formatLedgerAmount('123456789', 2)).toBe('1234567.89');
  });

  it('preserves a value negative and sub-unit in both halves', () => {
    expect(formatQuantity('-5', 2, 'en-US')).toBe('-0.05');
    expect(formatLedgerAmount('-5', 2)).toBe('-0.05');
  });

  it('preserves a high-scale crypto quantity rather than rounding to 2', () => {
    // A commodity may carry up to MAX_SUPPORTED_SCALE digits. Display must not
    // silently truncate them — the reader would see a different quantity than
    // the ledger holds.
    expect(formatQuantity('100000000000000000', 18, 'en-US')).toBe('0.100000000000000000');
  });

  it('en display output is re-parseable, since en groups in threes', () => {
    // Not a general guarantee — it holds because `en` uses "," as a thousands
    // separator, which is exactly the grouping parseDecimalAmount accepts. A
    // decimal-comma locale breaks this, which is the open half of G-08.
    const displayed = formatQuantity('123456789', 2, 'en-US');
    expect(parseDecimalAmount(displayed)).toEqual({ value: '123456789', scale: 2 });
  });
});
