import { describe, expect, it } from 'vitest';
import { parseCSVHeader, rankCSVProfiles, uniqueCSVProfileSuggestion } from './csv-profile';

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

describe('CSV profile suggestions', () => {
  const profile = (id: number, config: object) => ({ id, config: JSON.stringify(config) });
  const base = {
    delimiter: 'semicolon', date_column: 'Datum', amount_column: 'Bedrag',
    date_layout: 'DMY', decimal_separator: ',', invert_amount: false
  };

  it('ranks an exact header and normalized filename match first', () => {
    const ranked = rankCSVProfiles([
      profile(1, base),
      profile(2, { ...base, headers: ['Datum', 'Bedrag'], source_filename: 'statement-2026-07.csv' })
    ], 'statement-2026-08.csv', 'semicolon', ['Datum', 'Bedrag']);
    expect(ranked.map((entry) => [entry.profile.id, entry.score])).toEqual([[2, 90], [1, 20]]);
    expect(uniqueCSVProfileSuggestion(ranked)?.id).toBe(2);
  });

  it('does not auto-select between equally compatible mappings', () => {
    const ranked = rankCSVProfiles([profile(1, base), profile(2, base)], 'statement.csv', 'semicolon', ['Datum', 'Bedrag']);
    expect(uniqueCSVProfileSuggestion(ranked)).toBeNull();
  });

  it('keeps profiles created before matching hints and invert_amount were stored compatible', () => {
    const legacy = { ...base } as Partial<typeof base>;
    delete legacy.invert_amount;
    const ranked = rankCSVProfiles([profile(1, legacy)], 'statement.csv', 'semicolon', ['Datum', 'Bedrag']);
    expect(uniqueCSVProfileSuggestion(ranked)?.id).toBe(1);
  });

  it('excludes a profile whose mapped columns are missing', () => {
    expect(rankCSVProfiles([profile(1, base)], 'statement.csv', 'semicolon', ['Date', 'Amount'])).toEqual([]);
  });
});
