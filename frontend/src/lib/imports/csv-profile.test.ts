import { describe, expect, it } from 'vitest';
import { parseCSVHeader } from './csv-profile';

describe('parseCSVHeader', () => {
  it('detects an EU semicolon layout without splitting a quoted delimiter', () => {
    expect(parseCSVHeader('\uFEFFDatum;Omschrijving;"Bedrag; EUR"\n28/08/2026;Koffie;-3,50')).toEqual({
      delimiter: 'semicolon',
      headers: ['Datum', 'Omschrijving', 'Bedrag; EUR']
    });
  });

  it('detects a comma layout with escaped quotes', () => {
    expect(parseCSVHeader('Date,"Payee ""name""",Amount\n2026-08-28,Cafe,-3.50')).toEqual({
      delimiter: 'comma',
      headers: ['Date', 'Payee "name"', 'Amount']
    });
  });

  it('rejects duplicate headers because mappings would be ambiguous', () => {
    expect(() => parseCSVHeader('Date,Amount,Amount')).toThrow('invalid CSV header');
  });
});
