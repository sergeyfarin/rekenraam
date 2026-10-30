import { describe, it, expect } from 'vitest';
import { formatExactMoney, formatHomeMoney, formatQuantity, formatMoney, formatRatio, joinCommodityAmount } from './format';
import { divideAmountsForDisplay, formatLedgerAmount, parseDecimalAmount } from './amount';

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
 * The two halves of `#lib/money` deliberately render the same value
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
    // Read-only grouping is accepted by the parser for the same locale.
    // The six-locale matrix is covered by locale-input.test.ts (#100).
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

describe('formatExactMoney', () => {
	it('drops replay scale padding down to the currency standard scale', () => {
		expect(formatExactMoney('50000000', 6, 2, 'en-US')).toBe('50.00');
		expect(formatExactMoney('-20500000', 6, 2, 'en-US')).toBe('-20.50');
	});

	it('never rounds away a significant digit', () => {
		expect(formatExactMoney('50004000', 6, 2, 'en-US')).toBe('50.004');
	});

	it('pads a shallower value up to the standard scale and keeps zero plain', () => {
		expect(formatExactMoney('5', 0, 2, 'en-US')).toBe('5.00');
		expect(formatExactMoney('0', 6, 2, 'en-US')).toBe('0.00');
	});
});

describe('divideAmountsForDisplay (#106)', () => {
  it('rounds half-up, ties away from zero, like the backend scaledDivision', () => {
    // 2 / 3 = 0.666666666… → 0.66666667 at 8 decimals (truncation gave 0.66666666).
    expect(divideAmountsForDisplay({ value: '2', scale: 0 }, { value: '3', scale: 0 }, 8)).toBe('66666667');
    // 1 / 8 = 0.125 → tie at 2 decimals rounds away from zero.
    expect(divideAmountsForDisplay({ value: '1', scale: 0 }, { value: '8', scale: 0 }, 2)).toBe('13');
    expect(divideAmountsForDisplay({ value: '-1', scale: 0 }, { value: '8', scale: 0 }, 2)).toBe('-13');
    expect(divideAmountsForDisplay({ value: '1', scale: 0 }, { value: '-8', scale: 0 }, 2)).toBe('-13');
  });

  it('handles a numerator scale deeper than the result scale', () => {
    // 1.23456789012 / 1 at 8 decimals → 1.23456789
    expect(divideAmountsForDisplay({ value: '123456789012', scale: 11 }, { value: '1', scale: 0 }, 8)).toBe('123456789');
  });

  it('stays exact beyond Number.MAX_SAFE_INTEGER', () => {
    expect(
      divideAmountsForDisplay({ value: '123456789012345678901234567890', scale: 2 }, { value: '10', scale: 1 }, 2)
    ).toBe('123456789012345678901234567890');
  });

  it('returns null for a zero denominator', () => {
    expect(divideAmountsForDisplay({ value: '5', scale: 0 }, { value: '0', scale: 4 }, 8)).toBeNull();
  });
});

describe('formatRatio (#106)', () => {
  it('formats an FX rate stored as price per quantity, dropping trailing zeros', () => {
    // 108.50 USD per 100 EUR → 1.085
    expect(formatRatio({ value: '10850', scale: 2 }, { value: '100', scale: 0 }, 8, 'en-US')).toBe('1.085');
    expect(formatRatio({ value: '10850', scale: 2 }, { value: '100', scale: 0 }, 8, 'de-DE')).toBe('1,085');
  });

  it('rounds the 8th decimal half-up instead of truncating', () => {
    expect(formatRatio({ value: '2', scale: 0 }, { value: '3', scale: 0 }, 8, 'en-US')).toBe('0.66666667');
  });

  it('shows whole rates without a decimal separator and groups large ones', () => {
    expect(formatRatio({ value: '1600000', scale: 2 }, { value: '1', scale: 0 }, 8, 'en-US')).toBe('16,000');
    expect(formatRatio({ value: '0', scale: 2 }, { value: '1', scale: 0 }, 8, 'en-US')).toBe('0');
  });

  it('keeps exact digits for a rate beyond double precision', () => {
    expect(formatRatio({ value: '900719925474099312345678', scale: 8 }, { value: '1', scale: 0 }, 8, 'en-US')).toBe(
      '9,007,199,254,740,993.12345678'
    );
  });

  it('shows an em dash when the quantity is zero', () => {
    expect(formatRatio({ value: '5', scale: 0 }, { value: '0', scale: 0 }, 8, 'en-US')).toBe('\u2014');
  });
});

describe('hide amounts (#456)', () => {
  it('masks every exact display path, including quantities and rates, in every locale', async () => {
    const { amountPrivacy } = await import('./privacy.svelte.ts');
    const { maskFinancialText } = await import('./format');
    amountPrivacy.hidden = true;
    try {
      for (const locale of ['en', 'nl', 'de', 'es', 'fr', 'ru']) {
        for (const value of ['0', '12345678901234567890', '-12345678901234567890']) {
          expect(formatQuantity(value, 4, locale)).toBe('••••');
          expect(formatMoney(value, 4, 2, locale)).toBe('••••');
          expect(formatExactMoney(value, 4, 2, locale)).toBe('••••');
          expect(formatRatio({ value, scale: 4 }, { value: '2', scale: 0 }, 8, locale)).toBe('••••');
        }
        expect(formatQuantity('', 2, locale)).toBe('');
        expect(formatRatio({ value: '1', scale: 0 }, { value: '0', scale: 0 }, 8, locale)).toBe('—');
        expect(maskFinancialText('unparsed 12.34')).toBe('••••');
        expect(formatLedgerAmount('12345', 2)).toBe('123.45');
        expect(parseDecimalAmount('123.45', { locale: 'en' })?.value).toBe('12345');
      }
    } finally { amountPrivacy.hidden = false; }
    expect(formatQuantity('12345', 2, 'en')).toBe('123.45');
  });
});

describe('Home rounding (#476)', () => {
  it.each([
    ['5675000', 2, '56,750'],
    ['61249', 2, '612'],
    ['61250', 2, '613'],
    ['-61250', 2, '-613'],
    ['10000', 2, '100'],
    ['9640', 2, '96.40'],
    ['9999', 2, '99.99'],
    ['99995', 3, '100'],
    ['-9640', 2, '-96.40'],
    ['0', 2, '0.00']
  ])('%s at scale %i shows %s', (value, scale, expected) => {
    expect(formatHomeMoney(value, scale, 2, 'en-US')).toBe(expected);
  });

  it.each([
    ['en', '56,750', '96.40'],
    ['de', '56.750', '96,40'],
    ['nl', '56.750', '96,40'],
    ['fr', '56 750', '96,40'],
    ['es', '56.750', '96,40'],
    ['ru', '56 750', '96,40']
  ])('follows %s separators', (locale, large, small) => {
    expect(formatHomeMoney('5675000', 2, 2, locale)).toBe(large);
    expect(formatHomeMoney('9640', 2, 2, locale)).toBe(small);
  });

  it('keeps a zero-scale currency whole', () => {
    expect(formatHomeMoney('1234', 0, 0, 'en-US')).toBe('1,234');
    expect(formatHomeMoney('96', 0, 0, 'en-US')).toBe('96');
  });
});
