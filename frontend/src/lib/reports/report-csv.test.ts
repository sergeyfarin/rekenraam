import { describe, expect, it } from 'vitest';
import { csvField, csvFilename, exactDecimal, toCSV } from './report-csv';

describe('exactDecimal', () => {
  it('places the decimal point from the scale', () => {
    expect(exactDecimal('200000', 2)).toBe('2000.00');
    expect(exactDecimal('-5000', 2)).toBe('-50.00');
    expect(exactDecimal('0', 2)).toBe('0.00');
  });

  it('pads a coefficient shorter than its scale', () => {
    expect(exactDecimal('5', 4)).toBe('0.0005');
    expect(exactDecimal('-5', 4)).toBe('-0.0005');
  });

  it('emits an integer at scale zero', () => {
    expect(exactDecimal('123', 0)).toBe('123');
  });

  it('keeps full precision on a coefficient no float could hold', () => {
    expect(exactDecimal('12345678901234567890123456789012345678', 18)).toBe(
      '12345678901234567890.123456789012345678'
    );
  });

  it('never writes grouping or a locale decimal separator', () => {
    // The whole point: a spreadsheet must parse this, and "1.234,56" would not.
    expect(exactDecimal('123456', 2)).toBe('1234.56');
  });

  it('passes through a value it cannot read rather than guessing', () => {
    expect(exactDecimal('not-a-number', 2)).toBe('not-a-number');
  });
});

describe('csvField', () => {
  it('leaves an ordinary field unquoted', () => {
    expect(csvField('Groceries')).toBe('Groceries');
  });

  it('quotes and escapes what would otherwise break the row', () => {
    expect(csvField('Smith, Jones')).toBe('"Smith, Jones"');
    expect(csvField('The "Grocer"')).toBe('"The ""Grocer"""');
    expect(csvField('two\nlines')).toBe('"two\nlines"');
  });
});

describe('toCSV', () => {
  it('joins rows with CRLF as the format requires', () => {
    expect(toCSV([['a', 'b'], ['1', '2']])).toBe('a,b\r\n1,2');
  });

  it('round-trips a field containing a separator', () => {
    const csv = toCSV([['Payee', 'Amount'], ['Smith, Jones', '50.00']]);
    expect(csv).toBe('Payee,Amount\r\n"Smith, Jones",50.00');
  });
});

describe('csvFilename', () => {
  it('names the report and its range', () => {
    expect(csvFilename('spending', '2026-06-01', '2026-06-30')).toBe(
      'rekenraam-spending-2026-06-01-2026-06-30.csv'
    );
  });
});
