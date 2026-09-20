import { describe, it, expect } from 'vitest';
import { formatQuantity, formatMoney, joinCommodityAmount } from './format';
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

  it.each(['nl-NL', 'de-DE'])('uses the decimal comma in %s', (locale) => {
    expect(formatQuantity('123456', 2, locale)).toBe('1.234,56');
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

describe('joinCommodityAmount', () => {
	it('runs a punctuation symbol straight into its amount', () => {
		expect(joinCommodityAmount('$', '42.00')).toBe('$42.00');
		expect(joinCommodityAmount('€', '42.00')).toBe('€42.00');
		expect(joinCommodityAmount('£', '-7.50')).toBe('£-7.50');
	});

	it('separates a label that ends in a letter or digit', () => {
		// The T-62 case: "AAPL2.000" and "USD42.00" read as different numbers.
		expect(joinCommodityAmount('AAPL', '2.000')).toBe('AAPL 2.000');
		expect(joinCommodityAmount('USD', '42.00')).toBe('USD 42.00');
		expect(joinCommodityAmount('IWDA9', '1.5')).toBe('IWDA9 1.5');
	});

	it('treats non-Latin letters as letters', () => {
		expect(joinCommodityAmount('руб', '10,00')).toBe('руб 10,00');
		expect(joinCommodityAmount('₽', '10,00')).toBe('₽10,00');
	});

	it('returns the amount alone when there is no label', () => {
		expect(joinCommodityAmount('', '42.00')).toBe('42.00');
		expect(joinCommodityAmount('   ', '42.00')).toBe('42.00');
	});

	it('ignores surrounding whitespace in the label', () => {
		expect(joinCommodityAmount(' USD ', '42.00')).toBe('USD 42.00');
		expect(joinCommodityAmount(' $ ', '42.00')).toBe('$42.00');
	});
});


describe('currency summary display', () => {
  it.each([
    ['1666667', 6, 2, '1.67'],
    ['-1666667', 6, 2, '-1.67'],
    ['1005', 3, 2, '1.01'],
    ['-1005', 3, 2, '-1.01'],
    ['-4', 3, 2, '0.00'],
    ['9995', 3, 2, '10.00'],
    ['1235', 1, 0, '124'],
    ['1234567', 6, 3, '1.235'],
    ['10', 0, 3, '10.000'],
    ['90071992547409935', 3, 2, '90,071,992,547,409.94']
  ])('rounds %s at scale %i to %i places', (value, scale, standardScale, expected) => {
    expect(formatMoney(value, scale, standardScale, 'en-US')).toBe(expected);
  });

  it('formats currency precision with locale separators', () => {
    expect(formatMoney('1234567890', 6, 2, 'nl-NL')).toBe('1.234,57');
  });

  it('keeps the exact quantity formatter available for shares and details', () => {
    expect(formatQuantity('1666667', 6, 'en-US')).toBe('1.666667');
    expect(formatMoney('1666667', 6, 2, 'en-US')).toBe('1.67');
  });
});
