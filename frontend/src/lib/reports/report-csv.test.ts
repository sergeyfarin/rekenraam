import { describe, expect, it } from 'vitest';
import { csvField, csvFilename, exactDecimal, toCSV, withReportContext } from './report-csv';

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

  // exactDecimal delegates digit-shifting to formatLedgerAmount, which trusts
  // its input to be a signed digit string and does not validate it. The guard
  // in front of it is this module's own, so these pin what counts as readable.
  // A coefficient the backend could not have produced must survive to the cell
  // unchanged: an unreadable cell is recoverable, a silently wrong number in an
  // exported financial file is not.
  it.each(['', ' 12', '12 ', '+5', '1.5', '1,50', '12e3', '-'])(
    'passes through %o rather than coercing it to a number',
    (value) => {
      expect(exactDecimal(value, 2)).toBe(value);
    }
  );

  it('reads a plain signed digit string', () => {
    expect(exactDecimal('-12345', 2)).toBe('-123.45');
    expect(exactDecimal('-5', 2)).toBe('-0.05');
  });

  // A non-positive or non-integer scale is normalised to 0 rather than
  // throwing or shifting by a fraction of a digit.
  it('treats a scale that is not a positive integer as zero', () => {
    expect(exactDecimal('12345', 0)).toBe('12345');
    expect(exactDecimal('12345', -1)).toBe('12345');
    expect(exactDecimal('12345', 1.5)).toBe('12345');
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

describe('withReportContext', () => {
  const table = [
    ['Period', 'Amount'],
    ['2026-06-01', '100.00']
  ];

  it('keeps the header row first, so a naive reader still parses the table', () => {
    const rows = withReportContext(table, ['Period: June 2026', 'Excluded: commodity_trading']);
    expect(rows[0]).toEqual(['Period', 'Amount']);
    expect(rows[1]).toEqual(['2026-06-01', '100.00']);
  });

  it('separates the context from the table with a blank row', () => {
    const rows = withReportContext(table, ['Period: June 2026']);
    expect(rows[2]).toEqual(['', '']);
    expect(rows[3]).toEqual(['Period: June 2026', '']);
  });

  it('keeps the file rectangular, so a strict reader does not reject it', () => {
    const wide = [
      ['A', 'B', 'C'],
      ['1', '2', '3']
    ];
    const rows = withReportContext(wide, ['Period: June 2026', 'Excluded: none']);
    for (const row of rows) {
      expect(row).toHaveLength(3);
    }
  });

  it('drops empty lines rather than emitting blank context rows', () => {
    const rows = withReportContext(table, ['Period: June 2026', '', '   ']);
    expect(rows).toHaveLength(4);
  });

  it('returns the table untouched when there is no context to add', () => {
    expect(withReportContext(table, [])).toEqual(table);
  });

  it('survives the CSV encoder with its quoting intact', () => {
    const csv = toCSV(withReportContext(table, ['Accounts: Smith, Jones & Co']));
    expect(csv.split('\r\n')[0]).toBe('Period,Amount');
    expect(csv).toContain('"Accounts: Smith, Jones & Co",');
  });
});
